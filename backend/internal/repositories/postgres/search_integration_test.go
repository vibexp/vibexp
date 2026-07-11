//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
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

// embeddingDims matches the vector(1024) column declared in migration 001.
const embeddingDims = 1024

// testVector builds a 1024-dim embedding whose only non-zero components are the
// supplied index→value pairs, so a test can place chunks at controlled cosine
// angles from a query vector and thereby pin their relative distances.
func testVector(components map[int]float32) []float32 {
	v := make([]float32, embeddingDims)
	for i, val := range components {
		v[i] = val
	}
	return v
}

// insertTestEmbedding inserts one embedding chunk for an entity and returns its
// chunk id. Inserting several chunks under the same entityID models a long
// document split across multiple embeddings — the case per-entity dedup collapses.
func insertTestEmbedding(
	t *testing.T, userID, teamID, entityType, entityID, modelID, content string, vec []float32,
) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO embeddings (id, entity_type, entity_id, vector_embeddings, model_id, user_id, content, team_id) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
		id, entityType, entityID, pgvector.NewVector(vec), modelID, userID, content, teamID)
	require.NoError(t, err)
	return id
}

// TestSearchRepository_SearchKeyword_Integration exercises the full-text fallback
// against a real PostgreSQL: it proves the generated SQL is valid, that ts_rank_cd
// ordering (with title weighting and length normalization), status filtering,
// team/project scoping and pagination behave as the acceptance criteria for issues
// #18/#174/#183 require, and (via TestMain applying migrations 002 + 006) that the
// weighted FTS index migration is sound.
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

	t.Run("multi-word query stems via websearch_to_tsquery (english)", func(t *testing.T) {
		resetSearchTables(t)
		user := insertTestUser(t)
		team := insertTestTeam(t, user)
		project := insertTestProject(t, user, team)
		_ = insertTestPrompt(t, user, team, project,
			"indexing guide", "guidance on indexing documents for searching", "published")

		// "index searches" stems to index/search and matches "indexing"/"searching"
		// on the strict pass (both terms present, so AND semantics still hit).
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

		// websearch_to_tsquery('english', 'the and of') produces an empty tsquery (all
		// stopwords) on both passes: the relaxed OR-rewrite of an empty tsquery is
		// still empty, so it matches nothing. The repo must return an empty page, not
		// error, so the endpoint still answers HTTP 200.
		rows, total, err := repo.SearchKeyword(ctx, team, "the and of", allTypes, "", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 0, total)
		assert.Empty(t, rows)
	})

	t.Run("multi-word natural-language question falls back to the relaxed OR pass", func(t *testing.T) {
		resetSearchTables(t)
		user := insertTestUser(t)
		team := insertTestTeam(t, user)
		project := insertTestProject(t, user, team)
		// The document only covers postgres full-text search; it says nothing about
		// "performance", "large" or "tables".
		relevant := insertTestPrompt(t, user, team, project,
			"Postgres full text search", "how to configure postgres tsvector and ts_rank for document search", "published")
		// An unrelated published prompt that shares none of the query's content words.
		_ = insertTestPrompt(t, user, team, project,
			"Frontend routing", "react router navigation and lazy loading", "published")

		// AC #1: a 5+ word natural-language question with no exact phrase match. Under
		// strict AND semantics every content word must appear, so the extra words
		// ("performance", "large", "tables") drive the strict pass to zero; the relaxed
		// OR pass then matches on the shared postgres/search/document lexemes.
		q := "how do I tune postgres full text search performance for large tables"
		rows, total, err := repo.SearchKeyword(ctx, team, q, allTypes, "", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, rows, 1)
		assert.Equal(t, relevant, rows[0].EntityID)
		// Distance stays in the [0,1) score band the service maps back to a score.
		assert.GreaterOrEqual(t, rows[0].Distance, 0.0)
		assert.Less(t, rows[0].Distance, 1.0)
	})

	t.Run("strict pass wins so precise queries stay precise", func(t *testing.T) {
		resetSearchTables(t)
		user := insertTestUser(t)
		team := insertTestTeam(t, user)
		project := insertTestProject(t, user, team)
		// One prompt contains BOTH terms; another contains only one. Under the strict
		// AND pass only the first matches — the relaxed OR pass (which would also
		// return the second) must NOT run because the strict pass already has a hit.
		both := insertTestPrompt(t, user, team, project,
			"alpha", "alpha bravo together", "published")
		_ = insertTestPrompt(t, user, team, project,
			"bravo only", "bravo appears here without the other word", "published")

		rows, total, err := repo.SearchKeyword(ctx, team, "alpha bravo", []string{"prompt"}, "", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, rows, 1)
		assert.Equal(t, both, rows[0].EntityID)
	})

	t.Run("honours websearch quoted-phrase semantics", func(t *testing.T) {
		resetSearchTables(t)
		user := insertTestUser(t)
		team := insertTestTeam(t, user)
		project := insertTestProject(t, user, team)
		adjacent := insertTestPrompt(t, user, team, project,
			"adjacent", "the full text search pipeline", "published")
		// Same two words, but not adjacent and in the wrong order — a phrase query
		// must not match this one.
		_ = insertTestPrompt(t, user, team, project,
			"scattered", "text is processed, then later we do a full rebuild", "published")

		// AC #2: a quoted phrase matches only adjacency (websearch <-> phrase operator).
		rows, total, err := repo.SearchKeyword(ctx, team, `"full text"`, []string{"prompt"}, "", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, rows, 1)
		assert.Equal(t, adjacent, rows[0].EntityID)
	})

	t.Run("a title match outranks a body-only match (title weighting)", func(t *testing.T) {
		resetSearchTables(t)
		user := insertTestUser(t)
		team := insertTestTeam(t, user)
		project := insertTestProject(t, user, team)

		// AC: "a query term appearing in a resource title ranks that resource above
		// body-only matches of otherwise similar rank." The term "widget" lives in
		// exactly one field per prompt (title vs body), so the only thing separating
		// them is the setweight A-vs-D weighting.
		titleMatch := insertTestPrompt(t, user, team, project,
			"widget configuration guide", "general instructions and setup notes for the system", "published")
		bodyMatch := insertTestPrompt(t, user, team, project,
			"general reference manual", "detailed notes about widget behaviour and tuning", "published")

		rows, total, err := repo.SearchKeyword(ctx, team, "widget", []string{"prompt"}, "", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		require.Len(t, rows, 2)

		// The title match sorts first with a strictly smaller distance (= higher
		// ts_rank_cd) purely because its match is weight A, not weight D.
		assert.Equal(t, titleMatch, rows[0].EntityID, "title match should rank first")
		assert.Equal(t, bodyMatch, rows[1].EntityID)
		assert.Less(t, rows[0].Distance, rows[1].Distance,
			"title-weighted match must score higher (smaller distance) than the body-only match")
	})

	t.Run("a concise document outranks a verbose one on the same single match (length normalization)", func(t *testing.T) {
		resetSearchTables(t)
		user := insertTestUser(t)
		team := insertTestTeam(t, user)
		project := insertTestProject(t, user, team)

		// AC: "long documents no longer dominate purely by length (normalized
		// ranking)." Both prompts match "widget" exactly once in the body (title has
		// no match, so weighting is identical); they differ only in body length. The
		// old unnormalized ts_rank scored them equally (length-blind); ts_rank_cd with
		// normalization flag 1 divides by 1 + log(document length), so the padded
		// document ranks strictly below the concise one.
		concise := insertTestPrompt(t, user, team, project,
			"concise note", "widget", "published")
		verbose := insertTestPrompt(t, user, team, project,
			"verbose note", "widget "+strings.Repeat("lorem ipsum dolor sit amet consectetur ", 100), "published")

		rows, total, err := repo.SearchKeyword(ctx, team, "widget", []string{"prompt"}, "", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		require.Len(t, rows, 2)

		assert.Equal(t, concise, rows[0].EntityID, "the concise document should rank first")
		assert.Equal(t, verbose, rows[1].EntityID)
		assert.Less(t, rows[0].Distance, rows[1].Distance,
			"length normalization must rank the concise document above the verbose one")
	})

	t.Run("query plan uses the GIN full-text index (EXPLAIN)", func(t *testing.T) {
		resetSearchTables(t)
		user := insertTestUser(t)
		team := insertTestTeam(t, user)
		project := insertTestProject(t, user, team)
		// Seed many rows where the full-text predicate is the SELECTIVE one: only two
		// carry the rare term, the rest are filler. That makes the GIN the planner's
		// natural choice for the tsvector @@ tsquery match, so the EXPLAIN proves the
		// migration-006 weighted index is eligible for both tsquery expressions — the
		// assertion fails only if ftsExpr diverges from the indexed expression.
		for i := 0; i < 2; i++ {
			insertTestPrompt(t, user, team, project,
				"rare hit", "zqzxqterm keyword lives here", "published")
		}
		for i := 0; i < 300; i++ {
			insertTestPrompt(t, user, team, project,
				"filler", "unrelated lorem ipsum filler body text", "published")
		}
		_, err := integrationDB.ExecContext(ctx, "ANALYZE prompts")
		require.NoError(t, err)

		// AC #3: EXPLAIN the exact (tsvector, tsquery) pairing the repository emits.
		// Isolating the FTS predicate (no competing team/status filter) keeps the plan
		// about index eligibility for the operator, which is what the tsquery swap
		// could have broken.
		tsv := ftsExpr(entitySources["prompt"])
		strictPlan := explainFTS(t, ctx, tsv, keywordStrictTSQuery, "zqzxqterm")
		assert.Contains(t, strictPlan, "idx_prompts_fts",
			"strict tsquery should use the migration-006 weighted GIN index, got plan:\n%s", strictPlan)

		relaxedPlan := explainFTS(t, ctx, tsv, keywordRelaxedTSQuery, "zqzxqterm please")
		assert.Contains(t, relaxedPlan, "idx_prompts_fts",
			"relaxed OR-rewrite tsquery should use the migration-006 weighted GIN index, got plan:\n%s", relaxedPlan)
	})
}

// explainFTS EXPLAINs the isolated `tsv @@ tsquery` predicate the repository
// emits against the prompts table, binding $1=query, and returns the plan text.
// Sequential scans are disabled inside a rolled-back transaction so a divergence
// between ftsExpr and the indexed expression surfaces as a non-index plan.
func explainFTS(t *testing.T, ctx context.Context, tsv, tsquery, query string) string {
	t.Helper()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off")
	require.NoError(t, err)

	explainSQL := "EXPLAIN (FORMAT TEXT) SELECT 1 FROM prompts src WHERE " + tsv + " @@ " + tsquery
	rows, err := tx.QueryContext(ctx, explainSQL, query)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	var plan strings.Builder
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan.WriteString(line)
		plan.WriteString("\n")
	}
	require.NoError(t, rows.Err())
	return plan.String()
}

