package models

import "time"

// TeamAISummarySettings is a team's override of the instance AI summary
// defaults (the `ai_summary:` block of config.yaml).
//
// The override is whole-row, never per-field: a team either owns this complete
// profile or has no row at all and inherits every instance default. That is the
// same contract TeamSearchSettings carries, and for the same reason — blending a
// team's TopN with the instance's Style would make "what is in effect" depend on
// which columns happened to be written.
//
// There are deliberately no context-budget fields (per-document chars, total
// context chars, request timeout) and no MaxTopN: those size the work a single
// request may ask of the operator's model and hardware, so they stay
// instance-only and a team cannot raise them.
type TeamAISummarySettings struct {
	TeamID string `json:"team_id" db:"team_id"`
	// Enabled is an explicit on/off, independent of whether the team has a
	// usable model provider configured.
	Enabled bool `json:"enabled" db:"enabled"`
	// ModelProviderID selects one of the team's model providers. Empty means
	// "use the team's default provider" — which is also what the column becomes
	// when the referenced provider is deleted (ON DELETE SET NULL).
	ModelProviderID *string `json:"model_provider_id" db:"model_provider_id"`
	// TopN is how many documents are fed to the summariser. It is bounded above
	// by the instance's ai_summary.max_top_n.
	TopN            int       `json:"top_n" db:"top_n"`
	Style           string    `json:"style" db:"style"`
	MaxOutputTokens int       `json:"max_output_tokens" db:"max_output_tokens"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
	Version         int64     `json:"version" db:"version"`
}

// Provenance of the AI summary settings in effect for a team.
const (
	// TeamAISummarySettingsSourceInstance means the team has no override and
	// inherits the deployment defaults from config.yaml.
	TeamAISummarySettingsSourceInstance = "instance"
	// TeamAISummarySettingsSourceTeam means the team has stored its own profile.
	TeamAISummarySettingsSourceTeam = "team"
)

// The closed set of summary styles. It is mirrored by the CHECK constraint on
// team_ai_summary_settings.style and by config validation of
// ai_summary.style — adding a style means changing all three.
const (
	// AISummaryStyleConcise asks for the shortest useful answer.
	AISummaryStyleConcise = "concise"
	// AISummaryStyleBalanced is the default: a few sentences of substance.
	AISummaryStyleBalanced = "balanced"
	// AISummaryStyleDetailed asks for the fullest answer the token budget allows.
	AISummaryStyleDetailed = "detailed"
)

// AISummaryStyles lists every valid style, in increasing order of length. It is
// the single definition shared by config validation and the settings service,
// so the two can never disagree on what is acceptable.
var AISummaryStyles = []string{
	AISummaryStyleConcise,
	AISummaryStyleBalanced,
	AISummaryStyleDetailed,
}

// IsValidAISummaryStyle reports whether s is one of AISummaryStyles.
func IsValidAISummaryStyle(s string) bool {
	for _, style := range AISummaryStyles {
		if s == style {
			return true
		}
	}
	return false
}

// TeamAISummarySettingsValues is a complete summary profile, without the storage
// metadata (team id, timestamps, version) carried by TeamAISummarySettings. It
// is what clients send on an update and what the API echoes back.
type TeamAISummarySettingsValues struct {
	Enabled         bool    `json:"enabled"`
	ModelProviderID *string `json:"model_provider_id"`
	TopN            int     `json:"top_n"`
	Style           string  `json:"style"`
	MaxOutputTokens int     `json:"max_output_tokens"`
}

// TeamAISummarySettingsView is the read model for a team's AI summary settings:
// the effective values plus everything a client needs to render the whole
// settings surface from one response — where the values came from, the defaults
// a reset would restore, and the instance-owned cap on TopN.
type TeamAISummarySettingsView struct {
	// Source is TeamAISummarySettingsSourceInstance or
	// TeamAISummarySettingsSourceTeam.
	Source           string
	Values           TeamAISummarySettingsValues
	InstanceDefaults TeamAISummarySettingsValues
	// MaxTopN is instance-owned and never team-configurable; it bounds how much
	// context a single summary request may assemble. It is exposed so clients
	// can bound their own input control instead of guessing.
	MaxTopN int
}

// AISummaryAvailability tells a client whether an AI Summary can be generated
// for a team's search results (#1074). It rides on the REST search response so
// the SPA can decide whether to render the summary section without a second
// round-trip; the MCP and CLI search surfaces never carry it.
type AISummaryAvailability struct {
	// Available reports that the team has at least one model provider row.
	Available bool `json:"available"`
	// Enabled reports that the team has not turned the feature off.
	Enabled bool `json:"enabled"`
}
