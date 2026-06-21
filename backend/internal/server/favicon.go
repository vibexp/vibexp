package server

import (
	_ "embed"
	"net/http"

	"github.com/vibexp/vibexp/internal/contextkeys"
)

// faviconICO is the brand favicon (black square, white activity pulse).
// When a public host maps directly to this backend, browsers that open it
// (e.g. during the MCP OAuth flow) request /favicon.ico; serving the brand
// icon avoids a 404 and a default browser glyph.
//
//go:embed favicon.ico
var faviconICO []byte

// handleFavicon serves the embedded brand favicon.
func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(faviconICO); err != nil {
		contextkeys.GetLoggerFromContext(r.Context()).WithError(err).Error("Failed to write favicon response")
	}
}
