package oauthserver

import (
	"net/http"
	"strings"
)

// authorizationServerMetadata is the RFC 8414 document advertised at
// /.well-known/oauth-authorization-server.
type authorizationServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	JWKSURI                           string   `json:"jwks_uri"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
	ResourceIndicatorsSupported       bool     `json:"resource_indicators_supported"`
}

// Metadata handles GET /.well-known/oauth-authorization-server (RFC 8414).
func (s *Service) Metadata(w http.ResponseWriter, r *http.Request) {
	base := strings.TrimRight(s.cfg.Issuer, "/")
	s.writeJSON(w, http.StatusOK, authorizationServerMetadata{
		Issuer:                            s.cfg.Issuer,
		AuthorizationEndpoint:             base + AuthorizePath,
		TokenEndpoint:                     base + TokenPath,
		RegistrationEndpoint:              base + RegisterPath,
		JWKSURI:                           base + JWKSPath,
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported:     []string{"S256"},
		TokenEndpointAuthMethodsSupported: []string{"none"},
		ScopesSupported:                   []string{ScopeMCP},
		ResourceIndicatorsSupported:       true,
	})
}

// JWKS handles GET /oauth2/jwks.json, publishing active and retired public keys.
func (s *Service) JWKS(w http.ResponseWriter, r *http.Request) {
	set, err := s.keys.PublicJWKS(r.Context())
	if err != nil {
		s.logger.With("error", err).Error("oauth AS failed to load JWKS")
		s.renderError(w, "could not load signing keys")
		return
	}
	data, err := publicJWKSJSON(set)
	if err != nil {
		s.renderError(w, "could not encode signing keys")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, werr := w.Write(data); werr != nil {
		s.logger.With("error", werr).Error("oauth AS failed to write JWKS response")
	}
}
