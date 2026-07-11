package services

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

var sampleReplyArtifacts = []map[string]interface{}{
	{"artifactId": "a1", "parts": []interface{}{map[string]interface{}{"text": "the answer"}}},
}

// syncTestFixture wires the common mocks for a non-streaming invocation whose
// execution row is created as "exec-1".
type syncTestFixture struct {
	service   *AgentInvocationService
	agentRepo *mocks.MockAgentRepository
	execRepo  *mocks.MockAgentExecutionRepository
	a2aClient *MockA2AHTTPClient
	agent     *models.Agent
	input     map[string]interface{}
}

func newSyncTestFixture(t *testing.T) *syncTestFixture {
	t.Helper()
	agentRepo := new(mocks.MockAgentRepository)
	execRepo := new(mocks.MockAgentExecutionRepository)
	a2aClient := new(MockA2AHTTPClient)
	streamProcessor := new(MockA2AStreamProcessor)
	service := NewAgentInvocationService(agentRepo, execRepo, a2aClient, streamProcessor, slog.New(slog.DiscardHandler))

	agent := &models.Agent{ID: "agent-1", UserID: "user-1", Status: "active", AgentCard: &models.AgentCard{}}
	input := map[string]interface{}{"text": "question"}

	agentRepo.On("GetByIDCrossTeam", mock.Anything, "user-1", "agent-1").Return(agent, nil)
	a2aClient.On("SupportsStreaming", agent).Return(false)
	execRepo.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*models.AgentExecution).ID = "exec-1"
	}).Return(nil)
	execRepo.On("UpdateConversationID", mock.Anything, "exec-1", "exec-1").Return(nil)
	execRepo.On("UpdateTaskInfo", mock.Anything, "exec-1", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	agentRepo.On("UpdateExecutionStats", mock.Anything, "agent-1", mock.Anything, mock.Anything).Return(nil).Maybe()

	return &syncTestFixture{service, agentRepo, execRepo, a2aClient, agent, input}
}

func TestInvokeAgentSync_MessageReplyPersisted(t *testing.T) {
	f := newSyncTestFixture(t)
	reply := &models.AgentExecution{Status: "completed", Duration: intPtr(10), Artifacts: sampleReplyArtifacts}
	f.a2aClient.On("InvokeAgent", mock.Anything, f.agent, f.input, mock.Anything).Return(reply, nil)

	f.execRepo.On("Update", mock.Anything, mock.MatchedBy(func(e *models.AgentExecution) bool {
		return e.ID == "exec-1" && e.Status == "success"
	})).Return(nil)
	f.execRepo.On("UpdateArtifacts", mock.Anything, "exec-1", sampleReplyArtifacts).Return(nil).Once()

	result, err := f.service.InvokeAgent(context.Background(), "user-1", "agent-1", f.input, nil)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	f.execRepo.AssertExpectations(t)
	f.a2aClient.AssertExpectations(t)
}

func TestInvokeAgentSync_NonTerminalTask_PollsToCompleted(t *testing.T) {
	restore := shortenSyncPoll()
	defer restore()

	f := newSyncTestFixture(t)
	working := &models.AgentExecution{Status: "working", TaskID: strPtr("t1"), CurrentState: strPtr("TASK_STATE_WORKING")}
	completed := &models.AgentExecution{
		Status: "completed", TaskID: strPtr("t1"), CurrentState: strPtr("TASK_STATE_COMPLETED"),
		Duration: intPtr(20), Artifacts: sampleReplyArtifacts,
	}

	f.a2aClient.On("InvokeAgent", mock.Anything, f.agent, f.input, mock.Anything).Return(working, nil)
	f.a2aClient.On("GetTask", mock.Anything, f.agent, "t1").Return(working, nil).Once()
	f.a2aClient.On("GetTask", mock.Anything, f.agent, "t1").Return(completed, nil).Once()

	f.execRepo.On("Update", mock.Anything, mock.MatchedBy(func(e *models.AgentExecution) bool {
		return e.Status == "success"
	})).Return(nil)
	f.execRepo.On("UpdateArtifacts", mock.Anything, "exec-1", sampleReplyArtifacts).Return(nil).Once()

	result, err := f.service.InvokeAgent(context.Background(), "user-1", "agent-1", f.input, nil)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	f.a2aClient.AssertExpectations(t)
	f.execRepo.AssertExpectations(t)
}

func TestInvokeAgentSync_NonTerminalTask_PollsToFailed(t *testing.T) {
	restore := shortenSyncPoll()
	defer restore()

	f := newSyncTestFixture(t)
	working := &models.AgentExecution{Status: "working", TaskID: strPtr("t1"), CurrentState: strPtr("TASK_STATE_WORKING")}
	failed := &models.AgentExecution{Status: "failed", TaskID: strPtr("t1"), CurrentState: strPtr("TASK_STATE_FAILED"), Duration: intPtr(5)}

	f.a2aClient.On("InvokeAgent", mock.Anything, f.agent, f.input, mock.Anything).Return(working, nil)
	f.a2aClient.On("GetTask", mock.Anything, f.agent, "t1").Return(failed, nil).Once()

	f.execRepo.On("Update", mock.Anything, mock.MatchedBy(func(e *models.AgentExecution) bool {
		return e.Status == "error"
	})).Return(nil)

	result, err := f.service.InvokeAgent(context.Background(), "user-1", "agent-1", f.input, nil)

	require.NoError(t, err)
	assert.Equal(t, "error", result.Status)
	f.a2aClient.AssertExpectations(t)
	f.execRepo.AssertExpectations(t)
}

func TestInvokeAgentSync_PollTimeout(t *testing.T) {
	restore := shortenSyncPoll()
	defer restore()
	syncTaskPollDeadline = 25 * time.Millisecond

	f := newSyncTestFixture(t)
	working := &models.AgentExecution{Status: "working", TaskID: strPtr("t1"), CurrentState: strPtr("TASK_STATE_WORKING")}

	f.a2aClient.On("InvokeAgent", mock.Anything, f.agent, f.input, mock.Anything).Return(working, nil)
	f.a2aClient.On("GetTask", mock.Anything, f.agent, "t1").Return(working, nil)

	// Timeout finalizes the execution as an error.
	f.execRepo.On("Update", mock.Anything, mock.MatchedBy(func(e *models.AgentExecution) bool {
		return e.Status == "error" && e.Error != nil
	})).Return(nil)

	result, err := f.service.InvokeAgent(context.Background(), "user-1", "agent-1", f.input, nil)

	require.NoError(t, err)
	assert.Equal(t, "error", result.Status)
	require.NotNil(t, result.Error)
	assert.Contains(t, *result.Error, "did not reach a terminal state")
	f.execRepo.AssertExpectations(t)
}

// shortenSyncPoll makes the poll loop fast and deterministic for tests.
func shortenSyncPoll() func() {
	origInit, origMax, origDeadline := syncTaskPollInitialBackoff, syncTaskPollMaxBackoff, syncTaskPollDeadline
	syncTaskPollInitialBackoff = time.Millisecond
	syncTaskPollMaxBackoff = 2 * time.Millisecond
	syncTaskPollDeadline = time.Second
	return func() {
		syncTaskPollInitialBackoff, syncTaskPollMaxBackoff, syncTaskPollDeadline = origInit, origMax, origDeadline
	}
}
