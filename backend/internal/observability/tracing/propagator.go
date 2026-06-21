package tracing

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/vibexp/vibexp/internal/utils"
)

// CloudTraceContextPropagator implements propagation.TextMapPropagator for Google Cloud Trace
// Format: X-Cloud-Trace-Context: TRACE_ID/SPAN_ID;o=TRACE_TRUE
type CloudTraceContextPropagator struct{}

const (
	// CloudTraceHeader is the header used by Google Cloud Trace
	CloudTraceHeader = "X-Cloud-Trace-Context"
)

// Inject sets the trace context into the carrier for outgoing requests
func (p *CloudTraceContextPropagator) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return
	}

	// Format: TRACE_ID/SPAN_ID;o=TRACE_TRUE
	traceID := sc.TraceID().String()
	spanID := sc.SpanID().String()

	// Convert span ID from 16 hex chars to decimal for Cloud Trace format
	spanIDInt, err := strconv.ParseUint(spanID, 16, 64)
	if err != nil {
		return
	}

	// Determine trace flag (o=1 if sampled, o=0 if not)
	traceFlag := "0"
	if sc.IsSampled() {
		traceFlag = "1"
	}

	value := fmt.Sprintf("%s/%d;o=%s", traceID, spanIDInt, traceFlag)
	carrier.Set(CloudTraceHeader, value)
}

// Extract reads the trace context from the carrier for incoming requests
func (p *CloudTraceContextPropagator) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	header := carrier.Get(CloudTraceHeader)
	if header == "" {
		return ctx
	}

	traceID, spanID, sampled, ok := parseCloudTraceHeader(header)
	if !ok {
		return ctx
	}

	// Create span context
	tid, err := trace.TraceIDFromHex(traceID)
	if err != nil {
		return ctx
	}

	sid, err := trace.SpanIDFromHex(spanID)
	if err != nil {
		return ctx
	}

	var flags trace.TraceFlags
	if sampled {
		flags = trace.FlagsSampled
	}

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: flags,
		Remote:     true,
	})

	return trace.ContextWithRemoteSpanContext(ctx, sc)
}

// Fields returns the headers used by this propagator
func (p *CloudTraceContextPropagator) Fields() []string {
	return []string{CloudTraceHeader}
}

// parseCloudTraceHeader parses the X-Cloud-Trace-Context header
// Format: TRACE_ID/SPAN_ID;o=TRACE_TRUE
// Example: 105445aa7843bc8bf206b120001000/1;o=1
func parseCloudTraceHeader(header string) (traceID, spanID string, sampled bool, ok bool) {
	// Split by ;o= to get options
	parts := strings.Split(header, ";o=")
	mainPart := parts[0]

	// Check if sampled
	if len(parts) > 1 {
		sampled = parts[1] == "1"
	}

	// Split main part by /
	mainParts := strings.Split(mainPart, "/")
	if len(mainParts) < 2 {
		return "", "", false, false
	}

	traceID = mainParts[0]

	// Validate trace ID (should be 32 hex characters)
	if len(traceID) != 32 {
		return "", "", false, false
	}
	for _, c := range traceID {
		if !utils.IsHexChar(c) {
			return "", "", false, false
		}
	}

	// Parse span ID (decimal integer in Cloud Trace format)
	spanIDInt, err := strconv.ParseUint(mainParts[1], 10, 64)
	if err != nil {
		// Try parsing as hex if decimal fails
		spanID = mainParts[1]
		if len(spanID) != 16 {
			return "", "", false, false
		}
	} else {
		// Convert decimal to 16-character hex string
		spanID = fmt.Sprintf("%016x", spanIDInt)
	}

	return traceID, spanID, sampled, true
}
