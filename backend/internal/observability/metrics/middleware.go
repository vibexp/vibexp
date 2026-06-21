package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	// unknownRoute is the bounded fallback used when chi has not resolved a route
	// pattern, preventing cardinality explosion from unmatched requests (e.g. 404s).
	unknownRoute = "UNKNOWN_ROUTE"

	// contentTypeEventStream is the SSE media type. Responses with this
	// Content-Type are long-lived streams whose connection lifetime must be
	// excluded from the request-latency histogram.
	contentTypeEventStream = "text/event-stream"

	// mcpStreamRoutePrefix is the route-pattern prefix of the MCP Streamable
	// HTTP mount (see the r.Mount("/mcp/v1/common") in
	// internal/server/server.go). A GET under this prefix opens a long-lived
	// SSE stream; it is used as a defensive fallback for excluding that stream
	// from the latency histogram when the SSE Content-Type is not observable.
	mcpStreamRoutePrefix = "/mcp/"
)

// MetricsMiddleware returns a middleware that tracks HTTP request metrics
// This middleware records the vx_api_calls_total counter for each request
//
// The metric includes the following attributes:
// - http.method: The HTTP method (GET, POST, PUT, DELETE, etc.)
// - http.path: The request path pattern (e.g., /api/v1/prompts/{slug})
// - http.route: The bounded chi route pattern, for per-route latency slicing
// - http.status_code: The HTTP status code of the response (e.g., 200, 404, 500)
//
// Long-lived streaming responses (MCP Streamable HTTP / SSE) are still counted
// in vx_api_calls_total but excluded from the vx_api_call_duration_seconds
// histogram, so connection lifetime does not distort request latency.
//
// Usage:
//
//	metrics, _ := metrics.New()
//	router.Use(metrics.MetricsMiddleware(metrics))
func MetricsMiddleware(metrics *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap the response writer to capture the status code
			wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			// Call the next handler
			next.ServeHTTP(wrapped, r)

			// Resolve the route pattern AFTER serving: for chi Mount-ed
			// sub-routers the full matched pattern is only populated during the
			// route walk, so reading it earlier can yield an empty pattern.
			routePattern := resolveRoutePattern(r)

			// Record the metric after the handler completes
			if metrics != nil && metrics.APICallsTotal != nil {
				metrics.RecordAPICall(
					r.Context(),
					r.Method,
					routePattern,
					strconv.Itoa(wrapped.status),
					time.Since(start),
					isStreamingResponse(wrapped, r.Method, routePattern),
				)
			}
		})
	}
}

// resolveRoutePattern returns the bounded chi route pattern for the request,
// falling back to unknownRoute when chi has not matched a route.
func resolveRoutePattern(r *http.Request) string {
	if ctxRoute := chi.RouteContext(r.Context()); ctxRoute != nil && ctxRoute.RoutePattern() != "" {
		return ctxRoute.RoutePattern()
	}
	return unknownRoute
}

// isStreamingResponse reports whether the response is a long-lived streaming
// response that should be excluded from the latency histogram.
//
// Primary signal: the SSE Content-Type, matched case-insensitively and
// tolerating params (e.g. "text/event-stream; charset=utf-8"), which the MCP
// Streamable HTTP handler sets on the long-lived GET stream.
//
// Fallback: a GET under the MCP mount prefix, for the case where the SSE
// Content-Type is not observable. The fallback is restricted to GET because
// only the GET transport opens a long-lived stream — POST requests to the MCP
// mount are normal request/response JSON-RPC calls whose latency must stay
// visible.
func isStreamingResponse(wrapped *responseWriter, method, routePattern string) bool {
	contentType := strings.ToLower(wrapped.Header().Get("Content-Type"))
	if strings.HasPrefix(contentType, contentTypeEventStream) {
		return true
	}
	return method == http.MethodGet && strings.HasPrefix(routePattern, mcpStreamRoutePrefix)
}

// responseWriter wraps http.ResponseWriter to capture the status code
// It is safe for concurrent use due to mutex protection of wroteHeader and status fields
type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	mu          sync.Mutex
}

// WriteHeader captures the status code and delegates to the underlying ResponseWriter
// This method is safe for concurrent calls
func (rw *responseWriter) WriteHeader(code int) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

// writeHeaderLocked is like WriteHeader but must be called with mu already held
func (rw *responseWriter) writeHeaderLocked(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

// Write captures that we've written to the response
// This method is safe for concurrent calls
func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if !rw.wroteHeader {
		rw.writeHeaderLocked(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// Flush implements http.Flusher interface for SSE and streaming responses
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack implements http.Hijacker interface for WebSocket connections
func (rw *responseWriter) Hijack() (interface{}, interface{}, error) {
	if hijacker, ok := rw.ResponseWriter.(interface {
		Hijack() (interface{}, interface{}, error)
	}); ok {
		return hijacker.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Push implements http.Pusher interface for HTTP/2 server push
func (rw *responseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := rw.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}
