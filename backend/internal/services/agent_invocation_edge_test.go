package services

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// TestMapA2AStatusToDBStatus pins the full A2A-status -> DB-status mapping,
// including the default branch: any status the mapper does not recognize must
// land as "error", never leak through to the agent_executions_status_check
// constraint unmapped.
func TestMapA2AStatusToDBStatus(t *testing.T) {
	svc := NewAgentInvocationService(nil, nil, nil, nil, nil, slog.New(slog.DiscardHandler))

	cases := []struct {
		a2aStatus string
		want      string
	}{
		{"completed", "success"},
		{"cancelled", "cancelled"},
		{"working", "running"},
		{"submitted", "running"},
		{"running", "running"},
		{"error", "error"},
		{"failed", "error"},
		{"rejected", "error"},
		// Unknown / default branch: anything unrecognized maps to "error".
		{"input-required", "error"},
		{"auth-required", "error"},
		{"", "error"},
	}
	for _, tc := range cases {
		t.Run("status "+tc.a2aStatus, func(t *testing.T) {
			assert.Equal(t, tc.want, svc.mapA2AStatusToDBStatus(tc.a2aStatus))
		})
	}
}

// TestInvokeAgent_ConversationLookupFails pins that a failing conversation
// lookup aborts the invocation BEFORE any execution record is created: a bad
// conversation id must not leave an orphaned execution row behind.
func TestInvokeAgent_ConversationLookupFails(t *testing.T) {
	agentRepo := new(mocks.MockAgentRepository)
	execRepo := new(mocks.MockAgentExecutionRepository)
	eventRepo := new(mocks.MockAgentExecutionEventRepository)
	a2aClient := new(MockA2AHTTPClient)
	svc := NewAgentInvocationService(
		agentRepo, execRepo, eventRepo, a2aClient, new(MockA2AStreamProcessor), slog.New(slog.DiscardHandler),
	)

	agent := &models.Agent{ID: "agent-1", UserID: "user-1", Status: "active"}
	agentRepo.On("GetByIDCrossTeam", mock.Anything, "user-1", "agent-1").Return(agent, nil)
	execRepo.On("GetFirstExecutionInConversation", mock.Anything, "user-1", "conv-1").
		Return(nil, fmt.Errorf("conversation not found"))

	result, err := svc.InvokeAgent(
		context.Background(), "user-1", "agent-1", map[string]interface{}{}, strPtr("conv-1"),
	)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get conversation")
	execRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	a2aClient.AssertNotCalled(t, "InvokeAgent", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestInvokeAgent_EmptyConversationIDStartsNewConversation pins that an
// empty-string conversation id is treated exactly like nil: no conversation
// lookup, no context id sent to the agent, and the execution seeds a fresh
// conversation keyed by its own id.
func TestInvokeAgent_EmptyConversationIDStartsNewConversation(t *testing.T) {
	f := newSyncTestFixture(t)
	reply := &models.AgentExecution{Status: "completed", Duration: intPtr(10)}
	f.a2aClient.On(
		"InvokeAgent", mock.Anything, f.agent, f.input,
		mock.MatchedBy(func(contextID *string) bool { return contextID == nil }),
	).Return(reply, nil)
	f.execRepo.On("Update", mock.Anything, mock.MatchedBy(func(e *models.AgentExecution) bool {
		return e.ID == "exec-1" && e.Status == "success"
	})).Return(nil)

	result, err := f.service.InvokeAgent(context.Background(), "user-1", "agent-1", f.input, strPtr(""))

	require.NoError(t, err)
	require.NotNil(t, result.ConversationID)
	assert.Equal(t, "exec-1", *result.ConversationID,
		"a new conversation must be keyed by the first execution's id")
	f.execRepo.AssertNotCalled(t, "GetFirstExecutionInConversation", mock.Anything, mock.Anything, mock.Anything)
	f.execRepo.AssertExpectations(t)
	f.a2aClient.AssertExpectations(t)
}

// TestInvokeAgent_SetConversationIDFailureIsNonFatal pins that failing to
// persist the fresh conversation id is logged and swallowed: the invocation
// itself still runs to completion.
func TestInvokeAgent_SetConversationIDFailureIsNonFatal(t *testing.T) {
	agentRepo := new(mocks.MockAgentRepository)
	execRepo := new(mocks.MockAgentExecutionRepository)
	eventRepo := new(mocks.MockAgentExecutionEventRepository)
	a2aClient := new(MockA2AHTTPClient)
	svc := NewAgentInvocationService(
		agentRepo, execRepo, eventRepo, a2aClient, new(MockA2AStreamProcessor), slog.New(slog.DiscardHandler),
	)

	agent := &models.Agent{ID: "agent-1", UserID: "user-1", Status: "active"}
	agentRepo.On("GetByIDCrossTeam", mock.Anything, "user-1", "agent-1").Return(agent, nil)
	a2aClient.On("SupportsStreaming", agent).Return(false)
	execRepo.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*models.AgentExecution).ID = "exec-1"
	}).Return(nil)
	execRepo.On("UpdateConversationID", mock.Anything, "exec-1", "exec-1").
		Return(fmt.Errorf("db write failed"))
	a2aClient.On("InvokeAgent", mock.Anything, agent, mock.Anything, mock.Anything).
		Return(&models.AgentExecution{Status: "completed", Duration: intPtr(5)}, nil)
	execRepo.On("Update", mock.Anything, mock.MatchedBy(func(e *models.AgentExecution) bool {
		return e.Status == "success"
	})).Return(nil)
	agentRepo.On("UpdateExecutionStats", mock.Anything, "agent-1", true, 5).Return(nil)

	result, err := svc.InvokeAgent(context.Background(), "user-1", "agent-1", map[string]interface{}{}, nil)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	execRepo.AssertExpectations(t)
}

