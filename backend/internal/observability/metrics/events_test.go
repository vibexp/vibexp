package metrics

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/vibexp/vibexp/internal/logging/logtest"
)

// newTestMetricsWithLogger creates a Metrics instance backed by a ManualReader
// and a recording test logger with a hook, so a test can assert on both the
// emitted business-event log lines and the OTel counter samples.
func newTestMetricsWithLogger(t *testing.T) (*Metrics, sdkmetric.Reader, *logtest.Recorder) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	logger, hook := logtest.New()
	m, err := New(
		"test-version",
		WithReaderProvider(func(_ context.Context) (sdkmetric.Reader, error) {
			return reader, nil
		}),
		WithLogger(logger),
	)
	require.NoError(t, err)
	require.NotNil(t, m)
	return m, reader, hook
}

// counterValueByName returns the summed value of the named Int64 counter across
// all data points (0 if absent).
func counterValueByName(t *testing.T, rm *metricdata.ResourceMetrics, name string) int64 {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "metric %s should be Sum[int64]", name)
			var total int64
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
			return total
		}
	}
	return 0
}

func TestEmitEvent_NilSafe(t *testing.T) {
	// A Metrics built without a logger must not panic when a Record* method fires.
	m := newTestMetrics(t)
	require.Nil(t, m.logger)
	assert.NotPanics(t, func() {
		m.RecordUserCreated(context.Background())
	})

	// A nil *Metrics must also be safe.
	var nilMetrics *Metrics
	assert.NotPanics(t, func() {
		nilMetrics.RecordUserCreated(context.Background())
	})
}

func TestRecordUserCreated_EmitsEventAndCounter(t *testing.T) {
	m, reader, hook := newTestMetricsWithLogger(t)
	hook.Reset()

	m.RecordUserCreated(context.Background())

	require.Len(t, hook.AllEntries(), 1)
	entry := hook.LastEntry()
	assert.Equal(t, "user.created", entry.Data["event"])
	assert.Equal(t, eventCategoryUser, entry.Data["event_category"])
	assert.Equal(t, slog.LevelInfo, entry.Level)
	assert.Equal(t, "business event", entry.Message)

	rm := scrapeMetrics(t, reader)
	assert.Equal(t, int64(1), counterValueByName(t, rm, "vx_user_created"))
}

func TestRecordUserLoginFailed_ReasonField(t *testing.T) {
	tests := []struct {
		name       string
		reason     string
		wantReason bool
	}{
		{name: "with reason", reason: "invalid_password", wantReason: true},
		{name: "empty reason omits field", reason: "", wantReason: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, _, hook := newTestMetricsWithLogger(t)
			hook.Reset()

			m.RecordUserLoginFailed(context.Background(), tc.reason)

			require.Len(t, hook.AllEntries(), 1)
			entry := hook.LastEntry()
			assert.Equal(t, "user.login.failed", entry.Data["event"])
			assert.Equal(t, eventCategoryUser, entry.Data["event_category"])

			reason, ok := entry.Data["reason"]
			if tc.wantReason {
				assert.True(t, ok, "reason field should be present")
				assert.Equal(t, tc.reason, reason)
			} else {
				assert.False(t, ok, "reason field should be omitted when empty")
			}
		})
	}
}

func TestRecordAIToolsHooksCall_ToolNameField(t *testing.T) {
	tests := []struct {
		name         string
		toolName     string
		wantToolName bool
	}{
		{name: "with tool name", toolName: "search", wantToolName: true},
		{name: "empty tool name omits field", toolName: "", wantToolName: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, _, hook := newTestMetricsWithLogger(t)
			hook.Reset()

			m.RecordAIToolsHooksCall(context.Background(), tc.toolName)

			require.Len(t, hook.AllEntries(), 1)
			entry := hook.LastEntry()
			assert.Equal(t, "ai_tools.hooks.call", entry.Data["event"])
			assert.Equal(t, eventCategoryAITools, entry.Data["event_category"])

			toolName, ok := entry.Data["tool_name"]
			if tc.wantToolName {
				assert.True(t, ok, "tool_name field should be present")
				assert.Equal(t, tc.toolName, toolName)
			} else {
				assert.False(t, ok, "tool_name field should be omitted when empty")
			}
		})
	}
}

func TestRecordStripeWebhook_KnownEvents(t *testing.T) {
	tests := []struct {
		name          string
		eventType     string
		wantEvent     string
		wantCounter   string
		wantCounterID string
	}{
		{
			name:        "subscription created",
			eventType:   "customer.subscription.created",
			wantEvent:   "stripe.subscription.created",
			wantCounter: "vx_stripe_subscription_created",
		},
		{
			name:        "subscription updated",
			eventType:   "customer.subscription.updated",
			wantEvent:   "stripe.subscription.updated",
			wantCounter: "vx_stripe_subscription_updated",
		},
		{
			name:        "subscription deleted",
			eventType:   "customer.subscription.deleted",
			wantEvent:   "stripe.subscription.deleted",
			wantCounter: "vx_stripe_subscription_deleted",
		},
		{
			name:        "payment succeeded",
			eventType:   "invoice.payment_succeeded",
			wantEvent:   "stripe.payment.succeeded",
			wantCounter: "vx_stripe_payment_succeeded",
		},
		{
			name:        "payment failed",
			eventType:   "invoice.payment_failed",
			wantEvent:   "stripe.payment.failed",
			wantCounter: "vx_stripe_payment_failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, reader, hook := newTestMetricsWithLogger(t)
			hook.Reset()

			m.RecordStripeWebhook(context.Background(), tc.eventType)

			require.Len(t, hook.AllEntries(), 1)
			entry := hook.LastEntry()
			assert.Equal(t, tc.wantEvent, entry.Data["event"])
			assert.Equal(t, eventCategoryBilling, entry.Data["event_category"])
			assert.Equal(t, tc.eventType, entry.Data["stripe_event_type"])

			rm := scrapeMetrics(t, reader)
			assert.Equal(t, int64(1), counterValueByName(t, rm, tc.wantCounter))
		})
	}
}

