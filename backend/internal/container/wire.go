//go:build wireinject

package container

import (
	"log/slog"

	"github.com/google/wire"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/container/providers"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/services"
	notificationsvc "github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/pkg/events"
)

// ProviderSet is the Wire provider set for the entire application
var ProviderSet = wire.NewSet(
	// Observability
	providers.ProvideMetrics,

	// External dependencies
	providers.ProvideIdentityProviderRegistry,
	providers.ProvideEmailProvider,
	providers.ProvideSMTPClient,
	providers.ProvideGitHubAppClient,

	// Event system
	providers.ProvideEventManager,
	wire.Bind(new(events.EventPublisher), new(*events.EventManager)),
	wire.Struct(new(providers.EventListenerDeps), "*"),
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
	providers.ProvideCommentRepository,
	providers.ProvideTypeRepository,
	providers.ProvideContentVersionRepository,
	providers.ProvideBlueprintRepository,
	providers.ProvideEmbeddingProviderRepository,
	providers.ProvideModelProviderRepository,
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
	providers.ProvideAdminRepository,
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

	// Provider dependency structs (filled by Wire so provider functions stay
	// under the parameter-count lint limit)
	wire.Struct(new(services.PromptServiceDeps), "*"),
	wire.Struct(new(services.ArtifactServiceDeps), "*"),
	wire.Struct(new(services.BlueprintServiceDeps), "*"),
	wire.Struct(new(services.ResourceUsageServiceDeps), "*"),
	wire.Struct(new(services.TeamInvitationServiceDeps), "*"),
	wire.Struct(new(providers.ActivityServiceDeps), "*"),
	wire.Struct(new(providers.EmbeddingServiceDeps), "*"),
	wire.Struct(new(providers.NotificationServiceDeps), "*"),
	wire.Struct(new(providers.DigestRunnerDeps), "*"),

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
	providers.ProvideCommentService,
	providers.ProvideTypeService,
	providers.ProvideBlueprintService,
	providers.ProvideEmbeddingProviderService,
	providers.ProvideModelProviderService,
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
	providers.ProvideEmbeddingProcessor,
	providers.ProvideBackofficeService,
	providers.ProvideAdminService,
	providers.ProvideEmbeddingBackfillService,
	providers.ProvideEmbeddingStatusService,
	providers.ProvideUserPreferencesService,
	providers.ProvideAuthorizationService,
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

// InitializeContainer creates a new container with Wire-based dependency injection.
// The WireContainer itself is assembled by wire.Struct (field-by-field, replacing a
// hand-written 80+-parameter constructor); Wire may fill its unexported fields
// because the generated injector lives in this same package.
func InitializeContainer(db *database.DB, cfg *config.Config, logger *slog.Logger) (Container, error) {
	wire.Build(
		ProviderSet,
		wire.Struct(new(WireContainer), "*"),
		wire.Bind(new(Container), new(*WireContainer)),
	)
	return nil, nil
}
