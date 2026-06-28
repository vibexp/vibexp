package postgres

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
)

func newMockDB(t *testing.T) (*database.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	mockDB, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	return &database.DB{DB: mockDB}, dbMock, func() {
		if cerr := mockDB.Close(); cerr != nil {
			t.Logf("failed to close mock DB: %v", cerr)
		}
	}
}

func TestOAuthClientRepository_GetByID(t *testing.T) {
	db, dbMock, cleanup := newMockDB(t)
	defer cleanup()
	repo := NewOAuthClientRepository(db)

	rows := sqlmock.NewRows([]string{
		"id", "secret_hash", "redirect_uris", "grant_types", "response_types", "scopes",
		"audience", "public", "token_endpoint_auth_method", "client_name", "created_at",
	}).AddRow(
		"client-1", nil, "{https://app/cb}", "{authorization_code,refresh_token}", "{code}", "{}",
		"{https://mcp}", true, "none", "App", nowForTest(),
	)
	dbMock.ExpectQuery(`SELECT .* FROM oauth_clients WHERE id = \$1`).
		WithArgs("client-1").WillReturnRows(rows)

	client, err := repo.GetByID(context.Background(), "client-1")
	require.NoError(t, err)
	assert.Equal(t, "client-1", client.ID)
	assert.True(t, client.Public)
	assert.Equal(t, []string{"https://app/cb"}, client.RedirectURIs)
	assert.Equal(t, []string{"https://mcp"}, client.Audience)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestOAuthClientRepository_GetByID_NotFound(t *testing.T) {
	db, dbMock, cleanup := newMockDB(t)
	defer cleanup()
	repo := NewOAuthClientRepository(db)

	dbMock.ExpectQuery(`SELECT .* FROM oauth_clients WHERE id = \$1`).
		WithArgs("missing").WillReturnError(errNoRows())

	_, err := repo.GetByID(context.Background(), "missing")
	assert.ErrorIs(t, err, repositories.ErrOAuthClientNotFound)
}

func TestOAuthRequestRepository_GetActiveAndDeactivate(t *testing.T) {
	db, dbMock, cleanup := newMockDB(t)
	defer cleanup()
	repo := NewOAuthCodeRepository(db)

	rows := sqlmock.NewRows([]string{
		"signature", "request_id", "client_id", "subject", "requested_scope", "granted_scope",
		"requested_audience", "granted_audience", "requested_at", "form_data", "session_data", "active",
	}).AddRow(
		"sig-1", "req-1", "client-1", "user-1", "{}", "{}",
		"{}", "{https://mcp}", nowForTest(), []byte(`{}`), []byte(`{}`), true,
	)
	dbMock.ExpectQuery(`SELECT .* FROM oauth_authorization_codes WHERE signature = \$1`).
		WithArgs("sig-1").WillReturnRows(rows)

	got, err := repo.Get(context.Background(), "sig-1")
	require.NoError(t, err)
	assert.True(t, got.Active)
	assert.Equal(t, "req-1", got.RequestID)

	dbMock.ExpectExec(`UPDATE oauth_authorization_codes SET active = false WHERE signature = \$1`).
		WithArgs("sig-1").WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.Deactivate(context.Background(), "sig-1"))
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestOAuthRequestRepository_RevocationByRequestID(t *testing.T) {
	db, dbMock, cleanup := newMockDB(t)
	defer cleanup()
	refresh := NewOAuthRefreshTokenRepository(db)

	dbMock.ExpectExec(`UPDATE oauth_refresh_tokens SET active = false WHERE request_id = \$1`).
		WithArgs("req-1").WillReturnResult(sqlmock.NewResult(0, 2))
	require.NoError(t, refresh.DeactivateByRequestID(context.Background(), "req-1"))
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestOAuthSigningKeyRepository_Activate(t *testing.T) {
	db, dbMock, cleanup := newMockDB(t)
	defer cleanup()
	repo := NewOAuthSigningKeyRepository(db)

	dbMock.ExpectBegin()
	dbMock.ExpectExec(`UPDATE oauth_signing_keys SET active = false, rotated_at = CURRENT_TIMESTAMP\s+WHERE active AND kid <> \$1`).
		WithArgs("kid-2").WillReturnResult(sqlmock.NewResult(0, 1))
	dbMock.ExpectExec(`UPDATE oauth_signing_keys SET active = true WHERE kid = \$1`).
		WithArgs("kid-2").WillReturnResult(sqlmock.NewResult(0, 1))
	dbMock.ExpectCommit()

	require.NoError(t, repo.Activate(context.Background(), "kid-2"))
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestOAuthSigningKeyRepository_ActivateMissing(t *testing.T) {
	db, dbMock, cleanup := newMockDB(t)
	defer cleanup()
	repo := NewOAuthSigningKeyRepository(db)

	dbMock.ExpectBegin()
	dbMock.ExpectExec(`UPDATE oauth_signing_keys SET active = false`).
		WithArgs("nope").WillReturnResult(sqlmock.NewResult(0, 0))
	dbMock.ExpectExec(`UPDATE oauth_signing_keys SET active = true WHERE kid = \$1`).
		WithArgs("nope").WillReturnResult(sqlmock.NewResult(0, 0))
	dbMock.ExpectRollback()

	err := repo.Activate(context.Background(), "nope")
	assert.ErrorIs(t, err, repositories.ErrOAuthSigningKeyNotFound)
}

func TestOAuthLoginSessionRepository_AttachUserMissing(t *testing.T) {
	db, dbMock, cleanup := newMockDB(t)
	defer cleanup()
	repo := NewOAuthLoginSessionRepository(db)

	dbMock.ExpectExec(`UPDATE oauth_login_sessions SET user_id = \$2 WHERE id = \$1`).
		WithArgs("missing", "user-1").WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.AttachUser(context.Background(), "missing", "user-1")
	assert.ErrorIs(t, err, repositories.ErrOAuthLoginSessionNotFound)
}

func TestOAuthRequestRepository_CreatePersistsExpiresAt(t *testing.T) {
	db, dbMock, cleanup := newMockDB(t)
	defer cleanup()
	repo := NewOAuthAccessTokenRepository(db)

	exp := nowForTest().Add(15 * time.Minute)
	dbMock.ExpectExec(`INSERT INTO oauth_access_tokens`).
		WithArgs(
			"sig-1", "req-1", "client-1", "user-1",
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			nowForTest(), []byte(`{}`), []byte(`{}`), true, sql.NullTime{Time: exp, Valid: true},
		).WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Create(context.Background(), &models.OAuthRequest{
		Signature: "sig-1", RequestID: "req-1", ClientID: "client-1", Subject: "user-1",
		RequestedAt: nowForTest(), FormData: []byte(`{}`), SessionData: []byte(`{}`),
		Active: true, ExpiresAt: exp,
	})
	require.NoError(t, err)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestOAuthRequestRepository_CreateZeroExpiresAtIsNull(t *testing.T) {
	db, dbMock, cleanup := newMockDB(t)
	defer cleanup()
	repo := NewOAuthCodeRepository(db)

	dbMock.ExpectExec(`INSERT INTO oauth_authorization_codes`).
		WithArgs(
			"sig-2", "req-2", "client-1", "",
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			nowForTest(), []byte(`{}`), []byte(`{}`), true, sql.NullTime{},
		).WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Create(context.Background(), &models.OAuthRequest{
		Signature: "sig-2", RequestID: "req-2", ClientID: "client-1",
		RequestedAt: nowForTest(), FormData: []byte(`{}`), SessionData: []byte(`{}`), Active: true,
	})
	require.NoError(t, err)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

// TestOAuthRequestRepository_CreateNilArraysBindEmpty guards the regression where
// a request carrying an RFC 8707 `resource` parameter but no `audience` (as the
// Claude Code MCP client sends) yields a nil RequestedAudience. pq encodes a nil
// slice as SQL NULL, which violates the requested_audience text[] NOT NULL
// constraint and fails the authorization-code insert. All four array columns must
// be bound as the empty array '{}' instead.
func TestOAuthRequestRepository_CreateNilArraysBindEmpty(t *testing.T) {
	db, dbMock, cleanup := newMockDB(t)
	defer cleanup()
	repo := NewOAuthCodeRepository(db)

	dbMock.ExpectExec(`INSERT INTO oauth_authorization_codes`).
		WithArgs(
			"sig-3", "req-3", "client-1", "user-1",
			"{}", "{}", "{}", "{}", // requested/granted scope + requested/granted audience
			nowForTest(), []byte(`{}`), []byte(`{}`), true, sql.NullTime{},
		).WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Create(context.Background(), &models.OAuthRequest{
		Signature: "sig-3", RequestID: "req-3", ClientID: "client-1", Subject: "user-1",
		RequestedScope: nil, GrantedScope: nil, RequestedAudience: nil, GrantedAudience: nil,
		RequestedAt: nowForTest(), FormData: []byte(`{}`), SessionData: []byte(`{}`), Active: true,
	})
	require.NoError(t, err)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestOAuthRequestRepository_DeleteExpired(t *testing.T) {
	db, dbMock, cleanup := newMockDB(t)
	defer cleanup()
	repo := NewOAuthPKCERepository(db)

	dbMock.ExpectExec(`DELETE FROM oauth_pkce_sessions WHERE expires_at <= CURRENT_TIMESTAMP`).
		WillReturnResult(sqlmock.NewResult(0, 3))

	n, err := repo.DeleteExpired(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestOAuthSigningKeyRepository_DeleteRetiredBefore(t *testing.T) {
	db, dbMock, cleanup := newMockDB(t)
	defer cleanup()
	repo := NewOAuthSigningKeyRepository(db)

	cutoff := nowForTest()
	dbMock.ExpectExec(`DELETE FROM oauth_signing_keys WHERE active = false AND rotated_at IS NOT NULL AND rotated_at <= \$1`).
		WithArgs(cutoff).WillReturnResult(sqlmock.NewResult(0, 2))

	n, err := repo.DeleteRetiredBefore(context.Background(), cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestOAuthSigningKeyRepository_TryAdvisoryLock(t *testing.T) {
	db, dbMock, cleanup := newMockDB(t)
	defer cleanup()
	repo := NewOAuthSigningKeyRepository(db)

	// Acquired: lock returns true, release unlocks.
	dbMock.ExpectQuery(`SELECT pg_try_advisory_lock\(\$1\)`).
		WithArgs(signingKeyRotationLockID).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	dbMock.ExpectExec(`SELECT pg_advisory_unlock\(\$1\)`).
		WithArgs(signingKeyRotationLockID).WillReturnResult(sqlmock.NewResult(0, 1))

	acquired, release, err := repo.TryAdvisoryLock(context.Background())
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, release())
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestOAuthSigningKeyRepository_TryAdvisoryLockContended(t *testing.T) {
	db, dbMock, cleanup := newMockDB(t)
	defer cleanup()
	repo := NewOAuthSigningKeyRepository(db)

	// Contended: lock returns false; no unlock issued, release is a no-op.
	dbMock.ExpectQuery(`SELECT pg_try_advisory_lock\(\$1\)`).
		WithArgs(signingKeyRotationLockID).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))

	acquired, release, err := repo.TryAdvisoryLock(context.Background())
	require.NoError(t, err)
	assert.False(t, acquired)
	require.NoError(t, release())
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func errNoRows() error {
	return sql.ErrNoRows
}

func nowForTest() time.Time {
	return time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
}
