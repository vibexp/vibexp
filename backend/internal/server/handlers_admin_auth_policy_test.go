package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// Instance authentication policy (#1429): the admin operations, their audit
// trail and the password sign-in options on the public providers endpoint.
// authSettingsFixture.serve asserts every response against the spec.

func policyBody(passwordLogin bool, mode string) map[string]any {
	return map[string]any{"password_login_enabled": passwordLogin, "registration_mode": mode}
}

func decodeAuthPolicy(t *testing.T, rr *httptest.ResponseRecorder) admingen.AdminAuthPolicy {
	t.Helper()
	var policy admingen.AdminAuthPolicy
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &policy))
	return policy
}

func TestAdminAuthPolicy_DefaultsWhenNothingIsStored(t *testing.T) {
	f := newAuthSettingsFixture(t)

	rr := f.serve(t, asRoot, http.MethodGet, authPolicyPath, nil)

	// The defaults are the behaviour of an instance from before the policy
	// existed, so an upgrade changes nothing.
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.JSONEq(t, `{"password_login_enabled":false,"registration_mode":"open","version":null,
		"updated_at":null,"updated_by_user_id":null}`, rr.Body.String())
	assert.Empty(t, f.store.AuditEntries(models.InstanceSettingAuthPolicy), "reading stores nothing")
}

func TestAdminAuthPolicy_RoundTrip(t *testing.T) {
	f := newAuthSettingsFixture(t)
	sharedBefore := f.store.Version()

	// With nothing stored, a positive expected_version is a conflict.
	rr := f.serve(t, asRoot, http.MethodPut, authPolicyPath,
		map[string]any{"password_login_enabled": true, "registration_mode": "open", "expected_version": 1})
	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
	assert.Equal(t, sharedBefore, f.store.Version(), "a refused save bumps nothing")

	rr = f.serve(t, asRoot, http.MethodPut, authPolicyPath, policyBody(true, "invite_only"))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	stored := decodeAuthPolicy(t, rr)
	assert.True(t, stored.PasswordLoginEnabled)
	assert.Equal(t, admingen.AdminAuthRegistrationModeInviteOnly, stored.RegistrationMode)
	require.NotNil(t, stored.Version)
	assert.Equal(t, int64(1), *stored.Version)
	require.NotNil(t, stored.UpdatedAt)
	require.NotNil(t, stored.UpdatedByUserId)
	assert.Equal(t, authTestRootID, stored.UpdatedByUserId.String())
	assert.Equal(t, sharedBefore+1, f.store.Version(),
		"a save bumps the shared auth settings version, the resolver's cache key")

	// A stale version is the same conflict the other auth settings answer.
	rr = f.serve(t, asRoot, http.MethodPut, authPolicyPath,
		map[string]any{"password_login_enabled": false, "registration_mode": "open", "expected_version": 7})
	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
	code, _ := problemCodeAndReason(t, rr)
	assert.Equal(t, "INSTANCE_SETTINGS_VERSION_CONFLICT", code)

	_, err := f.store.Grants().Grant(context.Background(), authTestDBAdminID, nil)
	require.NoError(t, err)
	rr = f.serve(t, asDBAdmin, http.MethodPut, authPolicyPath,
		map[string]any{"password_login_enabled": false, "registration_mode": "invite_only", "expected_version": 1})
	require.Equal(t, http.StatusOK, rr.Code, "a DB-granted admin may change the policy: "+rr.Body.String())
	stored = decodeAuthPolicy(t, rr)
	assert.False(t, stored.PasswordLoginEnabled)
	assert.Equal(t, int64(2), *stored.Version)
	assert.Equal(t, authTestDBAdminID, stored.UpdatedByUserId.String())

	rr = f.serve(t, asRoot, http.MethodGet, authPolicyPath, nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, stored, decodeAuthPolicy(t, rr), "GET returns what the last PUT stored")
}

// Expected version 0 means "no policy stored yet": it saves only while nothing
// is stored, and is a 409 that changes nothing once one is.
func TestAdminAuthPolicy_ExpectedVersionZeroExpectsNothingStored(t *testing.T) {
	f := newAuthSettingsFixture(t)
	put := func(mode string) *httptest.ResponseRecorder {
		return f.serve(t, asRoot, http.MethodPut, authPolicyPath,
			map[string]any{"password_login_enabled": true, "registration_mode": mode, "expected_version": 0})
	}

	require.Equal(t, http.StatusOK, put("open").Code)

	rr := put("invite_only")
	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
	rr = f.serve(t, asRoot, http.MethodGet, authPolicyPath, nil)
	assert.Equal(t, admingen.AdminAuthRegistrationModeOpen, decodeAuthPolicy(t, rr).RegistrationMode,
		"the conflicting PUT changes nothing")
	assert.Len(t, f.store.AuditEntries(models.InstanceSettingAuthPolicy), 1)
}

func TestAdminAuthPolicy_BadBodiesAre400(t *testing.T) {
	cases := map[string]map[string]any{
		"unknown registration mode": {"password_login_enabled": true, "registration_mode": "closed"},
		"empty registration mode":   {"password_login_enabled": true, "registration_mode": ""},
		"missing switch":            {"registration_mode": "open"},
		"missing mode":              {"password_login_enabled": true},
		"null switch":               {"password_login_enabled": nil, "registration_mode": "open"},
		"null mode":                 {"password_login_enabled": true, "registration_mode": nil},
		"switch of the wrong type":  {"password_login_enabled": "yes", "registration_mode": "open"},
		"unknown field":             {"password_login_enabled": true, "registration_mode": "open", "mfa": true},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			f := newAuthSettingsFixture(t)

			rr := f.serve(t, asRoot, http.MethodPut, authPolicyPath, body)

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			assert.Empty(t, f.store.AuditEntries(models.InstanceSettingAuthPolicy), "a rejected body stores nothing")
		})
	}
}

