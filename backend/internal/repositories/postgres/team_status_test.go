package postgres

// sqlmock unit tests for TeamStatusRepository (team_status.go): argument
// wiring (team_id is bound on every statement), NULL created_by scanning,
// sentinel mapping, error wrapping, and the statement order of the three
// audited writes (change, audit entry, commit -- or a rollback). Constraint
// behaviour against a real schema is in team_status_integration_test.go, and
// the rollback itself in team_status_audit_integration_test.go.

import (
	"context"
	"database/sql"
	"encoding/json"
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

var teamStatusTestNow = time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)

var teamStatusTestColumns = []string{
	"id", "team_id", "resource_type", "slug", "name", "created_by", "created_at", "updated_at",
}

var errTeamStatusBoom = errors.New("connection reset")

func newTeamStatusMockRepo(t *testing.T) (repositories.TeamStatusRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, mockDB := newSquirrelMockRepo(t)
	t.Cleanup(func() { closeMockDB(t, mockDB) })
	return NewTeamStatusRepository(db), mock
}

func teamStatusRow(createdBy any) *sqlmock.Rows {
	return sqlmock.NewRows(teamStatusTestColumns).AddRow(
		"status-1", "team-1", "artifact", "in-review", "In review", createdBy,
		teamStatusTestNow, teamStatusTestNow,
	)
}

// testStatusAudit is the TeamStatusAuditBuilder for tests that are not about
// the audit entry: the smallest entry the table accepts, with no actor.
func testStatusAudit(change repositories.TeamStatusChange) (*models.TeamSettingsAudit, error) {
	return &models.TeamSettingsAudit{
		TeamID:  change.Status.TeamID,
		Surface: models.SettingsAuditSurfaceCustomStatuses,
	}, nil
}

// statusAuditRecorder is a TeamStatusAuditBuilder that keeps every change it
// was handed, and fails with err when set.
type statusAuditRecorder struct {
	changes []repositories.TeamStatusChange
	err     error
}

func (r *statusAuditRecorder) build(
	change repositories.TeamStatusChange,
) (*models.TeamSettingsAudit, error) {
	r.changes = append(r.changes, change)
	if r.err != nil {
		return nil, r.err
	}
	return testStatusAudit(change)
}

// expectTeamStatusAudit expects the insert of the entry testStatusAudit builds.
func expectTeamStatusAudit(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(`INSERT INTO team_settings_audit`).
		WithArgs("team-1", nil, models.SettingsAuditSurfaceCustomStatuses, nil, nil, nil, json.RawMessage(`{}`))
}

func teamStatusAuditStored() *sqlmock.Rows {
	return teamSettingsAuditRow(teamSettingsAuditRowArgs{
		ID: "audit-1", Surface: models.SettingsAuditSurfaceCustomStatuses, Detail: `{}`, CreatedAt: teamStatusTestNow,
	})
}

const (
	teamStatusInsertQuery = `INSERT INTO team_statuses \(team_id, resource_type, slug, name, created_by\)`
	teamStatusLockQuery   = `SELECT name\s+FROM team_statuses\s+WHERE team_id = \$1 AND id = \$2\s+FOR NO KEY UPDATE`
	teamStatusRenameQuery = `UPDATE team_statuses\s+SET name = \$3, updated_at = now\(\)\s+WHERE team_id = \$1 AND id = \$2`
	teamStatusDeleteQuery = `DELETE FROM team_statuses WHERE team_id = \$1 AND id = \$2 RETURNING`
)

func newTeamStatusToCreate(createdBy *string) *models.TeamStatus {
	return &models.TeamStatus{
		TeamID: "team-1", ResourceType: "artifact", Slug: "in-review", Name: "In review", CreatedBy: createdBy,
	}
}

