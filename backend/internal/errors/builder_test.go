package errors

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// builderContractScenario pins the RFC 9457 problem-details contract of one
// constructor: HTTP status, machine-readable code, title, detail, and the
// payload extensions (validation errors, duplicate emails, metadata). Wire
// values (code, title) are literal strings on purpose so a rename shows up as
// a contract break, not a silently co-evolving constant.
type builderContractScenario struct {
	name           string
	build          func() *APIError
	wantStatus     int
	wantCode       string
	wantTitle      string
	wantDetail     string
	wantValidation []ValidationError
	wantDupEmails  []string
	wantMetadata   map[string]any
}

// assertBuilderContract is the single shared assertion for every scenario.
func assertBuilderContract(t *testing.T, sc builderContractScenario) {
	t.Helper()
	got := sc.build()
	require.NotNil(t, got)
	// Default neutral base ("about:blank") yields a bare "about:blank" type.
	assert.Equal(t, "about:blank", got.Type)
	assert.Equal(t, sc.wantTitle, got.Title)
	assert.Equal(t, sc.wantStatus, got.Status)
	assert.Equal(t, sc.wantDetail, got.Detail)
	assert.Equal(t, sc.wantCode, got.Code)
	assert.Equal(t, sc.wantValidation, got.ValidationErrors)
	assert.Equal(t, sc.wantDupEmails, got.DuplicateEmails)
	assert.Equal(t, sc.wantMetadata, got.Metadata)
}

func TestBuilderConstructorContract(t *testing.T) {
	var scenarios []builderContractScenario
	scenarios = append(scenarios, coreBuilderScenarios()...)
	scenarios = append(scenarios, embeddingProviderBuilderScenarios()...)
	scenarios = append(scenarios, modelProviderBuilderScenarios()...)
	scenarios = append(scenarios, preferencesAndDateBuilderScenarios()...)

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			assertBuilderContract(t, sc)
		})
	}
}

