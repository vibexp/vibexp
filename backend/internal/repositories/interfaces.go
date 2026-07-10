// Package repositories defines the data-access contracts for the backend.
//
// Not-found contract: repositories signal "no row exists" via the exported
// Err*NotFound sentinel errors below (possibly wrapped with %w). Callers must
// detect the condition with errors.Is(err, repositories.ErrXNotFound) — never
// by matching on the error text. The handful of lookup methods that instead
// return an empty result for a missing row (e.g. (nil, nil)) document that
// behavior explicitly on the method's godoc.
package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/vibexp/vibexp/internal/models"
)

// Sentinel errors for repository operations
var (
	// ErrGitHubInstallationNotFound is returned when a GitHub installation is not found
	ErrGitHubInstallationNotFound = errors.New("GitHub installation not found")

	// ErrGitHubRepositoryNotFound is returned when a GitHub repository is not found or not accessible
	ErrGitHubRepositoryNotFound = errors.New("GitHub repository not found or not accessible")

	// ErrProjectNotFoundForRepo is returned when no project exists for a given repository
	ErrProjectNotFoundForRepo = errors.New("project not found for repository")

	// ErrDeviceTokenConflict is returned by DeviceTokenRepository.Upsert when the given
	// token is already registered to a different user. Callers should surface this as a
	// 409 Conflict so the originating user knows the token cannot be claimed.
	ErrDeviceTokenConflict = errors.New("device token already registered to another account")

	// ErrUserNotFound is returned by UserRepository.GetByID (and similar lookups) when no
	// row exists for the given identifier. Callers can distinguish a genuine missing user
	// from a transient DB error by checking errors.Is(err, repositories.ErrUserNotFound).
	ErrUserNotFound = errors.New("user not found")

	// ErrProjectSlugExists is returned by ProjectRepository.Create when a project
	// with the same slug already exists in the team (Postgres unique violation 23505).
	// Callers can detect collisions with errors.Is(err, ErrProjectSlugExists).
	ErrProjectSlugExists = errors.New("project slug already exists")

	// ErrProjectGitURLExists is returned by ProjectRepository.Create/Update when a project
	// with the same git_url already exists in the team (Postgres unique violation 23505 on
	// idx_projects_team_id_git_url_unique). Callers detect it with errors.Is(err, ErrProjectGitURLExists).
	ErrProjectGitURLExists = errors.New("project git_url already exists")

	// ErrFeedItemNotFound is returned by FeedItemRepository.Delete when the feed
	// item does not exist in the specified team. Callers detect it with errors.Is.
	ErrFeedItemNotFound = errors.New("feed item not found")

	// ErrFeedItemForbidden is returned by FeedItemRepository.Delete when the feed
	// item exists but the caller is neither its poster nor a team owner/admin.
	// Callers detect it with errors.Is and map to 403 Forbidden.
	ErrFeedItemForbidden = errors.New("feed item delete forbidden")

	// ErrTeamSubscriptionNotFound is returned by TeamSubscriptionRepository.Delete (and
	// future similar lookups) when no row matches the requested identifier. The terminal-dead
	// replacement path in TeamSubscriptionService.Create relies on errors.Is to treat a
	// concurrent delete as a benign no-op rather than a hard failure.
	ErrTeamSubscriptionNotFound = errors.New("team subscription not found")

	// ErrTeamInvitationNotFound is returned by TeamInvitationRepository.GetByToken
	// (and future similar lookups) when no row matches the requested token/id.
	// Callers detect it with errors.Is and map to 404 / a typed service error.
	ErrTeamInvitationNotFound = errors.New("team invitation not found")

	// ErrActivityNotFound is returned by ActivityRepository lookups/deletes when no
	// activity row matches the given identifier.
	ErrActivityNotFound = errors.New("activity not found")

	// ErrAgentNotFound is returned by AgentRepository lookups/updates/deletes when no
	// agent row matches the given identifier for the user/team.
	ErrAgentNotFound = errors.New("agent not found")

	// ErrAgentNameConflict is returned by AgentRepository.Create/Update when the agent
	// name would collide with an existing agent for the same user. Callers detect it
	// with errors.Is; the message keeps the legacy "already exists for this user"
	// substring so any remaining strings.Contains check still matches through wrapping.
	ErrAgentNameConflict = errors.New("agent with name already exists for this user")

	// ErrAgentExecutionNotFound is returned by AgentExecutionRepository lookups/updates
	// when no execution row matches the given identifier for the user.
	ErrAgentExecutionNotFound = errors.New("agent execution not found")

	// ErrAgentExecutionEventNotFound is returned by AgentExecutionEventRepository.GetByID
	// when no event row matches the given identifier.
	ErrAgentExecutionEventNotFound = errors.New("event not found")

	// ErrConversationNotFound is returned by
	// AgentExecutionRepository.GetFirstExecutionInConversation when the user has no
	// execution in the given conversation.
	ErrConversationNotFound = errors.New("conversation not found")

	// ErrAPIKeyNotFound is returned by APIKeyRepository lookups/deletes when no API key
	// row matches the given hash/identifier.
	ErrAPIKeyNotFound = errors.New("API key not found")

	// ErrArtifactNotFound is returned by ArtifactRepository lookups/deletes when no
	// artifact row matches the given identifier for the user/team.
	ErrArtifactNotFound = errors.New("artifact not found")

	// ErrAttachmentNotFound is returned by AttachmentRepository lookups/deletes
	// when no attachment row matches the given identifier for the owner.
	ErrAttachmentNotFound = errors.New("attachment not found")

	// ErrBlueprintNotFound is returned by BlueprintRepository lookups/deletes when no
	// blueprint row matches the given identifier for the user/team.
	ErrBlueprintNotFound = errors.New("blueprint not found")

	// ErrTypeNotFound is returned by TypeRepository lookups/deletes when no type
	// row matches the given identifier, or when a delete targets a row the caller
	// does not own (a system default or another team's type).
	ErrTypeNotFound = errors.New("type not found")

	// ErrTypeAlreadyExists is returned by TypeRepository.Create when a type with
	// the same (team_id, resource_type, slug) — or a colliding global default
	// slug — already exists.
	ErrTypeAlreadyExists = errors.New("type already exists")

	// ErrEmbeddingProviderNotFound is returned by EmbeddingProviderRepository
	// lookups/updates/deletes when no provider row matches the given identifier.
	ErrEmbeddingProviderNotFound = errors.New("embedding provider not found")

	// ErrDefaultEmbeddingProviderNotFound is returned by
	// EmbeddingProviderRepository.GetDefault when the user has no default provider.
	ErrDefaultEmbeddingProviderNotFound = errors.New("no default embedding provider found")

	// ErrNoActiveEmbeddingProvider is returned by
	// EmbeddingProviderRepository.GetActiveProvider when no embedding provider is
	// configured at all. Callers treat it as "embedding disabled" rather than a
	// failure: entity writes still succeed and no embedding is generated.
	ErrNoActiveEmbeddingProvider = errors.New("no active embedding provider configured")

	// ErrFeedNotFound is returned by FeedRepository lookups/updates/deletes when no
	// feed row matches the given identifier for the team.
	ErrFeedNotFound = errors.New("feed not found")

	// ErrFeedItemReplyNotFound is returned by FeedItemReplyRepository lookups when no
	// reply row matches the given identifier for the team.
	ErrFeedItemReplyNotFound = errors.New("feed item reply not found")

	// ErrHookSessionNotFound is returned by the Claude Code / Cursor IDE hook session
	// repositories when no session row matches the given identifier for the user. The
	// text deliberately does not reveal whether the session exists for another user.
	ErrHookSessionNotFound = errors.New("session not found or access denied")

	// ErrMemoryNotFound is returned by MemoryRepository lookups/updates/deletes when no
	// memory row matches the given identifier for the user/team.
	ErrMemoryNotFound = errors.New("memory not found")

	// ErrPromptNotFound is returned by PromptRepository (and the prompt gallery
	// repository) lookups/deletes when no prompt row matches the given identifier.
	ErrPromptNotFound = errors.New("prompt not found")

	// ErrPromptShareNotFound is returned by PromptShareRepository lookups/deletes when
	// no share row matches the given token/prompt.
	ErrPromptShareNotFound = errors.New("share not found")

	// ErrTeamNotFound is returned by TeamRepository lookups/updates/deletes when no
	// team row matches the given identifier. Distinct from services.ErrTeamNotFound,
	// which is the service-layer authorization-aware variant.
	ErrTeamNotFound = errors.New("team not found")

	// ErrTeamMemberNotFound is returned by TeamMemberRepository lookups/updates/deletes
	// when the user is not a member of the given team.
	ErrTeamMemberNotFound = errors.New("team member not found")

	// ErrWebhookEventNotFound is returned by WebhookEventRepository.GetByID when no
	// webhook event row matches the given identifier.
	ErrWebhookEventNotFound = errors.New("webhook event not found")

	// ErrContentVersionNotFound is returned by ContentVersionRepository.GetByVersionNumber
	// when no version row matches the given (resource_type, resource_id, version_number).
	ErrContentVersionNotFound = errors.New("content version not found")

	// ErrOAuthClientNotFound is returned by OAuthClientRepository.GetByID when no
	// OAuth client row matches the given client_id.
	ErrOAuthClientNotFound = errors.New("oauth client not found")

	// ErrOAuthRequestNotFound is returned by OAuthRequestRepository.Get when no
	// row matches the given token/code signature.
	ErrOAuthRequestNotFound = errors.New("oauth request not found")

	// ErrOAuthSigningKeyNotFound is returned by OAuthSigningKeyRepository lookups
	// when no signing key matches (e.g. there is no active key yet).
	ErrOAuthSigningKeyNotFound = errors.New("oauth signing key not found")

	// ErrOAuthLoginSessionNotFound is returned by OAuthLoginSessionRepository.Get
	// when no (or an expired) login session matches the given id.
	ErrOAuthLoginSessionNotFound = errors.New("oauth login session not found")
)

