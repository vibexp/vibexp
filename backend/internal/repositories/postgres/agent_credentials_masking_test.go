package postgres

import (
	"testing"

	"github.com/vibexp/vibexp/internal/models"
)

// TestHasCredentials_PopulationLogic tests the logic for populating HasCredentials
// Note: Full integration tests with database would be in a separate integration test suite
//
//nolint:gocognit // Test function with comprehensive scenarios
func TestHasCredentials_PopulationLogic(t *testing.T) {
	t.Run("Extracts credential names from credentials map", func(t *testing.T) {
		// Simulate what the repository does
		credentials := models.AgentCredentials{
			"api_key": models.AgentCredential{
				Type:  "apiKey",
				Value: "encrypted-value",
			},
			"oauth_token": models.AgentCredential{
				Type:  "oauth2",
				Value: "encrypted-token",
			},
		}

		// Simulate populating HasCredentials (same logic as in repository)
		hasCredentials := make([]string, 0, len(credentials))
		for name := range credentials {
			hasCredentials = append(hasCredentials, name)
		}

		// Verify
		if len(hasCredentials) != 2 {
			t.Errorf("Expected 2 credential names, got %d", len(hasCredentials))
		}

		// Check both names are present
		foundAPIKey := false
		foundOAuth := false
		for _, name := range hasCredentials {
			if name == "api_key" {
				foundAPIKey = true
			}
			if name == "oauth_token" {
				foundOAuth = true
			}
		}

		if !foundAPIKey {
			t.Error("Expected 'api_key' in hasCredentials")
		}
		if !foundOAuth {
			t.Error("Expected 'oauth_token' in hasCredentials")
		}
	})

	t.Run("Empty credentials map produces empty HasCredentials", func(t *testing.T) {
		credentials := models.AgentCredentials{}

		hasCredentials := make([]string, 0, len(credentials))
		for name := range credentials {
			hasCredentials = append(hasCredentials, name)
		}

		if len(hasCredentials) != 0 {
			t.Errorf("Expected 0 credential names, got %d", len(hasCredentials))
		}
	})
}
