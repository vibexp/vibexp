package postgres

import (
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

func TestAgentRepository_Create(t *testing.T) {
	ctx := contextWithLogger()

	tests := []struct {
		name       string
		agent      *models.Agent
		setupMock  func(mock sqlmock.Sqlmock)
		wantErr    string
		wantErrIs  error
		validateFn func(*testing.T, *models.Agent)
	}{
		{
			name: "successful create defaults status to active and zeroes stats",
			agent: &models.Agent{
				UserID:      "user-123",
				TeamID:      "team-123",
				Name:        "Code Reviewer",
				Description: "Reviews code",
				// Pre-set stats must be overwritten by Create.
				TotalRuns:   99,
				SuccessRate: 42.0,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`INSERT INTO agents`).
					WithArgs(
						sqlmock.AnyArg(), // id (generated)
						"user-123",
						"team-123",
						"Code Reviewer",
						"Reviews code",
						"active",         // defaulted status
						nil,              // card_url
						[]byte(nil),      // agent_card (nil card marshals to no bytes)
						int64(0),         // total_runs reset
						0.0,              // success_rate reset
						sqlmock.AnyArg(), // created_at
						sqlmock.AnyArg(), // updated_at
					).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			validateFn: func(t *testing.T, agent *models.Agent) {
				_, parseErr := uuid.Parse(agent.ID)
				assert.NoError(t, parseErr, "Create must assign a UUID id")
				assert.Equal(t, "active", agent.Status)
				assert.Equal(t, 0, agent.TotalRuns)
				assert.Zero(t, agent.SuccessRate)
				assert.False(t, agent.CreatedAt.IsZero())
				assert.False(t, agent.UpdatedAt.IsZero())
			},
		},
		{
			name: "explicit status is preserved",
			agent: &models.Agent{
				UserID: "user-123",
				TeamID: "team-123",
				Name:   "Paused Agent",
				Status: "paused",
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`INSERT INTO agents`).
					WithArgs(
						sqlmock.AnyArg(), "user-123", "team-123", "Paused Agent", "",
						"paused", nil, []byte(nil), int64(0), 0.0,
						sqlmock.AnyArg(), sqlmock.AnyArg(),
					).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			validateFn: func(t *testing.T, agent *models.Agent) {
				assert.Equal(t, "paused", agent.Status)
			},
		},
		{
			name: "unique violation maps to ErrAgentNameConflict naming the agent",
			agent: &models.Agent{
				UserID: "user-123",
				TeamID: "team-123",
				Name:   "Duplicate",
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`INSERT INTO agents`).
					WithArgs(
						sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
						sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
						sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					).
					WillReturnError(&pq.Error{Code: "23505"})
			},
			wantErrIs: repositories.ErrAgentNameConflict,
			wantErr:   `"Duplicate"`,
		},
		{
			name: "generic database error is wrapped",
			agent: &models.Agent{
				UserID: "user-123",
				TeamID: "team-123",
				Name:   "Broken",
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`INSERT INTO agents`).
					WithArgs(
						sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
						sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
						sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to create agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupAgentListTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			err := repo.Create(ctx, tt.agent)

			if tt.wantErrIs != nil || tt.wantErr != "" {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					assert.ErrorIs(t, err, tt.wantErrIs)
				}
				if tt.wantErr != "" {
					assert.Contains(t, err.Error(), tt.wantErr)
				}
			} else {
				require.NoError(t, err)
				if tt.validateFn != nil {
					tt.validateFn(t, tt.agent)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAgentRepository_UpdateExecutionStats(t *testing.T) {
	ctx := contextWithLogger()

	tests := []struct {
		name      string
		agentID   string
		success   bool
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
		wantErrIs error
	}{
		{
			name:    "success run binds increment 1",
			agentID: "agent-1",
			success: true,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`UPDATE agents\s+SET\s+total_runs = total_runs \+ 1`).
					WithArgs("agent-1", int64(1), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
		},
		{
			name:    "failed run binds increment 0",
			agentID: "agent-1",
			success: false,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`UPDATE agents\s+SET\s+total_runs = total_runs \+ 1`).
					WithArgs("agent-1", int64(0), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
		},
		{
			name:    "zero rows affected maps to ErrAgentNotFound with the agent id",
			agentID: "agent-missing",
			success: true,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`UPDATE agents`).
					WithArgs("agent-missing", int64(1), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantErrIs: repositories.ErrAgentNotFound,
			wantErr:   "agent-missing",
		},
		{
			name:    "exec error is wrapped",
			agentID: "agent-1",
			success: true,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`UPDATE agents`).
					WithArgs("agent-1", int64(1), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to update agent stats",
		},
		{
			name:    "RowsAffected error is wrapped",
			agentID: "agent-1",
			success: true,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`UPDATE agents`).
					WithArgs("agent-1", int64(1), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))
			},
			wantErr: "failed to get rows affected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupAgentListTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			err := repo.UpdateExecutionStats(ctx, tt.agentID, tt.success, 1500)

			if tt.wantErrIs != nil || tt.wantErr != "" {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					assert.ErrorIs(t, err, tt.wantErrIs)
				}
				if tt.wantErr != "" {
					assert.Contains(t, err.Error(), tt.wantErr)
				}
			} else {
				require.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