// OAuthClientRepository persists dynamically-registered OAuth 2.1 clients
// (RFC 7591) for the embedded Authorization Server (issue #31).
type OAuthClientRepository interface {
	Create(ctx context.Context, client *models.OAuthClient) error
	// GetByID returns the client or ErrOAuthClientNotFound.
	GetByID(ctx context.Context, clientID string) (*models.OAuthClient, error)
}

// OAuthRequestRepository persists fosite request sessions (authorization codes,
// access tokens, refresh tokens, or PKCE sessions). One implementation is bound
// to each backing table. Get returns the row even when inactive so the caller
// can distinguish invalidated/rotated tokens from missing ones; a missing row
// yields ErrOAuthRequestNotFound.
type OAuthRequestRepository interface {
	Create(ctx context.Context, req *models.OAuthRequest) error
	Get(ctx context.Context, signature string) (*models.OAuthRequest, error)
	Delete(ctx context.Context, signature string) error
	// Deactivate marks a single row inactive (authorization-code invalidation and
	// refresh-token rotation). Missing rows are a no-op.
	Deactivate(ctx context.Context, signature string) error
	// DeactivateByRequestID marks every row sharing a request id inactive
	// (refresh-token family revocation). Missing rows are a no-op.
	DeactivateByRequestID(ctx context.Context, requestID string) error
	// DeleteByRequestID removes every row sharing a request id (access-token
	// revocation). Missing rows are a no-op.
	DeleteByRequestID(ctx context.Context, requestID string) error
	// DeleteExpired purges rows whose expires_at is in the past (rows with a NULL
	// expires_at are left untouched); returns the number of rows removed.
	DeleteExpired(ctx context.Context) (int64, error)
}

// OAuthSigningKeyRepository persists the DB-backed JWT signing keys served via
// the JWKS endpoint, with at most one active key at a time.
type OAuthSigningKeyRepository interface {
	Create(ctx context.Context, key *models.OAuthSigningKey) error
	// GetActive returns the single active key or ErrOAuthSigningKeyNotFound.
	GetActive(ctx context.Context) (*models.OAuthSigningKey, error)
	// ListAll returns every key (active and retired) for building the JWKS.
	ListAll(ctx context.Context) ([]*models.OAuthSigningKey, error)
	// Activate atomically clears the active flag on all keys and sets it on kid,
	// stamping the previously-active key's rotated_at.
	Activate(ctx context.Context, kid string) error
	// DeleteRetiredBefore removes retired (inactive) keys whose rotated_at is at or
	// before cutoff. The active key is never removed. Callers pass a cutoff no
	// later than now minus the refresh-token TTL so no live token can still
	// reference a pruned key. Returns the number of keys removed.
	DeleteRetiredBefore(ctx context.Context, cutoff time.Time) (int64, error)
	// TryAdvisoryLock attempts a non-blocking, session-scoped Postgres advisory
	// lock that serializes signing-key rotation across instances. When acquired is
	// true the caller holds the lock and MUST call release exactly once to free it;
	// when false another instance holds it and release is a no-op.
	TryAdvisoryLock(ctx context.Context) (acquired bool, release func() error, err error)
}

// OAuthLoginSessionRepository persists the short-lived federated-login stash.
type OAuthLoginSessionRepository interface {
	Create(ctx context.Context, session *models.OAuthLoginSession) error
	// Get returns a non-expired session or ErrOAuthLoginSessionNotFound.
	Get(ctx context.Context, id string) (*models.OAuthLoginSession, error)
	// AttachUser records the resolved user id after the IdP callback succeeds.
	AttachUser(ctx context.Context, id, userID string) error
	Delete(ctx context.Context, id string) error
	// DeleteExpired purges sessions past their expiry; returns rows removed.
	DeleteExpired(ctx context.Context) (int64, error)
}

// UserRepository defines the interface for user data access operations
type UserRepository interface {
	GetByID(ctx context.Context, userID string) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByGoogleID(ctx context.Context, googleID string) (*models.User, error)
	// GetByIDPSubject looks up a user by the (idp_provider, idp_subject) tuple
	// populated by the provider-agnostic auth flow.
	GetByIDPSubject(ctx context.Context, provider, subject string) (*models.User, error)
	GetByStripeCustomerID(ctx context.Context, stripeCustomerID string) (*models.User, error)
	Create(ctx context.Context, user *models.User) error
	Update(ctx context.Context, user *models.User) error
	UpdateSubscriptionStatus(ctx context.Context, userID, status string, plan *string) error
	UpdateSubscriptionStatusWithTrial(ctx context.Context, userID, status string, plan *string, trialEnd *time.Time) error
	UpdateSubscriptionWithCancellation(
		ctx context.Context, userID, status string, plan *string, trialEnd *time.Time, canceledAt *time.Time,
	) error
	UpdateStripeCustomerID(ctx context.Context, userID, customerID string) error
	UpdateTrialEndsAt(ctx context.Context, userID string, trialEndsAt *time.Time) error
	UpdateDefaultTeamID(ctx context.Context, userID, teamID string) error
	MarkOnboardingCompleted(ctx context.Context, userID string) error
	// GetNamesByIDs returns a map of userID → display name (or email when name is blank)
	// for the given set of IDs. Unknown IDs are silently omitted from the result.
	GetNamesByIDs(ctx context.Context, ids []string) (map[string]string, error)
}

