package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

const (
	apiCallsTotalMetric   = "vx_api_calls_total"
	apiCallDurationMetric = "vx_api_call_duration_seconds"
	// mcpStreamingRoute is a full MCP route pattern fixture matching the live
	// mount (the production prefix constant mcpStreamRoutePrefix matches it).
	// contentTypeEventStream is defined in middleware.go and reused here.
	mcpStreamingRoute = "/mcp/v1/common"
)

// newTestMetricsWithReader creates a Metrics instance backed by a ManualReader
// that the caller can scrape via reader.Collect to assert on recorded samples.
func newTestMetricsWithReader(t *testing.T) (*Metrics, sdkmetric.Reader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	m, err := New("test-version", WithReaderProvider(func(_ context.Context) (sdkmetric.Reader, error) {
		return reader, nil
	}))
	require.NoError(t, err)
	require.NotNil(t, m)
	return m, reader
}

// scrapeMetrics collects the current metrics from the reader.
func scrapeMetrics(t *testing.T, reader sdkmetric.Reader) *metricdata.ResourceMetrics {
	t.Helper()
	rm := &metricdata.ResourceMetrics{}
	require.NoError(t, reader.Collect(context.Background(), rm))
	return rm
}

// durationSampleCount returns the total number of samples recorded into the
// vx_api_call_duration_seconds histogram across all data points (0 if absent).
func durationSampleCount(t *testing.T, rm *metricdata.ResourceMetrics) uint64 {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != apiCallDurationMetric {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[float64])
			require.True(t, ok, "metric %s should be Histogram[float64]", apiCallDurationMetric)
			var total uint64
			for _, dp := range hist.DataPoints {
				total += dp.Count
			}
			return total
		}
	}
	return 0
}

// callsCounterValue returns the summed value of the vx_api_calls_total counter
// across all data points (0 if absent).
func callsCounterValue(t *testing.T, rm *metricdata.ResourceMetrics) float64 {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != apiCallsTotalMetric {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[float64])
			require.True(t, ok, "metric %s should be Sum[float64]", apiCallsTotalMetric)
			var total float64
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
			return total
		}
	}
	return 0
}

// routeAttributePresent reports whether the named metric has a data point
// carrying the http.route attribute set to expectedRoute.
func routeAttributePresent(rm *metricdata.ResourceMetrics, metricName, expectedRoute string) bool {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != metricName {
				continue
			}
			if attrSetHasRoute(metricDataPointAttrs(m.Data), expectedRoute) {
				return true
			}
		}
	}
	return false
}

func metricDataPointAttrs(data metricdata.Aggregation) []attribute.Set {
	switch d := data.(type) {
	case metricdata.Histogram[float64]:
		sets := make([]attribute.Set, 0, len(d.DataPoints))
		for _, dp := range d.DataPoints {
			sets = append(sets, dp.Attributes)
		}
		return sets
	case metricdata.Sum[float64]:
		sets := make([]attribute.Set, 0, len(d.DataPoints))
		for _, dp := range d.DataPoints {
			sets = append(sets, dp.Attributes)
		}
		return sets
	default:
		return nil
	}
}

func attrSetHasRoute(sets []attribute.Set, expectedRoute string) bool {
	for _, set := range sets {
		for _, attr := range set.ToSlice() {
			if string(attr.Key) == AttrHTTPRoute && attr.Value.AsString() == expectedRoute {
				return true
			}
		}
	}
	return false
}

// newTestMetrics creates a Metrics instance for testing with a ManualReader
// that doesn't try to connect to any external OTLP endpoint.
func newTestMetrics(t *testing.T) *Metrics {
	t.Helper()
	m, err := New("test-version", WithReaderProvider(func(ctx context.Context) (sdkmetric.Reader, error) {
		return sdkmetric.NewManualReader(), nil
	}))
	require.NoError(t, err)
	require.NotNil(t, m)
	return m
}

// testWrite is a helper to write data and assert on success
func testWrite(t *testing.T, w http.ResponseWriter, data []byte) {
	t.Helper()
	n, err := w.Write(data)
	require.NoError(t, err, "Write should succeed")
	require.Equal(t, len(data), n, "Write should write all bytes")
}

