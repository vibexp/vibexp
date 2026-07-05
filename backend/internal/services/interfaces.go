package services

import (
	"context"
	"io"
	"time"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services/projectmigration"
)

// AuthServiceInterface defines the interface for authentication operations.
// JWT-based authentication is removed; sessions are managed via the session
// package (AES-GCM encrypted httpOnly cookies).
type AuthServiceInterface interface {
	// EnabledProviders returns the canonical names of the enabled login
	// providers (stable-sorted), e.g. ["github", "google"].
	EnabledProviders() []string
	// GetLoginURL returns the authorization URL for the named provider, or an
	// empty string when the provider is not enabled.
	GetLoginURL(state, provider string) string
	// HandleCallback exchanges the authorization code using the named provider,
	// looks up/creates the user, and returns the user, IDP tokens, and whether
	// the user is newly created.
	HandleCallback(ctx context.Context, code, provider string) (*models.User, *idp.Tokens, bool, error)
	// RefreshTokens exchanges a refresh token for new access/refresh tokens
	// using the named provider.
	RefreshTokens(ctx context.Context, provider, refreshToken string) (*idp.Tokens, error)
	// ProvisionFromClaims resolves or creates a user from upstream IdP claims,
	// used by the embedded OAuth Authorization Server's login leg (issue #31).
	ProvisionFromClaims(ctx context.Context, providerName string, claims *idp.Claims) (*models.User, error)
	// HandleDevLogin creates or retrieves a dev user by email (dev env only).
	// The caller is responsible for creating the session cookie.
	HandleDevLogin(ctx context.Context, email, name string) (*models.User, error)
	// GetUserByID retrieves a user by their internal ID.
	GetUserByID(ctx context.Context, userID string) (*models.User, error)
}

// APIKeyServiceInterface defines the interface for API key operations
type APIKeyServiceInterface interface {
	// GenerateAPIKey creates a new API key with multi-integration support
	GenerateAPIKey(
		ctx context.Context, userID, name string, integrationCodes []string,
	) (*models.APIKey, string, error)
	// GenerateAPIKeyLegacy is deprecated, use GenerateAPIKey instead
	GenerateAPIKeyLegacy(
		ctx context.Context, userID, name, usageType string,
	) (*models.APIKey, string, error)
	GetAPIKeysByUserID(ctx context.Context, userID string) ([]models.APIKey, error)
	ValidateAPIKey(ctx context.Context, key string) (*models.APIKey, error)
	ValidateAPIKeyForIntegration(ctx context.Context, key, integrationCode string) (*models.APIKey, error)
	DeleteAPIKey(ctx context.Context, userID, apiKeyID string) error
}

// PromptServiceInterface defines the interface for prompt operations
type PromptServiceInterface interface {
	CreatePrompt(userID, teamID string, req *models.CreatePromptRequest) (*models.Prompt, error)
	GetPrompt(userID, teamID, promptID string) (*models.Prompt, error)
	GetPromptBySlug(userID, teamID, slug string) (*models.Prompt, error)
	ListPrompts(userID string, filters PromptFilters) (*models.PromptListResponse, error)
	UpdatePromptBySlug(userID, teamID, slug string, req *models.UpdatePromptRequest) (*models.Prompt, error)
	UpdatePrompt(userID, teamID, promptID string, req *models.UpdatePromptRequest) (*models.Prompt, error)
	DeletePromptBySlug(userID, teamID, slug string) error
	DeletePrompt(userID, teamID, promptID string) error
	RenderPrompt(userID, teamID, slug string, placeholders map[string]string) (*models.RenderPromptResponse, error)
	RenderPromptBody(userID, body string) (string, error)
	GetPromptPlaceholders(userID, teamID, slug string) ([]string, error)
	ExtractAllPlaceholders(userID, body string, visitedRefs map[string]bool) ([]string, error)
	GetPromptDependencies(userID, teamID, promptID string) (*models.PromptDependenciesResponse, error)
	GetPromptDependenciesBySlug(userID, teamID, slug string) (*models.PromptDependenciesResponse, error)
	GetUserLabels(userID string) ([]string, error)
	// ListPromptVersions returns the content-version history (newest-first) for a prompt
	// identified by slug. The prompt is loaded through the authorization-enforcing
	// team-scoped lookup before its versions are read.
	ListPromptVersions(userID, teamID, slug string) ([]*models.ContentVersion, error)
	// GetPromptVersion returns a single content version of a prompt by version number.
	GetPromptVersion(
		userID, teamID, slug string, versionNumber int,
	) (*models.ContentVersion, error)
	// RestorePromptVersion restores a prompt's raw Body template to the given version,
	// returning the updated prompt.
	RestorePromptVersion(
		userID, teamID, slug string, versionNumber int,
	) (*models.Prompt, error)
}

