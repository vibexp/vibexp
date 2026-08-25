package models

import (
	"encoding/json"
	"time"
)

// Settings surfaces a cross-team copy can target (epic #827). The value is
// stored in team_settings_audit.surface and validated in the service layer,
// mirroring how resource_freshness_audit validates action/reason there rather
// than with a CHECK constraint — a constraint would make adding a surface a
// migration.
const (
	// SettingsAuditSurfaceEmbeddingProvider is a copy of one embedding
	// provider configuration.
	SettingsAuditSurfaceEmbeddingProvider = "embedding_provider"
	// SettingsAuditSurfaceModelProvider is a copy of one LLM model provider
	// configuration.
	SettingsAuditSurfaceModelProvider = "model_provider"
	// SettingsAuditSurfaceCustomTypes is a copy of a team's custom artifact
	// types. One action copies the whole set, so CreatedResourceID is nil and
	// the individual ids live in Detail.
	SettingsAuditSurfaceCustomTypes = "custom_types"
)

// SettingsAuditSurfaces is the closed set of valid surface values, in the order
// the epic lists them.
var SettingsAuditSurfaces = []string{
	SettingsAuditSurfaceEmbeddingProvider,
	SettingsAuditSurfaceModelProvider,
	SettingsAuditSurfaceCustomTypes,
}

// TeamSettingsAudit is one append-only entry in a team's settings-copy log.
//
// TeamID is the DESTINATION team — the one that now owns the configuration and
// whose owners the entry is written for. SourceTeamID is where it came from and
// carries no foreign key, so the entry survives the source team being deleted;
// it is nil only when the source is genuinely unknown.
//
// Both resource ids are polymorphic across the provider and type tables and so
// carry no foreign key either. CreatedResourceID is nil when one action created
// several rows — a custom-types copy — and the ids are then in Detail.
type TeamSettingsAudit struct {
	ID                string          `json:"id"                  db:"id"`
	TeamID            string          `json:"team_id"             db:"team_id"`
	ActorUserID       *string         `json:"actor_user_id"       db:"actor_user_id"`
	Surface           string          `json:"surface"             db:"surface"`
	SourceTeamID      *string         `json:"source_team_id"      db:"source_team_id"`
	SourceResourceID  *string         `json:"source_resource_id"  db:"source_resource_id"`
	CreatedResourceID *string         `json:"created_resource_id" db:"created_resource_id"`
	Detail            json.RawMessage `json:"detail"              db:"detail"`
	CreatedAt         time.Time       `json:"created_at"          db:"created_at"`
}

// TeamSettingsAuditEntryView is one stored entry plus the two display names the
// read path (#832) resolves live.
//
// It wraps the stored row rather than widening it because the two names are not
// persisted state: everything else on the entry is a snapshot taken at write
// time, and putting a looked-up value on the same struct would invite a caller
// to believe the audit table holds it. Both names are nil when the referenced
// row no longer exists — a deleted actor or a deleted source team — which is
// the normal, non-error case the log is designed to survive.
type TeamSettingsAuditEntryView struct {
	Entry          *TeamSettingsAudit
	ActorName      *string
	SourceTeamName *string
}

// TeamSettingsAuditPage is one page of the settings audit log plus the total
// row count, mirroring FreshnessAuditPage.
type TeamSettingsAuditPage struct {
	Entries    []*TeamSettingsAuditEntryView
	TotalCount int
	Page       int
	PerPage    int
}
