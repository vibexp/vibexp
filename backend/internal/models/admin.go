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
	ID        string
	Name      string
	Owner     AdminTeamOwner
	CreatedAt time.Time
	Members   []AdminTeamMember
}
