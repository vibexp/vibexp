package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
)

// Regression tests for #463. The install callback used to bind whatever
// installation_id it was handed, because the only checks were an HMAC state the
// caller mints for their own team and an app-JWT lookup that resolves every
// installation of the app. These assert the caller-authority leg, and that it
// lives in the service so a second caller cannot reach the store path without it.

// denyAllAuthz refuses every permission, standing in for a plain team member.
type denyAllAuthz struct{}

func (denyAllAuthz) Can(_ context.Context, _, _ string, perm authz.Permission) error {
	return ErrPermissionDenied
}

func (denyAllAuthz) CanActOnResource(
	_ context.Context, _, _, _ string, _, _ authz.Permission,
) error {
	return ErrPermissionDenied
}

func (denyAllAuthz) Authorize(
	_ context.Context, _, _ string, _ authz.Permission,
) (models.TeamMemberRole, error) {
	return "", ErrPermissionDenied
}

const (
	authorityTestTeamID         = "team-attacker"
	authorityTestUserID         = "user-attacker"
	authorityTestInstallationID = int64(12345)
)

// assertNothingStored is the point of the whole issue: a denied callback must
// leave no installation record behind.
func assertNothingStored(t *testing.T, installationRepo *MockGitHubInstallationRepository) {
	t.Helper()
	installationRepo.AssertNotCalled(t, "Create")
	installationRepo.AssertNotCalled(t, "Update")
}

// TestHandleInstallationCallback_ForeignInstallationRejected is the core case:
// the caller holds a valid code, but the installation is not one they can
// administer, so the bind is refused.
func TestHandleInstallationCallback_ForeignInstallationRejected(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	githubClient := new(MockGitHubAppClient)
	eventManager := new(MockEventPublisher)

	githubClient.On("ExchangeUserCode", mock.Anything, testInstallCode).
		Return(testInstallerGrant, nil)
	// GET /user/installations does not list the victim's installation.
	githubClient.On("UserCanAccessInstallation", mock.Anything, testInstallerGrant, authorityTestInstallationID).
		Return(false, nil)

	svc := newCallbackTestServiceWithAuthz(installationRepo, githubClient, eventManager, allowAllAuthz{})

	reconnected, err := svc.HandleInstallationCallback(
		context.Background(), authorityTestUserID, authorityTestTeamID,
		authorityTestInstallationID, testInstallCode,
	)

	assert.True(t, errors.Is(err, ErrInstallationNotAuthorized),
		"expected ErrInstallationNotAuthorized, got: %v", err)
	assert.False(t, reconnected)

	// The app-JWT lookup must not even be reached — authority is checked first.
	githubClient.AssertNotCalled(t, "GetInstallation")
	assertNothingStored(t, installationRepo)
	eventManager.AssertNotCalled(t, "Publish")
	githubClient.AssertExpectations(t)
}

// TestHandleInstallationCallback_InvalidCodeRejected verifies a code GitHub
// refuses is a denial, not an internal error.
func TestHandleInstallationCallback_InvalidCodeRejected(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	githubClient := new(MockGitHubAppClient)
	eventManager := new(MockEventPublisher)

	githubClient.On("ExchangeUserCode", mock.Anything, testInstallCode).
		Return("", external.ErrGitHubUserCodeInvalid)

	svc := newCallbackTestServiceWithAuthz(installationRepo, githubClient, eventManager, allowAllAuthz{})

	_, err := svc.HandleInstallationCallback(
		context.Background(), authorityTestUserID, authorityTestTeamID,
		authorityTestInstallationID, testInstallCode,
	)

	assert.True(t, errors.Is(err, ErrInstallationNotAuthorized),
		"expected ErrInstallationNotAuthorized, got: %v", err)
	githubClient.AssertNotCalled(t, "UserCanAccessInstallation")
	assertNothingStored(t, installationRepo)
}

