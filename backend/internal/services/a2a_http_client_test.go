package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
)

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestA2AHTTPClient_InvokeAgent_Success(t *testing.T) {
	// Create a test server that returns a successful JSON-RPC response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))

		// Parse request
		var req JSONRPC2Request
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, "2.0", req.JSONRPC)
		assert.Equal(t, "message/send", req.Method)

		// Verify A2A v0.3.0 message structure
		params, ok := req.Params.(map[string]interface{})
		require.True(t, ok)
		message, ok := params["message"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "user", message["role"])
		assert.NotEmpty(t, message["messageId"])

		// Send successful A2A v0.3.0 response
		response := JSONRPC2Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"kind": "task",
				"id":   "task-123",
				"status": map[string]interface{}{
					"state": "completed",
				},
				"artifacts": []map[string]interface{}{
					{
						"artifactId": "artifact-1",
						"name":       "response",
						"parts": []map[string]interface{}{
							{
								"kind": "text",
								"text": "Task completed successfully",
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(response))
	}))
	defer server.Close()

	// Create agent with test server URL
	agent := &models.Agent{
		ID:     "test-agent-1",
		UserID: "user-1",
		Name:   "Test Agent",
		Status: "active",
		AgentCard: &models.AgentCard{
			SupportedInterfaces: []*a2a.AgentInterface{
				{URL: server.URL, ProtocolBinding: a2a.TransportProtocolHTTPJSON},
			},
		},
	}

	// Create mock encryption service and authenticator
	encryptionService := &mockEncryptionService{}
	authenticator := NewAgentAuthenticator(encryptionService)
	cfg := &config.Config{} // Use default config for tests
	client := newTestA2AHTTPClient(authenticator, cfg)

	input := map[string]interface{}{
		"text": "Hello, agent!",
	}

	execution, err := client.InvokeAgent(context.Background(), agent, input, nil)

	require.NoError(t, err)
	assert.NotNil(t, execution)
	assert.Equal(t, "completed", execution.Status)
	// Artifacts are populated from streaming events, not from Output
	assert.Nil(t, execution.Error)
}

func TestA2AHTTPClient_InvokeAgent_HTTPInterface(t *testing.T) {
	// Test with AdditionalInterfaces.HTTP.URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := JSONRPC2Response{
			JSONRPC: "2.0",
			ID:      "test",
			Result: map[string]interface{}{
				"message": "success",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(response))
	}))
	defer server.Close()

	agent := &models.Agent{
		ID:     "test-agent-1",
		UserID: "user-1",
		Status: "active",
		AgentCard: &models.AgentCard{
			SupportedInterfaces: []*a2a.AgentInterface{
				{URL: "http://should-not-use.com", ProtocolBinding: a2a.TransportProtocolJSONRPC},
				{URL: server.URL, ProtocolBinding: a2a.TransportProtocolHTTPJSON},
			},
		},
	}

	encryptionService := &mockEncryptionService{}
	authenticator := NewAgentAuthenticator(encryptionService)
	cfg := &config.Config{} // Use default config for tests
	client := newTestA2AHTTPClient(authenticator, cfg)

	execution, err := client.InvokeAgent(context.Background(), agent, map[string]interface{}{}, nil)

	require.NoError(t, err)
	assert.Equal(t, "completed", execution.Status)
}

func TestA2AHTTPClient_InvokeAgent_JSONRPCError(t *testing.T) {
	// Create a test server that returns a JSON-RPC error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPC2Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		response := JSONRPC2Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPC2Error{
				Code:    -32601,
				Message: "Method not found",
				Data: map[string]interface{}{
					"method": req.Method,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(response))
	}))
	defer server.Close()

	agent := &models.Agent{
		ID:     "test-agent-1",
		UserID: "user-1",
		Status: "active",
		AgentCard: &models.AgentCard{
			SupportedInterfaces: []*a2a.AgentInterface{
				{URL: server.URL, ProtocolBinding: a2a.TransportProtocolHTTPJSON},
			},
		},
	}

	encryptionService := &mockEncryptionService{}
	authenticator := NewAgentAuthenticator(encryptionService)
	cfg := &config.Config{} // Use default config for tests
	client := newTestA2AHTTPClient(authenticator, cfg)

	execution, err := client.InvokeAgent(context.Background(), agent, map[string]interface{}{}, nil)

	require.NoError(t, err)
	assert.NotNil(t, execution)
	assert.Equal(t, "error", execution.Status)
	assert.NotNil(t, execution.Error)
	assert.Equal(t, "Method not found", *execution.Error)
}

