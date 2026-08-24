package scheduler

import (
	"context"
	"fmt"
	"time"
)

// ReconcileInterval is how often the reconcile loop runs. It is a constant
// rather than a config knob on purpose: this is a REPAIR path, not a feature
// cadence, so there is nothing an operator gains by tuning it, and an hour is
// already far faster than the failure it repairs (which was previously
// permanent). It is also the bound the acceptance criterion names -- an
// affected team acquires its schedule within one hour of the instance
// starting, with no user write.
//
// It is deliberately much longer than Config.TickInterval (1m by default):
// the sweep is one anti-join returning zero rows on a healthy instance, and
// running it per tick would buy nothing.
const ReconcileInterval = time.Hour

// reconcileLockID is the advisory-lock key the reconcile sweep claims. Unlike
// runDue's per-schedule keys it is a single fixed key for the whole sweep,
// because the sweep is instance-wide rather than per-team: two replicas
// sweeping at once would both see the same missing rows and both upsert them.
// The upsert is idempotent so that would be harmless rather than corrupting,
// but there is no reason to pay for it twice.
//
// It is derived through lockKey like every other key here, so it shares the
// same 64-bit space as the per-schedule locks. Collision with a schedule id's
// key is possible in principle and harmless in practice: the worst outcome is
// one sweep and one schedule run serializing against each other for the
// duration of one of them.
var reconcileLockID = lockKey("scheduler:reconcile-schedules")

// Reconciler repairs a feature's own `schedules` rows. The scheduler engine
// owns no writer -- a feature provisions its own row -- so a feature whose
// provisioning is best-effort needs a way back from a failed write. It
// registers a Reconciler and the engine calls it on a timer, knowing nothing
// about the feature.
//
// The contract is: sweep whatever rows are MISSING, be idempotent, and do not
// touch rows that already exist (re-arming a healthy schedule's next_run_at is
// #767's bug). An implementation should log and continue past one team's
// failure, returning an error only when the sweep as a whole could not run.
//
// One implementation exists today (FreshnessService.ReconcileSchedules). The
// interface is what makes a second one a registration rather than a rewrite,
// which is as much of "a generalised schedule reconciler" as is worth building
// before a second job needs it (#768, option 3).
type Reconciler interface {
	ReconcileSchedules(ctx context.Context) error
}

// runReconcile is the reconcile loop, a second and much slower ticker beside
// run. The first sweep runs immediately, so restarting an instance also
// repairs it rather than waiting out a full interval.
func (s *Scheduler) runReconcile(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(ReconcileInterval)
	defer ticker.Stop()

	s.reconcile(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reconcile(ctx)
		}
	}
}

// reconcile runs every registered Reconciler once, under a single instance-wide
// advisory lock so concurrent replicas do not duplicate the sweep. Like
// runDue, the lock is held on a PINNED connection for the whole sweep --
// acquiring and releasing through the pool could land on different sessions
// and silently leak the lock -- and the unlock uses context.Background() so a
// sweep cancelled by shutdown still releases it.
//
// A reconciler that fails is logged and the next one still runs: they are
// independent features, and one being broken must not stop another healing.
func (s *Scheduler) reconcile(ctx context.Context) {
	if len(s.reconcilers) == 0 {
		return
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		// Transient: the next sweep tries again.
		s.logger.Error("scheduler: failed to acquire reconcile lock connection", "error", err)
		return
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			s.logger.Error("scheduler: failed to close reconcile lock connection", "error", cerr)
		}
	}()

	var locked bool
	if err := conn.QueryRowContext(
		ctx, `SELECT pg_try_advisory_lock($1)`, reconcileLockID,
	).Scan(&locked); err != nil {
		s.logger.Error("scheduler: failed to acquire reconcile advisory lock", "error", err)
		return
	}
	if !locked {
		// Another replica is sweeping; nothing to do.
		s.logger.Debug("scheduler: reconcile sweep already claimed by another replica, skipping")
		return
	}
	defer func() {
		// Background context: the unlock must still run when the loop context
		// was cancelled (graceful shutdown mid-sweep).
		if _, err := conn.ExecContext(
			context.Background(), `SELECT pg_advisory_unlock($1)`, reconcileLockID,
		); err != nil {
			s.logger.Error("scheduler: failed to release reconcile advisory lock", "error", err)
		}
	}()

	for _, r := range s.reconcilers {
		s.runReconciler(ctx, r)
	}
}

// runReconciler invokes one Reconciler under the per-job timeout, recovering a
// panic so a misbehaving reconciler is logged but never kills the loop --
// the same contract runHandler gives a job handler.
func (s *Scheduler) runReconciler(ctx context.Context, r Reconciler) {
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Error("scheduler: reconciler panicked", "panic", fmt.Sprint(rec))
		}
	}()

	jobCtx, cancel := context.WithTimeout(ctx, s.cfg.JobTimeout)
	defer cancel()

	if err := r.ReconcileSchedules(jobCtx); err != nil {
		s.logger.Error("scheduler: reconciler failed", "error", err)
	}
}