// TestPersistTaskProgress_FieldMappingAndErrorTolerance pins the snapshot ->
// UpdateTaskInfo mapping (nil pointers become empty strings, set pointers pass
// through) and that a persistence failure is swallowed, never propagated.
func TestPersistTaskProgress_FieldMappingAndErrorTolerance(t *testing.T) {
	execRepo := mocks.NewMockAgentExecutionRepository(t)
	svc := NewAgentInvocationService(
		nil, execRepo, nil, nil, nil, slog.New(slog.DiscardHandler),
	)
	execution := &models.AgentExecution{ID: "exec-9"}

	// Set pointers pass through verbatim.
	execRepo.On("UpdateTaskInfo", mock.Anything, "exec-9", "t1", "c1", "TASK_STATE_WORKING").
		Return(nil).Once()
	svc.persistTaskProgress(context.Background(), execution, &models.AgentExecution{
		TaskID: strPtr("t1"), ContextID: strPtr("c1"), CurrentState: strPtr("TASK_STATE_WORKING"),
	})

	// Nil pointers degrade to empty strings, and a repo failure is tolerated
	// (persistTaskProgress logs and continues — nothing to return, no panic).
	execRepo.On("UpdateTaskInfo", mock.Anything, "exec-9", "", "", "").
		Return(fmt.Errorf("db down")).Once()
	svc.persistTaskProgress(context.Background(), execution, &models.AgentExecution{})
}