// TestSearchRepository_SearchSimilar_Integration exercises the semantic (pgvector)
// path against a real PostgreSQL, proving the per-entity dedup added for issue #172
// behaves at runtime — something the sqlmock unit tests (which only pin SQL text)
// cannot. It pins the two behavioural acceptance criteria: a document with several
// matching chunks appears exactly once carrying its closest chunk, total_count
// counts distinct entities, and pagination stays stable across pages on tied scores.
func TestSearchRepository_SearchSimilar_Integration(t *testing.T) {
	ctx := context.Background()
	repo := NewSearchRepository(integrationDB).(*SearchRepository)
	const model = "test-embed-1024"
	allTypes := []string{"prompt", "artifact", "blueprint", "memory"}

	t.Run("collapses many chunks of one entity to its closest chunk", func(t *testing.T) {
		resetSearchTables(t)
		user := insertTestUser(t)
		team := insertTestTeam(t, user)
		project := insertTestProject(t, user, team)

		// Entity A is a long prompt split into two chunks; entity B a single-chunk artifact.
		promptA := insertTestPrompt(t, user, team, project, "Alpha doc", "alpha body", "published")
		artifactB := insertTestArtifact(t, user, team, project, "Bravo doc", "bravo body", "active")

		query := testVector(map[int]float32{0: 1}) // points along axis 0

		// A's best chunk sits exactly on the query axis (cosine distance 0); its other
		// chunk is orthogonal (distance ~1). B's single chunk is slightly off-axis.
		bestChunkA := insertTestEmbedding(t, user, team, "prompt", promptA, model,
			"A best chunk", testVector(map[int]float32{0: 1}))
		_ = insertTestEmbedding(t, user, team, "prompt", promptA, model,
			"A far chunk", testVector(map[int]float32{1: 1}))
		_ = insertTestEmbedding(t, user, team, "artifact", artifactB, model,
			"B only chunk", testVector(map[int]float32{0: 1, 1: 0.2}))

		rows, total, err := repo.SearchSimilar(ctx, team, query, model, allTypes, "", 10, 0)
		require.NoError(t, err)

		// Three chunks match, but only two distinct entities: A collapses to one row.
		assert.Equal(t, 2, total, "total_count counts distinct entities, not chunks")
		require.Len(t, rows, 2)

		// A ranks first (best chunk exactly on-axis) and carries THAT chunk's id/content.
		assert.Equal(t, promptA, rows[0].EntityID)
		assert.Equal(t, "prompt", rows[0].EntityType)
		assert.Equal(t, bestChunkA, rows[0].ChunkID)
		assert.Equal(t, "A best chunk", rows[0].ChunkContent)
		assert.InDelta(t, 0.0, rows[0].Distance, 1e-5)

		// B appears exactly once; A's far chunk never surfaces as a duplicate second row.
		assert.Equal(t, artifactB, rows[1].EntityID)
		assert.ElementsMatch(t, []string{promptA, artifactB}, []string{rows[0].EntityID, rows[1].EntityID})
	})

	t.Run("stable pagination across pages on tied scores", func(t *testing.T) {
		resetSearchTables(t)
		user := insertTestUser(t)
		team := insertTestTeam(t, user)
		project := insertTestProject(t, user, team)

		// Two single-chunk entities whose chunks sit at the identical distance (a tie).
		p1 := insertTestPrompt(t, user, team, project, "Tie one", "one", "published")
		p2 := insertTestPrompt(t, user, team, project, "Tie two", "two", "published")
		query := testVector(map[int]float32{0: 1})
		_ = insertTestEmbedding(t, user, team, "prompt", p1, model, "chunk one", testVector(map[int]float32{0: 1}))
		_ = insertTestEmbedding(t, user, team, "prompt", p2, model, "chunk two", testVector(map[int]float32{0: 1}))

		page1, total, err := repo.SearchSimilar(ctx, team, query, model, []string{"prompt"}, "", 1, 0)
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		require.Len(t, page1, 1)

		page2, _, err := repo.SearchSimilar(ctx, team, query, model, []string{"prompt"}, "", 1, 1)
		require.NoError(t, err)
		require.Len(t, page2, 1)

		// The entity_id secondary sort key gives a deterministic split: the two pages
		// carry different entities and together cover both — no repeat, no skip.
		assert.NotEqual(t, page1[0].EntityID, page2[0].EntityID)
		assert.ElementsMatch(t, []string{p1, p2}, []string{page1[0].EntityID, page2[0].EntityID})
	})
}
