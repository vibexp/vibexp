package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// InstanceAuthSetupRepository implements repositories.InstanceAuthSetupRepository
// for PostgreSQL.
//
// The table's primary key is `id boolean CHECK (id)`, so it holds at most one
// row and statements carry no WHERE clause and no tenancy or role predicate.
type InstanceAuthSetupRepository struct {
	db *database.DB
}

// NewInstanceAuthSetupRepository creates a new InstanceAuthSetupRepository.
func NewInstanceAuthSetupRepository(db *database.DB) repositories.InstanceAuthSetupRepository {
	return &InstanceAuthSetupRepository{db: db}
}

// instanceAuthSetupSubject names the setup state in errors and logs.
const instanceAuthSetupSubject = "instance auth setup"

// setupStateWriteLock serializes setup writers. The table starts empty,
// and SELECT ... FOR UPDATE on an empty singleton locks nothing, so two replicas
// booting together would both mint; the table lock makes the second wait and
// then see the first one's row. It does not block plain reads.
const setupStateWriteLock = `LOCK TABLE instance_auth_setup IN SHARE ROW EXCLUSIVE MODE`

const instanceAuthSetupSelect = `SELECT token_hash, expires_at, consumed_at, consumed_by, rearmed, generation,
		created_at, updated_at
	FROM instance_auth_setup`

// instanceAuthSetupMint stores a fresh token. $3 is whether this mint re-arms
// setup; a boot-time mint passes false and so preserves an earlier re-arm that
// was never consumed.
const instanceAuthSetupMint = `INSERT INTO instance_auth_setup (token_hash, expires_at, rearmed)
	VALUES ($1, $2, $3)
	ON CONFLICT (id)
	DO UPDATE SET
		token_hash = EXCLUDED.token_hash,
		expires_at = EXCLUDED.expires_at,
		consumed_at = NULL,
		consumed_by = NULL,
		rearmed = instance_auth_setup.rearmed OR EXCLUDED.rearmed,
		generation = instance_auth_setup.generation + 1,
		updated_at = CURRENT_TIMESTAMP
	RETURNING token_hash, expires_at, consumed_at, consumed_by, rearmed, generation, created_at, updated_at`

const instanceAuthSetupConsume = `UPDATE instance_auth_setup
	SET token_hash = NULL,
		consumed_at = CURRENT_TIMESTAMP,
		consumed_by = $1,
		rearmed = false,
		generation = generation + 1,
		updated_at = CURRENT_TIMESTAMP
	RETURNING token_hash, expires_at, consumed_at, consumed_by, rearmed, generation, created_at, updated_at`

// Audit snapshot markers: what happened to the setup state, since every change
// is recorded under the one `upsert` action.
const (
	instanceAuthSetupEventKey      = "event"
	instanceAuthSetupEventMinted   = "token_minted"
	instanceAuthSetupEventRearmed  = "rearmed"
	instanceAuthSetupEventConsumed = "consumed"
)

func scanInstanceAuthSetup(row rowScanner) (*models.InstanceAuthSetup, error) {
	var s models.InstanceAuthSetup
	if err := row.Scan(
		&s.TokenHash, &s.ExpiresAt, &s.ConsumedAt, &s.ConsumedBy, &s.Rearmed, &s.Generation,
		&s.CreatedAt, &s.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &s, nil
}

// Get returns the stored setup state, or ErrInstanceAuthSetupNotFound.
func (r *InstanceAuthSetupRepository) Get(ctx context.Context) (*models.InstanceAuthSetup, error) {
	s, err := scanInstanceAuthSetup(r.db.QueryRowContext(ctx, instanceAuthSetupSelect))
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get instance auth setup: %w", err),
			repositories.ErrInstanceAuthSetupNotFound)
	}
	return s, nil
}

// MintIfAbsentOrExpired stores tokenHash unless a token is still exchangeable;
// see the interface.
func (r *InstanceAuthSetupRepository) MintIfAbsentOrExpired(
	ctx context.Context, tokenHash []byte, expiresAt, now time.Time,
) (*models.InstanceAuthSetup, bool, error) {
	var (
		stored *models.InstanceAuthSetup
		minted bool
	)
	err := r.inTx(ctx, "mint", func(tx *sql.Tx, before *models.InstanceAuthSetup) (bool, error) {
		if before != nil && before.HasLiveToken(now) {
			stored = before
			return false, nil
		}
		after, err := r.mint(ctx, tx, tokenHash, expiresAt, false)
		if err != nil {
			return false, err
		}
		stored, minted = after, true
		return true, appendInstanceAuthSetupAudit(ctx, tx, instanceAuthSetupEventMinted, nil, before, after)
	})
	if err != nil {
		return nil, false, err
	}
	return stored, minted, nil
}

