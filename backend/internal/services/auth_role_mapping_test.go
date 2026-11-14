package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repo_mocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// The provider role mapping at sign-in (#1644).

type syncCall struct {
	userID, providerID, slug, trigger string
	matched                           *string
}

type recordingProviderAdminSyncer struct {
	calls []syncCall
	err   error
}

func (r *recordingProviderAdminSyncer) SyncProviderAdmin(
	_ context.Context, user *models.User, providerID, providerSlug string, matched *string, trigger string,
) error {
	r.calls = append(r.calls, syncCall{
		userID: user.ID, providerID: providerID, slug: providerSlug, trigger: trigger, matched: matched,
	})
	return r.err
}

// roleMappingSignIn runs one first sign-in through a provider of typ with
// mapping and raw claims, and returns what the syncer saw.
func roleMappingSignIn(
	t *testing.T, typ idp.ProviderName, mapping *idp.RoleMapping, raw map[string]any, syncErr error,
) (*recordingProviderAdminSyncer, *models.User, error) {
	t.Helper()
	const slug = "corp-sso"
	claims := &idp.Claims{Subject: "sub-1", Email: "u@example.com", EmailVerified: true, Name: "U", Raw: raw}
	p := slugProvider(slug)
	p.On("ExchangeCode", mock.Anything, "code", "", mock.Anything).Return(&idp.Tokens{AccessToken: "a"}, claims, nil)
	repo := &repo_mocks.MockUserRepository{}
	repo.On("GetByIDPSubject", mock.Anything, mock.Anything, "sub-1").Return(nil, repositories.ErrUserNotFound)
	repo.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound).Maybe()
	repo.On("Create", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { args.Get(1).(*models.User).ID = "user-1" }).Return(nil)
	reg := idp.NewRegistryFromEntries(idp.Entry{
		Provider: p, Type: typ, DisplayName: "SSO", ProviderID: "provider-1", RoleMapping: mapping,
	})
	syncer := &recordingProviderAdminSyncer{err: syncErr}
	svc := newResolverAuthService(repo, staticIDPResolver{reg: reg})
	WithProviderAdminSync(syncer)(svc)

	user, _, _, err := svc.HandleCallback(context.Background(), "code", slug, idp.FlowParams{})
	return syncer, user, err
}

func TestHandleCallback_RoleMapping(t *testing.T) {
	mapping := &idp.RoleMapping{Claim: "realm_access.roles", AdminValues: []string{"platform-admins"}}
	matching := map[string]any{"realm_access": map[string]any{"roles": []any{"dev", "platform-admins"}}}

	t.Run("a matching claim promotes", func(t *testing.T) {
		syncer, user, err := roleMappingSignIn(t, idp.ProviderOIDC, mapping, matching, nil)
		require.NoError(t, err)
		require.NotNil(t, user)
		require.Len(t, syncer.calls, 1)
		call := syncer.calls[0]
		assert.Equal(t, "user-1", call.userID)
		assert.Equal(t, "provider-1", call.providerID)
		assert.Equal(t, "corp-sso", call.slug)
		assert.Equal(t, ProviderAdminTriggerSignIn, call.trigger)
		require.NotNil(t, call.matched)
		assert.Equal(t, "platform-admins", *call.matched)
	})
	t.Run("no matching value demotes", func(t *testing.T) {
		raw := map[string]any{"realm_access": map[string]any{"roles": []any{"dev"}}}
		syncer, _, err := roleMappingSignIn(t, idp.ProviderOIDC, mapping, raw, nil)
		require.NoError(t, err)
		require.Len(t, syncer.calls, 1)
		assert.Nil(t, syncer.calls[0].matched)
	})
	t.Run("a missing claim demotes", func(t *testing.T) {
		syncer, _, err := roleMappingSignIn(t, idp.ProviderOIDC, mapping, nil, nil)
		require.NoError(t, err)
		require.Len(t, syncer.calls, 1)
		assert.Nil(t, syncer.calls[0].matched)
	})
	t.Run("mapping off reads and writes nothing", func(t *testing.T) {
		syncer, user, err := roleMappingSignIn(t, idp.ProviderOIDC, nil, matching, nil)
		require.NoError(t, err)
		require.NotNil(t, user)
		assert.Empty(t, syncer.calls)
	})
	t.Run("google and github carry no mapping", func(t *testing.T) {
		for _, typ := range []idp.ProviderName{idp.ProviderGoogle, idp.ProviderGitHub} {
			syncer, _, err := roleMappingSignIn(t, typ, nil, nil, nil)
			require.NoError(t, err)
			assert.Empty(t, syncer.calls, typ)
		}
	})
	t.Run("a sync failure fails the sign-in", func(t *testing.T) {
		_, user, err := roleMappingSignIn(t, idp.ProviderOIDC, mapping, matching, errors.New("db down"))
		require.Error(t, err)
		assert.Nil(t, user)
	})
}

