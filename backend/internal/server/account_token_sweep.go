package server

import (
	"context"
	"time"
)

// accountLinkSweepInterval is how often consumed and expired one-time account
// links are deleted. A dead link already opens nothing, so the sweep only
// keeps the table small and an hour is plenty.
const accountLinkSweepInterval = time.Hour

// accountLinkSweepTimeout bounds one sweep, so a stuck database cannot hold a
// connection until the next one.
const accountLinkSweepTimeout = time.Minute

// runAccountLinkSweep deletes dead one-time account links (#1432) once at
// start-up and then every interval, until ctx ends. It returns when ctx ends,
// so the goroutine Start runs it in stops with the server.
//
// It is an in-process loop and not a job of internal/scheduler because that
// scheduler runs per-team jobs and this is instance-wide. It needs no lock
// across replicas: the sweep is one idempotent DELETE, and two replicas running
// it at once only race to delete the same rows.
func (s *Server) runAccountLinkSweep(ctx context.Context, interval time.Duration) {
	links := s.container.AccountTokenService()
	if links == nil {
		return
	}
	sweep := func() {
		sweepCtx, cancel := context.WithTimeout(ctx, accountLinkSweepTimeout)
		defer cancel()
		removed, err := links.Sweep(sweepCtx)
		switch {
		case ctx.Err() != nil:
			// Shutting down; an interrupted sweep is not a fault.
		case err != nil:
			s.logger.With("error", err.Error()).Warn("Failed to sweep dead account links")
		case removed > 0:
			s.logger.With("removed", removed).Info("Swept dead account links")
		}
	}

	sweep()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}
