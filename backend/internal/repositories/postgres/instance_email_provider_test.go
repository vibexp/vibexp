package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
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

// Statement-level suite for InstanceEmailProviderRepository: the error paths
// and argument plumbing. What the database does with the statements (the
// singleton, ON CONFLICT behaviour, health isolation) is pinned by the
// integration suite.

func setupInstanceEmailProviderTest(t *testing.T) (*InstanceEmailProviderRepository, sqlmock.Sqlmock) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})

	repo := NewInstanceEmailProviderRepository(&database.DB{DB: mockDB}).(*InstanceEmailProviderRepository)

	return repo, mock
}

var errInstanceEmailDB = errors.New("connection reset")

func TestInstanceEmailProviderRepository_Get(t *testing.T) {
	repo, mock := setupInstanceEmailProviderTest(t)
	now := time.Now().UTC()

	mock.ExpectQuery("SELECT (.+) FROM instance_email_provider").
		WillReturnRows(sqlmock.NewRows([]string{
			"provider_type", "settings", "secret_encrypted", "from_address", "from_name", "reply_to",
			"contact_recipient_address", "privacy_policy_url", "last_success_at", "last_error",
			"last_error_at", "created_at", "updated_at", "updated_by", "version",
		}).AddRow(
			"smtp", []byte(`{"host":"smtp.example.com"}`), nil, "noreply@example.com", nil, nil,
			"contact@example.com", nil, nil, nil, nil, now, now, nil, int64(3),
		))

	got, err := repo.Get(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "smtp", got.ProviderType)
	assert.Nil(t, got.SecretEncrypted, "a NULL secret scans as nil")
	require.NotNil(t, got.ContactRecipientAddress)
	assert.Equal(t, "contact@example.com", *got.ContactRecipientAddress)
	assert.Equal(t, int64(3), got.Version)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstanceEmailProviderRepository_Get_Errors(t *testing.T) {
	t.Run("no row maps to the sentinel", func(t *testing.T) {
		repo, mock := setupInstanceEmailProviderTest(t)
		mock.ExpectQuery("SELECT (.+) FROM instance_email_provider").WillReturnError(sql.ErrNoRows)

		_, err := repo.Get(context.Background())

		assert.ErrorIs(t, err, repositories.ErrInstanceEmailProviderNotFound)
	})

	t.Run("a database failure is not reported as not-found", func(t *testing.T) {
		repo, mock := setupInstanceEmailProviderTest(t)
		mock.ExpectQuery("SELECT (.+) FROM instance_email_provider").WillReturnError(errInstanceEmailDB)

		_, err := repo.Get(context.Background())

		assert.ErrorIs(t, err, errInstanceEmailDB)
		assert.NotErrorIs(t, err, repositories.ErrInstanceEmailProviderNotFound)
	})
}

func TestInstanceEmailProviderRepository_Upsert_NormalisesEmptySettings(t *testing.T) {
	repo, mock := setupInstanceEmailProviderTest(t)
	now := time.Now().UTC()

	mock.ExpectQuery("INSERT INTO instance_email_provider (.+) ON CONFLICT \\(id\\) DO UPDATE").
		WithArgs("sendgrid", json.RawMessage(`{}`), sqlmock.AnyArg(), "noreply@example.com",
			nil, nil, nil, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at", "version"}).
			AddRow(now, now, int64(2)))

	provider := &models.InstanceEmailProvider{ProviderType: "sendgrid", FromAddress: "noreply@example.com"}
	require.NoError(t, repo.Upsert(context.Background(), provider))

	assert.JSONEq(t, `{}`, string(provider.Settings))
	assert.Equal(t, int64(2), provider.Version)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstanceEmailProviderRepository_WriteErrors(t *testing.T) {
	provider := func() *models.InstanceEmailProvider {
		return &models.InstanceEmailProvider{ProviderType: "smtp", FromAddress: "noreply@example.com"}
	}

	t.Run("Upsert", func(t *testing.T) {
		repo, mock := setupInstanceEmailProviderTest(t)
		mock.ExpectQuery("INSERT INTO instance_email_provider").WillReturnError(errInstanceEmailDB)

		assert.ErrorIs(t, repo.Upsert(context.Background(), provider()), errInstanceEmailDB)
	})

	t.Run("InsertIfAbsent", func(t *testing.T) {
		repo, mock := setupInstanceEmailProviderTest(t)
		mock.ExpectQuery("INSERT INTO instance_email_provider (.+) ON CONFLICT \\(id\\) DO NOTHING").
			WillReturnError(errInstanceEmailDB)

		inserted, err := repo.InsertIfAbsent(context.Background(), provider())

		assert.False(t, inserted)
		assert.ErrorIs(t, err, errInstanceEmailDB)
	})

	t.Run("Delete", func(t *testing.T) {
		repo, mock := setupInstanceEmailProviderTest(t)
		mock.ExpectExec("DELETE FROM instance_email_provider").WillReturnError(errInstanceEmailDB)

		assert.ErrorIs(t, repo.Delete(context.Background()), errInstanceEmailDB)
	})

	t.Run("Delete rows affected", func(t *testing.T) {
		repo, mock := setupInstanceEmailProviderTest(t)
		mock.ExpectExec("DELETE FROM instance_email_provider").
			WillReturnResult(sqlmock.NewErrorResult(errInstanceEmailDB))

		assert.ErrorIs(t, repo.Delete(context.Background()), errInstanceEmailDB)
	})

	t.Run("RecordSuccess", func(t *testing.T) {
		repo, mock := setupInstanceEmailProviderTest(t)
		mock.ExpectExec("UPDATE instance_email_provider SET last_success_at").WillReturnError(errInstanceEmailDB)

		assert.ErrorIs(t, repo.RecordSuccess(context.Background(), time.Now()), errInstanceEmailDB)
	})

	t.Run("RecordError", func(t *testing.T) {
		repo, mock := setupInstanceEmailProviderTest(t)
		mock.ExpectExec("UPDATE instance_email_provider SET last_error").WillReturnError(errInstanceEmailDB)

		err := repo.RecordError(context.Background(), errors.New("smtp: 535"), time.Now())
		assert.ErrorIs(t, err, errInstanceEmailDB)
	})
}

func TestInstanceEmailProviderRepository_RecordError_NilErrorRecordsSuccess(t *testing.T) {
	repo, mock := setupInstanceEmailProviderTest(t)
	at := time.Now().UTC()

	mock.ExpectExec("UPDATE instance_email_provider SET last_success_at = \\$1").
		WithArgs(at).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.RecordError(context.Background(), nil, at))
	require.NoError(t, mock.ExpectationsWereMet())
}