// TeamRepository defines the interface for team data access operations
type TeamRepository interface {
	Create(ctx context.Context, team *models.Team) error
	GetByID(ctx context.Context, teamID string) (*models.Team, error)
	GetByOwnerID(ctx context.Context, ownerID string) (*models.Team, error)
	GetByOwnerAndSlug(ctx context.Context, ownerID, slug string) (*models.Team, error)
	Update(ctx context.Context, team *models.Team) error
	Delete(ctx context.Context, ownerID, teamID string) error
	ListByOwnerID(ctx context.Context, ownerID string, limit, offset int) ([]models.Team, int, error)
	ListByUserID(ctx context.Context, userID string, limit, offset int) ([]models.Team, int, error)
	CountByOwnerID(ctx context.Context, ownerID string) (int, error)
	// GetTeamStats returns team-wide resource counts (projects, prompts, artifacts,
	// blueprints, memories, feed_items) for the team. Authorization is the caller's
	// responsibility; this aggregates purely by team_id.
	GetTeamStats(ctx context.Context, teamID string) (*models.TeamStatsResponse, error)
	// GetTeamResourceCreationMetrics returns sparse per-day creation counts per
	// resource type (prompts, artifacts, blueprints, memories, projects) for the
	// team, counting rows created at or after `since`. Days with no creations are
	// omitted (the caller zero-fills).
	GetTeamResourceCreationMetrics(
		ctx context.Context, teamID string, since time.Time,
	) ([]models.TeamResourceCreationCount, error)
	// GetTeamFeedCreationMetrics returns sparse per-day creation counts for feeds
	// (channels, by created_at) and feed_items (AI updates, by posted_at) for the
	// team, counting rows created at or after `since`. Days with no creations are
	// omitted (the caller zero-fills).
	GetTeamFeedCreationMetrics(
		ctx context.Context, teamID string, since time.Time,
	) ([]models.TeamFeedCreationCount, error)
}

// TeamMemberRepository defines the interface for team member data access operations
type TeamMemberRepository interface {
	Create(ctx context.Context, member *models.TeamMember) error
	GetByTeamAndUser(ctx context.Context, teamID, userID string) (*models.TeamMember, error)
	GetByTeamID(ctx context.Context, teamID string) ([]models.TeamMember, error)
	GetByUserID(ctx context.Context, userID string) ([]models.TeamMember, error)
	UpdateRole(ctx context.Context, teamID, userID string, role models.TeamMemberRole) error
	Delete(ctx context.Context, teamID, userID string) error
}

// TeamInvitationRepository defines the interface for team invitation data access operations
type TeamInvitationRepository interface {
	Create(ctx context.Context, invitation *models.TeamInvitation) error
	GetByID(ctx context.Context, invitationID string) (*models.TeamInvitation, error)
	GetByToken(ctx context.Context, token string) (*models.TeamInvitation, error)
	GetByTeamID(ctx context.Context, teamID string) ([]models.TeamInvitation, error)
	GetPendingByEmail(ctx context.Context, email string) ([]models.TeamInvitation, error)
	UpdateStatus(ctx context.Context, invitationID string, status models.InvitationStatus) error
	Delete(ctx context.Context, invitationID string) error
}

// TeamSubscriptionRepository defines the interface for team subscription data access operations
type TeamSubscriptionRepository interface {
	// Core CRUD operations
	Create(ctx context.Context, subscription *models.TeamSubscription) error
	GetByID(ctx context.Context, id string) (*models.TeamSubscription, error)
	GetByTeamID(ctx context.Context, teamID string) (*models.TeamSubscription, error)
	GetByStripeSubscriptionID(ctx context.Context, stripeSubID string) (*models.TeamSubscription, error)
	Update(ctx context.Context, subscription *models.TeamSubscription) error
	Delete(ctx context.Context, id string) error

	// Webhook-optimized methods (avoid fetch+update round trips)
	UpdateStatus(ctx context.Context, stripeSubID, status string) error
	UpdateSeatCount(ctx context.Context, stripeSubID string, seatCount int) error

	// Deletion validation methods.
	// Both return (nil, nil) — not an error — when no matching subscription exists.
	GetActiveByTeamID(ctx context.Context, teamID string) (*models.TeamSubscription, error)
	GetCanceledByTeamID(ctx context.Context, teamID string) (*models.TeamSubscription, error)

	// Listing methods
	ListByStatus(ctx context.Context, status string, limit, offset int) ([]*models.TeamSubscription, int, error)
	ListByTier(ctx context.Context, tier string, limit, offset int) ([]*models.TeamSubscription, int, error)
}

// APIKeyRepository defines the interface for API key data access operations
type APIKeyRepository interface {
	Create(ctx context.Context, apiKey *models.APIKey) error
	GetByUserID(ctx context.Context, userID string) ([]models.APIKey, error)
	GetByKeyHash(ctx context.Context, keyHash string) (*models.APIKey, error)
	Delete(ctx context.Context, userID, keyID string) error
	UpdateLastUsed(ctx context.Context, keyID string, lastUsedAt time.Time) error
	// New integration-related methods
	GetIntegrationsByAPIKeyID(ctx context.Context, apiKeyID string) ([]string, error)
	HasIntegrationPermission(ctx context.Context, apiKeyID, integrationCode string) (bool, error)
	GetValidIntegrationCodes(ctx context.Context) ([]string, error)
	// GetNamesByIDs returns a map of apiKeyID → name for the given IDs owned by userID.
	// Unknown or inaccessible IDs are omitted from the result.
	GetNamesByIDs(ctx context.Context, userID string, ids []string) (map[string]string, error)
}

// PromptRepository defines the interface for prompt data access operations
type PromptRepository interface {
	Create(ctx context.Context, prompt *models.Prompt) error
	GetByID(ctx context.Context, userID, teamID, promptID string) (*models.Prompt, error)
	GetBySlug(ctx context.Context, userID, teamID, slug string) (*models.Prompt, error)
	// GetByIDCrossTeam searches for a prompt across all user's teams
	GetByIDCrossTeam(ctx context.Context, userID, promptID string) (*models.Prompt, error)
	// GetBySlugCrossTeam searches for a prompt across all user's teams
	GetBySlugCrossTeam(ctx context.Context, userID, slug string) (*models.Prompt, error)
	List(ctx context.Context, userID string, filters PromptFilters) ([]models.Prompt, int, error)
	Update(ctx context.Context, prompt *models.Prompt) error
	Delete(ctx context.Context, userID, teamID, promptID string) error
	CountByStatus(ctx context.Context, userID, status string) (int, error)
	GetUserLabels(ctx context.Context, userID string) ([]string, error)
	// GetNamesByIDsCrossTeam returns a map of promptID → name for the given IDs owned by userID,
	// searching across all the user's teams. Unknown or inaccessible IDs are omitted.
	GetNamesByIDsCrossTeam(ctx context.Context, userID string, ids []string) (map[string]string, error)
}

// PromptFilters represents filters for prompt queries
type PromptFilters struct {
	Status    string
	Search    string
	TeamID    string
	MCPExpose *bool
	IsShared  *bool
	Labels    []string
	ProjectID *string
	SortBy    string
	SortOrder string
	Page      int
	Limit     int
}

// PromptReferenceRepository defines the interface for prompt reference data access operations
type PromptReferenceRepository interface {
	// CreateBatch creates multiple prompt references
	CreateBatch(ctx context.Context, references []models.PromptReference) error
	// DeleteByPromptID deletes all references for a prompt
	DeleteByPromptID(ctx context.Context, promptID string) error
	// GetPromptsUsingPrompt returns prompts that reference the given prompt (used by)
	GetPromptsUsingPrompt(ctx context.Context, userID, promptID string) ([]models.PromptDependencyInfo, error)
	// GetPromptsUsedByPrompt returns prompts that are referenced by the given prompt (uses)
	GetPromptsUsedByPrompt(ctx context.Context, userID, promptID string) ([]models.PromptDependencyInfo, error)
	// HasDependents checks if a prompt is referenced by any other prompts
	HasDependents(ctx context.Context, promptID string) (bool, error)
}

