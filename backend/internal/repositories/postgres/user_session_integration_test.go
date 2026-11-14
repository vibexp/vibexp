//go:build integration

package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

// Real-Postgres coverage for the sessions table (#1646): create, read, the
// token and expiry updates, sign-out, the expired-row purge, and the cascade
// when the user goes.

func sessionHash(seed string) []byte {
	sum := sha256.Sum256([]byte(seed))
	return sum[:]
}

func newIntegrationSession(userID string, expiresIn time.Duration) *models.UserSession {
	now := time.Now()
	return &models.UserSession{
		IDHash: sessionHash(uuid.NewString()), UserID: userID, Provider: "oidc",
		AuthMethod: "oauth", AuthenticatedAt: now.Add(-time.Minute).UTC().Truncate(time.Microsecond),
		AccessTokenEnc: "enc-at", RefreshTokenEnc: "enc-rt",
		AccessExpiresAt: now.Add(time.Hour).UTC().Truncate(time.Microsecond),
		ExpiresAt:       now.Add(expiresIn).UTC().Truncate(time.Microsecond),
	}
}

func TestIntegrationUserSession_Lifecycle(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	repo := NewUserSessionRepository(integrationDB)
	userID := insertTestUser(t)
	s := newIntegrationSession(userID, 24*time.Hour)

	require.NoError(t, repo.Create(ctx, s))

	got, err := repo.GetByIDHash(ctx, s.IDHash)
	require.NoError(t, err)
	assert.Equal(t, userID, got.UserID)
	assert.Equal(t, "oidc", got.Provider)
	assert.Equal(t, "oauth", got.AuthMethod)
	assert.True(t, s.AuthenticatedAt.Equal(got.AuthenticatedAt))
	assert.Equal(t, "enc-at", got.AccessTokenEnc)
	assert.Equal(t, "enc-rt", got.RefreshTokenEnc)
	assert.True(t, s.AccessExpiresAt.Equal(got.AccessExpiresAt))
	assert.True(t, s.ExpiresAt.Equal(got.ExpiresAt))
	assert.Nil(t, got.LastRefreshedAt)
	assert.False(t, got.CreatedAt.IsZero())

	// A refresh rotates the tokens and stamps last_refreshed_at.
	newAccessExpiry := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Microsecond)
	require.NoError(t, repo.UpdateTokens(ctx, &models.UserSession{
		IDHash: s.IDHash, AccessTokenEnc: "enc-at-2", RefreshTokenEnc: "enc-rt-2",
		AccessExpiresAt: newAccessExpiry, ExpiresAt: s.ExpiresAt.Add(time.Hour),
	}))
	got, err = repo.GetByIDHash(ctx, s.IDHash)
	require.NoError(t, err)
	assert.Equal(t, "enc-at-2", got.AccessTokenEnc)
	assert.Equal(t, "enc-rt-2", got.RefreshTokenEnc)
	assert.True(t, newAccessExpiry.Equal(got.AccessExpiresAt))
	assert.True(t, s.ExpiresAt.Add(time.Hour).Equal(got.ExpiresAt))
	require.NotNil(t, got.LastRefreshedAt)

	// A sliding extension moves both expiries.
	slid := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Microsecond)
	require.NoError(t, repo.UpdateExpiry(ctx, s.IDHash, slid, slid))
	got, err = repo.GetByIDHash(ctx, s.IDHash)
	require.NoError(t, err)
	assert.True(t, slid.Equal(got.AccessExpiresAt))
	assert.True(t, slid.Equal(got.ExpiresAt))

	// Sign-out deletes it; deleting it again is not an error.
	require.NoError(t, repo.Delete(ctx, s.IDHash))
	_, err = repo.GetByIDHash(ctx, s.IDHash)
	require.ErrorIs(t, err, repositories.ErrUserSessionNotFound)
	require.NoError(t, repo.Delete(ctx, s.IDHash))
	require.ErrorIs(t, repo.UpdateTokens(ctx, &models.UserSession{IDHash: s.IDHash}), repositories.ErrUserSessionNotFound)
	require.ErrorIs(t, repo.UpdateExpiry(ctx, s.IDHash, slid, slid), repositories.ErrUserSessionNotFound)
}

