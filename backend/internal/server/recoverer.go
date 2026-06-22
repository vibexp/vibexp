package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/vibexp/vibexp/internal/contextkeys"
)

// maxPanicStackBytes caps the stack trace logged per panic to avoid hitting Cloud Logging's
// 256 KiB per-entry limit on very deep goroutine stacks.
const maxPanicStackBytes = 32 * 1024

// panicLoggerMiddleware recovers from panics and emits a single structured log entry with
// event="panic", the panic message, and the full stack trace. It uses the context logger so
// that the Cloud Logging trace-correlation fields injected by RequestIDMiddleware are preserved.
//
// This middleware is registered as the outermost middleware so that panics in any inner
// middleware (OTel, RequestID, structuredRequestLogger) are also captured. When a panic fires
// before RequestIDMiddleware populates the context logger, GetLoggerFromContext falls through
// to the package fallback — still structured JSON, but without request_id/trace fields.
//
// http.ErrAbortHandler panics are re-panicked so the standard library can handle intentional
// handler aborts (e.g. HTTP/2 stream resets, hijacked connections) without emitting spurious
// ERROR log entries.
//
// The logger parameter is accepted for signature consistency with other middleware constructors
// but logging always goes through the context-scoped logger.
func panicLoggerMiddleware(logger *slog.Logger) func(http.Handler) http.Handler { //nolint:unparam
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					// Re-panic for intentional handler aborts so net/http can handle them cleanly.
					if rec == http.ErrAbortHandler {
						panic(rec) //nolint:forbidigo
					}

					stack := string(debug.Stack())
					if len(stack) > maxPanicStackBytes {
						stack = stack[:maxPanicStackBytes] + "\n... [truncated]"
					}

					contextkeys.GetLoggerFromContext(r.Context()).With(
						"event", "panic",
						"panic.message", fmt.Sprintf("%v", rec),
						"panic.stack", stack,
						"request.method", r.Method,
						"request.path", r.URL.Path,
					).Error("recovered from panic")
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
