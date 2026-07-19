package postgres

// Coverage for the agent-repository methods no prior sub-issue pinned
// (coverage epic #358 / issue #393): the cross-team getters, GetStats,
// GetNamesByIDsCrossTeam, and the Update/Delete/GetByID error arms. sqlmock
// pins SQL text/shape only; the assertions target the branch each row of the
// table drives (error mapping, not-found sentinels, column projection).

import (
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

// agentCrossTeamColumns mirrors the 16-column projection of GetByIDCrossTeam.
var agentCrossTeamColumns = []string{
	"id", "user_id", "team_id", "name", "description", "status", "card_url",
	"agent_card", "credentials", "last_run", "last_synced_at", "total_runs",
	"success_rate", "created_at", "updated_at", "version",
}

func TestAgentRepository_GetByIDCrossTeam(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()

	tests := []struct {
		name       string
		setupMock  func(mock sqlmock.Sqlmock)
		wantErrIs  error
		wantErr    string
		validateFn func(*testing.T, *models.Agent)
	}{
		{
			name: "found returns the agent regardless of team",
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows(agentCrossTeamColumns).AddRow(
					"agent-9", "user-1", "team-other", "Shared Agent", "desc",
					"active", nil, nil, nil, nil, nil, 7, 80.0, now, now, 3,
				)
				mock.ExpectQuery(`FROM agents\s+WHERE id = \$1 AND user_id = \$2`).
					WithArgs("agent-9", "user-1").
					WillReturnRows(rows)
			},
			validateFn: func(t *testing.T, a *models.Agent) {
				assert.Equal(t, "agent-9", a.ID)
				assert.Equal(t, "team-other", a.TeamID)
				assert.Equal(t, 7, a.TotalRuns)
				assert.Equal(t, int64(3), a.Version)
			},
		},
		{
			name: "no rows maps to ErrAgentNotFound",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM agents`).
					WithArgs("missing", "user-1").
					WillReturnError(sql.ErrNoRows)
			},
			wantErrIs: repositories.ErrAgentNotFound,
		},
		{
			name: "generic error is wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM agents`).
					WithArgs("agent-9", "user-1").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to get agent (cross-team)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupAgentListTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			id := "agent-9"
			if tt.name == "no rows maps to ErrAgentNotFound" {
				id = "missing"
			}
			got, err := repo.GetByIDCrossTeam(ctx, "user-1", id)

			if tt.wantErrIs != nil || tt.wantErr != "" {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					assert.ErrorIs(t, err, tt.wantErrIs)
				}
				if tt.wantErr != "" {
					assert.Contains(t, err.Error(), tt.wantErr)
				}
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				tt.validateFn(t, got)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAgentRepository_GetStats(t *testing.T) {
	ctx := contextWithLogger()
	statsColumns := []string{
		"total_agents", "active_agents", "paused_agents",
		"error_agents", "total_runs", "avg_success_rate",
	}

	t.Run("all teams (empty teamID) sums across the user", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`FROM agents\s+WHERE user_id = \$1\s*$`).
			WithArgs("user-1").
			WillReturnRows(sqlmock.NewRows(statsColumns).AddRow(5, 3, 1, 1, 42, 77.5))

		got, err := repo.GetStats(ctx, "user-1", "")
		require.NoError(t, err)
		assert.Equal(t, 5, got.TotalAgents)
		assert.Equal(t, 3, got.ActiveAgents)
		assert.Equal(t, 77.5, got.AvgSuccessRate)
		// Non-nil empty slice so the required array never serializes as null.
		assert.NotNil(t, got.RecentActivities)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("specific team scopes by team_id", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`WHERE user_id = \$1 AND team_id = \$2`).
			WithArgs("user-1", "team-7").
			WillReturnRows(sqlmock.NewRows(statsColumns).AddRow(2, 2, 0, 0, 10, 100.0))

		got, err := repo.GetStats(ctx, "user-1", "team-7")
		require.NoError(t, err)
		assert.Equal(t, 2, got.TotalAgents)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`FROM agents`).
			WithArgs("user-1").
			WillReturnError(sql.ErrConnDone)

		got, err := repo.GetStats(ctx, "user-1", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get agent stats")
		assert.Nil(t, got)
	})
}

