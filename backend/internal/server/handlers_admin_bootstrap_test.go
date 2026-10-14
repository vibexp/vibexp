package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// Password-only bootstrap and the policy lockout guard (#1438).

const bootstrapTestPassphrase = "a long setup passphrase 1438"

func bootstrapBody(email string) map[string]any {
	return map[string]any{"email": email, "password": bootstrapTestPassphrase}
}

// asPasswordSessionOf signs userID in with a cookie session from a password
// sign-in.
func asPasswordSessionOf(userID string) authCaller {
	return func(t *testing.T, f *authSettingsFixture, r *http.Request) {
		t.Helper()
		rr := httptest.NewRecorder()
		require.NoError(t, f.srv.sessionManager.Write(rr, &sesslib.Session{
			UserID: userID, AuthMethod: sesslib.AuthMethodPassword, AuthenticatedAt: time.Now(),
			ExpiresAt: time.Now().Add(time.Hour),
		}))
		for _, c := range rr.Result().Cookies() {
			r.AddCookie(c)
		}
	}
}

func (f *authSettingsFixture) policy(t *testing.T) models.InstanceAuthPolicy {
	t.Helper()
	policy, err := f.store.Policy().Get(context.Background())
	if errors.Is(err, repositories.ErrInstanceAuthPolicyNotFound) {
		return models.DefaultInstanceAuthPolicy()
	}
	require.NoError(t, err)
	return *policy
}

func (f *authSettingsFixture) setupRequired(t *testing.T) bool {
	t.Helper()
	active, err := f.setup.IsActive(context.Background())
	require.NoError(t, err)
	return active
}

func TestBootstrapAdminPassword_MakesAPasswordOnlyInstanceUsable(t *testing.T) {
	f := newAuthSettingsFixture(t)
	require.True(t, f.setupRequired(t), "no provider, no password login: setup mode")
	root, err := f.users.GetByEmail(context.Background(), authTestRootEmail)
	require.NoError(t, err)
	require.Nil(t, root.EmailVerifiedAt)

	rr := f.serve(t, asSetupSession, http.MethodPut, authBootstrapPath, bootstrapBody(strings.ToUpper(authTestRootEmail)))

	require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	assert.Contains(t, f.creds.hashes, authTestRootID, "the root admin has a password")
	assert.True(t, f.policy(t).PasswordLoginEnabled, "password login is switched on")
	assert.Equal(t, models.RegistrationModeOpen, f.policy(t).RegistrationMode, "the registration mode is untouched")
	assert.False(t, f.setupRequired(t), "an admin with a usable password is a working way in")
	root, err = f.users.GetByEmail(context.Background(), authTestRootEmail)
	require.NoError(t, err)
	assert.NotNil(t, root.EmailVerifiedAt, "the operator vouches for a configured address")
	assert.EqualValues(t, 2, root.SessionEpoch, "the account's sessions are ended, before and after the change")
	assert.Empty(t, f.events.published, "the account existed: nothing to provision")
}

func TestBootstrapAdminPassword_CreatesTheAccountWhenMissing(t *testing.T) {
	f := newAuthSettingsFixture(t)
	require.NoError(t, f.users.DeleteByID(context.Background(), authTestRootID))

	rr := f.serve(t, asSetupSession, http.MethodPut, authBootstrapPath, bootstrapBody(authTestRootEmail))

	require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	created, err := f.users.GetByEmail(context.Background(), authTestRootEmail)
	require.NoError(t, err)
	assert.NotNil(t, created.EmailVerifiedAt)
	assert.Contains(t, f.creds.hashes, created.ID)
	require.Len(t, f.events.published, 1, "user.created provisions the workspace")
	assert.False(t, f.setupRequired(t))
}

