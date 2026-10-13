package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
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
					"id", "google_id", "idp_provider", "idp_subject", "email", "email_verified_at", "name", "avatar_url",
					"default_team_id", "status", "onboarding_completed", "onboarding_completed_at",
					"created_at", "updated_at", "version", "session_epoch",
				}).AddRow(
					"user-123", "google-123", nil, nil, "user@example.com", nil, "Test User", &avatarURL,
					&defaultTeamID, "active", false, nil,
					now, now, 1, 0,
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
			},
		},
		{
			name:   "successful retrieval with minimal fields",
			userID: "user-456",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "google_id", "idp_provider", "idp_subject", "email", "email_verified_at", "name", "avatar_url",
					"default_team_id", "status", "onboarding_completed", "onboarding_completed_at",
					"created_at", "updated_at", "version", "session_epoch",
				}).AddRow(
					"user-456", "google-456", nil, nil, "minimal@example.com", nil, "Minimal User", nil,
					nil, "active", false, nil,
					now, now, 1, 0,
				)

				mock.ExpectQuery(`SELECT .+ FROM users WHERE id`).
					WithArgs("user-456").
					WillReturnRows(rows)
			},
			expectErr: false,
			validateFn: func(t *testing.T, user *models.User) {
				assert.Equal(t, "user-456", user.ID)
				assert.Nil(t, user.AvatarURL)
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
					"id", "google_id", "idp_provider", "idp_subject", "email", "email_verified_at", "name", "avatar_url",
					"default_team_id", "status", "onboarding_completed", "onboarding_completed_at",
					"created_at", "updated_at", "version", "session_epoch",
				}).AddRow(
					"user-123", "google-123", nil, nil, "user@example.com", nil, "Test User", nil,
					nil, "active", false, nil,
					now, now, 1, 0,
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

// TestUserRepository_GetByIDPSubject pins that a sign-in identity is resolved
// through user_identities, not the users.idp_* columns (#1427).
func TestUserRepository_GetByIDPSubject(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	const lookup = `SELECT .+ FROM user_identities ui\s+JOIN users u ON u.id = ui.user_id\s+` +
		`WHERE ui.provider = \$1 AND ui.subject = \$2`

	t.Run("found through the identity", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"id", "google_id", "idp_provider", "idp_subject", "email", "email_verified_at", "name", "avatar_url",
			"default_team_id", "status", "onboarding_completed", "onboarding_completed_at",
			"created_at", "updated_at", "version", "session_epoch",
		}).AddRow(
			"user-123", nil, "corp-sso", "sub-1", "user@example.com", now, "Test User", nil,
			nil, "active", false, nil,
			now, now, 1, 0,
		)
		mock.ExpectQuery(lookup).WithArgs("corp-sso", "sub-1").WillReturnRows(rows)

		user, err := repo.GetByIDPSubject(ctx, "corp-sso", "sub-1")
		require.NoError(t, err)
		assert.Equal(t, "user-123", user.ID)
		require.NotNil(t, user.EmailVerifiedAt)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no such identity is ErrUserNotFound", func(t *testing.T) {
		mock.ExpectQuery(lookup).WithArgs(models.LegacyGoogleIdentityProvider, "google-404").
			WillReturnError(sql.ErrNoRows)

		user, err := repo.GetByIDPSubject(ctx, models.LegacyGoogleIdentityProvider, "google-404")
		require.ErrorIs(t, err, repositories.ErrUserNotFound)
		assert.Nil(t, user)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a database error is not a miss", func(t *testing.T) {
		mock.ExpectQuery(lookup).WithArgs("corp-sso", "sub-err").WillReturnError(sql.ErrConnDone)

		_, err := repo.GetByIDPSubject(ctx, "corp-sso", "sub-err")
		require.Error(t, err)
		assert.NotErrorIs(t, err, repositories.ErrUserNotFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
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
					"id", "default_team_id",
					"onboarding_completed", "onboarding_completed_at", "created_at", "updated_at",
				}).AddRow("user-123", nil, false, nil, now, now)

				mock.ExpectQuery(`INSERT INTO users`).
					WithArgs(
						"google-123",
						sqlmock.AnyArg(), // idp_provider
						sqlmock.AnyArg(), // idp_subject
						"new@example.com",
						sqlmock.AnyArg(), // email_verified_at
						"New User",
						&avatarURL,
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						models.LegacyGoogleIdentityProvider,
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
					"id",
					"default_team_id", "onboarding_completed", "onboarding_completed_at", "created_at", "updated_at", "version",
				}).AddRow("user-123", nil, false, nil, now, now, 2)

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
						sqlmock.AnyArg(), // email_verified_at
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
						sqlmock.AnyArg(),
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

// TestUserRepository_Create_DuplicateEmailIsDomainError pins the mapping that
// lets admin user creation answer 409 instead of 500. The unique constraint is
// the only place two concurrent inserts are actually serialized, so this — not a
// pre-check — is what makes the conflict reliable.
func TestUserRepository_Create_DuplicateEmailIsDomainError(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`INSERT INTO users`).
		WillReturnError(&pq.Error{Code: "23505", Constraint: "users_email_key"})

	err := repo.Create(context.Background(), &models.User{
		Email: "taken@example.com", Name: "Someone",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, repositories.ErrUserEmailTaken,
		"a 23505 unique violation must surface as ErrUserEmailTaken so callers can 409")
	assert.Contains(t, err.Error(), "taken@example.com")
}

// TestUserRepository_Create_OtherErrorsAreNotConflicts keeps a genuine failure
// from being misreported as a duplicate email.
func TestUserRepository_Create_OtherErrorsAreNotConflicts(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`INSERT INTO users`).WillReturnError(errors.New("connection reset"))

	err := repo.Create(context.Background(), &models.User{Email: "a@example.com", Name: "A"})
	require.Error(t, err)
	assert.NotErrorIs(t, err, repositories.ErrUserEmailTaken)
}

