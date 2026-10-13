package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// userColumns is the canonical user column list used by every SELECT.
// Keep in sync with scanUser below.
const userColumns = `id, google_id, idp_provider, idp_subject, email, email_verified_at, name, avatar_url,
		default_team_id, status, onboarding_completed,
		onboarding_completed_at, created_at, updated_at, version, session_epoch, api_tokens_valid_after`

// userColumnsQualified is userColumns for a query that joins users as u.
const userColumnsQualified = `u.id, u.google_id, u.idp_provider, u.idp_subject, u.email, u.email_verified_at,
		u.name, u.avatar_url, u.default_team_id, u.status, u.onboarding_completed,
		u.onboarding_completed_at, u.created_at, u.updated_at, u.version, u.session_epoch,
		u.api_tokens_valid_after`

// usersEmailUniqueConstraint is the unique constraint on users.email. Create
// writes to user_identities in the same statement, so a unique violation is a
// taken email only when it names this constraint.
const usersEmailUniqueConstraint = "users_email_key"

// UserRepository implements the repositories.UserRepository interface for PostgreSQL
type UserRepository struct {
	db *database.DB
}

// NewUserRepository creates a new UserRepository
func NewUserRepository(db *database.DB) repositories.UserRepository {
	return &UserRepository{
		db: db,
	}
}

// scanUser scans a row matching userColumns into a *models.User.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanUser(row rowScanner) (*models.User, error) {
	var user models.User
	err := row.Scan(
		&user.ID, &user.GoogleID, &user.IDPProvider, &user.IDPSubject,
		&user.Email, &user.EmailVerifiedAt, &user.Name, &user.AvatarURL,
		&user.DefaultTeamID, &user.Status,
		&user.OnboardingCompleted, &user.OnboardingCompletedAt,
		&user.CreatedAt, &user.UpdatedAt, &user.Version, &user.SessionEpoch,
		&user.APITokensValidAfter,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByID retrieves a user by their ID.
// Returns repositories.ErrUserNotFound when no row matches, so callers can distinguish
// a deleted user from a transient database error via errors.Is.
func (r *UserRepository) GetByID(ctx context.Context, userID string) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE id = $1`

	user, err := scanUser(r.db.QueryRowContext(ctx, query, userID))
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get user by ID: %w", err), repositories.ErrUserNotFound)
	}
	return user, nil
}

// GetByEmail retrieves a user by their email address. Stored addresses are
// always normalized (users_email_lowercase), so normalizing the argument makes
// the match case-insensitive while still using the users_email_key index.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE email = $1`

	user, err := scanUser(r.db.QueryRowContext(ctx, query, models.NormalizeEmail(email)))
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get user by email: %w", err), repositories.ErrUserNotFound)
	}
	return user, nil
}

// GetByIDPSubject retrieves the user who owns the (provider, subject) sign-in
// identity. user_identities is the only lookup source; users.idp_provider and
// users.idp_subject are not consulted.
func (r *UserRepository) GetByIDPSubject(
	ctx context.Context, provider, subject string,
) (*models.User, error) {
	query := `SELECT ` + userColumnsQualified + `
		FROM user_identities ui
		JOIN users u ON u.id = ui.user_id
		WHERE ui.provider = $1 AND ui.subject = $2`

	user, err := scanUser(r.db.QueryRowContext(ctx, query, provider, subject))
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get user by IDP subject: %w", err), repositories.ErrUserNotFound)
	}
	return user, nil
}

// Create creates a new user together with the sign-in identities the row
// carries, in ONE statement: a user must never exist without the identity it
// signed up with, or its next sign-in would find nobody and collide on the
// email. The identity rows are the (idp_provider, idp_subject) pair when both
// are set, and the legacy Google identity when google_id is set.
func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	query := `
		WITH new_user AS (
			INSERT INTO users (google_id, idp_provider, idp_subject, email, email_verified_at,
			                   name, avatar_url, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, default_team_id,
			          onboarding_completed, onboarding_completed_at, created_at, updated_at
		), idp_identity AS (
			INSERT INTO user_identities (user_id, provider, subject, created_at, last_used_at)
			SELECT id, $2, $3, created_at, created_at FROM new_user
			WHERE $2::varchar IS NOT NULL AND $3::varchar IS NOT NULL
		), legacy_google_identity AS (
			INSERT INTO user_identities (user_id, provider, subject, created_at, last_used_at)
			SELECT id, $10, $1, created_at, created_at FROM new_user
			WHERE $1::varchar IS NOT NULL
		)
		SELECT id, default_team_id,
		       onboarding_completed, onboarding_completed_at, created_at, updated_at
		FROM new_user
	`

	email := models.NormalizeEmail(user.Email)
	err := r.db.QueryRowContext(ctx, query,
		user.GoogleID, user.IDPProvider, user.IDPSubject,
		email, user.EmailVerifiedAt, user.Name, user.AvatarURL,
		user.CreatedAt, user.UpdatedAt, models.LegacyGoogleIdentityProvider,
	).Scan(&user.ID, &user.DefaultTeamID,
		&user.OnboardingCompleted, &user.OnboardingCompletedAt, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		// A duplicate email is a client-correctable conflict, not a server fault:
		// surface it as a domain error so callers answer 409. Doing it here rather
		// than pre-checking in a service is also race-proof — the constraint is
		// the only place two concurrent inserts are actually serialized.
		if pqErr := uniqueViolation(err); pqErr != nil && pqErr.Constraint == usersEmailUniqueConstraint {
			return fmt.Errorf("%w: %s", repositories.ErrUserEmailTaken, email)
		}
		return fmt.Errorf("failed to create user: %w", err)
	}
	user.Email = email

	return nil
}

