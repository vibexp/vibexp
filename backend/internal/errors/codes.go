package errors

// Error codes registry following a consistent naming convention
const (
	// Authentication errors
	CodeAuthRequired = "AUTH_REQUIRED"
	CodeAuthInvalid  = "AUTH_INVALID"
	CodeAuthExpired  = "AUTH_EXPIRED"

	// Authorization errors
	CodeForbidden    = "FORBIDDEN"
	CodeUnauthorized = "UNAUTHORIZED"
	// CodeAccessRestricted signals a sign-in denied by the access allowlist. Its
	// lowercase value is a stable contract shared with the frontend (the OAuth
	// callback also redirects with ?error=access_restricted); keep it in sync.
	CodeAccessRestricted = "access_restricted"

	// Validation errors
	CodeValidationFailed = "VALIDATION_FAILED"
	CodeInvalidRequest   = "INVALID_REQUEST"
	CodeInvalidFormat    = "INVALID_FORMAT"

	// Resource errors
	CodeResourceNotFound = "RESOURCE_NOT_FOUND"
	CodeResourceExists   = "RESOURCE_EXISTS"
	CodeResourceConflict = "RESOURCE_CONFLICT"
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
	CodeIDPAuthFailed = "IDP_AUTH_FAILED"

	// Request errors
	CodeBadRequest       = "BAD_REQUEST"
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	CodeNotImplemented   = "NOT_IMPLEMENTED"

	// Embedding provider errors
	CodeProviderNotFound          = "PROVIDER_NOT_FOUND"
	CodeProviderAlreadyExists     = "PROVIDER_ALREADY_EXISTS"
	CodeProviderCreateFailed      = "PROVIDER_CREATE_FAILED"
	CodeProviderUpdateFailed      = "PROVIDER_UPDATE_FAILED"
	CodeProviderDeleteFailed      = "PROVIDER_DELETE_FAILED"
	CodeProviderLastDeleteBlocked = "PROVIDER_LAST_DELETE_BLOCKED"
	CodeProviderValidationFailed  = "PROVIDER_VALIDATION_FAILED"

	// Model provider errors
	CodeModelProviderNotFound          = "MODEL_PROVIDER_NOT_FOUND"
	CodeModelProviderAlreadyExists     = "MODEL_PROVIDER_ALREADY_EXISTS"
	CodeModelProviderCreateFailed      = "MODEL_PROVIDER_CREATE_FAILED"
	CodeModelProviderUpdateFailed      = "MODEL_PROVIDER_UPDATE_FAILED"
	CodeModelProviderDeleteFailed      = "MODEL_PROVIDER_DELETE_FAILED"
	CodeModelProviderLastDeleteBlocked = "MODEL_PROVIDER_LAST_DELETE_BLOCKED"
	CodeModelProviderValidationFailed  = "MODEL_PROVIDER_VALIDATION_FAILED"

	// Preferences errors
	CodePreferencesUpdateFailed = "PREFERENCES_UPDATE_FAILED"
)

// Error titles for common error codes
// #nosec G101 - Error message strings, not actual credentials
var errorTitles = map[string]string{
	CodeAuthRequired:                   "Authentication Required",
	CodeAuthInvalid:                    "Invalid Authentication",
	CodeAuthExpired:                    "Authentication Expired",
	CodeForbidden:                      "Forbidden",
	CodeUnauthorized:                   "Unauthorized",
	CodeAccessRestricted:               "Access Restricted",
	CodeValidationFailed:               "Validation Failed",
	CodeInvalidRequest:                 "Invalid Request",
	CodeInvalidFormat:                  "Invalid Format",
	CodeResourceNotFound:               "Resource Not Found",
	CodeResourceExists:                 "Resource Already Exists",
	CodeResourceConflict:               "Resource Conflict",
	CodeDuplicateMembers:               "Duplicate Team Members",
	CodeRateLimitExceeded:              "Rate Limit Exceeded",
	CodeResourceLimitExceeded:          "Resource Limit Exceeded",
	CodeQuotaExceeded:                  "Quota Exceeded",
	CodeInternalError:                  "Internal Server Error",
	CodeServiceUnavailable:             "Service Unavailable",
	CodeDatabaseError:                  "Database Error",
	CodeIDPAuthFailed:                  "Identity Provider Authentication Failed",
	CodeBadRequest:                     "Bad Request",
	CodeMethodNotAllowed:               "Method Not Allowed",
	CodeNotImplemented:                 "Not Implemented",
	CodeProviderNotFound:               "Embedding Provider Not Found",
	CodeProviderAlreadyExists:          "Embedding Provider Already Exists",
	CodeProviderCreateFailed:           "Embedding Provider Creation Failed",
	CodeProviderUpdateFailed:           "Embedding Provider Update Failed",
	CodeProviderDeleteFailed:           "Embedding Provider Deletion Failed",
	CodeProviderLastDeleteBlocked:      "Cannot Delete Last Embedding Provider",
	CodeProviderValidationFailed:       "Embedding Provider Validation Failed",
	CodeModelProviderNotFound:          "Model Provider Not Found",
	CodeModelProviderAlreadyExists:     "Model Provider Already Exists",
	CodeModelProviderCreateFailed:      "Model Provider Creation Failed",
	CodeModelProviderUpdateFailed:      "Model Provider Update Failed",
	CodeModelProviderDeleteFailed:      "Model Provider Deletion Failed",
	CodeModelProviderLastDeleteBlocked: "Cannot Delete Last Model Provider",
	CodeModelProviderValidationFailed:  "Model Provider Validation Failed",
	CodePreferencesUpdateFailed:        "User Preferences Update Failed",
}

// GetErrorTitle returns the title for a given error code
func GetErrorTitle(code string) string {
	if title, ok := errorTitles[code]; ok {
		return title
	}
	return "Error"
}