// PromptShareRepository defines the interface for prompt share data access operations
type PromptShareRepository interface {
	Create(ctx context.Context, share *models.PromptShare) error
	GetByToken(ctx context.Context, token string) (*models.PromptShare, error)
	GetByPromptID(ctx context.Context, promptID string) (*models.PromptShare, error)
	Update(ctx context.Context, share *models.PromptShare) error
	Delete(ctx context.Context, shareID string) error
	IncrementAccessCount(ctx context.Context, shareID string) error
	AddAccessEmails(ctx context.Context, shareID string, emails []string) error
	RemoveAccessEmail(ctx context.Context, shareID, email string) error
	GetAccessEmails(ctx context.Context, shareID string) ([]string, error)
	HasAccess(ctx context.Context, shareID, email string) (bool, error)
}

// ProjectFilters represents filters for project queries
type ProjectFilters struct {
	Search string
	Page   int
	Limit  int
}

// ProjectInfo represents project information with context count
type ProjectInfo struct {
	ProjectID    string `json:"project_id"`
	ProjectName  string `json:"project_name"`
	ContextCount int    `json:"context_count"`
}

// ArtifactRepository defines the interface for artifact data access operations
type ArtifactRepository interface {
	Create(ctx context.Context, artifact *models.Artifact) error
	GetByID(ctx context.Context, userID, teamID, artifactID string) (*models.Artifact, error)
	GetByProjectIDAndSlug(ctx context.Context, userID, teamID, projectID, slug string) (*models.Artifact, error)
	// GetByIDCrossTeam searches for an artifact across all user's teams
	GetByIDCrossTeam(ctx context.Context, userID, artifactID string) (*models.Artifact, error)
	// GetByProjectIDAndSlugCrossTeam searches for an artifact across all user's teams
	GetByProjectIDAndSlugCrossTeam(ctx context.Context, userID, projectID, slug string) (*models.Artifact, error)
	// ListCrossTeam lists artifacts across all user's teams (no team_id filter, uses user_id ownership)
	ListCrossTeam(ctx context.Context, userID string, filters ArtifactFilters) ([]models.Artifact, int, error)
	List(ctx context.Context, userID string, filters ArtifactFilters) ([]models.Artifact, int, error)
	Update(ctx context.Context, artifact *models.Artifact) error
	Delete(ctx context.Context, userID, teamID, artifactID string) error
	GetStats(ctx context.Context, userID, teamID string) (*models.ArtifactStatsResponse, error)
	CountAll(ctx context.Context, userID string) (int, error)
	// GetNamesByIDsCrossTeam returns a map of artifactID → title for the given IDs owned by userID,
	// searching across all the user's teams. Unknown or inaccessible IDs are omitted.
	GetNamesByIDsCrossTeam(ctx context.Context, userID string, ids []string) (map[string]string, error)
}

// ArtifactFilters represents filters for artifact queries
type ArtifactFilters struct {
	ProjectID *string
	Status    *string
	Type      *string
	TeamID    string
	Search    string
	SortBy    string
	SortOrder string
	Metadata  map[string]string
	Page      int
	Limit     int
}

// EmbeddingProviderRepository defines the interface for embedding provider data access operations
type EmbeddingProviderRepository interface {
	Create(ctx context.Context, provider *models.EmbeddingProvider) error
	GetByID(ctx context.Context, teamID, providerID string) (*models.EmbeddingProvider, error)
	List(ctx context.Context, teamID string, filters EmbeddingProviderFilters) ([]models.EmbeddingProvider, int, error)
	Update(ctx context.Context, provider *models.EmbeddingProvider) error
	Delete(ctx context.Context, teamID, providerID string) error
	GetDefault(ctx context.Context, teamID string) (*models.EmbeddingProvider, error)
	// GetActiveProvider resolves the embedding provider used to generate document
	// and query embeddings for a team. It prefers the team's default-flagged
	// provider, then its most recently updated one. Returns
	// ErrNoActiveEmbeddingProvider when the team has none.
	GetActiveProvider(ctx context.Context, teamID string) (*models.EmbeddingProvider, error)
	SetDefault(ctx context.Context, teamID, providerID string) error
	UnsetAllDefaults(ctx context.Context, teamID string) error
	Count(ctx context.Context, teamID string) (int, error)
}

// EmbeddingProviderFilters represents filters for embedding provider queries
type EmbeddingProviderFilters struct {
	ProviderType *string
	Page         int
	Limit        int
}

// SubscriptionRepository was removed as part of subscription model simplification
// Subscription data is now stored directly in the User table

// ActivityRepository defines the interface for activity data access operations
type ActivityRepository interface {
	Create(ctx context.Context, activity *models.Activity) error
	GetByID(ctx context.Context, userID, activityID string) (*models.Activity, error)
	List(ctx context.Context, filters ActivityFilters) (*models.ActivityListResponse, error)
	GetStats(ctx context.Context, userID string) (*models.ActivityStatsResponse, error)
	Delete(ctx context.Context, activityID string) error
	// DeleteOlderThan deletes activity rows with created_at before the given cutoff time.
	// Returns the number of rows deleted.
	DeleteOlderThan(ctx context.Context, before time.Time) (int64, error)
}

// ResourceAccessRepository defines the interface for resource detail-access event data access operations.
type ResourceAccessRepository interface {
	// Create persists a new resource access event.
	Create(ctx context.Context, event *models.ResourceAccessEvent) error
	// GetMetricsByResource returns daily access counts grouped by source for a specific resource
	// since the given time, ordered by date then source.
	GetMetricsByResource(
		ctx context.Context,
		teamID, resourceType, resourceID string,
		since time.Time,
	) ([]models.DailyAccessCount, error)
	// GetTeamMetrics returns daily access counts grouped by source across the whole
	// team (every resource) since the given time, ordered by date then source.
	GetTeamMetrics(
		ctx context.Context,
		teamID string,
		since time.Time,
	) ([]models.DailyAccessCount, error)
	// GetTopAccessedResources returns the team's most-accessed resources since the
	// given time, ranked by access count descending and capped at `limit`, with each
	// resource's display name resolved from its owning table. An empty or "all"
	// source aggregates across channels; a concrete source (web/cli/mcp/api)
	// restricts the ranking to access events from that channel.
	GetTopAccessedResources(
		ctx context.Context,
		teamID string,
		since time.Time,
		source string,
		limit int,
	) ([]models.TopAccessedResource, error)
	// DeleteOlderThan deletes resource access event rows with created_at before the given cutoff time.
	// Returns the number of rows deleted.
	DeleteOlderThan(ctx context.Context, before time.Time) (int64, error)
}

// ContentVersionRepository defines the data-access contract for the polymorphic
// content-version history. Snapshots are keyed by (resource_type, resource_id) so
// any resource type can be versioned without a schema change.
type ContentVersionRepository interface {
	// Create inserts a snapshot, computing the next version_number for
	// (resourceType, resourceID) in SQL, and back-fills the generated id,
	// version_number, and created_at on v.
	Create(ctx context.Context, v *models.ContentVersion) error
	// ListByResource returns versions for the resource newest-first.
	ListByResource(ctx context.Context, teamID, resourceType, resourceID string) ([]*models.ContentVersion, error)
	// GetByVersionNumber returns a single version, or ErrContentVersionNotFound when absent.
	GetByVersionNumber(
		ctx context.Context, teamID, resourceType, resourceID string, versionNumber int,
	) (*models.ContentVersion, error)
	// PruneToCap deletes all but the newest `keep` versions for the resource.
	PruneToCap(ctx context.Context, resourceType, resourceID string, keep int) error
}

