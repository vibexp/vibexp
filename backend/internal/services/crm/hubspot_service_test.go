package crm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeJSON is a test helper that writes JSON to the response writer
func writeJSON(t *testing.T, w http.ResponseWriter, v interface{}) {
	t.Helper()
	err := json.NewEncoder(w).Encode(v)
	require.NoError(t, err)
}

// writeString is a test helper that writes a string to the response writer
func writeString(t *testing.T, w http.ResponseWriter, s string) {
	t.Helper()
	_, err := w.Write([]byte(s))
	require.NoError(t, err)
}

func TestNewHubSpotService(t *testing.T) {
	logger := logrus.New()
	service := NewHubSpotService("test-token", logger)

	assert.NotNil(t, service)
	assert.Equal(t, "test-token", service.accessToken)
	assert.NotNil(t, service.httpClient)
	assert.NotNil(t, service.logger)
}

func TestNewHubSpotService_NilLogger(t *testing.T) {
	service := NewHubSpotService("test-token", nil)

	assert.NotNil(t, service)
	assert.NotNil(t, service.logger)
}

func TestBuildContactProperties(t *testing.T) {
	logger := logrus.New()
	service := NewHubSpotService("test-token", logger)

	createdAt := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	lastSeenAt := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)

	contactData := ContactData{
		Email:              "test@example.com",
		FirstName:          "John",
		LastName:           "Doe",
		CreatedAt:          &createdAt,
		SubscriptionStatus: "active",
		SubscriptionPlan:   "premium",
		LastSeenAt:         &lastSeenAt,
	}

	properties := service.buildContactProperties(contactData)

	assert.Equal(t, "test@example.com", properties["email"])
	assert.Equal(t, "John", properties["firstname"])
	assert.Equal(t, "Doe", properties["lastname"])
	assert.Equal(t, "customer", properties["lifecyclestage"])
	assert.Equal(t, "premium", properties["subscription_plan"])
	assert.NotEmpty(t, properties["hs_analytics_last_timestamp"])
	assert.NotEmpty(t, properties["hs_analytics_last_visit_timestamp"])
}

func TestBuildContactProperties_MinimalData(t *testing.T) {
	logger := logrus.New()
	service := NewHubSpotService("test-token", logger)

	contactData := ContactData{
		Email:     "test@example.com",
		FirstName: "John",
	}

	properties := service.buildContactProperties(contactData)

	assert.Equal(t, "test@example.com", properties["email"])
	assert.Equal(t, "John", properties["firstname"])
	assert.Empty(t, properties["lastname"])
	assert.Empty(t, properties["lifecyclestage"])
	assert.Empty(t, properties["subscription_plan"])
	assert.Empty(t, properties["createdate"])
	assert.Empty(t, properties["hs_analytics_last_timestamp"])
}