// PromptShareServiceInterface defines the interface for prompt sharing operations
type PromptShareServiceInterface interface {
	CreateShare(userID, promptSlug string, req *models.CreateShareRequest) (*models.ShareResponse, error)
	GetShare(userID, promptSlug string) (*models.ShareResponse, error)
	DeleteShare(userID, promptSlug string) error
	GetSharedPrompt(token string, userEmail *string) (*models.SharedPromptResponse, error)
}

// ArtifactServiceInterface defines the interface for artifact operations
type ArtifactServiceInterface interface {
	CreateArtifact(userID, teamID string, req *models.CreateArtifactRequest) (*models.Artifact, error)
	GetArtifactByProjectIDAndSlug(userID, projectID, slug string) (*models.Artifact, error)
	// GetArtifactByProjectIDAndSlugInTeam retrieves an artifact scoped to a single team the user
	// belongs to, so a caller cannot reach an artifact in another of their teams via team_id.
	GetArtifactByProjectIDAndSlugInTeam(userID, teamID, projectID, slug string) (*models.Artifact, error)
	// GetArtifactByIDInTeam retrieves an artifact by id scoped to a single team the user
	// belongs to. Backs the attachment owner authorizer for owner_type="artifact".
	GetArtifactByIDInTeam(userID, teamID, artifactID string) (*models.Artifact, error)
	ListArtifacts(userID string, filters ArtifactFilters) (*models.ArtifactListResponse, error)
	ListArtifactsByProject(userID, projectID string, filters ArtifactFilters,
	) (*models.ArtifactListResponse, error)
	// ListArtifactsByProjectCrossTeam lists artifacts for a project across all teams the user owns.
	// No TeamID is required — uses user_id ownership (mirrors GetArtifactByProjectIDAndSlug semantics).
	ListArtifactsByProjectCrossTeam(userID, projectID string, filters ArtifactFilters,
	) (*models.ArtifactListResponse, error)
	UpdateArtifactByProjectIDAndSlug(userID, projectID, slug string, req *models.UpdateArtifactRequest,
	) (*models.Artifact, error)
	// UpdateArtifactByProjectIDAndSlugInTeam updates an artifact scoped to a single team the user
	// belongs to, enforcing the artifact lives in teamID before applying the update.
	UpdateArtifactByProjectIDAndSlugInTeam(userID, teamID, projectID, slug string,
		req *models.UpdateArtifactRequest) (*models.Artifact, error)
	DeleteArtifactByProjectIDAndSlug(userID, projectID, slug string) error
	GetArtifactStats(userID, teamID string) (*models.ArtifactStatsResponse, error)
	// ListArtifactVersionsInTeam returns the content-version history (newest-first) for a
	// team-scoped artifact identified by project and slug.
	ListArtifactVersionsInTeam(userID, teamID, projectID, slug string) ([]*models.ContentVersion, error)
	// GetArtifactVersionInTeam returns a single content version of a team-scoped artifact.
	GetArtifactVersionInTeam(
		userID, teamID, projectID, slug string, versionNumber int,
	) (*models.ContentVersion, error)
	// RestoreArtifactVersionInTeam restores a team-scoped artifact's content to the given
	// version, returning the updated artifact.
	RestoreArtifactVersionInTeam(
		userID, teamID, projectID, slug string, versionNumber int,
	) (*models.Artifact, error)
}

