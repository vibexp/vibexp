package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ErrSessionNotFound is returned by SessionStore.Load when the session's row is
// missing, past its expiry, belongs to another user or holds tokens that no
// longer decrypt (security.encryption_key was rotated). Every one of those is a
// session that no longer authenticates: the caller answers 401 and clears the
// cookie. Any other error is an infrastructure failure.
var ErrSessionNotFound = errors.New("session not found")

// sessionIDBytes is the entropy of a session id: 32 random bytes.
const sessionIDBytes = 32

// SessionStoreInterface keeps the server-side rows of cookie sessions (#1646).
// A cookie carries only the session id; the identity-provider tokens are
// stored here, encrypted.
type SessionStoreInterface interface {
	// Create stores sess as a new session and sets sess.ID.
	Create(ctx context.Context, sess *session.Session) error
	// Load fills sess's tokens and access-token expiry from its row, or returns
	// ErrSessionNotFound.
	Load(ctx context.Context, sess *session.Session) error
	// SaveTokens stores sess's (rotated) tokens and extends the row's lifetime.
	SaveTokens(ctx context.Context, sess *session.Session) error
	// Slide stores a sliding session's new expiry.
	Slide(ctx context.Context, sess *session.Session) error
	// Delete removes the session with id sid. A missing row is not an error.
	Delete(ctx context.Context, sid string) error
	// Sweep deletes expired rows and returns how many it removed.
	Sweep(ctx context.Context) (int64, error)
}

// SessionStore implements SessionStoreInterface over a UserSessionRepository,
// encrypting the tokens with the instance's EncryptionService.
type SessionStore struct {
	repo repositories.UserSessionRepository
	enc  EncryptionServiceInterface
	now  func() time.Time
}

// NewSessionStore returns a SessionStore.
func NewSessionStore(repo repositories.UserSessionRepository, enc EncryptionServiceInterface) *SessionStore {
	return &SessionStore{repo: repo, enc: enc, now: time.Now}
}

// sessionIDHash is the stored form of a session id.
func sessionIDHash(sid string) []byte {
	sum := sha256.Sum256([]byte(sid))
	return sum[:]
}

// Create generates a session id, stores sess under it and sets sess.ID.
func (s *SessionStore) Create(ctx context.Context, sess *session.Session) error {
	raw := make([]byte, sessionIDBytes)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("failed to generate session id: %w", err)
	}
	sid := base64.RawURLEncoding.EncodeToString(raw)

	access, refresh, err := s.encryptTokens(sess)
	if err != nil {
		return err
	}
	row := &models.UserSession{
		IDHash:          sessionIDHash(sid),
		UserID:          sess.UserID,
		Provider:        sess.Provider,
		AuthMethod:      sess.AuthMethod,
		AuthenticatedAt: sess.AuthenticatedAt,
		AccessTokenEnc:  access,
		RefreshTokenEnc: refresh,
		AccessExpiresAt: sess.ExpiresAt,
		ExpiresAt:       sess.EndsAt(s.now()),
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return err
	}
	sess.ID = sid
	return nil
}

// Load reads sess's row and fills its tokens and access-token expiry. The row,
// not the cookie, is authoritative for both: another request may have
// refreshed the tokens since this cookie was written.
func (s *SessionStore) Load(ctx context.Context, sess *session.Session) error {
	if sess.ID == "" {
		return fmt.Errorf("%w: the session has no id", ErrSessionNotFound)
	}
	row, err := s.repo.GetByIDHash(ctx, sessionIDHash(sess.ID))
	if err != nil {
		if errors.Is(err, repositories.ErrUserSessionNotFound) {
			return fmt.Errorf("%w: no unexpired row", ErrSessionNotFound)
		}
		return err
	}
	if row.UserID != sess.UserID {
		return fmt.Errorf("%w: the row belongs to another user", ErrSessionNotFound)
	}
	access, err := s.decrypt(row.AccessTokenEnc)
	if err != nil {
		return fmt.Errorf("%w: access token: %w", ErrSessionNotFound, err)
	}
	refresh, err := s.decrypt(row.RefreshTokenEnc)
	if err != nil {
		return fmt.Errorf("%w: refresh token: %w", ErrSessionNotFound, err)
	}
	sess.AccessToken = access
	sess.RefreshToken = refresh
	sess.ExpiresAt = row.AccessExpiresAt
	return nil
}

// SaveTokens stores sess's tokens and access-token expiry, and extends the
// row's lifetime as writing the cookie extends the cookie's.
func (s *SessionStore) SaveTokens(ctx context.Context, sess *session.Session) error {
	access, refresh, err := s.encryptTokens(sess)
	if err != nil {
		return err
	}
	return s.repo.UpdateTokens(ctx, &models.UserSession{
		IDHash:          sessionIDHash(sess.ID),
		AccessTokenEnc:  access,
		RefreshTokenEnc: refresh,
		AccessExpiresAt: sess.ExpiresAt,
		ExpiresAt:       sess.EndsAt(s.now()),
	})
}

// Slide stores the expiry a sliding session was just extended to.
func (s *SessionStore) Slide(ctx context.Context, sess *session.Session) error {
	return s.repo.UpdateExpiry(ctx, sessionIDHash(sess.ID), sess.ExpiresAt, sess.EndsAt(s.now()))
}

// Delete removes the session with id sid.
func (s *SessionStore) Delete(ctx context.Context, sid string) error {
	return s.repo.Delete(ctx, sessionIDHash(sid))
}

// Sweep deletes the rows past their expiry.
func (s *SessionStore) Sweep(ctx context.Context) (int64, error) {
	return s.repo.DeleteExpired(ctx)
}

// encryptTokens encrypts sess's tokens; an empty token stays empty.
func (s *SessionStore) encryptTokens(sess *session.Session) (access, refresh string, err error) {
	if access, err = s.encrypt(sess.AccessToken); err != nil {
		return "", "", fmt.Errorf("failed to encrypt the session's access token: %w", err)
	}
	if refresh, err = s.encrypt(sess.RefreshToken); err != nil {
		return "", "", fmt.Errorf("failed to encrypt the session's refresh token: %w", err)
	}
	return access, refresh, nil
}

func (s *SessionStore) encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	return s.enc.Encrypt(plaintext)
}

func (s *SessionStore) decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	return s.enc.Decrypt(ciphertext)
}
