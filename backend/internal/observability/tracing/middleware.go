package tracing

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// HTTPMiddleware returns an HTTP middleware that instruments requests with OpenTelemetry tracing
func HTTPMiddleware(tracer *Tracer) func(http.Handler) http.Handler {
	// If tracer is nil, return a no-op middleware
	if tracer == nil {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	// Use otelhttp middleware for automatic HTTP instrumentation
	return func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(next, "vibexp-api",
			otelhttp.WithSpanNameFormatter(spanNameFormatter),
			otelhttp.WithSpanOptions(
				trace.WithAttributes(
					attribute.String("service.name", "vibexp-api"),
				),
			),
		)
	}
}

// spanNameFormatter formats the span name based on the HTTP request
func spanNameFormatter(_ string, r *http.Request) string {
	return r.Method + " " + r.URL.Path
}