// TestResponseWriter_WriteHeader tests the responseWriter WriteHeader functionality
func TestResponseWriter_WriteHeader(t *testing.T) {
	tests := []struct {
		name           string
		operations     func(*responseWriter)
		expectedStatus int
	}{
		{
			name: "single writeheader",
			operations: func(rw *responseWriter) {
				rw.WriteHeader(http.StatusNotFound)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "multiple writeheader calls - should only write once",
			operations: func(rw *responseWriter) {
				rw.WriteHeader(http.StatusOK)
				rw.WriteHeader(http.StatusInternalServerError)
				rw.WriteHeader(http.StatusNotFound)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "default status when no writeheader called",
			operations: func(rw *responseWriter) {
				// Don't call WriteHeader
			},
			expectedStatus: http.StatusOK, // Default status
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			rw := &responseWriter{
				ResponseWriter: recorder,
				status:         http.StatusOK,
			}

			tt.operations(rw)

			assert.Equal(t, tt.expectedStatus, rw.status, "status should match expected value")

			// Verify the recorder got the correct status
			if tt.expectedStatus != http.StatusOK {
				assert.Equal(t, tt.expectedStatus, recorder.Code, "recorder code should match")
			}
		})
	}
}

// TestResponseWriter_Write tests the responseWriter Write functionality
func TestResponseWriter_Write(t *testing.T) {
	tests := []struct {
		name           string
		operations     func(*responseWriter)
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "write without writeheader",
			operations: func(rw *responseWriter) {
				testWrite(t, rw, []byte("hello"))
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "hello",
		},
		{
			name: "write after writeheader",
			operations: func(rw *responseWriter) {
				rw.WriteHeader(http.StatusCreated)
				testWrite(t, rw, []byte("created"))
			},
			expectedStatus: http.StatusCreated,
			expectedBody:   "created",
		},
		{
			name: "multiple writes",
			operations: func(rw *responseWriter) {
				testWrite(t, rw, []byte("hello "))
				testWrite(t, rw, []byte("world"))
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			rw := &responseWriter{
				ResponseWriter: recorder,
				status:         http.StatusOK,
			}

			tt.operations(rw)

			assert.Equal(t, tt.expectedStatus, rw.status, "status should match expected value")
			assert.Equal(t, tt.expectedBody, recorder.Body.String(), "body should match expected value")
		})
	}
}

// TestResponseWriter_ConcurrentWriteHeader tests concurrent WriteHeader calls for race conditions
func TestResponseWriter_ConcurrentWriteHeader(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := &responseWriter{
		ResponseWriter: recorder,
		status:         http.StatusOK,
	}

	// Launch multiple goroutines calling WriteHeader
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func(code int) {
			rw.WriteHeader(code)
			done <- true
		}(http.StatusOK + i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 100; i++ {
		<-done
	}

	// Verify that status was set to one of the codes and wroteHeader is true
	assert.True(t, rw.wroteHeader, "wroteHeader should be true")
	assert.GreaterOrEqual(t, rw.status, http.StatusOK, "status should be >= 200")
	assert.Less(t, rw.status, http.StatusOK+100, "status should be < 300")
}

// TestResponseWriter_Flush tests the Flush method
func TestResponseWriter_Flush(t *testing.T) {
	t.Run("with flusher", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		rw := &responseWriter{
			ResponseWriter: recorder,
		}

		// Should not panic
		rw.Flush()
	})

	t.Run("without flusher", func(t *testing.T) {
		// Create a responseWriter with a non-flushing underlying writer
		rw := &responseWriter{
			ResponseWriter: &nonFlusherWriter{},
		}

		// Should not panic
		rw.Flush()
	})
}

// TestResponseWriter_Hijack tests the Hijack method
func TestResponseWriter_Hijack(t *testing.T) {
	t.Run("with hijacker", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		rw := &responseWriter{
			ResponseWriter: recorder,
		}

		// httptest.NewRecorder doesn't implement Hijack, so it should return error
		conn, rw2, err := rw.Hijack()
		assert.Error(t, err, "should return error for non-hijacker")
		assert.Nil(t, conn, "conn should be nil")
		assert.Nil(t, rw2, "rw should be nil")
	})
}

// TestResponseWriter_Push tests the Push method
func TestResponseWriter_Push(t *testing.T) {
	t.Run("with pusher", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		rw := &responseWriter{
			ResponseWriter: recorder,
		}

		// httptest.NewRecorder doesn't implement Push, so it should return error
		err := rw.Push("/target", nil)
		assert.Error(t, err, "should return error for non-pusher")
	})
}

// TestRecordAPICall_InputValidation tests input validation in RecordAPICall
func TestRecordAPICall_InputValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("nil metrics", func(t *testing.T) {
		var m *Metrics
		// Should not panic
		m.RecordAPICall(ctx, "", "", "", 0, false)
	})

	t.Run("empty method defaults to UNKNOWN", func(t *testing.T) {
		// Note: We can't easily test the actual metric recording without
		// a real meter provider, but we can verify it doesn't panic
		m := newTestMetrics(t)
		// Should not panic with empty inputs
		m.RecordAPICall(ctx, "", "/path", "200", 0, false)
	})

	t.Run("empty path defaults to /unknown", func(t *testing.T) {
		m := newTestMetrics(t)
		// Should not panic with empty path
		m.RecordAPICall(ctx, "GET", "", "200", 0, false)
	})

	t.Run("empty statusCode defaults to 0", func(t *testing.T) {
		m := newTestMetrics(t)
		// Should not panic with empty statusCode
		m.RecordAPICall(ctx, "POST", "/api/test", "", 0, false)
	})

	t.Run("all empty inputs", func(t *testing.T) {
		m := newTestMetrics(t)
		// Should not panic with all empty inputs
		m.RecordAPICall(ctx, "", "", "", 0, false)
	})

	t.Run("valid inputs", func(t *testing.T) {
		m := newTestMetrics(t)
		// Should not panic with valid inputs
		m.RecordAPICall(ctx, "GET", "/api/v1/prompts", "200", 0, false)
	})
}

