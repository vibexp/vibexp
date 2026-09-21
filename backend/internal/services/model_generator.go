package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
// Issue #110 shipped the config + validation slice; #1069 added Complete as the
// first runtime method (a further provider type adds a matching arm in
// NewModelProvider). Nothing consumes it yet — LLMService is registered with Wire
// but has no caller until #1073.
type ModelProvider interface {
	// Model is the model identifier configured for this provider.
	Model() string
	// Type is the provider_type this implementation handles.
	Type() string
	// ListModels reports the models the provider exposes (#1070). Like Validate,
	// an unreachable or non-implementing provider is reported in the body
	// (supported:false) and a non-nil error signals an internal failure only.
	ListModels(ctx context.Context) (*models.ProviderModelList, error)
	// Validate probes the provider for reachability + auth without persisting
	// anything, reporting the outcome in the response body (never an error for a
	// merely-invalid config; a non-nil error signals an internal failure).
	Validate(ctx context.Context) (*models.ValidateModelProviderResponse, error)
	// Complete runs one non-streaming completion. It returns a typed
	// *completionHTTPError for a non-2xx response so a caller can classify the
	// failure without parsing a message, and never echoes the provider's body
	// into the error message of any other failure (#464).
	Complete(ctx context.Context, req models.CompletionRequest) (*models.CompletionResponse, error)
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
// guard is the SSRF policy applied to every request this provider makes; see
// NewOpenAICompatibleProvider for why it is required.
func NewOpenAICompatibleModelProvider(
	baseURL, apiKey, model string, timeout time.Duration, guard *ssrfGuard,
) (*OpenAICompatibleModelProvider, error) {
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("model is required")
	}
	provider, err := NewOpenAICompatibleModelLister(baseURL, apiKey, timeout, guard)
	if err != nil {
		return nil, err
	}
	provider.model = model
	return provider, nil
}

// NewOpenAICompatibleModelLister builds an OpenAICompatibleModelProvider with no
// model configured, for ListModels alone (#1070): listing is how a model is
// chosen, so it has to work before one exists. Every other method needs a model
// and must be reached through NewOpenAICompatibleModelProvider instead.
func NewOpenAICompatibleModelLister(
	baseURL, apiKey string, timeout time.Duration, guard *ssrfGuard,
) (*OpenAICompatibleModelProvider, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("model provider base_url is required")
	}
	if err := validateProviderBaseURLScheme(baseURL); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = validateModelProviderTimeout
	}
	return &OpenAICompatibleModelProvider{
		httpClient: newProviderHTTPClient(guard, timeout),
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     apiKey,
	}, nil
}

func (p *OpenAICompatibleModelProvider) Model() string { return p.model }
func (p *OpenAICompatibleModelProvider) Type() string  { return ProviderTypeOpenAICompatible }

type openAIChatCompletionsRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
	// MaxTokens is omitted when zero: an OpenAI-compatible server reads an
	// explicit max_tokens:0 as "generate nothing" rather than "use your default".
	MaxTokens   int      `json:"max_tokens,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
}

// openAIChatMessage is one message of a chat/completions request or of the choice
// a response carries back.
type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
	// The destination is caller-supplied on purpose; what bounds it is
	// p.httpClient's SSRF-guarded transport, which refuses reserved ranges at
	// dial time. Do not reinstate a #nosec here (#464).
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return 0, fmt.Errorf("failed to create models request: %w", err)
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", authorizationBearerPrefix+p.apiKey)
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
		Messages: []openAIChatMessage{
			{Role: completionRoleUser, Content: modelValidationProbeText},
		},
		MaxTokens: 1,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to marshal chat completions request: %w", err)
	}

	endpoint := p.baseURL + "/chat/completions"
	// See probeModels: the SSRF-guarded transport is the control, not a #nosec.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to create chat completions request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", authorizationBearerPrefix+p.apiKey)
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
	// detail is a fixed category, never the raw error or the URL: the difference
	// between "connection refused", "no such host", and a real HTTP status is
	// exactly the oracle an internal port scan needs (#464). The real error is
	// logged by the caller.
	//
	// A transport error on both probes means the host is unreachable.
	if listErr != nil && chatErr != nil {
		return "Failed to reach the model provider - please check your base URL", providerErrConnectionFailed
	}

	status := chatStatus
	if status == 0 {
		status = listStatus
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "Authentication failed - please check your API key", providerErrUnauthorized
	case http.StatusNotFound:
		return "Model endpoint not found - please check your base URL", providerErrMisconfigured
	default:
		return "Failed to validate model provider", providerErrConnectionFailed
	}
}

// NewModelProvider builds a ModelProvider from a stored provider row. It maps
// provider_type to a concrete implementation; a future provider type is a single
// additional case here plus its implementation. This is the seam a runtime
// consumer resolves against — issue #110 wires nothing to it.
func NewModelProvider(
	provider *models.ModelProvider, apiKey, model string, timeout time.Duration, guard *ssrfGuard,
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
		return NewOpenAICompatibleModelProvider(baseURL, apiKey, model, timeout, guard)
	default:
		return nil, fmt.Errorf("unsupported model provider type: %q", provider.ProviderType)
	}
}

// completionRoleUser is the OpenAI-compatible role name for a caller-authored
// message; it is also what the validation probe sends.
const completionRoleUser = "user"

// authorizationBearerPrefix is hoisted because this file now sets the header at
// three call sites, which is the S1192 duplicate-literal threshold.
const authorizationBearerPrefix = "Bearer "

// maxCompletionErrorBodyBytes caps how much of a non-2xx chat/completions body
// travels in the error — mirrors maxProviderErrorBodyBytes on the embeddings
// path, so an HTML error page cannot flood a log line.
const maxCompletionErrorBodyBytes = 512

// maxCompletionResponseBytes caps a SUCCESSFUL completion body. A model asked for
// a bounded number of tokens cannot legitimately answer with megabytes, and the
// base_url is caller-supplied, so the decode step gets a ceiling too.
const maxCompletionResponseBytes = 1 << 20

// completionHTTPError carries a non-2xx from the chat/completions endpoint,
// including a capped excerpt of the provider's own explanation.
//
// It is a sibling of providerHTTPError rather than a reuse of it: that type's
// Error() names the embeddings endpoint, and a completion failure reported as an
// embeddings failure is a misleading log line in the one place an operator looks.
type completionHTTPError struct {
	StatusCode int
	Body       string
}

func (e *completionHTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("chat completions endpoint returned status %d", e.StatusCode)
	}
	return fmt.Sprintf("chat completions endpoint returned status %d: %s", e.StatusCode, e.Body)
}

// errUnusableCompletionResponse marks a response the provider really DID return
// and that arrived intact, but that cannot be turned into a completion — a 2xx
// carrying HTML (a base_url missing its /v1 suffix hits a proxy's catch-all), or a
// body with no choices. A body that failed to ARRIVE is a transport fault and
// deliberately does not carry this marker.
// The provider answered, so this classifies as a refusal and never as
// unreachability: telling an operator to check the network when the real fault is
// a mistyped base_url sends them to the wrong place.
var errUnusableCompletionResponse = errors.New("provider returned an unusable completion response")

// openAIChatCompletionsChoice is one candidate answer.
type openAIChatCompletionsChoice struct {
	Message      openAIChatMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

// openAIChatCompletionsUsage is the optional token accounting. An
// OpenAI-compatible server may omit it entirely, which leaves both fields zero.
type openAIChatCompletionsUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type openAIChatCompletionsResponse struct {
	Choices []openAIChatCompletionsChoice `json:"choices"`
	Usage   openAIChatCompletionsUsage    `json:"usage"`
}

// Complete runs one non-streaming completion against {base_url}/chat/completions
// over the SSRF-guarded client the constructor built. The provider's model is
// always the one configured on the row — a caller chooses a model by choosing a
// provider, never by overriding it here.
func (p *OpenAICompatibleModelProvider) Complete(
	ctx context.Context, req models.CompletionRequest,
) (*models.CompletionResponse, error) {
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("completion request requires at least one message")
	}

	body, err := json.Marshal(openAIChatCompletionsRequest{
		Model:       p.model,
		Messages:    toOpenAIChatMessages(req.Messages),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chat completions request: %w", err)
	}

	raw, err := p.postChatCompletions(ctx, body)
	if err != nil {
		return nil, err
	}

	return decodeChatCompletion(raw)
}

// toOpenAIChatMessages maps the provider-agnostic request messages onto the wire
// shape. The role is passed through verbatim so a caller can use any role the
// target server understands.
func toOpenAIChatMessages(messages []models.CompletionMessage) []openAIChatMessage {
	out := make([]openAIChatMessage, 0, len(messages))
	for _, m := range messages {
		out = append(out, openAIChatMessage{Role: m.Role, Content: m.Content})
	}
	return out
}

// postChatCompletions POSTs body and returns the response bytes, or a
// *completionHTTPError for any non-2xx status.
func (p *OpenAICompatibleModelProvider) postChatCompletions(
	ctx context.Context, body []byte,
) ([]byte, error) {
	endpoint := p.baseURL + "/chat/completions"
	// See probeModels: the SSRF-guarded transport is the control, not a #nosec (#464).
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completions request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", authorizationBearerPrefix+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call chat completions endpoint: %w", err)
	}
	defer func() {
		_ = resp.Body.Close() //nolint:errcheck
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		// The body is the only place a provider explains itself (a 400 naming the
		// context window is what distinguishes an oversized prompt from any other
		// bad request). Read it through a LimitReader and carry it in the error;
		// LLMService logs it and returns a sentinel, so it never reaches a caller.
		errBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxCompletionErrorBodyBytes))
		if readErr != nil {
			errBody = nil
		}
		return nil, &completionHTTPError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(errBody)),
		}
	}

	// NOT errUnusableCompletionResponse: after 2xx headers this fails only on a real
	// transport fault (a reset or an unexpected EOF mid-body), which must keep the
	// transient ErrProviderUnreachable classification rather than becoming a refusal.
	// Truncation past the LimitReader does not error, so an over-long body still
	// reaches decodeChatCompletion and is correctly "unusable" there.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxCompletionResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read chat completions response: %w", err)
	}
	return raw, nil
}

// decodeChatCompletion flattens the first choice. A response with no choices is an
// error rather than an empty completion: a consumer that renders "" as an answer
// would present a provider fault as a result.
func decodeChatCompletion(raw []byte) (*models.CompletionResponse, error) {
	var decoded openAIChatCompletionsResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("%w: failed to decode chat completions response: %w", errUnusableCompletionResponse, err)
	}
	if len(decoded.Choices) == 0 {
		return nil, fmt.Errorf("%w: it carried no choices", errUnusableCompletionResponse)
	}

	choice := decoded.Choices[0]
	return &models.CompletionResponse{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
		Usage: models.TokenUsage{
			PromptTokens:     decoded.Usage.PromptTokens,
			CompletionTokens: decoded.Usage.CompletionTokens,
		},
	}, nil
}