// ActivityFilters represents filters for activity queries
type ActivityFilters struct {
	UserID       *string
	ActivityType *string
	EntityType   *string
	EntityID     *string
	SessionID    *string
	Search       *string
	DateFrom     *string
	DateTo       *string
	Limit        int
	Offset       int
}

// ClaudeCodeHooksRepository defines the interface for Claude Code hooks data access operations
type ClaudeCodeHooksRepository interface {
	Create(ctx context.Context, payload *models.ClaudeCodeHookPayload) error
	GetByID(ctx context.Context, userID string, id int) (*models.ClaudeCodeHookPayload, error)
	List(ctx context.Context, filters ClaudeCodeHooksFilters) (*models.ClaudeCodeHooksPaginatedResponse, error)
	GetSessions(ctx context.Context, filters SessionFilters) (*models.SessionsResponse, error)
	GetSessionCounts(ctx context.Context, userID string, days int) (*models.SessionCountsResponse, error)
	GetOverviewStats(ctx context.Context, userID string) (*models.OverviewStats, error)
	GetRecentActivities(ctx context.Context, filters RecentActivitiesFilters) (*models.RecentActivitiesResponse, error)
	SessionExists(ctx context.Context, userID, sessionID string) (bool, error)
	CountUniqueSessions(ctx context.Context, userID string) (int, error)
	DeleteSession(ctx context.Context, userID, sessionID string) error
}

// ClaudeCodeHooksFilters represents filters for Claude Code hooks queries
type ClaudeCodeHooksFilters struct {
	UserID        *string
	SessionID     *string
	HookEventName *string
	ToolName      *string
	Page          int
	Limit         int
}

// SessionFilters represents filters for session queries
type SessionFilters struct {
	UserID *string
	Page   int
	Limit  int
}

// RecentActivitiesFilters represents filters for recent activities queries
type RecentActivitiesFilters struct {
	UserID        *string
	SessionID     *string
	ToolName      *string
	HookEventName *string
	DateFrom      *string
	DateTo        *string
	Page          int
	Limit         int
}

// AgentRepository defines the interface for agent data access operations
type AgentRepository interface {
	Create(ctx context.Context, agent *models.Agent) error
	GetByID(ctx context.Context, userID, teamID, agentID string) (*models.Agent, error)
	// GetByIDCrossTeam searches for an agent across all user's teams
	GetByIDCrossTeam(ctx context.Context, userID, agentID string) (*models.Agent, error)
	List(ctx context.Context, userID string, filters AgentFilters) ([]models.Agent, int, error)
	Update(ctx context.Context, agent *models.Agent) error
	Delete(ctx context.Context, userID, teamID, agentID string) error
	GetStats(ctx context.Context, userID, teamID string) (*models.AgentStatsResponse, error)
	UpdateExecutionStats(ctx context.Context, agentID string, success bool, duration int) error
	// GetNamesByIDsCrossTeam returns a map of agentID → name for the given IDs visible to userID,
	// searching across all the user's teams. Unknown or inaccessible IDs are omitted.
	GetNamesByIDsCrossTeam(ctx context.Context, userID string, ids []string) (map[string]string, error)
}

// AgentFilters represents filters for agent queries
type AgentFilters struct {
	Status    string
	Search    string
	TeamID    string
	SortBy    string
	SortOrder string
	Page      int
	Limit     int
}

// AgentExecutionRepository defines the interface for agent execution data access operations
type AgentExecutionRepository interface {
	Create(ctx context.Context, execution *models.AgentExecution) error
	GetByID(ctx context.Context, userID, executionID string) (*models.AgentExecution, error)
	List(ctx context.Context, userID string, filters AgentExecutionFilters,
	) ([]models.AgentExecution, int, error)
	Update(ctx context.Context, execution *models.AgentExecution) error
	GetByAgentID(ctx context.Context, userID, agentID string, filters AgentExecutionFilters,
	) ([]models.AgentExecution, int, error)
	GetByTaskID(ctx context.Context, userID, taskID string) (*models.AgentExecution, error)
	UpdateTaskInfo(ctx context.Context, executionID, taskID, contextID, currentState string) error
	UpdateArtifacts(ctx context.Context, executionID string, artifacts []map[string]interface{}) error
	UpdateStatus(ctx context.Context, executionID, status string) error

	// Conversation-related methods
	GetByConversationID(ctx context.Context, userID, conversationID string, limit int, before *time.Time,
	) ([]models.AgentExecution, bool, int, error)
	GetFirstExecutionInConversation(ctx context.Context, userID, conversationID string,
	) (*models.AgentExecution, error)
	UpdateConversationID(ctx context.Context, executionID, conversationID string) error
	ListConversations(ctx context.Context, userID, agentID string, page, limit int,
	) ([]models.ConversationSummary, int, error)
}

// AgentExecutionFilters represents filters for agent execution queries
type AgentExecutionFilters struct {
	AgentID  *string
	Status   *string
	DateFrom *string
	DateTo   *string
	Page     int
	Limit    int
}

// AgentExecutionEventRepository defines the interface for agent execution event data access operations
type AgentExecutionEventRepository interface {
	Create(ctx context.Context, event *models.AgentExecutionEvent) error
	GetByID(ctx context.Context, eventID string) (*models.AgentExecutionEvent, error)
	ListByExecutionID(ctx context.Context, executionID string, limit, offset int,
	) ([]models.AgentExecutionEvent, int, error)
	ListAfterSequence(ctx context.Context, executionID string, afterSequence int) ([]models.AgentExecutionEvent, error)
	GetLatestByExecutionID(ctx context.Context, executionID string) (*models.AgentExecutionEvent, error)
	CountByExecutionID(ctx context.Context, executionID string) (int, error)
}

// MemoryRepository defines the interface for memory data access operations
type MemoryRepository interface {
	Create(ctx context.Context, memory *models.Memory) error
	GetByID(ctx context.Context, userID, teamID, memoryID string) (*models.Memory, error)
	// GetByIDCrossTeam searches for a memory across all user's teams
	GetByIDCrossTeam(ctx context.Context, userID, memoryID string) (*models.Memory, error)
	List(ctx context.Context, userID string, filters MemoryFilters) ([]models.Memory, int, error)
	Update(ctx context.Context, memory *models.Memory) error
	Delete(ctx context.Context, userID, teamID, memoryID string) error
	SearchByMetadata(ctx context.Context, userID string, metadataKey, metadataValue string,
		filters MemoryFilters) ([]models.Memory, int, error)
	CountAll(ctx context.Context, userID string) (int, error)
	// GetNamesByIDsCrossTeam returns a map of memoryID → truncated text for the given IDs visible to
	// userID, searching across all the user's teams. Unknown or inaccessible IDs are omitted.
	GetNamesByIDsCrossTeam(ctx context.Context, userID string, ids []string) (map[string]string, error)
}

// MemoryFilters represents filters for memory queries
type MemoryFilters struct {
	Search        string
	MetadataKey   *string
	MetadataValue *string
	Status        *string
	TeamID        string
	ProjectID     *string
	SortBy        string
	SortOrder     string
	Page          int
	Limit         int
}

