package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/password"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// Email and password sign-in (#1431), driven through the router with the real
// PasswordAuthService over an in-memory credential store, so the whole path is
// exercised: the policy gate, the body, the service, the session cookie and the
// activity record. Every response is asserted against the spec.

const (
	pwLoginPath     = "/api/v1/auth/password/login"
	pwLoginUserID   = "5f0b2d3e-7a41-4c1b-9a55-1c2d3e4f5a6b"
	pwLoginEmail    = "ada@example.com"
	pwLoginPassword = "correct horse battery staple"
	pwLoginWrong    = "definitely not the password"
)

// pwLoginCheapParams keeps the suite fast; a production hash costs 19 MiB.
var pwLoginCheapParams = password.Params{Memory: 64, Time: 1, Threads: 1, SaltLength: 16, KeyLength: 32}

// pwLoginLockout is the production schedule; the fixture's clock is moved
// instead of waiting.
var pwLoginLockout = models.DefaultPasswordLockout

// memCredentials is an in-memory UserCredentialRepository that applies the
// lockout schedule the way the SQL does.
type memCredentials struct {
	mu   sync.Mutex
	rows map[string]*models.UserCredential
}

func (m *memCredentials) Get(_ context.Context, userID string) (*models.UserCredential, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[userID]
	if !ok {
		return nil, repositories.ErrUserCredentialNotFound
	}
	c := *row
	return &c, nil
}

func (m *memCredentials) Upsert(_ context.Context, userID, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[userID] = &models.UserCredential{UserID: userID, PasswordHash: hash}
	return nil
}

func (m *memCredentials) UpdateHash(_ context.Context, userID, currentHash, newHash string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[userID]
	if !ok || row.PasswordHash != currentHash {
		return false, nil
	}
	row.PasswordHash = newHash
	return true, nil
}

// BeginAttempt admits attempts one at a time under the store's mutex, the way
// the row lock and the WHERE clause do in PostgreSQL: a disabled or locked
// credential is refused and not counted; anything else is counted and, from
// the threshold on, locked.
func (m *memCredentials) BeginAttempt(
	_ context.Context, userID string, now time.Time, lockout models.PasswordLockout,
) (*models.UserCredential, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[userID]
	if !ok {
		return nil, repositories.ErrUserCredentialNotFound
	}
	if row.IsDisabled() {
		return nil, repositories.ErrUserCredentialDisabled
	}
	if row.IsLockedAt(now) {
		return nil, repositories.ErrUserCredentialLocked
	}
	row.FailedAttempts++
	if lock := lockout.LockFor(row.FailedAttempts); lock > 0 {
		until := now.Add(lock)
		row.LockedUntil = &until
	}
	c := *row
	return &c, nil
}

func (m *memCredentials) RecordSuccess(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if row, ok := m.rows[userID]; ok {
		row.FailedAttempts, row.LockedUntil = 0, nil
	}
	return nil
}

func (m *memCredentials) Delete(_ context.Context, userID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.rows[userID]
	delete(m.rows, userID)
	return ok, nil
}

// stored returns the fixture user's credential as stored.
func (m *memCredentials) stored() models.UserCredential {
	m.mu.Lock()
	defer m.mu.Unlock()
	return *m.rows[pwLoginUserID]
}

// pwLoginFixture is a server with the password routes mounted over a real
// PasswordAuthService.
type pwLoginFixture struct {
	srv         *Server
	container   *MockAuthContainer
	credentials *memCredentials
	user        *models.User
	hash        string
	hasher      *password.Hasher
	// hashedAtStart is the hasher's computation count once the fixture is
	// built (it hashes the fixture password), so a test counts its own work.
	hashedAtStart int64
	logs          *logtest.Recorder
	now           time.Time
}

type pwLoginOption func(f *pwLoginFixture, deps *services.PasswordAuthDeps)

func pwLoginWithPolicy(p services.AuthPolicyResolver) pwLoginOption {
	return func(f *pwLoginFixture, deps *services.PasswordAuthDeps) {
		f.container.authPolicy = p
		deps.Policy = p
	}
}

func pwLoginWithAllowlist(emails ...string) pwLoginOption {
	return func(_ *pwLoginFixture, deps *services.PasswordAuthDeps) {
		deps.Allowlist = staticAllowlistResolver(nil, emails)
	}
}

