package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	repoMocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// The happy path (stale card refreshed + persisted in the background) and the
// fetch-failure path (stale card kept, read unaffected) are covered in
// agent_test.go. These tests close the remaining maybeRefreshStaleCard /
// refreshCardInBackground branches deterministically: the dedup guard is
// synchronous by construction, and the persist-failure branch is exercised by
// calling refreshCardInBackground directly (same code the goroutine runs)
// instead of racing a detached goroutine.

// TestMaybeRefreshStaleCard_DedupsInFlightRefresh pins the per-agent dedup
// decision: while a refresh is already in flight for an agent id, another read
// of the same stale agent must NOT spawn a second fetch.
func TestMaybeRefreshStaleCard_DedupsInFlightRefresh(t *testing.T) {
	agentRepo := repoMocks.NewMockAgentRepository(t)
	cardFetcher := &MockAgentCardFetcher{}
	service := createTestAgentService(agentRepo, nil, cardFetcher)

	agent := createTestAgent()
	stale := time.Now().Add(-48 * time.Hour)
	agent.LastSyncedAt = &stale

	// Simulate a refresh already in flight for this agent id.
	service.refreshInFlight.Store(agent.ID, struct{}{})

	service.maybeRefreshStaleCard(agent)

	// The dedup branch returns before spawning any goroutine, so this
	// assertion is race-free: no second fetch may have been started.
	cardFetcher.AssertNotCalled(t, "FetchAgentCard", mock.Anything, mock.Anything, mock.Anything)
	_, stillInFlight := service.refreshInFlight.Load(agent.ID)
	assert.True(t, stillInFlight, "the dedup path must not clear the original refresh's marker")
}

// TestRefreshCardInBackground_PersistFailureIsDroppedAndReleasesDedup pins the
// best-effort contract of the refresh: when the fetched card cannot be
// persisted, the failure is logged and dropped (never surfaced), and the
// in-flight marker is released so a later read can retry the refresh.
func TestRefreshCardInBackground_PersistFailureIsDroppedAndReleasesDedup(t *testing.T) {
	agentRepo := repoMocks.NewMockAgentRepository(t)
	cardFetcher := &MockAgentCardFetcher{}
	service := createTestAgentService(agentRepo, nil, cardFetcher)

	agent := createTestAgent()
	refreshed := createTestAgent().AgentCard
	refreshed.Name = "Refreshed Agent"

	cardFetcher.On("FetchAgentCard", mock.Anything, *agent.CardURL, mock.Anything).
		Return(refreshed, nil).Once()
	agentRepo.On("Update", mock.Anything, mock.Anything).
		Return(fmt.Errorf("db down")).Once()

	// Mirror maybeRefreshStaleCard's bookkeeping, then run the refresh body
	// synchronously — deterministic, no goroutine to wait on.
	service.refreshInFlight.Store(agent.ID, struct{}{})
	service.refreshCardInBackground(agent)

	_, stillInFlight := service.refreshInFlight.Load(agent.ID)
	assert.False(t, stillInFlight,
		"a failed persist must release the dedup marker so the next stale read retries")
	cardFetcher.AssertExpectations(t)
	agentRepo.AssertExpectations(t)
}
