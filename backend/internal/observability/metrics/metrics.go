package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/vibexp/vibexp/internal/config"
)

// Metrics holds all application metrics and the meter provider
type Metrics struct {
	// APICallsTotal is a counter that tracks the total number of API calls
	APICallsTotal metric.Float64Counter

	// APICallDuration is a histogram that tracks the duration of API calls
	APICallDuration metric.Float64Histogram

	// Business metrics - User events
	UserCreated         metric.Int64Counter
	UserLoginSuccessful metric.Int64Counter
	UserLoginFailed     metric.Int64Counter

	// Business metrics - Stripe webhook events
	StripeSubscriptionCreated metric.Int64Counter
	StripeSubscriptionUpdated metric.Int64Counter
	StripeSubscriptionDeleted metric.Int64Counter
	StripePaymentSucceeded    metric.Int64Counter
	StripePaymentFailed       metric.Int64Counter

	// Business metrics - API Key events
	APIKeyCreated metric.Int64Counter

	// Business metrics - AI Tools events
	AIToolsHooksCall metric.Int64Counter

	// Business metrics - Prompt events
	PromptCreated metric.Int64Counter
	PromptDeleted metric.Int64Counter

	// Business metrics - Artifact events
	ArtifactCreated metric.Int64Counter
	ArtifactDeleted metric.Int64Counter

	// Business metrics - Memory events
	MemoryCreated metric.Int64Counter
	MemoryDeleted metric.Int64Counter

	// Business metrics - Blueprint events
	BlueprintCreated metric.Int64Counter
	BlueprintDeleted metric.Int64Counter

	// Business metrics - MCP events
	MCPListTools       metric.Int64Counter
	MCPListPrompts     metric.Int64Counter
	MCPGetPrompt       metric.Int64Counter
	MCPSearchPrompts   metric.Int64Counter
	MCPListArtifacts   metric.Int64Counter
	MCPGetArtifact     metric.Int64Counter
	MCPCreateArtifact  metric.Int64Counter
	MCPUpdateArtifact  metric.Int64Counter
	MCPListMemories    metric.Int64Counter
	MCPGetMemory       metric.Int64Counter
	MCPCreateMemory    metric.Int64Counter
	MCPUpdateMemory    metric.Int64Counter
	MCPCreatePrompt    metric.Int64Counter
	MCPUpdatePrompt    metric.Int64Counter
	MCPCreateBlueprint metric.Int64Counter
	MCPUpdateBlueprint metric.Int64Counter
	MCPDateTime        metric.Int64Counter
	MCPGetUser         metric.Int64Counter

	// Event Bus Retry metrics
	EventBusRetryAttempts      metric.Int64Counter
	EventBusRetrySuccess       metric.Int64Counter
	EventBusRetryFailure       metric.Int64Counter
	EventBusRetryBackoff       metric.Float64Histogram
	EventBusEventDuration      metric.Float64Histogram
	EventBusCircuitBreakerOpen metric.Int64Counter

	// Notification metrics
	NotificationsSentTotal    metric.Int64Counter
	NotificationsDeliveryDur  metric.Float64Histogram
	NotificationsListenerErrs metric.Int64Counter

	// Digest runner metrics
	NotificationDigestEmailsSentTotal    metric.Int64Counter
	NotificationDigestRunnerDurationSecs metric.Float64Histogram
	// NotificationDigestQueueDepth is a point-in-time gauge of pending rows at job start.
	// A Gauge (not UpDownCounter) is used so each observation replaces the previous value
	// rather than accumulating — recording depth=5 twice does not produce 10.
	NotificationDigestQueueDepth metric.Int64Gauge

	// Deprecation tracking metrics
	DeprecatedEndpointCalls metric.Int64Counter

	// meterProvider is the OpenTelemetry meter provider for metrics export
	// It must be shut down gracefully to flush buffered metrics
	meterProvider *sdkmetric.MeterProvider

	// logger emits a durable structured log line for every business event,
	// alongside the OTel counter, so business metrics survive Cloud Run
	// scale-to-zero (OTel cumulative counters are lost on cold start).
	logger *slog.Logger
}

// Option is a functional option for configuring Metrics creation
type Option func(*metricsConfig)

type metricsConfig struct {
	readerProvider func(ctx context.Context) (sdkmetric.Reader, error)
	otlpEndpoint   string
	exportInterval time.Duration
	appConfig      *config.Config
	appLogger      *slog.Logger
}

// WithReaderProvider allows customizing the reader provider (used in tests)
func WithReaderProvider(provider func(ctx context.Context) (sdkmetric.Reader, error)) Option {
	return func(c *metricsConfig) {
		c.readerProvider = provider
	}
}

// WithOTelEndpoint sets the OTLP endpoint for metrics export
// For gRPC, use format "host:port" without scheme prefix.
func WithOTelEndpoint(endpoint string) Option {
	return func(c *metricsConfig) {
		c.otlpEndpoint = endpoint
	}
}

// WithExportInterval sets the metrics export interval
func WithExportInterval(interval time.Duration) Option {
	return func(c *metricsConfig) {
		c.exportInterval = interval
	}
}

// WithConfig sets the application config for metrics
func WithConfig(cfg *config.Config) Option {
	return func(c *metricsConfig) {
		c.appConfig = cfg
	}
}