// TestMetricsMiddleware_RecordsMetrics tests that the middleware records metrics
func TestMetricsMiddleware_RecordsMetrics(t *testing.T) {
	metrics := newTestMetrics(t)

	middleware := MetricsMiddleware(metrics)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		testWrite(t, w, []byte("ok"))
	})

	req := httptest.NewRequest("GET", "/test/path", nil)
	recorder := httptest.NewRecorder()

	middleware(handler).ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "ok", recorder.Body.String())
}

// TestMetricsMiddleware_NilMetrics tests that nil metrics doesn't panic
func TestMetricsMiddleware_NilMetrics(t *testing.T) {
	middleware := MetricsMiddleware(nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test/path", nil)
	recorder := httptest.NewRecorder()

	// Should not panic
	assert.NotPanics(t, func() {
		middleware(handler).ServeHTTP(recorder, req)
	})
}

// TestMetricsMiddleware_CapturesStatusCode tests that the middleware captures status codes
func TestMetricsMiddleware_CapturesStatusCode(t *testing.T) {
	metrics := newTestMetrics(t)

	middleware := MetricsMiddleware(metrics)

	testCases := []int{
		http.StatusOK,
		http.StatusCreated,
		http.StatusNotFound,
		http.StatusInternalServerError,
	}

	for _, status := range testCases {
		t.Run(http.StatusText(status), func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			recorder := httptest.NewRecorder()

			middleware(handler).ServeHTTP(recorder, req)

			assert.Equal(t, status, recorder.Code)
		})
	}
}

// TestNew_MetricsInitialization tests metrics initialization
func TestNew_MetricsInitialization(t *testing.T) {
	t.Run("successful initialization", func(t *testing.T) {
		m := newTestMetrics(t)
		assert.NotNil(t, m)
		assert.NotNil(t, m.APICallsTotal)
		assert.NotNil(t, m.APICallDuration)
		assert.NotNil(t, m.meterProvider)
	})
}

// TestMetrics_Shutdown tests the Shutdown functionality
func TestMetrics_Shutdown(t *testing.T) {
	t.Run("shutdown succeeds", func(t *testing.T) {
		m := newTestMetrics(t)

		ctx := context.Background()
		err := m.Shutdown(ctx)
		assert.NoError(t, err, "Shutdown should succeed")
	})

	t.Run("shutdown with timeout", func(t *testing.T) {
		m := newTestMetrics(t)

		ctx, cancel := context.WithTimeout(context.Background(), 5)
		defer cancel()
		err := m.Shutdown(ctx)
		assert.NoError(t, err, "Shutdown with timeout should succeed")
	})

	t.Run("shutdown nil metrics", func(t *testing.T) {
		var m *Metrics
		ctx := context.Background()
		err := m.Shutdown(ctx)
		assert.NoError(t, err, "Shutdown on nil metrics should not error")
	})

	t.Run("shutdown after record", func(t *testing.T) {
		m := newTestMetrics(t)

		// Record some metrics
		ctx := context.Background()
		m.RecordAPICall(ctx, "GET", "/api/test", "200", 0, false)
		m.RecordAPICall(ctx, "POST", "/api/test", "201", 0, false)

		// Shutdown should flush these metrics
		err := m.Shutdown(ctx)
		assert.NoError(t, err, "Shutdown after recording should succeed")
	})

	t.Run("multiple shutdown calls", func(t *testing.T) {
		m := newTestMetrics(t)

		ctx := context.Background()

		// First shutdown
		err := m.Shutdown(ctx)
		assert.NoError(t, err, "First shutdown should succeed")

		// Second shutdown returns an error (reader is shutdown)
		// This is expected behavior - meter provider can only be shut down once
		err = m.Shutdown(ctx)
		assert.Error(t, err, "Second shutdown should return error")
		assert.Contains(t, err.Error(), "reader is shutdown", "Error should mention reader is shutdown")
	})
}

// nonFlusherWriter is a minimal http.ResponseWriter that doesn't implement Flusher
type nonFlusherWriter struct{}

func (n *nonFlusherWriter) Header() http.Header {
	return http.Header{}
}

