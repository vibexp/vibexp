package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestA2AStreamProcessor_HandleArtifactUpdate_AppendTrue(t *testing.T) {
	executionRepo := new(mocks.MockAgentExecutionRepository)
	eventRepo := new(mocks.MockAgentExecutionEventRepository)
	logger := logrus.New()

	processor := NewA2AStreamProcessor(eventRepo, executionRepo, logger)

	executionID := uuid.New().String()
	artifactID := "artifact-1"

	// Create map to track artifacts (simulating internal state)
	artifacts := make(map[string]map[string]interface{})

	// First event: append=false (creates new artifact)
	event1 := &A2AStreamEvent{
		Type: "artifact-update",
		Data: map[string]interface{}{
			"artifactId": artifactID,
			"append":     false,
			"artifact": map[string]interface{}{
				"artifactId": artifactID,
				"parts": []interface{}{
					map[string]interface{}{
						"kind": "text",
						"text": "First chunk ",
					},
				},
			},
		},
		Timestamp: time.Now(),
	}

	processor.handleArtifactUpdate(executionID, event1, artifacts)

	// Verify first chunk is added
	assert.Contains(t, artifacts, artifactID)
	artifact := artifacts[artifactID]
	parts := artifact["parts"].([]interface{})
	assert.Equal(t, 1, len(parts))

	// Second event: append=true (appends to existing artifact)
	event2 := &A2AStreamEvent{
		Type: "artifact-update",
		Data: map[string]interface{}{
			"artifactId": artifactID,
			"append":     true,
			"artifact": map[string]interface{}{
				"artifactId": artifactID,
				"parts": []interface{}{
					map[string]interface{}{
						"kind": "text",
						"text": "Second chunk ",
					},
				},
			},
		},
		Timestamp: time.Now(),
	}

	processor.handleArtifactUpdate(executionID, event2, artifacts)

	// Verify second chunk is appended
	artifact = artifacts[artifactID]
	parts = artifact["parts"].([]interface{})
	assert.Equal(t, 2, len(parts))
	assert.Equal(t, "First chunk ", parts[0].(map[string]interface{})["text"])
	assert.Equal(t, "Second chunk ", parts[1].(map[string]interface{})["text"])
}

func TestA2AStreamProcessor_HandleArtifactUpdate_AppendFalse(t *testing.T) {
	executionRepo := new(mocks.MockAgentExecutionRepository)
	eventRepo := new(mocks.MockAgentExecutionEventRepository)
	logger := logrus.New()

	processor := NewA2AStreamProcessor(eventRepo, executionRepo, logger)

	executionID := uuid.New().String()
	artifactID := "artifact-1"
	artifacts := make(map[string]map[string]interface{})

	// First artifact
	event1 := &A2AStreamEvent{
		Type: "artifact-update",
		Data: map[string]interface{}{
			"artifactId": artifactID,
			"append":     false,
			"artifact": map[string]interface{}{
				"artifactId": artifactID,
				"parts": []interface{}{
					map[string]interface{}{
						"kind": "text",
						"text": "First artifact",
					},
				},
			},
		},
		Timestamp: time.Now(),
	}

	processor.handleArtifactUpdate(executionID, event1, artifacts)

	// Second artifact with append=false (should replace, not append)
	event2 := &A2AStreamEvent{
		Type: "artifact-update",
		Data: map[string]interface{}{
			"artifactId": artifactID,
			"append":     false,
			"artifact": map[string]interface{}{
				"artifactId": artifactID,
				"parts": []interface{}{
					map[string]interface{}{
						"kind": "text",
						"text": "Replaced artifact",
					},
				},
			},
		},
		Timestamp: time.Now(),
	}

	processor.handleArtifactUpdate(executionID, event2, artifacts)

	// Verify artifact was replaced, not appended
	artifact := artifacts[artifactID]
	parts := artifact["parts"].([]interface{})
	assert.Equal(t, 1, len(parts))
	assert.Equal(t, "Replaced artifact", parts[0].(map[string]interface{})["text"])
}

