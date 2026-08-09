package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// NewResourceFreshnessState is what every resource payload goes through, so
// both its nil handling and its wire shape are load-bearing (#735).
func TestNewResourceFreshnessState(t *testing.T) {
	t.Run("nil row yields nil so callers can assign unconditionally", func(t *testing.T) {
		assert.Nil(t, models.NewResourceFreshnessState(nil))
	})

	t.Run("projects the fields a client needs", func(t *testing.T) {
		since := time.Now().UTC().Truncate(time.Second)
		state := models.NewResourceFreshnessState(&models.ResourceFreshness{
			TeamID:         "team-1",
			ProjectID:      "project-1",
			ResourceType:   "artifact",
			ResourceID:     "art-1",
			Status:         models.FreshnessStatusStale,
			MatchedRuleIDs: []string{"rule-1", "rule-2"},
			Since:          since,
			Reason:         models.FreshnessReasonRuleRun,
		})

		require.NotNil(t, state)
		assert.Equal(t, models.FreshnessStatusStale, state.Status)
		assert.Equal(t, since, state.Since)
		assert.Equal(t, []string{"rule-1", "rule-2"}, []string(state.MatchedRuleIDs))
		assert.Equal(t, models.FreshnessReasonRuleRun, state.Reason)
	})

	t.Run("matched_rule_ids serializes as [] never null", func(t *testing.T) {
		// The spec marks it required, so a null would break clients that
		// iterate it. JSONArray is what guarantees this by construction.
		state := models.NewResourceFreshnessState(&models.ResourceFreshness{
			Status: models.FreshnessStatusStale, Reason: models.FreshnessReasonAccessed,
		})

		encoded, err := json.Marshal(state)

		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"matched_rule_ids":[]`)
	})
}