// The provider role mapping re-applied on token refresh (#1648).

// roleMappingRefresh re-applies the role mapping of one provider of typ with
// mapping for user-1 from claims, and returns what the syncer saw and what was
// logged. users answers GetByID; a test that expects no user read passes a mock
// without that expectation, so a read fails the test.
func roleMappingRefresh(
	t *testing.T, typ idp.ProviderName, mapping *idp.RoleMapping, slug string, claims *idp.Claims,
	users *repo_mocks.MockUserRepository, syncErr error,
) (*recordingProviderAdminSyncer, *logtest.Recorder, error) {
	t.Helper()
	reg := idp.NewRegistryFromEntries(idp.Entry{
		Provider: slugProvider("corp-sso"), Type: typ, DisplayName: "SSO", ProviderID: "provider-1", RoleMapping: mapping,
	})
	syncer := &recordingProviderAdminSyncer{err: syncErr}
	logger, recorder := logtest.New()
	svc := NewAuthService(users, permissiveIdentityRepo(), staticIDPResolver{reg: reg}, nil, logger,
		newStaticAllowlistResolver(logger, nil), WithProviderAdminSync(syncer))

	err := svc.ApplyRefreshedRoleMapping(context.Background(), slug, "user-1", claims)
	return syncer, recorder, err
}

// userOne answers GetByID for user-1.
func userOne() *repo_mocks.MockUserRepository {
	users := &repo_mocks.MockUserRepository{}
	users.On("GetByID", mock.Anything, "user-1").Return(&models.User{ID: "user-1", Email: "u@example.com"}, nil)
	return users
}

func refreshWarnings(recorder *logtest.Recorder) []*logtest.Entry {
	var out []*logtest.Entry
	for _, e := range recorder.AllEntries() {
		if e.Level == slog.LevelWarn {
			out = append(out, e)
		}
	}
	return out
}

