package audit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

type recorderFunc func(context.Context, Event)

func (f recorderFunc) Record(ctx context.Context, e Event) { f(ctx, e) }

func TestSafeRecord(t *testing.T) {
	ctx := context.Background()
	event := Event{Operation: models.AuditOpAuthLogin}

	t.Run("delivers the event before returning", func(t *testing.T) {
		var got Event
		SafeRecord(ctx, recorderFunc(func(_ context.Context, e Event) { got = e }), event)
		assert.Equal(t, event, got)
	})

	t.Run("a nil recorder is a no-op", func(t *testing.T) {
		assert.NotPanics(t, func() { SafeRecord(ctx, nil, event) })
	})

	t.Run("a panicking recorder never reaches the caller", func(t *testing.T) {
		assert.NotPanics(t, func() {
			SafeRecord(ctx, recorderFunc(func(context.Context, Event) { panic("boom") }), event)
		})
	})

	t.Run("a blocking recorder is abandoned", func(t *testing.T) {
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })

		start := time.Now()
		SafeRecord(ctx, recorderFunc(func(context.Context, Event) { <-release }), event)

		require.Less(t, time.Since(start), 5*safeRecordWait, "SafeRecord must not wait for a stuck recorder")
	})
}