func TestMapSubscriptionStatusToLifecycleStage(t *testing.T) {
	logger := logrus.New()
	service := NewHubSpotService("test-token", logger)

	tests := []struct {
		status   string
		expected string
	}{
		{"active", "customer"},
		{"Active", "customer"},
		{"ACTIVE", "customer"},
		{"trialing", "opportunity"},
		{"canceled", "former customer"},
		{"cancelled", "former customer"},
		{"unknown", "lead"},
		{"", "lead"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := service.mapSubscriptionStatusToLifecycleStage(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateBackoff(t *testing.T) {
	logger := logrus.New()
	service := NewHubSpotService("test-token", logger)

	// Test that backoff increases with attempt number
	backoff0 := service.calculateBackoff(0)
	backoff1 := service.calculateBackoff(1)
	backoff2 := service.calculateBackoff(2)

	// Verify exponential growth (approximately, with jitter)
	assert.Greater(t, backoff1.Nanoseconds(), backoff0.Nanoseconds())
	assert.Greater(t, backoff2.Nanoseconds(), backoff1.Nanoseconds())

	// Backoff should be at least the initial backoff
	assert.GreaterOrEqual(t, backoff0, initialBackoff)
}

func TestMakeRequest_InvalidURL(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	service := NewHubSpotService("test-token", logger)

	ctx := context.Background()
	statusCode, respBody, err := service.makeRequest(ctx, "GET", "://invalid-url", nil)

	assert.Error(t, err)
	assert.Equal(t, 0, statusCode)
	assert.Nil(t, respBody)
}

func TestCreateContact_ContextCancellation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	service := NewHubSpotService("test-token", logger)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	contactData := ContactData{
		Email:     "test@example.com",
		FirstName: "John",
	}

	err := service.CreateContact(ctx, contactData)

	// Should fail due to context cancellation
	assert.Error(t, err)
}

func TestUpdateContact_ContextCancellation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	service := NewHubSpotService("test-token", logger)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	contactData := ContactData{
		Email:     "test@example.com",
		FirstName: "John",
	}

	err := service.UpdateContact(ctx, "test@example.com", contactData)

	// Should fail due to context cancellation
	assert.Error(t, err)
}

func TestGetContactByEmail_ContextCancellation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	service := NewHubSpotService("test-token", logger)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	contact, err := service.GetContactByEmail(ctx, "test@example.com")

	// Should fail due to context cancellation
	assert.Error(t, err)
	assert.Nil(t, contact)
}

func TestBuildContactProperties_EmptyValues(t *testing.T) {
	logger := logrus.New()
	service := NewHubSpotService("test-token", logger)

	contactData := ContactData{}

	properties := service.buildContactProperties(contactData)

	// Should have empty properties for empty data
	assert.Empty(t, properties["email"])
	assert.Empty(t, properties["firstname"])
	assert.Empty(t, properties["lastname"])
}

func TestBuildContactProperties_Timestamps(t *testing.T) {
	logger := logrus.New()
	service := NewHubSpotService("test-token", logger)

	createdAt := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	contactData := ContactData{
		Email:     "test@example.com",
		CreatedAt: &createdAt,
	}

	properties := service.buildContactProperties(contactData)

	// createdate is read-only in HubSpot and should not be set
	assert.Empty(t, properties["createdate"])
}

func TestBuildContactProperties_PromptTracking(t *testing.T) {
	logger := logrus.New()
	service := NewHubSpotService("test-token", logger)

	lastPromptCreated := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	totalPrompts := 5

	contactData := ContactData{
		Email:               "test@example.com",
		FirstName:           "John",
		LastPromptCreatedAt: &lastPromptCreated,
		TotalPrompts:        &totalPrompts,
	}

	properties := service.buildContactProperties(contactData)

	// Verify prompt tracking fields are set
	assert.Equal(t, "test@example.com", properties["email"])
	assert.NotEmpty(t, properties["last_vibexp_prompt_created_at"])
	assert.Equal(t, "5", properties["total_vibexp_prompts"])

	// Verify timestamp is in milliseconds (Unix millisecond timestamp)
	expectedTimestamp := lastPromptCreated.UnixMilli()
	assert.True(t, time.UnixMilli(expectedTimestamp).Equal(lastPromptCreated),
		"Timestamp conversion should preserve the same instant in time")
}

func TestBuildContactProperties_AIToolIntegration(t *testing.T) {
	logger := logrus.New()
	service := NewHubSpotService("test-token", logger)

	totalTools := 2
	contactData := ContactData{
		Email:                  "test@example.com",
		FirstName:              "John",
		AIToolsIntegrated:      []string{"claude_code_cli", "cursor_ide"},
		TotalAIToolsIntegrated: &totalTools,
	}

	properties := service.buildContactProperties(contactData)

	// Verify AI tool integration fields are set
	assert.Equal(t, "test@example.com", properties["email"])
	assert.Equal(t, "claude_code_cli;cursor_ide", properties["vibexp_ai_tools_integrated"])
	assert.Equal(t, "2", properties["total_vibexp_ai_tools_integrated"])
}

func TestBuildContactProperties_AIToolIntegration_SingleTool(t *testing.T) {
	logger := logrus.New()
	service := NewHubSpotService("test-token", logger)

	totalTools := 1
	contactData := ContactData{
		Email:                  "test@example.com",
		AIToolsIntegrated:      []string{"claude_code_cli"},
		TotalAIToolsIntegrated: &totalTools,
	}

	properties := service.buildContactProperties(contactData)

	// Verify single tool integration
	assert.Equal(t, "claude_code_cli", properties["vibexp_ai_tools_integrated"])
	assert.Equal(t, "1", properties["total_vibexp_ai_tools_integrated"])
}

func TestBuildContactProperties_AIToolIntegration_EmptyArray(t *testing.T) {
	logger := logrus.New()
	service := NewHubSpotService("test-token", logger)

	contactData := ContactData{
		Email:             "test@example.com",
		AIToolsIntegrated: []string{},
	}

	properties := service.buildContactProperties(contactData)

	// Verify empty array doesn't set the field
	assert.Empty(t, properties["vibexp_ai_tools_integrated"])
}

func TestBuildContactProperties_AllNewFields(t *testing.T) {
	logger := logrus.New()
	service := NewHubSpotService("test-token", logger)

	lastPromptCreated := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	totalPrompts := 10
	totalTools := 2

	contactData := ContactData{
		Email:                  "test@example.com",
		FirstName:              "John",
		LastName:               "Doe",
		LastPromptCreatedAt:    &lastPromptCreated,
		TotalPrompts:           &totalPrompts,
		AIToolsIntegrated:      []string{"claude_code_cli", "cursor_ide"},
		TotalAIToolsIntegrated: &totalTools,
	}

	properties := service.buildContactProperties(contactData)

	// Verify all fields are set correctly
	assert.Equal(t, "test@example.com", properties["email"])
	assert.Equal(t, "John", properties["firstname"])
	assert.Equal(t, "Doe", properties["lastname"])
	assert.NotEmpty(t, properties["last_vibexp_prompt_created_at"])
	assert.Equal(t, "10", properties["total_vibexp_prompts"])
	assert.Equal(t, "claude_code_cli;cursor_ide", properties["vibexp_ai_tools_integrated"])
	assert.Equal(t, "2", properties["total_vibexp_ai_tools_integrated"])
}

func TestHubSpotService_UpdateContact_ContactNotFound(t *testing.T) {
	// Create mock server that returns 404 for contact lookup
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		writeString(t, w, `{"status":"error","message":"Contact not found"}`)
	}))
	defer server.Close()

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	service := NewHubSpotService("test-token", logger)
	service.baseURL = server.URL

	ctx := context.Background()

	contactData := ContactData{
		Email:     "test@example.com",
		FirstName: "John",
		LastName:  "Doe",
	}

	err := service.UpdateContact(ctx, "nonexistent@example.com", contactData)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get contact by email")
}

