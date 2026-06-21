package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/aiclient"
	"github.com/vibexp/vibexp/internal/logging"
)

// EmbeddingHandlers processes embedding responses from the AI service.
// Implementations receive the entity type (e.g. "prompt", "artifact", "memory")
// and the raw payload map so they can extract entity ID and chunks without an
// import cycle between pkg/events and internal/services.
type EmbeddingHandlers interface {
	HandleEmbedding(entityType string, payload map[string]interface{}) error
}

// HTTPSyncListener sends events to AI service synchronously and processes responses immediately
// This is used when Pub/Sub is disabled for local development
type HTTPSyncListener struct {
	client            *http.Client
	aiServiceURL      string
	eventTypes        []string
	logger            *logrus.Logger
	embeddingHandlers EmbeddingHandlers
}

// HTTPSyncListenerConfig holds configuration for the HTTP sync listener
type HTTPSyncListenerConfig struct {
	AIServiceURL      string
	EventTypes        []string
	Logger            *logrus.Logger
	EmbeddingHandlers EmbeddingHandlers
}

// NewHTTPSyncListener creates a new HTTP sync listener
func NewHTTPSyncListener(config HTTPSyncListenerConfig) (*HTTPSyncListener, error) {
	if config.AIServiceURL == "" {
		return nil, fmt.Errorf("AI service URL is required")
	}
	if config.EmbeddingHandlers == nil {
		return nil, fmt.Errorf("embedding handlers are required")
	}
	if config.Logger == nil {
		config.Logger = logging.NewCloudLogger(logging.CloudLoggerConfig{})
	}

	return &HTTPSyncListener{
		// Longer timeout for embedding generation. The client attaches an OIDC ID
		// token on Authorization for Cloud Run service-to-service auth (see
		// internal/aiclient); ai-service verifies the caller's SA identity.
		client:            aiclient.New(context.Background(), config.AIServiceURL, 60*time.Second, config.Logger),
		aiServiceURL:      config.AIServiceURL,
		eventTypes:        config.EventTypes,
		logger:            config.Logger,
		embeddingHandlers: config.EmbeddingHandlers,
	}, nil
}

// Handle processes an event by calling AI service synchronously
func (l *HTTPSyncListener) Handle(ctx context.Context, event Event) error {
	startTime := time.Now()

	requestBytes, err := l.createRequestPayload(event)
	if err != nil {
		return err
	}

	resp, err := l.sendEventToAIService(ctx, event, requestBytes, startTime)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			l.logger.WithError(closeErr).Error("Failed to close response body")
		}
	}()

	duration := time.Since(startTime)

	response, err := l.parseAIServiceResponse(resp, event, duration)
	if err != nil {
		return err
	}

	return l.processEmbeddingResponse(event, response, duration)
}

func (l *HTTPSyncListener) createRequestPayload(event Event) ([]byte, error) {
	requestData := map[string]interface{}{
		"type":      event.Type(),
		"user_id":   event.UserID(),
		"timestamp": event.Timestamp().Format(time.RFC3339),
		"payload":   event.Payload(),
	}

	requestBytes, err := json.Marshal(requestData)
	if err != nil {
		l.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"component":  "http-sync-listener",
			"event_type": event.Type(),
			"error":      fmt.Sprintf("%+v", err),
		}).Error("Failed to marshal request")
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	return requestBytes, nil
}

func (l *HTTPSyncListener) sendEventToAIService(
	ctx context.Context,
	event Event,
	requestBytes []byte,
	startTime time.Time,
) (*http.Response, error) {
	endpoint := fmt.Sprintf("%s/api/v1/events/sync", l.aiServiceURL)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(requestBytes))
	if err != nil {
		l.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"component":  "http-sync-listener",
			"event_type": event.Type(),
			"error":      fmt.Sprintf("%+v", err),
		}).Error("Failed to create HTTP request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	// Auth is the Cloud Run OIDC ID token attached by the aiclient transport
	// (see internal/aiclient); ai-service verifies the caller's SA identity.

	l.logger.WithFields(logrus.Fields{
		"service":    "vibexp-api",
		"component":  "http-sync-listener",
		"event_type": event.Type(),
		"user_id":    event.UserID(),
		"endpoint":   endpoint,
	}).Info("Sending event to AI service")

	// #nosec G704 - URL is from system configuration (l.aiServiceURL), not user input
	resp, err := l.client.Do(req)
	if err != nil {
		l.logger.WithFields(logrus.Fields{
			"service":     "vibexp-api",
			"component":   "http-sync-listener",
			"event_type":  event.Type(),
			"endpoint":    endpoint,
			"error":       fmt.Sprintf("%+v", err),
			"duration_ms": time.Since(startTime).Milliseconds(),
		}).Error("Failed to call AI service")
		return nil, fmt.Errorf("failed to call AI service: %w", err)
	}

	return resp, nil
}

