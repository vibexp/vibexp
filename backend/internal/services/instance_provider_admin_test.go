package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/testutils/memauthsettings"
)

// Provider-sourced instance admins (#1644).

type providerAdminFixture struct {
	svc    *InstanceAdminService
	users  *fakeInstanceAdminUsers
	grants *fakeInstanceAdminGrants
	store  *memauthsettings.Store
	events *auditEvents
	// provider is an enabled oidc provider with a role mapping.
	provider *models.InstanceAuthProvider
}

func newProviderAdminFixture(t *testing.T, grantedIDs ...string) *providerAdminFixture {
	t.Helper()
	f := &providerAdminFixture{
		grants: newFakeInstanceAdminGrants(grantedIDs...),
		store:  memauthsettings.New(),
		events: &auditEvents{},
	}
	_, f.users = newInstanceAdminFixture(f.grants)
	f.svc = NewInstanceAdminService([]string{"root@example.com"}, f.grants, f.users, slog.New(slog.DiscardHandler),
		WithProviderAdmins(f.store.ProviderAdmins(), f.events))
	f.provider = f.addProvider(t, "keycloak", true)
	return f
}

func (f *providerAdminFixture) addProvider(t *testing.T, slug string, enabled bool) *models.InstanceAuthProvider {
	t.Helper()
	issuer, claim := "https://idp.example.com", "groups"
	p := &models.InstanceAuthProvider{
		Type: models.InstanceAuthProviderOIDC, Slug: slug, DisplayName: slug, Enabled: enabled,
		ClientID: "client", IssuerURL: &issuer, RolesClaim: &claim, AdminValues: []string{"admins"},
	}
	require.NoError(t, f.store.Providers().Create(context.Background(), p, nil, nil))
	return p
}

func (f *providerAdminFixture) user(id string) *models.User { return f.users.users[id] }

func (f *providerAdminFixture) isAdmin(t *testing.T, id string) bool {
	t.Helper()
	ok, err := f.svc.IsInstanceAdmin(context.Background(), f.user(id))
	require.NoError(t, err)
	return ok
}

func (f *providerAdminFixture) promote(t *testing.T, id string) {
	t.Helper()
	matched := "admins"
	require.NoError(t, f.svc.SyncProviderAdmin(context.Background(), f.user(id), f.provider.ID, f.provider.Slug,
		&matched))
}

func (f *providerAdminFixture) demote(t *testing.T, id string) {
	t.Helper()
	require.NoError(t, f.svc.SyncProviderAdmin(context.Background(), f.user(id), f.provider.ID, f.provider.Slug, nil))
}

func (f *providerAdminFixture) takeEvents() []auditEventView {
	f.events.mu.Lock()
	defer f.events.mu.Unlock()
	out := make([]auditEventView, 0, len(f.events.got))
	for _, e := range f.events.got {
		v := auditEventView{op: e.Operation, resourceID: e.ResourceID, metadata: e.Metadata}
		if e.Actor != nil && e.Actor.UserID != nil {
			v.actor = *e.Actor.UserID
		}
		out = append(out, v)
	}
	f.events.got = nil
	return out
}

type auditEventView struct {
	op         models.AuditOperation
	resourceID string
	actor      string
	metadata   map[string]any
}

func TestSyncProviderAdmin_PromotesAndDemotesAuditingOnlyTransitions(t *testing.T) {
	f := newProviderAdminFixture(t)
	require.False(t, f.isAdmin(t, iaPlainID))

	f.promote(t, iaPlainID)
	assert.True(t, f.isAdmin(t, iaPlainID), "a matching sign-in makes the user an admin")
	assert.Equal(t, []auditEventView{{
		op: models.AuditOpAuthInstanceAdminPromoted, resourceID: iaPlainID, actor: iaPlainID,
		metadata: map[string]any{"provider_slug": "keycloak", "matched_value": "admins"},
	}}, f.takeEvents())

	f.promote(t, iaPlainID)
	assert.Empty(t, f.takeEvents(), "an unchanged result audits nothing")

	f.demote(t, iaPlainID)
	assert.False(t, f.isAdmin(t, iaPlainID), "a sign-in without the value removes the status")
	assert.Equal(t, []auditEventView{{
		op: models.AuditOpAuthInstanceAdminDemoted, resourceID: iaPlainID, actor: iaPlainID,
		metadata: map[string]any{"provider_slug": "keycloak", "reason": ProviderAdminDemotedNoMatch},
	}}, f.takeEvents())

	f.demote(t, iaPlainID)
	assert.Empty(t, f.takeEvents(), "demoting a non-admin audits nothing")
}

func TestSyncProviderAdmin_RootAndManualAdminsKeepTheirStatus(t *testing.T) {
	f := newProviderAdminFixture(t, iaDBAdminID)
	for _, id := range []string{iaRootID, iaDBAdminID} {
		f.promote(t, id)
		f.demote(t, id)
		assert.True(t, f.isAdmin(t, id), "%s keeps its own source when the claim is missing", id)
	}
	ok, err := f.grants.IsGranted(context.Background(), iaDBAdminID)
	require.NoError(t, err)
	assert.True(t, ok, "the mapping never touches a manual grant")
}

