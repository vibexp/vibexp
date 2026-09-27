//go:build integration

package postgres

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

// The #1201 upgrade bridge against real Postgres: a non-default config.yaml
// search: / ai_summary: block is imported into its instance settings row
// exactly once, with one import audit entry, and ignored once a row exists.

func resetLegacySettingsImportTables(t *testing.T) {
	t.Helper()
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, "instance_search_settings")
	resetInstanceSettingsTable(t, "instance_ai_summary_settings")
	t.Cleanup(func() { resetInstanceSettingsAuditTables(t) })
}

// importAuditEntries lists a setting's audit entries (any action).
func importAuditEntries(t *testing.T, setting string) []models.InstanceSettingsAuditEntry {
	t.Helper()
	rows, err := integrationDB.QueryContext(context.Background(),
		"SELECT action, actor_user_id, before, after FROM instance_settings_audit WHERE setting = $1", setting)
	require.NoError(t, err)
	defer func() { assert.NoError(t, rows.Close()) }()
	var out []models.InstanceSettingsAuditEntry
	for rows.Next() {
		var e models.InstanceSettingsAuditEntry
		var before, after []byte
		require.NoError(t, rows.Scan(&e.Action, &e.ActorUserID, &before, &after))
		e.Before, e.After = before, after
		out = append(out, e)
	}
	require.NoError(t, rows.Err())
	return out
}

// countWarnings counts WARN-or-above log entries for a section.
func countWarnings(logs *logtest.Recorder, section string) int {
	n := 0
	for _, e := range logs.AllEntries() {
		if e.Level >= slog.LevelWarn && e.Data["section"] == section {
			n++
		}
	}
	return n
}

func tunedSearchValues() models.InstanceSearchSettingsValues {
	v := services.BuiltInSearchDefaults()
	v.RecencyRankingEnabled = true
	v.RankCandidateCap = 400
	return v
}

func tunedAISummaryValues() models.InstanceAISummarySettingsValues {
	v := models.DefaultInstanceAISummarySettings()
	v.TopN = 7
	v.RequestTimeout = 45 * time.Second
	return v
}

// adminSearchRow is a row an admin stored before the import runs.
func adminSearchRow() *models.InstanceSearchSettings {
	return &models.InstanceSearchSettings{
		RankWeightRelevance: 0.9, RankWeightCreated: 0.05, RankWeightUpdated: 0.05,
		RankHalfLifeDays: 10, RankCandidateCap: 50,
	}
}

func adminAISummaryRow() *models.InstanceAISummarySettings {
	return &models.InstanceAISummarySettings{
		Enabled: false, TopN: 2, Style: models.AISummaryStyleDetailed, MaxOutputTokens: 100,
		PerDocumentChars: 1000, TotalContextChars: 2000, RequestTimeout: 5 * time.Second,
	}
}