// EmbeddingRepository defines the interface for embedding data access operations
type EmbeddingRepository interface {
	Create(ctx context.Context, embedding *models.Embedding) error
	GetByEntity(ctx context.Context, userID, entityType, entityID string) ([]models.Embedding, error)
	FindSimilar(ctx context.Context, userID, entityType string, vector []float32, limit int,
	) ([]models.EmbeddingSimilarity, error)
	DeleteByEntity(ctx context.Context, entityType, entityID string) error
	// DeleteByTeam removes every embedding owned by a team, returning the number of
	// rows deleted. Used to wipe a team's vectors before re-embedding when its
	// provider's model/endpoint changes (issue #79).
	DeleteByTeam(ctx context.Context, teamID string) (int64, error)
}

// SearchRepository defines the interface for cross-entity semantic search over embeddings.
type SearchRepository interface {
	// SearchSimilar returns the page of embedding chunks (one result per chunk) whose
	// denormalized team_id matches teamID, ordered by ascending cosine distance to vec,
	// restricted to the given singular entityTypes and embedding modelID. When
	// projectID is non-empty, results are further restricted to that project. It also
	// returns the total number of matching chunks (ignoring limit/offset).
	SearchSimilar(
		ctx context.Context,
		teamID string,
		vec []float32,
		modelID string,
		entityTypes []string,
		projectID string,
		limit, offset int,
	) ([]models.SearchResultRow, int, error)

	// SearchKeyword returns the page of source rows (one result per entity) matching
	// query via PostgreSQL full-text search. It is the fallback used when no embedding
	// provider is configured: the embeddings table is empty without one, so it reads
	// the source tables directly. Rows whose team_id matches teamID are restricted to
	// the given singular entityTypes (applying each type's status filter) and ordered
	// by ts_rank relevance descending; when projectID is non-empty results are further
	// restricted to that project. The returned SearchResultRow.Distance carries
	// 1 - ts_rank so callers derive Score identically to SearchSimilar. It also returns
	// the total number of matching rows (ignoring limit/offset).
	SearchKeyword(
		ctx context.Context,
		teamID string,
		query string,
		entityTypes []string,
		projectID string,
		limit, offset int,
	) ([]models.SearchResultRow, int, error)
}

// EmbeddingBackfillRepository enumerates every embeddable entity across all users
// and teams so the embedding pipeline can be re-run after a model/dimension change.
// It reads the source tables directly (rather than the user-scoped List methods,
// which require a per-user identity that a global backfill does not have) and
// returns only the fields needed to reconstruct each entity's `.created` event.
type EmbeddingBackfillRepository interface {
	// ListEntities returns up to limit entities of entityType ordered by created_at,
	// id (a stable total order so paging never skips or repeats a row), starting at
	// offset. entityType is one of the singular embeddable types
	// (prompt, artifact, memory, blueprint, feed_item); an unsupported type
	// returns an error. When missingOnly is true, only entities lacking an
	// embedding row for modelID are returned.
	ListEntities(
		ctx context.Context, entityType, modelID, teamID string, missingOnly bool, limit, offset int,
	) ([]models.BackfillEntity, error)
	// CountCoverage returns, per embeddable entity type, how many of a team's
	// entities exist (total) and how many have an embedding under modelID
	// (embedded), using the same "has an embedding for this model" predicate as the
	// missing-only backfill so the counts agree. An empty modelID reports 0 embedded
	// (nothing matches), so a team with no active provider reads as all-pending.
	CountCoverage(
		ctx context.Context, modelID, teamID string,
	) ([]models.EmbeddingCoverageCount, error)
}

// CursorIDEHooksRepository defines the interface for Cursor IDE hooks data access operations
type CursorIDEHooksRepository interface {
	Create(ctx context.Context, payload *models.CursorIDEHookPayload) error
	GetByID(ctx context.Context, userID string, id int) (*models.CursorIDEHookPayload, error)
	List(ctx context.Context, filters CursorIDEHooksFilters) (*models.CursorIDEHooksPaginatedResponse, error)
	GetSessions(ctx context.Context, filters CursorSessionFilters) (*models.CursorSessionsResponse, error)
	GetSessionCounts(ctx context.Context, userID string, days int) (*models.CursorSessionCountsResponse, error)
	GetOverviewStats(ctx context.Context, userID string) (*models.CursorOverviewStats, error)
	GetRecentActivities(ctx context.Context, filters CursorRecentActivitiesFilters,
	) (*models.CursorRecentActivitiesResponse, error)
	SessionExists(ctx context.Context, userID, sessionID string) (bool, error)
	CountUniqueSessions(ctx context.Context, userID string) (int, error)
	DeleteSession(ctx context.Context, userID, sessionID string) error
}

// CursorIDEHooksFilters represents filters for Cursor IDE hooks queries
type CursorIDEHooksFilters struct {
	UserID        *string
	SessionID     *string
	HookEventName *string
	ToolName      *string
	Page          int
	Limit         int
}

// CursorSessionFilters represents filters for Cursor IDE session queries
type CursorSessionFilters struct {
	UserID *string
	Page   int
	Limit  int
}

// CursorRecentActivitiesFilters represents filters for Cursor IDE recent activities queries
type CursorRecentActivitiesFilters struct {
	UserID        *string
	SessionID     *string
	ToolName      *string
	HookEventName *string
	DateFrom      *string
	DateTo        *string
	Page          int
	Limit         int
}

// ResourceUsageRepository defines the interface for resource usage data access operations
type ResourceUsageRepository interface {
	IncrementUsage(ctx context.Context, userID, resourceType string, periodStart, periodEnd time.Time) error
	DecrementUsage(ctx context.Context, userID, resourceType string, periodStart, periodEnd time.Time) error
	// GetUsageCount returns (0, nil) when no usage row exists for the period.
	GetUsageCount(ctx context.Context, userID, resourceType string, periodStart, periodEnd time.Time) (int, error)
	GetResourceCounts(ctx context.Context, userID string, periodStart, periodEnd time.Time) (map[string]int, error)
}

// PromptGalleryRepository defines the interface for prompt gallery data access operations
type PromptGalleryRepository interface {
	GetCategories(ctx context.Context) ([]models.PromptGalleryCategory, error)
	List(ctx context.Context, filters PromptGalleryFilters) ([]models.PromptGalleryTemplate, int, error)
	GetByID(ctx context.Context, promptID string) (*models.PromptGalleryTemplate, error)
}

// PromptGalleryFilters represents filters for prompt gallery queries
type PromptGalleryFilters struct {
	Category string
	Search   string
	Tags     []string // Filter by tags (OR condition - matches any of the provided tags)
	Page     int
	Limit    int
}

// BackofficeRepository defines the interface for back office data access operations
type BackofficeRepository interface {
	GetUsageMetrics(ctx context.Context, fromDate, toDate *time.Time) ([]models.UsageMetricsRow, error)
	GetUserActivities(ctx context.Context) ([]models.UserActivityRow, error)
}

// BlueprintRepository defines the interface for blueprint data access operations
type BlueprintRepository interface {
	Create(ctx context.Context, blueprint *models.Blueprint) error
	GetByID(ctx context.Context, userID, teamID, blueprintID string) (*models.Blueprint, error)
	// GetByIDCrossTeam searches for a blueprint across all user's teams
	GetByIDCrossTeam(ctx context.Context, userID, blueprintID string) (*models.Blueprint, error)
	GetByProjectIDAndSlug(ctx context.Context, userID, teamID, projectID, slug string) (*models.Blueprint, error)
	// GetByProjectIDAndSlugCrossTeam searches for a blueprint across all user's teams
	GetByProjectIDAndSlugCrossTeam(ctx context.Context, userID, projectID, slug string) (*models.Blueprint, error)
	List(ctx context.Context, userID string, filters BlueprintFilters) ([]models.Blueprint, int, error)
	Update(ctx context.Context, blueprint *models.Blueprint) error
	Delete(ctx context.Context, userID, teamID, blueprintID string) error
	// GetStats returns a zero-valued response — not an error — when the user has no data.
	GetStats(ctx context.Context, userID string) (*models.BlueprintStatsResponse, error)
	// GetNamesByIDsCrossTeam returns a map of blueprintID → title for the given IDs visible to userID,
	// searching across all the user's teams. Unknown or inaccessible IDs are omitted.
	GetNamesByIDsCrossTeam(ctx context.Context, userID string, ids []string) (map[string]string, error)
}

