package events

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeEmbeddingProcessor struct {
	mu    sync.Mutex
	calls []Event
	err   error
}

func (f *fakeEmbeddingProcessor) ProcessEvent(_ context.Context, event Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, event)
	return f.err
}

func (f *fakeEmbeddingProcessor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func TestEmbeddingWorker_HandleDelegates(t *testing.T) {
	proc := &fakeEmbeddingProcessor{}
	w := NewEmbeddingWorker(proc, slog.New(slog.DiscardHandler))

	event := NewPromptCreatedEvent("p1", "u1", "e", "proj", "slug", "t", "", "b", time.Now())
	require.NoError(t, w.Handle(context.Background(), event))

	require.Equal(t, 1, proc.callCount())
	assert.Equal(t, EventTypePromptCreated, proc.calls[0].Type())
}

func TestEmbeddingWorker_EventTypes(t *testing.T) {
	w := NewEmbeddingWorker(&fakeEmbeddingProcessor{}, slog.New(slog.DiscardHandler))
	types := w.EventTypes()

	// Entity create + update events drive embedding; feed items/replies are
	// immutable so only their created events are present.
	assert.Contains(t, types, EventTypePromptCreated)
	assert.Contains(t, types, EventTypePromptUpdated)
	assert.Contains(t, types, EventTypeBlueprintUpdated)
	assert.Contains(t, types, EventTypeFeedItemCreated)
	assert.Contains(t, types, EventTypeFeedItemReplyCreated)
	assert.NotContains(t, types, EventTypeUserCreated)
}

// TestEmbeddingWorker_AsyncOnBus verifies the worker runs off the in-memory event
// bus: publishing an entity created event invokes the processor asynchronously,
// with no broker involved.
func TestEmbeddingWorker_AsyncOnBus(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	bus := NewInMemoryEventBus(EventBusConfig{Config: Config{WorkerCount: 2, BufferSize: 10}, Logger: logger})
	require.NoError(t, bus.Start())
	defer func() { require.NoError(t, bus.Stop()) }()

	proc := &fakeEmbeddingProcessor{}
	require.NoError(t, bus.Subscribe(NewEmbeddingWorker(proc, logger)))

	event := NewPromptCreatedEvent("p1", "u1", "e", "proj", "slug", "Title", "", "Body", time.Now())
	require.NoError(t, bus.Publish(context.Background(), event))

	require.Eventually(t, func() bool { return proc.callCount() == 1 }, 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, EventTypePromptCreated, proc.calls[0].Type())
}