func TestHubSpotService_GetContactByEmail_NotFound(t *testing.T) {
	// Create mock server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		writeString(t, w, `{"status":"error","message":"Contact not found"}`)
	}))
	defer server.Close()

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	service := NewHubSpotService("test-token", logger)
	service.baseURL = server.URL

	ctx := context.Background()

	contact, err := service.GetContactByEmail(ctx, "nonexistent@example.com")

	assert.Error(t, err)
	assert.Nil(t, contact)
}

func TestHubSpotService_GetContactByEmail_Success(t *testing.T) {
	// Create mock server that returns a single contact object
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GetContactByEmail expects a single contact object, not results array
		response := map[string]interface{}{
			"id": "123",
			"properties": map[string]string{
				"email":     "test@example.com",
				"firstname": "John",
				"lastname":  "Doe",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		writeJSON(t, w, response)
	}))
	defer server.Close()

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	service := NewHubSpotService("test-token", logger)
	service.baseURL = server.URL

	ctx := context.Background()

	contact, err := service.GetContactByEmail(ctx, "test@example.com")

	assert.NoError(t, err)
	assert.NotNil(t, contact)
	assert.Equal(t, "123", contact.ID)
	assert.Equal(t, "test@example.com", contact.Email)
	assert.Equal(t, "John", contact.FirstName)
}

func TestHubSpotService_CreateContact_ServerError(t *testing.T) {
	// Create mock server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		writeString(t, w, `{"status":"error","message":"Internal server error"}`)
	}))
	defer server.Close()

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	service := NewHubSpotService("test-token", logger)
	service.baseURL = server.URL

	ctx := context.Background()

	contactData := ContactData{
		Email:     "test@example.com",
		FirstName: "John",
	}

	err := service.CreateContact(ctx, contactData)

	assert.Error(t, err)
}

func TestHubSpotService_CreateContact_Success(t *testing.T) {
	// Create mock server that returns 201 for contact creation
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id": "456",
			"properties": map[string]string{
				"email":     "test@example.com",
				"firstname": "John",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		writeJSON(t, w, response)
	}))
	defer server.Close()

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	service := NewHubSpotService("test-token", logger)
	service.baseURL = server.URL

	ctx := context.Background()

	contactData := ContactData{
		Email:     "test@example.com",
		FirstName: "John",
	}

	err := service.CreateContact(ctx, contactData)

	assert.NoError(t, err)
}

