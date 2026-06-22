package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// PubSubMessage represents the structure of a Pub/Sub push message
type PubSubMessage struct {
	Message struct {
		Data        string            `json:"data"`
		MessageID   string            `json:"messageId"`
		Attributes  map[string]string `json:"attributes"`
		PublishTime string            `json:"publishTime"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

// PubSubEventPayload represents the actual event data we forwarded
type PubSubEventPayload struct {
	Type      string                 `json:"type"`
	Payload   map[string]interface{} `json:"payload"`
	Timestamp string                 `json:"timestamp"`
	UserID    string                 `json:"user_id"`
}

// handlePubSubPush handles incoming Pub/Sub push notifications
// OIDC authentication is handled by the pubSubOIDCMiddleware
func (s *Server) handlePubSubPush(w http.ResponseWriter, r *http.Request) {
	// Get service account from context (set by middleware)
	serviceAccount, _ := r.Context().Value("pubsub_service_account").(string)

	pubsubMsg, eventPayload, err := s.parsePubSubMessage(r, serviceAccount)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	statusCode := s.routeEventToHandler(eventPayload, pubsubMsg.Message.MessageID)

	w.WriteHeader(statusCode)
	if statusCode == http.StatusOK {
		if _, err := w.Write([]byte("OK")); err != nil {
			s.logger.With("error", err).Error("Failed to write response")
		}
	} else {
		if _, err := w.Write([]byte("Error processing event")); err != nil {
			s.logger.With("error", err).Error("Failed to write response")
		}
	}
}

func (s *Server) parsePubSubMessage(
	r *http.Request,
	serviceAccount string,
) (*PubSubMessage, *PubSubEventPayload, error) {
	var pubsubMsg PubSubMessage
	if err := json.NewDecoder(r.Body).Decode(&pubsubMsg); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "pubsub-push",
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to decode Pub/Sub message")
		return nil, nil, err
	}

	decodedData, err := base64.StdEncoding.DecodeString(pubsubMsg.Message.Data)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "pubsub-push",
			"message_id", pubsubMsg.Message.MessageID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to decode message data")
		return nil, nil, err
	}

	var eventPayload PubSubEventPayload
	if err := json.Unmarshal(decodedData, &eventPayload); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "pubsub-push",
			"message_id", pubsubMsg.Message.MessageID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to parse event payload")
		return nil, nil, err
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "pubsub-push",
		"event_type", eventPayload.Type,
		"user_id", eventPayload.UserID,
		"message_id", pubsubMsg.Message.MessageID,
		"publish_time", pubsubMsg.Message.PublishTime,
		"subscription", pubsubMsg.Subscription,
		"service_account", serviceAccount,
		"message_attributes", pubsubMsg.Message.Attributes,
	).Info("Received event from Pub/Sub")

	return &pubsubMsg, &eventPayload, nil
}

// routeEventToHandler dispatches a Pub/Sub event to the appropriate handler.
// Events whose type matches the "<entity>.embedding.generated" pattern are routed
// to the generic embedding handler. All other events are acknowledged without
// processing to prevent Pub/Sub from retrying unknown types.
func (s *Server) routeEventToHandler(eventPayload *PubSubEventPayload, messageID string) int {
	const embeddingSuffix = ".embedding.generated"
	if strings.HasSuffix(eventPayload.Type, embeddingSuffix) {
		entityType := strings.TrimSuffix(eventPayload.Type, embeddingSuffix)
		if entityType == "" {
			s.logger.With(
				"service", "vibexp-api",
				"handler", "pubsub-push",
				"event_type", eventPayload.Type,
				"message_id", messageID,
			).Warn("Malformed embedding event type — acknowledging to prevent retries")
			return http.StatusOK
		}
		return s.handleEntityEmbeddingGenerated(entityType, *eventPayload, messageID)
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "pubsub-push",
		"event_type", eventPayload.Type,
		"message_id", messageID,
		"user_id", eventPayload.UserID,
	).Warn("Unknown event type received - acknowledging to prevent retries")
	return http.StatusOK
}
