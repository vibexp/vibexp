package errors

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

// ConvertValidatorErrors converts go-playground/validator errors to ValidationError slice
func ConvertValidatorErrors(err error) []ValidationError {
	var validationErrors []ValidationError

	if ve, ok := err.(validator.ValidationErrors); ok {
		for _, fe := range ve {
			validationErrors = append(validationErrors, ValidationError{
				Field:      strings.ToLower(fe.Field()[:1]) + fe.Field()[1:], // camelCase
				Message:    getValidationMessage(fe),
				Code:       strings.ToUpper(fe.Tag()),
				Constraint: fe.Param(),
			})
		}
	}

	return validationErrors
}

// getValidationMessage returns a human-readable message for a validation error
//
//nolint:gocyclo // This function has many validation cases by design
func getValidationMessage(fe validator.FieldError) string {
	field := fe.Field()

	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("Field '%s' is required", field)
	case "email":
		return fmt.Sprintf("Field '%s' must be a valid email address", field)
	case "min":
		return fmt.Sprintf("Field '%s' must be at least %s characters long", field, fe.Param())
	case "max":
		return fmt.Sprintf("Field '%s' must be at most %s characters long", field, fe.Param())
	case "len":
		return fmt.Sprintf("Field '%s' must be exactly %s characters long", field, fe.Param())
	case "oneof":
		return fmt.Sprintf("Field '%s' must be one of: %s", field, fe.Param())
	case "url":
		return fmt.Sprintf("Field '%s' must be a valid URL", field)
	case "uuid":
		return fmt.Sprintf("Field '%s' must be a valid UUID", field)
	case "numeric":
		return fmt.Sprintf("Field '%s' must be numeric", field)
	case "alpha":
		return fmt.Sprintf("Field '%s' must contain only letters", field)
	case "alphanum":
		return fmt.Sprintf("Field '%s' must contain only letters and numbers", field)
	case "gte":
		return fmt.Sprintf("Field '%s' must be greater than or equal to %s", field, fe.Param())
	case "lte":
		return fmt.Sprintf("Field '%s' must be less than or equal to %s", field, fe.Param())
	case "gt":
		return fmt.Sprintf("Field '%s' must be greater than %s", field, fe.Param())
	case "lt":
		return fmt.Sprintf("Field '%s' must be less than %s", field, fe.Param())
	default:
		return fmt.Sprintf("Field '%s' failed validation: %s", field, fe.Tag())
	}
}

// NewFieldValidationError creates a single field validation error
func NewFieldValidationError(field, message, code string) ValidationError {
	return ValidationError{
		Field:   field,
		Message: message,
		Code:    code,
	}
}

// NewRequiredFieldError creates a required field validation error
func NewRequiredFieldError(field string) ValidationError {
	return ValidationError{
		Field:      field,
		Message:    fmt.Sprintf("Field '%s' is required", field),
		Code:       "REQUIRED",
		Constraint: "required",
	}
}

// NewMaxLengthError creates a max length validation error
func NewMaxLengthError(field string, maxLength int) ValidationError {
	return ValidationError{
		Field:      field,
		Message:    fmt.Sprintf("Field '%s' cannot be longer than %d characters", field, maxLength),
		Code:       "MAX",
		Constraint: fmt.Sprintf("%d", maxLength),
	}
}

// NewInvalidValueError creates an invalid value validation error
func NewInvalidValueError(field string, allowedValues []string) ValidationError {
	return ValidationError{
		Field:      field,
		Message:    fmt.Sprintf("Field '%s' must be one of: %s", field, strings.Join(allowedValues, ", ")),
		Code:       "INVALID_VALUE",
		Constraint: strings.Join(allowedValues, ","),
	}
}
