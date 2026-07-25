package postgres

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

func setupGitHubAppConfigTest(t *testing.T) (*GitHubAppConfigRepository, sqlmock.Sqlmock) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})

	repo := NewGitHubAppConfigRepository(&database.DB{DB: mockDB}).(*GitHubAppConfigRepository)

	return repo, mock
}

// githubAppConfigColumnNames mirrors githubAppConfigColumns, in order.
var githubAppConfigColumnNames = []string{
	"id", "team_id", "user_id", "app_id", "app_slug", "client_id",
	"private_key_encrypted", "client_secret_encrypted", "webhook_secret_encrypted",
	"webhook_token", "created_at", "updated_at", "version",
}

func githubAppConfigRow(now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(githubAppConfigColumnNames).AddRow(
		"cfg-1", "team-1", "user-1", "123456", "my-app", "Iv1.abc",
		"enc-pk", "enc-cs", "enc-ws", "tok-abc", now, now, int64(1),
	)
}

func newGitHubAppConfig() *models.GitHubAppConfig {
	userID := "user-1"
	return &models.GitHubAppConfig{
		TeamID:                 "team-1",
		UserID:                 &userID,
		AppID:                  "123456",
		AppSlug:                "my-app",
		ClientID:               "Iv1.abc",
		PrivateKeyEncrypted:    "enc-pk",
		ClientSecretEncrypted:  "enc-cs",
		WebhookSecretEncrypted: "enc-ws",
		WebhookToken:           "tok-abc",
	}
}

