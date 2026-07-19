package postgres

// Coverage for the prompt-repository methods no prior sub-issue pinned
// (coverage epic #358 / issue #393): the cross-team getters, CountByStatus,
// GetUserLabels, GetNamesByIDsCrossTeam, and the Create/Update/Delete error
// arms. sqlmock pins SQL text/shape; the assertions target the branch each row
// drives (slug-conflict mapping, not-found sentinels, projection).

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

// promptCrossTeamColumns mirrors the 15-column projection of the cross-team
// getters (includes version; is_shared is computed).
var promptCrossTeamColumns = []string{
	"id", "name", "slug", "description", "body", "user_id", "team_id",
	"project_id", "status", "mcp_expose", "labels", "created_at", "updated_at",
	"version", "is_shared",
}

func promptCrossTeamRow(now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(promptCrossTeamColumns).AddRow(
		"prompt-1", "Prompt 1", "prompt-1", "Desc", "Body", "user-1", "team-9",
		"project-1", "published", true, "{}", now, now, int64(2), false,
	)
}

func TestPromptRepository_GetByIDCrossTeam(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("found returns the prompt regardless of team", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`FROM prompts p .* WHERE p\.id = \$1 AND p\.user_id = \$2`).
			WithArgs("prompt-1", "user-1").
			WillReturnRows(promptCrossTeamRow(now))

		got, err := repo.GetByIDCrossTeam(ctx, "user-1", "prompt-1")
		require.NoError(t, err)
		assert.Equal(t, "prompt-1", got.ID)
		assert.Equal(t, "team-9", got.TeamID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no rows maps to ErrPromptNotFound", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`FROM prompts p`).
			WithArgs("missing", "user-1").
			WillReturnError(sql.ErrNoRows)

		got, err := repo.GetByIDCrossTeam(ctx, "user-1", "missing")
		assert.ErrorIs(t, err, repositories.ErrPromptNotFound)
		assert.Nil(t, got)
	})
}

func TestPromptRepository_GetBySlugCrossTeam(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("found returns the prompt by slug", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`FROM prompts p .* WHERE p\.slug = \$1 AND p\.user_id = \$2`).
			WithArgs("my-slug", "user-1").
			WillReturnRows(promptCrossTeamRow(now))

		got, err := repo.GetBySlugCrossTeam(ctx, "user-1", "my-slug")
		require.NoError(t, err)
		assert.Equal(t, "prompt-1", got.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no rows maps to ErrPromptNotFound", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`FROM prompts p`).
			WithArgs("missing-slug", "user-1").
			WillReturnError(sql.ErrNoRows)

		got, err := repo.GetBySlugCrossTeam(ctx, "user-1", "missing-slug")
		assert.ErrorIs(t, err, repositories.ErrPromptNotFound)
		assert.Nil(t, got)
	})
}

func TestPromptRepository_CountByStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("returns the count", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM prompts WHERE user_id = \$1 AND status = \$2`).
			WithArgs("user-1", "published").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))

		got, err := repo.CountByStatus(ctx, "user-1", "published")
		require.NoError(t, err)
		assert.Equal(t, 4, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM prompts`).
			WithArgs("user-1", "draft").
			WillReturnError(sql.ErrConnDone)

		got, err := repo.CountByStatus(ctx, "user-1", "draft")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to count prompts by status")
		assert.Zero(t, got)
	})
}

func TestPromptRepository_GetUserLabels(t *testing.T) {
	ctx := context.Background()

	t.Run("returns distinct labels", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT DISTINCT unnest\(labels\)`).
			WithArgs("user-1").
			WillReturnRows(sqlmock.NewRows([]string{"label"}).AddRow("ai").AddRow("ops"))

		got, err := repo.GetUserLabels(ctx, "user-1")
		require.NoError(t, err)
		assert.Equal(t, []string{"ai", "ops"}, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("empty result is a non-nil empty slice", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT DISTINCT unnest\(labels\)`).
			WithArgs("user-1").
			WillReturnRows(sqlmock.NewRows([]string{"label"}))

		got, err := repo.GetUserLabels(ctx, "user-1")
		require.NoError(t, err)
		assert.Equal(t, []string{}, got)
	})

	t.Run("query error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT DISTINCT unnest\(labels\)`).
			WithArgs("user-1").
			WillReturnError(sql.ErrConnDone)

		got, err := repo.GetUserLabels(ctx, "user-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get user labels")
		assert.Nil(t, got)
	})
}

