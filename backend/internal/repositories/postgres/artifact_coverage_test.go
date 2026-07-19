package postgres

// Coverage for the artifact-repository methods no prior sub-issue pinned
// (coverage epic #358 / issue #393): the cross-team getters, GetStats, CountAll,
// GetNamesByIDsCrossTeam, and the Create/Update/Delete error arms. sqlmock pins
// SQL text/shape; the assertions target the branch each row drives (slug/FK
// conflict mapping, not-found sentinels, multi-query stats aggregation).

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// artifactCrossTeamColumns mirrors the 14-column projection of the cross-team getters.
var artifactCrossTeamColumns = []string{
	"id", "project_id", "slug", "user_id", "team_id", "title", "description",
	"content", "status", "type", "metadata", "created_at", "updated_at", "version",
}

func artifactCrossTeamRow(now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(artifactCrossTeamColumns).AddRow(
		"art-1", "proj-1", "my-slug", "user-1", "team-9", "Title", "Desc",
		"content", "published", "document", []byte(`{"k":"v"}`), now, now, int64(2),
	)
}

func TestArtifactRepository_GetByIDCrossTeam(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("found returns the artifact and decodes metadata", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`FROM artifacts\s+WHERE id = \$1 AND user_id = \$2`).
			WithArgs("art-1", "user-1").
			WillReturnRows(artifactCrossTeamRow(now))

		got, err := repo.GetByIDCrossTeam(ctx, "user-1", "art-1")
		require.NoError(t, err)
		assert.Equal(t, "art-1", got.ID)
		assert.Equal(t, "team-9", got.TeamID)
		assert.Equal(t, "v", got.Metadata["k"])
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no rows maps to ErrArtifactNotFound", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`FROM artifacts`).
			WithArgs("missing", "user-1").
			WillReturnError(sql.ErrNoRows)

		got, err := repo.GetByIDCrossTeam(ctx, "user-1", "missing")
		assert.ErrorIs(t, err, repositories.ErrArtifactNotFound)
		assert.Nil(t, got)
	})
}

func TestArtifactRepository_GetByProjectIDAndSlugCrossTeam(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("found returns the artifact by project+slug", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`WHERE project_id = \$1 AND slug = \$2 AND user_id = \$3`).
			WithArgs("proj-1", "my-slug", "user-1").
			WillReturnRows(artifactCrossTeamRow(now))

		got, err := repo.GetByProjectIDAndSlugCrossTeam(ctx, "user-1", "proj-1", "my-slug")
		require.NoError(t, err)
		assert.Equal(t, "art-1", got.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no rows maps to ErrArtifactNotFound", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`FROM artifacts`).
			WithArgs("proj-1", "missing", "user-1").
			WillReturnError(sql.ErrNoRows)

		got, err := repo.GetByProjectIDAndSlugCrossTeam(ctx, "user-1", "proj-1", "missing")
		assert.ErrorIs(t, err, repositories.ErrArtifactNotFound)
		assert.Nil(t, got)
	})
}

func TestArtifactRepository_GetStats(t *testing.T) {
	ctx := context.Background()

	t.Run("aggregates basic, by-type and by-status counts", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`COUNT\(DISTINCT project_id\)`).
			WithArgs("user-1", "team-1").
			WillReturnRows(sqlmock.NewRows(
				[]string{"total_projects", "total_artifacts", "added_this_week"},
			).AddRow(2, 5, 1))
		mock.ExpectQuery(`SELECT type, COUNT\(\*\)`).
			WithArgs("user-1", "team-1").
			WillReturnRows(sqlmock.NewRows([]string{"type", "count"}).
				AddRow("document", 3).AddRow("code", 2))
		mock.ExpectQuery(`SELECT status, COUNT\(\*\)`).
			WithArgs("user-1", "team-1").
			WillReturnRows(sqlmock.NewRows([]string{"status", "count"}).
				AddRow("published", 4).AddRow("draft", 1))

		got, err := repo.GetStats(ctx, "user-1", "team-1")
		require.NoError(t, err)
		assert.Equal(t, 2, got.TotalProjects)
		assert.Equal(t, 5, got.TotalArtifacts)
		assert.Equal(t, 3, got.TotalByType["document"])
		assert.Equal(t, 4, got.TotalByStatus["published"])
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("basic-stats error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`COUNT\(DISTINCT project_id\)`).
			WithArgs("user-1", "team-1").
			WillReturnError(sql.ErrConnDone)

		got, err := repo.GetStats(ctx, "user-1", "team-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get artifact stats")
		assert.Nil(t, got)
	})
}

