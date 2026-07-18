package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/vibexp/vibexp/internal/auth/authkit"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/contextkeys"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
)

// refreshLockFor returns a per-user mutex used to serialize refresh-token
// rotations. Many identity providers invalidate a refresh token on use, so
// concurrent refreshes for the same user would all race and most would 401.
// The mutex is created lazily and stored in s.refreshLocks (sync.Map).
func (s *Server) refreshLockFor(userID string) *sync.Mutex {
	m, _ := s.refreshLocks.LoadOrStore(userID, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// isTransientRefreshError reports whether a refresh failure looks transient
// (network/timeout/5xx) vs. permanent (revoked/invalid_grant). On transient
// errors the caller should return 503 and keep the cookie; on permanent
// errors the caller should return 401 and clear the cookie.
func isTransientRefreshError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// The provider client surfaces non-2xx status codes as "endpoint returned %d: ...".
	// 4xx => permanent; 5xx => transient. Network/timeout has no status.
	if strings.Contains(msg, "returned 5") {
		return true
	}
	if strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "EOF") {
		return true
	}
	return false
}

// authenticatedContext returns ctx carrying the authenticated user's context
// keys (UserID, AuthType) and a logger enriched with the auth metadata plus
// any extra fields. Every auth path sets up its success context through this
// helper so the context shape cannot drift between paths.
func authenticatedContext(ctx context.Context, userID, authType string, extra []any) context.Context {
	ctx = context.WithValue(ctx, contextkeys.UserID, userID)
	ctx = context.WithValue(ctx, contextkeys.AuthType, authType)

	fields := []any{"auth_type", authType, "user_id", userID}
	fields = append(fields, extra...)
	logger := contextkeys.GetLoggerFromContext(ctx).With(fields...)
	return context.WithValue(ctx, contextkeys.Logger, logger)
}

// isAPIKey checks if the token is an API key by checking for valid API key prefixes
func isAPIKey(token string) bool {
	return strings.HasPrefix(token, models.PrefixEverything) ||
		strings.HasPrefix(token, models.PrefixAITools) ||
		strings.HasPrefix(token, models.PrefixCLI) ||
		strings.HasPrefix(token, models.PrefixMCP) ||
		strings.HasPrefix(token, models.PrefixVibeXPKey)
}

// flexibleAuthMiddleware supports API key (Bearer header), AuthKit OAuth JWT
// (Bearer header, when API_OAUTH_ISSUER is configured), and cookie-based
// session authentication. The MCP endpoint does not use this middleware: it is
// an OAuth 2.1 resource server guarded by RequireBearerToken, and the insecure
// ?api_key query-string path (forbidden by the MCP authorization spec) has
// been removed.
//
// Authentication order (the Authorization header wins over the cookie; within
// the header, the API-key prefix match is tried before JWT verification):
//  1. If Authorization header contains a known API key prefix → API key auth
//  2. If Authorization header contains any other bearer token → AuthKit JWT
//     auth when configured, otherwise 401
//  3. If vx_session cookie is present → decrypt, check expiry, optionally refresh → session auth
//  4. Otherwise → 401
//
//nolint:gocognit // Auth middleware intentionally handles multiple auth paths
func (s *Server) flexibleAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := contextkeys.GetLoggerFromContext(r.Context())
		logger.With(
			"middleware", "flexibleAuthMiddleware",
		).Debug("Processing flexible authentication")

		// 1. Try API key from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			token, err := s.extractBearerToken(r)
			if err == nil && isAPIKey(token) {
				s.authenticateWithAPIKey(w, r, next, token)
				return
			}
			// Non-API-key bearer tokens are AuthKit OAuth JWTs when the API
			// verifier is configured; otherwise they are rejected as before.
			if err == nil && !isAPIKey(token) {
				if s.apiTokenVerifier != nil {
					s.authenticateWithOAuthJWT(w, r, next, token)
					return
				}
				logger.With(
					"middleware", "flexibleAuthMiddleware",
				).Warn("Bearer token is not a valid API key; cookie session required")
				apiErr := apierrors.NewAuthInvalidError("Invalid token; use session cookie authentication")
				apierrors.WriteJSONError(w, r, apiErr)
				return
			}
		}

		// 2. Try cookie-based session authentication
		if s.sessionManager != nil {
			s.authenticateWithSession(w, r, next)
			return
		}

		// No authentication method succeeded
		apiErr := apierrors.NewAuthRequiredError("Authentication required")
		apierrors.WriteJSONError(w, r, apiErr)
	})
}

