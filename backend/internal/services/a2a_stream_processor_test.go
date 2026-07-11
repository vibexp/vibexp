package services

import (
	"context"
	"log/slog"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

func newTestStreamProcessor() (*A2AStreamProcessor, *mocks.MockAgentExecutionEventRepository, *mocks.MockAgentExecutionRepository) {
	eventRepo := new(mocks.MockAgentExecutionEventRepository)
	executionRepo := new(mocks.MockAgentExecutionRepository)
	processor := NewA2AStreamProcessor(eventRepo, executionRepo, slog.New(slog.DiscardHandler))
	return processor, eventRepo, executionRepo
}

func runStream(t *testing.T, p *A2AStreamProcessor, execID string, events ...a2a.Event) {
	t.Helper()
	ch := make(chan a2a.Event, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	require.NoError(t, p.ProcessStream(context.Background(), execID, ch))
}

func TestProcessStream_TaskEvent(t *testing.T) {
	p, eventRepo, execRepo := newTestStreamProcessor()
	execID := uuid.New().String()

	eventRepo.On("Create", mock.Anything, mock.MatchedBy(func(e *models.AgentExecutionEvent) bool {
		return e.EventType == "task" && e.ExecutionID == execID
	})).Return(nil).Once()
	execRepo.On("UpdateTaskInfo", mock.Anything, execID, "t1", "c1", string(a2a.TaskStateCompleted)).Return(nil).Once()
	execRepo.On("UpdateStatus", mock.Anything, execID, "success").Return(nil).Once()

	runStream(t, p, execID, &a2a.Task{
		ID:        "t1",
		ContextID: "c1",
		Status:    a2a.TaskStatus{State: a2a.TaskStateCompleted},
	})

	eventRepo.AssertExpectations(t)
	execRepo.AssertExpectations(t)
}

func TestProcessStream_StatusUpdate_Terminal(t *testing.T) {
	cases := []struct {
		state  a2a.TaskState
		status string
	}{
		{a2a.TaskStateCompleted, "success"},
		{a2a.TaskStateCanceled, "cancelled"},
		{a2a.TaskStateFailed, "error"},
		{a2a.TaskStateRejected, "error"},
	}
	for _, tc := range cases {
		t.Run(string(tc.state), func(t *testing.T) {
			p, eventRepo, execRepo := newTestStreamProcessor()
			execID := uuid.New().String()

			eventRepo.On("Create", mock.Anything, mock.MatchedBy(func(e *models.AgentExecutionEvent) bool {
				return e.EventType == "status-update"
			})).Return(nil).Once()
			execRepo.On("UpdateTaskInfo", mock.Anything, execID, "t1", "c1", string(tc.state)).Return(nil).Once()
			execRepo.On("UpdateStatus", mock.Anything, execID, tc.status).Return(nil).Once()

			runStream(t, p, execID, &a2a.TaskStatusUpdateEvent{
				TaskID:    "t1",
				ContextID: "c1",
				Status:    a2a.TaskStatus{State: tc.state},
			})

			eventRepo.AssertExpectations(t)
			execRepo.AssertExpectations(t)
		})
	}
}

func TestProcessStream_ArtifactUpdate_Append(t *testing.T) {
	p, eventRepo, execRepo := newTestStreamProcessor()
	execID := uuid.New().String()

	eventRepo.On("Create", mock.Anything, mock.MatchedBy(func(e *models.AgentExecutionEvent) bool {
		return e.EventType == "artifact-update"
	})).Return(nil)

	var lastArtifacts []map[string]interface{}
	execRepo.On("UpdateArtifacts", mock.Anything, execID, mock.Anything).
		Run(func(args mock.Arguments) {
			lastArtifacts = args.Get(2).([]map[string]interface{})
		}).Return(nil)
	execRepo.On("UpdateStatus", mock.Anything, execID, "success").Return(nil).Once()

	runStream(t, p, execID,
		&a2a.TaskArtifactUpdateEvent{
			TaskID:   "t1",
			Append:   false,
			Artifact: &a2a.Artifact{ID: "a1", Parts: a2a.ContentParts{a2a.NewTextPart("First ")}},
		},
		&a2a.TaskArtifactUpdateEvent{
			TaskID:   "t1",
			Append:   true,
			Artifact: &a2a.Artifact{ID: "a1", Parts: a2a.ContentParts{a2a.NewTextPart("second")}},
		},
	)

	require.Len(t, lastArtifacts, 1)
	parts, ok := lastArtifacts[0]["parts"].([]interface{})
	require.True(t, ok)
	assert.Len(t, parts, 2, "appended artifact should have both chunks")

	eventRepo.AssertExpectations(t)
	execRepo.AssertExpectations(t)
}

func TestProcessStream_SkipsMessageEvent(t *testing.T) {
	p, eventRepo, execRepo := newTestStreamProcessor()
	execID := uuid.New().String()

	// A direct message reply is not a persisted execution event (that's #163).
	execRepo.On("UpdateStatus", mock.Anything, execID, "success").Return(nil).Once()

	runStream(t, p, execID, a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("hi")))

	eventRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	execRepo.AssertExpectations(t)
}
