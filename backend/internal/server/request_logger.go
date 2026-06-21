package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/utils"
)

// RequestIDMiddleware creates a middleware that generates request IDs and extracts Cloud Trace context
func RequestIDMiddleware(logger *logrus.Logger, cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Generate request ID
			requestID := generateRequestID(r, cfg)

			// Extract Cloud Trace context if available
			traceID := extractCloudTraceID(r, cfg)

			// Add request ID to context
			ctx := context.WithValue(r.Context(), contextkeys.RequestID, requestID)
			ctx = context.WithValue(ctx, contextkeys.TraceID, traceID)

			// Create request-scoped logger with request context
			reqLogger := logger.WithFields(logrus.Fields{
				"request_id": requestID,
				"path":       r.URL.Path,
				"method":     r.Method,
			})

			// Add trace ID if available
			if traceID != "" {
				reqLogger = reqLogger.WithField("logging.googleapis.com/trace", traceID)
			}

			// Store logger in context for use by handlers
			ctx = context.WithValue(ctx, contextkeys.Logger, reqLogger)

			// Log incoming request
			reqLogger.WithFields(logrus.Fields{
				"remote_addr": r.RemoteAddr,
				"user_agent":  r.Header.Get("User-Agent"),
			}).Debug("Incoming request")

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// generateRequestID creates a unique request ID
// Format: service-revision-uuid for Cloud Run, or uuid for other environments
func generateRequestID(r *http.Request, cfg *config.Config) string {
	// Check if request already has a request ID header (for request forwarding)
	if existingID := r.Header.Get("X-Request-ID"); existingID != "" {
		return existingID
	}

	// Get Cloud Run metadata from config
	service := cfg.KService
	revision := cfg.KRevision

	// Generate UUID
	id := uuid.New().String()[:8] // Use first 8 characters for brevity

	// Format: service-revision-uuid (e.g., vibexp-api-00042-abc123de)
	if service != "" && revision != "" {
		return fmt.Sprintf("%s-%s-%s", service, revision, id)
	}

	// Fallback: service-uuid
	if service != "" {
		return fmt.Sprintf("%s-%s", service, id)
	}

	// Fallback: just uuid
	return id
}

// isValidTraceID validates that a string is a 32-character hexadecimal trace ID
func isValidTraceID(traceID string) bool {
	if len(traceID) != 32 {
		return false
	}
	for _, c := range traceID {
		if !utils.IsHexChar(c) {
			return false
		}
	}
	return true
}

// extractCloudTraceID extracts the trace ID from Google Cloud Trace context header
// Format: X-Cloud-Trace-Context: TRACE_ID/SPAN_ID;o=TRACE_TRUE
// Returns: projects/PROJECT_ID/traces/TRACE_ID for Cloud Logging correlation
func extractCloudTraceID(r *http.Request, cfg *config.Config) string {
	traceHeader := r.Header.Get("X-Cloud-Trace-Context")
	if traceHeader == "" {
		return ""
	}

	// Parse trace header: TRACE_ID/SPAN_ID;o=TRACE_TRUE
	parts := strings.Split(traceHeader, "/")
	if len(parts) == 0 {
		return ""
	}

	traceID := parts[0]
	if !isValidTraceID(traceID) {
		return ""
	}

	// Get project ID from config. Prefer the GCP-canonical env vars when
	// they're set; fall back to GCPProjectID (GCP_PROJECT_ID, empty by
	// default) so the trace field serializes as
	// projects/<id>/traces/<traceid> for Cloud Logging correlation.
	// In multi-project setups, ensure GoogleCloudProject or GCPProject is
	// explicitly set to avoid stamping traces with the wrong project ID.
	projectID := cfg.GoogleCloudProject
	if projectID == "" {
		projectID = cfg.GCPProject
	}
	if projectID == "" {
		projectID = cfg.GCPProjectID
	}

	// Return full trace path for Cloud Logging correlation
	if projectID != "" {
		return fmt.Sprintf("projects/%s/traces/%s", projectID, traceID)
	}

	// Fallback: just the trace ID
	return traceID
}

// structuredRequestLogger is a logrus-based HTTP access logger that replaces chi's built-in
// middleware.Logger. It writes a single structured JSON entry per request (severity INFO/WARN/ERROR
// based on status code) to stderr via the Cloud-Logging-aware context logger so that every entry
// carries the trace-correlation field injected by RequestIDMiddleware.
//
// Required import: "github.com/go-chi/chi/v5/middleware" for NewWrapResponseWriter.
// This middleware MUST be placed after RequestIDMiddleware so the context logger is available.
func structuredRequestLogger(logger *logrus.Logger) func(http.Handler) http.Handler { //nolint:unparam
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

			entry := contextkeys.GetLoggerFromContext(r.Context()).WithFields(logrus.Fields{
				"method":     r.Method,
				"path":       r.URL.Path,
				"status":     status,
				"latency_ms": latencyMs,
				"bytes":      ww.BytesWritten(),
			})

			switch {
			case status >= 500:
				entry.Error("request completed")
			case status >= 400:
				entry.Warn("request completed")
			default:
				entry.Info("request completed")
			}
		})
	}
}
