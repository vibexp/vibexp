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
	registry *idp.Registry,
	eventManager events.EventPublisher,
	logger *slog.Logger,
	featureFlagSvc *feature_flags.FeatureFlagService,
) services.AuthServiceInterface {
	return services.NewAuthService(userRepo, registry, eventManager, logger, featureFlagSvc)
}

// ProvideAPIKeyService creates a new APIKeyService
func ProvideAPIKeyService(
	apiKeyRepo repositories.APIKeyRepository,
	logger *slog.Logger,
) services.APIKeyServiceInterface {
	return services.NewAPIKeyService(apiKeyRepo, logger)
}

// ProvidePromptService creates a new PromptService. Wire fills the deps struct
// via wire.Struct (see the container ProviderSet).
func ProvidePromptService(deps services.PromptServiceDeps) services.PromptServiceInterface {
	return services.NewPromptService(deps)
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

// ProvideContentVersionService creates a new ContentVersionService with the artifact adapter
// registered. New resource types are added by registering further adapters here. The user
// repository resolves version authors for read responses. The retention cap for every
// resource type comes from cfg.Retention.ContentVersionLimit (default 20, 0 = keep all).
func ProvideContentVersionService(
	repo repositories.ContentVersionRepository,
	users repositories.UserRepository,
	cfg *config.Config,
	logger *slog.Logger,
) services.ContentVersionServiceInterface {
	retentionCap := cfg.Retention.ContentVersionLimit
	return services.NewContentVersionService(
		repo,
		users,
		logger,
		services.ContentVersionAdapter{
			ResourceType:        "artifact",
			RetentionCap:        retentionCap,
			InitialVersionLabel: "Created the artifact",
		},
		services.ContentVersionAdapter{
			ResourceType:        "blueprint",
			RetentionCap:        retentionCap,
			InitialVersionLabel: "Created the blueprint",
		},
		services.ContentVersionAdapter{
			ResourceType:        "memory",
			RetentionCap:        retentionCap,
			InitialVersionLabel: "Created the memory",
		},
		services.ContentVersionAdapter{
			ResourceType:        "prompt",
			RetentionCap:        retentionCap,
			InitialVersionLabel: "Created the prompt",
		},
	)
}

// ProvideArtifactService creates a new ArtifactService. Wire fills the deps
// struct via wire.Struct (see the container ProviderSet).
func ProvideArtifactService(deps services.ArtifactServiceDeps) services.ArtifactServiceInterface {
	return services.NewArtifactService(deps)
}

// ProvideCommentService creates a new CommentService.
func ProvideCommentService(
	repo repositories.CommentRepository,
	teamService services.TeamServiceInterface,
	authzService services.AuthorizationServiceInterface,
	logger *slog.Logger,
) services.CommentServiceInterface {
	return services.NewCommentService(repo, teamService, authzService, logger)
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

// ProvideBlueprintService creates a new BlueprintService. Wire fills the deps
// struct via wire.Struct (see the container ProviderSet).
func ProvideBlueprintService(deps services.BlueprintServiceDeps) services.BlueprintServiceInterface {
	return services.NewBlueprintService(deps)
}

// ProvideEmbeddingProviderService creates a new EmbeddingProviderService. Secret
// encryption is delegated to the shared, fail-closed EncryptionService rather than
// a private inline AES implementation keyed off the raw config string (#294).
func ProvideEmbeddingProviderService(
	repo repositories.EmbeddingProviderRepository,
	enc services.EncryptionServiceInterface,
) services.EmbeddingProviderServiceInterface {
	return services.NewEmbeddingProviderService(repo, enc)
}

// ProvideModelProviderService creates a new ModelProviderService. Secret
// encryption is delegated to the shared, fail-closed EncryptionService rather than
// a private inline AES implementation keyed off the raw config string (#294).
func ProvideModelProviderService(
	repo repositories.ModelProviderRepository,
	enc services.EncryptionServiceInterface,
) services.ModelProviderServiceInterface {
	return services.NewModelProviderService(repo, enc)
}

// ProvideEmailService creates a new EmailService
func ProvideEmailService(
	provider external.EmailProvider,
	cfg *config.Config,
) services.EmailServiceInterface {
	return services.NewEmailService(provider, cfg)
}

// ActivityServiceDeps groups the dependencies of ProvideActivityService. Wire
// fills it via wire.Struct (see the container ProviderSet).
type ActivityServiceDeps struct {
	Repo          repositories.ActivityRepository
	ProjectRepo   repositories.ProjectRepository
	PromptRepo    repositories.PromptRepository
	ArtifactRepo  repositories.ArtifactRepository
	UserRepo      repositories.UserRepository
	AgentRepo     repositories.AgentRepository
	BlueprintRepo repositories.BlueprintRepository
	APIKeyRepo    repositories.APIKeyRepository
	MemoryRepo    repositories.MemoryRepository
	Cfg           *config.Config
}

// ProvideActivityService creates a new ActivityService
func ProvideActivityService(deps ActivityServiceDeps) activities.ActivityService {
	return activities.NewService(activities.ServiceDeps{
		Repo:          deps.Repo,
		ProjectRepo:   deps.ProjectRepo,
		PromptRepo:    deps.PromptRepo,
		ArtifactRepo:  deps.ArtifactRepo,
		UserRepo:      deps.UserRepo,
		AgentRepo:     deps.AgentRepo,
		BlueprintRepo: deps.BlueprintRepo,
		APIKeyRepo:    deps.APIKeyRepo,
		MemoryRepo:    deps.MemoryRepo,
		RetentionDays: deps.Cfg.Retention.ActivityDays,
	})
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
	return resourceaccess.NewService(repo, pool, logger, cfg.Retention.AccessEventDays)
}

// ProvideEncryptionService creates a new EncryptionService
func ProvideEncryptionService(cfg *config.Config) (services.EncryptionServiceInterface, error) {
	if cfg.Security.EncryptionKey == "" {
		// Return nil for tests and environments without encryption key
		return nil, nil
	}
	return services.NewEncryptionService(cfg.Security.EncryptionKey)
}

// ProvideAgentService creates a new AgentService
func ProvideAgentService(
	agentRepo repositories.AgentRepository,
	executionRepo repositories.AgentExecutionRepository,
	encryptionService services.EncryptionServiceInterface,
	teamService services.TeamServiceInterface,
	authzService services.AuthorizationServiceInterface,
	cfg *config.Config,
	logger *slog.Logger,
) services.AgentServiceInterface {
	return services.NewAgentService(
		agentRepo, executionRepo, encryptionService, teamService, authzService, cfg, logger,
	)
}

// ProvideAgentCardFetcher creates a new AgentCardFetcher. cfg drives the SSRF
// policy: loopback/private agent card hosts are reachable only in local development.
func ProvideAgentCardFetcher(cfg *config.Config) services.CardFetcher {
	return services.NewAgentCardFetcher(cfg)
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
) services.StreamProcessor {
	return services.NewA2AStreamProcessor(eventRepo, executionRepo, logger)
}

// ProvideAgentInvocationService creates a new AgentInvocationService
func ProvideAgentInvocationService(
	agentRepo repositories.AgentRepository,
	executionRepo repositories.AgentExecutionRepository,
	eventRepo repositories.AgentExecutionEventRepository,
	a2aClient services.A2AHTTPClientInterface,
	streamProcessor services.StreamProcessor,
	logger *slog.Logger,
) services.AgentInvocationServiceInterface {
	return services.NewAgentInvocationService(agentRepo, executionRepo, eventRepo, a2aClient, streamProcessor, logger)
}

// ProvideMemoryService creates a new MemoryService
func ProvideMemoryService(
	repo repositories.MemoryRepository,
	teamService services.TeamServiceInterface,
	authzService services.AuthorizationServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
	contentVersionSvc services.ContentVersionServiceInterface,
	commentRepo repositories.CommentRepository,
) services.MemoryServiceInterface {
	return services.NewMemoryService(
		repo, teamService, authzService, eventManager, logger, contentVersionSvc, commentRepo,
	)
}

// EmbeddingServiceDeps groups the dependencies of ProvideEmbeddingService. Wire
// fills it via wire.Struct (see the container ProviderSet).
type EmbeddingServiceDeps struct {
	Repo              repositories.EmbeddingRepository
	PromptRepo        repositories.PromptRepository
	ArtifactRepo      repositories.ArtifactRepository
	MemoryRepo        repositories.MemoryRepository
	BlueprintRepo     repositories.BlueprintRepository
	FeedItemRepo      repositories.FeedItemRepository
	FeedItemReplyRepo repositories.FeedItemReplyRepository
	Logger            *slog.Logger
}

// ProvideEmbeddingService creates a new EmbeddingService
func ProvideEmbeddingService(deps EmbeddingServiceDeps) services.EmbeddingServiceInterface {
	return services.NewEmbeddingService(services.EmbeddingServiceDeps{
		Repo:              deps.Repo,
		PromptRepo:        deps.PromptRepo,
		ArtifactRepo:      deps.ArtifactRepo,
		MemoryRepo:        deps.MemoryRepo,
		BlueprintRepo:     deps.BlueprintRepo,
		FeedItemRepo:      deps.FeedItemRepo,
		FeedItemReplyRepo: deps.FeedItemReplyRepo,
		Dimensions:        services.EmbeddingVectorDimensions,
		Logger:            deps.Logger,
	})
}

// ProvideQueryEmbedder creates a QueryEmbedder backed by the active system-wide
// embedding provider, so search queries embed with the same provider, model, and
// dimension as documents.
func ProvideQueryEmbedder(
	providerSvc services.EmbeddingProviderServiceInterface,
	logger *slog.Logger,
) services.QueryEmbedder {
	return services.NewProviderQueryEmbedder(providerSvc, logger)
}

// ProvideEmbeddingProcessor creates the events.EmbeddingProcessor the embedding
// worker uses to generate and persist embeddings off the event bus. The
// generation engine resolves the active provider, chunks entity text in Go,
// embeds it, and saves the chunks; the EmbeddingDispatcher wraps it to bound
// fan-out per provider (sized by each provider's concurrency) and keep generation
// off the bus's shared, unbounded worker pool (#142). Retry and resolve-stage
// sizing reuse the event_bus.* knobs so operators tune one set of values.
func ProvideEmbeddingProcessor(
	providerSvc services.EmbeddingProviderServiceInterface,
	embeddingService services.EmbeddingServiceInterface,
	cfg *config.Config,
	logger *slog.Logger,
) events.EmbeddingProcessor {
	engine := services.NewEmbeddingGenerationProcessor(providerSvc, embeddingService, logger)
	return services.NewEmbeddingDispatcher(
		engine,
		cfg.EventBus.WorkerCount,
		services.EmbeddingRetryConfig{
			MaxRetries:  cfg.EventBus.MaxRetries,
			BaseBackoff: cfg.EventBus.RetryBackoff,
			Jitter:      cfg.EventBus.RetryJitter,
		},
		logger,
	)
}

// ProvideSearchService creates a new SearchService, wiring the recency-ranking
// configuration from the typed Config.
func ProvideSearchService(
	repo repositories.SearchRepository,
	embedder services.QueryEmbedder,
	logger *slog.Logger,
	cfg *config.Config,
) services.Searcher {
	ranking := services.SearchRankingConfig{
		Enabled:         cfg.Search.RecencyRankingEnabled,
		WeightRelevance: cfg.Search.RankWeightRelevance,
		WeightCreated:   cfg.Search.RankWeightCreated,
		WeightUpdated:   cfg.Search.RankWeightUpdated,
		HalfLife:        time.Duration(cfg.Search.RankHalfLifeDays * float64(24*time.Hour)),
		CandidateCap:    cfg.Search.RankCandidateCap,
	}
	return services.NewSearchService(repo, embedder, logger, ranking)
}

// ProvideEnvironmentService creates a new EnvironmentService
func ProvideEnvironmentService(cfg *config.Config) *services.EnvironmentService {
	return services.NewEnvironmentService(cfg)
}

// ProvideResourceUsageService creates a new ResourceUsageService. Wire fills
// the deps struct via wire.Struct (see the container ProviderSet).
func ProvideResourceUsageService(deps services.ResourceUsageServiceDeps) services.ResourceUsageServiceInterface {
	return services.NewResourceUsageService(deps)
}

// ProvideFeatureFlagService creates a new FeatureFlagService and registers all feature flags.
//
// The sign-in allowlist is configured from cfg.Auth.AccessAllowlist
// (AUTH_ALLOWED_DOMAINS / AUTH_ALLOWED_EMAILS). Both lists empty means open
// registration; otherwise a user may sign in by exact email or by email domain.
func ProvideFeatureFlagService(cfg *config.Config, logger *slog.Logger) *feature_flags.FeatureFlagService {
	service := feature_flags.NewFeatureFlagService(logger)

	service.RegisterFlag(feature_flags.NewUserSignInAllowlistFlag(
		logger, cfg.Auth.AccessAllowlist.Domains, cfg.Auth.AccessAllowlist.Emails,
	))

	return service
}

// ProvideBackofficeService creates a new BackofficeService
func ProvideBackofficeService(
	backofficeRepo repositories.BackofficeRepository,
) services.UsageAndGrowthGetter {
	return services.NewBackofficeService(backofficeRepo)
}

// ProvideAdminService creates a new AdminService
func ProvideAdminService(
	adminRepo repositories.AdminRepository,
) services.AdminServiceInterface {
	return services.NewAdminService(adminRepo)
}

// ProvideEmbeddingBackfillService creates a new EmbeddingBackfillService that
// republishes `.created` events to regenerate embeddings after a model swap.
func ProvideEmbeddingBackfillService(
	repo repositories.EmbeddingBackfillRepository,
	publisher events.EventPublisher,
	promptService services.PromptServiceInterface,
	logger *slog.Logger,
) services.EmbeddingBackfiller {
	return services.NewEmbeddingBackfillService(repo, publisher, promptService, logger)
}

// ProvideEmbeddingStatusService creates a new EmbeddingStatusService that derives
// per-entity-type embedding coverage from a team's active provider model.
func ProvideEmbeddingStatusService(
	providerRepo repositories.EmbeddingProviderRepository,
	coverageRepo repositories.EmbeddingBackfillRepository,
	logger *slog.Logger,
) services.EmbeddingCoverageGetter {
	return services.NewEmbeddingStatusService(providerRepo, coverageRepo, logger)
}

// ProvideUserPreferencesService creates a new UserPreferencesService
func ProvideUserPreferencesService(
	repo repositories.UserPreferencesRepository,
) services.UserPreferencesServiceInterface {
	return services.NewUserPreferencesService(repo)
}

// ProvideAuthorizationService creates a new AuthorizationService
func ProvideAuthorizationService(
	teamMemberRepo repositories.TeamMemberRepository,
	logger *slog.Logger,
) services.AuthorizationServiceInterface {
	return services.NewAuthorizationService(teamMemberRepo, logger)
}

// ProvideTeamService creates a new TeamService
func ProvideTeamService(
	teamRepo repositories.TeamRepository,
	teamMemberRepo repositories.TeamMemberRepository,
	userRepo repositories.UserRepository,
	authzService services.AuthorizationServiceInterface,
	logger *slog.Logger,
	commentRepo repositories.CommentRepository,
) services.TeamServiceInterface {
	return services.NewTeamService(teamRepo, teamMemberRepo, userRepo, authzService, logger, commentRepo)
}

// ProvideTeamInvitationService creates a new TeamInvitationService. Wire fills
// the deps struct via wire.Struct (see the container ProviderSet).
func ProvideTeamInvitationService(deps services.TeamInvitationServiceDeps) *services.TeamInvitationService {
	return services.NewTeamInvitationService(deps)
}

// ProvideProjectService creates a new ProjectService
func ProvideProjectService(
	repo repositories.ProjectRepository,
	teamService services.TeamServiceInterface,
	authzService services.AuthorizationServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) services.ProjectServiceInterface {
	return services.NewProjectService(repo, teamService, authzService, eventManager, logger)
}

// ProvideFeedService creates a new FeedService
func ProvideFeedService(
	feedRepo repositories.FeedRepository,
	teamService services.TeamServiceInterface,
	authzService services.AuthorizationServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) services.FeedServiceInterface {
	return services.NewFeedService(feedRepo, teamService, authzService, eventManager, logger)
}

// ProvideFeedItemService creates a new FeedItemService
func ProvideFeedItemService(
	feedItemRepo repositories.FeedItemRepository,
	replyRepo repositories.FeedItemReplyRepository,
	projectRepo repositories.ProjectRepository,
	teamService services.TeamServiceInterface,
	authzService services.AuthorizationServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) services.FeedItemServiceInterface {
	return services.NewFeedItemService(
		feedItemRepo, replyRepo, projectRepo, teamService, authzService, eventManager, logger,
	)
}

// ProvideFeedItemReplyService creates a new FeedItemReplyService
func ProvideFeedItemReplyService(
	replyRepo repositories.FeedItemReplyRepository,
	feedItemRepo repositories.FeedItemRepository,
	teamService services.TeamServiceInterface,
	authzService services.AuthorizationServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) services.FeedItemReplyServiceInterface {
	return services.NewFeedItemReplyService(
		replyRepo, feedItemRepo, teamService, authzService, eventManager, logger,
	)
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

// NotificationServiceDeps groups the dependencies of ProvideNotificationService.
// Wire fills it via wire.Struct (see the container ProviderSet).
type NotificationServiceDeps struct {
	NotifRepo    repositories.NotificationRepository
	DeliveryRepo repositories.NotificationDeliveryRepository
	PrefRepo     repositories.UserPreferencesRepository
	UserRepo     repositories.UserRepository
	DigestRepo   repositories.NotificationDigestQueueRepository
	EmailSvc     services.EmailServiceInterface
	WebPushCh    *notifchannels.WebPushChannel
	AppMetrics   *metrics.Metrics
	Logger       *slog.Logger
}

// ProvideNotificationService creates a new NotificationService with all registered channels.
// The WebPush channel is included only when ProvideWebPushChannel returned a non-nil value
// (i.e. FCM is configured). No FCMEnabled() predicate is needed: a nil channel means FCM is
// disabled, and WebPushChannel.Deliver returns StatusSkipped when fcm is nil regardless.
func ProvideNotificationService(deps NotificationServiceDeps) *notificationsvc.NotificationService {
	inAppCh := notifchannels.NewInAppChannel()
	emailCh := notifchannels.NewEmailChannel(deps.EmailSvc, deps.DigestRepo, deps.Logger)

	channels := []notificationsvc.Channel{inAppCh, emailCh}

	// Only append the web push channel when FCM is configured.
	// ProvideWebPushChannel returns nil when FCM is disabled, making this a
	// straightforward nil check with no need for an FCMEnabled() predicate.
	if deps.WebPushCh != nil {
		channels = append(channels, deps.WebPushCh)
	}

	return notificationsvc.NewNotificationService(
		deps.NotifRepo,
		deps.DeliveryRepo,
		deps.PrefRepo,
		deps.UserRepo,
		channels,
		deps.AppMetrics,
		deps.Logger,
	)
}

// DigestRunnerDeps groups the dependencies of ProvideDigestRunner. Wire fills
// it via wire.Struct (see the container ProviderSet).
type DigestRunnerDeps struct {
	Cfg        *config.Config
	DigestRepo repositories.NotificationDigestQueueRepository
	NotifRepo  repositories.NotificationRepository
	UserRepo   repositories.UserRepository
	TeamRepo   repositories.TeamRepository
	PrefRepo   repositories.UserPreferencesRepository
	EmailSvc   services.EmailServiceInterface
	AppMetrics *metrics.Metrics
	Logger     *slog.Logger
}

// ProvideDigestRunner creates a new DigestRunner for the daily notification digest job.
// EmailServiceInterface satisfies notifications.DigestEmailSender via Go structural typing
// (both declare SendNotificationEmail(to, subject, htmlBody string) error).
func ProvideDigestRunner(deps DigestRunnerDeps) *notificationsvc.DigestRunner {
	renderer := notificationsvc.NewTemplateRenderer(deps.Cfg.Frontend.BaseURL)
	return notificationsvc.NewDigestRunner(notificationsvc.DigestRunnerDeps{
		DigestRepo: deps.DigestRepo,
		NotifRepo:  deps.NotifRepo,
		UserRepo:   deps.UserRepo,
		TeamRepo:   deps.TeamRepo,
		PrefRepo:   deps.PrefRepo,
		EmailSvc:   deps.EmailSvc,
		Renderer:   renderer,
		AppMetrics: deps.AppMetrics,
		Logger:     deps.Logger,
	})
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
	attachmentSvc services.AttachmentServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) services.GitHubAppServiceInterface {
	return services.NewGitHubAppService(
		installationRepo,
		projectRepo,
		blueprintRepo,
		githubClient,
		encryptionSvc,
		attachmentSvc,
		eventManager,
		logger,
	)
}
