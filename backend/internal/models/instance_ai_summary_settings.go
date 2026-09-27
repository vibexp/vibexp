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
