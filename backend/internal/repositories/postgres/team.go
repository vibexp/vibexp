package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// TeamRepository implements the repositories.TeamRepository interface for PostgreSQL
type TeamRepository struct {
	db *database.DB
}

// NewTeamRepository creates a new TeamRepository
func NewTeamRepository(db *database.DB) repositories.TeamRepository {
	return &TeamRepository{
		db: db,
	}
}

// Create creates a new team
func (r *TeamRepository) Create(ctx context.Context, team *models.Team) error {
	query := `
		INSERT INTO teams (owner_id, name, slug, description, is_personal, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, is_personal, created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		team.OwnerID, team.Name, team.Slug, team.Description, team.IsPersonal,
		team.CreatedAt, team.UpdatedAt,
	).Scan(&team.ID, &team.IsPersonal, &team.CreatedAt, &team.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create team: %w", err)
	}

	return nil
}

// GetByID retrieves a team by its ID
func (r *TeamRepository) GetByID(ctx context.Context, teamID string) (*models.Team, error) {
	query := `
		SELECT id, owner_id, name, slug, description, is_personal, created_at, updated_at
		FROM teams WHERE id = $1
	`

	var team models.Team
	err := r.db.QueryRowContext(ctx, query, teamID).Scan(
		&team.ID, &team.OwnerID, &team.Name, &team.Slug,
		&team.Description, &team.IsPersonal, &team.CreatedAt, &team.UpdatedAt,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get team by ID: %w", err), repositories.ErrTeamNotFound)
	}

	return &team, nil
}

// GetByOwnerID retrieves the first team owned by a user
func (r *TeamRepository) GetByOwnerID(ctx context.Context, ownerID string) (*models.Team, error) {
	query := `
		SELECT id, owner_id, name, slug, description, is_personal, created_at, updated_at
		FROM teams WHERE owner_id = $1
		ORDER BY created_at ASC
		LIMIT 1
	`

	var team models.Team
	err := r.db.QueryRowContext(ctx, query, ownerID).Scan(
		&team.ID, &team.OwnerID, &team.Name, &team.Slug,
		&team.Description, &team.IsPersonal, &team.CreatedAt, &team.UpdatedAt,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get team by owner ID: %w", err), repositories.ErrTeamNotFound)
	}

	return &team, nil
}

// GetByOwnerAndSlug retrieves a team by owner ID and slug
func (r *TeamRepository) GetByOwnerAndSlug(ctx context.Context, ownerID, slug string) (*models.Team, error) {
	query := `
		SELECT id, owner_id, name, slug, description, is_personal, created_at, updated_at
		FROM teams WHERE owner_id = $1 AND slug = $2
	`

	var team models.Team
	err := r.db.QueryRowContext(ctx, query, ownerID, slug).Scan(
		&team.ID, &team.OwnerID, &team.Name, &team.Slug,
		&team.Description, &team.IsPersonal, &team.CreatedAt, &team.UpdatedAt,
	)

	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get team by owner and slug: %w", err), repositories.ErrTeamNotFound)
	}

	return &team, nil
}

// TransferOwnership moves ownership of a team from fromUserID to toUserID.
//
// All three writes — teams.owner_id, the new owner's role, and the old owner's
// demotion to admin — happen in one transaction, because a team must always have
// exactly one owner: a partial apply would either strand the team with no owner
// or leave owner_id disagreeing with team_members.role, which is the authority
// the authz layer reads.
//
// It is the caller's job to authorize the transfer and to validate that both
// users are members; this method enforces only what the data can prove:
// ErrTeamNotFound when fromUserID does not currently own the team (which also
// makes a concurrent double-transfer a clean no-op rather than a lost update),
// and ErrTeamMemberNotFound when either membership row is missing.
func (r *TeamRepository) TransferOwnership(ctx context.Context, teamID, fromUserID, toUserID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			slog.Error("Failed to rollback ownership transfer transaction", "error", rollbackErr)
		}
	}()

	now := time.Now()

	// Guarding on owner_id makes this the concurrency control point: two racing
	// transfers cannot both win, the loser sees ErrTeamNotFound.
	res, err := tx.ExecContext(ctx, `
		UPDATE teams
		SET owner_id = $1, updated_at = $2
		WHERE id = $3 AND owner_id = $4
	`, toUserID, now, teamID, fromUserID)
	if err != nil {
		return fmt.Errorf("failed to update team owner: %w", err)
	}
	if rowErr := expectOneRow(res, repositories.ErrTeamNotFound); rowErr != nil {
		return rowErr
	}

	res, err = tx.ExecContext(ctx, `
		UPDATE team_members
		SET role = $1, updated_at = $2
		WHERE team_id = $3 AND user_id = $4
	`, models.TeamMemberRoleOwner, now, teamID, toUserID)
	if err != nil {
		return fmt.Errorf("failed to promote new owner: %w", err)
	}
	if rowErr := expectOneRow(res, repositories.ErrTeamMemberNotFound); rowErr != nil {
		return rowErr
	}

	res, err = tx.ExecContext(ctx, `
		UPDATE team_members
		SET role = $1, updated_at = $2
		WHERE team_id = $3 AND user_id = $4
	`, models.TeamMemberRoleAdmin, now, teamID, fromUserID)
	if err != nil {
		return fmt.Errorf("failed to demote previous owner: %w", err)
	}
	if rowErr := expectOneRow(res, repositories.ErrTeamMemberNotFound); rowErr != nil {
		return rowErr
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return fmt.Errorf("failed to commit ownership transfer: %w", commitErr)
	}

	return nil
}

// expectOneRow turns a zero-row UPDATE into notFound, so a transfer step that
// silently matched nothing aborts the transaction instead of committing a
// half-transferred team.
func expectOneRow(res sql.Result, notFound error) error {
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return notFound
	}
	return nil
}

// Update updates an existing team
func (r *TeamRepository) Update(ctx context.Context, team *models.Team) error {
	query := `
		UPDATE teams
		SET name = $1, slug = $2, description = $3, updated_at = $4
		WHERE id = $5 AND owner_id = $6
		RETURNING updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		team.Name, team.Slug, team.Description, team.UpdatedAt,
		team.ID, team.OwnerID,
	).Scan(&team.UpdatedAt)

	if err != nil {
		return mapNoRows(fmt.Errorf("failed to update team: %w", err), repositories.ErrTeamNotFound)
	}

	return nil
}