// WithLogger sets the application logger used to emit a durable structured
// log line for every business event alongside the OTel counter.
func WithLogger(logger *slog.Logger) Option {
	return func(c *metricsConfig) {
		c.appLogger = logger
	}
}

// newMeterProvider creates and configures the OpenTelemetry meter provider
// with OTLP exporter and periodic reader.
func newMeterProvider(
	ctx context.Context,
	res *resource.Resource,
	readerProvider func(ctx context.Context) (sdkmetric.Reader, error),
	otlpEndpoint string,
	exportInterval time.Duration,
) (*sdkmetric.MeterProvider, error) {
	var reader sdkmetric.Reader
	var err error

	// Define explicit buckets for HTTP latency (in seconds)
	// Optimized for GCM and typical API latency ranges (5ms to 10s)
	latencyView := sdkmetric.NewView(
		sdkmetric.Instrument{
			Name: "vx_api_call_duration_seconds",
		},
		sdkmetric.Stream{
			Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
				Boundaries: []float64{
					0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
				},
			},
		},
	)

	if readerProvider != nil {
		// Use custom reader provider (e.g., for tests with ManualReader)
		reader, err = readerProvider(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create custom reader: %w", err)
		}
	} else {
		// Default: Create OTLP exporter for production
		// For gRPC, use format "host:port" without scheme prefix
		// Strip http:// or https:// if present for gRPC compatibility
		endpoint := otlpEndpoint
		if endpoint != "" {
			// Remove scheme prefix for gRPC (http:// or https://)
			endpoint = strings.TrimPrefix(endpoint, "http://")
			endpoint = strings.TrimPrefix(endpoint, "https://")
		}
		if endpoint == "" {
			endpoint = "localhost:4317"
		}

		// Create OTLP metrics exporter with gRPC client
		// This connects to the OTel Collector sidecar
		exporter, err := otlpmetricgrpc.New(ctx,
			otlpmetricgrpc.WithEndpoint(endpoint),
			otlpmetricgrpc.WithInsecure(), // Local connection, no TLS needed
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP metric exporter for endpoint %s: %w", endpoint, err)
		}

		// Use configured export interval (default 60s)
		if exportInterval == 0 {
			exportInterval = 60 * time.Second
		}

		// Create periodic reader with configured export interval
		// The reader periodically exports metrics to the OTLP collector
		reader = sdkmetric.NewPeriodicReader(
			exporter,
			sdkmetric.WithInterval(exportInterval),
		)
	}

	// Create meter provider with the reader and views
	return sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
		sdkmetric.WithView(latencyView),
	), nil
}

