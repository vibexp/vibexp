package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/contextkeys"
)

func TestGenerateRequestID(t *testing.T) {
	t.Run("with Cloud Run metadata", func(t *testing.T) {
		cfg := &config.Config{KService: "vibexp-api", KRevision: "00042"}
		req := httptest.NewRequest("GET", "/test", nil)
		requestID := generateRequestID(req, cfg)
		require.NotEmpty(t, requestID)
		assert.Contains(t, requestID, "vibexp-api-00042-")
		assert.NotContains(t, requestID, "localhost")
	})

	t.Run("with service only", func(t *testing.T) {
		cfg := &config.Config{KService: "vibexp-api"}
		req := httptest.NewRequest("GET", "/test", nil)
		requestID := generateRequestID(req, cfg)
		require.NotEmpty(t, requestID)
		assert.Contains(t, requestID, "vibexp-api-")
		assert.NotContains(t, requestID, "localhost")
	})

	t.Run("without Cloud Run metadata", func(t *testing.T) {
		cfg := &config.Config{}
		req := httptest.NewRequest("GET", "/test", nil)
		requestID := generateRequestID(req, cfg)
		require.NotEmpty(t, requestID)
		assert.NotContains(t, requestID, "localhost")
	})

	t.Run("with existing request ID header", func(t *testing.T) {
		cfg := &config.Config{}
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Request-ID", "existing-request-id-123")
		requestID := generateRequestID(req, cfg)
		assert.Equal(t, "existing-request-id-123", requestID)
	})
}

func TestExtractCloudTraceID(t *testing.T) {
	tests := []struct {
		name           string
		traceHeader    string
		googleProject  string
		gcpProject     string
		expectedResult string
	}{
		{
			name:           "valid trace header with GOOGLE_CLOUD_PROJECT",
			traceHeader:    "105445aa7843bc8bf206b12000100000/1;o=1",
			googleProject:  "test-project-123",
			expectedResult: "projects/test-project-123/traces/105445aa7843bc8bf206b12000100000",
		},
		{
			name:           "valid trace header with GCP_PROJECT",
			traceHeader:    "105445aa7843bc8bf206b12000100000/1;o=1",
			gcpProject:     "test-project-456",
			expectedResult: "projects/test-project-456/traces/105445aa7843bc8bf206b12000100000",
		},
		{
			name:           "valid trace header without project ID",
			traceHeader:    "105445aa7843bc8bf206b12000100000/1;o=1",
			expectedResult: "105445aa7843bc8bf206b12000100000",
		},
		{
			name:           "no trace header",
			traceHeader:    "",
			expectedResult: "",
		},
		{
			name:           "malformed trace header (invalid hex)",
			traceHeader:    "invalid",
			expectedResult: "", // Should be rejected due to validation
		},
		{
			name:           "trace ID too short",
			traceHeader:    "abc123/1",
			expectedResult: "", // Should be rejected (not 32 chars)
		},
		{
			name:           "trace ID with non-hex characters",
			traceHeader:    "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz/1",
			expectedResult: "", // Should be rejected (invalid hex - 32 z's)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create config with test values
			cfg := &config.Config{
				GoogleCloudProject: tt.googleProject,
				GCPProject:         tt.gcpProject,
			}

			// Create request with trace header
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.traceHeader != "" {
				req.Header.Set("X-Cloud-Trace-Context", tt.traceHeader)
			}

			// Extract trace ID
			traceID := extractCloudTraceID(req, cfg)

			// Assertions
			assert.Equal(t, tt.expectedResult, traceID)
		})
	}
}

