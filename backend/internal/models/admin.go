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