func (n *nonFlusherWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

func (n *nonFlusherWriter) WriteHeader(code int) {}

// TestRecordDeprecatedEndpointCall tests the RecordDeprecatedEndpointCall functionality
func TestRecordDeprecatedEndpointCall(t *testing.T) {
	ctx := context.Background()

	t.Run("nil metrics", func(t *testing.T) {
		var m *Metrics
		// Should not panic
		assert.NotPanics(t, func() {
			m.RecordDeprecatedEndpointCall(ctx, "/api/v1/webhooks/stripe/teams")
		})
	})

	t.Run("records metric with endpoint attribute", func(t *testing.T) {
		m := newTestMetrics(t)
		assert.NotNil(t, m.DeprecatedEndpointCalls)
		// Should not panic and should record the metric
		assert.NotPanics(t, func() {
			m.RecordDeprecatedEndpointCall(ctx, "/api/v1/webhooks/stripe/teams")
		})
	})

	t.Run("records multiple calls", func(t *testing.T) {
		m := newTestMetrics(t)
		// Should handle multiple calls without panic
		assert.NotPanics(t, func() {
			m.RecordDeprecatedEndpointCall(ctx, "/api/v1/webhooks/stripe/teams")
			m.RecordDeprecatedEndpointCall(ctx, "/api/v1/webhooks/stripe/teams")
			m.RecordDeprecatedEndpointCall(ctx, "/api/v1/other/deprecated")
		})
	})

	t.Run("empty endpoint", func(t *testing.T) {
		m := newTestMetrics(t)
		// Should not panic even with empty endpoint
		assert.NotPanics(t, func() {
			m.RecordDeprecatedEndpointCall(ctx, "")
		})
	})
}

// serveOnce drives the metrics middleware once with a handler that sets the
// given Content-Type and writes a body, returning after the request completes.
func serveOnce(t *testing.T, m *Metrics, method, target, contentType string) {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(http.StatusOK)
		testWrite(t, w, []byte("body"))
	})

	req := httptest.NewRequest(method, target, nil)
	recorder := httptest.NewRecorder()
	MetricsMiddleware(m)(handler).ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
}

// TestMetricsMiddleware_StreamingExcludedFromLatency covers acceptance criteria
// 1 and 3: an SSE response contributes 0 samples to the latency histogram but
// is still counted in the calls counter.
func TestMetricsMiddleware_StreamingExcludedFromLatency(t *testing.T) {
	m, reader := newTestMetricsWithReader(t)

	serveOnce(t, m, "GET", "/stream", contentTypeEventStream+"; charset=utf-8")

	rm := scrapeMetrics(t, reader)
	assert.Equal(t, uint64(0), durationSampleCount(t, rm),
		"SSE response must contribute 0 latency samples")
	assert.Equal(t, 1.0, callsCounterValue(t, rm),
		"SSE response must still be counted in the calls counter")
}

// TestMetricsMiddleware_NormalRequestRecordsLatency covers acceptance criteria
// 2 and 3: a normal request contributes exactly 1 latency sample and is counted.
func TestMetricsMiddleware_NormalRequestRecordsLatency(t *testing.T) {
	m, reader := newTestMetricsWithReader(t)

	serveOnce(t, m, "GET", "/normal", "application/json")

	rm := scrapeMetrics(t, reader)
	assert.Equal(t, uint64(1), durationSampleCount(t, rm),
		"normal response must contribute exactly 1 latency sample")
	assert.Equal(t, 1.0, callsCounterValue(t, rm),
		"normal response must be counted in the calls counter")
}

// TestMetricsMiddleware_MCPRouteExcludedWithoutSSEHeader verifies the route
// allowlist fallback: an MCP-mounted route is treated as streaming even when the
// SSE Content-Type header is not observable.
func TestMetricsMiddleware_MCPRouteExcludedWithoutSSEHeader(t *testing.T) {
	m, reader := newTestMetricsWithReader(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		testWrite(t, w, []byte("body"))
	})

	// Inject a chi route context resolving to the MCP streaming mount.
	req := httptest.NewRequest("GET", "/mcp/v1/common", nil)
	rctx := chi.NewRouteContext()
	rctx.RoutePatterns = []string{mcpStreamingRoute}
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	recorder := httptest.NewRecorder()
	MetricsMiddleware(m)(handler).ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)

	rm := scrapeMetrics(t, reader)
	assert.Equal(t, uint64(0), durationSampleCount(t, rm),
		"MCP route must contribute 0 latency samples via route fallback")
	assert.Equal(t, 1.0, callsCounterValue(t, rm),
		"MCP route must still be counted in the calls counter")
}

