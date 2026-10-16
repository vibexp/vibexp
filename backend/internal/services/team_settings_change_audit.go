package services

import (
	"context"
	"reflect"
	"slices"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// Team settings changes in the instance audit log (#1520). Every team-scoped
// settings write records one team.settings.updated event after it commits,
// naming the surface, the action and the changed fields.
//
// This is not the per-team settings history (TeamSettingsAuditService, epic
// #827), which a team admin reads on the team's own settings pages and which
// some surfaces write in the same transaction as the change. The instance log
// is the instance admin's single trail across every team, so it records every
// surface, best-effort and after the commit, and leaves that history alone.

// The settings surfaces, as the `surface` metadata value. The custom types and
// statuses names are shared with the per-team history.
const (
	teamSettingsSurfaceSearch            = "search"
	teamSettingsSurfaceAISummary         = "ai_summary"
	teamSettingsSurfaceEmailProvider     = "email_provider"
	teamSettingsSurfaceModelProvider     = models.SettingsAuditSurfaceModelProvider
	teamSettingsSurfaceEmbeddingProvider = models.SettingsAuditSurfaceEmbeddingProvider
	teamSettingsSurfaceFreshness         = "freshness"
	teamSettingsSurfaceGitHubApp         = "github_app"
	teamSettingsSurfaceCustomTypes       = models.SettingsAuditSurfaceCustomTypes
	teamSettingsSurfaceCustomStatuses    = models.SettingsAuditSurfaceCustomStatuses
)

// teamSettingsSurfaceLabels name each surface as the event's resource name.
var teamSettingsSurfaceLabels = map[string]string{
	teamSettingsSurfaceSearch:            "Search settings",
	teamSettingsSurfaceAISummary:         "AI summary settings",
	teamSettingsSurfaceEmailProvider:     "Email provider",
	teamSettingsSurfaceModelProvider:     "Model provider",
	teamSettingsSurfaceEmbeddingProvider: "Embedding provider",
	teamSettingsSurfaceFreshness:         "Freshness",
	teamSettingsSurfaceGitHubApp:         "GitHub App",
	teamSettingsSurfaceCustomTypes:       "Custom types",
	teamSettingsSurfaceCustomStatuses:    "Custom statuses",
}

// The `action` metadata values. Custom statuses keep the per-team history's
// own vocabulary (created / renamed / deleted).
const (
	teamSettingsActionCreate     = "create"
	teamSettingsActionUpdate     = "update"
	teamSettingsActionDelete     = "delete"
	teamSettingsActionReset      = "reset"
	teamSettingsActionCopy       = "copy"
	teamSettingsActionConnect    = "connect"
	teamSettingsActionDisconnect = "disconnect"
	teamSettingsActionRotate     = "rotate"
)

// Metadata keys of a team.settings.updated event.
const (
	teamSettingsMetaSurface       = "surface"
	teamSettingsMetaAction        = "action"
	teamSettingsMetaChangedFields = "changed_fields"
)

// teamSettingsChange describes one team settings write.
type teamSettingsChange struct {
	TeamID string
	// ActorID is the user the service acted for; empty falls back to the
	// actor the recorder reads from the context.
	ActorID string
	Surface string
	Action  string
	// Before and After hold the surface's non-secret scalar fields, built by
	// the service from an explicit allowlist (never a whole struct). A nil
	// side does not exist: nothing stored yet, or deleted.
	Before, After map[string]any
	// Withheld names fields the write set or changed whose values are never
	// recorded: secrets (API keys, passwords, private keys) and free-form
	// blobs. They are listed in changed_fields by name only, without a value,
	// hash or length.
	Withheld []string
	// Details are extra identifiers, such as the rule id or the source team
	// of a copy.
	Details map[string]any
}

// teamSettingsAuditor is embedded by every team settings service. Its zero
// value records nothing, so a service built without the option, as in most
// tests, behaves as before.
type teamSettingsAuditor struct {
	recorder audit.Recorder
}

// enabled reports whether changes are recorded, so a service skips the extra
// read of the previous values when nobody will see them.
func (a teamSettingsAuditor) enabled() bool {
	return a.recorder != nil
}

// record records change as team.settings.updated. Call it only after the
// write has committed; it never fails the caller.
func (a teamSettingsAuditor) record(ctx context.Context, change teamSettingsChange) {
	if a.recorder == nil {
		return
	}
	audit.SafeRecord(ctx, a.recorder, audit.Event{
		Operation:    models.AuditOpTeamSettingsUpdated,
		Actor:        auditActorID(change.ActorID),
		TeamID:       change.TeamID,
		ResourceType: models.AuditResourceTypeTeamSettings,
		ResourceName: teamSettingsSurfaceLabels[change.Surface],
		Metadata:     teamSettingsChangeMetadata(change),
	})
}

// teamSettingsChangeMetadata builds {surface, action, changed_fields,
// <field>: {old, new}, <detail>: value}. A field is changed when its value
// differs between the sides, or when only one side exists. Only scalar values
// are written out; anything else is named in changed_fields alone.
func teamSettingsChangeMetadata(change teamSettingsChange) map[string]any {
	metadata := map[string]any{
		teamSettingsMetaSurface: change.Surface,
		teamSettingsMetaAction:  change.Action,
	}
	for key, value := range change.Details {
		metadata[key] = value
	}

	changed := changedSettingsFields(change.Before, change.After)
	for _, field := range changed {
		if _, taken := metadata[field]; taken {
			continue
		}
		if pair := settingsValuePair(change.Before, change.After, field); pair != nil {
			metadata[field] = pair
		}
	}
	for _, field := range change.Withheld {
		if !slices.Contains(changed, field) {
			changed = append(changed, field)
		}
	}
	metadata[teamSettingsMetaChangedFields] = changed
	return metadata
}

// changedSettingsFields lists, sorted, the fields whose values differ between
// before and after, or that exist on one side only.
func changedSettingsFields(before, after map[string]any) []string {
	changed := []string{}
	if before == nil && after == nil {
		return changed
	}
	for field, value := range after {
		old, ok := before[field]
		if before == nil || !ok || !reflect.DeepEqual(old, value) {
			changed = append(changed, field)
		}
	}
	for field := range before {
		if _, ok := after[field]; after == nil || !ok {
			changed = append(changed, field)
		}
	}
	slices.Sort(changed)
	return changed
}

// settingsValuePair is {old, new} for field, leaving out a side that does not
// exist. It is nil when a value present is not a scalar.
func settingsValuePair(before, after map[string]any, field string) map[string]any {
	pair := map[string]any{}
	for side, values := range map[string]map[string]any{"old": before, "new": after} {
		value, ok := values[field]
		if !ok {
			continue
		}
		scalar, ok := auditScalar(value)
		if !ok {
			return nil
		}
		pair[side] = scalar
	}
	return pair
}

// providerWrite describes a model or embedding provider write. The API key
// and the free-form configuration are reported as set or changed, never with
// their values.
type providerWrite struct {
	TeamID, ActorID, Surface, Action, ProviderID string
	Before, After                                map[string]any
	APIKeySet, ConfigurationSet                  bool
	// Details are extra identifiers, such as the source of a copy.
	Details map[string]any
}

// providerChange builds the change for a provider write.
func providerChange(w providerWrite) teamSettingsChange {
	details := map[string]any{"provider_id": w.ProviderID}
	for key, value := range w.Details {
		details[key] = value
	}
	change := teamSettingsChange{
		TeamID: w.TeamID, ActorID: w.ActorID, Surface: w.Surface, Action: w.Action,
		Before: w.Before, After: w.After, Details: details,
	}
	if w.APIKeySet {
		change.Withheld = append(change.Withheld, "api_key")
	}
	if w.ConfigurationSet {
		change.Withheld = append(change.Withheld, "configuration")
	}
	return change
}

// providerCopyChange describes a cross-team provider copy (#828/#831): the
// created provider's values, with the API key and configuration named only.
func providerCopyChange(
	teamID, actorID, sourceTeamID, surface, sourceProviderID, providerID string,
	after map[string]any, apiKeyEncrypted *string, configuration string,
) teamSettingsChange {
	return providerChange(providerWrite{
		TeamID: teamID, ActorID: actorID, Surface: surface, Action: teamSettingsActionCopy,
		ProviderID: providerID, After: after,
		APIKeySet:        nonEmpty(apiKeyEncrypted),
		ConfigurationSet: configurationSet(configuration),
		Details:          map[string]any{"source_team_id": sourceTeamID, "source_provider_id": sourceProviderID},
	})
}

// nonEmpty reports whether p holds a non-empty string.
func nonEmpty(p *string) bool {
	return p != nil && *p != ""
}

// configurationSet reports whether a provider's stored configuration holds
// anything beyond the empty default.
func configurationSet(configuration string) bool {
	return configuration != "" && configuration != "{}"
}

// optionalAuditString is *p, or nil for an unset value.
func optionalAuditString(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

// auditScalar returns v when it is a scalar (a string, bool or number, or
// nil), dereferencing a pointer to one first.
func auditScalar(v any) (any, bool) {
	if v == nil {
		return nil, true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, true
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return rv.Interface(), true
	default:
		return nil, false
	}
}
