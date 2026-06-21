package errors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/contextkeys"
)

// newRequestWithTestLogger creates an HTTP request with a test logger injected into context.
// The returned hook can be used to inspect log entries captured during the request.
func newRequestWithTestLogger(t *testing.T) (*http.Request, *test.Hook) {
	t.Helper()
	logger, hook := test.NewNullLogger()
	logger.SetLevel(logrus.DebugLevel)

	req := httptest.NewRequest(http.MethodGet, "/test/path", nil)
	ctx := context.WithValue(req.Context(), contextkeys.Logger, logrus.NewEntry(logger))
	return req.WithContext(ctx), hook
}

func TestWriteJSONError_5xxLogsAtError(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"500 InternalServerError", http.StatusInternalServerError},
		{"502 BadGateway", http.StatusBadGateway},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, hook := newRequestWithTestLogger(t)
			WriteJSONError(httptest.NewRecorder(), req, &APIError{
				Type: "https://api.vibexp.io/errors/test", Title: "Server Error",
				Status: tt.status, Detail: "detail", Code: "CODE",
			})
			require.NotEmpty(t, hook.Entries)
			assert.Equal(t, logrus.ErrorLevel, hook.Entries[len(hook.Entries)-1].Level)
		})
	}
}

func TestWriteJSONError_AuthAndNotFoundLogsAtInfo(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"401 Unauthorized", http.StatusUnauthorized},
		{"403 Forbidden", http.StatusForbidden},
		{"404 NotFound", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, hook := newRequestWithTestLogger(t)
			WriteJSONError(httptest.NewRecorder(), req, &APIError{
				Type: "https://api.vibexp.io/errors/test", Title: "Client Error",
				Status: tt.status, Detail: "detail", Code: "CODE",
			})
			require.NotEmpty(t, hook.Entries)
			assert.Equal(t, logrus.InfoLevel, hook.Entries[len(hook.Entries)-1].Level)
		})
	}
}

func TestWriteJSONError_Other4xxLogsAtWarn(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"400 BadRequest", http.StatusBadRequest},
		{"409 Conflict", http.StatusConflict},
		{"422 UnprocessableEntity", http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, hook := newRequestWithTestLogger(t)
			WriteJSONError(httptest.NewRecorder(), req, &APIError{
				Type: "https://api.vibexp.io/errors/test", Title: "Client Error",
				Status: tt.status, Detail: "detail", Code: "CODE",
			})
			require.NotEmpty(t, hook.Entries)
			assert.Equal(t, logrus.WarnLevel, hook.Entries[len(hook.Entries)-1].Level)
		})
	}
}

func TestWriteJSONError_DefaultLogsAtInfo(t *testing.T) {
	// Defensive: non-error status codes (shouldn't happen but handled gracefully)
	req, hook := newRequestWithTestLogger(t)
	WriteJSONError(httptest.NewRecorder(), req, &APIError{
		Type: "https://api.vibexp.io/errors/test", Title: "OK",
		Status: http.StatusOK, Detail: "detail", Code: "CODE",
	})
	require.NotEmpty(t, hook.Entries)
	assert.Equal(t, logrus.InfoLevel, hook.Entries[len(hook.Entries)-1].Level)
}

func TestWriteJSONError_LogFields(t *testing.T) {
	req, hook := newRequestWithTestLogger(t)
	w := httptest.NewRecorder()

	apiErr := &APIError{
		Type:   "https://api.vibexp.io/errors/test",
		Title:  "Test Error",
		Status: http.StatusInternalServerError,
		Detail: "something went wrong",
		Code:   "TEST_ERROR",
	}

	WriteJSONError(w, req, apiErr)

	require.NotEmpty(t, hook.Entries)
	entry := hook.Entries[len(hook.Entries)-1]

	assert.Equal(t, "TEST_ERROR", entry.Data["error_code"], "error_code field should be set")
	assert.Equal(t, "something went wrong", entry.Data["error_detail"], "error_detail field should be set")
	assert.Equal(t, http.StatusInternalServerError, entry.Data["status"], "status field should be set")
}

func TestWriteJSONError_ResponseShape(t *testing.T) {
	req, _ := newRequestWithTestLogger(t)
	w := httptest.NewRecorder()

	apiErr := NewAPIError("BAD_REQUEST", "Bad Request", "request body is invalid", http.StatusBadRequest)

	WriteJSONError(w, req, apiErr)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), `"BAD_REQUEST"`)
	assert.Contains(t, w.Body.String(), `"Bad Request"`)
}

func TestWriteJSONError_SetsInstanceAndTimestamp(t *testing.T) {
	req, _ := newRequestWithTestLogger(t)
	w := httptest.NewRecorder()

	apiErr := &APIError{
		Type:   "https://api.vibexp.io/errors/test",
		Title:  "Test",
		Status: http.StatusNotFound,
		Detail: "not found",
		Code:   "NOT_FOUND",
	}

	WriteJSONError(w, req, apiErr)

	assert.Equal(t, "/test/path", apiErr.Instance, "Instance should be set to request path")
	assert.NotEmpty(t, apiErr.Timestamp, "Timestamp should be set")
}

func TestAPIError_Error(t *testing.T) {
	// *APIError must satisfy the error interface so strict-server handlers can
	// return it directly (#1768). The string is "<code>: <detail>" for logs.
	var err error = NewBadRequestError("limit must be an integer between 1 and 100")
	assert.EqualError(t, err, "BAD_REQUEST: limit must be an integer between 1 and 100")

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr, "errors.As must recover the concrete *APIError")
	assert.Equal(t, CodeBadRequest, apiErr.Code)
}