// Delete deletes a team by owner ID and team ID
func (r *TeamRepository) Delete(ctx context.Context, ownerID, teamID string) error {
	query := `DELETE FROM teams WHERE id = $1 AND owner_id = $2`

	result, err := r.db.ExecContext(ctx, query, teamID, ownerID)
	if err != nil {
		return fmt.Errorf("failed to delete team: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrTeamNotFound
	}

	return nil
}

// ListByOwnerID retrieves all teams owned by a user with pagination
func (r *TeamRepository) ListByOwnerID(
	ctx context.Context, ownerID string, limit, offset int,
) ([]models.Team, int, error) {
	// Get total count
	countQuery := `SELECT COUNT(*) FROM teams WHERE owner_id = $1`
	var totalCount int
	err := r.db.QueryRowContext(ctx, countQuery, ownerID).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count teams: %w", err)
	}

	// Get teams with pagination
	query := `
		SELECT id, owner_id, name, slug, description, is_personal, created_at, updated_at
		FROM teams
		WHERE owner_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, ownerID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list teams: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var teams []models.Team
	for rows.Next() {
		var team models.Team
		err := rows.Scan(
			&team.ID, &team.OwnerID, &team.Name, &team.Slug,
			&team.Description, &team.IsPersonal, &team.CreatedAt, &team.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan team: %w", err)
		}
		teams = append(teams, team)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating teams: %w", err)
	}

	return teams, totalCount, nil
}

// ListByUserID retrieves all teams where user is owner OR member with pagination
// Uses EXISTS subqueries to avoid Cartesian product with multi-member teams
func (r *TeamRepository) ListByUserID(
	ctx context.Context, userID string, limit, offset int,
) ([]models.Team, int, error) {
	// Get total count - teams where user is owner OR member
	// No DISTINCT needed since EXISTS subqueries eliminate duplicates
	countQuery := `
		SELECT COUNT(*)
		FROM teams t
		WHERE t.owner_id = $1
			OR EXISTS (SELECT 1 FROM team_members WHERE team_id = t.id AND user_id = $1)
	`
	var totalCount int
	err := r.db.QueryRowContext(ctx, countQuery, userID).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count teams: %w", err)
	}

	// Get teams with pagination - include teams where user is owner OR member
	// No DISTINCT needed since EXISTS subqueries eliminate duplicates
	query := `
		SELECT t.id, t.owner_id, t.name, t.slug, t.description, t.is_personal, t.created_at, t.updated_at
		FROM teams t
		WHERE t.owner_id = $1
			OR EXISTS (SELECT 1 FROM team_members WHERE team_id = t.id AND user_id = $1)
		ORDER BY t.created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list teams: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var teams []models.Team
	for rows.Next() {
		var team models.Team
		err := rows.Scan(
			&team.ID, &team.OwnerID, &team.Name, &team.Slug,
			&team.Description, &team.IsPersonal, &team.CreatedAt, &team.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan team: %w", err)
		}
		teams = append(teams, team)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating teams: %w", err)
	}

	return teams, totalCount, nil
}

