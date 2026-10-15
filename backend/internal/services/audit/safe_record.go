package audit

import (
	"context"
	"log/slog"
	"time"
)

// safeRecordWait bounds how long SafeRecord waits for a Recorder. The real
// recorder returns in microseconds (it only reads the context and queues the
// write), so the bound only ever matters for a broken implementation.
const safeRecordWait = 100 * time.Millisecond

// SafeRecord calls r.Record so that no Recorder can affect the caller: a nil
// recorder is a no-op, a panic is recovered and logged, and a Record that has
// not returned after safeRecordWait is abandoned (logged, left to finish on
// its own goroutine). Auth and request handlers record through it, so an audit
// failure can neither break nor stall the response it describes (#1514).
func SafeRecord(ctx context.Context, r Recorder, e Event) {
	if r == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if rec := recover(); rec != nil {
				slog.Default().With("panic", rec, "operation", e.Operation).
					Warn("recovered from panic in the audit recorder")
			}
		}()
		r.Record(ctx, e)
	}()

	timer := time.NewTimer(safeRecordWait)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		slog.Default().With("operation", e.Operation).Warn("audit recorder did not return in time; event abandoned")
	}
}
