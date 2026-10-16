package models

import (
	"encoding/json"
	"time"
)

// AuditActorType says who performed an audited operation.
type AuditActorType string

// Actor types of an audit event. They match the audit_events_actor_type_check
// constraint.
const (
	// AuditActorUser is a signed-in user, including an AI tool acting on a
	// user's API key or OAuth grant: the human owner is the actor.
	AuditActorUser AuditActorType = "user"
	// AuditActorSystem is the server itself: a scheduled job or a migration.
	AuditActorSystem AuditActorType = "system"
	// AuditActorAnonymous is an unauthenticated HTTP request, e.g. a failed
	// sign-in.
	AuditActorAnonymous AuditActorType = "anonymous"
)

// AuditSource is the channel an audited operation arrived through. The first
// four values are the resourceaccess.Source* vocabulary.
type AuditSource string

// Sources of an audit event. They match the audit_events_source_check
// constraint.
const (
	AuditSourceWeb    AuditSource = "web"
	AuditSourceCLI    AuditSource = "cli"
	AuditSourceMCP    AuditSource = "mcp"
	AuditSourceAPI    AuditSource = "api"
	AuditSourceSystem AuditSource = "system"
)

// AuditOutcome says how an audited operation ended.
type AuditOutcome string

// Outcomes of an audit event. They match the audit_events_outcome_check
// constraint.
const (
	AuditOutcomeSuccess AuditOutcome = "success"
	// AuditOutcomeDenied is an authorization denial.
	AuditOutcomeDenied AuditOutcome = "denied"
	// AuditOutcomeFailure is an attempt that failed for another reason, e.g. a
	// sign-in with a wrong password.
	AuditOutcomeFailure AuditOutcome = "failure"
)

// AuditOperation names an audited operation as a dotted <domain>.<verb>.
//
// The set is closed: AllAuditOperations lists every value, and the admin API
// publishes the same list as its operation enum, so the operation filter can be
// a dropdown. Adding an operation means adding its constant here and to
// AllAuditOperations.
type AuditOperation string

// Resource writes. Only the operations a resource type supports exist: memories
// cannot be locked, and feeds can neither move nor be locked. A move is a
// resource changing project; a project changing team is project.migrated.
const (
	AuditOpPromptCreate AuditOperation = "prompt.create"
	AuditOpPromptUpdate AuditOperation = "prompt.update"
	AuditOpPromptMove   AuditOperation = "prompt.move"
	AuditOpPromptDelete AuditOperation = "prompt.delete"
	AuditOpPromptLock   AuditOperation = "prompt.lock"
	AuditOpPromptUnlock AuditOperation = "prompt.unlock"

	AuditOpBlueprintCreate AuditOperation = "blueprint.create"
	AuditOpBlueprintUpdate AuditOperation = "blueprint.update"
	AuditOpBlueprintMove   AuditOperation = "blueprint.move"
	AuditOpBlueprintDelete AuditOperation = "blueprint.delete"
	AuditOpBlueprintLock   AuditOperation = "blueprint.lock"
	AuditOpBlueprintUnlock AuditOperation = "blueprint.unlock"

	AuditOpArtifactCreate AuditOperation = "artifact.create"
	AuditOpArtifactUpdate AuditOperation = "artifact.update"
	AuditOpArtifactMove   AuditOperation = "artifact.move"
	AuditOpArtifactDelete AuditOperation = "artifact.delete"
	AuditOpArtifactLock   AuditOperation = "artifact.lock"
	AuditOpArtifactUnlock AuditOperation = "artifact.unlock"

	AuditOpMemoryCreate AuditOperation = "memory.create"
	AuditOpMemoryUpdate AuditOperation = "memory.update"
	AuditOpMemoryMove   AuditOperation = "memory.move"
	AuditOpMemoryDelete AuditOperation = "memory.delete"

	AuditOpFeedCreate AuditOperation = "feed.create"
	AuditOpFeedUpdate AuditOperation = "feed.update"
	AuditOpFeedDelete AuditOperation = "feed.delete"

	AuditOpFeedItemCreate    AuditOperation = "feed_item.create"
	AuditOpFeedItemDelete    AuditOperation = "feed_item.delete"
	AuditOpFeedItemArchive   AuditOperation = "feed_item.archive"
	AuditOpFeedItemUnarchive AuditOperation = "feed_item.unarchive"

	AuditOpProjectCreate   AuditOperation = "project.create"
	AuditOpProjectUpdate   AuditOperation = "project.update"
	AuditOpProjectDelete   AuditOperation = "project.delete"
	AuditOpProjectMigrated AuditOperation = "project.migrated"
)