// resolveTeamByIdentifierQuery resolves a team UUID or slug under an
// owner-OR-member predicate. Package-level so the integration suite can EXPLAIN
// the exact statement production runs (see ResolveByIdentifier for why each
// binding is shaped the way it is).
const resolveTeamByIdentifierQuery = `
	SELECT t.id, t.owner_id, t.name, t.slug, t.description, t.is_personal, t.created_at, t.updated_at
	FROM teams t
	WHERE (($3::uuid IS NOT NULL AND t.id = $3::uuid) OR t.slug = $2)
		AND (t.owner_id = $1
			OR EXISTS (SELECT 1 FROM team_members WHERE team_id = t.id AND user_id = $1))
	LIMIT 1
`

// uuidOrNil returns identifier as a bindable uuid parameter when it is
// UUID-shaped, and nil otherwise. Binding nil (rather than the raw string) is
// what lets a single statement carry an indexable `t.id = $3::uuid` arm that the
// planner prunes entirely for slug lookups.
//
// It binds the CANONICAL form, not the input: uuid.Parse also accepts
// `urn:uuid:...`, `{...}` and the hyphenless form, and Postgres rejects the first
// of those outright (22P02). Passing the input through would turn an identifier
// that is merely unusual into an infrastructure error — same generic message to
// the caller, but logged as a failure rather than a rejection.
func uuidOrNil(identifier string) interface{} {
	parsed, err := uuid.Parse(identifier)
	if err != nil {
		return nil
	}
	return parsed.String()
}

// ResolveByIdentifier resolves a team UUID or slug to the team in a single
// query, enforcing owner-OR-member access in the SQL rather than post-filtering
// in Go. Returns repositories.ErrTeamNotFound when nothing matches, whether the
// team does not exist or the user simply does not belong to it — callers rely on
// that indistinguishability for anti-enumeration.
func (r *TeamRepository) ResolveByIdentifier(
	ctx context.Context, userID, identifier string,
) (*models.Team, error) {
	// The identifier is bound TWICE, and both bindings are load-bearing:
	//
	//   $2 (text) serves the slug arm. It cannot also serve the id arm —
	//   `t.id = $2 OR t.slug = $2` asks Postgres to infer one type for two
	//   incompatible comparisons and fails for EVERY identifier, UUID ones
	//   included (measured 42883, "operator does not exist: character varying =
	//   uuid", since lib/pq sends the argument as text).
	//
	//   $3 (uuid, NULL unless the identifier is UUID-shaped) serves the id arm.
	//   Casting the COLUMN instead (`t.id::text = $2`) fixes the type error but
	//   matches no index, and an OR with one unindexable arm seq-scans the whole
	//   table — measured 10.09ms over 50k teams versus 0.08ms here, plus a seq
	//   scan of team_members as the correlated EXISTS degrades with it. Comparing
	//   the column bare against a typed parameter keeps teams_pkey reachable:
	//   a UUID identifier plans BitmapOr(teams_pkey, idx_teams_slug), and a slug
	//   identifier prunes the NULL arm to a plain index scan on idx_teams_slug.
	//
	// Neither property is visible to sqlmock (it never type-checks arguments and
	// never plans anything) — the integration suite proves both, which is why the
	// statement is a package-level const it can EXPLAIN directly rather than a
	// copy that would drift from this one.
	var team models.Team
	err := r.db.QueryRowContext(
		ctx, resolveTeamByIdentifierQuery, userID, identifier, uuidOrNil(identifier),
	).Scan(
		&team.ID, &team.OwnerID, &team.Name, &team.Slug,
		&team.Description, &team.IsPersonal, &team.CreatedAt, &team.UpdatedAt,
	)
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to resolve team by identifier: %w", err),
			repositories.ErrTeamNotFound,
		)
	}

	return &team, nil
}

