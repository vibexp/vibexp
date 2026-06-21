package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a valid agent card JSON
func createValidAgentCardJSON() string {
	return `{
		"protocolVersion": "0.3.0",
		"name": "Test Agent",
		"description": "A test agent for unit testing",
		"url": "http://localhost:8000",
		"version": "1.0.0",
		"capabilities": {},
		"defaultInputModes": ["text/plain"],
		"defaultOutputModes": ["text/plain"],
		"skills": [
			{
				"id": "test-skill",
				"name": "Test Skill",
				"description": "A test skill",
				"tags": ["test"]
			}
		]
	}`
}

func TestNewAgentCardFetcher(t *testing.T) {
	fetcher := NewAgentCardFetcher()

	assert.NotNil(t, fetcher)
	assert.NotNil(t, fetcher.httpClient)
	assert.Equal(t, 30*time.Second, fetcher.httpClient.Timeout)
}

func TestAgentCardFetcher_FetchAgentCard_Success(t *testing.T) {
	t.Run("Successful agent card fetch", func(t *testing.T) {
		validJSON := createValidAgentCardJSON()

		// Create test server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request headers
			assert.Equal(t, "GET", r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Accept"))
			assert.Equal(t, "VibExp-Agent-Discovery/1.0", r.Header.Get("User-Agent"))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(validJSON))
			require.NoError(t, err)
		}))
		defer server.Close()

		fetcher := newTestAgentCardFetcher()
		card, err := fetcher.FetchAgentCard(context.Background(), server.URL+"/.well-known/agent-card.json")

		assert.NoError(t, err)
		assert.NotNil(t, card)
		assert.Equal(t, "0.3.0", card.ProtocolVersion)
		assert.Equal(t, "Test Agent", card.Name)
		assert.Equal(t, "A test agent for unit testing", card.Description)
		assert.Equal(t, "http://localhost:8000", card.URL)
		assert.Equal(t, "1.0.0", card.Version)
		assert.NotNil(t, card.Capabilities)
		assert.Len(t, card.DefaultInputModes, 1)
		assert.Len(t, card.DefaultOutputModes, 1)
		assert.Len(t, card.Skills, 1)
		assert.Equal(t, "test-skill", card.Skills[0].ID)
	})
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAgentCardFetcher_FetchAgentCard_URLValidation(t *testing.T) {
	tests := []struct {
		name        string
		cardURL     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Invalid URL format",
			cardURL:     "not-a-url",
			expectError: true,
			errorMsg:    "invalid URL scheme",
		},
		{
			name:        "Invalid URL scheme - FTP",
			cardURL:     "ftp://localhost:8000/.well-known/agent-card.json",
			expectError: true,
			errorMsg:    "invalid URL scheme",
		},
		{
			name:        "Invalid URL scheme - file",
			cardURL:     "file:///tmp/agent-card.json",
			expectError: true,
			errorMsg:    "invalid URL scheme",
		},
		{
			name:        "Invalid path - root",
			cardURL:     "http://localhost:8000/",
			expectError: true,
			errorMsg:    "invalid URL path",
		},
		{
			name:        "Invalid path - custom API",
			cardURL:     "http://localhost:8000/api/agent-card",
			expectError: true,
			errorMsg:    "invalid URL path",
		},
		{
			name:        "Invalid path - almost correct",
			cardURL:     "http://localhost:8000/.well-known/agent.json",
			expectError: true,
			errorMsg:    "invalid URL path",
		},
		{
			name:        "Invalid path - wrong directory",
			cardURL:     "http://localhost:8000/well-known/agent-card.json",
			expectError: true,
			errorMsg:    "invalid URL path",
		},
		{
			name:        "Invalid path - metadata endpoint attempt",
			cardURL:     "http://169.254.169.254/latest/meta-data/",
			expectError: true,
			errorMsg:    "invalid URL path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := NewAgentCardFetcher()

			_, err := fetcher.FetchAgentCard(context.Background(), tt.cardURL)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAgentCardFetcher_FetchAgentCard_HTTPErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		errorMsg   string
	}{
		{
			name:       "Not Found - 404",
			statusCode: 404,
			errorMsg:   "agent card not found",
		},
		{
			name:       "Unauthorized - 401",
			statusCode: 401,
			errorMsg:   "unauthorized",
		},
		{
			name:       "Forbidden - 403",
			statusCode: 403,
			errorMsg:   "access forbidden",
		},
		{
			name:       "Internal Server Error - 500",
			statusCode: 500,
			errorMsg:   "server error",
		},
		{
			name:       "Bad Gateway - 502",
			statusCode: 502,
			errorMsg:   "bad gateway",
		},
		{
			name:       "Service Unavailable - 503",
			statusCode: 503,
			errorMsg:   "service unavailable",
		},
		{
			name:       "Gateway Timeout - 504",
			statusCode: 504,
			errorMsg:   "gateway timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server that returns the specified status code
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			fetcher := newTestAgentCardFetcher()
			_, err := fetcher.FetchAgentCard(context.Background(), server.URL+"/.well-known/agent-card.json")

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

func TestAgentCardFetcher_FetchAgentCard_InvalidJSON(t *testing.T) {
	t.Run("Invalid JSON format", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(`{"name": "Test Agent", "invalid": json}`))
			require.NoError(t, err)
		}))
		defer server.Close()

		fetcher := newTestAgentCardFetcher()
		_, err := fetcher.FetchAgentCard(context.Background(), server.URL+"/.well-known/agent-card.json")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unable to parse agent card response")
	})
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAgentCardFetcher_FetchAgentCard_ValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		cardJSON string
		errorMsg string
	}{
		{
			name: "Missing protocol version",
			cardJSON: `{
				"name": "Test Agent",
				"description": "A test agent",
				"url": "http://localhost:8000",
				"version": "1.0.0",
				"capabilities": {},
				"defaultInputModes": ["text/plain"],
				"defaultOutputModes": ["text/plain"],
				"skills": [{"id": "test", "name": "Test", "description": "Test", "tags": ["test"]}]
			}`,
			errorMsg: "the 'protocolVersion' field is required",
		},
		{
			name: "Missing name",
			cardJSON: `{
				"protocolVersion": "0.3.0",
				"description": "A test agent",
				"url": "http://localhost:8000",
				"version": "1.0.0",
				"capabilities": {},
				"defaultInputModes": ["text/plain"],
				"defaultOutputModes": ["text/plain"],
				"skills": [{"id": "test", "name": "Test", "description": "Test", "tags": ["test"]}]
			}`,
			errorMsg: "the 'name' field is required",
		},
		{
			name: "Missing description",
			cardJSON: `{
				"protocolVersion": "0.3.0",
				"name": "Test Agent",
				"url": "http://localhost:8000",
				"version": "1.0.0",
				"capabilities": {},
				"defaultInputModes": ["text/plain"],
				"defaultOutputModes": ["text/plain"],
				"skills": [{"id": "test", "name": "Test", "description": "Test", "tags": ["test"]}]
			}`,
			errorMsg: "the 'description' field is required",
		},
		{
			name: "Skill missing ID",
			cardJSON: `{
				"protocolVersion": "0.3.0",
				"name": "Test Agent",
				"description": "A test agent",
				"url": "http://localhost:8000",
				"version": "1.0.0",
				"capabilities": {},
				"defaultInputModes": ["text/plain"],
				"defaultOutputModes": ["text/plain"],
				"skills": [{"name": "Test", "description": "Test", "tags": ["test"]}]
			}`,
			errorMsg: "skill #1: the 'id' field is required",
		},
		{
			name: "Skill missing tags",
			cardJSON: `{
				"protocolVersion": "0.3.0",
				"name": "Test Agent",
				"description": "A test agent",
				"url": "http://localhost:8000",
				"version": "1.0.0",
				"capabilities": {},
				"defaultInputModes": ["text/plain"],
				"defaultOutputModes": ["text/plain"],
				"skills": [{"id": "test", "name": "Test", "description": "Test"}]
			}`,
			errorMsg: "skill #1 ('test'): the 'tags' field is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte(tt.cardJSON))
				require.NoError(t, err)
			}))
			defer server.Close()

			fetcher := newTestAgentCardFetcher()
			_, err := fetcher.FetchAgentCard(context.Background(), server.URL+"/.well-known/agent-card.json")

			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid agent card format")
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

