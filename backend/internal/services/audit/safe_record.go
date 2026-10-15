package audit

import (
	"context"
	"log/slog"
)

// SafeRecord calls r.Record so that a broken Recorder cannot affect the
// caller: a nil recorder is a no-op and a panic is recovered and logged.
// Request handlers and services record through it (#1514).
//
// It does not guard against a Record that blocks: the Recorder contract rules
// that out, and AsyncRecorder honours it (see Recorder).
func SafeRecord(ctx context.Context, r Recorder, e Event) {
	if r == nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			slog.Default().With("panic", rec, "operation", e.Operation).
				Warn("recovered from panic in the audit recorder")
		}
	}()
	r.Record(ctx, e)
}
