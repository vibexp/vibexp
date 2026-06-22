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

// ProviderTypeOpenAICompatible is the provider_type value for any endpoint that
// speaks the OpenAI /v1/embeddings protocol (OpenAI, Ollama, LocalAI, vLLM, TEI, …).
// It matches the type accepted by EmbeddingProviderService.ValidateEmbeddingProvider.
const ProviderTypeOpenAICompatible = "openai_compatible"

// generateEmbeddingsTimeout bounds a single outbound embeddings call.
const generateEmbeddingsTimeout = 30 * time.Second

// EmbeddingProvider generates embedding vectors for text. It is the pluggable
// seam that lets VibeXP target any embedding backend: adding a new backend is one
// implementation of this interface plus one arm in NewGenerationProvider.
type EmbeddingProvider interface {
	// GenerateEmbeddings returns one vector per input text, in input order. The
	// returned slice has the same length as texts; each vector has Dimensions()
	// elements. An empty input yields a nil slice and no error.
	GenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error)
	// Model is the model identifier used for the embedding request.
	Model() string
	// Dimensions is the expected length of every returned vector.
	Dimensions() int
	// Type is the provider_type this implementation handles.
	Type() string
}

// ActiveEmbeddingProviderResolver resolves the single system-wide embedding
// provider used by the embedding pipeline. A (nil, nil) result means no provider
// is configured — embedding is disabled, not failed — so callers no-op rather
// than erroring. It is the narrow seam the embedding worker and query embedder
// depend on (satisfied by *EmbeddingProviderService).
type ActiveEmbeddingProviderResolver interface {
	ResolveActiveProvider(ctx context.Context, model string, dimensions int) (EmbeddingProvider, error)
}

// OpenAICompatibleProvider calls an OpenAI-compatible POST {base_url}/embeddings
// endpoint with a bearer API key. base_url is the API root (e.g.
// "https://api.openai.com/v1", "http://localhost:11434/v1" for Ollama).
type OpenAICompatibleProvider struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
	dimensions int
}

// Ensure OpenAICompatibleProvider implements EmbeddingProvider.
var _ EmbeddingProvider = (*OpenAICompatibleProvider)(nil)

// NewOpenAICompatibleProvider builds an OpenAICompatibleProvider. baseURL and
// model must be non-empty and dimensions must be positive; apiKey may be empty
// for endpoints that do not require auth (e.g. a local Ollama).
func NewOpenAICompatibleProvider(
	baseURL, apiKey, model string, dimensions int, timeout time.Duration,
) (*OpenAICompatibleProvider, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("embedding provider base_url is required")
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("embedding model is required")
	}
	if dimensions < 1 {
		return nil, fmt.Errorf("embedding dimensions must be >= 1, got %d", dimensions)
	}
	if timeout <= 0 {
		timeout = generateEmbeddingsTimeout
	}
	return &OpenAICompatibleProvider{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		dimensions: dimensions,
	}, nil
}

func (p *OpenAICompatibleProvider) Model() string   { return p.model }
func (p *OpenAICompatibleProvider) Dimensions() int { return p.dimensions }
func (p *OpenAICompatibleProvider) Type() string    { return ProviderTypeOpenAICompatible }

type openAIEmbeddingsRequest struct {
	Input          []string `json:"input"`
	Model          string   `json:"model"`
	EncodingFormat string   `json:"encoding_format"`
}

type openAIEmbeddingsResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// GenerateEmbeddings embeds texts via the OpenAI-compatible endpoint and returns
// the vectors in input order, validating count and per-vector dimensionality.
func (p *OpenAICompatibleProvider) GenerateEmbeddings(
	ctx context.Context, texts []string,
) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(openAIEmbeddingsRequest{
		Input:          texts,
		Model:          p.model,
		EncodingFormat: "float",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embeddings request: %w", err)
	}

	endpoint := p.baseURL + "/embeddings"
	// #nosec G107 -- endpoint is built from admin-configured provider base_url, not user input
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create embeddings request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call embeddings endpoint: %w", err)
	}
	defer func() {
		_ = resp.Body.Close() //nolint:errcheck
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embeddings endpoint returned status %d", resp.StatusCode)
	}

	var decoded openAIEmbeddingsResponse
	if err = json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("failed to decode embeddings response: %w", err)
	}

	return p.orderVectors(decoded, len(texts))
}

// orderVectors validates the decoded response and returns the vectors in input
// order, keyed by each datum's index, validating count and per-vector width.
func (p *OpenAICompatibleProvider) orderVectors(decoded openAIEmbeddingsResponse, want int) ([][]float32, error) {
	if len(decoded.Data) != want {
		return nil, fmt.Errorf("embeddings endpoint returned %d vectors, expected %d", len(decoded.Data), want)
	}

	vectors := make([][]float32, want)
	for _, d := range decoded.Data {
		if d.Index < 0 || d.Index >= want {
			return nil, fmt.Errorf("embeddings endpoint returned out-of-range index %d", d.Index)
		}
		if len(d.Embedding) != p.dimensions {
			return nil, fmt.Errorf(
				"embeddings endpoint returned vector of length %d, expected %d",
				len(d.Embedding), p.dimensions,
			)
		}
		if vectors[d.Index] != nil {
			return nil, fmt.Errorf("embeddings endpoint returned duplicate index %d", d.Index)
		}
		vectors[d.Index] = d.Embedding
	}
	for i, v := range vectors {
		if v == nil {
			return nil, fmt.Errorf("embeddings endpoint did not return a vector for index %d", i)
		}
	}

	return vectors, nil
}

// NewGenerationProvider builds an EmbeddingProvider from a stored provider row.
// It maps provider_type to a concrete implementation; future provider types are a
// single additional case here plus their implementation. model and dimensions are
// the deployment-wide values (EMBEDDING_MODEL / EMBEDDING_DIMENSIONS) so document
// and query embeddings always share one model and one vector width.
func NewGenerationProvider(
	provider *models.EmbeddingProvider, apiKey, model string, dimensions int, timeout time.Duration,
) (EmbeddingProvider, error) {
	if provider == nil {
		return nil, fmt.Errorf("embedding provider is nil")
	}

	switch provider.ProviderType {
	case ProviderTypeOpenAICompatible:
		baseURL := ""
		if provider.BaseURL != nil {
			baseURL = *provider.BaseURL
		}
		return NewOpenAICompatibleProvider(baseURL, apiKey, model, dimensions, timeout)
	default:
		return nil, fmt.Errorf("unsupported embedding provider type: %q", provider.ProviderType)
	}
}
