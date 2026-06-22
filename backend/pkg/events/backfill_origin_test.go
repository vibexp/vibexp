package events

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/services/crm"
)

// countingCRMService is a thread-safe HubSpotCRMServiceInterface stub that counts
// UpdateContact calls, so an assertion across the bus worker goroutine and the
// test goroutine is race-free under `go test -race`.
type countingCRMService struct {
	updateContactCalls int32
}

func (c *countingCRMService) CreateContact(context.Context, crm.ContactData) error { return nil }

func (c *countingCRMService) UpdateContact(context.Context, string, crm.ContactData) error {
	atomic.AddInt32(&c.updateContactCalls, 1)
	return nil
}

func (c *countingCRMService) GetContactByEmail(context.Context, string) (*crm.Contact, error) {
	return nil, nil
}

func (c *countingCRMService) updateCalls() int32 { return atomic.LoadInt32(&c.updateContactCalls) }

// eventWithoutMarker is an Event implementation that does NOT embed BaseEvent and
// therefore carries no backfill-origin marker, exercising the helper's fallback.
type eventWithoutMarker struct{}

func (eventWithoutMarker) Type() string             { return "test.event" }
func (eventWithoutMarker) Payload() interface{}     { return nil }
func (eventWithoutMarker) Timestamp() time.Time     { return time.Time{} }
func (eventWithoutMarker) UserID() string           { return "" }
func (eventWithoutMarker) RetryPolicy() RetryPolicy { return RetryPolicyNone }

func TestIsBackfillOrigin_UnmarkedEvent(t *testing.T) {
	event := NewPromptCreatedEvent("p1", "u1", "e@x.com", "proj", "slug", "title", "body", time.Now())
	assert.False(t, IsBackfillOrigin(event), "a freshly built event must not be backfill-origin")
}

func TestMarkBackfillOrigin_MarksEvent(t *testing.T) {
	event := NewPromptCreatedEvent("p1", "u1", "e@x.com", "proj", "slug", "title", "body", time.Now())

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
	marked := NewPromptCreatedEvent("p1", "u1", "e@x.com", "proj", "slug", "title", "body", time.Now())
	unmarked := NewPromptCreatedEvent("p2", "u1", "e@x.com", "proj", "slug", "title", "body", time.Now())

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

// TestBackfillOrigin_BusDispatch_ForwarderReceivesCRMSkips is the integration
// assertion required by issue #1668's acceptance criteria: a backfill-origin
// event dispatched through the real in-memory bus must reach a type-routed
// listener (standing in for the Pub/Sub embedding forwarder) while the
// side-effect CRM listener skips it. It also confirms a normal event still
// drives the CRM listener, proving the skip is scoped to the marker.
func TestBackfillOrigin_BusDispatch_ForwarderReceivesCRMSkips(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	bus := NewInMemoryEventBus(EventBusConfig{
		Config: Config{WorkerCount: 2, BufferSize: 10},
		Logger: logger,
	})
	require.NoError(t, bus.Start())
	defer func() { require.NoError(t, bus.Stop()) }()

	// Stand-in for the Pub/Sub embedding forwarder: routes by event type only.
	forwarder := NewMockListener([]string{EventTypePromptCreated})
	require.NoError(t, bus.Subscribe(forwarder))

	mockCRM := &countingCRMService{}
	require.NoError(t, bus.Subscribe(NewHubSpotCRMListener(mockCRM, logger)))

	backfillEvent := NewPromptCreatedEvent(
		"prompt-1", "user-1", "test@example.com", "proj", "slug", "title", "body", time.Now(),
	)
	MarkBackfillOrigin(backfillEvent)
	require.NoError(t, bus.Publish(context.Background(), backfillEvent))

	// Allow the worker pool to drain.
	time.Sleep(200 * time.Millisecond)

	require.Equal(t, int32(1), forwarder.GetHandleCount(),
		"the embedding forwarder must still receive backfill-origin events")
	assert.True(t, IsBackfillOrigin(forwarder.GetHandledEvents()[0]),
		"the backfill-origin marker must survive bus dispatch")
	assert.Equal(t, int32(0), mockCRM.updateCalls(),
		"the CRM listener must skip backfill-origin events")

	// Control: a normal event of the same type still drives the CRM listener.
	normalEvent := NewPromptCreatedEvent(
		"prompt-2", "user-1", "test@example.com", "proj", "slug", "title", "body", time.Now(),
	)
	require.NoError(t, bus.Publish(context.Background(), normalEvent))
	time.Sleep(200 * time.Millisecond)

	assert.Equal(t, int32(1), mockCRM.updateCalls(),
		"a normal prompt.created must still sync to the CRM")
	assert.Equal(t, int32(2), forwarder.GetHandleCount())
}