func newPwLoginFixture(t *testing.T, opts ...pwLoginOption) *pwLoginFixture {
	t.Helper()
	logger, logs := logtest.New()
	hasher := password.NewHasher(pwLoginCheapParams)
	hash, err := hasher.Hash(context.Background(), pwLoginPassword)
	require.NoError(t, err)

	f := &pwLoginFixture{
		container:   newMockAuthContainer(t),
		credentials: &memCredentials{rows: map[string]*models.UserCredential{}},
		user: &models.User{
			ID: pwLoginUserID, Email: pwLoginEmail, Name: "Ada Lovelace", Status: models.UserStatusActive,
			SessionEpoch: 2,
			CreatedAt:    time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt:    time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC),
			Version:      1,
		},
		hash:   hash,
		hasher: hasher,
		logs:   logs,
		now:    time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC),
	}
	f.credentials.rows[pwLoginUserID] = &models.UserCredential{UserID: pwLoginUserID, PasswordHash: hash}
	f.hashedAtStart = hasher.Derivations()

	users := repomocks.NewMockUserRepository(t)
	users.On("GetByEmail", mock.Anything, pwLoginEmail).
		Return(func(context.Context, string) (*models.User, error) { u := *f.user; return &u, nil }).Maybe()
	users.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound).Maybe()
	users.On("GetByID", mock.Anything, pwLoginUserID).
		Return(func(context.Context, string) (*models.User, error) { u := *f.user; return &u, nil }).Maybe()
	f.container.userRepository = users
	f.container.authService.On("GetUserByID", mock.Anything, pwLoginUserID).
		Return(func(context.Context, string) (*models.User, error) { u := *f.user; return &u, nil }).Maybe()

	on := staticAuthPolicy{
		policy: models.InstanceAuthPolicy{PasswordLoginEnabled: true, RegistrationMode: models.RegistrationModeOpen},
		reset:  services.PasswordResetByAdmin,
	}
	f.container.authPolicy = on
	deps := services.PasswordAuthDeps{
		Users:       users,
		Credentials: f.credentials,
		Policy:      on,
		Allowlist:   staticAllowlistResolver(nil, nil),
		Hasher:      hasher,
		Lockout:     pwLoginLockout,
		Logger:      logger,
		Now:         func() time.Time { return f.now },
	}
	for _, opt := range opts {
		opt(f, &deps)
	}
	f.container.passwordAuth = services.NewPasswordAuthService(deps)

	f.srv = createTestAuthServer(f.container)
	f.srv.logger = logger
	f.srv.setupPasswordAuthRoutes()
	return f
}

// capturedActivity is the metadata of the last auth activity an expectation
// saw. Requests may be served concurrently, so it is read and written under a
// lock.
type capturedActivity struct {
	mu       sync.Mutex
	metadata map[string]interface{}
}

func (r *capturedActivity) set(metadata map[string]interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metadata = metadata
}

// get returns the value recorded under key, or nil.
func (r *capturedActivity) get(key string) interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.metadata[key]
}

// has reports whether the recorded metadata carries key.
func (r *capturedActivity) has(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.metadata[key]
	return ok
}

// expectActivity allows auth activities of the given type for the fixture user
// and returns the metadata the last one was recorded with.
func (f *pwLoginFixture) expectActivity(activityType string) *capturedActivity {
	recorded := &capturedActivity{}
	f.container.activityService.On("RecordAuthActivity", mock.Anything, pwLoginUserID, activityType,
		(*string)(nil), mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { recorded.set(args.Get(4).(map[string]interface{})) }).
		Return(nil)
	return recorded
}

// post sends a login request and asserts the response conforms to the spec.
func (f *pwLoginFixture) post(t *testing.T, email, plaintext string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"email": email, "password": plaintext})
	require.NoError(t, err)
	return f.postRaw(t, raw)
}

func (f *pwLoginFixture) postRaw(t *testing.T, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, pwLoginPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "pw-login-test")
	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, req)
	specconformance.AssertConformsToSpec(t, req, rec)
	return rec
}

// volatileProblemFields are the parts of a problem body that differ on every
// request by design.
var volatileProblemFields = regexp.MustCompile(`"(request_id|timestamp)":"[^"]*"`)

