package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// Forgot password (#1433), through the router. What happens to an address is
// the mailer's business and is tested there; these tests pin what the endpoint
// itself promises: one answer for every address, given before anything is
// looked up.

const forgotPath = "/api/v1/auth/password/forgot"

// recordingMailer is an AccountLinkMailerInterface that records the addresses
// it was asked about and does nothing else.
type recordingMailer struct {
	services.AccountLinkMailerInterface
	mu        sync.Mutex
	requested []string
}

func (m *recordingMailer) RequestPasswordReset(email string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requested = append(m.requested, email)
}

type forgotFixture struct {
	*pwLoginFixture
	mailer *recordingMailer
}

func newForgotFixture(t *testing.T, opts ...pwLoginOption) *forgotFixture {
	t.Helper()
	f := &forgotFixture{pwLoginFixture: newPwLoginFixture(t, opts...), mailer: &recordingMailer{}}
	f.container.accountLinkMailer = f.mailer
	return f
}

func (f *forgotFixture) send(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, forgotPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, req)
	specconformance.AssertConformsToSpec(t, req, rec)
	return rec
}

// Every address gets the same answer, byte for byte, and the handler touches
// no account to give it: the fixture's user store would fail the test on any
// lookup it did not expect, and the only thing the handler does is hand the
// address on.
func TestForgotPassword_OneAnswerForEveryAddress(t *testing.T) {
	f := newForgotFixture(t)
	addresses := []string{
		pwLoginEmail,               // an account with a password
		"nobody@example.com",       // no such account
		"Ada@Example.COM",          // spelled differently
		"suspended@example.com",    // whatever its state, the endpoint cannot tell
		"not-on-allowlist@else.io", // nor this
	}

	var reference string
	for _, address := range addresses {
		raw, err := json.Marshal(map[string]string{"email": address})
		require.NoError(t, err)

		rec := f.send(t, string(raw))

		require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
		if reference == "" {
			reference = rec.Body.String()
		}
		assert.Equal(t, reference, rec.Body.String())
		assert.JSONEq(t, `{"status":"accepted"}`, rec.Body.String())
		assert.Nil(t, sessionCookieIn(rec))
	}
	assert.Equal(t, addresses, f.mailer.requested, "each address is handed on once, unexamined")
}

// blockingMailer is an AccountLinkMailerInterface whose work for an address
// would take until release is closed, run the way the real one runs it: in
// the background.
type blockingMailer struct {
	services.AccountLinkMailerInterface
	started chan struct{}
	release chan struct{}
	done    chan struct{}
}

func (m *blockingMailer) RequestPasswordReset(string) {
	go func() {
		close(m.started)
		<-m.release
		close(m.done)
	}()
}

// The handler does not wait for the mailer's work: the 202 is on the wire
// while that work has not finished. This pins only the handler's half. That
// the real mailer returns before looking anything up is pinned in the services
// package (TestAccountLinkMailer_RequestPasswordResetReturnsBeforeTheWorkIsDone).
func TestForgotPassword_HandlerDoesNotWaitForTheMailer(t *testing.T) {
	f := newForgotFixture(t)
	mailer := &blockingMailer{
		started: make(chan struct{}), release: make(chan struct{}), done: make(chan struct{}),
	}
	f.container.accountLinkMailer = mailer

	rec := f.send(t, `{"email":"ada@example.com"}`)

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	<-mailer.started
	select {
	case <-mailer.done:
		t.Fatal("the work finished before it was released")
	default:
	}
	close(mailer.release)
	<-mailer.done
}

// The answer is the same on an instance with no email provider: the page
// learns that from GET /auth/providers, never from this endpoint.
func TestForgotPassword_SameAnswerWithoutAnEmailProvider(t *testing.T) {
	noEmail := staticAuthPolicy{
		policy: models.InstanceAuthPolicy{PasswordLoginEnabled: true, RegistrationMode: models.RegistrationModeOpen},
		reset:  services.PasswordResetByAdmin,
	}
	f := newForgotFixture(t, pwLoginWithPolicy(noEmail))

	rec := f.send(t, `{"email":"ada@example.com"}`)

	assert.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"status":"accepted"}`, rec.Body.String())
}

func TestForgotPassword_BadBodiesAre400AndReachNoMailer(t *testing.T) {
	for name, body := range map[string]string{
		"no email":        `{}`,
		"empty email":     `{"email":""}`,
		"oversized email": `{"email":"` + strings.Repeat("a", 321) + `"}`,
		"malformed":       `{not json`,
		"empty body":      ``,
		"email not text":  `{"email":42}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newForgotFixture(t)

			rec := f.send(t, body)

			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Empty(t, f.mailer.requested)
		})
	}
}

// Like sign-in, it is not reachable from an HTML form on another site.
func TestForgotPassword_RefusesABodyNotDeclaredAsJSON(t *testing.T) {
	for _, contentType := range []string{"text/plain", "application/x-www-form-urlencoded", ""} {
		f := newForgotFixture(t)
		req := httptest.NewRequest(http.MethodPost, forgotPath, strings.NewReader(`{"email":"ada@example.com"}`))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rec := httptest.NewRecorder()

		f.srv.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code, contentType)
		assert.Empty(t, f.mailer.requested, contentType)
	}
}

func TestForgotPassword_DisabledIs404(t *testing.T) {
	off := staticAuthPolicy{policy: models.DefaultInstanceAuthPolicy(), reset: services.PasswordResetByAdmin}
	f := newForgotFixture(t, pwLoginWithPolicy(off))

	rec := f.send(t, `{"email":"ada@example.com"}`)

	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.Empty(t, f.mailer.requested)
}

func TestForgotPassword_UnreadablePolicyIs503(t *testing.T) {
	f := newForgotFixture(t, pwLoginWithPolicy(staticAuthPolicy{err: errors.New("connection refused")}))

	rec := f.send(t, `{"email":"ada@example.com"}`)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	assert.Empty(t, f.mailer.requested)
}

// The per-IP limit of the sign-in group applies, so one client cannot ask for
// mail without bound.
func TestForgotPassword_RateLimitIs429(t *testing.T) {
	f := newForgotFixture(t)
	f.srv.config.Frontend.BaseURL = "https://vibexp.example"
	f.srv.config.RateLimit.AuthPerMinute = 2
	f.srv.router = chi.NewRouter()
	f.srv.setupPasswordAuthRoutes()

	var last *httptest.ResponseRecorder
	for range 3 {
		last = f.send(t, `{"email":"ada@example.com"}`)
	}

	require.Equal(t, http.StatusTooManyRequests, last.Code, last.Body.String())
	assert.Len(t, f.mailer.requested, 2, "the request over the limit reaches no mailer")
}

// Nothing the endpoint logs carries the address.
func TestForgotPassword_LogsNoAddress(t *testing.T) {
	f := newForgotFixture(t)

	f.send(t, `{"email":"private.person@example.com"}`)

	for _, entry := range f.logs.AllEntries() {
		assert.NotContains(t, entry.Message, "private.person")
		for _, value := range entry.Data {
			if text, ok := value.(string); ok {
				assert.NotContains(t, text, "private.person")
			}
		}
	}
}
