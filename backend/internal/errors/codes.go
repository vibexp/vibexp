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
	// CodeInvitationInvalid is the ONE answer of the public invitation
	// operations for a link that cannot be used (unknown, expired, revoked,
	// accepted or rejected). Stable: the invitation page branches on it (#1435).
	CodeInvitationInvalid = "invitation_invalid"
	// CodeAccountExists signals a registration through an invitation refused
	// because an account already has the invited email: sign in instead.
	// Stable, as above.
	CodeAccountExists = "account_exists"

	// Validation errors
	CodeValidationFailed = "VALIDATION_FAILED"
	CodeInvalidRequest   = "INVALID_REQUEST"
	CodeInvalidFormat    = "INVALID_FORMAT"

	// Resource errors
	CodeResourceNotFound = "RESOURCE_NOT_FOUND"
	CodeResourceExists   = "RESOURCE_EXISTS"
	CodeResourceConflict = "RESOURCE_CONFLICT"
	CodeDuplicateMembers = "DUPLICATE_MEMBERS"

	// Resource lock errors (#1288): locking a resource that is already locked,
	// and unlocking one that is not. Both are 409s; the code tells them apart.
	CodeResourceLocked    = "RESOURCE_LOCKED"
	CodeResourceNotLocked = "RESOURCE_NOT_LOCKED"

	// CodeResourceWriteLocked signals a write refused because the resource is
	// locked (#1289): a delete, or an update changing a frozen field. It is a
	// 423 and deliberately not CodeResourceLocked, which is the 409 for locking
	// what is already locked.
	CodeResourceWriteLocked = "RESOURCE_WRITE_LOCKED"

	// CodeHasLockedResources signals an indirect delete or move refused
	// because it would remove or rewrite locked resources (#1290): deleting or
	// migrating a project that holds them, or deleting a custom type or status
	// they carry. It is a 423, and names the resources in its metadata.
	CodeHasLockedResources = "HAS_LOCKED_RESOURCES"

	// Self-service account (#1437). Lower case by contract, like
	// CodeAccessRestricted: clients branch on these exact values.
	//
	// CodeReauthenticationRequired: the operation needs a sign-in within the
	// last few minutes and this session is older. The client asks the user to
	// sign in again and repeats the request.
	CodeReauthenticationRequired = "reauthentication_required"
	// CodeCookieSessionRequired: the operation changes how the account signs in
	// and is only open to a browser session, not an API key or a bearer token.
	CodeCookieSessionRequired = "cookie_session_required"
	// CodeLastSignInMethod: removing this password or identity would leave the
	// account with no usable way to sign in.
	CodeLastSignInMethod = "last_sign_in_method"
	// CodeLocalSignInDisabled: password sign-in is switched off on this
	// instance, so a password cannot be set or changed.
	CodeLocalSignInDisabled = "password_login_disabled"
	// CodeTooManyAttempts: too many wrong passwords; the account's password is
	// temporarily locked. The response carries Retry-After.
	CodeTooManyAttempts = "too_many_attempts"

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

	// Prompt render errors
	CodeUnresolvedReferences = "UNRESOLVED_REFERENCES"

	// Embedding provider errors
	CodeProviderNotFound          = "PROVIDER_NOT_FOUND"
	CodeProviderAlreadyExists     = "PROVIDER_ALREADY_EXISTS"
	CodeProviderCreateFailed      = "PROVIDER_CREATE_FAILED"
	CodeProviderUpdateFailed      = "PROVIDER_UPDATE_FAILED"
	CodeProviderDeleteFailed      = "PROVIDER_DELETE_FAILED"
	CodeProviderLastDeleteBlocked = "PROVIDER_LAST_DELETE_BLOCKED"
	CodeProviderValidationFailed  = "PROVIDER_VALIDATION_FAILED"

	// GitHub App configuration errors
	CodeGitHubAppNotConfigured     = "GITHUB_APP_NOT_CONFIGURED"
	CodeGitHubAppAlreadyRegistered = "GITHUB_APP_ALREADY_REGISTERED"
	CodeGitHubAppConfigExists      = "GITHUB_APP_CONFIG_EXISTS"
	CodeGitHubAppConfigConflict    = "GITHUB_APP_CONFIG_CONFLICT"

	// Team email provider errors
	CodeTeamEmailProviderNotConfigured    = "TEAM_EMAIL_PROVIDER_NOT_CONFIGURED"
	CodeTeamEmailProviderValidationFailed = "TEAM_EMAIL_PROVIDER_VALIDATION_FAILED"
	CodeTeamEmailProviderUpdateFailed     = "TEAM_EMAIL_PROVIDER_UPDATE_FAILED"
	CodeTeamEmailProviderDeleteFailed     = "TEAM_EMAIL_PROVIDER_DELETE_FAILED"

	// Instance email provider errors (#1189)
	CodeInstanceEmailProviderNotConfigured    = "INSTANCE_EMAIL_PROVIDER_NOT_CONFIGURED"
	CodeInstanceEmailProviderValidationFailed = "INSTANCE_EMAIL_PROVIDER_VALIDATION_FAILED"

	// Instance search + AI summary settings errors (#1200)
	CodeInstanceSettingsValidationFailed = "INSTANCE_SETTINGS_VALIDATION_FAILED"
	CodeInstanceSettingsVersionConflict  = "INSTANCE_SETTINGS_VERSION_CONFLICT"

	// Instance authentication settings errors (#1238)
	CodeInstanceAuthProviderConflict = "INSTANCE_AUTH_PROVIDER_CONFLICT"
	// CodeLockoutRisk signals an authentication settings change refused because
	// it could lock sign-in out. Its lowercase value is a stable contract with
	// the frontend, which re-sends the change with confirm_lockout_risk.
	CodeLockoutRisk = "lockout_risk"

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

	// Search summary errors (#1073). Each names a classified failure; the
	// provider's own response never reaches the client (#464).
	CodeAISummaryDisabled            = "AI_SUMMARY_DISABLED"
	CodeAISummaryNoProvider          = "AI_SUMMARY_NO_PROVIDER"
	CodeAISummaryNoResults           = "AI_SUMMARY_NO_RESULTS"
	CodeAISummaryProviderUnreachable = "AI_SUMMARY_PROVIDER_UNREACHABLE"
	CodeAISummaryUnauthorized        = "AI_SUMMARY_UNAUTHORIZED"
	CodeAISummaryModelError          = "AI_SUMMARY_MODEL_ERROR"
	CodeAISummaryTimeout             = "AI_SUMMARY_TIMEOUT"
)

