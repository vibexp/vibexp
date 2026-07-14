package errors

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// NewValidationError creates a validation error with field-level errors
func NewValidationError(detail string, validationErrors []ValidationError) *APIError {
	return &APIError{
		Type:             typeURI(CodeValidationFailed),
		Title:            GetErrorTitle(CodeValidationFailed),
		Status:           http.StatusBadRequest,
		Detail:           detail,
		Code:             CodeValidationFailed,
		ValidationErrors: validationErrors,
	}
}

// NewAuthRequiredError creates an authentication required error
func NewAuthRequiredError(detail string) *APIError {
	return NewAPIError(CodeAuthRequired, GetErrorTitle(CodeAuthRequired), detail, http.StatusUnauthorized)
}

// NewAuthInvalidError creates an invalid authentication error
func NewAuthInvalidError(detail string) *APIError {
	return NewAPIError(CodeAuthInvalid, GetErrorTitle(CodeAuthInvalid), detail, http.StatusUnauthorized)
}

// NewInvalidCredentialsError creates an invalid credentials error
func NewInvalidCredentialsError(detail string) *APIError {
	return NewAPIError(CodeInvalidCredentials, GetErrorTitle(CodeInvalidCredentials), detail, http.StatusUnauthorized)
}

// NewForbiddenError creates a forbidden error
func NewForbiddenError(detail string) *APIError {
	return NewAPIError(CodeForbidden, GetErrorTitle(CodeForbidden), detail, http.StatusForbidden)
}

// NewAccessRestrictedError creates a 403 for a sign-in denied by the access
// allowlist, carrying the stable machine-readable code "access_restricted" the
// frontend branches on.
func NewAccessRestrictedError(detail string) *APIError {
	return NewAPIError(CodeAccessRestricted, GetErrorTitle(CodeAccessRestricted), detail, http.StatusForbidden)
}

// NewResourceNotFoundError creates a resource not found error
func NewResourceNotFoundError(resourceType, detail string) *APIError {
	if detail == "" {
		detail = resourceType + " not found"
	}
	return NewAPIError(CodeResourceNotFound, GetErrorTitle(CodeResourceNotFound), detail, http.StatusNotFound)
}

// NewResourceExistsError creates a resource already exists error
func NewResourceExistsError(resourceType, detail string) *APIError {
	if detail == "" {
		detail = resourceType + " already exists"
	}
	return NewAPIError(CodeResourceExists, GetErrorTitle(CodeResourceExists), detail, http.StatusConflict)
}

// NewVersionConflictError creates a version conflict error for optimistic locking
func NewVersionConflictError(resourceType, detail string) *APIError {
	if detail == "" {
		detail = resourceType + " was modified by another request. Please refresh and try again"
	}
	return NewAPIError(CodeVersionConflict, GetErrorTitle(CodeVersionConflict), detail, http.StatusConflict)
}

// NewResourceLimitExceededError creates a resource limit exceeded error
func NewResourceLimitExceededError(detail string) *APIError {
	return NewAPIError(CodeResourceLimitExceeded, GetErrorTitle(CodeResourceLimitExceeded), detail, http.StatusForbidden)
}

// NewResourceLimitExceededErrorWithMetadata creates a resource limit exceeded error with additional metadata
func NewResourceLimitExceededErrorWithMetadata(detail string, metadata map[string]any) *APIError {
	apiErr := NewAPIError(
		CodeResourceLimitExceeded,
		GetErrorTitle(CodeResourceLimitExceeded),
		detail,
		http.StatusForbidden,
	)
	apiErr.Metadata = metadata
	return apiErr
}

// NewInternalError creates an internal server error
func NewInternalError(detail string) *APIError {
	if detail == "" {
		detail = "An internal error occurred"
	}
	return NewAPIError(CodeInternalError, GetErrorTitle(CodeInternalError), detail, http.StatusInternalServerError)
}

