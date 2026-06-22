// Package aiclient builds HTTP clients for calling the AI service with Cloud Run
// service-to-service authentication: a Google-signed OIDC ID token (audience = the
// AI service URL) attached to the Authorization header. ai-service verifies the
// caller's SA identity from this token. When no ID-token source is available
// (local development without service-account ADC), it falls back to a plain
// client; ai-service accepts that locally (REQUIRE_CALLER_OIDC=false).
package aiclient

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"google.golang.org/api/idtoken"
)

// New returns an *http.Client that attaches a Google-signed OIDC ID token on the
// Authorization header for every request to the AI service, so a locked-down
// (--no-allow-unauthenticated) Cloud Run service accepts the call. On Cloud Run the
// token is minted from the runtime service account via the metadata server.
//
// If an ID-token source cannot be created — e.g. local dev without service-account
// Application Default Credentials — it logs a warning and returns a plain client
// (no token). ai-service accepts unauthenticated callers locally when
// REQUIRE_CALLER_OIDC is disabled. To exercise the OIDC path locally, run with
// service-account ADC.
func New(ctx context.Context, aiServiceURL string, timeout time.Duration, logger *slog.Logger) *http.Client {
	// The audience must equal the receiving service's base URL (no trailing slash).
	audience := strings.TrimSuffix(aiServiceURL, "/")

	client, err := idtoken.NewClient(ctx, audience)
	if err != nil {
		if logger != nil {
			logger.Warn(
				"AI service OIDC ID-token client unavailable; calling without an identity token "+
					"(expected in local dev, not on Cloud Run)",
				"error", err,
			)
		}
		return &http.Client{Timeout: timeout}
	}

	client.Timeout = timeout
	return client
}
