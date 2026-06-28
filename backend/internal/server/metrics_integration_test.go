package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"

	"github.com/vibexp/vibexp/internal/observability/metrics"
)

// TestMetrics_UserCreated verifies that vx_user_created metric is recorded
func TestMetrics_UserCreated(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	m := setupTestMetrics(t, reader)

	// Record user created metric
	m.RecordUserCreated(context.Background())

	// Read and verify metrics
	rm := collectMetrics(t, reader)
	assertCounterValue(t, rm, "vx_user_created", 1)
}

// TestMetrics_UserLoginSuccessful verifies that vx_user_login_successful metric is recorded
func TestMetrics_UserLoginSuccessful(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	m := setupTestMetrics(t, reader)

	m.RecordUserLoginSuccessful(context.Background())

	rm := collectMetrics(t, reader)
	assertCounterValue(t, rm, "vx_user_login_successful", 1)
}

// TestMetrics_UserLoginFailed verifies that vx_user_login_failed metric is recorded with reason
func TestMetrics_UserLoginFailed(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	m := setupTestMetrics(t, reader)

	m.RecordUserLoginFailed(context.Background(), "invalid_credentials")

	rm := collectMetrics(t, reader)
	assertCounterValueWithAttributes(t, rm, "vx_user_login_failed", 1, map[string]string{
		"reason": "invalid_credentials",
	})
}

// TestMetrics_StripeWebhooks verifies all Stripe webhook metrics
func TestMetrics_StripeWebhooks(t *testing.T) {
	tests := []struct {
		name       string
		eventType  string
		metricName string
	}{
		{"subscription created", "customer.subscription.created", "vx_stripe_subscription_created"},
		{"subscription updated", "customer.subscription.updated", "vx_stripe_subscription_updated"},
		{"subscription deleted", "customer.subscription.deleted", "vx_stripe_subscription_deleted"},
		{"payment succeeded", "invoice.payment_succeeded", "vx_stripe_payment_succeeded"},
		{"payment failed", "invoice.payment_failed", "vx_stripe_payment_failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			m := setupTestMetrics(t, reader)

			m.RecordStripeWebhook(context.Background(), tt.eventType)

			rm := collectMetrics(t, reader)
			assertCounterValue(t, rm, tt.metricName, 1)
		})
	}
}

// TestMetrics_APIKeyCreated verifies that vx_api_key_created metric is recorded
func TestMetrics_APIKeyCreated(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	m := setupTestMetrics(t, reader)

	m.RecordAPIKeyCreated(context.Background())

	rm := collectMetrics(t, reader)
	assertCounterValue(t, rm, "vx_api_key_created", 1)
}

// TestMetrics_AIToolsHooksCall verifies that vx_ai_tools_hooks_call metric is recorded with tool name
func TestMetrics_AIToolsHooksCall(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	m := setupTestMetrics(t, reader)

	m.RecordAIToolsHooksCall(context.Background(), "Bash")

	rm := collectMetrics(t, reader)
	assertCounterValueWithAttributes(t, rm, "vx_ai_tools_hooks_call", 1, map[string]string{
		"tool_name": "Bash",
	})
}

// TestMetrics_PromptLifecycle verifies prompt created/deleted metrics
func TestMetrics_PromptLifecycle(t *testing.T) {
	t.Run("prompt created", func(t *testing.T) {
		reader := sdkmetric.NewManualReader()
		m := setupTestMetrics(t, reader)

		m.RecordPromptCreated(context.Background())

		rm := collectMetrics(t, reader)
		assertCounterValue(t, rm, "vx_prompt_created", 1)
	})

	t.Run("prompt deleted", func(t *testing.T) {
		reader := sdkmetric.NewManualReader()
		m := setupTestMetrics(t, reader)

		m.RecordPromptDeleted(context.Background())

		rm := collectMetrics(t, reader)
		assertCounterValue(t, rm, "vx_prompt_deleted", 1)
	})
}

// TestMetrics_ArtifactLifecycle verifies artifact created/deleted metrics
func TestMetrics_ArtifactLifecycle(t *testing.T) {
	t.Run("artifact created", func(t *testing.T) {
		reader := sdkmetric.NewManualReader()
		m := setupTestMetrics(t, reader)

		m.RecordArtifactCreated(context.Background())

		rm := collectMetrics(t, reader)
		assertCounterValue(t, rm, "vx_artifact_created", 1)
	})

	t.Run("artifact deleted", func(t *testing.T) {
		reader := sdkmetric.NewManualReader()
		m := setupTestMetrics(t, reader)

		m.RecordArtifactDeleted(context.Background())

		rm := collectMetrics(t, reader)
		assertCounterValue(t, rm, "vx_artifact_deleted", 1)
	})
}