// AttachmentServiceInterface defines generic, polymorphic file-attachment
// operations keyed by (ownerType, ownerID). It validates file type and size
// limits, stores the binary in object storage, and persists metadata. It has no
// artifact-specific knowledge — owner type is a parameter — so future resources
// can adopt attachments without changing this interface.
type AttachmentServiceInterface interface {
	// Upload validates and stores a single file for the given owner, returning
	// the persisted attachment metadata.
	Upload(ctx context.Context, params UploadAttachmentParams) (*models.Attachment, error)
	// List returns all attachments for (ownerType, ownerID) plus their total size.
	List(ctx context.Context, ownerType, ownerID string) (*models.AttachmentListResponse, error)
	// Get returns a single attachment scoped to (ownerType, ownerID), or
	// repositories.ErrAttachmentNotFound.
	Get(ctx context.Context, ownerType, ownerID, attachmentID string) (*models.Attachment, error)
	// GetByIDInTeam returns a single attachment scoped to teamID only (its owner
	// is read from the stored row), for the universal attachments endpoint where
	// item operations are keyed by the attachment id. Returns
	// repositories.ErrAttachmentNotFound when no such row exists in the team.
	GetByIDInTeam(ctx context.Context, teamID, attachmentID string) (*models.Attachment, error)
	// Download opens the binary stream for an attachment. The caller must Close
	// the returned reader.
	Download(ctx context.Context, attachment *models.Attachment) (io.ReadCloser, error)
	// Delete removes a single attachment (object + metadata) scoped to
	// (ownerType, ownerID).
	Delete(ctx context.Context, ownerType, ownerID, attachmentID string) error
	// DeleteAllForOwner removes every attachment (objects + metadata) for an
	// owner. Mirrors EmbeddingService.DeleteEmbeddingsByEntity; called from the
	// owning resource's delete path.
	DeleteAllForOwner(ctx context.Context, ownerType, ownerID string) error
}

// TypeServiceInterface defines the resource-type-agnostic, team-customizable
// type taxonomy operations. System defaults are global and read-only; custom
// types belong to a team. It has no artifact-specific knowledge in its
// signatures — resourceType is a parameter — so future resources can adopt
// custom types without changing this interface.
type TypeServiceInterface interface {
	// List returns every type visible to the team for resourceType: the global
	// system defaults plus the team's custom types.
	List(ctx context.Context, teamID, resourceType string) ([]models.Type, error)
	// CreateCustom validates and creates a team-owned custom type. It returns a
	// validation sentinel (ErrType*) for bad input, or
	// repositories.ErrTypeAlreadyExists when the slug collides with a global
	// default or an existing team type.
	CreateCustom(ctx context.Context, params CreateTypeParams) (*models.Type, error)
	// Delete removes a team-owned custom type by id and atomically reassigns the
	// resources that referenced it to the resource's system default. It returns
	// repositories.ErrTypeNotFound when the id does not match a deletable
	// (non-system, team-owned) row.
	Delete(ctx context.Context, teamID, id string) error
	// ValidateType reports whether (resourceType, slug) is a type visible to the
	// team — a global default or one of the team's custom types.
	ValidateType(ctx context.Context, teamID, resourceType, slug string) (bool, error)
}

// CreateTypeParams carries the inputs for creating a team-owned custom type.
type CreateTypeParams struct {
	TeamID       string
	UserID       string
	ResourceType string
	Slug         string
	Name         string
}

