package postgres

// sqlmock unit tests for TeamStatusRepository (team_status.go): argument
// wiring (team_id is bound on every statement), NULL created_by scanning,
// sentinel mapping and error wrapping. Constraint behaviour against a real
// schema is in team_status_integration_test.go.

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

func TestTeamStatusRepository_Create(t *testing.T) {
	creator := "user-1"
	cases := []struct {
		name      string
		createdBy *string
		queryErr  error
		wantIs    error
		wantSub   string
	}{
		{name: "binds the creator", createdBy: &creator},
		{name: "a nil creator is stored as NULL"},
		{
			name: "unique violation maps to the exists sentinel", createdBy: &creator,
			queryErr: &pq.Error{Code: uniqueViolationCode}, wantIs: repositories.ErrTeamStatusExists,
		},
		{
			name: "foreign-key violation names the missing parent", createdBy: &creator,
			queryErr: &pq.Error{Code: fkViolationCode}, wantSub: "team or creator not found",
		},
		{
			name: "any other error is wrapped", createdBy: &creator,
			queryErr: errTeamStatusBoom, wantIs: errTeamStatusBoom, wantSub: "failed to create team status",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock := newTeamStatusMockRepo(t)
			exp := mock.ExpectQuery(`INSERT INTO team_statuses \(team_id, resource_type, slug, name, created_by\)`).
				WithArgs("team-1", "artifact", "in-review", "In review", tc.createdBy)
			if tc.queryErr != nil {
				exp.WillReturnError(tc.queryErr)
			} else {
				exp.WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
					AddRow("status-1", teamStatusTestNow, teamStatusTestNow))
			}

			status := &models.TeamStatus{
				TeamID: "team-1", ResourceType: "artifact", Slug: "in-review",
				Name: "In review", CreatedBy: tc.createdBy,
			}
			err := repo.Create(context.Background(), status)

			assertTypeError(t, err, tc.wantIs, tc.wantSub)
			if tc.queryErr == nil {
				assert.Equal(t, "status-1", status.ID)
				assert.Equal(t, teamStatusTestNow, status.CreatedAt)
				assert.Equal(t, teamStatusTestNow, status.UpdatedAt)
			}
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
	const query = `UPDATE team_statuses\s+SET name = \$3, updated_at = now\(\)\s+WHERE team_id = \$1 AND id = \$2`

	t.Run("returns the updated row", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(query).WithArgs("team-1", "status-1", "In review").
			WillReturnRows(teamStatusRow("user-1"))

		got, err := repo.UpdateName(context.Background(), "team-1", "status-1", "In review")
		require.NoError(t, err)
		assert.Equal(t, "In review", got.Name)
		assert.Equal(t, "in-review", got.Slug)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no row maps to the not-found sentinel", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(query).WithArgs("team-1", "status-1", "X").WillReturnError(sql.ErrNoRows)

		_, err := repo.UpdateName(context.Background(), "team-1", "status-1", "X")
		assert.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)
	})

	t.Run("any other error is wrapped", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectQuery(query).WithArgs("team-1", "status-1", "X").WillReturnError(errTeamStatusBoom)

		_, err := repo.UpdateName(context.Background(), "team-1", "status-1", "X")
		assertTypeError(t, err, errTeamStatusBoom, "failed to rename team status")
	})
}

func TestTeamStatusRepository_Delete(t *testing.T) {
	const query = `DELETE FROM team_statuses WHERE team_id = \$1 AND id = \$2`

	t.Run("deletes the row", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectExec(query).WithArgs("team-1", "status-1").WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, repo.Delete(context.Background(), "team-1", "status-1"))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no row affected maps to the not-found sentinel", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectExec(query).WithArgs("team-1", "status-1").WillReturnResult(sqlmock.NewResult(0, 0))

		assert.ErrorIs(t, repo.Delete(context.Background(), "team-1", "status-1"),
			repositories.ErrTeamStatusNotFound)
	})

	t.Run("an exec error is wrapped", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectExec(query).WithArgs("team-1", "status-1").WillReturnError(errTeamStatusBoom)

		assertTypeError(t, repo.Delete(context.Background(), "team-1", "status-1"),
			errTeamStatusBoom, "failed to delete team status")
	})

	t.Run("an unreadable result is wrapped", func(t *testing.T) {
		repo, mock := newTeamStatusMockRepo(t)
		mock.ExpectExec(query).WithArgs("team-1", "status-1").
			WillReturnResult(sqlmock.NewErrorResult(errTeamStatusBoom))

		assertTypeError(t, repo.Delete(context.Background(), "team-1", "status-1"),
			errTeamStatusBoom, "failed to read team status delete result")
	})
}
