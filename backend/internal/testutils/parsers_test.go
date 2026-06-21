package testutils

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/vibexp/vibexp/internal/models"
)

func TestParseHTTPJSONResponse(t *testing.T) {
	response := httptest.NewRecorder()
	testData := map[string]interface{}{
		"id":   "test-123",
		"name": "Test Name",
	}

	jsonData, err := json.Marshal(testData)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}
	if _, err := response.Body.Write(jsonData); err != nil {
		t.Logf("Failed to write to response body: %v", err)
	}

	var result map[string]interface{}
	mockT := &MockTestingT{}
	ParseHTTPJSONResponse(mockT, response, &result)

	if mockT.fatalCalled {
		t.Error("Expected no fatal error for valid JSON")
	}

	if result["id"] != "test-123" {
		t.Errorf("Expected id 'test-123', got %v", result["id"])
	}
	if result["name"] != "Test Name" {
		t.Errorf("Expected name 'Test Name', got %v", result["name"])
	}
}

func TestParseErrorResponse(t *testing.T) {
	response := httptest.NewRecorder()
	errorResp := models.ErrorResponse{
		Error:   "validation_error",
		Message: "Invalid input data",
	}

	jsonData, err := json.Marshal(errorResp)
	if err != nil {
		t.Fatalf("Failed to marshal error response: %v", err)
	}
	if _, err := response.Body.Write(jsonData); err != nil {
		t.Logf("Failed to write to response body: %v", err)
	}

	mockT := &MockTestingT{}
	result := ParseErrorResponse(mockT, response)

	if mockT.fatalCalled {
		t.Error("Expected no fatal error for valid error response")
	}

	if result.Error != "validation_error" {
		t.Errorf("Expected error 'validation_error', got %s", result.Error)
	}
	if result.Message != "Invalid input data" {
		t.Errorf("Expected message 'Invalid input data', got %s", result.Message)
	}
}

func TestParsePaginatedResponse(t *testing.T) {
	response := httptest.NewRecorder()
	paginatedData := struct {
		TotalCount int `json:"total_count"`
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		TotalPages int `json:"total_pages"`
	}{
		TotalCount: 100,
		Page:       2,
		PerPage:    25,
		TotalPages: 4,
	}

	jsonData, err := json.Marshal(paginatedData)
	if err != nil {
		t.Fatalf("Failed to marshal paginated data: %v", err)
	}
	if _, err := response.Body.Write(jsonData); err != nil {
		t.Logf("Failed to write to response body: %v", err)
	}

	mockT := &MockTestingT{}
	result := ParsePaginatedResponse(mockT, response)

	if mockT.fatalCalled {
		t.Error("Expected no fatal error for valid paginated response")
	}

	if result.TotalCount != 100 {
		t.Errorf("Expected TotalCount 100, got %d", result.TotalCount)
	}
	if result.Page != 2 {
		t.Errorf("Expected Page 2, got %d", result.Page)
	}
	if result.PerPage != 25 {
		t.Errorf("Expected PerPage 25, got %d", result.PerPage)
	}
	if result.TotalPages != 4 {
		t.Errorf("Expected TotalPages 4, got %d", result.TotalPages)
	}
}

func TestPaginationMetaValidatePagination(t *testing.T) {
	// Test valid pagination
	validPagination := &PaginationMeta{
		TotalCount: 100,
		Page:       2,
		PerPage:    25,
		TotalPages: 4,
	}

	mockT := &MockTestingT{}
	validPagination.ValidatePagination(mockT)
	if mockT.errorCalled {
		t.Error("Expected no error for valid pagination")
	}

	// Test invalid pagination (wrong total pages calculation)
	invalidPagination := &PaginationMeta{
		TotalCount: 100,
		Page:       2,
		PerPage:    25,
		TotalPages: 5, // Should be 4
	}

	mockT = &MockTestingT{}
	invalidPagination.ValidatePagination(mockT)
	if !mockT.errorCalled {
		t.Error("Expected error for invalid total pages calculation")
	}
}

