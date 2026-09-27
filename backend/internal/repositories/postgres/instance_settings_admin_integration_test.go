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
// (#1200): the teams_with_override counts and the audited compare-and-set.

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
