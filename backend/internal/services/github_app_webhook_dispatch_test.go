package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Webhook dispatch is scoped to the App the delivery arrived for. Since #477,
// installation_id is unique PER APP, not globally — two teams may install their
// own Apps on the same GitHub org and receive the same numeric id. Resolving by
// installation_id alone would let one team's delivery mutate the other's row.
func TestGitHubAppService_HandleWebhookEvent_ScopedToApp(t *testing.T) {
	ctx := context.Background()

	newSvc := func(t *testing.T) (GitHubAppServiceInterface, *MockGitHubInstallationRepository) {
		t.Helper()
		installationRepo := new(MockGitHubInstallationRepository)
		svc := NewGitHubAppService(
			installationRepo, nil, nil, resolverFor(new(MockGitHubAppClient)),
			nil, nil, nil, allowAllAuthz{}, newTestLogger(),
		)
		return svc, installationRepo
	}

	t.Run("resolves within the delivering App", func(t *testing.T) {
		svc, installationRepo := newSvc(t)
		installationRepo.On("GetByAppConfigAndInstallationID", mock.Anything, "cfg-a", int64(4242)).
			Return(&models.GitHubInstallation{
				ID: "inst-a", TeamID: "team-a", AppConfigID: "cfg-a", InstallationID: 4242,
			}, nil)

		// An unhandled action is a no-op, which keeps this test about the LOOKUP.
		require.NoError(t, svc.HandleWebhookEvent(ctx, "cfg-a", "installation", 4242, "unhandled"))
		installationRepo.AssertExpectations(t)
	})

	// The cross-tenant case: the same installation id under a different App must
	// not resolve. The repository is asked with team B's App id, so team A's row
	// is unreachable by construction.
	t.Run("a foreign App id finds nothing", func(t *testing.T) {
		svc, installationRepo := newSvc(t)
		installationRepo.On("GetByAppConfigAndInstallationID", mock.Anything, "cfg-b", int64(4242)).
			Return(nil, repositories.ErrGitHubInstallationNotFound)

		// Not-found is deliberately not an error: GitHub retries deliveries, and
		// a delivery for an installation we do not know about is a no-op, not a
		// failure worth 500-ing over.
		assert.NoError(t, svc.HandleWebhookEvent(ctx, "cfg-b", "installation", 4242, "deleted"))
		installationRepo.AssertExpectations(t)
	})

	t.Run("a repository failure surfaces", func(t *testing.T) {
		svc, installationRepo := newSvc(t)
		installationRepo.On("GetByAppConfigAndInstallationID", mock.Anything, "cfg-a", int64(4242)).
			Return(nil, errors.New("connection reset"))

		assert.Error(t, svc.HandleWebhookEvent(ctx, "cfg-a", "installation", 4242, "created"))
		installationRepo.AssertExpectations(t)
	})
}
