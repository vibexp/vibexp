package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
)

func createTestPubSubMessage(t *testing.T) []byte {
	t.Helper()
	eventData := map[string]interface{}{
		"type":      "user.created",
		"payload":   map[string]interface{}{"user_id": "123"},
		"timestamp": time.Now().Format(time.RFC3339),
		"user_id":   "test-user",
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
	return body
}

func createTestPubSubRequest(t *testing.T, body []byte, authHeader string) *http.Request {
	t.Helper()
	req, err := http.NewRequest("POST", "/api/v1/events/pubsub", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	return req
}

func TestPubSubHandlers_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name       string
		authHeader string
		expected   int
	}{
		{"No Authorization header", "", http.StatusUnauthorized},
		{"Invalid Authorization format", "InvalidFormat token", http.StatusUnauthorized},
		{"Missing Bearer prefix", "token-without-bearer", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := createTestPubSubMessage(t)
			req := createTestPubSubRequest(t, body, tt.authHeader)

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

func TestPubSubHandlers_InvalidPayload(t *testing.T) {
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

// MockContainerForPubSub is a mock container for testing PubSub handlers
type MockContainerForPubSub struct {
	BaseMockContainer
	mock.Mock
	embeddingService *svcmocks.MockEmbeddingServiceInterface
}

func (m *MockContainerForPubSub) EmbeddingService() services.EmbeddingServiceInterface {
	return m.embeddingService
}

func (m *MockContainerForPubSub) SearchService() services.SearchServiceInterface {
	return nil
}

// Implement other required Container interface methods
func (m *MockContainerForPubSub) UserRepository() repositories.UserRepository { return nil }
func (m *MockContainerForPubSub) APIKeyRepository() repositories.APIKeyRepository {
	return nil
}
func (m *MockContainerForPubSub) PromptRepository() repositories.PromptRepository { return nil }
func (m *MockContainerForPubSub) ArtifactRepository() repositories.ArtifactRepository {
	return nil
}
func (m *MockContainerForPubSub) EmbeddingProviderRepository() repositories.EmbeddingProviderRepository {
	return nil
}
func (m *MockContainerForPubSub) ActivityRepository() repositories.ActivityRepository { return nil }
func (m *MockContainerForPubSub) ResourceAccessRepository() repositories.ResourceAccessRepository {
	return nil
}
func (m *MockContainerForPubSub) ClaudeCodeHooksRepository() repositories.ClaudeCodeHooksRepository {
	return nil
}
func (m *MockContainerForPubSub) CursorIDEHooksRepository() repositories.CursorIDEHooksRepository {
	return nil
}
func (m *MockContainerForPubSub) AgentRepository() repositories.AgentRepository { return nil }
func (m *MockContainerForPubSub) AgentExecutionRepository() repositories.AgentExecutionRepository {
	return nil
}
func (m *MockContainerForPubSub) AgentExecutionEventRepository() repositories.AgentExecutionEventRepository {
	return nil
}
func (m *MockContainerForPubSub) MemoryRepository() repositories.MemoryRepository { return nil }
func (m *MockContainerForPubSub) EmbeddingRepository() repositories.EmbeddingRepository {
	return nil
}
func (m *MockContainerForPubSub) BackofficeRepository() repositories.BackofficeRepository { return nil }
func (m *MockContainerForPubSub) AuthService() services.AuthServiceInterface              { return nil }
func (m *MockContainerForPubSub) APIKeyService() services.APIKeyServiceInterface {
	return nil
}
func (m *MockContainerForPubSub) PromptService() services.PromptServiceInterface { return nil }
func (m *MockContainerForPubSub) PromptGalleryService() services.PromptGalleryServiceInterface {
	return nil
}
func (m *MockContainerForPubSub) PromptGalleryRepository() repositories.PromptGalleryRepository {
	return nil
}
func (m *MockContainerForPubSub) PromptShareRepository() repositories.PromptShareRepository {
	return nil
}
func (m *MockContainerForPubSub) PromptShareService() services.PromptShareServiceInterface {
	return nil
}

func (m *MockContainerForPubSub) ArtifactService() services.ArtifactServiceInterface {
	return nil
}
func (m *MockContainerForPubSub) EmbeddingProviderService() services.EmbeddingProviderServiceInterface {
	return nil
}
func (m *MockContainerForPubSub) EmailService() services.EmailServiceInterface { return nil }
func (m *MockContainerForPubSub) ActivityService() activities.ActivityService  { return nil }
func (m *MockContainerForPubSub) ResourceAccessService() resourceaccess.ResourceAccessService {
	return nil
}
func (m *MockContainerForPubSub) AgentService() services.AgentServiceInterface { return nil }
func (m *MockContainerForPubSub) AgentCardFetcher() services.AgentCardFetcherInterface {
	return nil
}
func (m *MockContainerForPubSub) AgentInvocationService() services.AgentInvocationServiceInterface {
	return nil
}
func (m *MockContainerForPubSub) MemoryService() services.MemoryServiceInterface { return nil }
func (m *MockContainerForPubSub) EnvironmentService() *services.EnvironmentService {
	return nil
}
func (m *MockContainerForPubSub) ResourceUsageService() services.ResourceUsageServiceInterface {
	return nil
}
func (m *MockContainerForPubSub) BackofficeService() services.BackofficeServiceInterface { return nil }
func (m *MockContainerForPubSub) EmbeddingBackfillService() services.EmbeddingBackfillServiceInterface {
	return nil
}
func (m *MockContainerForPubSub) BlueprintService() services.BlueprintServiceInterface {
	return nil
}
func (m *MockContainerForPubSub) UserPreferencesRepository() repositories.UserPreferencesRepository {
	return nil
}
func (m *MockContainerForPubSub) UserPreferencesService() services.UserPreferencesServiceInterface {
	return nil
}
func (m *MockContainerForPubSub) TeamRepository() repositories.TeamRepository             { return nil }
func (m *MockContainerForPubSub) TeamMemberRepository() repositories.TeamMemberRepository { return nil }
func (m *MockContainerForPubSub) TeamService() services.TeamServiceInterface              { return nil }
func (m *MockContainerForPubSub) TeamInvitationService() *services.TeamInvitationService  { return nil }

func (m *MockContainerForPubSub) TeamSubscriptionRepository() repositories.TeamSubscriptionRepository {
	return nil
}
func (m *MockContainerForPubSub) GitHubAppClient() external.GitHubAppClient            { return nil }
func (m *MockContainerForPubSub) GitHubAppService() services.GitHubAppServiceInterface { return nil }
func (m *MockContainerForPubSub) GitHubInstallationRepository() repositories.GitHubInstallationRepository {
	return nil
}
func (m *MockContainerForPubSub) Close() error { return nil }

func createTestServerWithMockEmbedding(t *testing.T) (*Server, *svcmocks.MockEmbeddingServiceInterface) {
	t.Helper()
	mockEmbeddingService := svcmocks.NewMockEmbeddingServiceInterface(t)
	mockContainer := &MockContainerForPubSub{
		embeddingService: mockEmbeddingService,
	}

	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)

	srv := &Server{
		router:    nil,
		port:      "8080",
		apiKey:    "test-api-key",
		config:    cfg,
		container: mockContainer,
		logger:    logger,
	}
	return srv, mockEmbeddingService
}

func TestPubSubHandlers_PromptEmbeddingGenerated_Success(t *testing.T) {
	srv, mockEmbeddingService := createTestServerWithMockEmbedding(t)

	// Create test payload
	payload := map[string]interface{}{
		"userID":   "user-123",
		"promptID": "prompt-456",
		"model":    "text-embedding-ada-002",
		"embeddings": []interface{}{
			map[string]interface{}{
				"embedding": []interface{}{0.1, 0.2, 0.3},
				"content":   "test content 1",
			},
			map[string]interface{}{
				"embedding": []interface{}{0.4, 0.5, 0.6},
				"content":   "test content 2",
			},
		},
	}

	// Setup mock expectation
	mockEmbeddingService.On("SaveEmbeddingChunks",
		"user-123",
		"prompt",
		"prompt-456",
		"text-embedding-ada-002",
		mock.MatchedBy(func(chunks []services.EmbeddingChunk) bool {
			return len(chunks) == 2 &&
				chunks[0].Content == "test content 1" &&
				chunks[1].Content == "test content 2"
		}),
	).Return(nil)

	// Test the generic handler directly
	result := srv.handleEntityEmbeddingGenerated("prompt", PubSubEventPayload{
		Type:      "prompt.embedding.generated",
		Payload:   payload,
		Timestamp: time.Now().Format(time.RFC3339),
		UserID:    "user-123",
	}, "test-message-id-123")

	if result != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, result)
	}
}

func TestPubSubHandlers_PromptEmbeddingGenerated_InvalidPayload(t *testing.T) {
	srv, _ := createTestServerWithMockEmbedding(t)

	tests := []struct {
		name    string
		payload map[string]interface{}
	}{
		{"Missing userID", map[string]interface{}{
			"promptID": "prompt-456", "model": "test", "embeddings": []interface{}{}}},
		{"Missing promptID", map[string]interface{}{
			"userID": "user-123", "model": "test", "embeddings": []interface{}{}}},
		{"Missing model", map[string]interface{}{
			"userID": "user-123", "promptID": "prompt-456", "embeddings": []interface{}{}}},
		{"Missing embeddings", map[string]interface{}{
			"userID": "user-123", "promptID": "prompt-456", "model": "test"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := srv.handleEntityEmbeddingGenerated("prompt", PubSubEventPayload{
				Type:      "prompt.embedding.generated",
				Payload:   tt.payload,
				Timestamp: time.Now().Format(time.RFC3339),
				UserID:    "user-123",
			}, "test-message-id")

			if result != http.StatusBadRequest {
				t.Errorf("Expected status %d, got %d", http.StatusBadRequest, result)
			}
		})
	}
}

func TestPubSubHandlers_PromptEmbeddingGenerated_ServiceError(t *testing.T) {
	srv, mockEmbeddingService := createTestServerWithMockEmbedding(t)

	payload := map[string]interface{}{
		"userID":   "user-123",
		"promptID": "prompt-456",
		"model":    "text-embedding-ada-002",
		"embeddings": []interface{}{
			map[string]interface{}{
				"embedding": []interface{}{0.1, 0.2, 0.3},
				"content":   "test content",
			},
		},
	}

	// Setup mock to return error
	mockEmbeddingService.On("SaveEmbeddingChunks",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(http.ErrServerClosed)

	result := srv.handleEntityEmbeddingGenerated("prompt", PubSubEventPayload{
		Type:      "prompt.embedding.generated",
		Payload:   payload,
		Timestamp: time.Now().Format(time.RFC3339),
		UserID:    "user-123",
	}, "test-message-id")

	if result != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, result)
	}
}

