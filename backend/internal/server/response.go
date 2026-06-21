package server

import (
	"encoding/json"
	"net/http"

	"github.com/sirupsen/logrus"
)

// writeJSON serializes v as a JSON success response with the given status code.
// It mirrors the error path in errors.WriteJSONError but only logs encode
// failures (the status line is already committed once WriteHeader runs, so a
// fallback http.Error would be a no-op).
//
// logger must be non-nil; handlers pass s.logger.
func writeJSON(w http.ResponseWriter, status int, v any, logger *logrus.Logger) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.WithError(err).Error("Failed to encode response")
	}
}

// writeOK writes v as a 200 OK JSON response.
func writeOK(w http.ResponseWriter, v any, logger *logrus.Logger) {
	writeJSON(w, http.StatusOK, v, logger)
}

// writeCreated writes v as a 201 Created JSON response.
func writeCreated(w http.ResponseWriter, v any, logger *logrus.Logger) {
	writeJSON(w, http.StatusCreated, v, logger)
}

// writeNoContent writes a 204 No Content response with no body.
func writeNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