// NewBadRequestError creates a bad request error
func NewBadRequestError(detail string) *APIError {
	return NewAPIError(CodeBadRequest, GetErrorTitle(CodeBadRequest), detail, http.StatusBadRequest)
}

// NewExternalServiceError creates an external service error
func NewExternalServiceError(service, detail string) *APIError {
	if detail == "" {
		detail = "External service " + service + " is unavailable"
	}
	return NewAPIError(CodeExternalServiceError, GetErrorTitle(CodeExternalServiceError), detail, http.StatusBadGateway)
}

// NewGoogleAuthError creates a Google authentication error.
//
// Deprecated: use NewIDPAuthError for provider-agnostic flows.
// Kept temporarily for any legacy callers.
func NewGoogleAuthError(detail string) *APIError {
	return NewAPIError(CodeGoogleAuthFailed, GetErrorTitle(CodeGoogleAuthFailed), detail, http.StatusUnauthorized)
}

// NewIDPAuthError creates a 401 error for identity-provider authentication
// failures (token exchange failure, invalid OIDC response, etc.).
// Provider-agnostic — use this for any new IDP integration.
func NewIDPAuthError(detail string) *APIError {
	return NewAPIError(CodeIDPAuthFailed, GetErrorTitle(CodeIDPAuthFailed), detail, http.StatusUnauthorized)
}

// NewServiceUnavailableError creates a 503 Service Unavailable error.
// Use for transient failures (upstream IDP outage, downstream timeout)
// where the client should retry. Distinct from 500 which signals a bug.
func NewServiceUnavailableError(detail string) *APIError {
	return NewAPIError(
		CodeServiceUnavailable,
		GetErrorTitle(CodeServiceUnavailable),
		detail,
		http.StatusServiceUnavailable,
	)
}

// NewDatabaseError creates a database error
func NewDatabaseError(detail string) *APIError {
	if detail == "" {
		detail = "Database operation failed"
	}
	return NewAPIError(CodeDatabaseError, GetErrorTitle(CodeDatabaseError), detail, http.StatusInternalServerError)
}

// NewDuplicateMembersError creates an error for duplicate team members with email list
func NewDuplicateMembersError(duplicateEmails []string) *APIError {
	detail := fmt.Sprintf("Users already in team: %s", strings.Join(duplicateEmails, ", "))
	return &APIError{
		Type:            typeURI(CodeDuplicateMembers),
		Title:           GetErrorTitle(CodeDuplicateMembers),
		Status:          http.StatusConflict,
		Detail:          detail,
		Code:            CodeDuplicateMembers,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		DuplicateEmails: duplicateEmails,
	}
}

// NewSubscriptionValidationError creates a subscription validation error
func NewSubscriptionValidationError(detail string, validationErrors []ValidationError) *APIError {
	return &APIError{
		Type:             typeURI(CodeSubscriptionValidationFailed),
		Title:            GetErrorTitle(CodeSubscriptionValidationFailed),
		Status:           http.StatusBadRequest,
		Detail:           detail,
		Code:             CodeSubscriptionValidationFailed,
		ValidationErrors: validationErrors,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	}
}

// NewSubscriptionNotFoundError creates a subscription not found error
func NewSubscriptionNotFoundError(detail string) *APIError {
	return NewAPIError(CodeSubscriptionNotFound, GetErrorTitle(CodeSubscriptionNotFound), detail, http.StatusNotFound)
}

// NewSubscriptionCreateFailedError creates a subscription creation failed error
func NewSubscriptionCreateFailedError(detail string) *APIError {
	return NewAPIError(
		CodeSubscriptionCreateFailed,
		GetErrorTitle(CodeSubscriptionCreateFailed),
		detail,
		http.StatusInternalServerError,
	)
}

// NewSubscriptionUpdateFailedError creates a subscription update failed error
func NewSubscriptionUpdateFailedError(detail string) *APIError {
	return NewAPIError(
		CodeSubscriptionUpdateFailed,
		GetErrorTitle(CodeSubscriptionUpdateFailed),
		detail,
		http.StatusInternalServerError,
	)
}