// TestInvokeAgentSync_TaskProgressPersistenceFailureDoesNotFailInvocation pins
// the tolerance decision end-to-end through the poll loop: every UpdateTaskInfo
// write failing must not prevent the invocation from reaching "success".
func TestInvokeAgentSync_TaskProgressPersistenceFailureDoesNotFailInvocation(t *testing.T) {
	restore := shortenSyncPoll()
	defer restore()

	agentRepo := new(mocks.MockAgentRepository)
	execRepo := new(mocks.MockAgentExecutionRepository)
	eventRepo := new(mocks.MockAgentExecutionEventRepository)
	a2aClient := new(MockA2AHTTPClient)
	svc := NewAgentInvocationService(
		agentRepo, execRepo, eventRepo, a2aClient, new(MockA2AStreamProcessor), slog.New(slog.DiscardHandler),
	)

	agent := &models.Agent{ID: "agent-1", UserID: "user-1", Status: "active"}
	agentRepo.On("GetByIDCrossTeam", mock.Anything, "user-1", "agent-1").Return(agent, nil)
	a2aClient.On("SupportsStreaming", agent).Return(false)
	execRepo.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*models.AgentExecution).ID = "exec-1"
	}).Return(nil)
	execRepo.On("UpdateConversationID", mock.Anything, "exec-1", "exec-1").Return(nil)

	working := &models.AgentExecution{Status: "working", TaskID: strPtr("t1"), CurrentState: strPtr("TASK_STATE_WORKING")}
	completed := &models.AgentExecution{
		Status: "completed", TaskID: strPtr("t1"), CurrentState: strPtr("TASK_STATE_COMPLETED"), Duration: intPtr(7),
	}
	a2aClient.On("InvokeAgent", mock.Anything, agent, mock.Anything, mock.Anything).Return(working, nil)
	a2aClient.On("GetTask", mock.Anything, agent, "t1").Return(completed, nil)

	// Every live-progress write fails; the invocation must still finish.
	execRepo.On("UpdateTaskInfo", mock.Anything, "exec-1", mock.Anything, mock.Anything, mock.Anything).
		Return(fmt.Errorf("db down"))
	execRepo.On("Update", mock.Anything, mock.MatchedBy(func(e *models.AgentExecution) bool {
		return e.Status == "success"
	})).Return(nil)
	agentRepo.On("UpdateExecutionStats", mock.Anything, "agent-1", true, 7).Return(nil)

	result, err := svc.InvokeAgent(context.Background(), "user-1", "agent-1", map[string]interface{}{}, nil)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	a2aClient.AssertExpectations(t)
	execRepo.AssertExpectations(t)
}

// TestInvokeAgentSync_ArtifactPersistenceFailureDoesNotFailInvocation pins that
// failing to persist reply artifacts is best-effort: the execution still
// completes as "success" (the reply is lost, not the run).
func TestInvokeAgentSync_ArtifactPersistenceFailureDoesNotFailInvocation(t *testing.T) {
	f := newSyncTestFixture(t)
	reply := &models.AgentExecution{Status: "completed", Duration: intPtr(10), Artifacts: sampleReplyArtifacts}
	f.a2aClient.On("InvokeAgent", mock.Anything, f.agent, f.input, mock.Anything).Return(reply, nil)
	f.execRepo.On("Update", mock.Anything, mock.MatchedBy(func(e *models.AgentExecution) bool {
		return e.ID == "exec-1" && e.Status == "success"
	})).Return(nil)
	f.execRepo.On("UpdateArtifacts", mock.Anything, "exec-1", sampleReplyArtifacts).
		Return(fmt.Errorf("jsonb too large")).Once()

	result, err := f.service.InvokeAgent(context.Background(), "user-1", "agent-1", f.input, nil)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	f.execRepo.AssertExpectations(t)
}

// TestFinalizeCancelled_UpdateFailurePropagates pins that failing to persist
// the cancelled status IS fatal to the cancel request (unlike the event write):
// the caller must not be told the execution was cancelled when it was not.
func TestFinalizeCancelled_UpdateFailurePropagates(t *testing.T) {
	svc, execRepo, eventRepo, _ := newCancelTestService()
	exec := &models.AgentExecution{ID: "e1", Status: "running", StartedAt: time.Now()}

	execRepo.On("Update", mock.Anything, mock.MatchedBy(func(e *models.AgentExecution) bool {
		return e.ID == "e1" && e.Status == "cancelled"
	})).Return(fmt.Errorf("db down")).Once()

	result, err := svc.CancelExecution(context.Background(), exec, &models.Agent{})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to mark execution cancelled")
	// No terminal event may be emitted for a cancel that did not persist.
	eventRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	execRepo.AssertExpectations(t)
}

