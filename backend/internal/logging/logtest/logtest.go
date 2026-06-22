// Package logtest provides an slog.Handler that records emitted log entries for
// assertions in tests. Its API deliberately mirrors the shape of the logrus
// test hook it replaces (LastEntry / AllEntries / Reset, and an Entry exposing
// Level, Message and Data) so test migrations stay mechanical.
package logtest

import (
	"context"
	"log/slog"
	"sync"
)

// Entry is a captured log record. Data holds the record's attributes (including
// any accumulated via Logger.With) keyed by attribute name; grouped attributes
// are flattened to their leaf key.
type Entry struct {
	Level   slog.Level
	Message string
	Data    map[string]any
}

// Recorder captures entries emitted through the logger returned by New. It is
// safe for concurrent use.
type Recorder struct {
	mu      sync.Mutex
	entries []*Entry
}

// New returns a logger that records everything it emits, at every level, into
// the returned Recorder.
func New() (*slog.Logger, *Recorder) {
	r := &Recorder{}
	return slog.New(&recordHandler{rec: r}), r
}

// AllEntries returns a snapshot of all captured entries, oldest first.
func (r *Recorder) AllEntries() []*Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Entry, len(r.entries))
	copy(out, r.entries)
	return out
}

// Entries is an alias for AllEntries.
func (r *Recorder) Entries() []*Entry { return r.AllEntries() }

// LastEntry returns the most recent captured entry, or nil when none captured.
func (r *Recorder) LastEntry() *Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) == 0 {
		return nil
	}
	return r.entries[len(r.entries)-1]
}

// Reset discards all captured entries.
func (r *Recorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = nil
}

func (r *Recorder) add(e *Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
}

type recordHandler struct {
	rec   *Recorder
	attrs []slog.Attr
}

func (h *recordHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordHandler) Handle(_ context.Context, rec slog.Record) error {
	data := make(map[string]any, len(h.attrs)+rec.NumAttrs())
	for _, a := range h.attrs {
		data[a.Key] = a.Value.Resolve().Any()
	}
	rec.Attrs(func(a slog.Attr) bool {
		data[a.Key] = a.Value.Resolve().Any()
		return true
	})
	h.rec.add(&Entry{Level: rec.Level, Message: rec.Message, Data: data})
	return nil
}

func (h *recordHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &recordHandler{rec: h.rec, attrs: merged}
}

// WithGroup flattens groups: nested attributes are recorded by their leaf key,
// which matches how the previous logrus-based tests asserted on fields.
func (h *recordHandler) WithGroup(string) slog.Handler { return h }
