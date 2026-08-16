package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/models"
)

// ProviderTypeOpenAICompatible is the provider_type value for any endpoint that
// speaks the OpenAI /v1/embeddings protocol (OpenAI, Ollama, LocalAI, vLLM, TEI, …).
// It matches the type accepted by EmbeddingProviderService.ValidateEmbeddingProvider.
const ProviderTypeOpenAICompatible = "openai_compatible"

// EmbeddingVectorDimensions is the fixed embedding width VibeXP stores. It is a
// constant, not configuration: it is locked to the `vector(N)` column set by the
// image-baked migration, so an operator cannot change it (changing it is a VibeXP
// release that ships a new migration). Operators instead pick an embedding model
// that outputs this width.
//
// 1024 is the broadest OpenAI-compatible dimension: native for Bedrock Titan V2,
// Cohere v3, Mistral, mxbai-embed-large and bge-large, and requested from the
// Matryoshka models (OpenAI text-embedding-3-*, Gemini gemini-embedding-001) via
// the request's `dimensions` field. It MUST match the vector(N) column width in
// the latest embeddings migration.
const EmbeddingVectorDimensions = 1024

// generateEmbeddingsTimeout bounds a single outbound embeddings call.
const generateEmbeddingsTimeout = 30 * time.Second

// maxEmbeddingBatchSize caps how many chunks go into one /embeddings request.
// It matches text-embeddings-inference's default --max-client-batch-size, the
// most restrictive limit among the common OpenAI-compatible backends, so the
// default works against a stock TEI sidecar. Without it a long entity posts every
// chunk at once — a 58k-char memory at chunk_size 1000 is ~73 inputs — and TEI
// rejects the whole request with 422, making that entity permanently
// un-embeddable (#756).
const maxEmbeddingBatchSize = 32

// maxProviderErrorBodyBytes caps how much of a non-200 response body is carried
// in the error. The body is what makes a provider rejection diagnosable, but it
// can be an HTML error page, so it is truncated before it reaches a log line.
const maxProviderErrorBodyBytes = 512

// providerHTTPError is a non-200 response from an embeddings endpoint. It carries
// the status and the (truncated) body so the failure is diagnosable from one log
// line, and classifies itself so the caller can tell "retrying will help" from
// "this request will be rejected identically forever".
type providerHTTPError struct {
	StatusCode int
	Body       string
}

func (e *providerHTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("embeddings endpoint returned status %d", e.StatusCode)
	}
	return fmt.Sprintf("embeddings endpoint returned status %d: %s", e.StatusCode, e.Body)
}

// Permanent reports whether retrying this request is pointless. Every 4xx says
// the request itself is unacceptable, so the provider will reject an identical
// retry — except 408 (Request Timeout) and 429 (Too Many Requests), which are the
// two 4xx that genuinely describe a transient condition. 5xx stays retryable.
func (e *providerHTTPError) Permanent() bool {
	if e.StatusCode == http.StatusRequestTimeout || e.StatusCode == http.StatusTooManyRequests {
		return false
	}
	return e.StatusCode >= 400 && e.StatusCode < 500
}

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

// ResolvedEmbeddingProvider is a team's active provider ready to embed: the
// generation Provider (whose Model()/Dimensions() come from the stored row) plus
// the in-Go chunker sizing and per-provider concurrency limit from that same row.
// It is the resolver's unit so the document path can chunk per-provider and the
// processing path can bound fan-out (#142), without the generation seam having to
// know about chunking. Concurrency is plumbed here but not yet enforced (#144).
type ResolvedEmbeddingProvider struct {
	Provider     EmbeddingProvider
	ChunkSize    int
	ChunkOverlap int
	Concurrency  int
	// QueryPrefix / DocumentPrefix are the stored provider instruction prefixes,
	// applied only to the text sent to the provider: QueryPrefix is prepended to
	// each search query, DocumentPrefix to each document chunk. Empty means no
	// prefix (the default for symmetric models).
	QueryPrefix    string
	DocumentPrefix string
}

