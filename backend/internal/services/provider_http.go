package services

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Shared outbound-HTTP construction for the bring-your-own provider clients
// (embedding + model). Both take a caller-supplied base_url and connect to it,
// so both must go through the SSRF guard — the destination is attacker-chosen
// on any instance where a team member can edit provider settings (#464).

// Fixed categories for ValidateProviderResponse.Details.ErrorDetails.
//
// The raw error used to be echoed here, which handed the caller a precise
// success/failure oracle: "connection refused" vs "status 401" vs a DNS failure
// cleanly separates a closed port, an open port, and an authenticated service on
// the host's internal network. These categories tell an operator what to fix
// without distinguishing those cases; the real error is logged server-side (#464).
const (
	providerErrDestinationNotAllowed = "destination_not_allowed"
	providerErrMisconfigured         = "misconfigured_provider"
	providerErrUnauthorized          = "unauthorized"
	providerErrBadDimension          = "bad_dimension"
	providerErrConnectionFailed      = "connection_failed"
)

// logProviderValidationFailure records the real failure server-side, where the
// detail is useful, instead of returning it to the caller.
func logProviderValidationFailure(kind, baseURL string, err error) {
	slog.Warn("Provider validation probe failed",
		"provider_kind", kind,
		"base_url", baseURL,
		"error", err,
	)
}

// validateProviderBaseURLScheme rejects a base_url whose scheme is not http or
// https. Without this, schemes like file:// or gopher:// reach the transport and
// the dial-time guard (which only sees IPs) is not the right place to catch it.
func validateProviderBaseURLScheme(baseURL string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid base_url: %w", err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return nil
	case "":
		return fmt.Errorf("base_url must include a scheme (http or https)")
	default:
		return fmt.Errorf("base_url scheme %q is not supported (use http or https)", parsed.Scheme)
	}
}

// newProviderHTTPClient builds the HTTP client a provider uses to reach its
// configured base_url. The transport carries the guard's dial-time Control hook,
// so a hostname that resolves (or re-resolves) to a reserved range is refused at
// connect time — this is what covers DNS rebinding, and it applies to the stored
// provider's runtime traffic, not only the validate probe.
//
// A nil guard falls back to the fail-closed production policy rather than an
// unguarded client, so forgetting to thread one through cannot silently reopen
// the hole.
func newProviderHTTPClient(guard *ssrfGuard, timeout time.Duration) *http.Client {
	if guard == nil {
		guard = defaultSSRFGuard
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: guard.newSSRFSafeTransport(&http.Transport{}),
	}
}

// isDuplicateProviderError reports whether a repository Create failure is the
// database's unique-constraint rejection rather than a real failure. Shared by
// both provider services, which had the same three-way string match inline.
func isDuplicateProviderError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "unique constraint")
}
