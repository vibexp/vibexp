package postgres_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/postgres"
)

// A GitHub App config holds a team's GitHub credentials, so an under-scoped
// query here is a cross-tenant credential leak rather than a mere information
// disclosure. These tests pin the team_id predicate onto every method that
// takes one: the foreign-team caller must get the sentinel, never the row —
// and must not be able to tell "exists but not yours" from "does not exist".
func setupGitHubAppConfigIsolationTest(t *testing.T) (repositories.GitHubAppConfigRepository, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	})

	return postgres.NewGitHubAppConfigRepository(&database.DB{DB: db}), mock
}

const (
	isolationOwningTeam  = "team-aaa" // the config lives here
	isolationForeignTeam = "team-bbb" // the caller authenticates here
	isolationConfigID    = "cfg-123"
)

func TestGitHubAppConfigRepository_GetByID_CrossTeamIsolation(t *testing.T) {
	repo, mock := setupGitHubAppConfigIsolationTest(t)

	// The foreign team_id is bound into the WHERE clause, so the real database
	// returns zero rows even though the row exists under isolationOwningTeam.
	mock.ExpectQuery(`SELECT (.+) FROM github_app_configs\s+WHERE id = \$1 AND team_id = \$2`).
		WithArgs(isolationConfigID, isolationForeignTeam).
		WillReturnError(sql.ErrNoRows)

	config, err := repo.GetByID(context.Background(), isolationForeignTeam, isolationConfigID)
	assert.Nil(t, config)
	assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGitHubAppConfigRepository_GetByTeamID_CrossTeamIsolation(t *testing.T) {
	repo, mock := setupGitHubAppConfigIsolationTest(t)

	mock.ExpectQuery(`SELECT (.+) FROM github_app_configs\s+WHERE team_id = \$1`).
		WithArgs(isolationForeignTeam).
		WillReturnError(sql.ErrNoRows)

	config, err := repo.GetByTeamID(context.Background(), isolationForeignTeam)
	assert.Nil(t, config)
	assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGitHubAppConfigRepository_Update_CrossTeamIsolation(t *testing.T) {
	repo, mock := setupGitHubAppConfigIsolationTest(t)

	config := &models.GitHubAppConfig{
		ID:                     isolationConfigID,
		TeamID:                 isolationForeignTeam,
		AppID:                  "999",
		AppSlug:                "hijack",
		ClientID:               "Iv1.evil",
		PrivateKeyEncrypted:    "enc-pk",
		ClientSecretEncrypted:  "enc-cs",
		WebhookSecretEncrypted: "enc-ws",
		WebhookToken:           "tok-evil",
		Version:                1,
	}

	// team_id sits in the UPDATE's WHERE alongside id and version, so the
	// foreign-team write matches nothing and RETURNING yields no row.
	mock.ExpectQuery(`UPDATE github_app_configs`).
		WithArgs(isolationConfigID, isolationForeignTeam, int64(1), "999", "hijack", "Iv1.evil",
			"enc-pk", "enc-cs", "enc-ws", "tok-evil").
		WillReturnError(sql.ErrNoRows)

	err := repo.Update(context.Background(), config)
	assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigVersionConflict)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGitHubAppConfigRepository_Delete_CrossTeamIsolation(t *testing.T) {
	repo, mock := setupGitHubAppConfigIsolationTest(t)

	mock.ExpectExec(`DELETE FROM github_app_configs WHERE id = \$1 AND team_id = \$2`).
		WithArgs(isolationConfigID, isolationForeignTeam).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.Delete(context.Background(), isolationForeignTeam, isolationConfigID)
	assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}