func (s *Server) extractBearerToken(r *http.Request) (string, error) {
	logger := contextkeys.GetLoggerFromContext(r.Context())
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		logger.With(
			"middleware", "extractBearerToken",
		).Warn("Authorization header missing")
		return "", http.ErrNoCookie
	}

	bearerToken := strings.Split(authHeader, " ")
	// RFC 7235 auth-scheme names are case-insensitive; native clients may send
	// "bearer".
	if len(bearerToken) != 2 || !strings.EqualFold(bearerToken[0], "Bearer") {
		logger.With(
			"middleware", "extractBearerToken",
		).Warn("Invalid authorization header format")
		return "", http.ErrNoCookie
	}

	return bearerToken[1], nil
}

func (s *Server) authenticateWithAPIKey(w http.ResponseWriter, r *http.Request, next http.Handler, token string) {
	logger := contextkeys.GetLoggerFromContext(r.Context())
	logger.With(
		"middleware", "authenticateWithAPIKey",
		"auth_type", "api_key",
	).Debug("Attempting API key authentication")

	apiKey, err := s.container.APIKeyService().ValidateAPIKey(r.Context(), token)
	if err != nil {
		logger.With(
			"middleware", "authenticateWithAPIKey",
			"auth_type", "api_key",
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to validate API key")
		apiErr := apierrors.NewAuthInvalidError("Invalid API key")
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	// Set user context from API key
	ctx := context.WithValue(r.Context(), contextkeys.APIKeyID, apiKey.ID)
	ctx = authenticatedContext(ctx, apiKey.UserID, "api_key", []any{"api_key_id", apiKey.ID})

	contextkeys.GetLoggerFromContext(ctx).Debug("API key authentication successful")
	next.ServeHTTP(w, r.WithContext(ctx))
}

// authenticateWithOAuthJWT verifies an AuthKit-issued bearer JWT against the
// API-surface verifier and sets the user context, mirroring the MCP endpoint's
// context setup (auth_type "oauth"). Authentication failures yield a 401;
// infrastructure failures during subject resolution yield a 500, never a 401.
func (s *Server) authenticateWithOAuthJWT(w http.ResponseWriter, r *http.Request, next http.Handler, token string) {
	logger := contextkeys.GetLoggerFromContext(r.Context())
	logger.With(
		"middleware", "authenticateWithOAuthJWT",
		"auth_type", "oauth",
	).Debug("Attempting OAuth bearer JWT authentication")

	info, err := s.apiTokenVerifier.Verify(r.Context(), token)
	if err != nil {
		if errors.Is(err, authkit.ErrUserResolution) {
			logger.With(
				"middleware", "authenticateWithOAuthJWT",
				"auth_type", "oauth",
				"error", fmt.Sprintf("%+v", err),
			).Error("OAuth JWT subject resolution failed (infrastructure error)")
			apiErr := apierrors.NewInternalError("")
			apierrors.WriteJSONError(w, r, apiErr)
			return
		}
		logger.With(
			"middleware", "authenticateWithOAuthJWT",
			"auth_type", "oauth",
			"error", fmt.Sprintf("%+v", err),
		).Warn("OAuth bearer JWT validation failed")
		apiErr := apierrors.NewAuthInvalidError("Invalid token")
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	ctx := authenticatedContext(r.Context(), info.UserID, "oauth", nil)

	contextkeys.GetLoggerFromContext(ctx).Debug("OAuth bearer JWT authentication successful")
	next.ServeHTTP(w, r.WithContext(ctx))
}

// authenticateWithSession reads the vx_session cookie, decrypts it, checks
// expiry (with automatic refresh when a refresh token is present), and sets
// the user context.
//
//nolint:funlen // Session auth must handle cookie read, expiry check, refresh, and context setup
func (s *Server) authenticateWithSession(w http.ResponseWriter, r *http.Request, next http.Handler) {
	logger := contextkeys.GetLoggerFromContext(r.Context())
	logger.With(
		"middleware", "authenticateWithSession",
		"auth_type", "cookie",
	).Debug("Attempting cookie session authentication")

	sess, err := s.sessionManager.Read(r)
	if err != nil {
		if errors.Is(err, sesslib.ErrNoCookie) {
			logger.With(
				"middleware", "authenticateWithSession",
			).Debug("No session cookie present")
		} else {
			logger.With(
				"middleware", "authenticateWithSession",
				"error", fmt.Sprintf("%+v", err),
			).Warn("Invalid session cookie")
		}
		apiErr := apierrors.NewAuthRequiredError("Authentication required")
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	// If the access token is expired, attempt a refresh — serialized per
	// user to avoid racing on the provider's single-use refresh tokens (H1).
	if sess.IsExpired() {
		if sess.RefreshToken == "" {
			logger.With(
				"middleware", "authenticateWithSession",
				"user_id", sess.UserID,
			).Info("Session expired and no refresh token available")
			apiErr := apierrors.NewAuthInvalidError("Session expired")
			apierrors.WriteJSONError(w, r, apiErr)
			return
		}

		mu := s.refreshLockFor(sess.UserID)
		mu.Lock()

		// Re-read the cookie inside the lock — another goroutine may have
		// already refreshed it, in which case our copy of the refresh token
		// is stale and a refresh attempt would 401.
		if reread, rerr := s.sessionManager.Read(r); rerr == nil && !reread.IsExpired() {
			sess = reread
			mu.Unlock()
		} else {
			refreshed, ok := s.refreshSession(w, r, sess, logger)
			mu.Unlock()
			if !ok {
				return
			}
			sess = refreshed
		}
	}

	// Set user context from session
	ctx := authenticatedContext(r.Context(), sess.UserID, "cookie", nil)

	contextkeys.GetLoggerFromContext(ctx).Debug("Cookie session authentication successful")
	next.ServeHTTP(w, r.WithContext(ctx))
}

// refreshSession runs the provider's refresh-token rotation and rewrites the
// vx_session cookie. Returns the refreshed session and true on success.
// On permanent failures (revoked token, invalid_grant) it writes 401 +
// clears the cookie. On transient failures (5xx, network) it writes 503
// and KEEPS the cookie so the client can retry.
//
// Caller must hold the per-user refresh lock for serialization.
func (s *Server) refreshSession(
	w http.ResponseWriter,
	r *http.Request,
	sess *sesslib.Session,
	logger *slog.Logger,
) (*sesslib.Session, bool) {
	logger.With(
		"middleware", "authenticateWithSession",
		"user_id", sess.UserID,
	).Debug("Access token expired, attempting refresh")

	newTokens, refreshErr := s.container.AuthService().RefreshTokens(r.Context(), sess.Provider, sess.RefreshToken)
	if refreshErr != nil {
		fields := []any{
			"middleware", "authenticateWithSession",
			"user_id", sess.UserID,
			"error", fmt.Sprintf("%+v", refreshErr),
		}
		if isTransientRefreshError(refreshErr) {
			logger.With(fields...).Warn("Transient refresh failure; keeping session, asking client to retry")
			w.Header().Set("Retry-After", "5")
			apiErr := apierrors.NewServiceUnavailableError("Authentication provider temporarily unavailable")
			apierrors.WriteJSONError(w, r, apiErr)
			return nil, false
		}
		logger.With(fields...).Warn("Permanent refresh failure; clearing session")
		s.sessionManager.Clear(w)
		apiErr := apierrors.NewAuthInvalidError("Session expired")
		apierrors.WriteJSONError(w, r, apiErr)
		return nil, false
	}

	if newTokens.RefreshToken == "" {
		// A rotating provider returns a new refresh token on rotation. Empty
		// here means upstream behaviour changed; the cached refresh token
		// has been invalidated and the next refresh will fail.
		logger.With(
			"middleware", "authenticateWithSession",
			"user_id", sess.UserID,
		).Warn("Refresh response missing new refresh_token; subsequent refreshes may fail")
	} else {
		sess.RefreshToken = newTokens.RefreshToken
	}
	sess.AccessToken = newTokens.AccessToken
	sess.ExpiresAt = newTokens.ExpiresAt

	if writeErr := s.sessionManager.Write(w, sess); writeErr != nil {
		logger.With("error", writeErr).Warn("Failed to rewrite refreshed session cookie")
		// Continue anyway — the user is authenticated with the new tokens
	}

	return sess, true
}

// Optional auth middleware that allows both authenticated and unauthenticated access.
// If a valid session cookie or API key is provided, it sets the user context;
// otherwise allows the request to proceed unauthenticated.
//
//nolint:gocognit // Optional auth middleware handles multiple auth paths by design
func (s *Server) optionalAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try API key from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			token, err := s.extractBearerToken(r)
			if err == nil && isAPIKey(token) {
				apiKey, apiKeyErr := s.container.APIKeyService().ValidateAPIKey(r.Context(), token)
				if apiKeyErr == nil {
					ctx := context.WithValue(r.Context(), contextkeys.APIKeyID, apiKey.ID)
					ctx = authenticatedContext(ctx, apiKey.UserID, "api_key",
						[]any{"api_key_id", apiKey.ID})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			// Non-API-key bearer tokens may be AuthKit OAuth JWTs when the API
			// verifier is configured; on any failure fall through unauthenticated.
			if err == nil && !isAPIKey(token) && s.apiTokenVerifier != nil {
				if ctx, ok := s.optionalOAuthJWTContext(r, token); ok {
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			// Invalid or unrecognized bearer token — fall through unauthenticated
			next.ServeHTTP(w, r)
			return
		}

		// Try cookie session
		if s.sessionManager != nil {
			sess, err := s.sessionManager.Read(r)
			if err == nil && !sess.IsExpired() {
				ctx := authenticatedContext(r.Context(), sess.UserID, "cookie", nil)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// No auth provided, allow request to proceed without user context
		next.ServeHTTP(w, r)
	})
}

// optionalOAuthJWTContext verifies an AuthKit bearer JWT for the optional-auth
// path, returning the authenticated context on success. Failures report ok =
// false so the request proceeds anonymous (by design), but an infrastructure
// failure would silently de-authenticate users — that case is logged at Error
// to stay visible to operators.
func (s *Server) optionalOAuthJWTContext(r *http.Request, token string) (context.Context, bool) {
	info, err := s.apiTokenVerifier.Verify(r.Context(), token)
	if err == nil {
		return authenticatedContext(r.Context(), info.UserID, "oauth", nil), true
	}

	logEntry := contextkeys.GetLoggerFromContext(r.Context()).With(
		"middleware", "optionalAuthMiddleware",
		"auth_type", "oauth",
		"error", fmt.Sprintf("%+v", err),
	)
	if errors.Is(err, authkit.ErrUserResolution) {
		logEntry.Error("OAuth JWT subject resolution failed (infrastructure error); proceeding unauthenticated")
	} else {
		logEntry.Debug("OAuth bearer JWT validation failed; proceeding unauthenticated")
	}
	return nil, false
}

// getUserIDFromContext extracts the user ID from the request context, returning
// an empty string when no authenticated user is present.
func (s *Server) getUserIDFromContext(r *http.Request) string {
	userID := r.Context().Value(contextKeyUserID)
	if userID == nil {
		return ""
	}
	return userID.(string)
}

// securityHeadersMiddleware adds baseline security response headers. The API serves
// JSON, so these are conservative and safe: nosniff prevents MIME-type confusion,
// DENY framing blocks clickjacking, the referrer policy limits leakage, and HSTS
// instructs browsers to stick to HTTPS. TLS is terminated at the Cloud Run proxy,
// so HSTS here is advisory for any browser-originated requests.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

// Backoffice auth middleware that validates the back office admin API key
// All /bo/* endpoints must use this middleware for authentication
func (s *Server) backofficeAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := contextkeys.GetLoggerFromContext(r.Context())
		logger.With(
			"middleware", "backofficeAuthMiddleware",
		).Debug("Processing backoffice authentication")

		token, err := s.extractBearerToken(r)
		if err != nil {
			logger.With(
				"middleware", "backofficeAuthMiddleware",
			).Warn("Authorization header missing")
			apiErr := apierrors.NewAuthRequiredError("Authorization header is required")
			apierrors.WriteJSONError(w, r, apiErr)
			return
		}

		// Validate the token against the back office admin API key
		if s.config.Security.BackofficeAdminAPIKey == "" {
			logger.With(
				"middleware", "backofficeAuthMiddleware",
			).Error("Back office admin API key not configured")
			apiErr := apierrors.NewAPIError(
				"CONFIGURATION_ERROR",
				"Configuration Error",
				"Back office admin API key not configured",
				http.StatusInternalServerError,
			)
			apierrors.WriteJSONError(w, r, apiErr)
			return
		}

		if subtle.ConstantTimeCompare([]byte(token), []byte(s.config.Security.BackofficeAdminAPIKey)) != 1 {
			logger.With(
				"middleware", "backofficeAuthMiddleware",
			).Warn("Invalid back office admin API key")
			apiErr := apierrors.NewAuthInvalidError("Invalid back office admin API key")
			apierrors.WriteJSONError(w, r, apiErr)
			return
		}

		logger.With(
			"middleware", "backofficeAuthMiddleware",
		).Debug("Back office authentication successful")
		next.ServeHTTP(w, r)
	})
}

// instanceAdminMiddleware guards the /api/v1/admin surface. It runs after an
// auth middleware that only OPTIONALLY populates the user (optionalAuthMiddleware),
// resolves that user, and requires config.IsInstanceAdmin(user.Email). Any
// failure — no authenticated user, a lookup error, or a non-admin — returns 404
// (Not Found), not 401/403, so the admin surface is not advertised to non-admins
// (mirrors the dev-login non-advertisement pattern). It deliberately stays
// OUTSIDE internal/authz, which is team-scoped by that package's contract.
func (s *Server) instanceAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		notFound := func() {
			apierrors.WriteJSONError(w, r, apierrors.NewResourceNotFoundError("endpoint", "Endpoint not found"))
		}

		userID := s.getUserIDFromContext(r)
		if userID == "" {
			notFound()
			return
		}

		user, err := s.container.AuthService().GetUserByID(r.Context(), userID)
		if err != nil || user == nil {
			notFound()
			return
		}

		if !s.config.IsInstanceAdmin(user.Email) {
			notFound()
			return
		}

		next.ServeHTTP(w, r)
	})
}
