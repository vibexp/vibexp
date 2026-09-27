//go:build integration

package postgres

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Up/down round-trip for migration 022_instance_search_ai_summary_settings
// (#1197), on its OWN scratch database: this test migrates DOWN, which would
// corrupt the shared integrationDB.
//
// Beyond "it applies", it pins the singleton on both tables at the database
// level, exercises every CHECK at and just past its boundary, SET NULL on the
// editor, and that the down migration drops POPULATED tables.

// validInstanceSearchRow and validInstanceAISummaryRow are rows every CHECK
// accepts; each case below overrides one or two columns.
func validInstanceSearchRow() map[string]any {
	return map[string]any{
		"recency_ranking_enabled": true,
		"rank_weight_relevance":   0.7,
		"rank_weight_created":     0.1,
		"rank_weight_updated":     0.2,
		"rank_half_life_days":     30.0,
		"rank_candidate_cap":      200,
	}
}

func validInstanceAISummaryRow() map[string]any {
	return map[string]any{
		"enabled":             true,
		"top_n":               5,
		"style":               "balanced",
		"max_output_tokens":   1024,
		"per_document_chars":  4000,
		"total_context_chars": 20000,
		"request_timeout_ms":  30000,
	}
}

// insertInstanceSettingsRow inserts row into table, with overrides applied, in
// a deterministic column order.
func insertInstanceSettingsRow(db *sql.DB, table string, row, overrides map[string]any) error {
	merged := make(map[string]any, len(row))
	for k, v := range row {
		merged[k] = v
	}
	for k, v := range overrides {
		merged[k] = v
	}
	cols := make([]string, 0, len(merged))
	for k := range merged {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	placeholders := make([]string, len(cols))
	args := make([]any, len(cols))
	for i, c := range cols {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = merged[c]
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", // #nosec G201 -- test-only, fixed identifiers
		table, strings.Join(cols, ", "), strings.Join(placeholders, ", "))
	_, err := db.Exec(query, args...)
	return err
}

type instanceSettingsCheckCase struct {
	name      string
	overrides map[string]any
	valid     bool
}

// runInstanceSettingsChecks tries each case against an empty table, so the
// singleton never masks the CHECK under test.
func runInstanceSettingsChecks(
	t *testing.T, db *sql.DB, table string, row map[string]any, cases []instanceSettingsCheckCase,
) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.Exec("DELETE FROM " + table) // #nosec G202 -- test-only, fixed identifier
			require.NoError(t, err)

			err = insertInstanceSettingsRow(db, table, row, tc.overrides)
			if tc.valid {
				require.NoError(t, err)
				return
			}
			requirePQCode(t, err, pqCheckViolation)
		})
	}
	_, err := db.Exec("DELETE FROM " + table) // #nosec G202 -- test-only, fixed identifier
	require.NoError(t, err)
}

var instanceSearchCheckCases = []instanceSettingsCheckCase{
	{"a valid row", nil, true},
	{"negative relevance weight", map[string]any{"rank_weight_relevance": -0.1}, false},
	{"negative created weight", map[string]any{"rank_weight_created": -0.1}, false},
	{"negative updated weight", map[string]any{"rank_weight_updated": -0.1}, false},
	{"all-zero weights", map[string]any{
		"rank_weight_relevance": 0.0, "rank_weight_created": 0.0, "rank_weight_updated": 0.0,
	}, false},
	{"a single non-zero weight", map[string]any{
		"rank_weight_relevance": 1.0, "rank_weight_created": 0.0, "rank_weight_updated": 0.0,
	}, true},
	{"half-life 0", map[string]any{"rank_half_life_days": 0.0}, false},
	{"half-life 36500", map[string]any{"rank_half_life_days": 36500.0}, true},
	{"half-life 36501", map[string]any{"rank_half_life_days": 36501.0}, false},
	{"candidate cap 0", map[string]any{"rank_candidate_cap": 0}, false},
	{"candidate cap 1", map[string]any{"rank_candidate_cap": 1}, true},
	{"candidate cap 5000", map[string]any{"rank_candidate_cap": 5000}, true},
	{"candidate cap 5001", map[string]any{"rank_candidate_cap": 5001}, false},
}