// BlueprintFilters represents filters for blueprint queries
type BlueprintFilters struct {
	ProjectID *string
	Status    *string
	Type      *string
	Subtype   *string
	TeamID    string
	Search    string
	SortBy    string
	SortOrder string
	Metadata  map[string]string
	Page      int
	Limit     int
}

// UserPreferencesRepository defines the interface for user preferences data access operations
type UserPreferencesRepository interface {
	// GetByUserID returns (nil, nil) — not an error — when the user has no preferences row.
	GetByUserID(ctx context.Context, userID string) (*models.UserPreferences, error)
	Upsert(ctx context.Context, prefs *models.UserPreferences) error
}

// ProjectRepository defines the interface for project data access operations
type ProjectRepository interface {
	Create(ctx context.Context, project *models.Project) error
	GetBySlug(ctx context.Context, teamID, userID, slug string) (*models.Project, error)
	GetByID(ctx context.Context, userID, projectID string) (*models.Project, error)
	GetByGitURL(ctx context.Context, teamID, userID, gitURL string) (*models.Project, error)
	List(ctx context.Context, userID string, filters ProjectListFilters) ([]models.Project, int, error)
	Update(ctx context.Context, project *models.Project) error
	Delete(ctx context.Context, teamID, userID, slug string) error
	CountByTeamID(ctx context.Context, teamID string) (int, error)
	// GetNamesByIDs returns a map of projectID → name for the given IDs owned by userID.
	// Unknown or inaccessible IDs are omitted from the result.
	GetNamesByIDs(ctx context.Context, userID string, ids []string) (map[string]string, error)
	// GetProjectStats returns resource counts (prompts, artifacts, blueprints, memories, feed_items)
	// for the project identified by teamID + slug. Returns ErrProjectNotFoundForRepo when the project
	// does not exist or is not accessible to userID.
	GetProjectStats(ctx context.Context, teamID, userID, projectSlug string) (*models.ProjectStatsResponse, error)
	// GetProjectResourceCreationMetrics returns sparse per-day creation counts per
	// resource type (prompts, artifacts, blueprints, memories) for the project
	// identified by teamID + slug, counting rows created at or after `since`. Days
	// with no creations are omitted (the caller zero-fills). Returns
	// ErrProjectNotFoundForRepo when the project does not exist or is inaccessible.
	GetProjectResourceCreationMetrics(
		ctx context.Context, teamID, userID, projectSlug string, since time.Time,
	) ([]models.ProjectResourceCreationCount, error)
	// ListGitURLToSlugByTeam returns a map of git_url → slug for every project in teamID
	// that has a non-empty git_url and is accessible to userID (team owner or member).
	// Used to enrich the GitHub repositories list with the slug of any already-imported
	// project so the UI can link to the project instead of offering to import again.
	ListGitURLToSlugByTeam(ctx context.Context, teamID, userID string) (map[string]string, error)
}

// ProjectListFilters represents filters for project queries
type ProjectListFilters struct {
	Search    string
	SortBy    string
	SortOrder string
	TeamID    string
	Page      int
	Limit     int
}

// WebhookEventRepository defines the interface for webhook event data access operations
type WebhookEventRepository interface {
	// IsProcessed checks if a webhook event has already been processed
	IsProcessed(ctx context.Context, eventID string) (bool, error)
	// MarkProcessed records a webhook event as processed
	MarkProcessed(ctx context.Context, eventID, eventType string, teamID *string) error
	// GetByEventID retrieves a webhook event by its Stripe event ID
	GetByEventID(ctx context.Context, eventID string) (*models.WebhookEvent, error)
}

// GitHubInstallationRepository defines the interface for GitHub installation data access operations
type GitHubInstallationRepository interface {
	Create(ctx context.Context, installation *models.GitHubInstallation) error
	GetByTeamID(ctx context.Context, teamID string) (*models.GitHubInstallation, error)
	GetByInstallationID(ctx context.Context, installationID int64) (*models.GitHubInstallation, error)
	Update(ctx context.Context, installation *models.GitHubInstallation) error
	Delete(ctx context.Context, teamID string) error
}

// FeedRepository defines the interface for feed data access operations
type FeedRepository interface {
	Create(ctx context.Context, feed *models.Feed) error
	GetByID(ctx context.Context, userID, teamID, feedID string) (*models.Feed, error)
	List(ctx context.Context, userID string, filters FeedFilters) ([]models.Feed, int, error)
	// ListWithLastPost returns feeds enriched with the MAX(posted_at) of their feed items.
	// It is used exclusively by the MCP list-feeds tool to avoid N+1 queries.
	ListWithLastPost(ctx context.Context, userID string, filters FeedFilters) ([]models.FeedWithLastPost, error)
	Update(ctx context.Context, feed *models.Feed) error
	Delete(ctx context.Context, userID, teamID, feedID string) error
	// CountAll counts all feeds accessible to the user across all their teams.
	CountAll(ctx context.Context, userID string) (int, error)
}

// FeedFilters represents filters for feed queries
type FeedFilters struct {
	TeamID string
	Search string
	Page   int
	Limit  int
}

// FeedItemRepository defines the interface for feed item data access operations
type FeedItemRepository interface {
	Create(ctx context.Context, item *models.FeedItem) error
	GetByID(ctx context.Context, userID, teamID, itemID string) (*models.FeedItem, error)
	// GetByIDForPoster retrieves a feed item by ID scoped to the posting user
	// (posted_by_user_id). It mirrors how the embedding pipeline keys feed item
	// embeddings, so it is used to validate embedding payloads for the poster.
	GetByIDForPoster(ctx context.Context, posterUserID, itemID string) (*models.FeedItem, error)
	List(ctx context.Context, userID string, filters FeedItemFilters) ([]models.FeedItem, int, error)
	Archive(ctx context.Context, userID, teamID, itemID string) error
	Unarchive(ctx context.Context, userID, teamID, itemID string) error
	Delete(ctx context.Context, userID, teamID, itemID string) error
	// CountAll counts all feed items (including archived) accessible to the user across all their teams.
	CountAll(ctx context.Context, userID string) (int, error)
}

// FeedItemReplyPoster identifies a reply and the user who posted it. It is used to
// remove each reply's embedding row (keyed by its poster) when a feed item is
// hard-deleted, since the DB cascade removes the reply rows but not their embeddings.
type FeedItemReplyPoster struct {
	ReplyID        string
	PostedByUserID string
}

// FeedItemFilters represents filters for feed item queries
type FeedItemFilters struct {
	TeamID          string
	FeedID          *string
	ProjectID       *string
	AIAssistantName *string
	Archived        *bool // nil = default (active only), true = archived only, false = active only
	Page            int
	Limit           int
}