func TestIntegrationUserSession_EmptyTokensAndSignInTimeAreNull(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	repo := NewUserSessionRepository(integrationDB)
	s := newIntegrationSession(insertTestUser(t), time.Hour)
	s.Provider, s.AccessTokenEnc, s.RefreshTokenEnc, s.AuthenticatedAt = "", "", "", time.Time{}

	require.NoError(t, repo.Create(ctx, s))

	var provider, access, refresh sql.NullString
	var authenticatedAt sql.NullTime
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT provider, access_token_enc, refresh_token_enc, authenticated_at FROM sessions WHERE id_hash = $1`,
		s.IDHash).Scan(&provider, &access, &refresh, &authenticatedAt))
	assert.False(t, provider.Valid)
	assert.False(t, access.Valid)
	assert.False(t, refresh.Valid)
	assert.False(t, authenticatedAt.Valid)

	got, err := repo.GetByIDHash(ctx, s.IDHash)
	require.NoError(t, err)
	assert.Empty(t, got.RefreshTokenEnc)
	assert.True(t, got.AuthenticatedAt.IsZero())
}

func TestIntegrationUserSession_ExpiredRowsDoNotAuthenticateAndArePurged(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	repo := NewUserSessionRepository(integrationDB)
	userID := insertTestUser(t)
	live := newIntegrationSession(userID, time.Hour)
	expired := newIntegrationSession(userID, -time.Minute)
	require.NoError(t, repo.Create(ctx, live))
	require.NoError(t, repo.Create(ctx, expired))

	_, err := repo.GetByIDHash(ctx, expired.IDHash)
	require.ErrorIs(t, err, repositories.ErrUserSessionNotFound, "an expired row no longer authenticates")
	require.ErrorIs(t, repo.UpdateTokens(ctx, &models.UserSession{IDHash: expired.IDHash}),
		repositories.ErrUserSessionNotFound, "nor can it be refreshed back to life")

	removed, err := repo.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), removed)
	_, err = repo.GetByIDHash(ctx, live.IDHash)
	require.NoError(t, err, "the purge leaves live rows alone")

	// Idempotent: a second replica running the same purge removes nothing.
	removed, err = repo.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Zero(t, removed)
}

func TestIntegrationUserSession_RowsGoWithTheUser(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	repo := NewUserSessionRepository(integrationDB)
	userID := insertTestUser(t)
	s := newIntegrationSession(userID, time.Hour)
	require.NoError(t, repo.Create(ctx, s))

	_, err := integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	require.NoError(t, err)

	_, err = repo.GetByIDHash(ctx, s.IDHash)
	require.ErrorIs(t, err, repositories.ErrUserSessionNotFound)
}

func TestIntegrationUserSession_StoreKeepsTokensEncrypted(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	enc, err := services.NewEncryptionService("0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	store := services.NewSessionStore(NewUserSessionRepository(integrationDB), enc)
	userID := insertTestUser(t)
	sess := &session.Session{
		UserID: userID, Provider: "oidc", AuthMethod: session.AuthMethodOAuth, AuthenticatedAt: time.Now(),
		AccessToken: "plain-access-token", RefreshToken: "plain-refresh-token", ExpiresAt: time.Now().Add(time.Hour),
	}

	require.NoError(t, store.Create(ctx, sess))

	var access, refresh string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT access_token_enc, refresh_token_enc FROM sessions WHERE user_id = $1`, userID).Scan(&access, &refresh))
	assert.NotContains(t, access, "plain-access-token")
	assert.NotContains(t, refresh, "plain-refresh-token")

	loaded := &session.Session{ID: sess.ID, UserID: userID}
	require.NoError(t, store.Load(ctx, loaded))
	assert.Equal(t, "plain-access-token", loaded.AccessToken)
	assert.Equal(t, "plain-refresh-token", loaded.RefreshToken)

	require.NoError(t, store.Delete(ctx, sess.ID))
	require.ErrorIs(t, store.Load(ctx, loaded), services.ErrSessionNotFound)
}

