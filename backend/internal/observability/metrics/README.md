# OpenTelemetry Metrics Guide

This document explains how to add new metrics to the VibeXP backend API using OpenTelemetry.

## Overview

The VibeXP backend API uses OpenTelemetry for metrics collection. Metrics are exported to Google Cloud Managed Service for Prometheus via the OTel Collector sidecar running in Cloud Run.

### Current Metrics

| Metric Name | Type | Description | Attributes |
|-------------|------|-------------|------------|
| `vx_api_calls_total` | Counter | Total number of API calls | `http.method`, `http.path`, `http.status_code` |

## Architecture

```
┌─────────────────┐     ┌────────────────┐     ┌─────────────────────────────┐
│  Backend API    │────▶│ OTel Collector │────▶│ Google Cloud Managed        │
│  (Go SDK)       │     │  (Sidecar)     │     │ Service for Prometheus      │
└─────────────────┘     └────────────────┘     └─────────────────────────────┘
```

The OTel Collector sidecar is deployed alongside the backend API in Cloud Run. It receives metrics on `http://localhost:4317` and exports them to Google Cloud Monitoring.

## Adding a New Metric

### Step 1: Define the Metric in `metrics.go`

Add your metric to the `Metrics` struct in `internal/observability/metrics/metrics.go`:

```go
type Metrics struct {
    // Existing metrics
    APICallsTotal metric.Int64Counter

    // New metric (example: histogram for request duration)
    APIDuration metric.Float64Histogram
}
```

### Step 2: Register the Metric in `metrics.go`

Add one registration line to the matching `register*Instruments` function
(`registerCoreInstruments`, `registerContentAndMCPInstruments`, or
`registerEventBusAndNotificationInstruments`). The `registrar` collects the
first creation error — registration never panics, and on failure the app runs
metrics-less:

```go
func (m *Metrics) registerCoreInstruments(r *registrar) {
    // ... existing registrations ...
    m.APIDuration = r.float64Histogram(instrumentSpec{
        "vx_api_request_duration_seconds", "API request duration in seconds", "s",
    })
}
```

Then add the new instrument's `{name, description, unit, kind}` to
`expectedInstruments` in `metrics_test.go` — `TestNew_RegistersAllInstruments`
pins the full inventory because metric names are the dashboard contract with
Google Managed Prometheus.

### Step 3: Add a Recording Function

Add a function to record your metric with appropriate attributes:

```go
// RecordAPIDuration records the duration of an API call
func (m *Metrics) RecordAPIDuration(ctx context.Context, method, path string, duration float64) {
    if m == nil || m.APIDuration == nil {
        return
    }

    m.APIDuration.Record(
        ctx,
        duration,
        metric.WithAttributes(
            attribute.String("http.method", method),
            attribute.String("http.path", path),
        ),
    )
}
```

### Step 4: Use the Metric in Your Code

Here are two common patterns for using metrics:

#### Option A: Manual Recording in Handlers

```go
func (s *Server) handleCreatePrompt(w http.ResponseWriter, r *http.Request) {
    start := time.Now()

    // ... handler logic ...

    // Record duration after processing
    duration := time.Since(start).Seconds()
    s.metrics.RecordAPIDuration(r.Context(), r.Method, "/api/v1/prompts", duration)
}
```

#### Option B: Middleware (Recommended for Request-Level Metrics)

Create a new middleware in `middleware.go`:

```go
func DurationMiddleware(metrics *Metrics) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()

            // Wrap response writer
            wrapped := &responseWriter{ResponseWriter: w}

            // Call next handler
            next.ServeHTTP(wrapped, r)

            // Record duration
            duration := time.Since(start).Seconds()
            if metrics != nil && metrics.APIDuration != nil {
                metrics.RecordAPIDuration(r.Context(), r.Method, r.URL.Path, duration)
            }
        })
    }
}
```

Then add it to the server in `internal/server/server.go`:

```go
// Add after the metrics middleware
if appMetrics != nil {
    r.Use(metrics.DurationMiddleware(appMetrics))
}
```

## Metric Types

OpenTelemetry supports three main metric types:

### 1. Counter

**Use for:** Values that only increase (e.g., request count, errors, items processed)

