package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
)

func newAdminRepoMock(t *testing.T) (*AdminRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	repo := &AdminRepository{db: &database.DB{DB: mockDB}}
	return repo, mock, mockDB
}

func TestAdminRepository_GetInstanceCounts(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	repo := NewAdminRepository(&database.DB{DB: mockDB})

	rows := sqlmock.NewRows([]string{"users", "teams", "prompts", "artifacts", "memories"}).
		AddRow(42, 12, 340, 128, 512)
	mock.ExpectQuery(`SELECT`).WillReturnRows(rows)

	counts, err := repo.GetInstanceCounts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(42), counts.Users)
	assert.Equal(t, int64(12), counts.Teams)
	assert.Equal(t, int64(340), counts.Prompts)
	assert.Equal(t, int64(128), counts.Artifacts)
	assert.Equal(t, int64(512), counts.Memories)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_GetInstanceCounts_QueryError(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	repo := NewAdminRepository(&database.DB{DB: mockDB})
	mock.ExpectQuery(`SELECT`).WillReturnError(errors.New("boom"))

	_, err = repo.GetInstanceCounts(context.Background())
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_ListUsers(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`FROM users`).
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "name", "idp_provider", "created_at", "team_count"}).
			AddRow("u1", "a@example.com", "A", "oidc", time.Now(), 2).
			AddRow("u2", "b@example.com", "B", nil, time.Now(), 0))

	users, total, err := repo.ListUsers(context.Background(), 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, users, 2)
	assert.Equal(t, int64(2), users[0].TeamCount)
	require.NotNil(t, users[0].IDPProvider)
	assert.Nil(t, users[1].IDPProvider)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_GetUserDetail_Found(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM users WHERE id = \$1`).
		WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "name", "idp_provider", "created_at"}).
			AddRow("u1", "a@example.com", "A", "oidc", time.Now()))
	mock.ExpectQuery(`FROM team_members tm`).
		WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "role"}).
			AddRow("t1", "Acme", "owner"))

	detail, err := repo.GetUserDetail(context.Background(), "u1")
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, "u1", detail.ID)
	require.Len(t, detail.Memberships, 1)
	assert.Equal(t, "owner", detail.Memberships[0].Role)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_GetUserDetail_NotFound(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM users WHERE id = \$1`).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	detail, err := repo.GetUserDetail(context.Background(), "missing")
	require.NoError(t, err)
	assert.Nil(t, detail)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_ListUsers_CountError(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).WillReturnError(errors.New("boom"))

	_, _, err := repo.ListUsers(context.Background(), 1, 20)
	require.Error(t, err)
}

func TestAdminRepository_ListUsers_QueryError(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`FROM users`).WithArgs(20, 0).WillReturnError(errors.New("boom"))

	_, _, err := repo.ListUsers(context.Background(), 1, 20)
	require.Error(t, err)
}

func TestAdminRepository_GetUserDetail_QueryError(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM users WHERE id = \$1`).WithArgs("u1").WillReturnError(errors.New("boom"))

	_, err := repo.GetUserDetail(context.Background(), "u1")
	require.Error(t, err)
}

func TestAdminRepository_GetUserDetail_MembershipError(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM users WHERE id = \$1`).WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "name", "idp_provider", "created_at"}).
			AddRow("u1", "a@example.com", "A", nil, time.Now()))
	mock.ExpectQuery(`FROM team_members tm`).WithArgs("u1").WillReturnError(errors.New("boom"))

	_, err := repo.GetUserDetail(context.Background(), "u1")
	require.Error(t, err)
}

func TestAdminRepository_ListTeams(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM teams`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`FROM teams t`).
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "created_at", "owner_id", "owner_email", "owner_name", "member_count"}).
			AddRow("t1", "Acme", time.Now(), "o1", "o@example.com", "Owner", 3).
			AddRow("t2", "Beta", time.Now(), "o1", "o@example.com", "Owner", 0))

	teams, total, err := repo.ListTeams(context.Background(), 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, teams, 2)
	assert.Equal(t, "Owner", teams[0].Owner.Name)
	assert.Equal(t, int64(3), teams[0].MemberCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_ListTeams_CountError(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM teams`).WillReturnError(errors.New("boom"))
	_, _, err := repo.ListTeams(context.Background(), 1, 20)
	require.Error(t, err)
}

func TestAdminRepository_ListTeams_QueryError(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM teams`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM teams t`).WithArgs(20, 0).WillReturnError(errors.New("boom"))
	_, _, err := repo.ListTeams(context.Background(), 1, 20)
	require.Error(t, err)
}

func TestAdminRepository_GetTeamDetail_Found(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM teams t JOIN users u ON u.id = t.owner_id WHERE t.id = \$1`).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "created_at", "owner_id", "owner_email", "owner_name"}).
			AddRow("t1", "Acme", time.Now(), "o1", "o@example.com", "Owner"))
	mock.ExpectQuery(`FROM team_members tm`).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "name", "role", "created_at"}).
			AddRow("u1", "m@example.com", "M", "member", time.Now()))

	detail, err := repo.GetTeamDetail(context.Background(), "t1")
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, "Acme", detail.Name)
	assert.Equal(t, "Owner", detail.Owner.Name)
	require.Len(t, detail.Members, 1)
	assert.Equal(t, "member", detail.Members[0].Role)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_GetTeamDetail_NotFound(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM teams t JOIN users u ON u.id = t.owner_id WHERE t.id = \$1`).
		WithArgs("missing").WillReturnError(sql.ErrNoRows)

	detail, err := repo.GetTeamDetail(context.Background(), "missing")
	require.NoError(t, err)
	assert.Nil(t, detail)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_GetTeamDetail_MembersError(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM teams t JOIN users u ON u.id = t.owner_id WHERE t.id = \$1`).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "created_at", "owner_id", "owner_email", "owner_name"}).
			AddRow("t1", "Acme", time.Now(), "o1", "o@example.com", "Owner"))
	mock.ExpectQuery(`FROM team_members tm`).WithArgs("t1").WillReturnError(errors.New("boom"))

	_, err := repo.GetTeamDetail(context.Background(), "t1")
	require.Error(t, err)
}