// NewSubscriptionCancelFailedError creates a subscription cancellation failed error
func NewSubscriptionCancelFailedError(detail string) *APIError {
	return NewAPIError(
		CodeSubscriptionCancelFailed,
		GetErrorTitle(CodeSubscriptionCancelFailed),
		detail,
		http.StatusInternalServerError,
	)
}

// NewSubscriptionAlreadyExistsError creates a subscription already exists error
func NewSubscriptionAlreadyExistsError(detail string) *APIError {
	return NewAPIError(
		CodeSubscriptionAlreadyExists,
		GetErrorTitle(CodeSubscriptionAlreadyExists),
		detail,
		http.StatusConflict,
	)
}

// NewInvalidStatusTransitionError creates an invalid status transition error
func NewInvalidStatusTransitionError(detail string) *APIError {
	return NewAPIError(
		CodeInvalidStatusTransition,
		GetErrorTitle(CodeInvalidStatusTransition),
		detail,
		http.StatusBadRequest,
	)
}

// NewUsageTrackingFailedError creates a usage tracking failed error
func NewUsageTrackingFailedError(detail string) *APIError {
	return NewAPIError(
		CodeUsageTrackingFailed,
		GetErrorTitle(CodeUsageTrackingFailed),
		detail,
		http.StatusInternalServerError,
	)
}

// NewWebhookProcessingFailedError creates a webhook processing failed error
func NewWebhookProcessingFailedError(detail string) *APIError {
	return NewAPIError(
		CodeWebhookProcessingFailed,
		GetErrorTitle(CodeWebhookProcessingFailed),
		detail,
		http.StatusInternalServerError,
	)
}

// NewWebhookParseError creates a webhook parse error with validation details
func NewWebhookParseError(detail string, validationErrors []ValidationError) *APIError {
	apiErr := NewAPIError(CodeWebhookParseFailed, GetErrorTitle(CodeWebhookParseFailed), detail, http.StatusBadRequest)
	apiErr.ValidationErrors = validationErrors
	return apiErr
}

// NewWebhookAuthError creates a webhook authentication error
func NewWebhookAuthError(detail string) *APIError {
	return NewAPIError(CodeWebhookAuthFailed, GetErrorTitle(CodeWebhookAuthFailed), detail, http.StatusUnauthorized)
}

// NewWebhookDataInvalidError creates a webhook data invalid error with validation details
func NewWebhookDataInvalidError(detail string, validationErrors []ValidationError) *APIError {
	apiErr := NewAPIError(CodeWebhookDataInvalid, GetErrorTitle(CodeWebhookDataInvalid), detail, http.StatusBadRequest)
	apiErr.ValidationErrors = validationErrors
	return apiErr
}

// NewWebhookHandlerError creates a webhook handler failed error
func NewWebhookHandlerError(handler, detail string) *APIError {
	fullDetail := fmt.Sprintf("Handler '%s' failed: %s", handler, detail)
	return NewAPIError(
		CodeWebhookHandlerFailed,
		GetErrorTitle(CodeWebhookHandlerFailed),
		fullDetail,
		http.StatusInternalServerError,
	)
}

// NewPaymentProcessingError creates a payment processing failed error
func NewPaymentProcessingError(detail string) *APIError {
	return NewAPIError(
		CodePaymentProcessingFailed,
		GetErrorTitle(CodePaymentProcessingFailed),
		detail,
		http.StatusBadGateway,
	)
}

// NewCancellationError creates a cancellation failed error
func NewCancellationError(detail string) *APIError {
	return NewAPIError(
		CodeCancellationFailed,
		GetErrorTitle(CodeCancellationFailed),
		detail,
		http.StatusInternalServerError,
	)
}

// Embedding Provider Errors

