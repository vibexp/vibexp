package container

import (
	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/repositories"
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
	DeviceTokenRepository() repositories.DeviceTokenRepository

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
	ClaudeCodeHooksRepository() repositories.ClaudeCodeHooksRepository
	CursorIDEHooksRepository() repositories.CursorIDEHooksRepository
	AgentRepository() repositories.AgentRepository
	AgentExecutionRepository() repositories.AgentExecutionRepository
	AgentExecutionEventRepository() repositories.AgentExecutionEventRepository
	MemoryRepository() repositories.MemoryRepository
	EmbeddingRepository() repositories.EmbeddingRepository
	BackofficeRepository() repositories.BackofficeRepository
	UserPreferencesRepository() repositories.UserPreferencesRepository
	TeamRepository() repositories.TeamRepository
	TeamMemberRepository() repositories.TeamMemberRepository
	TeamSubscriptionRepository() repositories.TeamSubscriptionRepository
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

	// Service methods
	AuthService() services.AuthServiceInterface
	APIKeyService() services.APIKeyServiceInterface
	PromptService() services.PromptServiceInterface
	PromptGalleryService() services.PromptGalleryServiceInterface
	PromptShareService() services.PromptShareServiceInterface
	ArtifactService() services.ArtifactServiceInterface
	AttachmentService() services.AttachmentServiceInterface
	CommentService() services.CommentServiceInterface
	TypeService() services.TypeServiceInterface
	BlueprintService() services.BlueprintServiceInterface
	EmbeddingProviderService() services.EmbeddingProviderServiceInterface
	ModelProviderService() services.ModelProviderServiceInterface
	EmailService() services.EmailServiceInterface
	ActivityService() activities.ActivityService
	ResourceAccessService() resourceaccess.ResourceAccessService
	AgentService() services.AgentServiceInterface
	AgentCardFetcher() services.AgentCardFetcherInterface
	AgentInvocationService() services.AgentInvocationServiceInterface
	MemoryService() services.MemoryServiceInterface
	EmbeddingService() services.EmbeddingServiceInterface
	SearchService() services.SearchServiceInterface
	EnvironmentService() *services.EnvironmentService
	ResourceUsageService() services.ResourceUsageServiceInterface
	BackofficeService() services.BackofficeServiceInterface
	AdminService() services.AdminServiceInterface
	EmbeddingBackfillService() services.EmbeddingBackfillServiceInterface
	EmbeddingStatusService() services.EmbeddingStatusServiceInterface
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
	IdentityProviderRegistry() *idp.Registry
	SMTPClient() external.SMTPClient
	GitHubAppClient() external.GitHubAppClient

	// Legacy method for database access (TODO: Remove once all handlers use repositories)
	Database() *database.DB

	// Cleanup resources
	Close() error
}