func stableBody(rec *httptest.ResponseRecorder) string {
	return volatileProblemFields.ReplaceAllString(rec.Body.String(), `"$1":""`)
}

func TestPasswordLogin_Success(t *testing.T) {
	f := newPwLoginFixture(t)
	recorded := f.expectActivity(activities.ActivityTypeAuthLogin)

	// Letter case and surrounding space in the address do not matter.
	rec := f.post(t, "  Ada@Example.COM ", pwLoginPassword)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, pwLoginUserID, body["id"])
	assert.Equal(t, pwLoginEmail, body["email"])
	assert.NotContains(t, body, "session_epoch")
	assert.NotContains(t, body, "password_hash")

	cookie := sessionCookieIn(rec)
	require.NotNil(t, cookie, "a successful login sets vx_session")
	assert.True(t, cookie.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	assert.Equal(t, int(sesslib.SlidingLifetime/time.Second), cookie.MaxAge)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	sess, err := f.srv.sessionManager.Read(req)
	require.NoError(t, err)
	assert.Equal(t, pwLoginUserID, sess.UserID)
	assert.Equal(t, sesslib.AuthMethodPassword, sess.AuthMethod)
	assert.Equal(t, int64(2), sess.Epoch, "minted under the user's current session epoch")
	assert.Empty(t, sess.Provider)
	assert.Empty(t, sess.RefreshToken, "no refresh token: a sliding session")
	assert.WithinDuration(t, time.Now(), sess.AuthenticatedAt, time.Minute)
	assert.WithinDuration(t, time.Now().Add(sesslib.SlidingLifetime), sess.ExpiresAt, time.Minute)

	assert.Equal(t, "password", recorded.get("provider"))
	f.container.activityService.AssertNumberOfCalls(t, "RecordAuthActivity", 1)
}

// The cookie a password login sets is an ordinary session: it authenticates
// the next request like any other.
func TestPasswordLogin_CookieAuthenticatesTheNextRequest(t *testing.T) {
	f := newPwLoginFixture(t)
	f.expectActivity(activities.ActivityTypeAuthLogin)
	f.user.SessionEpoch = 2
	login := f.post(t, pwLoginEmail, pwLoginPassword)
	require.Equal(t, http.StatusOK, login.Code, login.Body.String())

	me := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	me.AddCookie(sessionCookieIn(login))
	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, me)

	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// Every account-dependent refusal is the same response, byte for byte apart
// from the request id and timestamp: nothing tells the caller whether the
// account exists, has a password, is locked or is disabled.
func TestPasswordLogin_RefusalsAreOneIdenticalResponse(t *testing.T) {
	disabledAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		arrange  func(f *pwLoginFixture)
		email    string
		password string
		// wantActivity is whether the failure is recorded against an account.
		wantActivity bool
		wantReason   string
	}{
		"unknown email": {
			arrange: func(*pwLoginFixture) {}, email: "nobody@example.com", password: pwLoginPassword,
		},
		"wrong password": {
			arrange: func(*pwLoginFixture) {}, email: pwLoginEmail, password: pwLoginWrong,
			wantActivity: true, wantReason: "wrong_password",
		},
		"account with no password": {
			arrange: func(f *pwLoginFixture) { delete(f.credentials.rows, pwLoginUserID) },
			email:   pwLoginEmail, password: pwLoginPassword, wantActivity: true, wantReason: "no_password",
		},
		"disabled password": {
			arrange: func(f *pwLoginFixture) { f.credentials.rows[pwLoginUserID].DisabledAt = &disabledAt },
			email:   pwLoginEmail, password: pwLoginPassword, wantActivity: true, wantReason: "password_disabled",
		},
		"locked account": {
			arrange: func(f *pwLoginFixture) {
				until := f.now.Add(time.Minute)
				f.credentials.rows[pwLoginUserID].LockedUntil = &until
			},
			email: pwLoginEmail, password: pwLoginPassword, wantActivity: true, wantReason: "locked",
		},
	}

	bodies := map[string]string{}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newPwLoginFixture(t)
			tc.arrange(f)
			recorded := f.expectActivity(activities.ActivityTypeAuthFailure)

			rec := f.post(t, tc.email, tc.password)

			require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
			assert.Nil(t, sessionCookieIn(rec), "a refused login sets no session")
			assert.Contains(t, rec.Body.String(), signInInvalidDetail)
			bodies[name] = stableBody(rec)

			if tc.wantActivity {
				f.container.activityService.AssertNumberOfCalls(t, "RecordAuthActivity", 1)
				assert.Equal(t, tc.wantReason, recorded.get("reason"))
				assert.False(t, recorded.has("email"))
			} else {
				f.container.activityService.AssertNotCalled(t, "RecordAuthActivity")
			}
		})
	}

	require.Len(t, bodies, len(cases))
	want := bodies["unknown email"]
	for name, body := range bodies {
		assert.Equal(t, want, body, "%s must answer exactly like an unknown email", name)
	}
}

