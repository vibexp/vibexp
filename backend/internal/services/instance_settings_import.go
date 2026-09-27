package services

import (
	"context"
	"errors"
	"log/slog"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// The one-release upgrade bridge for config.yaml's deprecated `search:` and
// `ai_summary:` sections (#1201, epic #1196). Since #1198 / #1199 the instance
// defaults are read only from the instance_search_settings /
// instance_ai_summary_settings rows, so an install upgrading with tuned values
// in config.yaml would otherwise fall silently back to the built-in defaults.
// At boot, per section:
//
//	row absent,  block differs from defaults → validate, import (audited), warn
//	row absent,  block equals defaults       → nothing
//	row present, block differs from defaults → warn that the block is ignored
//	row present, block equals defaults       → nothing
//
// "Differs" compares the effective loaded values with the built-in defaults:
// koanf merges defaults() first, so an unset key cannot be told apart from a
// default-valued one. Nothing here is fatal: an invalid block or a repository
// failure logs an ERROR and boot continues. #1203 removes both sections.

// LegacySettingsImportResult reports what one section's import did, for tests.
type LegacySettingsImportResult struct {
	// Imported is true when this run inserted the instance row.
	Imported bool
	// Ignored is true when the block differs from the defaults but a row is
	// already stored, so the row wins.
	Ignored bool
}

// legacySettingsImport is one section's side of the bridge; the decision
// table itself is shared (runLegacySettingsImport).
type legacySettingsImport[V comparable] struct {
	// section is the config.yaml section name, e.g. "search".
	section string
	// adminPage names where the settings are edited now.
	adminPage string
	defaults  V
	validate  func(V) error
	// exists reports whether the instance row is stored.
	exists func(ctx context.Context) (bool, error)
	// insert stores the values with an import audit entry when no row exists,
	// reporting whether it did.
	insert func(ctx context.Context, v V) (bool, error)
}

// ImportLegacySearchSettings runs the bridge for config.yaml's `search:`
// section. legacy is the block's effective values (config.SearchConfig's
// InstanceValues).
func ImportLegacySearchSettings(
	ctx context.Context, repo repositories.InstanceSearchSettingsRepository,
	legacy models.InstanceSearchSettingsValues, logger *slog.Logger,
) LegacySettingsImportResult {
	return runLegacySettingsImport(ctx, loggerOrDefault(logger), legacySettingsImport[models.InstanceSearchSettingsValues]{
		section:   "search",
		adminPage: "Admin → Settings → Search",
		defaults:  BuiltInSearchDefaults(),
		validate:  ValidateInstanceSearchSettings,
		exists: func(ctx context.Context) (bool, error) {
			_, err := repo.Get(ctx)
			return rowExists(err, repositories.ErrInstanceSearchSettingsNotFound)
		},
		insert: func(ctx context.Context, v models.InstanceSearchSettingsValues) (bool, error) {
			row := &models.InstanceSearchSettings{
				RecencyRankingEnabled: v.RecencyRankingEnabled,
				RankWeightRelevance:   v.RankWeightRelevance,
				RankWeightCreated:     v.RankWeightCreated,
				RankWeightUpdated:     v.RankWeightUpdated,
				RankHalfLifeDays:      v.RankHalfLifeDays,
				RankCandidateCap:      v.RankCandidateCap,
			}
			return repo.InsertIfAbsentAudited(ctx, row,
				instanceSearchAuditFunc(models.InstanceSettingsAuditActionImport, ""))
		},
	}, legacy)
}

// ImportLegacyAISummarySettings runs the bridge for config.yaml's
// `ai_summary:` section. legacy is the block's effective values
// (config.AISummaryConfig's InstanceValues); legacyCeilingsSet reports whether
// the ignored ai_summary.max_top_n / max_output_tokens_ceiling keys carry a
// non-default value, which is logged but never imported.
func ImportLegacyAISummarySettings(
	ctx context.Context, repo repositories.InstanceAISummarySettingsRepository,
	legacy models.InstanceAISummarySettingsValues, legacyCeilingsSet bool, logger *slog.Logger,
) LegacySettingsImportResult {
	logger = loggerOrDefault(logger)
	if legacyCeilingsSet {
		logger.Warn("config.yaml ai_summary.max_top_n and ai_summary.max_output_tokens_ceiling are ignored: "+
			"they no longer bound anything and are not imported; remove them",
			"section", "ai_summary")
	}
	return runLegacySettingsImport(ctx, logger, legacySettingsImport[models.InstanceAISummarySettingsValues]{
		section:   "ai_summary",
		adminPage: "Admin → Settings → AI Summary",
		defaults:  models.DefaultInstanceAISummarySettings(),
		validate:  ValidateInstanceAISummarySettings,
		exists: func(ctx context.Context) (bool, error) {
			_, err := repo.Get(ctx)
			return rowExists(err, repositories.ErrInstanceAISummarySettingsNotFound)
		},
		insert: func(ctx context.Context, v models.InstanceAISummarySettingsValues) (bool, error) {
			row := &models.InstanceAISummarySettings{
				Enabled:           v.Enabled,
				TopN:              v.TopN,
				Style:             v.Style,
				MaxOutputTokens:   v.MaxOutputTokens,
				PerDocumentChars:  v.PerDocumentChars,
				TotalContextChars: v.TotalContextChars,
				RequestTimeout:    v.RequestTimeout,
			}
			return repo.InsertIfAbsentAudited(ctx, row,
				instanceAISummaryAuditFunc(models.InstanceSettingsAuditActionImport, ""))
		},
	}, legacy)
}

// runLegacySettingsImport applies the decision table to one section.
func runLegacySettingsImport[V comparable](
	ctx context.Context, logger *slog.Logger, imp legacySettingsImport[V], legacy V,
) LegacySettingsImportResult {
	if legacy == imp.defaults {
		// No row gives the same result as importing these values, and a stored
		// row is authoritative either way: nothing to do or say.
		return LegacySettingsImportResult{}
	}

	exists, err := imp.exists(ctx)
	if err != nil {
		logger.Error("Failed to read the instance settings; skipping the config.yaml import",
			"section", imp.section, "error", err)
		return LegacySettingsImportResult{}
	}
	if exists {
		logger.Warn("The config.yaml section is ignored: the instance settings stored in the database are "+
			"authoritative; edit them under "+imp.adminPage+" and remove the section",
			"section", imp.section)
		return LegacySettingsImportResult{Ignored: true}
	}

	if invalid := imp.validate(legacy); invalid != nil {
		logger.Error("The config.yaml section is invalid; nothing was imported and the built-in defaults apply",
			"section", imp.section, "error", invalid)
		return LegacySettingsImportResult{}
	}

	inserted, err := imp.insert(ctx, legacy)
	if err != nil {
		logger.Error("Failed to import the config.yaml section",
			"section", imp.section, "error", err)
		return LegacySettingsImportResult{}
	}
	if !inserted {
		// Another replica imported first, or an admin saved in between: the row
		// that exists wins, and its writer owns the audit entry.
		logger.Info("The instance settings were stored concurrently; the config.yaml section was not imported",
			"section", imp.section)
		return LegacySettingsImportResult{}
	}

	logger.Warn("Imported the config.yaml section into the database. The section is deprecated, ignored from "+
		"now on and removed in the next minor release; edit the settings under "+imp.adminPage+
		" and remove the section after this boot",
		"section", imp.section)
	return LegacySettingsImportResult{Imported: true}
}

// rowExists maps a singleton repository Get error: nil means stored, notFound
// means absent, anything else is a read failure.
func rowExists(err, notFound error) (bool, error) {
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, notFound):
		return false, nil
	default:
		return false, err
	}
}

func loggerOrDefault(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger
}
