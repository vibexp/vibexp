package providers

import (
	"log/slog"
	"time"

	fcmmessaging "firebase.google.com/go/v4/messaging"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/observability/metrics"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	"github.com/vibexp/vibexp/internal/services/feature_flags"
	notificationsvc "github.com/vibexp/vibexp/internal/services/notifications"
	notifchannels "github.com/vibexp/vibexp/internal/services/notifications/channels"
	"github.com/vibexp/vibexp/internal/services/projectmigration"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
	"github.com/vibexp/vibexp/internal/storage"
	"github.com/vibexp/vibexp/pkg/events"
)

// ProvideAuthService creates a new AuthService
func ProvideAuthService(
	userRepo repositories.UserRepository,
	cfg *config.Config,
	identityProvider idp.IdentityProvider,
	eventManager events.EventPublisher,
	logger *slog.Logger,
	featureFlagSvc *feature_flags.FeatureFlagService,
) services.AuthServiceInterface {
	return services.NewAuthService(userRepo, cfg, identityProvider, eventManager, logger, featureFlagSvc)
}

// ProvideAPIKeyService creates a new APIKeyService
func ProvideAPIKeyService(
	apiKeyRepo repositories.APIKeyRepository,
	logger *slog.Logger,
) services.APIKeyServiceInterface {
	return services.NewAPIKeyService(apiKeyRepo, logger)
}

// ProvidePromptService creates a new PromptService
func ProvidePromptService(
	repo repositories.PromptRepository,
	refRepo repositories.PromptReferenceRepository,
	userRepo repositories.UserRepository,
	projectRepo repositories.ProjectRepository,
	teamService services.TeamServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
	contentVersionSvc services.ContentVersionServiceInterface,
) services.PromptServiceInterface {
	return services.NewPromptService(
		repo, refRepo, userRepo, projectRepo, teamService, eventManager, logger, contentVersionSvc,
	)
}

// ProvidePromptGalleryService creates a new PromptGalleryService
func ProvidePromptGalleryService(
	repo repositories.PromptGalleryRepository,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) services.PromptGalleryServiceInterface {
	return services.NewPromptGalleryService(repo, eventManager, logger)
}

// ProvidePromptShareService creates a new PromptShareService
func ProvidePromptShareService(
	shareRepo repositories.PromptShareRepository,
	promptRepo repositories.PromptRepository,
	promptService services.PromptServiceInterface,
	logger *slog.Logger,
) *services.PromptShareService {
	// Cast the interface to concrete type since we know the implementation
	concretePromptService, ok := promptService.(*services.PromptService)
	if !ok {
		// If casting fails, pass nil and the service will handle it gracefully
		concretePromptService = nil
	}
	return services.NewPromptShareService(shareRepo, promptRepo, concretePromptService, logger)
}

// artifactRetentionCap bounds how many content-version snapshots are kept per artifact.
const artifactRetentionCap = 5

// blueprintRetentionCap bounds how many content-version snapshots are kept per blueprint.
const blueprintRetentionCap = 5

// memoryRetentionCap bounds how many content-version snapshots are kept per memory.
const memoryRetentionCap = 5

// promptRetentionCap bounds how many content-version snapshots are kept per prompt.
const promptRetentionCap = 5

// ProvideContentVersionService creates a new ContentVersionService with the artifact adapter
// registered. New resource types are added by registering further adapters here. The user
// repository resolves version authors for read responses.
func ProvideContentVersionService(
	repo repositories.ContentVersionRepository,
	users repositories.UserRepository,
	logger *slog.Logger,
) services.ContentVersionServiceInterface {
	return services.NewContentVersionService(
		repo,
		users,
		logger,
		services.ContentVersionAdapter{
			ResourceType:        "artifact",
			RetentionCap:        artifactRetentionCap,
			InitialVersionLabel: "Created the artifact",
		},
		services.ContentVersionAdapter{
			ResourceType:        "blueprint",
			RetentionCap:        blueprintRetentionCap,
			InitialVersionLabel: "Created the blueprint",
		},
		services.ContentVersionAdapter{
			ResourceType:        "memory",
			RetentionCap:        memoryRetentionCap,
			InitialVersionLabel: "Created the memory",
		},
		services.ContentVersionAdapter{
			ResourceType:        "prompt",
			RetentionCap:        promptRetentionCap,
			InitialVersionLabel: "Created the prompt",
		},
	)
}

