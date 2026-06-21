package postgres

import (
	"context"
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

func setupDeviceTokenTest(t *testing.T) (*DeviceTokenRepository, sqlmock.Sqlmock, func()) {
	t.Helper()

	mockDB, dbMock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := NewDeviceTokenRepository(db).(*DeviceTokenRepository)

	return repo, dbMock, func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}
}

func TestDeviceTokenRepository_Upsert_Success(t *testing.T) {
	repo, dbMock, cleanup := setupDeviceTokenTest(t)
	defer cleanup()

	ctx := context.Background()
	dt := &models.DeviceToken{
		UserID:    "user-1",
		Token:     "fcm-token-abc",
		Platform:  "web",
		UserAgent: "Mozilla/5.0",
	}

	rows := sqlmock.NewRows([]string{"id"}).AddRow("some-id")
	dbMock.ExpectQuery(`INSERT INTO device_tokens`).
		WithArgs(dt.UserID, dt.Token, dt.Platform, dt.UserAgent).
		WillReturnRows(rows)

	err := repo.Upsert(ctx, dt)
	assert.NoError(t, err)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestDeviceTokenRepository_Upsert_DBError(t *testing.T) {
	repo, dbMock, cleanup := setupDeviceTokenTest(t)
	defer cleanup()

	ctx := context.Background()
	dt := &models.DeviceToken{
		UserID:   "user-1",
		Token:    "fcm-token-abc",
		Platform: "web",
	}

	dbMock.ExpectQuery(`INSERT INTO device_tokens`).
		WillReturnError(errors.New("db error"))

	err := repo.Upsert(ctx, dt)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "upsert device token")
}

// TestDeviceTokenRepository_Upsert_CrossUserConflict verifies that upserting a token
// that is already registered to a different user returns ErrDeviceTokenConflict and
// does not modify the existing record.
func TestDeviceTokenRepository_Upsert_CrossUserConflict(t *testing.T) {
	repo, dbMock, cleanup := setupDeviceTokenTest(t)
	defer cleanup()

	ctx := context.Background()

	// User B tries to register a token already owned by user A.
	// The DB's conditional ON CONFLICT ... WHERE clause produces no RETURNING row,
	// which the driver surfaces as sql.ErrNoRows.
	dtUserB := &models.DeviceToken{
		UserID:   "user-B",
		Token:    "shared-token",
		Platform: "web",
	}

	dbMock.ExpectQuery(`INSERT INTO device_tokens`).
		WithArgs(dtUserB.UserID, dtUserB.Token, dtUserB.Platform, dtUserB.UserAgent).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // empty result set → sql.ErrNoRows

	err := repo.Upsert(ctx, dtUserB)
	require.Error(t, err)
	// The conflict sentinel must be surfaced when the token belongs to a different user.
	assert.ErrorIs(t, err, repositories.ErrDeviceTokenConflict)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestDeviceTokenRepository_ListByUserID_Success(t *testing.T) {
	repo, dbMock, cleanup := setupDeviceTokenTest(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "token", "platform", "user_agent", "last_used_at", "created_at",
	}).AddRow("id-1", "user-1", "tok1", "web", "Mozilla", now, now)

	dbMock.ExpectQuery(`SELECT id, user_id, token, platform`).
		WithArgs("user-1").
		WillReturnRows(rows)

	tokens, err := repo.ListByUserID(ctx, "user-1")
	require.NoError(t, err)
	assert.Len(t, tokens, 1)
	assert.Equal(t, "tok1", tokens[0].Token)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestDeviceTokenRepository_ListByUserID_Empty(t *testing.T) {
	repo, dbMock, cleanup := setupDeviceTokenTest(t)
	defer cleanup()

	ctx := context.Background()

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "token", "platform", "user_agent", "last_used_at", "created_at",
	})

	dbMock.ExpectQuery(`SELECT id, user_id, token, platform`).
		WithArgs("user-no-tokens").
		WillReturnRows(rows)

	tokens, err := repo.ListByUserID(ctx, "user-no-tokens")
	require.NoError(t, err)
	assert.Empty(t, tokens)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestDeviceTokenRepository_ListByUserID_DBError(t *testing.T) {
	repo, dbMock, cleanup := setupDeviceTokenTest(t)
	defer cleanup()

	ctx := context.Background()

	dbMock.ExpectQuery(`SELECT id, user_id, token, platform`).
		WillReturnError(errors.New("conn error"))

	tokens, err := repo.ListByUserID(ctx, "user-1")
	assert.Error(t, err)
	assert.Nil(t, tokens)
	assert.Contains(t, err.Error(), "list device tokens by user_id")
}

func TestDeviceTokenRepository_ListByUserID_ScanError(t *testing.T) {
	repo, dbMock, cleanup := setupDeviceTokenTest(t)
	defer cleanup()

	ctx := context.Background()

	// Provide a row with the wrong column type to trigger a scan error
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "token", "platform", "user_agent", "last_used_at", "created_at",
	}).AddRow(nil, nil, nil, nil, nil, nil, nil) // nulls will fail non-nullable scan

	dbMock.ExpectQuery(`SELECT id, user_id, token, platform`).
		WithArgs("user-1").
		WillReturnRows(rows)

	_, err := repo.ListByUserID(ctx, "user-1")
	assert.Error(t, err)
}

func TestDeviceTokenRepository_Delete_Success(t *testing.T) {
	repo, dbMock, cleanup := setupDeviceTokenTest(t)
	defer cleanup()

	ctx := context.Background()

	dbMock.ExpectExec(`DELETE FROM device_tokens WHERE token = \$1 AND user_id = \$2`).
		WithArgs("tok1", "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Delete(ctx, "tok1", "user-1")
	assert.NoError(t, err)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestDeviceTokenRepository_Delete_DBError(t *testing.T) {
	repo, dbMock, cleanup := setupDeviceTokenTest(t)
	defer cleanup()

	ctx := context.Background()

	dbMock.ExpectExec(`DELETE FROM device_tokens WHERE token = \$1 AND user_id = \$2`).
		WillReturnError(errors.New("db error"))

	err := repo.Delete(ctx, "tok1", "user-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delete device token")
}

func TestDeviceTokenRepository_DeleteByTokens_Empty(t *testing.T) {
	repo, _, cleanup := setupDeviceTokenTest(t)
	defer cleanup()

	// Should be a no-op for empty slice
	err := repo.DeleteByTokens(context.Background(), []string{})
	assert.NoError(t, err)
}

func TestDeviceTokenRepository_DeleteByTokens_Success(t *testing.T) {
	repo, dbMock, cleanup := setupDeviceTokenTest(t)
	defer cleanup()

	ctx := context.Background()

	dbMock.ExpectExec(`DELETE FROM device_tokens WHERE token = ANY\(\$1\)`).
		WithArgs(pq.Array([]string{"tok1", "tok2"})).
		WillReturnResult(sqlmock.NewResult(0, 2))

	err := repo.DeleteByTokens(ctx, []string{"tok1", "tok2"})
	assert.NoError(t, err)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestDeviceTokenRepository_DeleteByTokens_DBError(t *testing.T) {
	repo, dbMock, cleanup := setupDeviceTokenTest(t)
	defer cleanup()

	ctx := context.Background()

	dbMock.ExpectExec(`DELETE FROM device_tokens WHERE token = ANY\(\$1\)`).
		WillReturnError(errors.New("db error"))

	err := repo.DeleteByTokens(ctx, []string{"tok1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delete device tokens by token list")
}
