package testutils

import (
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"

	"github.com/vibexp/vibexp/internal/models"
)

// ParseHTTPJSONResponse parses the JSON response into the provided interface (renamed to avoid conflicts)
func ParseHTTPJSONResponse(t TestingT, response *httptest.ResponseRecorder, target interface{}) {
	if !validateTestingParams(t, response) {
		return
	}
	if target == nil {
		t.Fatal("target interface cannot be nil")
		return
	}
	t.Helper()

	if response.Body == nil {
		t.Fatal("response body is nil")
		return
	}

	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("Failed to parse JSON response: %v. Response body: %s", err, response.Body.String())
	}
}

// ParseErrorResponse parses an error response
func ParseErrorResponse(t TestingT, response *httptest.ResponseRecorder) *models.ErrorResponse {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var errorResp models.ErrorResponse
	ParseHTTPJSONResponse(t, response, &errorResp)
	return &errorResp
}

// ParsePaginatedResponse parses a paginated response and extracts pagination metadata
func ParsePaginatedResponse(t TestingT, response *httptest.ResponseRecorder) *PaginationMeta {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var paginatedResp struct {
		TotalCount int `json:"total_count"`
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		TotalPages int `json:"total_pages"`
	}

	ParseHTTPJSONResponse(t, response, &paginatedResp)

	return &PaginationMeta{
		TotalCount: paginatedResp.TotalCount,
		Page:       paginatedResp.Page,
		PerPage:    paginatedResp.PerPage,
		TotalPages: paginatedResp.TotalPages,
	}
}

// PaginationMeta holds pagination metadata
type PaginationMeta struct {
	TotalCount int `json:"total_count"`
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalPages int `json:"total_pages"`
}

// ValidatePagination validates pagination metadata
func (p *PaginationMeta) ValidatePagination(t TestingT) {
	t.Helper()

	if p.Page <= 0 {
		t.Errorf("Expected page to be positive, got %d", p.Page)
	}
	if p.PerPage <= 0 {
		t.Errorf("Expected per page to be positive, got %d", p.PerPage)
	}
	if p.TotalCount < 0 {
		t.Errorf("Expected total count to be non-negative, got %d", p.TotalCount)
	}
	if p.TotalPages < 0 {
		t.Errorf("Expected total pages to be non-negative, got %d", p.TotalPages)
	}

	// Validate total pages calculation
	expectedTotalPages := 0
	if p.TotalCount > 0 && p.PerPage > 0 {
		expectedTotalPages = (p.TotalCount + p.PerPage - 1) / p.PerPage
	}
	if p.TotalPages != expectedTotalPages {
		t.Errorf("Expected total pages %d based on total count %d and per page %d, got %d",
			expectedTotalPages, p.TotalCount, p.PerPage, p.TotalPages)
	}
}

// ParsePromptResponse parses a prompt response
func ParsePromptResponse(t TestingT, response *httptest.ResponseRecorder) *models.Prompt {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var prompt models.Prompt
	ParseHTTPJSONResponse(t, response, &prompt)
	return &prompt
}

// ParsePromptListResponse parses a prompt list response
func ParsePromptListResponse(t TestingT, response *httptest.ResponseRecorder) *models.PromptListResponse {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var promptList models.PromptListResponse
	ParseHTTPJSONResponse(t, response, &promptList)
	return &promptList
}

// ParseAPIKeyResponse parses an API key response
func ParseAPIKeyResponse(t TestingT, response *httptest.ResponseRecorder) *models.APIKey {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var apiKey models.APIKey
	ParseHTTPJSONResponse(t, response, &apiKey)
	return &apiKey
}

// ParseCreateAPIKeyResponse parses a create API key response
func ParseCreateAPIKeyResponse(t TestingT, response *httptest.ResponseRecorder) *models.CreateAPIKeyResponse {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var createResp models.CreateAPIKeyResponse
	ParseHTTPJSONResponse(t, response, &createResp)
	return &createResp
}

// ParseEmbeddingProviderResponse parses an embedding provider response
func ParseEmbeddingProviderResponse(t TestingT, response *httptest.ResponseRecorder) *models.EmbeddingProviderResponse {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var provider models.EmbeddingProviderResponse
	ParseHTTPJSONResponse(t, response, &provider)
	return &provider
}

