package models

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