// ForceMint replaces any outstanding token and re-arms setup; see the interface.
func (r *InstanceAuthSetupRepository) ForceMint(
	ctx context.Context, tokenHash []byte, expiresAt time.Time,
) (*models.InstanceAuthSetup, error) {
	return r.mintUnconditionally(ctx, "rearm", tokenHash, expiresAt, true, instanceAuthSetupEventRearmed)
}

// MintReplacing replaces any outstanding token without re-arming setup; see the
// interface.
func (r *InstanceAuthSetupRepository) MintReplacing(
	ctx context.Context, tokenHash []byte, expiresAt time.Time,
) (*models.InstanceAuthSetup, error) {
	return r.mintUnconditionally(ctx, "mint", tokenHash, expiresAt, false, instanceAuthSetupEventMinted)
}

// mintUnconditionally stores tokenHash whatever the row holds and audits it as
// event.
func (r *InstanceAuthSetupRepository) mintUnconditionally(
	ctx context.Context, op string, tokenHash []byte, expiresAt time.Time, rearm bool, event string,
) (*models.InstanceAuthSetup, error) {
	var stored *models.InstanceAuthSetup
	err := r.inTx(ctx, op, func(tx *sql.Tx, before *models.InstanceAuthSetup) (bool, error) {
		after, err := r.mint(ctx, tx, tokenHash, expiresAt, rearm)
		if err != nil {
			return false, err
		}
		stored = after
		return true, appendInstanceAuthSetupAudit(ctx, tx, event, nil, before, after)
	})
	if err != nil {
		return nil, err
	}
	return stored, nil
}

// Consume ends setup on behalf of userID; see the interface.
func (r *InstanceAuthSetupRepository) Consume(ctx context.Context, userID string) (bool, error) {
	var consumed bool
	err := r.inTx(ctx, "consume", func(tx *sql.Tx, before *models.InstanceAuthSetup) (bool, error) {
		if before == nil || before.IsConsumed() {
			return false, nil
		}
		after, err := scanInstanceAuthSetup(tx.QueryRowContext(ctx, instanceAuthSetupConsume, userID))
		if err != nil {
			return false, fmt.Errorf("failed to consume instance auth setup: %w", err)
		}
		consumed = true
		return true, appendInstanceAuthSetupAudit(ctx, tx, instanceAuthSetupEventConsumed, &userID, before, after)
	})
	if isFKViolation(err) {
		return false, fmt.Errorf("%w: %w", repositories.ErrUserNotFound, err)
	}
	if err != nil {
		return false, err
	}
	return consumed, nil
}

// mint runs the mint statement inside tx and returns the stored row.
func (r *InstanceAuthSetupRepository) mint(
	ctx context.Context, tx *sql.Tx, tokenHash []byte, expiresAt time.Time, rearm bool,
) (*models.InstanceAuthSetup, error) {
	after, err := scanInstanceAuthSetup(tx.QueryRowContext(ctx, instanceAuthSetupMint, tokenHash, expiresAt, rearm))
	if err != nil {
		return nil, fmt.Errorf("failed to mint instance auth setup token: %w", err)
	}
	return after, nil
}

// inTx runs one audited setup write under the table lock with the current row
// (nil when none); see runAuditedSingletonTx.
func (r *InstanceAuthSetupRepository) inTx(
	ctx context.Context, op string,
	change func(tx *sql.Tx, before *models.InstanceAuthSetup) (bool, error),
) error {
	return runAuditedSingletonTx(ctx, r.db, instanceAuthSetupSubject, op, setupStateWriteLock,
		func(tx *sql.Tx) (*models.InstanceAuthSetup, error) {
			return scanInstanceAuthSetup(tx.QueryRowContext(ctx, instanceAuthSetupSelect))
		}, change)
}

// appendInstanceAuthSetupAudit records one setup change. The token hash is
// json:"-" on the model, so the snapshot is the row without it, plus the event
// that produced it.
func appendInstanceAuthSetupAudit(
	ctx context.Context, tx *sql.Tx, event string, actorUserID *string,
	before, after *models.InstanceAuthSetup,
) error {
	beforeDoc, err := optionalAuditSnapshot(before)
	if err != nil {
		return fmt.Errorf("failed to snapshot the instance auth setup: %w", err)
	}
	afterDoc, err := auditSnapshot(after, map[string]any{instanceAuthSetupEventKey: event})
	if err != nil {
		return fmt.Errorf("failed to snapshot the instance auth setup: %w", err)
	}
	return appendAuthSettingsAudit(ctx, tx, models.InstanceSettingAuthSetup,
		models.InstanceSettingsAuditActionUpsert, actorUserID, beforeDoc, afterDoc)
}
