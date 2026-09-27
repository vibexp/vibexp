package services

import "fmt"

// SettingsFieldError attributes a settings validation failure to the request
// fields that caused it, so an endpoint can answer with field-level
// validation_errors instead of parsing a message (#1200).
//
// Its Error() is the plain message, so wrapping it under a sentinel
// (fmt.Errorf("%w: %w", sentinel, fieldErr)) reads exactly like the
// "sentinel: message" errors the validators returned before; errors.As finds
// it through any wrapping.
type SettingsFieldError struct {
	// Fields are the request field names at fault. A rule spanning several
	// fields (e.g. "the weights must not all be zero") names all of them.
	Fields  []string
	Message string
}

// Error implements error.
func (e *SettingsFieldError) Error() string { return e.Message }

// settingsFieldError builds a SettingsFieldError with a formatted message.
func settingsFieldError(fields []string, format string, args ...any) *SettingsFieldError {
	return &SettingsFieldError{Fields: fields, Message: fmt.Sprintf(format, args...)}
}