func coreBuilderScenarios() []builderContractScenario {
	fieldErrs := []ValidationError{
		{Field: "name", Message: "Field 'name' is required", Code: "REQUIRED", Constraint: "required"},
	}
	limitMetadata := map[string]any{
		"team_id":     "team-123",
		"upgrade_url": "/settings/teams/team-123/subscription",
	}

	return []builderContractScenario{
		{
			name:           "validation error propagates field errors",
			build:          func() *APIError { return NewValidationError("Request validation failed", fieldErrs) },
			wantStatus:     http.StatusBadRequest,
			wantCode:       "VALIDATION_FAILED",
			wantTitle:      "Validation Failed",
			wantDetail:     "Request validation failed",
			wantValidation: fieldErrs,
		},
		{
			name:       "auth required",
			build:      func() *APIError { return NewAuthRequiredError("Sign in to continue") },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "AUTH_REQUIRED",
			wantTitle:  "Authentication Required",
			wantDetail: "Sign in to continue",
		},
		{
			name:       "auth invalid",
			build:      func() *APIError { return NewAuthInvalidError("Token signature mismatch") },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "AUTH_INVALID",
			wantTitle:  "Invalid Authentication",
			wantDetail: "Token signature mismatch",
		},
		{
			name:       "forbidden",
			build:      func() *APIError { return NewForbiddenError("Members cannot delete teams") },
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
			wantTitle:  "Forbidden",
			wantDetail: "Members cannot delete teams",
		},
		{
			name:       "access restricted keeps lowercase frontend-facing code",
			build:      func() *APIError { return NewAccessRestrictedError("Sign-ups are restricted") },
			wantStatus: http.StatusForbidden,
			wantCode:   "access_restricted",
			wantTitle:  "Access Restricted",
			wantDetail: "Sign-ups are restricted",
		},
		{
			name:       "resource not found with explicit detail",
			build:      func() *APIError { return NewResourceNotFoundError("prompt", "Prompt p-1 does not exist") },
			wantStatus: http.StatusNotFound,
			wantCode:   "RESOURCE_NOT_FOUND",
			wantTitle:  "Resource Not Found",
			wantDetail: "Prompt p-1 does not exist",
		},
		{
			name:       "resource not found defaults detail from resource type",
			build:      func() *APIError { return NewResourceNotFoundError("prompt", "") },
			wantStatus: http.StatusNotFound,
			wantCode:   "RESOURCE_NOT_FOUND",
			wantTitle:  "Resource Not Found",
			wantDetail: "prompt not found",
		},
		{
			name:       "resource exists with explicit detail",
			build:      func() *APIError { return NewResourceExistsError("team", "A team with this slug exists") },
			wantStatus: http.StatusConflict,
			wantCode:   "RESOURCE_EXISTS",
			wantTitle:  "Resource Already Exists",
			wantDetail: "A team with this slug exists",
		},
		{
			name:       "resource exists defaults detail from resource type",
			build:      func() *APIError { return NewResourceExistsError("team", "") },
			wantStatus: http.StatusConflict,
			wantCode:   "RESOURCE_EXISTS",
			wantTitle:  "Resource Already Exists",
			wantDetail: "team already exists",
		},
		{
			name:       "resource limit exceeded",
			build:      func() *APIError { return NewResourceLimitExceededError("Seat limit reached") },
			wantStatus: http.StatusForbidden,
			wantCode:   "RESOURCE_LIMIT_EXCEEDED",
			wantTitle:  "Resource Limit Exceeded",
			wantDetail: "Seat limit reached",
		},
		{
			name: "resource limit exceeded with metadata",
			build: func() *APIError {
				return NewResourceLimitExceededErrorWithMetadata("Team requires an active subscription", limitMetadata)
			},
			wantStatus:   http.StatusForbidden,
			wantCode:     "RESOURCE_LIMIT_EXCEEDED",
			wantTitle:    "Resource Limit Exceeded",
			wantDetail:   "Team requires an active subscription",
			wantMetadata: limitMetadata,
		},
		{
			name:       "internal error with explicit detail",
			build:      func() *APIError { return NewInternalError("Failed to render response") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
			wantTitle:  "Internal Server Error",
			wantDetail: "Failed to render response",
		},
		{
			name:       "internal error defaults detail",
			build:      func() *APIError { return NewInternalError("") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
			wantTitle:  "Internal Server Error",
			wantDetail: "An internal error occurred",
		},
		{
			name:       "bad request",
			build:      func() *APIError { return NewBadRequestError("Invalid request body") },
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantTitle:  "Bad Request",
			wantDetail: "Invalid request body",
		},
		{
			name:       "idp auth failed",
			build:      func() *APIError { return NewIDPAuthError("Token exchange failed") },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "IDP_AUTH_FAILED",
			wantTitle:  "Identity Provider Authentication Failed",
			wantDetail: "Token exchange failed",
		},
		{
			name:       "service unavailable",
			build:      func() *APIError { return NewServiceUnavailableError("Upstream IDP timed out") },
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "SERVICE_UNAVAILABLE",
			wantTitle:  "Service Unavailable",
			wantDetail: "Upstream IDP timed out",
		},
		{
			name:       "database error with explicit detail",
			build:      func() *APIError { return NewDatabaseError("Connection pool exhausted") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "DATABASE_ERROR",
			wantTitle:  "Database Error",
			wantDetail: "Connection pool exhausted",
		},
		{
			name:       "database error defaults detail",
			build:      func() *APIError { return NewDatabaseError("") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "DATABASE_ERROR",
			wantTitle:  "Database Error",
			wantDetail: "Database operation failed",
		},
		{
			name: "duplicate members carries emails in detail and payload",
			build: func() *APIError {
				return NewDuplicateMembersError([]string{"a@example.com", "b@example.com"})
			},
			wantStatus:    http.StatusConflict,
			wantCode:      "DUPLICATE_MEMBERS",
			wantTitle:     "Duplicate Team Members",
			wantDetail:    "Users already in team: a@example.com, b@example.com",
			wantDupEmails: []string{"a@example.com", "b@example.com"},
		},
	}
}

func embeddingProviderBuilderScenarios() []builderContractScenario {
	fieldErrs := []ValidationError{
		{Field: "api_key", Message: "API key is required", Code: "REQUIRED", Constraint: "required"},
	}

	return []builderContractScenario{
		{
			name:       "provider not found with id",
			build:      func() *APIError { return NewProviderNotFoundError("prov-1") },
			wantStatus: http.StatusNotFound,
			wantCode:   "PROVIDER_NOT_FOUND",
			wantTitle:  "Embedding Provider Not Found",
			wantDetail: "Embedding provider with ID 'prov-1' not found",
		},
		{
			name:       "provider not found without id",
			build:      func() *APIError { return NewProviderNotFoundError("") },
			wantStatus: http.StatusNotFound,
			wantCode:   "PROVIDER_NOT_FOUND",
			wantTitle:  "Embedding Provider Not Found",
			wantDetail: "Embedding provider not found",
		},
		{
			name:       "provider already exists with name",
			build:      func() *APIError { return NewProviderAlreadyExistsError("openai") },
			wantStatus: http.StatusConflict,
			wantCode:   "PROVIDER_ALREADY_EXISTS",
			wantTitle:  "Embedding Provider Already Exists",
			wantDetail: "Embedding provider 'openai' already exists",
		},
		{
			name:       "provider already exists without name",
			build:      func() *APIError { return NewProviderAlreadyExistsError("") },
			wantStatus: http.StatusConflict,
			wantCode:   "PROVIDER_ALREADY_EXISTS",
			wantTitle:  "Embedding Provider Already Exists",
			wantDetail: "Embedding provider already exists",
		},
		{
			name:       "provider create failed with explicit detail",
			build:      func() *APIError { return NewProviderCreateFailedError("Encryption failed") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "PROVIDER_CREATE_FAILED",
			wantTitle:  "Embedding Provider Creation Failed",
			wantDetail: "Encryption failed",
		},
		{
			name:       "provider create failed defaults detail",
			build:      func() *APIError { return NewProviderCreateFailedError("") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "PROVIDER_CREATE_FAILED",
			wantTitle:  "Embedding Provider Creation Failed",
			wantDetail: "Failed to create embedding provider",
		},
		{
			name:       "provider update failed with explicit detail",
			build:      func() *APIError { return NewProviderUpdateFailedError("Row locked") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "PROVIDER_UPDATE_FAILED",
			wantTitle:  "Embedding Provider Update Failed",
			wantDetail: "Row locked",
		},
		{
			name:       "provider update failed defaults detail",
			build:      func() *APIError { return NewProviderUpdateFailedError("") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "PROVIDER_UPDATE_FAILED",
			wantTitle:  "Embedding Provider Update Failed",
			wantDetail: "Failed to update embedding provider",
		},
		{
			name:       "provider delete failed with explicit detail",
			build:      func() *APIError { return NewProviderDeleteFailedError("Provider is in use") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "PROVIDER_DELETE_FAILED",
			wantTitle:  "Embedding Provider Deletion Failed",
			wantDetail: "Provider is in use",
		},
		{
			name:       "provider delete failed defaults detail",
			build:      func() *APIError { return NewProviderDeleteFailedError("") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "PROVIDER_DELETE_FAILED",
			wantTitle:  "Embedding Provider Deletion Failed",
			wantDetail: "Failed to delete embedding provider",
		},
		{
			name:       "provider last delete blocked",
			build:      NewProviderLastDeleteBlockedError,
			wantStatus: http.StatusBadRequest,
			wantCode:   "PROVIDER_LAST_DELETE_BLOCKED",
			wantTitle:  "Cannot Delete Last Embedding Provider",
			wantDetail: "Cannot delete the last embedding provider. Please add another provider first.",
		},
		{
			name:           "provider validation propagates field errors",
			build:          func() *APIError { return NewProviderValidationError("Provider config invalid", fieldErrs) },
			wantStatus:     http.StatusBadRequest,
			wantCode:       "PROVIDER_VALIDATION_FAILED",
			wantTitle:      "Embedding Provider Validation Failed",
			wantDetail:     "Provider config invalid",
			wantValidation: fieldErrs,
		},
	}
}

func modelProviderBuilderScenarios() []builderContractScenario {
	fieldErrs := []ValidationError{
		{Field: "base_url", Message: "Base URL must be absolute", Code: "INVALID_FORMAT", Constraint: "uri"},
	}

	return []builderContractScenario{
		{
			name:       "model provider not found with id",
			build:      func() *APIError { return NewModelProviderNotFoundError("mp-1") },
			wantStatus: http.StatusNotFound,
			wantCode:   "MODEL_PROVIDER_NOT_FOUND",
			wantTitle:  "Model Provider Not Found",
			wantDetail: "Model provider with ID 'mp-1' not found",
		},
		{
			name:       "model provider not found without id",
			build:      func() *APIError { return NewModelProviderNotFoundError("") },
			wantStatus: http.StatusNotFound,
			wantCode:   "MODEL_PROVIDER_NOT_FOUND",
			wantTitle:  "Model Provider Not Found",
			wantDetail: "Model provider not found",
		},
		{
			name:       "model provider already exists with name",
			build:      func() *APIError { return NewModelProviderAlreadyExistsError("anthropic") },
			wantStatus: http.StatusConflict,
			wantCode:   "MODEL_PROVIDER_ALREADY_EXISTS",
			wantTitle:  "Model Provider Already Exists",
			wantDetail: "Model provider 'anthropic' already exists",
		},
		{
			name:       "model provider already exists without name",
			build:      func() *APIError { return NewModelProviderAlreadyExistsError("") },
			wantStatus: http.StatusConflict,
			wantCode:   "MODEL_PROVIDER_ALREADY_EXISTS",
			wantTitle:  "Model Provider Already Exists",
			wantDetail: "Model provider already exists",
		},
		{
			name:       "model provider create failed with explicit detail",
			build:      func() *APIError { return NewModelProviderCreateFailedError("Encryption failed") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "MODEL_PROVIDER_CREATE_FAILED",
			wantTitle:  "Model Provider Creation Failed",
			wantDetail: "Encryption failed",
		},
		{
			name:       "model provider create failed defaults detail",
			build:      func() *APIError { return NewModelProviderCreateFailedError("") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "MODEL_PROVIDER_CREATE_FAILED",
			wantTitle:  "Model Provider Creation Failed",
			wantDetail: "Failed to create model provider",
		},
		{
			name:       "model provider update failed with explicit detail",
			build:      func() *APIError { return NewModelProviderUpdateFailedError("Row locked") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "MODEL_PROVIDER_UPDATE_FAILED",
			wantTitle:  "Model Provider Update Failed",
			wantDetail: "Row locked",
		},
		{
			name:       "model provider update failed defaults detail",
			build:      func() *APIError { return NewModelProviderUpdateFailedError("") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "MODEL_PROVIDER_UPDATE_FAILED",
			wantTitle:  "Model Provider Update Failed",
			wantDetail: "Failed to update model provider",
		},
		{
			name:       "model provider delete failed with explicit detail",
			build:      func() *APIError { return NewModelProviderDeleteFailedError("Provider is in use") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "MODEL_PROVIDER_DELETE_FAILED",
			wantTitle:  "Model Provider Deletion Failed",
			wantDetail: "Provider is in use",
		},
		{
			name:       "model provider delete failed defaults detail",
			build:      func() *APIError { return NewModelProviderDeleteFailedError("") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "MODEL_PROVIDER_DELETE_FAILED",
			wantTitle:  "Model Provider Deletion Failed",
			wantDetail: "Failed to delete model provider",
		},
		{
			name:       "model provider last delete blocked",
			build:      NewModelProviderLastDeleteBlockedError,
			wantStatus: http.StatusBadRequest,
			wantCode:   "MODEL_PROVIDER_LAST_DELETE_BLOCKED",
			wantTitle:  "Cannot Delete Last Model Provider",
			wantDetail: "Cannot delete the last model provider. Please add another provider first.",
		},
		{
			name:           "model provider validation propagates field errors",
			build:          func() *APIError { return NewModelProviderValidationError("Provider config invalid", fieldErrs) },
			wantStatus:     http.StatusBadRequest,
			wantCode:       "MODEL_PROVIDER_VALIDATION_FAILED",
			wantTitle:      "Model Provider Validation Failed",
			wantDetail:     "Provider config invalid",
			wantValidation: fieldErrs,
		},
	}
}

func preferencesAndDateBuilderScenarios() []builderContractScenario {
	return []builderContractScenario{
		{
			name:       "preferences update failed with explicit detail",
			build:      func() *APIError { return NewPreferencesUpdateFailedError("Serialization failed") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "PREFERENCES_UPDATE_FAILED",
			wantTitle:  "User Preferences Update Failed",
			wantDetail: "Serialization failed",
		},
		{
			name:       "preferences update failed defaults detail",
			build:      func() *APIError { return NewPreferencesUpdateFailedError("") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "PREFERENCES_UPDATE_FAILED",
			wantTitle:  "User Preferences Update Failed",
			wantDetail: "Failed to update user preferences",
		},
		{
			name:       "date validation error builds field-level detail",
			build:      func() *APIError { return NewDateValidationError("from", "13-2024") },
			wantStatus: http.StatusBadRequest,
			wantCode:   "VALIDATION_FAILED",
			wantTitle:  "Validation Failed",
			wantDetail: "Invalid 'from' date format. Expected YYYY-MM-DD format, got '13-2024'. Example: '2024-01-15'",
			wantValidation: []ValidationError{
				{
					Field:      "from",
					Message:    "Invalid date format, expected YYYY-MM-DD, got '13-2024'",
					Code:       "INVALID_FORMAT",
					Constraint: "YYYY-MM-DD",
				},
			},
		},
		{
			name:       "date range error flags both bounds",
			build:      NewDateRangeError,
			wantStatus: http.StatusBadRequest,
			wantCode:   "VALIDATION_FAILED",
			wantTitle:  "Validation Failed",
			wantDetail: "'from' date must be before 'to' date",
			wantValidation: []ValidationError{
				{Field: "from", Message: "'from' date must be before 'to' date", Code: "INVALID_RANGE"},
				{Field: "to", Message: "'to' date must be after 'from' date", Code: "INVALID_RANGE"},
			},
		},
	}
}

// TestAPIErrorJSONFieldNamesMatchSpec pins the serialized field names of
// APIError (and nested ValidationError) to the documented error shape:
// ErrorResponse + ValidationError in backend/schemas/common.yaml, plus the
// duplicate_emails and metadata extensions documented in
// backend/schemas/teams.yaml.
func TestAPIErrorJSONFieldNamesMatchSpec(t *testing.T) {
	documented := map[string]struct{}{
		"type":              {},
		"title":             {},
		"status":            {},
		"detail":            {},
		"code":              {},
		"request_id":        {},
		"timestamp":         {},
		"instance":          {},
		"validation_errors": {},
		"duplicate_emails":  {},
		"metadata":          {},
	}

	apiErr := NewValidationError("Request validation failed", []ValidationError{
		{Field: "name", Message: "Field 'name' is required", Code: "REQUIRED", Constraint: "required"},
	})
	apiErr.Instance = "/api/v1/agents"
	apiErr.DuplicateEmails = []string{"dev@example.com"}
	apiErr.Metadata = map[string]any{"team_id": "team-123"}

	raw, err := json.Marshal(apiErr)
	require.NoError(t, err)
	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &got))

	for key := range got {
		_, ok := documented[key]
		assert.True(t, ok, "APIError serializes field %q not documented in the error schemas", key)
	}

	// Every field the spec marks required on ErrorResponse must be present.
	for _, requiredField := range []string{"type", "title", "status", "detail", "code", "request_id", "timestamp"} {
		assert.Contains(t, got, requiredField)
	}

	var validationErrs []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(got["validation_errors"], &validationErrs))
	require.Len(t, validationErrs, 1)
	for _, key := range []string{"field", "message", "code", "constraint"} {
		assert.Contains(t, validationErrs[0], key)
	}
}

func TestTypeURI_ConfigurableBase(t *testing.T) {
	// Default neutral base: "about:blank" with no code appended.
	assert.Equal(t, "about:blank", typeURI(CodeResourceLimitExceeded))

	// Restore the default after the test so other tests see the neutral base.
	t.Cleanup(func() { SetTypeBaseURI("") })

	// A configured base joins "<base>/<code>".
	SetTypeBaseURI("https://example.com/errors")
	assert.Equal(t, "https://example.com/errors/RESOURCE_LIMIT_EXCEEDED", typeURI(CodeResourceLimitExceeded))
	assert.Equal(
		t,
		"https://example.com/errors/RESOURCE_LIMIT_EXCEEDED",
		NewResourceLimitExceededError("limit").Type,
	)

	// Empty resets to the neutral default.
	SetTypeBaseURI("")
	assert.Equal(t, "about:blank", typeURI(CodeResourceLimitExceeded))
}
