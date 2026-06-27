package events

// This file centralizes the "backfill-origin" event marker. The embeddings
// backfill tool (POST /bo/v1/embeddings/backfill) republishes domain `.created`
// events through the shared EventManager so the embedding pipeline regenerates
// vectors. Because Publish fans every event out to ALL in-process listeners,
// those republished events would otherwise also drive user-facing side effects
// (feed-item notifications) once per entity.
//
// Marking an event as backfill-origin lets side-effect listeners skip it via a
// single shared check (IsBackfillOrigin) while the Pub/Sub forwarder and the
// local-dev HTTP sync listener — which route purely by event type — keep
// processing it for embedding regeneration.

// IsBackfillOrigin reports whether the event was republished by the embeddings
// backfill tool rather than emitted by a genuine user action. Side-effect
// listeners short-circuit on a true result. It is nil-safe and returns false for
// any event that does not carry the marker.
func IsBackfillOrigin(event Event) bool {
	r, ok := event.(interface{ IsBackfillOrigin() bool })
	return ok && r.IsBackfillOrigin()
}

// MarkBackfillOrigin tags the event as backfill-origin if its implementation
// supports the marker (all events built on BaseEvent do) and returns it for
// convenient chaining. Events that do not support the marker pass through
// unchanged, so a caller never has to special-case event types.
func MarkBackfillOrigin(event Event) Event {
	if w, ok := event.(interface{ setBackfillOrigin() }); ok {
		w.setBackfillOrigin()
	}
	return event
}

// IsBackfillOrigin reports whether this event was republished by the embeddings
// backfill tool. Prefer the package-level IsBackfillOrigin helper at call sites
// so the check works uniformly across every Event implementation.
func (e *BaseEvent) IsBackfillOrigin() bool {
	return e.backfillOrigin
}

// setBackfillOrigin marks this event as backfill-origin. It is unexported so the
// flag can only be set within this package — callers go through MarkBackfillOrigin.
func (e *BaseEvent) setBackfillOrigin() {
	e.backfillOrigin = true
}
