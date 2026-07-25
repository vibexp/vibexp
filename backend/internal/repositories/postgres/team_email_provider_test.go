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

func setupTeamEmailProviderTest(t *testing.T) (*TeamEmailProviderRepository, sqlmock.Sqlmock) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})

	repo := NewTeamEmailProviderRepository(&database.DB{DB: mockDB}).(*TeamEmailProviderRepository)

	return repo, mock
}

// teamEmailProviderColumnNames mirrors teamEmailProviderColumns, in order.
var teamEmailProviderColumnNames = []string{
	"id", "team_id", "user_id", "provider_type", "settings",
	"secret_encrypted", "from_address", "from_name", "reply_to",
	"last_success_at", "last_error", "last_error_at", "created_at", "updated_at", "version",
}

func teamEmailProviderRow(now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(teamEmailProviderColumnNames).AddRow(
		"prov-1", "team-1", "user-1", "mailgun", []byte(`{"domain":"mg.example.com"}`),
		"enc-secret", "team@example.com", "Team", "reply@example.com",
		nil, nil, nil, now, now, int64(1),
	)
}

func newTeamEmailProvider() *models.TeamEmailProvider {
	userID := "user-1"
	fromName := "Team"
	replyTo := "reply@example.com"
	return &models.TeamEmailProvider{
		TeamID:          "team-1",
		UserID:          &userID,
		ProviderType:    "mailgun",
		Settings:        json.RawMessage(`{"domain":"mg.example.com"}`),
		SecretEncrypted: "enc-secret",
		FromAddress:     "team@example.com",
		FromName:        &fromName,
		ReplyTo:         &replyTo,
	}
}

