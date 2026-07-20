package postgres

// sqlmock unit tests for RelationRepository (relation.go). The integration file
// (relation_integration_test.go) covers real-database semantics (idempotent
// create via the unique index, both-direction list, either-side delete); these
// tests pin the unit surface: argument wiring, row scanning, sentinel mapping,
// and error wrapping.

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

var relTestNow = time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)

var relTestColumns = []string{
	"id", "team_id", "project_id", "from_type", "from_id", "to_type", "to_id",
	"relation_type", "origin", "status", "created_by", "confirmed_by", "created_at", "updated_at",
}

func relFixtureRow() *sqlmock.Rows {
	return sqlmock.NewRows(relTestColumns).AddRow(
		"rel-1", "team-1", "proj-1", "artifact", "art-1", "blueprint", "bp-1",
		"governed-by", "human", "confirmed", "user-1", nil, relTestNow, relTestNow,
	)
}

func newRelationMockRepo(t *testing.T) (repositories.RelationRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, mockDB := newSquirrelMockRepo(t)
	return NewRelationRepository(db), mock, mockDB
}

func newRelationFixture() *models.Relation {
	createdBy := "user-1"
	return &models.Relation{
		TeamID: "team-1", ProjectID: "proj-1",
		FromType: "artifact", FromID: "art-1", ToType: "blueprint", ToID: "bp-1",
		RelationType: "governed-by", Origin: "human", Status: "confirmed",
		CreatedBy: &createdBy,
	}
}

func assertHappyRelation(t *testing.T, got *models.Relation) {
	t.Helper()
	require.NotNil(t, got)
	assert.Equal(t, "rel-1", got.ID)
	assert.Equal(t, "team-1", got.TeamID)
	assert.Equal(t, "proj-1", got.ProjectID)
	assert.Equal(t, "artifact", got.FromType)
	assert.Equal(t, "bp-1", got.ToID)
	assert.Equal(t, "governed-by", got.RelationType)
	assert.Equal(t, "confirmed", got.Status)
	require.NotNil(t, got.CreatedBy)
	assert.Equal(t, "user-1", *got.CreatedBy)
	assert.Nil(t, got.ConfirmedBy)
}