func TestPubSubHandlers_ArtifactEmbeddingGenerated_Success(t *testing.T) {
	srv, mockEmbeddingService := createTestServerWithMockEmbedding(t)

	payload := map[string]interface{}{
		"userID":     "user-123",
		"artifactID": "artifact-789",
		"model":      "text-embedding-ada-002",
		"embeddings": []interface{}{
			map[string]interface{}{
				"embedding": []interface{}{0.1, 0.2, 0.3},
				"content":   "artifact content",
			},
		},
	}

	mockEmbeddingService.On("SaveEmbeddingChunks",
		"user-123",
		"artifact",
		"artifact-789",
		"text-embedding-ada-002",
		mock.MatchedBy(func(chunks []services.EmbeddingChunk) bool {
			return len(chunks) == 1 && chunks[0].Content == "artifact content"
		}),
	).Return(nil)

	result := srv.handleEntityEmbeddingGenerated("artifact", PubSubEventPayload{
		Type:      "artifact.embedding.generated",
		Payload:   payload,
		Timestamp: time.Now().Format(time.RFC3339),
		UserID:    "user-123",
	}, "test-message-id")

	if result != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, result)
	}
}

func TestPubSubHandlers_ArtifactEmbeddingGenerated_InvalidPayload(t *testing.T) {
	srv, _ := createTestServerWithMockEmbedding(t)

	tests := []struct {
		name    string
		payload map[string]interface{}
	}{
		{"Missing userID", map[string]interface{}{
			"artifactID": "artifact-789", "model": "test", "embeddings": []interface{}{}}},
		{"Missing artifactID", map[string]interface{}{
			"userID": "user-123", "model": "test", "embeddings": []interface{}{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := srv.handleEntityEmbeddingGenerated("artifact", PubSubEventPayload{
				Type:      "artifact.embedding.generated",
				Payload:   tt.payload,
				Timestamp: time.Now().Format(time.RFC3339),
				UserID:    "user-123",
			}, "test-message-id")

			if result != http.StatusBadRequest {
				t.Errorf("Expected status %d, got %d", http.StatusBadRequest, result)
			}
		})
	}
}

func TestPubSubHandlers_MemoryEmbeddingGenerated_Success(t *testing.T) {
	srv, mockEmbeddingService := createTestServerWithMockEmbedding(t)

	payload := map[string]interface{}{
		"userID":   "user-123",
		"memoryID": "memory-321",
		"model":    "text-embedding-ada-002",
		"embeddings": []interface{}{
			map[string]interface{}{
				"embedding": []interface{}{0.7, 0.8, 0.9},
				"content":   "memory content",
			},
		},
	}

	mockEmbeddingService.On("SaveEmbeddingChunks",
		"user-123",
		"memory",
		"memory-321",
		"text-embedding-ada-002",
		mock.MatchedBy(func(chunks []services.EmbeddingChunk) bool {
			return len(chunks) == 1 && chunks[0].Content == "memory content"
		}),
	).Return(nil)

	result := srv.handleEntityEmbeddingGenerated("memory", PubSubEventPayload{
		Type:      "memory.embedding.generated",
		Payload:   payload,
		Timestamp: time.Now().Format(time.RFC3339),
		UserID:    "user-123",
	}, "test-message-id")

	if result != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, result)
	}
}

