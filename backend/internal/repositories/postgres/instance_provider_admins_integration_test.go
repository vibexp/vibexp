//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Provider-sourced instance admins (#1644) against real Postgres: the upsert's
// inserted flag, the reads leaving out a disabled provider, the delete by
// provider, and the cascades from a deleted user or provider.

func createProviderAdminProvider(t *testing.T, slug string, enabled bool) *models.InstanceAuthProvider {
	t.Helper()
	p := oidcProviderFixture(slug, 0)
	p.Enabled = enabled
	claim := "groups"
	p.RolesClaim, p.AdminValues = &claim, []string{"platform-admins", "ops"}
	ciphertext := providerCiphertext
	p.ClientSecretEncrypted = &ciphertext
	require.NoError(t, NewInstanceAuthProviderRepository(integrationDB).Create(context.Background(), p, nil, nil))
	return p
}

func TestIntegrationInstanceProviderAdmins_Lifecycle(t *testing.T) {
	resetInstanceAuthSettings(t)
	repo := NewInstanceProviderAdminRepository(integrationDB)
	ctx := context.Background()
	keycloak := createProviderAdminProvider(t, "keycloak", true)
	disabled := createProviderAdminProvider(t, "old-idp", false)
	alice, bob := insertTestUser(t), insertTestUser(t)

	isAdmin, err := repo.IsProviderAdmin(ctx, alice)
	require.NoError(t, err)
	assert.False(t, isAdmin)

	inserted, err := repo.Upsert(ctx, alice, keycloak.ID, "platform-admins")
	require.NoError(t, err)
	assert.True(t, inserted)
	inserted, err = repo.Upsert(ctx, alice, keycloak.ID, "ops")
	require.NoError(t, err)
	assert.False(t, inserted, "a second evaluation refreshes the row")
	inserted, err = repo.Upsert(ctx, bob, disabled.ID, "platform-admins")
	require.NoError(t, err)
	assert.True(t, inserted)

	isAdmin, err = repo.IsProviderAdmin(ctx, alice)
	require.NoError(t, err)
	assert.True(t, isAdmin)
	isAdmin, err = repo.IsProviderAdmin(ctx, bob)
	require.NoError(t, err)
	assert.False(t, isAdmin, "a disabled provider's row grants nothing")

	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1, "a disabled provider's rows are left out")
	assert.Equal(t, alice, list[0].UserID)
	assert.Equal(t, keycloak.ID, list[0].ProviderID)
	assert.Equal(t, "keycloak", list[0].ProviderSlug)
	assert.Equal(t, "ops", list[0].MatchedValue)
	assert.False(t, list[0].EvaluatedAt.IsZero())

	byUser, err := repo.ListByUser(ctx, alice)
	require.NoError(t, err)
	require.Len(t, byUser, 1)
	byUser, err = repo.ListByUser(ctx, bob)
	require.NoError(t, err)
	assert.Empty(t, byUser)

	deleted, err := repo.Delete(ctx, alice, keycloak.ID)
	require.NoError(t, err)
	assert.True(t, deleted)
	deleted, err = repo.Delete(ctx, alice, keycloak.ID)
	require.NoError(t, err)
	assert.False(t, deleted)
}

func TestIntegrationInstanceProviderAdmins_DeleteByProvider(t *testing.T) {
	resetInstanceAuthSettings(t)
	repo := NewInstanceProviderAdminRepository(integrationDB)
	ctx := context.Background()
	keycloak, other := createProviderAdminProvider(t, "keycloak", true), createProviderAdminProvider(t, "other", true)
	alice, bob := insertTestUser(t), insertTestUser(t)
	for _, userID := range []string{alice, bob} {
		_, err := repo.Upsert(ctx, userID, keycloak.ID, "platform-admins")
		require.NoError(t, err)
	}
	_, err := repo.Upsert(ctx, alice, other.ID, "platform-admins")
	require.NoError(t, err)

	ids, err := repo.UserIDsByProvider(ctx, keycloak.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{alice, bob}, ids)

	removed, err := repo.DeleteByProvider(ctx, keycloak.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{alice, bob}, removed)

	removed, err = repo.DeleteByProvider(ctx, keycloak.ID)
	require.NoError(t, err)
	assert.Empty(t, removed)
	rows, err := repo.ListByUser(ctx, alice)
	require.NoError(t, err)
	require.Len(t, rows, 1, "another provider's rows stay")
}

func TestIntegrationInstanceProviderAdmins_Cascades(t *testing.T) {
	resetInstanceAuthSettings(t)
	repo := NewInstanceProviderAdminRepository(integrationDB)
	ctx := context.Background()
	keycloak := createProviderAdminProvider(t, "keycloak", true)
	alice, bob := insertTestUser(t), insertTestUser(t)
	for _, userID := range []string{alice, bob} {
		_, err := repo.Upsert(ctx, userID, keycloak.ID, "platform-admins")
		require.NoError(t, err)
	}

	_, err := integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", alice)
	require.NoError(t, err)
	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1, "deleting the user removes its row")
	assert.Equal(t, bob, list[0].UserID)

	require.NoError(t, NewInstanceAuthProviderRepository(integrationDB).Delete(ctx, keycloak.ID, nil, nil))
	var left int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT count(*) FROM instance_provider_admins").Scan(&left))
	assert.Zero(t, left, "deleting the provider removes its rows")
}

// TestIntegrationInstanceProviderAdmins_ClearedMappingGrantsNothing pins that a
// row outliving its provider's mapping (written by a sign-in in flight while
// the mapping was cleared) is no admin grant and is not listed.
func TestIntegrationInstanceProviderAdmins_ClearedMappingGrantsNothing(t *testing.T) {
	resetInstanceAuthSettings(t)
	repo := NewInstanceProviderAdminRepository(integrationDB)
	providers := NewInstanceAuthProviderRepository(integrationDB)
	ctx := context.Background()
	keycloak := createProviderAdminProvider(t, "keycloak", true)
	alice := insertTestUser(t)

	keycloak.RolesClaim, keycloak.AdminValues = nil, nil
	require.NoError(t, providers.Update(ctx, keycloak, nil, nil))
	_, err := repo.Upsert(ctx, alice, keycloak.ID, "platform-admins")
	require.NoError(t, err)

	isAdmin, err := repo.IsProviderAdmin(ctx, alice)
	require.NoError(t, err)
	assert.False(t, isAdmin)
	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, list)
	byUser, err := repo.ListByUser(ctx, alice)
	require.NoError(t, err)
	assert.Empty(t, byUser)
}