// New initializes and returns a new Metrics instance with a production-ready meter provider
//
// The meter provider is configured with:
// - Service identification attributes (name, version)
// - Deployment environment (from config)
// - Automatic resource detection from OTEL_* environment variables
//
// The serviceVersion parameter should be the actual version of the application,
// typically from git tag, build metadata, or configuration.
//
// Options can be passed to customize behavior:
// - WithOTelEndpoint(endpoint): Set OTLP endpoint (default: localhost:4317)
// - WithExportInterval(interval): Set export interval (default: 60s)
// - WithReaderProvider(provider): Use custom reader (for tests)
// - WithConfig(cfg): Use application config for deployment environment
func New(serviceVersion string, opts ...Option) (*Metrics, error) {
	ctx := context.Background()

	cfg := &metricsConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	res, err := createResource(ctx, serviceVersion, cfg.appConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	meterProvider, err := newMeterProvider(ctx, res, cfg.readerProvider, cfg.otlpEndpoint, cfg.exportInterval)
	if err != nil {
		return nil, err
	}

	meter := meterProvider.Meter("github.com/vibexp/vibexp")
	m := &Metrics{
		meterProvider: meterProvider,
	}
	m.logger = cfg.appLogger

	if err := m.initializeMetrics(meter); err != nil {
		if shutdownErr := meterProvider.Shutdown(ctx); shutdownErr != nil {
			return nil, fmt.Errorf(
				"failed to initialize metrics: %w, and failed to shutdown meter provider: %v",
				err, shutdownErr,
			)
		}
		return nil, err
	}

	return m, nil
}

func createResource(ctx context.Context, serviceVersion string, cfg *config.Config) (*resource.Resource, error) {
	env := "production"
	if cfg != nil {
		env = cfg.GetDeploymentEnvironment()
	}
	return resource.New(
		ctx,
		resource.WithFromEnv(),
		resource.WithAttributes(
			semconv.ServiceName("vibexp-api"),
			semconv.ServiceVersion(serviceVersion),
			semconv.DeploymentEnvironment(env),
		),
		resource.WithTelemetrySDK(),
	)
}

// instrumentSpec describes the registration identity of one instrument: the
// exact name, description, and unit emitted to the collector.
type instrumentSpec struct {
	name string
	desc string
	unit string
}

// registrar creates instruments against a single meter while capturing the
// first creation error, so registration reads as a flat assignment list.
// Errors are collected, never panicked on: ProvideMetrics logs the returned
// error and runs the application metrics-less.
type registrar struct {
	meter metric.Meter
	err   error
}

func (r *registrar) capture(name string, err error) {
	if err != nil && r.err == nil {
		r.err = fmt.Errorf("create instrument %s: %w", name, err)
	}
}

func (r *registrar) int64Counter(s instrumentSpec) metric.Int64Counter {
	c, err := r.meter.Int64Counter(s.name, metric.WithDescription(s.desc), metric.WithUnit(s.unit))
	r.capture(s.name, err)
	return c
}

func (r *registrar) float64Counter(s instrumentSpec) metric.Float64Counter {
	c, err := r.meter.Float64Counter(s.name, metric.WithDescription(s.desc), metric.WithUnit(s.unit))
	r.capture(s.name, err)
	return c
}

func (r *registrar) float64Histogram(s instrumentSpec) metric.Float64Histogram {
	h, err := r.meter.Float64Histogram(s.name, metric.WithDescription(s.desc), metric.WithUnit(s.unit))
	r.capture(s.name, err)
	return h
}

func (r *registrar) int64Gauge(s instrumentSpec) metric.Int64Gauge {
	g, err := r.meter.Int64Gauge(s.name, metric.WithDescription(s.desc), metric.WithUnit(s.unit))
	r.capture(s.name, err)
	return g
}

// initializeMetrics registers every instrument. The full inventory — names,
// descriptions, units, and instrument kinds — is the dashboard contract with
// Google Managed Prometheus and is pinned by TestNew_RegistersAllInstruments.
func (m *Metrics) initializeMetrics(meter metric.Meter) error {
	r := &registrar{meter: meter}
	m.registerCoreInstruments(r)
	m.registerContentAndMCPInstruments(r)
	m.registerEventBusAndNotificationInstruments(r)
	return r.err
}

// registerCoreInstruments registers API, user, Stripe, API key, AI tools, and
// deprecation instruments.
func (m *Metrics) registerCoreInstruments(r *registrar) {
	m.APICallsTotal = r.float64Counter(instrumentSpec{"vx_api_calls_total", "Total number of API calls", "1"})
	m.APICallDuration = r.float64Histogram(instrumentSpec{"vx_api_call_duration_seconds", "Duration of API calls", "s"})
	m.UserCreated = r.int64Counter(instrumentSpec{"vx_user_created", "Total number of users created", "1"})
	m.UserLoginSuccessful = r.int64Counter(instrumentSpec{
		"vx_user_login_successful", "Total number of successful user logins", "1",
	})
	m.UserLoginFailed = r.int64Counter(instrumentSpec{
		"vx_user_login_failed", "Total number of failed user login attempts", "1",
	})
	m.StripeSubscriptionCreated = r.int64Counter(instrumentSpec{
		"vx_stripe_subscription_created", "Total number of Stripe subscription.created webhooks received", "1",
	})
	m.StripeSubscriptionUpdated = r.int64Counter(instrumentSpec{
		"vx_stripe_subscription_updated", "Total number of Stripe subscription.updated webhooks received", "1",
	})
	m.StripeSubscriptionDeleted = r.int64Counter(instrumentSpec{
		"vx_stripe_subscription_deleted", "Total number of Stripe subscription.deleted webhooks received", "1",
	})
	m.StripePaymentSucceeded = r.int64Counter(instrumentSpec{
		"vx_stripe_payment_succeeded", "Total number of Stripe invoice.payment_succeeded webhooks received", "1",
	})
	m.StripePaymentFailed = r.int64Counter(instrumentSpec{
		"vx_stripe_payment_failed", "Total number of Stripe invoice.payment_failed webhooks received", "1",
	})
	m.APIKeyCreated = r.int64Counter(instrumentSpec{"vx_api_key_created", "Total number of API keys created", "1"})
	m.AIToolsHooksCall = r.int64Counter(instrumentSpec{
		"vx_ai_tools_hooks_call", "Total number of AI tools hooks calls by tool name", "1",
	})
	m.DeprecatedEndpointCalls = r.int64Counter(instrumentSpec{
		"vx_deprecated_endpoint_calls_total", "Total number of calls to deprecated API endpoints", "1",
	})
}

// registerContentAndMCPInstruments registers prompt, artifact, memory,
// blueprint, and MCP instruments.
func (m *Metrics) registerContentAndMCPInstruments(r *registrar) {
	m.PromptCreated = r.int64Counter(instrumentSpec{"vx_prompt_created", "Total number of prompts created", "1"})
	m.PromptDeleted = r.int64Counter(instrumentSpec{"vx_prompt_deleted", "Total number of prompts deleted", "1"})
	m.ArtifactCreated = r.int64Counter(instrumentSpec{"vx_artifact_created", "Total number of artifacts created", "1"})
	m.ArtifactDeleted = r.int64Counter(instrumentSpec{"vx_artifact_deleted", "Total number of artifacts deleted", "1"})
	m.MemoryCreated = r.int64Counter(instrumentSpec{"vx_memory_created", "Total number of memories created", "1"})
	m.MemoryDeleted = r.int64Counter(instrumentSpec{"vx_memory_deleted", "Total number of memories deleted", "1"})
	m.BlueprintCreated = r.int64Counter(instrumentSpec{
		"vx_blueprint_created", "Total number of blueprints created", "1",
	})
	m.BlueprintDeleted = r.int64Counter(instrumentSpec{
		"vx_blueprint_deleted", "Total number of blueprints deleted", "1",
	})
	m.MCPListTools = r.int64Counter(instrumentSpec{"vx_mcp_list_tools", "Total number of MCP list tools calls", "1"})
	m.MCPListPrompts = r.int64Counter(instrumentSpec{
		"vx_mcp_list_prompts", "Total number of MCP list prompts calls", "1",
	})
	m.MCPGetPrompt = r.int64Counter(instrumentSpec{"vx_mcp_get_prompt", "Total number of MCP get prompt calls", "1"})
	m.MCPSearchPrompts = r.int64Counter(instrumentSpec{
		"vx_mcp_search_prompts", "Total number of MCP search prompts calls", "1",
	})
	m.MCPListArtifacts = r.int64Counter(instrumentSpec{
		"vx_mcp_list_artifacts", "Total number of MCP list artifacts calls", "1",
	})
	m.MCPGetArtifact = r.int64Counter(instrumentSpec{
		"vx_mcp_get_artifact", "Total number of MCP get artifact calls", "1",
	})
	m.MCPCreateArtifact = r.int64Counter(instrumentSpec{
		"vx_mcp_create_artifact", "Total number of MCP create artifact calls", "1",
	})
	m.MCPUpdateArtifact = r.int64Counter(instrumentSpec{
		"vx_mcp_update_artifact", "Total number of MCP update artifact calls", "1",
	})
	m.MCPListMemories = r.int64Counter(instrumentSpec{
		"vx_mcp_list_memories", "Total number of MCP list memories calls", "1",
	})
	m.MCPGetMemory = r.int64Counter(instrumentSpec{"vx_mcp_get_memory", "Total number of MCP get memory calls", "1"})
	m.MCPCreateMemory = r.int64Counter(instrumentSpec{
		"vx_mcp_create_memory", "Total number of MCP create memory calls", "1",
	})
	m.MCPUpdateMemory = r.int64Counter(instrumentSpec{
		"vx_mcp_update_memory", "Total number of MCP update memory calls", "1",
	})
	m.MCPCreatePrompt = r.int64Counter(instrumentSpec{
		"vx_mcp_create_prompt", "Total number of MCP create prompt calls", "1",
	})
	m.MCPUpdatePrompt = r.int64Counter(instrumentSpec{
		"vx_mcp_update_prompt", "Total number of MCP update prompt calls", "1",
	})
	m.MCPCreateBlueprint = r.int64Counter(instrumentSpec{
		"vx_mcp_create_blueprint", "Total number of MCP create blueprint calls", "1",
	})
	m.MCPUpdateBlueprint = r.int64Counter(instrumentSpec{
		"vx_mcp_update_blueprint", "Total number of MCP update blueprint calls", "1",
	})
	m.MCPDateTime = r.int64Counter(instrumentSpec{"vx_mcp_datetime", "Total number of MCP datetime calls", "1"})
	m.MCPGetUser = r.int64Counter(instrumentSpec{"vx_mcp_get_user", "Total number of MCP get user calls", "1"})
}

// registerEventBusAndNotificationInstruments registers event-bus retry,
// notification, and digest-runner instruments.
func (m *Metrics) registerEventBusAndNotificationInstruments(r *registrar) {
	m.EventBusRetryAttempts = r.int64Counter(instrumentSpec{
		"vx_event_bus_retry_attempts_total", "Total number of retry attempts by listener type and event type", "1",
	})
	m.EventBusRetrySuccess = r.int64Counter(instrumentSpec{
		"vx_event_bus_retry_success_total", "Total number of successful retries by listener type and event type", "1",
	})
	m.EventBusRetryFailure = r.int64Counter(instrumentSpec{
		"vx_event_bus_retry_failure_total",
		"Total number of failed retries (after all attempts) by listener type and event type", "1",
	})
	m.EventBusRetryBackoff = r.float64Histogram(instrumentSpec{
		"vx_event_bus_retry_backoff_seconds", "Retry backoff duration in seconds by listener type and event type", "s",
	})
	m.EventBusEventDuration = r.float64Histogram(instrumentSpec{
		"vx_event_bus_event_duration_seconds",
		"Event processing duration in seconds by listener type, event type, and success status", "s",
	})
	m.EventBusCircuitBreakerOpen = r.int64Counter(instrumentSpec{
		"vx_event_bus_circuit_breaker_open_total",
		"Total number of times circuit breaker opened by listener type", "1",
	})
	m.NotificationsSentTotal = r.int64Counter(instrumentSpec{
		"vx_notifications_sent_total", "Total number of notifications sent by channel and status", "1",
	})
	m.NotificationsDeliveryDur = r.float64Histogram(instrumentSpec{
		"vx_notifications_delivery_duration_seconds", "Duration of notification delivery by channel", "s",
	})
	m.NotificationsListenerErrs = r.int64Counter(instrumentSpec{
		"vx_notifications_listener_errors_total",
		"Total number of notification event listener errors by event type", "1",
	})
	m.NotificationDigestEmailsSentTotal = r.int64Counter(instrumentSpec{
		"vx_notifications_digest_sent_total",
		"Total number of digest emails attempted by status (sent|failed|skipped)", "1",
	})
	m.NotificationDigestRunnerDurationSecs = r.float64Histogram(instrumentSpec{
		"vx_notifications_digest_runner_duration_seconds",
		"Total wall-clock duration of a digest runner execution", "s",
	})
	m.NotificationDigestQueueDepth = r.int64Gauge(instrumentSpec{
		"vx_notifications_digest_queue_depth",
		"Point-in-time count of pending rows in notification_digest_queue at job start", "1",
	})
}

// Shutdown flushes any buffered metrics and closes the meter provider
// This should be called during application graceful shutdown to ensure
// all pending metrics are exported before the application exits.
//
// The context can be used to set a timeout for the shutdown operation.
// If the context is canceled, shutdown will return immediately.
func (m *Metrics) Shutdown(ctx context.Context) error {
	if m == nil || m.meterProvider == nil {
		return nil
	}
	return m.meterProvider.Shutdown(ctx)
}

// RecordAPICall increments the API calls counter with the specified attributes
// This should be called for each API request to track usage
//
// Input validation is performed to prevent empty or malformed metric attributes:
// - Empty method defaults to "UNKNOWN"
// - Empty path defaults to "/unknown"
// - Empty statusCode defaults to "0"
//
// Both the counter and the latency histogram carry the bounded chi route pattern
// under the http.route attribute (in addition to http.path) so latency can be
// sliced per route.
//
// When isStreaming is true the request is still counted in the counter but is
// excluded from the latency histogram. Long-lived streaming responses (e.g. MCP
// Streamable HTTP / SSE) record their full connection lifetime as duration, which
// would otherwise peg the histogram's top bucket and distort p95 latency.
func (m *Metrics) RecordAPICall(ctx context.Context, method, route, statusCode string,
	duration time.Duration, isStreaming bool) {
	if m == nil || m.APICallsTotal == nil {
		return
	}

	// Validate and sanitize inputs to prevent invalid metrics
	if method == "" {
		method = "UNKNOWN"
	}
	if route == "" {
		route = "/unknown"
	}
	if statusCode == "" {
		statusCode = "0"
	}

	attrs := metric.WithAttributes(
		attribute.String(AttrHTTPMethod, method),
		attribute.String(AttrHTTPPath, route),
		attribute.String(AttrHTTPRoute, route),
		attribute.String(AttrHTTPStatus, statusCode),
	)

	m.APICallsTotal.Add(
		ctx,
		1.0,
		attrs,
	)

	// Exclude long-lived streaming responses from the latency histogram so p95
	// reflects real request/response timing, not connection lifetime.
	if m.APICallDuration != nil && !isStreaming {
		m.APICallDuration.Record(
			ctx,
			duration.Seconds(),
			attrs,
		)
	}
}

// Business event names and categories emitted as durable structured log lines.
// These values are stable: Terraform log-based-metric filters match them
// literally, so changing a string here breaks the corresponding metric.
const (
	eventCategoryUser    = "user"
	eventCategoryBilling = "billing"
	eventCategoryAPIKey  = "apikey"
	eventCategoryAITools = "ai_tools"
	eventCategoryContent = "content"
	eventCategoryMCP     = "mcp"
)

// emitEvent writes a single durable structured INFO log line for a business
// event. It is nil-safe so callers that construct Metrics without a logger
// (e.g. tests) never panic. The CloudFormatter flattens every field to the
// top level of the JSON payload (event -> jsonPayload.event), so no extra
// mapping is required for Cloud Logging log-based metrics.
func (m *Metrics) emitEvent(ctx context.Context, event, category string, fields []any) {
	if m == nil || m.logger == nil {
		return
	}
	logger := m.logger.With(
		"event", event,
		"event_category", category,
	)
	if len(fields) > 0 {
		logger = logger.With(fields...)
	}
	logger.InfoContext(ctx, "business event")
}

// RecordUserCreated increments the user created counter
func (m *Metrics) RecordUserCreated(ctx context.Context) {
	if m == nil || m.UserCreated == nil {
		return
	}
	m.UserCreated.Add(ctx, 1)
	m.emitEvent(ctx, "user.created", eventCategoryUser, nil)
}

// RecordUserLoginSuccessful increments the successful login counter
func (m *Metrics) RecordUserLoginSuccessful(ctx context.Context) {
	if m == nil || m.UserLoginSuccessful == nil {
		return
	}
	m.UserLoginSuccessful.Add(ctx, 1)
	m.emitEvent(ctx, "user.login.succeeded", eventCategoryUser, nil)
}

// RecordUserLoginFailed increments the failed login counter with optional attributes
func (m *Metrics) RecordUserLoginFailed(ctx context.Context, reason string) {
	if m == nil || m.UserLoginFailed == nil {
		return
	}
	attrs := metric.WithAttributes()
	var fields []any
	if reason != "" {
		attrs = metric.WithAttributes(attribute.String("reason", reason))
		fields = []any{"reason", reason}
	}
	m.UserLoginFailed.Add(ctx, 1, attrs)
	m.emitEvent(ctx, "user.login.failed", eventCategoryUser, fields)
}

// RecordStripeWebhook increments the appropriate Stripe webhook counter based on event type
func (m *Metrics) RecordStripeWebhook(ctx context.Context, eventType string) {
	if m == nil {
		return
	}

	stripeFields := []any{"stripe_event_type", eventType}
	switch eventType {
	case "customer.subscription.created":
		if m.StripeSubscriptionCreated != nil {
			m.StripeSubscriptionCreated.Add(ctx, 1)
		}
		m.emitEvent(ctx, "stripe.subscription.created", eventCategoryBilling, stripeFields)
	case "customer.subscription.updated":
		if m.StripeSubscriptionUpdated != nil {
			m.StripeSubscriptionUpdated.Add(ctx, 1)
		}
		m.emitEvent(ctx, "stripe.subscription.updated", eventCategoryBilling, stripeFields)
	case "customer.subscription.deleted":
		if m.StripeSubscriptionDeleted != nil {
			m.StripeSubscriptionDeleted.Add(ctx, 1)
		}
		m.emitEvent(ctx, "stripe.subscription.deleted", eventCategoryBilling, stripeFields)
	case "invoice.payment_succeeded":
		if m.StripePaymentSucceeded != nil {
			m.StripePaymentSucceeded.Add(ctx, 1)
		}
		m.emitEvent(ctx, "stripe.payment.succeeded", eventCategoryBilling, stripeFields)
	case "invoice.payment_failed":
		if m.StripePaymentFailed != nil {
			m.StripePaymentFailed.Add(ctx, 1)
		}
		m.emitEvent(ctx, "stripe.payment.failed", eventCategoryBilling, stripeFields)
	}
}

// RecordAPIKeyCreated increments the API key created counter
func (m *Metrics) RecordAPIKeyCreated(ctx context.Context) {
	if m == nil || m.APIKeyCreated == nil {
		return
	}
	m.APIKeyCreated.Add(ctx, 1)
	m.emitEvent(ctx, "apikey.created", eventCategoryAPIKey, nil)
}

// RecordAIToolsHooksCall increments the AI tools hooks call counter with tool name attribute
func (m *Metrics) RecordAIToolsHooksCall(ctx context.Context, toolName string) {
	if m == nil || m.AIToolsHooksCall == nil {
		return
	}
	attrs := metric.WithAttributes()
	var fields []any
	if toolName != "" {
		attrs = metric.WithAttributes(attribute.String("tool_name", toolName))
		fields = []any{"tool_name", toolName}
	}
	m.AIToolsHooksCall.Add(ctx, 1, attrs)
	m.emitEvent(ctx, "ai_tools.hooks.call", eventCategoryAITools, fields)
}

// RecordPromptCreated increments the prompt created counter
func (m *Metrics) RecordPromptCreated(ctx context.Context) {
	if m == nil || m.PromptCreated == nil {
		return
	}
	m.PromptCreated.Add(ctx, 1)
	m.emitEvent(ctx, "prompt.created", eventCategoryContent, nil)
}

// RecordPromptDeleted increments the prompt deleted counter
func (m *Metrics) RecordPromptDeleted(ctx context.Context) {
	if m == nil || m.PromptDeleted == nil {
		return
	}
	m.PromptDeleted.Add(ctx, 1)
	m.emitEvent(ctx, "prompt.deleted", eventCategoryContent, nil)
}

// RecordArtifactCreated increments the artifact created counter
func (m *Metrics) RecordArtifactCreated(ctx context.Context) {
	if m == nil || m.ArtifactCreated == nil {
		return
	}
	m.ArtifactCreated.Add(ctx, 1)
	m.emitEvent(ctx, "artifact.created", eventCategoryContent, nil)
}

// RecordArtifactDeleted increments the artifact deleted counter
func (m *Metrics) RecordArtifactDeleted(ctx context.Context) {
	if m == nil || m.ArtifactDeleted == nil {
		return
	}
	m.ArtifactDeleted.Add(ctx, 1)
	m.emitEvent(ctx, "artifact.deleted", eventCategoryContent, nil)
}

// RecordMemoryCreated increments the memory created counter
func (m *Metrics) RecordMemoryCreated(ctx context.Context) {
	if m == nil || m.MemoryCreated == nil {
		return
	}
	m.MemoryCreated.Add(ctx, 1)
	m.emitEvent(ctx, "memory.created", eventCategoryContent, nil)
}

// RecordMemoryDeleted increments the memory deleted counter
func (m *Metrics) RecordMemoryDeleted(ctx context.Context) {
	if m == nil || m.MemoryDeleted == nil {
		return
	}
	m.MemoryDeleted.Add(ctx, 1)
	m.emitEvent(ctx, "memory.deleted", eventCategoryContent, nil)
}

// RecordBlueprintCreated increments the blueprint created counter
func (m *Metrics) RecordBlueprintCreated(ctx context.Context) {
	if m == nil || m.BlueprintCreated == nil {
		return
	}
	m.BlueprintCreated.Add(ctx, 1)
	m.emitEvent(ctx, "blueprint.created", eventCategoryContent, nil)
}

// RecordBlueprintDeleted increments the blueprint deleted counter
func (m *Metrics) RecordBlueprintDeleted(ctx context.Context) {
	if m == nil || m.BlueprintDeleted == nil {
		return
	}
	m.BlueprintDeleted.Add(ctx, 1)
	m.emitEvent(ctx, "blueprint.deleted", eventCategoryContent, nil)
}

// RecordMCPListTools increments the MCP list tools counter
func (m *Metrics) RecordMCPListTools(ctx context.Context) {
	if m == nil || m.MCPListTools == nil {
		return
	}
	m.MCPListTools.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.list_tools", eventCategoryMCP, nil)
}

// RecordMCPListPrompts increments the MCP list prompts counter
func (m *Metrics) RecordMCPListPrompts(ctx context.Context) {
	if m == nil || m.MCPListPrompts == nil {
		return
	}
	m.MCPListPrompts.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.list_prompts", eventCategoryMCP, nil)
}

// RecordMCPGetPrompt increments the MCP get prompt counter
func (m *Metrics) RecordMCPGetPrompt(ctx context.Context) {
	if m == nil || m.MCPGetPrompt == nil {
		return
	}
	m.MCPGetPrompt.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.get_prompt", eventCategoryMCP, nil)
}

// RecordMCPSearchPrompts increments the MCP search prompts counter
func (m *Metrics) RecordMCPSearchPrompts(ctx context.Context) {
	if m == nil || m.MCPSearchPrompts == nil {
		return
	}
	m.MCPSearchPrompts.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.search_prompts", eventCategoryMCP, nil)
}

// RecordMCPListArtifacts increments the MCP list artifacts counter
func (m *Metrics) RecordMCPListArtifacts(ctx context.Context) {
	if m == nil || m.MCPListArtifacts == nil {
		return
	}
	m.MCPListArtifacts.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.list_artifacts", eventCategoryMCP, nil)
}

// RecordMCPGetArtifact increments the MCP get artifact counter
func (m *Metrics) RecordMCPGetArtifact(ctx context.Context) {
	if m == nil || m.MCPGetArtifact == nil {
		return
	}
	m.MCPGetArtifact.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.get_artifact", eventCategoryMCP, nil)
}

// RecordMCPCreateArtifact increments the MCP create artifact counter
func (m *Metrics) RecordMCPCreateArtifact(ctx context.Context) {
	if m == nil || m.MCPCreateArtifact == nil {
		return
	}
	m.MCPCreateArtifact.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.create_artifact", eventCategoryMCP, nil)
}

// RecordMCPUpdateArtifact increments the MCP update artifact counter
func (m *Metrics) RecordMCPUpdateArtifact(ctx context.Context) {
	if m == nil || m.MCPUpdateArtifact == nil {
		return
	}
	m.MCPUpdateArtifact.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.update_artifact", eventCategoryMCP, nil)
}

// RecordMCPListMemories increments the MCP list memories counter
func (m *Metrics) RecordMCPListMemories(ctx context.Context) {
	if m == nil || m.MCPListMemories == nil {
		return
	}
	m.MCPListMemories.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.list_memories", eventCategoryMCP, nil)
}

// RecordMCPGetMemory increments the MCP get memory counter
func (m *Metrics) RecordMCPGetMemory(ctx context.Context) {
	if m == nil || m.MCPGetMemory == nil {
		return
	}
	m.MCPGetMemory.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.get_memory", eventCategoryMCP, nil)
}

// RecordMCPCreateMemory increments the MCP create memory counter
func (m *Metrics) RecordMCPCreateMemory(ctx context.Context) {
	if m == nil || m.MCPCreateMemory == nil {
		return
	}
	m.MCPCreateMemory.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.create_memory", eventCategoryMCP, nil)
}

// RecordMCPUpdateMemory increments the MCP update memory counter
func (m *Metrics) RecordMCPUpdateMemory(ctx context.Context) {
	if m == nil || m.MCPUpdateMemory == nil {
		return
	}
	m.MCPUpdateMemory.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.update_memory", eventCategoryMCP, nil)
}

// RecordMCPCreatePrompt increments the MCP create prompt counter
func (m *Metrics) RecordMCPCreatePrompt(ctx context.Context) {
	if m == nil || m.MCPCreatePrompt == nil {
		return
	}
	m.MCPCreatePrompt.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.create_prompt", eventCategoryMCP, nil)
}

// RecordMCPUpdatePrompt increments the MCP update prompt counter
func (m *Metrics) RecordMCPUpdatePrompt(ctx context.Context) {
	if m == nil || m.MCPUpdatePrompt == nil {
		return
	}
	m.MCPUpdatePrompt.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.update_prompt", eventCategoryMCP, nil)
}

// RecordMCPCreateBlueprint increments the MCP create blueprint counter
func (m *Metrics) RecordMCPCreateBlueprint(ctx context.Context) {
	if m == nil || m.MCPCreateBlueprint == nil {
		return
	}
	m.MCPCreateBlueprint.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.create_blueprint", eventCategoryMCP, nil)
}

// RecordMCPUpdateBlueprint increments the MCP update blueprint counter
func (m *Metrics) RecordMCPUpdateBlueprint(ctx context.Context) {
	if m == nil || m.MCPUpdateBlueprint == nil {
		return
	}
	m.MCPUpdateBlueprint.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.update_blueprint", eventCategoryMCP, nil)
}

// RecordMCPDateTime increments the MCP datetime counter
func (m *Metrics) RecordMCPDateTime(ctx context.Context) {
	if m == nil || m.MCPDateTime == nil {
		return
	}
	m.MCPDateTime.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.datetime", eventCategoryMCP, nil)
}

// RecordMCPGetUser increments the MCP get user counter
func (m *Metrics) RecordMCPGetUser(ctx context.Context) {
	if m == nil || m.MCPGetUser == nil {
		return
	}
	m.MCPGetUser.Add(ctx, 1)
	m.emitEvent(ctx, "mcp.get_user", eventCategoryMCP, nil)
}

// RecordEventBusRetryAttempt increments the retry attempts counter with listener and event type attributes
func (m *Metrics) RecordEventBusRetryAttempt(ctx context.Context, listenerType, eventType string, attemptNum int) {
	if m == nil || m.EventBusRetryAttempts == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String("listener_type", listenerType),
		attribute.String("event_type", eventType),
		attribute.Int("attempt", attemptNum),
	)
	m.EventBusRetryAttempts.Add(ctx, 1, attrs)
}