func TestPubSubHandlers_MemoryEmbeddingGenerated_InvalidPayload(t *testing.T) {
	srv, _ := createTestServerWithMockEmbedding(t)

	tests := []struct {
		name    string
		payload map[string]interface{}
	}{
		{
			name: "Missing memoryID",
			payload: map[string]interface{}{
				"userID": "user-123",
				"model":  "text-embedding-ada-002",
				"embeddings": []interface{}{
					map[string]interface{}{
						"embedding": []interface{}{0.1, 0.2, 0.3},
						"content":   "test",
					},
				},
			},
		},
		{
			name: "Empty embeddings",
			payload: map[string]interface{}{
				"userID":     "user-123",
				"memoryID":   "memory-321",
				"model":      "text-embedding-ada-002",
				"embeddings": []interface{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := srv.handleEntityEmbeddingGenerated("memory", PubSubEventPayload{
				Type:      "memory.embedding.generated",
				Payload:   tt.payload,
				Timestamp: time.Now().Format(time.RFC3339),
				UserID:    "user-123",
			}, "test-message-id")

			if result != http.StatusBadRequest {
				t.Errorf("Expected status %d, got %d", http.StatusBadRequest, result)
			}
		})
	}
}

func TestPubSubHandlers_RouteEventToHandler_UnknownEventType(t *testing.T) {
	srv, _ := createTestServerWithMockEmbedding(t)

	// Test unknown event types - should acknowledge to prevent retries
	unknownEvents := []string{
		"user.created",
		"unknown.event.type",
		"something.else",
	}

	for _, eventType := range unknownEvents {
		t.Run(eventType, func(t *testing.T) {
			result := srv.routeEventToHandler(&PubSubEventPayload{
				Type:      eventType,
				Payload:   map[string]interface{}{},
				Timestamp: time.Now().Format(time.RFC3339),
				UserID:    "user-123",
			}, "test-message-id")

			if result != http.StatusOK {
				t.Errorf("Unknown event type should return OK to prevent retries, got %d", result)
			}
		})
	}
}

func TestPubSubHandlers_ParsePubSubMessage_InvalidBase64(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)

	srv := &Server{
		router:    nil,
		port:      "8080",
		apiKey:    "test-api-key",
		config:    cfg,
		container: nil,
		logger:    logger,
	}

	// Create a PubSub message with invalid base64
	pubsubMsg := PubSubMessage{}
	pubsubMsg.Message.Data = "not-valid-base64!@#$%"
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

	_, _, err = srv.parsePubSubMessage(req, "test-service-account")
	if err == nil {
		t.Error("Expected error for invalid base64, got nil")
	}
}