// TestExtractCloudTraceID_GCPProjectIDFallback covers the third-fallback path
// where neither GoogleCloudProject nor GCPProject is set: GCPProjectID (which
// has a non-empty default in config) must be consulted so the trace field
// always serializes as projects/<id>/traces/<traceid>.
func TestExtractCloudTraceID_GCPProjectIDFallback(t *testing.T) {
	const traceHeader = "105445aa7843bc8bf206b12000100000/1;o=1"
	const traceID = "105445aa7843bc8bf206b12000100000"

	tests := []struct {
		name         string
		cfg          *config.Config
		expectedPath string
	}{
		{
			name:         "GCP_PROJECT preferred over GCPProjectID fallback",
			cfg:          &config.Config{GCPProject: "preferred-project", GCPProjectID: "fallback-project"},
			expectedPath: "projects/preferred-project/traces/" + traceID,
		},
		{
			name:         "falls back to GCPProjectID when canonical vars empty",
			cfg:          &config.Config{GCPProjectID: "fallback-project"},
			expectedPath: "projects/fallback-project/traces/" + traceID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-Cloud-Trace-Context", traceHeader)
			assert.Equal(t, tt.expectedPath, extractCloudTraceID(req, tt.cfg))
		})
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	// Create logger
	logger := logrus.New()
	logger.SetOutput(nil) // Disable output for tests

	// Create config
	cfg := &config.Config{
		KService:           "test-service",
		KRevision:          "test-revision",
		GoogleCloudProject: "test-project",
	}

	// Create middleware
	middleware := RequestIDMiddleware(logger, cfg)

	// Create test handler
	handlerCalled := false
	var capturedContext context.Context
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		capturedContext = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	// Wrap handler with middleware
	wrappedHandler := middleware(testHandler)

	// Create request
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Cloud-Trace-Context", "105445aa7843bc8bf206b12000100000/1;o=1")
	rec := httptest.NewRecorder()

	// Execute middleware
	wrappedHandler.ServeHTTP(rec, req)

	// Assertions
	assert.True(t, handlerCalled, "Handler should be called")
	assert.NotNil(t, capturedContext, "Context should be captured")

	// Check request ID in context
	requestID := contextkeys.GetRequestID(capturedContext)
	assert.NotEmpty(t, requestID, "Request ID should be set in context")
	assert.Contains(t, requestID, "test-service", "Request ID should contain service name")

	// Check trace ID in context
	traceID := contextkeys.GetTraceID(capturedContext)
	assert.NotEmpty(t, traceID, "Trace ID should be set in context")
	assert.Contains(t, traceID, "test-project", "Trace ID should contain project ID")

	// Check logger in context
	contextLogger := contextkeys.GetLoggerFromContext(capturedContext)
	assert.NotNil(t, contextLogger, "Logger should be set in context")
}

func TestGetLoggerFromContext(t *testing.T) {
	t.Run("with logger in context", func(t *testing.T) {
		expectedLogger := logrus.WithField("test", "value")
		ctx := context.WithValue(context.Background(), contextkeys.Logger, expectedLogger)

		logger := contextkeys.GetLoggerFromContext(ctx)
		assert.Equal(t, expectedLogger, logger)
	})

	t.Run("without logger in context", func(t *testing.T) {
		ctx := context.Background()
		logger := contextkeys.GetLoggerFromContext(ctx)
		assert.NotNil(t, logger)
	})

	t.Run("with request ID in context but no logger", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), contextkeys.RequestID, "test-request-id")
		logger := contextkeys.GetLoggerFromContext(ctx)
		assert.NotNil(t, logger)
		// Logger should have request_id in fields
		assert.Contains(t, logger.Data, "request_id")
		assert.Equal(t, "test-request-id", logger.Data["request_id"])
	})
}

func TestGetRequestID(t *testing.T) {
	t.Run("with request ID in context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), contextkeys.RequestID, "test-123")
		requestID := contextkeys.GetRequestID(ctx)
		assert.Equal(t, "test-123", requestID)
	})

	t.Run("without request ID in context", func(t *testing.T) {
		ctx := context.Background()
		requestID := contextkeys.GetRequestID(ctx)
		assert.Empty(t, requestID)
	})
}

func TestGetTraceID(t *testing.T) {
	t.Run("with trace ID in context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), contextkeys.TraceID, "projects/test/traces/123")
		traceID := contextkeys.GetTraceID(ctx)
		assert.Equal(t, "projects/test/traces/123", traceID)
	})

	t.Run("without trace ID in context", func(t *testing.T) {
		ctx := context.Background()
		traceID := contextkeys.GetTraceID(ctx)
		assert.Empty(t, traceID)
	})
}

