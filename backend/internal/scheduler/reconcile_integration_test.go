//go:build integration

package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/postgres"
	"github.com/vibexp/vibexp/internal/services"
)

// The acceptance criterion end to end, against real Postgres and the real
// FreshnessService: a team with rules and no schedule acquires one WITHOUT ANY
// USER WRITE. The parts are covered elsewhere -- the anti-join in the postgres
// suite, the per-team loop in the services unit suite, the lock and the loop
// in this package's sqlmock suite -- but only the assembled path shows the
// engine actually reaching the repair. The seam this guards is the wiring:
// nothing else fails if ProvideScheduler stops passing the reconciler.
func TestIntegration_ReconcileProvisionsATeamWithRulesAndNoSchedule(t *testing.T) {
	resetReconcileIntegrationTables(t)
	ctx := context.Background()
	teamID := insertTestTeam(t, insertTestUser(t))
	rules := postgres.NewFreshnessRuleRepository(integrationDB)
	require.NoError(t, rules.Create(ctx, &models.FreshnessRule{
		TeamID:        teamID,
		ResourceTypes: []string{"artifact"},
		Mediums:       []string{"web"},
		ThresholdDays: 30,
		Enabled:       true,
	}))
	requireNoSchedule(t, teamID)

	newReconcileIntegrationScheduler().reconcile(ctx)

	sched := requireSchedule(t, teamID)
	assert.Equal(t, models.DefaultFreshnessIntervalSeconds, sched.IntervalSeconds)
	assert.Nil(t, sched.LastRunAt, "a freshly provisioned schedule has never run")
	assert.False(t, sched.NextRunAt.After(time.Now().Add(time.Minute)),
		"next_run_at is stamped now(), so the repaired team is due on the next tick")
}

// The sweep must be safe to run forever, which means the second pass leaves a
// healthy team's next_run_at exactly where the first (and any real run) left
// it. A sweep that re-armed every team each hour would be #767 instance-wide.
func TestIntegration_ReconcileLeavesAHealthyScheduleUntouched(t *testing.T) {
	resetReconcileIntegrationTables(t)
	ctx := context.Background()
	teamID := insertTestTeam(t, insertTestUser(t))
	rules := postgres.NewFreshnessRuleRepository(integrationDB)
	require.NoError(t, rules.Create(ctx, &models.FreshnessRule{
		TeamID:        teamID,
		ResourceTypes: []string{"artifact"},
		Mediums:       []string{"web"},
		ThresholdDays: 30,
		Enabled:       true,
	}))

	sweeper := newReconcileIntegrationScheduler()
	sweeper.reconcile(ctx)
	provisioned := requireSchedule(t, teamID)

	// Simulate a real run so next_run_at holds a value the sweep must not undo.
	require.NoError(t, postgres.NewScheduleRepository(integrationDB).MarkRun(ctx, provisioned.ID))
	afterRun := requireSchedule(t, teamID)
	require.False(t, afterRun.NextRunAt.Equal(provisioned.NextRunAt),
		"test setup must move next_run_at")

	sweeper.reconcile(ctx)

	final := requireSchedule(t, teamID)
	assert.True(t, final.NextRunAt.Equal(afterRun.NextRunAt),
		"the sweep must not re-arm a schedule that already exists")
	require.NotNil(t, final.LastRunAt)
	assert.True(t, final.LastRunAt.Equal(*afterRun.LastRunAt))
	assert.True(t, final.UpdatedAt.Equal(afterRun.UpdatedAt), "the row is not written at all")
}

// A team with no freshness rules is not swept into a schedule: freshness is
// simply not configured for it, and provisioning one would run the evaluator
// hourly forever over a team that asked for nothing.
func TestIntegration_ReconcileSkipsATeamWithNoRules(t *testing.T) {
	resetReconcileIntegrationTables(t)
	teamID := insertTestTeam(t, insertTestUser(t))

	newReconcileIntegrationScheduler().reconcile(context.Background())

	requireNoSchedule(t, teamID)
}

