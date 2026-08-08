// Package scheduler runs recurring per-team jobs in-process: a ticker loop
// claims due rows from the schedules table (#727) under a Postgres advisory
// lock and invokes the handler registered for each row's job_type.
//
// The scheduler owns timing only; features own logic and register handlers on
// the Registry at startup. Advisory locks make concurrent replicas safe (at
// most one runs a given schedule per due-time), per-job recover keeps a
// panicking handler from killing the loop, and Close drains an in-flight
// handler for graceful shutdown.
package scheduler

import (
	"context"
	"hash/fnv"
)

// Handler runs one due occurrence of a job for one team. The scheduler owns
// timing and claiming; the feature owns the logic. The context carries the
// per-job timeout; a returned error is logged and the schedule still advances
// (no hot-retry loop).
type Handler func(ctx context.Context, teamID string) error

// Registry maps job_type to its handler. Registration happens once at startup
// (wire time), so lookups during the run loop need no locking.
type Registry struct {
	handlers map[string]Handler
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]Handler)}
}

// Register associates jobType with handler, replacing any previous
// registration for that jobType. It is startup-only: calling it while the run
// loop is active is a data race.
func (r *Registry) Register(jobType string, handler Handler) {
	r.handlers[jobType] = handler
}

// Lookup returns the handler for jobType, or nil when none is registered.
func (r *Registry) Lookup(jobType string) Handler {
	return r.handlers[jobType]
}

// lockKey derives a stable Postgres advisory-lock key from the schedule ID.
// Distinct schedules get distinct locks so one team's due job never blocks
// another team's. FNV-1a is deterministic across processes and versions, which
// is the only requirement — the value is a lock key, not a checksum.
func lockKey(scheduleID string) int64 {
	h := fnv.New64a()
	// hash.Hash never returns an error from Write.
	_, _ = h.Write([]byte(scheduleID))
	return int64(h.Sum64()) // #nosec G115 -- intentional bit-preserving reinterpretation
}
