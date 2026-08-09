package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

func setupFreshnessCandidateTest(t *testing.T) (*FreshnessCandidateRepository, sqlmock.Sqlmock) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})

	repo := NewFreshnessCandidateRepository(&database.DB{DB: mockDB})
	return repo.(*FreshnessCandidateRepository), mock
}

func candidateQuery() models.FreshnessCandidateQuery {
	return models.FreshnessCandidateQuery{
		TeamID:        "team-1",
		ResourceType:  "prompt",
		ThresholdDays: 30,
		Limit:         2,
	}
}

// The query must scan the resource's own table and carry the resource type back
// on every candidate, since the caller keys freshness state on the pair.
func TestFreshnessCandidateRepository_ListStaleCandidates_ReturnsCandidates(t *testing.T) {
	repo, mock := setupFreshnessCandidateTest(t)

	mock.ExpectQuery(`SELECT id, project_id\s+FROM prompts`).
		WithArgs("team-1", nil, nil, 30, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id"}).
			AddRow("prompt-1", "proj-1").
			AddRow("prompt-2", "proj-2"))

	got, err := repo.ListStaleCandidates(context.Background(), candidateQuery())

	require.NoError(t, err)
	assert.Equal(t, []models.FreshnessCandidate{
		{ResourceType: "prompt", ResourceID: "prompt-1", ProjectID: "proj-1"},
		{ResourceType: "prompt", ResourceID: "prompt-2", ProjectID: "proj-2"},
	}, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

// An "any medium" rule must consider every medium, including `api`: the service
// layer refuses `api` as rule INPUT, but an access through a generic API client
// is still an access and must not leave a resource looking untouched.
func TestFreshnessCandidateRepository_ListStaleCandidates_AnyMediumUsesEveryColumn(t *testing.T) {
	repo, mock := setupFreshnessCandidateTest(t)

	mock.ExpectQuery(
		`GREATEST\(updated_at, last_accessed_web_at, last_accessed_cli_at, ` +
			`last_accessed_mcp_at, last_accessed_api_at\) < now\(\) - make_interval\(days => \$4::integer\)`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id"}))

	_, err := repo.ListStaleCandidates(context.Background(), candidateQuery())

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// A narrowed rule must compare ONLY its own mediums -- otherwise "stale unless
// read in the web app" would be silently satisfied by a CLI read.
func TestFreshnessCandidateRepository_ListStaleCandidates_NarrowedMediums(t *testing.T) {
	repo, mock := setupFreshnessCandidateTest(t)

	mock.ExpectQuery(`GREATEST\(updated_at, last_accessed_web_at, last_accessed_mcp_at\) <`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id"}))

	query := candidateQuery()
	query.Mediums = []string{"web", "mcp"}
	_, err := repo.ListStaleCandidates(context.Background(), query)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// `memories` is the only one of the four resource tables whose updated_at is a
// naive timestamp. Without the conversion, GREATEST would resolve to the naive
// type and reinterpret every timestamptz in the server's timezone.
func TestFreshnessCandidateRepository_ListStaleCandidates_ConvertsNaiveMemoryTimestamp(t *testing.T) {
	repo, mock := setupFreshnessCandidateTest(t)

	mock.ExpectQuery(`FROM memories.+GREATEST\(\(updated_at AT TIME ZONE 'UTC'\), last_accessed_web_at`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id"}))

	query := candidateQuery()
	query.ResourceType = "memory"
	_, err := repo.ListStaleCandidates(context.Background(), query)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// The three optional inputs must reach the database as bound parameters, and
// omitting one must bind NULL rather than an empty string -- the predicates are
// written as `$n IS NULL OR ...`, so an empty string would filter everything out.
func TestFreshnessCandidateRepository_ListStaleCandidates_BindsOptionalFilters(t *testing.T) {
	repo, mock := setupFreshnessCandidateTest(t)

	projectID := "proj-1"
	mock.ExpectQuery(`FROM prompts`).
		WithArgs("team-1", projectID, "prompt-9", 30, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id"}))

	query := candidateQuery()
	query.ProjectID = &projectID
	query.AfterID = "prompt-9"
	_, err := repo.ListStaleCandidates(context.Background(), query)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// A caller that forgets to set a batch size must still get a bounded query,
// not the team's entire resource table.
func TestFreshnessCandidateRepository_ListStaleCandidates_DefaultsTheLimit(t *testing.T) {
	repo, mock := setupFreshnessCandidateTest(t)

	mock.ExpectQuery(`FROM prompts`).
		WithArgs("team-1", nil, nil, 30, defaultFreshnessCandidateLimit).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id"}))

	query := candidateQuery()
	query.Limit = 0
	_, err := repo.ListStaleCandidates(context.Background(), query)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// An unevaluable type or medium must fail loudly and without touching the
// database: silently returning no candidates would under-report staleness with
// nothing in the logs to explain it.
func TestFreshnessCandidateRepository_ListStaleCandidates_RejectsUnsupportedInput(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(q *models.FreshnessCandidateQuery)
		wantErr string
	}{
		{
			name:    "unknown resource type",
			mutate:  func(q *models.FreshnessCandidateQuery) { q.ResourceType = "agent" },
			wantErr: `resource type "agent"`,
		},
		{
			name:    "unknown medium",
			mutate:  func(q *models.FreshnessCandidateQuery) { q.Mediums = []string{"carrier-pigeon"} },
			wantErr: `medium "carrier-pigeon"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := setupFreshnessCandidateTest(t)

			query := candidateQuery()
			tt.mutate(&query)
			_, err := repo.ListStaleCandidates(context.Background(), query)

			require.Error(t, err)
			assert.ErrorIs(t, err, repositories.ErrUnsupportedFreshnessResource)
			assert.Contains(t, err.Error(), tt.wantErr)
			require.NoError(t, mock.ExpectationsWereMet(), "no query should have been issued")
		})
	}
}

func TestFreshnessCandidateRepository_ListStaleCandidates_QueryError(t *testing.T) {
	repo, mock := setupFreshnessCandidateTest(t)

	mock.ExpectQuery(`FROM prompts`).WillReturnError(errors.New("connection reset"))

	_, err := repo.ListStaleCandidates(context.Background(), candidateQuery())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list stale prompt candidates")
}

func TestFreshnessCandidateRepository_ListStaleCandidates_ScanError(t *testing.T) {
	repo, mock := setupFreshnessCandidateTest(t)

	mock.ExpectQuery(`FROM prompts`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("prompt-1"))

	_, err := repo.ListStaleCandidates(context.Background(), candidateQuery())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan stale prompt candidate")
}

func TestFreshnessCandidateRepository_ListStaleCandidates_RowsError(t *testing.T) {
	repo, mock := setupFreshnessCandidateTest(t)

	mock.ExpectQuery(`FROM prompts`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id"}).
			AddRow("prompt-1", "proj-1").
			RowError(0, errors.New("stream broken")))

	_, err := repo.ListStaleCandidates(context.Background(), candidateQuery())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to iterate stale prompt candidates")
}

// The dispatch allowlists must cover exactly the resource types and mediums the
// rest of the system produces. A type missing here is a rule that silently
// evaluates nothing.
func TestFreshnessCandidateTargets_CoverEveryTypeAndMedium(t *testing.T) {
	tables, mediums := freshnessCandidateTargets()

	assert.Equal(t, map[string]string{
		"prompt":    "prompts",
		"artifact":  "artifacts",
		"blueprint": "blueprints",
		"memory":    "memories",
	}, tables)
	assert.Equal(t, map[string]string{
		"web": "last_accessed_web_at",
		"cli": "last_accessed_cli_at",
		"mcp": "last_accessed_mcp_at",
		"api": "last_accessed_api_at",
	}, mediums)

	// The ANY-medium set and the per-medium map must not drift apart: a new
	// medium column that "any" does not include would make a narrowed rule
	// stricter than the unnarrowed one.
	assert.Len(t, freshnessAnyMediumColumns, len(mediums))
	for _, column := range mediums {
		assert.Contains(t, freshnessAnyMediumColumns, column)
	}
}
