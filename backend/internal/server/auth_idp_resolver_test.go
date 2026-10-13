package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/specconformance"
	"github.com/vibexp/vibexp/internal/testutils/fakeoidc"
)

// The tests in this file drive the REAL AuthService over the REAL
// IdentityProviderResolver (#1234), backed by in-memory provider rows and fake
// OIDC issuers, through the auth HTTP handlers.

const (
	rtClientID     = "roundtrip-client"
	rtClientSecret = "roundtrip-sentinel"
	rtSubject      = "same-subject"
)

// memAuthProviders is an in-memory provider table plus the shared version.
type memAuthProviders struct {
	repositories.InstanceAuthProviderRepository
	mu      sync.Mutex
	rows    []*models.InstanceAuthProvider
	version atomic.Int64
}

func (m *memAuthProviders) List(context.Context) ([]*models.InstanceAuthProvider, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*models.InstanceAuthProvider, len(m.rows))
	for i, r := range m.rows {
		c := *r
		out[i] = &c
	}
	return out, nil
}

// replace swaps the rows and bumps the version, as every repository write does.
func (m *memAuthProviders) replace(rows ...*models.InstanceAuthProvider) {
	m.mu.Lock()
	m.rows = rows
	m.mu.Unlock()
	m.version.Add(1)
}

// memAuthVersions adapts memAuthProviders to the version repository.
type memAuthVersions struct{ p *memAuthProviders }

func (v memAuthVersions) Get(context.Context) (int64, error) { return v.p.version.Load(), nil }

// memUsersByIDP is an in-memory user store keyed by (idp_provider, idp_subject).
type memUsersByIDP struct {
	repositories.UserRepository
	mu    sync.Mutex
	users []*models.User
}

func (m *memUsersByIDP) GetByIDPSubject(_ context.Context, provider, subject string) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.IDPProvider != nil && *u.IDPProvider == provider && u.IDPSubject != nil && *u.IDPSubject == subject {
			return u, nil
		}
	}
	return nil, repositories.ErrUserNotFound
}

func (m *memUsersByIDP) GetByEmail(_ context.Context, email string) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if models.NormalizeEmail(u.Email) == models.NormalizeEmail(email) {
			return u, nil
		}
	}
	return nil, repositories.ErrUserNotFound
}

func (m *memUsersByIDP) Create(_ context.Context, u *models.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u.ID = "user-" + string(rune('a'+len(m.users)))
	m.users = append(m.users, u)
	return nil
}

func (m *memUsersByIDP) Update(context.Context, *models.User) error { return nil }

func (m *memUsersByIDP) BumpSessionEpoch(_ context.Context, userID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == userID {
			u.SessionEpoch++
			return u.SessionEpoch, nil
		}
	}
	return 0, repositories.ErrUserNotFound
}

// memIdentities accepts every identity a sign-in records and remembers it as
// "userID provider subject".
type memIdentities struct {
	repositories.UserIdentityRepository
	mu   sync.Mutex
	rows []string
	held map[string][]models.UserIdentity
}

func (m *memIdentities) Upsert(_ context.Context, userID, provider, subject string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows = append(m.rows, userID+" "+provider+" "+subject)
	return nil
}

// ListByUserID reports the identities seeded with hold; the ones Upsert
// recorded during the test are not replayed here.
func (m *memIdentities) ListByUserID(_ context.Context, userID string) ([]models.UserIdentity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]models.UserIdentity(nil), m.held[userID]...), nil
}

// hold makes userID already own a sign-in identity.
func (m *memIdentities) hold(userID, provider, subject string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.held == nil {
		m.held = map[string][]models.UserIdentity{}
	}
	m.held[userID] = append(m.held[userID], models.UserIdentity{UserID: userID, Provider: provider, Subject: subject})
}

func (m *memIdentities) recorded() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.rows...)
}

// realAuthContainer serves a real AuthService over the mock auth container.
type realAuthContainer struct {
	*MockAuthContainer
	auth services.AuthServiceInterface
}

func (c *realAuthContainer) AuthService() services.AuthServiceInterface { return c.auth }