func TestIntegrationLegacySearchImport_DecisionTable(t *testing.T) {
	ctx := context.Background()
	repo := NewInstanceSearchSettingsRepository(integrationDB)

	tests := []struct {
		name         string
		rowPresent   bool
		legacy       models.InstanceSearchSettingsValues
		wantResult   services.LegacySettingsImportResult
		wantWarnings int
	}{
		{"absent + differs → imported", false, tunedSearchValues(),
			services.LegacySettingsImportResult{Imported: true}, 1},
		{"absent + equal → nothing", false, services.BuiltInSearchDefaults(),
			services.LegacySettingsImportResult{}, 0},
		{"present + differs → ignored", true, tunedSearchValues(),
			services.LegacySettingsImportResult{Ignored: true}, 1},
		{"present + equal → nothing", true, services.BuiltInSearchDefaults(),
			services.LegacySettingsImportResult{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetLegacySettingsImportTables(t)
			if tt.rowPresent {
				require.NoError(t, repo.Upsert(ctx, adminSearchRow()))
			}
			logger, logs := logtest.New()

			got := services.ImportLegacySearchSettings(ctx, repo, tt.legacy, logger)

			assert.Equal(t, tt.wantResult, got)
			assert.Equal(t, tt.wantWarnings, countWarnings(logs, "search"))
			stored, err := repo.Get(ctx)
			entries := importAuditEntries(t, models.InstanceSettingSearch)
			switch {
			case tt.rowPresent:
				require.NoError(t, err)
				assert.Equal(t, 50, stored.RankCandidateCap, "the admin's row is untouched")
				assert.Empty(t, entries)
			case tt.wantResult.Imported:
				require.NoError(t, err)
				assert.Equal(t, tt.legacy.RankCandidateCap, stored.RankCandidateCap)
				assert.True(t, stored.RecencyRankingEnabled)
				assert.Nil(t, stored.UpdatedBy)
				require.Len(t, entries, 1)
				assert.Equal(t, models.InstanceSettingsAuditActionImport, entries[0].Action)
				assert.Nil(t, entries[0].ActorUserID)
				assert.Nil(t, entries[0].Before)
				assert.JSONEq(t, `{"recency_ranking_enabled":true,"rank_weight_relevance":0.5,
					"rank_weight_created":0.3,"rank_weight_updated":0.2,"rank_half_life_days":90,
					"rank_candidate_cap":400}`, string(entries[0].After))
			default:
				assert.ErrorIs(t, err, repositories.ErrInstanceSearchSettingsNotFound)
				assert.Empty(t, entries)
			}
		})
	}
}

func TestIntegrationLegacyAISummaryImport_DecisionTable(t *testing.T) {
	ctx := context.Background()
	repo := NewInstanceAISummarySettingsRepository(integrationDB)

	tests := []struct {
		name         string
		rowPresent   bool
		legacy       models.InstanceAISummarySettingsValues
		wantResult   services.LegacySettingsImportResult
		wantWarnings int
	}{
		{"absent + differs → imported", false, tunedAISummaryValues(),
			services.LegacySettingsImportResult{Imported: true}, 1},
		{"absent + equal → nothing", false, models.DefaultInstanceAISummarySettings(),
			services.LegacySettingsImportResult{}, 0},
		{"present + differs → ignored", true, tunedAISummaryValues(),
			services.LegacySettingsImportResult{Ignored: true}, 1},
		{"present + equal → nothing", true, models.DefaultInstanceAISummarySettings(),
			services.LegacySettingsImportResult{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetLegacySettingsImportTables(t)
			if tt.rowPresent {
				require.NoError(t, repo.Upsert(ctx, adminAISummaryRow()))
			}
			logger, logs := logtest.New()

			got := services.ImportLegacyAISummarySettings(ctx, repo, tt.legacy, false, logger)

			assert.Equal(t, tt.wantResult, got)
			assert.Equal(t, tt.wantWarnings, countWarnings(logs, "ai_summary"))
			stored, err := repo.Get(ctx)
			entries := importAuditEntries(t, models.InstanceSettingAISummary)
			switch {
			case tt.rowPresent:
				require.NoError(t, err)
				assert.Equal(t, 2, stored.TopN, "the admin's row is untouched")
				assert.Empty(t, entries)
			case tt.wantResult.Imported:
				require.NoError(t, err)
				assert.Equal(t, 7, stored.TopN)
				assert.Equal(t, 45*time.Second, stored.RequestTimeout)
				assert.Nil(t, stored.UpdatedBy)
				require.Len(t, entries, 1)
				assert.Equal(t, models.InstanceSettingsAuditActionImport, entries[0].Action)
				assert.Nil(t, entries[0].ActorUserID)
				assert.Nil(t, entries[0].Before)
				assert.JSONEq(t, `{"enabled":true,"top_n":7,"style":"balanced","max_output_tokens":800,
					"per_document_chars":8000,"total_context_chars":32000,"request_timeout_ms":45000}`,
					string(entries[0].After))
			default:
				assert.ErrorIs(t, err, repositories.ErrInstanceAISummarySettingsNotFound)
				assert.Empty(t, entries)
			}
		})
	}
}