// CountByOwnerID counts all teams owned by a user
func (r *TeamRepository) CountByOwnerID(ctx context.Context, ownerID string) (int, error) {
	var count int
	query := "SELECT COUNT(*) FROM teams WHERE owner_id = $1"
	err := r.db.QueryRowContext(ctx, query, ownerID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count teams: %w", err)
	}
	return count, nil
}

// GetTeamStats returns team-wide resource counts (projects, prompts, artifacts,
// blueprints, memories, feed_items) for the given team. Team access is authorized
// by the caller (the handler validates membership) so this counts purely by
// team_id; a team with no resources yields all-zero counts rather than an error.
func (r *TeamRepository) GetTeamStats(ctx context.Context, teamID string) (*models.TeamStatsResponse, error) {
	const query = `
		SELECT
			(SELECT COUNT(*) FROM projects   WHERE team_id = $1),
			(SELECT COUNT(*) FROM prompts    WHERE team_id = $1),
			(SELECT COUNT(*) FROM artifacts  WHERE team_id = $1),
			(SELECT COUNT(*) FROM blueprints WHERE team_id = $1),
			(SELECT COUNT(*) FROM memories   WHERE team_id = $1),
			(SELECT COUNT(*) FROM feed_items WHERE team_id = $1)`

	var stats models.TeamStatsResponse
	err := r.db.QueryRowContext(ctx, query, teamID).Scan(
		&stats.TotalProjects,
		&stats.TotalPrompts,
		&stats.TotalArtifacts,
		&stats.TotalBlueprints,
		&stats.TotalMemories,
		&stats.TotalFeedItems,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get team stats: %w", err)
	}

	return &stats, nil
}

