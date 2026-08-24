package providers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vibexp/vibexp/pkg/events"
)

// lifecycleProcessor is an events.EmbeddingProcessor that also owns background
// loops, like the real EmbeddingDispatcher.
type lifecycleProcessor struct {
	started int
	stopped int
}

func (p *lifecycleProcessor) ProcessEvent(context.Context, events.Event) error { return nil }
func (p *lifecycleProcessor) Start()                                           { p.started++ }
func (p *lifecycleProcessor) Stop()                                            { p.stopped++ }

// inertProcessor owns no loops, so neither hook applies to it.
type inertProcessor struct{}

func (inertProcessor) ProcessEvent(context.Context, events.Event) error { return nil }

// The start/stop hooks are a pair, and both are type assertions: a processor
// that implements neither must be left alone rather than crash the caller, since
// EmbeddingProcessor itself promises neither method.
func TestEventSystemDeps_StartAndShutdownListeners(t *testing.T) {
	proc := &lifecycleProcessor{}
	deps := &EventSystemDeps{embeddingProcessor: proc}

	deps.StartListeners()
	deps.ShutdownListeners()

	assert.Equal(t, 1, proc.started, "a processor with background loops must be started")
	assert.Equal(t, 1, proc.stopped)

	// A processor without the hooks, and a nil deps (the container's field is
	// only set once the event system is wired), must both be no-ops: this runs
	// on the server's startup and shutdown paths, where a panic is fatal.
	assert.NotPanics(t, func() {
		inert := &EventSystemDeps{embeddingProcessor: inertProcessor{}}
		inert.StartListeners()
		inert.ShutdownListeners()

		var nilDeps *EventSystemDeps
		nilDeps.StartListeners()
		nilDeps.ShutdownListeners()
	})
}
