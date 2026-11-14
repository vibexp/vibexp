package server

import (
	"context"
	"time"
)

// sessionSweepInterval is how often expired session rows are deleted. An
// expired row already authenticates nothing, so the sweep only keeps the table
// small and an hour is plenty.
const sessionSweepInterval = time.Hour

// sessionSweepTimeout bounds one sweep, so a stuck database cannot hold a
// connection until the next one.
const sessionSweepTimeout = time.Minute

// runSessionSweep deletes expired server-side session rows (#1646) once at
// start-up and then every interval, until ctx ends. It returns when ctx ends,
// so the goroutine Start runs it in stops with the server.
//
// Like runAccountLinkSweep it is an in-process, instance-wide loop and needs no
// lock across replicas: the sweep is one idempotent DELETE, and two replicas
// running it at once only race to delete the same rows.
func (s *Server) runSessionSweep(ctx context.Context, interval time.Duration) {
	store := s.sessionStore()
	if store == nil {
		return
	}
	sweep := func() {
		sweepCtx, cancel := context.WithTimeout(ctx, sessionSweepTimeout)
		defer cancel()
		removed, err := store.Sweep(sweepCtx)
		switch {
		case ctx.Err() != nil:
			// Shutting down; an interrupted sweep is not a fault.
		case err != nil:
			s.logger.With("error", err.Error()).Warn("Failed to sweep expired sessions")
		case removed > 0:
			s.logger.With("removed", removed).Info("Swept expired sessions")
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