func TestPromptRepository_GetNamesByIDsCrossTeam(t *testing.T) {
	ctx := context.Background()

	t.Run("empty ids short-circuits", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		got, err := repo.GetNamesByIDsCrossTeam(ctx, "user-1", nil)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{}, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns id→name map", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT pr.id, pr.name FROM prompts pr`).
			WithArgs("user-1", "p1", "p2").
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).
				AddRow("p1", "First").AddRow("p2", "Second"))

		got, err := repo.GetNamesByIDsCrossTeam(ctx, "user-1", []string{"p1", "p2"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"p1": "First", "p2": "Second"}, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT pr.id, pr.name FROM prompts pr`).
			WithArgs("user-1", "p1").
			WillReturnError(sql.ErrConnDone)

		got, err := repo.GetNamesByIDsCrossTeam(ctx, "user-1", []string{"p1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get prompt names by ids")
		assert.Nil(t, got)
	})
}

func TestPromptRepository_Create_ErrorArms(t *testing.T) {
	ctx := context.Background()
	prompt := func() *models.Prompt {
		return &models.Prompt{
			Name: "P", Slug: "p", UserID: "user-1", TeamID: "team-1", ProjectID: "proj-1",
			Labels: pq.StringArray{},
		}
	}

	t.Run("slug unique violation is a friendly conflict", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`INSERT INTO prompts`).
			WillReturnError(&pq.Error{Code: "23505", Detail: "Key (slug)=(p) already exists."})

		err := repo.Create(ctx, prompt())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("generic error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`INSERT INTO prompts`).
			WillReturnError(sql.ErrConnDone)

		err := repo.Create(ctx, prompt())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create prompt")
	})
}

func TestPromptRepository_Update_ErrorArms(t *testing.T) {
	ctx := context.Background()
	prompt := func() *models.Prompt {
		return &models.Prompt{
			ID: "prompt-1", TeamID: "team-1", Name: "P", Slug: "p", Version: 3,
			Labels: pq.StringArray{},
		}
	}
	validateRe := `SELECT EXISTS\(SELECT 1 FROM prompts p`
	updateRe := `UPDATE prompts`

	t.Run("missing in team maps to ErrPromptNotFound", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(validateRe).WithArgs("prompt-1", "team-1").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		err := repo.Update(ctx, prompt())
		assert.ErrorIs(t, err, repositories.ErrPromptNotFound)
	})

	t.Run("slug unique violation is a friendly conflict", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(validateRe).WithArgs("prompt-1", "team-1").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(updateRe).
			WillReturnError(&pq.Error{Code: "23505", Detail: "Key (slug)=(p) already exists."})

		err := repo.Update(ctx, prompt())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("no rows on RETURNING is a version conflict", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(validateRe).WithArgs("prompt-1", "team-1").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(updateRe).WillReturnError(sql.ErrNoRows)

		err := repo.Update(ctx, prompt())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version conflict")
	})
}

func TestPromptRepository_Delete_ErrorArms(t *testing.T) {
	ctx := context.Background()

	t.Run("no rows affected maps to ErrPromptNotFound", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectExec(`DELETE FROM prompts`).
			WithArgs("prompt-1", "team-1", "user-1").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Delete(ctx, "user-1", "team-1", "prompt-1")
		assert.ErrorIs(t, err, repositories.ErrPromptNotFound)
	})

	t.Run("exec error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupPromptListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectExec(`DELETE FROM prompts`).
			WithArgs("prompt-1", "team-1", "user-1").
			WillReturnError(sql.ErrConnDone)

		err := repo.Delete(ctx, "user-1", "team-1", "prompt-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete prompt")
	})
}
