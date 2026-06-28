package oauthserver

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/models"
)

// Default grant/response types for dynamically-registered public PKCE clients.
var (
	defaultGrantTypes    = []string{"authorization_code", "refresh_token"}
	defaultResponseTypes = []string{"code"}
)

// dcrRequest is the subset of the RFC 7591 client metadata VibeXP honors.
type dcrRequest struct {
	RedirectURIs            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	Scope                   string   `json:"scope"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

// dcrResponse is the RFC 7591 registration response for a public client.
type dcrResponse struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope,omitempty"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
}

type dcrError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// Register handles POST /oauth2/register: Dynamic Client Registration (RFC 7591).
// VibeXP registers PKCE-only public clients (token_endpoint_auth_method "none").
func (s *Service) Register(w http.ResponseWriter, r *http.Request) {
	var req dcrRequest
	if err := decodeJSONBody(r, &req); err != nil {
		s.writeDCRError(w, "invalid_client_metadata", "request body is not valid JSON")
		return
	}
	if errCode, msg := validateDCR(&req); errCode != "" {
		s.writeDCRError(w, errCode, msg)
		return
	}

	client, err := s.persistClient(r.Context(), &req)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, "could not register client")
		return
	}
	s.writeJSON(w, http.StatusCreated, dcrResponse{
		ClientID:                client.ID,
		ClientName:              client.ClientName,
		RedirectURIs:            client.RedirectURIs,
		GrantTypes:              client.GrantTypes,
		ResponseTypes:           client.ResponseTypes,
		TokenEndpointAuthMethod: client.TokenEndpointAuthMethod,
		Scope:                   strings.Join(client.Scopes, " "),
		ClientIDIssuedAt:        client.CreatedAt.Unix(),
	})
}

// persistClient builds and stores a public PKCE client from validated metadata.
func (s *Service) persistClient(ctx context.Context, req *dcrRequest) (*models.OAuthClient, error) {
	clientID, err := randomID()
	if err != nil {
		return nil, err
	}
	client := &models.OAuthClient{
		ID:                      clientID,
		SecretHash:              nil,
		RedirectURIs:            req.RedirectURIs,
		GrantTypes:              defaultGrantTypes,
		ResponseTypes:           defaultResponseTypes,
		Scopes:                  parseScopes(req.Scope),
		Audience:                []string{s.cfg.ResourceURI},
		Public:                  true,
		TokenEndpointAuthMethod: "none",
		ClientName:              req.ClientName,
		CreatedAt:               time.Now(),
	}
	if cerr := s.clients.Create(ctx, client); cerr != nil {
		s.logger.With("error", cerr).Error("oauth AS failed to persist client")
		return nil, cerr
	}
	return client, nil
}

// validateDCR enforces the public-client constraints; it returns an RFC 7591
// error code and description, or ("", "") when valid.
func validateDCR(req *dcrRequest) (code, msg string) {
	if len(req.RedirectURIs) == 0 {
		return "invalid_redirect_uri", "at least one redirect_uri is required"
	}
	for _, raw := range req.RedirectURIs {
		if !validRedirectURI(raw) {
			return "invalid_redirect_uri", "redirect_uri must be an absolute https or loopback URI without a fragment"
		}
	}
	if m := req.TokenEndpointAuthMethod; m != "" && m != "none" {
		return "invalid_client_metadata", "only public clients (token_endpoint_auth_method=none) are supported"
	}
	if !grantTypesSupported(req.GrantTypes) {
		return "invalid_client_metadata", "only authorization_code and refresh_token grant types are supported"
	}
	return "", ""
}

// validRedirectURI requires an absolute URI with no fragment, using https or a
// loopback http address (native apps), rejecting redirect_uri spoofing.
func validRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Fragment != "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme == "http" {
		host := u.Hostname()
		return host == "127.0.0.1" || host == "::1" || host == "localhost"
	}
	// Custom/private-use URI schemes (native apps) are permitted when opaque-free.
	return u.Scheme != "" && u.Host != ""
}

// grantTypesSupported returns true when every requested grant type is supported
// (empty defaults to authorization_code).
func grantTypesSupported(requested []string) bool {
	for _, g := range requested {
		if g != "authorization_code" && g != "refresh_token" {
			return false
		}
	}
	return true
}

func parseScopes(scope string) []string {
	fields := strings.Fields(scope)
	if len(fields) == 0 {
		return []string{}
	}
	return fields
}

func (s *Service) writeDCRError(w http.ResponseWriter, code, desc string) {
	s.writeJSON(w, http.StatusBadRequest, dcrError{Error: code, ErrorDescription: desc})
}