func TestTeamStatusRepository_Create(t *testing.T) {
	creator := "user-1"

	for name, createdBy := range map[string]*string{
		"binds the creator":               &creator,
		"a nil creator is stored as NULL": nil,
	} {
		t.Run(name+", audits the inserted row and commits", func(t *testing.T) {
			repo, mock := newTeamStatusMockRepo(t)
			mock.ExpectBegin()
			mock.ExpectQuery(teamStatusInsertQuery).
				WithArgs("team-1", "artifact", "in-review", "In review", createdBy).
				WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
					AddRow("status-1", teamStatusTestNow, teamStatusTestNow))
			expectTeamStatusAudit(mock).WillReturnRows(teamStatusAuditStored())
			mock.ExpectCommit()

			status := newTeamStatusToCreate(createdBy)
			audit := &statusAuditRecorder{}
			require.NoError(t, repo.Create(context.Background(), status, audit.build))

			assert.Equal(t, "status-1", status.ID)
			assert.Equal(t, teamStatusTestNow, status.CreatedAt)
			assert.Equal(t, teamStatusTestNow, status.UpdatedAt)
			// The builder sees the row as inserted: its id is what the entry's
			// created_resource_id is taken from.
			require.Len(t, audit.changes, 1)
			assert.Equal(t, "status-1", audit.changes[0].Status.ID)
			assert.Equal(t, "in-review", audit.changes[0].Status.Slug)
			assert.Empty(t, audit.changes[0].PreviousName)
			assert.Zero(t, audit.changes[0].ReassignedCount)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}

	failures := []struct {
		name     string
		queryErr error
		wantIs   error
		wantSub  string
	}{
		{
			name:     "unique violation maps to the exists sentinel",
			queryErr: &pq.Error{Code: uniqueViolationCode}, wantIs: repositories.ErrTeamStatusExists,
		},
		{
			name:     "foreign-key violation names the missing parent",
			queryErr: &pq.Error{Code: fkViolationCode}, wantSub: "team or creator not found",
		},
		{
			name:     "any other error is wrapped",
			queryErr: errTeamStatusBoom, wantIs: errTeamStatusBoom, wantSub: "failed to create team status",
		},
	}
	for _, tc := range failures {
		t.Run(tc.name+" and nothing is audited", func(t *testing.T) {
			repo, mock := newTeamStatusMockRepo(t)
			mock.ExpectBegin()
			mock.ExpectQuery(teamStatusInsertQuery).WillReturnError(tc.queryErr)
			mock.ExpectRollback()

			status := newTeamStatusToCreate(&creator)
			audit := &statusAuditRecorder{}
			err := repo.Create(context.Background(), status, audit.build)

			assertTypeError(t, err, tc.wantIs, tc.wantSub)
			assert.Empty(t, audit.changes, "no entry is built for a row that was not inserted")
			assert.Empty(t, status.ID)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// The three writes share one transaction shape, so its failure modes are
// pinned once for all of them: whatever fails after the change -- building the
// entry, inserting it, the commit -- the transaction is rolled back and the
// caller is told nothing was written.
func TestTeamStatusRepository_AuditedWrites_FailTogether(t *testing.T) {
	ctx := context.Background()
	writes := map[string]struct {
		// change expects the statements of the write itself, succeeding.
		change func(mock sqlmock.Sqlmock)
		// call runs the write and reports whether it returned any result.
		call func(repo repositories.TeamStatusRepository, audit repositories.TeamStatusAuditBuilder) (bool, error)
	}{
		"create": {
			change: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(teamStatusInsertQuery).
					WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
						AddRow("status-1", teamStatusTestNow, teamStatusTestNow))
			},
			call: func(repo repositories.TeamStatusRepository, audit repositories.TeamStatusAuditBuilder) (bool, error) {
				status := newTeamStatusToCreate(nil)
				err := repo.Create(ctx, status, audit)
				return status.ID != "" || !status.CreatedAt.IsZero(), err
			},
		},
		"rename": {
			change: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(teamStatusLockQuery).
					WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("Old name"))
				mock.ExpectQuery(teamStatusRenameQuery).WillReturnRows(teamStatusRow(nil))
			},
			call: func(repo repositories.TeamStatusRepository, audit repositories.TeamStatusAuditBuilder) (bool, error) {
				got, err := repo.UpdateName(ctx, "team-1", "status-1", "In review", audit)
				return got != nil, err
			},
		},
		"delete": {
			change: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(teamStatusDeleteQuery).WillReturnRows(teamStatusRow(nil))
				mock.ExpectExec(`SELECT set_config`).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(`UPDATE artifacts SET status`).WillReturnResult(sqlmock.NewResult(0, 3))
			},
			call: func(repo repositories.TeamStatusRepository, audit repositories.TeamStatusAuditBuilder) (bool, error) {
				deleted, reassigned, err := repo.DeleteAndReassign(ctx, "team-1", "status-1", "active", audit)
				return deleted != nil || reassigned != 0, err
			},
		},
	}

	for op, write := range writes {
		t.Run(op+": a begin failure is wrapped", func(t *testing.T) {
			repo, mock := newTeamStatusMockRepo(t)
			mock.ExpectBegin().WillReturnError(errTeamStatusBoom)

			reported, err := write.call(repo, testStatusAudit)
			assertTypeError(t, err, errTeamStatusBoom, "failed to begin team status "+op)
			assert.False(t, reported)
			assert.NoError(t, mock.ExpectationsWereMet())
		})

		t.Run(op+": an entry that cannot be built rolls the change back", func(t *testing.T) {
			repo, mock := newTeamStatusMockRepo(t)
			mock.ExpectBegin()
			write.change(mock)
			mock.ExpectRollback()

			audit := &statusAuditRecorder{err: errTeamStatusBoom}
			reported, err := write.call(repo, audit.build)
			assertTypeError(t, err, errTeamStatusBoom, "failed to build team status "+op+" audit entry")
			assert.False(t, reported)
			assert.NoError(t, mock.ExpectationsWereMet())
		})

		t.Run(op+": a builder that returns no entry rolls the change back", func(t *testing.T) {
			repo, mock := newTeamStatusMockRepo(t)
			mock.ExpectBegin()
			write.change(mock)
			mock.ExpectRollback()

			reported, err := write.call(repo,
				func(repositories.TeamStatusChange) (*models.TeamSettingsAudit, error) { return nil, nil })
			require.ErrorIs(t, err, errTeamStatusAuditRequired)
			assert.False(t, reported)
			assert.NoError(t, mock.ExpectationsWereMet())
		})

		t.Run(op+": an entry that cannot be stored rolls the change back", func(t *testing.T) {
			repo, mock := newTeamStatusMockRepo(t)
			mock.ExpectBegin()
			write.change(mock)
			expectTeamStatusAudit(mock).WillReturnError(errTeamStatusBoom)
			mock.ExpectRollback()

			reported, err := write.call(repo, testStatusAudit)
			assertTypeError(t, err, errTeamStatusBoom, "failed to audit team status "+op)
			assert.False(t, reported)
			assert.NoError(t, mock.ExpectationsWereMet())
		})

		t.Run(op+": a commit failure is wrapped and nothing is reported", func(t *testing.T) {
			repo, mock := newTeamStatusMockRepo(t)
			mock.ExpectBegin()
			write.change(mock)
			expectTeamStatusAudit(mock).WillReturnRows(teamStatusAuditStored())
			mock.ExpectCommit().WillReturnError(errTeamStatusBoom)

			reported, err := write.call(repo, testStatusAudit)
			assertTypeError(t, err, errTeamStatusBoom, "failed to commit team status "+op)
			assert.False(t, reported)
			assert.NoError(t, mock.ExpectationsWereMet())
		})

		t.Run(op+": a missing builder is refused before any statement", func(t *testing.T) {
			repo, mock := newTeamStatusMockRepo(t)

			reported, err := write.call(repo, nil)
			require.ErrorIs(t, err, errTeamStatusAuditRequired)
			assert.False(t, reported)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestTeamStatusRepository_GetByID(t *testing.T) {
	const query = `FROM team_statuses\s+WHERE team_id = \$1 AND id = \$2`

	t.Run("scans a row with a creator", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(query).WithArgs("team-1", "status-1").WillReturnRows(teamStatusRow("user-1"))

		got, err := repo.GetByID(context.Background(), "team-1", "status-1")
		require.NoError(t, err)
		creator := "user-1"
		assert.Equal(t, &models.TeamStatus{
			ID: "status-1", TeamID: "team-1", ResourceType: "artifact", Slug: "in-review",
			Name: "In review", CreatedBy: &creator,
			CreatedAt: teamStatusTestNow, UpdatedAt: teamStatusTestNow,
		}, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a NULL creator scans to nil", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(query).WithArgs("team-1", "status-1").WillReturnRows(teamStatusRow(nil))

		got, err := repo.GetByID(context.Background(), "team-1", "status-1")
		require.NoError(t, err)
		assert.Nil(t, got.CreatedBy)
	})

	t.Run("no row maps to the not-found sentinel", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(query).WithArgs("team-1", "status-1").WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByID(context.Background(), "team-1", "status-1")
		assert.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)
	})

	t.Run("any other error is wrapped", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(query).WithArgs("team-1", "status-1").WillReturnError(errTeamStatusBoom)

		_, err := repo.GetByID(context.Background(), "team-1", "status-1")
		assertTypeError(t, err, errTeamStatusBoom, "failed to get team status")
	})
}

func TestTeamStatusRepository_List(t *testing.T) {
	const query = `FROM team_statuses\s+WHERE team_id = \$1 AND resource_type = \$2\s+ORDER BY name ASC, slug ASC`

	t.Run("returns the rows in query order", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(query).WithArgs("team-1", "artifact").WillReturnRows(
			teamStatusRow("user-1").AddRow(
				"status-2", "team-1", "artifact", "shipped", "Shipped", nil,
				teamStatusTestNow, teamStatusTestNow,
			))

		got, err := repo.List(context.Background(), "team-1", "artifact")
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, "in-review", got[0].Slug)
		assert.Equal(t, "shipped", got[1].Slug)
		assert.Nil(t, got[1].CreatedBy)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no rows is an empty, non-nil slice", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(query).WithArgs("team-1", "artifact").
			WillReturnRows(sqlmock.NewRows(teamStatusTestColumns))

		got, err := repo.List(context.Background(), "team-1", "artifact")
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("a query error is wrapped", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(query).WithArgs("team-1", "artifact").WillReturnError(errTeamStatusBoom)

		_, err := repo.List(context.Background(), "team-1", "artifact")
		assertTypeError(t, err, errTeamStatusBoom, "team statuses")
	})

	t.Run("a scan error is wrapped", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(query).WithArgs("team-1", "artifact").
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("status-1"))

		_, err := repo.List(context.Background(), "team-1", "artifact")
		assertTypeError(t, err, nil, "failed to scan team statuses row")
	})
}

