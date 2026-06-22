package container

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/container/providers"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	"github.com/vibexp/vibexp/internal/services/feature_flags"
	"github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
	"github.com/vibexp/vibexp/pkg/events"
)

// WireContainer is the Wire-based implementation of the Container interface
type WireContainer struct {
	db     *database.DB
	config *config.Config
	logger *logrus.Logger

	// Repositories
	userRepo                repositories.UserRepository
	apiKeyRepo              repositories.APIKeyRepository
	promptRepo              repositories.PromptRepository
	promptRefRepo           repositories.PromptReferenceRepository
	promptGalleryRepo       repositories.PromptGalleryRepository
	promptShareRepo         repositories.PromptShareRepository
	artifactRepo            repositories.ArtifactRepository
	specLibraryRepo         repositories.BlueprintRepository
	embeddingProviderRepo   repositories.EmbeddingProviderRepository
	activityRepo            repositories.ActivityRepository
	resourceAccessRepo      repositories.ResourceAccessRepository
	claudeCodeHooksRepo     repositories.ClaudeCodeHooksRepository
	cursorIDEHooksRepo      repositories.CursorIDEHooksRepository
	agentRepo               repositories.AgentRepository
	agentExecutionRepo      repositories.AgentExecutionRepository
	agentExecutionEventRepo repositories.AgentExecutionEventRepository
	memoryRepo              repositories.MemoryRepository
	embeddingRepo           repositories.EmbeddingRepository
	resourceUsageRepo       repositories.ResourceUsageRepository
	backofficeRepo          repositories.BackofficeRepository
	userPreferencesRepo     repositories.UserPreferencesRepository
	teamRepo                repositories.TeamRepository
	teamMemberRepo          repositories.TeamMemberRepository
	teamInvitationRepo      repositories.TeamInvitationRepository
	teamSubscriptionRepo    repositories.TeamSubscriptionRepository
	projectRepo             repositories.ProjectRepository
	webhookEventRepo        repositories.WebhookEventRepository
	githubInstallationRepo  repositories.GitHubInstallationRepository
	feedRepo                repositories.FeedRepository
	feedItemRepo            repositories.FeedItemRepository
	feedItemReplyRepo       repositories.FeedItemReplyRepository
	notifRepo               repositories.NotificationRepository
	notifDeliveryRepo       repositories.NotificationDeliveryRepository
	notifDigestQueueRepo    repositories.NotificationDigestQueueRepository
	deviceTokenRepo         repositories.DeviceTokenRepository

	// Services
	authService              services.AuthServiceInterface
	apiKeyService            services.APIKeyServiceInterface
	promptService            services.PromptServiceInterface
	promptGalleryService     services.PromptGalleryServiceInterface
	promptShareService       *services.PromptShareService
	artifactService          services.ArtifactServiceInterface
	attachmentService        services.AttachmentServiceInterface
	typeService              services.TypeServiceInterface
	specLibraryService       services.BlueprintServiceInterface
	embeddingProviderService services.EmbeddingProviderServiceInterface
	emailService             services.EmailServiceInterface
	activityService          activities.ActivityService
	resourceAccessService    resourceaccess.ResourceAccessService
	agentService             services.AgentServiceInterface
	agentCardFetcher         services.AgentCardFetcherInterface
	agentInvocationService   services.AgentInvocationServiceInterface
	memoryService            services.MemoryServiceInterface
	embeddingService         services.EmbeddingServiceInterface
	searchService            services.SearchServiceInterface
	environmentService       *services.EnvironmentService
	resourceUsageService     services.ResourceUsageServiceInterface
	featureFlagService       *feature_flags.FeatureFlagService
	backofficeService        services.BackofficeServiceInterface
	embeddingBackfillService services.EmbeddingBackfillServiceInterface
	userPreferencesService   services.UserPreferencesServiceInterface
	teamService              services.TeamServiceInterface
	teamInvitationService    *services.TeamInvitationService
	projectService           services.ProjectServiceInterface
	projectMigrationService  services.ProjectMigrationServiceInterface
	githubAppService         services.GitHubAppServiceInterface
	feedService              services.FeedServiceInterface
	feedItemService          services.FeedItemServiceInterface
	feedItemReplyService     services.FeedItemReplyServiceInterface
	notificationService      notifications.NotificationServiceInterface
	digestRunner             *notifications.DigestRunner

	// External dependencies
	identityProvider idp.IdentityProvider
	smtpClient       external.SMTPClient
	githubAppClient  external.GitHubAppClient

	// Event system
	eventSystemDeps *providers.EventSystemDeps

	// Async worker pools (drained on Close)
	resourceAccessWorkerPool *events.WorkerPool
}