// EmbeddingProviderServiceInterface defines the interface for embedding provider operations
type EmbeddingProviderServiceInterface interface {
	CreateEmbeddingProvider(ctx context.Context, teamID, userID string, req models.CreateEmbeddingProviderRequest,
	) (*models.EmbeddingProvider, error)
	GetEmbeddingProvidersByTeamID(ctx context.Context, teamID string,
	) ([]models.EmbeddingProviderResponse, error)
	GetEmbeddingProvider(ctx context.Context, teamID, providerID string,
	) (*models.EmbeddingProviderResponse, error)
	UpdateEmbeddingProvider(ctx context.Context, teamID, providerID string,
		req models.UpdateEmbeddingProviderRequest) (*models.EmbeddingProvider, error)
	DeleteEmbeddingProvider(ctx context.Context, teamID, providerID string) error
	GetDefaultEmbeddingProvider(ctx context.Context, teamID string) (*models.EmbeddingProvider, error)
	ValidateEmbeddingProvider(ctx context.Context, req models.ValidateEmbeddingProviderRequest,
	) (*models.ValidateEmbeddingProviderResponse, error)
	// ResolveActiveProvider resolves the single system-wide embedding provider used
	// by the embedding pipeline (document + query embedding). Returns (nil, nil)
	// when none is configured so embedding silently no-ops. See
	// ActiveEmbeddingProviderResolver.
	ResolveActiveProvider(ctx context.Context, model string, dimensions int) (EmbeddingProvider, error)
}

// EmailServiceInterface defines the interface for email operations
type EmailServiceInterface interface {
	SendSupportRequest(userName, userEmail string, req *models.SupportRequest) error
	SendTeamInvitation(invitation *models.TeamInvitation, teamName, inviterName string) error
	// SendNotificationEmail sends a transactional notification email to the given address
	SendNotificationEmail(to, subject, htmlBody string) error
}

// AgentServiceInterface defines the interface for agent operations
type AgentServiceInterface interface {
	CreateAgent(ctx context.Context, userID, teamID string, req *models.CreateAgentRequest) (*models.Agent, error)
	GetAgentByID(ctx context.Context, userID, teamID, agentID string) (*models.Agent, error)
	ListAgents(ctx context.Context, userID string, filters AgentFilters) (*models.AgentListResponse, error)
	UpdateAgent(ctx context.Context, userID, teamID, agentID string, req *models.UpdateAgentRequest,
	) (*models.Agent, error)
	UpdateAgentCredentials(ctx context.Context, userID, teamID, agentID string,
		req *models.UpdateAgentCredentialsRequest) error
	DeleteAgent(ctx context.Context, userID, teamID, agentID string) error
	GetAgentStats(ctx context.Context, userID, teamID string) (*models.AgentStatsResponse, error)
	StartExecution(ctx context.Context, userID, teamID, agentID string, req *models.CreateAgentExecutionRequest,
	) (*models.AgentExecution, error)
	CompleteExecution(ctx context.Context, userID, executionID string, req *models.UpdateAgentExecutionRequest,
	) (*models.AgentExecution, error)
	GetExecution(ctx context.Context, userID, executionID string) (*models.AgentExecution, error)
	ListExecutions(ctx context.Context, userID string, filters AgentExecutionFilters,
	) ([]models.AgentExecution, int, error)
}

// EncryptionServiceInterface defines the interface for encryption operations
type EncryptionServiceInterface interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// MemoryServiceInterface defines the interface for memory operations
type MemoryServiceInterface interface {
	CreateMemory(userID, teamID string, req *models.CreateMemoryRequest) (*models.Memory, error)
	GetMemory(userID, teamID, memoryID string) (*models.Memory, error)
	ListMemories(userID string, filters MemoryFilters) (*models.MemoryListResponse, error)
	UpdateMemory(userID, teamID, memoryID string, req *models.UpdateMemoryRequest) (*models.Memory, error)
	DeleteMemory(userID, teamID, memoryID string) error
	SearchMemoriesByMetadata(userID, metadataKey, metadataValue string, filters MemoryFilters,
	) (*models.MemoryListResponse, error)
	// ListMemoryVersions returns the content-version history (newest-first) for a memory
	// identified by id. The memory is loaded through the authorization-enforcing
	// team-scoped lookup before its versions are read.
	ListMemoryVersions(userID, teamID, memoryID string) ([]*models.ContentVersion, error)
	// GetMemoryVersion returns a single content version of a memory by version number.
	GetMemoryVersion(
		userID, teamID, memoryID string, versionNumber int,
	) (*models.ContentVersion, error)
	// RestoreMemoryVersion restores a memory's text to the given version, returning the
	// updated memory.
	RestoreMemoryVersion(
		userID, teamID, memoryID string, versionNumber int,
	) (*models.Memory, error)
}

