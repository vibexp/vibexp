package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repo_mocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// The provider role mapping at sign-in (#1644).

type syncCall struct {
	userID, providerID, slug string
	matched                  *string
}

type recordingProviderAdminSyncer struct {
	calls []syncCall
	err   error
}

func (r *recordingProviderAdminSyncer) SyncProviderAdmin(
	_ context.Context, user *models.User, providerID, providerSlug string, matched *string,
) error {
	r.calls = append(r.calls, syncCall{userID: user.ID, providerID: providerID, slug: providerSlug, matched: matched})
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
