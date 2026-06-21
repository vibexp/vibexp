package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/container"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

// ====================
// INTEGRATION TESTS
// ====================

// TestEmbeddingHandlers_Unauthorized tests that embedding endpoints require authentication
func TestEmbeddingHandlers_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	// Create a sample Pub/Sub message with prompt embedding event
	eventData := map[string]interface{}{
		"type":   "prompt.embedding.generated",
		"userID": "test-user",
		"payload": map[string]interface{}{
			"userID":   "test-user",
			"promptID": "prompt-123",
			"model":    "text-embedding-3-small",
			"embeddings": []interface{}{
				[]interface{}{0.1, 0.2, 0.3},
			},
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}
	eventJSON, err := json.Marshal(eventData)
	if err != nil {
		t.Fatal(err)
	}
	encodedData := base64.StdEncoding.EncodeToString(eventJSON)

	pubsubMsg := PubSubMessage{}
	pubsubMsg.Message.Data = encodedData
	pubsubMsg.Message.MessageID = "test-message-id"
	pubsubMsg.Message.PublishTime = time.Now().Format(time.RFC3339)
	pubsubMsg.Subscription = "projects/test/subscriptions/test-sub"

	body, err := json.Marshal(pubsubMsg)
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("POST", "/api/v1/events/pubsub", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	// No Authorization header - should be unauthorized

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusUnauthorized {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusUnauthorized)
	}
}

