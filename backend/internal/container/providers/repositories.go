package providers

import (
	"log/slog"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/postgres"
)

// ProvideUserRepository creates a new UserRepository
func ProvideUserRepository(db *database.DB) repositories.UserRepository {
	return postgres.NewUserRepository(db)
}

// ProvideAPIKeyRepository creates a new APIKeyRepository
func ProvideAPIKeyRepository(db *database.DB) repositories.APIKeyRepository {
	return postgres.NewAPIKeyRepository(db)
}

// ProvidePromptRepository creates a new PromptRepository
func ProvidePromptRepository(db *database.DB) repositories.PromptRepository {
	return postgres.NewPromptRepository(db)
}

// ProvidePromptReferenceRepository creates a new PromptReferenceRepository
func ProvidePromptReferenceRepository(db *database.DB) repositories.PromptReferenceRepository {
	return postgres.NewPromptReferenceRepository(db)
}

// ProvidePromptGalleryRepository creates a new PromptGalleryRepository
func ProvidePromptGalleryRepository(db *database.DB) repositories.PromptGalleryRepository {
	return postgres.NewPromptGalleryRepository(db)
}

// ProvidePromptShareRepository creates a new PromptShareRepository
func ProvidePromptShareRepository(db *database.DB) repositories.PromptShareRepository {
	return postgres.NewPromptShareRepository(db)
}

// ProvideArtifactRepository creates a new ArtifactRepository
func ProvideArtifactRepository(db *database.DB) repositories.ArtifactRepository {
	return postgres.NewArtifactRepository(db)
}

// ProvideAttachmentRepository creates a new AttachmentRepository
func ProvideAttachmentRepository(db *database.DB) repositories.AttachmentRepository {
	return postgres.NewAttachmentRepository(db)
}

// ProvideCommentRepository creates a new CommentRepository
func ProvideCommentRepository(db *database.DB) repositories.CommentRepository {
	return postgres.NewCommentRepository(db)
}

// ProvideTypeRepository creates a new TypeRepository
func ProvideTypeRepository(db *database.DB) repositories.TypeRepository {
	return postgres.NewTypeRepository(db)
}

// ProvideBlueprintRepository creates a new BlueprintRepository
func ProvideBlueprintRepository(db *database.DB) repositories.BlueprintRepository {
	return postgres.NewBlueprintRepository(db)
}

// ProvideContentVersionRepository creates a new ContentVersionRepository
func ProvideContentVersionRepository(db *database.DB) repositories.ContentVersionRepository {
	return postgres.NewContentVersionRepository(db)
}

// ProvideEmbeddingProviderRepository creates a new EmbeddingProviderRepository
func ProvideEmbeddingProviderRepository(db *database.DB) repositories.EmbeddingProviderRepository {
	return postgres.NewEmbeddingProviderRepository(db)
}

// ProvideModelProviderRepository creates a new ModelProviderRepository
func ProvideModelProviderRepository(db *database.DB) repositories.ModelProviderRepository {
	return postgres.NewModelProviderRepository(db)
}

// ProvideActivityRepository creates a new ActivityRepository
func ProvideActivityRepository(db *database.DB) repositories.ActivityRepository {
	return postgres.NewActivityRepository(db)
}

// ProvideResourceAccessRepository creates a new ResourceAccessRepository
func ProvideResourceAccessRepository(db *database.DB) repositories.ResourceAccessRepository {
	return postgres.NewResourceAccessRepository(db)
}

// ProvideClaudeCodeHooksRepository creates a new ClaudeCodeHooksRepository
func ProvideClaudeCodeHooksRepository(db *database.DB) repositories.ClaudeCodeHooksRepository {
	return postgres.NewClaudeCodeHooksRepository(db)
}

// ProvideCursorIDEHooksRepository creates a new CursorIDEHooksRepository
func ProvideCursorIDEHooksRepository(db *database.DB) repositories.CursorIDEHooksRepository {
	return postgres.NewCursorIDEHooksRepository(db)
}

// ProvideAgentRepository creates a new AgentRepository
func ProvideAgentRepository(db *database.DB) repositories.AgentRepository {
	return postgres.NewAgentRepository(db)
}

// ProvideAgentExecutionRepository creates a new AgentExecutionRepository
func ProvideAgentExecutionRepository(db *database.DB) repositories.AgentExecutionRepository {
	return postgres.NewAgentExecutionRepository(db)
}

// ProvideAgentExecutionEventRepository creates a new AgentExecutionEventRepository
func ProvideAgentExecutionEventRepository(db *database.DB) repositories.AgentExecutionEventRepository {
	return postgres.NewAgentExecutionEventRepository(db)
}

// ProvideMemoryRepository creates a new MemoryRepository
func ProvideMemoryRepository(db *database.DB) repositories.MemoryRepository {
	return postgres.NewMemoryRepository(db)
}