// GetTeamResourceCreationMetrics returns sparse per-day creation counts per
// resource type (prompts, artifacts, blueprints, memories, projects) for the
// given team, counting rows created at or after `since`. Days with no creations
// for a type are omitted — the caller zero-fills them into a continuous daily
// series. Team access is authorized by the caller; this aggregates purely by
// team_id.
func (r *TeamRepository) GetTeamResourceCreationMetrics(
	ctx context.Context, teamID string, since time.Time,
) ([]models.TeamResourceCreationCount, error) {
	// The TO_CHAR keys must match the zero-fill series keys the handler builds,
	// which are UTC calendar days -- a key the handler did not generate is
	// dropped by the map lookup, silently, with no error and no log (#773).
	//
	// So the bucket has to be an explicitly UTC day, and the expression differs
	// by column family. prompts/artifacts/blueprints/projects are TIMESTAMPTZ:
	// `AT TIME ZONE 'UTC'` CONVERTS them, replacing the session timezone that a
	// bare DATE() would otherwise cast through. memories.created_at is a plain
	// TIMESTAMP, where the same operator does the OPPOSITE -- it assumes the
	// value is UTC and produces an aware one -- so that branch keeps a bare
	// DATE(), which is already session-independent. Same rule as
	// admin_dashboard.go, which documents the overload in full.
	//
	// Range predicates stay on the raw column so they remain index-eligible.
	// The memories branch needs its OWN placeholder, though: $2 is also bound to
	// TIMESTAMPTZ columns in the other branches, so Postgres infers it as
	// timestamptz -- and comparing the naive memories.created_at against a
	// timestamptz converts the COLUMN through the session timezone, dropping
	// every memory within the session's offset of `since`. That is the same
	// defect as the bucket, on the window edge, and casting the shared $2 does
	// not fix it (the cast then happens in the session zone too). A separate
	// $3::timestamp, bound to the same instant in UTC, is what makes the
	// comparison naive-to-naive. Verified against real Postgres under
	// Pacific/Auckland; see analytics_timezone_integration_test.go.
	const query = `
		SELECT TO_CHAR((created_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD') AS date,
		       'prompts' AS resource_type, COUNT(*) AS count
		FROM prompts WHERE team_id = $1 AND created_at >= $2
		GROUP BY (created_at AT TIME ZONE 'UTC')::date
		UNION ALL
		SELECT TO_CHAR((created_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD'), 'artifacts', COUNT(*)
		FROM artifacts WHERE team_id = $1 AND created_at >= $2
		GROUP BY (created_at AT TIME ZONE 'UTC')::date
		UNION ALL
		SELECT TO_CHAR((created_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD'), 'blueprints', COUNT(*)
		FROM blueprints WHERE team_id = $1 AND created_at >= $2
		GROUP BY (created_at AT TIME ZONE 'UTC')::date
		UNION ALL
		SELECT TO_CHAR(DATE(created_at), 'YYYY-MM-DD'), 'memories', COUNT(*)
		FROM memories WHERE team_id = $1 AND created_at >= $3::timestamp GROUP BY DATE(created_at)
		UNION ALL
		SELECT TO_CHAR((created_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD'), 'projects', COUNT(*)
		FROM projects WHERE team_id = $1 AND created_at >= $2
		GROUP BY (created_at AT TIME ZONE 'UTC')::date
		ORDER BY date, resource_type`

	rows, err := r.db.QueryContext(ctx, query, teamID, since, since.UTC())
	if err != nil {
		return nil, fmt.Errorf("failed to query team resource creation metrics: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	counts := []models.TeamResourceCreationCount{}
	for rows.Next() {
		var c models.TeamResourceCreationCount
		if scanErr := rows.Scan(&c.Date, &c.ResourceType, &c.Count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan team resource creation metric: %w", scanErr)
		}
		counts = append(counts, c)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("failed to iterate team resource creation metrics: %w", rowsErr)
	}

	return counts, nil
}

// GetTeamFeedCreationMetrics returns sparse per-day creation counts for feeds
// (channels) and feed_items (AI updates) belonging to the given team, counting
// rows created at or after `since`. Days with no creations for an entity kind are
// omitted — the caller zero-fills them into a continuous daily series. Team access
// is authorized by the caller; this aggregates purely by team_id.
func (r *TeamRepository) GetTeamFeedCreationMetrics(
	ctx context.Context, teamID string, since time.Time,
) ([]models.TeamFeedCreationCount, error) {
	// feeds.created_at and feed_items.posted_at are both TIMESTAMPTZ, so both are
	// converted to a UTC day before bucketing -- a bare DATE() would cast through
	// the session timezone and produce keys the handler's zero-fill series never
	// generated, which the map lookup then drops silently (#773). TO_CHAR renders
	// the exact YYYY-MM-DD form those keys take.
	const query = `
		SELECT TO_CHAR((created_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD') AS date,
		       'feeds' AS entity_type, COUNT(*) AS count
		FROM feeds WHERE team_id = $1 AND created_at >= $2
		GROUP BY (created_at AT TIME ZONE 'UTC')::date
		UNION ALL
		SELECT TO_CHAR((posted_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD'), 'feed_items', COUNT(*)
		FROM feed_items WHERE team_id = $1 AND posted_at >= $2
		GROUP BY (posted_at AT TIME ZONE 'UTC')::date
		ORDER BY date, entity_type`

	rows, err := r.db.QueryContext(ctx, query, teamID, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query team feed creation metrics: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	counts := []models.TeamFeedCreationCount{}
	for rows.Next() {
		var c models.TeamFeedCreationCount
		if scanErr := rows.Scan(&c.Date, &c.EntityType, &c.Count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan team feed creation metric: %w", scanErr)
		}
		counts = append(counts, c)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("failed to iterate team feed creation metrics: %w", rowsErr)
	}

	return counts, nil
}
