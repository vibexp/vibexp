package projectmigration

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sluggedTestService builds a Service that only needs a logger — migrateSluggedResources
// and the resolveIDs helpers operate purely on the passed *sql.Tx.
func sluggedTestService() *Service {
	return &Service{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

// beginMockTx returns a real *sql.Tx bound to a sqlmock driver plus the mock to
// program expectations against. Queries are matched by exact string (QueryMatcherEqual)
// so the assertions pin the SQL the service emits.
func beginMockTx(t *testing.T) (*sql.Tx, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	return tx, mock, func() { require.NoError(t, db.Close()) }
}

const (
	selectArtifactIDs = `SELECT id FROM artifacts WHERE project_id = $1`
	updateArtifacts   = `UPDATE artifacts SET project_id = $1, version = version + 1, updated_at = NOW() WHERE id = $2 AND project_id = $3`
)

// TestMigrateSluggedResources_MovesAllRows covers the happy path: every resolved
// row is reparented directly (no conflict path) and the migrated counter matches.
func TestMigrateSluggedResources_MovesAllRows(t *testing.T) {
	t.Parallel()
	tx, mock, cleanup := beginMockTx(t)
	defer cleanup()

	mock.ExpectQuery(selectArtifactIDs).WithArgs("src").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("a1").AddRow("a2"))
	mock.ExpectExec(updateArtifacts).WithArgs("dst", "a1", "src").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(updateArtifacts).WithArgs("dst", "a2", "src").
		WillReturnResult(sqlmock.NewResult(0, 1))

	var failed []ResourceOutcome
	var count int
	m := &sluggedMigration{
		srcProjectID:  "src",
		destProjectID: "dst",
		table:         "artifacts",
		sel:           ResourceSelection{All: true},
		allQuery:      selectArtifactIDs,
		failed:        &failed,
		count:         &count,
	}

	require.NoError(t, sluggedTestService().migrateSluggedResources(context.Background(), tx, m))
	assert.Equal(t, 2, count)
	assert.Empty(t, failed)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestMigrateSluggedResources_NoRows covers the early return when nothing is selected:
// no UPDATE is issued and the counter stays zero.
func TestMigrateSluggedResources_NoRows(t *testing.T) {
	t.Parallel()
	tx, mock, cleanup := beginMockTx(t)
	defer cleanup()

	mock.ExpectQuery(selectArtifactIDs).WithArgs("src").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	var failed []ResourceOutcome
	var count int
	m := &sluggedMigration{
		srcProjectID:  "src",
		destProjectID: "dst",
		table:         "artifacts",
		sel:           ResourceSelection{All: true},
		allQuery:      selectArtifactIDs,
		failed:        &failed,
		count:         &count,
	}

	require.NoError(t, sluggedTestService().migrateSluggedResources(context.Background(), tx, m))
	assert.Zero(t, count)
	assert.Empty(t, failed)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestMigrateSluggedResources_RecordsFailures covers the two per-row failure sinks:
// an exec error, and a zero-rows-affected concurrent modification.
func TestMigrateSluggedResources_RecordsFailures(t *testing.T) {
	t.Parallel()
	tx, mock, cleanup := beginMockTx(t)
	defer cleanup()

	mock.ExpectQuery(selectArtifactIDs).WithArgs("src").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("boom").AddRow("stale").AddRow("ok"))
	mock.ExpectExec(updateArtifacts).WithArgs("dst", "boom", "src").
		WillReturnError(errors.New("exec failed"))
	mock.ExpectExec(updateArtifacts).WithArgs("dst", "stale", "src").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(updateArtifacts).WithArgs("dst", "ok", "src").
		WillReturnResult(sqlmock.NewResult(0, 1))

	var failed []ResourceOutcome
	var count int
	m := &sluggedMigration{
		srcProjectID:  "src",
		destProjectID: "dst",
		table:         "artifacts",
		sel:           ResourceSelection{All: true},
		allQuery:      selectArtifactIDs,
		failed:        &failed,
		count:         &count,
	}

	require.NoError(t, sluggedTestService().migrateSluggedResources(context.Background(), tx, m))
	assert.Equal(t, 1, count)
	require.Len(t, failed, 2)
	assert.Equal(t, "boom", failed[0].ID)
	assert.Equal(t, "exec failed", failed[0].Reason)
	assert.Equal(t, "stale", failed[1].ID)
	assert.Equal(t, "concurrent_modification", failed[1].Reason)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestMigrateSluggedResources_ExplicitIDs covers the resolveIDs branch that validates
// an explicit ID list against the source project before moving.
func TestMigrateSluggedResources_ExplicitIDs(t *testing.T) {
	t.Parallel()
	tx, mock, cleanup := beginMockTx(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT id FROM artifacts WHERE project_id = $1 AND id IN ($2)`).
		WithArgs("src", "a1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("a1"))
	mock.ExpectExec(updateArtifacts).WithArgs("dst", "a1", "src").
		WillReturnResult(sqlmock.NewResult(0, 1))

	var failed []ResourceOutcome
	var count int
	m := &sluggedMigration{
		srcProjectID:  "src",
		destProjectID: "dst",
		table:         "artifacts",
		sel:           ResourceSelection{IDs: []string{"a1"}},
		allQuery:      selectArtifactIDs,
		failed:        &failed,
		count:         &count,
	}

	require.NoError(t, sluggedTestService().migrateSluggedResources(context.Background(), tx, m))
	assert.Equal(t, 1, count)
	assert.Empty(t, failed)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestMigrateArtifactsAndBlueprints covers the two thin builders, pinning the
// id-only selection query they now hand to migrateSluggedResources.
func TestMigrateArtifactsAndBlueprints(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		selectQuery string
		updateQuery string
		migrate     func(s *Service, ctx context.Context, tx *sql.Tx, req *MigrationRequest, res *MigrationResult) error
		count       func(res *MigrationResult) int
	}{
		{
			name:        "artifacts",
			selectQuery: selectArtifactIDs,
			updateQuery: updateArtifacts,
			migrate: func(s *Service, ctx context.Context, tx *sql.Tx, req *MigrationRequest, res *MigrationResult) error {
				return s.migrateArtifacts(ctx, tx, "src", req, res)
			},
			count: func(res *MigrationResult) int { return res.Migrated.Artifacts },
		},
		{
			name:        "blueprints",
			selectQuery: `SELECT id FROM blueprints WHERE project_id = $1`,
			updateQuery: `UPDATE blueprints SET project_id = $1, version = version + 1, updated_at = NOW() WHERE id = $2 AND project_id = $3`,
			migrate: func(s *Service, ctx context.Context, tx *sql.Tx, req *MigrationRequest, res *MigrationResult) error {
				return s.migrateBlueprints(ctx, tx, "src", req, res)
			},
			count: func(res *MigrationResult) int { return res.Migrated.Blueprints },
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tx, mock, cleanup := beginMockTx(t)
			defer cleanup()

			mock.ExpectQuery(tc.selectQuery).WithArgs("src").
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("r1"))
			mock.ExpectExec(tc.updateQuery).WithArgs("dst", "r1", "src").
				WillReturnResult(sqlmock.NewResult(0, 1))

			req := &MigrationRequest{
				DestinationProjectID: "dst",
				Resources: ResourceSelections{
					Artifacts:  ResourceSelection{All: true},
					Blueprints: ResourceSelection{All: true},
				},
			}
			var res MigrationResult
			require.NoError(t, tc.migrate(sluggedTestService(), context.Background(), tx, req, &res))
			assert.Equal(t, 1, tc.count(&res))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