func TestApplyRefreshedRoleMapping(t *testing.T) {
	mapping := &idp.RoleMapping{Claim: "realm_access.roles", AdminValues: []string{"platform-admins"}}
	claimsWith := func(roles ...any) *idp.Claims {
		return &idp.Claims{Subject: "sub-1", Raw: map[string]any{"realm_access": map[string]any{"roles": roles}}}
	}

	t.Run("a matching claim promotes, through the sign-in evaluation", func(t *testing.T) {
		syncer, _, err := roleMappingRefresh(t, idp.ProviderOIDC, mapping, "corp-sso",
			claimsWith("dev", "platform-admins"), userOne(), nil)
		require.NoError(t, err)
		require.Len(t, syncer.calls, 1)
		call := syncer.calls[0]
		assert.Equal(t, syncCall{userID: "user-1", providerID: "provider-1", slug: "corp-sso",
			trigger: ProviderAdminTriggerRefresh, matched: call.matched}, call)
		require.NotNil(t, call.matched)
		assert.Equal(t, "platform-admins", *call.matched)
	})
	t.Run("an observed claim without the value demotes", func(t *testing.T) {
		syncer, _, err := roleMappingRefresh(t, idp.ProviderOIDC, mapping, "corp-sso", claimsWith("dev"), userOne(), nil)
		require.NoError(t, err)
		require.Len(t, syncer.calls, 1)
		assert.Nil(t, syncer.calls[0].matched)
		assert.Equal(t, ProviderAdminTriggerRefresh, syncer.calls[0].trigger)
	})
	t.Run("an observed claim set missing the claim demotes", func(t *testing.T) {
		claims := &idp.Claims{Subject: "sub-1", Raw: map[string]any{"sub": "sub-1"}}
		syncer, _, err := roleMappingRefresh(t, idp.ProviderOIDC, mapping, "corp-sso", claims, userOne(), nil)
		require.NoError(t, err)
		require.Len(t, syncer.calls, 1)
		assert.Nil(t, syncer.calls[0].matched)
	})
	for name, claims := range map[string]*idp.Claims{
		"no claims at all":  nil,
		"no claim set read": {Subject: "sub-1"},
	} {
		t.Run(name+" changes nothing and warns", func(t *testing.T) {
			syncer, recorder, err := roleMappingRefresh(t, idp.ProviderOIDC, mapping, "corp-sso", claims,
				&repo_mocks.MockUserRepository{}, nil)
			require.NoError(t, err)
			assert.Empty(t, syncer.calls, "an unobserved claim never demotes")
			warnings := refreshWarnings(recorder)
			require.Len(t, warnings, 1)
			assert.Equal(t, "user-1", warnings[0].Data["user_id"])
			assert.Equal(t, "corp-sso", warnings[0].Data["provider"])
		})
	}
	t.Run("mapping off reads and writes nothing", func(t *testing.T) {
		syncer, _, err := roleMappingRefresh(t, idp.ProviderOIDC, nil, "corp-sso", claimsWith("platform-admins"),
			&repo_mocks.MockUserRepository{}, nil)
		require.NoError(t, err)
		assert.Empty(t, syncer.calls)
	})
	t.Run("google and github read and write nothing", func(t *testing.T) {
		for _, typ := range []idp.ProviderName{idp.ProviderGoogle, idp.ProviderGitHub} {
			syncer, _, err := roleMappingRefresh(t, typ, nil, "corp-sso", claimsWith("platform-admins"),
				&repo_mocks.MockUserRepository{}, nil)
			require.NoError(t, err)
			assert.Empty(t, syncer.calls, typ)
		}
	})
	t.Run("a provider no longer enabled reads and writes nothing", func(t *testing.T) {
		syncer, _, err := roleMappingRefresh(t, idp.ProviderOIDC, mapping, "gone", claimsWith("platform-admins"),
			&repo_mocks.MockUserRepository{}, nil)
		require.NoError(t, err)
		assert.Empty(t, syncer.calls)
	})
	t.Run("a session issued without a provider name uses the only provider", func(t *testing.T) {
		syncer, _, err := roleMappingRefresh(t, idp.ProviderOIDC, mapping, "", claimsWith("platform-admins"),
			userOne(), nil)
		require.NoError(t, err)
		require.Len(t, syncer.calls, 1)
		assert.Equal(t, "corp-sso", syncer.calls[0].slug)
	})
	t.Run("a sync failure is returned", func(t *testing.T) {
		_, _, err := roleMappingRefresh(t, idp.ProviderOIDC, mapping, "corp-sso", claimsWith("platform-admins"),
			userOne(), errors.New("db down"))
		require.Error(t, err)
	})
	t.Run("a user read failure is returned", func(t *testing.T) {
		users := &repo_mocks.MockUserRepository{}
		users.On("GetByID", mock.Anything, "user-1").Return(nil, errors.New("db down"))
		syncer, _, err := roleMappingRefresh(t, idp.ProviderOIDC, mapping, "corp-sso", claimsWith("platform-admins"),
			users, nil)
		require.Error(t, err)
		assert.Empty(t, syncer.calls)
	})
}

func TestApplyRefreshedRoleMapping_UnresolvableProviders(t *testing.T) {
	svc := newResolverAuthService(&repo_mocks.MockUserRepository{}, staticIDPResolver{err: errors.New("db down")})
	WithProviderAdminSync(&recordingProviderAdminSyncer{})(svc)
	err := svc.ApplyRefreshedRoleMapping(context.Background(), "corp-sso", "user-1", &idp.Claims{Raw: map[string]any{}})
	assert.ErrorIs(t, err, ErrIdentityProvidersUnresolvable)
}

func TestApplyRefreshedRoleMapping_NoSyncerDoesNothing(t *testing.T) {
	svc := newResolverAuthService(&repo_mocks.MockUserRepository{}, staticIDPResolver{err: errors.New("unused")})
	require.NoError(t, svc.ApplyRefreshedRoleMapping(context.Background(), "corp-sso", "user-1", nil))
}
