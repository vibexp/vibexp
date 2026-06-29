package oauthserver

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Endpoint paths for the Authorization Server, mounted by the HTTP server.
const (
	AuthorizePath = "/oauth2/authorize"
	TokenPath     = "/oauth2/token" // #nosec G101 -- OAuth token endpoint path, not a credential
	RegisterPath  = "/oauth2/register"
	JWKSPath      = "/oauth2/jwks.json"
	MetadataPath  = "/.well-known/oauth-authorization-server"

	// ConsentPagePath is the frontend SPA route the post-login browser is
	// redirected to (on FrontendBaseURL) to render the consent screen (issue #52).
	ConsentPagePath = "/oauth/consent"
	// ConsentAPIPath is the JSON consent endpoint the SPA calls. It lives in the
	// /api/v1 namespace, served same-origin alongside the SPA from the combined
	// image (issue #61), unlike the /oauth2/* protocol routes. It replaces the
	// former server-rendered /oauth2/consent HTML page.
	ConsentAPIPath = "/api/v1/oauth/consent"
	// ConsentAttachPath binds the authenticated app user to a user-less login
	// session (issue #54). Unlike ConsentAPIPath it is mounted behind the standard
	// /api auth middleware: the AS itself never authenticates anyone.
	ConsentAttachPath = "/api/v1/oauth/consent/attach"

	randomIDBytes = 32
)

// ScopeMCP is the OAuth scope advertised for access to the VibeXP MCP resource.
// It is advisory: the AS grants only the scopes a client requests (exact-match
// strategy), and the MCP resource server does not require it on a token — any
// valid, audience-bound token authorizes an MCP call. It is published in the AS
// and protected-resource metadata and in the WWW-Authenticate challenge so
// clients know which scope to request.
const ScopeMCP = "mcp"

// consentRedirect builds the SPA consent-page URL the browser is sent to after
// login: it targets the frontend origin (not the OAuth issuer), so the consent UI
// is served by the SPA and matches the design system (issue #52).
func (s *Service) consentRedirect(loginID string) string {
	return strings.TrimRight(s.cfg.FrontendBaseURL, "/") + ConsentPagePath + "?login=" + url.QueryEscape(loginID)
}

// randomID returns an unguessable URL-safe identifier.
func randomID() (string, error) {
	b := make([]byte, randomIDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("oauthserver: generate random id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// signConsent returns a CSRF token binding the consent form to a login session.
func (s *Service) signConsent(loginID string) string {
	mac := hmac.New(sha256.New, s.consentMACKey)
	mac.Write([]byte(loginID))
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyConsent checks a CSRF token in constant time.
func (s *Service) verifyConsent(loginID, token string) bool {
	expected := s.signConsent(loginID)
	return hmac.Equal([]byte(expected), []byte(token))
}

// renderError writes a minimal, non-leaking 500 error page for internal failures
// outside an OAuth redirect context (JWKS/registration/JSON-encoding plumbing);
// protocol errors with a valid client redirect go through fosite's error writers.
func (s *Service) renderError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusInternalServerError)
	if _, err := w.Write([]byte(msg)); err != nil {
		s.logger.With("error", err).Error("oauth AS failed to write error response")
	}
}

// writeJSON writes a JSON body with the given status.
func (s *Service) writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		s.renderError(w, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, werr := w.Write(body); werr != nil {
		s.logger.With("error", werr).Error("oauth AS failed to write json response")
	}
}

// writeJSONError writes a non-leaking JSON error body for the consent API. The
// SPA shows its own friendly wording, so only a short, safe message is exposed
// (no stack/HTML, no internal detail).
func (s *Service) writeJSONError(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]string{"error": msg})
}

// maxJSONBody bounds JSON request bodies (registration metadata, consent decision).
const maxJSONBody = 64 * 1024

// decodeJSONBody strictly decodes a bounded JSON request body.
func decodeJSONBody(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxJSONBody)
	// Unknown RFC 7591 metadata fields are ignored rather than rejected.
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return fmt.Errorf("oauthserver: decode json body: %w", err)
	}
	return nil
}

// hostOf returns the host of a redirect URI for display on the consent screen.
func hostOf(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Host
}