type resolverAuthHarness struct {
	srv        *Server
	providers  *memAuthProviders
	users      *memUsersByIDP
	identities *memIdentities
	gate       *switchableGate
	enc        services.EncryptionServiceInterface
}

// switchableGate is the registration gate of the harness: open until a test
// closes it, like an admin switching the instance to invite_only. While closed
// it admits the addresses in invited, the way a pending invitation does.
type switchableGate struct {
	mu      sync.Mutex
	closed  bool
	invited map[string]bool
}

func (g *switchableGate) AllowNewAccount(_ context.Context, email string, verified bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.closed || (verified && g.invited[models.NormalizeEmail(email)]) {
		return nil
	}
	return services.ErrRegistrationClosed
}

func (g *switchableGate) inviteOnly(invited ...string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closed = true
	g.invited = map[string]bool{}
	for _, email := range invited {
		g.invited[email] = true
	}
}

func newResolverAuthHarness(t *testing.T) *resolverAuthHarness {
	t.Helper()
	enc, err := services.NewEncryptionService(strings.Repeat("k", 32))
	require.NoError(t, err)
	logger := slog.New(slog.DiscardHandler)
	h := &resolverAuthHarness{
		providers: &memAuthProviders{}, users: &memUsersByIDP{}, identities: &memIdentities{},
		gate: &switchableGate{}, enc: enc,
	}

	resolver := services.NewIdentityProviderResolver(services.IdentityProviderResolverDeps{
		Providers:   h.providers,
		Versions:    memAuthVersions{h.providers},
		Enc:         enc,
		CallbackURL: "http://localhost:8080/api/v1/auth/callback",
		Logger:      logger,
	})
	auth := services.NewAuthService(h.users, h.identities, resolver, nil, logger,
		staticAllowlistResolver(nil, nil), // open access
		services.WithRegistrationGate(h.gate))

	mc := newMockAuthContainer(t)
	mc.activityService.On("RecordAuthActivity",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return(nil).Maybe()
	h.srv = createTestAuthServer(mc)
	h.srv.container = &realAuthContainer{MockAuthContainer: mc, auth: auth}
	return h
}

func (h *resolverAuthHarness) oidcRow(t *testing.T, id, slug, issuer string, sortOrder int) *models.InstanceAuthProvider {
	t.Helper()
	ct, err := h.enc.Encrypt(rtClientSecret)
	require.NoError(t, err)
	return &models.InstanceAuthProvider{
		ID: id, Type: models.InstanceAuthProviderOIDC, Slug: slug, DisplayName: "Sign in with " + slug,
		Enabled: true, SortOrder: sortOrder, ClientID: rtClientID, ClientSecretEncrypted: &ct,
		IssuerURL: &issuer, UpdatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	}
}

// login runs GET /auth/login?provider=slug and returns the state and cookie.
func (h *resolverAuthHarness) login(t *testing.T, slug string) (string, *http.Cookie) {
	t.Helper()
	w := httptest.NewRecorder()
	h.srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login?provider="+slug, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp LoginResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	u, err := url.Parse(resp.URL)
	require.NoError(t, err)
	state := u.Query().Get("state")
	require.NotEmpty(t, state)
	for _, c := range w.Result().Cookies() {
		if c.Name == stateCookieName {
			return state, c
		}
	}
	t.Fatal("no state cookie")
	return "", nil
}

// callback runs GET /auth/callback for state with the state cookie.
func (h *resolverAuthHarness) callback(t *testing.T, state string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?code=the-code&state="+url.QueryEscape(state), nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.srv.ServeHTTP(w, req)
	return w
}

func TestResolverAuth_TwoOIDCProvidersRoundTrip(t *testing.T) {
	h := newResolverAuthHarness(t)
	issA := fakeoidc.New(t, rtClientID, rtClientSecret, rtSubject, "a@example.com")
	issB := fakeoidc.New(t, rtClientID, rtClientSecret, rtSubject, "b@example.com")
	h.providers.replace(
		h.oidcRow(t, "id-a", "corp-sso", issA.URL, 0),
		h.oidcRow(t, "id-b", "partner-sso", issB.URL, 1),
	)

	for _, slug := range []string{"corp-sso", "partner-sso"} {
		state, cookie := h.login(t, slug)
		w := h.callback(t, state, cookie)
		require.Equal(t, http.StatusFound, w.Code, w.Body.String())
		assert.Equal(t, "http://localhost:5173/", w.Header().Get("Location"))
		var session *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == sesslib.CookieName {
				session = c
			}
		}
		require.NotNil(t, session, "%s: a session cookie is written", slug)
	}

	// The same `sub` from two issuers is two users, told apart by slug.
	require.Len(t, h.users.users, 2)
	assert.Equal(t, "corp-sso", *h.users.users[0].IDPProvider)
	assert.Equal(t, "partner-sso", *h.users.users[1].IDPProvider)
	assert.Equal(t, rtSubject, *h.users.users[0].IDPSubject)
	assert.Equal(t, rtSubject, *h.users.users[1].IDPSubject)
	assert.Nil(t, h.users.users[0].GoogleID, "an OIDC provider never receives a google_id")
	assert.EqualValues(t, 1, issA.Discoveries(), "one discovery per provider for the whole run")
	assert.EqualValues(t, 1, issB.Discoveries())
}

