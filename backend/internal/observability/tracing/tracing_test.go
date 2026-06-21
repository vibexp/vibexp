package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestNew_WithDefaultOptions(t *testing.T) {
	// Use in-memory exporter for testing
	exporter := tracetest.NewInMemoryExporter()

	tracer, err := New("test-version",
		WithExporterProvider(func(ctx context.Context) (sdktrace.SpanExporter, error) {
			return exporter, nil
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, tracer)

	defer func() {
		err := tracer.Shutdown(context.Background())
		assert.NoError(t, err)
	}()

	assert.NotNil(t, tracer.Tracer())
}

func TestNew_WithSampleRatio(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()

	tracer, err := New("test-version",
		WithExporterProvider(func(ctx context.Context) (sdktrace.SpanExporter, error) {
			return exporter, nil
		}),
		WithSampleRatio(0.5),
	)
	require.NoError(t, err)
	require.NotNil(t, tracer)

	defer func() {
		err := tracer.Shutdown(context.Background())
		assert.NoError(t, err)
	}()
}

func TestNew_WithZeroSampleRatio(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()

	tracer, err := New("test-version",
		WithExporterProvider(func(ctx context.Context) (sdktrace.SpanExporter, error) {
			return exporter, nil
		}),
		WithSampleRatio(0.0),
	)
	require.NoError(t, err)
	require.NotNil(t, tracer)

	defer func() {
		err := tracer.Shutdown(context.Background())
		assert.NoError(t, err)
	}()
}

func TestNew_WithFullSampleRatio(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()

	tracer, err := New("test-version",
		WithExporterProvider(func(ctx context.Context) (sdktrace.SpanExporter, error) {
			return exporter, nil
		}),
		WithSampleRatio(1.0),
	)
	require.NoError(t, err)
	require.NotNil(t, tracer)

	defer func() {
		err := tracer.Shutdown(context.Background())
		assert.NoError(t, err)
	}()
}

func TestTracer_StartSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()

	tracer, err := New("test-version",
		WithExporterProvider(func(ctx context.Context) (sdktrace.SpanExporter, error) {
			return exporter, nil
		}),
		WithSampleRatio(1.0),
	)
	require.NoError(t, err)
	defer func() {
		shutdownErr := tracer.Shutdown(context.Background())
		assert.NoError(t, shutdownErr)
	}()

	ctx, span := tracer.StartSpan(context.Background(), "test-span")
	require.NotNil(t, span)
	require.NotNil(t, ctx)

	span.End()

	// Force flush
	flushErr := tracer.tracerProvider.ForceFlush(context.Background())
	require.NoError(t, flushErr)

	// Check exported spans
	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "test-span", spans[0].Name)
}

func TestTracer_Shutdown_Nil(t *testing.T) {
	var tracer *Tracer
	err := tracer.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestTracer_StartSpan_Nil(t *testing.T) {
	var tracer *Tracer
	ctx, span := tracer.StartSpan(context.Background(), "test-span")
	assert.NotNil(t, ctx)
	assert.NotNil(t, span) // Returns no-op span from context
}
