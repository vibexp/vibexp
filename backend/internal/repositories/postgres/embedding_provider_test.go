package postgres

import (
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

func setupEmbeddingProviderTest(t *testing.T) (*EmbeddingProviderRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := NewEmbeddingProviderRepository(db).(*EmbeddingProviderRepository)

	return repo, mock, mockDB
}

// embeddingProviderListColumns mirrors the 10 columns selected by List
// (no version column).
var embeddingProviderListColumns = []string{
	"id", "user_id", "team_id", "name", "provider_type", "model", "chunk_size",
	"chunk_overlap", "is_default", "base_url",
	"api_key_encrypted", "configuration", "created_at", "updated_at",
}

//nolint:funlen // table-driven test with multiple test cases
func TestEmbeddingProviderRepository_List(t *testing.T) {
	repo, mock, mockDB := setupEmbeddingProviderTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()
	now := time.Now()
	openAI := "openai"
	empty := ""

	tests := []struct {
		name        string
		userID      string
		filters     repositories.EmbeddingProviderFilters
		setupMock   func()
		expectErr   bool
		expectCount int
		expectTotal int
	}{
		{
			name:   "unfiltered list binds only user_id and orders by default",
			userID: "user-123",
			filters: repositories.EmbeddingProviderFilters{
				Page:  1,
				Limit: 10,
			},
			setupMock: func() {
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM embedding_providers WHERE \(user_id = \$1\)`).
					WithArgs("user-123").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows(embeddingProviderListColumns).
					AddRow("p1", "user-123", nil, "Default", "openai", "m", 1000, 200, true, nil, "enc", "{}", now, now).
					AddRow("p2", "user-123", nil, "Other", "ollama", "m", 1000, 200, false, nil, "enc", "{}", now, now)

				// LIMIT/OFFSET are inlined literals by squirrel, so they are NOT bound args.
				// The baseline scenario pins the exact 14-column projection (no version)
				// so projection drift fails loudly as this template is copied to other repos.
				mock.ExpectQuery(
					`SELECT id, user_id, team_id, name, provider_type, model, chunk_size, ` +
						`chunk_overlap, is_default, base_url, ` +
						`api_key_encrypted, configuration, created_at, updated_at ` +
						`FROM embedding_providers WHERE \(user_id = \$1\) ` +
						`ORDER BY is_default DESC, created_at DESC LIMIT 10 OFFSET 0`,
				).
					WithArgs("user-123").
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCount: 2,
			expectTotal: 2,
		},
		{
			name:   "provider_type filter binds user_id and provider_type",
			userID: "user-123",
			filters: repositories.EmbeddingProviderFilters{
				ProviderType: &openAI,
				Page:         1,
				Limit:        10,
			},
			setupMock: func() {
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
				mock.ExpectQuery(
					`SELECT COUNT\(\*\) FROM embedding_providers WHERE \(user_id = \$1 AND provider_type = \$2\)`,
				).
					WithArgs("user-123", "openai").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows(embeddingProviderListColumns).
					AddRow("p1", "user-123", nil, "Default", "openai", "m", 1000, 200, true, nil, "enc", "{}", now, now)

				mock.ExpectQuery(
					`SELECT .+ FROM embedding_providers `+
						`WHERE \(user_id = \$1 AND provider_type = \$2\) `+
						`ORDER BY is_default DESC, created_at DESC LIMIT 10 OFFSET 0`,
				).
					WithArgs("user-123", "openai").
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCount: 1,
			expectTotal: 1,
		},
		{
			name:   "empty provider_type is treated as no filter",
			userID: "user-123",
			filters: repositories.EmbeddingProviderFilters{
				ProviderType: &empty,
				Page:         1,
				Limit:        10,
			},
			setupMock: func() {
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(0)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM embedding_providers WHERE \(user_id = \$1\)`).
					WithArgs("user-123").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows(embeddingProviderListColumns)
				mock.ExpectQuery(`SELECT .+ FROM embedding_providers WHERE \(user_id = \$1\)`).
					WithArgs("user-123").
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCount: 0,
			expectTotal: 0,
		},
		{
			name:   "pagination computes offset from page and limit",
			userID: "user-123",
			filters: repositories.EmbeddingProviderFilters{
				Page:  3,
				Limit: 5,
			},
			setupMock: func() {
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(20)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM embedding_providers WHERE \(user_id = \$1\)`).
					WithArgs("user-123").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows(embeddingProviderListColumns).
					AddRow("p11", "user-123", nil, "Eleven", "openai", "m", 1000, 200, false, nil, "enc", "{}", now, now)

				// offset = (3-1)*5 = 10
				mock.ExpectQuery(
					`SELECT .+ FROM embedding_providers WHERE \(user_id = \$1\) ` +
						`ORDER BY is_default DESC, created_at DESC LIMIT 5 OFFSET 10`,
				).
					WithArgs("user-123").
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCount: 1,
			expectTotal: 20,
		},
		{
			name:   "non-positive page and limit clamp to LIMIT 0 OFFSET 0",
			userID: "user-123",
			filters: repositories.EmbeddingProviderFilters{
				Page:  0,
				Limit: -5,
			},
			setupMock: func() {
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(3)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM embedding_providers WHERE \(user_id = \$1\)`).
					WithArgs("user-123").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows(embeddingProviderListColumns)
				// Negative inputs must clamp to zero, never wrap to huge uint64 values.
				mock.ExpectQuery(
					`SELECT .+ FROM embedding_providers WHERE \(user_id = \$1\) ` +
						`ORDER BY is_default DESC, created_at DESC LIMIT 0 OFFSET 0`,
				).
					WithArgs("user-123").
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCount: 0,
			expectTotal: 3,
		},
		{
			name:   "empty result returns non-nil empty slice",
			userID: "user-empty",
			filters: repositories.EmbeddingProviderFilters{
				Page:  1,
				Limit: 10,
			},
			setupMock: func() {
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(0)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM embedding_providers WHERE \(user_id = \$1\)`).
					WithArgs("user-empty").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows(embeddingProviderListColumns)
				mock.ExpectQuery(`SELECT .+ FROM embedding_providers WHERE \(user_id = \$1\)`).
					WithArgs("user-empty").
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCount: 0,
			expectTotal: 0,
		},
		{
			name:   "count query error propagates wrapped",
			userID: "user-err",
			filters: repositories.EmbeddingProviderFilters{
				Page:  1,
				Limit: 10,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM embedding_providers`).
					WithArgs("user-err").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr:   true,
			expectCount: 0,
			expectTotal: 0,
		},
		{
			name:   "list query error propagates wrapped",
			userID: "user-123",
			filters: repositories.EmbeddingProviderFilters{
				Page:  1,
				Limit: 10,
			},
			setupMock: func() {
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM embedding_providers`).
					WithArgs("user-123").
					WillReturnRows(countRows)

				mock.ExpectQuery(`SELECT .+ FROM embedding_providers`).
					WithArgs("user-123").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr:   true,
			expectCount: 0,
			expectTotal: 0,
		},
		{
			name:   "scan error propagates wrapped",
			userID: "user-123",
			filters: repositories.EmbeddingProviderFilters{
				Page:  1,
				Limit: 10,
			},
			setupMock: func() {
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM embedding_providers`).
					WithArgs("user-123").
					WillReturnRows(countRows)

				// One fewer column than the scan expects forces a scan error.
				badRows := sqlmock.NewRows([]string{"id"}).AddRow("p1")
				mock.ExpectQuery(`SELECT .+ FROM embedding_providers`).
					WithArgs("user-123").
					WillReturnRows(badRows)
			},
			expectErr:   true,
			expectCount: 0,
			expectTotal: 0,
		},
		{
			name:   "row iteration error propagates wrapped",
			userID: "user-123",
			filters: repositories.EmbeddingProviderFilters{
				Page:  1,
				Limit: 10,
			},
			setupMock: func() {
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM embedding_providers`).
					WithArgs("user-123").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows(embeddingProviderListColumns).
					AddRow("p1", "user-123", nil, "Default", "openai", "m", 1000, 200, true, nil, "enc", "{}", now, now).
					RowError(0, sql.ErrConnDone)
				mock.ExpectQuery(`SELECT .+ FROM embedding_providers`).
					WithArgs("user-123").
					WillReturnRows(rows)
			},
			expectErr:   true,
			expectCount: 0,
			expectTotal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			providers, total, err := repo.List(ctx, tt.userID, tt.filters)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, providers)
				assert.Zero(t, total)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, providers, "List must return a non-nil empty slice, never nil")
				assert.Len(t, providers, tt.expectCount)
				assert.Equal(t, tt.expectTotal, total)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
