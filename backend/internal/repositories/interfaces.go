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

	// ErrGitHubAppConfigNotFound is returned by GitHubAppConfigRepository reads,
	// updates and deletes when no config matches the identifiers -- including
	// when the row exists but belongs to another team, which is deliberately
	// indistinguishable from "absent" so a foreign id cannot be probed.
	ErrGitHubAppConfigNotFound = errors.New("GitHub App configuration not found")

	// ErrGitHubAppAlreadyRegistered is returned by GitHubAppConfigRepository
	// Create/Update when the App id is already registered by another team
	// (unique_github_app_id). A GitHub App has exactly one hook_url, so sharing
	// one App across teams would leave the second team's webhook token dead;
	// callers surface this as a 409 rather than storing a broken integration.
	ErrGitHubAppAlreadyRegistered = errors.New("GitHub App is already registered by another team")

	// ErrGitHubAppConfigVersionConflict is returned by
	// GitHubAppConfigRepository.Update when the supplied version no longer
	// matches the stored row (optimistic locking). Nothing is mutated; the
	// caller should re-read and retry.
	ErrGitHubAppConfigVersionConflict = errors.New("GitHub App configuration was modified concurrently")

	// ErrGitHubAppWebhookTokenTaken is returned by GitHubAppConfigRepository
	// Create/Update when the minted webhook token collides with an existing one
	// (idx_github_app_configs_webhook_token). With 32 bytes of crypto/rand this
	// is astronomically unlikely; it exists so the caller can re-mint and retry
	// instead of having to recognise a raw pq error by its message text.
	ErrGitHubAppWebhookTokenTaken = errors.New("GitHub App webhook token already in use")

	// ErrGitHubAppConfigTeamTaken is returned by GitHubAppConfigRepository.Create
	// when the team already has an App registered (unique_team_github_app). One
	// App per team is the design, so the caller should update the existing
	// config instead of creating a second one.
	ErrGitHubAppConfigTeamTaken = errors.New("team already has a GitHub App configured")

	// ErrProjectNotFoundForRepo is returned when no project exists for a given repository
	ErrProjectNotFoundForRepo = errors.New("project not found for repository")

	// ErrUserNotFound is returned by UserRepository.GetByID (and similar lookups) when no
	// row exists for the given identifier. Callers can distinguish a genuine missing user
	// from a transient DB error by checking errors.Is(err, repositories.ErrUserNotFound).
	ErrUserNotFound = errors.New("user not found")
	// ErrUserEmailTaken is returned by UserRepository.Create when the email is
	// already registered. It maps the users_email_key unique violation into a
	// domain error, so callers can answer 409 without importing lib/pq or
	// re-checking with a second query that a concurrent insert could invalidate.
	ErrUserEmailTaken = errors.New("user email already registered")

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

	// ErrAttachmentRelativePathConflict is returned by AttachmentRepository.Create
	// when the (owner_type, owner_id, relative_path) partial-unique index is
	// violated — the owner already has an attachment at that relative path (#338).
	ErrAttachmentRelativePathConflict = errors.New("attachment relative_path already exists for this owner")

	// ErrCommentNotFound is returned by CommentRepository lookups/updates/deletes
	// when no comment row matches the given identifier for the team.
	ErrCommentNotFound = errors.New("comment not found")

	// ErrRelationNotFound is returned by RelationRepository lookups/confirms/deletes
	// when no relation row matches the given identifier for the team.
	ErrRelationNotFound = errors.New("relation not found")

	// ErrFreshnessRuleNotFound is returned by FreshnessRuleRepository.Update
	// when no freshness rule row matches the given identifier for the team.
	ErrFreshnessRuleNotFound = errors.New("freshness rule not found")

	// ErrUnsupportedLastAccessedResource is returned by
	// ResourceLastAccessedRepository.UpdateLastAccessed when the resource type
	// or the access source has no denormalized last-accessed column — accesses
	// to projects and agents are recorded but are not freshness-eligible. It is
	// an expected no-op rather than a failure, so callers should recognize it
	// and not treat it as an error worth alerting on.
	ErrUnsupportedLastAccessedResource = errors.New("resource type or source has no last-accessed column")

	// ErrUnsupportedFreshnessResource is returned by
	// FreshnessCandidateRepository.ListStaleCandidates when the query names a
	// resource type or medium it cannot evaluate. Unlike
	// ErrUnsupportedLastAccessedResource this is NOT an expected no-op: rule
	// input is validated in the service layer, so reaching it means a rule was
	// stored with a value the evaluator cannot honour, and silently skipping it
	// would under-report staleness with no signal.
	ErrUnsupportedFreshnessResource = errors.New("resource type or medium is not freshness-evaluable")

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

	// ErrModelProviderNotFound is returned by ModelProviderRepository
	// lookups/updates/deletes when no provider row matches the given identifier.
	ErrModelProviderNotFound = errors.New("model provider not found")

	// ErrDefaultModelProviderNotFound is returned by
	// ModelProviderRepository.GetDefault when the team has no default provider.
	ErrDefaultModelProviderNotFound = errors.New("no default model provider found")

	// ErrTeamEmailProviderNotFound is returned by TeamEmailProviderRepository
	// reads and deletes when the team has no email provider row. For reads this
	// is the ordinary case, not a fault: it means the team has not overridden the
	// instance provider, and the caller falls back to it (epic #499 decision 2).
	ErrTeamEmailProviderNotFound = errors.New("team email provider not found")

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
	Create(ctx context.Context, user *models.User) error
	Update(ctx context.Context, user *models.User) error
	UpdateDefaultTeamID(ctx context.Context, userID, teamID string) error
	MarkOnboardingCompleted(ctx context.Context, userID string) error
	// GetNamesByIDs returns a map of userID → display name (or email when name is blank)
	// for the given set of IDs. Unknown IDs are silently omitted from the result.
	GetNamesByIDs(ctx context.Context, ids []string) (map[string]string, error)
	// DeleteByID removes a user row. It exists for compensating rollback: admin
	// user creation (#462) deletes the row it just inserted when publishing
	// `user.created` fails, so an unprovisioned account is never left behind.
	// It is NOT the admin-facing delete — that one is guarded (see #455) because a
	// user with data cascades far more widely.
	DeleteByID(ctx context.Context, userID string) error
}

