package models

import "time"

// Bounds and built-in defaults of the instance audit-log settings (#1512, epic
// #1510). The retention bounds live here, in the leaf models package, because
// they are shared by the service validator
// (services.ValidateInstanceAuditSettings) and the instance_audit_settings
// CHECK constraint (migration 023_consolidated). Change them together.
const (
	// MinAuditRetentionWeeks is the shortest retention an admin can set.
	MinAuditRetentionWeeks = 1
	// MaxAuditRetentionWeeks is the longest retention an admin can set: one
	// year.
	MaxAuditRetentionWeeks = 52
	// DefaultAuditRetentionWeeks is the retention with nothing stored.
	DefaultAuditRetentionWeeks = 2
	// DefaultAuditCaptureClientInfo is whether events store the client IP
	// address and user agent with nothing stored.
	DefaultAuditCaptureClientInfo = true
)

// InstanceAuditSettings is the instance's audit-log settings as stored.
//
// The table is a singleton (primary key `id boolean CHECK (id)`), so there is
// no ID field: the row is only present or absent.
type InstanceAuditSettings struct {
	RetentionWeeks int       `json:"retention_weeks" db:"retention_weeks"`
	CaptureIPUA    bool      `json:"capture_ip_ua" db:"capture_ip_ua"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
	// UpdatedBy is the user who last saved the settings; nil once that user
	// has been deleted.
	UpdatedBy *string `json:"updated_by,omitempty" db:"updated_by"`
	Version   int64   `json:"version" db:"version"`
}

// InstanceAuditSettingsValues is the complete set of audit-log settings,
// without the storage metadata carried by InstanceAuditSettings. Its JSON shape
// is also the instance_settings_audit snapshot.
type InstanceAuditSettingsValues struct {
	// RetentionWeeks is how many weeks an audit event is kept.
	RetentionWeeks int `json:"retention_weeks"`
	// CaptureIPUA is whether a newly recorded event stores the client IP
	// address and user agent.
	CaptureIPUA bool `json:"capture_ip_ua"`
}

// DefaultInstanceAuditSettingsValues returns the settings that apply when
// nothing is stored.
func DefaultInstanceAuditSettingsValues() InstanceAuditSettingsValues {
	return InstanceAuditSettingsValues{
		RetentionWeeks: DefaultAuditRetentionWeeks,
		CaptureIPUA:    DefaultAuditCaptureClientInfo,
	}
}

// RetentionWindow is how long an audit event is kept: RetentionWeeks whole
// weeks.
func (v InstanceAuditSettingsValues) RetentionWindow() time.Duration {
	return time.Duration(v.RetentionWeeks) * 7 * 24 * time.Hour
}

// InstanceAuditSettingsView is the read model for the audit-log settings: the
// values in effect plus the stored row's metadata.
type InstanceAuditSettingsView struct {
	Values InstanceAuditSettingsValues
	// IsDefault reports that nothing is stored and Values are the built-in
	// defaults.
	IsDefault bool
	// UpdatedAt, UpdatedBy and Version describe the stored row; all three are
	// nil when nothing is stored. UpdatedBy is also nil once the saving user
	// has been deleted. Version is the optimistic-lock counter Update's
	// expectedVersion is compared with.
	UpdatedAt *time.Time
	UpdatedBy *string
	Version   *int64
}
