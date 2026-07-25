package services_test

import (
	"bytes"
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

	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
)

// instanceDefaults is the ranking config a resolver falls back to. The values
// are deliberately distinct from storedProfile's so a test can tell which one
// came back.
func instanceDefaults() services.SearchRankingConfig {
	return services.SearchRankingConfig{
		Enabled:         true,
		WeightRelevance: 0.7,
		WeightCreated:   0.2,
		WeightUpdated:   0.1,
		HalfLife:        30 * 24 * time.Hour,
		CandidateCap:    200,
	}
}

func storedProfile(teamID string) *models.TeamSearchSettings {
	return &models.TeamSearchSettings{
		TeamID:                teamID,
		RecencyRankingEnabled: false,
		RankWeightRelevance:   0.1,
		RankWeightCreated:     0.6,
		RankWeightUpdated:     0.3,
		RankHalfLifeDays:      7,
	}
}

func newResolver(t *testing.T, logs *bytes.Buffer) (
	*services.TeamSearchSettingsResolver, *repomocks.MockTeamSearchSettingsRepository,
) {
	t.Helper()
	repo := repomocks.NewMockTeamSearchSettingsRepository(t)
	handler := slog.Handler(slog.DiscardHandler)
	if logs != nil {
		handler = slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	return services.NewTeamSearchSettingsResolver(repo, instanceDefaults(), slog.New(handler)), repo
}

func TestTeamSearchSettingsResolver_NoRowReturnsInstanceDefaults(t *testing.T) {
	resolver, repo := newResolver(t, nil)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, nil)

	got := resolver.Resolve(context.Background(), testTeamID)

	assert.Equal(t, instanceDefaults(), got, "a team with no override inherits the instance defaults verbatim")
}

func TestTeamSearchSettingsResolver_StoredRowIsUsed(t *testing.T) {
	resolver, repo := newResolver(t, nil)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(storedProfile(testTeamID), nil)

	got := resolver.Resolve(context.Background(), testTeamID)

	assert.False(t, got.Enabled)
	assert.InDelta(t, 0.1, got.WeightRelevance, 1e-9)
	assert.InDelta(t, 0.6, got.WeightCreated, 1e-9)
	assert.InDelta(t, 0.3, got.WeightUpdated, 1e-9)
	assert.Equal(t, 7*24*time.Hour, got.HalfLife, "days must convert to a duration")
}

// The candidate cap is a cost/isolation boundary: it bounds how many rows are
// pulled from Postgres and sorted in memory per query, so no team may widen it.
func TestTeamSearchSettingsResolver_StoredRowCannotChangeCandidateCap(t *testing.T) {
	resolver, repo := newResolver(t, nil)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(storedProfile(testTeamID), nil)

	got := resolver.Resolve(context.Background(), testTeamID)

	assert.Equal(t, instanceDefaults().CandidateCap, got.CandidateCap,
		"CandidateCap must always come from the instance config, never from the team row")
}

func TestTeamSearchSettingsResolver_RepositoryErrorFailsOpen(t *testing.T) {
	var logs bytes.Buffer
	resolver, repo := newResolver(t, &logs)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, errors.New("connection refused"))

	got := resolver.Resolve(context.Background(), testTeamID)

	assert.Equal(t, instanceDefaults(), got, "a settings read failure must not change ranking")
	assertWarnLoggedWithTeamID(t, logs.String(), testTeamID)
}

// assertWarnLoggedWithTeamID pins the observability contract: the fail-open path
// must be greppable, so it logs at warn and carries team_id. Logging it at debug
// would hide a real misconfiguration behind a silently-default ranking.
func assertWarnLoggedWithTeamID(t *testing.T, output, teamID string) {
	t.Helper()
	require.NotEmpty(t, output, "fail-open must emit a log line")

	var found bool
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["level"] == "WARN" && entry["team_id"] == teamID {
			found = true
		}
	}
	assert.True(t, found, "expected a WARN log carrying team_id=%s, got: %s", teamID, output)
}