// The lockout schedule through real requests: the fifth wrong password locks
// the account, a locked account refuses even the right password without
// counting the attempt, the lock ends by itself, and a success resets it.
func TestPasswordLogin_LockoutSchedule(t *testing.T) {
	f := newPwLoginFixture(t)
	f.expectActivity(activities.ActivityTypeAuthFailure)
	f.expectActivity(activities.ActivityTypeAuthLogin)
	wrong := func() { require.Equal(t, http.StatusUnauthorized, f.post(t, pwLoginEmail, pwLoginWrong).Code) }

	for range 4 {
		wrong()
	}
	assert.Nil(t, f.credentials.stored().LockedUntil, "four failures do not lock")

	wrong() // the fifth
	locked := f.credentials.stored()
	require.NotNil(t, locked.LockedUntil)
	assert.Equal(t, time.Minute, locked.LockedUntil.Sub(f.now), "the fifth failure locks for one minute")

	// Locked: the right password is refused, and nothing is counted.
	require.Equal(t, http.StatusUnauthorized, f.post(t, pwLoginEmail, pwLoginPassword).Code)
	wrong()
	assert.Equal(t, 5, f.credentials.stored().FailedAttempts, "attempts during a lock are not counted")
	assert.Equal(t, time.Minute, f.credentials.stored().LockedUntil.Sub(f.now),
		"hammering a locked account does not extend the lock")

	// Each failure after a lock ends doubles the next lock, up to 15 minutes.
	for _, want := range []time.Duration{2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute, 15 * time.Minute} {
		f.now = *f.credentials.stored().LockedUntil
		wrong()
		assert.Equal(t, want, f.credentials.stored().LockedUntil.Sub(f.now))
	}

	// Once the lock has passed, the right password signs in and resets it.
	f.now = *f.credentials.stored().LockedUntil
	require.Equal(t, http.StatusOK, f.post(t, pwLoginEmail, pwLoginPassword).Code)
	reset := f.credentials.stored()
	assert.Zero(t, reset.FailedAttempts)
	assert.Nil(t, reset.LockedUntil)
}

func TestPasswordLogin_Suspended(t *testing.T) {
	t.Run("the right password is told the account is suspended, and the refusal is recorded", func(t *testing.T) {
		f := newPwLoginFixture(t)
		f.user.Status = models.UserStatusSuspended
		recorded := f.expectActivity(activities.ActivityTypeAuthFailure)

		rec := f.post(t, pwLoginEmail, pwLoginPassword)

		require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), suspendedAuthDetail)
		assert.Nil(t, sessionCookieIn(rec))
		f.container.activityService.AssertNumberOfCalls(t, "RecordAuthActivity", 1)
		assert.Equal(t, "account_suspended", recorded.get("reason"))
	})

	t.Run("a wrong password gets the generic refusal", func(t *testing.T) {
		f := newPwLoginFixture(t)
		f.user.Status = models.UserStatusSuspended
		f.expectActivity(activities.ActivityTypeAuthFailure)

		rec := f.post(t, pwLoginEmail, pwLoginWrong)

		require.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Contains(t, rec.Body.String(), signInInvalidDetail)
		assert.NotContains(t, rec.Body.String(), suspendedAuthDetail)
	})
}

// An active access allowlist is the hard boundary for password sign-in too.
func TestPasswordLogin_OffAllowlistAccountIsRefused(t *testing.T) {
	f := newPwLoginFixture(t, pwLoginWithAllowlist("someone-else@example.com"))

	rec := f.post(t, pwLoginEmail, pwLoginPassword)

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	code, _ := problemCodeAndReason(t, rec)
	assert.Equal(t, "ACCESS_RESTRICTED", strings.ToUpper(code))
	assert.Nil(t, sessionCookieIn(rec))
}