// TestMetrics_MemoryLifecycle verifies memory created/deleted metrics
func TestMetrics_MemoryLifecycle(t *testing.T) {
	t.Run("memory created", func(t *testing.T) {
		reader := sdkmetric.NewManualReader()
		m := setupTestMetrics(t, reader)

		m.RecordMemoryCreated(context.Background())

		rm := collectMetrics(t, reader)
		assertCounterValue(t, rm, "vx_memory_created", 1)
	})

	t.Run("memory deleted", func(t *testing.T) {
		reader := sdkmetric.NewManualReader()
		m := setupTestMetrics(t, reader)

		m.RecordMemoryDeleted(context.Background())

		rm := collectMetrics(t, reader)
		assertCounterValue(t, rm, "vx_memory_deleted", 1)
	})
}

// TestMetrics_MCPPromptOperations verifies MCP prompt/tool metrics
func TestMetrics_MCPPromptOperations(t *testing.T) {
	testMCPMetric(t, "list_tools", "vx_mcp_list_tools",
		func(m *metrics.Metrics) { m.RecordMCPListTools(context.Background()) })
	testMCPMetric(t, "list_prompts", "vx_mcp_list_prompts",
		func(m *metrics.Metrics) { m.RecordMCPListPrompts(context.Background()) })
	testMCPMetric(t, "get_prompt", "vx_mcp_get_prompt",
		func(m *metrics.Metrics) { m.RecordMCPGetPrompt(context.Background()) })
	testMCPMetric(t, "search_prompts", "vx_mcp_search_prompts",
		func(m *metrics.Metrics) { m.RecordMCPSearchPrompts(context.Background()) })
}

// TestMetrics_MCPArtifactOperations verifies MCP artifact metrics
func TestMetrics_MCPArtifactOperations(t *testing.T) {
	testMCPMetric(t, "list_artifacts", "vx_mcp_list_artifacts",
		func(m *metrics.Metrics) { m.RecordMCPListArtifacts(context.Background()) })
	testMCPMetric(t, "get_artifact", "vx_mcp_get_artifact",
		func(m *metrics.Metrics) { m.RecordMCPGetArtifact(context.Background()) })
	testMCPMetric(t, "create_artifact", "vx_mcp_create_artifact",
		func(m *metrics.Metrics) { m.RecordMCPCreateArtifact(context.Background()) })
	testMCPMetric(t, "update_artifact", "vx_mcp_update_artifact",
		func(m *metrics.Metrics) { m.RecordMCPUpdateArtifact(context.Background()) })
}

// TestMetrics_MCPMemoryOperations verifies MCP memory metrics
func TestMetrics_MCPMemoryOperations(t *testing.T) {
	testMCPMetric(t, "list_memories", "vx_mcp_list_memories",
		func(m *metrics.Metrics) { m.RecordMCPListMemories(context.Background()) })
	testMCPMetric(t, "get_memory", "vx_mcp_get_memory",
		func(m *metrics.Metrics) { m.RecordMCPGetMemory(context.Background()) })
	testMCPMetric(t, "create_memory", "vx_mcp_create_memory",
		func(m *metrics.Metrics) { m.RecordMCPCreateMemory(context.Background()) })
	testMCPMetric(t, "update_memory", "vx_mcp_update_memory",
		func(m *metrics.Metrics) { m.RecordMCPUpdateMemory(context.Background()) })
	testMCPMetric(t, "datetime", "vx_mcp_datetime",
		func(m *metrics.Metrics) { m.RecordMCPDateTime(context.Background()) })
}

// TestMetrics_MCPDeleteResource verifies the generic delete_resource metric.
func TestMetrics_MCPDeleteResource(t *testing.T) {
	testMCPMetric(t, "delete_resource", "vx_mcp_delete_resource",
		func(m *metrics.Metrics) { m.RecordMCPDeleteResource(context.Background(), "memory") })
}

func testMCPMetric(t *testing.T, name, metricName string, recordFunc func(*metrics.Metrics)) {
	t.Run(name, func(t *testing.T) {
		reader := sdkmetric.NewManualReader()
		m := setupTestMetrics(t, reader)
		recordFunc(m)
		rm := collectMetrics(t, reader)
		assertCounterValue(t, rm, metricName, 1)
	})
}

