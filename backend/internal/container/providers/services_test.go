package providers

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
)

// TestProvideContentVersionService_RetentionCapFromConfig verifies that the
// retention cap configured via cfg.Retention.ContentVersionLimit flows through
// the provider into the prune call for every versioned resource type — i.e. the
// cap is no longer the old hardcoded 5.
func TestProvideContentVersionService_RetentionCapFromConfig(t *testing.T) {
	const configuredCap = 20

	for _, resourceType := range []string{"artifact", "blueprint", "memory", "prompt"} {
		t.Run(resourceType, func(t *testing.T) {
			repo := repomocks.NewMockContentVersionRepository(t)
			repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
			// PruneToCap must receive the configured cap, not a hardcoded value.
			repo.EXPECT().
				PruneToCap(mock.Anything, resourceType, "res-1", configuredCap).
				Return(nil).
				Once()

			cfg := &config.Config{Retention: config.RetentionConfig{ContentVersionLimit: configuredCap}}
			svc := ProvideContentVersionService(repo, nil, cfg, testLogger())

			err := svc.SnapshotIfChanged(context.Background(), services.SnapshotRequest{
				ResourceType: resourceType, ResourceID: "res-1", TeamID: "team-1", UserID: "user-1",
				OldContent: "old", NewContent: "new",
			})

			require.NoError(t, err)
		})
	}
}

// TestProvideContentVersionService_ZeroCapKeepsAll verifies that a zero cap is
// passed straight through; the repository layer treats a non-positive cap as
// "keep all versions".
func TestProvideContentVersionService_ZeroCapKeepsAll(t *testing.T) {
	repo := repomocks.NewMockContentVersionRepository(t)
	repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(v *models.ContentVersion) bool {
		return v.ResourceType == "memory"
	})).Return(nil).Once()
	repo.EXPECT().PruneToCap(mock.Anything, "memory", "res-1", 0).Return(nil).Once()

	cfg := &config.Config{Retention: config.RetentionConfig{ContentVersionLimit: 0}}
	svc := ProvideContentVersionService(repo, nil, cfg, testLogger())

	err := svc.SnapshotIfChanged(context.Background(), services.SnapshotRequest{
		ResourceType: "memory", ResourceID: "res-1", TeamID: "team-1", UserID: "user-1",
		OldContent: "old", NewContent: "new",
	})

	require.NoError(t, err)
}

// TestProvideSetupModeService_RecoveryModeFromConfig verifies auth.recovery_mode
// reaches the setup-mode service (#1237): with the flag on, setup mode is active
// while a provider is enabled; with it off, the enabled provider ends it.
func TestProvideSetupModeService_RecoveryModeFromConfig(t *testing.T) {
	for _, recovery := range []bool{true, false} {
		setup := repomocks.NewMockInstanceAuthSetupRepository(t)
		providerRepo := repomocks.NewMockInstanceAuthProviderRepository(t)
		if !recovery {
			// Only without the flag does the stored state decide.
			setup.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAuthSetupNotFound).Once()
			providerRepo.EXPECT().List(mock.Anything).
				Return([]*models.InstanceAuthProvider{{Slug: "sso", Enabled: true}}, nil).Once()
		}
		cfg := &config.Config{}
		cfg.Auth.RecoveryMode = config.EnvBool(recovery)

		svc := ProvideSetupModeService(setup, providerRepo,
			services.NewInstanceAdminService(nil, nil, nil, slog.New(slog.DiscardHandler)),
			cfg, slog.New(slog.DiscardHandler))

		active, err := svc.IsActive(context.Background())
		require.NoError(t, err)
		require.Equal(t, recovery, active)
	}
}
