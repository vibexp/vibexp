package tracing

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	apierrors "github.com/vibexp/vibexp/internal/errors"
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
		return otelhttp.NewHandler(redactPathAttribute(next), "vibexp-api",
			otelhttp.WithSpanNameFormatter(spanNameFormatter),
			otelhttp.WithSpanOptions(
				trace.WithAttributes(
					attribute.String("service.name", "vibexp-api"),
				),
			),
		)
	}
}

// urlPathAttribute is the semantic-convention attribute otelhttp sets from the
// raw request path on every server span.
const urlPathAttribute = "url.path"

// redactPathAttribute overwrites the span's url.path with the redacted path.
// otelhttp records the raw path there before the handler runs, and a path can
// carry a bearer credential (an invitation token) that must not be exported to
// the trace backend. For every other path the value is unchanged.
func redactPathAttribute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if redacted := apierrors.RedactSensitivePath(r.URL.Path); redacted != r.URL.Path {
			trace.SpanFromContext(r.Context()).SetAttributes(attribute.String(urlPathAttribute, redacted))
		}
		next.ServeHTTP(w, r)
	})
}

// spanNameFormatter formats the span name based on the HTTP request
func spanNameFormatter(_ string, r *http.Request) string {
	// A path can carry a bearer credential (an invitation token); a span name
	// must not.
	return r.Method + " " + apierrors.RedactSensitivePath(r.URL.Path)
}
