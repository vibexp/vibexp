package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// MinInterval is the floor for a schedule's interval. It mirrors the database
// CHECK constraint (interval_seconds >= 3600) so a caller bug is caught in
// code before the write reaches the database.
const MinInterval = time.Hour

// defaultTickInterval is the polling cadence run falls back to when the
// configured TickInterval is non-positive. It matches the code default in
// config's defaults() ("scheduler.tick_interval: 1m") so the backstop and the
// documented default never disagree.
const defaultTickInterval = time.Minute

// ValidateInterval returns an error when interval is below MinInterval.
// Schedule writes should validate through this before hitting the repository,
// where the DB CHECK constraint is the backstop.
func ValidateInterval(interval time.Duration) error {
	if interval < MinInterval {
		return fmt.Errorf("schedule interval %s is below the minimum %s", interval, MinInterval)
	}
	return nil
}

// Config tunes the run loop.
type Config struct {
	// TickInterval is how often the loop looks for due schedules. The 1-hour
	// job floor keeps work sparse, so this is a polling cadence, not a job
	// cadence.
	TickInterval time.Duration
	// JobTimeout bounds a single handler invocation.
	JobTimeout time.Duration
	// DueLimit caps how many due schedules one tick claims.
	DueLimit int
}

// Scheduler is the in-process scheduler engine. Start launches the run loop;
// Close stops it and waits for an in-flight handler.
type Scheduler struct {
	repo     repositories.ScheduleRepository
	db       *database.DB
	registry *Registry
	logger   *slog.Logger
	cfg      Config
	// reconcilers repair missing schedule rows on a slow timer; see
	// reconcile.go. Empty is valid and disables the reconcile loop entirely.
	reconcilers []Reconciler

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a Scheduler. Call Start to launch the run loop.
//
// reconcilers is variadic and may be empty: the run loop does not depend on
// it, and a Scheduler constructed with none simply never sweeps.
func New(
	repo repositories.ScheduleRepository,
	db *database.DB,
	registry *Registry,
	cfg Config,
	logger *slog.Logger,
	reconcilers ...Reconciler,
) *Scheduler {
	return &Scheduler{
		repo:        repo,
		db:          db,
		registry:    registry,
		cfg:         cfg,
		logger:      logger,
		reconcilers: reconcilers,
	}
}

// Start launches the run loop in the background, plus the reconcile loop when
// any Reconciler is registered. The first tick and the first sweep both run
// immediately, so neither due schedules nor a repairable instance wait out a
// full interval after boot.
func (s *Scheduler) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.wg.Add(1)
	go s.run(ctx)

	if len(s.reconcilers) > 0 {
		s.wg.Add(1)
		go s.runReconcile(ctx)
	}
}

// Close stops both loops and waits for an in-flight handler or sweep to finish
// rather than interrupting it mid-write. Close without Start is a no-op.
func (s *Scheduler) Close() {
	if s.cancel == nil {
		return
	}
	s.cancel()
	s.wg.Wait()
}

// run is the ticker loop. It runs until the context is cancelled; a slow tick
// delays the next one (time.Ticker drops ticks rather than stacking them).
func (s *Scheduler) run(ctx context.Context) {
	defer s.wg.Done()

	// time.NewTicker panics on a non-positive interval. config's
	// validateSchedulerConfig is the operator-facing check (a bad tick_interval
	// fails startup with a named error); this fallback is the caller-bug
	// backstop for a Scheduler constructed in code, mirroring how
	// oauthserver's cleanupLoop guards its own interval.
	interval := s.cfg.TickInterval
	if interval <= 0 {
		interval = defaultTickInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// tick claims and runs every due schedule, up to the configured limit.
func (s *Scheduler) tick(ctx context.Context) {
	due, err := s.repo.ListDue(ctx, s.cfg.DueLimit)
	if err != nil {
		// The loop keeps going: a transient DB error must not kill scheduling.
		s.logger.Error("scheduler: failed to list due schedules", "error", err)
		return
	}
	for _, sched := range due {
		s.runDue(ctx, sched)
	}
}

// runDue claims one due schedule under a Postgres advisory lock and runs its
// registered handler, then advances the schedule. The lock is held on a single
// pinned connection for the duration of the handler — acquiring and releasing
// through the pool could land on different sessions, silently leaking the
// lock. A handler that returns an error is logged and the schedule still
// advances: retrying on the next tick an hour later is the intended cadence,
// and not advancing would hot-retry a failing job every tick.
func (s *Scheduler) runDue(ctx context.Context, sched *models.Schedule) {
	log := s.logger.With("job_type", sched.JobType, "team_id", sched.TeamID, "schedule_id", sched.ID)

	handler := s.registry.Lookup(sched.JobType)
	if handler == nil {
		log.Warn("scheduler: no handler registered for job_type, skipping")
		return
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		log.Error("scheduler: failed to acquire lock connection", "error", err)
		return
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			log.Error("scheduler: failed to close lock connection", "error", cerr)
		}
	}()

	key := lockKey(sched.ID)
	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&locked); err != nil {
		log.Error("scheduler: failed to acquire advisory lock", "error", err)
		return
	}
	if !locked {
		// Another replica is running this schedule; nothing to do.
		log.Debug("scheduler: schedule already claimed by another replica, skipping")
		return
	}
	defer func() {
		// Background context: the unlock must still run when the loop context
		// was cancelled (graceful shutdown with a handler in flight).
		if _, err := conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, key); err != nil {
			log.Error("scheduler: failed to release advisory lock", "error", err)
		}
	}()

	jobCtx, cancel := context.WithTimeout(ctx, s.cfg.JobTimeout)
	defer cancel()
	s.runHandler(jobCtx, log, handler, sched.TeamID)

	if err := s.repo.MarkRun(ctx, sched.ID); err != nil {
		// The schedule stays due and is retried on the next tick. With the
		// lock now released that may be another replica, which is correct.
		log.Error("scheduler: failed to advance schedule after run", "error", err)
	}
}

// runHandler invokes a job handler, recovering a panic so a misbehaving job
// is logged but never kills the loop.
func (s *Scheduler) runHandler(
	ctx context.Context,
	log *slog.Logger,
	handler Handler,
	teamID string,
) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("scheduler: job handler panicked", "panic", fmt.Sprint(r))
		}
	}()

	if err := handler(ctx, teamID); err != nil {
		log.Error("scheduler: job handler failed", "error", err)
	}
}