// An ai_summary block the shared validator rejects imports nothing.
func TestIntegrationLegacyAISummaryImport_InvalidBlockImportsNothing(t *testing.T) {
	resetLegacySettingsImportTables(t)
	ctx := context.Background()
	repo := NewInstanceAISummarySettingsRepository(integrationDB)
	legacy := tunedAISummaryValues()
	legacy.Style = "bogus"

	got := services.ImportLegacyAISummarySettings(ctx, repo, legacy, false, slog.New(slog.DiscardHandler))

	assert.Equal(t, services.LegacySettingsImportResult{}, got)
	_, err := repo.Get(ctx)
	assert.ErrorIs(t, err, repositories.ErrInstanceAISummarySettingsNotFound)
	assert.Empty(t, importAuditEntries(t, models.InstanceSettingAISummary))
}

// Each barrier repo holds every replica after its Get until all replicas have
// read, so each sees "no row" and the insert's table lock alone decides who
// writes. Without the barrier one replica could finish before the other reads,
// and the test would pass even with an unguarded insert.
type barrierSearchRepo struct {
	repositories.InstanceSearchSettingsRepository
	readers *sync.WaitGroup
}

func (r barrierSearchRepo) Get(ctx context.Context) (*models.InstanceSearchSettings, error) {
	row, err := r.InstanceSearchSettingsRepository.Get(ctx)
	r.readers.Done()
	r.readers.Wait()
	return row, err
}

type barrierAISummaryRepo struct {
	repositories.InstanceAISummarySettingsRepository
	readers *sync.WaitGroup
}

func (r barrierAISummaryRepo) Get(ctx context.Context) (*models.InstanceAISummarySettings, error) {
	row, err := r.InstanceAISummarySettingsRepository.Get(ctx)
	r.readers.Done()
	r.readers.Wait()
	return row, err
}

func TestIntegrationLegacySettingsImport_ConcurrentBootsImportOnce(t *testing.T) {
	resetLegacySettingsImportTables(t)
	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)

	const replicas = 2
	searchResults := make([]services.LegacySettingsImportResult, replicas)
	aiResults := make([]services.LegacySettingsImportResult, replicas)
	var searchReaders, aiReaders, wg sync.WaitGroup
	searchReaders.Add(replicas)
	aiReaders.Add(replicas)
	for i := range replicas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			searchRepo := barrierSearchRepo{NewInstanceSearchSettingsRepository(integrationDB), &searchReaders}
			aiRepo := barrierAISummaryRepo{NewInstanceAISummarySettingsRepository(integrationDB), &aiReaders}
			searchResults[i] = services.ImportLegacySearchSettings(ctx, searchRepo, tunedSearchValues(), logger)
			aiResults[i] = services.ImportLegacyAISummarySettings(ctx, aiRepo, tunedAISummaryValues(), false, logger)
		}()
	}
	wg.Wait()

	for setting, results := range map[string][]services.LegacySettingsImportResult{
		models.InstanceSettingSearch:    searchResults,
		models.InstanceSettingAISummary: aiResults,
	} {
		entries := importAuditEntries(t, setting)
		require.Len(t, entries, 1, "%s: exactly one audit entry", setting)
		assert.Equal(t, models.InstanceSettingsAuditActionImport, entries[0].Action)
		imported := 0
		for _, r := range results {
			if r.Imported {
				imported++
			}
		}
		assert.Equal(t, 1, imported, "%s: exactly one replica imports", setting)
	}
	var searchRows, aiRows int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT count(*) FROM instance_search_settings").Scan(&searchRows))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT count(*) FROM instance_ai_summary_settings").Scan(&aiRows))
	assert.Equal(t, 1, searchRows)
	assert.Equal(t, 1, aiRows)
}