// ParseEmbeddingProviderListResponse parses an embedding provider list response
func ParseEmbeddingProviderListResponse(
	t TestingT, response *httptest.ResponseRecorder,
) *models.EmbeddingProviderListResponse {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var providerList models.EmbeddingProviderListResponse
	ParseHTTPJSONResponse(t, response, &providerList)
	return &providerList
}

// ParseValidateEmbeddingProviderResponse parses a validate embedding provider response
func ParseValidateEmbeddingProviderResponse(
	t TestingT, response *httptest.ResponseRecorder,
) *models.ValidateEmbeddingProviderResponse {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var validateResp models.ValidateEmbeddingProviderResponse
	ParseHTTPJSONResponse(t, response, &validateResp)
	return &validateResp
}

// ParseRenderPromptResponse parses a render prompt response
func ParseRenderPromptResponse(t TestingT, response *httptest.ResponseRecorder) *models.RenderPromptResponse {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var renderResp models.RenderPromptResponse
	ParseHTTPJSONResponse(t, response, &renderResp)
	return &renderResp
}

// ParseSubscriptionResponse parses a subscription response
func ParseSubscriptionResponse(t TestingT, response *httptest.ResponseRecorder) *models.Subscription {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var subscription models.Subscription
	ParseHTTPJSONResponse(t, response, &subscription)
	return &subscription
}

// ParseCreateSubscriptionResponse parses a create subscription response
func ParseCreateSubscriptionResponse(
	t TestingT, response *httptest.ResponseRecorder,
) *models.CreateSubscriptionResponse {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var createResp models.CreateSubscriptionResponse
	ParseHTTPJSONResponse(t, response, &createResp)
	return &createResp
}

// ParseSubscriptionStatusResponse parses a subscription status response
func ParseSubscriptionStatusResponse(
	t TestingT, response *httptest.ResponseRecorder,
) *models.SubscriptionStatusResponse {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var statusResp models.SubscriptionStatusResponse
	ParseHTTPJSONResponse(t, response, &statusResp)
	return &statusResp
}

// ParseCreatePortalSessionResponse parses a create portal session response
func ParseCreatePortalSessionResponse(
	t TestingT, response *httptest.ResponseRecorder,
) *models.CreatePortalSessionResponse {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var portalResp models.CreatePortalSessionResponse
	ParseHTTPJSONResponse(t, response, &portalResp)
	return &portalResp
}

// ParseProductConfigurationResponse parses a product configuration response
func ParseProductConfigurationResponse(
	t TestingT, response *httptest.ResponseRecorder,
) *models.ProductConfigurationResponse {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var configResp models.ProductConfigurationResponse
	ParseHTTPJSONResponse(t, response, &configResp)
	return &configResp
}

// ParseLocationHeader parses the Location header and extracts the resource ID
func ParseLocationHeader(t TestingT, response *httptest.ResponseRecorder) string {
	if !validateTestingParams(t, response) {
		return ""
	}
	t.Helper()

	location := response.Header().Get("Location")
	if location == "" {
		t.Error("Expected Location header to be present")
		return ""
	}

	// Extract ID from the end of the location URL
	parts := strings.Split(location, "/")
	if len(parts) == 0 {
		t.Errorf("Invalid Location header format: %s", location)
		return ""
	}

	return parts[len(parts)-1]
}

// ParseIntFromHeader parses an integer value from a response header
func ParseIntFromHeader(t TestingT, response *httptest.ResponseRecorder, headerName string) int {
	if !validateTestingParams(t, response) {
		return 0
	}
	t.Helper()

	headerValue := response.Header().Get(headerName)
	if headerValue == "" {
		t.Errorf("Expected header '%s' to be present", headerName)
		return 0
	}

	value, err := strconv.Atoi(headerValue)
	if err != nil {
		t.Errorf("Failed to parse header '%s' as integer: %v", headerName, err)
		return 0
	}

	return value
}

// ParseContentLength parses the Content-Length header
func ParseContentLength(t TestingT, response *httptest.ResponseRecorder) int {
	return ParseIntFromHeader(t, response, "Content-Length")
}

// ParseRawResponse returns the raw response body as a string
func ParseRawResponse(t TestingT, response *httptest.ResponseRecorder) string {
	if !validateTestingParams(t, response) {
		return ""
	}
	t.Helper()

	if response.Body == nil {
		t.Error("Response body is nil")
		return ""
	}

	return response.Body.String()
}

