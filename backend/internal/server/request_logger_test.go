package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/logging/logtest"
)

func TestGenerateRequestID(t *testing.T) {
	t.Run("generates a non-empty ID without an inbound header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		assert.NotEmpty(t, generateRequestID(req))
	})

	t.Run("honors an existing X-Request-ID header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Request-ID", "existing-request-id-123")
		assert.Equal(t, "existing-request-id-123", generateRequestID(req))
	})

	t.Run("generated IDs are unique per request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		assert.NotEqual(t, generateRequestID(req), generateRequestID(req))
	})
}

func TestRequestIDMiddleware(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	middleware := RequestIDMiddleware(logger)

	handlerCalled := false
	var capturedContext context.Context
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		capturedContext = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware(testHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "inbound-req-id")
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	assert.True(t, handlerCalled, "Handler should be called")
	require.NotNil(t, capturedContext, "Context should be captured")

	// The inbound X-Request-ID is honored and stored in the context.
	requestID := contextkeys.GetRequestID(capturedContext)
	assert.Equal(t, "inbound-req-id", requestID, "inbound X-Request-ID should be honored")

	// A request-scoped logger is available to downstream handlers.
	contextLogger := contextkeys.GetLoggerFromContext(capturedContext)
	assert.NotNil(t, contextLogger, "Logger should be set in context")
}

func TestGetLoggerFromContext(t *testing.T) {
	t.Run("with logger in context", func(t *testing.T) {
		expectedLogger := slog.New(slog.DiscardHandler).With("test", "value")
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
		// The fallback path derives from slog.Default(); swap in a recording
		// default logger so we can assert the request_id attribute it injects.
		recordLogger, hook := logtest.New()
		prev := slog.Default()
		slog.SetDefault(recordLogger)
		defer slog.SetDefault(prev)

		ctx := context.WithValue(context.Background(), contextkeys.RequestID, "test-request-id")
		logger := contextkeys.GetLoggerFromContext(ctx)
		assert.NotNil(t, logger)

		// Logger should emit request_id in its fields.
		logger.Info("probe")
		entry := hook.LastEntry()
		require.NotNil(t, entry)
		assert.Contains(t, entry.Data, "request_id")
		assert.Equal(t, "test-request-id", entry.Data["request_id"])
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
	// Create base logger that records what it emits.
	recordLogger, hook := logtest.New()
	baseLogger := recordLogger.With("base", "field")
	ctx := context.WithValue(context.Background(), contextkeys.Logger, baseLogger)

	// Add additional fields
	newLogger := contextkeys.AddLogFields(ctx,
		"additional", "value",
		"another", "field",
	)

	// Verify new fields are added
	require.NotNil(t, newLogger)
	newLogger.Info("probe")
	entry := hook.LastEntry()
	require.NotNil(t, entry)
	assert.Contains(t, entry.Data, "base")
	assert.Contains(t, entry.Data, "additional")
	assert.Contains(t, entry.Data, "another")
}

// injectContextLogger returns a copy of req with the provided slog logger injected as context entry.
func injectContextLogger(req *http.Request, l *slog.Logger) *http.Request {
	ctx := context.WithValue(req.Context(), contextkeys.Logger, l)
	return req.WithContext(ctx)
}

func TestStructuredRequestLogger_200(t *testing.T) {
	nullLogger, hook := logtest.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/prompts", nil)
	req = injectContextLogger(req, nullLogger)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := structuredRequestLogger(nullLogger)
	rr := httptest.NewRecorder()
	mw(handler).ServeHTTP(rr, req)

	require.Len(t, hook.AllEntries(), 1)
	entry := hook.AllEntries()[0]
	assert.Equal(t, slog.LevelInfo, entry.Level)
	assert.Equal(t, "request completed", entry.Message)
	assert.EqualValues(t, http.StatusOK, entry.Data["status"])
}

func TestStructuredRequestLogger_400(t *testing.T) {
	nullLogger, hook := logtest.New()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/prompts", nil)
	req = injectContextLogger(req, nullLogger)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	mw := structuredRequestLogger(nullLogger)
	rr := httptest.NewRecorder()
	mw(handler).ServeHTTP(rr, req)

	require.Len(t, hook.AllEntries(), 1)
	entry := hook.AllEntries()[0]
	assert.Equal(t, slog.LevelWarn, entry.Level)
	assert.EqualValues(t, http.StatusBadRequest, entry.Data["status"])
}

func TestStructuredRequestLogger_500(t *testing.T) {
	nullLogger, hook := logtest.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/crash", nil)
	req = injectContextLogger(req, nullLogger)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	mw := structuredRequestLogger(nullLogger)
	rr := httptest.NewRecorder()
	mw(handler).ServeHTTP(rr, req)

	require.Len(t, hook.AllEntries(), 1)
	entry := hook.AllEntries()[0]
	assert.Equal(t, slog.LevelError, entry.Level)
	assert.EqualValues(t, http.StatusInternalServerError, entry.Data["status"])
}

func TestStructuredRequestLogger_FieldsPresent(t *testing.T) {
	nullLogger, hook := logtest.New()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/resources/123", nil)
	req = injectContextLogger(req, nullLogger)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mw := structuredRequestLogger(nullLogger)
	rr := httptest.NewRecorder()
	mw(handler).ServeHTTP(rr, req)

	require.Len(t, hook.AllEntries(), 1)
	entry := hook.AllEntries()[0]

	// Verify all required structured fields are present.
	assert.Contains(t, entry.Data, "method")
	assert.Contains(t, entry.Data, "path")
	assert.Contains(t, entry.Data, "status")
	assert.Contains(t, entry.Data, "latency_ms")
	assert.Contains(t, entry.Data, "bytes")

	assert.Equal(t, http.MethodDelete, entry.Data["method"])
	assert.Equal(t, "/api/v1/resources/123", entry.Data["path"])
	assert.EqualValues(t, http.StatusNoContent, entry.Data["status"])
}