// TestMetricsMiddleware_MCPPostRecordsLatency verifies the route fallback is
// scoped to GET: a non-SSE POST under the MCP mount is a normal request/response
// JSON-RPC call and must keep its latency sample.
func TestMetricsMiddleware_MCPPostRecordsLatency(t *testing.T) {
	m, reader := newTestMetricsWithReader(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		testWrite(t, w, []byte("{}"))
	})

	req := httptest.NewRequest("POST", "/mcp/v1/common", nil)
	rctx := chi.NewRouteContext()
	rctx.RoutePatterns = []string{mcpStreamingRoute}
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	recorder := httptest.NewRecorder()
	MetricsMiddleware(m)(handler).ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)

	rm := scrapeMetrics(t, reader)
	assert.Equal(t, uint64(1), durationSampleCount(t, rm),
		"non-SSE POST under the MCP mount must record exactly 1 latency sample")
	assert.Equal(t, 1.0, callsCounterValue(t, rm),
		"MCP POST must still be counted in the calls counter")
}

// TestMetricsMiddleware_RouteAttributePopulated covers acceptance criterion 4:
// the duration histogram and the counter carry a bounded http.route attribute
// holding the resolved chi route pattern.
func TestMetricsMiddleware_RouteAttributePopulated(t *testing.T) {
	m, reader := newTestMetricsWithReader(t)

	const route = "/api/v1/{team_id}/prompts"
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		testWrite(t, w, []byte("body"))
	})

	req := httptest.NewRequest("GET", "/api/v1/team-1/prompts", nil)
	rctx := chi.NewRouteContext()
	rctx.RoutePatterns = []string{route}
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	recorder := httptest.NewRecorder()
	MetricsMiddleware(m)(handler).ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)

	rm := scrapeMetrics(t, reader)
	assert.True(t, routeAttributePresent(rm, apiCallDurationMetric, route),
		"latency histogram must carry the bounded http.route attribute")
	assert.True(t, routeAttributePresent(rm, apiCallsTotalMetric, route),
		"calls counter must carry the bounded http.route attribute")
}

// TestMetricsMiddleware_UnknownRouteFallback verifies the bounded UNKNOWN_ROUTE
// fallback when chi has not resolved a route pattern.
func TestMetricsMiddleware_UnknownRouteFallback(t *testing.T) {
	m, reader := newTestMetricsWithReader(t)

	serveOnce(t, m, "GET", "/no-chi-context", "application/json")

	rm := scrapeMetrics(t, reader)
	assert.True(t, routeAttributePresent(rm, apiCallDurationMetric, unknownRoute),
		"latency histogram must fall back to UNKNOWN_ROUTE")
	assert.Equal(t, uint64(1), durationSampleCount(t, rm))
}

// TestRecordAPICall_StreamingSkipsDuration verifies RecordAPICall directly:
// isStreaming=true skips the histogram but still increments the counter.
func TestRecordAPICall_StreamingSkipsDuration(t *testing.T) {
	tests := []struct {
		name             string
		isStreaming      bool
		wantSampleCount  uint64
		wantCounterValue float64
	}{
		{name: "streaming skips duration", isStreaming: true, wantSampleCount: 0, wantCounterValue: 1},
		{name: "non-streaming records duration", isStreaming: false, wantSampleCount: 1, wantCounterValue: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, reader := newTestMetricsWithReader(t)

			m.RecordAPICall(context.Background(), "GET", mcpStreamingRoute, "200", time.Second, tc.isStreaming)

			rm := scrapeMetrics(t, reader)
			assert.Equal(t, tc.wantSampleCount, durationSampleCount(t, rm))
			assert.Equal(t, tc.wantCounterValue, callsCounterValue(t, rm))
		})
	}
}

// TestIsStreamingResponse exercises the streaming-detection helper directly.
func TestIsStreamingResponse(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		contentType string
		routePat    string
		want        bool
	}{
		{name: "sse content type", method: "GET", contentType: contentTypeEventStream, routePat: "/x", want: true},
		{
			name: "sse with params", method: "GET",
			contentType: contentTypeEventStream + "; charset=utf-8", routePat: "/x", want: true,
		},
		{
			name: "sse content type uppercase", method: "GET",
			contentType: "Text/Event-Stream", routePat: "/x", want: true,
		},
		{
			name: "mcp route fallback on GET", method: "GET",
			contentType: "application/json", routePat: mcpStreamingRoute, want: true,
		},
		{
			name: "mcp route POST keeps latency", method: "POST",
			contentType: "application/json", routePat: mcpStreamingRoute, want: false,
		},
		{name: "normal json", method: "GET", contentType: "application/json", routePat: "/api/v1/prompts", want: false},
		{name: "empty content type non-mcp", method: "GET", contentType: "", routePat: unknownRoute, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rw := &responseWriter{ResponseWriter: httptest.NewRecorder()}
			if tc.contentType != "" {
				rw.Header().Set("Content-Type", tc.contentType)
			}
			assert.Equal(t, tc.want, isStreamingResponse(rw, tc.method, tc.routePat))
		})
	}
}