// RecordEventBusRetrySuccess increments the retry success counter with listener and event type attributes
func (m *Metrics) RecordEventBusRetrySuccess(ctx context.Context, listenerType, eventType string, attemptNum int) {
	if m == nil || m.EventBusRetrySuccess == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String("listener_type", listenerType),
		attribute.String("event_type", eventType),
		attribute.Int("retry_count", attemptNum),
	)
	m.EventBusRetrySuccess.Add(ctx, 1, attrs)
}

// RecordEventBusRetryFailure increments the retry failure counter with listener and event type attributes
func (m *Metrics) RecordEventBusRetryFailure(ctx context.Context, listenerType, eventType string) {
	if m == nil || m.EventBusRetryFailure == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String("listener_type", listenerType),
		attribute.String("event_type", eventType),
	)
	m.EventBusRetryFailure.Add(ctx, 1, attrs)
}

// RecordEventBusRetryBackoff records the backoff duration histogram
func (m *Metrics) RecordEventBusRetryBackoff(
	ctx context.Context,
	listenerType, eventType string,
	backoffDuration time.Duration,
) {
	if m == nil || m.EventBusRetryBackoff == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String("listener_type", listenerType),
		attribute.String("event_type", eventType),
	)
	m.EventBusRetryBackoff.Record(ctx, backoffDuration.Seconds(), attrs)
}