// NewProviderNotFoundError creates an embedding provider not found error
func NewProviderNotFoundError(providerID string) *APIError {
	detail := "Embedding provider not found"
	if providerID != "" {
		detail = fmt.Sprintf("Embedding provider with ID '%s' not found", providerID)
	}
	return NewAPIError(CodeProviderNotFound, GetErrorTitle(CodeProviderNotFound), detail, http.StatusNotFound)
}

// NewProviderAlreadyExistsError creates an embedding provider already exists error
func NewProviderAlreadyExistsError(providerName string) *APIError {
	detail := "Embedding provider already exists"
	if providerName != "" {
		detail = fmt.Sprintf("Embedding provider '%s' already exists", providerName)
	}
	return NewAPIError(CodeProviderAlreadyExists, GetErrorTitle(CodeProviderAlreadyExists), detail, http.StatusConflict)
}

// NewProviderCreateFailedError creates an embedding provider creation failed error
func NewProviderCreateFailedError(detail string) *APIError {
	if detail == "" {
		detail = "Failed to create embedding provider"
	}
	return NewAPIError(
		CodeProviderCreateFailed,
		GetErrorTitle(CodeProviderCreateFailed),
		detail,
		http.StatusInternalServerError,
	)
}

// NewProviderUpdateFailedError creates an embedding provider update failed error
func NewProviderUpdateFailedError(detail string) *APIError {
	if detail == "" {
		detail = "Failed to update embedding provider"
	}
	return NewAPIError(
		CodeProviderUpdateFailed,
		GetErrorTitle(CodeProviderUpdateFailed),
		detail,
		http.StatusInternalServerError,
	)
}

// NewProviderDeleteFailedError creates an embedding provider deletion failed error
func NewProviderDeleteFailedError(detail string) *APIError {
	if detail == "" {
		detail = "Failed to delete embedding provider"
	}
	return NewAPIError(
		CodeProviderDeleteFailed,
		GetErrorTitle(CodeProviderDeleteFailed),
		detail,
		http.StatusInternalServerError,
	)
}

// NewProviderLastDeleteBlockedError creates an error for attempting to delete the last provider
func NewProviderLastDeleteBlockedError() *APIError {
	return NewAPIError(
		CodeProviderLastDeleteBlocked,
		GetErrorTitle(CodeProviderLastDeleteBlocked),
		"Cannot delete the last embedding provider. Please add another provider first.",
		http.StatusBadRequest,
	)
}

// NewProviderValidationError creates an embedding provider validation error with field-level errors
func NewProviderValidationError(detail string, validationErrors []ValidationError) *APIError {
	return &APIError{
		Type:             typeURI(CodeProviderValidationFailed),
		Title:            GetErrorTitle(CodeProviderValidationFailed),
		Status:           http.StatusBadRequest,
		Detail:           detail,
		Code:             CodeProviderValidationFailed,
		ValidationErrors: validationErrors,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	}
}

// Model Provider Errors

// NewModelProviderNotFoundError creates a model provider not found error
func NewModelProviderNotFoundError(providerID string) *APIError {
	detail := "Model provider not found"
	if providerID != "" {
		detail = fmt.Sprintf("Model provider with ID '%s' not found", providerID)
	}
	return NewAPIError(
		CodeModelProviderNotFound,
		GetErrorTitle(CodeModelProviderNotFound),
		detail,
		http.StatusNotFound,
	)
}

// NewModelProviderAlreadyExistsError creates a model provider already exists error
func NewModelProviderAlreadyExistsError(providerName string) *APIError {
	detail := "Model provider already exists"
	if providerName != "" {
		detail = fmt.Sprintf("Model provider '%s' already exists", providerName)
	}
	return NewAPIError(
		CodeModelProviderAlreadyExists,
		GetErrorTitle(CodeModelProviderAlreadyExists),
		detail,
		http.StatusConflict,
	)
}

