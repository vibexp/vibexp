//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetSearchTables clears every table SearchKeyword reads plus the rows that hang
// off them, so each search integration test starts from a clean slate. Truncating
// users CASCADE reaches teams/projects/entities through their user_id FKs.
func resetSearchTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, teams, projects, prompts, artifacts, memories, blueprints, embeddings CASCADE")
	require.NoError(t, err)
}

func insertTestTeam(t *testing.T, ownerID string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO teams (id, owner_id, name, slug) VALUES ($1, $2, $3, $4)",
		id, ownerID, "Team "+id[:8], "team-"+id[:8])
	require.NoError(t, err)
	return id
}

func insertTestProject(t *testing.T, userID, teamID string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO projects (id, user_id, team_id, name, slug) VALUES ($1, $2, $3, $4, $5)",
		id, userID, teamID, "Project "+id[:8], "project-"+id[:8])
	require.NoError(t, err)
	return id
}

func insertTestPrompt(t *testing.T, userID, teamID, projectID, name, body, status string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO prompts (id, user_id, team_id, project_id, name, slug, body, status) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
		id, userID, teamID, projectID, name, "prompt-"+id[:8], body, status)
	require.NoError(t, err)
	return id
}

func insertTestArtifact(t *testing.T, userID, teamID, projectID, title, content, status string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO artifacts (id, user_id, team_id, project_id, title, slug, content, status) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
		id, userID, teamID, projectID, title, "artifact-"+id[:8], content, status)
	require.NoError(t, err)
	return id
}

func insertTestMemory(t *testing.T, userID, teamID, projectID, text string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO memories (id, user_id, team_id, project_id, text) VALUES ($1, $2, $3, $4, $5)",
		id, userID, teamID, projectID, text)
	require.NoError(t, err)
	return id
}