func (l *HTTPSyncListener) parseAIServiceResponse(
	resp *http.Response,
	event Event,
	duration time.Duration,
) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("%s/api/v1/events/sync", l.aiServiceURL)

	if resp.StatusCode != http.StatusOK {
		l.logger.WithFields(logrus.Fields{
			"service":     "vibexp-api",
			"component":   "http-sync-listener",
			"event_type":  event.Type(),
			"endpoint":    endpoint,
			"status_code": resp.StatusCode,
			"duration_ms": duration.Milliseconds(),
		}).Error("AI service returned non-OK status")
		return nil, fmt.Errorf("AI service returned status %d", resp.StatusCode)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		l.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"component":  "http-sync-listener",
			"event_type": event.Type(),
			"error":      fmt.Sprintf("%+v", err),
		}).Error("Failed to decode AI service response")
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response, nil
}

func (l *HTTPSyncListener) processEmbeddingResponse(
	event Event,
	response map[string]interface{},
	duration time.Duration,
) error {
	responseType, ok := response["type"].(string)
	if !ok {
		l.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"component":  "http-sync-listener",
			"event_type": event.Type(),
		}).Error("Response missing type field")
		return fmt.Errorf("response missing type field")
	}

	payload, ok := response["payload"].(map[string]interface{})
	if !ok {
		l.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"component":  "http-sync-listener",
			"event_type": event.Type(),
		}).Error("Response missing payload field")
		return fmt.Errorf("response missing payload field")
	}

	handlerErr := l.routeToEmbeddingHandler(responseType, payload)
	if handlerErr != nil {
		l.logger.WithFields(logrus.Fields{
			"service":       "vibexp-api",
			"component":     "http-sync-listener",
			"event_type":    event.Type(),
			"response_type": responseType,
			"error":         fmt.Sprintf("%+v", handlerErr),
		}).Error("Failed to process embedding")
		return fmt.Errorf("failed to process embedding: %w", handlerErr)
	}

	l.logger.WithFields(logrus.Fields{
		"service":       "vibexp-api",
		"component":     "http-sync-listener",
		"event_type":    event.Type(),
		"response_type": responseType,
		"duration_ms":   duration.Milliseconds(),
	}).Info("Successfully processed embedding from AI service")

	return nil
}

// routeToEmbeddingHandler routes an AI service response to the embedding handler.
// It extracts the entity type from the "<entity>.embedding.generated" response type string
// and delegates to the single generic HandleEmbedding method.
func (l *HTTPSyncListener) routeToEmbeddingHandler(responseType string, payload map[string]interface{}) error {
	const suffix = ".embedding.generated"
	if !strings.HasSuffix(responseType, suffix) {
		l.logger.WithFields(logrus.Fields{
			"service":       "vibexp-api",
			"component":     "http-sync-listener",
			"response_type": responseType,
		}).Warn("Unknown response type from AI service")
		return fmt.Errorf("unknown response type: %s", responseType)
	}

	entityType := strings.TrimSuffix(responseType, suffix)
	if entityType == "" {
		l.logger.WithFields(logrus.Fields{
			"service":       "vibexp-api",
			"component":     "http-sync-listener",
			"response_type": responseType,
		}).Warn("Empty entity type in response type")
		return fmt.Errorf("empty entity type in response type: %s", responseType)
	}

	return l.embeddingHandlers.HandleEmbedding(entityType, payload)
}

// EventTypes returns the event types this listener handles
func (l *HTTPSyncListener) EventTypes() []string {
	return l.eventTypes
}
