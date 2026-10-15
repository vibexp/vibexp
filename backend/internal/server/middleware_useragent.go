package server

import (
	"context"
	"net/http"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// userAgentMiddleware puts the request's User-Agent in the context under
// contextkeys.UserAgent, beside the client IP, for the audit recorder (#1511).
// The header is client-supplied and unbounded, so it is capped at
// audit.MaxUserAgentBytes on a rune boundary. An empty header stores nothing.
func userAgentMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := audit.TruncateBytes(r.UserAgent(), audit.MaxUserAgentBytes); ua != "" {
			r = r.WithContext(context.WithValue(r.Context(), contextkeys.UserAgent, ua))
		}
		next.ServeHTTP(w, r)
	})
}
