package observability

import "time"

// Config holds OpenTelemetry configuration for metrics and tracing.
// This is application-level infrastructure configuration for observability exports.
type Config struct {
	// Endpoint is the OTLP exporter endpoint (e.g., "localhost:4317")
	// Set via OTEL_EXPORTER_OTLP_ENDPOINT environment variable
	Endpoint string `envconfig:"OTEL_EXPORTER_OTLP_ENDPOINT" default:"localhost:4317"`

	// ExportInterval is how often to export metrics (e.g., "60s", "5m")
	// Set via OTEL_METRIC_EXPORT_INTERVAL environment variable
	ExportInterval time.Duration `envconfig:"OTEL_METRIC_EXPORT_INTERVAL" default:"60s"`

	// TraceSampleRatio is the sampling ratio for traces (0.0 to 1.0)
	// 1.0 = sample all traces, 0.1 = sample 10% of traces
	// Set via OTEL_TRACE_SAMPLE_RATIO environment variable
	// Default is 0.1 (10%) to control Cloud Trace ingestion costs.
	// A ParentBased sampler wraps this so that if the upstream (Cloud Run LB)
	// has already decided to sample, the child always follows.
	TraceSampleRatio float64 `envconfig:"OTEL_TRACE_SAMPLE_RATIO" default:"0.1"`

	// TracingEnabled controls whether tracing is enabled
	// Set via OTEL_TRACING_ENABLED environment variable
	// Disabled by default, enabled in production deployment
	TracingEnabled bool `envconfig:"OTEL_TRACING_ENABLED" default:"false"`
}