// Authentication and security. The auth.password.* operations are named
// AuditOpAuthLocalCredential* in Go: SonarCloud (go:S2068) treats any string
// constant whose name contains "password" as a hard-coded credential.
//
// #nosec G101 -- the api_key values are operation names, not credentials.
const (
	AuditOpAuthLogin                          AuditOperation = "auth.login"
	AuditOpAuthLoginFailed                    AuditOperation = "auth.login_failed"
	AuditOpAuthLogout                         AuditOperation = "auth.logout"
	AuditOpAuthSetupSessionStarted            AuditOperation = "auth.setup_session.started"
	AuditOpAuthSessionsRevoked                AuditOperation = "auth.sessions.revoked"
	AuditOpAuthAPIKeyCreated                  AuditOperation = "auth.api_key.created"
	AuditOpAuthAPIKeyRevoked                  AuditOperation = "auth.api_key.revoked"
	AuditOpAuthAPIAccessRevoked               AuditOperation = "auth.api_access.revoked"
	AuditOpAuthLocalCredentialSet             AuditOperation = "auth.password.set"
	AuditOpAuthLocalCredentialChanged         AuditOperation = "auth.password.changed"
	AuditOpAuthLocalCredentialRemoved         AuditOperation = "auth.password.removed"
	AuditOpAuthLocalCredentialResetRequested  AuditOperation = "auth.password.reset_requested"
	AuditOpAuthLocalCredentialResetLinkIssued AuditOperation = "auth.password.reset_link_issued"
	AuditOpAuthIdentityLinked                 AuditOperation = "auth.identity.linked"
	AuditOpAuthIdentityUnlinked               AuditOperation = "auth.identity.unlinked"
	AuditOpAuthMCPClientRegistered            AuditOperation = "auth.mcp_client.registered"
	AuditOpAuthMCPConsentGranted              AuditOperation = "auth.mcp_consent.granted"
	AuditOpAuthMCPConsentDenied               AuditOperation = "auth.mcp_consent.denied"
	// AuditOpAuthMCPGrantRevoked is reserved: there is no per-app grant
	// revocation yet, so nothing records it.
	AuditOpAuthMCPGrantRevoked AuditOperation = "auth.mcp_grant.revoked"
)

// Team administration.
const (
	AuditOpTeamCreated              AuditOperation = "team.created"
	AuditOpTeamUpdated              AuditOperation = "team.updated"
	AuditOpTeamDeleted              AuditOperation = "team.deleted"
	AuditOpTeamInviteSent           AuditOperation = "team.invite.sent"
	AuditOpTeamInviteAccepted       AuditOperation = "team.invite.accepted"
	AuditOpTeamInviteRejected       AuditOperation = "team.invite.rejected"
	AuditOpTeamInviteRevoked        AuditOperation = "team.invite.revoked"
	AuditOpTeamMemberAdded          AuditOperation = "team.member.added"
	AuditOpTeamMemberRoleChanged    AuditOperation = "team.member.role_changed"
	AuditOpTeamMemberRemoved        AuditOperation = "team.member.removed"
	AuditOpTeamOwnershipTransferred AuditOperation = "team.ownership.transferred"
)

// Instance administration.
const (
	AuditOpInstanceSettingsUpdated AuditOperation = "instance.settings.updated"
	AuditOpAdminUserCreated        AuditOperation = "admin.user.created"
	AuditOpAdminUserUpdated        AuditOperation = "admin.user.updated"
	AuditOpAdminUserSuspended      AuditOperation = "admin.user.suspended"
	AuditOpAdminUserUnsuspended    AuditOperation = "admin.user.unsuspended"
	AuditOpAdminUserDeleted        AuditOperation = "admin.user.deleted"
)

// Authorization and the audit log itself.
const (
	AuditOpAuthzDenied          AuditOperation = "authz.denied"
	AuditOpAuditExport          AuditOperation = "audit.export"
	AuditOpAuditSettingsUpdated AuditOperation = "audit.settings.updated"
)

// allAuditOperations is the closed vocabulary, in the order it is published.
var allAuditOperations = []AuditOperation{
	AuditOpPromptCreate, AuditOpPromptUpdate, AuditOpPromptMove,
	AuditOpPromptDelete, AuditOpPromptLock, AuditOpPromptUnlock,
	AuditOpBlueprintCreate, AuditOpBlueprintUpdate, AuditOpBlueprintMove,
	AuditOpBlueprintDelete, AuditOpBlueprintLock, AuditOpBlueprintUnlock,
	AuditOpArtifactCreate, AuditOpArtifactUpdate, AuditOpArtifactMove,
	AuditOpArtifactDelete, AuditOpArtifactLock, AuditOpArtifactUnlock,
	AuditOpMemoryCreate, AuditOpMemoryUpdate, AuditOpMemoryMove, AuditOpMemoryDelete,
	AuditOpFeedCreate, AuditOpFeedUpdate, AuditOpFeedDelete,
	AuditOpFeedItemCreate, AuditOpFeedItemDelete, AuditOpFeedItemArchive, AuditOpFeedItemUnarchive,
	AuditOpProjectCreate, AuditOpProjectUpdate, AuditOpProjectDelete, AuditOpProjectMigrated,

	AuditOpAuthLogin, AuditOpAuthLoginFailed, AuditOpAuthLogout,
	AuditOpAuthSetupSessionStarted, AuditOpAuthSessionsRevoked,
	AuditOpAuthAPIKeyCreated, AuditOpAuthAPIKeyRevoked, AuditOpAuthAPIAccessRevoked,
	AuditOpAuthLocalCredentialSet, AuditOpAuthLocalCredentialChanged, AuditOpAuthLocalCredentialRemoved,
	AuditOpAuthLocalCredentialResetRequested, AuditOpAuthLocalCredentialResetLinkIssued,
	AuditOpAuthIdentityLinked, AuditOpAuthIdentityUnlinked,
	AuditOpAuthMCPClientRegistered, AuditOpAuthMCPConsentGranted,
	AuditOpAuthMCPConsentDenied, AuditOpAuthMCPGrantRevoked,

	AuditOpTeamCreated, AuditOpTeamUpdated, AuditOpTeamDeleted,
	AuditOpTeamInviteSent, AuditOpTeamInviteAccepted, AuditOpTeamInviteRejected, AuditOpTeamInviteRevoked,
	AuditOpTeamMemberAdded, AuditOpTeamMemberRoleChanged, AuditOpTeamMemberRemoved,
	AuditOpTeamOwnershipTransferred,

	AuditOpInstanceSettingsUpdated,
	AuditOpAdminUserCreated, AuditOpAdminUserUpdated, AuditOpAdminUserSuspended,
	AuditOpAdminUserUnsuspended, AuditOpAdminUserDeleted,

	AuditOpAuthzDenied, AuditOpAuditExport, AuditOpAuditSettingsUpdated,
}