// ProvideArtifactService creates a new ArtifactService
func ProvideArtifactService(
	repo repositories.ArtifactRepository,
	teamService services.TeamServiceInterface,
	eventManager events.EventPublisher,
	resourceUsageSvc services.ResourceUsageServiceInterface,
	logger *slog.Logger,
	contentVersionSvc services.ContentVersionServiceInterface,
) services.ArtifactServiceInterface {
	return services.NewArtifactService(repo, teamService, eventManager, resourceUsageSvc, logger, contentVersionSvc)
}

// ProvideAttachmentService creates a new AttachmentService. The object store
// may be nil (storage disabled); the service degrades to 503 in that case.
func ProvideAttachmentService(
	repo repositories.AttachmentRepository,
	store storage.ObjectStore,
	logger *slog.Logger,
) services.AttachmentServiceInterface {
	return services.NewAttachmentService(repo, store, logger)
}

// ProvideTypeService creates a new TypeService
func ProvideTypeService(
	repo repositories.TypeRepository,
	logger *slog.Logger,
) services.TypeServiceInterface {
	return services.NewTypeService(repo, logger)
}

// ProvideBlueprintService creates a new BlueprintService
func ProvideBlueprintService(
	repo repositories.BlueprintRepository,
	teamService services.TeamServiceInterface,
	eventManager events.EventPublisher,
	resourceUsageSvc services.ResourceUsageServiceInterface,
	logger *slog.Logger,
	contentVersionSvc services.ContentVersionServiceInterface,
) services.BlueprintServiceInterface {
	return services.NewBlueprintService(repo, teamService, eventManager, resourceUsageSvc, logger, contentVersionSvc)
}

// ProvideEmbeddingProviderService creates a new EmbeddingProviderService
func ProvideEmbeddingProviderService(
	repo repositories.EmbeddingProviderRepository,
	cfg *config.Config,
) services.EmbeddingProviderServiceInterface {
	return services.NewEmbeddingProviderService(repo, cfg.EncryptionKey)
}

// ProvideEmailService creates a new EmailService
func ProvideEmailService(
	provider external.EmailProvider,
	cfg *config.Config,
) services.EmailServiceInterface {
	return services.NewEmailService(provider, cfg)
}

// ProvideActivityService creates a new ActivityService
func ProvideActivityService(
	repo repositories.ActivityRepository,
	projectRepo repositories.ProjectRepository,
	promptRepo repositories.PromptRepository,
	artifactRepo repositories.ArtifactRepository,
	userRepo repositories.UserRepository,
	agentRepo repositories.AgentRepository,
	blueprintRepo repositories.BlueprintRepository,
	apiKeyRepo repositories.APIKeyRepository,
	memoryRepo repositories.MemoryRepository,
	cfg *config.Config,
) activities.ActivityService {
	return activities.NewService(
		repo, projectRepo, promptRepo, artifactRepo, userRepo,
		agentRepo, blueprintRepo, apiKeyRepo, memoryRepo,
		cfg.ActivityRetentionDays,
	)
}

// resourceAccessWorkerCount is the number of worker goroutines dedicated to
// persisting resource access events off the request path.
const resourceAccessWorkerCount = 5

