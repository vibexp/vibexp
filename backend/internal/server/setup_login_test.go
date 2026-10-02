package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/testutils/fakeoidc"
)

// setupAuthContainer adds the real setup-mode service to the resolver auth
// harness's container, so a real provider sign-in runs the real consume hook.
type setupAuthContainer struct {
	*realAuthContainer
	setup services.SetupModeService
}

func (c *setupAuthContainer) SetupModeService() services.SetupModeService { return c.setup }

// signInThroughProvider arms setup on a fresh instance, configures one OIDC
// provider, and signs email in through it. It returns the callback response and
// the setup state.
func signInThroughProvider(t *testing.T, email string, state *memSetupState) *httptest.ResponseRecorder {
	t.Helper()
	h := newResolverAuthHarness(t)
	setup := newSetupModeService(state, h.providers)
	h.srv.container = &setupAuthContainer{realAuthContainer: h.srv.container.(*realAuthContainer), setup: setup}

	_, minted, err := setup.EnsureTokenAtBoot(context.Background())
	require.NoError(t, err)
	require.True(t, minted, "a fresh instance with no provider is in setup mode")

	issuer := fakeoidc.New(t, rtClientID, rtClientSecret, rtSubject, email)
	h.providers.replace(h.oidcRow(t, "id-a", "corp-sso", issuer.URL, 0))

	loginState, cookie := h.login(t, "corp-sso")
	return h.callback(t, loginState, cookie)
}

func hasSessionCookie(w *httptest.ResponseRecorder) bool {
	for _, c := range w.Result().Cookies() {
		if c.Name == sesslib.CookieName && c.Value != "" {
			return true
		}
	}
	return false
}

func TestCallback_RootAdminSignInConsumesSetup(t *testing.T) {
	state := &memSetupState{}
	w := signInThroughProvider(t, "ROOT@instance.test", state)

	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	require.True(t, hasSessionCookie(w))
	require.True(t, state.consumed(), "the root admin's first provider sign-in consumes the setup token")
	assert.NotNil(t, state.row.ConsumedBy)
	assert.Nil(t, state.row.TokenHash)
}

func TestCallback_NonRootSignInLeavesSetupOpen(t *testing.T) {
	state := &memSetupState{}
	w := signInThroughProvider(t, "member@instance.test", state)

	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	require.True(t, hasSessionCookie(w), "the sign-in itself succeeds under the normal rules")
	assert.False(t, state.consumed(), "a non-root sign-in does not consume the setup token")
	assert.NotNil(t, state.row.TokenHash)
}

func TestCallback_ConsumeFailureNeverFailsTheSignIn(t *testing.T) {
	state := &memSetupState{consumeErr: errors.New("db down")}
	w := signInThroughProvider(t, setupTestRootEmail, state)

	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	assert.Equal(t, "http://localhost:5173/", w.Header().Get("Location"))
	assert.True(t, hasSessionCookie(w))
	assert.False(t, state.consumed())
}

// Dev login is not a provider sign-in: it works while setup mode is active and
// never consumes the setup token, even for a root admin's address.
func TestDevLogin_WorksInSetupModeAndDoesNotConsume(t *testing.T) {
	mc := newMockAuthContainer(t)
	user := &models.User{ID: "dev-root", Email: setupTestRootEmail, Name: "Root"}
	mc.authService.On("HandleDevLogin", mock.Anything, setupTestRootEmail, "Root").Return(user, nil)
	mc.activityService.On("RecordAuthActivity",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return(nil)

	state := &memSetupState{}
	setup := newSetupModeService(state, &memAuthProviders{})
	srv := createTestAuthServer(mc)
	srv.container = &setupAuthContainer{
		realAuthContainer: &realAuthContainer{MockAuthContainer: mc, auth: mc.authService}, setup: setup,
	}
	_, minted, err := setup.EnsureTokenAtBoot(context.Background())
	require.NoError(t, err)
	require.True(t, minted)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/dev/login",
		strings.NewReader(`{"email":"`+setupTestRootEmail+`","name":"Root"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.True(t, hasSessionCookie(w))
	assert.False(t, state.consumed(), "dev login is not a provider sign-in")
	active, err := setup.IsActive(context.Background())
	require.NoError(t, err)
	assert.True(t, active, "setup mode is still shown")
}
