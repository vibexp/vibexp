package server

import (
	"context"
	"time"
)

// auditPurgeInterval is how often audit events past the retention are
// deleted. The retention is counted in weeks, so a daily run keeps the table
// within a day of it.
const auditPurgeInterval = 24 * time.Hour

// auditPurgeTimeout bounds one purge run, so a large backlog or a stuck
// database cannot hold the purge lock and its connection until the next run;
// whatever is left is deleted then.
const auditPurgeTimeout = 10 * time.Minute

// runAuditPurge enforces the audit-log retention (#1512): it purges once at
// start-up and then every interval, until ctx ends. It returns when ctx ends,
// so the goroutine startBackgroundWork runs it in stops with the server.
//
// Like runAccountLinkSweep it is an in-process loop, not a job of
// internal/scheduler, because that scheduler runs per-team jobs and this is
// instance-wide. Unlike the sweep it does need a lock across replicas, which
// AuditPurgeService.Purge takes: replicas started together would otherwise all
// scan and delete the same backlog.
func (s *Server) runAuditPurge(ctx context.Context, interval time.Duration) {
	purger := s.container.AuditPurgeService()
	if purger == nil {
		return
	}
	purge := func() {
		purgeCtx, cancel := context.WithTimeout(ctx, auditPurgeTimeout)
		defer cancel()
		deleted, skipped, err := purger.Purge(purgeCtx)
		switch {
		case ctx.Err() != nil:
			// Shutting down; an interrupted purge is not a fault.
		case err != nil:
			s.logger.With("error", err.Error(), "deleted", deleted).Warn("Failed to purge expired audit events")
		case skipped:
			s.logger.Debug("Audit purge skipped: another replica holds the purge lock")
		default:
			s.logger.With("deleted", deleted).Info("Purged expired audit events")
		}
	}

	purge()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			purge()
		}
	}
}