// Error titles for common error codes
// #nosec G101 - Error message strings, not actual credentials
var errorTitles = map[string]string{
	CodeAuthRequired:                      "Authentication Required",
	CodeAuthInvalid:                       "Invalid Authentication",
	CodeAuthExpired:                       "Authentication Expired",
	CodeForbidden:                         "Forbidden",
	CodeUnauthorized:                      "Unauthorized",
	CodeAccessRestricted:                  "Access Restricted",
	CodeInvitationInvalid:                 "Invitation Link Not Valid",
	CodeAccountExists:                     "Account Already Exists",
	CodeValidationFailed:                  "Validation Failed",
	CodeInvalidRequest:                    "Invalid Request",
	CodeInvalidFormat:                     "Invalid Format",
	CodeResourceNotFound:                  "Resource Not Found",
	CodeResourceExists:                    "Resource Already Exists",
	CodeResourceConflict:                  "Resource Conflict",
	CodeDuplicateMembers:                  "Duplicate Team Members",
	CodeReauthenticationRequired:          "Sign In Again To Continue",
	CodeCookieSessionRequired:             "Browser Session Required",
	CodeLastSignInMethod:                  "Last Sign-In Method",
	CodeLocalSignInDisabled:               "Password Sign-In Is Off",
	CodeTooManyAttempts:                   "Too Many Attempts",
	CodeRateLimitExceeded:                 "Rate Limit Exceeded",
	CodeResourceLimitExceeded:             "Resource Limit Exceeded",
	CodeQuotaExceeded:                     "Quota Exceeded",
	CodeInternalError:                     "Internal Server Error",
	CodeServiceUnavailable:                "Service Unavailable",
	CodeDatabaseError:                     "Database Error",
	CodeIDPAuthFailed:                     "Identity Provider Authentication Failed",
	CodeBadRequest:                        "Bad Request",
	CodeMethodNotAllowed:                  "Method Not Allowed",
	CodeNotImplemented:                    "Not Implemented",
	CodeUnresolvedReferences:              "Unresolved References",
	CodeProviderNotFound:                  "Embedding Provider Not Found",
	CodeProviderAlreadyExists:             "Embedding Provider Already Exists",
	CodeProviderCreateFailed:              "Embedding Provider Creation Failed",
	CodeProviderUpdateFailed:              "Embedding Provider Update Failed",
	CodeProviderDeleteFailed:              "Embedding Provider Deletion Failed",
	CodeProviderLastDeleteBlocked:         "Cannot Delete Last Embedding Provider",
	CodeProviderValidationFailed:          "Embedding Provider Validation Failed",
	CodeGitHubAppNotConfigured:            "GitHub App Not Configured",
	CodeGitHubAppAlreadyRegistered:        "GitHub App Already Registered",
	CodeGitHubAppConfigExists:             "GitHub App Already Configured",
	CodeGitHubAppConfigConflict:           "GitHub App Configuration Conflict",
	CodeTeamEmailProviderNotConfigured:    "Team Email Provider Not Configured",
	CodeTeamEmailProviderValidationFailed: "Team Email Provider Validation Failed",
	CodeTeamEmailProviderUpdateFailed:     "Team Email Provider Update Failed",
	CodeTeamEmailProviderDeleteFailed:     "Team Email Provider Delete Failed",
	CodeModelProviderNotFound:             "Model Provider Not Found",
	CodeModelProviderAlreadyExists:        "Model Provider Already Exists",
	CodeModelProviderCreateFailed:         "Model Provider Creation Failed",
	CodeModelProviderUpdateFailed:         "Model Provider Update Failed",
	CodeModelProviderDeleteFailed:         "Model Provider Deletion Failed",
	CodeModelProviderLastDeleteBlocked:    "Cannot Delete Last Model Provider",
	CodeModelProviderValidationFailed:     "Model Provider Validation Failed",
	CodePreferencesUpdateFailed:           "User Preferences Update Failed",
	CodeAISummaryDisabled:                 "AI Summary Disabled",
	CodeAISummaryNoProvider:               "No Model Provider Configured",
	CodeAISummaryNoResults:                "No Documents To Summarize",
	CodeAISummaryProviderUnreachable:      "Model Provider Unreachable",
	CodeAISummaryUnauthorized:             "Model Provider Rejected Credentials",
	CodeAISummaryModelError:               "Model Provider Rejected Request",
	CodeAISummaryTimeout:                  "Model Provider Timed Out",

	// Instance email provider errors (#1189)
	CodeInstanceEmailProviderNotConfigured:    "Instance Email Provider Not Configured",
	CodeInstanceEmailProviderValidationFailed: "Instance Email Provider Validation Failed",

	// Instance search + AI summary settings errors (#1200)
	CodeInstanceSettingsValidationFailed: "Instance Settings Validation Failed",
	CodeInstanceSettingsVersionConflict:  "Instance Settings Version Conflict",

	// Instance authentication settings errors (#1238)
	CodeInstanceAuthProviderConflict: "Instance Auth Provider Conflict",
	CodeLockoutRisk:                  "Lockout Risk",

	// Resource lock errors (#1288)
	CodeResourceLocked:    "Resource Locked",
	CodeResourceNotLocked: "Resource Not Locked",

	CodeResourceWriteLocked: "Resource Write Locked",

	// Indirect delete/move guards (#1290)
	CodeHasLockedResources: "Has Locked Resources",
}

// GetErrorTitle returns the title for a given error code
func GetErrorTitle(code string) string {
	if title, ok := errorTitles[code]; ok {
		return title
	}
	return "Error"
}
