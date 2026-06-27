package postgres

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
)

func setupSearchTest(t *testing.T) (*SearchRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := NewSearchRepository(db)

	return repo.(*SearchRepository), mock, mockDB
}

type buildBranchCase struct {
	name           string
	entityType     string
	mustContain    []string
	mustNotContain []string
}

func buildBranchCases() []buildBranchCase {
	// Every branch derives the project id/name uniformly and accepts the optional
	// project filter ($4), so these fragments are common to all entity types.
	commonProjectFragments := []string{
		"src.project_id::text AS project_id",
		"COALESCE(proj.name, '') AS project_name",
		"LEFT JOIN projects proj ON src.project_id = proj.id",
		"($4::uuid IS NULL OR src.project_id = $4::uuid)",
		// created_at is projected on every branch so the service can re-rank by recency.
		"src.created_at AS created_at",
		"src.updated_at AS updated_at",
	}
	return []buildBranchCase{
		{
			name:       "prompt branch filters published status",
			entityType: "prompt",
			mustContain: append([]string{
				"e.entity_type = 'prompt'",
				"e.team_id = $2",
				"e.model_id = $3",
				"src.status = 'published'",
				"JOIN prompts src",
				"src.name AS title",
				"src.body AS source_body",
				"src.slug AS slug",
			}, commonProjectFragments...),
		},
		{
			name:       "artifact branch filters active status",
			entityType: "artifact",
			mustContain: append([]string{
				"src.status = 'active'",
				"JOIN artifacts src",
				"src.title AS title",
				"src.content AS source_body",
				"src.slug AS slug",
			}, commonProjectFragments...),
		},
		{
			name:       "blueprint branch filters active status",
			entityType: "blueprint",
			mustContain: append([]string{
				"src.status = 'active'",
				"JOIN blueprints src",
				"src.slug AS slug",
			}, commonProjectFragments...),
		},
		{
			name:       "memory branch filters active status",
			entityType: "memory",
			mustContain: append([]string{
				"src.status = 'active'",
				"JOIN memories src",
				"LEFT(src.text, 100) AS title",
				"src.text AS source_body",
				// Memory routes by id, so its own slug is an empty literal.
				"'' AS slug",
			}, commonProjectFragments...),
		},
	}
}

func TestBuildBranch_AppliesVisibilityAndScoping(t *testing.T) {
	for _, tt := range buildBranchCases() {
		t.Run(tt.name, func(t *testing.T) {
			sqlText := buildBranch(tt.entityType, entitySources[tt.entityType])
			for _, fragment := range tt.mustContain {
				assert.Contains(t, sqlText, fragment)
			}
			for _, fragment := range tt.mustNotContain {
				assert.NotContains(t, sqlText, fragment)
			}
		})
	}

	memoryBranch := buildBranch("memory", entitySources["memory"])
	assert.Contains(t, memoryBranch, "src.status = 'active'")
}

func TestBuildUnion_OrdersAndCountsBranches(t *testing.T) {
	union, err := buildUnion([]string{"memory", "prompt"})
	require.NoError(t, err)

	// Deterministic order: prompt before memory regardless of request order.
	promptIdx := indexOf(union, "e.entity_type = 'prompt'")
	memoryIdx := indexOf(union, "e.entity_type = 'memory'")
	assert.GreaterOrEqual(t, promptIdx, 0)
	assert.GreaterOrEqual(t, memoryIdx, 0)
	assert.Less(t, promptIdx, memoryIdx)
	assert.Equal(t, 1, countOccurrences(union, "UNION ALL"))

	_, err = buildUnion(nil)
	assert.Error(t, err)
}

