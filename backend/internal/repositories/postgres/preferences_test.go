package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

const selectPrefsQuery = `SELECT id, user_id, preferences, ` +
	`created_at, updated_at, version FROM user_preferences WHERE user_id = \$1`

func setupPreferencesTest(t *testing.T) (*UserPreferencesRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := NewUserPreferencesRepository(db)

	return repo, mock, mockDB
}

//nolint:funlen // table-driven test with multiple test cases
func TestUserPreferencesRepository_GetByUserID(t *testing.T) {
	repo, mock, mockDB := setupPreferencesTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	prefs := models.Preferences{
		EmailNotification: models.EmailNotificationPreferences{
			PlatformAnnouncement: true,
			AccountSecurity:      true,
			NewFeature:           false,
			MarketingPromotional: false,
		},
	}
	prefsJSON, err := json.Marshal(prefs)
	require.NoError(t, err)

	tests := []struct {
		name       string
		userID     string
		setupMock  func()
		expectErr  bool
		expectNil  bool
		validateFn func(*testing.T, *models.UserPreferences)
	}{
		{
			name:   "successful retrieval",
			userID: "user-123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "user_id", "preferences", "created_at", "updated_at", "version",
				}).AddRow("pref-123", "user-123", prefsJSON, now, now, 1)

				mock.ExpectQuery(selectPrefsQuery).
					WithArgs("user-123").
					WillReturnRows(rows)
			},
			expectErr: false,
			expectNil: false,
			validateFn: func(t *testing.T, p *models.UserPreferences) {
				assert.Equal(t, "pref-123", p.ID)
				assert.Equal(t, "user-123", p.UserID)
				assert.True(t, p.Preferences.EmailNotification.PlatformAnnouncement)
				assert.True(t, p.Preferences.EmailNotification.AccountSecurity)
				assert.False(t, p.Preferences.EmailNotification.NewFeature)
				assert.False(t, p.Preferences.EmailNotification.MarketingPromotional)
			},
		},
		{
			name:   "not found returns nil without error",
			userID: "user-notfound",
			setupMock: func() {
				mock.ExpectQuery(selectPrefsQuery).
					WithArgs("user-notfound").
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: false,
			expectNil: true,
		},
		{
			name:   "database error",
			userID: "user-error",
			setupMock: func() {
				mock.ExpectQuery(selectPrefsQuery).
					WithArgs("user-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.GetByUserID(ctx, tt.userID)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectNil {
				assert.Nil(t, result)
			} else if tt.validateFn != nil {
				tt.validateFn(t, result)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestUserPreferencesRepository_Upsert(t *testing.T) {
	repo, mock, mockDB := setupPreferencesTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name      string
		prefs     *models.UserPreferences
		setupMock func()
		expectErr bool
	}{
		{
			name: "successful insert",
			prefs: &models.UserPreferences{
				UserID: "user-new",
				Preferences: models.Preferences{
					EmailNotification: models.EmailNotificationPreferences{
						PlatformAnnouncement: true,
						AccountSecurity:      true,
						NewFeature:           true,
						MarketingPromotional: false,
					},
				},
			},
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"id", "created_at", "version"}).
					AddRow("pref-new", now, 1)

				mock.ExpectQuery(`INSERT INTO user_preferences`).
					WithArgs(
						sqlmock.AnyArg(), // id
						"user-new",       // user_id
						sqlmock.AnyArg(), // preferences JSON
						sqlmock.AnyArg(), // created_at
						sqlmock.AnyArg(), // updated_at
					).
					WillReturnRows(rows)
			},
			expectErr: false,
		},
		{
			name: "successful update on conflict",
			prefs: &models.UserPreferences{
				ID:     "pref-existing",
				UserID: "user-existing",
				Preferences: models.Preferences{
					EmailNotification: models.EmailNotificationPreferences{
						PlatformAnnouncement: false,
						AccountSecurity:      true,
						NewFeature:           false,
						MarketingPromotional: true,
					},
				},
			},
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"id", "created_at", "version"}).
					AddRow("pref-existing", now, 2) // version incremented

				mock.ExpectQuery(`INSERT INTO user_preferences`).
					WithArgs(
						"pref-existing",
						"user-existing",
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
					).
					WillReturnRows(rows)
			},
			expectErr: false,
		},
		{
			name: "database error",
			prefs: &models.UserPreferences{
				UserID: "user-error",
				Preferences: models.Preferences{
					EmailNotification: models.DefaultPreferences().EmailNotification,
				},
			},
			setupMock: func() {
				mock.ExpectQuery(`INSERT INTO user_preferences`).
					WithArgs(
						sqlmock.AnyArg(),
						"user-error",
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

			err := repo.Upsert(ctx, tt.prefs)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, tt.prefs.ID)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUserPreferencesRepository_JSONSerialization(t *testing.T) {
	// Test that preferences are correctly serialized/deserialized as JSONB
	repo, mock, mockDB := setupPreferencesTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	// Create preferences with all fields set to specific values
	originalPrefs := models.Preferences{
		EmailNotification: models.EmailNotificationPreferences{
			PlatformAnnouncement: true,
			AccountSecurity:      true,
			NewFeature:           false,
			MarketingPromotional: true,
		},
	}
	prefsJSON, err := json.Marshal(originalPrefs)
	require.NoError(t, err)

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "preferences", "created_at", "updated_at", "version",
	}).AddRow("pref-json", "user-json", prefsJSON, now, now, 1)

	mock.ExpectQuery(selectPrefsQuery).
		WithArgs("user-json").
		WillReturnRows(rows)

	result, err := repo.GetByUserID(ctx, "user-json")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, originalPrefs, result.Preferences)
	assert.NoError(t, mock.ExpectationsWereMet())
}