// AllAuditOperations returns every audit operation. The result is a fresh copy,
// so a caller may sort or modify it.
func AllAuditOperations() []AuditOperation {
	return append([]AuditOperation(nil), allAuditOperations...)
}

// IsValid reports whether o is one of AllAuditOperations.
func (o AuditOperation) IsValid() bool {
	for _, op := range allAuditOperations {
		if op == o {
			return true
		}
	}
	return false
}

// IsValid reports whether t is one of the AuditActor* values.
func (t AuditActorType) IsValid() bool {
	switch t {
	case AuditActorUser, AuditActorSystem, AuditActorAnonymous:
		return true
	}
	return false
}

// IsValid reports whether s is one of the AuditSource* values.
func (s AuditSource) IsValid() bool {
	switch s {
	case AuditSourceWeb, AuditSourceCLI, AuditSourceMCP, AuditSourceAPI, AuditSourceSystem:
		return true
	}
	return false
}

// IsValid reports whether o is one of the AuditOutcome* values.
func (o AuditOutcome) IsValid() bool {
	switch o {
	case AuditOutcomeSuccess, AuditOutcomeDenied, AuditOutcomeFailure:
		return true
	}
	return false
}

// Resource types of auth and security audit events. Resource writes use the
// resource's own type name (prompt, artifact, ...).
const (
	AuditResourceTypeUser        = "user"
	AuditResourceTypeAPIKey      = "api_key"
	AuditResourceTypeOAuthClient = "oauth_client"
	AuditResourceTypeTeam        = "team"
	AuditResourceTypeTeamMember  = "team_member"
	AuditResourceTypeInvitation  = "team_invitation"
)

// AuditEvent is one row of the instance audit log (audit_events).
//
// The ids carry no foreign key and the names are snapshots taken when the event
// was recorded, so a row stays readable after what it names is deleted.
type AuditEvent struct {
	ID          string
	CreatedAt   time.Time
	ActorUserID *string
	// ActorEmail is the actor's address at the time, or the attempted address
	// of a failed sign-in for an unknown account. Empty for a system actor.
	ActorEmail string
	ActorName  string
	ActorType  AuditActorType

	TeamID      *string
	TeamName    *string
	ProjectID   *string
	ProjectName *string

	Operation AuditOperation

	ResourceType *string
	ResourceID   *string
	ResourceName *string

	Source        AuditSource
	AssistantName *string
	APIKeyID      *string
	APIKeyName    *string

	Outcome AuditOutcome

	// IP and UserAgent are nil when the instance does not capture client info.
	IP        *string
	UserAgent *string
	RequestID *string

	// Metadata is a JSON object, never null: '{}' when there is nothing to add.
	Metadata json.RawMessage
}

// AuditCursor is the keyset position of an audit event in the newest-first
// list: (created_at, id), the list's sort key.
type AuditCursor struct {
	CreatedAt time.Time
	ID        string
}

// AuditEventFilter narrows an audit log listing. Every set field must match
// (AND); a slice matches any of its values (IN); a nil field or an empty slice
// does not filter.
type AuditEventFilter struct {
	ActorUserID  *string
	TeamID       *string
	ProjectID    *string
	ResourceType *string

	Operations []AuditOperation
	Sources    []AuditSource
	Outcomes   []AuditOutcome

	// From is inclusive and To exclusive.
	From *time.Time
	To   *time.Time

	// Query is a case-insensitive substring of resource_name, actor_email or
	// ip. LIKE wildcards in it match literally.
	Query *string

	// Limit is the page size. The repository clamps it to 1..500.
	Limit int

	// After returns only the rows strictly older than this cursor in the list
	// order. Nil starts at the newest row.
	After *AuditCursor
}
