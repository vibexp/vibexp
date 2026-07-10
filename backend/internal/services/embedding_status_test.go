package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

func newStatusService(t *testing.T) (
	*EmbeddingStatusService,
	*repomocks.MockEmbeddingProviderRepository,
	*repomocks.MockEmbeddingBackfillRepository,
) {
	t.Helper()
	providerRepo := repomocks.NewMockEmbeddingProviderRepository(t)
	coverageRepo := repomocks.NewMockEmbeddingBackfillRepository(t)
	svc := NewEmbeddingStatusService(providerRepo, coverageRepo, slog.New(slog.DiscardHandler))
	return svc, providerRepo, coverageRepo
}

// TestGetCoverage_ActiveProvider_DerivesPendingAndPercent verifies the service
// resolves the active model, passes it to the coverage count, and derives pending +
// rounded percentage per type.
func TestGetCoverage_ActiveProvider_DerivesPendingAndPercent(t *testing.T) {
	svc, providerRepo, coverageRepo := newStatusService(t)

	providerRepo.EXPECT().GetActiveProvider(mock.Anything, "team-1").
		Return(&models.EmbeddingProvider{Model: "text-embedding-3-small"}, nil)
	coverageRepo.EXPECT().CountCoverage(mock.Anything, "text-embedding-3-small", "team-1").
		Return([]models.EmbeddingCoverageCount{
			{EntityType: "prompt", Total: 8, Embedded: 6}, // 75%
			{EntityType: "artifact", Total: 3, Embedded: 3},
			{EntityType: "memory", Total: 0, Embedded: 0}, // zero-guard → 0%
		}, nil)

	resp, err := svc.GetCoverage(context.Background(), "team-1")
	require.NoError(t, err)

	assert.True(t, resp.HasActiveProvider)
	require.NotNil(t, resp.ActiveModel)
	assert.Equal(t, "text-embedding-3-small", *resp.ActiveModel)
	require.Len(t, resp.Coverage, 3)

	assert.Equal(t, "prompt", resp.Coverage[0].EntityType)
	assert.Equal(t, int64(2), resp.Coverage[0].Pending)
	assert.Equal(t, 75, resp.Coverage[0].EmbeddedPercent)

	assert.Equal(t, int64(0), resp.Coverage[1].Pending)
	assert.Equal(t, 100, resp.Coverage[1].EmbeddedPercent)

	assert.Equal(t, int64(0), resp.Coverage[2].Pending)
	assert.Equal(t, 0, resp.Coverage[2].EmbeddedPercent)
}

// TestGetCoverage_NoActiveProvider_AllPending verifies a team with no provider is
// reported as all-pending (0%) against an empty model, not an error.
func TestGetCoverage_NoActiveProvider_AllPending(t *testing.T) {
	svc, providerRepo, coverageRepo := newStatusService(t)

	providerRepo.EXPECT().GetActiveProvider(mock.Anything, "team-1").
		Return(nil, repositories.ErrNoActiveEmbeddingProvider)
	// The empty model id is what the repo turns into 0 embedded.
	coverageRepo.EXPECT().CountCoverage(mock.Anything, "", "team-1").
		Return([]models.EmbeddingCoverageCount{
			{EntityType: "prompt", Total: 5, Embedded: 0},
			{EntityType: "artifact", Total: 2, Embedded: 0},
		}, nil)

	resp, err := svc.GetCoverage(context.Background(), "team-1")
	require.NoError(t, err)

	assert.False(t, resp.HasActiveProvider)
	assert.Nil(t, resp.ActiveModel)
	require.Len(t, resp.Coverage, 2)
	assert.Equal(t, int64(5), resp.Coverage[0].Pending)
	assert.Equal(t, 0, resp.Coverage[0].EmbeddedPercent)
	assert.Equal(t, int64(2), resp.Coverage[1].Pending)
}

// TestGetCoverage_ProviderError_Propagates verifies a non-"no provider" error from
// the provider lookup aborts rather than being swallowed.
func TestGetCoverage_ProviderError_Propagates(t *testing.T) {
	svc, providerRepo, _ := newStatusService(t)

	providerRepo.EXPECT().GetActiveProvider(mock.Anything, "team-1").
		Return(nil, errors.New("db down"))

	_, err := svc.GetCoverage(context.Background(), "team-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve active embedding provider")
}

// TestGetCoverage_CountError_Propagates verifies a coverage-count failure is returned.
func TestGetCoverage_CountError_Propagates(t *testing.T) {
	svc, providerRepo, coverageRepo := newStatusService(t)

	providerRepo.EXPECT().GetActiveProvider(mock.Anything, "team-1").
		Return(&models.EmbeddingProvider{Model: "m"}, nil)
	coverageRepo.EXPECT().CountCoverage(mock.Anything, "m", "team-1").
		Return(nil, errors.New("query failed"))

	_, err := svc.GetCoverage(context.Background(), "team-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "count embedding coverage")
}

func TestEmbeddedPercent(t *testing.T) {
	cases := []struct {
		embedded, total int64
		want            int
	}{
		{0, 0, 0},  // zero-guard
		{0, 10, 0}, // none embedded
		{10, 10, 100},
		{6, 8, 75},
		{1, 3, 33}, // 33.33 → 33
		{2, 3, 67}, // 66.66 → 67
		{5, 0, 0},  // defensive: total 0 wins
	}
	for _, c := range cases {
		assert.Equal(t, c.want, embeddedPercent(c.embedded, c.total),
			"embeddedPercent(%d,%d)", c.embedded, c.total)
	}
}
