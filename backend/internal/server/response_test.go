package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteJSON(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   any
	}{
		{name: "ok", status: http.StatusOK, body: map[string]string{"key": "value"}},
		{name: "created", status: http.StatusCreated, body: map[string]int{"id": 7}},
		{name: "accepted", status: http.StatusAccepted, body: []string{"a", "b"}},
		{name: "nil body encodes as JSON null", status: http.StatusOK, body: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger, _ := test.NewNullLogger()
			rr := httptest.NewRecorder()

			writeJSON(rr, tc.status, tc.body, logger)

			assert.Equal(t, tc.status, rr.Code)
			assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

			want, err := json.Marshal(tc.body)
			require.NoError(t, err)
			assert.JSONEq(t, string(want), rr.Body.String())
		})
	}
}

func TestWriteOK(t *testing.T) {
	logger, _ := test.NewNullLogger()
	rr := httptest.NewRecorder()

	writeOK(rr, map[string]string{"message": "ok"}, logger)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var got map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, "ok", got["message"])
}

func TestWriteCreated(t *testing.T) {
	logger, _ := test.NewNullLogger()
	rr := httptest.NewRecorder()

	writeCreated(rr, map[string]string{"id": "abc"}, logger)

	assert.Equal(t, http.StatusCreated, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var got map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, "abc", got["id"])
}

func TestWriteNoContent(t *testing.T) {
	rr := httptest.NewRecorder()

	writeNoContent(rr)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Empty(t, rr.Body.String())
	assert.Empty(t, rr.Header().Get("Content-Type"))
}

func TestWriteJSON_EncodeFailureLogsError(t *testing.T) {
	logger, hook := test.NewNullLogger()
	rr := httptest.NewRecorder()

	// A channel cannot be JSON-encoded, forcing json.Encoder.Encode to fail.
	writeJSON(rr, http.StatusOK, make(chan int), logger)

	entry := hook.LastEntry()
	require.NotNil(t, entry)
	assert.Equal(t, logrus.ErrorLevel, entry.Level)
	assert.Equal(t, "Failed to encode response", entry.Message)
	assert.Error(t, entry.Data[logrus.ErrorKey].(error))
}
