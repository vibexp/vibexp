package services

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// tunedLegacySearch is an operator's non-default, valid search: block.
func tunedLegacySearch() models.InstanceSearchSettingsValues {
	v := BuiltInSearchDefaults()
	v.RecencyRankingEnabled = true
	v.RankHalfLifeDays = 30
	return v
}

// tunedLegacyAISummary is an operator's non-default, valid ai_summary: block.
func tunedLegacyAISummary() models.InstanceAISummarySettingsValues {
	v := models.DefaultInstanceAISummarySettings()
	v.TopN = 3
	v.Style = models.AISummaryStyleConcise
	v.RequestTimeout = 30 * time.Second
	return v
}

// warnings returns the messages logged at WARN or above.
func warnings(logs *logtest.Recorder) []string {
	var out []string
	for _, e := range logs.AllEntries() {
		if e.Level >= slog.LevelWarn {
			out = append(out, e.Message)
		}
	}
	return out
}

// entryContaining returns the one entry whose message contains substr.
func entryContaining(t *testing.T, logs *logtest.Recorder, substr string) *logtest.Entry {
	t.Helper()
	var found []*logtest.Entry
	for _, e := range logs.AllEntries() {
		if strings.Contains(e.Message, substr) {
			found = append(found, e)
		}
	}
	require.Len(t, found, 1, "want exactly one log entry containing %q", substr)
	return found[0]
}

func TestImportLegacySearchSettings_DefaultsDoNothing(t *testing.T) {
	// The strict mock fails on any call: equal-to-defaults reads nothing either.
	repo := repomocks.NewMockInstanceSearchSettingsRepository(t)
	logger, logs := logtest.New()

	res := ImportLegacySearchSettings(context.Background(), repo, BuiltInSearchDefaults(), logger)

	assert.Equal(t, LegacySettingsImportResult{}, res)
	assert.Empty(t, logs.AllEntries())
}

func TestImportLegacySearchSettings_AbsentRowImportsAudited(t *testing.T) {
	repo := repomocks.NewMockInstanceSearchSettingsRepository(t)
	logger, logs := logtest.New()
	legacy := tunedLegacySearch()

	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceSearchSettingsNotFound)
	var gotRow *models.InstanceSearchSettings
	var gotEntry *models.InstanceSettingsAuditEntry
	repo.EXPECT().InsertIfAbsentAudited(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(
			_ context.Context, row *models.InstanceSearchSettings, audit repositories.InstanceSearchSettingsAuditFunc,
		) (bool, error) {
			gotRow = row
			entry, err := audit(nil, row)
			gotEntry = entry
			return true, err
		})

	res := ImportLegacySearchSettings(context.Background(), repo, legacy, logger)

	assert.Equal(t, LegacySettingsImportResult{Imported: true}, res)
	require.NotNil(t, gotRow)
	assert.Equal(t, legacy, instanceSearchValuesFromStored(gotRow))
	assert.Nil(t, gotRow.UpdatedBy, "an import has no actor")
	require.NotNil(t, gotEntry)
	assert.Equal(t, models.InstanceSettingSearch, gotEntry.Setting)
	assert.Equal(t, models.InstanceSettingsAuditActionImport, gotEntry.Action)
	assert.Nil(t, gotEntry.ActorUserID)
	assert.Nil(t, gotEntry.Before)
	var after models.InstanceSearchSettingsValues
	require.NoError(t, json.Unmarshal(gotEntry.After, &after))
	assert.Equal(t, legacy, after)

	e := entryContaining(t, logs, "Imported the config.yaml section")
	assert.Equal(t, slog.LevelWarn, e.Level, "the import doubles as the deprecation warning")
	assert.Equal(t, "search", e.Data["section"])
	assert.Contains(t, e.Message, "next minor release")
	assert.Contains(t, e.Message, "Admin → Settings → Search")
}

