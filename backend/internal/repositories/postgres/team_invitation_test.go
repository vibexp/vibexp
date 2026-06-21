package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

func setupTeamInvitationTest(t *testing.T) (*TeamInvitationRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := NewTeamInvitationRepository(db).(*TeamInvitationRepository)

	return repo, mock, mockDB
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamInvitationRepository_Create(t *testing.T) {
	repo, mock, mockDB := setupTeamInvitationTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)

	tests := []struct {
		name       string
		invitation *models.TeamInvitation
		setupMock  func()
		expectErr  bool
	}{
		{
			name: "successful create",
			invitation: &models.TeamInvitation{
				TeamID:       "team-123",
				InviterID:    "user-123",
				InviteeEmail: "invite@example.com",
				Role:         models.TeamMemberRoleMember,
				Token:        "token-abc123",
				Status:       models.InvitationStatusPending,
				ExpiresAt:    expiresAt,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
					AddRow("inv-123", now, now)

				mock.ExpectQuery(`INSERT INTO team_invitations`).
					WithArgs(
						"team-123",
						"user-123",
						"invite@example.com",
						models.TeamMemberRoleMember,
						"token-abc123",
						models.InvitationStatusPending,
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
					).
					WillReturnRows(rows)
			},
			expectErr: false,
		},
		{
			name: "database error",
			invitation: &models.TeamInvitation{
				TeamID:       "team-error",
				InviterID:    "user-error",
				InviteeEmail: "error@example.com",
				Role:         models.TeamMemberRoleAdmin,
				Token:        "token-error",
				Status:       models.InvitationStatusPending,
				ExpiresAt:    expiresAt,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			setupMock: func() {
				mock.ExpectQuery(`INSERT INTO team_invitations`).
					WithArgs(
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
					).
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.Create(ctx, tt.invitation)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, tt.invitation.ID)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamInvitationRepository_GetByID(t *testing.T) {
	repo, mock, mockDB := setupTeamInvitationTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)

	tests := []struct {
		name         string
		invitationID string
		setupMock    func()
		expectErr    bool
		validateFn   func(*testing.T, *models.TeamInvitation)
	}{
		{
			name:         "successful retrieval",
			invitationID: "inv-123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "team_id", "inviter_id", "invitee_email", "role", "token",
					"status", "expires_at", "created_at", "updated_at",
				}).AddRow(
					"inv-123", "team-123", "user-123", "invite@example.com",
					models.TeamMemberRoleMember, "token-abc123",
					models.InvitationStatusPending, expiresAt, now, now,
				)

				mock.ExpectQuery(`SELECT .+ FROM team_invitations WHERE id`).
					WithArgs("inv-123").
					WillReturnRows(rows)
			},
			expectErr: false,
			validateFn: func(t *testing.T, inv *models.TeamInvitation) {
				assert.Equal(t, "inv-123", inv.ID)
				assert.Equal(t, "team-123", inv.TeamID)
				assert.Equal(t, "user-123", inv.InviterID)
				assert.Equal(t, "invite@example.com", inv.InviteeEmail)
				assert.Equal(t, models.TeamMemberRoleMember, inv.Role)
				assert.Equal(t, models.InvitationStatusPending, inv.Status)
			},
		},
		{
			name:         "not found",
			invitationID: "inv-notfound",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM team_invitations WHERE id`).
					WithArgs("inv-notfound").
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
		},
		{
			name:         "database error",
			invitationID: "inv-error",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM team_invitations WHERE id`).
					WithArgs("inv-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.GetByID(ctx, tt.invitationID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.validateFn != nil {
					tt.validateFn(t, result)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamInvitationRepository_GetByToken(t *testing.T) {
	repo, mock, mockDB := setupTeamInvitationTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)

	tests := []struct {
		name       string
		token      string
		setupMock  func()
		expectErr  bool
		validateFn func(*testing.T, *models.TeamInvitation)
	}{
		{
			name:  "successful retrieval",
			token: "token-abc123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "team_id", "inviter_id", "invitee_email", "role", "token",
					"status", "expires_at", "created_at", "updated_at",
				}).AddRow(
					"inv-123", "team-123", "user-123", "invite@example.com",
					models.TeamMemberRoleMember, "token-abc123",
					models.InvitationStatusPending, expiresAt, now, now,
				)

				mock.ExpectQuery(`SELECT .+ FROM team_invitations WHERE token`).
					WithArgs("token-abc123").
					WillReturnRows(rows)
			},
			expectErr: false,
			validateFn: func(t *testing.T, inv *models.TeamInvitation) {
				assert.Equal(t, "inv-123", inv.ID)
				assert.Equal(t, "token-abc123", inv.Token)
			},
		},
		{
			name:  "not found",
			token: "token-notfound",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM team_invitations WHERE token`).
					WithArgs("token-notfound").
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
		},
		{
			name:  "database error",
			token: "token-error",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM team_invitations WHERE token`).
					WithArgs("token-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.GetByToken(ctx, tt.token)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.validateFn != nil {
					tt.validateFn(t, result)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamInvitationRepository_GetByTeamID(t *testing.T) {
	repo, mock, mockDB := setupTeamInvitationTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)

	tests := []struct {
		name      string
		teamID    string
		setupMock func()
		expectErr bool
		expectLen int
	}{
		{
			name:   "successful retrieval with multiple invitations",
			teamID: "team-123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "team_id", "inviter_id", "invitee_email", "role", "token",
					"status", "expires_at", "created_at", "updated_at",
				}).AddRow(
					"inv-1", "team-123", "user-123", "invite1@example.com",
					models.TeamMemberRoleMember, "token-1",
					models.InvitationStatusPending, expiresAt, now, now,
				).AddRow(
					"inv-2", "team-123", "user-123", "invite2@example.com",
					models.TeamMemberRoleAdmin, "token-2",
					models.InvitationStatusPending, expiresAt, now, now,
				)

				mock.ExpectQuery(`SELECT .+ FROM team_invitations WHERE team_id`).
					WithArgs("team-123").
					WillReturnRows(rows)
			},
			expectErr: false,
			expectLen: 2,
		},
		{
			name:   "successful retrieval with no invitations",
			teamID: "team-empty",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "team_id", "inviter_id", "invitee_email", "role", "token",
					"status", "expires_at", "created_at", "updated_at",
				})

				mock.ExpectQuery(`SELECT .+ FROM team_invitations WHERE team_id`).
					WithArgs("team-empty").
					WillReturnRows(rows)
			},
			expectErr: false,
			expectLen: 0,
		},
		{
			name:   "database error",
			teamID: "team-error",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM team_invitations WHERE team_id`).
					WithArgs("team-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
			expectLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.GetByTeamID(ctx, tt.teamID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tt.expectLen)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamInvitationRepository_GetPendingByEmail(t *testing.T) {
	repo, mock, mockDB := setupTeamInvitationTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)

	tests := []struct {
		name      string
		email     string
		setupMock func()
		expectErr bool
		expectLen int
	}{
		{
			name:  "successful retrieval with pending invitations",
			email: "invite@example.com",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "team_id", "inviter_id", "invitee_email", "role", "token",
					"status", "expires_at", "created_at", "updated_at",
				}).AddRow(
					"inv-1", "team-1", "user-1", "invite@example.com",
					models.TeamMemberRoleMember, "token-1",
					models.InvitationStatusPending, expiresAt, now, now,
				).AddRow(
					"inv-2", "team-2", "user-2", "invite@example.com",
					models.TeamMemberRoleAdmin, "token-2",
					models.InvitationStatusPending, expiresAt, now, now,
				)

				mock.ExpectQuery(`SELECT .+ FROM team_invitations WHERE invitee_email`).
					WithArgs("invite@example.com", models.InvitationStatusPending).
					WillReturnRows(rows)
			},
			expectErr: false,
			expectLen: 2,
		},
		{
			name:  "no pending invitations",
			email: "noinvites@example.com",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "team_id", "inviter_id", "invitee_email", "role", "token",
					"status", "expires_at", "created_at", "updated_at",
				})

				mock.ExpectQuery(`SELECT .+ FROM team_invitations WHERE invitee_email`).
					WithArgs("noinvites@example.com", models.InvitationStatusPending).
					WillReturnRows(rows)
			},
			expectErr: false,
			expectLen: 0,
		},
		{
			name:  "database error",
			email: "error@example.com",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM team_invitations WHERE invitee_email`).
					WithArgs("error@example.com", models.InvitationStatusPending).
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
			expectLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.GetPendingByEmail(ctx, tt.email)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tt.expectLen)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamInvitationRepository_UpdateStatus(t *testing.T) {
	repo, mock, mockDB := setupTeamInvitationTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	tests := []struct {
		name         string
		invitationID string
		status       models.InvitationStatus
		setupMock    func()
		expectErr    bool
	}{
		{
			name:         "successful update to accepted",
			invitationID: "inv-123",
			status:       models.InvitationStatusAccepted,
			setupMock: func() {
				mock.ExpectExec(`UPDATE team_invitations SET status`).
					WithArgs(models.InvitationStatusAccepted, "inv-123").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:         "successful update to rejected",
			invitationID: "inv-456",
			status:       models.InvitationStatusRejected,
			setupMock: func() {
				mock.ExpectExec(`UPDATE team_invitations SET status`).
					WithArgs(models.InvitationStatusRejected, "inv-456").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:         "successful update to revoked",
			invitationID: "inv-789",
			status:       models.InvitationStatusRevoked,
			setupMock: func() {
				mock.ExpectExec(`UPDATE team_invitations SET status`).
					WithArgs(models.InvitationStatusRevoked, "inv-789").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:         "not found",
			invitationID: "inv-notfound",
			status:       models.InvitationStatusAccepted,
			setupMock: func() {
				mock.ExpectExec(`UPDATE team_invitations SET status`).
					WithArgs(models.InvitationStatusAccepted, "inv-notfound").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			expectErr: true,
		},
		{
			name:         "database error",
			invitationID: "inv-error",
			status:       models.InvitationStatusAccepted,
			setupMock: func() {
				mock.ExpectExec(`UPDATE team_invitations SET status`).
					WithArgs(models.InvitationStatusAccepted, "inv-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.UpdateStatus(ctx, tt.invitationID, tt.status)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamInvitationRepository_Delete(t *testing.T) {
	repo, mock, mockDB := setupTeamInvitationTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	tests := []struct {
		name         string
		invitationID string
		setupMock    func()
		expectErr    bool
	}{
		{
			name:         "successful delete",
			invitationID: "inv-123",
			setupMock: func() {
				mock.ExpectExec(`DELETE FROM team_invitations WHERE id`).
					WithArgs("inv-123").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:         "not found",
			invitationID: "inv-notfound",
			setupMock: func() {
				mock.ExpectExec(`DELETE FROM team_invitations WHERE id`).
					WithArgs("inv-notfound").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			expectErr: true,
		},
		{
			name:         "database error",
			invitationID: "inv-error",
			setupMock: func() {
				mock.ExpectExec(`DELETE FROM team_invitations WHERE id`).
					WithArgs("inv-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.Delete(ctx, tt.invitationID)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestTeamInvitationRepository_GetByTeamID_ScanError tests scan error handling
func TestTeamInvitationRepository_GetByTeamID_ScanError(t *testing.T) {
	repo, mock, mockDB := setupTeamInvitationTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	// Return rows with invalid data type to trigger scan error
	rows := sqlmock.NewRows([]string{
		"id", "team_id", "inviter_id", "invitee_email", "role", "token",
		"status", "expires_at", "created_at", "updated_at",
	}).AddRow(
		"inv-1", "team-123", "user-123", "invite@example.com",
		models.TeamMemberRoleMember, "token-1",
		models.InvitationStatusPending, "invalid-time", time.Now(), time.Now(), // invalid time
	)

	mock.ExpectQuery(`SELECT .+ FROM team_invitations WHERE team_id`).
		WithArgs("team-123").
		WillReturnRows(rows)

	result, err := repo.GetByTeamID(ctx, "team-123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTeamInvitationRepository_UpdateStatus_RowsAffectedError tests rows affected error
func TestTeamInvitationRepository_UpdateStatus_RowsAffectedError(t *testing.T) {
	repo, mock, mockDB := setupTeamInvitationTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	mock.ExpectExec(`UPDATE team_invitations SET status`).
		WithArgs(models.InvitationStatusAccepted, "inv-123").
		WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

	err := repo.UpdateStatus(ctx, "inv-123", models.InvitationStatusAccepted)

	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTeamInvitationRepository_Delete_RowsAffectedError tests rows affected error
func TestTeamInvitationRepository_Delete_RowsAffectedError(t *testing.T) {
	repo, mock, mockDB := setupTeamInvitationTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	mock.ExpectExec(`DELETE FROM team_invitations WHERE id`).
		WithArgs("inv-123").
		WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

	err := repo.Delete(ctx, "inv-123")

	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTeamInvitationRepository_GetPendingByEmail_RowsIterationError tests row iteration error
func TestTeamInvitationRepository_GetPendingByEmail_RowsIterationError(t *testing.T) {
	repo, mock, mockDB := setupTeamInvitationTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)

	// Create rows that will return an error on iteration
	rows := sqlmock.NewRows([]string{
		"id", "team_id", "inviter_id", "invitee_email", "role", "token",
		"status", "expires_at", "created_at", "updated_at",
	}).AddRow(
		"inv-1", "team-1", "user-1", "invite@example.com",
		models.TeamMemberRoleMember, "token-1",
		models.InvitationStatusPending, expiresAt, now, now,
	).RowError(0, driver.ErrBadConn)

	mock.ExpectQuery(`SELECT .+ FROM team_invitations WHERE invitee_email`).
		WithArgs("invite@example.com", models.InvitationStatusPending).
		WillReturnRows(rows)

	result, err := repo.GetPendingByEmail(ctx, "invite@example.com")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}
