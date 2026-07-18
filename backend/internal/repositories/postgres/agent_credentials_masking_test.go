package postgres

import (
	"testing"

	"github.com/vibexp/vibexp/internal/models"
)

// credentialNames simulates populating HasCredentials (same logic as in the
// repository): it extracts the credential names from the credentials map.
func credentialNames(credentials models.AgentCredentials) []string {
	hasCredentials := make([]string, 0, len(credentials))
	for name := range credentials {
		hasCredentials = append(hasCredentials, name)
	}
	return hasCredentials
}

// assertHasCredentialName fails the test when want is absent from names.
func assertHasCredentialName(t *testing.T, names []string, want string) {
	t.Helper()
	for _, name := range names {
		if name == want {
			return
		}
	}
	t.Errorf("Expected '%s' in hasCredentials", want)
}

// TestHasCredentials_PopulationLogic tests the logic for populating HasCredentials
// Note: Full integration tests with database would be in a separate integration test suite
func TestHasCredentials_PopulationLogic(t *testing.T) {
	t.Run("Extracts credential names from credentials map", func(t *testing.T) {
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

		hasCredentials := credentialNames(credentials)

		// Verify
		if len(hasCredentials) != 2 {
			t.Errorf("Expected 2 credential names, got %d", len(hasCredentials))
		}

		// Check both names are present
		assertHasCredentialName(t, hasCredentials, "api_key")
		assertHasCredentialName(t, hasCredentials, "oauth_token")
	})

	t.Run("Empty credentials map produces empty HasCredentials", func(t *testing.T) {
		hasCredentials := credentialNames(models.AgentCredentials{})

		if len(hasCredentials) != 0 {
			t.Errorf("Expected 0 credential names, got %d", len(hasCredentials))
		}
	})
}
