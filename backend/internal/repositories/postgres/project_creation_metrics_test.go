package postgres_test

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
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/postgres"
)

const testCreationProjectID = "11111111-1111-1111-1111-111111111111"

// TestProjectRepository_GetProjectResourceCreationMetrics_Success verifies the
// method resolves+authorizes the project, then returns the sparse per-day
// per-type counts from the aggregate query verbatim (zero-fill is the handler's
// job, so the repository returns only the days that have creations).
func TestProjectRepository_GetProjectResourceCreationMetrics_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()
	since := time.Now().UTC().AddDate(0, 0, -7)

	// 1. Project resolution + authorization (slug, teamID, userID).
	mock.ExpectQuery(`SELECT p\.id`).
		WithArgs("my-project", "team-123", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(testCreationProjectID))

	// 2. Aggregate creation counts (projectID, since), grouped by date+type.
	mock.ExpectQuery(`UNION ALL`).
		WithArgs(testCreationProjectID, since).
		WillReturnRows(sqlmock.NewRows([]string{"date", "resource_type", "count"}).
			AddRow("2026-05-28", "artifacts", 1).
			AddRow("2026-05-28", "prompts", 3).
			AddRow("2026-05-29", "memories", 2))

	got, err := repo.GetProjectResourceCreationMetrics(ctx, "team-123", "user-1", "my-project", since)

	require.NoError(t, err)
	assert.Equal(t, []models.ProjectResourceCreationCount{
		{Date: "2026-05-28", ResourceType: "artifacts", Count: 1},
		{Date: "2026-05-28", ResourceType: "prompts", Count: 3},
		{Date: "2026-05-29", ResourceType: "memories", Count: 2},
	}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_GetProjectResourceCreationMetrics_Empty verifies a
// project with no creations in the window returns an empty (non-nil) slice and
// never runs the aggregate against a missing project.
func TestProjectRepository_GetProjectResourceCreationMetrics_Empty(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()
	since := time.Now().UTC().AddDate(0, 0, -30)

	mock.ExpectQuery(`SELECT p\.id`).
		WithArgs("my-project", "team-123", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(testCreationProjectID))
	mock.ExpectQuery(`UNION ALL`).
		WithArgs(testCreationProjectID, since).
		WillReturnRows(sqlmock.NewRows([]string{"date", "resource_type", "count"}))

	got, err := repo.GetProjectResourceCreationMetrics(ctx, "team-123", "user-1", "my-project", since)

	require.NoError(t, err)
	assert.Equal(t, []models.ProjectResourceCreationCount{}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestProjectRepository_GetProjectResourceCreationMetrics_NotFound verifies an
// unknown/inaccessible project (the auth CTE resolves no row) maps to
// ErrProjectNotFoundForRepo and the aggregate query is never run.
func TestProjectRepository_GetProjectResourceCreationMetrics_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()
	since := time.Now().UTC().AddDate(0, 0, -7)

	mock.ExpectQuery(`SELECT p\.id`).
		WithArgs("missing", "team-123", "user-1").
		WillReturnError(sql.ErrNoRows)

	got, err := repo.GetProjectResourceCreationMetrics(ctx, "team-123", "user-1", "missing", since)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, repositories.ErrProjectNotFoundForRepo)
	assert.NoError(t, mock.ExpectationsWereMet())
}