func TestAddLogFields(t *testing.T) {
	// Create base logger
	baseLogger := logrus.WithField("base", "field")
	ctx := context.WithValue(context.Background(), contextkeys.Logger, baseLogger)

	// Add additional fields
	newLogger := contextkeys.AddLogFields(ctx, logrus.Fields{
		"additional": "value",
		"another":    "field",
	})

	// Verify new fields are added
	assert.NotNil(t, newLogger)
	assert.Contains(t, newLogger.Data, "base")
	assert.Contains(t, newLogger.Data, "additional")
	assert.Contains(t, newLogger.Data, "another")
}

// injectContextLogger returns a copy of req with the provided logrus logger injected as context entry.
func injectContextLogger(req *http.Request, l *logrus.Logger) *http.Request {
	entry := logrus.NewEntry(l)
	ctx := context.WithValue(req.Context(), contextkeys.Logger, entry)
	return req.WithContext(ctx)
}

func TestStructuredRequestLogger_200(t *testing.T) {
	nullLogger, hook := test.NewNullLogger()
	nullLogger.SetLevel(logrus.DebugLevel)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/prompts", nil)
	req = injectContextLogger(req, nullLogger)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := structuredRequestLogger(nullLogger)
	rr := httptest.NewRecorder()
	mw(handler).ServeHTTP(rr, req)

	require.Len(t, hook.Entries, 1)
	entry := hook.Entries[0]
	assert.Equal(t, logrus.InfoLevel, entry.Level)
	assert.Equal(t, "request completed", entry.Message)
	assert.Equal(t, http.StatusOK, entry.Data["status"])
}

func TestStructuredRequestLogger_400(t *testing.T) {
	nullLogger, hook := test.NewNullLogger()
	nullLogger.SetLevel(logrus.DebugLevel)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/prompts", nil)
	req = injectContextLogger(req, nullLogger)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	mw := structuredRequestLogger(nullLogger)
	rr := httptest.NewRecorder()
	mw(handler).ServeHTTP(rr, req)

	require.Len(t, hook.Entries, 1)
	entry := hook.Entries[0]
	assert.Equal(t, logrus.WarnLevel, entry.Level)
	assert.Equal(t, http.StatusBadRequest, entry.Data["status"])
}

func TestStructuredRequestLogger_500(t *testing.T) {
	nullLogger, hook := test.NewNullLogger()
	nullLogger.SetLevel(logrus.DebugLevel)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/crash", nil)
	req = injectContextLogger(req, nullLogger)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	mw := structuredRequestLogger(nullLogger)
	rr := httptest.NewRecorder()
	mw(handler).ServeHTTP(rr, req)

	require.Len(t, hook.Entries, 1)
	entry := hook.Entries[0]
	assert.Equal(t, logrus.ErrorLevel, entry.Level)
	assert.Equal(t, http.StatusInternalServerError, entry.Data["status"])
}

func TestStructuredRequestLogger_FieldsPresent(t *testing.T) {
	nullLogger, hook := test.NewNullLogger()
	nullLogger.SetLevel(logrus.DebugLevel)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/resources/123", nil)
	req = injectContextLogger(req, nullLogger)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mw := structuredRequestLogger(nullLogger)
	rr := httptest.NewRecorder()
	mw(handler).ServeHTTP(rr, req)

	require.Len(t, hook.Entries, 1)
	entry := hook.Entries[0]

	// Verify all required structured fields are present.
	assert.Contains(t, entry.Data, "method")
	assert.Contains(t, entry.Data, "path")
	assert.Contains(t, entry.Data, "status")
	assert.Contains(t, entry.Data, "latency_ms")
	assert.Contains(t, entry.Data, "bytes")

	assert.Equal(t, http.MethodDelete, entry.Data["method"])
	assert.Equal(t, "/api/v1/resources/123", entry.Data["path"])
	assert.Equal(t, http.StatusNoContent, entry.Data["status"])
}
