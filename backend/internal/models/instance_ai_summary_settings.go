package models

import "time"

// InstanceAISummarySettings is the instance's own AI summary defaults and
// budgets (#1197, epic #1196): the database-stored counterpart of config.yaml's
// `ai_summary:` block, editable by an instance admin without file access or a
// restart.
//
// Enabled, TopN, Style and MaxOutputTokens are the defaults a team may override
// (see TeamAISummarySettings). PerDocumentChars, TotalContextChars and
// RequestTimeout are instance-only budgets. There is deliberately no MaxTopN or
// output-token ceiling (epic decision 3): the hard limits are constants.
//
// The table is a singleton (primary key `id boolean CHECK (id)`), so there is no
// ID field: the row is only present or absent, and absent means the built-in
// defaults are in effect.
type InstanceAISummarySettings struct {
	Enabled           bool   `json:"enabled" db:"enabled"`
	TopN              int    `json:"top_n" db:"top_n"`
	Style             string `json:"style" db:"style"`
	MaxOutputTokens   int    `json:"max_output_tokens" db:"max_output_tokens"`
	PerDocumentChars  int    `json:"per_document_chars" db:"per_document_chars"`
	TotalContextChars int    `json:"total_context_chars" db:"total_context_chars"`
	// RequestTimeout is stored as request_timeout_ms; the repository converts,
	// truncating to whole milliseconds.
	RequestTimeout time.Duration `json:"request_timeout" db:"request_timeout_ms"`
	CreatedAt      time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at" db:"updated_at"`
	// UpdatedBy is the user who last saved the defaults; nil for the boot-time
	// config.yaml import, or once that user has been deleted.
	UpdatedBy *string `json:"updated_by,omitempty" db:"updated_by"`
	Version   int64   `json:"version" db:"version"`
}

// MaxAISummaryTopN is the hard limit on how many documents a single summary
// request may assemble, for every team and for the instance defaults alike. It
// is a code constant, not a setting (epic #1196, decision 3): the
// team_ai_summary_settings.top_n and instance_ai_summary_settings.top_n CHECK
// constraints (migrations 017 and 022) mirror it bound for bound, so a value
// the database would reject can never be saved. Change them together.
const MaxAISummaryTopN = 10

// MaxAISummaryOutputTokens is the hard limit on a summary's max_output_tokens,
// for every team and for the instance defaults alike. The
// instance_ai_summary_settings.max_output_tokens CHECK constraint (migration
// 022) pins it for the instance row; the team row is checked only for > 0
// (migration 017), so for a team the bound is enforced by the settings service
// on save and clamped again by the summary service at request time.
const MaxAISummaryOutputTokens = 32768

// Provenance of the instance AI summary settings.
const (
	// InstanceAISummarySettingsSourceInstance means an instance admin (or the
	// boot-time config import) stored the settings in
	// instance_ai_summary_settings.
	InstanceAISummarySettingsSourceInstance = "instance"
	// InstanceAISummarySettingsSourceDefault means no row is stored and the
	// built-in defaults are in effect.
	InstanceAISummarySettingsSourceDefault = "default"
)

// InstanceAISummarySettingsValues is the complete set of instance AI summary
// defaults and budgets, without the storage metadata carried by
// InstanceAISummarySettings.
type InstanceAISummarySettingsValues struct {
	Enabled           bool          `json:"enabled"`
	TopN              int           `json:"top_n"`
	Style             string        `json:"style"`
	MaxOutputTokens   int           `json:"max_output_tokens"`
	PerDocumentChars  int           `json:"per_document_chars"`
	TotalContextChars int           `json:"total_context_chars"`
	RequestTimeout    time.Duration `json:"request_timeout"`
}

// DefaultInstanceAISummarySettings returns the built-in instance AI summary
// defaults and budgets, in effect when no instance_ai_summary_settings row is
// stored. It is the single definition of those numbers: config.yaml's
// `ai_summary:` defaults are built from it until that block is removed (#1203).
func DefaultInstanceAISummarySettings() InstanceAISummarySettingsValues {
	return InstanceAISummarySettingsValues{
		Enabled:           true,
		TopN:              5,
		Style:             AISummaryStyleBalanced,
		MaxOutputTokens:   800,
		PerDocumentChars:  8000,
		TotalContextChars: 32000,
		RequestTimeout:    60 * time.Second,
	}
}

// TeamValues projects the instance defaults onto the team-overridable profile.
// ModelProviderID is always nil: the instance has no opinion on which of a
// team's model providers to use, so inheriting means "the team default".
func (v InstanceAISummarySettingsValues) TeamValues() TeamAISummarySettingsValues {
	return TeamAISummarySettingsValues{
		Enabled:         v.Enabled,
		ModelProviderID: nil,
		TopN:            v.TopN,
		Style:           v.Style,
		MaxOutputTokens: v.MaxOutputTokens,
	}
}

// InstanceAISummarySettingsView is the read model for the instance AI summary
// settings: the values in effect and where they came from.
type InstanceAISummarySettingsView struct {
	// Source is InstanceAISummarySettingsSourceInstance or
	// InstanceAISummarySettingsSourceDefault.
	Source string
	Values InstanceAISummarySettingsValues
	// UpdatedAt and UpdatedBy describe the stored row; both are nil when
	// Source is InstanceAISummarySettingsSourceDefault, and UpdatedBy is also
	// nil for the boot-time import or once the saving user has been deleted.
	UpdatedAt *time.Time
	UpdatedBy *string
	// Version is the stored row's optimistic-lock counter, nil when Source is
	// InstanceAISummarySettingsSourceDefault. Update's expectedVersion is compared with it.
	Version *int64
}
