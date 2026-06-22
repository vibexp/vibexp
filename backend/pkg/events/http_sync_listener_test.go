package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock embedding handlers for testing
type mockEmbeddingHandlers struct {
	lastEntityType string
	lastPayload    map[string]interface{}
	err            error
}

func (m *mockEmbeddingHandlers) HandleEmbedding(entityType string, payload map[string]interface{}) error {
	m.lastEntityType = entityType
	m.lastPayload = payload
	return m.err
}

//nolint:funlen // Test function naturally requires multiple test cases for comprehensive validation
func TestNewHTTPSyncListener(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	handlers := &mockEmbeddingHandlers{}

	tests := []struct {
		name        string
		config      HTTPSyncListenerConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: HTTPSyncListenerConfig{
				AIServiceURL:      "http://localhost:8000",
				EventTypes:        []string{"prompt.created"},
				Logger:            logger,
				EmbeddingHandlers: handlers,
			},
			expectError: false,
		},
		{
			name: "missing AI service URL",
			config: HTTPSyncListenerConfig{
				EventTypes:        []string{"prompt.created"},
				Logger:            logger,
				EmbeddingHandlers: handlers,
			},
			expectError: true,
		},
		{
			name: "missing embedding handlers",
			config: HTTPSyncListenerConfig{
				AIServiceURL: "http://localhost:8000",
				EventTypes:   []string{"prompt.created"},
				Logger:       logger,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listener, err := NewHTTPSyncListener(tt.config)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, listener)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, listener)
			}
		})
	}
}

//nolint:funlen // Test function requires comprehensive event handling validation
func TestHTTPSyncListener_Handle_PromptCreated(t *testing.T) {
	// Create mock AI service
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/events/sync", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Parse request body
		var requestBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&requestBody)
		require.NoError(t, err)

		// Verify request structure
		assert.Equal(t, "prompt.created", requestBody["type"])
		assert.Equal(t, "user-123", requestBody["user_id"])

		// Send response
		response := map[string]interface{}{
			"type":      "prompt.embedding.generated",
			"user_id":   "user-123",
			"timestamp": time.Now().Format(time.RFC3339),
			"payload": map[string]interface{}{
				"userID":   "user-123",
				"promptID": "prompt-456",
				"model":    "all-MiniLM-L6-v2",
				"embeddings": []map[string]interface{}{
					{
						"embedding": []float64{0.1, 0.2, 0.3},
						"content":   "test content",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Logf("Failed to encode response: %v", err)
		}
	}))
	defer mockServer.Close()

	// Create listener
	logger := slog.New(slog.DiscardHandler)
	handlers := &mockEmbeddingHandlers{}

	listener, err := NewHTTPSyncListener(HTTPSyncListenerConfig{
		AIServiceURL:      mockServer.URL,
		EventTypes:        []string{"prompt.created"},
		Logger:            logger,
		EmbeddingHandlers: handlers,
	})
	require.NoError(t, err)

	// Create test event
	event := &PromptCreatedEvent{
		BaseEvent: NewBaseEvent("prompt.created", map[string]interface{}{
			"PromptID": "prompt-456",
			"UserID":   "user-123",
			"Title":    "Test Prompt",
			"Body":     "Test content",
		}, "user-123"),
	}

	// Handle event
	ctx := context.Background()
	err = listener.Handle(ctx, event)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, "prompt", handlers.lastEntityType)
	assert.Equal(t, "user-123", handlers.lastPayload["userID"])
	assert.Equal(t, "prompt-456", handlers.lastPayload["promptID"])
}