func TestBootstrapAdminPassword_Refusals(t *testing.T) {
	unchanged := func(t *testing.T, f *authSettingsFixture) {
		t.Helper()
		assert.Empty(t, f.creds.hashes, "no password was stored")
		assert.False(t, f.policy(t).PasswordLoginEnabled, "password login stays off")
	}

	t.Run("an email that is not a root admin", func(t *testing.T) {
		f := newAuthSettingsFixture(t)

		// A granted admin and an ordinary member alike: only configuration counts.
		for _, email := range []string{authTestMemberEmail, "delegate@instance.test", "nobody@instance.test"} {
			rr := f.serve(t, asSetupSession, http.MethodPut, authBootstrapPath, bootstrapBody(email))

			require.Equal(t, http.StatusBadRequest, rr.Code, email)
			assert.Contains(t, rr.Body.String(), `"code":"not_root_admin"`)
		}
		unchanged(t, f)
	})

	t.Run("a password that breaks the policy", func(t *testing.T) {
		f := newAuthSettingsFixture(t)

		rr := f.serve(t, asSetupSession, http.MethodPut, authBootstrapPath,
			map[string]any{"email": authTestRootEmail, "password": "short"})

		require.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), `"field":"password"`)
		assert.NotContains(t, rr.Body.String(), `"short"`)
		unchanged(t, f)
	})

	t.Run("a signed-in admin, even the root admin, is refused: setup session only", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		_, err := f.store.Grants().Grant(context.Background(), authTestDBAdminID, nil)
		require.NoError(t, err)

		for name, caller := range map[string]authCaller{
			"root admin": asRoot, "granted admin": asDBAdmin, "root by password session": asPasswordSessionOf(authTestRootID),
		} {
			rr := f.serve(t, caller, http.MethodPut, authBootstrapPath, bootstrapBody(authTestRootEmail))
			require.Equal(t, http.StatusForbidden, rr.Code, name)
		}
		unchanged(t, f)
	})

	t.Run("anyone else gets the admin surface's 404", func(t *testing.T) {
		f := newAuthSettingsFixture(t)

		for name, caller := range map[string]authCaller{"anonymous": asAnonymous, "member": asMember} {
			rr := f.serve(t, caller, http.MethodPut, authBootstrapPath, bootstrapBody(authTestRootEmail))
			require.Equal(t, http.StatusNotFound, rr.Code, name)
		}
		unchanged(t, f)
	})

	t.Run("a missing or unknown field", func(t *testing.T) {
		f := newAuthSettingsFixture(t)

		for name, body := range map[string]map[string]any{
			"no password":   {"email": authTestRootEmail},
			"no email":      {"password": bootstrapTestPassphrase},
			"unknown field": {"email": authTestRootEmail, "password": bootstrapTestPassphrase, "name": "Root"},
		} {
			rr := f.serve(t, asSetupSession, http.MethodPut, authBootstrapPath, body)
			require.Equal(t, http.StatusBadRequest, rr.Code, name)
		}
		unchanged(t, f)
	})

	t.Run("a suspended account", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		root, err := f.users.GetByEmail(context.Background(), authTestRootEmail)
		require.NoError(t, err)
		root.Status = models.UserStatusSuspended

		rr := f.serve(t, asSetupSession, http.MethodPut, authBootstrapPath, bootstrapBody(authTestRootEmail))

		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		unchanged(t, f)
	})
}

// Replacing the password of an account that already had one is allowed (the
// setup URL holder can already repoint sign-in), ends its sessions, and is not
// silent.
func TestBootstrapAdminPassword_ReplacesAnExistingPassword(t *testing.T) {
	f := newAuthSettingsFixture(t)
	f.creds.hashes[authTestRootID] = "earlier"

	rr := f.serve(t, asSetupSession, http.MethodPut, authBootstrapPath, bootstrapBody(authTestRootEmail))

	require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	assert.Equal(t, "stored", f.creds.hashes[authTestRootID])
	root, err := f.users.GetByEmail(context.Background(), authTestRootEmail)
	require.NoError(t, err)
	assert.EqualValues(t, 2, root.SessionEpoch)
}

func putPolicy(passwordLogin bool, confirm bool) map[string]any {
	body := map[string]any{"password_login_enabled": passwordLogin, "registration_mode": models.RegistrationModeOpen}
	if confirm {
		body["confirm_lockout_risk"] = true
	}
	return body
}