// The policy routes are instance-admin only: everyone else, a first-run setup
// session included, gets the admin surface's 404.
func TestAdminAuthPolicy_AdminOnly(t *testing.T) {
	callers := map[string]authCaller{
		"anonymous":     asAnonymous,
		"member":        asMember,
		"setup session": asSetupSession,
	}
	for name, caller := range callers {
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			t.Run(name+" "+method, func(t *testing.T) {
				f := newAuthSettingsFixture(t)
				var body any
				if method == http.MethodPut {
					body = policyBody(false, "invite_only")
				}

				rr := f.serve(t, caller, method, authPolicyPath, body)

				assert.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
				assert.Empty(t, f.store.AuditEntries(models.InstanceSettingAuthPolicy))
			})
		}
	}
}

// Each change writes exactly one audit entry with its actor and the values
// before and after, and the auth settings audit endpoint lists them.
func TestAdminAuthPolicy_Audit(t *testing.T) {
	f := newAuthSettingsFixture(t)
	_, err := f.store.Grants().Grant(context.Background(), authTestDBAdminID, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK,
		f.serve(t, asRoot, http.MethodPut, authPolicyPath, policyBody(true, "open")).Code)
	require.Equal(t, http.StatusOK,
		f.serve(t, asDBAdmin, http.MethodPut, authPolicyPath, policyBody(true, "invite_only")).Code)

	entries := f.store.AuditEntries(models.InstanceSettingAuthPolicy)
	require.Len(t, entries, 2, "one entry per change")
	assert.Nil(t, entries[0].Before, "the first save has nothing before it")
	require.NotNil(t, entries[0].ActorUserID)
	assert.Equal(t, authTestRootID, *entries[0].ActorUserID)

	rr := f.serve(t, asRoot, http.MethodGet, authAuditPath+"?setting=auth_policy", nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var page struct {
		Entries []struct {
			Setting     string         `json:"setting"`
			Action      string         `json:"action"`
			ActorUserID string         `json:"actor_user_id"`
			Before      map[string]any `json:"before"`
			After       map[string]any `json:"after"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
	require.Len(t, page.Entries, 2)
	latest := page.Entries[0]
	assert.Equal(t, "auth_policy", latest.Setting)
	assert.Equal(t, "upsert", latest.Action)
	assert.Equal(t, authTestDBAdminID, latest.ActorUserID)
	assert.Equal(t, "open", latest.Before["registration_mode"])
	assert.Equal(t, "invite_only", latest.After["registration_mode"])
	assert.Equal(t, true, latest.After["password_login_enabled"])
}

func TestAdminAuthPolicy_StoreFailureIs500(t *testing.T) {
	f := newAuthSettingsFixture(t)
	f.store.Err = errors.New("connection refused")

	// The caller is authenticated before the store is consulted, so both
	// failures are the handler's own.
	assert.Equal(t, http.StatusInternalServerError,
		f.serve(t, asRoot, http.MethodGet, authPolicyPath, nil).Code)
	assert.Equal(t, http.StatusInternalServerError,
		f.serve(t, asRoot, http.MethodPut, authPolicyPath, policyBody(true, "open")).Code)
}

// --- sign-in discovery ---------------------------------------------------------

func TestHandleListProviders_PasswordLogin(t *testing.T) {
	cases := []struct {
		name   string
		policy services.AuthPolicyResolver // nil: the container's default
		want   PasswordLogin
	}{
		{
			name: "nothing stored and no email provider",
			want: PasswordLogin{Enabled: false, Reset: "admin"},
		},
		{
			name: "password login on, resets by an administrator",
			policy: staticAuthPolicy{
				policy: models.InstanceAuthPolicy{PasswordLoginEnabled: true, RegistrationMode: "invite_only"},
				reset:  services.PasswordResetByAdmin,
			},
			want: PasswordLogin{Enabled: true, Reset: "admin"},
		},
		{
			name: "password login on, resets by email",
			policy: staticAuthPolicy{
				policy: models.InstanceAuthPolicy{PasswordLoginEnabled: true, RegistrationMode: "open"},
				reset:  services.PasswordResetByEmail,
			},
			want: PasswordLogin{Enabled: true, Reset: "email"},
		},
		{
			name: "the reset channel is reported while password login is off",
			policy: staticAuthPolicy{
				policy: models.DefaultInstanceAuthPolicy(), reset: services.PasswordResetByEmail,
			},
			want: PasswordLogin{Enabled: false, Reset: "email"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mc := newMockAuthContainer(t)
			mc.authPolicy = tc.policy
			mc.authService.On("EnabledProviders", mock.Anything).Return(providerInfos("google"), nil)
			srv := createTestAuthServer(mc)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			specconformance.AssertConformsToSpec(t, req, w)
			var response ProvidersResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, tc.want, response.Password)
			assert.Len(t, response.Providers, 1, "the provider list is unaffected")
		})
	}
}

// A sign-in page told "password login is off" because the policy could not be
// read would hide the only way in on a password-only instance, so the failure
// is reported instead of guessed.
func TestHandleListProviders_PolicyUnreadableIs503(t *testing.T) {
	mc := newMockAuthContainer(t)
	mc.authPolicy = staticAuthPolicy{err: errors.New("connection refused")}
	mc.authService.On("EnabledProviders", mock.Anything).Return(providerInfos("google"), nil)
	srv := createTestAuthServer(mc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
	assert.NotContains(t, w.Body.String(), "connection refused", "the cause is logged, not returned")
}