// TestEmbeddingHandlers_InvalidPayload tests handling of invalid payloads
func TestEmbeddingHandlers_InvalidPayload(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		body     string
		expected int
	}{
		{"Invalid JSON", `{invalid json}`, http.StatusUnauthorized}, // Will fail at OIDC validation first
		{"Empty body", ``, http.StatusUnauthorized},
		{"Invalid base64", `{"message":{"data":"not-base64!!!"}}`, http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/api/v1/events/pubsub", strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

// ====================
// UNIT TESTS
// ====================

func buildEmbeddingPayload(entityType, entityID string) PubSubEventPayload {
	// Resolve the payload id-field from the shared registry so multi-word entity
	// types (e.g. feed_item → feedItemID) use the correct key, not "<type>ID".
	cfg, ok := services.GetEmbeddingEntityConfig(entityType)
	idField := entityType + "ID"
	if ok {
		idField = cfg.EntityIDField
	}
	return PubSubEventPayload{
		Type:   entityType + ".embedding.generated",
		UserID: "user-123",
		Payload: map[string]interface{}{
			"userID": "user-123",
			idField:  entityID,
			"model":  "text-embedding-3-small",
			"embeddings": []interface{}{
				map[string]interface{}{
					"embedding": []interface{}{0.1, 0.2, 0.3},
					"content":   "Title text",
				},
			},
		},
	}
}

// TestHandleEntityEmbeddingGenerated_AllEntityTypes verifies that the generic handler
// correctly routes prompt, artifact, and memory entity types.
func TestHandleEntityEmbeddingGenerated_AllEntityTypes(t *testing.T) {
	tests := []struct {
		entityType string
		entityID   string
	}{
		{"prompt", "prompt-456"},
		{"artifact", "artifact-789"},
		{"memory", "memory-999"},
		{"feed_item", "item-456"},
		{"feed_item_reply", "reply-789"},
	}

	for _, tt := range tests {
		t.Run(tt.entityType, func(t *testing.T) {
			cfg := &config.Config{}
			logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
			logger.SetLevel(logrus.ErrorLevel)
			srv := New("8080", nil, "test-api-key", cfg, logger)

			mockEmbeddingService := mocks.NewMockEmbeddingServiceInterface(t)
			srv.container = &EmbeddingTestContainer{EmbeddingServiceMock: mockEmbeddingService}

			mockEmbeddingService.On("SaveEmbeddingChunks",
				"user-123",
				tt.entityType,
				tt.entityID,
				"text-embedding-3-small",
				mock.MatchedBy(func(chunks []services.EmbeddingChunk) bool {
					return len(chunks) == 1 &&
						len(chunks[0].Embedding) == 3 &&
						chunks[0].Embedding[0] == float32(0.1) &&
						chunks[0].Content == "Title text"
				}),
			).Return(nil).Once()

			payload := buildEmbeddingPayload(tt.entityType, tt.entityID)
			statusCode := srv.handleEntityEmbeddingGenerated(tt.entityType, payload, "msg-id")

			assert.Equal(t, http.StatusOK, statusCode)
			mockEmbeddingService.AssertExpectations(t)
		})
	}
}

// TestHandleEntityEmbeddingGenerated_UnknownEntityType verifies that an unregistered
// entity type returns 400.
func TestHandleEntityEmbeddingGenerated_UnknownEntityType(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	mockEmbeddingService := mocks.NewMockEmbeddingServiceInterface(t)
	srv.container = &EmbeddingTestContainer{EmbeddingServiceMock: mockEmbeddingService}
	// No mock setup — SaveEmbeddingChunks must NOT be called for unknown types.

	payload := PubSubEventPayload{
		Type:    "blueprint.embedding.generated",
		UserID:  "user-123",
		Payload: map[string]interface{}{"userID": "user-123"},
	}
	statusCode := srv.handleEntityEmbeddingGenerated("blueprint", payload, "msg-id")

	assert.Equal(t, http.StatusBadRequest, statusCode)
	mockEmbeddingService.AssertExpectations(t)
}

func missingFieldPayloads() []struct {
	name    string
	payload PubSubEventPayload
} {
	oneChunk := []interface{}{map[string]interface{}{
		"embedding": []interface{}{0.1, 0.2, 0.3},
		"content":   "text",
	}}
	evtType := "prompt.embedding.generated"
	return []struct {
		name    string
		payload PubSubEventPayload
	}{
		{"missing userID", PubSubEventPayload{Type: evtType,
			Payload: map[string]interface{}{
				"promptID": "prompt-456", "model": "text-embedding-3-small", "embeddings": oneChunk,
			}}},
		{"missing entityID", PubSubEventPayload{Type: evtType,
			Payload: map[string]interface{}{
				"userID": "user-123", "model": "text-embedding-3-small", "embeddings": oneChunk,
			}}},
		{"missing model", PubSubEventPayload{Type: evtType,
			Payload: map[string]interface{}{
				"userID": "user-123", "promptID": "prompt-456", "embeddings": oneChunk,
			}}},
		{"empty embeddings", PubSubEventPayload{Type: evtType,
			Payload: map[string]interface{}{
				"userID": "user-123", "promptID": "prompt-456",
				"model": "text-embedding-3-small", "embeddings": []interface{}{},
			}}},
	}
}

// TestHandleEntityEmbeddingGenerated_MissingFields tests validation of required payload fields.
func TestHandleEntityEmbeddingGenerated_MissingFields(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	mockEmbeddingService := mocks.NewMockEmbeddingServiceInterface(t)
	srv.container = &EmbeddingTestContainer{EmbeddingServiceMock: mockEmbeddingService}

	for _, tt := range missingFieldPayloads() {
		t.Run(tt.name, func(t *testing.T) {
			statusCode := srv.handleEntityEmbeddingGenerated("prompt", tt.payload, "msg-id")
			assert.Equal(t, http.StatusBadRequest, statusCode)
			mockEmbeddingService.AssertExpectations(t)
		})
	}
}

// TestHandleEntityEmbeddingGenerated_ServiceError tests handling of service errors.
func TestHandleEntityEmbeddingGenerated_ServiceError(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	mockEmbeddingService := mocks.NewMockEmbeddingServiceInterface(t)
	srv.container = &EmbeddingTestContainer{EmbeddingServiceMock: mockEmbeddingService}

	mockEmbeddingService.On("SaveEmbeddingChunks",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return(errors.New("database error")).Once()

	payload := buildEmbeddingPayload("prompt", "prompt-456")
	statusCode := srv.handleEntityEmbeddingGenerated("prompt", payload, "msg-id")

	assert.Equal(t, http.StatusInternalServerError, statusCode)
	mockEmbeddingService.AssertExpectations(t)
}

// TestHandleEntityEmbeddingGenerated_EntityNotFound verifies that a permanent
// "entity deleted" validation failure is acked with 200 instead of 500: a 5xx
// makes Pub/Sub redeliver the same poison message forever, sustaining 5xx/SLO
// alerts, while retrying can never succeed because the entity is gone.
func TestHandleEntityEmbeddingGenerated_EntityNotFound(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	mockEmbeddingService := mocks.NewMockEmbeddingServiceInterface(t)
	srv.container = &EmbeddingTestContainer{EmbeddingServiceMock: mockEmbeddingService}

	// Production-shaped chain: repo sentinel → validator wrap → service wrap.
	saveErr := fmt.Errorf("entity validation failed: %w",
		fmt.Errorf("prompt not found: %w: %w", services.ErrEntityNotFound, repositories.ErrPromptNotFound))
	mockEmbeddingService.On("SaveEmbeddingChunks",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return(saveErr).Once()

	payload := buildEmbeddingPayload("prompt", "prompt-456")
	statusCode := srv.handleEntityEmbeddingGenerated("prompt", payload, "msg-id")

	assert.Equal(t, http.StatusOK, statusCode)
	mockEmbeddingService.AssertExpectations(t)
}

// ====================
// TEST CONTAINER
// ====================

// EmbeddingTestContainer implements container.Container interface for testing
type EmbeddingTestContainer struct {
	BaseMockContainer    // Embed base container for default nil implementations
	EmbeddingServiceMock services.EmbeddingServiceInterface
}

// EmbeddingService overrides the BaseMockContainer method to return our mock
func (tc *EmbeddingTestContainer) EmbeddingService() services.EmbeddingServiceInterface {
	return tc.EmbeddingServiceMock
}

// Ensure EmbeddingTestContainer implements container.Container interface
var _ container.Container = (*EmbeddingTestContainer)(nil)
