package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/auth/idp"
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

// ErrSessionRefreshRejected marks a refresh failure that ends the session: the
// identity provider refused the refresh token (invalid_grant) or answered for
// another subject. A SessionStore.RefreshIfDue refresh function wraps its error
// in it to have the row deleted.
var ErrSessionRefreshRejected = errors.New("session refresh rejected")

// SessionRefreshFunc refreshes a session's tokens at its identity provider. It
// runs while the session's row is locked.
type SessionRefreshFunc func(ctx context.Context, sess *session.Session) (*idp.Tokens, error)

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
	// RefreshIfDue refreshes sess under a lock on its row (#1647). With the
	// lock held it re-reads the row and asks due whether a refresh is still
	// needed: another request or replica may have refreshed the session while
	// this one waited. If not, sess takes the row's tokens and nothing is
	// called. Otherwise refresh runs and the tokens it returns are stored,
	// with last_refreshed_at, before the lock is released, so a rotated
	// refresh token is never presented twice. A refresh error wrapping
	// ErrSessionRefreshRejected deletes the row; any other error leaves it.
	// It reports whether refresh ran and succeeded, and returns
	// ErrSessionNotFound when the row is gone.
	RefreshIfDue(
		ctx context.Context, sess *session.Session, due func(*session.Session) bool, refresh SessionRefreshFunc,
	) (bool, error)
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
	sess.LastRefreshedAt = s.now()
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
		return mapSessionNotFound(err)
	}
	return s.fill(sess, row)
}

// fill sets sess's tokens, access-token expiry and last refresh from row.
func (s *SessionStore) fill(sess *session.Session, row *models.UserSession) error {
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
	sess.LastRefreshedAt = row.CreatedAt
	if row.LastRefreshedAt != nil {
		sess.LastRefreshedAt = *row.LastRefreshedAt
	}
	return nil
}

// mapSessionNotFound maps the repository's missing-row error to
// ErrSessionNotFound and passes any other error through.
func mapSessionNotFound(err error) error {
	if errors.Is(err, repositories.ErrUserSessionNotFound) {
		return fmt.Errorf("%w: no unexpired row", ErrSessionNotFound)
	}
	return err
}

// RefreshIfDue refreshes sess under a lock on its row; see
// SessionStoreInterface.RefreshIfDue.
func (s *SessionStore) RefreshIfDue(
	ctx context.Context, sess *session.Session, due func(*session.Session) bool, refresh SessionRefreshFunc,
) (bool, error) {
	if sess.ID == "" {
		return false, fmt.Errorf("%w: the session has no id", ErrSessionNotFound)
	}
	locked := *sess
	refreshed := false
	err := s.repo.RefreshLocked(ctx, sessionIDHash(sess.ID),
		func(row *models.UserSession) (repositories.UserSessionRefresh, error) {
			if err := s.fill(&locked, row); err != nil {
				return repositories.UserSessionRefresh{}, err
			}
			if !due(&locked) {
				return repositories.UserSessionRefresh{}, nil
			}
			tokens, err := refresh(ctx, &locked)
			if err != nil {
				return repositories.UserSessionRefresh{Delete: errors.Is(err, ErrSessionRefreshRejected)}, err
			}
			update, err := s.applyRefreshedTokens(&locked, tokens)
			if err != nil {
				return repositories.UserSessionRefresh{}, err
			}
			refreshed = true
			return repositories.UserSessionRefresh{Update: update}, nil
		})
	if err != nil {
		return false, mapSessionNotFound(err)
	}
	*sess = locked
	return refreshed, nil
}

// applyRefreshedTokens sets tokens on sess and returns the row update that
// stores them. A response without a refresh token keeps the current one.
func (s *SessionStore) applyRefreshedTokens(sess *session.Session, tokens *idp.Tokens) (*models.UserSession, error) {
	if tokens.RefreshToken != "" {
		sess.RefreshToken = tokens.RefreshToken
	}
	sess.AccessToken = tokens.AccessToken
	sess.ExpiresAt = tokens.ExpiresAt
	now := s.now()
	sess.LastRefreshedAt = now
	access, refresh, err := s.encryptTokens(sess)
	if err != nil {
		return nil, err
	}
	return &models.UserSession{
		AccessTokenEnc:  access,
		RefreshTokenEnc: refresh,
		AccessExpiresAt: sess.ExpiresAt,
		ExpiresAt:       sess.EndsAt(now),
	}, nil
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
