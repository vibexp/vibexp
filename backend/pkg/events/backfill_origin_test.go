package events

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// eventWithoutMarker is an Event implementation that does NOT embed BaseEvent and
// therefore carries no backfill-origin marker, exercising the helper's fallback.
type eventWithoutMarker struct{}

func (eventWithoutMarker) Type() string             { return "test.event" }
func (eventWithoutMarker) Payload() interface{}     { return nil }
func (eventWithoutMarker) Timestamp() time.Time     { return time.Time{} }
func (eventWithoutMarker) UserID() string           { return "" }
func (eventWithoutMarker) RetryPolicy() RetryPolicy { return RetryPolicyNone }

func TestIsBackfillOrigin_UnmarkedEvent(t *testing.T) {
	event := NewPromptCreatedEvent("p1", "u1", "e@x.com", "proj", "slug", "title", "", "body", time.Now())
	assert.False(t, IsBackfillOrigin(event), "a freshly built event must not be backfill-origin")
}

func TestMarkBackfillOrigin_MarksEvent(t *testing.T) {
	event := NewPromptCreatedEvent("p1", "u1", "e@x.com", "proj", "slug", "title", "", "body", time.Now())

	returned := MarkBackfillOrigin(event)

	assert.Same(t, event, returned, "MarkBackfillOrigin returns the same event for chaining")
	assert.True(t, IsBackfillOrigin(event), "event must report backfill-origin after marking")
}

func TestMarkBackfillOrigin_IsIdempotent(t *testing.T) {
	event := NewFeedItemCreatedEvent("i1", "u1", "t1", "f1", "title", "content", "exc", time.Now())

	MarkBackfillOrigin(event)
	MarkBackfillOrigin(event)

	assert.True(t, IsBackfillOrigin(event))
}

func TestMarkBackfillOrigin_DoesNotAffectOtherEvents(t *testing.T) {
	marked := NewPromptCreatedEvent("p1", "u1", "e@x.com", "proj", "slug", "title", "", "body", time.Now())
	unmarked := NewPromptCreatedEvent("p2", "u1", "e@x.com", "proj", "slug", "title", "", "body", time.Now())

	MarkBackfillOrigin(marked)

	assert.True(t, IsBackfillOrigin(marked))
	assert.False(t, IsBackfillOrigin(unmarked), "marking one event must not affect another")
}

func TestIsBackfillOrigin_EventWithoutMarker(t *testing.T) {
	assert.False(t, IsBackfillOrigin(eventWithoutMarker{}),
		"an event that does not support the marker must report false, not panic")
}

func TestMarkBackfillOrigin_EventWithoutMarkerPassesThrough(t *testing.T) {
	e := eventWithoutMarker{}
	returned := MarkBackfillOrigin(e)
	assert.Equal(t, e, returned, "unsupported events pass through unchanged")
	assert.False(t, IsBackfillOrigin(returned))
}

// TestBackfillOrigin_BusDispatch_MarkerSurvives confirms a backfill-origin event
// dispatched through the real in-memory bus reaches a type-routed listener
// (standing in for the embedding forwarder) with its marker intact, so
// side-effect listeners can still detect and skip it after dispatch.
func TestBackfillOrigin_BusDispatch_MarkerSurvives(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	bus := NewInMemoryEventBus(EventBusConfig{
		Config: Config{WorkerCount: 2, BufferSize: 10},
		Logger: logger,
	})
	require.NoError(t, bus.Start())
	defer func() { require.NoError(t, bus.Stop()) }()

	// Stand-in for the embedding forwarder: routes by event type only.
	forwarder := NewMockListener([]string{EventTypePromptCreated})
	require.NoError(t, bus.Subscribe(forwarder))

	backfillEvent := NewPromptCreatedEvent(
		"prompt-1", "user-1", "test@example.com", "proj", "slug", "title", "", "body", time.Now(),
	)
	MarkBackfillOrigin(backfillEvent)
	require.NoError(t, bus.Publish(context.Background(), backfillEvent))

	// Allow the worker pool to drain.
	time.Sleep(200 * time.Millisecond)

	require.Equal(t, int32(1), forwarder.GetHandleCount(),
		"the embedding forwarder must still receive backfill-origin events")
	assert.True(t, IsBackfillOrigin(forwarder.GetHandledEvents()[0]),
		"the backfill-origin marker must survive bus dispatch")
}
