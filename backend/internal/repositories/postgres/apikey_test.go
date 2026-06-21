package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

func setupAPIKeyTest(t *testing.T) (*APIKeyRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := NewAPIKeyRepository(db).(*APIKeyRepository)

	return repo, mock, mockDB
}

//nolint:funlen // table-driven test with multiple test cases
func TestAPIKeyRepository_Create(t *testing.T) {
	repo, mock, mockDB := setupAPIKeyTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name      string
		apiKey    *models.APIKey
		setupMock func()
		expectErr bool
	}{
		{
			name: "successful create with integrations",
			apiKey: &models.APIKey{
				Name:         "Test API Key",
				UserID:       "user-123",
				KeyHash:      "hash-abc123",
				KeyPrefix:    "vxk_abc",
				Integrations: []string{"ai_tools", "cli"},
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			setupMock: func() {
				mock.ExpectBegin()

				// Insert API key
				rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at", "version"}).
					AddRow("key-123", now, now, 1)

				mock.ExpectQuery(`INSERT INTO api_keys`).
					WithArgs(
						"Test API Key",
						"user-123",
						"hash-abc123",
						"vxk_abc",
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
					).
					WillReturnRows(rows)

				// Insert integration permissions
				mock.ExpectExec(`INSERT INTO api_key_integration_permissions`).
					WithArgs("key-123", "ai_tools").
					WillReturnResult(sqlmock.NewResult(1, 1))

				mock.ExpectExec(`INSERT INTO api_key_integration_permissions`).
					WithArgs("key-123", "cli").
					WillReturnResult(sqlmock.NewResult(1, 1))

				mock.ExpectCommit()
			},
			expectErr: false,
		},
		{
			name: "successful create without integrations",
			apiKey: &models.APIKey{
				Name:         "Simple Key",
				UserID:       "user-456",
				KeyHash:      "hash-def456",
				KeyPrefix:    "vxk_def",
				Integrations: []string{},
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			setupMock: func() {
				mock.ExpectBegin()

				rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at", "version"}).
					AddRow("key-456", now, now, 1)

				mock.ExpectQuery(`INSERT INTO api_keys`).
					WithArgs(
						"Simple Key",
						"user-456",
						"hash-def456",
						"vxk_def",
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
					).
					WillReturnRows(rows)

				mock.ExpectCommit()
			},
			expectErr: false,
		},
		{
			name: "database error on insert",
			apiKey: &models.APIKey{
				Name:      "Error Key",
				UserID:    "user-error",
				KeyHash:   "hash-error",
				KeyPrefix: "vxk_err",
				CreatedAt: now,
				UpdatedAt: now,
			},
			setupMock: func() {
				mock.ExpectBegin()

				mock.ExpectQuery(`INSERT INTO api_keys`).
					WithArgs(
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
					).
					WillReturnError(sql.ErrConnDone)

				mock.ExpectRollback()
			},
			expectErr: true,
		},
		{
			name: "error on begin transaction",
			apiKey: &models.APIKey{
				Name:      "Begin Error",
				UserID:    "user-begin",
				KeyHash:   "hash-begin",
				KeyPrefix: "vxk_beg",
				CreatedAt: now,
				UpdatedAt: now,
			},
			setupMock: func() {
				mock.ExpectBegin().WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.Create(ctx, tt.apiKey)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, tt.apiKey.ID)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestAPIKeyRepository_GetByUserID(t *testing.T) {
	repo, mock, mockDB := setupAPIKeyTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	usageType := "ai_tools"

	tests := []struct {
		name        string
		userID      string
		setupMock   func()
		expectErr   bool
		expectCount int
	}{
		{
			name:   "successful retrieval with multiple keys",
			userID: "user-123",
			setupMock: func() {
				// Main query for API keys
				rows := sqlmock.NewRows([]string{
					"id", "name", "user_id", "key_hash", "key_prefix", "usage_type",
					"is_legacy", "migration_notes", "last_used_at", "expires_at", "created_at", "updated_at", "version",
				}).AddRow(
					"key-1", "Key 1", "user-123", "hash-1", "vxk_1", usageType,
					false, nil, nil, nil, now, now, 1,
				).AddRow(
					"key-2", "Key 2", "user-123", "hash-2", "vxk_2", usageType,
					false, nil, nil, nil, now, now, 1,
				)

				mock.ExpectQuery(`SELECT .+ FROM api_keys WHERE user_id`).
					WithArgs("user-123").
					WillReturnRows(rows)

				// Integration queries for each key
				intRows1 := sqlmock.NewRows([]string{"integration_code"}).
					AddRow("ai_tools").AddRow("cli")
				mock.ExpectQuery(`SELECT integration_code FROM api_key_integration_permissions WHERE api_key_id`).
					WithArgs("key-1").
					WillReturnRows(intRows1)

				intRows2 := sqlmock.NewRows([]string{"integration_code"}).
					AddRow("mcp_server")
				mock.ExpectQuery(`SELECT integration_code FROM api_key_integration_permissions WHERE api_key_id`).
					WithArgs("key-2").
					WillReturnRows(intRows2)
			},
			expectErr:   false,
			expectCount: 2,
		},
		{
			name:   "empty list",
			userID: "user-empty",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "name", "user_id", "key_hash", "key_prefix", "usage_type",
					"is_legacy", "migration_notes", "last_used_at", "expires_at", "created_at", "updated_at", "version",
				})

				mock.ExpectQuery(`SELECT .+ FROM api_keys WHERE user_id`).
					WithArgs("user-empty").
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCount: 0,
		},
		{
			name:   "database error",
			userID: "user-error",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM api_keys WHERE user_id`).
					WithArgs("user-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr:   true,
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			keys, err := repo.GetByUserID(ctx, tt.userID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, keys)
			} else {
				assert.NoError(t, err)
				assert.Len(t, keys, tt.expectCount)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestAPIKeyRepository_GetByKeyHash(t *testing.T) {
	repo, mock, mockDB := setupAPIKeyTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	usageType := "ai_tools"

	tests := []struct {
		name       string
		keyHash    string
		setupMock  func()
		expectErr  bool
		validateFn func(*testing.T, *models.APIKey)
	}{
		{
			name:    "successful retrieval",
			keyHash: "hash-abc123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "name", "user_id", "key_hash", "key_prefix", "usage_type",
					"is_legacy", "migration_notes", "last_used_at", "expires_at", "created_at", "updated_at", "version",
				}).AddRow(
					"key-123", "Test Key", "user-123", "hash-abc123", "vxk_abc", usageType,
					false, nil, nil, nil, now, now, 1,
				)

				mock.ExpectQuery(`SELECT .+ FROM api_keys WHERE key_hash`).
					WithArgs("hash-abc123").
					WillReturnRows(rows)

				intRows := sqlmock.NewRows([]string{"integration_code"}).
					AddRow("ai_tools").AddRow("cli")
				mock.ExpectQuery(`SELECT integration_code FROM api_key_integration_permissions WHERE api_key_id`).
					WithArgs("key-123").
					WillReturnRows(intRows)
			},
			expectErr: false,
			validateFn: func(t *testing.T, key *models.APIKey) {
				assert.Equal(t, "key-123", key.ID)
				assert.Equal(t, "hash-abc123", key.KeyHash)
				assert.Len(t, key.Integrations, 2)
			},
		},
		{
			name:    "not found",
			keyHash: "hash-notfound",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM api_keys WHERE key_hash`).
					WithArgs("hash-notfound").
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
		},
		{
			name:    "database error",
			keyHash: "hash-error",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM api_keys WHERE key_hash`).
					WithArgs("hash-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
		{
			name:    "error loading integrations",
			keyHash: "hash-interror",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "name", "user_id", "key_hash", "key_prefix", "usage_type",
					"is_legacy", "migration_notes", "last_used_at", "expires_at", "created_at", "updated_at", "version",
				}).AddRow(
					"key-interr", "Test Key", "user-123", "hash-interror", "vxk_int", usageType,
					false, nil, nil, nil, now, now, 1,
				)

				mock.ExpectQuery(`SELECT .+ FROM api_keys WHERE key_hash`).
					WithArgs("hash-interror").
					WillReturnRows(rows)

				mock.ExpectQuery(`SELECT integration_code FROM api_key_integration_permissions WHERE api_key_id`).
					WithArgs("key-interr").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.GetByKeyHash(ctx, tt.keyHash)

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

// TestAPIKeyRepository_GetByKeyHash_Expiry verifies that GetByKeyHash enforces the
// expiry clause: an expired key is excluded by the SQL WHERE filter (returns no rows,
// surfaced as "not found"), while a key with a future expiry is returned with its
// ExpiresAt populated.
func TestAPIKeyRepository_GetByKeyHash_Expiry(t *testing.T) {
	repo, mock, mockDB := setupAPIKeyTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	future := now.Add(24 * time.Hour)

	t.Run("future expiry returned with ExpiresAt set", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"id", "name", "user_id", "key_hash", "key_prefix", "usage_type",
			"is_legacy", "migration_notes", "last_used_at", "expires_at", "created_at", "updated_at", "version",
		}).AddRow(
			"key-future", "Future Key", "user-1", "hash-future", "vxk_f", "ai_tools",
			false, nil, nil, future, now, now, 1,
		)
		// The query must carry the expiry guard so expired rows never match.
		mock.ExpectQuery(`SELECT .+ FROM api_keys WHERE key_hash = \$1\s+AND \(expires_at IS NULL OR expires_at > NOW\(\)\)`).
			WithArgs("hash-future").
			WillReturnRows(rows)
		mock.ExpectQuery(`SELECT integration_code FROM api_key_integration_permissions WHERE api_key_id`).
			WithArgs("key-future").
			WillReturnRows(sqlmock.NewRows([]string{"integration_code"}).AddRow("cli"))

		result, err := repo.GetByKeyHash(ctx, "hash-future")

		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.ExpiresAt)
		assert.WithinDuration(t, future, *result.ExpiresAt, time.Second)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("expired key excluded by WHERE clause is not found", func(t *testing.T) {
		// Postgres applies the expires_at guard, so an expired key returns no rows.
		mock.ExpectQuery(`SELECT .+ FROM api_keys WHERE key_hash = \$1\s+AND \(expires_at IS NULL OR expires_at > NOW\(\)\)`).
			WithArgs("hash-expired").
			WillReturnError(sql.ErrNoRows)

		result, err := repo.GetByKeyHash(ctx, "hash-expired")

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

//nolint:funlen // table-driven test with multiple test cases
func TestAPIKeyRepository_Delete(t *testing.T) {
	repo, mock, mockDB := setupAPIKeyTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	tests := []struct {
		name      string
		userID    string
		keyID     string
		setupMock func()
		expectErr bool
	}{
		{
			name:   "successful delete",
			userID: "user-123",
			keyID:  "key-123",
			setupMock: func() {
				mock.ExpectExec(`DELETE FROM api_keys WHERE id`).
					WithArgs("key-123", "user-123").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:   "not found",
			userID: "user-123",
			keyID:  "key-notfound",
			setupMock: func() {
				mock.ExpectExec(`DELETE FROM api_keys WHERE id`).
					WithArgs("key-notfound", "user-123").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			expectErr: true,
		},
		{
			name:   "database error",
			userID: "user-123",
			keyID:  "key-error",
			setupMock: func() {
				mock.ExpectExec(`DELETE FROM api_keys WHERE id`).
					WithArgs("key-error", "user-123").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
		{
			name:   "rows affected error",
			userID: "user-123",
			keyID:  "key-rowserr",
			setupMock: func() {
				mock.ExpectExec(`DELETE FROM api_keys WHERE id`).
					WithArgs("key-rowserr", "user-123").
					WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.Delete(ctx, tt.userID, tt.keyID)

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
func TestAPIKeyRepository_UpdateLastUsed(t *testing.T) {
	repo, mock, mockDB := setupAPIKeyTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name       string
		keyID      string
		lastUsedAt time.Time
		setupMock  func()
		expectErr  bool
	}{
		{
			name:       "successful update",
			keyID:      "key-123",
			lastUsedAt: now,
			setupMock: func() {
				mock.ExpectExec(`UPDATE api_keys SET last_used_at`).
					WithArgs("key-123", sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:       "database error",
			keyID:      "key-error",
			lastUsedAt: now,
			setupMock: func() {
				mock.ExpectExec(`UPDATE api_keys SET last_used_at`).
					WithArgs("key-error", sqlmock.AnyArg()).
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.UpdateLastUsed(ctx, tt.keyID, tt.lastUsedAt)

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
func TestAPIKeyRepository_GetIntegrationsByAPIKeyID(t *testing.T) {
	repo, mock, mockDB := setupAPIKeyTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	tests := []struct {
		name        string
		apiKeyID    string
		setupMock   func()
		expectErr   bool
		expectCount int
	}{
		{
			name:     "successful retrieval with multiple integrations",
			apiKeyID: "key-123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"integration_code"}).
					AddRow("ai_tools").
					AddRow("cli").
					AddRow("mcp_server")

				mock.ExpectQuery(`SELECT integration_code FROM api_key_integration_permissions WHERE api_key_id`).
					WithArgs("key-123").
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCount: 3,
		},
		{
			name:     "empty list",
			apiKeyID: "key-empty",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"integration_code"})

				mock.ExpectQuery(`SELECT integration_code FROM api_key_integration_permissions WHERE api_key_id`).
					WithArgs("key-empty").
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCount: 0,
		},
		{
			name:     "database error",
			apiKeyID: "key-error",
			setupMock: func() {
				mock.ExpectQuery(`SELECT integration_code FROM api_key_integration_permissions WHERE api_key_id`).
					WithArgs("key-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr:   true,
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			integrations, err := repo.GetIntegrationsByAPIKeyID(ctx, tt.apiKeyID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, integrations)
			} else {
				assert.NoError(t, err)
				assert.Len(t, integrations, tt.expectCount)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestAPIKeyRepository_HasIntegrationPermission(t *testing.T) {
	repo, mock, mockDB := setupAPIKeyTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	tests := []struct {
		name            string
		apiKeyID        string
		integrationCode string
		setupMock       func()
		expectErr       bool
		expectResult    bool
	}{
		{
			name:            "has permission",
			apiKeyID:        "key-123",
			integrationCode: "ai_tools",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"count"}).AddRow(1)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM api_key_integration_permissions`).
					WithArgs("key-123", "ai_tools").
					WillReturnRows(rows)
			},
			expectErr:    false,
			expectResult: true,
		},
		{
			name:            "no permission",
			apiKeyID:        "key-123",
			integrationCode: "cli",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"count"}).AddRow(0)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM api_key_integration_permissions`).
					WithArgs("key-123", "cli").
					WillReturnRows(rows)
			},
			expectErr:    false,
			expectResult: false,
		},
		{
			name:            "database error",
			apiKeyID:        "key-error",
			integrationCode: "ai_tools",
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM api_key_integration_permissions`).
					WithArgs("key-error", "ai_tools").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr:    true,
			expectResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.HasIntegrationPermission(ctx, tt.apiKeyID, tt.integrationCode)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectResult, result)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestAPIKeyRepository_GetValidIntegrationCodes(t *testing.T) {
	repo, mock, mockDB := setupAPIKeyTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	tests := []struct {
		name        string
		setupMock   func()
		expectErr   bool
		expectCodes []string
	}{
		{
			name: "successful retrieval",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"integration_code"}).
					AddRow("ai_tools").
					AddRow("cli").
					AddRow("mcp_server")

				mock.ExpectQuery(`SELECT integration_code FROM api_key_integrations_catalog WHERE is_active`).
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCodes: []string{"ai_tools", "cli", "mcp_server"},
		},
		{
			name: "empty catalog",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"integration_code"})

				mock.ExpectQuery(`SELECT integration_code FROM api_key_integrations_catalog WHERE is_active`).
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCodes: []string{},
		},
		{
			name: "database error",
			setupMock: func() {
				mock.ExpectQuery(`SELECT integration_code FROM api_key_integrations_catalog WHERE is_active`).
					WillReturnError(sql.ErrConnDone)
			},
			expectErr:   true,
			expectCodes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			codes, err := repo.GetValidIntegrationCodes(ctx)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, codes)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectCodes, codes)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestAPIKeyRepository_GetByUserID_ScanError tests scan error handling
