package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/models"
)

const (
	// MaxResponseSize limits agent card response size to 1MB to prevent DoS attacks
	MaxResponseSize = 1024 * 1024 // 1MB
	// RequestTimeout defines the maximum time to wait for agent card responses
	RequestTimeout = 30 * time.Second
)

// AgentCardFetcherInterface defines methods for fetching agent cards
type AgentCardFetcherInterface interface {
	FetchAgentCard(ctx context.Context, cardURL string) (*models.AgentCard, error)
}

// newAgentCardHTTPClient builds the HTTP client used to fetch agent cards. The
// transport uses an SSRF-safe dialer (from guard) that rejects connections to
// reserved IP ranges at connect time, defeating DNS rebinding on top of URL host
// validation.
func newAgentCardHTTPClient(guard *ssrfGuard) *http.Client {
	transport := guard.newSSRFSafeTransport(&http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ForceAttemptHTTP2:     true,
	})
	return &http.Client{
		Timeout:   RequestTimeout,
		Transport: transport,
	}
}

// AgentCardFetcher is responsible for fetching agent cards from URLs
type AgentCardFetcher struct {
	httpClient *http.Client
	guard      *ssrfGuard
}

// NewAgentCardFetcher creates a new AgentCardFetcher instance
func NewAgentCardFetcher() *AgentCardFetcher {
	return &AgentCardFetcher{
		httpClient: newAgentCardHTTPClient(defaultSSRFGuard),
		guard:      defaultSSRFGuard,
	}
}

// Close gracefully shuts down the HTTP client connections
// This method should be called when the service is shutting down
func (f *AgentCardFetcher) Close() {
	if transport, ok := f.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}

// validateAgentCardURL validates the agent card URL format, scheme, path, and that
// the host does not resolve to a reserved/internal IP range (SSRF protection).
func (f *AgentCardFetcher) validateAgentCardURL(ctx context.Context, cardURL string) error {
	parsedURL, err := url.Parse(cardURL)
	if err != nil {
		return fmt.Errorf("invalid agent card URL format: not a valid URL")
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: agent card URL must use HTTP or HTTPS protocol")
	}

	if parsedURL.Path != "/.well-known/agent-card.json" {
		return fmt.Errorf("invalid URL path: agent card must be served from " +
			"'/.well-known/agent-card.json' path according to A2A specification")
	}

	if err := f.guard.validateOutboundHost(ctx, cardURL); err != nil {
		return fmt.Errorf("agent card URL is not allowed: %w", err)
	}

	return nil
}

// handleHTTPError converts HTTP status codes to user-friendly error messages
func handleHTTPError(statusCode int) error {
	switch statusCode {
	case http.StatusNotFound:
		return fmt.Errorf("agent card not found: received 404 Not Found")
	case http.StatusUnauthorized:
		return fmt.Errorf("unauthorized: agent card requires authentication")
	case http.StatusForbidden:
		return fmt.Errorf("access forbidden: server refused the request")
	case http.StatusInternalServerError:
		return fmt.Errorf("server error: remote server is experiencing issues")
	case http.StatusBadGateway:
		return fmt.Errorf("bad gateway: proxy or gateway error")
	case http.StatusServiceUnavailable:
		return fmt.Errorf("service unavailable: remote service is temporarily unavailable")
	case http.StatusGatewayTimeout:
		return fmt.Errorf("gateway timeout: remote server response timeout")
	default:
		return fmt.Errorf("HTTP error: received status %d, expected 200 OK", statusCode)
	}
}

// handleRequestError converts network errors to user-friendly error messages
func handleRequestError(err error) error {
	if urlErr, ok := err.(*url.Error); ok {
		if urlErr.Timeout() {
			return fmt.Errorf("request timeout: agent card URL took too long to respond")
		}
		if urlErr.Temporary() {
			return fmt.Errorf("temporary network error: unable to connect to agent card URL")
		}
	}
	return fmt.Errorf("network error: unable to fetch agent card")
}