func TestArtifactRepository_CountAll(t *testing.T) {
	ctx := context.Background()

	t.Run("returns the count", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts WHERE user_id = \$1`).
			WithArgs("user-1").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(9))

		got, err := repo.CountAll(ctx, "user-1")
		require.NoError(t, err)
		assert.Equal(t, 9, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts`).
			WithArgs("user-1").
			WillReturnError(sql.ErrConnDone)

		got, err := repo.CountAll(ctx, "user-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to count artifacts")
		assert.Zero(t, got)
	})
}

func TestArtifactRepository_GetNamesByIDsCrossTeam(t *testing.T) {
	ctx := context.Background()

	t.Run("empty ids short-circuits", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		got, err := repo.GetNamesByIDsCrossTeam(ctx, "user-1", nil)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{}, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns id→title map", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT a.id, a.title FROM artifacts a`).
			WithArgs("user-1", "a1", "a2").
			WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).
				AddRow("a1", "First").AddRow("a2", "Second"))

		got, err := repo.GetNamesByIDsCrossTeam(ctx, "user-1", []string{"a1", "a2"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"a1": "First", "a2": "Second"}, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT a.id, a.title FROM artifacts a`).
			WithArgs("user-1", "a1").
			WillReturnError(sql.ErrConnDone)

		got, err := repo.GetNamesByIDsCrossTeam(ctx, "user-1", []string{"a1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get artifact names by ids")
		assert.Nil(t, got)
	})
}

func TestArtifactRepository_Create_ErrorArms(t *testing.T) {
	ctx := context.Background()
	artifact := func() *models.Artifact {
		return &models.Artifact{
			ProjectID: "proj-1", Slug: "my-slug", UserID: "user-1", TeamID: "team-1",
			Title: "T", Type: "document", Metadata: map[string]interface{}{},
		}
	}

	t.Run("unique violation maps to a slug conflict", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`INSERT INTO artifacts`).
			WillReturnError(&pq.Error{Code: "23505", Detail: "Key (project_id, slug)=(proj-1, my-slug) already exists."})

		err := repo.Create(ctx, artifact())
		require.Error(t, err)
	})

	t.Run("foreign-key violation maps to project-not-found", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`INSERT INTO artifacts`).
			WillReturnError(&pq.Error{Code: "23503"})

		err := repo.Create(ctx, artifact())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "project not found")
	})

	t.Run("generic error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`INSERT INTO artifacts`).
			WillReturnError(sql.ErrConnDone)

		err := repo.Create(ctx, artifact())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create artifact")
	})
}

func TestArtifactRepository_Update_ErrorArms(t *testing.T) {
	ctx := context.Background()
	artifact := func() *models.Artifact {
		return &models.Artifact{
			ID: "art-1", TeamID: "team-1", Slug: "my-slug", Title: "T",
			Type: "document", Version: 3, Metadata: map[string]interface{}{},
		}
	}
	validateRe := `SELECT EXISTS\(SELECT 1 FROM artifacts a`
	updateRe := `UPDATE artifacts`

	t.Run("missing in team maps to ErrArtifactNotFound", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(validateRe).WithArgs("art-1", "team-1").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		err := repo.Update(ctx, artifact())
		assert.ErrorIs(t, err, repositories.ErrArtifactNotFound)
	})

	t.Run("foreign-key violation maps to project-not-found", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(validateRe).WithArgs("art-1", "team-1").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(updateRe).WillReturnError(&pq.Error{Code: "23503"})

		err := repo.Update(ctx, artifact())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "project not found")
	})

	t.Run("no rows on RETURNING is a version conflict", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(validateRe).WithArgs("art-1", "team-1").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(updateRe).WillReturnError(sql.ErrNoRows)

		err := repo.Update(ctx, artifact())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version conflict")
	})
}

func TestArtifactRepository_Delete_ErrorArms(t *testing.T) {
	ctx := context.Background()

	t.Run("no rows affected maps to ErrArtifactNotFound", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectExec(`DELETE FROM artifacts`).
			WithArgs("art-1", "team-1", "user-1").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Delete(ctx, "user-1", "team-1", "art-1")
		assert.ErrorIs(t, err, repositories.ErrArtifactNotFound)
	})

	t.Run("exec error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupArtifactListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectExec(`DELETE FROM artifacts`).
			WithArgs("art-1", "team-1", "user-1").
			WillReturnError(sql.ErrConnDone)

		err := repo.Delete(ctx, "user-1", "team-1", "art-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete artifact")
	})
}