func TestBuildCountUnion_OmitsDistanceAndRenumbersArgs(t *testing.T) {
	union, err := buildCountUnion([]string{"prompt", "memory"})
	require.NoError(t, err)

	// The count path must never run the cosine-distance operator per row, and it
	// binds only team_id ($1), model_id ($2) and project_id ($3) — not the query
	// vector — so the highest positional arg is $3.
	assert.NotContains(t, union, "<=>")
	assert.NotContains(t, union, "$4")
	assert.Contains(t, union, "e.team_id = $1")
	assert.Contains(t, union, "e.model_id = $2")
	assert.Contains(t, union, "($3::uuid IS NULL OR src.project_id = $3::uuid)")
	// Scoping/visibility filters still apply.
	assert.Contains(t, union, "e.entity_type = 'prompt'")
	assert.Contains(t, union, "src.status = 'published'")

	_, err = buildCountUnion(nil)
	assert.Error(t, err)
}

//nolint:funlen // Test function with comprehensive scenarios
func TestSearchRepository_SearchSimilar(t *testing.T) {
	repo, mock, mockDB := setupSearchTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	vec := make([]float32, 384)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	modelID := "all-MiniLM-L6-v2"

	t.Run("returns page and total with filters applied", func(t *testing.T) {
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(`).
			WithArgs(teamID, modelID, nil).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

		resultRows := sqlmock.NewRows([]string{
			"entity_type", "entity_id", "chunk_id", "title", "slug", "project_id", "project_name",
			"chunk_content", "source_body", "created_at", "updated_at", "distance",
		}).
			AddRow("prompt", "p-1", "c-1", "Title 1", "prompt-slug", "proj-1",
				"Project One", "chunk 1", "body 1", now, now, 0.13).
			AddRow("artifact", "a-1", "c-2", "Title 2", "artifact-slug", "proj-2",
				"Project Two", "chunk 2", "body 2", now, now, 0.42)

		// The outer projection must carry slug, project_id, project_name and created_at
		// so the service can build UUID-based deep links, display the project, and
		// re-rank by recency.
		mock.ExpectQuery(`source_body, created_at, updated_at, distance .*ORDER BY distance ASC LIMIT \$5 OFFSET \$6`).
			WithArgs(sqlmock.AnyArg(), teamID, modelID, nil, 10, 0).
			WillReturnRows(resultRows)

		rows, total, err := repo.SearchSimilar(ctx, teamID, vec, modelID, []string{"prompt", "artifact"}, "", 10, 0)

		require.NoError(t, err)
		assert.Equal(t, 2, total)
		require.Len(t, rows, 2)
		assert.Equal(t, "prompt", rows[0].EntityType)
		assert.Equal(t, "prompt-slug", rows[0].Slug)
		assert.Equal(t, "proj-1", rows[0].ProjectID)
		assert.Equal(t, "Project One", rows[0].ProjectName)
		assert.InDelta(t, 0.13, rows[0].Distance, 0.0001)
		assert.Equal(t, now, rows[0].CreatedAt)
		assert.Equal(t, "artifact-slug", rows[1].Slug)
		assert.Equal(t, "proj-2", rows[1].ProjectID)
		assert.Equal(t, "Project Two", rows[1].ProjectName)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("threads project filter into count and page queries", func(t *testing.T) {
		projectID := "11111111-2222-3333-4444-555555555555"
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(`).
			WithArgs(teamID, modelID, projectID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		resultRows := sqlmock.NewRows([]string{
			"entity_type", "entity_id", "chunk_id", "title", "slug", "project_id", "project_name",
			"chunk_content", "source_body", "created_at", "updated_at", "distance",
		}).
			AddRow("artifact", "a-1", "c-1", "Title", "art-slug", projectID, "Scoped Project", "chunk", "body", now, now, 0.2)

		mock.ExpectQuery(`ORDER BY distance ASC LIMIT \$5 OFFSET \$6`).
			WithArgs(sqlmock.AnyArg(), teamID, modelID, projectID, 10, 0).
			WillReturnRows(resultRows)

		rows, total, err := repo.SearchSimilar(ctx, teamID, vec, modelID, []string{"artifact"}, projectID, 10, 0)

		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, rows, 1)
		assert.Equal(t, projectID, rows[0].ProjectID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("short-circuits when count is zero", func(t *testing.T) {
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(`).
			WithArgs(teamID, modelID, nil).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		rows, total, err := repo.SearchSimilar(ctx, teamID, vec, modelID, []string{"prompt"}, "", 10, 0)

		require.NoError(t, err)
		assert.Equal(t, 0, total)
		assert.Empty(t, rows)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("propagates count query error", func(t *testing.T) {
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(`).
			WithArgs(teamID, modelID, nil).
			WillReturnError(sql.ErrConnDone)

		_, _, err := repo.SearchSimilar(ctx, teamID, vec, modelID, []string{"prompt"}, "", 10, 0)

		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("propagates page query error", func(t *testing.T) {
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(`).
			WithArgs(teamID, modelID, nil).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`ORDER BY distance ASC LIMIT \$5 OFFSET \$6`).
			WithArgs(sqlmock.AnyArg(), teamID, modelID, nil, 10, 0).
			WillReturnError(sql.ErrConnDone)

		_, _, err := repo.SearchSimilar(ctx, teamID, vec, modelID, []string{"prompt"}, "", 10, 0)

		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	// The team filter is now applied on the denormalized embeddings.team_id column
	// ($2 in the page query, $1 in the count query). Content authored by any member
	// of a team shares that team's team_id, so a single team query returns rows from
	// every member; a query for a different team binds a different team_id and the
	// DB filter excludes those rows. These two subtests pin both halves of that
	// contract at the bind-argument boundary the repository controls.
	t.Run("returns content from any member of the queried team", func(t *testing.T) {
		const memberAItem, memberBItem = "p-member-a", "p-member-b"
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(`).
			WithArgs(teamID, modelID, nil).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

		resultRows := sqlmock.NewRows([]string{
			"entity_type", "entity_id", "chunk_id", "title", "slug", "project_id", "project_name",
			"chunk_content", "source_body", "created_at", "updated_at", "distance",
		}).
			AddRow("prompt", memberAItem, "c-1", "By Member A", "slug-a", "proj-1",
				"Project One", "chunk a", "body a", now, now, 0.11).
			AddRow("prompt", memberBItem, "c-2", "By Member B", "slug-b", "proj-1",
				"Project One", "chunk b", "body b", now, now, 0.22)

		mock.ExpectQuery(`WHERE e.entity_type = 'prompt' AND e.team_id = \$2`).
			WithArgs(sqlmock.AnyArg(), teamID, modelID, nil, 10, 0).
			WillReturnRows(resultRows)

		rows, total, err := repo.SearchSimilar(ctx, teamID, vec, modelID, []string{"prompt"}, "", 10, 0)

		require.NoError(t, err)
		assert.Equal(t, 2, total)
		require.Len(t, rows, 2)
		assert.Equal(t, memberAItem, rows[0].EntityID)
		assert.Equal(t, memberBItem, rows[1].EntityID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns nothing for a non-member team", func(t *testing.T) {
		const otherTeamID = "99999999-8888-7777-6666-555555555555"
		// A query bound to a team that authored no matching content returns zero,
		// so SearchSimilar short-circuits before the page query.
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(`).
			WithArgs(otherTeamID, modelID, nil).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		rows, total, err := repo.SearchSimilar(ctx, otherTeamID, vec, modelID, []string{"prompt"}, "", 10, 0)

		require.NoError(t, err)
		assert.Equal(t, 0, total)
		assert.Empty(t, rows)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestBuildKeywordBranch_AppliesFTSVisibilityAndScoping(t *testing.T) {
	// Common fragments every keyword branch must carry: full-text match on the
	// combined title+body tsvector, ts_rank scoring (mapped into distance), source
	// table read directly (no embeddings join), team scope on the source row, and the
	// optional project filter. Args are $1=query, $2=team_id, $3=project_id.
	commonFragments := []string{
		"plainto_tsquery('english', $1)",
		"@@ ",
		"ts_rank(",
		"1 - ts_rank(",
		"AS distance",
		"WHERE src.team_id = $2",
		"($3::uuid IS NULL OR src.project_id = $3::uuid)",
		"src.project_id::text AS project_id",
		"COALESCE(proj.name, '') AS project_name",
		"LEFT JOIN projects proj ON src.project_id = proj.id",
		"src.id AS entity_id",
		"src.id AS chunk_id",
		"'' AS chunk_content",
		"src.created_at AS created_at",
		"src.updated_at AS updated_at",
	}

	cases := []struct {
		name           string
		entityType     string
		mustContain    []string
		mustNotContain []string
	}{
		{
			name:       "prompt branch filters published status and matches name+body",
			entityType: "prompt",
			mustContain: append([]string{
				"src.status = 'published'",
				"FROM prompts src",
				"src.name AS title",
				"src.body AS source_body",
				"src.slug AS slug",
				"to_tsvector('english', coalesce(src.name, '') || ' ' || coalesce(src.body, ''))",
			}, commonFragments...),
			// The keyword path reads source tables, never the embeddings table.
			mustNotContain: []string{"FROM embeddings", "<=>", "e.model_id"},
		},
		{
			name:       "artifact branch filters active status and matches title+content",
			entityType: "artifact",
			mustContain: append([]string{
				"src.status = 'active'",
				"FROM artifacts src",
				"src.title AS title",
				"src.content AS source_body",
				"to_tsvector('english', coalesce(src.title, '') || ' ' || coalesce(src.content, ''))",
			}, commonFragments...),
			mustNotContain: []string{"FROM embeddings", "<=>"},
		},
		{
			name:       "blueprint branch filters active status",
			entityType: "blueprint",
			mustContain: append([]string{
				"src.status = 'active'",
				"FROM blueprints src",
			}, commonFragments...),
			mustNotContain: []string{"FROM embeddings", "<=>"},
		},
		{
			name:       "memory branch filters active status",
			entityType: "memory",
			mustContain: append([]string{
				"src.status = 'active'",
				"FROM memories src",
				"LEFT(src.text, 100) AS title",
				"src.text AS source_body",
				"'' AS slug",
			}, commonFragments...),
			mustNotContain: []string{"FROM embeddings"},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			sqlText := buildKeywordBranch(tt.entityType, entitySources[tt.entityType])
			for _, fragment := range tt.mustContain {
				assert.Contains(t, sqlText, fragment)
			}
			for _, fragment := range tt.mustNotContain {
				assert.NotContains(t, sqlText, fragment)
			}
		})
	}
}

func TestBuildKeywordCountUnion_OmitsRankAndBindsThreeArgs(t *testing.T) {
	union, err := buildKeywordCountUnion([]string{"prompt", "memory"})
	require.NoError(t, err)

	// The count path must not compute ts_rank per row and binds only query ($1),
	// team_id ($2) and project_id ($3) — no LIMIT/OFFSET args.
	assert.NotContains(t, union, "ts_rank")
	assert.NotContains(t, union, "$4")
	assert.Contains(t, union, "plainto_tsquery('english', $1)")
	assert.Contains(t, union, "WHERE src.team_id = $2")
	assert.Contains(t, union, "($3::uuid IS NULL OR src.project_id = $3::uuid)")
	// Visibility filters still apply, and prompt precedes memory deterministically.
	assert.Contains(t, union, "src.status = 'published'")
	assert.Less(t, indexOf(union, "FROM prompts src"), indexOf(union, "FROM memories src"))
	assert.Equal(t, 1, countOccurrences(union, "UNION ALL"))

	_, err = buildKeywordCountUnion(nil)
	assert.Error(t, err)
}

//nolint:funlen // Test function with comprehensive scenarios
func TestSearchRepository_SearchKeyword(t *testing.T) {
	repo, mock, mockDB := setupSearchTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	query := "hello world"

	resultCols := []string{
		"entity_type", "entity_id", "chunk_id", "title", "slug", "project_id", "project_name",
		"chunk_content", "source_body", "created_at", "updated_at", "distance",
	}

	t.Run("returns ts_rank-ordered page and total with filters applied", func(t *testing.T) {
		// Count binds query ($1), team_id ($2), project_id ($3).
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(`).
			WithArgs(query, teamID, nil).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

		resultRows := sqlmock.NewRows(resultCols).
			AddRow("prompt", "p-1", "p-1", "Title 1", "prompt-slug", "proj-1",
				"Project One", "", "body 1", now, now, 0.13).
			AddRow("artifact", "a-1", "a-1", "Title 2", "artifact-slug", "proj-2",
				"Project Two", "", "body 2", now, now, 0.42)

		// Page query binds query ($1), team_id ($2), project_id ($3), limit ($4), offset ($5),
		// and orders by distance ASC (= ts_rank DESC).
		mock.ExpectQuery(`source_body, created_at, updated_at, distance .*ORDER BY distance ASC, entity_id ASC LIMIT \$4 OFFSET \$5`).
			WithArgs(query, teamID, nil, 10, 0).
			WillReturnRows(resultRows)

		rows, total, err := repo.SearchKeyword(ctx, teamID, query, []string{"prompt", "artifact"}, "", 10, 0)

		require.NoError(t, err)
		assert.Equal(t, 2, total)
		require.Len(t, rows, 2)
		assert.Equal(t, "prompt", rows[0].EntityType)
		assert.Equal(t, "prompt-slug", rows[0].Slug)
		assert.InDelta(t, 0.13, rows[0].Distance, 0.0001)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("threads project filter into count and page queries", func(t *testing.T) {
		projectID := "11111111-2222-3333-4444-555555555555"
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(`).
			WithArgs(query, teamID, projectID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		resultRows := sqlmock.NewRows(resultCols).
			AddRow("artifact", "a-1", "a-1", "Title", "art-slug", projectID, "Scoped Project", "", "body", now, now, 0.2)

		mock.ExpectQuery(`ORDER BY distance ASC, entity_id ASC LIMIT \$4 OFFSET \$5`).
			WithArgs(query, teamID, projectID, 10, 0).
			WillReturnRows(resultRows)

		rows, total, err := repo.SearchKeyword(ctx, teamID, query, []string{"artifact"}, projectID, 10, 0)

		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, rows, 1)
		assert.Equal(t, projectID, rows[0].ProjectID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("short-circuits when count is zero", func(t *testing.T) {
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(`).
			WithArgs(query, teamID, nil).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		rows, total, err := repo.SearchKeyword(ctx, teamID, query, []string{"prompt"}, "", 10, 0)

		require.NoError(t, err)
		assert.Equal(t, 0, total)
		assert.Empty(t, rows)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("propagates page query error", func(t *testing.T) {
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(`).
			WithArgs(query, teamID, nil).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`ORDER BY distance ASC, entity_id ASC LIMIT \$4 OFFSET \$5`).
			WithArgs(query, teamID, nil, 10, 0).
			WillReturnError(sql.ErrConnDone)

		_, _, err := repo.SearchKeyword(ctx, teamID, query, []string{"prompt"}, "", 10, 0)

		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func indexOf(haystack, needle string) int {
	loc := regexp.MustCompile(regexp.QuoteMeta(needle)).FindStringIndex(haystack)
	if loc == nil {
		return -1
	}
	return loc[0]
}

func countOccurrences(haystack, needle string) int {
	return len(regexp.MustCompile(regexp.QuoteMeta(needle)).FindAllStringIndex(haystack, -1))
}
