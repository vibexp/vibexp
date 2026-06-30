package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
)

// GitHubWebhookPayload represents the common structure of GitHub webhook payloads
type GitHubWebhookPayload struct {
	Action       string                 `json:"action"`
	Installation *GitHubInstallationRef `json:"installation"`
}

// GitHubInstallationRef represents the installation reference in webhook payloads
type GitHubInstallationRef struct {
	ID int64 `json:"id"`
}

// handleGitHubWebhook processes GitHub App webhook events
//
//nolint:funlen // Webhook handler requires processing multiple event types
func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	// Verify webhook signature
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		s.logger.Warn("Missing webhook signature")
		writeErrorResponse(w, r, "unauthorized", "Missing signature", http.StatusUnauthorized)
		return
	}

	// Limit request body size to prevent denial of service
	const MaxBodyBytes = int64(65536) // 64KB limit for webhook payloads
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	//nolint:errcheck // Body will be closed by io.ReadAll, explicit close for clarity
	defer r.Body.Close()

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error("Failed to read webhook body", "error", err)
		writeErrorResponse(w, r, "invalid_request", "Failed to read body", http.StatusBadRequest)
		return
	}

	// Verify signature
	if !s.verifyGitHubWebhookSignature(body, signature) {
		s.logger.Warn("Invalid webhook signature")
		writeErrorResponse(w, r, "unauthorized", "Invalid signature", http.StatusUnauthorized)
		return
	}

	// Get event type
	eventType := r.Header.Get("X-GitHub-Event")
	deliveryID := r.Header.Get("X-GitHub-Delivery")

	// Check for duplicate event
	processed, err := s.container.WebhookEventRepository().IsProcessed(r.Context(), deliveryID)
	if err != nil {
		s.logger.Error("Failed to check webhook event", "error", err)
		writeErrorResponse(w, r, "internal_error", "Failed to process webhook", http.StatusInternalServerError)
		return
	}

	if processed {
		s.logger.With("delivery_id", deliveryID).Info("Webhook event already processed")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Parse payload
	var payload GitHubWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		s.logger.Error("Failed to parse webhook payload", "error", err)
		writeErrorResponse(w, r, "invalid_request", "Invalid payload", http.StatusBadRequest)
		return
	}

	if payload.Installation == nil {
		s.logger.Warn("Webhook missing installation reference")
		writeErrorResponse(w, r, "invalid_request", "Missing installation", http.StatusBadRequest)
		return
	}

	// Handle the event
	if err := s.container.GitHubAppService().HandleWebhookEvent(
		r.Context(),
		eventType,
		payload.Installation.ID,
		payload.Action,
	); err != nil {
		s.logger.Error("Failed to handle webhook event", "error", err)
		writeErrorResponse(w, r, "internal_error", "Failed to process event", http.StatusInternalServerError)
		return
	}

	// Mark as processed
	if err := s.container.WebhookEventRepository().MarkProcessed(
		r.Context(),
		deliveryID,
		eventType,
		nil,
	); err != nil {
		s.logger.Error("Failed to mark webhook as processed", "error", err)
	}

	w.WriteHeader(http.StatusOK)
}

// verifyGitHubWebhookSignature verifies the HMAC signature of a GitHub webhook
func (s *Server) verifyGitHubWebhookSignature(payload []byte, signature string) bool {
	if s.config.GitHub.WebhookSecret == "" {
		s.logger.Warn("GitHub webhook secret not configured")
		return false
	}

	// Remove "sha256=" prefix
	if len(signature) < 7 {
		return false
	}
	signature = signature[7:]

	// Compute HMAC
	mac := hmac.New(sha256.New, []byte(s.config.GitHub.WebhookSecret))
	// #nosec G104 - Write to hash never fails
	mac.Write(payload)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedMAC))
}
