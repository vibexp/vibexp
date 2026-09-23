//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/repositories"
)

// #1100: prompt @references resolve within the prompt's own team. These tests
// pin, against real Postgres, (1) the tenancy-only GetBySlugInTeam lookup the
// services now use, and (2) migration 018, which corrects the reference graph
// written by the old author-scoped, any-team lookup. Migration 018 runs on its
// OWN scratch database, as the other migration tests do, so the shared
// integrationDB is never re-migrated.

type refScopeFixture struct {
	db                     *sql.DB
	author, teammate       string
	teamT, teamOther       string
	projectT, projectOther string
}

func newRefScopeFixture(t *testing.T, db *sql.DB) refScopeFixture {
	t.Helper()
	f := refScopeFixture{db: db}
	f.author = refScopeInsertUser(t, db)
	f.teammate = refScopeInsertUser(t, db)
	f.teamT = refScopeInsertTeam(t, db, f.author)
	f.teamOther = refScopeInsertTeam(t, db, f.author)
	f.projectT = refScopeInsertProject(t, db, f.author, f.teamT)
	f.projectOther = refScopeInsertProject(t, db, f.author, f.teamOther)
	return f
}

func refScopeInsertUser(t *testing.T, db *sql.DB) string {
	t.Helper()
	id := uuid.New().String()
	_, err := db.Exec("INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
		id, "refscope-"+id[:8]+"@example.com", "Ref Scope")
	require.NoError(t, err)
	return id
}

func refScopeInsertTeam(t *testing.T, db *sql.DB, ownerID string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := db.Exec("INSERT INTO teams (id, owner_id, name, slug) VALUES ($1, $2, $3, $4)",
		id, ownerID, "Team "+id[:8], "team-"+id[:8])
	require.NoError(t, err)
	return id
}

func refScopeInsertProject(t *testing.T, db *sql.DB, userID, teamID string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := db.Exec(
		"INSERT INTO projects (id, user_id, team_id, name, slug) VALUES ($1, $2, $3, $4, $5)",
		id, userID, teamID, "Project "+id[:8], "project-"+id[:8])
	require.NoError(t, err)
	return id
}

func (f refScopeFixture) prompt(t *testing.T, userID, teamID, projectID, slug, body string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := f.db.Exec(
		`INSERT INTO prompts (id, user_id, team_id, project_id, name, slug, body, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'published')`,
		id, userID, teamID, projectID, "P "+slug, slug, body)
	require.NoError(t, err)
	return id
}

func (f refScopeFixture) edge(t *testing.T, from, to string) {
	t.Helper()
	_, err := f.db.Exec(
		"INSERT INTO prompt_references (prompt_id, referenced_prompt_id) VALUES ($1, $2)", from, to)
	require.NoError(t, err)
}

func (f refScopeFixture) hasEdge(t *testing.T, from, to string) bool {
	t.Helper()
	return countRows(t, f.db,
		"SELECT count(*) FROM prompt_references WHERE prompt_id = $1 AND referenced_prompt_id = $2",
		from, to) == 1
}

func TestIntegrationPromptRepository_GetBySlugInTeam(t *testing.T) {
	f := newRefScopeFixture(t, integrationDB.DB)
	// The same author owns slug "shared" in two teams; a teammate authored
	// "teammates" in T.
	inT := f.prompt(t, f.author, f.teamT, f.projectT, "shared-"+f.teamT[:8], "in T")
	f.prompt(t, f.author, f.teamOther, f.projectOther, "shared-"+f.teamT[:8], "in other")
	teammates := f.prompt(t, f.teammate, f.teamT, f.projectT, "teammates-"+f.teamT[:8], "by teammate")

	repo := NewPromptRepository(integrationDB)
	ctx := context.Background()

	got, err := repo.GetBySlugInTeam(ctx, f.teamT, "shared-"+f.teamT[:8])
	require.NoError(t, err)
	assert.Equal(t, inT, got.ID, "the team's row, never the same slug in another team")
	assert.Equal(t, "in T", got.Body)

	got, err = repo.GetBySlugInTeam(ctx, f.teamT, "teammates-"+f.teamT[:8])
	require.NoError(t, err)
	assert.Equal(t, teammates, got.ID, "resolves whoever authored the prompt")

	_, err = repo.GetBySlugInTeam(ctx, f.teamOther, "teammates-"+f.teamT[:8])
	assert.ErrorIs(t, err, repositories.ErrPromptNotFound)
}

func TestMigration018_PromptReferencesTeamScope(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)
	require.NoError(t, m.Migrate(17), "migrate to 017")

	f := newRefScopeFixture(t, db)
	styleT := f.prompt(t, f.teammate, f.teamT, f.projectT, "style", "team style")
	styleOther := f.prompt(t, f.author, f.teamOther, f.projectOther, "style", "other style")
	onlyOther := f.prompt(t, f.author, f.teamOther, f.projectOther, "only-other", "x")
	footerT := f.prompt(t, f.author, f.teamT, f.projectT, "footer", "f")
	review := f.prompt(t, f.author, f.teamT, f.projectT, "review",
		"Use @style and @only-other, mail me at a@@footer, self @review, @style again")
	escapedJoin := f.prompt(t, f.author, f.teamT, f.projectT, "joined", "@foo@@ter is not @footer-less")

	// What the old author-scoped, any-team lookup could have stored.
	f.edge(t, review, styleOther) // cross-team edge
	f.edge(t, review, onlyOther)  // cross-team edge
	f.edge(t, review, review)     // self edge
	// ...and the teammate edge review -> styleT is missing.

	require.NoError(t, m.Migrate(18), "migrate to 018")

	assertGraph := func(t *testing.T) {
		t.Helper()
		assert.True(t, f.hasEdge(t, review, styleT), "missing same-team teammate edge is added")
		assert.False(t, f.hasEdge(t, review, styleOther), "cross-team edge is dropped")
		assert.False(t, f.hasEdge(t, review, onlyOther), "slug only in another team is not linked")
		assert.False(t, f.hasEdge(t, review, review), "self edge is dropped")
		assert.False(t, f.hasEdge(t, review, footerT), "an escaped @@footer is not a reference")
		assert.False(t, f.hasEdge(t, escapedJoin, footerT),
			"text around @@ must not join into a new slug; @footer-less is a different slug")
		assert.Equal(t, 0, countRows(t, db,
			`SELECT count(*) FROM prompt_references pr
			   JOIN prompts a ON a.id = pr.prompt_id
			   JOIN prompts b ON b.id = pr.referenced_prompt_id
			  WHERE a.team_id IS DISTINCT FROM b.team_id`),
			"no edge links prompts in different teams")
		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM prompt_references WHERE prompt_id = $1", review),
			"review depends on exactly one prompt: T's style")
	}
	t.Run("corrects the graph", assertGraph)

	t.Run("re-running the up SQL is a no-op", func(t *testing.T) {
		up, err := os.ReadFile("../../../migrations/018_prompt_references_team_scope.up.sql")
		require.NoError(t, err)
		before := countRows(t, db, "SELECT count(*) FROM prompt_references")
		_, err = db.Exec(string(up))
		require.NoError(t, err)
		assert.Equal(t, before, countRows(t, db, "SELECT count(*) FROM prompt_references"))
		assertGraph(t)
	})

	t.Run("down is a no-op that leaves the corrected graph", func(t *testing.T) {
		require.NoError(t, m.Migrate(17), "migrate down to 017")
		assertGraph(t)
	})
}