// FeedItemReplyRepository defines the interface for feed item reply data access operations
type FeedItemReplyRepository interface {
	CreateReply(ctx context.Context, reply *models.FeedItemReply) (*models.FeedItemReply, error)
	GetReply(ctx context.Context, userID, teamID, replyID string) (*models.FeedItemReply, error)
	// GetReplyForPoster retrieves a reply by ID scoped to the posting user
	// (posted_by_user_id). It mirrors how the embedding pipeline keys reply
	// embeddings, so it is used to validate embedding payloads for the poster.
	GetReplyForPoster(ctx context.Context, posterUserID, replyID string) (*models.FeedItemReply, error)
	ListReplies(ctx context.Context, teamID, feedItemID string, page, limit int) ([]models.FeedItemReply, int, error)
	// ListReplyPostersByItemID returns the (reply_id, posted_by_user_id) pairs for every
	// reply on feedItemID, scoped to teamID. Used to clean up each reply's embedding row
	// (keyed by its poster) before a feed item is hard-deleted.
	ListReplyPostersByItemID(ctx context.Context, teamID, feedItemID string) ([]FeedItemReplyPoster, error)
	CountRepliesByItemIDs(ctx context.Context, teamID string, itemIDs []string) (map[string]int, error)
	// CountAll counts all feed item replies accessible to the user across all their teams.
	CountAll(ctx context.Context, userID string) (int, error)
}

// NotificationListFilters controls pagination and filtering for notification queries
type NotificationListFilters struct {
	UnreadOnly bool
	Limit      int
	Offset     int
}

// NotificationRepository defines the interface for notification data access operations
type NotificationRepository interface {
	// Insert persists a notification. When DedupeKey is set the INSERT uses
	// ON CONFLICT (recipient_user_id, dedupe_key) DO NOTHING so a second call
	// with the same key returns nil without inserting a duplicate row.
	Insert(ctx context.Context, n *models.Notification) error
	ListForUser(ctx context.Context, userID string, f NotificationListFilters) ([]*models.Notification, error)
	// GetByIDsForUser returns the notifications matching ids that belong to userID.
	// The recipient_user_id filter is defence-in-depth: it ensures a future bug that
	// enqueues user B's notification ID for user A cannot leak B's content to A's digest.
	GetByIDsForUser(ctx context.Context, userID string, ids []string) ([]*models.Notification, error)
	GetUnreadCount(ctx context.Context, userID string) (int, error)
	MarkRead(ctx context.Context, userID, notifID string) error
	MarkAllRead(ctx context.Context, userID string) error
	DeleteOlderThan(ctx context.Context, before time.Time) (int64, error)
}

// NotificationDeliveryRepository defines the interface for notification delivery records
type NotificationDeliveryRepository interface {
	Insert(ctx context.Context, d *models.NotificationDelivery) error
}

// NotificationDigestQueueRepository defines the interface for the digest delivery queue
type NotificationDigestQueueRepository interface {
	Enqueue(ctx context.Context, userID, notifID string, scheduledFor time.Time) error
	// FetchPending returns all rows whose scheduled_for is before the given time and sent_at is NULL.
	FetchPending(ctx context.Context, before time.Time) ([]*models.NotificationDigestQueueRow, error)
	// MarkSent sets sent_at = sentAt for the given row IDs that have not yet been marked sent.
	// Passing an explicit sentAt makes the operation deterministic and testable.
	// The WHERE clause guards against overwriting an already-set sent_at in a race.
	MarkSent(ctx context.Context, ids []string, sentAt time.Time) error
	// TryAdvisoryLock attempts to acquire a session-level PostgreSQL advisory lock
	// identified by key. Returns (true, nil) when acquired, (false, nil) when already
	// held by another session (caller should skip the job gracefully), or (false, err)
	// on a database error. Call ReleaseAdvisoryLock when the critical section is done.
	TryAdvisoryLock(ctx context.Context, key int64) (bool, error)
	// ReleaseAdvisoryLock releases a previously acquired session-level advisory lock.
	ReleaseAdvisoryLock(ctx context.Context, key int64) error
}

// DeviceTokenRepository defines the interface for device push token data access operations
type DeviceTokenRepository interface {
	// Upsert inserts or updates a device token; on conflict updates last_used_at and user_agent
	Upsert(ctx context.Context, token *models.DeviceToken) error
	// ListByUserID returns all device tokens registered for the user
	ListByUserID(ctx context.Context, userID string) ([]*models.DeviceToken, error)
	// Delete removes a single device token scoped to the given user
	Delete(ctx context.Context, token string, userID string) error
	// DeleteByTokens removes multiple tokens (used to clean up expired FCM tokens)
	DeleteByTokens(ctx context.Context, tokens []string) error
}

// AttachmentRepository persists generic, polymorphic file-attachment metadata
// keyed by (owner_type, owner_id). The binary itself lives in object storage;
// this repository owns only the metadata rows. owner_id has no DB foreign key
// (it is polymorphic, cf. embeddings), so owner cleanup is performed in app code.
type AttachmentRepository interface {
	// Create inserts an attachment row, populating its ID and CreatedAt from the
	// persisted row on return.
	Create(ctx context.Context, attachment *models.Attachment) error
	// GetByID returns the attachment with the given id scoped to (ownerType,
	// ownerID); it returns ErrAttachmentNotFound when no such row exists so a
	// caller cannot reach another owner's attachment by id.
	GetByID(ctx context.Context, ownerType, ownerID, id string) (*models.Attachment, error)
	// GetByIDInTeam returns the attachment with the given id scoped to teamID
	// only (its owner is read from the stored row). Used by the universal
	// attachments endpoint, where item operations are keyed by the attachment's
	// own id and the caller does not supply the owner. Returns
	// ErrAttachmentNotFound when no such row exists in the team.
	GetByIDInTeam(ctx context.Context, teamID, id string) (*models.Attachment, error)
	// ListByOwner returns all attachments for (ownerType, ownerID), newest first.
	ListByOwner(ctx context.Context, ownerType, ownerID string) ([]models.Attachment, error)
	// SumSizeByOwner returns the total size_bytes of all attachments for
	// (ownerType, ownerID), used to enforce the per-owner cumulative size limit.
	SumSizeByOwner(ctx context.Context, ownerType, ownerID string) (int64, error)
	// Delete removes the attachment with the given id scoped to (ownerType,
	// ownerID); it returns ErrAttachmentNotFound when no row was deleted.
	Delete(ctx context.Context, ownerType, ownerID, id string) error
	// DeleteByOwner removes every attachment for (ownerType, ownerID) and returns
	// the deleted rows so the caller can delete the corresponding objects from
	// storage.
	DeleteByOwner(ctx context.Context, ownerType, ownerID string) ([]models.Attachment, error)
}

// TypeRepository persists the resource-type-agnostic, team-customizable type
// taxonomy (table `types`). System defaults are global rows (team_id NULL,
// is_system true) visible to every team; custom types belong to one team.
// Lookups and lists union the global rows with the caller's team rows.
type TypeRepository interface {
	// Create inserts a custom (non-system) type row, populating its ID,
	// CreatedAt and UpdatedAt from the persisted row on return. It returns
	// ErrTypeAlreadyExists when the (team_id, resource_type, slug) already
	// exists.
	Create(ctx context.Context, t *models.Type) error
	// GetBySlug returns the type matching (resourceType, slug) that is visible to
	// teamID — either a global system default or one of the team's own rows. It
	// returns ErrTypeNotFound when no such type exists.
	GetBySlug(ctx context.Context, teamID, resourceType, slug string) (*models.Type, error)
	// List returns every type visible to teamID for resourceType: the global
	// system defaults plus the team's own custom types, system defaults first.
	List(ctx context.Context, teamID, resourceType string) ([]models.Type, error)
	// DeleteCustom removes the custom type with the given id, scoped to teamID and
	// is_system = false, and atomically reassigns any resource rows that
	// reference its slug to fallbackSlug (artifacts only today). It returns
	// ErrTypeNotFound when no deletable row matched (missing, system default, or
	// another team's), leaving all rows untouched.
	DeleteCustom(ctx context.Context, teamID, id, fallbackSlug string) error
}