func TestRelationRepository_Create(t *testing.T) {
	t.Run("happy path inserts and scans the new row", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`INSERT INTO resource_relations`).
			WithArgs("team-1", "proj-1", "artifact", "art-1", "blueprint", "bp-1",
				"governed-by", "human", "confirmed", "user-1").
			WillReturnRows(relFixtureRow())

		got, created, err := repo.Create(context.Background(), newRelationFixture())
		require.NoError(t, err)
		assert.True(t, created, "a fresh insert reports created=true")
		assertHappyRelation(t, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("conflict (no row) fetches and returns the existing edge", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`INSERT INTO resource_relations`).
			WillReturnError(sql.ErrNoRows)
		mock.ExpectQuery(`FROM resource_relations WHERE team_id = \$1 AND from_type = \$2 AND from_id = \$3 AND relation_type = \$4 AND to_type = \$5 AND to_id = \$6`).
			WithArgs("team-1", "artifact", "art-1", "governed-by", "blueprint", "bp-1").
			WillReturnRows(relFixtureRow())

		got, created, err := repo.Create(context.Background(), newRelationFixture())
		require.NoError(t, err)
		assert.False(t, created, "a suppressed insert (conflict) reports created=false")
		assertHappyRelation(t, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("FK violation maps to team-or-project-not-found", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`INSERT INTO resource_relations`).
			WillReturnError(&pq.Error{Code: fkViolationCode})

		_, _, err := repo.Create(context.Background(), newRelationFixture())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "team or project not found for relation")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRelationRepository_GetByID(t *testing.T) {
	t.Run("happy path scans the full row", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`FROM resource_relations WHERE id = \$1 AND team_id = \$2`).
			WithArgs("rel-1", "team-1").WillReturnRows(relFixtureRow())

		got, err := repo.GetByID(context.Background(), "team-1", "rel-1")
		require.NoError(t, err)
		assertHappyRelation(t, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no rows maps to the not-found sentinel", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`FROM resource_relations WHERE id = \$1 AND team_id = \$2`).
			WithArgs("rel-1", "team-1").WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByID(context.Background(), "team-1", "rel-1")
		assert.ErrorIs(t, err, repositories.ErrRelationNotFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRelationRepository_Confirm(t *testing.T) {
	t.Run("happy path returns the confirmed row", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)

		confirmed := sqlmock.NewRows(relTestColumns).AddRow(
			"rel-1", "team-1", "proj-1", "artifact", "art-1", "blueprint", "bp-1",
			"governed-by", "ai", "confirmed", "user-1", "user-2", relTestNow, relTestNow)
		mock.ExpectQuery(`UPDATE resource_relations SET status = 'confirmed', confirmed_by = \$1, updated_at = now\(\) WHERE id = \$2 AND team_id = \$3 AND status = 'suggested'`).
			WithArgs("user-2", "rel-1", "team-1").WillReturnRows(confirmed)

		got, err := repo.Confirm(context.Background(), "team-1", "rel-1", "user-2")
		require.NoError(t, err)
		assert.Equal(t, "confirmed", got.Status)
		require.NotNil(t, got.ConfirmedBy)
		assert.Equal(t, "user-2", *got.ConfirmedBy)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no suggested row maps to the not-found sentinel", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`UPDATE resource_relations SET status = 'confirmed'`).
			WithArgs("user-2", "rel-1", "team-1").WillReturnError(sql.ErrNoRows)

		_, err := repo.Confirm(context.Background(), "team-1", "rel-1", "user-2")
		assert.ErrorIs(t, err, repositories.ErrRelationNotFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRelationRepository_Delete(t *testing.T) {
	t.Run("happy path deletes one row", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectExec(`DELETE FROM resource_relations WHERE id = \$1 AND team_id = \$2`).
			WithArgs("rel-1", "team-1").WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, repo.Delete(context.Background(), "team-1", "rel-1"))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("zero rows maps to the not-found sentinel", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectExec(`DELETE FROM resource_relations WHERE id = \$1 AND team_id = \$2`).
			WithArgs("rel-1", "team-1").WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Delete(context.Background(), "team-1", "rel-1")
		assert.ErrorIs(t, err, repositories.ErrRelationNotFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// DeleteByResource must match the resource on EITHER endpoint.
func TestRelationRepository_DeleteByResource(t *testing.T) {
	repo, mock, mockDB := newRelationMockRepo(t)
	defer closeMockDB(t, mockDB)

	mock.ExpectExec(`DELETE FROM resource_relations WHERE team_id = \$1 AND \(\(from_type = \$2 AND from_id = \$3\) OR \(to_type = \$2 AND to_id = \$3\)\)`).
		WithArgs("team-1", "artifact", "art-1").WillReturnResult(sqlmock.NewResult(0, 3))

	n, err := repo.DeleteByResource(context.Background(), "team-1", "artifact", "art-1")
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRelationRepository_ResourceProjectID(t *testing.T) {
	t.Run("returns project and exists=true", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT project_id FROM artifacts WHERE id = \$1 AND team_id = \$2`).
			WithArgs("art-1", "team-1").
			WillReturnRows(sqlmock.NewRows([]string{"project_id"}).AddRow("proj-1"))

		proj, exists, err := repo.ResourceProjectID(context.Background(), "team-1", "artifact", "art-1")
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, "proj-1", proj)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("missing resource reports exists=false without error", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT project_id FROM prompts WHERE id = \$1 AND team_id = \$2`).
			WithArgs("p-1", "team-1").WillReturnError(sql.ErrNoRows)

		proj, exists, err := repo.ResourceProjectID(context.Background(), "team-1", "prompt", "p-1")
		require.NoError(t, err)
		assert.False(t, exists)
		assert.Empty(t, proj)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("unknown resource type is rejected without touching the DB", func(t *testing.T) {
		repo, _, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)

		_, _, err := repo.ResourceProjectID(context.Background(), "team-1", "widget", "x")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown resource type")
	})
}

func TestRelationRepository_ListByResource(t *testing.T) {
	t.Run("happy path pins offset math and hydrates the other endpoint", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM resource_relations`).
			WithArgs("team-1", "artifact", "art-1").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
		listCols := []string{"id", "relation_type", "direction", "origin", "status",
			"other_type", "other_id", "created_at", "title", "project_id", "slug"}
		mock.ExpectQuery(`FROM edges e`).
			WithArgs("team-1", "artifact", "art-1", 10, 10).
			WillReturnRows(sqlmock.NewRows(listCols).
				AddRow("rel-1", "governed-by", "outgoing", "human", "confirmed",
					"blueprint", "bp-1", relTestNow, "Blueprint Title", "proj-1", "bp-slug").
				AddRow("rel-2", "explained-by", "incoming", "ai", "suggested",
					"memory", "mem-1", relTestNow, "memory excerpt", "proj-1", nil))

		related, total, err := repo.ListByResource(context.Background(), "team-1", "artifact", "art-1", 2, 10)
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		require.Len(t, related, 2)
		assert.Equal(t, "outgoing", related[0].Direction)
		assert.Equal(t, "blueprint", related[0].ResourceType)
		assert.Equal(t, "Blueprint Title", related[0].Title)
		require.NotNil(t, related[0].Slug)
		assert.Equal(t, "bp-slug", *related[0].Slug)
		assert.Equal(t, "incoming", related[1].Direction)
		assert.Nil(t, related[1].Slug, "a memory has no slug: NULL must map to nil")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("empty page returns a non-nil empty slice", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM resource_relations`).
			WithArgs("team-1", "artifact", "art-1").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(`FROM edges e`).
			WithArgs("team-1", "artifact", "art-1", 20, 0).
			WillReturnRows(sqlmock.NewRows([]string{"id", "relation_type", "direction", "origin", "status",
				"other_type", "other_id", "created_at", "title", "project_id", "slug"}))

		related, total, err := repo.ListByResource(context.Background(), "team-1", "artifact", "art-1", 1, 20)
		require.NoError(t, err)
		assert.Equal(t, 0, total)
		require.NotNil(t, related)
		assert.Empty(t, related)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// These pin the error-wrapping branches (driver failures, scan/iterate errors,
// rows-affected failures) that the happy-path suite skips.

func TestRelationRepository_Create_ErrorBranches(t *testing.T) {
	t.Run("generic driver error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`INSERT INTO resource_relations`).WillReturnError(sql.ErrConnDone)

		_, _, err := repo.Create(context.Background(), newRelationFixture())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create relation")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("conflict then missing existing row maps to not-found", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`INSERT INTO resource_relations`).WillReturnError(sql.ErrNoRows)
		mock.ExpectQuery(`FROM resource_relations WHERE team_id = \$1 AND from_type`).
			WillReturnError(sql.ErrNoRows)

		_, _, err := repo.Create(context.Background(), newRelationFixture())
		assert.ErrorIs(t, err, repositories.ErrRelationNotFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRelationRepository_ListByResource_ErrorBranches(t *testing.T) {
	countPattern := `SELECT COUNT\(\*\) FROM resource_relations`
	listCols := []string{"id", "relation_type", "direction", "origin", "status",
		"other_type", "other_id", "created_at", "title", "project_id", "slug"}

	t.Run("count error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)
		mock.ExpectQuery(countPattern).WithArgs("team-1", "artifact", "art-1").
			WillReturnError(sql.ErrConnDone)

		_, _, err := repo.ListByResource(context.Background(), "team-1", "artifact", "art-1", 1, 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to count relations")
	})

	t.Run("list query error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)
		mock.ExpectQuery(countPattern).WithArgs("team-1", "artifact", "art-1").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`FROM edges e`).WillReturnError(sql.ErrConnDone)

		_, _, err := repo.ListByResource(context.Background(), "team-1", "artifact", "art-1", 1, 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list relations")
	})

	t.Run("scan error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)
		mock.ExpectQuery(countPattern).WithArgs("team-1", "artifact", "art-1").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`FROM edges e`).WillReturnRows(sqlmock.NewRows(listCols).AddRow(
			"rel-1", "governed-by", "outgoing", "human", "confirmed",
			"blueprint", "bp-1", "not-a-time", "Title", "proj-1", "slug"))

		_, _, err := repo.ListByResource(context.Background(), "team-1", "artifact", "art-1", 1, 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to scan related resource")
	})

	t.Run("row iteration error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)
		mock.ExpectQuery(countPattern).WithArgs("team-1", "artifact", "art-1").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`FROM edges e`).WillReturnRows(sqlmock.NewRows(listCols).AddRow(
			"rel-1", "governed-by", "outgoing", "human", "confirmed",
			"blueprint", "bp-1", relTestNow, "Title", "proj-1", "slug").
			RowError(0, sql.ErrConnDone))

		_, _, err := repo.ListByResource(context.Background(), "team-1", "artifact", "art-1", 1, 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to iterate relations")
	})
}

func TestRelationRepository_Delete_ErrorBranches(t *testing.T) {
	t.Run("exec error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)
		mock.ExpectExec(`DELETE FROM resource_relations WHERE id = \$1 AND team_id = \$2`).
			WillReturnError(sql.ErrConnDone)

		err := repo.Delete(context.Background(), "team-1", "rel-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete relation")
	})

	t.Run("rows-affected error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)
		mock.ExpectExec(`DELETE FROM resource_relations WHERE id = \$1 AND team_id = \$2`).
			WillReturnResult(sqlmock.NewErrorResult(errors.New("boom")))

		err := repo.Delete(context.Background(), "team-1", "rel-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read delete result")
	})
}

func TestRelationRepository_DeleteByResource_ErrorBranches(t *testing.T) {
	deletePattern := `DELETE FROM resource_relations WHERE team_id = \$1`

	t.Run("exec error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)
		mock.ExpectExec(deletePattern).WillReturnError(sql.ErrConnDone)

		_, err := repo.DeleteByResource(context.Background(), "team-1", "artifact", "art-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete relations for resource")
	})

	t.Run("rows-affected error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := newRelationMockRepo(t)
		defer closeMockDB(t, mockDB)
		mock.ExpectExec(deletePattern).
			WillReturnResult(sqlmock.NewErrorResult(errors.New("boom")))

		_, err := repo.DeleteByResource(context.Background(), "team-1", "artifact", "art-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read delete result")
	})
}

func TestRelationRepository_ResourceProjectID_DriverError(t *testing.T) {
	repo, mock, mockDB := newRelationMockRepo(t)
	defer closeMockDB(t, mockDB)
	mock.ExpectQuery(`SELECT project_id FROM memories WHERE id = \$1 AND team_id = \$2`).
		WithArgs("m-1", "team-1").WillReturnError(sql.ErrConnDone)

	_, _, err := repo.ResourceProjectID(context.Background(), "team-1", "memory", "m-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve resource project")
}