```go
metric.Int64Counter("vx_errors_total", metric.WithDescription("Total errors"))
```

**Recording:** `counter.Add(ctx, 1, metric.WithAttributes(...))`

### 2. Gauge

**Use for:** Values that can go up or down (e.g., current connections, memory usage)

```go
metric.Int64UpDownCounter("vx_active_connections", metric.WithDescription("Active connections"))
```

**Recording:** `gauge.Add(ctx, 1, metric.WithAttributes(...))` or `gauge.Add(ctx, -1, ...)`

### 3. Histogram

**Use for:** Distributions of values (e.g., request duration, response size)

```go
metric.Float64Histogram("vx_request_duration_seconds", metric.WithDescription("Request duration"))
```

**Recording:** `histogram.Record(ctx, duration, metric.WithAttributes(...))`

## Best Practices

### Naming Conventions

- All VibeXP metrics MUST be prefixed with `vx_` for easy filtering and querying
- Use lowercase with underscores: `vx_api_calls_total`, `vx_request_duration_seconds`
- Include units in the name: `_seconds`, `_bytes`, `_total`
- Use suffixes to indicate type: `_total` for counters, `_seconds` for durations

### Attributes (Labels)

Attributes provide context for metrics. Common attributes:

| Attribute | Description | Example |
|-----------|-------------|---------|
| `http.method` | HTTP method | `GET`, `POST`, `PUT` |
| `http.path` | Request path pattern | `/api/v1/prompts/{slug}` |
| `http.status_code` | Response status | `200`, `404`, `500` |
| `user.id` | User identifier | `12345` |
| `subscription.tier` | Subscription tier | `basic`, `pro` |

**Warning:** Avoid high-cardinality attributes (like user IDs) in widely-used metrics, as they can create many time series.

### Description and Units

Always provide descriptions and units:

```go
meter.Int64Counter(
    "vx_api_calls_total",
    metric.WithDescription("Total number of API calls"),
    metric.WithUnit("1"),  // Use "1" for count-based metrics
)
```

## Viewing Metrics

### Google Cloud Monitoring

1. Go to **Google Cloud Console** → **Monitoring** → **Metrics Explorer**
2. Select your metric: `vx_api_calls_total`
3. Filter by attributes (e.g., `http.method = "GET"`)
4. Create graphs and dashboards

### Example Queries in Cloud Monitoring

```
# Total API calls by method
fetch vx_api_calls_total
| group_by [http.method]
| sum

# API calls by status code
fetch vx_api_calls_total
| group_by [http.status_code]
| sum

# Rate of API calls per minute
fetch vx_api_calls_total
| rate 1m
```

## Troubleshooting

### Metrics Not Appearing

1. **Check OTel Collector Logs:**
   ```bash
   gcloud logging read "resource.labels.container_name=\"otel-collector\"" \
     --project=shaharia-lab --limit=50
   ```

2. **Verify Cloud Run Configuration:**
   - Ensure `OTEL_EXPORTER_OTLP_ENDPOINT` is set to `http://localhost:4317`
   - Check that the OTel sidecar container is running

3. **Check Network:**
   - The collector listens on `localhost:4317` inside the container
   - Verify the OTel SDK is configured to export to the correct endpoint

### High Cardinality Warnings

If you see warnings about high cardinality:

- Reduce the number of unique attribute values
- Use path patterns instead of full URLs (e.g., `/api/v1/prompts/{slug}` instead of `/api/v1/prompts/my-prompt`)
- Avoid including IDs, timestamps, or other high-cardinality values in attributes

## Testing Locally

When developing locally, you can verify metrics are being created by:

1. Adding a debug log in the `RecordAPICall` function
2. Setting `OTEL_LOG_LEVEL=debug` environment variable
3. Using a local OTel collector for testing (requires Docker)

## References

- [OpenTelemetry Go SDK Documentation](https://opentelemetry.io/docs/instrumentation/go/)
- [Google Cloud Managed Service for Prometheus](https://cloud.google.com/stackdriver/docs/managed-prometheus)
- [OpenTelemetry Semantic Conventions](https://opentelemetry.io/docs/reference/specification/metrics/semantic_conventions/)
