package models

import "time"

// Schedule is one team's recurring-job schedule for the in-process scheduler
// (epic #725). One row per (TeamID, JobType): it records when the job is next
// due (NextRunAt) and the interval to advance by after each run. JobType is a
// plain string -- the valid set is owned by the scheduler job registry
// (#728), so registering a new job never needs a migration.
type Schedule struct {
	ID              string     `json:"id"               db:"id"`
	TeamID          string     `json:"team_id"          db:"team_id"`
	JobType         string     `json:"job_type"         db:"job_type"`
	IntervalSeconds int        `json:"interval_seconds" db:"interval_seconds"`
	LastRunAt       *time.Time `json:"last_run_at"      db:"last_run_at"`
	NextRunAt       time.Time  `json:"next_run_at"      db:"next_run_at"`
	CreatedAt       time.Time  `json:"created_at"       db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"       db:"updated_at"`
}