// ProvideEmbeddingRepository creates a new EmbeddingRepository
func ProvideEmbeddingRepository(db *database.DB) repositories.EmbeddingRepository {
	return postgres.NewEmbeddingRepository(db)
}

// ProvideSearchRepository creates a new SearchRepository
func ProvideSearchRepository(db *database.DB) repositories.SearchRepository {
	return postgres.NewSearchRepository(db)
}

// ProvideEmbeddingBackfillRepository creates a new EmbeddingBackfillRepository
func ProvideEmbeddingBackfillRepository(db *database.DB) repositories.EmbeddingBackfillRepository {
	return postgres.NewEmbeddingBackfillRepository(db)
}

// ProvideResourceUsageRepository creates a new ResourceUsageRepository
func ProvideResourceUsageRepository(db *database.DB, logger *slog.Logger) repositories.ResourceUsageRepository {
	if db == nil || db.DB == nil {
		return nil
	}
	return postgres.NewResourceUsageRepository(db.DB, logger)
}

// ProvideBackofficeRepository creates a new BackofficeRepository
func ProvideBackofficeRepository(db *database.DB) repositories.BackofficeRepository {
	return postgres.NewBackofficeRepository(db)
}

// ProvideAdminRepository creates a new AdminRepository
func ProvideAdminRepository(db *database.DB) repositories.AdminRepository {
	return postgres.NewAdminRepository(db)
}

// ProvideUserPreferencesRepository creates a new UserPreferencesRepository
func ProvideUserPreferencesRepository(db *database.DB) repositories.UserPreferencesRepository {
	return postgres.NewUserPreferencesRepository(db)
}

// ProvideTeamRepository creates a new TeamRepository
func ProvideTeamRepository(db *database.DB) repositories.TeamRepository {
	return postgres.NewTeamRepository(db)
}

// ProvideTeamMemberRepository creates a new TeamMemberRepository
func ProvideTeamMemberRepository(db *database.DB) repositories.TeamMemberRepository {
	return postgres.NewTeamMemberRepository(db)
}

// ProvideTeamInvitationRepository creates a new TeamInvitationRepository
func ProvideTeamInvitationRepository(db *database.DB) repositories.TeamInvitationRepository {
	return postgres.NewTeamInvitationRepository(db)
}

// ProvideTeamSubscriptionRepository creates a new TeamSubscriptionRepository
func ProvideTeamSubscriptionRepository(db *database.DB) repositories.TeamSubscriptionRepository {
	return postgres.NewTeamSubscriptionRepository(db)
}

// ProvideProjectRepository creates a new ProjectRepository
func ProvideProjectRepository(db *database.DB) repositories.ProjectRepository {
	return postgres.NewProjectRepository(db)
}

// ProvideWebhookEventRepository creates a new WebhookEventRepository
func ProvideWebhookEventRepository(db *database.DB) repositories.WebhookEventRepository {
	if db == nil {
		return nil
	}
	return postgres.NewWebhookEventRepository(db.DB)
}

// ProvideGitHubInstallationRepository creates a new GitHubInstallationRepository
func ProvideGitHubInstallationRepository(
	db *database.DB,
	logger *slog.Logger,
) repositories.GitHubInstallationRepository {
	if db == nil || db.DB == nil {
		return nil
	}
	return postgres.NewGitHubInstallationRepository(db.DB, logger)
}

// ProvideFeedRepository creates a new FeedRepository
func ProvideFeedRepository(db *database.DB) repositories.FeedRepository {
	return postgres.NewFeedRepository(db)
}

// ProvideFeedItemRepository creates a new FeedItemRepository
func ProvideFeedItemRepository(db *database.DB) repositories.FeedItemRepository {
	return postgres.NewFeedItemRepository(db)
}

// ProvideFeedItemReplyRepository creates a new FeedItemReplyRepository
func ProvideFeedItemReplyRepository(db *database.DB) repositories.FeedItemReplyRepository {
	return postgres.NewFeedItemReplyRepository(db)
}

// ProvideNotificationRepository creates a new NotificationRepository
func ProvideNotificationRepository(db *database.DB) repositories.NotificationRepository {
	return postgres.NewNotificationRepository(db)
}

// ProvideNotificationDeliveryRepository creates a new NotificationDeliveryRepository
func ProvideNotificationDeliveryRepository(db *database.DB) repositories.NotificationDeliveryRepository {
	return postgres.NewNotificationDeliveryRepository(db)
}

// ProvideNotificationDigestQueueRepository creates a new NotificationDigestQueueRepository
func ProvideNotificationDigestQueueRepository(db *database.DB) repositories.NotificationDigestQueueRepository {
	return postgres.NewNotificationDigestQueueRepository(db)
}

// ProvideDeviceTokenRepository creates a new DeviceTokenRepository
func ProvideDeviceTokenRepository(db *database.DB) repositories.DeviceTokenRepository {
	return postgres.NewDeviceTokenRepository(db)
}