// ProvideResourceAccessWorkerPool constructs and starts the worker pool used to
// persist resource access events asynchronously. Start() must be called here or
// no workers consume the queue.
//
// A dedicated pool is used (rather than the shared event bus) because recording
// access is a generic off-read-path write, not a domain pub/sub event; routing it
// through the event bus would add needless coupling. Note that WorkerPool.Submit's
// queue-full fallback spawns an unbounded `go task()`, so a sustained burst can
// briefly exceed the worker count.
func ProvideResourceAccessWorkerPool() *events.WorkerPool {
	pool := events.NewWorkerPool(resourceAccessWorkerCount)
	pool.Start()
	return pool
}

// ProvideResourceAccessService creates a new ResourceAccessService
func ProvideResourceAccessService(
	repo repositories.ResourceAccessRepository,
	pool *events.WorkerPool,
	logger *slog.Logger,
	cfg *config.Config,
) resourceaccess.ResourceAccessService {
	return resourceaccess.NewService(repo, pool, logger, cfg.AccessEventRetentionDays)
}

// ProvideEncryptionService creates a new EncryptionService
func ProvideEncryptionService(cfg *config.Config) (services.EncryptionServiceInterface, error) {
	if cfg.EncryptionKey == "" {
		// Return nil for tests and environments without encryption key
		return nil, nil
	}
	return services.NewEncryptionService(cfg.EncryptionKey)
}

// ProvideAgentService creates a new AgentService
func ProvideAgentService(
	agentRepo repositories.AgentRepository,
	executionRepo repositories.AgentExecutionRepository,
	encryptionService services.EncryptionServiceInterface,
	teamService services.TeamServiceInterface,
	logger *slog.Logger,
) services.AgentServiceInterface {
	return services.NewAgentService(agentRepo, executionRepo, encryptionService, teamService, logger)
}

// ProvideAgentCardFetcher creates a new AgentCardFetcher
func ProvideAgentCardFetcher() services.AgentCardFetcherInterface {
	return services.NewAgentCardFetcher()
}

// ProvideAgentAuthenticator creates a new AgentAuthenticator
func ProvideAgentAuthenticator(
	encryptionService services.EncryptionServiceInterface,
) *services.AgentAuthenticator {
	return services.NewAgentAuthenticator(encryptionService)
}

// ProvideA2AHTTPClient creates a new A2AHTTPClient
func ProvideA2AHTTPClient(
	authenticator *services.AgentAuthenticator,
	cfg *config.Config,
) services.A2AHTTPClientInterface {
	return services.NewA2AHTTPClient(authenticator, cfg)
}

// ProvideA2AStreamProcessor creates a new A2AStreamProcessor
func ProvideA2AStreamProcessor(
	eventRepo repositories.AgentExecutionEventRepository,
	executionRepo repositories.AgentExecutionRepository,
	logger *slog.Logger,
) services.A2AStreamProcessorInterface {
	return services.NewA2AStreamProcessor(eventRepo, executionRepo, logger)
}

// ProvideAgentInvocationService creates a new AgentInvocationService
func ProvideAgentInvocationService(
	agentRepo repositories.AgentRepository,
	executionRepo repositories.AgentExecutionRepository,
	a2aClient services.A2AHTTPClientInterface,
	streamProcessor services.A2AStreamProcessorInterface,
	logger *slog.Logger,
) services.AgentInvocationServiceInterface {
	return services.NewAgentInvocationService(agentRepo, executionRepo, a2aClient, streamProcessor, logger)
}

// ProvideMemoryService creates a new MemoryService
func ProvideMemoryService(
	repo repositories.MemoryRepository,
	teamService services.TeamServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
	contentVersionSvc services.ContentVersionServiceInterface,
) services.MemoryServiceInterface {
	return services.NewMemoryService(repo, teamService, eventManager, logger, contentVersionSvc)
}