// ActiveEmbeddingProviderResolver resolves the embedding provider used to embed a
// given team's resources. A (nil, nil) result means the team has no provider
// configured — embedding is disabled, not failed — so callers no-op rather than
// erroring. It is the narrow seam the embedding worker and query embedder depend
// on (satisfied by *EmbeddingProviderService).
type ActiveEmbeddingProviderResolver interface {
	ResolveActiveProvider(ctx context.Context, teamID string) (*ResolvedEmbeddingProvider, error)
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
//
// guard is the SSRF policy applied to every request this provider makes. It is
// required rather than optional because base_url is caller-supplied: pass
// ssrfGuardForConfig(cfg) in production; a nil guard degrades to the fail-closed
// production policy (see newProviderHTTPClient).
func NewOpenAICompatibleProvider(
	baseURL, apiKey, model string, dimensions int, timeout time.Duration, guard *ssrfGuard,
) (*OpenAICompatibleProvider, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("embedding provider base_url is required")
	}
	if err := validateProviderBaseURLScheme(baseURL); err != nil {
		return nil, err
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
		httpClient: newProviderHTTPClient(guard, timeout),
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
	// Dimensions pins the output width for Matryoshka models (OpenAI
	// text-embedding-3-*, Gemini gemini-embedding-001, Bedrock Titan V2) whose
	// native default differs from the configured width. Fixed-dimension endpoints
	// ignore it and return their native width, which must equal Dimensions or the
	// response is rejected.
	Dimensions int `json:"dimensions,omitempty"`
}

type openAIEmbeddingsResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// GenerateEmbeddings embeds texts via the OpenAI-compatible endpoint and returns
// the vectors in input order, validating count and per-vector dimensionality.
//
// The request is split into batches of at most maxEmbeddingBatchSize, run
// sequentially and concatenated, so an entity of any length embeds regardless of
// the backend's per-request input limit (#756). Batching is an implementation
// detail: the interface contract — one vector per input, in input order — is
// unchanged, and callers still pass the entity's whole chunk list. Sequential
// rather than concurrent on purpose: a single-threaded local TEI is the common
// deployment, and per-provider fan-out is already bounded one layer up by the
// dispatcher.
func (p *OpenAICompatibleProvider) GenerateEmbeddings(
	ctx context.Context, texts []string,
) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	vectors := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += maxEmbeddingBatchSize {
		end := min(start+maxEmbeddingBatchSize, len(texts))

		batch, err := p.generateBatch(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, batch...)
	}

	return vectors, nil
}

// generateBatch embeds one batch in a single request. The caller guarantees the
// batch is non-empty and within maxEmbeddingBatchSize; count and per-vector width
// are validated per batch, so a bad response fails the whole entity rather than
// silently contributing short output.
func (p *OpenAICompatibleProvider) generateBatch(
	ctx context.Context, texts []string,
) ([][]float32, error) {
	body, err := json.Marshal(openAIEmbeddingsRequest{
		Input:          texts,
		Model:          p.model,
		EncodingFormat: "float",
		Dimensions:     p.dimensions,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embeddings request: %w", err)
	}

	endpoint := p.baseURL + "/embeddings"
	// The destination IS caller-supplied; what makes it safe is p.httpClient's
	// SSRF-guarded transport (built in NewOpenAICompatibleProvider), which
	// refuses reserved ranges at dial time. Do not reinstate a #nosec here — it
	// previously claimed "not user input", which was false (#464).
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
		// The body is the only place a provider explains itself (TEI's 422 names the
		// batch limit it hit). Read it through a LimitReader so an HTML error page
		// cannot flood the log, and carry it in the error rather than logging here —
		// the dispatcher logs it once, with the entity's identifiers attached.
		errBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxProviderErrorBodyBytes))
		if readErr != nil {
			errBody = nil
		}
		return nil, &providerHTTPError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(errBody)),
		}
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
// single additional case here plus their implementation. model is the provider's
// and dimensions is the fixed EmbeddingVectorDimensions constant, so document and
// query embeddings always share one model and one vector width.
func NewGenerationProvider(
	provider *models.EmbeddingProvider, apiKey, model string, dimensions int,
	timeout time.Duration, guard *ssrfGuard,
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
		return NewOpenAICompatibleProvider(baseURL, apiKey, model, dimensions, timeout, guard)
	default:
		return nil, fmt.Errorf("unsupported embedding provider type: %q", provider.ProviderType)
	}
}
