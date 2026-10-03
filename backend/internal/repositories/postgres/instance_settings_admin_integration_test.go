//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Integration coverage for the instance-admin settings endpoints' data access
// (#1200): the teams_with_override counts and the audited compare-and-set,
// including the "nothing stored yet" expected version (#1220).

// TestIntegrationAdminRepository_CountTeamsWithSettingsOverride: each count is
// one per team row in its own settings table. The shared database holds other
// suites' teams, so the test asserts the delta its own rows add.
func TestIntegrationAdminRepository_CountTeamsWithSettingsOverride(t *testing.T) {
	ctx := context.Background()
	repo := NewAdminRepository(integrationDB)
	searchBefore, err := repo.CountTeamsWithSearchSettingsOverride(ctx)
	require.NoError(t, err)
	summaryBefore, err := repo.CountTeamsWithAISummarySettingsOverride(ctx)
	require.NoError(t, err)

	owner := insertTestUser(t)
	both, searchOnly := insertTestTeam(t, owner), insertTestTeam(t, owner)
	insertTestTeam(t, owner) // no settings of its own: counted by neither
	search := "INSERT INTO team_search_settings (team_id, recency_ranking_enabled, rank_weight_relevance, " +
		"rank_weight_created, rank_weight_updated, rank_half_life_days) VALUES ($1, true, 1, 0, 0, 30)"
	adminListExec(t, search, both)
	adminListExec(t, search, searchOnly)
	adminListExec(t, "INSERT INTO team_ai_summary_settings (team_id, enabled, top_n, style, max_output_tokens) "+
		"VALUES ($1, false, 3, 'balanced', 500)", both)

	searchAfter, err := repo.CountTeamsWithSearchSettingsOverride(ctx)
	require.NoError(t, err)
	summaryAfter, err := repo.CountTeamsWithAISummarySettingsOverride(ctx)
	require.NoError(t, err)
	assert.Equal(t, searchBefore+2, searchAfter)
	assert.Equal(t, summaryBefore+1, summaryAfter, "a disabled team profile is still an override")
}

