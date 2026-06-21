package postgres_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories/postgres"
)

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
