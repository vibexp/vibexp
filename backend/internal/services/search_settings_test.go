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
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	servicemocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// resolverInstanceValues is what the instance resolver hands back. The values
// are deliberately distinct from storedProfile's and from the built-in
// defaults, so a test can tell which one came back.
func resolverInstanceValues() models.InstanceSearchSettingsValues {
	return models.InstanceSearchSettingsValues{
		RecencyRankingEnabled: true,
		RankWeightRelevance:   0.7,
		RankWeightCreated:     0.2,
		RankWeightUpdated:     0.1,
		RankHalfLifeDays:      30,
		RankCandidateCap:      350,
	}
}

// instanceDefaults is resolverInstanceValues as a ranking config.
func instanceDefaults() services.SearchRankingConfig {
	return services.SearchRankingConfig{
		Enabled:         true,
		WeightRelevance: 0.7,
		WeightCreated:   0.2,
		WeightUpdated:   0.1,
		HalfLife:        30 * 24 * time.Hour,
		CandidateCap:    350,
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

func testLogger(logs *bytes.Buffer) *slog.Logger {
	handler := slog.Handler(slog.DiscardHandler)
	if logs != nil {
		handler = slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	return slog.New(handler)
}

func newResolver(t *testing.T, logs *bytes.Buffer) (
	*services.TeamSearchSettingsResolver, *repomocks.MockTeamSearchSettingsRepository,
) {
	t.Helper()
	repo := repomocks.NewMockTeamSearchSettingsRepository(t)
	instance := servicemocks.NewMockInstanceSearchSettingsResolver(t)
	instance.EXPECT().Resolve(mock.Anything).Return(resolverInstanceValues())
	return services.NewTeamSearchSettingsResolver(repo, instance, testLogger(logs)), repo
}

// newResolverOverInstanceRepo wires the team resolver to the REAL instance
// service over a mocked instance repository, so a test can change the instance
// row between two searches.
func newResolverOverInstanceRepo(t *testing.T) (
	*services.TeamSearchSettingsResolver,
	*repomocks.MockTeamSearchSettingsRepository,
	*repomocks.MockInstanceSearchSettingsRepository,
) {
	t.Helper()
	teamRepo := repomocks.NewMockTeamSearchSettingsRepository(t)
	instanceRepo := repomocks.NewMockInstanceSearchSettingsRepository(t)
	instance := services.NewInstanceSearchSettingsService(instanceRepo, testLogger(nil))
	return services.NewTeamSearchSettingsResolver(teamRepo, instance, testLogger(nil)), teamRepo, instanceRepo
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
		"CandidateCap must always come from the instance defaults, never from the team row")
}

func TestTeamSearchSettingsResolver_RepositoryErrorFailsOpen(t *testing.T) {
	var logs bytes.Buffer
	resolver, repo := newResolver(t, &logs)
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, errors.New("connection refused"))

	got := resolver.Resolve(context.Background(), testTeamID)

	assert.Equal(t, instanceDefaults(), got, "a settings read failure must not change ranking")
	assertWarnLoggedWithTeamID(t, logs.String(), testTeamID)
}

// A change to the instance row applies to the very next search of a team with
// no stored profile: nothing is captured at wire time and nothing is cached.
func TestTeamSearchSettingsResolver_InstanceChangeAppliesToNextSearch(t *testing.T) {
	resolver, teamRepo, instanceRepo := newResolverOverInstanceRepo(t)
	teamRepo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, nil).Times(2)
	instanceRepo.EXPECT().Get(mock.Anything).
		Return(nil, repositories.ErrInstanceSearchSettingsNotFound).Once()
	instanceRepo.EXPECT().Get(mock.Anything).Return(&models.InstanceSearchSettings{
		RecencyRankingEnabled: true,
		RankWeightRelevance:   0.2,
		RankWeightCreated:     0.4,
		RankWeightUpdated:     0.4,
		RankHalfLifeDays:      14,
		RankCandidateCap:      900,
	}, nil).Once()

	before := resolver.Resolve(context.Background(), testTeamID)
	after := resolver.Resolve(context.Background(), testTeamID)

	assert.Equal(t, services.SearchRankingConfig{
		Enabled: false, WeightRelevance: 0.5, WeightCreated: 0.3, WeightUpdated: 0.2,
		HalfLife: 90 * 24 * time.Hour, CandidateCap: 200,
	}, before, "with no instance row the built-in defaults apply")
	assert.Equal(t, services.SearchRankingConfig{
		Enabled: true, WeightRelevance: 0.2, WeightCreated: 0.4, WeightUpdated: 0.4,
		HalfLife: 14 * 24 * time.Hour, CandidateCap: 900,
	}, after, "the stored instance row must apply on the next search, with no restart")
}

// A team with its own profile keeps its weights, half-life and enabled flag
// whatever the instance row says; only the instance-owned cap follows it.
func TestTeamSearchSettingsResolver_TeamProfileUnaffectedByInstanceChange(t *testing.T) {
	resolver, teamRepo, instanceRepo := newResolverOverInstanceRepo(t)
	teamRepo.EXPECT().Get(mock.Anything, testTeamID).Return(storedProfile(testTeamID), nil).Times(2)
	instanceRepo.EXPECT().Get(mock.Anything).
		Return(nil, repositories.ErrInstanceSearchSettingsNotFound).Once()
	instanceRepo.EXPECT().Get(mock.Anything).Return(&models.InstanceSearchSettings{
		RecencyRankingEnabled: true,
		RankWeightRelevance:   0.2,
		RankWeightCreated:     0.4,
		RankWeightUpdated:     0.4,
		RankHalfLifeDays:      14,
		RankCandidateCap:      900,
	}, nil).Once()

	before := resolver.Resolve(context.Background(), testTeamID)
	after := resolver.Resolve(context.Background(), testTeamID)

	team := services.SearchRankingConfig{
		Enabled: false, WeightRelevance: 0.1, WeightCreated: 0.6, WeightUpdated: 0.3,
		HalfLife: 7 * 24 * time.Hour,
	}
	team.CandidateCap = 200
	assert.Equal(t, team, before)
	team.CandidateCap = 900
	assert.Equal(t, team, after, "only the instance-owned candidate cap may follow the instance row")
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
