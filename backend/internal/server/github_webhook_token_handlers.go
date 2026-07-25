package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Per-App webhook delivery (#481, epic #476).
//
// Each team's App has its own webhook secret, so one global endpoint with one
// secret cannot work. Deliveries arrive at a per-App URL carrying an opaque
// routing token, and the token is what selects the secret to verify against.
//
// THE ORDER BELOW IS THE SECURITY PROPERTY. A delivery is:
//  1. rejected on token charset before any database work,
//  2. resolved to an App config (unknown token → bare 404, no HMAC computed),
//  3. verified against THAT App's secret,
//  4. and only then parsed.
//
// No payload byte is trusted before the signature is verified. That is the
// whole reason for a per-App URL rather than "parse the body to find the
// installation, then pick a secret" — the latter has to trust attacker-supplied
// JSON to decide which key to check it with.

// githubWebhookTokenPattern matches what the service mints:
// base64.RawURLEncoding, so unpadded and URL-safe BY CONSTRUCTION. Nothing here
// is percent-decoded, because nothing minted this way ever needs it (#251/#257).
var githubWebhookTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// maxWebhookBodyBytes caps a delivery at 64 KB, as the previous handler did.
const maxWebhookBodyBytes = int64(65536)

// githubWebhookSignatureHeader is GitHub's HMAC-SHA256 signature header.
const githubWebhookSignatureHeader = "X-Hub-Signature-256"

// githubWebhookSignaturePrefix is the algorithm prefix on that header's value.
const githubWebhookSignaturePrefix = "sha256="

// handleGitHubWebhookByToken serves POST /api/v1/webhooks/github/{token}.
func (s *Server) handleGitHubWebhookByToken(w http.ResponseWriter, r *http.Request) {
	// (1) Charset first: an unknown or malformed token must be cheap to reject.
	// This endpoint is public and unauthenticated, so a bad token costs one
	// regexp — no database round-trip, no HMAC, no body read.
	token := chi.URLParam(r, "token")
	if !githubWebhookTokenPattern.MatchString(token) {
		s.logger.Warn("Rejected GitHub webhook with a malformed routing token")
		writeErrorResponse(w, r, "not_found", "Not found", http.StatusNotFound)
		return
	}

	// (2) Resolve the token to an App. A 404 with no detail: distinguishing
	// "unknown token" from "malformed token" would turn this into an
	// enumeration oracle.
	target, err := s.container.GitHubAppConfigService().ResolveWebhookToken(r.Context(), token)
	if err != nil {
		s.logger.With("error", err).Warn("Unresolvable GitHub webhook routing token")
		writeErrorResponse(w, r, "not_found", "Not found", http.StatusNotFound)
		return
	}

	signature := r.Header.Get(githubWebhookSignatureHeader)
	if signature == "" {
		s.logger.With("team_id", target.TeamID).Warn("Missing GitHub webhook signature")
		writeErrorResponse(w, r, "unauthorized", "Missing signature", http.StatusUnauthorized)
		return
	}

	body, ok := s.readWebhookBody(w, r)
	if !ok {
		return
	}

	// (3) Verify against THIS App's secret, constant-time.
	if !verifyGitHubSignature(body, signature, target.WebhookSecret) {
		s.logger.With("team_id", target.TeamID).Warn("Invalid GitHub webhook signature")
		writeErrorResponse(w, r, "unauthorized", "Invalid signature", http.StatusUnauthorized)
		return
	}

	// (4) Signature verified — the payload can now be trusted enough to parse.
	s.processVerifiedWebhook(w, r, target.AppConfigID, body)
}

// readWebhookBody reads the delivery body under the size cap.
func (s *Server) readWebhookBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxWebhookBodyBytes)
	defer func() {
		if closeErr := r.Body.Close(); closeErr != nil {
			s.logger.With("error", closeErr).Debug("Failed to close webhook body")
		}
	}()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.With("error", err).Warn("Failed to read GitHub webhook body")
		writeErrorResponse(w, r, "invalid_request", "Failed to read body", http.StatusBadRequest)
		return nil, false
	}
	return body, true
}

// processVerifiedWebhook handles a delivery whose signature already verified:
// dedup by delivery id, parse, dispatch scoped to the owning App, and record.
func (s *Server) processVerifiedWebhook(
	w http.ResponseWriter, r *http.Request, appConfigID string, body []byte,
) {
	eventType := r.Header.Get("X-GitHub-Event")
	deliveryID := r.Header.Get("X-GitHub-Delivery")

	// GitHub delivery ids are globally unique, so the shared webhook_events
	// table still dedups correctly across per-team Apps.
	processed, err := s.container.WebhookEventRepository().IsProcessed(r.Context(), deliveryID)
	if err != nil {
		s.logger.With("error", err).Error("Failed to check webhook event")
		writeErrorResponse(w, r, "internal_error", "Failed to process webhook", http.StatusInternalServerError)
		return
	}
	if processed {
		s.logger.With("delivery_id", deliveryID).Info("Webhook event already processed")
		w.WriteHeader(http.StatusOK)
		return
	}

	var payload GitHubWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		s.logger.With("error", err).Warn("Failed to parse GitHub webhook payload")
		writeErrorResponse(w, r, "invalid_request", "Invalid payload", http.StatusBadRequest)
		return
	}
	if payload.Installation == nil {
		s.logger.Warn("GitHub webhook missing installation reference")
		writeErrorResponse(w, r, "invalid_request", "Missing installation", http.StatusBadRequest)
		return
	}

	// Dispatch scoped to the App that received the delivery: installation ids
	// are unique per App, not globally.
	if err := s.container.GitHubAppService().HandleWebhookEvent(
		r.Context(), appConfigID, eventType, payload.Installation.ID, payload.Action,
	); err != nil {
		s.logger.With("error", err).Error("Failed to handle GitHub webhook event")
		writeErrorResponse(w, r, "internal_error", "Failed to process event", http.StatusInternalServerError)
		return
	}

	if err := s.container.WebhookEventRepository().MarkProcessed(
		r.Context(), deliveryID, eventType, nil,
	); err != nil {
		s.logger.With("error", err).Error("Failed to mark webhook as processed")
	}

	w.WriteHeader(http.StatusOK)
}

// verifyGitHubSignature checks an X-Hub-Signature-256 header against a secret.
//
// An empty secret rejects everything: a config that somehow stored no secret
// must not be treated as "any signature is fine".
func verifyGitHubSignature(payload []byte, signature, secret string) bool {
	if secret == "" {
		return false
	}
	if !strings.HasPrefix(signature, githubWebhookSignaturePrefix) {
		return false
	}
	provided := strings.TrimPrefix(signature, githubWebhookSignaturePrefix)

	mac := hmac.New(sha256.New, []byte(secret))
	// #nosec G104 - Write to hash never fails
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(provided), []byte(expected))
}

// handleRetiredGitHubWebhook answers the pre-#476 webhook routes.
//
// They cannot keep working: they verified against the instance-wide secret that
// per-team Apps replace. A 410 says "this endpoint is gone" rather than letting
// deliveries fail as 401s that look like a secret mismatch.
func (s *Server) handleRetiredGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	s.logger.Warn("Delivery to a retired GitHub webhook endpoint")
	writeErrorResponse(w, r, "gone",
		"This webhook endpoint has been retired. Each team's GitHub App now has its own webhook URL — "+
			"find it under the team's GitHub App settings and update it on GitHub.",
		http.StatusGone)
}
