package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// TeamSubscriptionRepository implements the repositories.TeamSubscriptionRepository interface for PostgreSQL
type TeamSubscriptionRepository struct {
	db *database.DB
}

// NewTeamSubscriptionRepository creates a new TeamSubscriptionRepository
func NewTeamSubscriptionRepository(db *database.DB) repositories.TeamSubscriptionRepository {
	return &TeamSubscriptionRepository{
		db: db,
	}
}

// Create creates a new team subscription
func (r *TeamSubscriptionRepository) Create(ctx context.Context, subscription *models.TeamSubscription) error {
	query := `
		INSERT INTO team_subscriptions (
			team_id, stripe_subscription_id, stripe_customer_id, tier, seat_count,
			status, billing_interval, current_period_start, current_period_end,
			trial_end, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		subscription.TeamID,
		subscription.StripeSubscriptionID,
		subscription.StripeCustomerID,
		subscription.Tier,
		subscription.SeatCount,
		subscription.Status,
		subscription.BillingInterval,
		subscription.CurrentPeriodStart,
		subscription.CurrentPeriodEnd,
		subscription.TrialEnd,
		subscription.CreatedAt,
		subscription.UpdatedAt,
	).Scan(&subscription.ID, &subscription.CreatedAt, &subscription.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create team subscription: %w", err)
	}

	return nil
}

// GetByID retrieves a team subscription by its ID
func (r *TeamSubscriptionRepository) GetByID(ctx context.Context, id string) (*models.TeamSubscription, error) {
	query := `
		SELECT id, team_id, stripe_subscription_id, stripe_customer_id, tier, seat_count,
			   status, billing_interval, current_period_start, current_period_end,
			   trial_end, canceled_at, created_at, updated_at
		FROM team_subscriptions
		WHERE id = $1
	`

	var sub models.TeamSubscription
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&sub.ID, &sub.TeamID, &sub.StripeSubscriptionID, &sub.StripeCustomerID,
		&sub.Tier, &sub.SeatCount, &sub.Status, &sub.BillingInterval,
		&sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &sub.TrialEnd,
		&sub.CanceledAt, &sub.CreatedAt, &sub.UpdatedAt,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get team subscription: %w", err),
			repositories.ErrTeamSubscriptionNotFound,
		)
	}

	return &sub, nil
}

// GetByTeamID retrieves a team subscription by team ID
func (r *TeamSubscriptionRepository) GetByTeamID(ctx context.Context, teamID string) (*models.TeamSubscription, error) {
	query := `
		SELECT id, team_id, stripe_subscription_id, stripe_customer_id, tier, seat_count,
			   status, billing_interval, current_period_start, current_period_end,
			   trial_end, canceled_at, created_at, updated_at
		FROM team_subscriptions
		WHERE team_id = $1
	`

	var sub models.TeamSubscription
	err := r.db.QueryRowContext(ctx, query, teamID).Scan(
		&sub.ID, &sub.TeamID, &sub.StripeSubscriptionID, &sub.StripeCustomerID,
		&sub.Tier, &sub.SeatCount, &sub.Status, &sub.BillingInterval,
		&sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &sub.TrialEnd,
		&sub.CanceledAt, &sub.CreatedAt, &sub.UpdatedAt,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get team subscription by team ID: %w", err),
			repositories.ErrTeamSubscriptionNotFound,
		)
	}

	return &sub, nil
}

// GetByStripeSubscriptionID retrieves a team subscription by Stripe subscription ID
func (r *TeamSubscriptionRepository) GetByStripeSubscriptionID(
	ctx context.Context, stripeSubID string,
) (*models.TeamSubscription, error) {
	query := `
		SELECT id, team_id, stripe_subscription_id, stripe_customer_id, tier, seat_count,
			   status, billing_interval, current_period_start, current_period_end,
			   trial_end, canceled_at, created_at, updated_at
		FROM team_subscriptions
		WHERE stripe_subscription_id = $1
	`

	var sub models.TeamSubscription
	err := r.db.QueryRowContext(ctx, query, stripeSubID).Scan(
		&sub.ID, &sub.TeamID, &sub.StripeSubscriptionID, &sub.StripeCustomerID,
		&sub.Tier, &sub.SeatCount, &sub.Status, &sub.BillingInterval,
		&sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &sub.TrialEnd,
		&sub.CanceledAt, &sub.CreatedAt, &sub.UpdatedAt,
	)

	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get team subscription by Stripe ID: %w", err),
			repositories.ErrTeamSubscriptionNotFound,
		)
	}

	return &sub, nil
}

// Update updates an existing team subscription
func (r *TeamSubscriptionRepository) Update(ctx context.Context, subscription *models.TeamSubscription) error {
	query := `
		UPDATE team_subscriptions
		SET tier = $1, seat_count = $2, status = $3, billing_interval = $4,
			current_period_start = $5, current_period_end = $6, trial_end = $7,
			canceled_at = $8, updated_at = $9
		WHERE id = $10
		RETURNING updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		subscription.Tier,
		subscription.SeatCount,
		subscription.Status,
		subscription.BillingInterval,
		subscription.CurrentPeriodStart,
		subscription.CurrentPeriodEnd,
		subscription.TrialEnd,
		subscription.CanceledAt,
		subscription.UpdatedAt,
		subscription.ID,
	).Scan(&subscription.UpdatedAt)

	if err != nil {
		return mapNoRows(fmt.Errorf("failed to update team subscription: %w", err), repositories.ErrTeamSubscriptionNotFound)
	}

	return nil
}

