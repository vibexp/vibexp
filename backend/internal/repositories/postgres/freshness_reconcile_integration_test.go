//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ListTeamIDsMissingSchedule against real Postgres (#768). The anti-join is
// the whole point of the method, and an anti-join is exactly the kind of query
// the sqlmock suite cannot check: it returns whatever rows the test declares,
// so a JOIN with the wrong ON clause, the wrong direction, or a missing
// job_type predicate passes there and fails here.

// resetReconcileTables clears the chain this suite touches. It names
// `schedules` explicitly: resetFreshnessTables does not, and a schedule row
// left behind by another test is precisely the thing that would make the
// "missing" assertions vacuous.
func resetReconcileTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, teams, projects, freshness_rules, schedules CASCADE")
	require.NoError(t, err)
}

// seedRule gives the team one enabled rule over every project.
func seedRule(t *testing.T, repo repositories.FreshnessRuleRepository, teamID string) {
	t.Helper()
	require.NoError(t, repo.Create(context.Background(), &models.FreshnessRule{
		TeamID:        teamID,
		ResourceTypes: []string{"artifact"},
		Mediums:       []string{"web"},
		ThresholdDays: 30,
		Enabled:       true,
	}))
}

// The three states the sweep must distinguish, asserted together so the query
// is proved to discriminate rather than merely to return something.
func TestIntegrationFreshnessRule_ListTeamIDsMissingSchedule(t *testing.T) {
	resetReconcileTables(t)
	ctx := context.Background()
	rules := NewFreshnessRuleRepository(integrationDB)
	schedules := NewScheduleRepository(integrationDB)

	userID := insertTestUser(t)
	// (1) rules, no schedule — the broken team the sweep exists to repair.
	broken := insertTestTeam(t, userID)
	seedRule(t, rules, broken)

	// (2) rules AND a schedule — healthy; must NOT be returned, because a
	// returned team gets an Upsert and an Upsert re-arms next_run_at (#767).
	healthy := insertTestTeam(t, userID)
	seedRule(t, rules, healthy)
	require.NoError(t, schedules.Upsert(ctx, &models.Schedule{
		TeamID:          healthy,
		JobType:         models.JobTypeFreshnessEvaluate,
		IntervalSeconds: 3600,
	}))

	// (3) no rules at all — never gets a schedule; freshness is not configured.
	unconfigured := insertTestTeam(t, userID)

	got, err := rules.ListTeamIDsMissingSchedule(ctx, models.JobTypeFreshnessEvaluate)
	require.NoError(t, err)
	assert.Equal(t, []string{broken}, got,
		"only the team with rules and no schedule is reported")
	assert.NotContains(t, got, healthy)
	assert.NotContains(t, got, unconfigured)
}

// A schedule for a DIFFERENT job type must not count as this job's schedule —
// the ON clause's job_type predicate is what makes the join per-job, and
// dropping it would silently exclude every team that has any schedule at all.
func TestIntegrationFreshnessRule_ListTeamIDsMissingScheduleIsPerJobType(t *testing.T) {
	resetReconcileTables(t)
	ctx := context.Background()
	rules := NewFreshnessRuleRepository(integrationDB)
	schedules := NewScheduleRepository(integrationDB)

	teamID := insertTestTeam(t, insertTestUser(t))
	seedRule(t, rules, teamID)
	require.NoError(t, schedules.Upsert(ctx, &models.Schedule{
		TeamID:          teamID,
		JobType:         "some_other_job",
		IntervalSeconds: 3600,
	}))

	got, err := rules.ListTeamIDsMissingSchedule(ctx, models.JobTypeFreshnessEvaluate)
	require.NoError(t, err)
	assert.Equal(t, []string{teamID}, got,
		"another job's schedule does not satisfy freshness_evaluate")
}

// A team with several rules is reported ONCE. Without DISTINCT the sweep would
// upsert the same team n times per pass.
func TestIntegrationFreshnessRule_ListTeamIDsMissingScheduleDeduplicates(t *testing.T) {
	resetReconcileTables(t)
	ctx := context.Background()
	rules := NewFreshnessRuleRepository(integrationDB)

	teamID := insertTestTeam(t, insertTestUser(t))
	seedRule(t, rules, teamID)
	seedRule(t, rules, teamID)
	seedRule(t, rules, teamID)

	got, err := rules.ListTeamIDsMissingSchedule(ctx, models.JobTypeFreshnessEvaluate)
	require.NoError(t, err)
	assert.Equal(t, []string{teamID}, got)
}

