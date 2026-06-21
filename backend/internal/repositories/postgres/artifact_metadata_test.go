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

// TestArtifactList_InvalidMetadataKey verifies the allowlist rejects malicious keys
// before any query is issued (defense-in-depth alongside parameter binding).
func TestArtifactList_InvalidMetadataKey(t *testing.T) {
	db, _, repo := setupArtifactMockDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("failed to close db: %v", err)
		}
	}()

	testCases := []struct {
		name        string
		metadataKey string
		shouldError bool
	}{
		{"UNION injection attempt", "x') OR '1'='1' --", true},
		{"semicolon drop attempt", "key'; DROP TABLE artifacts; --", true},
		{"special characters", "key@#$", true},
		{"empty key", "", true},
		{"valid key", "environment", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filters := repositories.ArtifactFilters{
				TeamID:   "team-123",
				Metadata: map[string]string{tc.metadataKey: "value"},
			}

			_, _, err := repo.List(context.Background(), "user-123", filters)

			if tc.shouldError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid metadata key")
			} else if err != nil {
				assert.NotContains(t, err.Error(), "invalid metadata key")
			}
		})
	}
}

// TestArtifactList_MetadataKeyBoundAsParameter asserts the generated SQL binds the
// metadata key as a parameter (metadata->>$N = $M) instead of interpolating it,
// so even an allowlist-passing key cannot alter query structure.
func TestArtifactList_MetadataKeyBoundAsParameter(t *testing.T) {
	db, mock, repo := setupArtifactMockDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("failed to close db: %v", err)
		}
	}()

	userID := "user-123"
	teamID := "team-123"
	metadataKey := "environment"
	metadataValue := "production"

	filters := repositories.ArtifactFilters{
		TeamID:   teamID,
		Metadata: map[string]string{metadataKey: metadataValue},
	}

	// squirrel binds the team/user pair individually per EXISTS clause:
	// team_id, then (team, user, team, user), then the implicit "hide archived"
	// default-status bind, then the metadata key/value pair.
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(0)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a.*metadata->>\$\d+ = \$\d+`).
		WithArgs(teamID, teamID, userID, teamID, userID, "archived", metadataKey, metadataValue).
		WillReturnRows(countRows)

	dataRows := sqlmock.NewRows([]string{
		"id", "project_id", "slug", "user_id", "team_id", "title", "description",
		"status", "type", "metadata", "created_at", "updated_at",
	})
	mock.ExpectQuery(`SELECT (.+) FROM artifacts a.*metadata->>\$\d+ = \$\d+`).
		WillReturnRows(dataRows)

	_, _, err := repo.List(context.Background(), userID, filters)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
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
