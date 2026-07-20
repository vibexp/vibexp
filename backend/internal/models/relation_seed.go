package models

import "time"

// RelationSeedCandidate is one similar (entity, entity) pair produced by the
// embedding-similarity seed query (issue #426). The seed service types each
// candidate deterministically via RelationTypeMatrix and creates the resulting
// edge as origin=ai, status=suggested. UpdatedAt on both ends drives the
// same-type supersedes heuristic (newer supersedes older).
type RelationSeedCandidate struct {
	FromType      string    `db:"from_type"`
	FromID        string    `db:"from_id"`
	ToType        string    `db:"to_type"`
	ToID          string    `db:"to_id"`
	Distance      float64   `db:"distance"`
	FromUpdatedAt time.Time `db:"from_updated_at"`
	ToUpdatedAt   time.Time `db:"to_updated_at"`
}

// RelationSeedSummary is the run report for one team's seed backfill — logged and
// returned so a run is never silent.
type RelationSeedSummary struct {
	Candidates      int `json:"candidates"`
	Seeded          int `json:"seeded"`
	SkippedExisting int `json:"skipped_existing"`
	SkippedInvalid  int `json:"skipped_invalid"`
}
