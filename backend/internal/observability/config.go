package observability

import "time"

// Config holds OpenTelemetry configuration for metrics and tracing. It is
// populated from the otel section of config.yaml (the koanf tags name the keys).
// This is application-level infrastructure configuration for observability exports.
type Config struct {
	// Endpoint is the OTLP exporter endpoint (e.g., "localhost:4317")
	// Set via otel.endpoint
	Endpoint string `koanf:"endpoint"`

	// ExportInterval is how often to export metrics (e.g., "60s", "5m")
	// Set via otel.export_interval
	ExportInterval time.Duration `koanf:"export_interval"`

	// TraceSampleRatio is the sampling ratio for traces (0.0 to 1.0)
	// 1.0 = sample all traces, 0.1 = sample 10% of traces
	// Set via otel.trace_sample_ratio
	// Default is 0.1 (10%) to control Cloud Trace ingestion costs.
	// A ParentBased sampler wraps this so that if the upstream (Cloud Run LB)
	// has already decided to sample, the child always follows.
	TraceSampleRatio float64 `koanf:"trace_sample_ratio"`

	// TracingEnabled controls whether tracing is enabled
	// Set via otel.tracing_enabled
	// Disabled by default, enabled in production deployment
	TracingEnabled bool `koanf:"tracing_enabled"`
}