func TestImportLegacySearchSettings_PresentRowIgnoresBlock(t *testing.T) {
	repo := repomocks.NewMockInstanceSearchSettingsRepository(t)
	logger, logs := logtest.New()
	repo.EXPECT().Get(mock.Anything).Return(&models.InstanceSearchSettings{}, nil)

	res := ImportLegacySearchSettings(context.Background(), repo, tunedLegacySearch(), logger)

	assert.Equal(t, LegacySettingsImportResult{Ignored: true}, res)
	e := entryContaining(t, logs, "is ignored")
	assert.Equal(t, slog.LevelWarn, e.Level)
	assert.Equal(t, "search", e.Data["section"])
	assert.Contains(t, e.Message, "Admin → Settings → Search")
}

func TestImportLegacySearchSettings_ConcurrentLoserWritesNothing(t *testing.T) {
	repo := repomocks.NewMockInstanceSearchSettingsRepository(t)
	logger, logs := logtest.New()
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceSearchSettingsNotFound)
	repo.EXPECT().InsertIfAbsentAudited(mock.Anything, mock.Anything, mock.Anything).Return(false, nil)

	res := ImportLegacySearchSettings(context.Background(), repo, tunedLegacySearch(), logger)

	assert.Equal(t, LegacySettingsImportResult{}, res)
	assert.Equal(t, slog.LevelInfo, entryContaining(t, logs, "stored concurrently").Level,
		"losing the race is not the ignored-block warning")
	assert.Empty(t, warnings(logs))
}

func TestImportLegacySearchSettings_ReadErrorSkips(t *testing.T) {
	repo := repomocks.NewMockInstanceSearchSettingsRepository(t)
	logger, logs := logtest.New()
	repo.EXPECT().Get(mock.Anything).Return(nil, errors.New("connection refused"))

	res := ImportLegacySearchSettings(context.Background(), repo, tunedLegacySearch(), logger)

	assert.Equal(t, LegacySettingsImportResult{}, res)
	e := entryContaining(t, logs, "Failed to read the instance settings")
	assert.Equal(t, slog.LevelError, e.Level)
	assert.Equal(t, "search", e.Data["section"])
}

func TestImportLegacySearchSettings_InsertErrorIsLogged(t *testing.T) {
	repo := repomocks.NewMockInstanceSearchSettingsRepository(t)
	logger, logs := logtest.New()
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceSearchSettingsNotFound)
	repo.EXPECT().InsertIfAbsentAudited(mock.Anything, mock.Anything, mock.Anything).
		Return(false, errors.New("lock timeout"))

	res := ImportLegacySearchSettings(context.Background(), repo, tunedLegacySearch(), logger)

	assert.Equal(t, LegacySettingsImportResult{}, res)
	assert.Equal(t, slog.LevelError, entryContaining(t, logs, "Failed to import").Level)
}

// config still fails fast on an invalid search: block, so this is defence in
// depth: the bridge re-validates and imports nothing.
func TestImportLegacySearchSettings_InvalidBlockImportsNothing(t *testing.T) {
	repo := repomocks.NewMockInstanceSearchSettingsRepository(t)
	logger, logs := logtest.New()
	legacy := tunedLegacySearch()
	legacy.RankCandidateCap = 0
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceSearchSettingsNotFound)

	res := ImportLegacySearchSettings(context.Background(), repo, legacy, logger)

	assert.Equal(t, LegacySettingsImportResult{}, res)
	e := entryContaining(t, logs, "is invalid")
	assert.Equal(t, slog.LevelError, e.Level)
	assert.Contains(t, e.Data["error"].(error).Error(), "rank_candidate_cap")
}

func TestImportLegacyAISummarySettings_DefaultsDoNothing(t *testing.T) {
	repo := repomocks.NewMockInstanceAISummarySettingsRepository(t)
	logger, logs := logtest.New()

	res := ImportLegacyAISummarySettings(context.Background(), repo,
		models.DefaultInstanceAISummarySettings(), false, logger)

	assert.Equal(t, LegacySettingsImportResult{}, res)
	assert.Empty(t, logs.AllEntries())
}