// sessionCookie returns the session cookie a response set, or nil.
func sessionCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == sesslib.CookieName && c.MaxAge >= 0 && c.Value != "" {
			return c
		}
	}
	return nil
}

// A provider sign-in with no identity yet and the email of an existing account
// (#1430): linked when the provider verified the email, refused with a stable
// code when it did not, and never attached to a suspended account.
func TestResolverAuth_CallbackLinksByVerifiedEmail(t *testing.T) {
	const email = "ada@example.com"
	setup := func(t *testing.T, status string) (*resolverAuthHarness, *fakeoidc.Issuer) {
		t.Helper()
		h := newResolverAuthHarness(t)
		iss := fakeoidc.New(t, rtClientID, rtClientSecret, rtSubject, "Ada@Example.com")
		h.providers.replace(h.oidcRow(t, "id-a", "corp-sso", iss.URL, 0))
		// An admin-created account: an email, no sign-in identity.
		h.users.users = append(h.users.users, &models.User{ID: "pre-created", Email: email, Name: "Ada", Status: status})
		return h, iss
	}

	t.Run("a verified email signs into the existing account", func(t *testing.T) {
		h, _ := setup(t, models.UserStatusActive)
		state, cookie := h.login(t, "corp-sso")

		w := h.callback(t, state, cookie)

		require.Equal(t, http.StatusFound, w.Code, w.Body.String())
		assert.Equal(t, "http://localhost:5173/", w.Header().Get("Location"))
		require.NotNil(t, sessionCookie(w), "a session is issued")
		require.Len(t, h.users.users, 1, "no second account is created")
		assert.Equal(t, []string{"pre-created corp-sso " + rtSubject}, h.identities.recorded())
		assert.EqualValues(t, 1, h.users.users[0].SessionEpoch, "claiming the account ends its earlier sessions")
	})

	t.Run("a never-verified account that already has a sign-in method is not linked to", func(t *testing.T) {
		// What an open instance holds after someone signed in with an
		// UNVERIFIED claim of this address: an account, with their identity.
		h, _ := setup(t, models.UserStatusActive)
		h.identities.hold("pre-created", "other-sso", "someone-else")
		state, cookie := h.login(t, "corp-sso")

		w := h.callback(t, state, cookie)

		require.Equal(t, http.StatusFound, w.Code, w.Body.String())
		assert.Equal(t, "http://localhost:5173/auth/callback?error=existing_sign_in_required", w.Header().Get("Location"))
		assert.Nil(t, sessionCookie(w), "no session is issued")
		assert.Empty(t, h.identities.recorded(), "no identity is attached")
		assert.Nil(t, h.users.users[0].EmailVerifiedAt)
		assert.EqualValues(t, 0, h.users.users[0].SessionEpoch)
	})

	t.Run("an unverified email is refused with email_not_verified", func(t *testing.T) {
		h, iss := setup(t, models.UserStatusActive)
		iss.SetEmailVerified(false)
		state, cookie := h.login(t, "corp-sso")

		w := h.callback(t, state, cookie)

		require.Equal(t, http.StatusFound, w.Code, w.Body.String())
		assert.Equal(t, "http://localhost:5173/auth/callback?error=email_not_verified", w.Header().Get("Location"))
		assert.Nil(t, sessionCookie(w), "no session is issued")
		require.Len(t, h.users.users, 1, "no account is created")
		assert.Empty(t, h.identities.recorded(), "no identity is attached")
	})

	t.Run("a suspended account is refused and gains no identity", func(t *testing.T) {
		h, _ := setup(t, models.UserStatusSuspended)
		state, cookie := h.login(t, "corp-sso")

		w := h.callback(t, state, cookie)

		require.Equal(t, http.StatusFound, w.Code, w.Body.String())
		assert.Equal(t, "http://localhost:5173/auth/callback?error=account_suspended", w.Header().Get("Location"))
		assert.Nil(t, sessionCookie(w))
		assert.Empty(t, h.identities.recorded())
	})
}

