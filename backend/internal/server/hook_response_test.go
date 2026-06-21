package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

func newTestLogger() *logrus.Logger {
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	return logger
}

// TestRespondWithHookError locks the {"status":"error","message":...} wire
// shape and status code consumed by the vibexp CLI clients.
func TestRespondWithHookError(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		message string
	}{
		{"bad request", http.StatusBadRequest, "Invalid JSON payload"},
		{"not found", http.StatusNotFound, "Session not found or access denied"},
		{"internal error", http.StatusInternalServerError, "Failed to check session"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()

			respondWithHookError(rr, tt.status, tt.message, newTestLogger())

			assert.Equal(t, tt.status, rr.Code)
			assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

			var body map[string]any
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
			assert.Equal(t, map[string]any{
				"status":  "error",
				"message": tt.message,
			}, body)
		})
	}
}

// TestHookLimitErrorShape locks the {status,message,details} shape emitted by
// the 403 resource-limit responses in the hook handlers.
func TestHookLimitErrorShape(t *testing.T) {
	rr := httptest.NewRecorder()

	writeJSON(rr, http.StatusForbidden, map[string]any{
		"status":  "error",
		"message": "AI session limit reached for your subscription plan",
		"details": map[string]any{
			"resource_type": "ai_session",
			"current_usage": 5,
			"limit":         5,
		},
	}, newTestLogger())

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "error", body["status"])
	assert.Equal(t, "AI session limit reached for your subscription plan", body["message"])

	details, ok := body["details"].(map[string]any)
	require.True(t, ok, "details must be an object")
	assert.Equal(t, "ai_session", details["resource_type"])
	assert.Contains(t, details, "current_usage")
	assert.Contains(t, details, "limit")
}

// TestRespondWithHookSuccess locks the 201 hook-creation wire shape
// {status,message,data{id,created_at,updated_at}}.
func TestRespondWithHookSuccess(t *testing.T) {
	rr := httptest.NewRecorder()

	respondWithHookSuccess(rr, map[string]any{
		"status":  "success",
		"message": "Hook payloads retrieved successfully",
	}, newTestLogger())

	// hookPayload that is not a known model yields a zero-valued data object,
	// but the envelope keys must remain stable.
	assert.Equal(t, http.StatusCreated, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "success", body["status"])
	assert.Equal(t, "Hook payload stored successfully", body["message"])
	require.Contains(t, body, "data")
	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "data must be an object")
	assert.Contains(t, data, "id")
	assert.Contains(t, data, "created_at")
	assert.Contains(t, data, "updated_at")
}

// TestRespondWithHookSuccess_ModelPayload locks the id/timestamp population
// and RFC 3339 formatting for a real model payload.
func TestRespondWithHookSuccess_ModelPayload(t *testing.T) {
	rr := httptest.NewRecorder()
	created := time.Date(2026, 6, 4, 8, 30, 0, 0, time.UTC)
	updated := time.Date(2026, 6, 4, 9, 45, 0, 0, time.UTC)

	respondWithHookSuccess(rr, &models.ClaudeCodeHookPayload{
		ID:        42,
		CreatedAt: created,
		UpdatedAt: updated,
	}, newTestLogger())

	assert.Equal(t, http.StatusCreated, rr.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "data must be an object")
	assert.Equal(t, float64(42), data["id"])
	assert.Equal(t, "2026-06-04T08:30:00Z", data["created_at"])
	assert.Equal(t, "2026-06-04T09:45:00Z", data["updated_at"])
}

// TestRespondWithJSON locks the 200 success envelope used by the hook GET
// endpoints.
func TestRespondWithJSON(t *testing.T) {
	rr := httptest.NewRecorder()

	respondWithJSON(rr, map[string]any{
		"status":  "success",
		"message": "Sessions retrieved successfully",
		"data":    []string{},
	}, newTestLogger())

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "success", body["status"])
	assert.Equal(t, "Sessions retrieved successfully", body["message"])
	assert.Contains(t, body, "data")
}
