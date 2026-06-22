// Package contextkeys provides shared context key constants used across
// the backend for storing and retrieving request-scoped values.
// Using a shared package prevents import cycles and ensures type safety.
package contextkeys

import (
	"context"
	"log/slog"
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

	// AccessedResourceID holds the resolved UUID of a resource read on the
	// current request, set by a detail handler and read back by the
	// resource-access recording middleware. See accessedResourceIDHolder.
	AccessedResourceID ContextKey = "accessed_resource_id"

	// PubSub middleware context keys
	PubSubServiceAccount ContextKey = "pubsub_service_account"
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