// ProvideEmbeddingService creates a new EmbeddingService
func ProvideEmbeddingService(
	repo repositories.EmbeddingRepository,
	promptRepo repositories.PromptRepository,
	artifactRepo repositories.ArtifactRepository,
	memoryRepo repositories.MemoryRepository,
	blueprintRepo repositories.BlueprintRepository,
	feedItemRepo repositories.FeedItemRepository,
	feedItemReplyRepo repositories.FeedItemReplyRepository,
	cfg *config.Config,
	logger *slog.Logger,
) services.EmbeddingServiceInterface {
	return services.NewEmbeddingService(
		repo, promptRepo, artifactRepo, memoryRepo, blueprintRepo, feedItemRepo, feedItemReplyRepo,
		cfg.EmbeddingDimensions, logger,
	)
}

// ProvideQueryEmbedder creates a new QueryEmbedder backed by the AI service.
func ProvideQueryEmbedder(cfg *config.Config, logger *slog.Logger) services.QueryEmbedder {
	return services.NewAIQueryEmbedder(services.AIQueryEmbedderConfig{
		AIServiceURL: cfg.AIServiceURL,
		Model:        cfg.EmbeddingModel,
		Dimensions:   cfg.EmbeddingDimensions,
		Logger:       logger,
	})
}

// ProvideSearchService creates a new SearchService, wiring the recency-ranking
// configuration from the typed Config.
func ProvideSearchService(
	repo repositories.SearchRepository,
	embedder services.QueryEmbedder,
	logger *slog.Logger,
	cfg *config.Config,
) services.SearchServiceInterface {
	ranking := services.SearchRankingConfig{
		Enabled:         cfg.SearchRecencyRankingEnabled,
		WeightRelevance: cfg.SearchRankWeightRelevance,
		WeightCreated:   cfg.SearchRankWeightCreated,
		WeightUpdated:   cfg.SearchRankWeightUpdated,
		HalfLife:        time.Duration(cfg.SearchRankHalfLifeDays * float64(24*time.Hour)),
		CandidateCap:    cfg.SearchRankCandidateCap,
	}
	return services.NewSearchService(repo, embedder, logger, ranking, cfg.EmbeddingModel)
}

// ProvideEnvironmentService creates a new EnvironmentService
func ProvideEnvironmentService(cfg *config.Config) *services.EnvironmentService {
	return services.NewEnvironmentService(cfg)
}

// ProvideResourceUsageService creates a new ResourceUsageService
func ProvideResourceUsageService(
	userRepo repositories.UserRepository,
	promptRepo repositories.PromptRepository,
	artifactRepo repositories.ArtifactRepository,
	memoryRepo repositories.MemoryRepository,
	agentRepo repositories.AgentRepository,
	agentExecRepo repositories.AgentExecutionRepository,
	claudeCodeRepo repositories.ClaudeCodeHooksRepository,
	cursorIDERepo repositories.CursorIDEHooksRepository,
	blueprintRepo repositories.BlueprintRepository,
	teamRepo repositories.TeamRepository,
	teamMemberRepo repositories.TeamMemberRepository,
	teamSubscriptionRepo repositories.TeamSubscriptionRepository,
	feedRepo repositories.FeedRepository,
	feedItemRepo repositories.FeedItemRepository,
	feedItemReplyRepo repositories.FeedItemReplyRepository,
	logger *slog.Logger,
) services.ResourceUsageServiceInterface {
	return services.NewResourceUsageService(
		userRepo,
		promptRepo,
		artifactRepo,
		memoryRepo,
		agentRepo,
		agentExecRepo,
		claudeCodeRepo,
		cursorIDERepo,
		blueprintRepo,
		teamRepo,
		teamMemberRepo,
		teamSubscriptionRepo,
		feedRepo,
		feedItemRepo,
		feedItemReplyRepo,
		logger,
	)
}

// ProvideFeatureFlagService creates a new FeatureFlagService and registers all feature flags.
//
// The sign-in allowlist is configured from cfg.SignInAllowedEmails
// (SIGNIN_ALLOWED_EMAILS). An empty list means open registration.
func ProvideFeatureFlagService(cfg *config.Config, logger *slog.Logger) *feature_flags.FeatureFlagService {
	service := feature_flags.NewFeatureFlagService(logger)

	service.RegisterFlag(feature_flags.NewUserSignInAllowlistFlag(logger, cfg.SignInAllowedEmails))

	return service
}

