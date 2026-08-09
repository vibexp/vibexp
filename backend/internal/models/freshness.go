package models

import "time"

// Freshness status values. A resource_freshness row exists only while the
// resource is stale, so today this set has exactly one member; it is a plain
// string column so widening it later needs no migration.
const (
	// FreshnessStatusStale marks a resource that no longer satisfies any of
	// the rules matching it.
	FreshnessStatusStale = "stale"
)

// Freshness audit actions -- what happened to a resource's freshness state.
const (
	// FreshnessActionMarked records a resource becoming stale.
	FreshnessActionMarked = "marked"
	// FreshnessActionCleared records a resource ceasing to be stale.
	FreshnessActionCleared = "cleared"
)

// Freshness audit reasons -- why a mark or clear happened.
const (
	// FreshnessReasonRuleRun attributes the change to a scheduled evaluation.
	FreshnessReasonRuleRun = "rule_run"
	// FreshnessReasonAccessed attributes a clear to the resource being read.
	FreshnessReasonAccessed = "accessed"
	// FreshnessReasonEdited attributes a clear to the resource being edited.
	FreshnessReasonEdited = "edited"
)

// ResourceFreshness is the system-owned staleness state of a single resource
// (epic #726). The row exists only while the resource is stale -- clearing
// staleness deletes it, and ResourceFreshnessAudit is what preserves the
// history.
//
// ResourceType/ResourceID are a polymorphic reference into prompts,
// artifacts, blueprints or memories with no database foreign key, mirroring
// comments (#272) and relations (#421). ProjectID is denormalized from the
// resource so listings can filter by project without joining four tables.
type ResourceFreshness struct {
	ID             string    `json:"id"               db:"id"`
	TeamID         string    `json:"team_id"          db:"team_id"`
	ProjectID      string    `json:"project_id"       db:"project_id"`
	ResourceType   string    `json:"resource_type"    db:"resource_type"`
	ResourceID     string    `json:"resource_id"      db:"resource_id"`
	Status         string    `json:"status"           db:"status"`
	MatchedRuleIDs []string  `json:"matched_rule_ids" db:"matched_rule_ids"`
	Since          time.Time `json:"since"            db:"since"`
	Reason         string    `json:"reason"           db:"reason"`
	CreatedAt      time.Time `json:"created_at"       db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"       db:"updated_at"`
}

// ResourceFreshnessFilters narrows a freshness listing. TeamID is required --
// it is the tenancy boundary. ResourceType and ProjectID are optional; an
// empty string means "any".
type ResourceFreshnessFilters struct {
	TeamID       string
	ResourceType string
	ProjectID    string
	Limit        int
	Offset       int
}

// FreshnessCandidateQuery selects the resources one rule currently considers
// stale. It is deliberately one resource type per query: the four resource
// types live in four tables, and a union across them would defeat the
// per-table indexes for no gain -- the evaluator fans out over a rule's types
// anyway.
//
// ProjectID nil means "any project in the team" and an empty Mediums means
// "any medium", exactly as on FreshnessRule. Limit plus AfterID is keyset
// pagination over the resource id, so a rule matching an unbounded number of
// resources is read in bounded batches.
type FreshnessCandidateQuery struct {
	TeamID        string
	ResourceType  string
	ProjectID     *string
	Mediums       []string
	ThresholdDays int
	Limit         int
	// AfterID returns only resources with a greater id; empty starts at the
	// beginning.
	AfterID string
}

// FreshnessCandidate is one resource a rule matched. ProjectID comes from the
// resource row so the evaluator can denormalize it onto resource_freshness
// without a second lookup.
type FreshnessCandidate struct {
	ResourceType string
	ResourceID   string
	ProjectID    string
}

