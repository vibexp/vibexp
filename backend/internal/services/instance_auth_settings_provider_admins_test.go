package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Provider-sourced admins in the auth settings (#1644): clearing a provider's
// role mapping or deleting it removes its admins, a change that would take the
// caller's own admin status away is a lockout risk, and the admin list reports
// them.

// mappedProvider creates an enabled oidc provider with slug whose role mapping
// makes "platform-admins" an admin, plus a second enabled provider so taking
// the first away never trips the no_enabled_provider guard.
func (f *authSettingsFixture) mappedProvider(t *testing.T, slug string) *models.InstanceAuthProvider {
	t.Helper()
	req := oidcCreate(slug)
	claim := "groups"
	req.RolesClaim, req.AdminValues = &claim, []string{"platform-admins", "ops"}
	f.create(t, req)
	if f.store.Provider("spare") == nil {
		f.create(t, oidcCreate("spare"))
	}
	return f.store.Provider(slug)
}

// promote records userID as an admin of provider through its mapping.
func (f *authSettingsFixture) promote(t *testing.T, userID string, provider *models.InstanceAuthProvider) {
	t.Helper()
	user, err := f.users.GetByID(context.Background(), userID)
	require.NoError(t, err)
	matched := "platform-admins"
	require.NoError(t, f.admins.SyncProviderAdmin(context.Background(), user, provider.ID, provider.Slug, &matched))
	f.events.got = nil
}

// memberIsAdmin reports whether the member is an instance admin.
func (f *authSettingsFixture) memberIsAdmin(t *testing.T) bool {
	t.Helper()
	user, err := f.users.GetByID(context.Background(), authSettingsMemberID)
	require.NoError(t, err)
	ok, err := f.admins.IsInstanceAdmin(context.Background(), user)
	require.NoError(t, err)
	return ok
}

// demotions returns the user ids of the recorded demotions with reason.
func (f *authSettingsFixture) demotions(t *testing.T, reason string) []string {
	t.Helper()
	ids := []string{}
	for _, e := range f.events.got {
		require.Equal(t, models.AuditOpAuthInstanceAdminDemoted, e.Operation)
		assert.Equal(t, reason, e.Metadata["reason"])
		assert.Equal(t, "okta", e.Metadata["provider_slug"])
		ids = append(ids, e.ResourceID)
	}
	return ids
}

func withMapping(req models.UpdateInstanceAuthProviderRequest, claim *string, values []string) models.UpdateInstanceAuthProviderRequest {
	req.RolesClaim, req.AdminValues = claim, &values
	return req
}

func TestInstanceAuthSettings_ClearingTheMappingRemovesItsAdmins(t *testing.T) {
	f := newAuthSettingsFixture(t)
	p := f.mappedProvider(t, "okta")
	f.promote(t, authSettingsMemberID, p)
	f.promote(t, authSettingsDBAdminID, p)

	_, err := f.svc.UpdateProvider(context.Background(), authSettingsRootID, p.ID,
		withMapping(updateOf(*p, f.store.Version()), nil, []string{}))
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{authSettingsMemberID, authSettingsDBAdminID},
		f.demotions(t, ProviderAdminDemotedMappingCleared))
	assert.False(t, f.memberIsAdmin(t))
	assert.Nil(t, f.store.Provider("okta").RolesClaim)
}

func TestInstanceAuthSettings_KeepingTheMappingKeepsItsAdmins(t *testing.T) {
	f := newAuthSettingsFixture(t)
	p := f.mappedProvider(t, "okta")
	f.promote(t, authSettingsMemberID, p)

	// A save that omits the mapping keeps it; one that changes admin_values
	// takes effect at each admin's next sign-in.
	_, err := f.svc.UpdateProvider(context.Background(), authSettingsRootID, p.ID, updateOf(*p, f.store.Version()))
	require.NoError(t, err)
	claim := "groups"
	_, err = f.svc.UpdateProvider(context.Background(), authSettingsRootID, p.ID,
		withMapping(updateOf(*p, f.store.Version()), &claim, []string{"other"}))
	require.NoError(t, err)

	assert.Empty(t, f.events.got)
	assert.True(t, f.memberIsAdmin(t))
}

func TestInstanceAuthSettings_DeletingTheProviderRemovesItsAdmins(t *testing.T) {
	f := newAuthSettingsFixture(t)
	p := f.mappedProvider(t, "okta")
	f.promote(t, authSettingsMemberID, p)

	require.NoError(t, f.svc.DeleteProvider(context.Background(), authSettingsRootID, p.ID,
		models.DeleteInstanceAuthProviderRequest{}))

	assert.Equal(t, []string{authSettingsMemberID}, f.demotions(t, ProviderAdminDemotedProviderDeleted))
	assert.False(t, f.memberIsAdmin(t))
}