// TestEmitCancelledEvent_Shape pins the exact wire shape of the terminal
// status-update event the polling frontend converges on: task/context ids from
// the execution and the canonical A2A canceled state string.
func TestEmitCancelledEvent_Shape(t *testing.T) {
	eventRepo := mocks.NewMockAgentExecutionEventRepository(t)
	svc := NewAgentInvocationService(
		nil, nil, eventRepo, nil, nil, slog.New(slog.DiscardHandler),
	)
	execution := &models.AgentExecution{ID: "e1", TaskID: strPtr("t1"), ContextID: strPtr("c1")}

	var captured *models.AgentExecutionEvent
	eventRepo.On("CountByExecutionID", mock.Anything, "e1").Return(5, nil).Once()
	eventRepo.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		captured = args.Get(1).(*models.AgentExecutionEvent)
	}).Return(nil).Once()

	svc.emitCancelledEvent(context.Background(), execution)

	require.NotNil(t, captured)
	assert.NotEmpty(t, captured.ID)
	assert.Equal(t, "e1", captured.ExecutionID)
	assert.Equal(t, "status-update", captured.EventType)
	assert.Equal(t, 5, captured.SequenceNumber, "event appends after the existing event count")
	assert.False(t, captured.ReceivedAt.IsZero())
	assert.Equal(t, "t1", captured.EventData["taskId"])
	assert.Equal(t, "c1", captured.EventData["contextId"])
	status, ok := captured.EventData["status"].(map[string]interface{})
	require.True(t, ok, "status must be a nested object")
	assert.Equal(t, string(a2a.TaskStateCanceled), status["state"])
}

// TestEmitCancelledEvent_CountFailureFallsBackToSequenceZero pins the fallback:
// when the event count cannot be read the event is still emitted, at sequence 0.
func TestEmitCancelledEvent_CountFailureFallsBackToSequenceZero(t *testing.T) {
	svc, execRepo, eventRepo, _ := newCancelTestService()
	exec := &models.AgentExecution{ID: "e1", Status: "running", StartedAt: time.Now()}

	execRepo.On("Update", mock.Anything, mock.Anything).Return(nil).Once()
	execRepo.On("UpdateTaskInfo", mock.Anything, "e1", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	eventRepo.On("CountByExecutionID", mock.Anything, "e1").Return(0, fmt.Errorf("count failed")).Once()
	eventRepo.On("Create", mock.Anything, mock.MatchedBy(func(ev *models.AgentExecutionEvent) bool {
		return ev.EventType == "status-update" && ev.SequenceNumber == 0
	})).Return(nil).Once()

	result, err := svc.CancelExecution(context.Background(), exec, &models.Agent{})

	require.NoError(t, err)
	assert.Equal(t, "cancelled", result.Status)
	eventRepo.AssertExpectations(t)
}

// TestEmitCancelledEvent_CreateFailureTolerated pins that a failing event write
// does not undo the cancel: the execution stays cancelled and the caller sees
// no error (the event is convergence sugar, not the source of truth).
func TestEmitCancelledEvent_CreateFailureTolerated(t *testing.T) {
	svc, execRepo, eventRepo, _ := newCancelTestService()
	exec := &models.AgentExecution{ID: "e1", Status: "running", StartedAt: time.Now()}

	execRepo.On("Update", mock.Anything, mock.Anything).Return(nil).Once()
	execRepo.On("UpdateTaskInfo", mock.Anything, "e1", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	eventRepo.On("CountByExecutionID", mock.Anything, "e1").Return(2, nil).Once()
	eventRepo.On("Create", mock.Anything, mock.Anything).Return(fmt.Errorf("event insert failed")).Once()

	result, err := svc.CancelExecution(context.Background(), exec, &models.Agent{})

	require.NoError(t, err)
	assert.Equal(t, "cancelled", result.Status)
	eventRepo.AssertExpectations(t)
}