// TestIntegrationInstanceSettings_UpsertAuditedCompareAndSet: against real
// Postgres, an expected version with nothing stored or a stale one is a
// conflict that writes nothing, and the current one writes and bumps it.
func TestIntegrationInstanceSettings_UpsertAuditedCompareAndSet(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, "instance_search_settings")
	repo := NewInstanceSearchSettingsRepository(integrationDB)
	ctx := context.Background()
	audit := searchAuditEntry(models.InstanceSettingsAuditActionUpsert)
	v := func(n int64) *int64 { return &n }

	err := repo.UpsertAudited(ctx, instanceSearchSettingsFixture(), v(1), audit)
	require.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict, "nothing stored yet")
	_, err = repo.Get(ctx)
	require.ErrorIs(t, err, repositories.ErrInstanceSearchSettingsNotFound)

	first := instanceSearchSettingsFixture()
	require.NoError(t, repo.UpsertAudited(ctx, first, nil, audit))
	require.Equal(t, int64(1), first.Version)

	stale := instanceSearchSettingsFixture()
	stale.RankCandidateCap = 999
	require.ErrorIs(t, repo.UpsertAudited(ctx, stale, v(0), audit), repositories.ErrInstanceSettingsVersionConflict)

	current := instanceSearchSettingsFixture()
	current.RankCandidateCap = 777
	require.NoError(t, repo.UpsertAudited(ctx, current, v(1), audit))
	assert.Equal(t, int64(2), current.Version)

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, 777, got.RankCandidateCap, "the stale write never landed")

	var entries int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM instance_settings_audit WHERE setting = $1`, models.InstanceSettingSearch).Scan(&entries))
	assert.Equal(t, 2, entries, "only the two successful writes are audited")
}

// expectNothingStoredCase drives one audited singleton through the
// "nothing stored yet" expected version (#1220) against real Postgres.
type expectNothingStoredCase struct {
	name  string
	table string
	// save upserts a fresh fixture with expected and returns the version it
	// was stored at.
	save func(ctx context.Context, expected *int64) (int64, error)
	// reset deletes the stored row.
	reset func(ctx context.Context) (bool, error)
	// stored reports whether a row exists.
	stored func(ctx context.Context) bool
	// auditCount is the number of audit entries written for this setting.
	auditCount func(t *testing.T) int
}

func expectNothingStoredCases() []expectNothingStoredCase {
	search := NewInstanceSearchSettingsRepository(integrationDB)
	summary := NewInstanceAISummarySettingsRepository(integrationDB)
	auditCount := func(setting string) func(t *testing.T) int {
		return func(t *testing.T) int {
			t.Helper()
			entries, _, err := NewInstanceSettingsAuditRepository(integrationDB).
				List(context.Background(), setting, 50, nil)
			require.NoError(t, err)
			return len(entries)
		}
	}
	return []expectNothingStoredCase{
		{
			name: "search", table: "instance_search_settings",
			save: func(ctx context.Context, expected *int64) (int64, error) {
				s := instanceSearchSettingsFixture()
				err := search.UpsertAudited(ctx, s, expected, searchAuditEntry(models.InstanceSettingsAuditActionUpsert))
				return s.Version, err
			},
			reset: func(ctx context.Context) (bool, error) {
				return search.DeleteAudited(ctx, searchAuditEntry(models.InstanceSettingsAuditActionDelete))
			},
			stored: func(ctx context.Context) bool {
				_, err := search.Get(ctx)
				return err == nil
			},
			auditCount: auditCount(models.InstanceSettingSearch),
		},
		{
			name: "ai summary", table: "instance_ai_summary_settings",
			save: func(ctx context.Context, expected *int64) (int64, error) {
				s := instanceAISummarySettingsFixture()
				err := summary.UpsertAudited(ctx, s, expected, aiSummaryAuditEntry(models.InstanceSettingsAuditActionUpsert))
				return s.Version, err
			},
			reset: func(ctx context.Context) (bool, error) {
				return summary.DeleteAudited(ctx, aiSummaryAuditEntry(models.InstanceSettingsAuditActionDelete))
			},
			stored: func(ctx context.Context) bool {
				_, err := summary.Get(ctx)
				return err == nil
			},
			auditCount: auditCount(models.InstanceSettingAISummary),
		},
	}
}

// TestIntegrationInstanceSettings_ExpectNothingStored: expected version 0
// saves only while no row exists. A second save carrying it is a conflict that
// writes and audits nothing, and it works again once the row is reset.
func TestIntegrationInstanceSettings_ExpectNothingStored(t *testing.T) {
	for _, tc := range expectNothingStoredCases() {
		t.Run(tc.name, func(t *testing.T) {
			resetInstanceSettingsAuditTables(t)
			resetInstanceSettingsTable(t, tc.table)
			ctx := context.Background()
			nothing := repositories.InstanceSettingsNoStoredVersion

			version, err := tc.save(ctx, &nothing)
			require.NoError(t, err, "nothing is stored, so the first save lands")
			assert.Equal(t, int64(1), version)

			_, err = tc.save(ctx, &nothing)
			require.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict, "a row is stored now")
			assert.Equal(t, 1, tc.auditCount(t), "the conflict audits nothing")

			current := int64(1)
			version, err = tc.save(ctx, &current)
			require.NoError(t, err, "the conflict left the row at version 1")
			assert.Equal(t, int64(2), version)

			deleted, err := tc.reset(ctx)
			require.NoError(t, err)
			require.True(t, deleted)
			version, err = tc.save(ctx, &nothing)
			require.NoError(t, err, "a reset leaves nothing stored again")
			assert.Equal(t, int64(1), version)
		})
	}
}

// TestIntegrationInstanceSettings_ConcurrentFirstSavesExpectingNothing: of
// several first saves that all expect nothing stored, the lock lets exactly
// one through; the rest are conflicts instead of silent overwrites.
func TestIntegrationInstanceSettings_ConcurrentFirstSavesExpectingNothing(t *testing.T) {
	for _, tc := range expectNothingStoredCases() {
		t.Run(tc.name, func(t *testing.T) {
			resetInstanceSettingsAuditTables(t)
			resetInstanceSettingsTable(t, tc.table)
			ctx := context.Background()

			const writers = 4
			start := make(chan struct{})
			errs := make(chan error, writers)
			for i := 0; i < writers; i++ {
				go func() {
					<-start
					nothing := repositories.InstanceSettingsNoStoredVersion
					_, err := tc.save(ctx, &nothing)
					errs <- err
				}()
			}
			close(start)
			var ok, conflicts int
			for i := 0; i < writers; i++ {
				err := <-errs
				switch {
				case err == nil:
					ok++
				case assert.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict):
					conflicts++
				}
			}

			assert.Equal(t, 1, ok)
			assert.Equal(t, writers-1, conflicts)
			assert.True(t, tc.stored(ctx))
			assert.Equal(t, 1, tc.auditCount(t), "only the winning save is audited")
		})
	}
}