// NewWireContainer creates a new Wire-based container
//
//nolint:funlen // This function is auto-generated by Wire and needs to wire all dependencies
func NewWireContainer(
	db *database.DB,
	cfg *config.Config,
	logger *logrus.Logger,
	// Repositories
	userRepo repositories.UserRepository,
	apiKeyRepo repositories.APIKeyRepository,
	promptRepo repositories.PromptRepository,
	promptRefRepo repositories.PromptReferenceRepository,
	promptGalleryRepo repositories.PromptGalleryRepository,
	promptShareRepo repositories.PromptShareRepository,
	artifactRepo repositories.ArtifactRepository,
	specLibraryRepo repositories.BlueprintRepository,
	embeddingProviderRepo repositories.EmbeddingProviderRepository,
	activityRepo repositories.ActivityRepository,
	resourceAccessRepo repositories.ResourceAccessRepository,
	claudeCodeHooksRepo repositories.ClaudeCodeHooksRepository,
	cursorIDEHooksRepo repositories.CursorIDEHooksRepository,
	agentRepo repositories.AgentRepository,
	agentExecutionRepo repositories.AgentExecutionRepository,
	agentExecutionEventRepo repositories.AgentExecutionEventRepository,
	memoryRepo repositories.MemoryRepository,
	embeddingRepo repositories.EmbeddingRepository,
	resourceUsageRepo repositories.ResourceUsageRepository,
	backofficeRepo repositories.BackofficeRepository,
	userPreferencesRepo repositories.UserPreferencesRepository,
	teamRepo repositories.TeamRepository,
	teamMemberRepo repositories.TeamMemberRepository,
	teamInvitationRepo repositories.TeamInvitationRepository,
	teamSubscriptionRepo repositories.TeamSubscriptionRepository,
	projectRepo repositories.ProjectRepository,
	webhookEventRepo repositories.WebhookEventRepository,
	githubInstallationRepo repositories.GitHubInstallationRepository,
	feedRepo repositories.FeedRepository,
	feedItemRepo repositories.FeedItemRepository,
	feedItemReplyRepo repositories.FeedItemReplyRepository,
	notifRepo repositories.NotificationRepository,
	notifDeliveryRepo repositories.NotificationDeliveryRepository,
	notifDigestQueueRepo repositories.NotificationDigestQueueRepository,
	deviceTokenRepo repositories.DeviceTokenRepository,
	// Services
	authService services.AuthServiceInterface,
	apiKeyService services.APIKeyServiceInterface,
	promptService services.PromptServiceInterface,
	promptGalleryService services.PromptGalleryServiceInterface,
	promptShareService *services.PromptShareService,
	artifactService services.ArtifactServiceInterface,
	attachmentService services.AttachmentServiceInterface,
	typeService services.TypeServiceInterface,
	specLibraryService services.BlueprintServiceInterface,
	embeddingProviderService services.EmbeddingProviderServiceInterface,
	emailService services.EmailServiceInterface,
	activityService activities.ActivityService,
	resourceAccessService resourceaccess.ResourceAccessService,
	agentService services.AgentServiceInterface,
	agentCardFetcher services.AgentCardFetcherInterface,
	agentInvocationService services.AgentInvocationServiceInterface,
	memoryService services.MemoryServiceInterface,
	embeddingService services.EmbeddingServiceInterface,
	searchService services.SearchServiceInterface,
	environmentService *services.EnvironmentService,
	resourceUsageService services.ResourceUsageServiceInterface,
	featureFlagService *feature_flags.FeatureFlagService,
	backofficeService services.BackofficeServiceInterface,
	embeddingBackfillService services.EmbeddingBackfillServiceInterface,
	userPreferencesService services.UserPreferencesServiceInterface,
	teamService services.TeamServiceInterface,
	teamInvitationService *services.TeamInvitationService,
	projectService services.ProjectServiceInterface,
	projectMigrationService services.ProjectMigrationServiceInterface,
	githubAppService services.GitHubAppServiceInterface,
	feedService services.FeedServiceInterface,
	feedItemService services.FeedItemServiceInterface,
	feedItemReplyService services.FeedItemReplyServiceInterface,
	notificationService notifications.NotificationServiceInterface,
	digestRunner *notifications.DigestRunner,
	// External dependencies
	identityProvider idp.IdentityProvider,
	smtpClient external.SMTPClient,
	githubAppClient external.GitHubAppClient,
	// Event system
	eventSystemDeps *providers.EventSystemDeps,
	// Async worker pools
	resourceAccessWorkerPool *events.WorkerPool,
) Container {
	return &WireContainer{
		db:     db,
		config: cfg,
		logger: logger,
		// Repositories
		userRepo:                userRepo,
		apiKeyRepo:              apiKeyRepo,
		promptRepo:              promptRepo,
		promptRefRepo:           promptRefRepo,
		promptGalleryRepo:       promptGalleryRepo,
		promptShareRepo:         promptShareRepo,
		artifactRepo:            artifactRepo,
		specLibraryRepo:         specLibraryRepo,
		embeddingProviderRepo:   embeddingProviderRepo,
		activityRepo:            activityRepo,
		resourceAccessRepo:      resourceAccessRepo,
		claudeCodeHooksRepo:     claudeCodeHooksRepo,
		cursorIDEHooksRepo:      cursorIDEHooksRepo,
		agentRepo:               agentRepo,
		agentExecutionRepo:      agentExecutionRepo,
		agentExecutionEventRepo: agentExecutionEventRepo,
		memoryRepo:              memoryRepo,
		embeddingRepo:           embeddingRepo,
		resourceUsageRepo:       resourceUsageRepo,
		backofficeRepo:          backofficeRepo,
		userPreferencesRepo:     userPreferencesRepo,
		teamRepo:                teamRepo,
		teamMemberRepo:          teamMemberRepo,
		teamInvitationRepo:      teamInvitationRepo,
		teamSubscriptionRepo:    teamSubscriptionRepo,
		projectRepo:             projectRepo,
		webhookEventRepo:        webhookEventRepo,
		githubInstallationRepo:  githubInstallationRepo,
		feedRepo:                feedRepo,
		feedItemRepo:            feedItemRepo,
		feedItemReplyRepo:       feedItemReplyRepo,
		notifRepo:               notifRepo,
		notifDeliveryRepo:       notifDeliveryRepo,
		notifDigestQueueRepo:    notifDigestQueueRepo,
		deviceTokenRepo:         deviceTokenRepo,
		// Services
		authService:              authService,
		apiKeyService:            apiKeyService,
		promptService:            promptService,
		promptGalleryService:     promptGalleryService,
		promptShareService:       promptShareService,
		artifactService:          artifactService,
		attachmentService:        attachmentService,
		typeService:              typeService,
		specLibraryService:       specLibraryService,
		embeddingProviderService: embeddingProviderService,
		emailService:             emailService,
		activityService:          activityService,
		resourceAccessService:    resourceAccessService,
		agentService:             agentService,
		agentCardFetcher:         agentCardFetcher,
		agentInvocationService:   agentInvocationService,
		memoryService:            memoryService,
		embeddingService:         embeddingService,
		searchService:            searchService,
		environmentService:       environmentService,
		resourceUsageService:     resourceUsageService,
		featureFlagService:       featureFlagService,
		backofficeService:        backofficeService,
		embeddingBackfillService: embeddingBackfillService,
		userPreferencesService:   userPreferencesService,
		teamService:              teamService,
		teamInvitationService:    teamInvitationService,
		projectService:           projectService,
		projectMigrationService:  projectMigrationService,
		githubAppService:         githubAppService,
		feedService:              feedService,
		feedItemService:          feedItemService,
		feedItemReplyService:     feedItemReplyService,
		notificationService:      notificationService,
		digestRunner:             digestRunner,
		// External dependencies
		identityProvider: identityProvider,
		smtpClient:       smtpClient,
		githubAppClient:  githubAppClient,
		// Event system
		eventSystemDeps: eventSystemDeps,
		// Async worker pools
		resourceAccessWorkerPool: resourceAccessWorkerPool,
	}
}

