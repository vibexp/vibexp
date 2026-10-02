package container

import (
	"context"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/scheduler"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	"github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
	"github.com/vibexp/vibexp/pkg/events"
)

// Container interface defines the dependency injection container contract
type Container interface {
	// Notification repositories
	NotificationRepository() repositories.NotificationRepository
	NotificationDeliveryRepository() repositories.NotificationDeliveryRepository
	NotificationDigestQueueRepository() repositories.NotificationDigestQueueRepository

	// Repository methods
	UserRepository() repositories.UserRepository
	APIKeyRepository() repositories.APIKeyRepository
	PromptRepository() repositories.PromptRepository
	PromptGalleryRepository() repositories.PromptGalleryRepository
	PromptShareRepository() repositories.PromptShareRepository
	ArtifactRepository() repositories.ArtifactRepository
	BlueprintRepository() repositories.BlueprintRepository
	EmbeddingProviderRepository() repositories.EmbeddingProviderRepository
	ModelProviderRepository() repositories.ModelProviderRepository
	ActivityRepository() repositories.ActivityRepository
	ResourceAccessRepository() repositories.ResourceAccessRepository
	AgentRepository() repositories.AgentRepository
	AgentExecutionRepository() repositories.AgentExecutionRepository
	AgentExecutionEventRepository() repositories.AgentExecutionEventRepository
	MemoryRepository() repositories.MemoryRepository
	EmbeddingRepository() repositories.EmbeddingRepository
	BackofficeRepository() repositories.BackofficeRepository
	UserPreferencesRepository() repositories.UserPreferencesRepository
	TeamRepository() repositories.TeamRepository
	// TeamSettingsAuditRepository backs the instance-admin read of a team's
	// settings audit log (#1140), which must not go through the role-checked
	// TeamSettingsAuditService.ListAudit.
	TeamSettingsAuditRepository() repositories.TeamSettingsAuditRepository
	// InstanceSettingsAuditRepository backs the instance-admin read of the
	// instance settings audit log (#1189).
	InstanceSettingsAuditRepository() repositories.InstanceSettingsAuditRepository
	TeamMemberRepository() repositories.TeamMemberRepository
	ProjectRepository() repositories.ProjectRepository
	WebhookEventRepository() repositories.WebhookEventRepository
	GitHubInstallationRepository() repositories.GitHubInstallationRepository
	FeedRepository() repositories.FeedRepository
	FeedItemRepository() repositories.FeedItemRepository
	FeedItemReplyRepository() repositories.FeedItemReplyRepository

	// Notification service
	NotificationService() notifications.NotificationServiceInterface
	// DigestRunner runs the daily notification digest job
	DigestRunner() *notifications.DigestRunner

	// Scheduler is the in-process scheduler engine (epic #725). Started and
	// stopped with the container.
	Scheduler() *scheduler.Scheduler

	// Service methods
	AuthService() services.AuthServiceInterface
	APIKeyService() services.APIKeyServiceInterface
	PromptService() services.PromptServiceInterface
	PromptGalleryService() services.PromptGalleryServiceInterface
	PromptShareService() services.PromptShareServiceInterface
	ArtifactService() services.ArtifactServiceInterface
	AttachmentService() services.AttachmentServiceInterface
	CommentService() services.CommentServiceInterface
	RelationService() services.RelationServiceInterface
	RelationSeedService() services.RelationSeedServiceInterface
	TypeService() services.TypeServiceInterface
	BlueprintService() services.BlueprintServiceInterface
	EmbeddingProviderService() services.EmbeddingProviderServiceInterface
	ModelProviderService() services.ModelProviderServiceInterface
	GitHubAppConfigService() services.GitHubAppConfigServiceInterface
	EmailService() services.EmailServiceInterface
	ActivityService() activities.ActivityService
	ResourceAccessService() resourceaccess.ResourceAccessService
	AgentService() services.AgentServiceInterface
	AgentCardFetcher() services.CardFetcher
	AgentInvocationService() services.AgentInvocationServiceInterface
	MemoryService() services.MemoryServiceInterface
	EmbeddingService() services.EmbeddingServiceInterface
	SearchService() services.Searcher
	// SearchSummaryService answers a query from the team's top search results
	// with the team's model provider (#1073).
	SearchSummaryService() services.SearchSummaryServiceInterface
	// AISummaryAvailability reports whether a team can generate an AI Summary;
	// it populates the REST search response's ai_summary field (#1074).
	AISummaryAvailability() services.AISummaryAvailabilityResolver
	TeamEmailProviderService() services.TeamEmailProviderServiceInterface
	// InstanceEmailProviderService serves the instance-admin email settings
	// API (#1189): the database-stored instance provider (#1188).
	InstanceEmailProviderService() services.InstanceEmailProviderServiceInterface
	// InstanceAdminResolver answers who is an instance admin (root from
	// auth.instance_admins, or DB-granted) and grants/revokes DB admins (#1233).
	InstanceAdminResolver() services.InstanceAdminResolver
	// AccessAllowlistResolver decides who may use the instance from the
	// database-stored access allowlist; it is enforced at sign-in, at MCP
	// consent and on every authenticated request (#1235).
	AccessAllowlistResolver() services.AccessAllowlistResolver
	// SetupModeService owns first-run authentication setup: the one-time setup
	// token and the scoped setup session (#1236).
	SetupModeService() services.SetupModeService
	// InstanceSearchSettingsService and InstanceAISummarySettingsService serve
	// the instance-admin search and AI summary settings API (#1200).
	InstanceSearchSettingsService() services.InstanceSearchSettingsServiceInterface
	InstanceAISummarySettingsService() services.InstanceAISummarySettingsServiceInterface
	EmailSenderResolver() services.EmailSenderResolver
	TeamSearchSettingsService() services.TeamSearchSettingsServiceInterface
	// TeamAISummarySettingsService serves the team AI summary settings API
	// (get/update/reset, #1072) — the authoritative, non-fail-open surface.
	TeamAISummarySettingsService() services.TeamAISummarySettingsServiceInterface
	// TeamSettingsAuditService reads and writes the team settings copy audit
	// log (epic #827); the read path is #832.
	TeamSettingsAuditService() services.TeamSettingsAuditServiceInterface
	// FreshnessService manages per-team freshness rules and settings (epic #726).
	FreshnessService() services.FreshnessServiceInterface
	MetadataCatalogService() services.MetadataCatalogServiceInterface
	EnvironmentService() *services.EnvironmentService
	BackofficeService() services.UsageAndGrowthGetter
	AdminService() services.AdminServiceInterface
	EmbeddingBackfillService() services.EmbeddingBackfiller
	EmbeddingStatusService() services.EmbeddingCoverageGetter
	UserPreferencesService() services.UserPreferencesServiceInterface
	AuthorizationService() services.AuthorizationServiceInterface
	TeamService() services.TeamServiceInterface
	TeamInvitationService() *services.TeamInvitationService
	ProjectService() services.ProjectServiceInterface
	ProjectMigrationService() services.ProjectMigrationServiceInterface
	GitHubAppService() services.GitHubAppServiceInterface
	FeedService() services.FeedServiceInterface
	FeedItemService() services.FeedItemServiceInterface
	FeedItemReplyService() services.FeedItemReplyServiceInterface

	// Event system
	EventManager() events.EventPublisher

	// External dependencies
	// IdentityProviderResolver resolves the sign-in providers from the
	// database at runtime (#1234).
	IdentityProviderResolver() services.IdentityProviderResolver
	EmailSender() external.EmailSender

	// Legacy method for database access (TODO: Remove once all handlers use repositories)
	Database() *database.DB

	// StartEventListeners launches event listeners that own background loops
	// deliberately not started at construction time (the embedding dispatcher's
	// durable-queue poller, #820). Call it once the database is migrated and
	// ready; Close drains them again.
	StartEventListeners()

	// RunStartupImports runs the one-release config.yaml → database bridges
	// once, after migrations and before the scheduler and server start (#1190:
	// the deprecated email: section; #1201: search: and ai_summary:). It never
	// fails boot: every problem is logged.
	RunStartupImports(ctx context.Context)

	// Cleanup resources
	Close() error
}