// Registration mode at the callback (#1434): in invite_only mode a sign-in that
// would create an account is redirected with registration_closed and creates
// nothing, an invited address gets its account, and an existing account is
// never affected.
func TestResolverAuth_CallbackRegistrationMode(t *testing.T) {
	setup := func(t *testing.T) *resolverAuthHarness {
		t.Helper()
		h := newResolverAuthHarness(t)
		iss := fakeoidc.New(t, rtClientID, rtClientSecret, rtSubject, "Newcomer@Example.com")
		h.providers.replace(h.oidcRow(t, "id-a", "corp-sso", iss.URL, 0))
		return h
	}
	signIn := func(t *testing.T, h *resolverAuthHarness) *httptest.ResponseRecorder {
		t.Helper()
		state, cookie := h.login(t, "corp-sso")
		w := h.callback(t, state, cookie)
		require.Equal(t, http.StatusFound, w.Code, w.Body.String())
		return w
	}

	t.Run("open: a newcomer gets an account", func(t *testing.T) {
		h := setup(t)

		w := signIn(t, h)

		assert.Equal(t, "http://localhost:5173/", w.Header().Get("Location"))
		require.NotNil(t, sessionCookie(w))
		require.Len(t, h.users.users, 1)
	})

	t.Run("invite_only: an uninvited newcomer is refused and no account is created", func(t *testing.T) {
		h := setup(t)
		h.gate.inviteOnly()

		w := signIn(t, h)

		assert.Equal(t, "http://localhost:5173/auth/callback?error=registration_closed", w.Header().Get("Location"))
		assert.Nil(t, sessionCookie(w), "no session is issued")
		assert.Empty(t, h.users.users, "no account is created")
		assert.Empty(t, h.identities.recorded())
	})

	t.Run("invite_only: an invited address gets its account", func(t *testing.T) {
		h := setup(t)
		h.gate.inviteOnly("newcomer@example.com")

		w := signIn(t, h)

		assert.Equal(t, "http://localhost:5173/", w.Header().Get("Location"))
		require.NotNil(t, sessionCookie(w))
		require.Len(t, h.users.users, 1)
	})

	t.Run("invite_only: an admin-created account signs in", func(t *testing.T) {
		h := setup(t)
		h.gate.inviteOnly()
		h.users.users = append(h.users.users,
			&models.User{ID: "pre-created", Email: "newcomer@example.com", Status: models.UserStatusActive})

		w := signIn(t, h)

		assert.Equal(t, "http://localhost:5173/", w.Header().Get("Location"))
		require.NotNil(t, sessionCookie(w))
		require.Len(t, h.users.users, 1, "the existing account is used; the registration mode does not apply to it")
		assert.Equal(t, []string{"pre-created corp-sso " + rtSubject}, h.identities.recorded())
	})

	t.Run("invite_only: a returning user is unaffected", func(t *testing.T) {
		h := setup(t)
		signIn(t, h) // registers while the instance is open
		require.Len(t, h.users.users, 1)
		h.gate.inviteOnly()

		w := signIn(t, h)

		assert.Equal(t, "http://localhost:5173/", w.Header().Get("Location"))
		require.NotNil(t, sessionCookie(w))
		require.Len(t, h.users.users, 1)
	})
}

