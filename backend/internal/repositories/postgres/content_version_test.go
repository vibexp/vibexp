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
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

func setupContentVersionRepoTest(t *testing.T) (*contentVersionRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := &contentVersionRepository{db: &database.DB{DB: mockDB}}
	return repo, mock, mockDB
}

func TestNewContentVersionRepository(t *testing.T) {
	t.Parallel()

	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	assert.NotNil(t, NewContentVersionRepository(&database.DB{DB: mockDB}))
}

func TestContentVersionRepository_Create_Success(t *testing.T) {
	t.Parallel()

	repo, dbMock, mockDB := setupContentVersionRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	now := time.Now().UTC()
	createdBy := "user-1"
	v := &models.ContentVersion{
		TeamID:       "team-1",
		ResourceType: "artifact",
		ResourceID:   "res-1",
		Content:      "old content",
		CreatedBy:    &createdBy,
	}

	// Args: team_id, resource_type, resource_id, content, change_summary, actor_type, created_by.
	// actor_type defaults to "human" when unset; the nullable change_summary/created_by are AnyArg.
	dbMock.ExpectQuery(`INSERT INTO content_versions`).
		WithArgs(v.TeamID, v.ResourceType, v.ResourceID, v.Content, sqlmock.AnyArg(), "human", sqlmock.AnyArg()).
		WillReturnRows(
			sqlmock.NewRows([]string{"id", "version_number", "created_at"}).AddRow("gen-id", 3, now),
		)

	err := repo.Create(context.Background(), v)

	require.NoError(t, err)
	assert.Equal(t, "gen-id", v.ID)
	assert.Equal(t, 3, v.VersionNumber)
	assert.Equal(t, now, v.CreatedAt)
	assert.Equal(t, models.ActorTypeHuman, v.ActorType) // default backfilled onto the model
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestContentVersionRepository_ListByResource_Success(t *testing.T) {
	t.Parallel()

	repo, dbMock, mockDB := setupContentVersionRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	now := time.Now().UTC()
	rows := sqlmock.NewRows(
		[]string{
			"id", "team_id", "resource_type", "resource_id", "version_number", "content",
			"change_summary", "actor_type", "created_by", "created_at",
		},
	).
		AddRow("v2", "team-1", "artifact", "res-1", 2, "content-2", "Tightened wording", "human", "user-1", now).
		AddRow("v1", "team-1", "artifact", "res-1", 1, "content-1", nil, "system", nil, now)

	dbMock.ExpectQuery(`SELECT .* FROM content_versions`).
		WithArgs("team-1", "artifact", "res-1").
		WillReturnRows(rows)

	versions, err := repo.ListByResource(context.Background(), "team-1", "artifact", "res-1")

	require.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, 2, versions[0].VersionNumber)
	require.NotNil(t, versions[0].CreatedBy)
	assert.Equal(t, "user-1", *versions[0].CreatedBy)
	require.NotNil(t, versions[0].ChangeSummary)
	assert.Equal(t, "Tightened wording", *versions[0].ChangeSummary)
	assert.Equal(t, models.ActorTypeHuman, versions[0].ActorType)
	assert.Nil(t, versions[1].CreatedBy)     // NULL created_by decodes to nil
	assert.Nil(t, versions[1].ChangeSummary) // NULL change_summary decodes to nil
	assert.Equal(t, models.ActorTypeSystem, versions[1].ActorType)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestContentVersionRepository_GetByVersionNumber_NotFound(t *testing.T) {
	t.Parallel()

	repo, dbMock, mockDB := setupContentVersionRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	dbMock.ExpectQuery(`SELECT .* FROM content_versions`).
		WithArgs("team-1", "artifact", "res-1", 99).
		WillReturnError(sql.ErrNoRows)

	_, err := repo.GetByVersionNumber(context.Background(), "team-1", "artifact", "res-1", 99)

	require.Error(t, err)
	assert.True(t, errors.Is(err, repositories.ErrContentVersionNotFound))
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestContentVersionRepository_GetByVersionNumber_Success(t *testing.T) {
	t.Parallel()

	repo, dbMock, mockDB := setupContentVersionRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	now := time.Now().UTC()
	dbMock.ExpectQuery(`SELECT .* FROM content_versions`).
		WithArgs("team-1", "artifact", "res-1", 2).
		WillReturnRows(sqlmock.NewRows(
			[]string{
				"id", "team_id", "resource_type", "resource_id", "version_number", "content",
				"change_summary", "actor_type", "created_by", "created_at",
			},
		).AddRow("v2", "team-1", "artifact", "res-1", 2, "content-2", "Tightened wording", "human", "user-1", now))

	v, err := repo.GetByVersionNumber(context.Background(), "team-1", "artifact", "res-1", 2)

	require.NoError(t, err)
	assert.Equal(t, 2, v.VersionNumber)
	assert.Equal(t, "content-2", v.Content)
	require.NotNil(t, v.ChangeSummary)
	assert.Equal(t, "Tightened wording", *v.ChangeSummary)
	assert.Equal(t, models.ActorTypeHuman, v.ActorType)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestContentVersionRepository_PruneToCap(t *testing.T) {
	t.Parallel()

	repo, dbMock, mockDB := setupContentVersionRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	dbMock.ExpectExec(`DELETE FROM content_versions`).
		WithArgs("artifact", "res-1", 5).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.PruneToCap(context.Background(), "artifact", "res-1", 5)

	require.NoError(t, err)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestContentVersionRepository_PruneToCap_NonPositiveKeepIsNoOp(t *testing.T) {
	t.Parallel()

	repo, dbMock, mockDB := setupContentVersionRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	// No ExpectExec registered: a query would fail ExpectationsWereMet.
	err := repo.PruneToCap(context.Background(), "artifact", "res-1", 0)

	require.NoError(t, err)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}