// NotificationRepository returns the notification repository
func (c *WireContainer) NotificationRepository() repositories.NotificationRepository {
	return c.notifRepo
}

// NotificationDeliveryRepository returns the notification delivery repository
func (c *WireContainer) NotificationDeliveryRepository() repositories.NotificationDeliveryRepository {
	return c.notifDeliveryRepo
}

// NotificationDigestQueueRepository returns the notification digest queue repository
func (c *WireContainer) NotificationDigestQueueRepository() repositories.NotificationDigestQueueRepository {
	return c.notifDigestQueueRepo
}

// DeviceTokenRepository returns the device token repository
func (c *WireContainer) DeviceTokenRepository() repositories.DeviceTokenRepository {
	return c.deviceTokenRepo
}

// NotificationService returns the notification service
func (c *WireContainer) NotificationService() notifications.NotificationServiceInterface {
	return c.notificationService
}

// DigestRunner returns the notification digest runner
func (c *WireContainer) DigestRunner() *notifications.DigestRunner {
	return c.digestRunner
}

// Repository methods
func (c *WireContainer) UserRepository() repositories.UserRepository {
	return c.userRepo
}

func (c *WireContainer) APIKeyRepository() repositories.APIKeyRepository {
	return c.apiKeyRepo
}

func (c *WireContainer) PromptRepository() repositories.PromptRepository {
	return c.promptRepo
}

