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
	"github.com/vibexp/vibexp/internal/repositories"
)

func setupEmbeddingProviderTest(t *testing.T) (*EmbeddingProviderRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := NewEmbeddingProviderRepository(db).(*EmbeddingProviderRepository)

	return repo, mock, mockDB
}

// embeddingProviderListColumns mirrors the columns selected by List
// (no version column).
var embeddingProviderListColumns = []string{
	"id", "user_id", "team_id", "name", "provider_type", "model", "chunk_size",
	"chunk_overlap", "concurrency", "query_prefix", "document_prefix", "is_default",
	"base_url", "api_key_encrypted", "configuration", "created_at", "updated_at",
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
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM embedding_providers WHERE \(team_id = \$1\)`).
					WithArgs("user-123").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows(embeddingProviderListColumns).
					AddRow("p1", "user-123", nil, "Default", "openai", "m", 1000, 200, 1, nil, nil, true, nil, "enc", "{}", now, now).
					AddRow("p2", "user-123", nil, "Other", "ollama", "m", 1000, 200, 1, nil, nil, false, nil, "enc", "{}", now, now)

				// LIMIT/OFFSET are inlined literals by squirrel, so they are NOT bound args.
				// The baseline scenario pins the exact 17-column projection (no version)
				// so projection drift fails loudly as this template is copied to other repos.
				mock.ExpectQuery(
					`SELECT id, user_id, team_id, name, provider_type, model, chunk_size, ` +
						`chunk_overlap, concurrency, query_prefix, document_prefix, is_default, ` +
						`base_url, api_key_encrypted, configuration, created_at, updated_at ` +
						`FROM embedding_providers WHERE \(team_id = \$1\) ` +
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
					`SELECT COUNT\(\*\) FROM embedding_providers WHERE \(team_id = \$1 AND provider_type = \$2\)`,
				).
					WithArgs("user-123", "openai").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows(embeddingProviderListColumns).
					AddRow("p1", "user-123", nil, "Default", "openai", "m", 1000, 200, 1, nil, nil, true, nil, "enc", "{}", now, now)

				mock.ExpectQuery(
					`SELECT .+ FROM embedding_providers `+
						`WHERE \(team_id = \$1 AND provider_type = \$2\) `+
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
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM embedding_providers WHERE \(team_id = \$1\)`).
					WithArgs("user-123").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows(embeddingProviderListColumns)
				mock.ExpectQuery(`SELECT .+ FROM embedding_providers WHERE \(team_id = \$1\)`).
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
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM embedding_providers WHERE \(team_id = \$1\)`).
					WithArgs("user-123").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows(embeddingProviderListColumns).
					AddRow("p11", "user-123", nil, "Eleven", "openai", "m", 1000, 200, 1, nil, nil, false, nil, "enc", "{}", now, now)

				// offset = (3-1)*5 = 10
				mock.ExpectQuery(
					`SELECT .+ FROM embedding_providers WHERE \(team_id = \$1\) ` +
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
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM embedding_providers WHERE \(team_id = \$1\)`).
					WithArgs("user-123").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows(embeddingProviderListColumns)
				// Negative inputs must clamp to zero, never wrap to huge uint64 values.
				mock.ExpectQuery(
					`SELECT .+ FROM embedding_providers WHERE \(team_id = \$1\) ` +
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
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM embedding_providers WHERE \(team_id = \$1\)`).
					WithArgs("user-empty").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows(embeddingProviderListColumns)
				mock.ExpectQuery(`SELECT .+ FROM embedding_providers WHERE \(team_id = \$1\)`).
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
					AddRow("p1", "user-123", nil, "Default", "openai", "m", 1000, 200, 1, nil, nil, true, nil, "enc", "{}", now, now).
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

// testCipherText is an opaque ciphertext blob: the repository must round-trip
// it verbatim — encryption/decryption is the service layer's job.
const testCipherText = "vault:v1:c2VjcmV0LWNpcGhlcg=="

// embProviderTestTime is the fixed timestamp used by the CRUD tests below.
var embProviderTestTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// embProviderFullColumnsTest mirrors the 18-column projection (with version)
// used by GetByID and GetActiveProvider.
var embProviderFullColumnsTest = []string{
	"id", "user_id", "team_id", "name", "provider_type", "model", "chunk_size",
	"chunk_overlap", "concurrency", "query_prefix", "document_prefix", "is_default",
	"base_url", "api_key_encrypted", "configuration", "created_at", "updated_at", "version",
}

// embProviderFullRow returns one full 18-column provider row.
func embProviderFullRow() *sqlmock.Rows {
	return sqlmock.NewRows(embProviderFullColumnsTest).
		AddRow("prov-1", "user-1", "team-1", "Primary", "openai", "text-embedding-3-small",
			1000, 200, 2, nil, nil, true, nil, testCipherText, "{}",
			embProviderTestTime, embProviderTestTime, int64(3))
}

// embProviderDefaultRow returns one 17-column row (no version) as selected by GetDefault.
func embProviderDefaultRow() *sqlmock.Rows {
	return sqlmock.NewRows(embeddingProviderListColumns).
		AddRow("prov-1", "user-1", "team-1", "Primary", "openai", "text-embedding-3-small",
			1000, 200, 2, nil, nil, true, nil, testCipherText, "{}",
			embProviderTestTime, embProviderTestTime)
}

// testEmbeddingProviderFixture builds the provider written by Create/Update tests.
func testEmbeddingProviderFixture() *models.EmbeddingProvider {
	teamID := "team-1"
	cipher := testCipherText
	return &models.EmbeddingProvider{
		UserID:          "user-1",
		TeamID:          &teamID,
		Name:            "Primary",
		ProviderType:    "openai",
		Model:           "text-embedding-3-small",
		ChunkSize:       1000,
		ChunkOverlap:    200,
		Concurrency:     2,
		IsDefault:       true,
		APIKeyEncrypted: &cipher,
		Configuration:   "{}",
		CreatedAt:       embProviderTestTime,
		UpdatedAt:       embProviderTestTime,
		Version:         3,
	}
}

// providerOpScenario drives one embedding provider repository call.
type providerOpScenario struct {
	name    string
	setup   func(mock sqlmock.Sqlmock)
	wantIs  error
	wantSub string
}

func runProviderCreateScenario(t *testing.T, sc providerOpScenario) {
	repo, mock, mockDB := setupEmbeddingProviderTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	provider := testEmbeddingProviderFixture()
	err := repo.Create(context.Background(), provider)

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, "prov-1", provider.ID)
		require.NotNil(t, provider.APIKeyEncrypted)
		assert.Equal(t, testCipherText, *provider.APIKeyEncrypted,
			"ciphertext must round-trip verbatim — the repository performs no crypto")
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestEmbeddingProviderRepository_Create(t *testing.T) {
	// The 16 bound args, in column order; the verbatim ciphertext is pinned at
	// the SQL boundary via WithArgs.
	expectInsert := func(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
		return mock.ExpectQuery(`INSERT INTO embedding_providers`).
			WithArgs("user-1", "team-1", "Primary", "openai", "text-embedding-3-small",
				1000, 200, 2, nil, nil, true, nil, testCipherText, "{}",
				embProviderTestTime, embProviderTestTime)
	}

	scenarios := []providerOpScenario{
		{
			name: "insert returns id and timestamps",
			setup: func(mock sqlmock.Sqlmock) {
				expectInsert(mock).WillReturnRows(
					sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
						AddRow("prov-1", embProviderTestTime, embProviderTestTime))
			},
		},
		{
			name: "driver error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				expectInsert(mock).WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to create embedding provider",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runProviderCreateScenario(t, sc) })
	}
}

func runProviderGetByIDScenario(t *testing.T, sc providerOpScenario) {
	repo, mock, mockDB := setupEmbeddingProviderTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	provider, err := repo.GetByID(context.Background(), "team-1", "prov-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, "prov-1", provider.ID)
		assert.Equal(t, int64(3), provider.Version)
		assert.True(t, provider.IsDefault)
		require.NotNil(t, provider.APIKeyEncrypted)
		assert.Equal(t, testCipherText, *provider.APIKeyEncrypted)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestEmbeddingProviderRepository_GetByID(t *testing.T) {
	const getRE = `SELECT .+, version FROM embedding_providers WHERE id = \$1 AND team_id = \$2`

	scenarios := []providerOpScenario{
		{
			name: "found returns the provider including version",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getRE).WithArgs("prov-1", "team-1").WillReturnRows(embProviderFullRow())
			},
		},
		{
			name: "no rows maps to the not-found sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getRE).WithArgs("prov-1", "team-1").WillReturnError(sql.ErrNoRows)
			},
			wantIs: repositories.ErrEmbeddingProviderNotFound,
		},
		{
			name: "driver error is wrapped, not the sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getRE).WithArgs("prov-1", "team-1").WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to get embedding provider by ID",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runProviderGetByIDScenario(t, sc) })
	}
}

func runProviderUpdateScenario(t *testing.T, sc providerOpScenario) {
	repo, mock, mockDB := setupEmbeddingProviderTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	provider := testEmbeddingProviderFixture()
	provider.ID = "prov-1"
	err := repo.Update(context.Background(), provider)

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, int64(4), provider.Version, "a successful update must bump the version")
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestEmbeddingProviderRepository_Update(t *testing.T) {
	expectUpdate := func(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
		return mock.ExpectQuery(`UPDATE embedding_providers SET .+ WHERE id = \$1 AND team_id = \$15 AND version = \$16`).
			WithArgs("prov-1", "Primary", "openai", "text-embedding-3-small", 1000, 200, 2,
				nil, nil, true, nil, testCipherText, "{}", embProviderTestTime, "team-1", int64(3))
	}

	scenarios := []providerOpScenario{
		{
			name: "version-checked update bumps the version",
			setup: func(mock sqlmock.Sqlmock) {
				expectUpdate(mock).WillReturnRows(
					sqlmock.NewRows([]string{"updated_at", "version"}).AddRow(embProviderTestTime, int64(4)))
			},
		},
		{
			name: "stale version yields not-found-or-version-mismatch",
			setup: func(mock sqlmock.Sqlmock) {
				expectUpdate(mock).WillReturnError(sql.ErrNoRows)
			},
			wantSub: "embedding provider not found or version mismatch",
		},
		{
			name: "driver error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				expectUpdate(mock).WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to update embedding provider",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runProviderUpdateScenario(t, sc) })
	}
}

func runProviderDeleteScenario(t *testing.T, sc providerOpScenario) {
	repo, mock, mockDB := setupEmbeddingProviderTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	err := repo.Delete(context.Background(), "team-1", "prov-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestEmbeddingProviderRepository_Delete(t *testing.T) {
	const deleteRE = `DELETE FROM embedding_providers WHERE id = \$1 AND team_id = \$2`

	scenarios := []providerOpScenario{
		{
			name: "deletes the provider",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(deleteRE).WithArgs("prov-1", "team-1").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
		},
		{
			name: "zero rows affected maps to the not-found sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(deleteRE).WithArgs("prov-1", "team-1").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantIs: repositories.ErrEmbeddingProviderNotFound,
		},
		{
			name: "exec error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(deleteRE).WithArgs("prov-1", "team-1").WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to delete embedding provider",
		},
		{
			name: "rows-affected error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(deleteRE).WithArgs("prov-1", "team-1").
					WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))
			},
			wantSub: "failed to get rows affected",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runProviderDeleteScenario(t, sc) })
	}
}

func runProviderGetDefaultScenario(t *testing.T, sc providerOpScenario) {
	repo, mock, mockDB := setupEmbeddingProviderTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	provider, err := repo.GetDefault(context.Background(), "team-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, "prov-1", provider.ID)
		assert.True(t, provider.IsDefault)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestEmbeddingProviderRepository_GetDefault(t *testing.T) {
	const getDefaultRE = `SELECT .+ FROM embedding_providers WHERE team_id = \$1 AND is_default = true LIMIT 1`

	scenarios := []providerOpScenario{
		{
			name: "found returns the default provider",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getDefaultRE).WithArgs("team-1").WillReturnRows(embProviderDefaultRow())
			},
		},
		{
			name: "no rows maps to the default-not-found sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getDefaultRE).WithArgs("team-1").WillReturnError(sql.ErrNoRows)
			},
			wantIs: repositories.ErrDefaultEmbeddingProviderNotFound,
		},
		{
			name: "driver error is wrapped, not the sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getDefaultRE).WithArgs("team-1").WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to get default embedding provider",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runProviderGetDefaultScenario(t, sc) })
	}
}

func runProviderGetActiveScenario(t *testing.T, sc providerOpScenario) {
	repo, mock, mockDB := setupEmbeddingProviderTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	provider, err := repo.GetActiveProvider(context.Background(), "team-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, "prov-1", provider.ID)
		assert.Equal(t, int64(3), provider.Version)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestEmbeddingProviderRepository_GetActiveProvider(t *testing.T) {
	// The ordering IS the resolution contract: default-flagged wins, then most
	// recently updated.
	const getActiveRE = `SELECT .+ FROM embedding_providers WHERE team_id = \$1 ` +
		`ORDER BY is_default DESC, updated_at DESC LIMIT 1`

	scenarios := []providerOpScenario{
		{
			name: "resolves default-first, then most recently updated",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getActiveRE).WithArgs("team-1").WillReturnRows(embProviderFullRow())
			},
		},
		{
			name: "no rows maps to the no-active-provider sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getActiveRE).WithArgs("team-1").WillReturnError(sql.ErrNoRows)
			},
			wantIs: repositories.ErrNoActiveEmbeddingProvider,
		},
		{
			name: "driver error is wrapped, not the sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getActiveRE).WithArgs("team-1").WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to get active embedding provider",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runProviderGetActiveScenario(t, sc) })
	}
}

func runProviderSetDefaultScenario(t *testing.T, sc providerOpScenario) {
	repo, mock, mockDB := setupEmbeddingProviderTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	err := repo.SetDefault(context.Background(), "team-1", "prov-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestEmbeddingProviderRepository_SetDefault(t *testing.T) {
	const unsetRE = `UPDATE embedding_providers SET is_default = false WHERE team_id = \$1`
	const setRE = `UPDATE embedding_providers SET is_default = true WHERE id = \$1 AND team_id = \$2`

	scenarios := []providerOpScenario{
		{
			name: "unset-all-then-set-one commits",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(unsetRE).WithArgs("team-1").WillReturnResult(sqlmock.NewResult(0, 2))
				mock.ExpectExec(setRE).WithArgs("prov-1", "team-1").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			},
		},
		{
			name: "missing target rolls back with the not-found sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(unsetRE).WithArgs("team-1").WillReturnResult(sqlmock.NewResult(0, 2))
				mock.ExpectExec(setRE).WithArgs("prov-1", "team-1").WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectRollback()
			},
			wantIs: repositories.ErrEmbeddingProviderNotFound,
		},
		{
			name: "first-step error rolls back",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(unsetRE).WithArgs("team-1").WillReturnError(sql.ErrConnDone)
				mock.ExpectRollback()
			},
			wantSub: "failed to unset default providers",
		},
		{
			name: "begin error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin().WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to begin transaction",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runProviderSetDefaultScenario(t, sc) })
	}
}

func runProviderUnsetAllScenario(t *testing.T, sc providerOpScenario) {
	repo, mock, mockDB := setupEmbeddingProviderTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	err := repo.UnsetAllDefaults(context.Background(), "team-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestEmbeddingProviderRepository_UnsetAllDefaults(t *testing.T) {
	const unsetRE = `UPDATE embedding_providers SET is_default = false WHERE team_id = \$1`

	scenarios := []providerOpScenario{
		{
			name: "unsets every default for the team",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(unsetRE).WithArgs("team-1").WillReturnResult(sqlmock.NewResult(0, 3))
			},
		},
		{
			name: "exec error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(unsetRE).WithArgs("team-1").WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to unset all default providers",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runProviderUnsetAllScenario(t, sc) })
	}
}

func runProviderCountScenario(t *testing.T, sc providerOpScenario) {
	repo, mock, mockDB := setupEmbeddingProviderTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	count, err := repo.Count(context.Background(), "team-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, 4, count)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestEmbeddingProviderRepository_Count(t *testing.T) {
	const countRE = `SELECT COUNT\(\*\) FROM embedding_providers WHERE team_id = \$1`

	scenarios := []providerOpScenario{
		{
			name: "counts the team's providers",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(countRE).WithArgs("team-1").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))
			},
		},
		{
			name: "query error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(countRE).WithArgs("team-1").WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to count embedding providers",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runProviderCountScenario(t, sc) })
	}
}
