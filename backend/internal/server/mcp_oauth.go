package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/vibexp/vibexp/internal/auth/mcptoken"
	"github.com/vibexp/vibexp/internal/auth/oauthserver"
	"github.com/vibexp/vibexp/internal/contextkeys"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/repositories"
)

const (
	// protectedResourceMetadataPrefix is the RFC 9728 well-known prefix. The
	// resource path is inserted after it (path-insertion form).
	protectedResourceMetadataPrefix = "/.well-known/oauth-protected-resource"

	// mcpAuthorizationServerMetadataPath is the legacy AS-metadata probe path some
	// older MCP clients hit on the resource server. We redirect it to AuthKit.
	mcpAuthorizationServerMetadataPath = "/.well-known/oauth-authorization-server"
)

// deriveMCPMetadata computes, from the configured MCP resource URI, the RFC 9728
// protected-resource metadata route path (relative) and its absolute URL. The
// absolute URL is advertised in the WWW-Authenticate challenge and the route
// path is where the metadata document is served; deriving both from the same
// source keeps them consistent across environments (the issuer and resource are
// per-environment, so this must not be hardcoded to any host).
//
// If MCP_RESOURCE_URI is empty or unparseable (MCP not configured), it returns
// only the bare well-known prefix as the path and an empty absolute URL; the MCP
// endpoint rejects all tokens in that mode anyway, so no host needs to be
// invented.
func deriveMCPMetadata(resourceURI string) (metadataPath, metadataURL string) {
	u, err := url.Parse(resourceURI)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return protectedResourceMetadataPrefix, ""
	}

	resourcePath := strings.TrimSuffix(u.Path, "/")
	metadataPath = protectedResourceMetadataPrefix + resourcePath
	metadataURL = u.Scheme + "://" + u.Host + metadataPath
	return metadataPath, metadataURL
}

// userResolverAdapter adapts the user repository to mcptoken.UserResolver,
// returning the internal VibeXP user ID for a (provider, subject) tuple.
type userResolverAdapter struct {
	users repositories.UserRepository
}

// ResolveUserID looks up the internal user ID for the given IDP subject.
func (a userResolverAdapter) ResolveUserID(ctx context.Context, provider, subject string) (string, error) {
	user, err := a.users.GetByIDPSubject(ctx, provider, subject)
	if err != nil {
		return "", err
	}
	if user == nil {
		return "", nil
	}
	return user.ID, nil
}

// mcpUserResolverAdapter resolves the subject of an MCP access token to an
// internal VibeXP user ID. VibeXP's embedded Authorization Server mints tokens
// whose `sub` is already the internal user ID (see oauthserver.newIssuingSession),
// so the subject is resolved directly via GetByID rather than as an IdP subject.
// The provider argument from the authkit.UserResolver contract is unused here;
// the IdP-subject path (web/API tokens) keeps userResolverAdapter unchanged.
type mcpUserResolverAdapter struct {
	users repositories.UserRepository
}

// ResolveUserID looks up the internal user by treating the token subject as the
// VibeXP user ID. A missing user surfaces repositories.ErrUserNotFound, which
// authkit maps to an unknown-subject auth failure (401).
func (a mcpUserResolverAdapter) ResolveUserID(ctx context.Context, _, subject string) (string, error) {
	user, err := a.users.GetByID(ctx, subject)
	if err != nil {
		return "", err
	}
	if user == nil {
		return "", nil
	}
	return user.ID, nil
}

// setupMCPRoutes mounts the MCP endpoint as an OAuth 2.1 resource server. The
// route is always mounted so unauthenticated requests get a spec-compliant 401
// with a WWW-Authenticate challenge. When MCP OAuth is not configured (no issuer
// — stub/test environments) a verifier that rejects every token is used, so the
// endpoint denies all access rather than disappearing.
func (s *Server) setupMCPRoutes() {
	verifier, err := s.newMCPTokenVerifier()
	if err != nil {
		s.logger.Error("Failed to initialize MCP OAuth token verifier", "error", err)
		os.Exit(1)
	}

	verify := unconfiguredMCPVerifier
	if verifier != nil {
		verify = verifier.Verify
	} else {
		s.logger.Warn("MCP_OAUTH_ISSUER not set; MCP endpoint will reject all tokens (stub/test mode)")
	}

	_, metadataURL := deriveMCPMetadata(s.config.MCPResourceURI)
	requireToken := mcpauth.RequireBearerToken(verify, &mcpauth.RequireBearerTokenOptions{
		ResourceMetadataURL: metadataURL,
	})

	s.router.Group(func(r chi.Router) {
		// advertiseChallengeScope runs outermost so it can append the advisory
		// scope to the WWW-Authenticate challenge RequireBearerToken emits, without
		// the SDK's opts.Scopes enforcement (which would 403 valid tokens that did
		// not request the scope — the AS grants scopes exact-match).
		r.Use(advertiseChallengeScope(oauthserver.ScopeMCP))
		r.Use(requireToken)
		r.Use(s.mcpTokenContextMiddleware)
		r.Mount("/mcp/v1/common", s.createMCPHandlerCommon())
	})
}

