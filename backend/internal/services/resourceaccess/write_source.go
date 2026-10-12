package resourceaccess

import (
	"context"

	"github.com/vibexp/vibexp/internal/models"
)

// SourceUnknown is the creation-metrics bucket for resources that have no
// create event: they were created before write events were recorded (#1392), or
// by an internal caller that records none. It is a reporting key only. It is
// never stored as an event's source.
const SourceUnknown = "unknown"

// writeSources are the channels a write event can be recorded for, in the
// order the metrics report them.
var writeSources = []string{SourceWeb, SourceCLI, SourceMCP, SourceAPI}

// IsWriteSource reports whether source is one of the four write channels.
func IsWriteSource(source string) bool {
	for _, s := range writeSources {
		if s == source {
			return true
		}
	}
	return false
}

// writeOriginKey is the context key WithWriteOrigin stores under. It is an
// unexported type so no other package can set or shadow the value.
type writeOriginKey struct{}

// WithWriteOrigin returns a context that says how the request's writes arrive:
// through source, and for an MCP request under the client-reported
// assistantName. The server sets it once per request, from how the request was
// authenticated and routed (the REST write-source middleware, the MCP tool
// layer), never from anything the client put in a body.
//
// A source that is not a write channel returns ctx unchanged, so a write is
// either labelled with a channel the metrics know or not labelled at all.
func WithWriteOrigin(ctx context.Context, source, assistantName string) context.Context {
	if !IsWriteSource(source) {
		return ctx
	}
	return context.WithValue(ctx, writeOriginKey{}, models.WriteOrigin{
		Source:        source,
		AssistantName: models.NormalizeAssistantName(assistantName),
	})
}

// WriteOriginFromContext returns the origin set by WithWriteOrigin, or nil when
// there is none. Nil is what internal callers see (import, restore, scheduled
// jobs): their writes record no event and leave a version's source empty.
func WriteOriginFromContext(ctx context.Context) *models.WriteOrigin {
	origin, ok := ctx.Value(writeOriginKey{}).(models.WriteOrigin)
	if !ok {
		return nil
	}
	return &origin
}