// TestSearchRepository_SearchKeyword_Integration exercises the full-text fallback
// against a real PostgreSQL: it proves the generated SQL is valid, that ts_rank
// ordering, status filtering, team/project scoping and pagination behave as the
// acceptance criteria for issue #18 require, and (via TestMain applying migration
// 002) that the FTS index migration is sound.
func TestSearchRepository_SearchKeyword_Integration(t *testing.T) {
	ctx := context.Background()
	repo := NewSearchRepository(integrationDB).(*SearchRepository)

	allTypes := []string{"prompt", "artifact", "blueprint", "memory"}

	t.Run("matches across types, honours status and ranks by relevance", func(t *testing.T) {
		resetSearchTables(t)
		user := insertTestUser(t)
		team := insertTestTeam(t, user)
		project := insertTestProject(t, user, team)

		// A published prompt dense with the query term (4 hits) ranks highest; a memory
		// (2 hits) next; an active artifact (1 hit) lowest — a strict relevance order.
		strongPrompt := insertTestPrompt(t, user, team, project,
			"postgres full text search", "postgres full text search with postgres tsvector and postgres ts_rank", "published")
		weakArtifact := insertTestArtifact(t, user, team, project,
			"Database notes", "a passing mention of postgres here", "active")
		_ = insertTestMemory(t, user, team, project,
			"remember to tune postgres autovacuum")
		// Excluded: a draft prompt (status filter) and an unrelated active artifact.
		_ = insertTestPrompt(t, user, team, project,
			"draft postgres", "postgres postgres postgres draft only", "draft")
		_ = insertTestArtifact(t, user, team, project,
			"Frontend", "react components and hooks", "active")

		rows, total, err := repo.SearchKeyword(ctx, team, "postgres", allTypes, "", 10, 0)
		require.NoError(t, err)

		// Three matches (strong prompt, memory, artifact); the draft is filtered out
		// even though it repeats the term, and the frontend artifact doesn't match.
		assert.Equal(t, 3, total)
		require.Len(t, rows, 3)
		// Most relevant first, least relevant last (ORDER BY distance ASC == ts_rank DESC).
		assert.Equal(t, strongPrompt, rows[0].EntityID)
		assert.Equal(t, "prompt", rows[0].EntityType)
		assert.Equal(t, weakArtifact, rows[2].EntityID)
		// Distances are non-decreasing and stay within [0,1); the draft never appears.
		for i, r := range rows {
			assert.GreaterOrEqual(t, r.Distance, 0.0)
			assert.Less(t, r.Distance, 1.0)
			assert.NotEqual(t, "draft postgres", r.Title)
			if i > 0 {
				assert.GreaterOrEqual(t, r.Distance, rows[i-1].Distance)
			}
		}
	})

	t.Run("multi-word query stems via plainto_tsquery (english)", func(t *testing.T) {
		resetSearchTables(t)
		user := insertTestUser(t)
		team := insertTestTeam(t, user)
		project := insertTestProject(t, user, team)
		_ = insertTestPrompt(t, user, team, project,
			"indexing guide", "guidance on indexing documents for searching", "published")

		// "index searches" stems to index/search and matches "indexing"/"searching".
		rows, total, err := repo.SearchKeyword(ctx, team, "index searches", []string{"prompt"}, "", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, rows, 1)
	})

	t.Run("project filter restricts results to that project", func(t *testing.T) {
		resetSearchTables(t)
		user := insertTestUser(t)
		team := insertTestTeam(t, user)
		projectA := insertTestProject(t, user, team)
		projectB := insertTestProject(t, user, team)
		_ = insertTestPrompt(t, user, team, projectA, "alpha doc", "shared keyword alpha", "published")
		_ = insertTestPrompt(t, user, team, projectB, "beta doc", "shared keyword beta", "published")

		all, allTotal, err := repo.SearchKeyword(ctx, team, "shared keyword", []string{"prompt"}, "", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 2, allTotal)
		require.Len(t, all, 2)

		scoped, scopedTotal, err := repo.SearchKeyword(ctx, team, "shared keyword", []string{"prompt"}, projectA, 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 1, scopedTotal)
		require.Len(t, scoped, 1)
		assert.Equal(t, projectA, scoped[0].ProjectID)
	})

	t.Run("scopes to the queried team", func(t *testing.T) {
		resetSearchTables(t)
		user := insertTestUser(t)
		team := insertTestTeam(t, user)
		project := insertTestProject(t, user, team)
		_ = insertTestPrompt(t, user, team, project, "scoped", "team scoped content", "published")

		otherUser := insertTestUser(t)
		otherTeam := insertTestTeam(t, otherUser)

		rows, total, err := repo.SearchKeyword(ctx, otherTeam, "team scoped", []string{"prompt"}, "", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 0, total)
		assert.Empty(t, rows)
	})

	t.Run("paginates the ranked results", func(t *testing.T) {
		resetSearchTables(t)
		user := insertTestUser(t)
		team := insertTestTeam(t, user)
		project := insertTestProject(t, user, team)
		for i := 0; i < 3; i++ {
			insertTestPrompt(t, user, team, project,
				"page item", "pagination keyword content number", "published")
		}

		page1, total, err := repo.SearchKeyword(ctx, team, "pagination keyword", []string{"prompt"}, "", 2, 0)
		require.NoError(t, err)
		assert.Equal(t, 3, total)
		require.Len(t, page1, 2)

		page2, _, err := repo.SearchKeyword(ctx, team, "pagination keyword", []string{"prompt"}, "", 2, 2)
		require.NoError(t, err)
		require.Len(t, page2, 1)

		// The three prompts share an identical ts_rank, so without a deterministic
		// tiebreaker LIMIT/OFFSET could repeat or skip rows across pages. Assert the
		// pages together cover three distinct entities — this fails if the tiebreaker
		// is removed.
		ids := map[string]bool{}
		for _, r := range append(page1, page2...) {
			ids[r.EntityID] = true
		}
		assert.Len(t, ids, 3)
	})

	t.Run("no matches returns empty without error", func(t *testing.T) {
		resetSearchTables(t)
		user := insertTestUser(t)
		team := insertTestTeam(t, user)
		project := insertTestProject(t, user, team)
		_ = insertTestPrompt(t, user, team, project, "doc", "completely unrelated text", "published")

		rows, total, err := repo.SearchKeyword(ctx, team, "nonexistentterm", allTypes, "", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 0, total)
		assert.Empty(t, rows)
	})

	t.Run("stopword-only query yields empty results, not an error", func(t *testing.T) {
		resetSearchTables(t)
		user := insertTestUser(t)
		team := insertTestTeam(t, user)
		project := insertTestProject(t, user, team)
		_ = insertTestPrompt(t, user, team, project, "doc", "the quick brown fox", "published")

		// plainto_tsquery('english', 'the and of') produces an empty tsquery (all
		// stopwords), which matches nothing — the repo must return an empty page, not
		// error, so the endpoint still answers HTTP 200.
		rows, total, err := repo.SearchKeyword(ctx, team, "the and of", allTypes, "", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 0, total)
		assert.Empty(t, rows)
	})
}
