package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
)

const (
	// DefaultA2ATimeout is the default timeout for A2A HTTP requests (5 minutes)
	DefaultA2ATimeout = 5 * time.Minute
	// MaxA2AResponseSize is the maximum size of A2A response (10MB)
	MaxA2AResponseSize = 10 * 1024 * 1024
)

// JSONRPC2Request represents a JSON-RPC 2.0 request
type JSONRPC2Request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// JSONRPC2Response represents a JSON-RPC 2.0 response
type JSONRPC2Response struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      string                 `json:"id"`
	Result  map[string]interface{} `json:"result,omitempty"`
	Error   *JSONRPC2Error         `json:"error,omitempty"`
}

// JSONRPC2Error represents a JSON-RPC 2.0 error
type JSONRPC2Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// A2AStreamEvent represents a streaming event from an A2A agent
type A2AStreamEvent struct {
	Type      string                 // "task", "status-update", "artifact-update"
	Data      map[string]interface{} // Full event data
	Timestamp time.Time
}

// A2AHTTPClientInterface defines the interface for A2A HTTP communication
type A2AHTTPClientInterface interface {
	InvokeAgent(
		ctx context.Context, agent *models.Agent, input map[string]interface{}, contextID *string,
	) (*models.AgentExecution, error)
	InvokeAgentStreaming(
		ctx context.Context, agent *models.Agent, input map[string]interface{},
		contextID *string, eventChan chan<- *A2AStreamEvent,
	) error
	SupportsStreaming(agent *models.Agent) bool
}

// A2AHTTPClient handles A2A protocol communication over HTTP
type A2AHTTPClient struct {
	httpClient    *http.Client
	authenticator *AgentAuthenticator
	timeout       time.Duration
	guard         *ssrfGuard
}

// NewA2AHTTPClient creates a new A2A HTTP client
func NewA2AHTTPClient(authenticator *AgentAuthenticator, cfg *config.Config) *A2AHTTPClient {
	timeout := DefaultA2ATimeout

	// Use timeout from config if set (defaults to 5m)
	if cfg != nil && cfg.A2A.DefaultTimeout > 0 {
		timeout = cfg.A2A.DefaultTimeout
	}

	// The transport uses an SSRF-safe dialer that rejects connections to reserved IP
	// ranges at connect time, defeating DNS rebinding on top of endpoint validation.
	transport := defaultSSRFGuard.newSSRFSafeTransport(&http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	})

	return &A2AHTTPClient{
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		authenticator: authenticator,
		timeout:       timeout,
		guard:         defaultSSRFGuard,
	}
}

// InvokeAgent sends a task to an A2A agent and returns the result
func buildA2AMessage(requestID string, input map[string]interface{}, contextID *string) map[string]interface{} {
	message := map[string]interface{}{
		"kind":      "message",
		"role":      "user",
		"messageId": requestID,
		"parts":     []map[string]interface{}{},
		"metadata":  map[string]interface{}{},
	}

	if contextID != nil && *contextID != "" {
		message["contextId"] = *contextID
	}

	if text, ok := input["text"].(string); ok {
		message["parts"] = []map[string]interface{}{
			{"kind": "text", "text": text},
		}
	} else {
		message["parts"] = []map[string]interface{}{
			{"kind": "text", "text": fmt.Sprintf("%v", input)},
		}
	}

	return message
}

func determineA2AEndpoint(agent *models.Agent) (string, error) {
	switch {
	case agent.AgentCard != nil &&
		agent.AgentCard.AdditionalInterfaces != nil &&
		agent.AgentCard.AdditionalInterfaces.HTTP != nil:
		return agent.AgentCard.AdditionalInterfaces.HTTP.URL, nil
	case agent.AgentCard != nil:
		return agent.AgentCard.URL, nil
	default:
		return "", fmt.Errorf("agent card or URL is missing")
	}
}