// RecordEventBusEventDuration records the event processing duration
func (m *Metrics) RecordEventBusEventDuration(
	ctx context.Context,
	listenerType, eventType string,
	duration time.Duration,
	success bool,
) {
	if m == nil || m.EventBusEventDuration == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String("listener_type", listenerType),
		attribute.String("event_type", eventType),
		attribute.Bool("success", success),
	)
	m.EventBusEventDuration.Record(ctx, duration.Seconds(), attrs)
}

// RecordEventBusCircuitBreakerOpen increments the circuit breaker open counter with listener type attribute
func (m *Metrics) RecordEventBusCircuitBreakerOpen(ctx context.Context, listenerType string) {
	if m == nil || m.EventBusCircuitBreakerOpen == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String("listener_type", listenerType),
	)
	m.EventBusCircuitBreakerOpen.Add(ctx, 1, attrs)
}

// RecordDigestEmailSent increments the digest email sent counter with the given status label.
// status should be one of: "sent", "failed", "skipped".
func (m *Metrics) RecordDigestEmailSent(ctx context.Context, status string) {
	if m == nil || m.NotificationDigestEmailsSentTotal == nil {
		return
	}
	m.NotificationDigestEmailsSentTotal.Add(ctx, 1,
		metric.WithAttributes(attribute.String("status", status)),
	)
}