func TestA2AHTTPClient_InvokeAgent_HTTPError(t *testing.T) {
	// Create a test server that returns an HTTP error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte("Internal server error"))
		require.NoError(t, err)
	}))
	defer server.Close()

	agent := &models.Agent{
		ID:     "test-agent-1",
		UserID: "user-1",
		Status: "active",
		AgentCard: &models.AgentCard{
			SupportedInterfaces: []*a2a.AgentInterface{
				{URL: server.URL, ProtocolBinding: a2a.TransportProtocolHTTPJSON},
			},
		},
	}

	encryptionService := &mockEncryptionService{}
	authenticator := NewAgentAuthenticator(encryptionService)
	cfg := &config.Config{} // Use default config for tests
	client := newTestA2AHTTPClient(authenticator, cfg)

	execution, err := client.InvokeAgent(context.Background(), agent, map[string]interface{}{}, nil)

	require.Error(t, err)
	assert.Nil(t, execution)
	assert.Contains(t, err.Error(), "agent returned status 500")
}

func TestA2AHTTPClient_InvokeAgent_InvalidJSON(t *testing.T) {
	// Create a test server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte("not valid json"))
		require.NoError(t, err)
	}))
	defer server.Close()

	agent := &models.Agent{
		ID:     "test-agent-1",
		UserID: "user-1",
		Status: "active",
		AgentCard: &models.AgentCard{
			SupportedInterfaces: []*a2a.AgentInterface{
				{URL: server.URL, ProtocolBinding: a2a.TransportProtocolHTTPJSON},
			},
		},
	}

	encryptionService := &mockEncryptionService{}
	authenticator := NewAgentAuthenticator(encryptionService)
	cfg := &config.Config{} // Use default config for tests
	client := newTestA2AHTTPClient(authenticator, cfg)

	execution, err := client.InvokeAgent(context.Background(), agent, map[string]interface{}{}, nil)

	require.Error(t, err)
	assert.Nil(t, execution)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestA2AHTTPClient_InvokeAgent_Timeout(t *testing.T) {
	// Create a test server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_, err := w.Write([]byte("should not reach"))
		if err != nil {
			// Timeout may cause write failure, which is expected
			return
		}
	}))
	defer server.Close()

	agent := &models.Agent{
		ID:     "test-agent-1",
		UserID: "user-1",
		Status: "active",
		AgentCard: &models.AgentCard{
			SupportedInterfaces: []*a2a.AgentInterface{
				{URL: server.URL, ProtocolBinding: a2a.TransportProtocolHTTPJSON},
			},
		},
	}

	encryptionService := &mockEncryptionService{}
	authenticator := NewAgentAuthenticator(encryptionService)
	cfg := &config.Config{} // Use default config for tests
	client := newTestA2AHTTPClient(authenticator, cfg)

	// Override timeout for faster test
	client.httpClient.Timeout = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	execution, err := client.InvokeAgent(ctx, agent, map[string]interface{}{}, nil)

	require.Error(t, err)
	assert.Nil(t, execution)
	assert.Contains(t, err.Error(), "request failed")
}

func TestA2AHTTPClient_InvokeAgent_MissingAgentCard(t *testing.T) {
	agent := &models.Agent{
		ID:        "test-agent-1",
		UserID:    "user-1",
		Status:    "active",
		AgentCard: nil,
	}

	encryptionService := &mockEncryptionService{}
	authenticator := NewAgentAuthenticator(encryptionService)
	cfg := &config.Config{} // Use default config for tests
	client := newTestA2AHTTPClient(authenticator, cfg)

	execution, err := client.InvokeAgent(context.Background(), agent, map[string]interface{}{}, nil)

	require.Error(t, err)
	assert.Nil(t, execution)
	assert.Contains(t, err.Error(), "agent card or endpoint URL is missing")
}