var instanceAISummaryCheckCases = []instanceSettingsCheckCase{
	{"a valid row", nil, true},
	{"top_n 0", map[string]any{"top_n": 0}, false},
	{"top_n 1", map[string]any{"top_n": 1}, true},
	{"top_n 10", map[string]any{"top_n": 10}, true},
	{"top_n 11", map[string]any{"top_n": 11}, false},
	{"max_output_tokens 0", map[string]any{"max_output_tokens": 0}, false},
	{"max_output_tokens 32768", map[string]any{"max_output_tokens": 32768}, true},
	{"max_output_tokens 32769", map[string]any{"max_output_tokens": 32769}, false},
	{"style concise", map[string]any{"style": "concise"}, true},
	{"style detailed", map[string]any{"style": "detailed"}, true},
	{"an unknown style", map[string]any{"style": "verbose"}, false},
	{"per_document_chars 0", map[string]any{"per_document_chars": 0}, false},
	{"total_context_chars below per_document_chars", map[string]any{
		"per_document_chars": 4000, "total_context_chars": 3999,
	}, false},
	{"total_context_chars equal to per_document_chars", map[string]any{
		"per_document_chars": 4000, "total_context_chars": 4000,
	}, true},
	{"total_context_chars 0", map[string]any{"total_context_chars": 0}, false},
	{"request_timeout_ms 0", map[string]any{"request_timeout_ms": 0}, false},
	{"request_timeout_ms -1", map[string]any{"request_timeout_ms": -1}, false},
}

func TestMigration022_InstanceSearchAISummarySettings_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)
	tables := map[string]map[string]any{
		"instance_search_settings":     validInstanceSearchRow(),
		"instance_ai_summary_settings": validInstanceAISummaryRow(),
	}

	// 1. The version immediately before this migration: nothing exists yet.
	require.NoError(t, m.Migrate(21), "migrate to 021")
	for table := range tables {
		require.False(t, tableExists(t, db, table), "fixture: %s must not exist before 022", table)
	}

	// 2. Apply 022.
	require.NoError(t, m.Migrate(22), "migrate to 022")

	for table, row := range tables {
		t.Run(table+" is a singleton", func(t *testing.T) {
			assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM "+table))

			require.NoError(t, insertInstanceSettingsRow(db, table, row, nil))
			assert.Equal(t, 1, countRows(t, db,
				"SELECT count(*) FROM "+table+" WHERE id AND version = 1 AND updated_by IS NULL"),
				"a first row takes the defaults")

			requirePQCode(t, insertInstanceSettingsRow(db, table, row, nil), "23505")
			assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM "+table))

			_, err := db.Exec("DELETE FROM " + table) // #nosec G202 -- test-only, fixed identifier
			require.NoError(t, err)
			requirePQCode(t, insertInstanceSettingsRow(db, table, row, map[string]any{"id": false}),
				pqCheckViolation)
		})
	}

	t.Run("instance_search_settings CHECKs", func(t *testing.T) {
		runInstanceSettingsChecks(t, db, "instance_search_settings",
			validInstanceSearchRow(), instanceSearchCheckCases)
	})

	t.Run("instance_ai_summary_settings CHECKs", func(t *testing.T) {
		runInstanceSettingsChecks(t, db, "instance_ai_summary_settings",
			validInstanceAISummaryRow(), instanceAISummaryCheckCases)
	})

	t.Run("instance_ai_summary_settings has no max_top_n or ceiling column", func(t *testing.T) {
		assert.Equal(t, 0, countRows(t, db,
			`SELECT count(*) FROM information_schema.columns
			  WHERE table_schema = 'public' AND table_name = 'instance_ai_summary_settings'
			    AND column_name IN ('max_top_n', 'max_output_tokens_ceiling')`))
	})

	for table, row := range tables {
		t.Run(table+": deleting the editor keeps the row and blanks updated_by", func(t *testing.T) {
			userID := uuid.New().String()
			_, err := db.Exec("INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
				userID, "settings-"+userID[:8]+"@example.com", "Settings Fixture")
			require.NoError(t, err)
			require.NoError(t, insertInstanceSettingsRow(db, table, row, map[string]any{"updated_by": userID}))

			_, err = db.Exec("DELETE FROM users WHERE id = $1", userID)
			require.NoError(t, err)

			assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM "+table+" WHERE updated_by IS NULL"))
		})
	}

	// 3. Roll back with both tables populated.
	for table := range tables {
		require.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM "+table),
			"fixture: %s must hold its row before rolling back", table)
	}
	require.NoError(t, m.Migrate(21), "migrate down to 021")
	for table := range tables {
		assert.False(t, tableExists(t, db, table), "down must drop the populated %s", table)
	}
	assert.True(t, tableExists(t, db, "instance_settings_audit"), "rolling back 022 must leave 021 in place")

	// 4. Re-apply: an operator who rolls back must be able to roll forward.
	require.NoError(t, m.Migrate(22), "re-apply 022")
	for table, row := range tables {
		assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM "+table), "the re-applied %s starts empty", table)
		require.NoError(t, insertInstanceSettingsRow(db, table, row, nil), "the re-applied %s is usable", table)
	}
}