// ProvideBackofficeService creates a new BackofficeService
func ProvideBackofficeService(
	backofficeRepo repositories.BackofficeRepository,
) services.BackofficeServiceInterface {
	return services.NewBackofficeService(backofficeRepo)
}

// ProvideEmbeddingBackfillService creates a new EmbeddingBackfillService that
// republishes `.created` events to regenerate embeddings after a model swap.
func ProvideEmbeddingBackfillService(
	repo repositories.EmbeddingBackfillRepository,
	publisher events.EventPublisher,
	promptService services.PromptServiceInterface,
	cfg *config.Config,
	logger *slog.Logger,
) services.EmbeddingBackfillServiceInterface {
	return services.NewEmbeddingBackfillService(repo, publisher, promptService, cfg.EmbeddingModel, logger)
}

// ProvideUserPreferencesService creates a new UserPreferencesService
func ProvideUserPreferencesService(
	repo repositories.UserPreferencesRepository,
) services.UserPreferencesServiceInterface {
	return services.NewUserPreferencesService(repo)
}

// ProvideTeamService creates a new TeamService
func ProvideTeamService(
	teamRepo repositories.TeamRepository,
	teamMemberRepo repositories.TeamMemberRepository,
	userRepo repositories.UserRepository,
	logger *slog.Logger,
) services.TeamServiceInterface {
	return services.NewTeamService(teamRepo, teamMemberRepo, userRepo, logger)
}

// ProvideTeamInvitationService creates a new TeamInvitationService
func ProvideTeamInvitationService(
	invitationRepo repositories.TeamInvitationRepository,
	teamRepo repositories.TeamRepository,
	teamMemberRepo repositories.TeamMemberRepository,
	userRepo repositories.UserRepository,
	emailService services.EmailServiceInterface,
	cfg *config.Config,
	logger *slog.Logger,
) *services.TeamInvitationService {
	return services.NewTeamInvitationService(
		invitationRepo,
		teamRepo,
		teamMemberRepo,
		userRepo,
		emailService,
		cfg,
		logger,
	)
}

// ProvideProjectService creates a new ProjectService
func ProvideProjectService(
	repo repositories.ProjectRepository,
	teamService services.TeamServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) services.ProjectServiceInterface {
	return services.NewProjectService(repo, teamService, eventManager, logger)
}

// ProvideFeedService creates a new FeedService
func ProvideFeedService(
	feedRepo repositories.FeedRepository,
	teamService services.TeamServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) services.FeedServiceInterface {
	return services.NewFeedService(feedRepo, teamService, eventManager, logger)
}

// ProvideFeedItemService creates a new FeedItemService
func ProvideFeedItemService(
	feedItemRepo repositories.FeedItemRepository,
	replyRepo repositories.FeedItemReplyRepository,
	projectRepo repositories.ProjectRepository,
	teamService services.TeamServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) services.FeedItemServiceInterface {
	return services.NewFeedItemService(feedItemRepo, replyRepo, projectRepo, teamService, eventManager, logger)
}

// ProvideFeedItemReplyService creates a new FeedItemReplyService
func ProvideFeedItemReplyService(
	replyRepo repositories.FeedItemReplyRepository,
	feedItemRepo repositories.FeedItemRepository,
	teamService services.TeamServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) services.FeedItemReplyServiceInterface {
	return services.NewFeedItemReplyService(replyRepo, feedItemRepo, teamService, eventManager, logger)
}

