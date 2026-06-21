package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

func setupTeamTest(t *testing.T) (*TeamRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := NewTeamRepository(db).(*TeamRepository)

	return repo, mock, mockDB
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamRepository_Create(t *testing.T) {
	repo, mock, mockDB := setupTeamTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name      string
		team      *models.Team
		setupMock func()
		expectErr bool
	}{
		{
			name: "successful create",
			team: &models.Team{
				OwnerID:     "user-123",
				Name:        "Private Workspace",
				Slug:        "private-workspace",
				Description: "Your personal workspace for individual projects and resources",
				IsPersonal:  true,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"id", "is_personal", "created_at", "updated_at"}).
					AddRow("team-123", true, now, now)

				mock.ExpectQuery(`INSERT INTO teams`).
					WithArgs(
						"user-123",
						"Private Workspace",
						"private-workspace",
						"Your personal workspace for individual projects and resources",
						true, // is_personal
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
					).
					WillReturnRows(rows)
			},
			expectErr: false,
		},
		{
			name: "database error",
			team: &models.Team{
				OwnerID:     "user-error",
				Name:        "Error Team",
				Slug:        "error-team",
				Description: "Team that errors",
				IsPersonal:  false,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			setupMock: func() {
				mock.ExpectQuery(`INSERT INTO teams`).
					WithArgs("user-error", "Error Team", "error-team", "Team that errors", false, sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.Create(ctx, tt.team)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, tt.team.ID)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamRepository_GetByID(t *testing.T) {
	repo, mock, mockDB := setupTeamTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name       string
		teamID     string
		setupMock  func()
		expectErr  bool
		expectNil  bool
		validateFn func(*testing.T, *models.Team)
	}{
		{
			name:   "successful retrieval",
			teamID: "team-123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "owner_id", "name", "slug", "description", "is_personal", "created_at", "updated_at",
				}).AddRow(
					"team-123",
					"user-123",
					"Private Workspace",
					"private-workspace",
					"Your personal workspace for individual projects and resources",
					true,
					now,
					now,
				)

				mock.ExpectQuery(`SELECT id, owner_id, name, slug, description, is_personal, created_at, updated_at`).
					WithArgs("team-123").
					WillReturnRows(rows)
			},
			expectErr: false,
			expectNil: false,
			validateFn: func(t *testing.T, team *models.Team) {
				assert.Equal(t, "team-123", team.ID)
				assert.Equal(t, "user-123", team.OwnerID)
				assert.Equal(t, "Private Workspace", team.Name)
				assert.Equal(t, "private-workspace", team.Slug)
				assert.Equal(t, "Your personal workspace for individual projects and resources", team.Description)
				assert.True(t, team.IsPersonal)
			},
		},
		{
			name:   "not found returns error",
			teamID: "team-notfound",
			setupMock: func() {
				mock.ExpectQuery(`SELECT id, owner_id, name, slug, description, is_personal, created_at, updated_at`).
					WithArgs("team-notfound").
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
			expectNil: true,
		},
		{
			name:   "database error",
			teamID: "team-error",
			setupMock: func() {
				mock.ExpectQuery(`SELECT id, owner_id, name, slug, description, is_personal, created_at, updated_at`).
					WithArgs("team-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.GetByID(ctx, tt.teamID)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectNil {
				assert.Nil(t, result)
			} else if tt.validateFn != nil {
				tt.validateFn(t, result)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamRepository_GetByOwnerID(t *testing.T) {
	repo, mock, mockDB := setupTeamTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name       string
		ownerID    string
		setupMock  func()
		expectErr  bool
		expectNil  bool
		validateFn func(*testing.T, *models.Team)
	}{
		{
			name:    "successful retrieval",
			ownerID: "user-123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "owner_id", "name", "slug", "description", "is_personal", "created_at", "updated_at",
				}).AddRow(
					"team-123",
					"user-123",
					"Private Workspace",
					"private-workspace",
					"Your personal workspace for individual projects and resources",
					true,
					now,
					now,
				)

				mock.ExpectQuery(`SELECT id, owner_id, name, slug, description, is_personal, created_at, updated_at`).
					WithArgs("user-123").
					WillReturnRows(rows)
			},
			expectErr: false,
			expectNil: false,
			validateFn: func(t *testing.T, team *models.Team) {
				assert.Equal(t, "team-123", team.ID)
				assert.Equal(t, "user-123", team.OwnerID)
				assert.Equal(t, "Private Workspace", team.Name)
			},
		},
		{
			name:    "not found returns error",
			ownerID: "user-notfound",
			setupMock: func() {
				mock.ExpectQuery(`SELECT id, owner_id, name, slug, description, is_personal, created_at, updated_at`).
					WithArgs("user-notfound").
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
			expectNil: true,
		},
		{
			name:    "database error",
			ownerID: "user-error",
			setupMock: func() {
				mock.ExpectQuery(`SELECT id, owner_id, name, slug, description, is_personal, created_at, updated_at`).
					WithArgs("user-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.GetByOwnerID(ctx, tt.ownerID)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectNil {
				assert.Nil(t, result)
			} else if tt.validateFn != nil {
				tt.validateFn(t, result)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamRepository_GetByOwnerAndSlug(t *testing.T) {
	repo, mock, mockDB := setupTeamTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name       string
		ownerID    string
		slug       string
		setupMock  func()
		expectErr  bool
		expectNil  bool
		validateFn func(*testing.T, *models.Team)
	}{
		{
			name:    "successful retrieval",
			ownerID: "user-123",
			slug:    "private-workspace",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "owner_id", "name", "slug", "description", "is_personal", "created_at", "updated_at",
				}).AddRow(
					"team-123",
					"user-123",
					"Private Workspace",
					"private-workspace",
					"Your personal workspace for individual projects and resources",
					true,
					now,
					now,
				)

				mock.ExpectQuery(`SELECT id, owner_id, name, slug, description, is_personal, created_at, updated_at`).
					WithArgs("user-123", "private-workspace").
					WillReturnRows(rows)
			},
			expectErr: false,
			expectNil: false,
			validateFn: func(t *testing.T, team *models.Team) {
				assert.Equal(t, "team-123", team.ID)
				assert.Equal(t, "user-123", team.OwnerID)
				assert.Equal(t, "Private Workspace", team.Name)
				assert.Equal(t, "private-workspace", team.Slug)
			},
		},
		{
			name:    "not found returns error",
			ownerID: "user-123",
			slug:    "nonexistent",
			setupMock: func() {
				mock.ExpectQuery(`SELECT id, owner_id, name, slug, description, is_personal, created_at, updated_at`).
					WithArgs("user-123", "nonexistent").
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
			expectNil: true,
		},
		{
			name:    "database error",
			ownerID: "user-error",
			slug:    "error-slug",
			setupMock: func() {
				mock.ExpectQuery(`SELECT id, owner_id, name, slug, description, is_personal, created_at, updated_at`).
					WithArgs("user-error", "error-slug").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.GetByOwnerAndSlug(ctx, tt.ownerID, tt.slug)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectNil {
				assert.Nil(t, result)
			} else if tt.validateFn != nil {
				tt.validateFn(t, result)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamRepository_CountByOwnerID(t *testing.T) {
	repo, mock, mockDB := setupTeamTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	tests := []struct {
		name        string
		ownerID     string
		setupMock   func()
		expectedCnt int
		expectErr   bool
	}{
		{
			name:    "successful count with teams",
			ownerID: "user-123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"count"}).
					AddRow(3)
				mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM teams WHERE owner_id = \\$1").
					WithArgs("user-123").
					WillReturnRows(rows)
			},
			expectedCnt: 3,
			expectErr:   false,
		},
		{
			name:    "successful count with zero teams",
			ownerID: "user-456",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"count"}).
					AddRow(0)
				mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM teams WHERE owner_id = \\$1").
					WithArgs("user-456").
					WillReturnRows(rows)
			},
			expectedCnt: 0,
			expectErr:   false,
		},
		{
			name:    "database error",
			ownerID: "user-789",
			setupMock: func() {
				mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM teams WHERE owner_id = \\$1").
					WithArgs("user-789").
					WillReturnError(sql.ErrConnDone)
			},
			expectedCnt: 0,
			expectErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			count, err := repo.CountByOwnerID(ctx, tt.ownerID)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedCnt, count)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamRepository_Update(t *testing.T) {
	repo, mock, mockDB := setupTeamTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name      string
		team      *models.Team
		setupMock func()
		expectErr bool
	}{
		{
			name: "successful update",
			team: &models.Team{
				ID:          "team-123",
				OwnerID:     "user-123",
				Name:        "Updated Team",
				Slug:        "updated-team",
				Description: "Updated description",
				UpdatedAt:   now,
			},
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"updated_at"}).AddRow(now)

				mock.ExpectQuery(`UPDATE teams SET`).
					WithArgs(
						"Updated Team",
						"updated-team",
						"Updated description",
						sqlmock.AnyArg(),
						"team-123",
						"user-123",
					).
					WillReturnRows(rows)
			},
			expectErr: false,
		},
		{
			name: "team not found",
			team: &models.Team{
				ID:          "team-notfound",
				OwnerID:     "user-123",
				Name:        "Nonexistent",
				Slug:        "nonexistent",
				Description: "Does not exist",
				UpdatedAt:   now,
			},
			setupMock: func() {
				mock.ExpectQuery(`UPDATE teams SET`).
					WithArgs(
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						"team-notfound",
						"user-123",
					).
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
		},
		{
			name: "database error",
			team: &models.Team{
				ID:          "team-error",
				OwnerID:     "user-123",
				Name:        "Error Team",
				Slug:        "error-team",
				Description: "Will cause error",
				UpdatedAt:   now,
			},
			setupMock: func() {
				mock.ExpectQuery(`UPDATE teams SET`).
					WithArgs(
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						"team-error",
						"user-123",
					).
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.Update(ctx, tt.team)

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
func TestTeamRepository_Delete(t *testing.T) {
	repo, mock, mockDB := setupTeamTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	tests := []struct {
		name      string
		ownerID   string
		teamID    string
		setupMock func()
		expectErr bool
	}{
		{
			name:    "successful delete",
			ownerID: "user-123",
			teamID:  "team-123",
			setupMock: func() {
				mock.ExpectExec(`DELETE FROM teams WHERE id`).
					WithArgs("team-123", "user-123").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:    "team not found",
			ownerID: "user-123",
			teamID:  "team-notfound",
			setupMock: func() {
				mock.ExpectExec(`DELETE FROM teams WHERE id`).
					WithArgs("team-notfound", "user-123").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			expectErr: true,
		},
		{
			name:    "database error",
			ownerID: "user-123",
			teamID:  "team-error",
			setupMock: func() {
				mock.ExpectExec(`DELETE FROM teams WHERE id`).
					WithArgs("team-error", "user-123").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
		{
			name:    "rows affected error",
			ownerID: "user-123",
			teamID:  "team-rowserr",
			setupMock: func() {
				mock.ExpectExec(`DELETE FROM teams WHERE id`).
					WithArgs("team-rowserr", "user-123").
					WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.Delete(ctx, tt.ownerID, tt.teamID)

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
func TestTeamRepository_ListByOwnerID(t *testing.T) {
	repo, mock, mockDB := setupTeamTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name        string
		ownerID     string
		limit       int
		offset      int
		setupMock   func()
		expectErr   bool
		expectCount int
		expectTotal int
	}{
		{
			name:    "successful list with teams",
			ownerID: "user-123",
			limit:   10,
			offset:  0,
			setupMock: func() {
				// Count query
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM teams WHERE owner_id`).
					WithArgs("user-123").
					WillReturnRows(countRows)

				// List query
				rows := sqlmock.NewRows([]string{
					"id", "owner_id", "name", "slug", "description", "is_personal", "created_at", "updated_at",
				}).AddRow(
					"team-1", "user-123", "Team 1", "team-1", "Description 1", false, now, now,
				).AddRow(
					"team-2", "user-123", "Team 2", "team-2", "Description 2", true, now, now,
				)

				mock.ExpectQuery(`SELECT .+ FROM teams WHERE owner_id`).
					WithArgs("user-123", 10, 0).
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCount: 2,
			expectTotal: 2,
		},
		{
			name:    "empty list",
			ownerID: "user-empty",
			limit:   10,
			offset:  0,
			setupMock: func() {
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(0)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM teams WHERE owner_id`).
					WithArgs("user-empty").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows([]string{
					"id", "owner_id", "name", "slug", "description", "is_personal", "created_at", "updated_at",
				})

				mock.ExpectQuery(`SELECT .+ FROM teams WHERE owner_id`).
					WithArgs("user-empty", 10, 0).
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCount: 0,
			expectTotal: 0,
		},
		{
			name:    "count query error",
			ownerID: "user-error",
			limit:   10,
			offset:  0,
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM teams WHERE owner_id`).
					WithArgs("user-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr:   true,
			expectCount: 0,
			expectTotal: 0,
		},
		{
			name:    "list query error",
			ownerID: "user-listerror",
			limit:   10,
			offset:  0,
			setupMock: func() {
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM teams WHERE owner_id`).
					WithArgs("user-listerror").
					WillReturnRows(countRows)

				mock.ExpectQuery(`SELECT .+ FROM teams WHERE owner_id`).
					WithArgs("user-listerror", 10, 0).
					WillReturnError(sql.ErrConnDone)
			},
			expectErr:   true,
			expectCount: 0,
			expectTotal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			teams, total, err := repo.ListByOwnerID(ctx, tt.ownerID, tt.limit, tt.offset)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, teams, tt.expectCount)
				assert.Equal(t, tt.expectTotal, total)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamRepository_ListByUserID(t *testing.T) {
	repo, mock, mockDB := setupTeamTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name        string
		userID      string
		limit       int
		offset      int
		setupMock   func()
		expectErr   bool
		expectCount int
		expectTotal int
	}{
		{
			name:   "successful list - includes owned and member teams",
			userID: "user-123",
			limit:  10,
			offset: 0,
			setupMock: func() {
				// Count query - uses EXISTS subquery, no DISTINCT needed
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(3)
				mock.ExpectQuery(`SELECT COUNT\(\*\)\s+FROM teams t\s+WHERE t.owner_id = \$1\s+OR EXISTS`).
					WithArgs("user-123").
					WillReturnRows(countRows)

				// List query - uses EXISTS subquery, no DISTINCT needed
				rows := sqlmock.NewRows([]string{
					"id", "owner_id", "name", "slug", "description", "is_personal", "created_at", "updated_at",
				}).AddRow(
					"team-1", "user-123", "Owned Team", "owned-team", "Team owned by user", true, now, now,
				).AddRow(
					"team-2", "user-456", "Member Team 1", "member-team-1", "Team where user is member", false, now, now,
				).AddRow(
					"team-3", "user-789", "Member Team 2", "member-team-2", "Another team", false, now, now,
				)

				mock.ExpectQuery(`SELECT t.id, t.owner_id, t.name, t.slug`).
					WithArgs("user-123", 10, 0).
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCount: 3,
			expectTotal: 3,
		},
		{
			name:   "empty list",
			userID: "user-empty",
			limit:  10,
			offset: 0,
			setupMock: func() {
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(0)
				mock.ExpectQuery(`SELECT COUNT\(\*\)\s+FROM teams t\s+WHERE t.owner_id = \$1\s+OR EXISTS`).
					WithArgs("user-empty").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows([]string{
					"id", "owner_id", "name", "slug", "description", "is_personal", "created_at", "updated_at",
				})

				mock.ExpectQuery(`SELECT t.id, t.owner_id, t.name, t.slug`).
					WithArgs("user-empty", 10, 0).
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCount: 0,
			expectTotal: 0,
		},
		{
			name:   "count query error",
			userID: "user-error",
			limit:  10,
			offset: 0,
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\)\s+FROM teams t\s+WHERE t.owner_id = \$1\s+OR EXISTS`).
					WithArgs("user-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr:   true,
			expectCount: 0,
			expectTotal: 0,
		},
		{
			name:   "list query error",
			userID: "user-listerror",
			limit:  10,
			offset: 0,
			setupMock: func() {
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
				mock.ExpectQuery(`SELECT COUNT\(\*\)\s+FROM teams t\s+WHERE t.owner_id = \$1\s+OR EXISTS`).
					WithArgs("user-listerror").
					WillReturnRows(countRows)

				mock.ExpectQuery(`SELECT t.id, t.owner_id, t.name, t.slug`).
					WithArgs("user-listerror", 10, 0).
					WillReturnError(sql.ErrConnDone)
			},
			expectErr:   true,
			expectCount: 0,
			expectTotal: 0,
		},
		{
			name:   "pagination - page 2",
			userID: "user-123",
			limit:  10,
			offset: 10,
			setupMock: func() {
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(15)
				mock.ExpectQuery(`SELECT COUNT\(\*\)\s+FROM teams t\s+WHERE t.owner_id = \$1\s+OR EXISTS`).
					WithArgs("user-123").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows([]string{
					"id", "owner_id", "name", "slug", "description", "is_personal", "created_at", "updated_at",
				}).AddRow(
					"team-11", "user-123", "Team 11", "team-11", "Description", false, now, now,
				)

				mock.ExpectQuery(`SELECT t.id, t.owner_id, t.name, t.slug`).
					WithArgs("user-123", 10, 10).
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCount: 1,
			expectTotal: 15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			teams, total, err := repo.ListByUserID(ctx, tt.userID, tt.limit, tt.offset)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, teams, tt.expectCount)
				assert.Equal(t, tt.expectTotal, total)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