func TestIsInstanceAdmin_ProviderSourced(t *testing.T) {
	t.Run("a row of a disabled provider grants nothing", func(t *testing.T) {
		f := newProviderAdminFixture(t)
		disabled := f.addProvider(t, "old-idp", false)
		matched := "admins"
		require.NoError(t, f.svc.SyncProviderAdmin(context.Background(), f.user(iaPlainID), disabled.ID,
			disabled.Slug, &matched))
		assert.False(t, f.isAdmin(t, iaPlainID))
	})
	t.Run("a suspended user is never an admin", func(t *testing.T) {
		f := newProviderAdminFixture(t)
		f.promote(t, iaSuspendedID)
		assert.False(t, f.isAdmin(t, iaSuspendedID))
	})
	t.Run("a read error fails closed", func(t *testing.T) {
		f := newProviderAdminFixture(t)
		f.store.Err = errors.New("db down")
		ok, err := f.svc.IsInstanceAdmin(context.Background(), f.user(iaPlainID))
		require.Error(t, err)
		assert.False(t, ok)
	})
	t.Run("without the source nothing is read or written", func(t *testing.T) {
		svc, users := newInstanceAdminFixture(newFakeInstanceAdminGrants())
		matched := "admins"
		require.NoError(t, svc.SyncProviderAdmin(context.Background(), users.users[iaPlainID], "p", "s", &matched))
		require.NoError(t, svc.RemoveProviderAdmins(context.Background(), "p", "s", ProviderAdminDemotedProviderDeleted))
		ok, err := svc.IsInstanceAdmin(context.Background(), users.users[iaPlainID])
		require.NoError(t, err)
		assert.False(t, ok)
	})
}

func TestGrantInstanceAdmin_RefusesAProviderSourcedAdmin(t *testing.T) {
	f := newProviderAdminFixture(t)
	f.promote(t, iaPlainID)

	_, err := f.svc.GrantInstanceAdmin(context.Background(), iaRootID, iaPlainID)

	var sourced *ErrInstanceAdminTargetIsProviderSourced
	require.ErrorAs(t, err, &sourced)
	assert.Equal(t, "keycloak", sourced.ProviderSlug)
	assert.Contains(t, err.Error(), `"keycloak"`)
	assert.Empty(t, f.grants.actors, "nothing was granted")
}

func TestRevokeInstanceAdmin_ProviderSourced(t *testing.T) {
	t.Run("refused without a manual grant", func(t *testing.T) {
		f := newProviderAdminFixture(t)
		f.promote(t, iaPlainID)

		err := f.svc.RevokeInstanceAdmin(context.Background(), iaRootID, iaPlainID)

		var sourced *ErrInstanceAdminTargetIsProviderSourced
		require.ErrorAs(t, err, &sourced)
		assert.True(t, f.isAdmin(t, iaPlainID))
	})
	t.Run("with a manual grant only the grant goes", func(t *testing.T) {
		f := newProviderAdminFixture(t, iaPlainID)
		f.promote(t, iaPlainID)

		require.NoError(t, f.svc.RevokeInstanceAdmin(context.Background(), iaRootID, iaPlainID))

		ok, err := f.grants.IsGranted(context.Background(), iaPlainID)
		require.NoError(t, err)
		assert.False(t, ok, "the manual grant is revoked")
		assert.True(t, f.isAdmin(t, iaPlainID), "the mapping keeps the user an admin")
	})
	t.Run("a read error is returned", func(t *testing.T) {
		f := newProviderAdminFixture(t, iaPlainID)
		f.store.Err = errors.New("db down")
		err := f.svc.RevokeInstanceAdmin(context.Background(), iaRootID, iaPlainID)
		require.Error(t, err)
		assert.True(t, f.grants.granted[iaPlainID], "nothing was revoked")
	})
}

func TestRemoveProviderAdmins_AuditsEachDemotion(t *testing.T) {
	f := newProviderAdminFixture(t)
	other := f.addProvider(t, "other", true)
	f.promote(t, iaPlainID)
	f.promote(t, iaDBAdminID)
	matched := "admins"
	require.NoError(t, f.svc.SyncProviderAdmin(context.Background(), f.user(iaRoot2ID), other.ID, other.Slug, &matched))
	f.takeEvents()

	require.NoError(t, f.svc.RemoveProviderAdmins(context.Background(), f.provider.ID, f.provider.Slug,
		ProviderAdminDemotedMappingCleared))

	got := f.takeEvents()
	require.Len(t, got, 2)
	ids := []string{}
	for _, e := range got {
		assert.Equal(t, models.AuditOpAuthInstanceAdminDemoted, e.op)
		assert.Empty(t, e.actor, "the actor is the admin in the request context")
		assert.Equal(t, map[string]any{"provider_slug": "keycloak", "reason": ProviderAdminDemotedMappingCleared},
			e.metadata)
		ids = append(ids, e.resourceID)
	}
	assert.ElementsMatch(t, []string{iaPlainID, iaDBAdminID}, ids)
	assert.False(t, f.isAdmin(t, iaPlainID))
	rows, err := f.store.ProviderAdmins().ListByUser(context.Background(), iaRoot2ID)
	require.NoError(t, err)
	assert.Len(t, rows, 1, "another provider's rows are untouched")
}

func TestSyncProviderAdmin_WriteErrorsAreReturned(t *testing.T) {
	f := newProviderAdminFixture(t)
	f.store.Err = errors.New("db down")
	matched := "admins"
	require.Error(t, f.svc.SyncProviderAdmin(context.Background(), f.user(iaPlainID), f.provider.ID, "keycloak",
		&matched))
	require.Error(t, f.svc.SyncProviderAdmin(context.Background(), f.user(iaPlainID), f.provider.ID, "keycloak", nil))
	require.Error(t, f.svc.RemoveProviderAdmins(context.Background(), f.provider.ID, "keycloak",
		ProviderAdminDemotedProviderDeleted))
	assert.Empty(t, f.takeEvents())
}