// TeamRepository defines the interface for team data access operations
type TeamRepository interface {
	Create(ctx context.Context, team *models.Team) error
	GetByID(ctx context.Context, teamID string) (*models.Team, error)
	GetByOwnerID(ctx context.Context, ownerID string) (*models.Team, error)
	GetByOwnerAndSlug(ctx context.Context, ownerID, slug string) (*models.Team, error)
	Update(ctx context.Context, team *models.Team) error
	// TransferOwnership moves team ownership from fromUserID to toUserID,
	// updating teams.owner_id and both team_members.role rows in ONE
	// transaction so the team always has exactly one owner. Returns
	// ErrTeamNotFound if fromUserID no longer owns the team, and
	// ErrTeamMemberNotFound if either user has no membership row.
	// Authorization is the caller's responsibility.
	TransferOwnership(ctx context.Context, teamID, fromUserID, toUserID string) error
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
	// MetadataFilter is the JSONB containment filter behind the `metadata`
	// query parameter: keys ANDed, values within a key ORed.
	MetadataFilter MetadataFilter
	Page           int
	Limit          int
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

// ModelProviderRepository defines the interface for model provider data access operations
type ModelProviderRepository interface {
	Create(ctx context.Context, provider *models.ModelProvider) error
	GetByID(ctx context.Context, teamID, providerID string) (*models.ModelProvider, error)
	List(ctx context.Context, teamID string, filters ModelProviderFilters) ([]models.ModelProvider, int, error)
	Update(ctx context.Context, provider *models.ModelProvider) error
	Delete(ctx context.Context, teamID, providerID string) error
	GetDefault(ctx context.Context, teamID string) (*models.ModelProvider, error)
	SetDefault(ctx context.Context, teamID, providerID string) error
	UnsetAllDefaults(ctx context.Context, teamID string) error
	Count(ctx context.Context, teamID string) (int, error)
}

// ModelProviderFilters represents filters for model provider queries
type ModelProviderFilters struct {
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
	// Create persists a new resource access event, populating the event's ID
	// and CreatedAt from the stored row. Callers denormalizing off the event
	// (see ResourceLastAccessedRepository) depend on CreatedAt being the
	// database's own timestamp, so both writes agree on one instant.
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
	CountAll(ctx context.Context, userID string) (int, error)
	// GetNamesByIDsCrossTeam returns a map of memoryID → truncated text for the given IDs visible to
	// userID, searching across all the user's teams. Unknown or inaccessible IDs are omitted.
	GetNamesByIDsCrossTeam(ctx context.Context, userID string, ids []string) (map[string]string, error)
}

// MemoryFilters represents filters for memory queries
type MemoryFilters struct {
	Search string
	// MetadataFilter is the JSONB containment filter behind the `metadata`
	// query parameter: keys ANDed, values within a key ORed.
	MetadataFilter MetadataFilter
	Status         *string
	TeamID         string
	ProjectID      *string
	SortBy         string
	SortOrder      string
	Page           int
	Limit          int
}

// EmbeddingRepository defines the interface for embedding data access operations
type EmbeddingRepository interface {
	Create(ctx context.Context, embedding *models.Embedding) error
	GetByEntity(ctx context.Context, userID, entityType, entityID string) ([]models.Embedding, error)
	FindSimilar(ctx context.Context, userID, entityType string, vector []float32, limit int,
	) ([]models.EmbeddingSimilarity, error)
	// FindSimilarInTeam returns up to limit nearest cross-type neighbors of the
	// resource (entityType, entityID) within teamID — the computed `similar`
	// read payload (#427). It excludes the resource itself and any embedding
	// whose source row has been deleted, and returns an empty slice (no error)
	// when the resource has no stored embedding yet (embedding-worker lag).
	FindSimilarInTeam(
		ctx context.Context, teamID, entityType, entityID string, limit int,
	) ([]models.SimilarResource, error)
	DeleteByEntity(ctx context.Context, entityType, entityID string) error
	// DeleteByTeam removes every embedding owned by a team, returning the number of
	// rows deleted. Used to wipe a team's vectors before re-embedding when its
	// provider's model/endpoint changes (issue #79).
	DeleteByTeam(ctx context.Context, teamID string) (int64, error)
}

// Page is a limit/offset pagination window.
type Page struct {
	Limit  int
	Offset int
}

// SearchRepository defines the interface for cross-entity semantic search over embeddings.
type SearchRepository interface {
	// SearchSimilar returns the page of embedding chunks (one result per chunk) whose
	// denormalized team_id matches teamID, ordered by ascending cosine distance to vec,
	// restricted to the given singular entityTypes and embedding modelID. When
	// projectID is non-empty, results are further restricted to that project. It also
	// returns the total number of matching chunks (ignoring page's limit/offset).
	SearchSimilar(
		ctx context.Context,
		teamID string,
		vec []float32,
		modelID string,
		entityTypes []string,
		projectID string,
		page Page,
	) ([]models.SearchResultRow, int, error)

	// SearchKeyword returns the page of source rows (one result per entity) matching
	// query via PostgreSQL full-text search. It is the fallback used when no embedding
	// provider is configured: the embeddings table is empty without one, so it reads
	// the source tables directly. Rows whose team_id matches teamID are restricted to
	// the given singular entityTypes (applying each type's status filter) and ordered
	// by ts_rank relevance descending; when projectID is non-empty results are further
	// restricted to that project. The returned SearchResultRow.Distance carries
	// 1 - ts_rank so callers derive Score identically to SearchSimilar. It also returns
	// the total number of matching rows (ignoring page's limit/offset).
	SearchKeyword(
		ctx context.Context,
		teamID string,
		query string,
		entityTypes []string,
		projectID string,
		page Page,
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

// AdminUserFilters narrows and orders the instance-wide admin user listing.
// Nil pointer fields mean "no filter"; SortBy/SortOrder are allowlisted in the
// repository, so an unrecognized value falls back to the default ordering
// rather than reaching SQL text.
type AdminUserFilters struct {
	Search      *string
	IDPProvider *string
	Status      *string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	SortBy      string
	SortOrder   string
	Page        int
	Limit       int
}

// AdminTeamFilters narrows and orders the instance-wide admin team listing.
// Nil pointer fields mean "no filter"; SortBy/SortOrder are allowlisted in the
// repository.
type AdminTeamFilters struct {
	Search      *string
	IsPersonal  *bool
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	SortBy      string
	SortOrder   string
	Page        int
	Limit       int
}

// AdminProjectFilters narrows and orders the instance-wide admin project
// listing. Nil pointer fields mean "no filter"; SortBy/SortOrder are allowlisted
// in the repository.
type AdminProjectFilters struct {
	Search      *string
	TeamID      *string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	SortBy      string
	SortOrder   string
	Page        int
	Limit       int
}

// AdminRepository defines instance-level administrative data access for the
// /api/v1/admin surface. Reads are instance-wide (unscoped), unlike the
// team/user-scoped repositories.
type AdminRepository interface {
	// GetInstanceCounts returns unscoped COUNT(*) totals for the top-level
	// entities (users, teams, prompts, artifacts, memories).
	GetInstanceCounts(ctx context.Context) (models.InstanceCounts, error)
	// ListUsers returns a page of users matching the filters with each user's
	// team count, plus the total count of the filtered set. filters.Page/Limit
	// are already clamped.
	ListUsers(ctx context.Context, filters AdminUserFilters) ([]models.AdminUserListItem, int, error)
	// GetUserDetail returns one user with their team memberships, or (nil, nil)
	// when no user with that id exists.
	GetUserDetail(ctx context.Context, id string) (*models.AdminUserDetail, error)
	// ListTeams returns a page of teams matching the filters with owner and
	// member count, plus the total count of the filtered set. filters.Page/Limit
	// are already clamped.
	ListTeams(ctx context.Context, filters AdminTeamFilters) ([]models.AdminTeamListItem, int, error)
	// GetTeamDetail returns one team with owner and member list, or (nil, nil)
	// when no team with that id exists.
	GetTeamDetail(ctx context.Context, id string) (*models.AdminTeamDetail, error)

	// GetExtendedCounts returns unscoped COUNT(*) totals for every top-level
	// entity (a superset of GetInstanceCounts).
	GetExtendedCounts(ctx context.Context) (models.AdminExtendedCounts, error)
	// GetEntityBreakdowns returns a GROUP BY per entity table that has a status
	// or type column, buckets ordered most frequent first.
	GetEntityBreakdowns(ctx context.Context) ([]models.AdminEntityBreakdown, error)
	// GetSystemHealth returns the database size plus per-table ESTIMATED row
	// counts (pg_stat_user_tables.n_live_tup, not an exact COUNT(*)).
	GetSystemHealth(ctx context.Context) (models.AdminSystemHealth, error)
	// GetGrowthSeries returns SPARSE (entity, bucket, count) rows for rows
	// created in [from, to); the caller pivots and gap-fills. granularity is
	// mapped to a date_trunc unit through an allowlist.
	GetGrowthSeries(
		ctx context.Context, from, to time.Time, granularity string,
	) ([]models.AdminGrowthCount, error)
	// GetSignInSeries returns SPARSE (bucket, count) rows of successful sign-ins
	// in [from, to); the caller gap-fills.
	GetSignInSeries(
		ctx context.Context, from, to time.Time, granularity string,
	) ([]models.AdminCountPoint, error)
	// GetAccessBySourceSeries returns SPARSE (bucket, source, count) rows in
	// [from, to); the caller gap-fills.
	GetAccessBySourceSeries(
		ctx context.Context, from, to time.Time, granularity string,
	) ([]models.AdminSourcePoint, error)
	// UpdateUserStatus sets a user's lifecycle status, reporting false when no
	// user with that id exists so the caller can 404 without a second query.
	UpdateUserStatus(ctx context.Context, id, status string) (bool, error)
	// UpdateUserName updates the only admin-editable user field. Reports false
	// when no user with that id exists. Identity fields are IdP-owned and are
	// deliberately not representable here.
	UpdateUserName(ctx context.Context, id, name string) (bool, error)
	// ListProjects returns a page of projects matching the filters with their team
	// and owner, plus the total count of the filtered set. filters.Page/Limit are
	// already clamped.
	ListProjects(
		ctx context.Context, filters AdminProjectFilters,
	) ([]models.AdminProjectListItem, int, error)
	// GetProjectDetail returns one project with its team, owner and per-type
	// resource counts, or (nil, nil) when no project with that id exists.
	GetProjectDetail(ctx context.Context, id string) (*models.AdminProjectDetail, error)
	// DeleteUserIfUnblocked hard-deletes a user, but ONLY after confirming in the
	// same transaction that they own no shared team with other members — deleting
	// such a user would cascade that team away and take its members' data with it.
	// Returns (blockers, true, nil) when refused (nothing deleted),
	// (nil, true, nil) when deleted, and (nil, false, nil) for an unknown id.
	DeleteUserIfUnblocked(
		ctx context.Context, id string,
	) ([]models.AdminDeleteBlocker, bool, error)
}

// BlueprintRepository defines the interface for blueprint data access operations
type BlueprintRepository interface {
	Create(ctx context.Context, blueprint *models.Blueprint) error
	GetByID(ctx context.Context, userID, teamID, blueprintID string) (*models.Blueprint, error)
	// GetByIDCrossTeam searches for a blueprint across all user's teams
	GetByIDCrossTeam(ctx context.Context, userID, blueprintID string) (*models.Blueprint, error)
	GetByProjectIDAndSlug(ctx context.Context, userID, teamID, projectID, slug string) (*models.Blueprint, error)
	// GetByProjectIDAndPath resolves a blueprint by its canonical (project_id, path),
	// team-scoped by membership. Used by update-aware re-import (#341) to match a
	// source file to an existing blueprint before falling back to slug.
	GetByProjectIDAndPath(ctx context.Context, userID, teamID, projectID, path string) (*models.Blueprint, error)
	// GetByProjectIDAndSlugCrossTeam searches for a blueprint across all user's teams
	GetByProjectIDAndSlugCrossTeam(ctx context.Context, userID, projectID, slug string) (*models.Blueprint, error)
	List(ctx context.Context, userID string, filters BlueprintFilters) ([]models.Blueprint, int, error)
	Update(ctx context.Context, blueprint *models.Blueprint) error
	// UpdateOnReimport refreshes an existing blueprint from a changed repo file:
	// content/raw/content_sha/metadata/title/description/path AND provenance
	// (source_*/imported_at). Unlike Update (which preserves provenance across a
	// user edit), re-import intentionally rewrites it (#341).
	UpdateOnReimport(ctx context.Context, blueprint *models.Blueprint) error
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
	// MetadataFilter is the JSONB containment filter behind the `metadata`
	// query parameter: keys ANDed, values within a key ORed.
	MetadataFilter MetadataFilter
	Page           int
	Limit          int
}

// UserPreferencesRepository defines the interface for user preferences data access operations
type UserPreferencesRepository interface {
	// GetByUserID returns (nil, nil) — not an error — when the user has no preferences row.
	GetByUserID(ctx context.Context, userID string) (*models.UserPreferences, error)
	Upsert(ctx context.Context, prefs *models.UserPreferences) error
}

// TeamSearchSettingsRepository defines the interface for per-team search
// ranking override data access operations.
//
// The override is whole-row: a team either has a complete profile stored or no
// row at all, in which case it inherits the instance defaults from config.yaml.
type TeamSearchSettingsRepository interface {
	// Get returns (nil, nil) — not an error — when the team has no override
	// row, so callers can fall back to the instance defaults.
	Get(ctx context.Context, teamID string) (*models.TeamSearchSettings, error)
	Upsert(ctx context.Context, settings *models.TeamSearchSettings) error
	// Delete removes the team's override row. Deleting when no row exists is a
	// no-op, not an error.
	Delete(ctx context.Context, teamID string) error
}

// MetadataCatalogRepository enumerates the metadata keys and values in use
// across a team's artifacts, blueprints or memories (epic #519).
//
// Both lookups carry the same tenancy predicate as the corresponding list
// query, so the catalog can never surface a key or value that originates from
// a team the caller cannot read.
type MetadataCatalogRepository interface {
	// Keys returns the distinct metadata keys in ascending order.
	Keys(ctx context.Context, query MetadataCatalogQuery) (MetadataCatalogResult, error)
	// Values returns the distinct values stored under query.Key, in ascending
	// order, flattening array-valued metadata and skipping object-valued keys.
	Values(ctx context.Context, query MetadataCatalogQuery) (MetadataCatalogResult, error)
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
	// ListByTeamID returns every project in the team, ordered by name. It is
	// tenancy-only by design: unlike List it takes no userID, because its
	// callers report on the team as a whole (membership having already been
	// established) and a per-user view would silently drop projects from a
	// team-wide total.
	ListByTeamID(ctx context.Context, teamID string) ([]models.Project, error)
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
	// GetByAppConfigAndInstallationID resolves an installation within ONE App.
	// installation_id is only unique per App since #477, so a webhook delivery
	// must be matched against the App it arrived for — otherwise a delivery for
	// team A could mutate team B's installation when the numeric ids collide.
	GetByAppConfigAndInstallationID(
		ctx context.Context, appConfigID string, installationID int64,
	) (*models.GitHubInstallation, error)
	Update(ctx context.Context, installation *models.GitHubInstallation) error
	Delete(ctx context.Context, teamID string) error
}

// GitHubAppConfigRepository defines the data access operations for a team's own
// GitHub App registration (#477).
//
// Every method except GetByWebhookToken takes a teamID and puts it in the WHERE
// clause: an under-scoped query here hands another team's GitHub credentials
// out, so tenancy is enforced by construction rather than by the caller
// remembering to check. The repository stores whatever ciphertext it is handed
// and never encrypts or decrypts.
type GitHubAppConfigRepository interface {
	// Create inserts a config, filling ID/CreatedAt/UpdatedAt/Version on the
	// passed struct. Returns ErrGitHubAppConfigTeamTaken when the team already
	// has one, and ErrGitHubAppAlreadyRegistered when the App id is taken.
	Create(ctx context.Context, config *models.GitHubAppConfig) error
	// GetByTeamID returns the team's config, or ErrGitHubAppConfigNotFound.
	GetByTeamID(ctx context.Context, teamID string) (*models.GitHubAppConfig, error)
	// GetByID returns the config only when it belongs to teamID; a foreign id
	// yields ErrGitHubAppConfigNotFound, never the row.
	GetByID(ctx context.Context, teamID, configID string) (*models.GitHubAppConfig, error)
	// Update applies an optimistic-locked update keyed on
	// (id, team_id, version), refreshing UpdatedAt/Version on success. Returns
	// ErrGitHubAppConfigVersionConflict when the version is stale or the config
	// does not belong to teamID, and ErrGitHubAppAlreadyRegistered on an App-id
	// collision.
	Update(ctx context.Context, config *models.GitHubAppConfig) error
	// Delete removes the team's config by id, cascading to its installations.
	// Returns ErrGitHubAppConfigNotFound when nothing matched.
	Delete(ctx context.Context, teamID, configID string) error
	// GetByWebhookToken resolves a config from the opaque token embedded in its
	// webhook URL. This is the ONE deliberately un-team-scoped read: the public
	// webhook route has no team context until this lookup supplies it. Returns
	// ErrGitHubAppConfigNotFound for an unknown token.
	GetByWebhookToken(ctx context.Context, token string) (*models.GitHubAppConfig, error)
}

// TeamEmailProviderRepository defines the data access operations for a team's
// own outbound email provider (#501, epic #499).
//
// The table holds at most one row per team, so every method is keyed on teamID
// rather than a row id — there is no provider to address independently of its
// team, and an under-scoped query here would hand another team's mail
// credentials out. The repository stores whatever ciphertext it is handed and
// never encrypts or decrypts.
type TeamEmailProviderRepository interface {
	// GetByTeamID returns the team's provider, or ErrTeamEmailProviderNotFound
	// when the team has none — which means "inherits the instance provider",
	// not "broken".
	GetByTeamID(ctx context.Context, teamID string) (*models.TeamEmailProvider, error)
	// Upsert creates or replaces the team's provider in one statement, keyed on
	// team_id, bumping Version and refreshing ID/CreatedAt/UpdatedAt on the
	// passed struct. Calling it twice for a team updates in place; it can never
	// produce a second row.
	Upsert(ctx context.Context, provider *models.TeamEmailProvider) error
	// Delete removes the team's provider, reverting it to the instance
	// provider. Returns ErrTeamEmailProviderNotFound when there was nothing to
	// delete.
	Delete(ctx context.Context, teamID string) error
	// RecordSendResult stamps the outcome of one send attempt: last_success_at
	// when sendErr is nil, otherwise last_error and last_error_at. It writes
	// only those health columns and deliberately does NOT bump Version, so it
	// can never clobber a concurrent configuration change. A success does not
	// clear the previous error — current health is derived by comparing the two
	// timestamps (see models.TeamEmailProvider.IsHealthy), which keeps the last
	// failure readable after recovery.
	RecordSendResult(ctx context.Context, teamID string, sendErr error, at time.Time) error
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
	Search          string
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

// CommentRepository persists team-visible resource comments, keyed by the
// polymorphic (resource_type, resource_id) pair. resource_id has no DB foreign
// key (it spans four resource tables), so a resource's comments are removed in
// app code (DeleteByResource) when the resource is deleted. Every query is
// scoped by team_id (tenancy only, no role predicates — decision D3); the
// own-vs-any and author-only decisions live in the service layer.
type CommentRepository interface {
	// Create inserts a comment row, populating its ID, CreatedAt and UpdatedAt
	// from the persisted row on return.
	Create(ctx context.Context, comment *models.Comment) error
	// GetByID returns the comment with the given id scoped to teamID; it returns
	// ErrCommentNotFound when no such row exists in the team.
	GetByID(ctx context.Context, teamID, id string) (*models.Comment, error)
	// ListByResource returns a page of comments for (resourceType, resourceID)
	// within teamID, newest-first, together with the total count.
	ListByResource(
		ctx context.Context, teamID, resourceType, resourceID string, page, limit int,
	) ([]models.Comment, int, error)
	// ListRecentByTeam returns up to limit comments across the team ordered by
	// latest activity (GREATEST(created_at, updated_at) DESC), each enriched with
	// its resource's resolved title and link fields. Comments whose resource has
	// been deleted are omitted.
	ListRecentByTeam(ctx context.Context, teamID string, limit int) ([]models.CommentActivity, error)
	// UpdateContent sets the content (and updated_at) of the comment with the
	// given id in teamID, returning the updated row. It returns ErrCommentNotFound
	// when no such row exists. Author enforcement is done in the service layer.
	UpdateContent(ctx context.Context, teamID, id, content string) (*models.Comment, error)
	// Delete removes the comment with the given id scoped to teamID; it returns
	// ErrCommentNotFound when no row was deleted.
	Delete(ctx context.Context, teamID, id string) error
	// DeleteByResource removes every comment for (resourceType, resourceID) within
	// teamID. Used by the resource-delete cascade; a zero count is not an error.
	DeleteByResource(ctx context.Context, teamID, resourceType, resourceID string) (int64, error)
	// DeleteByUser removes every comment authored by userID within teamID. Used by
	// the team-member-removal cascade; a zero count is not an error.
	DeleteByUser(ctx context.Context, teamID, userID string) (int64, error)
	// ResourceExists reports whether a resource of resourceType with resourceID
	// exists in teamID, used to reject a comment on a non-existent/foreign resource.
	ResourceExists(ctx context.Context, teamID, resourceType, resourceID string) (bool, error)
}

// RelationRepository persists typed, directed edges between the four resource
// types, keyed by the polymorphic (from_type, from_id) and (to_type, to_id)
// endpoints. Neither *_id carries a DB foreign key (each spans four resource
// tables), so a resource's edges are removed in app code (DeleteByResource)
// when it is deleted. Every query is scoped by team_id (tenancy only, no role
// predicates — decision D3); the own-vs-any and confirm decisions live in the
// service layer.
type RelationRepository interface {
	// Create inserts an edge idempotently: on a duplicate of the unique
	// (team_id, from_type, from_id, relation_type, to_type, to_id) tuple it is a
	// no-op that returns the pre-existing row. The returned Relation is always
	// the persisted row (new or existing), with ID/timestamps populated; the
	// bool reports whether a new row was inserted (true) or the edge already
	// existed (false) — the REST layer maps it to 201 vs 200.
	Create(ctx context.Context, relation *models.Relation) (*models.Relation, bool, error)
	// GetByID returns the relation with the given id scoped to teamID; it returns
	// ErrRelationNotFound when no such row exists in the team.
	GetByID(ctx context.Context, teamID, id string) (*models.Relation, error)
	// ListByResource returns a page of the relations touching (resourceType,
	// resourceID) in teamID — both directions (the resource as subject or as
	// object) — newest-first, each enriched with the OTHER endpoint's resolved
	// title/link fields, together with the total count.
	ListByResource(
		ctx context.Context, teamID, resourceType, resourceID string, page, limit int,
	) ([]models.RelatedResource, int, error)
	// Confirm flips a suggested edge to confirmed and records confirmedBy,
	// returning the updated row. It only affects a row still in the suggested
	// state; it returns ErrRelationNotFound when no such suggested row exists
	// (already confirmed rows are left untouched — the service pre-checks to give
	// a distinct already-confirmed error).
	Confirm(ctx context.Context, teamID, id, confirmedBy string) (*models.Relation, error)
	// Delete removes the relation with the given id scoped to teamID; it returns
	// ErrRelationNotFound when no row was deleted.
	Delete(ctx context.Context, teamID, id string) error
	// DeleteByResource removes every edge in teamID where (resourceType,
	// resourceID) appears on EITHER endpoint. Used by the resource-delete
	// cascade; a zero count is not an error.
	DeleteByResource(ctx context.Context, teamID, resourceType, resourceID string) (int64, error)
	// ResourceProjectID returns the project_id of the resource of resourceType
	// with resourceID in teamID, and whether it exists. It doubles as the
	// endpoint existence check and the same-project comparand for relation
	// creation.
	ResourceProjectID(
		ctx context.Context, teamID, resourceType, resourceID string,
	) (projectID string, exists bool, err error)
	// FindSeedCandidates returns similar (entity, entity) pairs within teamID —
	// a cosine self-join over the embeddings table (same model, distinct
	// entities, both of a relatable type) with distance below maxDistance,
	// nearest first, capped at limit. Used by the one-shot relation seed
	// backfill (#426); the service types each pair via the constraint matrix.
	FindSeedCandidates(
		ctx context.Context, teamID string, maxDistance float64, limit int,
	) ([]models.RelationSeedCandidate, error)
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

// ScheduleRepository persists per-team recurring-job schedules for the
// in-process scheduler (table `schedules`, epic #725). Tenancy-only: every
// operation is scoped by team_id, with no role predicates (authz decision
// D3).
type ScheduleRepository interface {
	// Upsert creates the schedule for (TeamID, JobType) or updates the existing
	// row's interval and next run time, populating the model's ID, CreatedAt and
	// UpdatedAt from the persisted row on return. Relies on the
	// (team_id, job_type) unique constraint.
	Upsert(ctx context.Context, s *models.Schedule) error
	// ListDue returns schedules whose next_run_at is at or before the database
	// clock (now()), ordered by next_run_at ascending (most overdue first),
	// capped at limit. Due-ness is computed by the database so all replicas
	// agree on what is due regardless of app-server clock skew.
	ListDue(ctx context.Context, limit int) ([]*models.Schedule, error)
	// MarkRun records a run of the schedule with the given id against the
	// database clock: sets last_run_at = now() and atomically advances
	// next_run_at to now() + interval_seconds. Advancing from the run time (not
	// the old next_run_at) skips runs missed during downtime instead of
	// catching them up. It is an error when no schedule has the given id.
	MarkRun(ctx context.Context, id string) error
	// Delete removes the schedule for (teamID, jobType). It is not an error
	// when no such schedule exists.
	Delete(ctx context.Context, teamID, jobType string) error
}

// ResourceFreshnessRepository persists system-owned staleness state, one row
// per resource (table `resource_freshness`, epic #726). A row exists only
// while the resource is stale, so "clear" is a delete and the audit log is
// what preserves history. Tenancy-only: no role predicates (authz decision
// D3).
type ResourceFreshnessRepository interface {
	// Upsert marks the resource stale, or refreshes the state of an already
	// stale one, keyed on (ResourceType, ResourceID). Since is preserved from
	// the existing row on conflict, so it keeps meaning "first marked at" for
	// a resource that stays stale across evaluations; the model is populated
	// from the persisted row on return.
	Upsert(ctx context.Context, f *models.ResourceFreshness) error
	// GetByResource returns the freshness state of one resource, or
	// (nil, nil) -- not an error -- when the resource is not stale.
	GetByResource(ctx context.Context, resourceType, resourceID string) (*models.ResourceFreshness, error)
	// List returns a team's stale resources, newest-stale first, together with
	// the total row count matching the filters (ignoring limit/offset) for
	// pagination.
	List(ctx context.Context, filters models.ResourceFreshnessFilters) ([]*models.ResourceFreshness, int, error)
	// ListAllByTeam returns every freshness row of a team, unpaginated and in
	// no particular order. It exists for rule evaluation, which reconciles the
	// team's whole stale set in one pass and therefore needs the complete
	// stored state rather than a page of it.
	ListAllByTeam(ctx context.Context, teamID string) ([]*models.ResourceFreshness, error)
	// DeleteByResource clears the freshness state of one resource, reporting
	// whether a row was actually removed. Clearing a resource that is not
	// stale is a no-op, not an error.
	DeleteByResource(ctx context.Context, resourceType, resourceID string) (bool, error)
	// CountStaleByType returns how many resources are stale per resource type
	// in the team, SPARSE: a type with nothing stale is absent. The caller
	// zero-fills, because it — not the database — knows the full type set.
	CountStaleByType(ctx context.Context, teamID string) ([]models.FreshnessBucketCount, error)
	// CountStaleByProject returns the same grouped by project id, sparse.
	CountStaleByProject(ctx context.Context, teamID string) ([]models.FreshnessBucketCount, error)
	// CountStaleByRule returns the same grouped by matching rule id, sparse.
	// A resource matched by several rules contributes to each of them, because
	// staleness is a union across rules.
	CountStaleByRule(ctx context.Context, teamID string) ([]models.FreshnessBucketCount, error)
	// CountStale returns how many DISTINCT resources are stale in the team --
	// not the sum of any of the groupings above, which double-count a resource
	// matched by more than one rule.
	CountStale(ctx context.Context, teamID string) (int, error)
	// RemoveRule strips ruleID from every row's matched_rule_ids and then
	// deletes the rows left matching no rule at all. This is the cleanup a
	// rule deletion must perform: matched_rule_ids carries no foreign key (one
	// would cascade-delete freshness rows), so without it the state would
	// retain ids of rules that no longer exist. It returns the number of rows
	// deleted for having no remaining match.
	RemoveRule(ctx context.Context, ruleID string) (int64, error)
}

// ResourceLastAccessedRepository maintains the per-medium last-accessed
// columns denormalized onto the four resource tables in migration 013 (epic
// #726). It exists so freshness evaluation is an indexed column compare rather
// than an aggregate over `resource_access_events`, whose rows are pruned on a
// retention TTL and so cannot answer a threshold longer than that window.
//
// This is a write-only seam on the asynchronous access path; reads of the
// columns belong to whoever evaluates the rules.
type ResourceLastAccessedRepository interface {
	// UpdateLastAccessed advances the resource's column for the given access
	// source to `at`. The write is monotonic — a late-arriving event can never
	// move the value backwards — and deliberately leaves `updated_at` alone, so
	// a read never looks like an edit.
	//
	// It returns ErrUnsupportedLastAccessedResource, without touching the
	// database, when the resource type or source has no column; that is the
	// expected outcome for project and agent accesses, not a failure.
	UpdateLastAccessed(ctx context.Context, resourceType, resourceID, source string, at time.Time) error
}

// FreshnessRuleRepository persists per-team freshness rules (table
// `freshness_rules`, epic #726). Tenancy-only: every operation is scoped by
// team_id, with no role predicates (authz decision D3) -- authorization
// happens in the service layer.
type FreshnessRuleRepository interface {
	// Create inserts the rule, populating ID, CreatedAt and UpdatedAt from the
	// persisted row on return.
	Create(ctx context.Context, rule *models.FreshnessRule) error
	// GetByID returns one rule scoped to its team, or (nil, nil) -- not an
	// error -- when no such rule exists in that team.
	GetByID(ctx context.Context, teamID, ruleID string) (*models.FreshnessRule, error)
	// ListByTeam returns the team's rules, oldest first. When enabledOnly is
	// true only enabled rules are returned, which is what rule evaluation
	// loads.
	ListByTeam(ctx context.Context, teamID string, enabledOnly bool) ([]*models.FreshnessRule, error)
	// Update replaces the mutable fields of the rule identified by
	// (rule.TeamID, rule.ID), refreshing UpdatedAt from the persisted row. It
	// returns ErrFreshnessRuleNotFound when no row matched.
	Update(ctx context.Context, rule *models.FreshnessRule) error
	// Delete removes the rule, reporting whether a row was actually removed.
	// Callers must also run ResourceFreshnessRepository.RemoveRule so no
	// freshness state keeps referencing the deleted rule.
	Delete(ctx context.Context, teamID, ruleID string) (bool, error)
}

// TeamFreshnessSettingsRepository persists the per-team freshness settings
// singleton (table `team_freshness_settings`, epic #726). The override is
// whole-row: a team either has a complete profile stored or no row at all, in
// which case it inherits the defaults (models.DefaultTeamFreshnessSettings).
type TeamFreshnessSettingsRepository interface {
	// Get returns (nil, nil) -- not an error -- when the team has no settings
	// row, so callers can fall back to the defaults.
	Get(ctx context.Context, teamID string) (*models.TeamFreshnessSettings, error)
	// Upsert writes the whole row and increments its version, populating
	// CreatedAt, UpdatedAt and Version from the persisted row on return. Like
	// team_search_settings, the write is unconditional: Version is a monotonic
	// counter for callers to compare, not a compare-and-swap performed here.
	Upsert(ctx context.Context, settings *models.TeamFreshnessSettings) error
	// Delete removes the team's settings row, reverting it to the defaults.
	// Deleting when no row exists is a no-op, not an error.
	Delete(ctx context.Context, teamID string) error
}

// FreshnessCandidateRepository answers "which resources does this rule
// currently consider stale" against the four resource tables (issue #732).
//
// It is separate from ResourceFreshnessRepository on purpose: that repository
// owns the system-owned state table, while this one only READS the resource
// tables to derive candidates. Keeping them apart is what stops the state
// repository from acquiring knowledge of the resource schemas.
type FreshnessCandidateRepository interface {
	// ListStaleCandidates returns the resources matching the query's scope
	// whose most recent touch -- the later of the resource's own updated_at
	// and its selected per-medium last-accessed columns -- is older than the
	// threshold. A resource never accessed through any selected medium falls
	// back to updated_at, so "never accessed" is eligible rather than exempt.
	//
	// Results are ordered by resource id ascending, so passing the last id
	// back as Query.AfterID reads the next batch.
	ListStaleCandidates(ctx context.Context, query models.FreshnessCandidateQuery) ([]models.FreshnessCandidate, error)
}

// FreshnessAuditRepository appends to and reads the freshness mark/clear log
// (table `resource_freshness_audit`, epic #726). The log is append-only:
// there is deliberately no update or delete, and rows disappear only with
// their team.
type FreshnessAuditRepository interface {
	// Create appends one entry, populating ID and CreatedAt from the
	// persisted row on return.
	Create(ctx context.Context, entry *models.ResourceFreshnessAudit) error
	// ListByTeam returns a team's audit entries newest first, together with
	// the total entry count for pagination. Ordering breaks created_at ties on
	// id so pagination is stable: a single rule run writes one transaction
	// timestamp to every row it marks.
	ListByTeam(ctx context.Context, teamID string, limit, offset int) ([]*models.ResourceFreshnessAudit, int, error)
	// CountTransitionsByDay returns how many resources were marked and cleared
	// per UTC day at or after `since`, SPARSE: a day with no activity is
	// absent, and the caller zero-fills the series. The date is rendered as
	// text in the exact YYYY-MM-DD layout the series keys use, so the two
	// align without any parsing.
	CountTransitionsByDay(
		ctx context.Context, teamID string, since time.Time,
	) ([]models.FreshnessTransitionCount, error)
}