func TestAPIKeyRepository_GetByUserID_ScanError(t *testing.T) {
	repo, mock, mockDB := setupAPIKeyTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	// Return rows with invalid data type to trigger scan error
	rows := sqlmock.NewRows([]string{
		"id", "name", "user_id", "key_hash", "key_prefix", "usage_type",
		"is_legacy", "migration_notes", "last_used_at", "expires_at", "created_at", "updated_at", "version",
	}).AddRow(
		"key-1", "Key 1", "user-123", "hash-1", "vxk_1", "ai_tools",
		"invalid_bool", nil, nil, nil, time.Now(), time.Now(), 1, // invalid bool
	)

	mock.ExpectQuery(`SELECT .+ FROM api_keys WHERE user_id`).
		WithArgs("user-123").
		WillReturnRows(rows)

	keys, err := repo.GetByUserID(ctx, "user-123")

	assert.Error(t, err)
	assert.Nil(t, keys)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAPIKeyRepository_GetIntegrationsByAPIKeyID_RowsIterationError tests row iteration error
func TestAPIKeyRepository_GetIntegrationsByAPIKeyID_RowsIterationError(t *testing.T) {
	repo, mock, mockDB := setupAPIKeyTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"integration_code"}).
		AddRow("ai_tools").
		RowError(0, driver.ErrBadConn)

	mock.ExpectQuery(`SELECT integration_code FROM api_key_integration_permissions WHERE api_key_id`).
		WithArgs("key-123").
		WillReturnRows(rows)

	integrations, err := repo.GetIntegrationsByAPIKeyID(ctx, "key-123")

	assert.Error(t, err)
	assert.Nil(t, integrations)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAPIKeyRepository_GetValidIntegrationCodes_RowsIterationError tests row iteration error
