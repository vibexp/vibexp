package postgres

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

func setupArtifactMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *ArtifactRepository) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: sqlDB}
	repo := &ArtifactRepository{db: db}
	return sqlDB, mock, repo
}

// TestArtifactListCrossTeam_MembershipGuard asserts the cross-team listing query
// includes a team-membership EXISTS guard so user_id alone does not leak rows.
func TestArtifactListCrossTeam_MembershipGuard(t *testing.T) {
	db, mock, repo := setupArtifactMockDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("failed to close db: %v", err)
		}
	}()

	userID := "user-123"

	// squirrel binds userID individually for each EXISTS clause (3×), then the
	// implicit "hide archived" default-status bind.
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(0)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a.*EXISTS.*team_members`).
		WithArgs(userID, userID, userID, "archived").
		WillReturnRows(countRows)

	dataRows := sqlmock.NewRows([]string{
		"id", "project_id", "slug", "user_id", "team_id", "title", "description",
		"status", "type", "metadata", "created_at", "updated_at",
	})
	mock.ExpectQuery(`SELECT (.+) FROM artifacts a.*EXISTS.*team_members`).
		WillReturnRows(dataRows)

	_, _, err := repo.ListCrossTeam(context.Background(), userID, repositories.ArtifactFilters{})

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
