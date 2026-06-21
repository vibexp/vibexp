package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// mapCarrier implements propagation.TextMapCarrier for testing
type mapCarrier map[string]string

var _ propagation.TextMapCarrier = mapCarrier{}

func (c mapCarrier) Get(key string) string {
	return c[key]
}

func (c mapCarrier) Set(key, val string) {
	c[key] = val
}

func (c mapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

func TestCloudTraceContextPropagator_Fields(t *testing.T) {
	p := &CloudTraceContextPropagator{}
	fields := p.Fields()

	assert.Len(t, fields, 1)
	assert.Equal(t, "X-Cloud-Trace-Context", fields[0])
}

func TestCloudTraceContextPropagator_Extract_ValidHeader(t *testing.T) {
	p := &CloudTraceContextPropagator{}
	carrier := mapCarrier{
		"X-Cloud-Trace-Context": "105445aa7843bc8bf206b12000100000/1;o=1",
	}

	ctx := p.Extract(context.Background(), carrier)
	sc := trace.SpanContextFromContext(ctx)

	assert.True(t, sc.IsValid())
	assert.True(t, sc.IsSampled())
	assert.Equal(t, "105445aa7843bc8bf206b12000100000", sc.TraceID().String())
}

func TestCloudTraceContextPropagator_Extract_ValidHeader_NotSampled(t *testing.T) {
	p := &CloudTraceContextPropagator{}
	carrier := mapCarrier{
		"X-Cloud-Trace-Context": "105445aa7843bc8bf206b12000100000/1;o=0",
	}

	ctx := p.Extract(context.Background(), carrier)
	sc := trace.SpanContextFromContext(ctx)

	assert.True(t, sc.IsValid())
	assert.False(t, sc.IsSampled())
}

func TestCloudTraceContextPropagator_Extract_EmptyHeader(t *testing.T) {
	p := &CloudTraceContextPropagator{}
	carrier := mapCarrier{}

	ctx := p.Extract(context.Background(), carrier)
	sc := trace.SpanContextFromContext(ctx)

	assert.False(t, sc.IsValid())
}

func TestCloudTraceContextPropagator_Extract_InvalidTraceID(t *testing.T) {
	p := &CloudTraceContextPropagator{}
	carrier := mapCarrier{
		"X-Cloud-Trace-Context": "invalid/1;o=1",
	}

	ctx := p.Extract(context.Background(), carrier)
	sc := trace.SpanContextFromContext(ctx)

	assert.False(t, sc.IsValid())
}

func TestCloudTraceContextPropagator_Extract_NoSpanID(t *testing.T) {
	p := &CloudTraceContextPropagator{}
	carrier := mapCarrier{
		"X-Cloud-Trace-Context": "105445aa7843bc8bf206b12000100000",
	}

	ctx := p.Extract(context.Background(), carrier)
	sc := trace.SpanContextFromContext(ctx)

	assert.False(t, sc.IsValid())
}

func TestCloudTraceContextPropagator_Inject(t *testing.T) {
	p := &CloudTraceContextPropagator{}

	traceID, err := trace.TraceIDFromHex("105445aa7843bc8bf206b12000100000")
	assert.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("0000000000000001")
	assert.NoError(t, err)

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})

	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	carrier := mapCarrier{}

	p.Inject(ctx, carrier)

	assert.NotEmpty(t, carrier["X-Cloud-Trace-Context"])
	assert.Contains(t, carrier["X-Cloud-Trace-Context"], "105445aa7843bc8bf206b12000100000")
	assert.Contains(t, carrier["X-Cloud-Trace-Context"], ";o=1")
}

func TestCloudTraceContextPropagator_Inject_NotSampled(t *testing.T) {
	p := &CloudTraceContextPropagator{}

	traceID, err := trace.TraceIDFromHex("105445aa7843bc8bf206b12000100000")
	assert.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("0000000000000001")
	assert.NoError(t, err)

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
		// No TraceFlags = not sampled
	})

	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	carrier := mapCarrier{}

	p.Inject(ctx, carrier)

	assert.NotEmpty(t, carrier["X-Cloud-Trace-Context"])
	assert.Contains(t, carrier["X-Cloud-Trace-Context"], ";o=0")
}

func TestCloudTraceContextPropagator_Inject_InvalidContext(t *testing.T) {
	p := &CloudTraceContextPropagator{}
	carrier := mapCarrier{}

	p.Inject(context.Background(), carrier)

	assert.Empty(t, carrier["X-Cloud-Trace-Context"])
}

func TestParseCloudTraceHeader_ValidDecimalSpanID(t *testing.T) {
	traceID, spanID, sampled, ok := parseCloudTraceHeader("105445aa7843bc8bf206b12000100000/1;o=1")
	assert.True(t, ok)
	assert.Equal(t, "105445aa7843bc8bf206b12000100000", traceID)
	assert.Equal(t, "0000000000000001", spanID)
	assert.True(t, sampled)
}

func TestParseCloudTraceHeader_ValidNotSampled(t *testing.T) {
	traceID, spanID, sampled, ok := parseCloudTraceHeader("105445aa7843bc8bf206b12000100000/123;o=0")
	assert.True(t, ok)
	assert.Equal(t, "105445aa7843bc8bf206b12000100000", traceID)
	assert.Equal(t, "000000000000007b", spanID)
	assert.False(t, sampled)
}

func TestParseCloudTraceHeader_ValidWithoutOptions(t *testing.T) {
	traceID, spanID, sampled, ok := parseCloudTraceHeader("105445aa7843bc8bf206b12000100000/1")
	assert.True(t, ok)
	assert.Equal(t, "105445aa7843bc8bf206b12000100000", traceID)
	assert.Equal(t, "0000000000000001", spanID)
	assert.False(t, sampled)
}

func TestParseCloudTraceHeader_InvalidTraceIDLength(t *testing.T) {
	_, _, _, ok := parseCloudTraceHeader("short/1;o=1")
	assert.False(t, ok)
}

func TestParseCloudTraceHeader_NoSpanID(t *testing.T) {
	_, _, _, ok := parseCloudTraceHeader("105445aa7843bc8bf206b12000100000")
	assert.False(t, ok)
}

func TestParseCloudTraceHeader_InvalidCharsInTraceID(t *testing.T) {
	_, _, _, ok := parseCloudTraceHeader("zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz/1;o=1")
	assert.False(t, ok)
}