func TestTeamEmailProviderRepository_GetByTeamID(t *testing.T) {
	repo, mock := setupTeamEmailProviderTest(t)
	now := time.Now().UTC()

	mock.ExpectQuery("SELECT (.+) FROM team_email_providers WHERE team_id = \\$1").
		WithArgs("team-1").
		WillReturnRows(teamEmailProviderRow(now))

	provider, err := repo.GetByTeamID(context.Background(), "team-1")

	require.NoError(t, err)
	assert.Equal(t, "prov-1", provider.ID)
	assert.Equal(t, "team-1", provider.TeamID)
	assert.Equal(t, "mailgun", provider.ProviderType)
	assert.Equal(t, "enc-secret", provider.SecretEncrypted)
	assert.Equal(t, "team@example.com", provider.FromAddress)
	assert.JSONEq(t, `{"domain":"mg.example.com"}`, string(provider.Settings))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamEmailProviderRepository_GetByTeamID_NotFound(t *testing.T) {
	repo, mock := setupTeamEmailProviderTest(t)

	mock.ExpectQuery("SELECT (.+) FROM team_email_providers WHERE team_id = \\$1").
		WithArgs("team-nope").
		WillReturnError(sql.ErrNoRows)

	provider, err := repo.GetByTeamID(context.Background(), "team-nope")

	assert.Nil(t, provider)
	// A team without its own provider is the ordinary case: callers detect it
	// with errors.Is and fall back to the instance provider.
	assert.ErrorIs(t, err, repositories.ErrTeamEmailProviderNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamEmailProviderRepository_GetByTeamID_QueryError(t *testing.T) {
	repo, mock := setupTeamEmailProviderTest(t)

	mock.ExpectQuery("SELECT (.+) FROM team_email_providers WHERE team_id = \\$1").
		WithArgs("team-1").
		WillReturnError(errors.New("connection reset"))

	provider, err := repo.GetByTeamID(context.Background(), "team-1")

	assert.Nil(t, provider)
	require.Error(t, err)
	// A real failure must NOT be reported as an absent provider, or the caller
	// would silently fall back to the instance provider on a DB blip.
	assert.NotErrorIs(t, err, repositories.ErrTeamEmailProviderNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamEmailProviderRepository_Upsert_Insert(t *testing.T) {
	repo, mock := setupTeamEmailProviderTest(t)
	now := time.Now().UTC()
	provider := newTeamEmailProvider()

	mock.ExpectQuery("INSERT INTO team_email_providers (.+) ON CONFLICT \\(team_id\\) DO UPDATE").
		WithArgs(
			"team-1", provider.UserID, "mailgun", provider.Settings,
			"enc-secret", "team@example.com", provider.FromName, provider.ReplyTo,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "version"}).
			AddRow("prov-1", now, now, int64(1)))

	err := repo.Upsert(context.Background(), provider)

	require.NoError(t, err)
	assert.Equal(t, "prov-1", provider.ID)
	assert.Equal(t, int64(1), provider.Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamEmailProviderRepository_Upsert_ConflictUpdateBumpsVersion(t *testing.T) {
	repo, mock := setupTeamEmailProviderTest(t)
	now := time.Now().UTC()
	provider := newTeamEmailProvider()

	// Second write for the same team: the conflict target keeps the original id
	// and row, so the version comes back incremented rather than a new row being
	// created.
	mock.ExpectQuery("INSERT INTO team_email_providers (.+) ON CONFLICT \\(team_id\\) DO UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "version"}).
			AddRow("prov-1", now.Add(-time.Hour), now, int64(2)))

	err := repo.Upsert(context.Background(), provider)

	require.NoError(t, err)
	assert.Equal(t, "prov-1", provider.ID, "the conflicting write must update the existing row")
	assert.Equal(t, int64(2), provider.Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A provider whose only configuration is its secret (SendGrid) has no settings.
// The column is NOT NULL and its '{}' default cannot fire for a named column, so
// the repository must substitute an empty object rather than send NULL.
func TestTeamEmailProviderRepository_Upsert_NilSettingsBecomesEmptyObject(t *testing.T) {
	repo, mock := setupTeamEmailProviderTest(t)
	now := time.Now().UTC()

	provider := &models.TeamEmailProvider{
		TeamID:          "team-1",
		ProviderType:    "sendgrid",
		SecretEncrypted: "enc-secret",
		FromAddress:     "team@example.com",
	}

	mock.ExpectQuery("INSERT INTO team_email_providers (.+) ON CONFLICT \\(team_id\\) DO UPDATE").
		WithArgs(
			"team-1", nil, "sendgrid", json.RawMessage(`{}`),
			"enc-secret", "team@example.com", nil, nil,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "version"}).
			AddRow("prov-1", now, now, int64(1)))

	err := repo.Upsert(context.Background(), provider)

	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(provider.Settings),
		"the stored value must be reflected back on the struct")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamEmailProviderRepository_Upsert_Error(t *testing.T) {
	repo, mock := setupTeamEmailProviderTest(t)

	mock.ExpectQuery("INSERT INTO team_email_providers (.+) ON CONFLICT \\(team_id\\) DO UPDATE").
		WillReturnError(errors.New("fk violation"))

	err := repo.Upsert(context.Background(), newTeamEmailProvider())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upsert team email provider")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamEmailProviderRepository_Delete(t *testing.T) {
	repo, mock := setupTeamEmailProviderTest(t)

	mock.ExpectExec("DELETE FROM team_email_providers WHERE team_id = \\$1").
		WithArgs("team-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Delete(context.Background(), "team-1")

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamEmailProviderRepository_Delete_NotFound(t *testing.T) {
	repo, mock := setupTeamEmailProviderTest(t)

	mock.ExpectExec("DELETE FROM team_email_providers WHERE team_id = \\$1").
		WithArgs("team-nope").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.Delete(context.Background(), "team-nope")

	assert.ErrorIs(t, err, repositories.ErrTeamEmailProviderNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamEmailProviderRepository_Delete_Error(t *testing.T) {
	repo, mock := setupTeamEmailProviderTest(t)

	mock.ExpectExec("DELETE FROM team_email_providers WHERE team_id = \\$1").
		WithArgs("team-1").
		WillReturnError(errors.New("connection reset"))

	err := repo.Delete(context.Background(), "team-1")

	require.Error(t, err)
	assert.NotErrorIs(t, err, repositories.ErrTeamEmailProviderNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamEmailProviderRepository_RecordSendResult_Success(t *testing.T) {
	repo, mock := setupTeamEmailProviderTest(t)
	at := time.Now().UTC()

	// Only last_success_at is written: a success must not erase the previous
	// error, and must not bump version (that would break a concurrent
	// optimistic-locked config update).
	mock.ExpectExec("UPDATE team_email_providers SET last_success_at = \\$2 WHERE team_id = \\$1").
		WithArgs("team-1", at).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.RecordSendResult(context.Background(), "team-1", nil, at)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamEmailProviderRepository_RecordSendResult_Failure(t *testing.T) {
	repo, mock := setupTeamEmailProviderTest(t)
	at := time.Now().UTC()

	mock.ExpectExec(
		"UPDATE team_email_providers SET last_error = \\$2, last_error_at = \\$3 WHERE team_id = \\$1").
		WithArgs("team-1", "smtp: connection refused", at).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.RecordSendResult(
		context.Background(), "team-1", errors.New("smtp: connection refused"), at)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamEmailProviderRepository_RecordSendResult_MissingRowIsNotAnError(t *testing.T) {
	repo, mock := setupTeamEmailProviderTest(t)
	at := time.Now().UTC()

	// The provider can be deleted while a send is in flight. Failing here would
	// turn that race into a spurious error on an already-completed send.
	mock.ExpectExec("UPDATE team_email_providers SET last_success_at = \\$2 WHERE team_id = \\$1").
		WithArgs("team-gone", at).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.RecordSendResult(context.Background(), "team-gone", nil, at)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamEmailProviderRepository_RecordSendResult_Error(t *testing.T) {
	repo, mock := setupTeamEmailProviderTest(t)
	at := time.Now().UTC()

	mock.ExpectExec("UPDATE team_email_providers SET last_success_at = \\$2 WHERE team_id = \\$1").
		WillReturnError(errors.New("connection reset"))

	err := repo.RecordSendResult(context.Background(), "team-1", nil, at)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to record team email provider send result")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamEmailProvider_IsHealthy(t *testing.T) {
	now := time.Now().UTC()
	earlier := now.Add(-time.Hour)

	tests := []struct {
		name     string
		provider models.TeamEmailProvider
		want     bool
	}{
		{
			name:     "never sent anything is healthy",
			provider: models.TeamEmailProvider{},
			want:     true,
		},
		{
			name:     "only a success is healthy",
			provider: models.TeamEmailProvider{LastSuccessAt: &now},
			want:     true,
		},
		{
			name:     "only a failure is unhealthy",
			provider: models.TeamEmailProvider{LastErrorAt: &now},
			want:     false,
		},
		{
			name:     "recovered: success after the failure",
			provider: models.TeamEmailProvider{LastSuccessAt: &now, LastErrorAt: &earlier},
			want:     true,
		},
		{
			name:     "regressed: failure after the success",
			provider: models.TeamEmailProvider{LastSuccessAt: &earlier, LastErrorAt: &now},
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.provider.IsHealthy())
		})
	}
}