func TestPubSubHandlers_ParsePubSubMessage_InvalidJSON(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)

	srv := &Server{
		router:    nil,
		port:      "8080",
		apiKey:    "test-api-key",
		config:    cfg,
		container: nil,
		logger:    logger,
	}

	// Create a PubSub message with invalid JSON in the data
	invalidJSON := "not valid json"
	encodedData := base64.StdEncoding.EncodeToString([]byte(invalidJSON))

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

	_, _, err = srv.parsePubSubMessage(req, "test-service-account")
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func (m *MockContainerForPubSub) ProjectRepository() repositories.ProjectRepository {
	return nil
}

func (m *MockContainerForPubSub) ProjectService() services.ProjectServiceInterface {
	return nil
}

func (m *MockContainerForPubSub) WebhookEventRepository() repositories.WebhookEventRepository {
	return nil
}

func (m *MockContainerForPubSub) FeedRepository() repositories.FeedRepository         { return nil }
func (m *MockContainerForPubSub) FeedItemRepository() repositories.FeedItemRepository { return nil }
func (m *MockContainerForPubSub) FeedItemReplyRepository() repositories.FeedItemReplyRepository {
	return nil
}
func (m *MockContainerForPubSub) FeedService() services.FeedServiceInterface         { return nil }
func (m *MockContainerForPubSub) FeedItemService() services.FeedItemServiceInterface { return nil }
func (m *MockContainerForPubSub) FeedItemReplyService() services.FeedItemReplyServiceInterface {
	return nil
}

func (m *MockContainerForPubSub) NotificationRepository() repositories.NotificationRepository {
	return nil
}
func (m *MockContainerForPubSub) NotificationDeliveryRepository() repositories.NotificationDeliveryRepository {
	return nil
}
func (m *MockContainerForPubSub) NotificationDigestQueueRepository() repositories.NotificationDigestQueueRepository {
	return nil
}
func (m *MockContainerForPubSub) NotificationService() notifications.NotificationServiceInterface {
	return nil
}
func (m *MockContainerForPubSub) DigestRunner() *notifications.DigestRunner { return nil }
func (m *MockContainerForPubSub) DeviceTokenRepository() repositories.DeviceTokenRepository {
	return nil
}
func (m *MockContainerForPubSub) ProjectMigrationService() services.ProjectMigrationServiceInterface {
	return nil
}

func (m *MockContainerForPubSub) TypeService() services.TypeServiceInterface { return nil }

func (m *MockContainerForPubSub) AttachmentService() services.AttachmentServiceInterface { return nil }
