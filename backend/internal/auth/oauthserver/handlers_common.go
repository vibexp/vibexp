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
	AuthorizePath   = "/oauth2/authorize"
	TokenPath       = "/oauth2/token" // #nosec G101 -- OAuth token endpoint path, not a credential
	RegisterPath    = "/oauth2/register"
	JWKSPath        = "/oauth2/jwks.json"
	IDPCallbackPath = "/oauth2/idp/callback"
	ConsentPath     = "/oauth2/consent"
	MetadataPath    = "/.well-known/oauth-authorization-server"

	randomIDBytes = 32
)

// ScopeMCP is the OAuth scope advertised for access to the VibeXP MCP resource.
// It is advisory: the AS grants only the scopes a client requests (exact-match
// strategy), and the MCP resource server does not require it on a token — any
// valid, audience-bound token authorizes an MCP call. It is published in the AS
// and protected-resource metadata and in the WWW-Authenticate challenge so
// clients know which scope to request.
const ScopeMCP = "mcp"

// idpCallbackURI is the absolute redirect URI the AS registers with the upstream
// IdP; operators must allow-list it in their IdP application configuration.
func (s *Service) idpCallbackURI() string {
	return strings.TrimRight(s.cfg.Issuer, "/") + IDPCallbackPath
}

func (s *Service) consentRedirect(loginID string) string {
	return strings.TrimRight(s.cfg.Issuer, "/") + ConsentPath + "?login=" + url.QueryEscape(loginID)
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

// renderError writes a minimal, non-leaking error page. It is used only for
// failures outside an OAuth redirect context (login/consent plumbing); protocol
// errors with a valid client redirect go through fosite's error writers.
func (s *Service) renderError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if _, err := w.Write([]byte(msg)); err != nil {
		s.logger.With("error", err).Error("oauth AS failed to write error response")
	}
}

// writeJSON writes a JSON body with the given status.
func (s *Service) writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, werr := w.Write(body); werr != nil {
		s.logger.With("error", werr).Error("oauth AS failed to write json response")
	}
}

// maxJSONBody bounds the registration request body; maxFormBody bounds the
// consent form POST.
const (
	maxJSONBody = 64 * 1024
	maxFormBody = 16 * 1024
)

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