// Two replicas sweeping at once: the second finds the instance-wide advisory
// lock held and does nothing. Held on a separate connection, as a busy peer
// would.
func TestIntegration_ReconcileLockContentionSkipsSecondReplica(t *testing.T) {
	resetReconcileIntegrationTables(t)
	ctx := context.Background()
	teamID := insertTestTeam(t, insertTestUser(t))
	require.NoError(t, postgres.NewFreshnessRuleRepository(integrationDB).Create(ctx,
		&models.FreshnessRule{
			TeamID:        teamID,
			ResourceTypes: []string{"artifact"},
			Mediums:       []string{"web"},
			ThresholdDays: 30,
			Enabled:       true,
		}))

	holder, err := integrationDB.Conn(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, holder.Close()) }()
	var held bool
	require.NoError(t, holder.QueryRowContext(ctx,
		`SELECT pg_try_advisory_lock($1)`, reconcileLockID).Scan(&held))
	require.True(t, held, "test setup must hold the reconcile lock")
	defer func() {
		_, uerr := holder.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, reconcileLockID)
		require.NoError(t, uerr)
	}()

	newReconcileIntegrationScheduler().reconcile(ctx)

	requireNoSchedule(t, teamID)
}

// newReconcileIntegrationScheduler wires the engine to the REAL freshness
// service against the integration database, the way ProvideScheduler does.
// Only the reconcile path is exercised, so the nil dependencies the service
// never reaches on it are left nil deliberately rather than mocked.
func newReconcileIntegrationScheduler() *Scheduler {
	freshnessSvc := services.NewFreshnessService(
		postgres.NewFreshnessRuleRepository(integrationDB),
		postgres.NewResourceFreshnessRepository(integrationDB),
		postgres.NewTeamFreshnessSettingsRepository(integrationDB),
		postgres.NewFreshnessAuditRepository(integrationDB),
		postgres.NewScheduleRepository(integrationDB),
		postgres.NewProjectRepository(integrationDB),
		nil, // authz: the reconcile path is system-invoked and authorizes nothing.
		discardLogger(),
	)
	return New(
		postgres.NewScheduleRepository(integrationDB), integrationDB, NewRegistry(),
		Config{TickInterval: time.Hour, JobTimeout: time.Minute, DueLimit: 10},
		discardLogger(), freshnessSvc,
	)
}

// resetReconcileIntegrationTables clears this suite's chain. freshness_rules
// hangs off teams, which the shared reset truncates, but naming it explicitly
// keeps the suite honest if the shared list ever narrows.
func resetReconcileIntegrationTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, teams, projects, freshness_rules, "+
			"team_freshness_settings, schedules CASCADE")
	require.NoError(t, err)
}

func requireSchedule(t *testing.T, teamID string) *models.Schedule {
	t.Helper()
	var s models.Schedule
	err := integrationDB.QueryRowContext(context.Background(),
		`SELECT id, team_id, job_type, interval_seconds, last_run_at, next_run_at,
		        created_at, updated_at
		 FROM schedules WHERE team_id = $1 AND job_type = $2`,
		teamID, models.JobTypeFreshnessEvaluate,
	).Scan(&s.ID, &s.TeamID, &s.JobType, &s.IntervalSeconds,
		&s.LastRunAt, &s.NextRunAt, &s.CreatedAt, &s.UpdatedAt)
	require.NoError(t, err, "expected a freshness_evaluate schedule for the team")
	return &s
}

func requireNoSchedule(t *testing.T, teamID string) {
	t.Helper()
	var count int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		`SELECT count(*) FROM schedules WHERE team_id = $1 AND job_type = $2`,
		teamID, models.JobTypeFreshnessEvaluate).Scan(&count))
	require.Zero(t, count, "the team must have no freshness_evaluate schedule")
}