// TestUserRepository_DeleteByID covers the compensating rollback used by admin
// user creation when publishing `user.created` fails.
func TestUserRepository_DeleteByID(t *testing.T) {
	tests := []struct {
		name    string
		result  driver.Result
		execErr error
		wantErr error
	}{
		{name: "deletes", result: sqlmock.NewResult(0, 1)},
		{name: "no such user", result: sqlmock.NewResult(0, 0), wantErr: repositories.ErrUserNotFound},
		{name: "exec fails", execErr: errors.New("boom")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock, mockDB := setupUserTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			exp := mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).WithArgs("u1")
			if tc.execErr != nil {
				exp.WillReturnError(tc.execErr)
			} else {
				exp.WillReturnResult(tc.result)
			}

			err := repo.DeleteByID(context.Background(), "u1")
			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.execErr != nil:
				require.Error(t, err)
			default:
				require.NoError(t, err)
			}
		})
	}
}

// TestUserRepository_Create_IdentityConflictIsNotATakenEmail: Create also
// writes user_identities, so a unique violation there must not be reported as a
// duplicate email (#1427).
func TestUserRepository_Create_IdentityConflictIsNotATakenEmail(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`INSERT INTO users`).
		WillReturnError(&pq.Error{Code: "23505", Constraint: "user_identities_provider_subject_key"})

	err := repo.Create(context.Background(), &models.User{
		Email: "a@example.com", Name: "A", IDPProvider: strPtr("corp-sso"), IDPSubject: strPtr("sub-1"),
	})
	require.Error(t, err)
	assert.NotErrorIs(t, err, repositories.ErrUserEmailTaken)
}

// TestUserRepository_NormalizesEmail pins that every email the repository
// reads by or writes is the normalized form, whatever casing the caller passes
// (#1427), and that the model is updated to what was stored.
func TestUserRepository_NormalizesEmail(t *testing.T) {
	repo, mock, mockDB := setupUserTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()
	ctx := context.Background()
	now := time.Now()

	t.Run("GetByEmail", func(t *testing.T) {
		mock.ExpectQuery(`SELECT .+ FROM users WHERE email = \$1`).
			WithArgs("alice@example.com").WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByEmail(ctx, "  Alice@Example.COM ")
		require.ErrorIs(t, err, repositories.ErrUserNotFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Create", func(t *testing.T) {
		mock.ExpectQuery(`INSERT INTO users`).
			WithArgs(nil, nil, nil, "alice@example.com", nil, "Alice", nil, now, now,
				models.LegacyGoogleIdentityProvider).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "default_team_id", "onboarding_completed", "onboarding_completed_at", "created_at", "updated_at",
			}).AddRow("user-1", nil, false, nil, now, now))

		user := &models.User{Email: "Alice@Example.COM", Name: "Alice", CreatedAt: now, UpdatedAt: now}
		require.NoError(t, repo.Create(ctx, user))
		assert.Equal(t, "alice@example.com", user.Email)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Update", func(t *testing.T) {
		mock.ExpectQuery(`UPDATE users SET`).
			WithArgs("user-1", "alice@example.com", "Alice", nil, nil, nil, sqlmock.AnyArg(), int64(1), &now).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "default_team_id", "onboarding_completed", "onboarding_completed_at",
				"created_at", "updated_at", "version",
			}).AddRow("user-1", nil, false, nil, now, now, 2))

		user := &models.User{ID: "user-1", Email: "Alice@Example.COM", Name: "Alice", Version: 1, EmailVerifiedAt: &now}
		require.NoError(t, repo.Update(ctx, user))
		assert.Equal(t, "alice@example.com", user.Email)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestUserRepository_BumpSessionEpoch(t *testing.T) {
	ctx := context.Background()

	t.Run("returns the incremented epoch", func(t *testing.T) {
		repo, mock, mockDB := setupUserTest(t)
		defer func() {
			if closeErr := mockDB.Close(); closeErr != nil {
				t.Logf("Failed to close mock DB: %v", closeErr)
			}
		}()

		mock.ExpectQuery(`UPDATE users\s+SET session_epoch = session_epoch \+ 1\s+WHERE id = \$1\s+RETURNING session_epoch`).
			WithArgs("user-123").
			WillReturnRows(sqlmock.NewRows([]string{"session_epoch"}).AddRow(int64(4)))

		epoch, err := repo.BumpSessionEpoch(ctx, "user-123")

		require.NoError(t, err)
		assert.Equal(t, int64(4), epoch)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("unknown user is ErrUserNotFound", func(t *testing.T) {
		repo, mock, mockDB := setupUserTest(t)
		defer func() {
			if closeErr := mockDB.Close(); closeErr != nil {
				t.Logf("Failed to close mock DB: %v", closeErr)
			}
		}()

		mock.ExpectQuery(`UPDATE users\s+SET session_epoch`).
			WithArgs("user-missing").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.BumpSessionEpoch(ctx, "user-missing")

		require.ErrorIs(t, err, repositories.ErrUserNotFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("database failure is not reported as not found", func(t *testing.T) {
		repo, mock, mockDB := setupUserTest(t)
		defer func() {
			if closeErr := mockDB.Close(); closeErr != nil {
				t.Logf("Failed to close mock DB: %v", closeErr)
			}
		}()

		mock.ExpectQuery(`UPDATE users\s+SET session_epoch`).
			WithArgs("user-123").
			WillReturnError(sql.ErrConnDone)

		_, err := repo.BumpSessionEpoch(ctx, "user-123")

		require.Error(t, err)
		assert.NotErrorIs(t, err, repositories.ErrUserNotFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