func TestResolverAuth_ProvidersEndpointFollowsTheRows(t *testing.T) {
	h := newResolverAuthHarness(t)
	issA := fakeoidc.New(t, rtClientID, rtClientSecret, rtSubject, "a@example.com")
	issB := fakeoidc.New(t, rtClientID, rtClientSecret, rtSubject, "b@example.com")
	h.providers.replace(
		h.oidcRow(t, "id-b", "partner-sso", issB.URL, 0),
		h.oidcRow(t, "id-a", "corp-sso", issA.URL, 1),
	)

	list := func() []AuthProvider {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)
		w := httptest.NewRecorder()
		h.srv.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		specconformance.AssertConformsToSpec(t, req, w)
		var resp ProvidersResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp.Providers
	}

	assert.Equal(t, []AuthProvider{
		{Name: "partner-sso", DisplayName: "Sign in with partner-sso", Type: "oidc"},
		{Name: "corp-sso", DisplayName: "Sign in with corp-sso", Type: "oidc"},
	}, list(), "two OIDC providers side by side, in sort order")

	// Disabling a row takes effect on the next call, with no restart.
	partner := h.oidcRow(t, "id-b", "partner-sso", issB.URL, 0)
	partner.Enabled = false
	h.providers.replace(partner, h.oidcRow(t, "id-a", "corp-sso", issA.URL, 1))
	assert.Equal(t, []AuthProvider{{Name: "corp-sso", DisplayName: "Sign in with corp-sso", Type: "oidc"}}, list())

	// So does the single-provider login default.
	w := httptest.NewRecorder()
	h.srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil))
	assert.Equal(t, http.StatusOK, w.Code, "one enabled provider is the default again")
}

func TestResolverAuth_UnknownSlugAtLoginIs400(t *testing.T) {
	h := newResolverAuthHarness(t)
	iss := fakeoidc.New(t, rtClientID, rtClientSecret, rtSubject, "a@example.com")
	h.providers.replace(h.oidcRow(t, "id-a", "corp-sso", iss.URL, 0))

	w := httptest.NewRecorder()
	h.srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login?provider=oidc", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code, "the type name is not a slug here")
}

func TestResolverAuth_SlugDisabledBetweenLoginAndCallbackRedirects(t *testing.T) {
	h := newResolverAuthHarness(t)
	iss := fakeoidc.New(t, rtClientID, rtClientSecret, rtSubject, "a@example.com")
	row := h.oidcRow(t, "id-a", "corp-sso", iss.URL, 0)
	h.providers.replace(row)

	state, cookie := h.login(t, "corp-sso")
	disabled := *row
	disabled.Enabled = false
	h.providers.replace(&disabled)

	w := h.callback(t, state, cookie)
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "http://localhost:5173/auth/callback?error=provider_unavailable", w.Header().Get("Location"))
	assert.Empty(t, h.users.users, "no user is provisioned")
}

// TestResolverAuth_StateCookieCarriesHyphenatedSlug pins that a slug with '-'
// survives the signed state cookie round trip.
func TestResolverAuth_StateCookieCarriesHyphenatedSlug(t *testing.T) {
	srv := createTestAuthServer(newMockAuthContainer(t))
	signed := srv.signState("abcdef0123", "corp-sso-eu")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?state=abcdef0123", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: signed})
	got, err := srv.validateStateCookie(req, "abcdef0123")
	require.NoError(t, err)
	assert.Equal(t, "corp-sso-eu", got)
}