// FreshnessRule is one team's staleness policy: the resources it covers and
// how long they may go unaccessed before being marked stale.
//
// ProjectID is nil for "any project in the team". An EMPTY Mediums means "any
// medium" rather than "no medium" -- the distinction matters, so callers must
// not treat the zero value as an exclusion. The membership of ResourceTypes
// and Mediums is validated in the service layer (#731), not by the schema, so
// adding a resource type later is not a migration.
type FreshnessRule struct {
	ID            string    `json:"id"             db:"id"`
	TeamID        string    `json:"team_id"        db:"team_id"`
	ProjectID     *string   `json:"project_id"     db:"project_id"`
	ResourceTypes []string  `json:"resource_types" db:"resource_types"`
	Mediums       []string  `json:"mediums"        db:"mediums"`
	ThresholdDays int       `json:"threshold_days" db:"threshold_days"`
	Enabled       bool      `json:"enabled"        db:"enabled"`
	CreatedAt     time.Time `json:"created_at"     db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"     db:"updated_at"`
}

// TeamFreshnessSettings is a team's freshness evaluation configuration, one
// row per team (mirroring TeamSearchSettings). An ABSENT row means the team
// inherits the defaults, which is what makes deleting the row the "reset to
// defaults" path.
//
// IntervalSeconds carries a storage-enforced floor of one hour, matching the
// scheduler's own floor (#727).
type TeamFreshnessSettings struct {
	TeamID               string    `json:"team_id"               db:"team_id"`
	IntervalSeconds      int       `json:"interval_seconds"      db:"interval_seconds"`
	ReversibilityEnabled bool      `json:"reversibility_enabled" db:"reversibility_enabled"`
	CreatedAt            time.Time `json:"created_at"            db:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"            db:"updated_at"`
	Version              int64     `json:"version"               db:"version"`
}

// Freshness settings defaults, applied when a team has no settings row.
const (
	// DefaultFreshnessIntervalSeconds evaluates a team's rules once a day.
	DefaultFreshnessIntervalSeconds = 86400
	// MinFreshnessIntervalSeconds is the one-hour floor the schema enforces.
	MinFreshnessIntervalSeconds = 3600
	// DefaultFreshnessReversibilityEnabled clears stale state when a resource
	// is accessed or edited (decision #6).
	DefaultFreshnessReversibilityEnabled = true
)

// DefaultTeamFreshnessSettings returns the settings a team without a stored
// row inherits. TeamID is set so the value can be returned to a caller as-is.
func DefaultTeamFreshnessSettings(teamID string) *TeamFreshnessSettings {
	return &TeamFreshnessSettings{
		TeamID:               teamID,
		IntervalSeconds:      DefaultFreshnessIntervalSeconds,
		ReversibilityEnabled: DefaultFreshnessReversibilityEnabled,
	}
}

// Provenance of the freshness settings a team is operating under.
const (
	// FreshnessSettingsSourceInstance means the team has no stored settings
	// and is inheriting the defaults.
	FreshnessSettingsSourceInstance = "instance"
	// FreshnessSettingsSourceTeam means the team stores its own settings.
	FreshnessSettingsSourceTeam = "team"
)

// FreshnessSettingsValues is the tunable part of a team's freshness settings —
// exactly what a client sends on an update and what the API echoes back.
// Keeping it separate from TeamFreshnessSettings is what lets one response
// carry both the effective values and the defaults a reset would restore.
type FreshnessSettingsValues struct {
	IntervalSeconds      int  `json:"interval_seconds"`
	ReversibilityEnabled bool `json:"reversibility_enabled"`
}

// TeamFreshnessSettingsView is the read model for a team's freshness settings:
// the effective values, where they came from, and the defaults a reset would
// restore — everything a client needs to render the settings card, and to
// preview a reset, from a single response. Mirrors TeamSearchSettingsView.
type TeamFreshnessSettingsView struct {
	// Source is FreshnessSettingsSourceInstance or FreshnessSettingsSourceTeam.
	Source   string
	Values   FreshnessSettingsValues
	Defaults FreshnessSettingsValues
}

// DefaultFreshnessSettingsValues returns the values a team inherits when it has
// stored none.
func DefaultFreshnessSettingsValues() FreshnessSettingsValues {
	return FreshnessSettingsValues{
		IntervalSeconds:      DefaultFreshnessIntervalSeconds,
		ReversibilityEnabled: DefaultFreshnessReversibilityEnabled,
	}
}