// PromptGalleryServiceInterface defines the interface for prompt gallery operations
type PromptGalleryServiceInterface interface {
	GetCategories() ([]models.PromptGalleryCategory, error)
	ListPrompts(category, search string, tags []string, page, limit int) (*models.PromptGalleryListResponse, error)
	GetPromptByID(promptID string) (*models.PromptGalleryTemplate, error)
	TrackPromptUsage(userID string, req *models.PromptGalleryUsageRequest) error
}

// BackofficeServiceInterface defines the interface for back office operations
type BackofficeServiceInterface interface {
	GetUsageAndGrowth(ctx context.Context, fromDate, toDate *time.Time) (*models.UsageAndGrowthResponse, error)
}

// BlueprintServiceInterface defines the interface for blueprint operations
type BlueprintServiceInterface interface {
	CreateBlueprint(userID, teamID string, req *models.CreateBlueprintRequest) (*models.Blueprint, error)
	GetBlueprintByProjectIDAndSlug(userID, projectID, slug string) (*models.Blueprint, error)
	GetBlueprintByIDInTeam(userID, teamID, blueprintID string) (*models.Blueprint, error)
	ListBlueprints(userID string, filters BlueprintFilters) (*models.BlueprintListResponse, error)
	ListBlueprintsByProject(userID, projectID string, filters BlueprintFilters,
	) (*models.BlueprintListResponse, error)
	UpdateBlueprintByProjectIDAndSlug(userID, projectID, slug string, req *models.UpdateBlueprintRequest,
	) (*models.Blueprint, error)
	DeleteBlueprintByProjectIDAndSlug(userID, projectID, slug string) error
	GetBlueprintStats(userID string) (*models.BlueprintStatsResponse, error)
	// ListBlueprintVersions returns the content-version history (newest-first) for a
	// blueprint identified by project and slug. The blueprint is loaded through the
	// authorization-enforcing cross-team lookup before its versions are read.
	ListBlueprintVersions(userID, projectID, slug string) ([]*models.ContentVersion, error)
	// GetBlueprintVersion returns a single content version of a blueprint by version number.
	GetBlueprintVersion(
		userID, projectID, slug string, versionNumber int,
	) (*models.ContentVersion, error)
	// RestoreBlueprintVersion restores a blueprint's content to the given version,
	// returning the updated blueprint.
	RestoreBlueprintVersion(
		userID, projectID, slug string, versionNumber int,
	) (*models.Blueprint, error)
}

// UserPreferencesServiceInterface defines the interface for user preferences operations
type UserPreferencesServiceInterface interface {
	GetPreferences(ctx context.Context, userID string) (*models.PreferencesResponse, error)
	UpdatePreferences(
		ctx context.Context,
		userID string,
		req models.UpdatePreferencesRequest,
	) (*models.PreferencesResponse, error)
}