func TestAgentRepository_GetNamesByIDsCrossTeam(t *testing.T) {
	ctx := contextWithLogger()

	t.Run("empty ids short-circuits without a query", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		got, err := repo.GetNamesByIDsCrossTeam(ctx, "user-1", nil)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{}, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns id→name for accessible agents", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		rows := sqlmock.NewRows([]string{"id", "name"}).
			AddRow("a1", "Reviewer").
			AddRow("a2", "Summarizer")
		mock.ExpectQuery(`SELECT a.id, a.name FROM agents a`).
			WithArgs("user-1", "a1", "a2").
			WillReturnRows(rows)

		got, err := repo.GetNamesByIDsCrossTeam(ctx, "user-1", []string{"a1", "a2"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"a1": "Reviewer", "a2": "Summarizer"}, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT a.id, a.name FROM agents a`).
			WithArgs("user-1", "a1").
			WillReturnError(sql.ErrConnDone)

		got, err := repo.GetNamesByIDsCrossTeam(ctx, "user-1", []string{"a1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get agent names by ids")
		assert.Nil(t, got)
	})
}

func TestAgentRepository_Update_ErrorArms(t *testing.T) {
	ctx := contextWithLogger()
	agent := func() *models.Agent {
		return &models.Agent{
			ID: "agent-1", TeamID: "team-1", Name: "Renamed", Version: 4,
		}
	}
	existsRe := `SELECT EXISTS`
	updateRe := `UPDATE agents\s+SET name`

	t.Run("existence check error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(existsRe).WithArgs("agent-1", "team-1").
			WillReturnError(sql.ErrConnDone)

		err := repo.Update(ctx, agent())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to validate agent ownership")
	})

	t.Run("not present in team maps to ErrAgentNotFound", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(existsRe).WithArgs("agent-1", "team-1").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		err := repo.Update(ctx, agent())
		assert.ErrorIs(t, err, repositories.ErrAgentNotFound)
	})

	t.Run("no rows on RETURNING is a version conflict", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(existsRe).WithArgs("agent-1", "team-1").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(updateRe).WillReturnError(sql.ErrNoRows)

		err := repo.Update(ctx, agent())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version conflict")
	})

	t.Run("unique violation maps to ErrAgentNameConflict", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(existsRe).WithArgs("agent-1", "team-1").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(updateRe).WillReturnError(&pq.Error{Code: "23505"})

		err := repo.Update(ctx, agent())
		assert.ErrorIs(t, err, repositories.ErrAgentNameConflict)
	})

	t.Run("generic update error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(existsRe).WithArgs("agent-1", "team-1").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(updateRe).WillReturnError(sql.ErrConnDone)

		err := repo.Update(ctx, agent())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update agent")
	})
}

func TestAgentRepository_Delete_ErrorArms(t *testing.T) {
	ctx := contextWithLogger()

	t.Run("no rows affected maps to ErrAgentNotFound", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectExec(`DELETE FROM agents`).
			WithArgs("agent-1", "team-1", "user-1").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Delete(ctx, "user-1", "team-1", "agent-1")
		assert.ErrorIs(t, err, repositories.ErrAgentNotFound)
	})

	t.Run("exec error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectExec(`DELETE FROM agents`).
			WithArgs("agent-1", "team-1", "user-1").
			WillReturnError(sql.ErrConnDone)

		err := repo.Delete(ctx, "user-1", "team-1", "agent-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete agent")
	})

	t.Run("success deletes exactly one row", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectExec(`DELETE FROM agents`).
			WithArgs("agent-1", "team-1", "user-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Delete(ctx, "user-1", "team-1", "agent-1")
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