// Delete deletes a team subscription. Returns repositories.ErrTeamSubscriptionNotFound
// (wrapped) when no row matched the id, so callers can distinguish a benign "already
// gone" outcome (e.g. concurrent webhook deleted the stale terminal-dead row first)
// from a real database error.
func (r *TeamSubscriptionRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM team_subscriptions WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete team subscription: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("delete team subscription %s: %w", id, repositories.ErrTeamSubscriptionNotFound)
	}

	return nil
}

// UpdateStatus updates the subscription status (optimized for webhooks)
func (r *TeamSubscriptionRepository) UpdateStatus(ctx context.Context, stripeSubID, status string) error {
	query := `
		UPDATE team_subscriptions
		SET status = $1, updated_at = NOW()
		WHERE stripe_subscription_id = $2
	`

	result, err := r.db.ExecContext(ctx, query, status, stripeSubID)
	if err != nil {
		return fmt.Errorf("failed to update subscription status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrTeamSubscriptionNotFound
	}

	return nil
}

// UpdateSeatCount updates the seat count (optimized for webhooks)
func (r *TeamSubscriptionRepository) UpdateSeatCount(ctx context.Context, stripeSubID string, seatCount int) error {
	query := `
		UPDATE team_subscriptions
		SET seat_count = $1, updated_at = NOW()
		WHERE stripe_subscription_id = $2
	`

	result, err := r.db.ExecContext(ctx, query, seatCount, stripeSubID)
	if err != nil {
		return fmt.Errorf("failed to update seat count: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrTeamSubscriptionNotFound
	}

	return nil
}

// ListByStatus lists subscriptions by status with pagination
func (r *TeamSubscriptionRepository) ListByStatus(
	ctx context.Context, status string, limit, offset int,
) ([]*models.TeamSubscription, int, error) {
	// Get total count
	countQuery := `SELECT COUNT(*) FROM team_subscriptions WHERE status = $1`
	var totalCount int
	err := r.db.QueryRowContext(ctx, countQuery, status).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count subscriptions: %w", err)
	}

	// Get subscriptions with pagination
	query := `
		SELECT id, team_id, stripe_subscription_id, stripe_customer_id, tier, seat_count,
			   status, billing_interval, current_period_start, current_period_end,
			   trial_end, canceled_at, created_at, updated_at
		FROM team_subscriptions
		WHERE status = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, status, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list subscriptions: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var subscriptions []*models.TeamSubscription
	for rows.Next() {
		var sub models.TeamSubscription
		err := rows.Scan(
			&sub.ID, &sub.TeamID, &sub.StripeSubscriptionID, &sub.StripeCustomerID,
			&sub.Tier, &sub.SeatCount, &sub.Status, &sub.BillingInterval,
			&sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &sub.TrialEnd,
			&sub.CanceledAt, &sub.CreatedAt, &sub.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan subscription: %w", err)
		}
		subscriptions = append(subscriptions, &sub)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating subscriptions: %w", err)
	}

	return subscriptions, totalCount, nil
}

// ListByTier lists subscriptions by tier with pagination
func (r *TeamSubscriptionRepository) ListByTier(
	ctx context.Context, tier string, limit, offset int,
) ([]*models.TeamSubscription, int, error) {
	// Get total count
	countQuery := `SELECT COUNT(*) FROM team_subscriptions WHERE tier = $1`
	var totalCount int
	err := r.db.QueryRowContext(ctx, countQuery, tier).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count subscriptions: %w", err)
	}

	// Get subscriptions with pagination
	query := `
		SELECT id, team_id, stripe_subscription_id, stripe_customer_id, tier, seat_count,
			   status, billing_interval, current_period_start, current_period_end,
			   trial_end, canceled_at, created_at, updated_at
		FROM team_subscriptions
		WHERE tier = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, tier, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list subscriptions: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var subscriptions []*models.TeamSubscription
	for rows.Next() {
		var sub models.TeamSubscription
		err := rows.Scan(
			&sub.ID, &sub.TeamID, &sub.StripeSubscriptionID, &sub.StripeCustomerID,
			&sub.Tier, &sub.SeatCount, &sub.Status, &sub.BillingInterval,
			&sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &sub.TrialEnd,
			&sub.CanceledAt, &sub.CreatedAt, &sub.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan subscription: %w", err)
		}
		subscriptions = append(subscriptions, &sub)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating subscriptions: %w", err)
	}

	return subscriptions, totalCount, nil
}

// GetActiveByTeamID returns active subscription (status: active, trialing, past_due).
//
// When the team has no active subscription it returns (nil, nil) — not an
// error — so callers can treat "no subscription" as the free tier.
func (r *TeamSubscriptionRepository) GetActiveByTeamID(
	ctx context.Context, teamID string,
) (*models.TeamSubscription, error) {
	query := `
		SELECT id, team_id, stripe_subscription_id, stripe_customer_id, tier, seat_count,
			   status, billing_interval, current_period_start, current_period_end,
			   trial_end, canceled_at, created_at, updated_at
		FROM team_subscriptions
		WHERE team_id = $1
		  AND status IN ('active', 'trialing', 'past_due')
		LIMIT 1
	`

	var sub models.TeamSubscription
	err := r.db.QueryRowContext(ctx, query, teamID).Scan(
		&sub.ID, &sub.TeamID, &sub.StripeSubscriptionID, &sub.StripeCustomerID,
		&sub.Tier, &sub.SeatCount, &sub.Status, &sub.BillingInterval,
		&sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &sub.TrialEnd,
		&sub.CanceledAt, &sub.CreatedAt, &sub.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get active team subscription: %w", err)
	}

	return &sub, nil
}

// GetCanceledByTeamID returns canceled subscription that hasn't been deleted yet.
//
// When the team has no such subscription it returns (nil, nil) — not an error.
func (r *TeamSubscriptionRepository) GetCanceledByTeamID(
	ctx context.Context, teamID string,
) (*models.TeamSubscription, error) {
	query := `
		SELECT id, team_id, stripe_subscription_id, stripe_customer_id, tier, seat_count,
			   status, billing_interval, current_period_start, current_period_end,
			   trial_end, canceled_at, created_at, updated_at
		FROM team_subscriptions
		WHERE team_id = $1
		  AND status = 'canceled'
		  AND current_period_end > NOW()
		LIMIT 1
	`

	var sub models.TeamSubscription
	err := r.db.QueryRowContext(ctx, query, teamID).Scan(
		&sub.ID, &sub.TeamID, &sub.StripeSubscriptionID, &sub.StripeCustomerID,
		&sub.Tier, &sub.SeatCount, &sub.Status, &sub.BillingInterval,
		&sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &sub.TrialEnd,
		&sub.CanceledAt, &sub.CreatedAt, &sub.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get canceled team subscription: %w", err)
	}

	return &sub, nil
}
