package models

import (
	"time"
)

// Bounds of a provider's batch_size and request_timeout_seconds (#1665). The
// batch size cap matches text-embeddings-inference's default
// --max-client-batch-size: the setting only lowers the batch for a slow backend,
// never raises it past the limit that made long entities un-embeddable (#756).
// The spec's minimum/maximum, the validate tags and the DB CHECKs mirror these.
const (
	EmbeddingBatchSizeMin             = 1
	EmbeddingBatchSizeMax             = 32
	EmbeddingRequestTimeoutSecondsMin = 5
	EmbeddingRequestTimeoutSecondsMax = 300
)

type EmbeddingProvider struct {
	ID           string  `json:"id" db:"id"`
	UserID       string  `json:"user_id" db:"user_id"`
	TeamID       *string `json:"team_id,omitempty" db:"team_id"`
	Name         string  `json:"name" db:"name"`
	ProviderType string  `json:"provider_type" db:"provider_type"`
	Model        string  `json:"model" db:"model"`
	ChunkSize    int     `json:"chunk_size" db:"chunk_size"`
	ChunkOverlap int     `json:"chunk_overlap" db:"chunk_overlap"`
	Concurrency  int     `json:"concurrency" db:"concurrency"`
	// BatchSize is how many chunks go into one /embeddings request and
	// RequestTimeoutSeconds bounds each such request (#1665). Lower the first and
	// raise the second for a slow or CPU-only provider.
	BatchSize             int       `json:"batch_size" db:"batch_size"`
	RequestTimeoutSeconds int       `json:"request_timeout_seconds" db:"request_timeout_seconds"`
	QueryPrefix           *string   `json:"query_prefix,omitempty" db:"query_prefix"`
	DocumentPrefix        *string   `json:"document_prefix,omitempty" db:"document_prefix"`
	IsDefault             bool      `json:"is_default" db:"is_default"`
	BaseURL               *string   `json:"base_url,omitempty" db:"base_url"`
	APIKeyEncrypted       *string   `json:"-" db:"api_key_encrypted"`
	Configuration         string    `json:"configuration" db:"configuration"`
	CreatedAt             time.Time `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time `json:"updated_at" db:"updated_at"`
	Version               int64     `json:"version" db:"version"`
}

type CreateEmbeddingProviderRequest struct {
	Name         string `json:"name" validate:"required,min=1,max=255"`
	ProviderType string `json:"provider_type" validate:"required"`
	Model        string `json:"model" validate:"required,min=1,max=255"`
	ChunkSize    *int   `json:"chunk_size,omitempty" validate:"omitempty,min=1"`
	ChunkOverlap *int   `json:"chunk_overlap,omitempty" validate:"omitempty,min=0"`
	Concurrency  *int   `json:"concurrency,omitempty" validate:"omitempty,min=1"`
	// BatchSize / RequestTimeoutSeconds: nil (omitted) stores the default
	// (32 chunks, 30 seconds), which is today's hardcoded behaviour.
	BatchSize             *int `json:"batch_size,omitempty" validate:"omitempty,min=1,max=32"`
	RequestTimeoutSeconds *int `json:"request_timeout_seconds,omitempty" validate:"omitempty,min=5,max=300"`
	// QueryPrefix / DocumentPrefix are instruction prefixes for asymmetric
	// embedding models, applied only to the text sent to the provider. Capped
	// at 256 chars; nil (omitted) leaves the current value / empty default.
	QueryPrefix    *string `json:"query_prefix,omitempty" validate:"omitempty,max=256"`
	DocumentPrefix *string `json:"document_prefix,omitempty" validate:"omitempty,max=256"`
	IsDefault      *bool   `json:"is_default,omitempty"`
	BaseURL        *string `json:"base_url,omitempty" validate:"omitempty,url"`
	// #nosec G117 - Request struct field for API key input, not a hardcoded secret
	APIKey        *string                `json:"api_key,omitempty"`
	Configuration map[string]interface{} `json:"configuration,omitempty"`
}

type UpdateEmbeddingProviderRequest struct {
	Name                  *string `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	ProviderType          *string `json:"provider_type,omitempty"`
	Model                 *string `json:"model,omitempty" validate:"omitempty,min=1,max=255"`
	ChunkSize             *int    `json:"chunk_size,omitempty" validate:"omitempty,min=1"`
	ChunkOverlap          *int    `json:"chunk_overlap,omitempty" validate:"omitempty,min=0"`
	Concurrency           *int    `json:"concurrency,omitempty" validate:"omitempty,min=1"`
	BatchSize             *int    `json:"batch_size,omitempty" validate:"omitempty,min=1,max=32"`
	RequestTimeoutSeconds *int    `json:"request_timeout_seconds,omitempty" validate:"omitempty,min=5,max=300"`
	QueryPrefix           *string `json:"query_prefix,omitempty" validate:"omitempty,max=256"`
	DocumentPrefix        *string `json:"document_prefix,omitempty" validate:"omitempty,max=256"`
	IsDefault             *bool   `json:"is_default,omitempty"`
	BaseURL               *string `json:"base_url,omitempty" validate:"omitempty,url"`
	// #nosec G117 - Request struct field for API key input, not a hardcoded secret
	APIKey        *string                `json:"api_key,omitempty"`
	Configuration map[string]interface{} `json:"configuration,omitempty"`
}

type EmbeddingProviderResponse struct {
	EmbeddingProvider
	HasAPIKey bool `json:"has_api_key"`
}

type EmbeddingProviderListResponse struct {
	EmbeddingProviders JSONArray[EmbeddingProviderResponse] `json:"embedding_providers"`
	TotalCount         int                                  `json:"total_count"`
	Page               int                                  `json:"page"`
	PerPage            int                                  `json:"per_page"`
	TotalPages         int                                  `json:"total_pages"`
}

type ValidateEmbeddingProviderRequest struct {
	ProviderType string `json:"provider_type" validate:"required"`
	Model        string `json:"model" validate:"required,min=1,max=255"`
	BaseURL      string `json:"base_url" validate:"required,url"`
	// BatchSize / RequestTimeoutSeconds let the probe run with the values that
	// will be saved; nil means the default.
	BatchSize             *int `json:"batch_size,omitempty" validate:"omitempty,min=1,max=32"`
	RequestTimeoutSeconds *int `json:"request_timeout_seconds,omitempty" validate:"omitempty,min=5,max=300"`
	// #nosec G117 - Request struct field for API key input, not a hardcoded secret
	APIKey        *string                `json:"api_key,omitempty"`
	Configuration map[string]interface{} `json:"configuration,omitempty"`
}

type ValidateEmbeddingProviderResponse struct {
	IsValid bool                             `json:"is_valid"`
	Message string                           `json:"message"`
	Details ValidateEmbeddingProviderDetails `json:"details,omitempty"`
}

// ValidateEmbeddingProviderDetails carries diagnostic details from a validation
// probe. Dimension is the vector width the provider returned, checked against the
// fixed EmbeddingVectorDimensions the store requires.
type ValidateEmbeddingProviderDetails struct {
	ResponseTime int    `json:"response_time_ms,omitempty"`
	StatusCode   int    `json:"status_code,omitempty"`
	Dimension    int    `json:"dimension,omitempty"`
	ErrorDetails string `json:"error_details,omitempty"`
}