func TestRecordStripeWebhook_UnknownEventDoesNothing(t *testing.T) {
	m, reader, hook := newTestMetricsWithLogger(t)
	hook.Reset()

	m.RecordStripeWebhook(context.Background(), "customer.created")

	assert.Empty(t, hook.AllEntries(), "unknown event type must not emit a log line")

	rm := scrapeMetrics(t, reader)
	assert.Equal(t, int64(0), counterValueByName(t, rm, "vx_stripe_subscription_created"))
	assert.Equal(t, int64(0), counterValueByName(t, rm, "vx_stripe_payment_succeeded"))
}

// TestSimpleBusinessEvents covers every no-field Record* method, asserting it
// emits exactly one log line with the expected event/category pair.
func TestSimpleBusinessEvents(t *testing.T) {
	tests := []struct {
		name         string
		record       func(*Metrics, context.Context)
		wantEvent    string
		wantCategory string
	}{
		{"user created", (*Metrics).RecordUserCreated, "user.created", eventCategoryUser},
		{"user login succeeded", (*Metrics).RecordUserLoginSuccessful, "user.login.succeeded", eventCategoryUser},
		{"apikey created", (*Metrics).RecordAPIKeyCreated, "apikey.created", eventCategoryAPIKey},
		{"prompt created", (*Metrics).RecordPromptCreated, "prompt.created", eventCategoryContent},
		{"prompt deleted", (*Metrics).RecordPromptDeleted, "prompt.deleted", eventCategoryContent},
		{"artifact created", (*Metrics).RecordArtifactCreated, "artifact.created", eventCategoryContent},
		{"artifact deleted", (*Metrics).RecordArtifactDeleted, "artifact.deleted", eventCategoryContent},
		{"memory created", (*Metrics).RecordMemoryCreated, "memory.created", eventCategoryContent},
		{"memory deleted", (*Metrics).RecordMemoryDeleted, "memory.deleted", eventCategoryContent},
		{"blueprint created", (*Metrics).RecordBlueprintCreated, "blueprint.created", eventCategoryContent},
		{"blueprint deleted", (*Metrics).RecordBlueprintDeleted, "blueprint.deleted", eventCategoryContent},
		{"mcp list tools", (*Metrics).RecordMCPListTools, "mcp.list_tools", eventCategoryMCP},
		{"mcp list prompts", (*Metrics).RecordMCPListPrompts, "mcp.list_prompts", eventCategoryMCP},
		{"mcp get prompt", (*Metrics).RecordMCPGetPrompt, "mcp.get_prompt", eventCategoryMCP},
		{"mcp search prompts", (*Metrics).RecordMCPSearchPrompts, "mcp.search_prompts", eventCategoryMCP},
		{"mcp list artifacts", (*Metrics).RecordMCPListArtifacts, "mcp.list_artifacts", eventCategoryMCP},
		{"mcp get artifact", (*Metrics).RecordMCPGetArtifact, "mcp.get_artifact", eventCategoryMCP},
		{"mcp create artifact", (*Metrics).RecordMCPCreateArtifact, "mcp.create_artifact", eventCategoryMCP},
		{"mcp update artifact", (*Metrics).RecordMCPUpdateArtifact, "mcp.update_artifact", eventCategoryMCP},
		{"mcp list memories", (*Metrics).RecordMCPListMemories, "mcp.list_memories", eventCategoryMCP},
		{"mcp get memory", (*Metrics).RecordMCPGetMemory, "mcp.get_memory", eventCategoryMCP},
		{"mcp create memory", (*Metrics).RecordMCPCreateMemory, "mcp.create_memory", eventCategoryMCP},
		{"mcp update memory", (*Metrics).RecordMCPUpdateMemory, "mcp.update_memory", eventCategoryMCP},
		{"mcp create prompt", (*Metrics).RecordMCPCreatePrompt, "mcp.create_prompt", eventCategoryMCP},
		{"mcp update prompt", (*Metrics).RecordMCPUpdatePrompt, "mcp.update_prompt", eventCategoryMCP},
		{"mcp create blueprint", (*Metrics).RecordMCPCreateBlueprint, "mcp.create_blueprint", eventCategoryMCP},
		{"mcp update blueprint", (*Metrics).RecordMCPUpdateBlueprint, "mcp.update_blueprint", eventCategoryMCP},
		{"mcp datetime", (*Metrics).RecordMCPDateTime, "mcp.datetime", eventCategoryMCP},
		{"mcp get user", (*Metrics).RecordMCPGetUser, "mcp.get_user", eventCategoryMCP},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, _, hook := newTestMetricsWithLogger(t)
			hook.Reset()

			tc.record(m, context.Background())

			require.Len(t, hook.AllEntries(), 1)
			entry := hook.LastEntry()
			assert.Equal(t, tc.wantEvent, entry.Data["event"])
			assert.Equal(t, tc.wantCategory, entry.Data["event_category"])
		})
	}
}
