# Metrics Testing Guide

This document explains how to verify that handlers actually call metrics recording methods.

## Why Handler Integration Tests Matter

The unit tests in `metrics_integration_test.go` verify that **metrics methods work correctly**, but they don't verify that **handlers actually call them**.

Someone could remove this line from a handler:
```go
s.metrics.RecordUserCreated(r.Context())  // Deleted!
```

And the unit test would still pass because it only tests the `RecordUserCreated` method itself.

## Recommended Approach: Real Metrics in Handler Tests

Use OpenTelemetry's `ManualReader` to capture actual metrics in handler integration tests:

```go
func TestAuthHandler_UserCreationRecordsMetric(t *testing.T) {
    // 1. Setup real metrics with ManualReader
    reader := sdkmetric.NewManualReader()
    testMetrics := setupTestMetrics(t, reader)

    // 2. Create test server with real metrics
    mockContainer := newMockAuthContainer(t)
    mockContainer.authService.On("HandleGoogleCallback", mock.Anything, "code").
        Return(user, token, true, nil) // isNewUser = true

    srv := createTestServer(mockContainer)
    srv.metrics = testMetrics  // Inject real metrics

    // 3. Make HTTP request
    req := httptest.NewRequest("POST", "/api/v1/auth/google/callback", body)
    w := httptest.NewRecorder()
    srv.ServeHTTP(w, req)

    // 4. CRITICAL: Verify the metric was actually recorded
    rm := collectMetrics(t, reader)
    assertCounterValue(t, rm, "vx_user_created", 1)
    assertCounterValue(t, rm, "vx_user_login_successful", 1)
}
```

## What This Verifies

✅ **Handler calls the metrics method** - If removed, test fails
✅ **Correct metric name** - Typos caught
✅ **Correct value** - Accumulation verified
✅ **Correct attributes** - tool_name, reason, etc.
✅ **Real OTel SDK** - Not just mocks

## Vs. Mock-Based Approach

```go
// Mock approach (less valuable)
mockMetrics.On("RecordUserCreated", mock.Anything).Once()
// ... handler call ...
mockMetrics.AssertExpectations(t)
```

**Why real metrics are better:**
- Tests actual metric recording, not just method calls
- Verifies metric values and attributes
- No need for metric mocks
- More confidence in production behavior

## Example: Verify Attributes

```go
func TestAIToolsHooks_RecordsToolName(t *testing.T) {
    reader := sdkmetric.NewManualReader()
    testMetrics := setupTestMetrics(t, reader)

    // ... setup and make request with tool_name="Bash" ...

    // Verify both value AND attributes
    rm := collectMetrics(t, reader)
    assertCounterValueWithAttributes(t, rm, "vx_ai_tools_hooks_call", 1,
        map[string]string{
            "tool_name": "Bash",
        })
}
```

## Adding New Handler Metric Tests

When adding a new business metric:

1. Add unit test in `metrics_integration_test.go` - verifies method works
2. Add handler test (recommended) - verifies handler calls it
3. Use the helper functions from `metrics_integration_test.go`:
   - `setupTestMetrics(t, reader)` - creates test metrics
   - `collectMetrics(t, reader)` - reads recorded metrics
   - `assertCounterValue(t, rm, name, value)` - verifies value
   - `assertCounterValueWithAttributes(t, rm, name, value, attrs)` - verifies attributes

## Current Coverage

### Unit Tests (39 tests) ✅
Verify all metrics recording methods work correctly.

### Handler Tests (Recommended to Add)
- [ ] Auth handler - user created, login success/fail
- [ ] API key handler - key creation
- [ ] Prompt handler - create/delete
- [ ] Artifact handler - create/delete
- [ ] Memory handler - create/delete
- [ ] Stripe webhook handler - all event types
- [ ] AI tools hooks - with tool_name attribute
- [ ] MCP handlers - all operations

## Benefits

1. **Prevents Silent Breakage**: If someone removes metrics call, test fails immediately
2. **Refactoring Safety**: Can refactor handlers with confidence
3. **Documentation**: Tests show where metrics are recorded
4. **Attribute Verification**: Ensures attributes like tool_name are correct
5. **Production Confidence**: Tests use real OTel SDK

## Implementation Priority

**High Priority** (add these first):
- User auth metrics (most critical for monitoring)
- Stripe webhook metrics (revenue tracking)
- API key creation (security monitoring)

**Medium Priority**:
- Resource CRUD metrics (prompts, artifacts, memories)
- AI tools hooks metrics

**Low Priority**:
- MCP protocol metrics (less critical for MVP)
