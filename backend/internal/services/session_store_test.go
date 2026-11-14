package services

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// sessionStoreTestKey is the 32-byte encryption key of these tests.
// #nosec G101 - test key, not a real secret
const sessionStoreTestKey = "ssssssssssssssssssssssssssssssss"

var sessionStoreTestNow = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

func newTestSessionStore(t *testing.T) (*SessionStore, *repomocks.MockUserSessionRepository, *EncryptionService) {
	t.Helper()
	enc, err := NewEncryptionService(sessionStoreTestKey)
	require.NoError(t, err)
	repo := repomocks.NewMockUserSessionRepository(t)
	store := NewSessionStore(repo, enc)
	store.now = func() time.Time { return sessionStoreTestNow }
	return store, repo, enc
}

func hashOf(sid string) []byte {
	sum := sha256.Sum256([]byte(sid))
	return sum[:]
}

func TestSessionStore_Create(t *testing.T) {
	t.Run("stores the tokens encrypted under the hash of a fresh random id", func(t *testing.T) {
		store, repo, enc := newTestSessionStore(t)
		var stored *models.UserSession
		repo.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { stored = args.Get(1).(*models.UserSession) }).Return(nil)
		accessExpiry := sessionStoreTestNow.Add(time.Hour)
		sess := &session.Session{
			UserID: "u-1", Provider: "oidc", AuthMethod: session.AuthMethodOAuth,
			AuthenticatedAt: sessionStoreTestNow, AccessToken: "at", RefreshToken: "rt", ExpiresAt: accessExpiry,
		}

		require.NoError(t, store.Create(context.Background(), sess))

		raw, err := base64.RawURLEncoding.DecodeString(sess.ID)
		require.NoError(t, err, "the id is unpadded base64url")
		assert.Len(t, raw, sessionIDBytes)
		require.NotNil(t, stored)
		assert.Equal(t, hashOf(sess.ID), stored.IDHash, "only the hash of the id is stored")
		assert.Equal(t, "u-1", stored.UserID)
		assert.Equal(t, "oidc", stored.Provider)
		assert.Equal(t, session.AuthMethodOAuth, stored.AuthMethod)
		assert.Equal(t, sessionStoreTestNow, stored.AuthenticatedAt)
		assert.Equal(t, accessExpiry, stored.AccessExpiresAt)
		assert.Equal(t, sessionStoreTestNow.Add(24*time.Hour), stored.ExpiresAt, "a refreshable session lives as long as its cookie")
		assert.NotEqual(t, "at", stored.AccessTokenEnc)
		assert.NotEqual(t, "rt", stored.RefreshTokenEnc)
		got, err := enc.Decrypt(stored.AccessTokenEnc)
		require.NoError(t, err)
		assert.Equal(t, "at", got)
		got, err = enc.Decrypt(stored.RefreshTokenEnc)
		require.NoError(t, err)
		assert.Equal(t, "rt", got)
	})

	t.Run("a sliding session stores no refresh token and ends at its own expiry", func(t *testing.T) {
		store, repo, _ := newTestSessionStore(t)
		var stored *models.UserSession
		repo.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { stored = args.Get(1).(*models.UserSession) }).Return(nil)
		sess := &session.Session{UserID: "u-1", AuthMethod: session.AuthMethodPassword}
		sess.StartSliding(sessionStoreTestNow)

		require.NoError(t, store.Create(context.Background(), sess))

		assert.Empty(t, stored.AccessTokenEnc)
		assert.Empty(t, stored.RefreshTokenEnc)
		assert.Equal(t, sess.ExpiresAt, stored.ExpiresAt)
	})

	t.Run("two sessions get different ids", func(t *testing.T) {
		store, repo, _ := newTestSessionStore(t)
		repo.On("Create", mock.Anything, mock.Anything).Return(nil)
		a, b := &session.Session{UserID: "u-1"}, &session.Session{UserID: "u-1"}
		require.NoError(t, store.Create(context.Background(), a))
		require.NoError(t, store.Create(context.Background(), b))
		assert.NotEqual(t, a.ID, b.ID)
	})

	t.Run("a failed insert leaves the session without an id", func(t *testing.T) {
		store, repo, _ := newTestSessionStore(t)
		repo.On("Create", mock.Anything, mock.Anything).Return(errors.New("db down"))
		sess := &session.Session{UserID: "u-1"}

		require.Error(t, store.Create(context.Background(), sess))
		assert.Empty(t, sess.ID)
	})
}

// storedRow returns the row Create would store for sess, with its tokens
// encrypted by enc.
func storedRow(t *testing.T, enc *EncryptionService, userID, access, refresh string, accessExpiry time.Time) *models.UserSession {
	t.Helper()
	row := &models.UserSession{UserID: userID, AccessExpiresAt: accessExpiry}
	var err error
	if access != "" {
		row.AccessTokenEnc, err = enc.Encrypt(access)
		require.NoError(t, err)
	}
	if refresh != "" {
		row.RefreshTokenEnc, err = enc.Encrypt(refresh)
		require.NoError(t, err)
	}
	return row
}