func TestA2AHTTPClient_InvokeAgent_A2AMessageFormat(t *testing.T) {
	// Test that the client sends correct A2A v0.3.0 message format
	var capturedRequest JSONRPC2Request

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedRequest))

		response := JSONRPC2Response{
			JSONRPC: "2.0",
			ID:      capturedRequest.ID,
			Result: map[string]interface{}{
				"kind": "task",
				"id":   "task-123",
				"status": map[string]interface{}{
					"state": "completed",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(response))
	}))
	defer server.Close()

	agent := &models.Agent{
		ID:     "test-agent-1",
		UserID: "user-1",
		Status: "active",
		AgentCard: &models.AgentCard{
			SupportedInterfaces: []*a2a.AgentInterface{
				{URL: server.URL, ProtocolBinding: a2a.TransportProtocolHTTPJSON},
			},
		},
	}

	encryptionService := &mockEncryptionService{}
	authenticator := NewAgentAuthenticator(encryptionService)
	cfg := &config.Config{} // Use default config for tests
	client := newTestA2AHTTPClient(authenticator, cfg)

	input := map[string]interface{}{
		"text": "Hello from test",
	}

	_, err := client.InvokeAgent(context.Background(), agent, input, nil)
	require.NoError(t, err)

	// Verify captured request has correct A2A v0.3.0 structure
	assert.Equal(t, "2.0", capturedRequest.JSONRPC)
	assert.Equal(t, "message/send", capturedRequest.Method)
	assert.NotNil(t, capturedRequest.Params)
}

func TestA2AHTTPClient_InvokeAgent_AuthenticationFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should not reach here
		t.Fatal("Should not reach handler when authentication fails")
	}))
	defer server.Close()

	agent := &models.Agent{
		ID:     "test-agent-1",
		UserID: "user-1",
		Status: "active",
		AgentCard: &models.AgentCard{
			SupportedInterfaces: []*a2a.AgentInterface{
				{URL: server.URL, ProtocolBinding: a2a.TransportProtocolHTTPJSON},
			},
			SecurityRequirements: a2a.SecurityRequirementsOptions{
				{"apiKey": {}},
			},
			SecuritySchemes: a2a.NamedSecuritySchemes{
				"apiKey": a2a.APIKeySecurityScheme{
					Name:     "X-API-Key",
					Location: a2a.APIKeySecuritySchemeLocationHeader,
				},
			},
		},
		Credentials: &models.AgentCredentials{
			"apiKey": models.AgentCredential{
				Type:  "apiKey",
				Value: "encrypted-value",
			},
		},
	}

	// Use a mock encryption service that returns an error
	encryptionService := &mockEncryptionServiceWithError{}
	authenticator := NewAgentAuthenticator(encryptionService)
	cfg := &config.Config{} // Use default config for tests
	client := newTestA2AHTTPClient(authenticator, cfg)

	execution, err := client.InvokeAgent(context.Background(), agent, map[string]interface{}{}, nil)

	require.Error(t, err)
	assert.Nil(t, execution)
	assert.Contains(t, err.Error(), "failed to apply authentication")
}

// Mock encryption service
type mockEncryptionService struct{}

func (m *mockEncryptionService) Encrypt(plaintext string) (string, error) {
	return "encrypted:" + plaintext, nil
}

func (m *mockEncryptionService) Decrypt(ciphertext string) (string, error) {
	return "decrypted-value", nil
}

// Mock encryption service that returns error
type mockEncryptionServiceWithError struct{}

func (m *mockEncryptionServiceWithError) Encrypt(plaintext string) (string, error) {
	return "", assert.AnError
}

func (m *mockEncryptionServiceWithError) Decrypt(ciphertext string) (string, error) {
	return "", assert.AnError
}