// Update updates an existing user. The WHERE clause uses the user's id (UUID)
// and version for optimistic concurrency control. google_id is no longer used
// in the WHERE clause because non-Google users do not have a google_id.
//
// It writes the users row only. Recording the sign-in identity is
// UserIdentityRepository.Upsert.
func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
	now := time.Now()
	query := `
		UPDATE users
		SET email = $2, name = $3, avatar_url = $4,
		    idp_provider = COALESCE($5, idp_provider),
		    idp_subject  = COALESCE($6, idp_subject),
		    updated_at = $7, version = version + 1,
		    email_verified_at = $9
		WHERE id = $1 AND version = $8
		RETURNING id,
			default_team_id, onboarding_completed, onboarding_completed_at, created_at, updated_at, version
	`

	email := models.NormalizeEmail(user.Email)
	err := r.db.QueryRowContext(ctx, query,
		user.ID, email, user.Name, user.AvatarURL,
		user.IDPProvider, user.IDPSubject, now, user.Version, user.EmailVerifiedAt,
	).Scan(
		&user.ID, &user.DefaultTeamID, &user.OnboardingCompleted,
		&user.OnboardingCompletedAt, &user.CreatedAt, &user.UpdatedAt, &user.Version,
	)

	if err != nil {
		return mapNoRows(fmt.Errorf("failed to update user: %w", err), fmt.Errorf("user not found or version mismatch"))
	}
	user.Email = email

	return nil
}

// UpdateName sets the user's display name. It does not use the optimistic
// version check Update does: the name is the only thing written, so a
// concurrent sign-in refreshing other columns must not make it fail.
func (r *UserRepository) UpdateName(ctx context.Context, userID, name string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET name = $2, updated_at = NOW(), version = version + 1 WHERE id = $1`, userID, name)
	if err != nil {
		return fmt.Errorf("failed to update user name: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read affected rows for user name update: %w", err)
	}
	if affected == 0 {
		return repositories.ErrUserNotFound
	}
	return nil
}

// UpdateDefaultTeamID updates a user's default team ID
func (r *UserRepository) UpdateDefaultTeamID(ctx context.Context, userID, teamID string) error {
	query := `
		UPDATE users
		SET default_team_id = $2, updated_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, userID, teamID)
	if err != nil {
		return fmt.Errorf("failed to update default team ID: %w", err)
	}

	return nil
}

// BumpSessionEpoch increments the user's session revocation counter and returns
// the new value (#1428). Every cookie session minted under an earlier epoch
// stops authenticating on its next request. The row's version and updated_at
// are left alone: this is not an edit of the user, and bumping version would
// fail a concurrent optimistic update for no reason.
func (r *UserRepository) BumpSessionEpoch(ctx context.Context, userID string) (int64, error) {
	query := `
		UPDATE users
		SET session_epoch = session_epoch + 1
		WHERE id = $1
		RETURNING session_epoch
	`

	var epoch int64
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&epoch); err != nil {
		return 0, mapNoRows(fmt.Errorf("failed to bump session epoch: %w", err), repositories.ErrUserNotFound)
	}
	return epoch, nil
}

// MarkOnboardingCompleted marks the user's onboarding as completed
func (r *UserRepository) MarkOnboardingCompleted(ctx context.Context, userID string) error {
	query := `
		UPDATE users
		SET onboarding_completed = true,
		    onboarding_completed_at = NOW(),
		    updated_at = NOW(),
		    version = version + 1
		WHERE id = $1
	`

	result, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to mark onboarding completed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrUserNotFound
	}

	return nil
}

// GetNamesByIDs returns a map of userID → display name for the given set of IDs.
// When a user's name field is blank the email address is used as the display value.
// Unknown IDs are silently omitted from the result map.
func (r *UserRepository) GetNamesByIDs(ctx context.Context, ids []string) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(
		`SELECT id, name, email FROM users WHERE id IN (%s)`,
		strings.Join(placeholders, ", "),
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get user names by ids: %w", err)
	}

	result, scanErr := scanUserNameRows(rows, len(ids))
	if closeErr := rows.Close(); closeErr != nil && scanErr == nil {
		return nil, fmt.Errorf("close user name rows: %w", closeErr)
	}
	if scanErr != nil {
		return nil, scanErr
	}
	return result, nil
}

func scanUserNameRows(rows interface {
	Next() bool
	Scan(...interface{}) error
	Err() error
}, capacity int) (map[string]string, error) {
	result := make(map[string]string, capacity)
	for rows.Next() {
		var id, name, email string
		if err := rows.Scan(&id, &name, &email); err != nil {
			return nil, fmt.Errorf("scan user name row: %w", err)
		}
		if name != "" {
			result[id] = name
		} else {
			result[id] = email
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user name rows: %w", err)
	}
	return result, nil
}

// DeleteByID removes a user row. See the interface doc: this is the compensating
// rollback for admin user creation, not the guarded admin delete.
//
// Deleting a user cascades widely (teams the user owns, and through them their
// members' data), which is safe HERE only because the caller uses it on a row it
// has just inserted and which therefore owns nothing yet.
func (r *UserRepository) DeleteByID(ctx context.Context, userID string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read affected rows for user delete: %w", err)
	}
	if affected == 0 {
		return repositories.ErrUserNotFound
	}
	return nil
}
