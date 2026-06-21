package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

func setupTeamSubscriptionTest(t *testing.T) (*TeamSubscriptionRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := NewTeamSubscriptionRepository(db).(*TeamSubscriptionRepository)

	return repo, mock, mockDB
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamSubscriptionRepository_Create(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	trialEnd := now.Add(14 * 24 * time.Hour)

	tests := []struct {
		name         string
		subscription *models.TeamSubscription
		setupMock    func()
		expectErr    bool
	}{
		{
			name: "successful create",
			subscription: &models.TeamSubscription{
				TeamID:               "team-123",
				StripeSubscriptionID: "sub_abc123",
				StripeCustomerID:     "cus_xyz789",
				Tier:                 models.TeamTierStarter,
				SeatCount:            5,
				Status:               models.TeamSubscriptionStatusTrialing,
				BillingInterval:      models.BillingIntervalMonth,
				CurrentPeriodStart:   now,
				CurrentPeriodEnd:     now.Add(30 * 24 * time.Hour),
				TrialEnd:             &trialEnd,
				CreatedAt:            now,
				UpdatedAt:            now,
			},
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
					AddRow("sub-123", now, now)

				mock.ExpectQuery(`INSERT INTO team_subscriptions`).
					WithArgs(
						"team-123",
						"sub_abc123",
						"cus_xyz789",
						models.TeamTierStarter,
						5,
						models.TeamSubscriptionStatusTrialing,
						models.BillingIntervalMonth,
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						&trialEnd,
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
					).
					WillReturnRows(rows)
			},
			expectErr: false,
		},
		{
			name: "database error",
			subscription: &models.TeamSubscription{
				TeamID:               "team-error",
				StripeSubscriptionID: "sub_error",
				StripeCustomerID:     "cus_error",
				Tier:                 models.TeamTierProfessional,
				SeatCount:            10,
				Status:               models.TeamSubscriptionStatusActive,
				BillingInterval:      models.BillingIntervalYear,
				CurrentPeriodStart:   now,
				CurrentPeriodEnd:     now.Add(365 * 24 * time.Hour),
				CreatedAt:            now,
				UpdatedAt:            now,
			},
			setupMock: func() {
				mock.ExpectQuery(`INSERT INTO team_subscriptions`).
					WithArgs(
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
					).
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.Create(ctx, tt.subscription)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, tt.subscription.ID)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamSubscriptionRepository_GetByID(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name      string
		id        string
		setupMock func()
		expectErr bool
	}{
		{
			name: "successful get",
			id:   "sub-123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "team_id", "stripe_subscription_id", "stripe_customer_id",
					"tier", "seat_count", "status", "billing_interval",
					"current_period_start", "current_period_end", "trial_end",
					"canceled_at", "created_at", "updated_at",
				}).AddRow(
					"sub-123", "team-123", "sub_abc123", "cus_xyz789",
					models.TeamTierStarter, 5, models.TeamSubscriptionStatusActive, models.BillingIntervalMonth,
					now, now.Add(30*24*time.Hour), nil,
					nil, now, now,
				)

				mock.ExpectQuery(`SELECT .+ FROM team_subscriptions WHERE id`).
					WithArgs("sub-123").
					WillReturnRows(rows)
			},
			expectErr: false,
		},
		{
			name: "not found",
			id:   "sub-notfound",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM team_subscriptions WHERE id`).
					WithArgs("sub-notfound").
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			sub, err := repo.GetByID(ctx, tt.id)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, sub)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, sub)
				assert.Equal(t, tt.id, sub.ID)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestTeamSubscriptionRepository_GetByTeamID(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "team_id", "stripe_subscription_id", "stripe_customer_id",
		"tier", "seat_count", "status", "billing_interval",
		"current_period_start", "current_period_end", "trial_end",
		"canceled_at", "created_at", "updated_at",
	}).AddRow(
		"sub-123", "team-123", "sub_abc123", "cus_xyz789",
		models.TeamTierStarter, 5, models.TeamSubscriptionStatusActive, models.BillingIntervalMonth,
		now, now.Add(30*24*time.Hour), nil,
		nil, now, now,
	)

	mock.ExpectQuery(`SELECT .+ FROM team_subscriptions WHERE team_id`).
		WithArgs("team-123").
		WillReturnRows(rows)

	sub, err := repo.GetByTeamID(ctx, "team-123")

	assert.NoError(t, err)
	assert.NotNil(t, sub)
	assert.Equal(t, "team-123", sub.TeamID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamSubscriptionRepository_GetByStripeSubscriptionID(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "team_id", "stripe_subscription_id", "stripe_customer_id",
		"tier", "seat_count", "status", "billing_interval",
		"current_period_start", "current_period_end", "trial_end",
		"canceled_at", "created_at", "updated_at",
	}).AddRow(
		"sub-123", "team-123", "sub_abc123", "cus_xyz789",
		models.TeamTierStarter, 5, models.TeamSubscriptionStatusActive, models.BillingIntervalMonth,
		now, now.Add(30*24*time.Hour), nil,
		nil, now, now,
	)

	mock.ExpectQuery(`SELECT .+ FROM team_subscriptions WHERE stripe_subscription_id`).
		WithArgs("sub_abc123").
		WillReturnRows(rows)

	sub, err := repo.GetByStripeSubscriptionID(ctx, "sub_abc123")

	assert.NoError(t, err)
	assert.NotNil(t, sub)
	assert.Equal(t, "sub_abc123", sub.StripeSubscriptionID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamSubscriptionRepository_Update(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name         string
		subscription *models.TeamSubscription
		setupMock    func()
		expectErr    bool
	}{
		{
			name: "successful update",
			subscription: &models.TeamSubscription{
				ID:                   "sub-123",
				TeamID:               "team-123",
				StripeSubscriptionID: "sub_abc123",
				StripeCustomerID:     "cus_xyz789",
				Tier:                 models.TeamTierProfessional,
				SeatCount:            10,
				Status:               models.TeamSubscriptionStatusActive,
				BillingInterval:      models.BillingIntervalYear,
				CurrentPeriodStart:   now,
				CurrentPeriodEnd:     now.Add(365 * 24 * time.Hour),
				UpdatedAt:            now,
			},
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"updated_at"}).AddRow(now)

				mock.ExpectQuery(`UPDATE team_subscriptions SET`).
					WithArgs(
						models.TeamTierProfessional,
						10,
						models.TeamSubscriptionStatusActive,
						models.BillingIntervalYear,
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						"sub-123",
					).
					WillReturnRows(rows)
			},
			expectErr: false,
		},
		{
			name: "not found",
			subscription: &models.TeamSubscription{
				ID:        "sub-notfound",
				UpdatedAt: now,
			},
			setupMock: func() {
				mock.ExpectQuery(`UPDATE team_subscriptions SET`).
					WithArgs(
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						"sub-notfound",
					).
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.Update(ctx, tt.subscription)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestTeamSubscriptionRepository_UpdateStatus(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	mock.ExpectExec(`UPDATE team_subscriptions SET status`).
		WithArgs(models.TeamSubscriptionStatusCanceled, "sub_abc123").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.UpdateStatus(ctx, "sub_abc123", models.TeamSubscriptionStatusCanceled)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamSubscriptionRepository_UpdateSeatCount(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	mock.ExpectExec(`UPDATE team_subscriptions SET seat_count`).
		WithArgs(8, "sub_abc123").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.UpdateSeatCount(ctx, "sub_abc123", 8)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamSubscriptionRepository_ListByStatus(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	// Mock count query
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM team_subscriptions WHERE status`).
		WithArgs(models.TeamSubscriptionStatusActive).
		WillReturnRows(countRows)

	// Mock list query
	rows := sqlmock.NewRows([]string{
		"id", "team_id", "stripe_subscription_id", "stripe_customer_id",
		"tier", "seat_count", "status", "billing_interval",
		"current_period_start", "current_period_end", "trial_end",
		"canceled_at", "created_at", "updated_at",
	}).AddRow(
		"sub-1", "team-1", "sub_1", "cus_1",
		models.TeamTierStarter, 5, models.TeamSubscriptionStatusActive, models.BillingIntervalMonth,
		now, now.Add(30*24*time.Hour), nil,
		nil, now, now,
	).AddRow(
		"sub-2", "team-2", "sub_2", "cus_2",
		models.TeamTierProfessional, 10, models.TeamSubscriptionStatusActive, models.BillingIntervalYear,
		now, now.Add(365*24*time.Hour), nil,
		nil, now, now,
	)

	mock.ExpectQuery(`SELECT .+ FROM team_subscriptions WHERE status`).
		WithArgs(models.TeamSubscriptionStatusActive, 10, 0).
		WillReturnRows(rows)

	subs, total, err := repo.ListByStatus(ctx, models.TeamSubscriptionStatusActive, 10, 0)

	assert.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, subs, 2)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamSubscriptionRepository_ListByTier(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	// Mock count query
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM team_subscriptions WHERE tier`).
		WithArgs(models.TeamTierEnterprise).
		WillReturnRows(countRows)

	// Mock list query
	rows := sqlmock.NewRows([]string{
		"id", "team_id", "stripe_subscription_id", "stripe_customer_id",
		"tier", "seat_count", "status", "billing_interval",
		"current_period_start", "current_period_end", "trial_end",
		"canceled_at", "created_at", "updated_at",
	}).AddRow(
		"sub-1", "team-1", "sub_1", "cus_1",
		models.TeamTierEnterprise, 20, models.TeamSubscriptionStatusActive, models.BillingIntervalYear,
		now, now.Add(365*24*time.Hour), nil,
		nil, now, now,
	)

	mock.ExpectQuery(`SELECT .+ FROM team_subscriptions WHERE tier`).
		WithArgs(models.TeamTierEnterprise, 10, 0).
		WillReturnRows(rows)

	subs, total, err := repo.ListByTier(ctx, models.TeamTierEnterprise, 10, 0)

	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, subs, 1)
	assert.Equal(t, models.TeamTierEnterprise, subs[0].Tier)
	assert.NoError(t, mock.ExpectationsWereMet())
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamSubscriptionRepository_Delete(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	tests := []struct {
		name              string
		id                string
		setupMock         func()
		expectErr         bool
		expectNotFoundErr bool
	}{
		{
			name: "successful delete",
			id:   "sub-123",
			setupMock: func() {
				mock.ExpectExec(`DELETE FROM team_subscriptions WHERE id`).
					WithArgs("sub-123").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name: "not found returns ErrTeamSubscriptionNotFound sentinel",
			id:   "sub-notfound",
			setupMock: func() {
				mock.ExpectExec(`DELETE FROM team_subscriptions WHERE id`).
					WithArgs("sub-notfound").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			expectErr:         true,
			expectNotFoundErr: true,
		},
		{
			name: "database error is not the sentinel",
			id:   "sub-error",
			setupMock: func() {
				mock.ExpectExec(`DELETE FROM team_subscriptions WHERE id`).
					WithArgs("sub-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.Delete(ctx, tt.id)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.expectNotFoundErr {
					assert.True(t, errors.Is(err, repositories.ErrTeamSubscriptionNotFound),
						"Delete with no matching row must wrap ErrTeamSubscriptionNotFound; got: %v", err)
				} else {
					assert.False(t, errors.Is(err, repositories.ErrTeamSubscriptionNotFound),
						"non-not-found errors must NOT be wrapped as ErrTeamSubscriptionNotFound")
				}
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestTeamSubscriptionRepository_GetByTeamID_NotFound tests not found error
func TestTeamSubscriptionRepository_GetByTeamID_NotFound(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	mock.ExpectQuery(`SELECT .+ FROM team_subscriptions WHERE team_id`).
		WithArgs("team-notfound").
		WillReturnError(sql.ErrNoRows)

	sub, err := repo.GetByTeamID(ctx, "team-notfound")

	assert.Error(t, err)
	assert.Nil(t, sub)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTeamSubscriptionRepository_GetByStripeSubscriptionID_NotFound tests not found error
func TestTeamSubscriptionRepository_GetByStripeSubscriptionID_NotFound(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	mock.ExpectQuery(`SELECT .+ FROM team_subscriptions WHERE stripe_subscription_id`).
		WithArgs("sub_notfound").
		WillReturnError(sql.ErrNoRows)

	sub, err := repo.GetByStripeSubscriptionID(ctx, "sub_notfound")

	assert.Error(t, err)
	assert.Nil(t, sub)
	assert.NoError(t, mock.ExpectationsWereMet())
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamSubscriptionRepository_UpdateStatus_Errors(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	tests := []struct {
		name        string
		stripeSubID string
		status      string
		setupMock   func()
		expectErr   bool
	}{
		{
			name:        "subscription not found",
			stripeSubID: "sub_notfound",
			status:      models.TeamSubscriptionStatusCanceled,
			setupMock: func() {
				mock.ExpectExec(`UPDATE team_subscriptions SET status`).
					WithArgs(models.TeamSubscriptionStatusCanceled, "sub_notfound").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			expectErr: true,
		},
		{
			name:        "database error",
			stripeSubID: "sub_error",
			status:      models.TeamSubscriptionStatusCanceled,
			setupMock: func() {
				mock.ExpectExec(`UPDATE team_subscriptions SET status`).
					WithArgs(models.TeamSubscriptionStatusCanceled, "sub_error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
		{
			name:        "rows affected error",
			stripeSubID: "sub_rows_err",
			status:      models.TeamSubscriptionStatusActive,
			setupMock: func() {
				mock.ExpectExec(`UPDATE team_subscriptions SET status`).
					WithArgs(models.TeamSubscriptionStatusActive, "sub_rows_err").
					WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.UpdateStatus(ctx, tt.stripeSubID, tt.status)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamSubscriptionRepository_UpdateSeatCount_Errors(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	tests := []struct {
		name        string
		stripeSubID string
		seatCount   int
		setupMock   func()
		expectErr   bool
	}{
		{
			name:        "subscription not found",
			stripeSubID: "sub_notfound",
			seatCount:   5,
			setupMock: func() {
				mock.ExpectExec(`UPDATE team_subscriptions SET seat_count`).
					WithArgs(5, "sub_notfound").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			expectErr: true,
		},
		{
			name:        "database error",
			stripeSubID: "sub_error",
			seatCount:   10,
			setupMock: func() {
				mock.ExpectExec(`UPDATE team_subscriptions SET seat_count`).
					WithArgs(10, "sub_error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
		{
			name:        "rows affected error",
			stripeSubID: "sub_rows_err",
			seatCount:   15,
			setupMock: func() {
				mock.ExpectExec(`UPDATE team_subscriptions SET seat_count`).
					WithArgs(15, "sub_rows_err").
					WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.UpdateSeatCount(ctx, tt.stripeSubID, tt.seatCount)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestTeamSubscriptionRepository_GetActiveByTeamID tests GetActiveByTeamID
//
//nolint:funlen // table-driven test with multiple test cases requiring detailed setup
func TestTeamSubscriptionRepository_GetActiveByTeamID(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name      string
		teamID    string
		setupMock func()
		expectNil bool
		expectErr bool
	}{
		{
			name:   "active subscription found",
			teamID: "team-123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "team_id", "stripe_subscription_id", "stripe_customer_id",
					"tier", "seat_count", "status", "billing_interval",
					"current_period_start", "current_period_end", "trial_end",
					"canceled_at", "created_at", "updated_at",
				}).AddRow(
					"sub-123", "team-123", "sub_abc123", "cus_xyz789",
					models.TeamTierStarter, 5, models.TeamSubscriptionStatusActive, models.BillingIntervalMonth,
					now, now.Add(30*24*time.Hour), nil,
					nil, now, now,
				)

				mock.ExpectQuery(`SELECT .+ FROM team_subscriptions WHERE team_id`).
					WithArgs("team-123").
					WillReturnRows(rows)
			},
			expectNil: false,
			expectErr: false,
		},
		{
			name:   "no active subscription - returns nil without error",
			teamID: "team-no-active",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM team_subscriptions WHERE team_id`).
					WithArgs("team-no-active").
					WillReturnError(sql.ErrNoRows)
			},
			expectNil: true,
			expectErr: false,
		},
		{
			name:   "database error",
			teamID: "team-error",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM team_subscriptions WHERE team_id`).
					WithArgs("team-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectNil: true,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			sub, err := repo.GetActiveByTeamID(ctx, tt.teamID)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectNil {
				assert.Nil(t, sub)
			} else {
				assert.NotNil(t, sub)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestTeamSubscriptionRepository_GetCanceledByTeamID tests GetCanceledByTeamID
//
//nolint:funlen // table-driven test with multiple test cases requiring detailed setup
func TestTeamSubscriptionRepository_GetCanceledByTeamID(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	canceledAt := now.Add(-24 * time.Hour)

	tests := []struct {
		name      string
		teamID    string
		setupMock func()
		expectNil bool
		expectErr bool
	}{
		{
			name:   "canceled subscription found",
			teamID: "team-123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "team_id", "stripe_subscription_id", "stripe_customer_id",
					"tier", "seat_count", "status", "billing_interval",
					"current_period_start", "current_period_end", "trial_end",
					"canceled_at", "created_at", "updated_at",
				}).AddRow(
					"sub-123", "team-123", "sub_abc123", "cus_xyz789",
					models.TeamTierStarter, 5, models.TeamSubscriptionStatusCanceled, models.BillingIntervalMonth,
					now.Add(-30*24*time.Hour), now.Add(7*24*time.Hour), nil,
					canceledAt, now.Add(-30*24*time.Hour), now,
				)

				mock.ExpectQuery(`SELECT .+ FROM team_subscriptions WHERE team_id`).
					WithArgs("team-123").
					WillReturnRows(rows)
			},
			expectNil: false,
			expectErr: false,
		},
		{
			name:   "no canceled subscription - returns nil without error",
			teamID: "team-no-canceled",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM team_subscriptions WHERE team_id`).
					WithArgs("team-no-canceled").
					WillReturnError(sql.ErrNoRows)
			},
			expectNil: true,
			expectErr: false,
		},
		{
			name:   "database error",
			teamID: "team-error",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM team_subscriptions WHERE team_id`).
					WithArgs("team-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectNil: true,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			sub, err := repo.GetCanceledByTeamID(ctx, tt.teamID)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectNil {
				assert.Nil(t, sub)
			} else {
				assert.NotNil(t, sub)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestTeamSubscriptionRepository_ListByStatus_CountError tests count query error
func TestTeamSubscriptionRepository_ListByStatus_CountError(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM team_subscriptions WHERE status`).
		WithArgs(models.TeamSubscriptionStatusActive).
		WillReturnError(sql.ErrConnDone)

	subs, total, err := repo.ListByStatus(ctx, models.TeamSubscriptionStatusActive, 10, 0)

	assert.Error(t, err)
	assert.Nil(t, subs)
	assert.Equal(t, 0, total)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTeamSubscriptionRepository_ListByStatus_ListError tests list query error
