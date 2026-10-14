//go:build integration

package postgres

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

// Linking a provider sign-in to an existing account by verified email (#1430),
// through the real AuthService over the real repositories: what the rows look
// like afterwards is the point, and mocks cannot show that.
//
// ProvisionFromClaims is the exported entry to the resolution the web callback
// shares; it needs no provider registry or allowlist.

func newLinkingAuthService() *services.AuthService {
	return services.NewAuthService(
		NewUserRepository(integrationDB), NewUserIdentityRepository(integrationDB),
		nil, nil, slog.New(slog.DiscardHandler), openAllowlist{})
}

// openAllowlist is an instance with no access allowlist: everyone is admitted.
type openAllowlist struct {
	services.AccessAllowlistResolver
}

func (openAllowlist) IsEmailAllowed(context.Context, string) (allowed, active bool, err error) {
	return true, false, nil
}

func TestIntegrationLinking_AdminCreatedAccountCompletesFirstSignIn(t *testing.T) {
	resetSearchTables(t)
	ctx := context.Background()
	users := NewUserRepository(integrationDB)
	identities := NewUserIdentityRepository(integrationDB)

	// What admin user creation stores: an email and a name, no identity.
	local := uuid.New().String()
	account := &models.User{Email: "ada-" + local + "@example.com", Name: "Ada (admin-given)"}
	require.NoError(t, users.Create(ctx, account))
	teamID := insertTestTeam(t, account.ID) // the workspace provisioning gave them

	signedIn, err := newLinkingAuthService().ProvisionFromClaims(ctx, "corp-sso", &idp.Claims{
		Subject: "sub-ada", Email: "Ada-" + local + "@Example.com", Name: "Provider Ada",
		Picture: "https://provider.example/ada.png", EmailVerified: true,
	})

	require.NoError(t, err, "before #1430 this collided on users_email_key")
	assert.Equal(t, account.ID, signedIn.ID, "the sign-in lands in the pre-created account")
	assert.Equal(t, 1, countRows(t, integrationDB.DB, "SELECT count(*) FROM users"), "no second account")
	assert.Equal(t, 1, countRows(t, integrationDB.DB, "SELECT count(*) FROM teams WHERE id = $1 AND owner_id = $2",
		teamID, account.ID), "the workspace is intact")

	stored, err := users.GetByID(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, account.Email, stored.Email)
	assert.Equal(t, "Ada (admin-given)", stored.Name, "the admin-given name is kept")
	require.NotNil(t, stored.AvatarURL, "an empty avatar is filled")
	require.NotNil(t, stored.EmailVerifiedAt, "the provider verified the address")
	assert.EqualValues(t, 1, stored.SessionEpoch, "claiming the account ends any earlier session")
	assert.Equal(t, stored.SessionEpoch, signedIn.SessionEpoch,
		"the returned user carries the new epoch, or the session minted for this sign-in would be dead on arrival")

	got, err := identities.ListByUserID(ctx, account.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "corp-sso", got[0].Provider)
	assert.Equal(t, "sub-ada", got[0].Subject)

	// The next sign-in resolves by identity, not by email.
	again, err := newLinkingAuthService().ProvisionFromClaims(ctx, "corp-sso", &idp.Claims{
		Subject: "sub-ada", Email: "moved-" + local + "@elsewhere.example", EmailVerified: true,
	})
	require.NoError(t, err)
	assert.Equal(t, account.ID, again.ID)
	stored, err = users.GetByID(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, account.Email, stored.Email, "the account email does not follow the provider")
}

func TestIntegrationLinking_TwoProvidersOneAccount(t *testing.T) {
	resetSearchTables(t)
	ctx := context.Background()
	svc := newLinkingAuthService()
	email := "grace-" + uuid.New().String() + "@example.com"

	first, err := svc.ProvisionFromClaims(ctx, "google",
		&idp.Claims{Subject: "g-grace", Email: email, Name: "Grace", EmailVerified: true})
	require.NoError(t, err)
	second, err := svc.ProvisionFromClaims(ctx, "github",
		&idp.Claims{Subject: "gh-grace", Email: email, Name: "grace-gh", EmailVerified: true})
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID, "one person, one account")
	assert.Equal(t, 1, countRows(t, integrationDB.DB, "SELECT count(*) FROM users"))
	got, err := NewUserIdentityRepository(integrationDB).ListByUserID(ctx, first.ID)
	require.NoError(t, err)
	providers := make([]string, 0, len(got))
	for _, identity := range got {
		providers = append(providers, identity.Provider)
	}
	// "google" is a Google-TYPE provider name here, so the first sign-in also
	// stored the legacy Google identity.
	assert.ElementsMatch(t, []string{"google", models.LegacyGoogleIdentityProvider, "github"}, providers)

	// Each identity keeps resolving to the same account.
	for provider, subject := range map[string]string{"google": "g-grace", "github": "gh-grace"} {
		user, lookupErr := NewUserRepository(integrationDB).GetByIDPSubject(ctx, provider, subject)
		require.NoError(t, lookupErr)
		assert.Equal(t, first.ID, user.ID)
	}
	stored, err := NewUserRepository(integrationDB).GetByID(ctx, first.ID)
	require.NoError(t, err)
	assert.Equal(t, "Grace", stored.Name, "the second provider does not rename the account")
}