func TestHandleListProviders_ResolverError(t *testing.T) {
	mc := newMockAuthContainer(t)
	mc.authService.On("EnabledProviders", mock.Anything).
		Return(nil, services.ErrIdentityProvidersUnresolvable)
	srv := createTestAuthServer(mc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
}

func TestHandleLogin_ResolverError(t *testing.T) {
	t.Run("listing the providers fails", func(t *testing.T) {
		mc := newMockAuthContainer(t)
		mc.authService.On("EnabledProviders", mock.Anything).
			Return(nil, services.ErrIdentityProvidersUnresolvable)
		srv := createTestAuthServer(mc)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		specconformance.AssertConformsToSpec(t, req, w)
	})
	t.Run("building the login URL fails", func(t *testing.T) {
		mc := newMockAuthContainer(t)
		mc.authService.On("EnabledProviders", mock.Anything).Return(providerInfos("corp-sso"), nil)
		mc.authService.On("GetLoginURL", mock.Anything, mock.Anything, "corp-sso").
			Return("", services.ErrIdentityProvidersUnresolvable)
		srv := createTestAuthServer(mc)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		specconformance.AssertConformsToSpec(t, req, w)
	})
}

func TestHandleCallback_ProviderErrors(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		code     int
		location string
	}{
		{"disabled provider redirects", services.ErrIdentityProviderUnavailable, http.StatusFound,
			"http://localhost:5173/auth/callback?error=provider_unavailable"},
		{"unresolvable providers is 503", services.ErrIdentityProvidersUnresolvable, http.StatusServiceUnavailable, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mc := newMockAuthContainer(t)
			mc.authService.On("HandleCallback", mock.Anything, "code-1", "corp-sso").
				Return((*models.User)(nil), (*idp.Tokens)(nil), false, errors.Join(tc.err, errors.New("detail")))
			srv := createTestAuthServer(mc)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?code=code-1&state=st4te", nil)
			req.AddCookie(buildStateCookie(t, srv, "st4te", "corp-sso"))
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			assert.Equal(t, tc.code, w.Code)
			assert.Equal(t, tc.location, w.Header().Get("Location"))
			specconformance.AssertConformsToSpec(t, req, w)
		})
	}
}

// expiredSessionCookie returns a vx_session cookie whose access token has
// expired, so the next authenticated request refreshes it through slug.
func expiredSessionCookie(t *testing.T, srv *Server, slug string) *http.Cookie {
	t.Helper()
	w := httptest.NewRecorder()
	require.NoError(t, srv.sessionManager.Write(w, &sesslib.Session{
		AccessToken: "old", RefreshToken: "refresh-1", ExpiresAt: time.Now().Add(-time.Minute),
		IDPSubject: rtSubject, UserID: "user-1", Provider: slug,
	}))
	for _, c := range w.Result().Cookies() {
		if c.Name == sesslib.CookieName {
			return c
		}
	}
	t.Fatal("no session cookie written")
	return nil
}

// sessionCleared reports whether the response expires the session cookie.
func sessionCleared(w *httptest.ResponseRecorder) bool {
	for _, c := range w.Result().Cookies() {
		if c.Name == sesslib.CookieName && c.MaxAge < 0 {
			return true
		}
	}
	return false
}

// TestResolverAuth_RefreshKeepsSessionWhenProviderIsUnhealthy pins that a
// session whose provider is enabled but failed to build is kept (503, retry),
// while one whose provider is no longer enabled is cleared (401).
func TestResolverAuth_RefreshKeepsSessionWhenProviderIsUnhealthy(t *testing.T) {
	h := newResolverAuthHarness(t)
	// Nothing listens on port 1, so discovery fails at once: enabled, unhealthy.
	h.providers.replace(h.oidcRow(t, "id-a", "corp-sso", "http://127.0.0.1:1", 0))

	get := func(slug string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.AddCookie(expiredSessionCookie(t, h.srv, slug))
		w := httptest.NewRecorder()
		h.srv.ServeHTTP(w, req)
		return w
	}

	w := get("corp-sso")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	assert.Equal(t, "5", w.Header().Get("Retry-After"))
	assert.False(t, sessionCleared(w), "an unhealthy provider must not sign the user out")

	w = get("partner-sso")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.True(t, sessionCleared(w), "a provider that is not enabled ends the session")
}

func TestRefresh_ResolverErrorKeepsSession(t *testing.T) {
	mc := newMockAuthContainer(t)
	mc.authService.On("RefreshTokens", mock.Anything, "corp-sso", "refresh-1").
		Return(nil, fmt.Errorf("%w: driver: bad connection", services.ErrIdentityProvidersUnresolvable))
	srv := createTestAuthServer(mc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(expiredSessionCookie(t, srv, "corp-sso"))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.False(t, sessionCleared(w), "a database hiccup must not sign the user out")
}