// RecordDigestRunnerDuration records the wall-clock duration of a digest runner run.
func (m *Metrics) RecordDigestRunnerDuration(ctx context.Context, d time.Duration) {
	if m == nil || m.NotificationDigestRunnerDurationSecs == nil {
		return
	}
	m.NotificationDigestRunnerDurationSecs.Record(ctx, d.Seconds())
}

// RecordDigestQueueDepth records a point-in-time gauge observation of the pending queue depth.
// Using Record (gauge semantics) ensures each observation replaces the previous value rather
// than summing — suitable for a snapshot metric sampled once per digest run.
func (m *Metrics) RecordDigestQueueDepth(ctx context.Context, depth int) {
	if m == nil || m.NotificationDigestQueueDepth == nil {
		return
	}
	m.NotificationDigestQueueDepth.Record(ctx, int64(depth))
}

// RecordDeprecatedEndpointCall increments the deprecated endpoint calls counter
// with the endpoint path attribute for tracking usage of deprecated API endpoints
func (m *Metrics) RecordDeprecatedEndpointCall(ctx context.Context, endpoint string) {
	if m == nil || m.DeprecatedEndpointCalls == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String("endpoint", endpoint),
	)
	m.DeprecatedEndpointCalls.Add(ctx, 1, attrs)
}

// Common attribute keys for metrics
const (
	AttrHTTPMethod     = "http.method"
	AttrHTTPPath       = "http.path"
	AttrHTTPStatus     = "http.status_code"
	AttrHTTPRoute      = "http.route"
	AttrUserID         = "user.id"
	AttrSubscription   = "subscription.tier"
	AttrServiceName    = "service.name"
	AttrServiceVersion = "service.version"
)