// TeamServiceInterface defines the interface for team operations
type TeamServiceInterface interface {
	CreateDefaultTeam(ctx context.Context, userID string) (*models.Team, error)
	GetTeamByOwnerID(ctx context.Context, ownerID string) (*models.Team, error)
	CreateTeam(ctx context.Context, userID string, req *models.CreateTeamRequest) (*models.Team, error)
	GetTeam(ctx context.Context, userID, teamID string) (*models.Team, error)
	UpdateTeam(ctx context.Context, userID, teamID string, req *models.UpdateTeamRequest) (*models.Team, error)
	DeleteTeam(ctx context.Context, userID, teamID string) error
	ListTeams(ctx context.Context, userID string, page, pageSize int) (*models.TeamListResponse, error)
	IsUserMemberOfTeam(ctx context.Context, userID, teamID string) (bool, error)
	GetTeamMembers(ctx context.Context, userID, teamID string, page, pageSize int) (*models.TeamMembersListResponse, error)
	RemoveTeamMember(ctx context.Context, userID, teamID, memberUserID string) error
	// GetTeamStats returns team-wide resource counts (projects, prompts, artifacts,
	// blueprints, memories, feed_items). Team membership is validated by the caller.
	GetTeamStats(ctx context.Context, teamID string) (*models.TeamStatsResponse, error)
	// GetTeamResourceCreationMetrics returns sparse per-day creation counts per
	// resource type (prompts, artifacts, blueprints, memories, projects) for the
	// team, counting rows created at or after `since`. The handler zero-fills it.
	GetTeamResourceCreationMetrics(
		ctx context.Context, teamID string, since time.Time,
	) ([]models.TeamResourceCreationCount, error)
	// GetTeamFeedCreationMetrics returns sparse per-day creation counts for feeds
	// and feed_items belonging to the team, counting rows created at or after
	// `since`. The handler zero-fills it into a continuous daily series.
	GetTeamFeedCreationMetrics(
		ctx context.Context, teamID string, since time.Time,
	) ([]models.TeamFeedCreationCount, error)
}

// ProjectServiceInterface defines the interface for project operations
type ProjectServiceInterface interface {
	CreateProject(userID, teamID string, req *models.CreateProjectRequest) (*models.Project, error)
	GetProjectBySlug(teamID, userID, slug string) (*models.Project, error)
	ListProjects(userID string, filters ProjectFilters) (*models.ProjectListResponse, error)
	UpdateProject(teamID, userID, slug string, req *models.UpdateProjectRequest) (*models.Project, error)
	DeleteProject(teamID, userID, slug string) error
	// GetProjectStats returns resource counts for the project identified by teamID + slug.
	GetProjectStats(teamID, userID, slug string) (*models.ProjectStatsResponse, error)
	// GetProjectResourceCreationMetrics returns sparse per-day creation counts per
	// resource type (prompts, artifacts, blueprints, memories) for the project,
	// counting rows created at or after `since`. The handler zero-fills the result.
	GetProjectResourceCreationMetrics(
		teamID, userID, slug string, since time.Time,
	) ([]models.ProjectResourceCreationCount, error)
}

// ProjectFilters represents filters for project queries
type ProjectFilters struct {
	Search    string
	SortBy    string
	SortOrder string
	TeamID    string
	Page      int
	Limit     int
}

// FeedServiceInterface defines the interface for feed operations
type FeedServiceInterface interface {
	CreateFeed(ctx context.Context, userID, teamID string, req *models.CreateFeedRequest) (*models.Feed, error)
	GetFeed(ctx context.Context, userID, teamID, feedID string) (*models.Feed, error)
	ListFeeds(ctx context.Context, userID string, filters FeedFilters) (*models.FeedListResponse, error)
	// ListFeedsForMCP returns feeds enriched with last_post_at for the MCP list-feeds tool.
	// It does NOT alter the REST API response shape.
	ListFeedsForMCP(ctx context.Context, userID string, filters FeedFilters) (*models.MCPFeedListResponse, error)
	UpdateFeed(ctx context.Context, userID, teamID, feedID string, req *models.UpdateFeedRequest) (*models.Feed, error)
	DeleteFeed(ctx context.Context, userID, teamID, feedID string) error
}

// FeedItemServiceInterface defines the interface for feed item operations
type FeedItemServiceInterface interface {
	CreateFeedItem(
		ctx context.Context, userID, teamID, feedID string, req *models.CreateFeedItemRequest,
	) (*models.FeedItem, error)
	GetFeedItem(ctx context.Context, userID, teamID, itemID string) (*models.FeedItem, error)
	ListFeedItems(ctx context.Context, userID string, filters FeedItemFilters) (*models.FeedItemListResponse, error)
	ArchiveFeedItem(ctx context.Context, userID, teamID, itemID string) error
	UnarchiveFeedItem(ctx context.Context, userID, teamID, itemID string) error
	DeleteFeedItem(ctx context.Context, userID, teamID, itemID string) error
	// EnrichWithReplyCounts annotates each FeedItem in the slice with its reply count.
	// A single bulk COUNT query is used to avoid N+1 queries.
	EnrichWithReplyCounts(ctx context.Context, teamID string, items []models.FeedItem) ([]models.FeedItem, error)
}