// instrumentDescriptor captures the externally visible identity of one
// instrument: the exact values that form the dashboard contract with
// Google Managed Prometheus (PromQL queries match these names 1:1).
type instrumentDescriptor struct {
	name        string
	description string
	unit        string
	kind        string
}

// expectedInstruments is the authoritative snapshot of every instrument the
// Metrics struct must register. Any drift in name, description, unit, or
// instrument kind breaks production dashboards (epic #1484), so a change here
// must be deliberate and coordinated with the PromQL dashboard definitions.
var expectedInstruments = []instrumentDescriptor{
	{"vx_ai_tools_hooks_call", "Total number of AI tools hooks calls by tool name", "1", "Int64Counter"},
	{"vx_api_call_duration_seconds", "Duration of API calls", "s", "Float64Histogram"},
	{"vx_api_calls_total", "Total number of API calls", "1", "Float64Counter"},
	{"vx_api_key_created", "Total number of API keys created", "1", "Int64Counter"},
	{"vx_artifact_created", "Total number of artifacts created", "1", "Int64Counter"},
	{"vx_artifact_deleted", "Total number of artifacts deleted", "1", "Int64Counter"},
	{"vx_blueprint_created", "Total number of blueprints created", "1", "Int64Counter"},
	{"vx_blueprint_deleted", "Total number of blueprints deleted", "1", "Int64Counter"},
	{"vx_deprecated_endpoint_calls_total", "Total number of calls to deprecated API endpoints", "1", "Int64Counter"},
	{
		"vx_event_bus_circuit_breaker_open_total",
		"Total number of times circuit breaker opened by listener type", "1", "Int64Counter",
	},
	{
		"vx_event_bus_event_duration_seconds",
		"Event processing duration in seconds by listener type, event type, and success status", "s", "Float64Histogram",
	},
	{
		"vx_event_bus_retry_attempts_total",
		"Total number of retry attempts by listener type and event type", "1", "Int64Counter",
	},
	{
		"vx_event_bus_retry_backoff_seconds",
		"Retry backoff duration in seconds by listener type and event type", "s", "Float64Histogram",
	},
	{
		"vx_event_bus_retry_failure_total",
		"Total number of failed retries (after all attempts) by listener type and event type", "1", "Int64Counter",
	},
	{
		"vx_event_bus_retry_success_total",
		"Total number of successful retries by listener type and event type", "1", "Int64Counter",
	},
	{"vx_mcp_create_artifact", "Total number of MCP create artifact calls", "1", "Int64Counter"},
	{"vx_mcp_create_blueprint", "Total number of MCP create blueprint calls", "1", "Int64Counter"},
	{"vx_mcp_create_memory", "Total number of MCP create memory calls", "1", "Int64Counter"},
	{"vx_mcp_create_prompt", "Total number of MCP create prompt calls", "1", "Int64Counter"},
	{"vx_mcp_datetime", "Total number of MCP datetime calls", "1", "Int64Counter"},
	{"vx_mcp_get_artifact", "Total number of MCP get artifact calls", "1", "Int64Counter"},
	{"vx_mcp_get_memory", "Total number of MCP get memory calls", "1", "Int64Counter"},
	{"vx_mcp_get_prompt", "Total number of MCP get prompt calls", "1", "Int64Counter"},
	{"vx_mcp_get_user", "Total number of MCP get user calls", "1", "Int64Counter"},
	{"vx_mcp_list_artifacts", "Total number of MCP list artifacts calls", "1", "Int64Counter"},
	{"vx_mcp_list_memories", "Total number of MCP list memories calls", "1", "Int64Counter"},
	{"vx_mcp_list_prompts", "Total number of MCP list prompts calls", "1", "Int64Counter"},
	{"vx_mcp_list_tools", "Total number of MCP list tools calls", "1", "Int64Counter"},
	{"vx_mcp_search_prompts", "Total number of MCP search prompts calls", "1", "Int64Counter"},
	{"vx_mcp_update_artifact", "Total number of MCP update artifact calls", "1", "Int64Counter"},
	{"vx_mcp_update_blueprint", "Total number of MCP update blueprint calls", "1", "Int64Counter"},
	{"vx_mcp_update_memory", "Total number of MCP update memory calls", "1", "Int64Counter"},
	{"vx_mcp_update_prompt", "Total number of MCP update prompt calls", "1", "Int64Counter"},
	{"vx_memory_created", "Total number of memories created", "1", "Int64Counter"},
	{"vx_memory_deleted", "Total number of memories deleted", "1", "Int64Counter"},
	{
		"vx_notifications_delivery_duration_seconds",
		"Duration of notification delivery by channel", "s", "Float64Histogram",
	},
	{
		"vx_notifications_digest_queue_depth",
		"Point-in-time count of pending rows in notification_digest_queue at job start", "1", "Int64Gauge",
	},
	{
		"vx_notifications_digest_runner_duration_seconds",
		"Total wall-clock duration of a digest runner execution", "s", "Float64Histogram",
	},
	{
		"vx_notifications_digest_sent_total",
		"Total number of digest emails attempted by status (sent|failed|skipped)", "1", "Int64Counter",
	},
	{
		"vx_notifications_listener_errors_total",
		"Total number of notification event listener errors by event type", "1", "Int64Counter",
	},
	{"vx_notifications_sent_total", "Total number of notifications sent by channel and status", "1", "Int64Counter"},
	{"vx_prompt_created", "Total number of prompts created", "1", "Int64Counter"},
	{"vx_prompt_deleted", "Total number of prompts deleted", "1", "Int64Counter"},
	{"vx_stripe_payment_failed", "Total number of Stripe invoice.payment_failed webhooks received", "1", "Int64Counter"},
	{
		"vx_stripe_payment_succeeded",
		"Total number of Stripe invoice.payment_succeeded webhooks received", "1", "Int64Counter",
	},
	{
		"vx_stripe_subscription_created",
		"Total number of Stripe subscription.created webhooks received", "1", "Int64Counter",
	},
	{
		"vx_stripe_subscription_deleted",
		"Total number of Stripe subscription.deleted webhooks received", "1", "Int64Counter",
	},
	{
		"vx_stripe_subscription_updated",
		"Total number of Stripe subscription.updated webhooks received", "1", "Int64Counter",
	},
	{"vx_user_created", "Total number of users created", "1", "Int64Counter"},
	{"vx_user_login_failed", "Total number of failed user login attempts", "1", "Int64Counter"},
	{"vx_user_login_successful", "Total number of successful user logins", "1", "Int64Counter"},
}

