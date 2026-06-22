//go:build wireinject

package container

import (
	"log/slog"

	"github.com/google/wire"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/container/providers"
	"github.com/vibexp/vibexp/internal/database"
	notificationsvc "github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/pkg/events"
)

// ProviderSet is the Wire provider set for the entire application
var ProviderSet = wire.NewSet(
	// Observability
	providers.ProvideMetrics,

	// External dependencies
	providers.ProvideIdentityProvider,
	providers.ProvideEmailProvider,
	providers.ProvideSMTPClient,
	providers.ProvideGitHubAppClient,

	// Event system
	providers.ProvideEventManager,
	wire.Bind(new(events.EventPublisher), new(*events.EventManager)),
	providers.ProvideEventSystemDeps,

	// Cache
	providers.ProvideCache,

	// Repositories
	providers.ProvideUserRepository,
	providers.ProvideAPIKeyRepository,
	providers.ProvidePromptRepository,
	providers.ProvidePromptReferenceRepository,
	providers.ProvidePromptGalleryRepository,
	providers.ProvidePromptShareRepository,
	providers.ProvideArtifactRepository,
	providers.ProvideAttachmentRepository,
	providers.ProvideTypeRepository,
	providers.ProvideContentVersionRepository,
	providers.ProvideBlueprintRepository,
	providers.ProvideEmbeddingProviderRepository,
	providers.ProvideActivityRepository,
	providers.ProvideResourceAccessRepository,
	providers.ProvideClaudeCodeHooksRepository,
	providers.ProvideCursorIDEHooksRepository,
	providers.ProvideAgentRepository,
	providers.ProvideAgentExecutionRepository,
	providers.ProvideAgentExecutionEventRepository,
	providers.ProvideMemoryRepository,
	providers.ProvideEmbeddingRepository,
	providers.ProvideEmbeddingBackfillRepository,
	providers.ProvideSearchRepository,
	providers.ProvideResourceUsageRepository,
	providers.ProvideBackofficeRepository,
	providers.ProvideUserPreferencesRepository,
	providers.ProvideTeamRepository,
	providers.ProvideTeamMemberRepository,
	providers.ProvideTeamInvitationRepository,
	providers.ProvideTeamSubscriptionRepository,
	providers.ProvideProjectRepository,
	providers.ProvideWebhookEventRepository,
	providers.ProvideGitHubInstallationRepository,
	providers.ProvideFeedRepository,
	providers.ProvideFeedItemRepository,
	providers.ProvideFeedItemReplyRepository,
	providers.ProvideNotificationRepository,
	providers.ProvideNotificationDeliveryRepository,
	providers.ProvideNotificationDigestQueueRepository,
	providers.ProvideDeviceTokenRepository,

	// Services
	providers.ProvideFeatureFlagService,
	providers.ProvideAuthService,
	providers.ProvideAPIKeyService,
	providers.ProvidePromptService,
	providers.ProvidePromptGalleryService,
	providers.ProvidePromptShareService,
	providers.ProvideResourceUsageService,
	providers.ProvideContentVersionService,
	providers.ProvideArtifactService,
	providers.ProvideObjectStore,
	providers.ProvideAttachmentService,
	providers.ProvideTypeService,
	providers.ProvideBlueprintService,
	providers.ProvideEmbeddingProviderService,
	providers.ProvideEmailService,
	providers.ProvideActivityService,
	providers.ProvideResourceAccessWorkerPool,
	providers.ProvideResourceAccessService,
	providers.ProvideEncryptionService,
	providers.ProvideAgentService,
	providers.ProvideAgentCardFetcher,
	providers.ProvideAgentAuthenticator,
	providers.ProvideA2AHTTPClient,
	providers.ProvideA2AStreamProcessor,
	providers.ProvideAgentInvocationService,
	providers.ProvideMemoryService,
	providers.ProvideEmbeddingService,
	providers.ProvideQueryEmbedder,
	providers.ProvideSearchService,
	providers.ProvideEnvironmentService,
	providers.ProvideEmbeddingHandlerAdapter,
	providers.ProvideBackofficeService,
	providers.ProvideEmbeddingBackfillService,
	providers.ProvideUserPreferencesService,
	providers.ProvideTeamService,
	providers.ProvideTeamInvitationService,
	providers.ProvideProjectService,
	providers.ProvideProjectMigrationService,
	providers.ProvideGitHubAppService,
	providers.ProvideFeedService,
	providers.ProvideFeedItemService,
	providers.ProvideFeedItemReplyService,
	providers.ProvideFirebaseMessagingClient,
	providers.ProvideWebPushChannel,
	providers.ProvideNotificationService,
	wire.Bind(new(notificationsvc.NotificationServiceInterface), new(*notificationsvc.NotificationService)),
	providers.ProvideDigestRunner,
)

// InitializeContainer creates a new container with Wire-based dependency injection
func InitializeContainer(db *database.DB, cfg *config.Config, logger *slog.Logger) (Container, error) {
	wire.Build(
		ProviderSet,
		NewWireContainer,
	)
	return nil, nil
}
