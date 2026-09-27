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
	Fields []string
	// Code classifies the rule that failed: SettingsFieldOutOfRange or
	// SettingsFieldInvalidValue.
	Code    string
	Message string
}

// Classes of settings validation failure, carried as SettingsFieldError.Code
// and published as the validation error's code.
const (
	// SettingsFieldOutOfRange is a value outside its numeric bounds.
	SettingsFieldOutOfRange = "OUT_OF_RANGE"
	// SettingsFieldInvalidValue is a value that is not allowed at all, such
	// as an unknown enum member or a combination the rules reject.
	SettingsFieldInvalidValue = "INVALID_VALUE"
)

// Error implements error.
func (e *SettingsFieldError) Error() string { return e.Message }

// settingsFieldError builds an out-of-range SettingsFieldError with a
// formatted message.
func settingsFieldError(fields []string, format string, args ...any) *SettingsFieldError {
	return &SettingsFieldError{Fields: fields, Code: SettingsFieldOutOfRange, Message: fmt.Sprintf(format, args...)}
}

// settingsInvalidValueError builds an invalid-value SettingsFieldError with a
// formatted message.
func settingsInvalidValueError(fields []string, format string, args ...any) *SettingsFieldError {
	return &SettingsFieldError{Fields: fields, Code: SettingsFieldInvalidValue, Message: fmt.Sprintf(format, args...)}
}
