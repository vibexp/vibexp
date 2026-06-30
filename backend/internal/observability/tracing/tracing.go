// Package tracing provides OpenTelemetry tracing for Cloud Run with Cloud Trace integration
package tracing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/vibexp/vibexp/internal/config"
)

// Tracer holds the OpenTelemetry tracer provider and tracer
type Tracer struct {
	tracerProvider *sdktrace.TracerProvider
	tracer         trace.Tracer
}

// Option is a functional option for configuring Tracer creation
type Option func(*tracerConfig)

type tracerConfig struct {
	readerProvider func(ctx context.Context) (sdktrace.SpanExporter, error)
	otlpEndpoint   string
	sampleRatio    float64
	appConfig      *config.Config
}

// WithExporterProvider allows customizing the exporter provider (used in tests)
func WithExporterProvider(provider func(ctx context.Context) (sdktrace.SpanExporter, error)) Option {
	return func(c *tracerConfig) {
		c.readerProvider = provider
	}
}

// WithOTelEndpoint sets the OTLP endpoint for trace export
// For gRPC, use format "host:port" without scheme prefix.
func WithOTelEndpoint(endpoint string) Option {
	return func(c *tracerConfig) {
		c.otlpEndpoint = endpoint
	}
}

// WithSampleRatio sets the trace sampling ratio (0.0 to 1.0)
// 1.0 means sample all traces, 0.5 means sample 50% of traces
func WithSampleRatio(ratio float64) Option {
	return func(c *tracerConfig) {
		c.sampleRatio = ratio
	}
}

// WithConfig sets the application config for tracing
func WithConfig(cfg *config.Config) Option {
	return func(c *tracerConfig) {
		c.appConfig = cfg
	}
}

// New initializes and returns a new Tracer instance with a production-ready tracer provider
func New(serviceVersion string, opts ...Option) (*Tracer, error) {
	ctx := context.Background()

	cfg := &tracerConfig{
		sampleRatio: 0.1, // Default: sample 10% of traces
	}
	for _, opt := range opts {
		opt(cfg)
	}

	res, err := createResource(ctx, serviceVersion, cfg.appConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	tracerProvider, err := newTracerProvider(ctx, res, cfg)
	if err != nil {
		return nil, err
	}

	// Set global tracer provider and propagator
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
		&CloudTraceContextPropagator{}, // GCP Cloud Trace context propagator
	))

	tracer := tracerProvider.Tracer("github.com/vibexp/vibexp")

	return &Tracer{
		tracerProvider: tracerProvider,
		tracer:         tracer,
	}, nil
}

func createResource(ctx context.Context, serviceVersion string, cfg *config.Config) (*resource.Resource, error) {
	env := "production"
	if cfg != nil {
		env = cfg.GetDeploymentEnvironment()
	}

	attrs := []attribute.KeyValue{
		semconv.ServiceName("vibexp-api"),
		semconv.ServiceVersion(serviceVersion),
		semconv.DeploymentEnvironment(env),
	}

	// Add Cloud Run specific attributes if available
	if cfg != nil {
		if cfg.Deployment.KService != "" {
			attrs = append(attrs, attribute.String("cloud.run.service", cfg.Deployment.KService))
		}
		if cfg.Deployment.KRevision != "" {
			attrs = append(attrs, attribute.String("cloud.run.revision", cfg.Deployment.KRevision))
		}
		if cfg.GCP.ProjectID != "" {
			attrs = append(attrs, semconv.CloudAccountID(cfg.GCP.ProjectID))
			attrs = append(attrs, semconv.CloudProviderGCP)
			attrs = append(attrs, semconv.CloudPlatformGCPCloudRun)
		}
	}

	return resource.New(
		ctx,
		resource.WithFromEnv(),
		resource.WithAttributes(attrs...),
		resource.WithTelemetrySDK(),
	)
}

func newTracerProvider(
	ctx context.Context,
	res *resource.Resource,
	cfg *tracerConfig,
) (*sdktrace.TracerProvider, error) {
	var exporter sdktrace.SpanExporter
	var err error

	if cfg.readerProvider != nil {
		// Use custom exporter provider (e.g., for tests)
		exporter, err = cfg.readerProvider(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create custom exporter: %w", err)
		}
	} else {
		// Default: Create OTLP exporter for production
		endpoint := cfg.otlpEndpoint
		if endpoint != "" {
			// Remove scheme prefix for gRPC
			endpoint = strings.TrimPrefix(endpoint, "http://")
			endpoint = strings.TrimPrefix(endpoint, "https://")
		}
		if endpoint == "" {
			endpoint = "localhost:4317"
		}

		// Create OTLP trace exporter with gRPC client
		exporter, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(endpoint),
			otlptracegrpc.WithInsecure(), // Local connection, no TLS needed
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP trace exporter for endpoint %s: %w", endpoint, err)
		}
	}

	// Configure sampler: always use ParentBased so that when Cloud Run's
	// load balancer marks a trace as not-sampled, the app follows the parent
	// decision. When there is no parent (or the parent is sampled), the inner
	// TraceIDRatioBased sampler applies the configured ratio.
	var innerSampler sdktrace.Sampler
	switch {
	case cfg.sampleRatio >= 1.0:
		innerSampler = sdktrace.AlwaysSample()
	case cfg.sampleRatio <= 0.0:
		innerSampler = sdktrace.NeverSample()
	default:
		innerSampler = sdktrace.TraceIDRatioBased(cfg.sampleRatio)
	}
	sampler := sdktrace.ParentBased(innerSampler)

	// Create tracer provider with batch processor
	return sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
		),
		sdktrace.WithSampler(sampler),
	), nil
}

// Tracer returns the OpenTelemetry tracer for creating spans
func (t *Tracer) Tracer() trace.Tracer {
	if t == nil {
		return nil
	}
	return t.tracer
}

// Shutdown flushes any buffered traces and closes the tracer provider
func (t *Tracer) Shutdown(ctx context.Context) error {
	if t == nil || t.tracerProvider == nil {
		return nil
	}
	return t.tracerProvider.Shutdown(ctx)
}

// StartSpan starts a new span with the given name and options
func (t *Tracer) StartSpan(
	ctx context.Context,
	name string,
	opts ...trace.SpanStartOption,
) (context.Context, trace.Span) {
	if t == nil || t.tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	return t.tracer.Start(ctx, name, opts...)
}