// ProvideWebPushChannel creates a new WebPushChannel. Returns nil when the FCM
// client is nil (FCM_ENABLED=false) so that
// ProvideNotificationService can skip registration via a simple nil check.
// The nil guard here is the single place where FCM-disabled state is handled;
// WebPushChannel.Deliver also defends against nil fcm (returns StatusSkipped),
// but avoiding registration altogether is cleaner.
func ProvideWebPushChannel(
	fcmClient *fcmmessaging.Client,
	tokenRepo repositories.DeviceTokenRepository,
	logger *slog.Logger,
) *notifchannels.WebPushChannel {
	if fcmClient == nil {
		return nil
	}
	return notifchannels.NewWebPushChannel(fcmClient, tokenRepo, logger)
}

// ProvideNotificationService creates a new NotificationService with all registered channels.
// The WebPush channel is included only when ProvideWebPushChannel returned a non-nil value
// (i.e. FCM is configured). No FCMEnabled() predicate is needed: a nil channel means FCM is
// disabled, and WebPushChannel.Deliver returns StatusSkipped when fcm is nil regardless.
func ProvideNotificationService(
	notifRepo repositories.NotificationRepository,
	deliveryRepo repositories.NotificationDeliveryRepository,
	prefRepo repositories.UserPreferencesRepository,
	userRepo repositories.UserRepository,
	digestRepo repositories.NotificationDigestQueueRepository,
	emailSvc services.EmailServiceInterface,
	webPushCh *notifchannels.WebPushChannel,
	appMetrics *metrics.Metrics,
	logger *slog.Logger,
) *notificationsvc.NotificationService {
	inAppCh := notifchannels.NewInAppChannel()
	emailCh := notifchannels.NewEmailChannel(emailSvc, digestRepo, logger)

	channels := []notificationsvc.Channel{inAppCh, emailCh}

	// Only append the web push channel when FCM is configured.
	// ProvideWebPushChannel returns nil when FCM is disabled, making this a
	// straightforward nil check with no need for an FCMEnabled() predicate.
	if webPushCh != nil {
		channels = append(channels, webPushCh)
	}

	return notificationsvc.NewNotificationService(
		notifRepo,
		deliveryRepo,
		prefRepo,
		userRepo,
		channels,
		appMetrics,
		logger,
	)
}

// ProvideDigestRunner creates a new DigestRunner for the daily notification digest job.
// EmailServiceInterface satisfies notifications.DigestEmailSender via Go structural typing
// (both declare SendNotificationEmail(to, subject, htmlBody string) error).
func ProvideDigestRunner(
	cfg *config.Config,
	digestRepo repositories.NotificationDigestQueueRepository,
	notifRepo repositories.NotificationRepository,
	userRepo repositories.UserRepository,
	teamRepo repositories.TeamRepository,
	prefRepo repositories.UserPreferencesRepository,
	emailSvc services.EmailServiceInterface,
	appMetrics *metrics.Metrics,
	logger *slog.Logger,
) *notificationsvc.DigestRunner {
	renderer := notificationsvc.NewTemplateRenderer(cfg.FrontendBaseURL)
	return notificationsvc.NewDigestRunner(
		digestRepo,
		notifRepo,
		userRepo,
		teamRepo,
		prefRepo,
		emailSvc,
		renderer,
		appMetrics,
		logger,
	)
}

// ProvideProjectMigrationService creates a new ProjectMigrationService
func ProvideProjectMigrationService(
	db *database.DB,
	projectRepo repositories.ProjectRepository,
	logger *slog.Logger,
) services.ProjectMigrationServiceInterface {
	return projectmigration.NewService(db, projectRepo, logger)
}

// ProvideGitHubAppService creates a new GitHubAppService
func ProvideGitHubAppService(
	installationRepo repositories.GitHubInstallationRepository,
	projectRepo repositories.ProjectRepository,
	blueprintRepo repositories.BlueprintRepository,
	githubClient external.GitHubAppClient,
	encryptionSvc services.EncryptionServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) services.GitHubAppServiceInterface {
	return services.NewGitHubAppService(
		installationRepo,
		projectRepo,
		blueprintRepo,
		githubClient,
		encryptionSvc,
		eventManager,
		logger,
	)
}
