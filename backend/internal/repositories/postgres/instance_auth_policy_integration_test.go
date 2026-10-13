//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Real-Postgres coverage for the instance authentication policy (#1429): the
// singleton row, its compare-and-set, the audit entry and shared version bump
// every write makes in one transaction, and the database's own backstops. The
// table is global to the shared test database, so no test runs in parallel.

func resetInstanceAuthPolicy(t *testing.T) {
	t.Helper()
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, "instance_auth_policy")
}

func TestIntegrationInstanceAuthPolicy_Lifecycle(t *testing.T) {
	resetInstanceAuthPolicy(t)
	repo := NewInstanceAuthPolicyRepository(integrationDB)
	ctx := context.Background()
	actor := insertTestUser(t)
	start := authSettingsVersion(t)

	_, err := repo.Get(ctx)
	assert.ErrorIs(t, err, repositories.ErrInstanceAuthPolicyNotFound,
		"the migration seeds no row: no row = the default policy")

	first := &models.InstanceAuthPolicy{PasswordLoginEnabled: true, RegistrationMode: models.RegistrationModeOpen}
	require.NoError(t, repo.UpsertAudited(ctx, first, &actor, nil))
	assert.Equal(t, int64(1), first.Version)
	assert.False(t, first.CreatedAt.IsZero())
	assert.Equal(t, start+1, authSettingsVersion(t), "a write bumps the shared version in the same transaction")

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.True(t, got.PasswordLoginEnabled)
	assert.Equal(t, models.RegistrationModeOpen, got.RegistrationMode)
	require.NotNil(t, got.UpdatedBy)
	assert.Equal(t, actor, *got.UpdatedBy)

	second := &models.InstanceAuthPolicy{RegistrationMode: models.RegistrationModeInviteOnly}
	stale := int64(7)
	assert.ErrorIs(t, repo.UpsertAudited(ctx, second, &actor, &stale), repositories.ErrInstanceSettingsVersionConflict)
	assert.Equal(t, start+1, authSettingsVersion(t), "a conflict does not bump the version")
	unchanged, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.True(t, unchanged.PasswordLoginEnabled, "a conflict writes nothing")

	expected := int64(1)
	require.NoError(t, repo.UpsertAudited(ctx, second, &actor, &expected))
	assert.Equal(t, int64(2), second.Version)
	assert.True(t, second.CreatedAt.Equal(first.CreatedAt), "a replacement keeps the row's creation time")
	assert.Equal(t, start+2, authSettingsVersion(t))
	got, err = repo.Get(ctx)
	require.NoError(t, err)
	assert.False(t, got.PasswordLoginEnabled)
	assert.Equal(t, models.RegistrationModeInviteOnly, got.RegistrationMode)

	entries := authAuditEntries(t, models.InstanceSettingAuthPolicy) // newest first
	require.Len(t, entries, 2, "one audit entry per successful write, none for the conflict")
	latest, oldest := entries[0], entries[1]
	assert.Equal(t, models.InstanceSettingsAuditActionUpsert, latest.Action)
	require.NotNil(t, latest.ActorUserID)
	assert.Equal(t, actor, *latest.ActorUserID)
	before, after := decodeAuditDoc(t, latest.Before), decodeAuditDoc(t, latest.After)
	assert.Equal(t, true, before["password_login_enabled"])
	assert.Equal(t, models.RegistrationModeOpen, before["registration_mode"])
	assert.Equal(t, false, after["password_login_enabled"])
	assert.Equal(t, models.RegistrationModeInviteOnly, after["registration_mode"])
	assert.Nil(t, oldest.Before, "the first save has nothing before it")
}

// Expected version 0 (InstanceSettingsNoStoredVersion) saves only while no row
// is stored.
func TestIntegrationInstanceAuthPolicy_ExpectNothingStored(t *testing.T) {
	resetInstanceAuthPolicy(t)
	repo := NewInstanceAuthPolicyRepository(integrationDB)
	ctx := context.Background()
	none := repositories.InstanceSettingsNoStoredVersion
	fixture := func() *models.InstanceAuthPolicy {
		return &models.InstanceAuthPolicy{PasswordLoginEnabled: true, RegistrationMode: models.RegistrationModeOpen}
	}

	require.NoError(t, repo.UpsertAudited(ctx, fixture(), nil, &none))
	assert.ErrorIs(t, repo.UpsertAudited(ctx, fixture(), nil, &none), repositories.ErrInstanceSettingsVersionConflict)
	assert.Len(t, authAuditEntries(t, models.InstanceSettingAuthPolicy), 1)
}

// The database refuses what the validator is there to catch first, and a
// refused write leaves no row, no audit entry and no version bump.
func TestIntegrationInstanceAuthPolicy_RejectedWritesRollBack(t *testing.T) {
	resetInstanceAuthPolicy(t)
	repo := NewInstanceAuthPolicyRepository(integrationDB)
	ctx := context.Background()
	start := authSettingsVersion(t)

	err := repo.UpsertAudited(ctx, &models.InstanceAuthPolicy{RegistrationMode: "closed"}, nil, nil)
	require.Error(t, err, "the CHECK on registration_mode is the storage backstop")

	ghost := uuid.New().String()
	err = repo.UpsertAudited(ctx,
		&models.InstanceAuthPolicy{RegistrationMode: models.RegistrationModeOpen}, &ghost, nil)
	assert.ErrorIs(t, err, repositories.ErrUserNotFound, "an actor naming no user is reported as such")

	_, err = repo.Get(ctx)
	assert.ErrorIs(t, err, repositories.ErrInstanceAuthPolicyNotFound)
	assert.Empty(t, authAuditEntries(t, models.InstanceSettingAuthPolicy))
	assert.Equal(t, start, authSettingsVersion(t))
}

// Deleting the admin who last saved the policy keeps the policy.
func TestIntegrationInstanceAuthPolicy_SurvivesItsAuthorsDeletion(t *testing.T) {
	resetInstanceAuthPolicy(t)
	repo := NewInstanceAuthPolicyRepository(integrationDB)
	ctx := context.Background()
	actor := insertTestUser(t)
	require.NoError(t, repo.UpsertAudited(ctx,
		&models.InstanceAuthPolicy{PasswordLoginEnabled: true, RegistrationMode: models.RegistrationModeInviteOnly},
		&actor, nil))

	_, err := integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", actor)
	require.NoError(t, err)

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Nil(t, got.UpdatedBy)
	assert.True(t, got.PasswordLoginEnabled)
}
