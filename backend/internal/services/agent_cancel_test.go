package services

import (
	"context"
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

func newCancelTestService() (
	*AgentInvocationService,
	*mocks.MockAgentExecutionRepository,
	*mocks.MockAgentExecutionEventRepository,
	*MockA2AHTTPClient,
) {
	agentRepo := new(mocks.MockAgentRepository)
	execRepo := new(mocks.MockAgentExecutionRepository)
	eventRepo := new(mocks.MockAgentExecutionEventRepository)
	a2aClient := new(MockA2AHTTPClient)
	streamProcessor := new(MockA2AStreamProcessor)
	svc := NewAgentInvocationService(agentRepo, execRepo, eventRepo, a2aClient, streamProcessor, slog.New(slog.DiscardHandler))
	return svc, execRepo, eventRepo, a2aClient
}

// expectCancelledPersistence wires the repo calls made when finalizing a cancel.
func expectCancelledPersistence(execRepo *mocks.MockAgentExecutionRepository, eventRepo *mocks.MockAgentExecutionEventRepository, execID string) {
	execRepo.On("Update", mock.Anything, mock.MatchedBy(func(e *models.AgentExecution) bool {
		return e.ID == execID && e.Status == "cancelled"
	})).Return(nil).Once()
	execRepo.On("UpdateTaskInfo", mock.Anything, execID, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	eventRepo.On("CountByExecutionID", mock.Anything, execID).Return(3, nil)
	eventRepo.On("Create", mock.Anything, mock.MatchedBy(func(ev *models.AgentExecutionEvent) bool {
		return ev.EventType == "status-update" && ev.SequenceNumber == 3
	})).Return(nil).Once()
}

func TestCancelExecution_WithTask(t *testing.T) {
	svc, execRepo, eventRepo, a2aClient := newCancelTestService()
	agent := &models.Agent{ID: "a1", AgentCard: &models.AgentCard{}}
	exec := &models.AgentExecution{ID: "e1", Status: "pending", TaskID: strPtr("t1"), StartedAt: time.Now()}

	a2aClient.On("CancelTask", mock.Anything, agent, "t1").Return(&models.AgentExecution{Status: "cancelled"}, nil).Once()
	expectCancelledPersistence(execRepo, eventRepo, "e1")

	result, err := svc.CancelExecution(context.Background(), exec, agent)

	require.NoError(t, err)
	assert.Equal(t, "cancelled", result.Status)
	require.NotNil(t, result.EndedAt)
	a2aClient.AssertExpectations(t)
	execRepo.AssertExpectations(t)
	eventRepo.AssertExpectations(t)
}

func TestCancelExecution_NoTaskID(t *testing.T) {
	svc, execRepo, eventRepo, a2aClient := newCancelTestService()
	exec := &models.AgentExecution{ID: "e1", Status: "working", StartedAt: time.Now()}
	expectCancelledPersistence(execRepo, eventRepo, "e1")

	result, err := svc.CancelExecution(context.Background(), exec, &models.Agent{})

	require.NoError(t, err)
	assert.Equal(t, "cancelled", result.Status)
	a2aClient.AssertNotCalled(t, "CancelTask", mock.Anything, mock.Anything, mock.Anything)
}

func TestCancelExecution_NotCancelable(t *testing.T) {
	svc, _, _, a2aClient := newCancelTestService()
	exec := &models.AgentExecution{ID: "e1", Status: "pending", TaskID: strPtr("t1"), StartedAt: time.Now()}

	a2aClient.On("CancelTask", mock.Anything, mock.Anything, "t1").Return(nil, a2a.ErrTaskNotCancelable).Once()

	_, err := svc.CancelExecution(context.Background(), exec, &models.Agent{})

	require.ErrorIs(t, err, a2a.ErrTaskNotCancelable)
	a2aClient.AssertExpectations(t)
}

func TestCancelExecution_AlreadyTerminal(t *testing.T) {
	svc, _, _, a2aClient := newCancelTestService()
	exec := &models.AgentExecution{ID: "e1", Status: "success"}

	_, err := svc.CancelExecution(context.Background(), exec, &models.Agent{})

	require.ErrorIs(t, err, ErrExecutionNotCancelable)
	a2aClient.AssertNotCalled(t, "CancelTask", mock.Anything, mock.Anything, mock.Anything)
}

func TestCancelExecution_RemoteFailNonFatal(t *testing.T) {
	// A non-"not-cancelable" remote error still cancels locally.
	svc, execRepo, eventRepo, a2aClient := newCancelTestService()
	exec := &models.AgentExecution{ID: "e1", Status: "pending", TaskID: strPtr("t1"), StartedAt: time.Now()}

	a2aClient.On("CancelTask", mock.Anything, mock.Anything, "t1").Return(nil, assertRemoteErr()).Once()
	expectCancelledPersistence(execRepo, eventRepo, "e1")

	result, err := svc.CancelExecution(context.Background(), exec, &models.Agent{})

	require.NoError(t, err)
	assert.Equal(t, "cancelled", result.Status)
}

func assertRemoteErr() error { return context.DeadlineExceeded }
