package postgres

import (
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
)

func setupModelProviderTest(t *testing.T) (*ModelProviderRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := NewModelProviderRepository(db).(*ModelProviderRepository)

	return repo, mock, mockDB
}

// modelProviderListColumns mirrors the columns selected by List (no version).
var modelProviderListColumns = []string{
	"id", "user_id", "team_id", "name", "provider_type", "model",
	"is_default", "base_url", "api_key_encrypted", "configuration",
	"created_at", "updated_at",
}

func TestModelProviderRepository_Create(t *testing.T) {
	repo, mock, mockDB := setupModelProviderTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()
	now := time.Now()
	teamID := "team-1"
	baseURL := "https://api.openai.com/v1"
	enc := "encrypted"
	provider := &models.ModelProvider{
		UserID: "user-1", TeamID: &teamID, Name: "OpenAI", ProviderType: "openai_compatible",
		Model: "gpt-4o-mini", IsDefault: true, BaseURL: &baseURL, APIKeyEncrypted: &enc,
		Configuration: "{}", CreatedAt: now, UpdatedAt: now,
	}

	returned := sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow("prov-1", now, now)
	mock.ExpectQuery(`INSERT INTO model_providers`).
		WithArgs("user-1", &teamID, "OpenAI", "openai_compatible", "gpt-4o-mini",
			true, &baseURL, &enc, "{}", now, now).
		WillReturnRows(returned)

	require.NoError(t, repo.Create(ctx, provider))
	assert.Equal(t, "prov-1", provider.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelProviderRepository_GetByID(t *testing.T) {
	repo, mock, mockDB := setupModelProviderTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()
	now := time.Now()

	t.Run("found", func(t *testing.T) {
		cols := append(append([]string{}, modelProviderListColumns...), "version")
		rows := sqlmock.NewRows(cols).
			AddRow("prov-1", "user-1", nil, "OpenAI", "openai_compatible", "gpt-4o-mini",
				true, nil, "enc", "{}", now, now, int64(3))
		mock.ExpectQuery(`SELECT .+ FROM model_providers\s+WHERE id = \$1 AND team_id = \$2`).
			WithArgs("prov-1", "team-1").
			WillReturnRows(rows)

		got, err := repo.GetByID(ctx, "team-1", "prov-1")
		require.NoError(t, err)
		assert.Equal(t, "prov-1", got.ID)
		assert.Equal(t, int64(3), got.Version)
	})

	t.Run("not found maps to sentinel", func(t *testing.T) {
		mock.ExpectQuery(`SELECT .+ FROM model_providers\s+WHERE id = \$1 AND team_id = \$2`).
			WithArgs("missing", "team-1").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByID(ctx, "team-1", "missing")
		require.ErrorIs(t, err, repositories.ErrModelProviderNotFound)
	})

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelProviderRepository_Update_OptimisticLock(t *testing.T) {
	repo, mock, mockDB := setupModelProviderTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()
	now := time.Now()
	teamID := "team-1"
	provider := &models.ModelProvider{
		ID: "prov-1", TeamID: &teamID, Name: "Renamed", ProviderType: "openai_compatible",
		Model: "gpt-4o-mini", IsDefault: false, Configuration: "{}", UpdatedAt: now, Version: 4,
	}

	t.Run("success bumps version", func(t *testing.T) {
		mock.ExpectQuery(`UPDATE model_providers\s+SET .+ version = version \+ 1\s+WHERE id = \$1 AND team_id = \$10 AND version = \$11`).
			WillReturnRows(sqlmock.NewRows([]string{"updated_at", "version"}).AddRow(now, int64(5)))

		require.NoError(t, repo.Update(ctx, provider))
		assert.Equal(t, int64(5), provider.Version)
	})

	t.Run("version mismatch is an error", func(t *testing.T) {
		mock.ExpectQuery(`UPDATE model_providers`).WillReturnError(sql.ErrNoRows)
		require.Error(t, repo.Update(ctx, provider))
	})

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelProviderRepository_Delete(t *testing.T) {
	repo, mock, mockDB := setupModelProviderTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()

	t.Run("success", func(t *testing.T) {
		mock.ExpectExec(`DELETE FROM model_providers WHERE id = \$1 AND team_id = \$2`).
			WithArgs("prov-1", "team-1").
			WillReturnResult(sqlmock.NewResult(0, 1))
		require.NoError(t, repo.Delete(ctx, "team-1", "prov-1"))
	})

	t.Run("no rows maps to sentinel", func(t *testing.T) {
		mock.ExpectExec(`DELETE FROM model_providers WHERE id = \$1 AND team_id = \$2`).
			WithArgs("missing", "team-1").
			WillReturnResult(sqlmock.NewResult(0, 0))
		require.ErrorIs(t, repo.Delete(ctx, "team-1", "missing"), repositories.ErrModelProviderNotFound)
	})

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelProviderRepository_GetDefault(t *testing.T) {
	repo, mock, mockDB := setupModelProviderTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()
	now := time.Now()

	t.Run("found", func(t *testing.T) {
		rows := sqlmock.NewRows(modelProviderListColumns).
			AddRow("prov-1", "user-1", nil, "OpenAI", "openai_compatible", "gpt-4o-mini",
				true, nil, "enc", "{}", now, now)
		mock.ExpectQuery(`SELECT .+ FROM model_providers\s+WHERE team_id = \$1 AND is_default = true`).
			WithArgs("team-1").
			WillReturnRows(rows)

		got, err := repo.GetDefault(ctx, "team-1")
		require.NoError(t, err)
		assert.True(t, got.IsDefault)
	})

	t.Run("none maps to sentinel", func(t *testing.T) {
		mock.ExpectQuery(`SELECT .+ FROM model_providers\s+WHERE team_id = \$1 AND is_default = true`).
			WithArgs("team-1").
			WillReturnError(sql.ErrNoRows)
		_, err := repo.GetDefault(ctx, "team-1")
		require.ErrorIs(t, err, repositories.ErrDefaultModelProviderNotFound)
	})

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelProviderRepository_SetDefault(t *testing.T) {
	repo, mock, mockDB := setupModelProviderTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE model_providers SET is_default = false WHERE team_id = \$1`).
		WithArgs("team-1").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`UPDATE model_providers SET is_default = true WHERE id = \$1 AND team_id = \$2`).
		WithArgs("prov-1", "team-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, repo.SetDefault(ctx, "team-1", "prov-1"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelProviderRepository_Count(t *testing.T) {
	repo, mock, mockDB := setupModelProviderTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM model_providers WHERE team_id = \$1`).
		WithArgs("team-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	count, err := repo.Count(ctx, "team-1")
	require.NoError(t, err)
	assert.Equal(t, 3, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelProviderRepository_List(t *testing.T) {
	repo, mock, mockDB := setupModelProviderTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()
	now := time.Now()

	countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM model_providers WHERE \(team_id = \$1\)`).
		WithArgs("team-1").
		WillReturnRows(countRows)

	rows := sqlmock.NewRows(modelProviderListColumns).
		AddRow("p1", "user-1", nil, "Default", "openai_compatible", "gpt-4o-mini", true, nil, "enc", "{}", now, now).
		AddRow("p2", "user-1", nil, "Other", "openai_compatible", "gpt-4o", false, nil, "enc", "{}", now, now)
	mock.ExpectQuery(`SELECT .+ FROM model_providers WHERE \(team_id = \$1\)`).
		WithArgs("team-1").
		WillReturnRows(rows)

	providers, total, err := repo.List(ctx, "team-1", repositories.ModelProviderFilters{Page: 1, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, providers, 2)
	assert.Equal(t, "p1", providers[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelProviderRepository_Count_Error(t *testing.T) {
	repo, mock, mockDB := setupModelProviderTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM model_providers WHERE team_id = \$1`).
		WithArgs("team-1").
		WillReturnError(errors.New("boom"))

	_, err := repo.Count(ctx, "team-1")
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