func TestAgentCardFetcher_FetchAgentCard_ContentTypeWarning(t *testing.T) {
	t.Run("Unexpected content type should still work", func(t *testing.T) {
		validJSON := createValidAgentCardJSON()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain") // Unexpected content type
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(validJSON))
			require.NoError(t, err)
		}))
		defer server.Close()

		fetcher := newTestAgentCardFetcher()
		card, err := fetcher.FetchAgentCard(context.Background(), server.URL+"/.well-known/agent-card.json")

		// Should succeed despite unexpected content type
		assert.NoError(t, err)
		assert.NotNil(t, card)
		assert.Equal(t, "Test Agent", card.Name)
	})
}

func TestAgentCardFetcher_FetchAgentCard_ResponseSizeLimit(t *testing.T) {
	t.Run("Large response should be rejected", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			// Create a response larger than MaxResponseSize (1MB)
			largeResponse := make([]byte, MaxResponseSize+1000)
			for i := range largeResponse {
				largeResponse[i] = 'a'
			}
			_, err := w.Write(largeResponse)
			if err != nil {
				// Error writing large response is acceptable in this test
				return
			}
		}))
		defer server.Close()

		fetcher := newTestAgentCardFetcher()
		card, err := fetcher.FetchAgentCard(context.Background(), server.URL+"/.well-known/agent-card.json")

		assert.Error(t, err)
		assert.Nil(t, card)
		// Should fail with size limit error
		assert.Contains(t, err.Error(), "agent card response too large")
	})
}