// advertiseChallengeScope augments the WWW-Authenticate Bearer challenge on 401
// and 403 responses with a `scope` parameter, so a client knows which scope to
// request (RFC 6750 §3). It is purely informational: unlike the MCP SDK's
// opts.Scopes — which also rejects an otherwise-valid token that lacks the scope
// — this never blocks a request, it only annotates the challenge the inner
// RequireBearerToken middleware already produced.
func advertiseChallengeScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(&challengeScopeWriter{ResponseWriter: w, scope: scope}, r)
		})
	}
}

// challengeScopeWriter appends `scope="<scope>"` to an existing Bearer
// WWW-Authenticate header when a 401/403 status is written. It only annotates a
// header already set by the inner middleware (it never creates one) and acts at
// most once.
type challengeScopeWriter struct {
	http.ResponseWriter
	scope       string
	wroteHeader bool
}

func (w *challengeScopeWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		if code == http.StatusUnauthorized || code == http.StatusForbidden {
			if c := w.Header().Get("WWW-Authenticate"); strings.HasPrefix(c, "Bearer") &&
				!strings.Contains(c, "scope=") {
				w.Header().Set("WWW-Authenticate", fmt.Sprintf("%s, scope=%q", c, w.scope))
			}
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

// unconfiguredMCPVerifier rejects every token. It is used when MCP OAuth is not
// configured so the endpoint denies access with a 401 rather than 500.
func unconfiguredMCPVerifier(_ context.Context, _ string, _ *http.Request) (*mcpauth.TokenInfo, error) {
	return nil, fmt.Errorf("%w: MCP OAuth not configured", mcpauth.ErrInvalidToken)
}

// newMCPTokenVerifier builds the AuthKit JWT verifier for the MCP resource. It
// returns nil when MCP OAuth is not configured (no issuer), which the caller
// treats as "MCP OAuth disabled".
func (s *Server) newMCPTokenVerifier() (*mcptoken.Verifier, error) {
	if s.config.MCPOAuthIssuer == "" {
		return nil, nil
	}
	resolver := mcpUserResolverAdapter{users: s.container.UserRepository()}
	return mcptoken.New(
		context.Background(),
		s.config.MCPOAuthIssuer,
		s.config.MCPResourceURI,
		resolver,
	)
}

// mcpProtectedResourceMetadataHandler serves the RFC 9728 protected-resource
// metadata document for the MCP resource. It is a public, no-auth endpoint.
func (s *Server) mcpProtectedResourceMetadataHandler() http.Handler {
	metadata := &oauthex.ProtectedResourceMetadata{
		Resource:               s.config.MCPResourceURI,
		AuthorizationServers:   []string{s.config.MCPOAuthIssuer},
		BearerMethodsSupported: []string{"header"},
		ScopesSupported:        []string{oauthserver.ScopeMCP},
	}
	return mcpauth.ProtectedResourceMetadataHandler(metadata)
}

// handleMCPAuthorizationServerMetadata redirects to the AuthKit authorization
// server metadata document. Older MCP clients probe the resource server for AS
// metadata; AuthKit is the source of truth, so we 302 to it rather than proxy.
func (s *Server) handleMCPAuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	if s.config.MCPOAuthIssuer == "" {
		apiErr := apierrors.NewAPIError(
			"NOT_FOUND",
			"Not Found",
			"The requested resource was not found",
			http.StatusNotFound,
		)
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}
	target := s.config.MCPOAuthIssuer + "/.well-known/oauth-authorization-server"
	http.Redirect(w, r, target, http.StatusFound)
}

// mcpTokenContextMiddleware bridges the OAuth bearer-token context to the
// context keys the MCP tool layer reads. RequireBearerToken stores an
// *auth.TokenInfo in context; the MCP handler expects contextkeys.UserID. This
// middleware runs immediately after RequireBearerToken and copies the resolved
// internal user ID across, mirroring the other auth paths' context setup.
func (s *Server) mcpTokenContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenInfo := mcpauth.TokenInfoFromContext(r.Context())
		if tokenInfo == nil || tokenInfo.UserID == "" {
			apiErr := apierrors.NewAuthRequiredError("Authentication required")
			apierrors.WriteJSONError(w, r, apiErr)
			return
		}

		ctx := context.WithValue(r.Context(), contextkeys.UserID, tokenInfo.UserID)
		ctx = context.WithValue(ctx, contextkeys.AuthType, "oauth")

		updatedLogger := contextkeys.GetLoggerFromContext(ctx).With(
			"auth_type", "oauth",
			"user_id", tokenInfo.UserID,
		)
		ctx = context.WithValue(ctx, contextkeys.Logger, updatedLogger)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
