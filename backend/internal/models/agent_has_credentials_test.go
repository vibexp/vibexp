package models

import (
	"encoding/json"
	"testing"
)

// TestAgent_HasCredentials tests that HasCredentials field is properly marshaled/unmarshaled
func TestAgent_HasCredentials(t *testing.T) {
	t.Run("Agent with HasCredentials is properly marshaled to JSON", func(t *testing.T) {
		agent := &Agent{
			ID:             "test-123",
			UserID:         "user-456",
			Name:           "Test Agent",
			Description:    "Test Description",
			Status:         "active",
			HasCredentials: []string{"api_key", "oauth_token"},
		}

		result := marshalAndUnmarshalAgent(t, agent)
		verifyHasCredentialsExists(t, result, 2)
	})

	t.Run("Agent without HasCredentials omits field from JSON", func(t *testing.T) {
		agent := createTestAgentWithoutCredentials()
		result := marshalAndUnmarshalAgent(t, agent)
		verifyHasCredentialsOmitted(t, result)
	})

	t.Run("Agent with empty HasCredentials array omits field from JSON", func(t *testing.T) {
		agent := createTestAgentWithEmptyCredentials()
		result := marshalAndUnmarshalAgent(t, agent)
		verifyHasCredentialsOmitted(t, result)
	})

	t.Run("Credentials field is never exposed in JSON", func(t *testing.T) {
		agent := createTestAgentWithCredentials()
		result := marshalAndUnmarshalAgent(t, agent)
		verifyCredentialsNotExposed(t, result)
	})
}

// createTestAgentWithoutCredentials creates a test agent without credentials
func createTestAgentWithoutCredentials() *Agent {
	return &Agent{
		ID:          "test-123",
		UserID:      "user-456",
		Name:        "Test Agent",
		Description: "Test Description",
		Status:      "active",
	}
}

// createTestAgentWithEmptyCredentials creates a test agent with empty credentials array
func createTestAgentWithEmptyCredentials() *Agent {
	return &Agent{
		ID:             "test-123",
		UserID:         "user-456",
		Name:           "Test Agent",
		Description:    "Test Description",
		Status:         "active",
		HasCredentials: []string{},
	}
}

// createTestAgentWithCredentials creates a test agent with credentials
func createTestAgentWithCredentials() *Agent {
	creds := AgentCredentials{
		"api_key": AgentCredential{
			Type:  "apiKey",
			Value: "secret-value",
		},
	}

	return &Agent{
		ID:             "test-123",
		UserID:         "user-456",
		Name:           "Test Agent",
		Description:    "Test Description",
		Status:         "active",
		Credentials:    &creds,
		HasCredentials: []string{"api_key"},
	}
}

// marshalAndUnmarshalAgent marshals and unmarshals an agent to/from JSON
func marshalAndUnmarshalAgent(t *testing.T, agent *Agent) map[string]interface{} {
	t.Helper()

	jsonData, err := json.Marshal(agent)
	if err != nil {
		t.Fatalf("Failed to marshal agent: %v", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal(jsonData, &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	return result
}

// verifyHasCredentialsExists checks that has_credentials field exists with expected count
func verifyHasCredentialsExists(t *testing.T, result map[string]interface{}, expectedCount int) {
	t.Helper()

	hasCreds, exists := result["has_credentials"]
	if !exists {
		t.Error("Expected has_credentials field in JSON")
		return
	}

	credsList, ok := hasCreds.([]interface{})
	if !ok {
		t.Fatalf("Expected has_credentials to be an array, got %T", hasCreds)
	}

	if len(credsList) != expectedCount {
		t.Errorf("Expected %d credentials, got %d", expectedCount, len(credsList))
	}
}

// verifyHasCredentialsOmitted checks that has_credentials field is omitted
func verifyHasCredentialsOmitted(t *testing.T, result map[string]interface{}) {
	t.Helper()

	_, exists := result["has_credentials"]
	if exists {
		t.Error("Expected has_credentials field to be omitted")
	}
}

// verifyCredentialsNotExposed checks that credentials are not exposed and has_credentials is present
func verifyCredentialsNotExposed(t *testing.T, result map[string]interface{}) {
	t.Helper()

	// Credentials should NOT be in JSON (json:"-" tag)
	_, exists := result["credentials"]
	if exists {
		t.Error("Credentials field should never be exposed in JSON")
	}

	// But HasCredentials should be present
	hasCreds, exists := result["has_credentials"]
	if !exists {
		t.Error("Expected has_credentials field in JSON")
		return
	}

	credsList, ok := hasCreds.([]interface{})
	if !ok {
		t.Fatalf("Expected has_credentials to be an array, got %T", hasCreds)
	}

	if len(credsList) != 1 {
		t.Errorf("Expected 1 credential name, got %d", len(credsList))
		return
	}

	if credsList[0] != "api_key" {
		t.Errorf("Expected 'api_key', got %v", credsList[0])
	}
}