// TestIntegrationUserSession_RefreshLockedSerializesOnTheRow proves the row
// lock holds across connections (#1647): while one caller's decision runs,
// a second caller on the same session waits, and then decides on the row the
// first one stored.
func TestIntegrationUserSession_RefreshLockedSerializesOnTheRow(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	repo := NewUserSessionRepository(integrationDB)
	s := newIntegrationSession(insertTestUser(t), 24*time.Hour)
	require.NoError(t, repo.Create(ctx, s))

	firstHolds := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- repo.RefreshLocked(ctx, s.IDHash,
			func(row *models.UserSession) (repositories.UserSessionRefresh, error) {
				close(firstHolds)
				<-release
				return repositories.UserSessionRefresh{Update: &models.UserSession{
					AccessTokenEnc: "enc-at-2", RefreshTokenEnc: "enc-rt-2",
					AccessExpiresAt: row.AccessExpiresAt, ExpiresAt: row.ExpiresAt,
				}}, nil
			})
	}()
	<-firstHolds

	secondSaw := make(chan string, 1)
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- repo.RefreshLocked(ctx, s.IDHash,
			func(row *models.UserSession) (repositories.UserSessionRefresh, error) {
				secondSaw <- row.RefreshTokenEnc
				return repositories.UserSessionRefresh{}, nil
			})
	}()

	select {
	case got := <-secondSaw:
		t.Fatalf("the second caller read the row (%q) while the first held its lock", got)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)

	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)
	assert.Equal(t, "enc-rt-2", <-secondSaw, "the second caller decided on the refreshed row")
	got, err := repo.GetByIDHash(ctx, s.IDHash)
	require.NoError(t, err)
	assert.Equal(t, "enc-rt-2", got.RefreshTokenEnc)
	require.NotNil(t, got.LastRefreshedAt)
}

func TestIntegrationUserSession_RefreshLockedDecisions(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	repo := NewUserSessionRepository(integrationDB)
	userID := insertTestUser(t)

	t.Run("a delete decision removes the row and returns the decision's error", func(t *testing.T) {
		s := newIntegrationSession(userID, 24*time.Hour)
		require.NoError(t, repo.Create(ctx, s))
		refused := errors.New("invalid_grant")

		err := repo.RefreshLocked(ctx, s.IDHash, func(*models.UserSession) (repositories.UserSessionRefresh, error) {
			return repositories.UserSessionRefresh{Delete: true}, refused
		})

		require.ErrorIs(t, err, refused)
		_, err = repo.GetByIDHash(ctx, s.IDHash)
		require.ErrorIs(t, err, repositories.ErrUserSessionNotFound)
	})

	t.Run("an error with no decision changes nothing", func(t *testing.T) {
		s := newIntegrationSession(userID, 24*time.Hour)
		require.NoError(t, repo.Create(ctx, s))
		outage := errors.New("issuer unreachable")

		err := repo.RefreshLocked(ctx, s.IDHash, func(*models.UserSession) (repositories.UserSessionRefresh, error) {
			return repositories.UserSessionRefresh{}, outage
		})

		require.ErrorIs(t, err, outage)
		got, err := repo.GetByIDHash(ctx, s.IDHash)
		require.NoError(t, err)
		assert.Equal(t, "enc-rt", got.RefreshTokenEnc)
		assert.Nil(t, got.LastRefreshedAt)
	})

	t.Run("an expired or missing row is ErrUserSessionNotFound", func(t *testing.T) {
		expired := newIntegrationSession(userID, -time.Minute)
		require.NoError(t, repo.Create(ctx, expired))
		called := false
		decide := func(*models.UserSession) (repositories.UserSessionRefresh, error) {
			called = true
			return repositories.UserSessionRefresh{}, nil
		}

		require.ErrorIs(t, repo.RefreshLocked(ctx, expired.IDHash, decide), repositories.ErrUserSessionNotFound)
		require.ErrorIs(t, repo.RefreshLocked(ctx, sessionHash("missing"), decide), repositories.ErrUserSessionNotFound)
		assert.False(t, called)
	})
}

