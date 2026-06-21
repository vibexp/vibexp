package server

import (
	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/container"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	"github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
	"github.com/vibexp/vibexp/pkg/events"
)

// BaseMockContainer provides default nil implementations for all Container interface methods.
// Test-specific mock containers can embed this and only override methods they need.
//
// Usage Example:
//
//	type MockPromptContainer struct {
//	    BaseMockContainer // Embed base container
//	    promptService *mocks.MockPromptServiceInterface
//	}
//
//	func (m *MockPromptContainer) PromptService() services.PromptServiceInterface {
//	    return m.promptService
//	}
//
// This pattern eliminates boilerplate by providing default nil implementations for all
// Container interface methods, allowing test files to only override what they need.
type BaseMockContainer struct{}

// Compile-time interface compliance check
var _ container.Container = (*BaseMockContainer)(nil)

// Repository methods - all return nil by default
func (b *BaseMockContainer) UserRepository() repositories.UserRepository {
	return nil
}

func (b *BaseMockContainer) APIKeyRepository() repositories.APIKeyRepository {
	return nil
}

func (b *BaseMockContainer) PromptRepository() repositories.PromptRepository {
	return nil
}

func (b *BaseMockContainer) PromptGalleryRepository() repositories.PromptGalleryRepository {
	return nil
}

func (b *BaseMockContainer) PromptShareRepository() repositories.PromptShareRepository {
	return nil
}

func (b *BaseMockContainer) ArtifactRepository() repositories.ArtifactRepository {
	return nil
}

func (b *BaseMockContainer) BlueprintRepository() repositories.BlueprintRepository {
	return nil
}

func (b *BaseMockContainer) EmbeddingProviderRepository() repositories.EmbeddingProviderRepository {
	return nil
}

func (b *BaseMockContainer) ActivityRepository() repositories.ActivityRepository {
	return nil
}

func (b *BaseMockContainer) ResourceAccessRepository() repositories.ResourceAccessRepository {
	return nil
}

func (b *BaseMockContainer) ClaudeCodeHooksRepository() repositories.ClaudeCodeHooksRepository {
	return nil
}

func (b *BaseMockContainer) CursorIDEHooksRepository() repositories.CursorIDEHooksRepository {
	return nil
}

func (b *BaseMockContainer) AgentRepository() repositories.AgentRepository {
	return nil
}

func (b *BaseMockContainer) AgentExecutionRepository() repositories.AgentExecutionRepository {
	return nil
}

func (b *BaseMockContainer) AgentExecutionEventRepository() repositories.AgentExecutionEventRepository {
	return nil
}

func (b *BaseMockContainer) MemoryRepository() repositories.MemoryRepository {
	return nil
}

func (b *BaseMockContainer) EmbeddingRepository() repositories.EmbeddingRepository {
	return nil
}

func (b *BaseMockContainer) BackofficeRepository() repositories.BackofficeRepository {
	return nil
}

func (b *BaseMockContainer) UserPreferencesRepository() repositories.UserPreferencesRepository {
	return nil
}

func (b *BaseMockContainer) TeamRepository() repositories.TeamRepository {
	return nil
}

func (b *BaseMockContainer) TeamMemberRepository() repositories.TeamMemberRepository {
	return nil
}

func (b *BaseMockContainer) TeamSubscriptionRepository() repositories.TeamSubscriptionRepository {
	return nil
}

func (b *BaseMockContainer) ProjectRepository() repositories.ProjectRepository {
	return nil
}

func (b *BaseMockContainer) WebhookEventRepository() repositories.WebhookEventRepository {
	return nil
}

func (b *BaseMockContainer) GitHubInstallationRepository() repositories.GitHubInstallationRepository {
	return nil
}

func (b *BaseMockContainer) FeedRepository() repositories.FeedRepository {
	return nil
}

func (b *BaseMockContainer) FeedItemRepository() repositories.FeedItemRepository {
	return nil
}

// Service methods - all return nil by default
func (b *BaseMockContainer) AuthService() services.AuthServiceInterface {
	return nil
}

func (b *BaseMockContainer) APIKeyService() services.APIKeyServiceInterface {
	return nil
}

func (b *BaseMockContainer) PromptService() services.PromptServiceInterface {
	return nil
}

func (b *BaseMockContainer) PromptGalleryService() services.PromptGalleryServiceInterface {
	return nil
}