func TestPasswordLogin_OnAllowlistAccountSignsIn(t *testing.T) {
	f := newPwLoginFixture(t, pwLoginWithAllowlist(pwLoginEmail))
	f.expectActivity(activities.ActivityTypeAuthLogin)

	assert.Equal(t, http.StatusOK, f.post(t, pwLoginEmail, pwLoginPassword).Code)
}

// With password login switched off the endpoint does not exist: it answers the
// same 404 dev login gives outside development, whatever the body is, and
// before the body is decoded.
func TestPasswordLogin_DisabledIs404(t *testing.T) {
	off := staticAuthPolicy{policy: models.DefaultInstanceAuthPolicy(), reset: services.PasswordResetByAdmin}

	for name, body := range map[string]string{
		"valid credentials": fmt.Sprintf(`{"email":%q,"password":%q}`, pwLoginEmail, pwLoginPassword),
		"malformed body":    `{not json`,
		"empty body":        ``,
	} {
		t.Run(name, func(t *testing.T) {
			f := newPwLoginFixture(t, pwLoginWithPolicy(off))

			rec := f.postRaw(t, []byte(body))

			assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
			assert.Nil(t, sessionCookieIn(rec))
			f.container.activityService.AssertNotCalled(t, "RecordAuthActivity")
		})
	}
}

// An unreadable policy is not "switched off": it is a 503, so a database fault
// cannot make the only way in look as if it did not exist.
func TestPasswordLogin_UnreadablePolicyIs503(t *testing.T) {
	f := newPwLoginFixture(t, pwLoginWithPolicy(staticAuthPolicy{err: errors.New("connection refused")}))

	rec := f.post(t, pwLoginEmail, pwLoginPassword)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "connection refused")
}