// DeleteOthersForUser ends a user's other sessions (#1649): every row of the
// user but the kept one goes, expired ones included, and each removed row's
// provider and refresh token come back for revocation. Another user's rows
// are untouched.
func TestIntegrationUserSession_DeleteOthersForUser(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	repo := NewUserSessionRepository(integrationDB)
	userID, otherUserID := insertTestUser(t), insertTestUser(t)

	kept := newIntegrationSession(userID, 24*time.Hour)
	other := newIntegrationSession(userID, 24*time.Hour)
	other.Provider, other.RefreshTokenEnc = "corp-sso", "enc-rt-other"
	password := newIntegrationSession(userID, 24*time.Hour)
	password.Provider, password.AccessTokenEnc, password.RefreshTokenEnc = "", "", ""
	expired := newIntegrationSession(userID, -time.Minute)
	expired.RefreshTokenEnc = "enc-rt-expired"
	foreign := newIntegrationSession(otherUserID, 24*time.Hour)
	for _, s := range []*models.UserSession{kept, other, password, expired, foreign} {
		require.NoError(t, repo.Create(ctx, s))
	}

	removed, err := repo.DeleteOthersForUser(ctx, userID, kept.IDHash)
	require.NoError(t, err)

	got := map[string]string{}
	for _, s := range removed {
		assert.Equal(t, userID, s.UserID)
		got[s.RefreshTokenEnc] = s.Provider
	}
	assert.Equal(t, map[string]string{"enc-rt-other": "corp-sso", "": "", "enc-rt-expired": "oidc"}, got)
	_, err = repo.GetByIDHash(ctx, kept.IDHash)
	require.NoError(t, err, "the kept session stays")
	_, err = repo.GetByIDHash(ctx, foreign.IDHash)
	require.NoError(t, err, "another user's session stays")
	_, err = repo.GetByIDHash(ctx, other.IDHash)
	require.ErrorIs(t, err, repositories.ErrUserSessionNotFound)

	// No session to keep: every row of the user goes.
	removed, err = repo.DeleteOthersForUser(ctx, userID, nil)
	require.NoError(t, err)
	assert.Len(t, removed, 1)
	_, err = repo.GetByIDHash(ctx, kept.IDHash)
	require.ErrorIs(t, err, repositories.ErrUserSessionNotFound)
	_, err = repo.GetByIDHash(ctx, foreign.IDHash)
	require.NoError(t, err)
}

// DeleteReturning removes one row, expired or not, and returns what it held
// (#1649); a missing row is nil.
func TestIntegrationUserSession_DeleteReturning(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	repo := NewUserSessionRepository(integrationDB)
	userID := insertTestUser(t)
	live := newIntegrationSession(userID, 24*time.Hour)
	expired := newIntegrationSession(userID, -time.Minute)
	expired.Provider, expired.RefreshTokenEnc = "", ""
	require.NoError(t, repo.Create(ctx, live))
	require.NoError(t, repo.Create(ctx, expired))

	got, err := repo.DeleteReturning(ctx, live.IDHash)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, userID, got.UserID)
	assert.Equal(t, "oidc", got.Provider)
	assert.Equal(t, "enc-rt", got.RefreshTokenEnc)
	_, err = repo.GetByIDHash(ctx, live.IDHash)
	require.ErrorIs(t, err, repositories.ErrUserSessionNotFound)

	got, err = repo.DeleteReturning(ctx, expired.IDHash)
	require.NoError(t, err)
	require.NotNil(t, got, "an expired row is removed too")
	assert.Empty(t, got.Provider)
	assert.Empty(t, got.RefreshTokenEnc)

	got, err = repo.DeleteReturning(ctx, live.IDHash)
	require.NoError(t, err)
	assert.Nil(t, got, "no row left")
}
