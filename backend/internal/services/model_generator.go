package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/models"
)

// validateModelProviderTimeout bounds a single outbound validation probe.
const validateModelProviderTimeout = 30 * time.Second

// modelValidationProbeText is the sample prompt sent to a provider during the
// chat/completions fallback probe. It is short and neutral; only whether the
// endpoint answers (reachability + auth) matters, not the content.
const modelValidationProbeText = "ping"

// ModelProvider is the pluggable seam for a chat/completion-style model backend.
// Issue #110 ships only the config + validation slice, so the interface is
// intentionally minimal: a future runtime consumer adds methods here plus a
// matching arm in NewModelProvider. Nothing is wired to it yet.
type ModelProvider interface {
	// Model is the model identifier configured for this provider.
	Model() string
	// Type is the provider_type this implementation handles.
	Type() string
	// Validate probes the provider for reachability + auth without persisting
	// anything, reporting the outcome in the response body (never an error for a
	// merely-invalid config; a non-nil error signals an internal failure).
	Validate(ctx context.Context) (*models.ValidateModelProviderResponse, error)
}

// OpenAICompatibleModelProvider talks to an OpenAI-compatible API root (e.g.
// "https://api.openai.com/v1", "http://localhost:11434/v1" for Ollama) with a
// bearer API key.
type OpenAICompatibleModelProvider struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
}

// Ensure OpenAICompatibleModelProvider implements ModelProvider.
var _ ModelProvider = (*OpenAICompatibleModelProvider)(nil)

// NewOpenAICompatibleModelProvider builds an OpenAICompatibleModelProvider.
// baseURL and model must be non-empty; apiKey may be empty for endpoints that do
// not require auth (e.g. a local Ollama).
func NewOpenAICompatibleModelProvider(
	baseURL, apiKey, model string, timeout time.Duration,
) (*OpenAICompatibleModelProvider, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("model provider base_url is required")
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("model is required")
	}
	if timeout <= 0 {
		timeout = validateModelProviderTimeout
	}
	return &OpenAICompatibleModelProvider{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
	}, nil
}

func (p *OpenAICompatibleModelProvider) Model() string { return p.model }
func (p *OpenAICompatibleModelProvider) Type() string  { return ProviderTypeOpenAICompatible }

type openAIChatCompletionsRequest struct {
	Model     string                   `json:"model"`
	Messages  []map[string]interface{} `json:"messages"`
	MaxTokens int                      `json:"max_tokens"`
}

// Validate confirms reachability + auth. It first tries the cheap
// GET {base_url}/models listing; if that is not answered with 2xx it falls back
// to POST {base_url}/chat/completions with max_tokens:1 (some gateways expose
// only the completions route). The provider is accepted when either returns 2xx.
// A merely-invalid configuration is reported via the response body, not an error.
func (p *OpenAICompatibleModelProvider) Validate(
	ctx context.Context,
) (*models.ValidateModelProviderResponse, error) {
	response := &models.ValidateModelProviderResponse{
		IsValid: false,
		Message: "Validation failed",
	}

	start := time.Now()
	status, listErr := p.probeModels(ctx)
	if listErr == nil && status >= 200 && status < 300 {
		response.Details.ResponseTime = int(time.Since(start).Milliseconds())
		response.Details.StatusCode = status
		response.IsValid = true
		response.Message = "Model provider validation successful"
		return response, nil
	}

	// Fall back to the chat/completions route.
	chatStatus, chatErr := p.probeChatCompletions(ctx)
	response.Details.ResponseTime = int(time.Since(start).Milliseconds())
	if chatErr == nil && chatStatus >= 200 && chatStatus < 300 {
		response.Details.StatusCode = chatStatus
		response.IsValid = true
		response.Message = "Model provider validation successful"
		return response, nil
	}

	// Neither probe succeeded: surface the most informative failure. A transport
	// error (unreachable host) has no status; an auth/endpoint error has one.
	response.Details.StatusCode = chatStatus
	if chatStatus == 0 {
		response.Details.StatusCode = status
	}
	response.Message, response.Details.ErrorDetails = describeModelValidationFailure(
		status, listErr, chatStatus, chatErr,
	)
	return response, nil
}

// probeModels issues GET {base_url}/models and returns the HTTP status. A
// transport error returns status 0 plus the error. Request build + Do live in
// this one function so gosec's SSRF/bodyclose analysers stay satisfied.
func (p *OpenAICompatibleModelProvider) probeModels(ctx context.Context) (int, error) {
	endpoint := p.baseURL + "/models"
	// #nosec G107 -- base_url is a caller-supplied bring-your-own provider endpoint that
	// VibeXP intentionally probes; connecting to it is the whole point of validation.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return 0, fmt.Errorf("failed to create models request: %w", err)
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to call models endpoint: %w", err)
	}
	defer func() {
		_ = resp.Body.Close() //nolint:errcheck
	}()

	return resp.StatusCode, nil
}

// probeChatCompletions issues POST {base_url}/chat/completions with max_tokens:1
// and returns the HTTP status. A transport error returns status 0 plus the error.
func (p *OpenAICompatibleModelProvider) probeChatCompletions(ctx context.Context) (int, error) {
	body, err := json.Marshal(openAIChatCompletionsRequest{
		Model: p.model,
		Messages: []map[string]interface{}{
			{"role": "user", "content": modelValidationProbeText},
		},
		MaxTokens: 1,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to marshal chat completions request: %w", err)
	}

	endpoint := p.baseURL + "/chat/completions"
	// #nosec G107 -- base_url is a caller-supplied bring-your-own provider endpoint that
	// VibeXP intentionally probes; connecting to it is the whole point of validation.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to create chat completions request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to call chat completions endpoint: %w", err)
	}
	defer func() {
		_ = resp.Body.Close() //nolint:errcheck
	}()

	return resp.StatusCode, nil
}

// describeModelValidationFailure maps a failed probe pair to a concise,
// user-facing message plus the underlying error detail. It prefers the
// chat/completions outcome (the last thing tried) but reports a transport error
// when the host was simply unreachable.
func describeModelValidationFailure(
	listStatus int, listErr error, chatStatus int, chatErr error,
) (message, detail string) {
	// A transport error on both probes means the host is unreachable.
	if listErr != nil && chatErr != nil {
		return "Failed to reach the model provider - please check your base URL", chatErr.Error()
	}

	status := chatStatus
	if status == 0 {
		status = listStatus
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "Authentication failed - please check your API key", fmt.Sprintf("provider returned status %d", status)
	case http.StatusNotFound:
		return "Model endpoint not found - please check your base URL", fmt.Sprintf("provider returned status %d", status)
	default:
		return "Failed to validate model provider", fmt.Sprintf("provider returned status %d", status)
	}
}

// NewModelProvider builds a ModelProvider from a stored provider row. It maps
// provider_type to a concrete implementation; a future provider type is a single
// additional case here plus its implementation. This is the seam a runtime
// consumer resolves against — issue #110 wires nothing to it.
func NewModelProvider(
	provider *models.ModelProvider, apiKey, model string, timeout time.Duration,
) (ModelProvider, error) {
	if provider == nil {
		return nil, fmt.Errorf("model provider is nil")
	}

	switch provider.ProviderType {
	case ProviderTypeOpenAICompatible:
		baseURL := ""
		if provider.BaseURL != nil {
			baseURL = *provider.BaseURL
		}
		return NewOpenAICompatibleModelProvider(baseURL, apiKey, model, timeout)
	default:
		return nil, fmt.Errorf("unsupported model provider type: %q", provider.ProviderType)
	}
}