// TestMetrics_MultipleRecordings verifies that metrics accumulate correctly
func TestMetrics_MultipleRecordings(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	m := setupTestMetrics(t, reader)

	// Record multiple user creations
	m.RecordUserCreated(context.Background())
	m.RecordUserCreated(context.Background())
	m.RecordUserCreated(context.Background())

	// Record multiple login successes
	m.RecordUserLoginSuccessful(context.Background())
	m.RecordUserLoginSuccessful(context.Background())

	rm := collectMetrics(t, reader)
	assertCounterValue(t, rm, "vx_user_created", 3)
	assertCounterValue(t, rm, "vx_user_login_successful", 2)
}

// Helper functions

func setupTestMetrics(t *testing.T, reader sdkmetric.Reader) *metrics.Metrics {
	t.Helper()
	res, err := resource.New(context.Background())
	require.NoError(t, err)

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)

	meter := meterProvider.Meter("test")
	m := &metrics.Metrics{}

	// Initialize all metrics using the test meter
	require.NoError(t, initializeTestMetrics(m, meter))

	return m
}

func initializeTestMetrics(m *metrics.Metrics, meter metric.Meter) error {
	if err := initAPIMetrics(m, meter); err != nil {
		return err
	}
	if err := initUserAndStripeMetrics(m, meter); err != nil {
		return err
	}
	if err := initResourceMetrics(m, meter); err != nil {
		return err
	}
	return initMCPMetrics(m, meter)
}

func initAPIMetrics(m *metrics.Metrics, meter metric.Meter) error {
	var err error
	if m.APICallsTotal, err = meter.Float64Counter("vx_api_calls_total"); err != nil {
		return err
	}
	if m.APICallDuration, err = meter.Float64Histogram("vx_api_call_duration"); err != nil {
		return err
	}
	return nil
}

func initUserAndStripeMetrics(m *metrics.Metrics, meter metric.Meter) error {
	var err error
	// User metrics
	if m.UserCreated, err = meter.Int64Counter("vx_user_created"); err != nil {
		return err
	}
	if m.UserLoginSuccessful, err = meter.Int64Counter("vx_user_login_successful"); err != nil {
		return err
	}
	if m.UserLoginFailed, err = meter.Int64Counter("vx_user_login_failed"); err != nil {
		return err
	}
	// Stripe metrics
	if m.StripeSubscriptionCreated, err = meter.Int64Counter("vx_stripe_subscription_created"); err != nil {
		return err
	}
	if m.StripeSubscriptionUpdated, err = meter.Int64Counter("vx_stripe_subscription_updated"); err != nil {
		return err
	}
	if m.StripeSubscriptionDeleted, err = meter.Int64Counter("vx_stripe_subscription_deleted"); err != nil {
		return err
	}
	if m.StripePaymentSucceeded, err = meter.Int64Counter("vx_stripe_payment_succeeded"); err != nil {
		return err
	}
	if m.StripePaymentFailed, err = meter.Int64Counter("vx_stripe_payment_failed"); err != nil {
		return err
	}
	return nil
}

func initResourceMetrics(m *metrics.Metrics, meter metric.Meter) error {
	var err error
	if m.APIKeyCreated, err = meter.Int64Counter("vx_api_key_created"); err != nil {
		return err
	}
	if m.AIToolsHooksCall, err = meter.Int64Counter("vx_ai_tools_hooks_call"); err != nil {
		return err
	}
	if m.PromptCreated, err = meter.Int64Counter("vx_prompt_created"); err != nil {
		return err
	}
	if m.PromptDeleted, err = meter.Int64Counter("vx_prompt_deleted"); err != nil {
		return err
	}
	if m.ArtifactCreated, err = meter.Int64Counter("vx_artifact_created"); err != nil {
		return err
	}
	if m.ArtifactDeleted, err = meter.Int64Counter("vx_artifact_deleted"); err != nil {
		return err
	}
	if m.MemoryCreated, err = meter.Int64Counter("vx_memory_created"); err != nil {
		return err
	}
	if m.MemoryDeleted, err = meter.Int64Counter("vx_memory_deleted"); err != nil {
		return err
	}
	return nil
}

func initMCPMetrics(m *metrics.Metrics, meter metric.Meter) error {
	if err := initMCPPromptAndToolMetrics(m, meter); err != nil {
		return err
	}
	if err := initMCPArtifactAndMemoryMetrics(m, meter); err != nil {
		return err
	}
	var err error
	if m.MCPDeleteResource, err = meter.Int64Counter("vx_mcp_delete_resource"); err != nil {
		return err
	}
	if m.MCPDateTime, err = meter.Int64Counter("vx_mcp_datetime"); err != nil {
		return err
	}
	return nil
}