// TestHandleInstallationCallback_EmptyCodeRejected verifies the service refuses
// on its own rather than relying on the handler's validation — the guarantee
// must hold for any caller of HandleInstallationCallback.
func TestHandleInstallationCallback_EmptyCodeRejected(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	githubClient := new(MockGitHubAppClient)
	eventManager := new(MockEventPublisher)

	svc := newCallbackTestServiceWithAuthz(installationRepo, githubClient, eventManager, allowAllAuthz{})

	_, err := svc.HandleInstallationCallback(
		context.Background(), authorityTestUserID, authorityTestTeamID,
		authorityTestInstallationID, "",
	)

	assert.True(t, errors.Is(err, ErrInstallationNotAuthorized),
		"expected ErrInstallationNotAuthorized, got: %v", err)
	githubClient.AssertNotCalled(t, "ExchangeUserCode")
	githubClient.AssertNotCalled(t, "GetInstallation")
	assertNothingStored(t, installationRepo)
}

// TestHandleInstallationCallback_UserAuthNotConfigured verifies the flow fails
// closed on an instance with no GitHub App OAuth credentials, instead of
// falling back to the pre-#463 behaviour.
func TestHandleInstallationCallback_UserAuthNotConfigured(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	githubClient := new(MockGitHubAppClient)
	eventManager := new(MockEventPublisher)

	githubClient.On("ExchangeUserCode", mock.Anything, testInstallCode).
		Return("", external.ErrGitHubUserAuthNotConfigured)

	svc := newCallbackTestServiceWithAuthz(installationRepo, githubClient, eventManager, allowAllAuthz{})

	_, err := svc.HandleInstallationCallback(
		context.Background(), authorityTestUserID, authorityTestTeamID,
		authorityTestInstallationID, testInstallCode,
	)

	assert.True(t, errors.Is(err, ErrGitHubUserAuthUnavailable),
		"expected ErrGitHubUserAuthUnavailable, got: %v", err)
	assertNothingStored(t, installationRepo)
}

// TestHandleInstallationCallback_MemberRoleRejected verifies connecting a
// GitHub org is an owner/admin action, refused before any GitHub traffic.
func TestHandleInstallationCallback_MemberRoleRejected(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	githubClient := new(MockGitHubAppClient)
	eventManager := new(MockEventPublisher)

	svc := newCallbackTestServiceWithAuthz(installationRepo, githubClient, eventManager, denyAllAuthz{})

	_, err := svc.HandleInstallationCallback(
		context.Background(), authorityTestUserID, authorityTestTeamID,
		authorityTestInstallationID, testInstallCode,
	)

	assert.True(t, errors.Is(err, ErrPermissionDenied),
		"expected ErrPermissionDenied, got: %v", err)
	githubClient.AssertNotCalled(t, "ExchangeUserCode")
	githubClient.AssertNotCalled(t, "GetInstallation")
	assertNothingStored(t, installationRepo)
}

// TestDisconnectInstallation_MemberRoleRejected verifies disconnect is gated at
// the same level as connect, and touches nothing when refused.
func TestDisconnectInstallation_MemberRoleRejected(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	githubClient := new(MockGitHubAppClient)
	eventManager := new(MockEventPublisher)

	svc := newCallbackTestServiceWithAuthz(installationRepo, githubClient, eventManager, denyAllAuthz{})

	err := svc.DisconnectInstallation(context.Background(), authorityTestUserID, authorityTestTeamID)

	assert.True(t, errors.Is(err, ErrPermissionDenied),
		"expected ErrPermissionDenied, got: %v", err)
	installationRepo.AssertNotCalled(t, "Delete")
	eventManager.AssertNotCalled(t, "Publish")
}

// TestInstallationAuthorityErrorsAreSentinels keeps the new errors usable with
// errors.Is through wrapping, which the handler's status mapping relies on.
func TestInstallationAuthorityErrorsAreSentinels(t *testing.T) {
	assert.True(t, errors.Is(
		errors.Join(errors.New("outer"), ErrInstallationNotAuthorized), ErrInstallationNotAuthorized))
	assert.True(t, errors.Is(
		errors.Join(errors.New("outer"), ErrGitHubUserAuthUnavailable), ErrGitHubUserAuthUnavailable))
}
