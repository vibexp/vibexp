package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/aiclient"
)

// queryTaskType marks query-side embeddings so Gemini applies its asymmetric
// query↔document retrieval objective (ai-service embeds documents with
// RETRIEVAL_DOCUMENT). Local models ignore the field.
const queryTaskType = "RETRIEVAL_QUERY"

// queryEmbedTimeout bounds the outbound call to the AI service.
const queryEmbedTimeout = 10 * time.Second

// QueryEmbedder converts a free-text query into an embedding vector via the AI service.
type QueryEmbedder interface {
	// EmbedQuery returns the embedding vector for query. The returned slice always
	// has the configured embedding dimensionality on success.
	EmbedQuery(ctx context.Context, query string) ([]float32, error)
}

// AIQueryEmbedder calls the AI service's OpenAI-compatible /v1/embeddings endpoint.
type AIQueryEmbedder struct {
	client       *http.Client
	aiServiceURL string
	model        string
	dimensions   int
	logger       *logrus.Logger
}

// AIQueryEmbedderConfig configures an AIQueryEmbedder. Model and Dimensions come
// from the typed Config and must match the model ai-service embeds documents with.
type AIQueryEmbedderConfig struct {
	AIServiceURL string
	Model        string
	Dimensions   int
	Logger       *logrus.Logger
}

// NewAIQueryEmbedder creates a new AIQueryEmbedder.
func NewAIQueryEmbedder(cfg AIQueryEmbedderConfig) *AIQueryEmbedder {
	return &AIQueryEmbedder{
		client:       aiclient.New(context.Background(), cfg.AIServiceURL, queryEmbedTimeout, cfg.Logger),
		aiServiceURL: cfg.AIServiceURL,
		model:        cfg.Model,
		dimensions:   cfg.Dimensions,
		logger:       cfg.Logger,
	}
}

type embeddingsRequest struct {
	Input          string `json:"input"`
	Model          string `json:"model"`
	EncodingFormat string `json:"encoding_format"`
	TaskType       string `json:"task_type,omitempty"`
}

type embeddingsResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// EmbedQuery embeds query via the AI service and validates the returned vector.
func (e *AIQueryEmbedder) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	if e.aiServiceURL == "" {
		return nil, fmt.Errorf("AI service URL is not configured")
	}

	body, err := json.Marshal(embeddingsRequest{
		Input:          query,
		Model:          e.model,
		EncodingFormat: "float",
		TaskType:       queryTaskType,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embeddings request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v1/embeddings", e.aiServiceURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create embeddings request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Auth is the Cloud Run OIDC ID token attached by the aiclient transport
	// (see internal/aiclient); ai-service verifies the caller's SA identity.

	// #nosec G704 - endpoint is built from system configuration (e.aiServiceURL), not user input
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call AI embeddings service: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && e.logger != nil {
			e.logger.WithError(closeErr).Error("Failed to close embeddings response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI embeddings service returned status %d", resp.StatusCode)
	}

	var decoded embeddingsResponse
	if err = json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("failed to decode embeddings response: %w", err)
	}

	if len(decoded.Data) == 0 {
		return nil, fmt.Errorf("AI embeddings service returned no embedding data")
	}

	vector := decoded.Data[0].Embedding
	if len(vector) != e.dimensions {
		return nil, fmt.Errorf(
			"AI embeddings service returned vector of length %d, expected %d",
			len(vector), e.dimensions,
		)
	}

	return vector, nil
}
