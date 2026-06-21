package postgres

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

func setupTeamMemberTestDB(t *testing.T) (*database.DB, sqlmock.Sqlmock) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	return &database.DB{DB: mockDB}, mock
}

func TestTeamMemberRepository_Create(t *testing.T) {
	db, mock := setupTeamMemberTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewTeamMemberRepository(db)
	now := time.Now()

	member := &models.TeamMember{
		TeamID:    "team-123",
		UserID:    "user-456",
		Role:      models.TeamMemberRoleOwner,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO team_members (team_id, user_id, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`)).WithArgs(member.TeamID, member.UserID, member.Role, member.CreatedAt, member.UpdatedAt).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow("member-789", now, now))

	err := repo.Create(context.Background(), member)
	assert.NoError(t, err)
	assert.Equal(t, "member-789", member.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamMemberRepository_Create_Error(t *testing.T) {
	db, mock := setupTeamMemberTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewTeamMemberRepository(db)
	now := time.Now()

	member := &models.TeamMember{
		TeamID:    "team-123",
		UserID:    "user-456",
		Role:      models.TeamMemberRoleOwner,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO team_members (team_id, user_id, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`)).WithArgs(member.TeamID, member.UserID, member.Role, member.CreatedAt, member.UpdatedAt).
		WillReturnError(sql.ErrConnDone)

	err := repo.Create(context.Background(), member)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create team member")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamMemberRepository_GetByTeamAndUser(t *testing.T) {
	db, mock := setupTeamMemberTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewTeamMemberRepository(db)
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, team_id, user_id, role, created_at, updated_at
		FROM team_members WHERE team_id = $1 AND user_id = $2
	`)).WithArgs("team-123", "user-456").
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_id", "user_id", "role", "created_at", "updated_at"}).
			AddRow("member-789", "team-123", "user-456", "owner", now, now))

	member, err := repo.GetByTeamAndUser(context.Background(), "team-123", "user-456")
	assert.NoError(t, err)
	assert.NotNil(t, member)
	assert.Equal(t, "member-789", member.ID)
	assert.Equal(t, "team-123", member.TeamID)
	assert.Equal(t, "user-456", member.UserID)
	assert.Equal(t, models.TeamMemberRoleOwner, member.Role)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamMemberRepository_GetByTeamAndUser_NotFound(t *testing.T) {
	db, mock := setupTeamMemberTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewTeamMemberRepository(db)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, team_id, user_id, role, created_at, updated_at
		FROM team_members WHERE team_id = $1 AND user_id = $2
	`)).WithArgs("team-123", "user-456").
		WillReturnError(sql.ErrNoRows)

	member, err := repo.GetByTeamAndUser(context.Background(), "team-123", "user-456")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "team member not found")
	assert.Nil(t, member)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamMemberRepository_GetByTeamID(t *testing.T) {
	db, mock := setupTeamMemberTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewTeamMemberRepository(db)
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, team_id, user_id, role, created_at, updated_at
		FROM team_members WHERE team_id = $1
		ORDER BY created_at ASC
	`)).WithArgs("team-123").
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_id", "user_id", "role", "created_at", "updated_at"}).
			AddRow("member-1", "team-123", "user-1", "owner", now, now).
			AddRow("member-2", "team-123", "user-2", "admin", now, now).
			AddRow("member-3", "team-123", "user-3", "member", now, now))

	members, err := repo.GetByTeamID(context.Background(), "team-123")
	assert.NoError(t, err)
	assert.Len(t, members, 3)
	assert.Equal(t, "member-1", members[0].ID)
	assert.Equal(t, models.TeamMemberRoleOwner, members[0].Role)
	assert.Equal(t, "member-2", members[1].ID)
	assert.Equal(t, models.TeamMemberRoleAdmin, members[1].Role)
	assert.Equal(t, "member-3", members[2].ID)
	assert.Equal(t, models.TeamMemberRoleMember, members[2].Role)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamMemberRepository_GetByTeamID_Empty(t *testing.T) {
	db, mock := setupTeamMemberTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewTeamMemberRepository(db)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, team_id, user_id, role, created_at, updated_at
		FROM team_members WHERE team_id = $1
		ORDER BY created_at ASC
	`)).WithArgs("team-123").
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_id", "user_id", "role", "created_at", "updated_at"}))

	members, err := repo.GetByTeamID(context.Background(), "team-123")
	assert.NoError(t, err)
	assert.Empty(t, members)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamMemberRepository_GetByUserID(t *testing.T) {
	db, mock := setupTeamMemberTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewTeamMemberRepository(db)
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, team_id, user_id, role, created_at, updated_at
		FROM team_members WHERE user_id = $1
		ORDER BY created_at ASC
	`)).WithArgs("user-456").
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_id", "user_id", "role", "created_at", "updated_at"}).
			AddRow("member-1", "team-1", "user-456", "owner", now, now).
			AddRow("member-2", "team-2", "user-456", "member", now, now))

	members, err := repo.GetByUserID(context.Background(), "user-456")
	assert.NoError(t, err)
	assert.Len(t, members, 2)
	assert.Equal(t, "team-1", members[0].TeamID)
	assert.Equal(t, models.TeamMemberRoleOwner, members[0].Role)
	assert.Equal(t, "team-2", members[1].TeamID)
	assert.Equal(t, models.TeamMemberRoleMember, members[1].Role)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamMemberRepository_UpdateRole(t *testing.T) {
	db, mock := setupTeamMemberTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewTeamMemberRepository(db)

	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE team_members
		SET role = $1, updated_at = $2
		WHERE team_id = $3 AND user_id = $4
	`)).WithArgs(models.TeamMemberRoleAdmin, sqlmock.AnyArg(), "team-123", "user-456").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.UpdateRole(context.Background(), "team-123", "user-456", models.TeamMemberRoleAdmin)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamMemberRepository_UpdateRole_NotFound(t *testing.T) {
	db, mock := setupTeamMemberTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewTeamMemberRepository(db)

	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE team_members
		SET role = $1, updated_at = $2
		WHERE team_id = $3 AND user_id = $4
	`)).WithArgs(models.TeamMemberRoleAdmin, sqlmock.AnyArg(), "team-123", "user-456").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.UpdateRole(context.Background(), "team-123", "user-456", models.TeamMemberRoleAdmin)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "team member not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamMemberRepository_Delete(t *testing.T) {
	db, mock := setupTeamMemberTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewTeamMemberRepository(db)

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM team_members WHERE team_id = $1 AND user_id = $2`)).
		WithArgs("team-123", "user-456").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Delete(context.Background(), "team-123", "user-456")
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamMemberRepository_Delete_NotFound(t *testing.T) {
	db, mock := setupTeamMemberTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewTeamMemberRepository(db)

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM team_members WHERE team_id = $1 AND user_id = $2`)).
		WithArgs("team-123", "user-456").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.Delete(context.Background(), "team-123", "user-456")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "team member not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamMemberRole_IsValid(t *testing.T) {
	tests := []struct {
		role  models.TeamMemberRole
		valid bool
	}{
		{models.TeamMemberRoleOwner, true},
		{models.TeamMemberRoleAdmin, true},
		{models.TeamMemberRoleMember, true},
		{models.TeamMemberRole("invalid"), false},
		{models.TeamMemberRole(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.role.IsValid())
		})
	}
}

func TestTeamMemberRole_String(t *testing.T) {
	tests := []struct {
		role     models.TeamMemberRole
		expected string
	}{
		{models.TeamMemberRoleOwner, "owner"},
		{models.TeamMemberRoleAdmin, "admin"},
		{models.TeamMemberRoleMember, "member"},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.role.String())
		})
	}
}