func TestMapSubscriptionStatusToLifecycleStage_AllCases(t *testing.T) {
	logger := logrus.New()
	service := NewHubSpotService("test-token", logger)

	tests := []struct {
		status   string
		expected string
	}{
		{"active", "customer"},
		{"Active", "customer"},
		{"ACTIVE", "customer"},
		{"trialing", "opportunity"},
		{"Trialing", "opportunity"},
		{"TRIALING", "opportunity"},
		{"canceled", "former customer"},
		{"Canceled", "former customer"},
		{"CANCELED", "former customer"},
		{"cancelled", "former customer"},
		{"Cancelled", "former customer"},
		{"CANCELLED", "former customer"},
		{"pending", "lead"},
		{"expired", "lead"},
		{"unknown", "lead"},
		{"", "lead"},
		{"random_status", "lead"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := service.mapSubscriptionStatusToLifecycleStage(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateBackoff_ExponentialGrowth(t *testing.T) {
	logger := logrus.New()
	service := NewHubSpotService("test-token", logger)

	// Run multiple iterations to account for jitter
	for i := 0; i < 10; i++ {
		backoff0 := service.calculateBackoff(0)
		backoff1 := service.calculateBackoff(1)
		backoff2 := service.calculateBackoff(2)
		backoff3 := service.calculateBackoff(3)

		// Verify backoff is at least the base duration
		assert.GreaterOrEqual(t, backoff0, initialBackoff)

		// Verify exponential growth (approximately, accounting for jitter)
		// backoff1 should be roughly 2x backoff0
		// backoff2 should be roughly 2x backoff1
		assert.Greater(t, backoff1.Nanoseconds(), backoff0.Nanoseconds())
		assert.Greater(t, backoff2.Nanoseconds(), backoff1.Nanoseconds())
		assert.Greater(t, backoff3.Nanoseconds(), backoff2.Nanoseconds())
	}
}

func TestHubSpotService_MakeRequest_WithBody(t *testing.T) {
	// Create mock server that accepts POST requests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer test-token")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		writeString(t, w, `{"success": true}`)
	}))
	defer server.Close()

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	service := NewHubSpotService("test-token", logger)

	ctx := context.Background()
	body := []byte(`{"test": "data"}`)

	statusCode, respBody, err := service.makeRequest(ctx, "POST", server.URL+"/test", body)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.NotNil(t, respBody)
	assert.Contains(t, string(respBody), "success")
}

func TestHubSpotService_MakeRequest_GET(t *testing.T) {
	// Create mock server that accepts GET requests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer test-token")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		writeString(t, w, `{"results": []}`)
	}))
	defer server.Close()

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	service := NewHubSpotService("test-token", logger)

	ctx := context.Background()

	statusCode, respBody, err := service.makeRequest(ctx, "GET", server.URL+"/crm/v3/objects/contacts", nil)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.NotNil(t, respBody)
	assert.Contains(t, string(respBody), "results")
}

func TestHubSpotService_MakeRequest_Unauthorized(t *testing.T) {
	// Create mock server that returns 401
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		writeString(t, w, `{"status":"error","message":"Unauthorized"}`)
	}))
	defer server.Close()

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	service := NewHubSpotService("invalid-token", logger)

	ctx := context.Background()

	statusCode, respBody, err := service.makeRequest(ctx, "GET", server.URL+"/test", nil)

	assert.NoError(t, err) // No network error, just auth error
	assert.Equal(t, http.StatusUnauthorized, statusCode)
	assert.NotNil(t, respBody)
}

func TestHubSpotService_UpdateContact_Success(t *testing.T) {
	callCount := 0
	// Create mock server that handles both GET (lookup) and PATCH (update)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call: GetContactByEmail - returns single contact object
			response := map[string]interface{}{
				"id": "123",
				"properties": map[string]string{
					"email":     "test@example.com",
					"firstname": "John",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			writeJSON(t, w, response)
		} else {
			// Second call: Update contact
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			writeString(t, w, `{"id": "123"}`)
		}
	}))
	defer server.Close()

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	service := NewHubSpotService("test-token", logger)
	service.baseURL = server.URL

	ctx := context.Background()

	contactData := ContactData{
		Email:     "test@example.com",
		FirstName: "John",
		LastName:  "Doe",
	}

	err := service.UpdateContact(ctx, "test@example.com", contactData)

	assert.NoError(t, err)
	assert.Equal(t, 2, callCount)
}
