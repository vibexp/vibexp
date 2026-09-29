package models

import (
	"encoding/json"
	"time"
)

// Instance settings an audit entry can describe (epic #1185). The value is
// stored in instance_settings_audit.setting, which the database constrains only
// to be non-empty so that a new setting needs no migration; this list is the
// closed set.
const (
	// InstanceSettingEmailProvider is the instance's outbound email provider
	// (table instance_email_provider, #1186).
	InstanceSettingEmailProvider = "email_provider"
	// InstanceSettingSearch is the instance's search ranking defaults (table
	// instance_search_settings, #1197).
	InstanceSettingSearch = "search"
	// InstanceSettingAISummary is the instance's AI summary defaults and
	// budgets (table instance_ai_summary_settings, #1197).
	InstanceSettingAISummary = "ai_summary"
	// InstanceSettingAuthProviders is the instance's DB-managed sign-in
	// identity providers (table instance_auth_providers, #1231).
	InstanceSettingAuthProviders = "auth_providers"
	// InstanceSettingAuthAllowlist is the instance's sign-in access allowlist
	// (table instance_auth_allowlist, #1231).
	InstanceSettingAuthAllowlist = "auth_allowlist"
	// InstanceSettingInstanceAdmins is the set of DB-granted instance admins
	// (table instance_admins, #1231).
	InstanceSettingInstanceAdmins = "instance_admins"
	// InstanceSettingAuthSetup is the first-run authentication setup (epic
	// #1230). Declared with the other auth settings; written by #1236.
	InstanceSettingAuthSetup = "auth_setup"
)

// Actions an instance settings audit entry records. Unlike the setting, the
// action set is fixed, so the database enforces it with a CHECK constraint.
const (
	// InstanceSettingsAuditActionUpsert is a setting created or replaced by an
	// instance admin.
	InstanceSettingsAuditActionUpsert = "upsert"
	// InstanceSettingsAuditActionDelete is a setting removed by an instance
	// admin.
	InstanceSettingsAuditActionDelete = "delete"
	// InstanceSettingsAuditActionImport is the boot-time import from
	// config.yaml. It has no actor.
	InstanceSettingsAuditActionImport = "import"
)

// Values a redacted snapshot may carry in place of a credential. A snapshot
// records only whether the credential changed, never the credential itself.
const (
	InstanceSettingsAuditSecretChanged   = "changed"
	InstanceSettingsAuditSecretUnchanged = "unchanged"
)

// InstanceSettingsAuditEntry is one append-only entry in the instance settings
// log.
//
// ActorUserID is nil for the boot-time import, and becomes nil when the actor's
// account is deleted: the entry outlives it. Before is nil when the change
// created the setting, After is nil when it deleted it. Both are REDACTED
// snapshots and never hold a credential.
type InstanceSettingsAuditEntry struct {
	ID          string          `json:"id"            db:"id"`
	Setting     string          `json:"setting"       db:"setting"`
	Action      string          `json:"action"        db:"action"`
	ActorUserID *string         `json:"actor_user_id" db:"actor_user_id"`
	Before      json.RawMessage `json:"before"        db:"before"`
	After       json.RawMessage `json:"after"         db:"after"`
	CreatedAt   time.Time       `json:"created_at"    db:"created_at"`
}

// InstanceSettingsAuditCursor is the keyset position after the last entry of a
// page. The tuple is the log's full sort key, so paging is stable even when
// several entries share one created_at.
type InstanceSettingsAuditCursor struct {
	CreatedAt time.Time
	ID        string
}
