package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	idpmocks "github.com/vibexp/vibexp/internal/auth/idp/mocks"
	"github.com/vibexp/vibexp/internal/models"
	repo_mocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// Sign-in against the database-backed access allowlist (#1235).

const allowlistRootAdmin = "root@corp.example"

// newAllowlistAuthService builds an AuthService over the real allowlist
// resolver, with allowlistRootAdmin as the only root admin.
func newAllowlistAuthService(
	repo *repo_mocks.MockUserRepository, provider *idpmocks.MockIdentityProvider,
) (*AuthService, *allowlistHarness) {
	h := newAllowlistHarness(allowlistRootAdmin)
	return NewAuthService(repo, newTestRegistry(provider), nil, h.resolver.logger, h.resolver), h
}

func TestAuthService_AllowlistChangeAppliesToTheNextSignIn(t *testing.T) {
	ctx := context.Background()
	repo := &repo_mocks.MockUserRepository{}
	service, h := newAllowlistAuthService(repo, &idpmocks.MockIdentityProvider{})
	repo.On("GetByEmail", ctx, "dev@example.com").
		Return(&models.User{ID: "user-1", Email: "dev@example.com"}, nil)

	_, err := service.HandleDevLogin(ctx, "dev@example.com", "Dev")
	require.NoError(t, err, "no allowlist stored is open access")

	// An allowlist is stored at runtime: no restart, the next sign-in sees it.
	h.store.set([]string{"corp.example"}, nil)
	h.expire()
	_, err = service.HandleDevLogin(ctx, "dev@example.com", "Dev")
	require.ErrorIs(t, err, ErrAccessRestricted)
	assert.Equal(t, "not_on_allowlist", denialReason(t, h.logs))
}

func TestAuthService_RootAdminIsExemptFromTheAllowlistAtSignIn(t *testing.T) {
	ctx := context.Background()
	repo := &repo_mocks.MockUserRepository{}
	service, h := newAllowlistAuthService(repo, &idpmocks.MockIdentityProvider{})
	h.store.set([]string{"example.com"}, nil)
	repo.On("GetByEmail", ctx, allowlistRootAdmin).
		Return(&models.User{ID: "root-1", Email: allowlistRootAdmin}, nil)

	user, err := service.HandleDevLogin(ctx, allowlistRootAdmin, "Root")
	require.NoError(t, err)
	assert.Equal(t, "root-1", user.ID)
	assert.Equal(t, 1, h.logged(
		"Root instance admin is exempt from the access allowlist their email does not match"))
}

// An unverified claim of a root admin's address must not ride the exemption:
// the exemption grants access by address, exactly like an allowlist entry.
func TestAuthService_UnverifiedRootAdminEmailIsDeniedWhileAllowlistActive(t *testing.T) {
	ctx := context.Background()
	repo := &repo_mocks.MockUserRepository{}
	provider := &idpmocks.MockIdentityProvider{}
	service, h := newAllowlistAuthService(repo, provider)
	h.store.set([]string{"example.com"}, nil)

	claims := createTestClaims()
	claims.Email = allowlistRootAdmin
	claims.EmailVerified = false
	provider.On("Name").Return(idp.ProviderOIDC)
	provider.On("ExchangeCode", ctx, "test-auth-code", "").
		Return(&idp.Tokens{AccessToken: "test-access-token"}, claims, nil)

	_, _, _, err := service.HandleCallback(ctx, "test-auth-code", string(idp.ProviderOIDC))
	require.ErrorIs(t, err, ErrAccessRestricted)
	assert.Equal(t, "unverified_email", denialReason(t, h.logs))
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAuthService_SignInFailsClosedWhenTheActiveAllowlistIsUnreadable(t *testing.T) {
	ctx := context.Background()
	repo := &repo_mocks.MockUserRepository{}
	service, h := newAllowlistAuthService(repo, &idpmocks.MockIdentityProvider{})
	h.store.set([]string{"example.com"}, nil)
	repo.On("GetByEmail", ctx, "dev@example.com").
		Return(&models.User{ID: "user-1", Email: "dev@example.com"}, nil).Once()
	_, err := service.HandleDevLogin(ctx, "dev@example.com", "Dev")
	require.NoError(t, err)

	dbDown := errors.New("db down")
	h.store.fail(dbDown, nil)
	h.expire()

	_, err = service.HandleDevLogin(ctx, "dev@example.com", "Dev")
	require.ErrorIs(t, err, dbDown)
	assert.NotErrorIs(t, err, ErrAccessRestricted, "an unreadable allowlist is a failure, not a policy denial")
	repo.AssertExpectations(t) // the user was looked up once only: before the outage
}