func TestHTTPSyncListener_Handle_ArtifactCreated(t *testing.T) {
	// Create mock AI service
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"type":      "artifact.embedding.generated",
			"user_id":   "user-123",
			"timestamp": time.Now().Format(time.RFC3339),
			"payload": map[string]interface{}{
				"userID":     "user-123",
				"artifactID": "artifact-789",
				"model":      "all-MiniLM-L6-v2",
				"embeddings": []map[string]interface{}{
					{
						"embedding": []float64{0.1, 0.2, 0.3},
						"content":   "artifact content",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Logf("Failed to encode response: %v", err)
		}
	}))
	defer mockServer.Close()

	// Create listener
	logger := slog.New(slog.DiscardHandler)
	handlers := &mockEmbeddingHandlers{}

	listener, err := NewHTTPSyncListener(HTTPSyncListenerConfig{
		AIServiceURL:      mockServer.URL,
		EventTypes:        []string{"artifact.created"},
		Logger:            logger,
		EmbeddingHandlers: handlers,
	})
	require.NoError(t, err)

	// Create test event
	event := &ArtifactCreatedEvent{
		BaseEvent: NewBaseEvent("artifact.created", map[string]interface{}{
			"ArtifactID": "artifact-789",
			"UserID":     "user-123",
			"Title":      "Test Artifact",
		}, "user-123"),
	}

	// Handle event
	ctx := context.Background()
	err = listener.Handle(ctx, event)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, "artifact", handlers.lastEntityType)
	assert.Equal(t, "artifact-789", handlers.lastPayload["artifactID"])
}

func TestHTTPSyncListener_Handle_MemoryCreated(t *testing.T) {
	// Create mock AI service
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"type":      "memory.embedding.generated",
			"user_id":   "user-123",
			"timestamp": time.Now().Format(time.RFC3339),
			"payload": map[string]interface{}{
				"userID":   "user-123",
				"memoryID": "memory-999",
				"model":    "all-MiniLM-L6-v2",
				"embeddings": []map[string]interface{}{
					{
						"embedding": []float64{0.1, 0.2, 0.3},
						"content":   "memory content",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Logf("Failed to encode response: %v", err)
		}
	}))
	defer mockServer.Close()

	// Create listener
	logger := slog.New(slog.DiscardHandler)
	handlers := &mockEmbeddingHandlers{}

	listener, err := NewHTTPSyncListener(HTTPSyncListenerConfig{
		AIServiceURL:      mockServer.URL,
		EventTypes:        []string{"memory.created"},
		Logger:            logger,
		EmbeddingHandlers: handlers,
	})
	require.NoError(t, err)

	// Create test event
	event := &MemoryCreatedEvent{
		BaseEvent: NewBaseEvent("memory.created", map[string]interface{}{
			"MemoryID": "memory-999",
			"UserID":   "user-123",
			"Text":     "Test memory",
		}, "user-123"),
	}

	// Handle event
	ctx := context.Background()
	err = listener.Handle(ctx, event)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, "memory", handlers.lastEntityType)
	assert.Equal(t, "memory-999", handlers.lastPayload["memoryID"])
}

func TestHTTPSyncListener_Handle_ServiceError(t *testing.T) {
	// Create mock AI service that returns error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte("Internal Server Error")); err != nil {
			t.Logf("Failed to write response: %v", err)
		}
	}))
	defer mockServer.Close()

	// Create listener
	logger := slog.New(slog.DiscardHandler)
	handlers := &mockEmbeddingHandlers{}

	listener, err := NewHTTPSyncListener(HTTPSyncListenerConfig{
		AIServiceURL:      mockServer.URL,
		EventTypes:        []string{"prompt.created"},
		Logger:            logger,
		EmbeddingHandlers: handlers,
	})
	require.NoError(t, err)

	// Create test event
	event := &PromptCreatedEvent{
		BaseEvent: NewBaseEvent("prompt.created", map[string]interface{}{
			"PromptID": "prompt-456",
			"UserID":   "user-123",
		}, "user-123"),
	}

	// Handle event
	ctx := context.Background()
	err = listener.Handle(ctx, event)

	// Verify error is returned
	assert.Error(t, err)
	assert.Empty(t, handlers.lastEntityType)
}

func TestHTTPSyncListener_EventTypes(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	handlers := &mockEmbeddingHandlers{}

	expectedTypes := []string{"prompt.created", "artifact.created", "memory.created"}

	listener, err := NewHTTPSyncListener(HTTPSyncListenerConfig{
		AIServiceURL:      "http://localhost:8000",
		EventTypes:        expectedTypes,
		Logger:            logger,
		EmbeddingHandlers: handlers,
	})
	require.NoError(t, err)

	actualTypes := listener.EventTypes()
	assert.Equal(t, expectedTypes, actualTypes)
}
