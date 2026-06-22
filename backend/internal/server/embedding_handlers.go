package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/vibexp/vibexp/internal/services"
)

// handleEntityEmbeddingGenerated is the single generic handler for all
// "<entity>.embedding.generated" events. It validates the entity type against
// the shared registry in services, extracts and validates fields from the
// payload, and delegates to EmbeddingService.
func (s *Server) handleEntityEmbeddingGenerated(
	entityType string, eventPayload PubSubEventPayload, messageID string,
) int {
	userID, entityID, model, chunks, status := s.parseEmbeddingPayload(entityType, eventPayload, messageID)
	if status != http.StatusOK {
		return status
	}

	s.logEmbeddingReceived(eventPayload.Type, entityType, entityID, userID, model, messageID, len(chunks))

	if err := s.container.EmbeddingService().SaveEmbeddingChunks(userID, entityType, entityID, model, chunks); err != nil {
		// The entity was deleted before its embedding event arrived — a permanent
		// failure. Ack with 2xx so Pub/Sub drops the message; a 5xx would make it
		// redeliver the same poison message forever.
		if errors.Is(err, services.ErrEntityNotFound) {
			s.logEmbeddingEntityGone(entityType, entityID, userID, messageID, err)
			return http.StatusOK
		}
		s.logEmbeddingError(entityType, entityID, userID, messageID, "save", "Failed to save entity embeddings", err)
		return http.StatusInternalServerError
	}

	s.logger.With(
		"user_id", userID,
		"entity_type", entityType,
		"entity_id", entityID,
		"model", model,
	).Info("Successfully saved entity embeddings")

	return http.StatusOK
}

// parseEmbeddingPayload validates entity type, extracts and validates required fields,
// and parses embedding chunks. Returns http.StatusOK on success or the appropriate error status.
func (s *Server) parseEmbeddingPayload(
	entityType string, eventPayload PubSubEventPayload, messageID string,
) (userID, entityID, model string, chunks []services.EmbeddingChunk, status int) {
	cfg, ok := services.GetEmbeddingEntityConfig(entityType)
	if !ok {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "entity-embedding-generated",
			"entity_type", entityType,
			"message_id", messageID,
		).Error("Unknown entity type for embedding event")
		return "", "", "", nil, http.StatusBadRequest
	}

	userID, _ = eventPayload.Payload["userID"].(string)
	model, _ = eventPayload.Payload["model"].(string)
	entityID, _ = eventPayload.Payload[cfg.EntityIDField].(string)

	if userID == "" || entityID == "" || model == "" {
		s.logger.With(
			"user_id", userID,
			"entity_type", entityType,
			"entity_id", entityID,
			"model", model,
			"message_id", messageID,
		).Error("Invalid payload: missing required fields")
		return "", "", "", nil, http.StatusBadRequest
	}

	embeddingsRaw, _ := eventPayload.Payload["embeddings"].([]interface{})
	chunks, err := services.ParseEmbeddingChunks(embeddingsRaw)
	if err != nil {
		s.logEmbeddingError(entityType, entityID, userID, messageID, "parse", "Failed to parse embedding chunks", err)
		return "", "", "", nil, http.StatusBadRequest
	}
	if len(chunks) == 0 {
		s.logger.With(
			"user_id", userID,
			"entity_type", entityType,
			"entity_id", entityID,
			"model", model,
			"message_id", messageID,
		).Error("Invalid payload: no embedding chunks provided")
		return "", "", "", nil, http.StatusBadRequest
	}

	return userID, entityID, model, chunks, http.StatusOK
}

func (s *Server) logEmbeddingReceived(eventType, entityType, entityID, userID, model, messageID string, count int) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "entity-embedding-generated",
		"event_type", eventType,
		"entity_type", entityType,
		"entity_id", entityID,
		"user_id", userID,
		"model", model,
		"embeddings_count", count,
		"message_id", messageID,
	).Info("Received entity embedding generated event")
}

// logEmbeddingEntityGone records, at WARN rather than ERROR, an embedding event
// acked without saving because its target entity no longer exists — expected
// when an entity is deleted between embedding generation and event delivery.
func (s *Server) logEmbeddingEntityGone(entityType, entityID, userID, messageID string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "entity-embedding-generated",
		"entity_type", entityType,
		"entity_id", entityID,
		"user_id", userID,
		"message_id", messageID,
		"phase", "save",
		"error", fmt.Sprintf("%+v", err),
	).Warn("Entity deleted before embedding arrived; acked event without saving")
}

func (s *Server) logEmbeddingError(entityType, entityID, userID, messageID, phase, msg string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "entity-embedding-generated",
		"entity_type", entityType,
		"entity_id", entityID,
		"user_id", userID,
		"message_id", messageID,
		"phase", phase,
		"error", fmt.Sprintf("%+v", err),
	).Error(msg)
}