func TestTeamSubscriptionRepository_ListByStatus_ListError(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM team_subscriptions WHERE status`).
		WithArgs(models.TeamSubscriptionStatusActive).
		WillReturnRows(countRows)

	mock.ExpectQuery(`SELECT .+ FROM team_subscriptions WHERE status`).
		WithArgs(models.TeamSubscriptionStatusActive, 10, 0).
		WillReturnError(sql.ErrConnDone)

	subs, total, err := repo.ListByStatus(ctx, models.TeamSubscriptionStatusActive, 10, 0)

	assert.Error(t, err)
	assert.Nil(t, subs)
	assert.Equal(t, 0, total)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTeamSubscriptionRepository_ListByTier_CountError tests count query error
func TestTeamSubscriptionRepository_ListByTier_CountError(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM team_subscriptions WHERE tier`).
		WithArgs(models.TeamTierEnterprise).
		WillReturnError(sql.ErrConnDone)

	subs, total, err := repo.ListByTier(ctx, models.TeamTierEnterprise, 10, 0)

	assert.Error(t, err)
	assert.Nil(t, subs)
	assert.Equal(t, 0, total)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTeamSubscriptionRepository_Delete_RowsAffectedError tests rows affected error
func TestTeamSubscriptionRepository_Delete_RowsAffectedError(t *testing.T) {
	repo, mock, mockDB := setupTeamSubscriptionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	mock.ExpectExec(`DELETE FROM team_subscriptions WHERE id`).
		WithArgs("sub-123").
		WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

	err := repo.Delete(ctx, "sub-123")

	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