func (c *WireContainer) PromptGalleryRepository() repositories.PromptGalleryRepository {
	return c.promptGalleryRepo
}

func (c *WireContainer) PromptShareRepository() repositories.PromptShareRepository {
	return c.promptShareRepo
}

func (c *WireContainer) ArtifactRepository() repositories.ArtifactRepository {
	return c.artifactRepo
}

func (c *WireContainer) BlueprintRepository() repositories.BlueprintRepository {
	return c.specLibraryRepo
}

func (c *WireContainer) EmbeddingProviderRepository() repositories.EmbeddingProviderRepository {
	return c.embeddingProviderRepo
}

func (c *WireContainer) ActivityRepository() repositories.ActivityRepository {
	return c.activityRepo
}

func (c *WireContainer) ResourceAccessRepository() repositories.ResourceAccessRepository {
	return c.resourceAccessRepo
}

func (c *WireContainer) ClaudeCodeHooksRepository() repositories.ClaudeCodeHooksRepository {
	return c.claudeCodeHooksRepo
}

func (c *WireContainer) CursorIDEHooksRepository() repositories.CursorIDEHooksRepository {
	return c.cursorIDEHooksRepo
}

func (c *WireContainer) AgentRepository() repositories.AgentRepository {
	return c.agentRepo
}

func (c *WireContainer) AgentExecutionRepository() repositories.AgentExecutionRepository {
	return c.agentExecutionRepo
}

func (c *WireContainer) AgentExecutionEventRepository() repositories.AgentExecutionEventRepository {
	return c.agentExecutionEventRepo
}

func (c *WireContainer) MemoryRepository() repositories.MemoryRepository {
	return c.memoryRepo
}

func (c *WireContainer) EmbeddingRepository() repositories.EmbeddingRepository {
	return c.embeddingRepo
}

func (c *WireContainer) BackofficeRepository() repositories.BackofficeRepository {
	return c.backofficeRepo
}

func (c *WireContainer) UserPreferencesRepository() repositories.UserPreferencesRepository {
	return c.userPreferencesRepo
}

func (c *WireContainer) TeamRepository() repositories.TeamRepository {
	return c.teamRepo
}

func (c *WireContainer) TeamMemberRepository() repositories.TeamMemberRepository {
	return c.teamMemberRepo
}

func (c *WireContainer) TeamSubscriptionRepository() repositories.TeamSubscriptionRepository {
	return c.teamSubscriptionRepo
}

func (c *WireContainer) ProjectRepository() repositories.ProjectRepository {
	return c.projectRepo
}

func (c *WireContainer) WebhookEventRepository() repositories.WebhookEventRepository {
	return c.webhookEventRepo
}

func (c *WireContainer) GitHubInstallationRepository() repositories.GitHubInstallationRepository {
	return c.githubInstallationRepo
}

func (c *WireContainer) FeedRepository() repositories.FeedRepository {
	return c.feedRepo
}

func (c *WireContainer) FeedItemRepository() repositories.FeedItemRepository {
	return c.feedItemRepo
}

func (c *WireContainer) FeedItemReplyRepository() repositories.FeedItemReplyRepository {
	return c.feedItemReplyRepo
}

// Service methods
func (c *WireContainer) AuthService() services.AuthServiceInterface {
	return c.authService
}

func (c *WireContainer) APIKeyService() services.APIKeyServiceInterface {
	return c.apiKeyService
}

func (c *WireContainer) PromptService() services.PromptServiceInterface {
	return c.promptService
}

func (c *WireContainer) PromptGalleryService() services.PromptGalleryServiceInterface {
	return c.promptGalleryService
}

