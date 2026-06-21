package errors

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/contextkeys"
)

// typeBaseURI is the base used to build the RFC 9457 "type" member of error
// responses, joined with the error code as "<base>/<code>". It defaults to the
// neutral "about:blank" (a valid problem "type" that signals "no further
// information"). Set it once at startup via SetTypeBaseURI (wired from
// config.ErrorTypeBaseURI) to point clients at hosted error-code docs.
//
// It is a package-level var rather than threaded through every constructor to
// keep the error constructors dependency-free; it is written exactly once
// during config load, before any request is served.
var typeBaseURI = "about:blank"

// SetTypeBaseURI configures the base URI used to build RFC 9457 "type" members.
// An empty value resets to the neutral "about:blank". Call this once at startup.
func SetTypeBaseURI(base string) {
	if base == "" {
		base = "about:blank"
	}
	typeBaseURI = base
}

// typeURI builds the RFC 9457 "type" URI for an error code. When the base is the
// neutral "about:blank" the code is not appended (about:blank is only meaningful
// on its own), otherwise the code is joined as "<base>/<code>".
func typeURI(code string) string {
	if typeBaseURI == "about:blank" {
		return "about:blank"
	}
	return typeBaseURI + "/" + code
}

// ValidationError represents a single validation error for a field
type ValidationError struct {
	Field      string `json:"field"`
	Message    string `json:"message"`
	Code       string `json:"code"`
	Constraint string `json:"constraint,omitempty"`
}

// APIError represents RFC 9457 compliant error response
type APIError struct {
	Type             string            `json:"type"`
	Title            string            `json:"title"`
	Status           int               `json:"status"`
	Detail           string            `json:"detail"`
	Code             string            `json:"code"`
	RequestID        string            `json:"request_id"`
	Timestamp        string            `json:"timestamp"`
	Instance         string            `json:"instance,omitempty"`
	ValidationErrors []ValidationError `json:"validation_errors,omitempty"`
	DuplicateEmails  []string          `json:"duplicate_emails,omitempty"`
	Metadata         map[string]any    `json:"metadata,omitempty"`
}

// Error makes *APIError satisfy the error interface so it can be returned
// directly from oapi-codegen strict-server handlers (and any other error
// path) instead of each domain wrapping it in a bespoke adapter type. The
// string is for logs only — the wire response is rendered by WriteJSONError
// as RFC 9457 application/problem+json, not from this value.
func (e *APIError) Error() string {
	return e.Code + ": " + e.Detail
}

// WriteJSONError writes an RFC 9457 compliant error response
func WriteJSONError(w http.ResponseWriter, r *http.Request, apiErr *APIError) {
	// Get request ID from context
	if apiErr.RequestID == "" {
		if reqID := contextkeys.GetRequestID(r.Context()); reqID != "" {
			apiErr.RequestID = reqID
		}
	}

	// Set timestamp if not already set
	if apiErr.Timestamp == "" {
		apiErr.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	// Set instance to the request path if not already set
	if apiErr.Instance == "" {
		apiErr.Instance = r.URL.Path
	}

	// Log the error with context logger at severity matching HTTP status.
	// 5xx → ERROR (server faults), 401/403/404 → INFO (expected client conditions),
	// other 4xx → WARN (client errors worth noting), others → INFO (defensive default).
	logger := contextkeys.GetLoggerFromContext(r.Context())
	logEvent := logger.WithFields(logrus.Fields{
		"error_code":   apiErr.Code,
		"error_detail": apiErr.Detail,
		"status":       apiErr.Status,
		"instance":     apiErr.Instance,
	})

	switch {
	case apiErr.Status >= 500:
		logEvent.Error(apiErr.Title)
	case apiErr.Status == http.StatusUnauthorized,
		apiErr.Status == http.StatusForbidden,
		apiErr.Status == http.StatusNotFound:
		logEvent.Info(apiErr.Title)
	case apiErr.Status >= 400:
		logEvent.Warn(apiErr.Title)
	default:
		logEvent.Info(apiErr.Title)
	}

	// Set content type and status code
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(apiErr.Status)

	// Write JSON response
	if err := json.NewEncoder(w).Encode(apiErr); err != nil {
		// If encoding fails, write a fallback error
		logger.WithError(err).Error("Failed to encode error response")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// NewAPIError creates a new APIError with required fields
func NewAPIError(code string, title string, detail string, status int) *APIError {
	return &APIError{
		Type:      typeURI(code),
		Title:     title,
		Status:    status,
		Detail:    detail,
		Code:      code,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}
