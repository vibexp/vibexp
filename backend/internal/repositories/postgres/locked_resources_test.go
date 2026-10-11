package postgres

// sqlmock helpers and unit tests for the locked-resource guards
// (locked_resources.go, issue #1290). The guards against a real schema are in
// locked_resources_integration_test.go.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

var lockedResourceTestColumns = []string{"resource_type", "id", "slug", "title"}

// expectLockedResources expects one locked-resource statement matching pattern,
// bound to args plus the row limit every such statement takes last, and has it
// return refs. No refs is the "nothing is locked" answer.
func expectLockedResources(
	mock sqlmock.Sqlmock, pattern string, args []driver.Value, refs ...models.LockedResourceRef,
) {
	rows := sqlmock.NewRows(lockedResourceTestColumns)
	for _, ref := range refs {
		rows.AddRow(ref.ResourceType, ref.ID, ref.Slug, ref.Title)
	}
	bound := append(append([]driver.Value{}, args...), models.LockedResourceListLimit)
	mock.ExpectQuery(pattern).WithArgs(bound...).WillReturnRows(rows)
}

// assertRefusedByLock asserts err is the LockedResourcesError a guard raises
// for reason, naming exactly want.
func assertRefusedByLock(t *testing.T, err, reason error, want ...models.LockedResourceRef) {
	t.Helper()
	var locked *repositories.LockedResourcesError
	require.ErrorAs(t, err, &locked)
	require.ErrorIs(t, err, reason)
	assert.Equal(t, want, locked.Resources)
}

var lockedTestArtifact = models.LockedResourceRef{
	ResourceType: models.RelationResourceTypeArtifact, ID: "artifact-1", Slug: "runbook", Title: "Runbook",
}

const projectLockedPattern = `FROM prompts WHERE team_id = \$1 AND project_id = \$2 AND locked_by IS NOT NULL ` +
	`UNION ALL SELECT 'blueprint', id::text, slug, title FROM blueprints ` +
	`WHERE team_id = \$1 AND project_id = \$2 AND locked_by IS NOT NULL ` +
	`UNION ALL SELECT 'artifact', id::text, slug, title FROM artifacts ` +
	`WHERE team_id = \$1 AND project_id = \$2 AND locked_by IS NOT NULL`

func TestProjectRepository_ListLockedResources(t *testing.T) {
	ctx := context.Background()
	lockedPrompt := models.LockedResourceRef{
		ResourceType: models.RelationResourceTypePrompt, ID: "prompt-1", Slug: "triage", Title: "Triage",
	}

	t.Run("lists the locked rows of the three tables, scoped to team and project", func(t *testing.T) {
		db, mock, mockDB := newSquirrelMockRepo(t)
		defer closeMockDB(t, mockDB)
		expectLockedResources(mock, projectLockedPattern,
			[]driver.Value{"team-1", "project-1"}, lockedTestArtifact, lockedPrompt)

		got, err := NewProjectRepository(db).ListLockedResources(ctx, "team-1", "project-1")
		require.NoError(t, err)
		assert.Equal(t, []models.LockedResourceRef{lockedTestArtifact, lockedPrompt}, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("nothing locked is an empty list, never nil", func(t *testing.T) {
		db, mock, mockDB := newSquirrelMockRepo(t)
		defer closeMockDB(t, mockDB)
		expectLockedResources(mock, projectLockedPattern, []driver.Value{"team-1", "project-1"})

		got, err := NewProjectRepository(db).ListLockedResources(ctx, "team-1", "project-1")
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("a query error is wrapped", func(t *testing.T) {
		db, mock, mockDB := newSquirrelMockRepo(t)
		defer closeMockDB(t, mockDB)
		mock.ExpectQuery(projectLockedPattern).WillReturnError(sql.ErrConnDone)

		got, err := NewProjectRepository(db).ListLockedResources(ctx, "team-1", "project-1")
		require.ErrorIs(t, err, sql.ErrConnDone)
		assert.ErrorContains(t, err, "failed to query locked resources")
		assert.Nil(t, got)
	})

	t.Run("the transactional read runs on the transaction", func(t *testing.T) {
		db, mock, mockDB := newSquirrelMockRepo(t)
		defer closeMockDB(t, mockDB)
		mock.ExpectBegin()
		expectLockedResources(mock, projectLockedPattern, []driver.Value{"team-1", "project-1"}, lockedPrompt)
		mock.ExpectRollback()

		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		got, err := NewProjectRepository(db).ListLockedResourcesTx(ctx, tx, "team-1", "project-1")
		require.NoError(t, err)
		assert.Equal(t, []models.LockedResourceRef{lockedPrompt}, got)
		require.NoError(t, tx.Rollback())
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