func TestA2AStreamProcessor_HandleArtifactUpdate_MultipleArtifacts(t *testing.T) {
	executionRepo := new(mocks.MockAgentExecutionRepository)
	eventRepo := new(mocks.MockAgentExecutionEventRepository)
	logger := logrus.New()

	processor := NewA2AStreamProcessor(eventRepo, executionRepo, logger)

	executionID := uuid.New().String()
	artifacts := make(map[string]map[string]interface{})

	// Create first artifact
	event1 := &A2AStreamEvent{
		Type: "artifact-update",
		Data: map[string]interface{}{
			"artifactId": "artifact-1",
			"append":     false,
			"artifact": map[string]interface{}{
				"artifactId": "artifact-1",
				"parts": []interface{}{
					map[string]interface{}{
						"kind": "text",
						"text": "First artifact",
					},
				},
			},
		},
		Timestamp: time.Now(),
	}

	processor.handleArtifactUpdate(executionID, event1, artifacts)

	// Create second artifact
	event2 := &A2AStreamEvent{
		Type: "artifact-update",
		Data: map[string]interface{}{
			"artifactId": "artifact-2",
			"append":     false,
			"artifact": map[string]interface{}{
				"artifactId": "artifact-2",
				"parts": []interface{}{
					map[string]interface{}{
						"kind": "text",
						"text": "Second artifact",
					},
				},
			},
		},
		Timestamp: time.Now(),
	}

	processor.handleArtifactUpdate(executionID, event2, artifacts)

	// Verify both artifacts exist independently
	assert.Equal(t, 2, len(artifacts))
	assert.Contains(t, artifacts, "artifact-1")
	assert.Contains(t, artifacts, "artifact-2")

	parts1 := artifacts["artifact-1"]["parts"].([]interface{})
	parts2 := artifacts["artifact-2"]["parts"].([]interface{})

	assert.Equal(t, "First artifact", parts1[0].(map[string]interface{})["text"])
	assert.Equal(t, "Second artifact", parts2[0].(map[string]interface{})["text"])
}

func TestA2AStreamProcessor_ProcessStream_SavesEvents(t *testing.T) {
	executionRepo := new(mocks.MockAgentExecutionRepository)
	eventRepo := new(mocks.MockAgentExecutionEventRepository)
	logger := logrus.New()

	processor := NewA2AStreamProcessor(eventRepo, executionRepo, logger)

	executionID := uuid.New().String()
	eventChan := make(chan *A2AStreamEvent, 10)

	// Setup expectations
	eventRepo.On("Create", mock.Anything, mock.MatchedBy(func(event *models.AgentExecutionEvent) bool {
		return event.ExecutionID == executionID && event.EventType == "status-update"
	})).Return(nil).Once()

	// Mock UpdateTaskInfo for status-update event
	// (expects empty strings for task_id and context_id, and "working" for state)
	executionRepo.On("UpdateTaskInfo", mock.Anything, executionID, "", "", "working").Return(nil).Once()

	eventRepo.On("Create", mock.Anything, mock.MatchedBy(func(event *models.AgentExecutionEvent) bool {
		return event.ExecutionID == executionID && event.EventType == "artifact-update"
	})).Return(nil).Once()

	// UpdateArtifacts is called twice:
	// 1. During artifact-update event processing (incremental save)
	// 2. After channel closes (final save at line 121 in ProcessStream)
	executionRepo.On("UpdateArtifacts", mock.Anything, executionID, mock.Anything).Return(nil).Twice()

	// Mock UpdateStatus for final status update when channel closes
	executionRepo.On("UpdateStatus", mock.Anything, executionID, "success").Return(nil).Once()

	// Send events
	go func() {
		eventChan <- &A2AStreamEvent{
			Type: "status-update",
			Data: map[string]interface{}{
				"status": map[string]interface{}{
					"state": "working",
				},
			},
			Timestamp: time.Now(),
		}

		eventChan <- &A2AStreamEvent{
			Type: "artifact-update",
			Data: map[string]interface{}{
				"artifactId": "art-1",
				"append":     false,
				"artifact": map[string]interface{}{
					"artifactId": "art-1",
					"parts": []interface{}{
						map[string]interface{}{
							"kind": "text",
							"text": "Test response",
						},
					},
				},
			},
			Timestamp: time.Now(),
		}

		close(eventChan)
	}()

	// Process stream
	err := processor.ProcessStream(context.Background(), executionID, eventChan)

	assert.NoError(t, err)
	eventRepo.AssertExpectations(t)
	executionRepo.AssertExpectations(t)
}

func TestA2AStreamProcessor_SaveArtifacts_OnlyNonEmptyParts(t *testing.T) {
	executionRepo := new(mocks.MockAgentExecutionRepository)
	eventRepo := new(mocks.MockAgentExecutionEventRepository)
	logger := logrus.New()

	processor := NewA2AStreamProcessor(eventRepo, executionRepo, logger)

	executionID := uuid.New().String()
	artifacts := map[string]map[string]interface{}{
		"artifact-1": {
			"artifactId": "artifact-1",
			"parts": []interface{}{
				map[string]interface{}{
					"kind": "text",
					"text": "Valid content",
				},
			},
		},
		"artifact-2": {
			"artifactId": "artifact-2",
			"parts":      []interface{}{}, // Empty parts
		},
	}

	// Expect only artifact-1 to be saved (has non-empty parts)
	executionRepo.On("UpdateArtifacts", mock.Anything, executionID,
		mock.MatchedBy(func(arts []map[string]interface{}) bool {
			// Should only contain artifact with non-empty parts
			return len(arts) == 1 && arts[0]["artifactId"] == "artifact-1"
		})).Return(nil)

	err := processor.saveArtifacts(context.Background(), executionID, artifacts)

	assert.NoError(t, err)
	executionRepo.AssertExpectations(t)
}