func TestParsePromptResponse(t *testing.T) {
	response := httptest.NewRecorder()
	prompt := TestPrompt("user-123")

	jsonData, err := json.Marshal(prompt)
	if err != nil {
		t.Fatalf("Failed to marshal prompt: %v", err)
	}
	if _, err := response.Body.Write(jsonData); err != nil {
		t.Logf("Failed to write to response body: %v", err)
	}

	mockT := &MockTestingT{}
	result := ParsePromptResponse(mockT, response)

	if mockT.fatalCalled {
		t.Error("Expected no fatal error for valid prompt response")
	}

	if result.ID != prompt.ID {
		t.Errorf("Expected ID %s, got %s", prompt.ID, result.ID)
	}
	if result.Name != prompt.Name {
		t.Errorf("Expected Name %s, got %s", prompt.Name, result.Name)
	}
	if result.UserID != prompt.UserID {
		t.Errorf("Expected UserID %s, got %s", prompt.UserID, result.UserID)
	}
}

func TestParseLocationHeader(t *testing.T) {
	response := httptest.NewRecorder()
	response.Header().Set("Location", "/api/v1/prompts/prompt-123")

	mockT := &MockTestingT{}
	result := ParseLocationHeader(mockT, response)

	if mockT.errorCalled {
		t.Error("Expected no error for valid Location header")
	}

	if result != "prompt-123" {
		t.Errorf("Expected resource ID 'prompt-123', got %s", result)
	}

	// Test with missing Location header
	response = httptest.NewRecorder()
	mockT = &MockTestingT{}
	_ = ParseLocationHeader(mockT, response)

	if !mockT.errorCalled {
		t.Error("Expected error for missing Location header")
	}
}

func TestParseIntFromHeader(t *testing.T) {
	response := httptest.NewRecorder()
	response.Header().Set("X-Total-Count", "42")

	mockT := &MockTestingT{}
	result := ParseIntFromHeader(mockT, response, "X-Total-Count")

	if mockT.errorCalled {
		t.Error("Expected no error for valid integer header")
	}

	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}

	// Test with invalid integer
	response = httptest.NewRecorder()
	response.Header().Set("X-Total-Count", "not-a-number")

	mockT = &MockTestingT{}
	_ = ParseIntFromHeader(mockT, response, "X-Total-Count")

	if !mockT.errorCalled {
		t.Error("Expected error for invalid integer header")
	}
}

func TestParseAndValidateID(t *testing.T) {
	response := httptest.NewRecorder()
	responseData := map[string]interface{}{
		"id":   "test-id-123",
		"name": "Test Name",
	}

	jsonData, err := json.Marshal(responseData)
	if err != nil {
		t.Fatalf("Failed to marshal response data: %v", err)
	}
	if _, writeErr := response.Body.Write(jsonData); writeErr != nil {
		t.Logf("Failed to write to response body: %v", writeErr)
	}

	mockT := &MockTestingT{}
	result := ParseAndValidateID(mockT, response)

	if mockT.errorCalled {
		t.Error("Expected no error for valid ID field")
	}

	if result != "test-id-123" {
		t.Errorf("Expected ID 'test-id-123', got %s", result)
	}

	// Test with missing ID
	response = httptest.NewRecorder()
	responseData = map[string]interface{}{
		"name": "Test Name",
	}

	jsonData, err = json.Marshal(responseData)
	if err != nil {
		t.Fatalf("Failed to marshal response data: %v", err)
	}
	if _, err = response.Body.Write(jsonData); err != nil {
		t.Logf("Failed to write to response body: %v", err)
	}

	mockT = &MockTestingT{}
	_ = ParseAndValidateID(mockT, response)

	if !mockT.errorCalled {
		t.Error("Expected error for missing ID field")
	}
}

func TestValidateResponseStructure(t *testing.T) {
	response := httptest.NewRecorder()
	responseData := map[string]interface{}{
		"id":          "test-123",
		"name":        "Test Name",
		"description": "Test Description",
	}

	jsonData, err := json.Marshal(responseData)
	if err != nil {
		t.Fatalf("Failed to marshal response data: %v", err)
	}
	if _, writeErr := response.Body.Write(jsonData); writeErr != nil {
		t.Logf("Failed to write to response body: %v", writeErr)
	}

	expectedFields := []string{"id", "name", "description"}

	mockT := &MockTestingT{}
	ValidateResponseStructure(mockT, response, expectedFields)

	if mockT.errorCalled {
		t.Error("Expected no error when all expected fields are present")
	}

	// Test with missing field
	expectedFields = []string{"id", "name", "missing_field"}

	// Reset the response body
	response = httptest.NewRecorder()
	jsonData, err = json.Marshal(responseData)
	if err != nil {
		t.Fatalf("Failed to marshal response data: %v", err)
	}
	if _, err = response.Body.Write(jsonData); err != nil {
		t.Logf("Failed to write to response body: %v", err)
	}

	mockT = &MockTestingT{}
	ValidateResponseStructure(mockT, response, expectedFields)

	if !mockT.errorCalled {
		t.Error("Expected error when expected field is missing")
	}
}

