package providers

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	eventmocks "github.com/vibexp/vibexp/pkg/events/mocks"
)

// The registration gate and the admin allowlist reach their services as
// OPTIONS whose defaults admit everyone, so a provider that forgot to pass one
// would switch the feature off without failing anything. These tests build the
// services the way the container does and check the refusal (#1434).

type closedGate struct{}

func (closedGate) AllowNewAccount(context.Context, string, bool) error {
	return services.ErrRegistrationClosed
}

func TestProvideAuthService_InstallsTheRegistrationGate(t *testing.T) {
	ctx := context.Background()
	users := repomocks.NewMockUserRepository(t)
	users.On("GetByIDPSubject", ctx, "corp-sso", "sub-new").Return(nil, repositories.ErrUserNotFound)
	users.On("GetByEmail", ctx, "new@example.com").Return(nil, repositories.ErrUserNotFound)
	allowlist := servicesmocks.NewMockAccessAllowlistResolver(t)
	allowlist.On("IsEmailAllowed", ctx, "new@example.com").Return(true, false, nil)

	auth := ProvideAuthService(AuthServiceDeps{
		UserRepo: users, IdentityRepo: repomocks.NewMockUserIdentityRepository(t),
		Logger: slog.New(slog.DiscardHandler), Allowlist: allowlist, Gate: closedGate{},
	})

	_, err := auth.ProvisionFromClaims(ctx, "corp-sso",
		&idp.Claims{Subject: "sub-new", Email: "new@example.com", EmailVerified: true})

	require.ErrorIs(t, err, services.ErrRegistrationClosed,
		"the provider must hand the gate to the service; users is a strict mock, so nothing was created")
}

func TestProvideRegistrationGate_EnforcesTheStoredMode(t *testing.T) {
	ctx := context.Background()
	policy := servicesmocks.NewMockAuthPolicyResolver(t)
	policy.On("Policy", ctx).
		Return(models.InstanceAuthPolicy{RegistrationMode: models.RegistrationModeInviteOnly}, nil)
	invitations := repomocks.NewMockTeamInvitationRepository(t)
	invitations.On("GetPendingByEmail", ctx, "new@example.com").Return([]models.TeamInvitation{}, nil)
	rootAdmins := servicesmocks.NewMockInstanceAdminResolver(t)
	rootAdmins.On("IsRootAdmin", "new@example.com").Return(false)

	gate := ProvideRegistrationGate(policy, invitations, rootAdmins, slog.New(slog.DiscardHandler))

	require.ErrorIs(t, gate.AllowNewAccount(ctx, "new@example.com", true), services.ErrRegistrationClosed)
}

func TestProvideAdminService_InstallsTheAccessAllowlist(t *testing.T) {
	ctx := context.Background()
	allowlist := servicesmocks.NewMockAccessAllowlistResolver(t)
	allowlist.On("IsEmailAllowed", mock.Anything, "outside@elsewhere.test").Return(false, true, nil)

	admin := ProvideAdminService(repomocks.NewMockAdminRepository(t), repomocks.NewMockUserRepository(t),
		eventmocks.NewMockEventPublisher(t), nil, allowlist, nil, nil)

	_, err := admin.CreateUser(ctx, services.AdminUserCreateRequest{Email: "outside@elsewhere.test", Name: "O"})

	var notAllowed *services.ErrAdminUserEmailNotAllowed
	require.ErrorAs(t, err, &notAllowed, "the provider must hand the allowlist to the service")
}