func (c *WireContainer) PromptShareService() services.PromptShareServiceInterface {
	return c.promptShareService
}

func (c *WireContainer) ArtifactService() services.ArtifactServiceInterface {
	return c.artifactService
}

func (c *WireContainer) AttachmentService() services.AttachmentServiceInterface {
	return c.attachmentService
}

func (c *WireContainer) TypeService() services.TypeServiceInterface {
	return c.typeService
}

func (c *WireContainer) BlueprintService() services.BlueprintServiceInterface {
	return c.specLibraryService
}

func (c *WireContainer) EmbeddingProviderService() services.EmbeddingProviderServiceInterface {
	return c.embeddingProviderService
}

func (c *WireContainer) EmailService() services.EmailServiceInterface {
	return c.emailService
}

func (c *WireContainer) ActivityService() activities.ActivityService {
	return c.activityService
}

func (c *WireContainer) ResourceAccessService() resourceaccess.ResourceAccessService {
	return c.resourceAccessService
}

func (c *WireContainer) AgentService() services.AgentServiceInterface {
	return c.agentService
}

func (c *WireContainer) AgentCardFetcher() services.AgentCardFetcherInterface {
	return c.agentCardFetcher
}

func (c *WireContainer) AgentInvocationService() services.AgentInvocationServiceInterface {
	return c.agentInvocationService
}

func (c *WireContainer) MemoryService() services.MemoryServiceInterface {
	return c.memoryService
}

func (c *WireContainer) EmbeddingService() services.EmbeddingServiceInterface {
	return c.embeddingService
}

func (c *WireContainer) SearchService() services.SearchServiceInterface {
	return c.searchService
}

func (c *WireContainer) EnvironmentService() *services.EnvironmentService {
	return c.environmentService
}

func (c *WireContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return c.resourceUsageService
}

func (c *WireContainer) BackofficeService() services.BackofficeServiceInterface {
	return c.backofficeService
}

// EmbeddingBackfillService returns the embedding backfill service.
func (c *WireContainer) EmbeddingBackfillService() services.EmbeddingBackfillServiceInterface {
	return c.embeddingBackfillService
}

func (c *WireContainer) UserPreferencesService() services.UserPreferencesServiceInterface {
	return c.userPreferencesService
}

func (c *WireContainer) TeamService() services.TeamServiceInterface {
	return c.teamService
}

func (c *WireContainer) TeamInvitationService() *services.TeamInvitationService {
	return c.teamInvitationService
}

func (c *WireContainer) ProjectService() services.ProjectServiceInterface {
	return c.projectService
}

func (c *WireContainer) ProjectMigrationService() services.ProjectMigrationServiceInterface {
	return c.projectMigrationService
}

func (c *WireContainer) GitHubAppService() services.GitHubAppServiceInterface {
	return c.githubAppService
}

func (c *WireContainer) FeedService() services.FeedServiceInterface {
	return c.feedService
}

func (c *WireContainer) FeedItemService() services.FeedItemServiceInterface {
	return c.feedItemService
}

func (c *WireContainer) FeedItemReplyService() services.FeedItemReplyServiceInterface {
	return c.feedItemReplyService
}

// External dependencies
func (c *WireContainer) IdentityProvider() idp.IdentityProvider {
	return c.identityProvider
}

func (c *WireContainer) SMTPClient() external.SMTPClient {
	return c.smtpClient
}

func (c *WireContainer) GitHubAppClient() external.GitHubAppClient {
	return c.githubAppClient
}

// Legacy method for database access
func (c *WireContainer) Database() *database.DB {
	return c.db
}

// EventManager returns the event manager
func (c *WireContainer) EventManager() events.EventPublisher {
	if c.eventSystemDeps != nil {
		return c.eventSystemDeps.EventManager
	}
	return nil
}

// Close cleans up resources
func (c *WireContainer) Close() error {
	// Stop event manager
	if c.eventSystemDeps != nil && c.eventSystemDeps.EventManager != nil {
		if err := c.eventSystemDeps.EventManager.Stop(); err != nil {
			c.logger.WithFields(logrus.Fields{
				"service": "vibexp-api",
				"error":   fmt.Sprintf("%+v", err),
			}).Error("Failed to stop event manager")
		}
	}

	// Drain the resource-access worker pool (best-effort flush of buffered
	// access events) in the same shutdown phase as the event manager.
	if c.resourceAccessWorkerPool != nil {
		c.resourceAccessWorkerPool.Stop()
	}

	return nil
}
