package audit

import (
	"context"
	"maps"

	"github.com/vibexp/vibexp/internal/models"
)

// Attribution is who and what every event recorded by a process is attributed
// to, when the process is not serving a request: the recovery CLI (#1535)
// records each of its changes as the system acting through the cli channel,
// with the OS user and host that ran it.
type Attribution struct {
	// Source replaces the event's channel; empty keeps the event's own.
	Source models.AuditSource
	// Actor replaces the event's actor.
	Actor Actor
	// Metadata is added to each event's metadata. An event's own key wins over
	// an attribution key of the same name.
	Metadata map[string]any
}

// AttributedRecorder stamps an Attribution onto every event before handing it
// to the next recorder.
//
// The recovery CLI wraps its recorder in one and gives that to both of its
// recording paths: the instance settings observer (which builds its events
// from repository entries and knows nothing of the CLI) and its own explicit
// root-password event. Wrapping the recorder rather than carrying the
// attribution in the context keeps it out of every context the CLI passes
// around, and makes it impossible for one path to forget it.
type AttributedRecorder struct {
	next        Recorder
	attribution Attribution
}

var _ Recorder = (*AttributedRecorder)(nil)

// NewAttributedRecorder wraps next so every event carries attribution.
func NewAttributedRecorder(next Recorder, attribution Attribution) *AttributedRecorder {
	return &AttributedRecorder{next: next, attribution: attribution}
}

// Record implements Recorder. The event's metadata map is copied, never
// modified.
func (r *AttributedRecorder) Record(ctx context.Context, e Event) {
	if r.attribution.Source != "" {
		e.Source = r.attribution.Source
	}
	actor := r.attribution.Actor
	e.Actor = &actor
	if len(r.attribution.Metadata) > 0 {
		merged := make(map[string]any, len(r.attribution.Metadata)+len(e.Metadata))
		maps.Copy(merged, r.attribution.Metadata)
		maps.Copy(merged, e.Metadata)
		e.Metadata = merged
	}
	r.next.Record(ctx, e)
}