func TestValidateResponseDoesNotContain(t *testing.T) {
	response := httptest.NewRecorder()
	responseData := map[string]interface{}{
		"id":          "test-123",
		"name":        "Test Name",
		"description": "Test Description",
	}

	jsonData, err := json.Marshal(responseData)
	if err != nil {
		t.Fatalf("Failed to marshal response data: %v", err)
	}
	if _, writeErr := response.Body.Write(jsonData); writeErr != nil {
		t.Logf("Failed to write to response body: %v", writeErr)
	}

	forbiddenFields := []string{"password", "secret_key"}

	mockT := &MockTestingT{}
	ValidateResponseDoesNotContain(mockT, response, forbiddenFields)

	if mockT.errorCalled {
		t.Error("Expected no error when forbidden fields are not present")
	}

	// Test with forbidden field present
	responseData["password"] = "secret"
	response = httptest.NewRecorder()
	jsonData, err = json.Marshal(responseData)
	if err != nil {
		t.Fatalf("Failed to marshal response data: %v", err)
	}
	if _, err = response.Body.Write(jsonData); err != nil {
		t.Logf("Failed to write to response body: %v", err)
	}

	mockT = &MockTestingT{}
	ValidateResponseDoesNotContain(mockT, response, forbiddenFields)

	if !mockT.errorCalled {
		t.Error("Expected error when forbidden field is present")
	}
}

func TestExtractFieldFromResponse(t *testing.T) {
	response := httptest.NewRecorder()
	responseData := map[string]interface{}{
		"id":     "test-123",
		"name":   "Test Name",
		"count":  42,
		"active": true,
	}

	jsonData, err := json.Marshal(responseData)
	if err != nil {
		t.Fatalf("Failed to marshal response data: %v", err)
	}
	if _, writeErr := response.Body.Write(jsonData); writeErr != nil {
		t.Logf("Failed to write to response body: %v", writeErr)
	}

	mockT := &MockTestingT{}

	// Test string field
	result := ExtractStringField(mockT, response, "name")
	if result != "Test Name" {
		t.Errorf("Expected 'Test Name', got %s", result)
	}

	// Reset response body for next test
	response = httptest.NewRecorder()
	jsonData, err = json.Marshal(responseData)
	if err != nil {
		t.Fatalf("Failed to marshal response data: %v", err)
	}
	if _, err = response.Body.Write(jsonData); err != nil {
		t.Logf("Failed to write to response body: %v", err)
	}

	// Test int field
	intResult := ExtractIntField(mockT, response, "count")
	if intResult != 42 {
		t.Errorf("Expected 42, got %d", intResult)
	}

	// Reset response body for next test
	response = httptest.NewRecorder()
	jsonData, err = json.Marshal(responseData)
	if err != nil {
		t.Fatalf("Failed to marshal response data: %v", err)
	}
	if _, err = response.Body.Write(jsonData); err != nil {
		t.Logf("Failed to write to response body: %v", err)
	}

	// Test bool field
	boolResult := ExtractBoolField(mockT, response, "active")
	if !boolResult {
		t.Errorf("Expected true, got %v", boolResult)
	}
}

func TestParseRawResponse(t *testing.T) {
	response := httptest.NewRecorder()
	response.Body.WriteString("This is a raw response")

	mockT := &MockTestingT{}
	result := ParseRawResponse(mockT, response)

	if mockT.errorCalled {
		t.Error("Expected no error for raw response")
	}

	if result != "This is a raw response" {
		t.Errorf("Expected 'This is a raw response', got %s", result)
	}
}