func TestGitHubAppConfigRepository_Create(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()

	t.Run("success", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)
		config := newGitHubAppConfig()

		mock.ExpectQuery(`INSERT INTO github_app_configs`).
			WithArgs("team-1", config.UserID, "123456", "my-app", "Iv1.abc",
				"enc-pk", "enc-cs", "enc-ws", "tok-abc").
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "version"}).
				AddRow("cfg-1", now, now, int64(1)))

		require.NoError(t, repo.Create(ctx, config))
		assert.Equal(t, "cfg-1", config.ID)
		assert.Equal(t, int64(1), config.Version)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	// The two unique constraints must be told apart by CONSTRAINT NAME, because
	// they mean different things to the caller: one team registering a second
	// App vs. two teams registering the same App.
	t.Run("duplicate app id maps to already-registered", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		mock.ExpectQuery(`INSERT INTO github_app_configs`).
			WillReturnError(&pq.Error{Code: "23505", Constraint: "unique_github_app_id"})

		err := repo.Create(ctx, newGitHubAppConfig())
		assert.ErrorIs(t, err, repositories.ErrGitHubAppAlreadyRegistered)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("duplicate team maps to team-taken", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		mock.ExpectQuery(`INSERT INTO github_app_configs`).
			WillReturnError(&pq.Error{Code: "23505", Constraint: "unique_team_github_app"})

		err := repo.Create(ctx, newGitHubAppConfig())
		assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigTeamTaken)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	// A unique violation on some other constraint must NOT be laundered into a
	// 409-shaped sentinel that says something untrue about which App is taken.
	t.Run("unrecognised unique constraint stays a raw error", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		mock.ExpectQuery(`INSERT INTO github_app_configs`).
			WillReturnError(&pq.Error{Code: "23505", Constraint: "idx_github_app_configs_webhook_token"})

		err := repo.Create(ctx, newGitHubAppConfig())
		require.Error(t, err)
		assert.NotErrorIs(t, err, repositories.ErrGitHubAppAlreadyRegistered)
		assert.NotErrorIs(t, err, repositories.ErrGitHubAppConfigTeamTaken)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("non-unique error is wrapped", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		mock.ExpectQuery(`INSERT INTO github_app_configs`).
			WillReturnError(errors.New("boom"))

		err := repo.Create(ctx, newGitHubAppConfig())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create GitHub App config")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGitHubAppConfigRepository_GetByTeamID(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()

	t.Run("found", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		mock.ExpectQuery(`SELECT (.+) FROM github_app_configs\s+WHERE team_id = \$1`).
			WithArgs("team-1").
			WillReturnRows(githubAppConfigRow(now))

		config, err := repo.GetByTeamID(ctx, "team-1")
		require.NoError(t, err)
		assert.Equal(t, "cfg-1", config.ID)
		assert.Equal(t, "123456", config.AppID)
		assert.Equal(t, "enc-pk", config.PrivateKeyEncrypted)
		assert.Equal(t, "tok-abc", config.WebhookToken)
		assert.Equal(t, int64(1), config.Version)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		mock.ExpectQuery(`SELECT (.+) FROM github_app_configs`).
			WithArgs("team-1").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByTeamID(ctx, "team-1")
		assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigNotFound)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGitHubAppConfigRepository_GetByID(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()

	t.Run("found", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		mock.ExpectQuery(`SELECT (.+) FROM github_app_configs\s+WHERE id = \$1 AND team_id = \$2`).
			WithArgs("cfg-1", "team-1").
			WillReturnRows(githubAppConfigRow(now))

		config, err := repo.GetByID(ctx, "team-1", "cfg-1")
		require.NoError(t, err)
		assert.Equal(t, "cfg-1", config.ID)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		mock.ExpectQuery(`SELECT (.+) FROM github_app_configs`).
			WithArgs("cfg-1", "team-1").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByID(ctx, "team-1", "cfg-1")
		assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigNotFound)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGitHubAppConfigRepository_GetByWebhookToken(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()

	t.Run("valid token resolves the config", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		mock.ExpectQuery(`SELECT (.+) FROM github_app_configs\s+WHERE webhook_token = \$1`).
			WithArgs("tok-abc").
			WillReturnRows(githubAppConfigRow(now))

		config, err := repo.GetByWebhookToken(ctx, "tok-abc")
		require.NoError(t, err)
		assert.Equal(t, "cfg-1", config.ID)
		// The whole point of this read: it is what supplies the team context a
		// public webhook request does not carry.
		assert.Equal(t, "team-1", config.TeamID)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("unknown token", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		mock.ExpectQuery(`SELECT (.+) FROM github_app_configs`).
			WithArgs("nope").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByWebhookToken(ctx, "nope")
		assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigNotFound)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGitHubAppConfigRepository_Update(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()

	t.Run("success bumps version", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		config := newGitHubAppConfig()
		config.ID = "cfg-1"
		config.Version = 3

		mock.ExpectQuery(`UPDATE github_app_configs`).
			WithArgs("cfg-1", "team-1", int64(3), "123456", "my-app", "Iv1.abc",
				"enc-pk", "enc-cs", "enc-ws", "tok-abc").
			WillReturnRows(sqlmock.NewRows([]string{"updated_at", "version"}).AddRow(now, int64(4)))

		require.NoError(t, repo.Update(ctx, config))
		assert.Equal(t, int64(4), config.Version)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("stale version returns the conflict sentinel", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		config := newGitHubAppConfig()
		config.ID = "cfg-1"
		config.Version = 1

		mock.ExpectQuery(`UPDATE github_app_configs`).WillReturnError(sql.ErrNoRows)

		err := repo.Update(ctx, config)
		assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigVersionConflict)
		// Nothing was mutated: the caller's version must still be the stale one
		// it passed in, so a retry re-reads rather than silently advancing.
		assert.Equal(t, int64(1), config.Version)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("duplicate app id maps to already-registered", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		config := newGitHubAppConfig()
		config.ID = "cfg-1"

		mock.ExpectQuery(`UPDATE github_app_configs`).
			WillReturnError(&pq.Error{Code: "23505", Constraint: "unique_github_app_id"})

		err := repo.Update(ctx, config)
		assert.ErrorIs(t, err, repositories.ErrGitHubAppAlreadyRegistered)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGitHubAppConfigRepository_Delete(t *testing.T) {
	ctx := contextWithLogger()

	t.Run("success", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		mock.ExpectExec(`DELETE FROM github_app_configs WHERE id = \$1 AND team_id = \$2`).
			WithArgs("cfg-1", "team-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, repo.Delete(ctx, "team-1", "cfg-1"))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no rows affected", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		mock.ExpectExec(`DELETE FROM github_app_configs`).
			WithArgs("cfg-1", "team-1").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Delete(ctx, "team-1", "cfg-1")
		assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigNotFound)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("exec error is wrapped", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		mock.ExpectExec(`DELETE FROM github_app_configs`).
			WithArgs("cfg-1", "team-1").
			WillReturnError(errors.New("boom"))

		err := repo.Delete(ctx, "team-1", "cfg-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete GitHub App config")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rows-affected error is wrapped", func(t *testing.T) {
		repo, mock := setupGitHubAppConfigTest(t)

		mock.ExpectExec(`DELETE FROM github_app_configs`).
			WithArgs("cfg-1", "team-1").
			WillReturnResult(sqlmock.NewErrorResult(errors.New("rows boom")))

		err := repo.Delete(ctx, "team-1", "cfg-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get rows affected")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// The secrets must never reach a response body. GitHubAppConfigResponse is what
// the API returns, so the assertion is on the embedding type as marshaled.
func TestGitHubAppConfig_SecretsAreNotSerialized(t *testing.T) {
	repo, mock := setupGitHubAppConfigTest(t)
	mock.ExpectQuery(`SELECT (.+) FROM github_app_configs`).
		WithArgs("team-1").
		WillReturnRows(githubAppConfigRow(time.Now()))

	config, err := repo.GetByTeamID(contextWithLogger(), "team-1")
	require.NoError(t, err)

	encoded, err := json.Marshal(models.GitHubAppConfigResponse{
		GitHubAppConfig:  *config,
		HasPrivateKey:    true,
		HasClientSecret:  true,
		HasWebhookSecret: true,
		WebhookURL:       "https://vibexp.test/webhooks/github/tok-abc",
	})
	require.NoError(t, err)

	body := string(encoded)
	for _, secret := range []string{"enc-pk", "enc-cs", "enc-ws"} {
		assert.NotContains(t, body, secret, "secret leaked into the marshaled response")
	}
	// The raw token is exposed only as part of the webhook URL the operator has
	// to paste into GitHub — never as a field of its own.
	assert.NotContains(t, body, `"webhook_token"`)
	assert.Contains(t, body, `"has_private_key":true`)
	assert.Contains(t, body, `"webhook_url":"https://vibexp.test/webhooks/github/tok-abc"`)
}
