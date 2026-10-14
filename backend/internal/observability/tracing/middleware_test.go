package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// A path can carry a bearer credential (an invitation token, #1435). Neither
// the span name nor any of its attributes may export it to the trace backend.
func TestHTTPMiddleware_RedactsSensitivePathsFromTheSpan(t *testing.T) {
	const segment = "an-invitation-link-segment"
	exporter := tracetest.NewInMemoryExporter()
	tracer, err := New("test-version",
		WithExporterProvider(func(context.Context) (sdktrace.SpanExporter, error) { return exporter, nil }),
		WithSampleRatio(1.0))
	require.NoError(t, err)
	handler := HTTPMiddleware(tracer)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, path := range []string{
		"/api/v1/invitations/" + segment + "/register",
		"/api/v1/invitations/" + segment + "/preview",
		"/api/v1/teams/team-1/prompts",
	} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, path, nil))
	}
	// Shutdown flushes the batcher into the exporter's last snapshot.
	require.NoError(t, tracer.tracerProvider.ForceFlush(context.Background()))
	spans := exporter.GetSpans()
	require.Len(t, spans, 3)

	names := make([]string, 0, len(spans))
	for _, span := range spans {
		names = append(names, span.Name)
		assert.NotContains(t, span.Name, segment)
		for _, attr := range span.Attributes {
			assert.NotContains(t, attr.Value.String(), segment, "attribute %s", attr.Key)
		}
	}
	assert.Contains(t, names, "POST /api/v1/invitations/[redacted]/register")
	assert.Contains(t, names, "POST /api/v1/invitations/[redacted]/preview")
	// Any other path is recorded as it is.
	assert.Contains(t, names, "POST /api/v1/teams/team-1/prompts")
	require.NoError(t, tracer.Shutdown(context.Background()))
}