// FeedItemReplyServiceInterface defines the interface for feed item reply operations
type FeedItemReplyServiceInterface interface {
	CreateReply(
		ctx context.Context, userID, teamID, feedItemID string, req *models.CreateFeedItemReplyRequest,
	) (*models.FeedItemReply, error)
	GetReply(ctx context.Context, userID, teamID, replyID string) (*models.FeedItemReply, error)
	ListReplies(
		ctx context.Context, userID, teamID, feedItemID string, page, limit int,
	) (*models.FeedItemReplyListResponse, error)
	// ListReplyPosters returns the (reply_id, posted_by_user_id) pairs for every reply on
	// feedItemID, after verifying team membership. Used by the delete handler to clean up
	// each reply's embedding row (keyed by its poster) before a feed item is hard-deleted.
	ListReplyPosters(
		ctx context.Context, userID, teamID, feedItemID string,
	) ([]repositories.FeedItemReplyPoster, error)
}

// FeedFilters represents filters for feed service queries
type FeedFilters struct {
	TeamID string
	Search string
	Page   int
	Limit  int
}

// FeedItemFilters represents filters for feed item service queries
type FeedItemFilters struct {
	TeamID          string
	FeedID          *string
	ProjectID       *string
	AIAssistantName *string
	Archived        *bool
	Page            int
	Limit           int
}

// ProjectMigrationServiceInterface defines the interface for project resource migration operations.
type ProjectMigrationServiceInterface interface {
	// GetInventory returns the count and list of all resources in the given project.
	GetInventory(ctx context.Context, userID, teamID, projectID string) (*projectmigration.MigrationInventory, error)
	// Migrate moves the selected resources from sourceProjectID to req.DestinationProjectID
	// within the same team. The operation is always a MOVE (not a copy).
	Migrate(
		ctx context.Context,
		userID, teamID, sourceProjectID string,
		req *projectmigration.MigrationRequest,
	) (*projectmigration.MigrationResult, error)
}

// SearchServiceInterface defines the interface for team-scoped semantic search operations.
type SearchServiceInterface interface {
	// Search embeds the query, runs a semantic search scoped to teamID over the
	// requested resource types (or all four when empty), and returns a paginated,
	// relevance-ranked response.
	Search(ctx context.Context, teamID string, req *models.SearchRequest) (*models.SearchResultsResponse, error)
}

// GitHubAppServiceInterface defines the interface for GitHub App operations
type GitHubAppServiceInterface interface {
	GetInstallationStatus(ctx context.Context, teamID string) (*models.GitHubInstallationStatus, error)
	// HandleInstallationCallback processes the GitHub App installation callback.
	// Returns (reconnected, error) where reconnected=true indicates the same team reconnected an existing installation.
	HandleInstallationCallback(ctx context.Context, userID, teamID string, installationID int64) (bool, error)
	GetRepositories(ctx context.Context, teamID, userID string, page int) (*models.GitHubRepositoriesResponse, error)
	GetAccessibleRepoURLs(ctx context.Context, teamID string) (map[string]bool, error)
	DisconnectInstallation(ctx context.Context, userID, teamID string) error
	RefreshInstallationToken(ctx context.Context, teamID string) error
	HandleWebhookEvent(ctx context.Context, eventType string, installationID int64, action string) error
	ImportProjectFromRepository(ctx context.Context, userID, teamID string, repoID int64) (*models.Project, bool, error)
	ImportBlueprintsFromRepository(
		ctx context.Context, userID, teamID string, repoID int64,
	) (*models.BlueprintImportReport, error)
}