// readAndValidateResponseBody reads the response body with size limits and decodes JSON
func (f *AgentCardFetcher) readAndValidateResponseBody(resp *http.Response, cardURL string) (*models.AgentCard, error) {
	// Create a limited reader that prevents reading more than MaxResponseSize
	limitedReader := io.LimitReader(resp.Body, MaxResponseSize+1)

	// Try to read with size limit detection
	peekBuffer := make([]byte, MaxResponseSize+1)
	n, err := io.ReadFull(limitedReader, peekBuffer)

	// If we read exactly MaxResponseSize+1 bytes, response is too large
	if n == MaxResponseSize+1 {
		return nil, fmt.Errorf("agent card response too large: maximum allowed size is %d bytes", MaxResponseSize)
	}

	// Handle read errors other than EOF
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Decode JSON from the data we read
	actualData := peekBuffer[:n]
	decoder := json.NewDecoder(strings.NewReader(string(actualData)))

	var agentCard models.AgentCard
	if err := decoder.Decode(&agentCard); err != nil {
		return nil, fmt.Errorf("invalid JSON format: unable to parse agent card response")
	}

	// Validate the agent card
	if err := f.validateAgentCard(&agentCard, cardURL); err != nil {
		return nil, fmt.Errorf("invalid agent card format: %v", err)
	}

	return &agentCard, nil
}

func (f *AgentCardFetcher) FetchAgentCard(ctx context.Context, cardURL string) (*models.AgentCard, error) {
	// Validate URL (scheme, path, and SSRF-safe host)
	if err := f.validateAgentCardURL(ctx, cardURL); err != nil {
		return nil, err
	}

	logrus.WithFields(logrus.Fields{
		"service": "agent-card-fetcher",
		"url":     cardURL,
	}).Info("Fetching agent card")

	// Create and execute HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", cardURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "VibExp-Agent-Discovery/1.0")

	// #nosec G704 - URL is validated via validateAgentCardURL before this call
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, handleRequestError(err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close response body")
		}
	}()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, handleHTTPError(resp.StatusCode)
	}

	// Validate content type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" && contentType != "application/json; charset=utf-8" {
		logrus.WithFields(logrus.Fields{
			"service":      "agent-card-fetcher",
			"url":          cardURL,
			"content_type": contentType,
		}).Warn("Unexpected content type for agent card")
	}

	// Read and validate response body
	agentCard, err := f.readAndValidateResponseBody(resp, cardURL)
	if err != nil {
		return nil, err
	}

	logrus.WithFields(logrus.Fields{
		"service": "agent-card-fetcher",
		"url":     cardURL,
		"name":    agentCard.Name,
		"version": agentCard.Version,
	}).Info("Successfully fetched agent card")

	return agentCard, nil
}

// validateAgentCard performs validation on the fetched agent card based on A2A specification
func validateAgentCardStringField(fieldValue, fieldName string) error {
	if fieldValue == "" {
		return fmt.Errorf("the '%s' field is required in the agent card but was not found or is empty", fieldName)
	}
	return nil
}

func validateAgentCardRequiredFields(card *models.AgentCard) error {
	stringFields := map[string]string{
		"protocolVersion": card.ProtocolVersion,
		"name":            card.Name,
		"description":     card.Description,
		"url":             card.URL,
		"version":         card.Version,
	}

	for fieldName, fieldValue := range stringFields {
		if err := validateAgentCardStringField(fieldValue, fieldName); err != nil {
			return err
		}
	}

	if card.Capabilities == nil {
		return fmt.Errorf("the 'capabilities' field is required in the agent card but was not found")
	}
	if card.DefaultInputModes == nil {
		return fmt.Errorf("the 'defaultInputModes' field is required in the agent card but was not found")
	}
	if card.DefaultOutputModes == nil {
		return fmt.Errorf("the 'defaultOutputModes' field is required in the agent card but was not found")
	}
	if card.Skills == nil {
		return fmt.Errorf("the 'skills' field is required in the agent card but was not found")
	}

	return nil
}

func validateAgentCardSkill(i int, skill models.AgentSkill) error {
	if skill.ID == "" {
		return fmt.Errorf("skill #%d: the 'id' field is required but was not found or is empty", i+1)
	}
	if skill.Name == "" {
		return fmt.Errorf(
			"skill #%d ('%s'): the 'name' field is required but was not found or is empty",
			i+1, skill.ID,
		)
	}
	if skill.Description == "" {
		return fmt.Errorf(
			"skill #%d ('%s'): the 'description' field is required but was not found or is empty",
			i+1, skill.ID,
		)
	}
	if skill.Tags == nil {
		return fmt.Errorf("skill #%d ('%s'): the 'tags' field is required but was not found", i+1, skill.ID)
	}
	return nil
}

func (f *AgentCardFetcher) validateAgentCard(card *models.AgentCard, _cardURL string) error {
	if err := validateAgentCardRequiredFields(card); err != nil {
		return err
	}

	for i, skill := range card.Skills {
		if err := validateAgentCardSkill(i, skill); err != nil {
			return err
		}
	}

	return nil
}
