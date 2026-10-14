package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/testutils/fakeoidc"
)

// #1485: a write of the auth settings in this process is what the very next
// request in this process sees. The policy resolver caches for 2 s; before
// the fix the sign-in right after the write was answered 404 ("password
// sign-in is off") from that cache.

// warmPasswordLoginOff makes the resolver cache "password login off", as a
// visit to the sign-in page does just before the operator acts.
func warmPasswordLoginOff(t *testing.T, f *authSettingsFixture) {
	t.Helper()
	policy, err := f.policyResolver.Policy(context.Background())
	require.NoError(t, err)
	require.False(t, policy.PasswordLoginEnabled)
}

func (f *authSettingsFixture) passwordLogin(t *testing.T, email string) int {
	t.Helper()
	return f.serve(t, asAnonymous, http.MethodPost, pwLoginPath,
		map[string]any{"email": email, "password": bootstrapTestPassphrase}).Code
}

func (f *authSettingsFixture) loginSucceedsFor(userID, email string) {
	f.passwords.On("Login", mock.Anything, email, bootstrapTestPassphrase).
		Return(&models.User{ID: userID, Email: email, Status: models.UserStatusActive}, nil).Once()
}

func TestBootstrapAdminPassword_TheImmediateSignInSeesPasswordLoginOn(t *testing.T) {
	f := newAuthSettingsFixture(t)
	warmPasswordLoginOff(t, f)

	rr := f.serve(t, asSetupSession, http.MethodPut, authBootstrapPath, bootstrapBody(authTestRootEmail))
	require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())

	f.loginSucceedsFor(authTestRootID, authTestRootEmail)
	require.Equal(t, http.StatusOK, f.passwordLogin(t, authTestRootEmail),
		"the setup page signs the operator in at once, not after the cache expires")
}

func TestUpdatePolicy_PasswordLoginIsUsableAtOnce(t *testing.T) {
	f := newAuthSettingsFixture(t)
	warmPasswordLoginOff(t, f)

	rr := f.serve(t, asRoot, http.MethodPut, authPolicyPath, policyBody(true, "open"))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	f.loginSucceedsFor(authTestRootID, authTestRootEmail)
	require.Equal(t, http.StatusOK, f.passwordLogin(t, authTestRootEmail))
}

// The public sign-in options list a provider the moment an admin adds it. The
// identity provider resolver reads the settings version on every call, so it
// needs no invalidation; this pins that, through the real resolver and the
// real auth service.
func TestCreateProvider_ThePublicListShowsItAtOnce(t *testing.T) {
	f := newAuthSettingsFixture(t)
	iss := fakeoidc.New(t, "client-corp-sso", authTestSecretSentinel, "sub-1", "user@example.com")
	resolver := services.NewIdentityProviderResolver(services.IdentityProviderResolverDeps{
		Providers: f.store.Providers(), Versions: f.store.Versions(), Enc: f.enc,
		CallbackURL: authTestCallbackURL, Logger: slog.New(slog.DiscardHandler),
	})
	f.srv.container.(*authSettingsContainer).auth = services.NewAuthService(
		f.users, nil, resolver, nil, slog.New(slog.DiscardHandler), nil)
	listed := func() []string {
		rr := f.serve(t, asAnonymous, http.MethodGet, "/api/v1/auth/providers", nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var body ProvidersResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
		out := []string{}
		for _, p := range body.Providers {
			out = append(out, p.Name)
		}
		return out
	}
	require.Empty(t, listed(), "nothing configured yet; the list is now cached")

	body := oidcProviderBody("corp-sso")
	body["issuer_url"] = iss.URL
	rr := f.serve(t, asRoot, http.MethodPost, authProvidersPath, body)
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	assert.Equal(t, []string{"corp-sso"}, listed())
}