func TestTeamStatusRepository_Count(t *testing.T) {
	const query = `SELECT count\(\*\) FROM team_statuses WHERE team_id = \$1 AND resource_type = \$2`

	t.Run("returns the count", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(query).WithArgs("team-1", "memory").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

		got, err := repo.Count(context.Background(), "team-1", "memory")
		require.NoError(t, err)
		assert.Equal(t, 3, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("an error is wrapped", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(query).WithArgs("team-1", "memory").WillReturnError(errTeamStatusBoom)

		_, err := repo.Count(context.Background(), "team-1", "memory")
		assertTypeError(t, err, errTeamStatusBoom, "failed to count team statuses")
	})
}

func TestTeamStatusRepository_UpdateName(t *testing.T) {
	ctx := context.Background()

	t.Run("locks the row, renames it and audits the name it replaced", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery(teamStatusLockQuery).WithArgs("team-1", "status-1").
			WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("Old name"))
		mock.ExpectQuery(teamStatusRenameQuery).WithArgs("team-1", "status-1", "In review").
			WillReturnRows(teamStatusRow("user-1"))
		expectTeamStatusAudit(mock).WillReturnRows(teamStatusAuditStored())
		mock.ExpectCommit()

		audit := &statusAuditRecorder{}
		got, err := repo.UpdateName(ctx, "team-1", "status-1", "In review", audit.build)
		require.NoError(t, err)
		assert.Equal(t, "In review", got.Name)
		assert.Equal(t, "in-review", got.Slug)

		require.Len(t, audit.changes, 1)
		assert.Equal(t, "Old name", audit.changes[0].PreviousName)
		assert.Equal(t, "In review", audit.changes[0].Status.Name)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no row under the lock is not-found: nothing is renamed or audited", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery(teamStatusLockQuery).WithArgs("team-1", "status-1").WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()

		audit := &statusAuditRecorder{}
		_, err := repo.UpdateName(ctx, "team-1", "status-1", "X", audit.build)
		require.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)
		assert.Empty(t, audit.changes)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a lock failure is wrapped", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery(teamStatusLockQuery).WillReturnError(errTeamStatusBoom)
		mock.ExpectRollback()

		_, err := repo.UpdateName(ctx, "team-1", "status-1", "X", testStatusAudit)
		assertTypeError(t, err, errTeamStatusBoom, "failed to read team status for rename")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	for name, tc := range map[string]struct {
		updateErr error
		wantIs    error
		wantSub   string
	}{
		"an update that matches no row maps to the not-found sentinel": {
			updateErr: sql.ErrNoRows, wantIs: repositories.ErrTeamStatusNotFound,
		},
		"any other update error is wrapped": {
			updateErr: errTeamStatusBoom, wantIs: errTeamStatusBoom, wantSub: "failed to rename team status",
		},
	} {
		t.Run(name, func(t *testing.T) {
			repo, mock := newTeamStatusMockRepo(t)
			mock.ExpectBegin()
			mock.ExpectQuery(teamStatusLockQuery).
				WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("Old name"))
			mock.ExpectQuery(teamStatusRenameQuery).WithArgs("team-1", "status-1", "X").WillReturnError(tc.updateErr)
			mock.ExpectRollback()

			audit := &statusAuditRecorder{}
			_, err := repo.UpdateName(ctx, "team-1", "status-1", "X", audit.build)
			assertTypeError(t, err, tc.wantIs, tc.wantSub)
			assert.Empty(t, audit.changes)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestTeamStatusRepository_UsageCounts(t *testing.T) {
	tables := map[string]string{
		models.StatusResourceTypePrompt:    "prompts",
		models.StatusResourceTypeArtifact:  "artifacts",
		models.StatusResourceTypeBlueprint: "blueprints",
		models.StatusResourceTypeMemory:    "memories",
	}
	require.Len(t, tables, len(models.StatusResourceTypes), "every status resource type has a table")

	for resourceType, table := range tables {
		t.Run(resourceType+" counts its own table by status, scoped to the team", func(t *testing.T) {
			repo, mock := newTeamStatusMockRepo(t)
			mock.ExpectQuery(`SELECT status, count\(\*\) FROM ` + table + ` WHERE team_id = \$1 GROUP BY status`).
				WithArgs("team-1").
				WillReturnRows(sqlmock.NewRows([]string{"status", "count"}).
					AddRow("active", 4).AddRow("in-review", 2))

			got, err := repo.UsageCounts(context.Background(), "team-1", resourceType)
			require.NoError(t, err)
			assert.Equal(t, map[string]int{"active": 4, "in-review": 2}, got)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}

	t.Run("no resources is an empty map, not nil", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(`FROM artifacts`).WithArgs("team-1").
			WillReturnRows(sqlmock.NewRows([]string{"status", "count"}))

		got, err := repo.UsageCounts(context.Background(), "team-1", models.StatusResourceTypeArtifact)
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("an unknown resource type runs no query", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)

		_, err := repo.UsageCounts(context.Background(), "team-1", "artifacts; DROP TABLE users")
		require.ErrorIs(t, err, errTeamStatusResourceTypeUnknown)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a query error is wrapped", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(`FROM memories`).WithArgs("team-1").WillReturnError(errTeamStatusBoom)

		_, err := repo.UsageCounts(context.Background(), "team-1", models.StatusResourceTypeMemory)
		assertTypeError(t, err, errTeamStatusBoom, "failed to query status usage counts")
	})
}

func TestTeamStatusRepository_DeleteAndReassign(t *testing.T) {
	const (
		deleteQuery   = teamStatusDeleteQuery
		preserveQuery = `SELECT set_config\('vibexp.preserve_updated_at', 'on', true\)`
		reassignQuery = `UPDATE artifacts SET status = \$2 WHERE team_id = \$1 AND status = \$3`
	)
	ctx := context.Background()

	t.Run("deletes, reassigns by the deleted row's slug, and commits", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery(deleteQuery).WithArgs("team-1", "status-1").WillReturnRows(teamStatusRow("user-1"))
		mock.ExpectExec(preserveQuery).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(reassignQuery).WithArgs("team-1", "active", "in-review").
			WillReturnResult(sqlmock.NewResult(0, 7))
		expectTeamStatusAudit(mock).WillReturnRows(teamStatusAuditStored())
		mock.ExpectCommit()

		audit := &statusAuditRecorder{}
		deleted, reassigned, err := repo.DeleteAndReassign(ctx, "team-1", "status-1", "active", audit.build)
		require.NoError(t, err)
		// The entry is built from the deleted row and the count the
		// reassignment reported, after both ran.
		require.Len(t, audit.changes, 1)
		assert.Equal(t, 7, audit.changes[0].ReassignedCount)
		assert.Equal(t, "in-review", audit.changes[0].Status.Slug)
		assert.Equal(t, "in-review", deleted.Slug)
		assert.Equal(t, "In review", deleted.Name)
		assert.Equal(t, 7, reassigned)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("each resource type rewrites only its own table", func(t *testing.T) {
		for resourceType, table := range map[string]string{
			models.StatusResourceTypePrompt:    "prompts",
			models.StatusResourceTypeBlueprint: "blueprints",
			models.StatusResourceTypeMemory:    "memories",
		} {
			repo, mock := newTeamStatusMockRepo(t)
			mock.ExpectBegin()
			mock.ExpectQuery(deleteQuery).WithArgs("team-1", "status-1").
				WillReturnRows(sqlmock.NewRows(teamStatusTestColumns).AddRow(
					"status-1", "team-1", resourceType, "in-review", "In review", nil,
					teamStatusTestNow, teamStatusTestNow))
			mock.ExpectExec(preserveQuery).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(`UPDATE `+table+` SET status = \$2 WHERE team_id = \$1 AND status = \$3`).
				WithArgs("team-1", "draft", "in-review").WillReturnResult(sqlmock.NewResult(0, 1))
			expectTeamStatusAudit(mock).WillReturnRows(teamStatusAuditStored())
			mock.ExpectCommit()

			_, reassigned, err := repo.DeleteAndReassign(ctx, "team-1", "status-1", "draft", testStatusAudit)
			require.NoError(t, err, resourceType)
			assert.Equal(t, 1, reassigned, resourceType)
			assert.NoError(t, mock.ExpectationsWereMet(), resourceType)
		}
	})

	t.Run("an unknown or foreign id rolls back as not-found", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery(deleteQuery).WithArgs("team-1", "status-1").WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()

		_, _, err := repo.DeleteAndReassign(ctx, "team-1", "status-1", "active", testStatusAudit)
		require.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a row of an unknown resource type rolls back", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery(deleteQuery).WithArgs("team-1", "status-1").
			WillReturnRows(sqlmock.NewRows(teamStatusTestColumns).AddRow(
				"status-1", "team-1", "widget", "in-review", "In review", nil,
				teamStatusTestNow, teamStatusTestNow))
		mock.ExpectRollback()

		_, _, err := repo.DeleteAndReassign(ctx, "team-1", "status-1", "active", testStatusAudit)
		require.ErrorIs(t, err, errTeamStatusResourceTypeUnknown)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	failures := map[string]struct {
		arrange func(mock sqlmock.Sqlmock)
		msg     string
	}{
		"delete": {
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(deleteQuery).WillReturnError(errTeamStatusBoom)
				mock.ExpectRollback()
			},
			msg: "failed to delete team status",
		},
		"preserve updated_at": {
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(deleteQuery).WillReturnRows(teamStatusRow(nil))
				mock.ExpectExec(preserveQuery).WillReturnError(errTeamStatusBoom)
				mock.ExpectRollback()
			},
			msg: "failed to preserve updated_at for status reassignment",
		},
		"reassign": {
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(deleteQuery).WillReturnRows(teamStatusRow(nil))
				mock.ExpectExec(preserveQuery).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(reassignQuery).WillReturnError(errTeamStatusBoom)
				mock.ExpectRollback()
			},
			msg: "failed to reassign artifact resources after status delete",
		},
		"reassign result": {
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(deleteQuery).WillReturnRows(teamStatusRow(nil))
				mock.ExpectExec(preserveQuery).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(reassignQuery).WillReturnResult(sqlmock.NewErrorResult(errTeamStatusBoom))
				mock.ExpectRollback()
			},
			msg: "failed to read status reassignment result",
		},
	}
	for name, tc := range failures {
		t.Run("a "+name+" failure is wrapped and nothing is reported as deleted", func(t *testing.T) {
			repo, mock := newTeamStatusMockRepo(t)
			tc.arrange(mock)

			deleted, reassigned, err := repo.DeleteAndReassign(ctx, "team-1", "status-1", "active", testStatusAudit)
			assertTypeError(t, err, errTeamStatusBoom, tc.msg)
			assert.Nil(t, deleted)
			assert.Zero(t, reassigned)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestTeamStatusRepository_ConfirmSlug(t *testing.T) {
	// FOR KEY SHARE is the whole point of the statement: without it the read
	// would not wait for a concurrent delete and would see its row (#1332).
	const confirmQuery = `SELECT 1 FROM team_statuses ` +
		`WHERE team_id = \$1 AND resource_type = \$2 AND slug = \$3 FOR KEY SHARE`
	ctx := context.Background()

	t.Run("a row is a confirmation", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(confirmQuery).WithArgs("team-1", "artifact", "in-review").
			WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))

		confirmed, err := repo.ConfirmSlug(ctx, "team-1", "artifact", "in-review")
		require.NoError(t, err)
		assert.True(t, confirmed)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no row is an answer, not an error", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(confirmQuery).WithArgs("team-1", "artifact", "in-review").
			WillReturnError(sql.ErrNoRows)

		confirmed, err := repo.ConfirmSlug(ctx, "team-1", "artifact", "in-review")
		require.NoError(t, err)
		assert.False(t, confirmed)
	})

	t.Run("a query error is wrapped and confirms nothing", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(confirmQuery).WillReturnError(errTeamStatusBoom)

		confirmed, err := repo.ConfirmSlug(ctx, "team-1", "artifact", "in-review")
		assertTypeError(t, err, errTeamStatusBoom, "failed to confirm team status")
		assert.False(t, confirmed)
	})
}

func TestTeamStatusRepository_ReassignResource(t *testing.T) {
	ctx := context.Background()

	for resourceType, table := range map[string]string{
		models.StatusResourceTypePrompt:    "prompts",
		models.StatusResourceTypeArtifact:  "artifacts",
		models.StatusResourceTypeBlueprint: "blueprints",
		models.StatusResourceTypeMemory:    "memories",
	} {
		t.Run(resourceType+" moves the one row, scoped to the team and the old status", func(t *testing.T) {
			repo, mock := newTeamStatusMockRepo(t)
			mock.ExpectExec(`UPDATE `+table+` SET status = \$2 WHERE team_id = \$1 AND status = \$3 AND id = \$4`).
				WithArgs("team-1", "active", "in-review", "resource-1").
				WillReturnResult(sqlmock.NewResult(0, 1))

			require.NoError(t, repo.ReassignResource(ctx, "team-1", resourceType, "resource-1", "in-review", "active"))
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}

	// Something else already moved the resource: that is the outcome wanted.
	t.Run("no matching row is not an error", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectExec(`UPDATE artifacts`).WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, repo.ReassignResource(ctx, "team-1", "artifact", "resource-1", "in-review", "active"))
	})

	t.Run("an unknown resource type runs no statement", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)

		err := repo.ReassignResource(ctx, "team-1", "widget", "resource-1", "in-review", "active")
		require.ErrorIs(t, err, errTeamStatusResourceTypeUnknown)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a statement error is wrapped with the resource", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectExec(`UPDATE memories`).WillReturnError(errTeamStatusBoom)

		err := repo.ReassignResource(ctx, "team-1", "memory", "resource-1", "in-review", "active")
		assertTypeError(t, err, errTeamStatusBoom, "failed to reassign memory resource-1 after status delete")
	})
}