func TestAPIKeyRepository_GetValidIntegrationCodes_RowsIterationError(t *testing.T) {
	repo, mock, mockDB := setupAPIKeyTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"integration_code"}).
		AddRow("ai_tools").
		RowError(0, driver.ErrBadConn)

	mock.ExpectQuery(`SELECT integration_code FROM api_key_integrations_catalog WHERE is_active`).
		WillReturnRows(rows)

	codes, err := repo.GetValidIntegrationCodes(ctx)

	assert.Error(t, err)
	assert.Nil(t, codes)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAPIKeyRepository_Create_IntegrationInsertError tests error during integration insertion
func TestAPIKeyRepository_Create_IntegrationInsertError(t *testing.T) {
	repo, mock, mockDB := setupAPIKeyTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	apiKey := &models.APIKey{
		Name:         "Test Key",
		UserID:       "user-123",
		KeyHash:      "hash-abc123",
		KeyPrefix:    "vxk_abc",
		Integrations: []string{"ai_tools"},
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	mock.ExpectBegin()

	rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at", "version"}).
		AddRow("key-123", now, now, 1)

	mock.ExpectQuery(`INSERT INTO api_keys`).
		WithArgs(
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnRows(rows)

	mock.ExpectExec(`INSERT INTO api_key_integration_permissions`).
		WithArgs("key-123", "ai_tools").
		WillReturnError(sql.ErrConnDone)

	mock.ExpectRollback()

	err := repo.Create(ctx, apiKey)

	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAPIKeyRepository_Create_CommitError tests error during commit
func TestAPIKeyRepository_Create_CommitError(t *testing.T) {
	repo, mock, mockDB := setupAPIKeyTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	apiKey := &models.APIKey{
		Name:         "Test Key",
		UserID:       "user-123",
		KeyHash:      "hash-abc123",
		KeyPrefix:    "vxk_abc",
		Integrations: []string{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	mock.ExpectBegin()

	rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at", "version"}).
		AddRow("key-123", now, now, 1)

	mock.ExpectQuery(`INSERT INTO api_keys`).
		WithArgs(
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnRows(rows)

	mock.ExpectCommit().WillReturnError(sql.ErrConnDone)

	err := repo.Create(ctx, apiKey)

	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