func (b *BaseMockContainer) PromptShareService() services.PromptShareServiceInterface {
	return nil
}

func (b *BaseMockContainer) ArtifactService() services.ArtifactServiceInterface {
	return nil
}

func (b *BaseMockContainer) AttachmentService() services.AttachmentServiceInterface {
	return nil
}

func (b *BaseMockContainer) TypeService() services.TypeServiceInterface {
	return nil
}

func (b *BaseMockContainer) BlueprintService() services.BlueprintServiceInterface {
	return nil
}

func (b *BaseMockContainer) EmbeddingProviderService() services.EmbeddingProviderServiceInterface {
	return nil
}

func (b *BaseMockContainer) EmailService() services.EmailServiceInterface {
	return nil
}

func (b *BaseMockContainer) ResourceAccessService() resourceaccess.ResourceAccessService {
	return nil
}

func (b *BaseMockContainer) ActivityService() activities.ActivityService {
	return nil
}

func (b *BaseMockContainer) AgentService() services.AgentServiceInterface {
	return nil
}

func (b *BaseMockContainer) AgentCardFetcher() services.AgentCardFetcherInterface {
	return nil
}

func (b *BaseMockContainer) AgentInvocationService() services.AgentInvocationServiceInterface {
	return nil
}

func (b *BaseMockContainer) MemoryService() services.MemoryServiceInterface {
	return nil
}

func (b *BaseMockContainer) EmbeddingService() services.EmbeddingServiceInterface {
	return nil
}

func (b *BaseMockContainer) SearchService() services.SearchServiceInterface {
	return nil
}

func (b *BaseMockContainer) EnvironmentService() *services.EnvironmentService {
	return nil
}

func (b *BaseMockContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return nil
}

func (b *BaseMockContainer) BackofficeService() services.BackofficeServiceInterface {
	return nil
}

func (b *BaseMockContainer) EmbeddingBackfillService() services.EmbeddingBackfillServiceInterface {
	return nil
}

func (b *BaseMockContainer) UserPreferencesService() services.UserPreferencesServiceInterface {
	return nil
}

func (b *BaseMockContainer) TeamService() services.TeamServiceInterface {
	return nil
}

func (b *BaseMockContainer) TeamInvitationService() *services.TeamInvitationService {
	return nil
}

func (b *BaseMockContainer) ProjectService() services.ProjectServiceInterface {
	return nil
}

func (b *BaseMockContainer) ProjectMigrationService() services.ProjectMigrationServiceInterface {
	return nil
}

func (b *BaseMockContainer) GitHubAppService() services.GitHubAppServiceInterface {
	return nil
}

func (b *BaseMockContainer) FeedService() services.FeedServiceInterface {
	return nil
}

func (b *BaseMockContainer) FeedItemService() services.FeedItemServiceInterface {
	return nil
}

func (b *BaseMockContainer) FeedItemReplyService() services.FeedItemReplyServiceInterface {
	return nil
}

func (b *BaseMockContainer) FeedItemReplyRepository() repositories.FeedItemReplyRepository {
	return nil
}

// Notification repositories
func (b *BaseMockContainer) NotificationRepository() repositories.NotificationRepository {
	return nil
}

func (b *BaseMockContainer) NotificationDeliveryRepository() repositories.NotificationDeliveryRepository {
	return nil
}

func (b *BaseMockContainer) NotificationDigestQueueRepository() repositories.NotificationDigestQueueRepository {
	return nil
}

func (b *BaseMockContainer) DeviceTokenRepository() repositories.DeviceTokenRepository {
	return nil
}

// Notification service
func (b *BaseMockContainer) NotificationService() notifications.NotificationServiceInterface {
	return nil
}

// DigestRunner returns a nil DigestRunner (not used in most tests)
func (b *BaseMockContainer) DigestRunner() *notifications.DigestRunner {
	return nil
}

// Event system
func (b *BaseMockContainer) EventManager() events.EventPublisher {
	return nil
}

// External dependencies
func (b *BaseMockContainer) IdentityProvider() idp.IdentityProvider {
	return nil
}

func (b *BaseMockContainer) SMTPClient() external.SMTPClient {
	return nil
}

func (b *BaseMockContainer) GitHubAppClient() external.GitHubAppClient {
	return nil
}

// Legacy method for database access
func (b *BaseMockContainer) Database() *database.DB {
	return nil
}

// Cleanup resources
func (b *BaseMockContainer) Close() error {
	return nil
}