func TestImportLegacyAISummarySettings_AbsentRowImportsAudited(t *testing.T) {
	repo := repomocks.NewMockInstanceAISummarySettingsRepository(t)
	logger, logs := logtest.New()
	legacy := tunedLegacyAISummary()

	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAISummarySettingsNotFound)
	var gotRow *models.InstanceAISummarySettings
	var gotEntry *models.InstanceSettingsAuditEntry
	repo.EXPECT().InsertIfAbsentAudited(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(
			_ context.Context, row *models.InstanceAISummarySettings,
			audit repositories.InstanceAISummarySettingsAuditFunc,
		) (bool, error) {
			gotRow = row
			entry, err := audit(nil, row)
			gotEntry = entry
			return true, err
		})

	res := ImportLegacyAISummarySettings(context.Background(), repo, legacy, false, logger)

	assert.Equal(t, LegacySettingsImportResult{Imported: true}, res)
	require.NotNil(t, gotRow)
	assert.Equal(t, legacy, instanceAISummaryValuesFromStored(gotRow))
	assert.Nil(t, gotRow.UpdatedBy)
	require.NotNil(t, gotEntry)
	assert.Equal(t, models.InstanceSettingAISummary, gotEntry.Setting)
	assert.Equal(t, models.InstanceSettingsAuditActionImport, gotEntry.Action)
	assert.Nil(t, gotEntry.ActorUserID)
	assert.Nil(t, gotEntry.Before)
	assert.JSONEq(t, `{"enabled":true,"top_n":3,"style":"concise","max_output_tokens":800,
		"per_document_chars":8000,"total_context_chars":32000,"request_timeout_ms":30000}`, string(gotEntry.After))

	e := entryContaining(t, logs, "Imported the config.yaml section")
	assert.Equal(t, slog.LevelWarn, e.Level)
	assert.Equal(t, "ai_summary", e.Data["section"])
	assert.Contains(t, e.Message, "Admin → Settings → AI Summary")
}

func TestImportLegacyAISummarySettings_PresentRowIgnoresBlock(t *testing.T) {
	repo := repomocks.NewMockInstanceAISummarySettingsRepository(t)
	logger, logs := logtest.New()
	repo.EXPECT().Get(mock.Anything).Return(&models.InstanceAISummarySettings{}, nil)

	res := ImportLegacyAISummarySettings(context.Background(), repo, tunedLegacyAISummary(), false, logger)

	assert.Equal(t, LegacySettingsImportResult{Ignored: true}, res)
	e := entryContaining(t, logs, "is ignored")
	assert.Equal(t, "ai_summary", e.Data["section"])
	assert.Contains(t, e.Message, "Admin → Settings → AI Summary")
}

// A block the shared validator rejects (it used to fail boot in config) logs
// an error, imports nothing, and returns normally so boot continues.
func TestImportLegacyAISummarySettings_InvalidBlockImportsNothing(t *testing.T) {
	repo := repomocks.NewMockInstanceAISummarySettingsRepository(t)
	logger, logs := logtest.New()
	legacy := tunedLegacyAISummary()
	legacy.Style = "bogus"
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAISummarySettingsNotFound)

	res := ImportLegacyAISummarySettings(context.Background(), repo, legacy, false, logger)

	assert.Equal(t, LegacySettingsImportResult{}, res)
	e := entryContaining(t, logs, "is invalid")
	assert.Equal(t, slog.LevelError, e.Level)
	assert.Equal(t, "ai_summary", e.Data["section"])
	assert.ErrorIs(t, e.Data["error"].(error), ErrInvalidInstanceAISummarySettings)
}

