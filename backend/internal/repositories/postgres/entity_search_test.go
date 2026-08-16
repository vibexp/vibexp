package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Unit coverage for the shared keyword ladder (#813). What Postgres actually
// does with these queries — tokenization, ranking, index usage — is proven in
// team_project_search_integration_test.go; what is proven HERE is the wiring
// sqlmock can see: which passes run, in what order, with which arguments, and
// what happens on each error path.

func setupLadderTest(t *testing.T) (*database.DB, sqlmock.Sqlmock) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})
	return &database.DB{DB: mockDB}, mock
}

// countingScan records how many passes ran, returning found rows only for the
// pass at hitIndex (-1 = every pass comes back empty).
func countingScan(hitIndex int, calls *int) scanRowsFunc {
	return func(rows *sql.Rows) (int, error) {
		idx := *calls
		*calls++
		found := 0
		for rows.Next() {
			found++
		}
		if idx == hitIndex {
			return found, nil
		}
		return 0, nil
	}
}

func expectLadderBegin(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec(`set_config\('pg_trgm.word_similarity_threshold'`).
		WithArgs(keywordTrgmThreshold).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

// TestRunSearchLadder_StopsAtFirstMatchingPass is the ladder's defining
// property: a precise query keeps its precise ranking because looser passes
// never run once one has matched.
func TestRunSearchLadder_StopsAtFirstMatchingPass(t *testing.T) {
	db, mock := setupLadderTest(t)
	expectLadderBegin(mock)
	// Only ONE query is expected. sqlmock fails the test if the ladder issues a
	// second, which is exactly the regression this guards.
	mock.ExpectQuery(`SELECT t.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "score"}).AddRow("team-1", 1.0))
	mock.ExpectRollback()

	calls := 0
	err := runSearchLadder(context.Background(), db, teamSearchSpec(),
		searchInputs{query: "acme", userID: "user-1", limit: 10}, countingScan(0, &calls))

	require.NoError(t, err)
	assert.Equal(t, 1, calls, "the exact pass matched, so no later pass may run")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestRunSearchLadder_FallsThroughToTrigram walks the whole ladder: three empty
// passes then a match on the fourth.
func TestRunSearchLadder_FallsThroughToTrigram(t *testing.T) {
	db, mock := setupLadderTest(t)
	expectLadderBegin(mock)
	for i := 0; i < 3; i++ {
		mock.ExpectQuery(`SELECT t.id`).WillReturnRows(sqlmock.NewRows([]string{"id", "score"}))
	}
	mock.ExpectQuery(`word_similarity`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "score"}).AddRow("team-1", 0.42))
	mock.ExpectRollback()

	calls := 0
	err := runSearchLadder(context.Background(), db, teamSearchSpec(),
		searchInputs{query: "vibxep", userID: "user-1", limit: 10}, countingScan(3, &calls))

	require.NoError(t, err)
	assert.Equal(t, 4, calls, "every pass must run when none before it matched")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestRunSearchLadder_AllPassesEmpty is the no-results case: the ladder runs out
// without an error, and the caller sees an empty set rather than a failure.
func TestRunSearchLadder_AllPassesEmpty(t *testing.T) {
	db, mock := setupLadderTest(t)
	expectLadderBegin(mock)
	for i := 0; i < 4; i++ {
		mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"id", "score"}))
	}
	mock.ExpectRollback()

	calls := 0
	err := runSearchLadder(context.Background(), db, teamSearchSpec(),
		searchInputs{query: "nothing matches", userID: "user-1", limit: 10}, countingScan(-1, &calls))

	require.NoError(t, err)
	assert.Equal(t, 4, calls)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRunSearchLadder_ErrorPaths(t *testing.T) {
	t.Run("begin fails", func(t *testing.T) {
		db, mock := setupLadderTest(t)
		mock.ExpectBegin().WillReturnError(errors.New("no connection"))

		err := runSearchLadder(context.Background(), db, teamSearchSpec(),
			searchInputs{query: "acme", userID: "user-1", limit: 10},
			func(*sql.Rows) (int, error) { return 0, nil })

		require.Error(t, err)
		assert.Contains(t, err.Error(), "begin entity search transaction")
	})

	t.Run("threshold set fails", func(t *testing.T) {
		db, mock := setupLadderTest(t)
		mock.ExpectBegin()
		mock.ExpectExec(`set_config`).WillReturnError(errors.New("permission denied"))
		mock.ExpectRollback()

		err := runSearchLadder(context.Background(), db, teamSearchSpec(),
			searchInputs{query: "acme", userID: "user-1", limit: 10},
			func(*sql.Rows) (int, error) { return 0, nil })

		require.Error(t, err)
		assert.Contains(t, err.Error(), "trgm word_similarity threshold")
	})

	t.Run("a pass query fails, naming the pass", func(t *testing.T) {
		db, mock := setupLadderTest(t)
		expectLadderBegin(mock)
		mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"id", "score"}))
		mock.ExpectQuery(`SELECT`).WillReturnError(errors.New("syntax error"))
		mock.ExpectRollback()

		calls := 0
		err := runSearchLadder(context.Background(), db, teamSearchSpec(),
			searchInputs{query: "acme", userID: "user-1", limit: 10}, countingScan(-1, &calls))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "strict search pass failed",
			"the error must say WHICH rung broke")
	})

	t.Run("scan fails", func(t *testing.T) {
		db, mock := setupLadderTest(t)
		expectLadderBegin(mock)
		mock.ExpectQuery(`SELECT`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "score"}).AddRow("team-1", 1.0))
		mock.ExpectRollback()

		err := runSearchLadder(context.Background(), db, teamSearchSpec(),
			searchInputs{query: "acme", userID: "user-1", limit: 10},
			func(*sql.Rows) (int, error) { return 0, errors.New("bad column") })

		require.Error(t, err)
		assert.Contains(t, err.Error(), "bad column")
	})
}

// TestBuildPasses_ArgumentsMatchPlaceholders is the guard for the 42P18 trap:
// Postgres rejects a statement that binds a parameter it never references
// ("could not determine data type of parameter"), so only the exact pass may
// carry the uuid argument. Counting distinct placeholders against len(args) is
// what makes a future edit to either side fail loudly.
func TestBuildPasses_ArgumentsMatchPlaceholders(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec entitySearchSpec
		in   searchInputs
	}{
		{
			name: "teams",
			spec: teamSearchSpec(),
			in:   searchInputs{query: "q", userID: "u", uuidArg: nil, limit: 10},
		},
		{
			name: "projects cross-team",
			spec: projectSearchSpec(),
			in:   searchInputs{query: "q", userID: "u", uuidArg: nil, teamArg: nil, limit: 10},
		},
		{
			name: "projects narrowed",
			spec: projectSearchSpec(),
			in: searchInputs{
				query: "q", userID: "u", uuidArg: nil, teamArg: "team-1", limit: 10,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			passes := tc.spec.buildPasses(tc.in)
			require.Len(t, passes, 4, "exact, strict, relaxed, trigram")

			for _, pass := range passes {
				highest := 0
				for i := 1; i <= 9; i++ {
					if strings.Contains(pass.query, "$"+string(rune('0'+i))) {
						highest = i
					}
				}
				assert.Equal(t, len(pass.args), highest,
					"pass %q binds %d args but references up to $%d — an unreferenced "+
						"parameter is rejected by Postgres as 42P18; query was:\n%s",
					pass.name, len(pass.args), highest, pass.query)
				assert.NotContains(t, pass.query, "$T", "the team placeholder must be rewritten")
			}

			// The uuid rides on the exact pass alone.
			assert.Contains(t, passes[0].query, "$3::uuid")
			for _, pass := range passes[1:] {
				assert.NotContains(t, pass.query, "::uuid IS NOT NULL",
					"only the exact pass matches on id")
			}
		})
	}
}

// TestBuildPasses_ExpressionsMatchMigration016 keeps the query expressions
// byte-identical to the index expressions in 016_team_project_search.up.sql. The
// integration suite proves the planner agrees; this catches the drift in the
// cheap layer, and states the two strings that must move together.
func TestBuildPasses_ExpressionsMatchMigration016(t *testing.T) {
	for _, tc := range []struct {
		name     string
		spec     entitySearchSpec
		wantFTS  string
		wantTrgm string
	}{
		{
			name: "teams", spec: teamSearchSpec(),
			wantFTS:  "to_tsvector('english', coalesce(t.name, '') || ' ' || coalesce(t.description, ''))",
			wantTrgm: "coalesce(t.name, '')",
		},
		{
			name: "projects", spec: projectSearchSpec(),
			wantFTS:  "to_tsvector('english', coalesce(p.name, '') || ' ' || coalesce(p.description, ''))",
			wantTrgm: "coalesce(p.name, '')",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantFTS, tc.spec.ftsMatchExpr())
			assert.Equal(t, tc.wantTrgm, tc.spec.trgmNameExpr())
		})
	}
}

// TestExactPredicate_CastsTheParameterNotTheColumn pins the #812 lesson in the
// place it would most easily be undone: casting `id::text` to match a text
// parameter is correct SQL that no index can serve.
func TestExactPredicate_CastsTheParameterNotTheColumn(t *testing.T) {
	teams := teamSearchSpec().exactPredicate()
	assert.Contains(t, teams, "($3::uuid IS NOT NULL AND t.id = $3::uuid)")
	assert.NotContains(t, teams, "::text", "casting the column would kill the index")

	projects := projectSearchSpec().exactPredicate()
	assert.Contains(t, projects, "p.git_url = $1", "an agent pasting a clone URL must land on score 1.0")
	assert.NotContains(t, projects, "::text")
}

func TestNullableTeamID(t *testing.T) {
	assert.Nil(t, nullableTeamID(""), "an empty team means every team the caller can read")
	assert.Nil(t, nullableTeamID("   "))
	assert.Equal(t, "team-1", nullableTeamID("team-1"))
}

// setupProjectSearchTest builds a ProjectRepository over sqlmock. The existing
// project unit tests live in the external postgres_test package; this one is
// internal because it drives the unexported ladder alongside the exported method.
func setupProjectSearchTest(t *testing.T) (*ProjectRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	return NewProjectRepository(&database.DB{DB: mockDB}).(*ProjectRepository), mock, mockDB
}

// TestProjectRepository_SearchProjects covers the repository half of the
// cross-team project ladder. Row-level tenancy and ranking are proven against a
// real server in team_project_search_integration_test.go.
func TestProjectRepository_SearchProjects(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("blank query short-circuits without touching the database", func(t *testing.T) {
		repo, mock, mockDB := setupProjectSearchTest(t)
		defer func() {
			if closeErr := mockDB.Close(); closeErr != nil {
				t.Logf("Failed to close mock DB: %v", closeErr)
			}
		}()

		results, err := repo.SearchProjects(ctx, "user-1", repositories.ProjectSearchFilters{Limit: 10})

		require.NoError(t, err)
		assert.Empty(t, results)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("cross-team search binds a NULL team filter", func(t *testing.T) {
		repo, mock, mockDB := setupProjectSearchTest(t)
		defer func() {
			if closeErr := mockDB.Close(); closeErr != nil {
				t.Logf("Failed to close mock DB: %v", closeErr)
			}
		}()

		mock.ExpectBegin()
		mock.ExpectExec(`set_config`).WillReturnResult(sqlmock.NewResult(0, 0))
		// $3 is the uuid (nil here), $4 the team filter (nil = every team).
		mock.ExpectQuery(`SELECT p.id`).
			WithArgs("revenue", "user-1", nil, nil, 10).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "user_id", "team_id", "name", "slug", "description",
				"git_url", "homepage", "created_at", "updated_at", "version", "score",
			}).AddRow("proj-1", "user-2", "team-9", "Revenue", "revenue", "d", "", "", now, now, int64(1), 0.5))
		mock.ExpectRollback()

		results, err := repo.SearchProjects(ctx, "user-1",
			repositories.ProjectSearchFilters{Query: "revenue", Limit: 10})

		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "team-9", results[0].TeamID, "a project from another of the caller's teams")
		assert.InDelta(t, 0.5, results[0].Score, 0.0001)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a team filter narrows the same query", func(t *testing.T) {
		repo, mock, mockDB := setupProjectSearchTest(t)
		defer func() {
			if closeErr := mockDB.Close(); closeErr != nil {
				t.Logf("Failed to close mock DB: %v", closeErr)
			}
		}()

		mock.ExpectBegin()
		mock.ExpectExec(`set_config`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT p.id`).
			WithArgs("revenue", "user-1", nil, "team-9", 10).
			WillReturnRows(sqlmock.NewRows([]string{"id", "score"}))
		mock.ExpectQuery(`SELECT p.id`).WillReturnRows(sqlmock.NewRows([]string{"id", "score"}))
		mock.ExpectQuery(`SELECT p.id`).WillReturnRows(sqlmock.NewRows([]string{"id", "score"}))
		mock.ExpectQuery(`SELECT p.id`).WillReturnRows(sqlmock.NewRows([]string{"id", "score"}))
		mock.ExpectRollback()

		results, err := repo.SearchProjects(ctx, "user-1",
			repositories.ProjectSearchFilters{Query: "revenue", TeamID: "team-9", Limit: 10})

		require.NoError(t, err)
		assert.Empty(t, results)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestClampSearchLimit pins the zero-value trap: `LIMIT 0` is valid SQL that
// returns nothing, so an unset limit must not reach the database unchanged or a
// caller who omitted the field sees an empty result rather than an error, and
// cannot tell it from a genuine miss. Negative values are worse still — Postgres
// rejects them outright.
func TestClampSearchLimit(t *testing.T) {
	assert.Equal(t, defaultSearchLimit, clampSearchLimit(0), "the zero value must not mean LIMIT 0")
	assert.Equal(t, defaultSearchLimit, clampSearchLimit(-5), "Postgres rejects a negative LIMIT")
	assert.Equal(t, 10, clampSearchLimit(10))
	assert.Equal(t, maxSearchLimit, clampSearchLimit(maxSearchLimit+1))
	assert.Equal(t, maxSearchLimit, clampSearchLimit(maxSearchLimit))
}

// TestBuildPasses_ClampsTheLimitItBinds verifies the clamp is applied where the
// argument is actually bound, not merely available as a helper.
func TestBuildPasses_ClampsTheLimitItBinds(t *testing.T) {
	passes := teamSearchSpec().buildPasses(searchInputs{query: "q", userID: "u", limit: 0})
	for _, pass := range passes {
		assert.Equal(t, defaultSearchLimit, pass.args[len(pass.args)-1],
			"pass %q must bind the clamped limit", pass.name)
	}
}
