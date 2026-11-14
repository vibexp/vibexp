package server

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sesslib "github.com/vibexp/vibexp/internal/auth/session"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

// testSessionEncryptionKey is the 32-byte key of the test session stores.
// #nosec G101 - test key, not a real secret
const testSessionEncryptionKey = "kkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkk"

// memSessionRepo is an in-memory repositories.UserSessionRepository with the
// postgres repository's semantics: lookups and updates ignore expired rows.
type memSessionRepo struct {
	mu   sync.Mutex
	rows map[string]models.UserSession
	// rowLocks are the per-row locks RefreshLocked holds, standing in for
	// SELECT ... FOR UPDATE.
	rowLocks map[string]*sync.Mutex
	// failWith, when set, is returned by every method.
	failWith error
}

func newMemSessionRepo() *memSessionRepo {
	return &memSessionRepo{rows: map[string]models.UserSession{}, rowLocks: map[string]*sync.Mutex{}}
}

// newMemSessionStore returns a real SessionStore over a fresh memSessionRepo.
func newMemSessionStore() (*services.SessionStore, *memSessionRepo) {
	enc, err := services.NewEncryptionService(testSessionEncryptionKey)
	if err != nil {
		panic(err)
	}
	repo := newMemSessionRepo()
	return services.NewSessionStore(repo, enc), repo
}

func (m *memSessionRepo) live(idHash []byte) (models.UserSession, bool) {
	row, ok := m.rows[string(idHash)]
	if !ok || !row.ExpiresAt.After(time.Now()) {
		return models.UserSession{}, false
	}
	return row, true
}

func (m *memSessionRepo) Create(_ context.Context, s *models.UserSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWith != nil {
		return m.failWith
	}
	row := *s
	row.CreatedAt = time.Now()
	m.rows[string(s.IDHash)] = row
	return nil
}

func (m *memSessionRepo) GetByIDHash(_ context.Context, idHash []byte) (*models.UserSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWith != nil {
		return nil, m.failWith
	}
	row, ok := m.live(idHash)
	if !ok {
		return nil, repositories.ErrUserSessionNotFound
	}
	return &row, nil
}

func (m *memSessionRepo) UpdateTokens(_ context.Context, s *models.UserSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWith != nil {
		return m.failWith
	}
	row, ok := m.live(s.IDHash)
	if !ok {
		return repositories.ErrUserSessionNotFound
	}
	now := time.Now()
	row.AccessTokenEnc, row.RefreshTokenEnc = s.AccessTokenEnc, s.RefreshTokenEnc
	row.AccessExpiresAt, row.ExpiresAt, row.LastRefreshedAt = s.AccessExpiresAt, s.ExpiresAt, &now
	m.rows[string(s.IDHash)] = row
	return nil
}

func (m *memSessionRepo) UpdateExpiry(_ context.Context, idHash []byte, accessExpiresAt, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWith != nil {
		return m.failWith
	}
	row, ok := m.live(idHash)
	if !ok {
		return repositories.ErrUserSessionNotFound
	}
	row.AccessExpiresAt, row.ExpiresAt = accessExpiresAt, expiresAt
	m.rows[string(idHash)] = row
	return nil
}

func (m *memSessionRepo) Delete(_ context.Context, idHash []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWith != nil {
		return m.failWith
	}
	delete(m.rows, string(idHash))
	return nil
}

func (m *memSessionRepo) DeleteExpired(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWith != nil {
		return 0, m.failWith
	}
	var n int64
	for k, row := range m.rows {
		if !row.ExpiresAt.After(time.Now()) {
			delete(m.rows, k)
			n++
		}
	}
	return n, nil
}

// RefreshLocked holds idHash's row lock while decide runs, like the postgres
// repository's SELECT ... FOR UPDATE, then applies the decision.
func (m *memSessionRepo) RefreshLocked(
	_ context.Context, idHash []byte, decide repositories.UserSessionRefreshFunc,
) error {
	m.mu.Lock()
	if m.failWith != nil {
		m.mu.Unlock()
		return m.failWith
	}
	lock, ok := m.rowLocks[string(idHash)]
	if !ok {
		lock = &sync.Mutex{}
		m.rowLocks[string(idHash)] = lock
	}
	m.mu.Unlock()

	lock.Lock()
	defer lock.Unlock()

	m.mu.Lock()
	row, ok := m.live(idHash)
	m.mu.Unlock()
	if !ok {
		return repositories.ErrUserSessionNotFound
	}
	decision, err := decide(&row)
	if err != nil && !decision.Delete && decision.Update == nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case decision.Delete:
		delete(m.rows, string(idHash))
	case decision.Update != nil:
		now := time.Now()
		u := decision.Update
		row.AccessTokenEnc, row.RefreshTokenEnc = u.AccessTokenEnc, u.RefreshTokenEnc
		row.AccessExpiresAt, row.ExpiresAt, row.LastRefreshedAt = u.AccessExpiresAt, u.ExpiresAt, &now
		m.rows[string(idHash)] = row
	}
	return err
}

// count returns the number of stored rows, expired ones included.
func (m *memSessionRepo) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.rows)
}

// only returns the single stored row; it panics unless exactly one exists.
func (m *memSessionRepo) only() models.UserSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.rows) != 1 {
		panic("memSessionRepo: expected exactly one row")
	}
	for _, row := range m.rows {
		return row
	}
	return models.UserSession{}
}

// backdateRefresh moves every row's creation and last refresh d into the past.
func (m *memSessionRepo) backdateRefresh(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, row := range m.rows {
		row.CreatedAt = row.CreatedAt.Add(-d)
		if row.LastRefreshedAt != nil {
			t := row.LastRefreshedAt.Add(-d)
			row.LastRefreshedAt = &t
		}
		m.rows[k] = row
	}
}

// setFail makes every later call return err (nil restores normal behaviour).
func (m *memSessionRepo) setFail(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failWith = err
}

// sharedTestSessionStore is the session store every BaseMockContainer
// returns. Session ids are random, so tests sharing it never see each other's
// rows; a test that inspects rows installs its own with newMemSessionStore.
var sharedTestSessionStore, _ = newMemSessionStore()

// storedSessionCookie stores sess through srv's session store, as a sign-in
// does, and returns the cookie that names it.
func storedSessionCookie(t *testing.T, srv *Server, sess *sesslib.Session) *http.Cookie {
	t.Helper()
	c, err := srv.newSessionCookie(context.Background(), sess)
	require.NoError(t, err)
	return c
}

// legacySessionCookie returns a vx_session cookie in the format minted before
// #1646: no session id, the tokens in the payload itself. It seals payload the
// way session.Manager does (AES-256-GCM, hex(nonce) + "." + hex(ciphertext))
// under the test servers' cookie key.
func legacySessionCookie(t *testing.T, payload map[string]any) *http.Cookie {
	t.Helper()
	plaintext, err := json.Marshal(payload)
	require.NoError(t, err)
	key, err := hex.DecodeString(testCookiePassword)
	require.NoError(t, err)
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	nonce := make([]byte, gcm.NonceSize())
	_, err = rand.Read(nonce)
	require.NoError(t, err)
	value := hex.EncodeToString(nonce) + "." + hex.EncodeToString(gcm.Seal(nil, nonce, plaintext, nil))
	return &http.Cookie{Name: sesslib.CookieName, Value: value}
}