// On an open instance anyone can create an account from an UNVERIFIED claim of
// somebody else's address. The mailbox's owner must never be linked into that
// account: whoever created it can still sign in to it.
func TestIntegrationLinking_AccountCreatedFromUnverifiedClaimIsNeverLinkedTo(t *testing.T) {
	resetSearchTables(t)
	ctx := context.Background()
	svc := newLinkingAuthService()
	users := NewUserRepository(integrationDB)
	email := "victim-" + uuid.New().String() + "@example.com"

	mallory, err := svc.ProvisionFromClaims(ctx, "lax-sso",
		&idp.Claims{Subject: "mallory", Email: email, Name: "Mallory", EmailVerified: false})
	require.NoError(t, err, "an open instance accepts an unverified claim for a new account")
	before, err := users.GetByID(ctx, mallory.ID)
	require.NoError(t, err)
	require.Nil(t, before.EmailVerifiedAt)

	owner, err := svc.ProvisionFromClaims(ctx, "corp-sso",
		&idp.Claims{Subject: "real-owner", Email: email, Name: "Owner", EmailVerified: true})

	require.ErrorIs(t, err, services.ErrExistingSignInRequired)
	assert.Nil(t, owner)
	assert.Equal(t, 1, countRows(t, integrationDB.DB, "SELECT count(*) FROM users"))
	assert.Equal(t, 0, countRows(t, integrationDB.DB,
		"SELECT count(*) FROM user_identities WHERE provider = 'corp-sso'"), "the owner's identity is not attached")
	after, err := users.GetByID(ctx, mallory.ID)
	require.NoError(t, err)
	assert.Equal(t, before.Version, after.Version, "the account row is untouched")
	assert.Nil(t, after.EmailVerifiedAt, "the account does not become verified by a refused link")
	assert.Equal(t, before.SessionEpoch, after.SessionEpoch)
}

// A user from before email verification was recorded (email_verified_at NULL,
// one identity): a second provider is refused until they have signed in once
// with their own provider, then it links.
func TestIntegrationLinking_PreVerificationUserLinksAfterSigningInTheirOwnWay(t *testing.T) {
	resetSearchTables(t)
	ctx := context.Background()
	svc := newLinkingAuthService()
	users := NewUserRepository(integrationDB)
	email := "old-" + uuid.New().String() + "@example.com"

	// What migration 022_consolidated leaves for an existing user: an identity, no verification.
	account := &models.User{Email: email, Name: "Old Timer", IDPProvider: strRef("corp-sso"), IDPSubject: strRef("old-sub")}
	require.NoError(t, users.Create(ctx, account))

	second := &idp.Claims{Subject: "gh-old", Email: email, EmailVerified: true}
	_, err := svc.ProvisionFromClaims(ctx, "github", second)
	require.ErrorIs(t, err, services.ErrExistingSignInRequired)

	own, err := svc.ProvisionFromClaims(ctx, "corp-sso", &idp.Claims{Subject: "old-sub", Email: email, EmailVerified: true})
	require.NoError(t, err)
	assert.Equal(t, account.ID, own.ID)
	stored, err := users.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.EmailVerifiedAt, "their own verified sign-in verifies the account")

	linked, err := svc.ProvisionFromClaims(ctx, "github", second)
	require.NoError(t, err, "the second provider now links")
	assert.Equal(t, account.ID, linked.ID)
	got, err := NewUserIdentityRepository(integrationDB).ListByUserID(ctx, account.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	stored, err = users.GetByID(ctx, account.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 0, stored.SessionEpoch, "linking to a verified account ends no session")
}

func TestIntegrationLinking_UnverifiedEmailWritesNothing(t *testing.T) {
	resetSearchTables(t)
	ctx := context.Background()
	users := NewUserRepository(integrationDB)
	account := &models.User{Email: "alan-" + uuid.New().String() + "@example.com", Name: "Alan"}
	require.NoError(t, users.Create(ctx, account))
	before, err := users.GetByID(ctx, account.ID)
	require.NoError(t, err)

	user, err := newLinkingAuthService().ProvisionFromClaims(ctx, "corp-sso",
		&idp.Claims{Subject: "sub-mallory", Email: account.Email, Name: "Mallory", EmailVerified: false})

	require.ErrorIs(t, err, services.ErrEmailNotVerified)
	assert.Nil(t, user)
	assert.Equal(t, 1, countRows(t, integrationDB.DB, "SELECT count(*) FROM users"), "no account is created")
	assert.Equal(t, 0, countRows(t, integrationDB.DB, "SELECT count(*) FROM user_identities"), "no identity row")
	after, err := users.GetByID(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, before.Version, after.Version, "the account row is untouched")
	assert.Nil(t, after.EmailVerifiedAt)
}

func TestIntegrationLinking_SuspendedAccountGainsNoIdentity(t *testing.T) {
	resetSearchTables(t)
	ctx := context.Background()
	users := NewUserRepository(integrationDB)
	account := &models.User{Email: "sam-" + uuid.New().String() + "@example.com", Name: "Sam"}
	require.NoError(t, users.Create(ctx, account))
	_, err := integrationDB.ExecContext(ctx, "UPDATE users SET status = $1 WHERE id = $2",
		models.UserStatusSuspended, account.ID)
	require.NoError(t, err)

	user, err := newLinkingAuthService().ProvisionFromClaims(ctx, "corp-sso",
		&idp.Claims{Subject: "sub-sam", Email: account.Email, EmailVerified: true})

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.True(t, user.IsSuspended(), "the caller's suspension gate refuses the sign-in")
	assert.Equal(t, 0, countRows(t, integrationDB.DB, "SELECT count(*) FROM user_identities"))
	stored, err := users.GetByID(ctx, account.ID)
	require.NoError(t, err)
	assert.Nil(t, stored.EmailVerifiedAt)
}
