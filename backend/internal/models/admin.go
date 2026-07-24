package models

import "time"

// InstanceCounts holds instance-wide totals for the top-level entities, as
// surfaced by the admin stats endpoint (GET /api/v1/admin/stats). These are
// unscoped COUNT(*) totals across all users/teams, distinct from the
// user-scoped counts used elsewhere.
type InstanceCounts struct {
	Users     int64
	Teams     int64
	Prompts   int64
	Artifacts int64
	Memories  int64
}

// AdminUserListItem is one row of the instance-wide admin user listing
// (GET /api/v1/admin/users): identity fields plus the number of teams the user
// belongs to.
type AdminUserListItem struct {
	ID          string
	Email       string
	Name        string
	IDPProvider *string
	CreatedAt   time.Time
	TeamCount   int64
}

// AdminUserList is a page of the admin user listing plus pagination metadata.
type AdminUserList struct {
	Users      []AdminUserListItem
	TotalCount int
	Page       int
	PerPage    int
	TotalPages int
}

// AdminTeamMembership is one team a user belongs to, with the user's role in
// that team (returned as a plain column — no role predicate in SQL, per D3).
type AdminTeamMembership struct {
	TeamID   string
	TeamName string
	Role     string
}

// AdminUserDetail is the per-user admin view (GET /api/v1/admin/users/{id}):
// identity fields plus the user's team memberships.
type AdminUserDetail struct {
	ID          string
	Email       string
	Name        string
	IDPProvider *string
	CreatedAt   time.Time
	Memberships []AdminTeamMembership
}

// AdminTeamOwner is the owning user of a team, as shown in the admin team views.
type AdminTeamOwner struct {
	ID    string
	Email string
	Name  string
}

// AdminTeamListItem is one row of the instance-wide admin team listing
// (GET /api/v1/admin/teams): identity, owner, and member count.
type AdminTeamListItem struct {
	ID          string
	Name        string
	Slug        string
	IsPersonal  bool
	Owner       AdminTeamOwner
	MemberCount int64
	CreatedAt   time.Time
}

// AdminTeamList is a page of the admin team listing plus pagination metadata.
type AdminTeamList struct {
	Teams      []AdminTeamListItem
	TotalCount int
	Page       int
	PerPage    int
	TotalPages int
}

// AdminTeamMember is one member of a team, with the member's role and join time
// (role returned as a plain column — no role predicate in SQL, per D3).
type AdminTeamMember struct {
	UserID   string
	Email    string
	Name     string
	Role     string
	JoinedAt time.Time
}

// AdminTeamDetail is the per-team admin view (GET /api/v1/admin/teams/{id}):
// identity, owner, and the member list.
type AdminTeamDetail struct {
	ID         string
	Name       string
	Slug       string
	IsPersonal bool
	Owner      AdminTeamOwner
	CreatedAt  time.Time
	Members    []AdminTeamMember
}

// AdminExtendedCounts holds instance-wide totals for every top-level entity, as
// surfaced by the admin dashboard overview (GET /api/v1/admin/dashboard/overview).
// It is a superset of InstanceCounts, which stays as-is so the legacy
// /admin/stats endpoint keeps its shape.
type AdminExtendedCounts struct {
	Users      int64
	Teams      int64
	Projects   int64
	Prompts    int64
	Artifacts  int64
	Memories   int64
	Blueprints int64
	Agents     int64
	Feeds      int64
	APIKeys    int64
}

// AdminBreakdownBucket is one value of a grouped status/type column plus its
// row count. A NULL column value is reported as an empty string.
type AdminBreakdownBucket struct {
	Value string
	Count int64
}

// AdminEntityBreakdown is a GROUP BY over one status/type column of one entity
// table, most frequent value first.
type AdminEntityBreakdown struct {
	Entity  string
	Field   string
	Buckets []AdminBreakdownBucket
}

// AdminTableStat is an approximate row count for one table, taken from
// pg_stat_user_tables.n_live_tup rather than an exact COUNT(*).
type AdminTableStat struct {
	Table         string
	EstimatedRows int64
}

// AdminSystemHealth is the instance's storage health: total database size plus
// per-table estimated row counts.
type AdminSystemHealth struct {
	DatabaseSizeBytes int64
	Tables            []AdminTableStat
}

// AdminDashboardOverview is the full overview payload: totals, breakdowns,
// system health, and the running app version.
type AdminDashboardOverview struct {
	Counts       AdminExtendedCounts
	Breakdowns   []AdminEntityBreakdown
	SystemHealth AdminSystemHealth
	Version      string
}

// AdminGrowthPoint is the number of rows created per entity within one bucket.
type AdminGrowthPoint struct {
	Bucket    time.Time
	Users     int64
	Teams     int64
	Projects  int64
	Prompts   int64
	Artifacts int64
	Memories  int64
}

// AdminGrowthCount is one repository row of the growth aggregate: how many rows
// of one entity fall in one bucket. The service pivots these into
// AdminGrowthPoint and gap-fills the missing buckets.
type AdminGrowthCount struct {
	Entity string
	Bucket time.Time
	Count  int64
}

// AdminCountPoint is a single count within one bucket.
type AdminCountPoint struct {
	Bucket time.Time
	Count  int64
}

// AdminSourcePoint is a count for one access source within one bucket.
type AdminSourcePoint struct {
	Bucket time.Time
	Source string
	Count  int64
}

// AdminDataWindow states the earliest instant for which each TTL-pruned event
// source still holds data, so a client can tell "no activity" apart from
// "pruned".
type AdminDataWindow struct {
	SignInsEarliestRetainedAt        time.Time
	AccessBySourceEarliestRetainedAt time.Time
}

// AdminTimeseries is the full bucketed-metrics payload. Every series is
// gap-filled across the whole range — no sparse buckets reach the client.
type AdminTimeseries struct {
	From           time.Time
	To             time.Time
	Granularity    string
	Growth         []AdminGrowthPoint
	SignIns        []AdminCountPoint
	AccessBySource []AdminSourcePoint
	DataWindow     AdminDataWindow
}