// latencyViewBoundaries mirrors the explicit bucket boundaries pinned by the
// latencyView in newMeterProvider — also part of the dashboard contract.
var latencyViewBoundaries = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// recordOneSampleEachInstrument touches every instrument once via the exported
// struct fields, so the subsequent scrape exports a descriptor for each. The
// SDK only exports instruments that have recorded at least one measurement.
// Fields are exercised directly (not through Record* wrappers) because three
// notification instruments have no wrapper method.
func recordOneSampleEachInstrument(ctx context.Context, m *Metrics) {
	for _, counter := range []metric.Int64Counter{
		m.UserCreated, m.UserLoginSuccessful, m.UserLoginFailed,
		m.StripeSubscriptionCreated, m.StripeSubscriptionUpdated, m.StripeSubscriptionDeleted,
		m.StripePaymentSucceeded, m.StripePaymentFailed,
		m.APIKeyCreated, m.AIToolsHooksCall,
		m.PromptCreated, m.PromptDeleted,
		m.ArtifactCreated, m.ArtifactDeleted,
		m.MemoryCreated, m.MemoryDeleted,
		m.BlueprintCreated, m.BlueprintDeleted,
		m.MCPListTools, m.MCPListPrompts, m.MCPGetPrompt, m.MCPSearchPrompts,
		m.MCPListArtifacts, m.MCPGetArtifact, m.MCPCreateArtifact, m.MCPUpdateArtifact,
		m.MCPListMemories, m.MCPGetMemory, m.MCPCreateMemory, m.MCPUpdateMemory,
		m.MCPCreatePrompt, m.MCPUpdatePrompt,
		m.MCPCreateBlueprint, m.MCPUpdateBlueprint,
		m.MCPDateTime, m.MCPGetUser,
		m.EventBusRetryAttempts, m.EventBusRetrySuccess, m.EventBusRetryFailure,
		m.EventBusCircuitBreakerOpen,
		m.NotificationsSentTotal, m.NotificationsListenerErrs,
		m.NotificationDigestEmailsSentTotal,
		m.DeprecatedEndpointCalls,
	} {
		counter.Add(ctx, 1)
	}
	m.APICallsTotal.Add(ctx, 1)
	for _, histogram := range []metric.Float64Histogram{
		m.APICallDuration,
		m.EventBusRetryBackoff, m.EventBusEventDuration,
		m.NotificationsDeliveryDur, m.NotificationDigestRunnerDurationSecs,
	} {
		histogram.Record(ctx, 0.1)
	}
	m.NotificationDigestQueueDepth.Record(ctx, 1)
}

// instrumentKind maps the scraped aggregation back to the instrument
// constructor that produced it.
func instrumentKind(t *testing.T, mtr metricdata.Metrics) string {
	t.Helper()
	switch data := mtr.Data.(type) {
	case metricdata.Sum[int64]:
		require.True(t, data.IsMonotonic, "counter %s must be monotonic", mtr.Name)
		return "Int64Counter"
	case metricdata.Sum[float64]:
		require.True(t, data.IsMonotonic, "counter %s must be monotonic", mtr.Name)
		return "Float64Counter"
	case metricdata.Histogram[float64]:
		return "Float64Histogram"
	case metricdata.Gauge[int64]:
		return "Int64Gauge"
	default:
		t.Fatalf("unexpected aggregation %T for instrument %s", mtr.Data, mtr.Name)
		return ""
	}
}