// A team whose only rules are DISABLED still needs a schedule: syncSchedule
// keeps the row while the team has ANY rule, because disabling the last
// enabled rule does not clear the state it produced — only an evaluation run
// does. The sweep must agree with that condition or it would strand exactly
// the teams syncSchedule is careful not to strand.
func TestIntegrationFreshnessRule_ListTeamIDsMissingScheduleCountsDisabledRules(t *testing.T) {
	resetReconcileTables(t)
	ctx := context.Background()
	rules := NewFreshnessRuleRepository(integrationDB)

	teamID := insertTestTeam(t, insertTestUser(t))
	require.NoError(t, rules.Create(ctx, &models.FreshnessRule{
		TeamID:        teamID,
		ResourceTypes: []string{"artifact"},
		Mediums:       []string{"web"},
		ThresholdDays: 30,
		Enabled:       false,
	}))

	got, err := rules.ListTeamIDsMissingSchedule(ctx, models.JobTypeFreshnessEvaluate)
	require.NoError(t, err)
	assert.Equal(t, []string{teamID}, got)
}

// The healthy steady state: nothing missing returns an EMPTY slice, never nil,
// and the sweep's caller can range over it without a guard.
func TestIntegrationFreshnessRule_ListTeamIDsMissingScheduleEmptyWhenHealthy(t *testing.T) {
	resetReconcileTables(t)
	ctx := context.Background()
	rules := NewFreshnessRuleRepository(integrationDB)

	got, err := rules.ListTeamIDsMissingSchedule(ctx, models.JobTypeFreshnessEvaluate)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, got)
}

// End-to-end proof of the acceptance criterion the query alone cannot show:
// running the reconciliation twice leaves a healthy team's next_run_at
// EXACTLY as it was. The first pass provisions; the second must be a no-op,
// because a second Upsert would reset next_run_at to now() and re-arm the team
// on every sweep — #767's monopolisation bug, instance-wide.
func TestIntegrationFreshnessRule_ReconcileIsIdempotentOnNextRunAt(t *testing.T) {
	resetReconcileTables(t)
	ctx := context.Background()
	rules := NewFreshnessRuleRepository(integrationDB)
	schedules := NewScheduleRepository(integrationDB)

	teamID := insertTestTeam(t, insertTestUser(t))
	seedRule(t, rules, teamID)

	// Pass 1: the team is missing a schedule, so it is provisioned.
	missing, err := rules.ListTeamIDsMissingSchedule(ctx, models.JobTypeFreshnessEvaluate)
	require.NoError(t, err)
	require.Equal(t, []string{teamID}, missing)
	require.NoError(t, schedules.Upsert(ctx, &models.Schedule{
		TeamID:          teamID,
		JobType:         models.JobTypeFreshnessEvaluate,
		IntervalSeconds: 3600,
	}))

	before := readNextRunAt(t, teamID)

	// A real run advances the schedule; the sweep must not undo that.
	// (Sleeping is avoided: MarkRun stamps from the database clock, so the
	// value simply has to CHANGE for the second read to be discriminating.)
	sched := readSchedule(t, teamID)
	require.NoError(t, schedules.MarkRun(ctx, sched.ID))
	afterRun := readNextRunAt(t, teamID)
	require.False(t, afterRun.Equal(before), "test setup must move next_run_at")

	// Pass 2: nothing is missing, so nothing is upserted and the advanced
	// next_run_at survives untouched.
	missing, err = rules.ListTeamIDsMissingSchedule(ctx, models.JobTypeFreshnessEvaluate)
	require.NoError(t, err)
	assert.Empty(t, missing, "a provisioned team is never swept again")
	assert.True(t, readNextRunAt(t, teamID).Equal(afterRun),
		"the sweep must not re-arm a healthy schedule")
}

// readSchedule returns the team's freshness_evaluate schedule row.
func readSchedule(t *testing.T, teamID string) *models.Schedule {
	t.Helper()
	var s models.Schedule
	err := integrationDB.QueryRowContext(context.Background(),
		`SELECT `+scheduleColumns+` FROM schedules WHERE team_id = $1 AND job_type = $2`,
		teamID, models.JobTypeFreshnessEvaluate,
	).Scan(scanScheduleDest(&s)...)
	require.NoError(t, err)
	return &s
}

func readNextRunAt(t *testing.T, teamID string) time.Time {
	t.Helper()
	return readSchedule(t, teamID).NextRunAt
}
