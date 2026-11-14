package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	idpmocks "github.com/vibexp/vibexp/internal/auth/idp/mocks"
	repo_mocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// revokingProvider is an identity provider that can revoke tokens (#1649): it
// records the tokens it was asked to revoke and answers err.
type revokingProvider struct {
	*idpmocks.MockIdentityProvider
	revoked []string
	err     error
}

func (p *revokingProvider) RevokeRefreshToken(_ context.Context, refreshToken string) error {
	p.revoked = append(p.revoked, refreshToken)
	return p.err
}

func newRevokingProvider(slug string, err error) *revokingProvider {
	return &revokingProvider{MockIdentityProvider: slugProvider(slug), err: err}
}

func TestAuthService_RevokeRefreshToken_RoutesToTheSessionsProvider(t *testing.T) {
	corp, partner := newRevokingProvider("corp-sso", nil), newRevokingProvider("partner-sso", nil)
	svc := newResolverAuthService(&repo_mocks.MockUserRepository{}, staticIDPResolver{reg: idp.NewRegistry(corp, partner)})

	require.NoError(t, svc.RevokeRefreshToken(context.Background(), "partner-sso", "rt-1"))

	assert.Empty(t, corp.revoked)
	assert.Equal(t, []string{"rt-1"}, partner.revoked)
}

func TestAuthService_RevokeRefreshToken_EmptySlugUsesTheSoleProvider(t *testing.T) {
	sole := newRevokingProvider("oidc", nil)
	svc := newResolverAuthService(&repo_mocks.MockUserRepository{}, staticIDPResolver{reg: idp.NewRegistry(sole)})

	require.NoError(t, svc.RevokeRefreshToken(context.Background(), "", "legacy-rt"))

	assert.Equal(t, []string{"legacy-rt"}, sole.revoked)
}

func TestAuthService_RevokeRefreshToken_PassesTheProvidersErrorThrough(t *testing.T) {
	failure := errors.New("revocation endpoint answered 503")
	svc := newResolverAuthService(&repo_mocks.MockUserRepository{},
		staticIDPResolver{reg: idp.NewRegistry(newRevokingProvider("oidc", failure))})

	assert.ErrorIs(t, svc.RevokeRefreshToken(context.Background(), "oidc", "rt"), failure)
}

func TestAuthService_RevokeRefreshToken_ProviderWithoutRevocationIsUnsupported(t *testing.T) {
	svc := newResolverAuthService(&repo_mocks.MockUserRepository{},
		staticIDPResolver{reg: idp.NewRegistry(slugProvider("github"))})

	assert.ErrorIs(t, svc.RevokeRefreshToken(context.Background(), "github", "rt"), idp.ErrRevocationUnsupported)
}

func TestAuthService_RevokeRefreshToken_ResolutionErrors(t *testing.T) {
	ctx := context.Background()
	unresolvable := newResolverAuthService(&repo_mocks.MockUserRepository{}, staticIDPResolver{err: errors.New("db down")})
	assert.ErrorIs(t, unresolvable.RevokeRefreshToken(ctx, "oidc", "rt"), ErrIdentityProvidersUnresolvable)

	health := map[string]ProviderHealth{"corp-sso": {Healthy: false, LastError: "discovery timed out"}}
	svc := newResolverAuthService(&repo_mocks.MockUserRepository{},
		staticIDPResolver{reg: idp.NewRegistry(slugProvider("github")), health: health})
	assert.ErrorIs(t, svc.RevokeRefreshToken(ctx, "corp-sso", "rt"), ErrIdentityProviderTemporarilyUnavailable)
	assert.ErrorIs(t, svc.RevokeRefreshToken(ctx, "partner-sso", "rt"), ErrIdentityProviderUnavailable)
}
