package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/logging/logtest"
)

func TestPanicLoggerMiddleware_PanicWithString(t *testing.T) {
	nullLogger, hook := logtest.New()

	req := httptest.NewRequest(http.MethodGet, "/test-panic", nil)
	ctx := context.WithValue(req.Context(), contextkeys.Logger, nullLogger)
	req = req.WithContext(ctx)

	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went very wrong")
	})

	mw := panicLoggerMiddleware(nullLogger)
	rr := httptest.NewRecorder()
	mw(panicHandler).ServeHTTP(rr, req)

	// Should return 500
	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	// Exactly one log entry
	require.Len(t, hook.AllEntries(), 1)

	entry0 := hook.AllEntries()[0]
	assert.Equal(t, slog.LevelError, entry0.Level)
	assert.Equal(t, "recovered from panic", entry0.Message)

	// Verify required fields
	assert.Equal(t, "panic", entry0.Data["event"])
	assert.Equal(t, "something went very wrong", entry0.Data["panic.message"])
	assert.NotEmpty(t, entry0.Data["panic.stack"])
}

func TestPanicLoggerMiddleware_PanicWithError(t *testing.T) {
	nullLogger, hook := logtest.New()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/crash", nil)
	ctx := context.WithValue(req.Context(), contextkeys.Logger, nullLogger)
	req = req.WithContext(ctx)

	panicErr := errors.New("database connection lost")
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(panicErr)
	})

	mw := panicLoggerMiddleware(nullLogger)
	rr := httptest.NewRecorder()
	mw(panicHandler).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	require.Len(t, hook.AllEntries(), 1)

	entry0 := hook.AllEntries()[0]
	assert.Equal(t, slog.LevelError, entry0.Level)
	assert.Equal(t, "recovered from panic", entry0.Message)

	assert.Equal(t, "panic", entry0.Data["event"])
	assert.Equal(t, "database connection lost", entry0.Data["panic.message"])
	assert.NotEmpty(t, entry0.Data["panic.stack"])
	assert.Equal(t, http.MethodPost, entry0.Data["request.method"])
	assert.Equal(t, "/api/v1/crash", entry0.Data["request.path"])
}

func TestPanicLoggerMiddleware_ErrAbortHandlerIsRepanicked(t *testing.T) {
	nullLogger, hook := logtest.New()

	req := httptest.NewRequest(http.MethodGet, "/abort", nil)
	ctx := context.WithValue(req.Context(), contextkeys.Logger, nullLogger)
	req = req.WithContext(ctx)

	abortHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	})

	mw := panicLoggerMiddleware(nullLogger)
	rr := httptest.NewRecorder()

	// ErrAbortHandler should be re-panicked, so the middleware itself panics.
	assert.Panics(t, func() {
		mw(abortHandler).ServeHTTP(rr, req)
	})

	// No log entry should be emitted for intentional aborts.
	assert.Empty(t, hook.AllEntries())
}

func TestPanicLoggerMiddleware_NoPanic(t *testing.T) {
	nullLogger, hook := logtest.New()

	req := httptest.NewRequest(http.MethodGet, "/healthy", nil)
	ctx := context.WithValue(req.Context(), contextkeys.Logger, nullLogger)
	req = req.WithContext(ctx)

	successHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := panicLoggerMiddleware(nullLogger)
	rr := httptest.NewRecorder()
	mw(successHandler).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	// No log entries when no panic occurs
	assert.Empty(t, hook.AllEntries())
}
