package errors

// Error codes registry following a consistent naming convention
const (
	// Authentication errors
	CodeAuthRequired = "AUTH_REQUIRED"
	CodeAuthInvalid  = "AUTH_INVALID"
	CodeAuthExpired  = "AUTH_EXPIRED"
	// CodeInvalidCredentials represents an invalid credentials error code (not a credential itself)
	// #nosec G101 -- This is an error code constant, not a hardcoded credential
	CodeInvalidCredentials = "INVALID_CREDENTIALS"

	// Authorization errors
	CodeForbidden    = "FORBIDDEN"
	CodeUnauthorized = "UNAUTHORIZED"

	// Validation errors
	CodeValidationFailed = "VALIDATION_FAILED"
	CodeInvalidRequest   = "INVALID_REQUEST"
	CodeInvalidFormat    = "INVALID_FORMAT"

	// Resource errors
	CodeResourceNotFound = "RESOURCE_NOT_FOUND"
	CodeResourceExists   = "RESOURCE_EXISTS"
	CodeResourceConflict = "RESOURCE_CONFLICT"
	CodeVersionConflict  = "VERSION_CONFLICT"
	CodeDuplicateMembers = "DUPLICATE_MEMBERS"

	// Rate limiting and quotas
	CodeRateLimitExceeded     = "RATE_LIMIT_EXCEEDED"
	CodeResourceLimitExceeded = "RESOURCE_LIMIT_EXCEEDED"
	CodeQuotaExceeded         = "QUOTA_EXCEEDED"

	// Server errors
	CodeInternalError      = "INTERNAL_ERROR"
	CodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	CodeDatabaseError      = "DATABASE_ERROR"

	// External service errors
	CodeExternalServiceError = "EXTERNAL_SERVICE_ERROR"
	CodeGoogleAuthFailed     = "GOOGLE_AUTH_FAILED"
	CodeIDPAuthFailed        = "IDP_AUTH_FAILED"

	// Request errors
	CodeBadRequest       = "BAD_REQUEST"
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	CodeNotImplemented   = "NOT_IMPLEMENTED"

	// Subscription errors
	CodeSubscriptionValidationFailed = "SUBSCRIPTION_VALIDATION_FAILED"
	CodeSubscriptionNotFound         = "SUBSCRIPTION_NOT_FOUND"
	CodeSubscriptionCreateFailed     = "SUBSCRIPTION_CREATE_FAILED"
	CodeSubscriptionUpdateFailed     = "SUBSCRIPTION_UPDATE_FAILED"
	CodeSubscriptionCancelFailed     = "SUBSCRIPTION_CANCEL_FAILED"
	CodeSubscriptionAlreadyExists    = "SUBSCRIPTION_ALREADY_EXISTS"
	CodeInvalidStatusTransition      = "INVALID_STATUS_TRANSITION"
	CodeUsageTrackingFailed          = "USAGE_TRACKING_FAILED"

	// Webhook errors
	CodeWebhookParseFailed      = "WEBHOOK_PARSE_FAILED"
	CodeWebhookAuthFailed       = "WEBHOOK_AUTH_FAILED"
	CodeWebhookDataInvalid      = "WEBHOOK_DATA_INVALID"
	CodeWebhookHandlerFailed    = "WEBHOOK_HANDLER_FAILED"
	CodeWebhookProcessingFailed = "WEBHOOK_PROCESSING_FAILED"
	CodePaymentProcessingFailed = "PAYMENT_PROCESSING_FAILED"
	CodeCancellationFailed      = "CANCELLATION_FAILED"

	// Embedding provider errors
	CodeProviderNotFound          = "PROVIDER_NOT_FOUND"
	CodeProviderAlreadyExists     = "PROVIDER_ALREADY_EXISTS"
	CodeProviderCreateFailed      = "PROVIDER_CREATE_FAILED"
	CodeProviderUpdateFailed      = "PROVIDER_UPDATE_FAILED"
	CodeProviderDeleteFailed      = "PROVIDER_DELETE_FAILED"
	CodeProviderLastDeleteBlocked = "PROVIDER_LAST_DELETE_BLOCKED"
	CodeProviderValidationFailed  = "PROVIDER_VALIDATION_FAILED"

	// Preferences errors
	CodePreferencesNotFound     = "PREFERENCES_NOT_FOUND"
	CodePreferencesUpdateFailed = "PREFERENCES_UPDATE_FAILED"
)

