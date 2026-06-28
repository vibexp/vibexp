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
const userColumns = `id, google_id, idp_provider, idp_subject, email, name, avatar_url,
		stripe_customer_id, subscription_status, trial_ends_at, subscription_plan,
		subscription_canceled_at, default_team_id, onboarding_completed,
		onboarding_completed_at, created_at, updated_at, version`

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
		&user.Email, &user.Name, &user.AvatarURL, &user.StripeCustomerID,
		&user.SubscriptionStatus, &user.TrialEndsAt, &user.SubscriptionPlan,
		&user.SubscriptionCanceledAt, &user.DefaultTeamID,
		&user.OnboardingCompleted, &user.OnboardingCompletedAt,
		&user.CreatedAt, &user.UpdatedAt, &user.Version,
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

// GetByEmail retrieves a user by their email address
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE email = $1`

	user, err := scanUser(r.db.QueryRowContext(ctx, query, email))
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get user by email: %w", err), repositories.ErrUserNotFound)
	}
	return user, nil
}

// GetByGoogleID retrieves a user by their Google ID
func (r *UserRepository) GetByGoogleID(ctx context.Context, googleID string) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE google_id = $1`

	user, err := scanUser(r.db.QueryRowContext(ctx, query, googleID))
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get user by Google ID: %w", err), repositories.ErrUserNotFound)
	}
	return user, nil
}

// GetByIDPSubject retrieves a user by their (idp_provider, idp_subject) tuple.
func (r *UserRepository) GetByIDPSubject(
	ctx context.Context, provider, subject string,
) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE idp_provider = $1 AND idp_subject = $2`

	user, err := scanUser(r.db.QueryRowContext(ctx, query, provider, subject))
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get user by IDP subject: %w", err), repositories.ErrUserNotFound)
	}
	return user, nil
}

// GetByStripeCustomerID retrieves a user by their Stripe customer ID
func (r *UserRepository) GetByStripeCustomerID(ctx context.Context, stripeCustomerID string) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE stripe_customer_id = $1`

	user, err := scanUser(r.db.QueryRowContext(ctx, query, stripeCustomerID))
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get user by Stripe customer ID: %w", err), repositories.ErrUserNotFound)
	}
	return user, nil
}

// Create creates a new user
func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (google_id, idp_provider, idp_subject, email, name, avatar_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, subscription_status, subscription_plan, default_team_id,
		          onboarding_completed, onboarding_completed_at, created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		user.GoogleID, user.IDPProvider, user.IDPSubject,
		user.Email, user.Name, user.AvatarURL,
		user.CreatedAt, user.UpdatedAt,
	).Scan(&user.ID, &user.SubscriptionStatus, &user.SubscriptionPlan, &user.DefaultTeamID,
		&user.OnboardingCompleted, &user.OnboardingCompletedAt, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// Update updates an existing user. The WHERE clause uses the user's id (UUID)
// and version for optimistic concurrency control. google_id is no longer used
// in the WHERE clause because non-Google users do not have a google_id.
func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
	now := time.Now()
	query := `
		UPDATE users
		SET email = $2, name = $3, avatar_url = $4,
		    idp_provider = COALESCE($5, idp_provider),
		    idp_subject  = COALESCE($6, idp_subject),
		    updated_at = $7, version = version + 1
		WHERE id = $1 AND version = $8
		RETURNING id, subscription_status, trial_ends_at, subscription_plan, subscription_canceled_at,
			default_team_id, onboarding_completed, onboarding_completed_at, created_at, updated_at, version
	`

	err := r.db.QueryRowContext(ctx, query,
		user.ID, user.Email, user.Name, user.AvatarURL,
		user.IDPProvider, user.IDPSubject, now, user.Version,
	).Scan(
		&user.ID, &user.SubscriptionStatus, &user.TrialEndsAt, &user.SubscriptionPlan,
		&user.SubscriptionCanceledAt, &user.DefaultTeamID, &user.OnboardingCompleted,
		&user.OnboardingCompletedAt, &user.CreatedAt, &user.UpdatedAt, &user.Version,
	)

	if err != nil {
		return mapNoRows(fmt.Errorf("failed to update user: %w", err), fmt.Errorf("user not found or version mismatch"))
	}

	return nil
}

// UpdateSubscriptionStatus updates a user's subscription status and plan
func (r *UserRepository) UpdateSubscriptionStatus(ctx context.Context, userID, status string, plan *string) error {
	query := `
		UPDATE users
		SET subscription_status = $2, subscription_plan = $3, updated_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, userID, status, plan)
	if err != nil {
		return fmt.Errorf("failed to update subscription status: %w", err)
	}

	return nil
}

// UpdateSubscriptionStatusWithTrial updates a user's subscription status,
// plan, and trial end date
func (r *UserRepository) UpdateSubscriptionStatusWithTrial(
	ctx context.Context, userID, status string, plan *string, trialEnd *time.Time,
) error {
	// Build query with proper parameterization to avoid SQL injection
	query := `UPDATE users SET
		subscription_status = $1,
		subscription_plan = CASE WHEN $2::text IS NOT NULL THEN $2 ELSE subscription_plan END,
		trial_ends_at = CASE WHEN $3::timestamptz IS NOT NULL THEN $3 ELSE trial_ends_at END,
		updated_at = NOW()
		WHERE id = $4`

	var planValue interface{}
	var trialEndValue interface{}

	if plan != nil {
		planValue = *plan
	}
	if trialEnd != nil {
		trialEndValue = *trialEnd
	}

	result, err := r.db.ExecContext(ctx, query, status, planValue, trialEndValue, userID)
	if err != nil {
		return fmt.Errorf("failed to update subscription status with trial: %w", err)
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

// UpdateStripeCustomerID updates a user's Stripe customer ID
func (r *UserRepository) UpdateStripeCustomerID(ctx context.Context, userID, customerID string) error {
	query := `
		UPDATE users
		SET stripe_customer_id = $2, updated_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, userID, customerID)
	if err != nil {
		return fmt.Errorf("failed to update Stripe customer ID: %w", err)
	}

	return nil
}

// UpdateSubscriptionWithCancellation updates a user's subscription status, plan, trial end,
// and cancellation timestamp
func (r *UserRepository) UpdateSubscriptionWithCancellation(
	ctx context.Context, userID, status string, plan *string, trialEnd *time.Time, canceledAt *time.Time,
) error {
	query := `UPDATE users SET
		subscription_status = $1,
		subscription_plan = CASE WHEN $2::text IS NOT NULL THEN $2 ELSE subscription_plan END,
		trial_ends_at = CASE WHEN $3::timestamptz IS NOT NULL THEN $3 ELSE trial_ends_at END,
		subscription_canceled_at = $4,
		updated_at = NOW()
		WHERE id = $5`

	var planValue interface{}
	var trialEndValue interface{}

	if plan != nil {
		planValue = *plan
	}
	if trialEnd != nil {
		trialEndValue = *trialEnd
	}

	result, err := r.db.ExecContext(ctx, query, status, planValue, trialEndValue, canceledAt, userID)
	if err != nil {
		return fmt.Errorf("failed to update subscription with cancellation: %w", err)
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

// UpdateTrialEndsAt updates a user's trial end date
func (r *UserRepository) UpdateTrialEndsAt(ctx context.Context, userID string, trialEndsAt *time.Time) error {
	query := `
		UPDATE users
		SET trial_ends_at = $2, updated_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, userID, trialEndsAt)
	if err != nil {
		return fmt.Errorf("failed to update trial end date: %w", err)
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