func TestAdminAuthPolicy_LockoutGuard(t *testing.T) {
	// enablePasswordLogin stores "password login on" and gives the root admin
	// a password, so password login is a working way in.
	enablePasswordLogin := func(t *testing.T, f *authSettingsFixture) {
		t.Helper()
		rr := f.serve(t, asRoot, http.MethodPut, authPolicyPath, putPolicy(true, false))
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		f.creds.hashes[authTestRootID] = "stored"
	}
	refused := func(t *testing.T, rr *httptest.ResponseRecorder, wantReason string) map[string]any {
		t.Helper()
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		var p struct {
			Code     string         `json:"code"`
			Metadata map[string]any `json:"metadata"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &p))
		assert.Equal(t, "lockout_risk", p.Code)
		assert.Equal(t, wantReason, p.Metadata["reason"])
		return p.Metadata
	}

	t.Run("the only working way in", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		enablePasswordLogin(t, f)

		refused(t, f.serve(t, asRoot, http.MethodPut, authPolicyPath, putPolicy(false, false)),
			services.AuthLockoutNoUsableMethod)
		assert.True(t, f.policy(t).PasswordLoginEnabled, "nothing was written")

		rr := f.serve(t, asRoot, http.MethodPut, authPolicyPath, putPolicy(false, true))
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.False(t, f.policy(t).PasswordLoginEnabled)
		assert.Contains(t, f.creds.hashes, authTestRootID, "stored passwords are kept")
	})

	t.Run("the acting admin's own method, when a provider remains", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		f.createProvider(t, oidcProviderBody("okta"))
		enablePasswordLogin(t, f)

		refused(t, f.serve(t, asPasswordSessionOf(authTestRootID), http.MethodPut, authPolicyPath, putPolicy(false, false)),
			services.AuthLockoutOwnMethod)

		// The same change from a provider session locks nobody out.
		rr := f.serve(t, asSessionOf(authTestRootID, "okta"), http.MethodPut, authPolicyPath, putPolicy(false, false))
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	})

	t.Run("accounts whose only method is a password", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		f.createProvider(t, oidcProviderBody("okta"))
		enablePasswordLogin(t, f)
		f.creds.passwordOnly = 3

		metadata := refused(t, f.serve(t, asRoot, http.MethodPut, authPolicyPath, putPolicy(false, false)),
			services.AuthLockoutPasswordOnlyAccounts)
		assert.EqualValues(t, 3, metadata["password_only_accounts"])

		rr := f.serve(t, asRoot, http.MethodPut, authPolicyPath, putPolicy(false, true))
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	})

	t.Run("nothing to lose: no confirmation needed", func(t *testing.T) {
		f := newAuthSettingsFixture(t)

		// Already off (nothing stored), then on, then off with a provider and
		// nobody depending on a password.
		rr := f.serve(t, asRoot, http.MethodPut, authPolicyPath, putPolicy(false, false))
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		f.createProvider(t, oidcProviderBody("okta"))
		enablePasswordLogin(t, f)
		rr = f.serve(t, asRoot, http.MethodPut, authPolicyPath, putPolicy(false, false))
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	})

	t.Run("switching it on is never judged", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		f.creds.passwordOnly = 9

		rr := f.serve(t, asPasswordSessionOf(authTestRootID), http.MethodPut, authPolicyPath, putPolicy(true, false))
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	})
}

// With password login a working way in, the last provider can go without a
// confirmation; without it the existing guard still holds.
func TestAdminAuthProviders_LockoutGuardCountsPasswordLogin(t *testing.T) {
	f := newAuthSettingsFixture(t)
	id := f.createProvider(t, oidcProviderBody("okta"))
	rr := f.serve(t, asRoot, http.MethodPut, authPolicyPath, putPolicy(true, false))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	path := authProvidersPath + "/" + id

	// On, but no admin has a password yet: not a way in.
	rr = f.serve(t, asRoot, http.MethodDelete, path, nil)
	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
	_, reason := problemCodeAndReason(t, rr)
	assert.Equal(t, services.AuthLockoutNoEnabledProvider, reason)

	f.creds.hashes[authTestRootID] = "stored"
	rr = f.serve(t, asRoot, http.MethodDelete, path, nil)
	require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	assert.False(t, f.setupRequired(t), "password login keeps the instance out of setup mode")
}

// bootstrapActivities records the auth activity entries the bootstrap writes.
type bootstrapActivities struct {
	activities.ActivityService
	entries []bootstrapActivity
}

type bootstrapActivity struct {
	userID, activityType string
	metadata             map[string]interface{}
}

func (a *bootstrapActivities) RecordAuthActivity(
	_ context.Context, userID, activityType string, _ *string, metadata map[string]interface{}, _, _ *string,
) error {
	a.entries = append(a.entries, bootstrapActivity{userID, activityType, metadata})
	return nil
}

// The owner of an EXISTING account did not set this password, so the account's
// activity says it happened: when one was replaced and when a first one was
// added. A new account has no owner to tell.
func TestBootstrapAdminPassword_LeavesTheAccountOwnerATrace(t *testing.T) {
	for name, tc := range map[string]struct {
		arrange      func(t *testing.T, f *authSettingsFixture)
		wantEntry    bool
		wantReplaced bool
	}{
		"an existing account with no password": {arrange: func(*testing.T, *authSettingsFixture) {}, wantEntry: true},
		"an existing password is replaced": {arrange: func(_ *testing.T, f *authSettingsFixture) {
			f.creds.hashes[authTestRootID] = "earlier"
		}, wantEntry: true, wantReplaced: true},
		"a new account": {arrange: func(t *testing.T, f *authSettingsFixture) {
			require.NoError(t, f.users.DeleteByID(context.Background(), authTestRootID))
		}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newAuthSettingsFixture(t)
			recorded := &bootstrapActivities{}
			f.admin.activityService = recorded
			tc.arrange(t, f)

			rr := f.serve(t, asSetupSession, http.MethodPut, authBootstrapPath, bootstrapBody(authTestRootEmail))

			require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
			if !tc.wantEntry {
				assert.Empty(t, recorded.entries)
				return
			}
			require.Len(t, recorded.entries, 1)
			entry := recorded.entries[0]
			assert.Equal(t, authTestRootID, entry.userID)
			assert.Equal(t, activities.ActivityTypeAuthAdminBootstrap, entry.activityType)
			assert.Equal(t, tc.wantReplaced, entry.metadata["replaced"])
			assert.Equal(t, "setup_session", entry.metadata["source"])
		})
	}
}