func TestSessionStore_Load(t *testing.T) {
	accessExpiry := sessionStoreTestNow.Add(time.Hour)

	t.Run("fills the tokens and the access-token expiry from the row", func(t *testing.T) {
		store, repo, enc := newTestSessionStore(t)
		repo.On("GetByIDHash", mock.Anything, hashOf("sid-1")).
			Return(storedRow(t, enc, "u-1", "at", "rt", accessExpiry), nil)
		sess := &session.Session{ID: "sid-1", UserID: "u-1", ExpiresAt: sessionStoreTestNow.Add(-time.Hour)}

		require.NoError(t, store.Load(context.Background(), sess))

		assert.Equal(t, "at", sess.AccessToken)
		assert.Equal(t, "rt", sess.RefreshToken)
		assert.Equal(t, accessExpiry, sess.ExpiresAt, "the row, not the cookie, says when the token expires")
	})

	t.Run("a session without tokens loads empty ones", func(t *testing.T) {
		store, repo, enc := newTestSessionStore(t)
		repo.On("GetByIDHash", mock.Anything, hashOf("sid-1")).
			Return(storedRow(t, enc, "u-1", "", "", accessExpiry), nil)
		sess := &session.Session{ID: "sid-1", UserID: "u-1"}

		require.NoError(t, store.Load(context.Background(), sess))
		assert.Empty(t, sess.AccessToken)
		assert.True(t, sess.IsSliding())
	})

	notFound := []struct {
		name string
		row  func(enc *EncryptionService) (*models.UserSession, error)
	}{
		{"a missing or expired row", func(*EncryptionService) (*models.UserSession, error) {
			return nil, repositories.ErrUserSessionNotFound
		}},
		{"a row of another user", func(enc *EncryptionService) (*models.UserSession, error) {
			return storedRow(t, enc, "u-2", "at", "", accessExpiry), nil
		}},
		{"tokens that no longer decrypt", func(*EncryptionService) (*models.UserSession, error) {
			return &models.UserSession{UserID: "u-1", AccessTokenEnc: "garbage"}, nil
		}},
		{"a refresh token that no longer decrypts", func(enc *EncryptionService) (*models.UserSession, error) {
			row := storedRow(t, enc, "u-1", "at", "", accessExpiry)
			row.RefreshTokenEnc = "not base64 at all"
			return row, nil
		}},
	}
	for _, tc := range notFound {
		t.Run(tc.name+" is ErrSessionNotFound", func(t *testing.T) {
			store, repo, enc := newTestSessionStore(t)
			row, err := tc.row(enc)
			repo.On("GetByIDHash", mock.Anything, hashOf("sid-1")).Return(row, err)
			sess := &session.Session{ID: "sid-1", UserID: "u-1"}

			err = store.Load(context.Background(), sess)

			require.ErrorIs(t, err, ErrSessionNotFound)
			assert.Empty(t, sess.AccessToken)
		})
	}

	t.Run("a session without an id is ErrSessionNotFound", func(t *testing.T) {
		store, _, _ := newTestSessionStore(t)
		require.ErrorIs(t, store.Load(context.Background(), &session.Session{UserID: "u-1"}), ErrSessionNotFound)
	})

	t.Run("a failed read is not ErrSessionNotFound", func(t *testing.T) {
		store, repo, _ := newTestSessionStore(t)
		repo.On("GetByIDHash", mock.Anything, hashOf("sid-1")).Return(nil, errors.New("db down"))

		err := store.Load(context.Background(), &session.Session{ID: "sid-1", UserID: "u-1"})

		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrSessionNotFound, "a database outage must not read as a signed-out session")
	})
}

func TestSessionStore_SaveTokens(t *testing.T) {
	store, repo, enc := newTestSessionStore(t)
	var saved *models.UserSession
	repo.On("UpdateTokens", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { saved = args.Get(1).(*models.UserSession) }).Return(nil)
	newExpiry := sessionStoreTestNow.Add(time.Hour)
	sess := &session.Session{ID: "sid-1", UserID: "u-1", AccessToken: "at2", RefreshToken: "rt2", ExpiresAt: newExpiry}

	require.NoError(t, store.SaveTokens(context.Background(), sess))

	require.NotNil(t, saved)
	assert.Equal(t, hashOf("sid-1"), saved.IDHash)
	assert.Equal(t, newExpiry, saved.AccessExpiresAt)
	assert.Equal(t, sessionStoreTestNow.Add(24*time.Hour), saved.ExpiresAt, "a refresh extends the row as it does the cookie")
	got, err := enc.Decrypt(saved.RefreshTokenEnc)
	require.NoError(t, err)
	assert.Equal(t, "rt2", got)
	got, err = enc.Decrypt(saved.AccessTokenEnc)
	require.NoError(t, err)
	assert.Equal(t, "at2", got)
}

func TestSessionStore_SlideDeleteSweep(t *testing.T) {
	store, repo, _ := newTestSessionStore(t)
	sess := &session.Session{ID: "sid-1", UserID: "u-1"}
	sess.StartSliding(sessionStoreTestNow)
	repo.On("UpdateExpiry", mock.Anything, hashOf("sid-1"), sess.ExpiresAt, sess.ExpiresAt).Return(nil).Once()
	repo.On("Delete", mock.Anything, hashOf("sid-1")).Return(nil).Once()
	repo.On("DeleteExpired", mock.Anything).Return(int64(3), nil).Once()

	require.NoError(t, store.Slide(context.Background(), sess))
	require.NoError(t, store.Delete(context.Background(), "sid-1"))
	n, err := store.Sweep(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
}