func initMCPPromptAndToolMetrics(m *metrics.Metrics, meter metric.Meter) error {
	var err error
	if m.MCPListTools, err = meter.Int64Counter("vx_mcp_list_tools"); err != nil {
		return err
	}
	if m.MCPListPrompts, err = meter.Int64Counter("vx_mcp_list_prompts"); err != nil {
		return err
	}
	if m.MCPGetPrompt, err = meter.Int64Counter("vx_mcp_get_prompt"); err != nil {
		return err
	}
	if m.MCPSearchPrompts, err = meter.Int64Counter("vx_mcp_search_prompts"); err != nil {
		return err
	}
	return nil
}

func initMCPArtifactAndMemoryMetrics(m *metrics.Metrics, meter metric.Meter) error {
	var err error
	if m.MCPListArtifacts, err = meter.Int64Counter("vx_mcp_list_artifacts"); err != nil {
		return err
	}
	if m.MCPGetArtifact, err = meter.Int64Counter("vx_mcp_get_artifact"); err != nil {
		return err
	}
	if m.MCPCreateArtifact, err = meter.Int64Counter("vx_mcp_create_artifact"); err != nil {
		return err
	}
	if m.MCPUpdateArtifact, err = meter.Int64Counter("vx_mcp_update_artifact"); err != nil {
		return err
	}
	if m.MCPListMemories, err = meter.Int64Counter("vx_mcp_list_memories"); err != nil {
		return err
	}
	if m.MCPGetMemory, err = meter.Int64Counter("vx_mcp_get_memory"); err != nil {
		return err
	}
	if m.MCPCreateMemory, err = meter.Int64Counter("vx_mcp_create_memory"); err != nil {
		return err
	}
	if m.MCPUpdateMemory, err = meter.Int64Counter("vx_mcp_update_memory"); err != nil {
		return err
	}
	return nil
}

func collectMetrics(t *testing.T, reader sdkmetric.Reader) *metricdata.ResourceMetrics {
	t.Helper()
	rm := &metricdata.ResourceMetrics{}
	err := reader.Collect(context.Background(), rm)
	require.NoError(t, err)
	return rm
}

func assertCounterValue(t *testing.T, rm *metricdata.ResourceMetrics, metricName string, expectedValue int64) {
	t.Helper()
	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == metricName {
				found = true
				sum, ok := m.Data.(metricdata.Sum[int64])
				require.True(t, ok, "metric %s should be Sum[int64]", metricName)
				require.Len(t, sum.DataPoints, 1, "metric %s should have exactly 1 data point", metricName)
				assert.Equal(t, expectedValue, sum.DataPoints[0].Value,
					"metric %s should have value %d", metricName, expectedValue)
			}
		}
	}
	assert.True(t, found, "metric %s not found in collected metrics", metricName)
}

func assertCounterValueWithAttributes(t *testing.T, rm *metricdata.ResourceMetrics, metricName string,
	expectedValue int64, expectedAttrs map[string]string) {
	t.Helper()
	dataPoint, found := findInt64CounterDataPoint(rm, metricName)
	require.True(t, found, "metric %s not found in collected metrics", metricName)

	assert.Equal(t, expectedValue, dataPoint.Value,
		"metric %s should have value %d", metricName, expectedValue)

	verifyAttributes(t, dataPoint, metricName, expectedAttrs)
}

func findInt64CounterDataPoint(rm *metricdata.ResourceMetrics, metricName string,
) (metricdata.DataPoint[int64], bool) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == metricName {
				if sum, ok := m.Data.(metricdata.Sum[int64]); ok && len(sum.DataPoints) > 0 {
					return sum.DataPoints[0], true
				}
			}
		}
	}
	return metricdata.DataPoint[int64]{}, false
}

func verifyAttributes(t *testing.T, dp metricdata.DataPoint[int64],
	metricName string, expectedAttrs map[string]string) {
	t.Helper()
	for key, val := range expectedAttrs {
		foundAttr := false
		for _, attr := range dp.Attributes.ToSlice() {
			if string(attr.Key) == key {
				foundAttr = true
				assert.Equal(t, val, attr.Value.AsString(),
					"metric %s attribute %s should be %s", metricName, key, val)
			}
		}
		assert.True(t, foundAttr, "metric %s should have attribute %s", metricName, key)
	}
}