// collectInstrumentDescriptors scrapes the reader and flattens every exported
// instrument into a name-sorted descriptor slice.
func collectInstrumentDescriptors(t *testing.T, rm *metricdata.ResourceMetrics) []instrumentDescriptor {
	t.Helper()
	var got []instrumentDescriptor
	for _, sm := range rm.ScopeMetrics {
		for _, mtr := range sm.Metrics {
			got = append(got, instrumentDescriptor{
				name:        mtr.Name,
				description: mtr.Description,
				unit:        mtr.Unit,
				kind:        instrumentKind(t, mtr),
			})
		}
	}
	sort.Slice(got, func(i, j int) bool { return got[i].name < got[j].name })
	return got
}

// TestNew_RegistersAllInstruments is the parity snapshot guard for the
// metric inventory: it asserts the exact set of {name, description, unit,
// kind} for every instrument New registers, plus the pinned latency-view
// bucket boundaries. It must pass identically before and after any refactor
// of the instrument registration code.
func TestNew_RegistersAllInstruments(t *testing.T) {
	m, reader := newTestMetricsWithReader(t)

	recordOneSampleEachInstrument(context.Background(), m)

	rm := scrapeMetrics(t, reader)
	got := collectInstrumentDescriptors(t, rm)

	assert.Equal(t, expectedInstruments, got,
		"instrument inventory drifted — metric names/units/descriptions/kinds are the production dashboard contract")
}

// TestNew_LatencyViewBucketsPinned asserts the explicit histogram buckets the
// latencyView applies to vx_api_call_duration_seconds.
func TestNew_LatencyViewBucketsPinned(t *testing.T) {
	m, reader := newTestMetricsWithReader(t)

	m.RecordAPICall(context.Background(), "GET", "/api/v1/prompts", "200", 50*time.Millisecond, false)

	rm := scrapeMetrics(t, reader)
	for _, sm := range rm.ScopeMetrics {
		for _, mtr := range sm.Metrics {
			if mtr.Name != apiCallDurationMetric {
				continue
			}
			hist, ok := mtr.Data.(metricdata.Histogram[float64])
			require.True(t, ok, "%s must be Histogram[float64]", apiCallDurationMetric)
			require.NotEmpty(t, hist.DataPoints)
			assert.Equal(t, latencyViewBoundaries, hist.DataPoints[0].Bounds,
				"latencyView bucket boundaries are pinned to the dashboard contract")
			return
		}
	}
	t.Fatalf("%s not found in scrape", apiCallDurationMetric)
}

// TestRegistrar_CollectsFirstError verifies the registrar's error contract:
// creation errors are captured (wrapped with the instrument name), the first
// error wins, registration never panics, and later registrations still run.
func TestRegistrar_CollectsFirstError(t *testing.T) {
	meter := sdkmetric.NewMeterProvider().Meter("registrar-test")
	r := &registrar{meter: meter}

	// OTel instrument names must start with a letter — these both fail.
	assert.NotPanics(t, func() {
		r.int64Counter(instrumentSpec{"1st_invalid", "first failure", "1"})
		r.float64Histogram(instrumentSpec{"2nd_invalid", "second failure", "s"})
	})
	require.Error(t, r.err)
	assert.Contains(t, r.err.Error(), "create instrument 1st_invalid",
		"first error must win and carry the failing instrument name")
	assert.NotContains(t, r.err.Error(), "2nd_invalid")

	// A valid registration after a captured error still produces a usable instrument.
	c := r.int64Counter(instrumentSpec{"vx_registrar_test_ok", "valid after failure", "1"})
	assert.NotNil(t, c)
}

// TestRegistrar_NoErrorOnValidSpecs verifies the zero-value registrar reports
// no error after successful registrations.
func TestRegistrar_NoErrorOnValidSpecs(t *testing.T) {
	meter := sdkmetric.NewMeterProvider().Meter("registrar-test")
	r := &registrar{meter: meter}

	assert.NotNil(t, r.int64Counter(instrumentSpec{"vx_ok_counter", "d", "1"}))
	assert.NotNil(t, r.float64Counter(instrumentSpec{"vx_ok_fcounter", "d", "1"}))
	assert.NotNil(t, r.float64Histogram(instrumentSpec{"vx_ok_hist", "d", "s"}))
	assert.NotNil(t, r.int64Gauge(instrumentSpec{"vx_ok_gauge", "d", "1"}))
	assert.NoError(t, r.err)
}