func TestImportLegacyAISummarySettings_ConcurrentLoserAndErrors(t *testing.T) {
	t.Run("concurrent loser", func(t *testing.T) {
		repo := repomocks.NewMockInstanceAISummarySettingsRepository(t)
		logger, logs := logtest.New()
		repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAISummarySettingsNotFound)
		repo.EXPECT().InsertIfAbsentAudited(mock.Anything, mock.Anything, mock.Anything).Return(false, nil)

		res := ImportLegacyAISummarySettings(context.Background(), repo, tunedLegacyAISummary(), false, logger)

		assert.Equal(t, LegacySettingsImportResult{}, res)
		assert.Empty(t, warnings(logs))
	})
	t.Run("read error", func(t *testing.T) {
		repo := repomocks.NewMockInstanceAISummarySettingsRepository(t)
		logger, logs := logtest.New()
		repo.EXPECT().Get(mock.Anything).Return(nil, errors.New("connection refused"))

		res := ImportLegacyAISummarySettings(context.Background(), repo, tunedLegacyAISummary(), false, logger)

		assert.Equal(t, LegacySettingsImportResult{}, res)
		assert.Equal(t, slog.LevelError, entryContaining(t, logs, "Failed to read").Level)
	})
}

// max_top_n: 3, top_n: 5 used to fail boot; now top_n=5 is imported and the
// ignored ceilings get their own warning.
func TestImportLegacyAISummarySettings_IgnoredCeilingsWarn(t *testing.T) {
	repo := repomocks.NewMockInstanceAISummarySettingsRepository(t)
	logger, logs := logtest.New()
	legacy := models.DefaultInstanceAISummarySettings()
	legacy.TopN = 6
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAISummarySettingsNotFound)
	repo.EXPECT().InsertIfAbsentAudited(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(
			_ context.Context, row *models.InstanceAISummarySettings, _ repositories.InstanceAISummarySettingsAuditFunc,
		) (bool, error) {
			assert.Equal(t, 6, row.TopN)
			return true, nil
		})

	res := ImportLegacyAISummarySettings(context.Background(), repo, legacy, true, logger)

	assert.True(t, res.Imported)
	e := entryContaining(t, logs, "max_top_n and ai_summary.max_output_tokens_ceiling are ignored")
	assert.Equal(t, slog.LevelWarn, e.Level)
}

// The ceiling warning does not depend on the rest of the block.
func TestImportLegacyAISummarySettings_CeilingsOnlyWarnWithoutImport(t *testing.T) {
	repo := repomocks.NewMockInstanceAISummarySettingsRepository(t)
	logger, logs := logtest.New()

	res := ImportLegacyAISummarySettings(context.Background(), repo,
		models.DefaultInstanceAISummarySettings(), true, logger)

	assert.Equal(t, LegacySettingsImportResult{}, res)
	assert.Len(t, warnings(logs), 1)
}

// The committed config.example.yaml and the baked config.docker.yaml (no env
// overrides) carry exactly the built-in values: booting with either imports
// nothing, reads nothing and logs nothing (#1201 acceptance criterion).
func TestImportLegacySettings_ShippedConfigsImportNothing(t *testing.T) {
	encryptionKey := strings.Repeat("k", 32)
	for name, path := range map[string]string{
		"config.example.yaml": "../../config.example.yaml",
		"config.docker.yaml":  "../../config.docker.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("ENCRYPTION_KEY", encryptionKey)
			t.Setenv("DB_PASSWORD", "local_password")
			for _, env := range []string{
				"AI_SUMMARY_ENABLED", "AI_SUMMARY_TOP_N", "AI_SUMMARY_REQUEST_TIMEOUT", "AI_SUMMARY_STYLE",
			} {
				t.Setenv(env, "")
			}
			cfg, err := config.Load(path)
			require.NoError(t, err)

			searchRepo := repomocks.NewMockInstanceSearchSettingsRepository(t)
			aiRepo := repomocks.NewMockInstanceAISummarySettingsRepository(t)
			logger, logs := logtest.New()

			assert.Equal(t, LegacySettingsImportResult{},
				ImportLegacySearchSettings(context.Background(), searchRepo, cfg.Search.InstanceValues(), logger))
			assert.Equal(t, LegacySettingsImportResult{},
				ImportLegacyAISummarySettings(context.Background(), aiRepo, cfg.AISummary.InstanceValues(),
					cfg.AISummary.LegacyCeilingsSet(), logger))
			assert.Empty(t, logs.AllEntries())
		})
	}
}

func TestLoggerOrDefault(t *testing.T) {
	assert.Same(t, slog.Default(), loggerOrDefault(nil))
}
