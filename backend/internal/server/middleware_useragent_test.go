package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// serveUserAgent runs userAgentMiddleware over a request carrying ua and
// returns the context value the handler saw, and whether it was set.
func serveUserAgent(t *testing.T, ua string) (string, bool) {
	t.Helper()
	var got string
	var present bool
	handler := userAgentMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, present = r.Context().Value(contextkeys.UserAgent).(string)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("User-Agent", ua)
	handler.ServeHTTP(httptest.NewRecorder(), req)
	return got, present
}

func TestUserAgentMiddleware(t *testing.T) {
	t.Run("stores the user agent", func(t *testing.T) {
		got, present := serveUserAgent(t, "VibeXP-CLI/1.2.3")

		require.True(t, present)
		assert.Equal(t, "VibeXP-CLI/1.2.3", got)
	})

	t.Run("an empty user agent stores nothing", func(t *testing.T) {
		_, present := serveUserAgent(t, "")

		assert.False(t, present)
	})

	t.Run("caps an oversized user agent", func(t *testing.T) {
		got, present := serveUserAgent(t, strings.Repeat("a", audit.MaxUserAgentBytes+100))

		require.True(t, present)
		assert.Len(t, got, audit.MaxUserAgentBytes)
	})

	t.Run("a user agent at the cap is kept whole", func(t *testing.T) {
		ua := strings.Repeat("b", audit.MaxUserAgentBytes)
		got, _ := serveUserAgent(t, ua)

		assert.Equal(t, ua, got)
	})

	t.Run("the cap never splits a rune", func(t *testing.T) {
		// "é" is two bytes, so an odd cap would land inside one.
		ua := "x" + strings.Repeat("é", audit.MaxUserAgentBytes)
		got, present := serveUserAgent(t, ua)

		require.True(t, present)
		assert.True(t, utf8.ValidString(got), "the stored value must be valid UTF-8")
		assert.LessOrEqual(t, len(got), audit.MaxUserAgentBytes)
		assert.Equal(t, audit.MaxUserAgentBytes-1, len(got), "the cut backs off to the rune boundary")
	})
}
