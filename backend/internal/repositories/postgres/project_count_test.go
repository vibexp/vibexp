package postgres_test

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
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/postgres"
)

// newProjectRepoForTest builds a sqlmock-backed project repository.
func newProjectRepoForTest(t *testing.T) (repositories.ProjectRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return postgres.NewProjectRepository(&database.DB{DB: db}), mock, func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}
}

// TestProjectRepository_CountByTeamID_Success verifies counting projects for a team
func TestProjectRepository_CountByTeamID_Success(t *testing.T) {
	tests := []struct {
		name           string
		teamID         string
		expectedCount  int
		mockReturnRows *sqlmock.Rows
	}{
		{
			name:           "team with no projects",
			teamID:         "team-empty",
			expectedCount:  0,
			mockReturnRows: sqlmock.NewRows([]string{"count"}).AddRow(0),
		},
		{
			name:           "team with one project",
			teamID:         "team-one",
			expectedCount:  1,
			mockReturnRows: sqlmock.NewRows([]string{"count"}).AddRow(1),
		},
		{
			name:           "team with multiple projects",
			teamID:         "team-many",
			expectedCount:  5,
			mockReturnRows: sqlmock.NewRows([]string{"count"}).AddRow(5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() {
				if closeErr := db.Close(); closeErr != nil {
					t.Logf("Failed to close database: %v", closeErr)
				}
			}()

			repo := postgres.NewProjectRepository(&database.DB{DB: db})
			ctx := context.Background()

			// Mock the count query
			countQuery := `SELECT COUNT\(\*\) FROM projects WHERE team_id = \$1`
			mock.ExpectQuery(countQuery).
				WithArgs(tt.teamID).
				WillReturnRows(tt.mockReturnRows)

			// Execute count
			count, err := repo.CountByTeamID(ctx, tt.teamID)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedCount, count)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestProjectRepository_CountByTeamID_DatabaseError verifies error handling
func TestProjectRepository_CountByTeamID_DatabaseError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()
	teamID := "team-error"

	// Mock database error
	countQuery := `SELECT COUNT\(\*\) FROM projects WHERE team_id = \$1`
	mock.ExpectQuery(countQuery).
		WithArgs(teamID).
		WillReturnError(sql.ErrConnDone)

	// Execute count
	count, err := repo.CountByTeamID(ctx, teamID)

	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "failed to count projects")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ListByTeamID scans eleven columns positionally, and `name`/`slug` are
// adjacent and same-typed — a transposition would compile, pass every other
// test, and surface only as wrong labels and broken deep-links in the
// by-project freshness chart. This pins the projection against the scan order.
func TestProjectRepository_ListByTeamID(t *testing.T) {
	repo, mock, closeDB := newProjectRepoForTest(t)
	defer closeDB()

	now := time.Now().UTC()
	mock.ExpectQuery(`SELECT id, user_id, team_id, name, slug, description, git_url, homepage,\s+` +
		`created_at, updated_at, version\s+FROM projects\s+WHERE team_id = \$1\s+ORDER BY name, id`).
		WithArgs("team-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "team_id", "name", "slug", "description",
			"git_url", "homepage", "created_at", "updated_at", "version",
		}).AddRow(
			"project-1", "user-1", "team-1", "Platform", "platform", "The platform",
			"git@example.com:acme/platform.git", "https://example.com", now, now, 3,
		))

	projects, err := repo.ListByTeamID(context.Background(), "team-1")

	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, "project-1", projects[0].ID)
	assert.Equal(t, "user-1", projects[0].UserID)
	assert.Equal(t, "team-1", projects[0].TeamID)
	assert.Equal(t, "Platform", projects[0].Name)
	assert.Equal(t, "platform", projects[0].Slug, "name and slug must not be transposed")
	assert.Equal(t, "The platform", projects[0].Description)
	assert.Equal(t, "git@example.com:acme/platform.git", projects[0].GitURL)
	assert.Equal(t, "https://example.com", projects[0].Homepage)
	assert.Equal(t, int64(3), projects[0].Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A team with no projects gets an empty slice, not nil: the by-project metric
// ranges over it.
func TestProjectRepository_ListByTeamID_EmptyIsNotNil(t *testing.T) {
	repo, mock, closeDB := newProjectRepoForTest(t)
	defer closeDB()

	mock.ExpectQuery(`FROM projects`).WillReturnRows(sqlmock.NewRows([]string{
		"id", "user_id", "team_id", "name", "slug", "description",
		"git_url", "homepage", "created_at", "updated_at", "version",
	}))

	projects, err := repo.ListByTeamID(context.Background(), "team-1")

	require.NoError(t, err)
	assert.Equal(t, []models.Project{}, projects)
}

func TestProjectRepository_ListByTeamID_Errors(t *testing.T) {
	tests := []struct {
		name    string
		arrange func(mock sqlmock.Sqlmock)
		wantIn  string
	}{
		{
			name: "query fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM projects`).WillReturnError(errors.New("boom"))
			},
			wantIn: "failed to list projects by team",
		},
		{
			name: "scan fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM projects`).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("project-1"))
			},
			wantIn: "failed to scan project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, closeDB := newProjectRepoForTest(t)
			defer closeDB()
			tt.arrange(mock)

			_, err := repo.ListByTeamID(context.Background(), "team-1")

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantIn)
		})
	}
}