// NewModelProviderCreateFailedError creates a model provider creation failed error
func NewModelProviderCreateFailedError(detail string) *APIError {
	if detail == "" {
		detail = "Failed to create model provider"
	}
	return NewAPIError(
		CodeModelProviderCreateFailed,
		GetErrorTitle(CodeModelProviderCreateFailed),
		detail,
		http.StatusInternalServerError,
	)
}

// NewModelProviderUpdateFailedError creates a model provider update failed error
func NewModelProviderUpdateFailedError(detail string) *APIError {
	if detail == "" {
		detail = "Failed to update model provider"
	}
	return NewAPIError(
		CodeModelProviderUpdateFailed,
		GetErrorTitle(CodeModelProviderUpdateFailed),
		detail,
		http.StatusInternalServerError,
	)
}

// NewModelProviderDeleteFailedError creates a model provider deletion failed error
func NewModelProviderDeleteFailedError(detail string) *APIError {
	if detail == "" {
		detail = "Failed to delete model provider"
	}
	return NewAPIError(
		CodeModelProviderDeleteFailed,
		GetErrorTitle(CodeModelProviderDeleteFailed),
		detail,
		http.StatusInternalServerError,
	)
}

// NewModelProviderLastDeleteBlockedError creates an error for attempting to delete the last provider
func NewModelProviderLastDeleteBlockedError() *APIError {
	return NewAPIError(
		CodeModelProviderLastDeleteBlocked,
		GetErrorTitle(CodeModelProviderLastDeleteBlocked),
		"Cannot delete the last model provider. Please add another provider first.",
		http.StatusBadRequest,
	)
}

// NewModelProviderValidationError creates a model provider validation error with field-level errors
func NewModelProviderValidationError(detail string, validationErrors []ValidationError) *APIError {
	return &APIError{
		Type:             typeURI(CodeModelProviderValidationFailed),
		Title:            GetErrorTitle(CodeModelProviderValidationFailed),
		Status:           http.StatusBadRequest,
		Detail:           detail,
		Code:             CodeModelProviderValidationFailed,
		ValidationErrors: validationErrors,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	}
}

// User Preferences Errors

// NewPreferencesNotFoundError creates a user preferences not found error
func NewPreferencesNotFoundError(detail string) *APIError {
	if detail == "" {
		detail = "User preferences not found"
	}
	return NewAPIError(CodePreferencesNotFound, GetErrorTitle(CodePreferencesNotFound), detail, http.StatusNotFound)
}

// NewPreferencesUpdateFailedError creates a user preferences update failed error
func NewPreferencesUpdateFailedError(detail string) *APIError {
	if detail == "" {
		detail = "Failed to update user preferences"
	}
	return NewAPIError(
		CodePreferencesUpdateFailed,
		GetErrorTitle(CodePreferencesUpdateFailed),
		detail,
		http.StatusInternalServerError,
	)
}

// NewDateValidationError creates a date validation error with format example
func NewDateValidationError(fieldName, providedValue string) *APIError {
	detail := fmt.Sprintf(
		"Invalid '%s' date format. Expected YYYY-MM-DD format, got '%s'. Example: '2024-01-15'",
		fieldName,
		providedValue,
	)
	validationErrors := []ValidationError{
		{
			Field:      fieldName,
			Message:    fmt.Sprintf("Invalid date format, expected YYYY-MM-DD, got '%s'", providedValue),
			Code:       "INVALID_FORMAT",
			Constraint: "YYYY-MM-DD",
		},
	}
	return NewValidationError(detail, validationErrors)
}

// NewDateRangeError creates a date range validation error
func NewDateRangeError() *APIError {
	detail := "'from' date must be before 'to' date"
	validationErrors := []ValidationError{
		{
			Field:   "from",
			Message: "'from' date must be before 'to' date",
			Code:    "INVALID_RANGE",
		},
		{
			Field:   "to",
			Message: "'to' date must be after 'from' date",
			Code:    "INVALID_RANGE",
		},
	}
	return NewValidationError(detail, validationErrors)
}
