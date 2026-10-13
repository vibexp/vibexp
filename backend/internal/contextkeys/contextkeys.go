// Package contextkeys provides shared context key constants used across
// the backend for storing and retrieving request-scoped values.
// Using a shared package prevents import cycles and ensures type safety.
package contextkeys

import (
	"context"
	"log/slog"
	"time"
)

// ContextKey is a custom type for context keys to avoid collisions
type ContextKey string

// Context keys for request-scoped values
// These are shared across packages to ensure consistent context access
const (
	// Logging and tracing context keys
	RequestID ContextKey = "request_id"
	Logger    ContextKey = "logger"

	// User authentication and authorization context keys
	User        ContextKey = "user"
	UserID      ContextKey = "userID"
	UserEmail   ContextKey = "userEmail"
	AgentID     ContextKey = "agentID"
	ExecutionID ContextKey = "executionID"

	// Authentication metadata context keys
	AuthType ContextKey = "auth_type"
	APIKeyID ContextKey = "api_key_id"

	// SetupSession marks a request admitted on a first-run setup session
	// (#1236) rather than on a user's credentials. Such a request has no user:
	// UserID is empty.
	SetupSession ContextKey = "setup_session"
	// SessionProvider holds the slug of the identity provider that issued the
	// caller's cookie session (#1238). It is set only on the instance
	// authentication settings routes, for the lockout guard, and is absent for
	// API keys, setup sessions and dev-login sessions.
	SessionProvider ContextKey = "session_provider"
	// AuthMethod holds how the caller's cookie session was signed in: one of
	// the session.AuthMethod* values (#1428). It is set for cookie
	// authentication only, and is empty for an API key, a bearer token and a
	// cookie minted before sign-in methods were recorded. Read it with
	// GetAuthMethod.
	AuthMethod ContextKey = "auth_method"
	// AuthenticatedAt holds when the caller's cookie session was signed in
	// (#1428), under the same conditions as AuthMethod. Read it with
	// GetAuthenticatedAt.
	AuthenticatedAt ContextKey = "authenticated_at"

	// AccessedResourceID holds the resolved UUID of a resource read on the
	// current request, set by a detail handler and read back by the
	// resource-access recording middleware. See accessedResourceIDHolder.
	AccessedResourceID ContextKey = "accessed_resource_id"

	// PubSub middleware context keys
	PubSubServiceAccount ContextKey = "pubsub_service_account"

	// ClientIP holds the client IP resolved by clientIPMiddleware, honouring
	// X-Forwarded-For / X-Real-IP only when the peer is a configured trusted
	// proxy (#465). It is the single source for the rate limiter, request logs,
	// resource-access events, and activity records — downstream code must read
	// this rather than parsing the headers again.
	ClientIP ContextKey = "client_ip"
)

// accessedResourceIDHolder is a mutable container for the resolved resource UUID.
//
// A plain context value set by a handler via r.WithContext is invisible to an
// outer middleware after next.ServeHTTP returns, because the handler mutates its
// own copy of the request. Instead the middleware injects this pointer holder
// BEFORE calling the handler, the handler fills in the id, and the middleware
// reads it afterwards. Each request is served on a single goroutine, so no
// synchronization is needed.
type accessedResourceIDHolder struct {
	id string
}

// ContextWithAccessedResourceID returns a context carrying an empty resource-id
// holder. The recording middleware calls this before invoking the next handler.
func ContextWithAccessedResourceID(ctx context.Context) context.Context {
	return context.WithValue(ctx, AccessedResourceID, &accessedResourceIDHolder{})
}

// SetAccessedResourceID records the resolved resource UUID on the holder placed
// in ctx by ContextWithAccessedResourceID. It is a no-op when no holder is
// present, so handlers stay safe on routes without the recording middleware.
func SetAccessedResourceID(ctx context.Context, id string) {
	if holder, ok := ctx.Value(AccessedResourceID).(*accessedResourceIDHolder); ok {
		holder.id = id
	}
}

// GetAccessedResourceID returns the resolved resource UUID and whether one was
// set. The recording middleware calls this after the handler returns.
func GetAccessedResourceID(ctx context.Context) (string, bool) {
	holder, ok := ctx.Value(AccessedResourceID).(*accessedResourceIDHolder)
	if !ok || holder.id == "" {
		return "", false
	}
	return holder.id, true
}

// GetLoggerFromContext retrieves the request-scoped logger from context.
// If no logger is found, returns the default logger enriched with the
// request_id from the context when available.
func GetLoggerFromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(Logger).(*slog.Logger); ok {
		return logger
	}

	// Fallback: derive from the default logger with whatever context we have.
	logger := slog.Default()
	if requestID, ok := ctx.Value(RequestID).(string); ok && requestID != "" {
		logger = logger.With("request_id", requestID)
	}
	return logger
}

// GetAuthMethod returns how the caller's cookie session was signed in, or ""
// when the request was not authenticated by a cookie or the cookie predates
// recorded sign-in methods.
func GetAuthMethod(ctx context.Context) string {
	method, _ := ctx.Value(AuthMethod).(string)
	return method
}

// GetAuthenticatedAt returns when the caller's cookie session was signed in.
// It is the zero time when the request was not authenticated by a cookie or the
// cookie predates recorded sign-in times, so a "signed in recently" check on
// the result fails closed.
func GetAuthenticatedAt(ctx context.Context) time.Time {
	at, _ := ctx.Value(AuthenticatedAt).(time.Time)
	return at
}

// GetRequestID retrieves the request ID from context.
// Returns empty string if not found.
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestID).(string); ok {
		return requestID
	}
	return ""
}

// AddLogFields returns the request-scoped logger augmented with additional
// attributes, supplied as alternating key/value pairs (slog style).
func AddLogFields(ctx context.Context, args ...any) *slog.Logger {
	return GetLoggerFromContext(ctx).With(args...)
}
