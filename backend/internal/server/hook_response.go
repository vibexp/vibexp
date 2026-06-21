package server

import (
	"net/http"

	"github.com/sirupsen/logrus"
)

// respondWithHookError writes a JSON error response in the hook wire shape
// {"status":"error","message":...} consumed by the vibexp CLI clients.
//
// The hooks endpoints intentionally keep this legacy shape rather than the
// RFC 9457 errors used elsewhere; see issue #1589. logger must be non-nil;
// handlers pass s.logger.
func respondWithHookError(w http.ResponseWriter, status int, message string, logger *logrus.Logger) {
	writeJSON(w, status, map[string]any{"status": "error", "message": message}, logger)
}
