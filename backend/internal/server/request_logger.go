package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/contextkeys"
)

// requestCompletedMsg is the access-log message emitted once per request at a
// level derived from the response status.
const requestCompletedMsg = "request completed"

// RequestIDMiddleware creates a middleware that assigns each request a stable
// request ID — honoring an inbound X-Request-ID header when present (so IDs
// survive request forwarding), otherwise generating a UUID — and stores a
// request-scoped logger carrying that ID in the context for downstream handlers.
func RequestIDMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := generateRequestID(r)

			// Add request ID to context
			ctx := context.WithValue(r.Context(), contextkeys.RequestID, requestID)

			// Create request-scoped logger with request context
			reqLogger := logger.With(
				"request_id", requestID,
				"path", r.URL.Path,
				"method", r.Method,
			)

			// Store logger in context for use by handlers
			ctx = context.WithValue(ctx, contextkeys.Logger, reqLogger)

			// Log incoming request
			reqLogger.Debug("Incoming request",
				"remote_addr", clientIP(r),
				"user_agent", r.Header.Get("User-Agent"),
			)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// generateRequestID returns a request ID for r: an inbound X-Request-ID header
// when present (for request forwarding), otherwise a newly generated UUID.
func generateRequestID(r *http.Request) string {
	if existingID := r.Header.Get("X-Request-ID"); existingID != "" {
		return existingID
	}
	return uuid.New().String()
}

// structuredRequestLogger is an slog-based HTTP access logger that replaces chi's
// built-in middleware.Logger. It writes a single structured entry per request
// (level INFO/WARN/ERROR based on status code) via the request-scoped context
// logger, so every entry carries the request_id injected by RequestIDMiddleware.
//
// Required import: "github.com/go-chi/chi/v5/middleware" for NewWrapResponseWriter.
// This middleware MUST be placed after RequestIDMiddleware so the context logger is available.
func structuredRequestLogger(logger *slog.Logger) func(http.Handler) http.Handler { //nolint:unparam
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap the ResponseWriter so we can capture the status code and bytes written.
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				// Default status when WriteHeader was never called explicitly.
				status = http.StatusOK
			}

			latencyMs := time.Since(start).Milliseconds()

			entry := contextkeys.GetLoggerFromContext(r.Context()).With(
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"latency_ms", latencyMs,
				"bytes", ww.BytesWritten(),
			)

			switch {
			case status >= 500:
				entry.Error(requestCompletedMsg)
			case status >= 400:
				entry.Warn(requestCompletedMsg)
			default:
				entry.Info(requestCompletedMsg)
			}
		})
	}
}
