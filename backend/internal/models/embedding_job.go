package models

import "time"

// Embedding job states. A job is created pending, becomes claimed while a
// worker holds its lease, and ends either done or failed. Only pending and
// claimed are "outstanding": the partial unique index that coalesces a
// re-enqueue and the claim query's partial index are both scoped to those two.
const (
	// EmbeddingJobStatePending is queued work no worker holds.
	EmbeddingJobStatePending = "pending"
	// EmbeddingJobStateClaimed is work leased by a worker. A lease that expires
	// (the worker's process died) makes the job claimable again -- that is the
	// restart-recovery path, not a separate boot-time sweep.
	EmbeddingJobStateClaimed = "claimed"
	// EmbeddingJobStateDone is work whose embeddings were generated and saved.
	EmbeddingJobStateDone = "done"
	// EmbeddingJobStateFailed is terminal failure: a provider rejection an
	// identical retry cannot fix, or a job that exhausted its attempt bound. A
	// failed job is never reclaimed (the poison-pill guard).
	EmbeddingJobStateFailed = "failed"
)

// EmbeddingJobPayload is the normalized embedding input persisted with a job.
//
// It is stored rather than referenced because a domain event cannot be rebuilt
// from an entity id: the live pipeline embeds the text the event carried, and
// the entity may have changed (or been deleted) since. The field names are the
// stored JSON contract -- a rename is a schema change for already-queued rows.
type EmbeddingJobPayload struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Body        string `json:"body"`
}

// EmbeddingJob is one unit of outstanding embedding work (issue #820): embed
// this entity, using this text. It is the durable counterpart of what used to
// live only in the dispatcher's in-memory FIFO.
//
// There is no TeamID: the team is resolved per job from the entity at run time
// and is not known when the job is enqueued.
type EmbeddingJob struct {
	ID         string              `json:"id"          db:"id"`
	EntityType string              `json:"entity_type" db:"entity_type"`
	EntityID   string              `json:"entity_id"   db:"entity_id"`
	UserID     string              `json:"user_id"     db:"user_id"`
	Payload    EmbeddingJobPayload `json:"payload"     db:"payload"`
	State      string              `json:"state"       db:"state"`
	// Attempts is incremented at CLAIM time, so a worker that dies mid-flight
	// still consumes an attempt and a poison pill is bounded.
	Attempts       int        `json:"attempts"         db:"attempts"`
	AvailableAt    time.Time  `json:"available_at"     db:"available_at"`
	ClaimedBy      *string    `json:"claimed_by"       db:"claimed_by"`
	ClaimedAt      *time.Time `json:"claimed_at"       db:"claimed_at"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at" db:"lease_expires_at"`
	LastError      *string    `json:"last_error"       db:"last_error"`
	CreatedAt      time.Time  `json:"created_at"       db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"       db:"updated_at"`
}
