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

// Each account's sign-in state in the admin user list, detail and export
// (#1436), computed in the listing's one query.

func TestIntegrationAdminUsers_SignInState(t *testing.T) {
	ctx := context.Background()
	repo := NewAdminRepository(integrationDB)
	credentials := NewUserCredentialRepository(integrationDB)
	token := "si" + uuid.NewString()[:8]
	exec := func(query string, args ...any) {
		t.Helper()
		_, err := integrationDB.ExecContext(ctx, query, args...)
		require.NoError(t, err)
	}
	identity := func(userID, provider string) {
		exec(`INSERT INTO user_identities (user_id, provider, subject) VALUES ($1, $2, $3)`,
			userID, provider, provider+"-"+uuid.NewString())
	}

	pending := insertAdminListUser(t, token, "pending")
	passwordOnly := insertAdminListUser(t, token, "password")
	require.NoError(t, credentials.Upsert(ctx, passwordOnly, credentialTestHash))
	disabled := insertAdminListUser(t, token, "disabled")
	require.NoError(t, credentials.Upsert(ctx, disabled, credentialTestHash))
	exec(`UPDATE user_credentials SET disabled_at = now() WHERE user_id = $1`, disabled)
	providers := insertAdminListUser(t, token, "providers")
	identity(providers, "google")
	identity(providers, "github")
	both := insertAdminListUser(t, token, "both")
	require.NoError(t, credentials.Upsert(ctx, both, credentialTestHash))
	identity(both, "oidc")
	devOnly := insertAdminListUser(t, token, "devonly")
	identity(devOnly, models.DevIdentityProvider)

	want := map[string]models.AdminUserSignIn{
		pending:      {Methods: []string{}, PendingSetup: true},
		passwordOnly: {Methods: []string{"password"}, HasPassword: true},
		disabled:     {Methods: []string{"password"}, HasPassword: true, PasswordDisabled: true},
		providers:    {Methods: []string{"github", "google"}},
		both:         {Methods: []string{"password", "oidc"}, HasPassword: true},
		// The dev login is not a method an admin manages, but somebody has
		// signed in, so the account is not pending.
		devOnly: {Methods: []string{}},
	}

	list := func(pendingSetup *bool) map[string]models.AdminUserSignIn {
		t.Helper()
		users, total, err := repo.ListUsers(ctx, repositories.AdminUserFilters{
			Search: &token, PendingSetup: pendingSetup, Page: 1, Limit: 50,
		})
		require.NoError(t, err)
		got := map[string]models.AdminUserSignIn{}
		for _, u := range users {
			got[u.ID] = u.SignIn
		}
		require.Len(t, got, total, "the count and the page agree")
		return got
	}

	t.Run("the list", func(t *testing.T) {
		assert.Equal(t, want, list(nil))
	})

	t.Run("the pending filter", func(t *testing.T) {
		yes, no := true, false
		assert.Equal(t, map[string]models.AdminUserSignIn{pending: want[pending]}, list(&yes))
		notPending := list(&no)
		assert.Len(t, notPending, len(want)-1)
		assert.NotContains(t, notPending, pending)
	})

	t.Run("the detail agrees with the list", func(t *testing.T) {
		for id, signIn := range want {
			detail, err := repo.GetUserDetail(ctx, id)
			require.NoError(t, err)
			require.NotNil(t, detail)
			assert.Equal(t, signIn, detail.SignIn, id)
		}
	})

	t.Run("the export stream carries it too", func(t *testing.T) {
		got := map[string]models.AdminUserSignIn{}
		err := repo.StreamUsers(ctx, repositories.AdminUserFilters{Search: &token}, 100,
			func(u models.AdminUserListItem) error {
				got[u.ID] = u.SignIn
				return nil
			})
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
}
