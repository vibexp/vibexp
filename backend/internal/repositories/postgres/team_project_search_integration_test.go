//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Behavior-level tests for the team/project keyword ladder against real Postgres
// (#813). Everything here needs a real server: FTS tokenization, trigram
// scoring, the transaction-local similarity threshold and the migration-016
// index expressions are all Postgres behaviour that sqlmock can only pretend to
// have.

// searchFixture is one caller plus the teams and projects around them: two teams
// they can read and one they cannot, so every assertion can also check tenancy.
type searchFixture struct {
	userID      string
	teamID      string // owned by userID, name "VibeXP Platform"
	memberTeam  string // userID is a member, name "Analytics Guild"
	foreignTeam string // userID has no relationship at all
	projectID   string // in teamID, name "shaharia-lab/games-for-agents"
	foreignProj string // in foreignTeam, deliberately named to match every query
}

func seedSearchFixture(t *testing.T) searchFixture {
	t.Helper()
	ctx := context.Background()

	f := searchFixture{userID: insertTestUser(t)}
	otherUser := insertTestUser(t)

	insertTeam := func(ownerID, name, slug, description string) string {
		id := uuid.New().String()
		_, err := integrationDB.ExecContext(ctx,
			"INSERT INTO teams (id, owner_id, name, slug, description) VALUES ($1, $2, $3, $4, $5)",
			id, ownerID, name, slug+"-"+id[:8], description)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = integrationDB.ExecContext(ctx, "DELETE FROM teams WHERE id = $1", id)
		})
		return id
	}

	insertProject := func(teamID, ownerID, name, slug, description, gitURL string) string {
		id := uuid.New().String()
		_, err := integrationDB.ExecContext(ctx,
			`INSERT INTO projects (id, user_id, team_id, name, slug, description, git_url)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			id, ownerID, teamID, name, slug+"-"+id[:8], description, gitURL)
		require.NoError(t, err)
		return id
	}

	f.teamID = insertTeam(f.userID, "VibeXP Platform", "vibexp-platform", "The shared brain for AI tools")
	f.memberTeam = insertTeam(otherUser, "Analytics Guild", "analytics-guild", "Dashboards and reporting")
	f.foreignTeam = insertTeam(otherUser, "VibeXP Platform", "vibexp-platform-foreign", "The shared brain for AI tools")

	_, err := integrationDB.ExecContext(ctx,
		"INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)",
		f.memberTeam, f.userID, models.TeamMemberRoleMember)
	require.NoError(t, err)

	f.projectID = insertProject(f.teamID, f.userID, "shaharia-lab/games-for-agents",
		"games-for-agents", "Chess and other games agents can play", "https://github.com/shaharia-lab/games-for-agents")
	insertProject(f.memberTeam, otherUser, "Revenue Dashboards",
		"revenue-dashboards", "Weekly revenue reporting", "")
	// Named and described to match every query the tests run, so tenancy failures
	// surface as a wrong ROW rather than as an empty result that looks like a
	// different bug.
	f.foreignProj = insertProject(f.foreignTeam, otherUser, "shaharia-lab/games-for-agents",
		"games-for-agents-foreign", "Chess and other games agents can play",
		"https://github.com/shaharia-lab/games-for-agents")

	return f
}

func projectIDs(results []models.ProjectSearchResult) []string {
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	return ids
}

func teamIDs(results []models.TeamSearchResult) []string {
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	return ids
}

// TestIntegrationSearch_ProjectSlashHyphenNameViaTrigram is the acceptance
// criterion that drove the ladder's shape: `games for agents` must find
// `shaharia-lab/games-for-agents`.
//
// It is NOT an FTS match and cannot be made one. The english parser tokenizes
// that name as a file path — to_tsvector yields '/games-for-agents',
// 'shaharia-lab', 'shaharia', 'lab' — so neither the strict nor the relaxed pass
// matches the query's lexemes. word_similarity scores it 1.0 and the trigram
// pass carries it. That is why the test names the pass it expects.
func TestIntegrationSearch_ProjectSlashHyphenNameViaTrigram(t *testing.T) {
	resetIntegrationTables(t)
	f := seedSearchFixture(t)
	repo := NewProjectRepository(integrationDB)

	results, err := repo.SearchProjects(context.Background(), f.userID,
		repositories.ProjectSearchFilters{Query: "games for agents", Limit: 10})

	require.NoError(t, err)
	require.NotEmpty(t, results, "the trigram pass must match a slash/hyphen-heavy name")
	assert.Equal(t, f.projectID, results[0].ID)
	assert.NotContains(t, projectIDs(results), f.foreignProj,
		"an identically named project in a team the caller cannot read must never appear")

	// Pin the premise: if a future change makes this an FTS match, the comment
	// above (and the migration's reasoning) is wrong and should be revisited.
	var strictMatch bool
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		`SELECT to_tsvector('english', coalesce($1, '') || ' ' || coalesce('', ''))
		        @@ websearch_to_tsquery('english', $2)`,
		"shaharia-lab/games-for-agents", "games for agents").Scan(&strictMatch))
	assert.False(t, strictMatch, "premise: the FTS pass does NOT match this name")
}

// TestIntegrationSearch_SingleTranspositionTypo pins the trigram threshold's
// lower bound: `vibxep` -> `VibeXP Platform` scores ~0.43 word_similarity, which
// clears the ladder's 0.3 but would be rejected by pg_trgm's default 0.6. If the
// transaction-local set_config is ever dropped, this test goes red.
func TestIntegrationSearch_SingleTranspositionTypo(t *testing.T) {
	resetIntegrationTables(t)
	f := seedSearchFixture(t)
	repo := NewTeamRepository(integrationDB)

	results, err := repo.SearchTeams(context.Background(), f.userID, "vibxep", 10)

	require.NoError(t, err)
	require.NotEmpty(t, results, "a single transposition must still match via the trigram pass")
	assert.Equal(t, f.teamID, results[0].ID)
	assert.NotContains(t, teamIDs(results), f.foreignTeam)
}

// TestIntegrationSearch_ExactShortCircuits proves the exact pass both matches and
// STOPS the ladder. The check is behavioural rather than a spy: the fixture's
// other team also matches the query by FTS, so if the ladder continued past the
// exact pass the result set would be larger than one.
func TestIntegrationSearch_ExactShortCircuits(t *testing.T) {
	resetIntegrationTables(t)
	f := seedSearchFixture(t)
	ctx := context.Background()

	var slug string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT slug FROM teams WHERE id = $1", f.teamID).Scan(&slug))

	teamRepo := NewTeamRepository(integrationDB)

	bySlug, err := teamRepo.SearchTeams(ctx, f.userID, slug, 10)
	require.NoError(t, err)
	require.Len(t, bySlug, 1, "an exact slug match must short-circuit, not fall through to FTS")
	assert.Equal(t, f.teamID, bySlug[0].ID)
	assert.InDelta(t, 1.0, bySlug[0].Score, 0.0001, "exact matches score 1.0")

	byID, err := teamRepo.SearchTeams(ctx, f.userID, f.teamID, 10)
	require.NoError(t, err)
	require.Len(t, byID, 1, "an exact UUID match must resolve")
	assert.Equal(t, f.teamID, byID[0].ID)
	assert.InDelta(t, 1.0, byID[0].Score, 0.0001)
}

// TestIntegrationSearch_ProjectExactGitURL covers the identifier agents most
// often hold before they know a project's name.
func TestIntegrationSearch_ProjectExactGitURL(t *testing.T) {
	resetIntegrationTables(t)
	f := seedSearchFixture(t)
	repo := NewProjectRepository(integrationDB)

	results, err := repo.SearchProjects(context.Background(), f.userID,
		repositories.ProjectSearchFilters{
			Query: "https://github.com/shaharia-lab/games-for-agents", Limit: 10,
		})

	require.NoError(t, err)
	require.Len(t, results, 1, "the foreign team's identical git_url must not be reachable")
	assert.Equal(t, f.projectID, results[0].ID)
	assert.InDelta(t, 1.0, results[0].Score, 0.0001)
}

// TestIntegrationSearch_FTSPasses covers the two full-text rungs: a word that
// appears in a description matches strictly, and a multi-word natural-language
// query that ANDs to nothing is rescued by the relaxed OR-rewrite.
func TestIntegrationSearch_FTSPasses(t *testing.T) {
	resetIntegrationTables(t)
	f := seedSearchFixture(t)
	repo := NewTeamRepository(integrationDB)
	ctx := context.Background()

	strict, err := repo.SearchTeams(ctx, f.userID, "dashboards", 10)
	require.NoError(t, err)
	require.NotEmpty(t, strict, "a description word must match via the strict FTS pass")
	assert.Equal(t, f.memberTeam, strict[0].ID, "membership, not ownership, is enough to read")

	// "reporting" and "brain" never co-occur, so the strict AND finds nothing and
	// only the relaxed pass can return rows.
	relaxed, err := repo.SearchTeams(ctx, f.userID, "reporting brain", 10)
	require.NoError(t, err)
	require.NotEmpty(t, relaxed, "the relaxed pass must rescue a multi-word query that ANDs to nothing")
	assert.NotContains(t, teamIDs(relaxed), f.foreignTeam)
}

// TestIntegrationSearch_CrossTeamAndNarrowing is the capability List cannot
// express: find a project without knowing which team holds it, then narrow.
func TestIntegrationSearch_CrossTeamAndNarrowing(t *testing.T) {
	resetIntegrationTables(t)
	f := seedSearchFixture(t)
	repo := NewProjectRepository(integrationDB)
	ctx := context.Background()

	across, err := repo.SearchProjects(ctx, f.userID,
		repositories.ProjectSearchFilters{Query: "revenue", Limit: 10})
	require.NoError(t, err)
	require.NotEmpty(t, across, "a project in a team the caller merely belongs to must be findable with no team_id")
	assert.Equal(t, f.memberTeam, across[0].TeamID)

	narrowed, err := repo.SearchProjects(ctx, f.userID,
		repositories.ProjectSearchFilters{Query: "revenue", TeamID: f.teamID, Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, narrowed, "narrowing to a team that does not hold the match returns nothing")
}

// TestIntegrationSearch_TenancyPerPass asserts the tenancy predicate on EVERY
// rung, not just the happy path. The foreign team and its project are named,
// described and git_url'd identically to the caller's own, so each query below
// would return them if the predicate were missing from that pass — and the
// caller's own row is excluded from the assertion by searching as a user who
// owns nothing.
func TestIntegrationSearch_TenancyPerPass(t *testing.T) {
	resetIntegrationTables(t)
	f := seedSearchFixture(t)
	outsider := insertTestUser(t)
	ctx := context.Background()

	teamRepo := NewTeamRepository(integrationDB)
	projectRepo := NewProjectRepository(integrationDB)

	var foreignSlug string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT slug FROM teams WHERE id = $1", f.foreignTeam).Scan(&foreignSlug))

	for _, tc := range []struct {
		pass  string
		query string
	}{
		{"exact slug", foreignSlug},
		{"exact uuid", f.foreignTeam},
		{"strict fts", "shared"},
		{"relaxed fts", "shared reporting"},
		{"trigram", "vibxep"},
	} {
		t.Run("teams/"+tc.pass, func(t *testing.T) {
			results, err := teamRepo.SearchTeams(ctx, outsider, tc.query, 10)
			require.NoError(t, err)
			assert.Empty(t, results, "a non-member must reach no team on the %s pass", tc.pass)
		})
	}

	for _, tc := range []struct {
		pass  string
		query string
	}{
		{"exact git_url", "https://github.com/shaharia-lab/games-for-agents"},
		{"exact uuid", f.foreignProj},
		{"strict fts", "chess"},
		{"relaxed fts", "chess reporting"},
		{"trigram", "games for agents"},
	} {
		t.Run("projects/"+tc.pass, func(t *testing.T) {
			results, err := projectRepo.SearchProjects(ctx, outsider,
				repositories.ProjectSearchFilters{Query: tc.query, Limit: 10})
			require.NoError(t, err)
			assert.Empty(t, results, "a non-member must reach no project on the %s pass", tc.pass)
		})
	}
}

// TestIntegrationSearch_BlankQueryReturnsNothing pins the deliberate choice not
// to treat an empty query as "list everything" — the shape that turns a search
// box into an accidental full-table dump.
func TestIntegrationSearch_BlankQueryReturnsNothing(t *testing.T) {
	resetIntegrationTables(t)
	f := seedSearchFixture(t)
	ctx := context.Background()

	teams, err := NewTeamRepository(integrationDB).SearchTeams(ctx, f.userID, "   ", 10)
	require.NoError(t, err)
	assert.Empty(t, teams)

	projects, err := NewProjectRepository(integrationDB).SearchProjects(ctx, f.userID,
		repositories.ProjectSearchFilters{Query: "", Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, projects)
}

// TestIntegrationSearch_UsesTheMigration016Indexes guards the invariant the
// migration's comment states: the index expressions and the query expressions
// must stay byte-identical, or the planner silently ignores the indexes and
// every pass seq-scans while all the correctness tests above stay green.
//
// SET LOCAL inside a transaction (not SET against the pool, which can land on a
// different connection than the EXPLAIN), and every plan line is read — a bitmap
// scan names its index only in a child node.
func TestIntegrationSearch_UsesTheMigration016Indexes(t *testing.T) {
	resetIntegrationTables(t)
	f := seedSearchFixture(t)
	ctx := context.Background()

	teamSpec := teamSearchSpec()
	projectSpec := projectSearchSpec()

	for _, tc := range []struct {
		name      string
		query     string
		spec      entitySearchSpec
		passIndex int
		wantIndex string
	}{
		{"teams strict fts", "dashboards", teamSpec, 1, "idx_teams_fts"},
		{"teams trigram", "vibxep", teamSpec, 3, "idx_teams_name_trgm"},
		{"projects strict fts", "chess", projectSpec, 1, "idx_projects_fts"},
		{"projects trigram", "games for agents", projectSpec, 3, "idx_projects_name_trgm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// EXPLAIN the pass the production ladder builds, arguments included, so
			// the assertion cannot drift from what actually runs.
			pass := tc.spec.buildPasses(searchInputs{
				query: tc.query, userID: f.userID, uuidArg: uuidOrNil(tc.query), limit: 10,
			})[tc.passIndex]
			explain := "EXPLAIN " + pass.query

			tx, err := integrationDB.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()

			_, err = tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off")
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx,
				"SELECT set_config('pg_trgm.word_similarity_threshold', $1, true)", keywordTrgmThreshold)
			require.NoError(t, err)

			rows, err := tx.QueryContext(ctx, explain, pass.args...)
			require.NoError(t, err)
			defer func() { _ = rows.Close() }()

			var plan strings.Builder
			for rows.Next() {
				var line string
				require.NoError(t, rows.Scan(&line))
				plan.WriteString(line + "\n")
			}
			require.NoError(t, rows.Err())

			assert.Contains(t, plan.String(), tc.wantIndex,
				"the %s pass must use %s — an index expression that drifts from the query "+
					"expression is silently ignored; plan was:\n%s", tc.name, tc.wantIndex, plan.String())
		})
	}
}
