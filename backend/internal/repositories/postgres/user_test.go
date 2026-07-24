package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

func setupUserTest(t *testing.T) (*UserRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := NewUserRepository(db).(*UserRepository)

	return repo, mock, mockDB
}

//nolint:funlen // table-driven test with multiple test cases
func TestUserRepository_GetByID(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	avatarURL := "https://example.com/avatar.png"
	stripeCustomerID := "cus_123"
	subscriptionPlan := "pro"
	defaultTeamID := "team-123"

	tests := []struct {
		name       string
		userID     string
		setupMock  func()
		expectErr  bool
		validateFn func(*testing.T, *models.User)
	}{
		{
			name:   "successful retrieval with all fields",
			userID: "user-123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "google_id", "idp_provider", "idp_subject", "email", "name", "avatar_url", "stripe_customer_id",
					"subscription_status", "trial_ends_at", "subscription_plan", "subscription_canceled_at",
					"default_team_id", "status", "onboarding_completed", "onboarding_completed_at",
					"created_at", "updated_at", "version",
				}).AddRow(
					"user-123", "google-123", nil, nil, "user@example.com", "Test User", &avatarURL, &stripeCustomerID,
					"active", nil, &subscriptionPlan, nil,
					&defaultTeamID, "active", false, nil,
					now, now, 1,
				)

				mock.ExpectQuery(`SELECT .+ FROM users WHERE id`).
					WithArgs("user-123").
					WillReturnRows(rows)
			},
			expectErr: false,
			validateFn: func(t *testing.T, user *models.User) {
				assert.Equal(t, "user-123", user.ID)
				assert.Equal(t, strPtr("google-123"), user.GoogleID)
				assert.Equal(t, "user@example.com", user.Email)
				assert.Equal(t, "Test User", user.Name)
				assert.Equal(t, &avatarURL, user.AvatarURL)
				assert.Equal(t, "active", user.SubscriptionStatus)
			},
		},
		{
			name:   "successful retrieval with minimal fields",
			userID: "user-456",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "google_id", "idp_provider", "idp_subject", "email", "name", "avatar_url", "stripe_customer_id",
					"subscription_status", "trial_ends_at", "subscription_plan", "subscription_canceled_at",
					"default_team_id", "status", "onboarding_completed", "onboarding_completed_at",
					"created_at", "updated_at", "version",
				}).AddRow(
					"user-456", "google-456", nil, nil, "minimal@example.com", "Minimal User", nil, nil,
					"trial", nil, nil, nil,
					nil, "active", false, nil,
					now, now, 1,
				)

				mock.ExpectQuery(`SELECT .+ FROM users WHERE id`).
					WithArgs("user-456").
					WillReturnRows(rows)
			},
			expectErr: false,
			validateFn: func(t *testing.T, user *models.User) {
				assert.Equal(t, "user-456", user.ID)
				assert.Nil(t, user.AvatarURL)
				assert.Nil(t, user.StripeCustomerID)
			},
		},
		{
			name:   "not found",
			userID: "user-notfound",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM users WHERE id`).
					WithArgs("user-notfound").
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
		},
		{
			name:   "database error",
			userID: "user-error",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM users WHERE id`).
					WithArgs("user-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.GetByID(ctx, tt.userID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.validateFn != nil {
					tt.validateFn(t, result)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestUserRepository_GetByEmail(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name       string
		email      string
		setupMock  func()
		expectErr  bool
		validateFn func(*testing.T, *models.User)
	}{
		{
			name:  "successful retrieval",
			email: "user@example.com",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "google_id", "idp_provider", "idp_subject", "email", "name", "avatar_url", "stripe_customer_id",
					"subscription_status", "trial_ends_at", "subscription_plan", "subscription_canceled_at",
					"default_team_id", "status", "onboarding_completed", "onboarding_completed_at",
					"created_at", "updated_at", "version",
				}).AddRow(
					"user-123", "google-123", nil, nil, "user@example.com", "Test User", nil, nil,
					"active", nil, nil, nil,
					nil, "active", false, nil,
					now, now, 1,
				)

				mock.ExpectQuery(`SELECT .+ FROM users WHERE email`).
					WithArgs("user@example.com").
					WillReturnRows(rows)
			},
			expectErr: false,
			validateFn: func(t *testing.T, user *models.User) {
				assert.Equal(t, "user@example.com", user.Email)
			},
		},
		{
			name:  "not found",
			email: "notfound@example.com",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM users WHERE email`).
					WithArgs("notfound@example.com").
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
		},
		{
			name:  "database error",
			email: "error@example.com",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM users WHERE email`).
					WithArgs("error@example.com").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.GetByEmail(ctx, tt.email)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.validateFn != nil {
					tt.validateFn(t, result)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestUserRepository_GetByGoogleID(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name       string
		googleID   string
		setupMock  func()
		expectErr  bool
		validateFn func(*testing.T, *models.User)
	}{
		{
			name:     "successful retrieval",
			googleID: "google-123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "google_id", "idp_provider", "idp_subject", "email", "name", "avatar_url", "stripe_customer_id",
					"subscription_status", "trial_ends_at", "subscription_plan", "subscription_canceled_at",
					"default_team_id", "status", "onboarding_completed", "onboarding_completed_at",
					"created_at", "updated_at", "version",
				}).AddRow(
					"user-123", "google-123", nil, nil, "user@example.com", "Test User", nil, nil,
					"active", nil, nil, nil,
					nil, "active", false, nil,
					now, now, 1,
				)

				mock.ExpectQuery(`SELECT .+ FROM users WHERE google_id`).
					WithArgs("google-123").
					WillReturnRows(rows)
			},
			expectErr: false,
			validateFn: func(t *testing.T, user *models.User) {
				assert.Equal(t, strPtr("google-123"), user.GoogleID)
			},
		},
		{
			name:     "not found",
			googleID: "google-notfound",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM users WHERE google_id`).
					WithArgs("google-notfound").
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
		},
		{
			name:     "database error",
			googleID: "google-error",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM users WHERE google_id`).
					WithArgs("google-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.GetByGoogleID(ctx, tt.googleID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.validateFn != nil {
					tt.validateFn(t, result)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestUserRepository_GetByStripeCustomerID(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	stripeCustomerID := "cus_123"

	tests := []struct {
		name             string
		stripeCustomerID string
		setupMock        func()
		expectErr        bool
		validateFn       func(*testing.T, *models.User)
	}{
		{
			name:             "successful retrieval",
			stripeCustomerID: "cus_123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "google_id", "idp_provider", "idp_subject", "email", "name", "avatar_url", "stripe_customer_id",
					"subscription_status", "trial_ends_at", "subscription_plan", "subscription_canceled_at",
					"default_team_id", "status", "onboarding_completed", "onboarding_completed_at",
					"created_at", "updated_at", "version",
				}).AddRow(
					"user-123", "google-123", nil, nil, "user@example.com", "Test User", nil, &stripeCustomerID,
					"active", nil, nil, nil,
					nil, "active", false, nil,
					now, now, 1,
				)

				mock.ExpectQuery(`SELECT .+ FROM users WHERE stripe_customer_id`).
					WithArgs("cus_123").
					WillReturnRows(rows)
			},
			expectErr: false,
			validateFn: func(t *testing.T, user *models.User) {
				assert.Equal(t, &stripeCustomerID, user.StripeCustomerID)
			},
		},
		{
			name:             "not found",
			stripeCustomerID: "cus_notfound",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM users WHERE stripe_customer_id`).
					WithArgs("cus_notfound").
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
		},
		{
			name:             "database error",
			stripeCustomerID: "cus_error",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM users WHERE stripe_customer_id`).
					WithArgs("cus_error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.GetByStripeCustomerID(ctx, tt.stripeCustomerID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.validateFn != nil {
					tt.validateFn(t, result)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestUserRepository_Create(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	avatarURL := "https://example.com/avatar.png"

	tests := []struct {
		name      string
		user      *models.User
		setupMock func()
		expectErr bool
	}{
		{
			name: "successful create",
			user: &models.User{
				GoogleID:  strPtr("google-123"),
				Email:     "new@example.com",
				Name:      "New User",
				AvatarURL: &avatarURL,
				CreatedAt: now,
				UpdatedAt: now,
			},
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "subscription_status", "subscription_plan", "default_team_id",
					"onboarding_completed", "onboarding_completed_at", "created_at", "updated_at",
				}).AddRow("user-123", "trial", nil, nil, false, nil, now, now)

				mock.ExpectQuery(`INSERT INTO users`).
					WithArgs(
						"google-123",
						sqlmock.AnyArg(), // idp_provider
						sqlmock.AnyArg(), // idp_subject
						"new@example.com",
						"New User",
						&avatarURL,
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
					).
					WillReturnRows(rows)
			},
			expectErr: false,
		},
		{
			name: "database error - duplicate email",
			user: &models.User{
				GoogleID:  strPtr("google-dup"),
				Email:     "duplicate@example.com",
				Name:      "Duplicate User",
				CreatedAt: now,
				UpdatedAt: now,
			},
			setupMock: func() {
				mock.ExpectQuery(`INSERT INTO users`).
					WithArgs(
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

			err := repo.Create(ctx, tt.user)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, tt.user.ID)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestUserRepository_Update(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name      string
		user      *models.User
		setupMock func()
		expectErr bool
	}{
		{
			name: "successful update",
			user: &models.User{
				ID:        "user-123",
				GoogleID:  strPtr("google-123"),
				Email:     "updated@example.com",
				Name:      "Updated User",
				Version:   1,
				UpdatedAt: now,
			},
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "subscription_status", "trial_ends_at", "subscription_plan", "subscription_canceled_at",
					"default_team_id", "onboarding_completed", "onboarding_completed_at", "created_at", "updated_at", "version",
				}).AddRow("user-123", "active", nil, nil, nil, nil, false, nil, now, now, 2)

				// Update query now uses id ($1) not google_id
				mock.ExpectQuery(`UPDATE users SET`).
					WithArgs(
						"user-123", // id ($1)
						"updated@example.com",
						"Updated User",
						sqlmock.AnyArg(), // avatar_url
						sqlmock.AnyArg(), // idp_provider
						sqlmock.AnyArg(), // idp_subject
						sqlmock.AnyArg(), // updated_at
						int64(1),         // version
					).
					WillReturnRows(rows)
			},
			expectErr: false,
		},
		{
			name: "version mismatch",
			user: &models.User{
				ID:        "user-old",
				GoogleID:  strPtr("google-old"),
				Email:     "old@example.com",
				Name:      "Old User",
				Version:   1,
				UpdatedAt: now,
			},
			setupMock: func() {
				mock.ExpectQuery(`UPDATE users SET`).
					WithArgs(
						sqlmock.AnyArg(), // id
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						int64(1),
					).
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
		},
		{
			name: "database error",
			user: &models.User{
				ID:        "user-error",
				GoogleID:  strPtr("google-error"),
				Email:     "error@example.com",
				Name:      "Error User",
				Version:   1,
				UpdatedAt: now,
			},
			setupMock: func() {
				mock.ExpectQuery(`UPDATE users SET`).
					WithArgs(
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						int64(1),
					).
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.Update(ctx, tt.user)

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
func TestUserRepository_UpdateSubscriptionStatus(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	proPlan := "pro"

	tests := []struct {
		name      string
		userID    string
		status    string
		plan      *string
		setupMock func()
		expectErr bool
	}{
		{
			name:   "successful update with plan",
			userID: "user-123",
			status: "active",
			plan:   &proPlan,
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET subscription_status`).
					WithArgs("user-123", "active", &proPlan).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:   "successful update without plan",
			userID: "user-456",
			status: "trial",
			plan:   nil,
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET subscription_status`).
					WithArgs("user-456", "trial", nil).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:   "database error",
			userID: "user-error",
			status: "active",
			plan:   nil,
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET subscription_status`).
					WithArgs("user-error", "active", nil).
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.UpdateSubscriptionStatus(ctx, tt.userID, tt.status, tt.plan)

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
func TestUserRepository_UpdateSubscriptionStatusWithTrial(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	trialEnd := time.Now().Add(14 * 24 * time.Hour)
	proPlan := "pro"

	tests := []struct {
		name      string
		userID    string
		status    string
		plan      *string
		trialEnd  *time.Time
		setupMock func()
		expectErr bool
	}{
		{
			name:     "successful update with all fields",
			userID:   "user-123",
			status:   "trialing",
			plan:     &proPlan,
			trialEnd: &trialEnd,
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET`).
					WithArgs("trialing", proPlan, sqlmock.AnyArg(), "user-123").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:     "user not found",
			userID:   "user-notfound",
			status:   "active",
			plan:     nil,
			trialEnd: nil,
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET`).
					WithArgs("active", nil, nil, "user-notfound").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			expectErr: true,
		},
		{
			name:     "database error",
			userID:   "user-error",
			status:   "active",
			plan:     nil,
			trialEnd: nil,
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET`).
					WithArgs("active", nil, nil, "user-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.UpdateSubscriptionStatusWithTrial(ctx, tt.userID, tt.status, tt.plan, tt.trialEnd)

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
func TestUserRepository_UpdateStripeCustomerID(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	tests := []struct {
		name       string
		userID     string
		customerID string
		setupMock  func()
		expectErr  bool
	}{
		{
			name:       "successful update",
			userID:     "user-123",
			customerID: "cus_new123",
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET stripe_customer_id`).
					WithArgs("user-123", "cus_new123").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:       "database error",
			userID:     "user-error",
			customerID: "cus_error",
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET stripe_customer_id`).
					WithArgs("user-error", "cus_error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.UpdateStripeCustomerID(ctx, tt.userID, tt.customerID)

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
func TestUserRepository_UpdateTrialEndsAt(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	trialEndsAt := time.Now().Add(14 * 24 * time.Hour)

	tests := []struct {
		name        string
		userID      string
		trialEndsAt *time.Time
		setupMock   func()
		expectErr   bool
	}{
		{
			name:        "successful update with date",
			userID:      "user-123",
			trialEndsAt: &trialEndsAt,
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET trial_ends_at`).
					WithArgs("user-123", sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:        "successful update to nil",
			userID:      "user-456",
			trialEndsAt: nil,
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET trial_ends_at`).
					WithArgs("user-456", nil).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:        "database error",
			userID:      "user-error",
			trialEndsAt: &trialEndsAt,
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET trial_ends_at`).
					WithArgs("user-error", sqlmock.AnyArg()).
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.UpdateTrialEndsAt(ctx, tt.userID, tt.trialEndsAt)

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
func TestUserRepository_UpdateDefaultTeamID(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	tests := []struct {
		name      string
		userID    string
		teamID    string
		setupMock func()
		expectErr bool
	}{
		{
			name:   "successful update",
			userID: "user-123",
			teamID: "team-456",
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET default_team_id`).
					WithArgs("user-123", "team-456").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:   "database error",
			userID: "user-error",
			teamID: "team-error",
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET default_team_id`).
					WithArgs("user-error", "team-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.UpdateDefaultTeamID(ctx, tt.userID, tt.teamID)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestUserRepository_UpdateSubscriptionStatusWithTrial_RowsAffectedError tests rows affected error
func TestUserRepository_UpdateSubscriptionStatusWithTrial_RowsAffectedError(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	mock.ExpectExec(`UPDATE users SET`).
		WithArgs("active", nil, nil, "user-123").
		WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

	err := repo.UpdateSubscriptionStatusWithTrial(ctx, "user-123", "active", nil, nil)

	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

//nolint:funlen // table-driven test with multiple test cases
func TestUserRepository_MarkOnboardingCompleted(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	tests := []struct {
		name      string
		userID    string
		setupMock func()
		expectErr bool
	}{
		{
			name:   "successful mark onboarding completed",
			userID: "user-123",
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET onboarding_completed`).
					WithArgs("user-123").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:   "idempotent - already completed",
			userID: "user-456",
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET onboarding_completed`).
					WithArgs("user-456").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:   "user not found",
			userID: "user-notfound",
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET onboarding_completed`).
					WithArgs("user-notfound").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			expectErr: true,
		},
		{
			name:   "database error",
			userID: "user-error",
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET onboarding_completed`).
					WithArgs("user-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.MarkOnboardingCompleted(ctx, tt.userID)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestUserRepository_MarkOnboardingCompleted_RowsAffectedError tests rows affected error
func TestUserRepository_MarkOnboardingCompleted_RowsAffectedError(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	mock.ExpectExec(`UPDATE users SET onboarding_completed`).
		WithArgs("user-123").
		WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

	err := repo.MarkOnboardingCompleted(ctx, "user-123")

	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestUserRepository_UpdateSubscriptionWithCancellation tests subscription cancellation tracking
//
//nolint:funlen // Table-driven test with multiple test cases for cancellation tracking
func TestUserRepository_UpdateSubscriptionWithCancellation(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	plan := "professional"
	trialEnd := time.Now().Add(7 * 24 * time.Hour)
	canceledAt := time.Now()

	tests := []struct {
		name       string
		userID     string
		status     string
		plan       *string
		trialEnd   *time.Time
		canceledAt *time.Time
		setupMock  func()
		expectErr  bool
	}{
		{
			name:       "update with cancellation",
			userID:     "user-123",
			status:     "active",
			plan:       &plan,
			trialEnd:   &trialEnd,
			canceledAt: &canceledAt,
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET`).
					WithArgs("active", plan, trialEnd, canceledAt, "user-123").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:       "clear cancellation (reactivation)",
			userID:     "user-456",
			status:     "active",
			plan:       &plan,
			trialEnd:   nil,
			canceledAt: nil,
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET`).
					WithArgs("active", plan, nil, nil, "user-456").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:       "user not found",
			userID:     "nonexistent",
			status:     "active",
			plan:       &plan,
			trialEnd:   nil,
			canceledAt: &canceledAt,
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET`).
					WithArgs("active", plan, nil, canceledAt, "nonexistent").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			expectErr: true,
		},
		{
			name:       "database error",
			userID:     "user-error",
			status:     "active",
			plan:       &plan,
			trialEnd:   nil,
			canceledAt: &canceledAt,
			setupMock: func() {
				mock.ExpectExec(`UPDATE users SET`).
					WithArgs("active", plan, nil, canceledAt, "user-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.UpdateSubscriptionWithCancellation(ctx, tt.userID, tt.status, tt.plan, tt.trialEnd, tt.canceledAt)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
