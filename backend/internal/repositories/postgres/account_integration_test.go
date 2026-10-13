//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Real-Postgres coverage for the self-service account API's writes (#1437):
// when a password counts as "changed", disconnecting an identity, and renaming.

// password_changed_at moves only when a new password is stored. Attempts,
// successes and a rehash of the same password bump updated_at and must leave it
// alone, or the account page would show "changed just now" after every sign-in.
func TestIntegrationUserCredential_PasswordChangedAtMovesOnlyOnANewPassword(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserCredentialRepository(integrationDB)
	ctx := context.Background()
	userID := insertTestUser(t)
	require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash))

	// Age the row, so "unchanged" and "set to now" cannot look alike.
	_, err := integrationDB.ExecContext(ctx,
		`UPDATE user_credentials
		    SET password_changed_at = now() - interval '30 days', updated_at = now() - interval '30 days'
		  WHERE user_id = $1`, userID)
	require.NoError(t, err)
	aged, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now().Add(-30*24*time.Hour), aged.PasswordChangedAt, time.Hour)

	unchanged := func(t *testing.T, what string) {
		t.Helper()
		got, getErr := repo.Get(ctx, userID)
		require.NoError(t, getErr)
		assert.True(t, aged.PasswordChangedAt.Equal(got.PasswordChangedAt), "%s moved password_changed_at", what)
		assert.True(t, got.UpdatedAt.After(aged.UpdatedAt), "fixture: %s must have touched the row", what)
	}

	attempt, err := repo.BeginAttempt(ctx, userID, time.Now().UTC(), models.DefaultPasswordLockout)
	require.NoError(t, err)
	assert.True(t, aged.PasswordChangedAt.Equal(attempt.PasswordChangedAt), "BeginAttempt returns the column too")
	unchanged(t, "a sign-in attempt")

	require.NoError(t, repo.RecordSuccess(ctx, userID))
	unchanged(t, "a successful sign-in")

	updated, err := repo.UpdateHash(ctx, userID, credentialTestHash, credentialTestHash+"rehashed")
	require.NoError(t, err)
	require.True(t, updated)
	unchanged(t, "a rehash of the same password")

	require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash+"new"))
	changed, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), changed.PasswordChangedAt, time.Minute, "a new password is a change")
}

func TestIntegrationUserIdentity_DeleteForUser(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	identities := NewUserIdentityRepository(integrationDB)
	owner := insertTestUser(t)
	other := insertTestUser(t)
	idOf := func(t *testing.T, userID, provider string) string {
		t.Helper()
		var id string
		require.NoError(t, integrationDB.QueryRowContext(ctx,
			"SELECT id FROM user_identities WHERE user_id = $1 AND provider = $2", userID, provider).Scan(&id))
		return id
	}
	providersOf := func(t *testing.T, userID string) []string {
		t.Helper()
		got, err := identities.ListByUserID(ctx, userID)
		require.NoError(t, err)
		providers := make([]string, 0, len(got))
		for _, identity := range got {
			providers = append(providers, identity.Provider)
		}
		return providers
	}
	// The owner signs in with Google (which also holds the legacy alias for the
	// same subject) and GitHub. Another user has a Google identity of their own
	// and, to prove the scoping, a legacy alias with the SAME subject string.
	for _, row := range []struct{ user, provider, subject string }{
		{owner, "google", "g-owner"},
		{owner, models.LegacyGoogleIdentityProvider, "g-owner"},
		{owner, "github", "gh-owner"},
		{other, "google", "g-other"},
		{other, models.LegacyGoogleIdentityProvider, "g-other"},
	} {
		require.NoError(t, identities.Upsert(ctx, row.user, row.provider, row.subject))
	}

	t.Run("someone else's identity is not found and nothing is removed", func(t *testing.T) {
		err := identities.DeleteForUser(ctx, owner, idOf(t, other, "google"))

		require.ErrorIs(t, err, repositories.ErrUserIdentityNotFound)
		assert.Len(t, providersOf(t, other), 2)
		assert.Len(t, providersOf(t, owner), 3)
	})

	t.Run("an unknown identity is not found", func(t *testing.T) {
		require.ErrorIs(t, identities.DeleteForUser(ctx, owner, uuid.New().String()), repositories.ErrUserIdentityNotFound)
	})

	t.Run("disconnecting Google removes its legacy alias too, and only the owner's", func(t *testing.T) {
		require.NoError(t, identities.DeleteForUser(ctx, owner, idOf(t, owner, "google")))

		assert.Equal(t, []string{"github"}, providersOf(t, owner),
			"left behind, the alias would keep that Google account able to sign in")
		assert.ElementsMatch(t, []string{"google", models.LegacyGoogleIdentityProvider}, providersOf(t, other))
		_, err := NewUserRepository(integrationDB).GetByIDPSubject(ctx, models.LegacyGoogleIdentityProvider, "g-owner")
		require.ErrorIs(t, err, repositories.ErrUserNotFound)
	})

	t.Run("an identity with no alias is simply removed", func(t *testing.T) {
		require.NoError(t, identities.DeleteForUser(ctx, owner, idOf(t, owner, "github")))

		assert.Empty(t, providersOf(t, owner))
	})

	t.Run("removing it twice is not found", func(t *testing.T) {
		gone := idOf(t, other, "google")
		require.NoError(t, identities.DeleteForUser(ctx, other, gone))
		require.ErrorIs(t, identities.DeleteForUser(ctx, other, gone), repositories.ErrUserIdentityNotFound)
	})
}

func TestIntegrationUser_UpdateName(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	users := NewUserRepository(integrationDB)
	userID := insertTestUser(t)
	bystander := insertTestUser(t)
	before, err := users.GetByID(ctx, userID)
	require.NoError(t, err)

	require.NoError(t, users.UpdateName(ctx, userID, "Ada Lovelace"))

	after, err := users.GetByID(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, "Ada Lovelace", after.Name)
	assert.Equal(t, before.Email, after.Email)
	assert.Equal(t, before.Version+1, after.Version,
		"the version moves, so a sign-in that loaded the old row cannot write the old name back")
	untouched, err := users.GetByID(ctx, bystander)
	require.NoError(t, err)
	assert.Equal(t, "Integration Test User", untouched.Name)

	// A sign-in that read the row BEFORE the rename must not undo it.
	before.Name = "Stale Name From An Old Read"
	require.Error(t, users.Update(ctx, before), "the stale write is rejected by the version check")
	current, err := users.GetByID(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, "Ada Lovelace", current.Name)

	require.ErrorIs(t, users.UpdateName(ctx, uuid.New().String(), "Nobody"), repositories.ErrUserNotFound)
}