func (c *A2AHTTPClient) createA2AHTTPRequest(
	ctx context.Context, agent *models.Agent, body []byte,
) (*http.Request, error) {
	endpoint, err := determineA2AEndpoint(agent)
	if err != nil {
		return nil, err
	}

	if err = c.guard.validateOutboundHost(ctx, endpoint); err != nil {
		return nil, fmt.Errorf("agent endpoint is not allowed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return req, nil
}

func executeA2AHTTPRequest(httpClient *http.Client, req *http.Request) ([]byte, time.Duration, error) {
	startTime := time.Now()
	// #nosec G704 - Request is constructed by caller with agent endpoint URL from configuration
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log the error but don't fail the operation since we already got the response
			fmt.Printf("Warning: failed to close response body: %v\n", closeErr)
		}
	}()

	duration := time.Since(startTime)

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxA2AResponseSize))
	if err != nil {
		return nil, duration, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, duration, fmt.Errorf("agent returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, duration, nil
}

func processA2AResponse(respBody []byte, duration time.Duration) (*models.AgentExecution, error) {
	var rpcResponse JSONRPC2Response
	if err := json.Unmarshal(respBody, &rpcResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if rpcResponse.Error != nil {
		errorMsg := rpcResponse.Error.Message
		return &models.AgentExecution{
			Status:   "error",
			Error:    &errorMsg,
			Duration: intPtr(int(duration.Milliseconds())),
		}, nil
	}

	execution := &models.AgentExecution{
		Status:   "completed",
		Duration: intPtr(int(duration.Milliseconds())),
	}

	return execution, nil
}

func (c *A2AHTTPClient) InvokeAgent(
	ctx context.Context,
	agent *models.Agent,
	input map[string]interface{},
	contextID *string,
) (*models.AgentExecution, error) {
	requestID := fmt.Sprintf("msg-%d", time.Now().UnixNano())
	message := buildA2AMessage(requestID, input, contextID)

	rpcRequest := JSONRPC2Request{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  "message/send",
		Params: map[string]interface{}{
			"message": message,
			"configuration": map[string]interface{}{
				"acceptedOutputModes": []string{"text/plain"},
			},
		},
	}

	body, err := json.Marshal(rpcRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := c.createA2AHTTPRequest(ctx, agent, body)
	if err != nil {
		return nil, err
	}

	if authErr := c.authenticator.ApplyAuthentication(req, agent); authErr != nil {
		return nil, fmt.Errorf("failed to apply authentication: %w", authErr)
	}

	respBody, duration, err := executeA2AHTTPRequest(c.httpClient, req)
	if err != nil {
		return nil, err
	}

	return processA2AResponse(respBody, duration)
}

// intPtr returns a pointer to an int
func intPtr(i int) *int {
	return &i
}

// SupportsStreaming checks if the agent supports streaming based on agent card capabilities
func (c *A2AHTTPClient) SupportsStreaming(agent *models.Agent) bool {
	if agent.AgentCard == nil || agent.AgentCard.Capabilities == nil {
		return false
	}
	return agent.AgentCard.Capabilities.Streaming
}

func buildA2AStreamMessage(requestID string, input map[string]interface{}, contextID *string) map[string]interface{} {
	message := map[string]interface{}{
		"kind":      "message",
		"role":      "user",
		"messageId": requestID,
		"parts":     []map[string]interface{}{},
		"metadata":  map[string]interface{}{},
	}

	if contextID != nil && *contextID != "" {
		message["contextId"] = *contextID
	}

	if text, ok := input["text"].(string); ok {
		message["parts"] = []map[string]interface{}{
			{
				"kind": "text",
				"text": text,
			},
		}
	} else {
		message["parts"] = []map[string]interface{}{
			{
				"kind": "text",
				"text": fmt.Sprintf("%v", input),
			},
		}
	}

	return message
}

func (c *A2AHTTPClient) createStreamingRequest(
	ctx context.Context, agent *models.Agent, body []byte,
) (*http.Request, error) {
	endpoint, err := determineA2AEndpoint(agent)
	if err != nil {
		return nil, err
	}

	if err = c.guard.validateOutboundHost(ctx, endpoint); err != nil {
		return nil, fmt.Errorf("agent endpoint is not allowed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	return req, nil
}

func validateStreamingResponse(resp *http.Response) error {
	if resp.StatusCode != http.StatusOK {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, MaxA2AResponseSize))
		if readErr != nil {
			return fmt.Errorf("agent returned status %d, failed to read error response: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("agent returned status %d: %s", resp.StatusCode, string(respBody))
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		return fmt.Errorf("expected text/event-stream, got %s", contentType)
	}

	return nil
}

// InvokeAgentStreaming sends a task to an A2A agent with streaming support
func (c *A2AHTTPClient) InvokeAgentStreaming(
	ctx context.Context,
	agent *models.Agent,
	input map[string]interface{},
	contextID *string,
	eventChan chan<- *A2AStreamEvent,
) error {
	defer close(eventChan)

	requestID := fmt.Sprintf("msg-%d", time.Now().UnixNano())
	message := buildA2AStreamMessage(requestID, input, contextID)

	rpcRequest := JSONRPC2Request{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  "message/stream",
		Params: map[string]interface{}{
			"message": message,
			"configuration": map[string]interface{}{
				"acceptedOutputModes": []string{"text/plain"},
			},
		},
	}

	body, err := json.Marshal(rpcRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := c.createStreamingRequest(ctx, agent, body)
	if err != nil {
		return err
	}

	if authErr := c.authenticator.ApplyAuthentication(req, agent); authErr != nil {
		return fmt.Errorf("failed to apply authentication: %w", authErr)
	}

	// #nosec G704 - URL is from agent configuration endpoint, admin-controlled
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log the error but don't fail the operation since we already got the response
			fmt.Printf("Warning: failed to close response body: %v\n", closeErr)
		}
	}()

	if err := validateStreamingResponse(resp); err != nil {
		return err
	}

	return c.parseSSEStream(ctx, resp.Body, eventChan)
}

// parseSSEStream parses Server-Sent Events from the response stream
func (c *A2AHTTPClient) parseSSEStream(
	ctx context.Context,
	reader io.Reader,
	eventChan chan<- *A2AStreamEvent,
) error {
	scanner := bufio.NewScanner(reader)
	var eventData strings.Builder

	for scanner.Scan() {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()

		// Empty line signals end of event
		if line == "" {
			if eventData.Len() > 0 {
				// Process the accumulated event data
				if err := c.processSSEEvent(eventData.String(), eventChan); err != nil {
					// Log error but continue processing (don't fail entire stream)
					// The error is already logged in processSSEEvent
					eventData.Reset()
					continue
				}
				eventData.Reset()
			}
			continue
		}

		// Parse SSE line format: "field: value"
		if strings.HasPrefix(line, "data: ") {
			eventData.WriteString(strings.TrimPrefix(line, "data: "))
		}
		// Ignore other SSE fields (event:, id:, retry:) for now
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream reading error: %w", err)
	}

	return nil
}

// processSSEEvent processes a single SSE event
func (c *A2AHTTPClient) processSSEEvent(data string, eventChan chan<- *A2AStreamEvent) error {
	// Parse JSON-RPC response from data
	var rpcResponse JSONRPC2Response
	if err := json.Unmarshal([]byte(data), &rpcResponse); err != nil {
		// Skip malformed events
		return fmt.Errorf("failed to parse SSE data as JSON-RPC: %w", err)
	}

	// Check for JSON-RPC error
	if rpcResponse.Error != nil {
		return fmt.Errorf("JSON-RPC error: %s", rpcResponse.Error.Message)
	}

	// Extract event from result
	if rpcResponse.Result == nil {
		return fmt.Errorf("JSON-RPC response missing result")
	}

	// Determine event type from "kind" field
	eventType := "unknown"
	if kind, ok := rpcResponse.Result["kind"].(string); ok {
		eventType = kind
	}

	// Create and send stream event
	event := &A2AStreamEvent{
		Type:      eventType,
		Data:      rpcResponse.Result,
		Timestamp: time.Now(),
	}

	eventChan <- event
	return nil
}