func TestInstanceAuthSettings_OwnAdminStatusGuard(t *testing.T) {
	ctx := context.Background()
	claim := "groups"
	type change func(f *authSettingsFixture, p *models.InstanceAuthProvider, lockout models.InstanceAuthLockoutContext) error
	update := func(edit func(*models.UpdateInstanceAuthProviderRequest)) change {
		return func(f *authSettingsFixture, p *models.InstanceAuthProvider, lockout models.InstanceAuthLockoutContext) error {
			req := updateOf(*p, f.store.Version())
			req.Lockout = lockout
			edit(&req)
			_, err := f.svc.UpdateProvider(ctx, authSettingsMemberID, p.ID, req)
			return err
		}
	}
	clearMapping := update(func(r *models.UpdateInstanceAuthProviderRequest) {
		*r = withMapping(*r, nil, []string{})
	})
	dropValue := update(func(r *models.UpdateInstanceAuthProviderRequest) {
		*r = withMapping(*r, &claim, []string{"ops"})
	})
	keepValue := update(func(r *models.UpdateInstanceAuthProviderRequest) {
		*r = withMapping(*r, &claim, []string{"platform-admins"})
	})
	disable := update(func(r *models.UpdateInstanceAuthProviderRequest) { r.Enabled = false })
	deleteIt := func(f *authSettingsFixture, p *models.InstanceAuthProvider, lockout models.InstanceAuthLockoutContext) error {
		return f.svc.DeleteProvider(ctx, authSettingsMemberID, p.ID, models.DeleteInstanceAuthProviderRequest{Lockout: lockout})
	}

	for name, c := range map[string]change{
		"clearing the mapping": clearMapping, "dropping the matched value": dropValue,
		"disabling the provider": disable, "deleting the provider": deleteIt,
	} {
		t.Run(name+" is refused", func(t *testing.T) {
			f := newAuthSettingsFixture(t)
			p := f.mappedProvider(t, "okta")
			f.promote(t, authSettingsMemberID, p)

			err := c(f, p, models.InstanceAuthLockoutContext{})

			var lockout *ErrAuthLockoutRisk
			require.ErrorAs(t, err, &lockout)
			assert.Equal(t, AuthLockoutOwnAdminStatus, lockout.Reason)
			assert.Contains(t, err.Error(), "your own instance admin status")
			assert.True(t, f.memberIsAdmin(t), "nothing changed")
			assert.NotNil(t, f.store.Provider("okta"), "nothing changed")
		})
		t.Run(name+" applies when confirmed", func(t *testing.T) {
			f := newAuthSettingsFixture(t)
			p := f.mappedProvider(t, "okta")
			f.promote(t, authSettingsMemberID, p)

			require.NoError(t, c(f, p, models.InstanceAuthLockoutContext{Confirmed: true}))
		})
	}

	t.Run("keeping the matched value is not a risk", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		p := f.mappedProvider(t, "okta")
		f.promote(t, authSettingsMemberID, p)
		require.NoError(t, keepValue(f, p, models.InstanceAuthLockoutContext{}))
	})
	t.Run("another admin source is not a risk", func(t *testing.T) {
		sources := map[string]func(t *testing.T, f *authSettingsFixture){
			"a manual grant": func(t *testing.T, f *authSettingsFixture) {
				_, err := f.store.Grants().Grant(ctx, authSettingsMemberID, nil)
				require.NoError(t, err)
			},
			"another provider's mapping": func(t *testing.T, f *authSettingsFixture) {
				f.promote(t, authSettingsMemberID, f.mappedProvider(t, "auth0"))
			},
		}
		for name, add := range sources {
			t.Run(name, func(t *testing.T) {
				f := newAuthSettingsFixture(t)
				p := f.mappedProvider(t, "okta")
				f.promote(t, authSettingsMemberID, p)
				add(t, f)
				require.NoError(t, clearMapping(f, p, models.InstanceAuthLockoutContext{}))
			})
		}
	})
	t.Run("a root admin and a setup session are not at risk", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		p := f.mappedProvider(t, "okta")
		f.promote(t, authSettingsRootID, p)
		_, err := f.svc.UpdateProvider(ctx, authSettingsRootID, p.ID,
			withMapping(updateOf(*p, f.store.Version()), nil, []string{}))
		require.NoError(t, err)

		p = f.mappedProvider(t, "auth0")
		f.promote(t, authSettingsMemberID, p)
		require.NoError(t, f.svc.DeleteProvider(ctx, "", p.ID, models.DeleteInstanceAuthProviderRequest{}))
	})
	t.Run("a caller who is not a provider admin of it is not at risk", func(t *testing.T) {
		f := newAuthSettingsFixture(t)
		p := f.mappedProvider(t, "okta")
		require.NoError(t, clearMapping(f, p, models.InstanceAuthLockoutContext{}))
	})
}

func TestInstanceAuthSettings_ListAdmins_ReportsProviderAdmins(t *testing.T) {
	f := newAuthSettingsFixture(t)
	p := f.mappedProvider(t, "okta")
	f.promote(t, authSettingsMemberID, p)

	list, err := f.svc.ListAdmins(context.Background())
	require.NoError(t, err)

	require.Len(t, list.ProviderAdmins, 1)
	got := list.ProviderAdmins[0]
	assert.Equal(t, authSettingsMemberID, got.UserID)
	assert.Equal(t, authSettingsMemberEmail, got.Email)
	assert.Equal(t, "Ada", got.Name)
	assert.Equal(t, p.ID, got.ProviderID)
	assert.Equal(t, "okta", got.ProviderSlug)
	assert.Equal(t, "platform-admins", got.MatchedValue)
	assert.Empty(t, list.Admins)
}