// ResourceFreshnessAudit is one append-only entry in the freshness mark/clear
// log. RuleID is nil when the change is not attributable to a rule (a clear
// caused by an access or an edit).
type ResourceFreshnessAudit struct {
	ID           string    `json:"id"            db:"id"`
	TeamID       string    `json:"team_id"       db:"team_id"`
	ResourceType string    `json:"resource_type" db:"resource_type"`
	ResourceID   string    `json:"resource_id"   db:"resource_id"`
	RuleID       *string   `json:"rule_id"       db:"rule_id"`
	Action       string    `json:"action"        db:"action"`
	Reason       string    `json:"reason"        db:"reason"`
	CreatedAt    time.Time `json:"created_at"    db:"created_at"`
}

// FreshnessBucketCount is one row of a grouped count over resource_freshness:
// a bucket key (a resource type, a project id or a rule id) and how many stale
// resources fall in it.
//
// Grouped counts are returned SPARSE — a bucket with nothing stale is simply
// absent — because the database cannot know the full key set (every resource
// type, every project, every rule) without a join that would fan out. The
// service fills the missing buckets with zero, which is where the full key set
// is actually known. That split follows the existing analytics code, which
// zero-fills its daily series in Go rather than in SQL.
type FreshnessBucketCount struct {
	Key   string
	Count int
}

// FreshnessTransitionCount is one row of the audit log grouped by day and
// action: how many resources were marked or cleared on a given UTC day. Also
// sparse — days with no activity are absent.
type FreshnessTransitionCount struct {
	// Date is the UTC day, formatted YYYY-MM-DD to match the zero-filled
	// series keys the handler builds.
	Date   string
	Action string
	Count  int
}

// FreshnessDailyStale is one day of the over-time series: the flows (how many
// resources changed state) and the level (how many were stale at the end of
// the day).
type FreshnessDailyStale struct {
	Date       string
	Marked     int
	Cleared    int
	StaleTotal int
}

// FreshnessOverTimeMetrics is the over-time chart's data, zero-filled: Days
// holds one entry per day in the window, oldest first, with no gaps.
type FreshnessOverTimeMetrics struct {
	RangeDays    int
	TotalMarked  int
	TotalCleared int
	Days         []FreshnessDailyStale
}

// FreshnessTypeMetrics is the by-type breakdown. Counts always covers all four
// resource types so the chart's bars never move.
type FreshnessTypeMetrics struct {
	TotalStale int
	Counts     []FreshnessBucketCount
}

// FreshnessProjectMetrics is the by-project breakdown. Counts covers every
// project in the team, including those with nothing stale.
type FreshnessProjectMetrics struct {
	TotalStale int
	Counts     []FreshnessProjectStale
}

// FreshnessProjectStale is one project's stale count, carrying the name and
// slug so a client can label and deep-link the bar without a second request.
type FreshnessProjectStale struct {
	ProjectID string
	Name      string
	Slug      string
	Count     int
}

// FreshnessRuleMetrics is the per-rule impact breakdown.
//
// TotalStale counts DISTINCT stale resources and is deliberately not the sum
// of the per-rule counts: staleness is a union across rules (decision #5), so
// a resource matched by two rules appears under both.
type FreshnessRuleMetrics struct {
	TotalStale int
	Counts     []FreshnessRuleImpact
}

// FreshnessRuleImpact is one rule and how many resources it currently marks.
// Rules have no name, so the defining fields travel with the count.
type FreshnessRuleImpact struct {
	RuleID        string
	ProjectID     *string
	ResourceTypes []string
	ThresholdDays int
	Enabled       bool
	Count         int
}

// FreshnessAuditPage is one page of the audit log plus the total row count.
type FreshnessAuditPage struct {
	Entries    []*ResourceFreshnessAudit
	TotalCount int
	Page       int
	PerPage    int
}