// ParseAndValidateID validates that a response contains a valid ID field
func ParseAndValidateID(t TestingT, response *httptest.ResponseRecorder) string {
	if !validateTestingParams(t, response) {
		return ""
	}
	t.Helper()

	var responseWithID struct {
		ID string `json:"id"`
	}

	ParseHTTPJSONResponse(t, response, &responseWithID)

	if responseWithID.ID == "" {
		t.Error("Expected response to contain a non-empty ID field")
		return ""
	}

	return responseWithID.ID
}

// ParseMultipleResponses parses multiple responses of the same type from a JSON array
func ParseMultipleResponses(t TestingT, response *httptest.ResponseRecorder, target interface{}) {
	if !validateTestingParams(t, response) {
		return
	}
	if target == nil {
		t.Fatal("target interface cannot be nil")
		return
	}
	t.Helper()

	ParseHTTPJSONResponse(t, response, target)
}

// ValidateResponseStructure validates that a response has the expected JSON structure
func ValidateResponseStructure(t TestingT, response *httptest.ResponseRecorder, expectedFields []string) {
	if !validateTestingParams(t, response) {
		return
	}
	t.Helper()

	var jsonData map[string]interface{}
	ParseHTTPJSONResponse(t, response, &jsonData)

	for _, field := range expectedFields {
		if _, exists := jsonData[field]; !exists {
			t.Errorf("Expected field '%s' to be present in response", field)
		}
	}
}

// ValidateResponseDoesNotContain validates that a response does not contain certain fields
func ValidateResponseDoesNotContain(t TestingT, response *httptest.ResponseRecorder, forbiddenFields []string) {
	if !validateTestingParams(t, response) {
		return
	}
	t.Helper()

	var jsonData map[string]interface{}
	ParseHTTPJSONResponse(t, response, &jsonData)

	for _, field := range forbiddenFields {
		if _, exists := jsonData[field]; exists {
			t.Errorf("Expected field '%s' to NOT be present in response", field)
		}
	}
}

// ParseValidationErrors parses validation error responses and returns field-specific errors
func ParseValidationErrors(t TestingT, response *httptest.ResponseRecorder) map[string][]string {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var validationResp struct {
		Error   string              `json:"error"`
		Message string              `json:"message"`
		Details map[string][]string `json:"details,omitempty"`
	}

	ParseHTTPJSONResponse(t, response, &validationResp)
	return validationResp.Details
}

// ExtractFieldFromResponse extracts a specific field value from a JSON response
func ExtractFieldFromResponse(t TestingT, response *httptest.ResponseRecorder, fieldName string) interface{} {
	if !validateTestingParams(t, response) {
		return nil
	}
	t.Helper()

	var jsonData map[string]interface{}
	ParseHTTPJSONResponse(t, response, &jsonData)

	value, exists := jsonData[fieldName]
	if !exists {
		t.Errorf("Field '%s' not found in response", fieldName)
		return nil
	}

	return value
}

// ExtractStringField extracts a string field from a JSON response
func ExtractStringField(t TestingT, response *httptest.ResponseRecorder, fieldName string) string {
	value := ExtractFieldFromResponse(t, response, fieldName)
	if value == nil {
		return ""
	}

	str, ok := value.(string)
	if !ok {
		t.Errorf("Field '%s' is not a string, got %T", fieldName, value)
		return ""
	}

	return str
}

// ExtractIntField extracts an integer field from a JSON response
func ExtractIntField(t TestingT, response *httptest.ResponseRecorder, fieldName string) int {
	value := ExtractFieldFromResponse(t, response, fieldName)
	if value == nil {
		return 0
	}

	// JSON numbers are parsed as float64
	if floatVal, ok := value.(float64); ok {
		return int(floatVal)
	}

	t.Errorf("Field '%s' is not a number, got %T", fieldName, value)
	return 0
}

// ExtractBoolField extracts a boolean field from a JSON response
func ExtractBoolField(t TestingT, response *httptest.ResponseRecorder, fieldName string) bool {
	value := ExtractFieldFromResponse(t, response, fieldName)
	if value == nil {
		return false
	}

	boolVal, ok := value.(bool)
	if !ok {
		t.Errorf("Field '%s' is not a boolean, got %T", fieldName, value)
		return false
	}

	return boolVal
}