func TestPasswordLogin_BadBodiesAre400(t *testing.T) {
	tooLongEmail := strings.Repeat("a", maxSignInEmailBytes) + "@example.com"
	tooLongPassword := strings.Repeat("p", maxSignInPasswordBytes+1)
	for name, body := range map[string]string{
		"email over the spec's maximum":    fmt.Sprintf(`{"email":%q,"password":%q}`, tooLongEmail, pwLoginPassword),
		"password over the spec's maximum": fmt.Sprintf(`{"email":%q,"password":%q}`, pwLoginEmail, tooLongPassword),
		"malformed JSON":                   `{not json`,
		"empty body":                       ``,
		"missing password":                 `{"email":"ada@example.com"}`,
		"missing email":                    `{"password":"correct horse battery staple"}`,
		"empty email":                      `{"email":"","password":"correct horse battery staple"}`,
		"empty password":                   `{"email":"ada@example.com","password":""}`,
		"password a number":                `{"email":"ada@example.com","password":12345}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newPwLoginFixture(t)

			rec := f.postRaw(t, []byte(body))

			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.NotContains(t, rec.Body.String(), "correct horse", "a rejected body is not echoed")
			assert.Equal(t, f.hashedAtStart, f.hasher.Derivations(), "a malformed request is never hashed")
		})
	}
}

// A password at the spec's maximum is a sign-in attempt like any other: it is
// refused as wrong, not rejected as malformed.
func TestPasswordLogin_PasswordAtTheMaximumIsChecked(t *testing.T) {
	f := newPwLoginFixture(t)
	f.expectActivity(activities.ActivityTypeAuthFailure)

	rec := f.post(t, pwLoginEmail, strings.Repeat("p", maxSignInPasswordBytes))

	assert.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
}

// Login CSRF: a cross-site HTML form cannot send application/json, but it can
// send text/plain bytes that parse as JSON. Such a post must not sign the
// visitor's browser in to the attacker's account, whatever the credentials.
func TestPasswordLogin_RefusesABodyNotDeclaredAsJSON(t *testing.T) {
	// What <form enctype="text/plain"> with one crafted input name can submit.
	formBody := fmt.Sprintf(`{"email":%q,"password":%q,"x":"="}`, pwLoginEmail, pwLoginPassword)

	for name, contentType := range map[string]string{
		"text/plain form":       "text/plain",
		"urlencoded form":       "application/x-www-form-urlencoded",
		"multipart form":        "multipart/form-data; boundary=x",
		"no content type":       "",
		"malformed media type":  "application/json; charset",
		"a json-looking suffix": "application/jsonp",
	} {
		t.Run(name, func(t *testing.T) {
			f := newPwLoginFixture(t)
			req := httptest.NewRequest(http.MethodPost, pwLoginPath, strings.NewReader(formBody))
			if contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}
			req.Header.Set("Origin", "https://evil.example")
			rec := httptest.NewRecorder()

			f.srv.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Nil(t, sessionCookieIn(rec), "no session may be set from a form post")
			assert.Equal(t, f.hashedAtStart, f.hasher.Derivations(), "the credentials are never checked")
			f.container.activityService.AssertNotCalled(t, "RecordAuthActivity")
		})
	}

	t.Run("application/json with a charset parameter is accepted", func(t *testing.T) {
		f := newPwLoginFixture(t)
		f.expectActivity(activities.ActivityTypeAuthLogin)
		raw := fmt.Sprintf(`{"email":%q,"password":%q}`, pwLoginEmail, pwLoginPassword)
		req := httptest.NewRequest(http.MethodPost, pwLoginPath, strings.NewReader(raw))
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		rec := httptest.NewRecorder()

		f.srv.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	})
}

// TestPasswordLogin_ConcurrentGuessesAreBoundedByTheThreshold is the throttle
// under the load it exists for, through the router: a burst of wrong passwords
// arriving together. However many arrive, no more than the threshold are
// checked against the hash, and all are answered identically.
func TestPasswordLogin_ConcurrentGuessesAreBoundedByTheThreshold(t *testing.T) {
	f := newPwLoginFixture(t)
	f.expectActivity(activities.ActivityTypeAuthFailure)
	before := f.hashedAtStart

	const guesses = 60
	codes := make([]int, guesses)
	var wg sync.WaitGroup
	for i := range guesses {
		wg.Go(func() {
			raw := fmt.Sprintf(`{"email":%q,"password":"guess number %d of many"}`, pwLoginEmail, i)
			req := httptest.NewRequest(http.MethodPost, pwLoginPath, strings.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			f.srv.ServeHTTP(rec, req)
			codes[i] = rec.Code
		})
	}
	wg.Wait()

	for i, code := range codes {
		assert.Equal(t, http.StatusUnauthorized, code, "guess %d", i)
	}
	stored := f.credentials.stored()
	assert.Equal(t, pwLoginLockout.Threshold, stored.FailedAttempts,
		"only the threshold's worth of attempts are admitted and counted")
	require.NotNil(t, stored.LockedUntil)
	assert.Equal(t, int64(guesses), f.hasher.Derivations()-before,
		"every guess costs one computation, admitted or not")
}

// The per-IP limit answers the 429 the spec documents, as a problem document.
func TestPasswordLogin_RateLimitIs429Problem(t *testing.T) {
	f := newPwLoginFixture(t)
	f.expectActivity(activities.ActivityTypeAuthFailure)
	// A production-like instance: not local development, two sign-in requests
	// a minute. The routes are mounted again on a fresh router with that
	// configuration.
	f.srv.config.Frontend.BaseURL = "https://vibexp.example"
	f.srv.config.RateLimit.AuthPerMinute = 2
	f.srv.router = chi.NewRouter()
	f.srv.setupPasswordAuthRoutes()

	var last *httptest.ResponseRecorder
	for range 3 {
		last = f.post(t, pwLoginEmail, pwLoginWrong)
	}

	require.Equal(t, http.StatusTooManyRequests, last.Code, last.Body.String())
	assert.Contains(t, last.Header().Get("Content-Type"), "application/problem+json")
	code, _ := problemCodeAndReason(t, last)
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", code)
}

// A password that fails the policy becomes one field error on `password` per
// rule broken, with a stable code and a sentence that never quotes the
// password. Every flow that sets a password answers with this.
func TestPasswordPolicyProblem(t *testing.T) {
	const rejected = "ada@example.com"
	err := password.Validate("ada", rejected) // too short and the local part
	var policyErr *password.PolicyError
	require.ErrorAs(t, err, &policyErr)

	problem := passwordPolicyProblem(policyErr)

	assert.Equal(t, http.StatusBadRequest, problem.Status)
	assert.Equal(t, "VALIDATION_FAILED", problem.Code)
	require.Len(t, problem.ValidationErrors, 2)
	codes := []string{problem.ValidationErrors[0].Code, problem.ValidationErrors[1].Code}
	assert.ElementsMatch(t, []string{"too_short", "matches_email"}, codes)
	for _, fieldErr := range problem.ValidationErrors {
		assert.Equal(t, "password", fieldErr.Field)
		assert.NotEmpty(t, fieldErr.Message)
		assert.NotContains(t, fieldErr.Message, "ada")
	}
}

// A sign-in that cannot be decided (the account store is down) is a 503, not a
// refusal, and says nothing about the cause.
func TestPasswordLogin_StoreFailureIs503(t *testing.T) {
	f := newPwLoginFixture(t)
	users := repomocks.NewMockUserRepository(t)
	users.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, errors.New("connection refused"))
	f.container.passwordAuth = services.NewPasswordAuthService(services.PasswordAuthDeps{
		Users: users, Credentials: f.credentials, Policy: f.container.authPolicy,
		Allowlist: staticAllowlistResolver(nil, nil), Hasher: password.NewHasher(pwLoginCheapParams),
		Logger: f.srv.logger,
	})

	rec := f.post(t, pwLoginEmail, pwLoginPassword)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "connection refused")
}

// A request whose client went away before it could be checked is answered 503
// and logged as what it is, not as a fault of the policy or the database.
func TestPasswordLogin_AbandonedRequestIsNotLoggedAsAFault(t *testing.T) {
	f := newPwLoginFixture(t)
	raw := fmt.Sprintf(`{"email":%q,"password":%q}`, pwLoginEmail, pwLoginPassword)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, pwLoginPath, strings.NewReader(raw)).WithContext(cancelled)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	assert.Nil(t, sessionCookieIn(rec))
	assert.Equal(t, f.hashedAtStart, f.hasher.Derivations(), "an abandoned request is never hashed")
	var messages []string
	for _, entry := range f.logs.AllEntries() {
		messages = append(messages, entry.Message)
		assert.Less(t, entry.Level, slog.LevelError, "entry %q", entry.Message)
	}
	assert.Contains(t, messages, "Password sign-in abandoned before it was checked")
}

// TestPasswordLogin_NothingSecretLeaves drives the success path and every
// refusal, then scans everything the server emitted (log entries, activity
// metadata, response bodies and headers) for the passwords and the stored
// hash.
func TestPasswordLogin_NothingSecretLeaves(t *testing.T) {
	f := newPwLoginFixture(t)
	var activityMetadata []map[string]interface{}
	f.container.activityService.On("RecordAuthActivity", mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			activityMetadata = append(activityMetadata, args.Get(4).(map[string]interface{}))
		}).Return(nil)

	var responses []string
	record := func(rec *httptest.ResponseRecorder) {
		responses = append(responses, rec.Body.String(), fmt.Sprintf("%v", rec.Header()))
	}
	record(f.post(t, pwLoginEmail, pwLoginPassword))
	record(f.post(t, pwLoginEmail, pwLoginWrong))
	record(f.post(t, "nobody@example.com", pwLoginPassword))
	for range 5 {
		record(f.post(t, pwLoginEmail, pwLoginWrong)) // through the lock
	}
	record(f.post(t, pwLoginEmail, pwLoginPassword)) // locked
	record(f.postRaw(t, []byte(`{"email":"ada@example.com","password":"`+pwLoginPassword+`"`)))

	secrets := []string{pwLoginPassword, pwLoginWrong, f.hash}
	require.NotEmpty(t, f.logs.AllEntries())
	for _, entry := range f.logs.AllEntries() {
		line := entry.Message + " " + fmt.Sprintf("%v", entry.Data)
		for _, secret := range secrets {
			assert.NotContains(t, line, secret, "log entry %q", entry.Message)
		}
	}
	require.NotEmpty(t, activityMetadata)
	for _, metadata := range activityMetadata {
		for _, secret := range secrets {
			assert.NotContains(t, fmt.Sprintf("%v", metadata), secret)
		}
	}
	for _, response := range responses {
		for _, secret := range secrets {
			assert.NotContains(t, response, secret)
		}
	}
}