// Error titles for common error codes
// #nosec G101 - Error message strings, not actual credentials
var errorTitles = map[string]string{
	CodeAuthRequired:                 "Authentication Required",
	CodeAuthInvalid:                  "Invalid Authentication",
	CodeAuthExpired:                  "Authentication Expired",
	CodeInvalidCredentials:           "Invalid Credentials",
	CodeForbidden:                    "Forbidden",
	CodeUnauthorized:                 "Unauthorized",
	CodeValidationFailed:             "Validation Failed",
	CodeInvalidRequest:               "Invalid Request",
	CodeInvalidFormat:                "Invalid Format",
	CodeResourceNotFound:             "Resource Not Found",
	CodeResourceExists:               "Resource Already Exists",
	CodeResourceConflict:             "Resource Conflict",
	CodeVersionConflict:              "Version Conflict",
	CodeDuplicateMembers:             "Duplicate Team Members",
	CodeRateLimitExceeded:            "Rate Limit Exceeded",
	CodeResourceLimitExceeded:        "Resource Limit Exceeded",
	CodeQuotaExceeded:                "Quota Exceeded",
	CodeInternalError:                "Internal Server Error",
	CodeServiceUnavailable:           "Service Unavailable",
	CodeDatabaseError:                "Database Error",
	CodeExternalServiceError:         "External Service Error",
	CodeGoogleAuthFailed:             "Google Authentication Failed",
	CodeIDPAuthFailed:                "Identity Provider Authentication Failed",
	CodeBadRequest:                   "Bad Request",
	CodeMethodNotAllowed:             "Method Not Allowed",
	CodeNotImplemented:               "Not Implemented",
	CodeSubscriptionValidationFailed: "Subscription Validation Failed",
	CodeSubscriptionNotFound:         "Subscription Not Found",
	CodeSubscriptionCreateFailed:     "Subscription Creation Failed",
	CodeSubscriptionUpdateFailed:     "Subscription Update Failed",
	CodeSubscriptionCancelFailed:     "Subscription Cancellation Failed",
	CodeSubscriptionAlreadyExists:    "Subscription Already Exists",
	CodeInvalidStatusTransition:      "Invalid Status Transition",
	CodeUsageTrackingFailed:          "Usage Tracking Failed",
	CodeWebhookParseFailed:           "Webhook Parse Failed",
	CodeWebhookAuthFailed:            "Webhook Authentication Failed",
	CodeWebhookDataInvalid:           "Webhook Data Invalid",
	CodeWebhookHandlerFailed:         "Webhook Handler Failed",
	CodeWebhookProcessingFailed:      "Webhook Processing Failed",
	CodePaymentProcessingFailed:      "Payment Processing Failed",
	CodeCancellationFailed:           "Cancellation Failed",
	CodeProviderNotFound:             "Embedding Provider Not Found",
	CodeProviderAlreadyExists:        "Embedding Provider Already Exists",
	CodeProviderCreateFailed:         "Embedding Provider Creation Failed",
	CodeProviderUpdateFailed:         "Embedding Provider Update Failed",
	CodeProviderDeleteFailed:         "Embedding Provider Deletion Failed",
	CodeProviderLastDeleteBlocked:    "Cannot Delete Last Embedding Provider",
	CodeProviderValidationFailed:     "Embedding Provider Validation Failed",
	CodePreferencesNotFound:          "User Preferences Not Found",
	CodePreferencesUpdateFailed:      "User Preferences Update Failed",
}

// GetErrorTitle returns the title for a given error code
func GetErrorTitle(code string) string {
	if title, ok := errorTitles[code]; ok {
		return title
	}
	return "Error"
}
