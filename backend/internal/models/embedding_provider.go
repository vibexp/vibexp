package models

import (
	"time"
)

type EmbeddingProvider struct {
	ID              string    `json:"id" db:"id"`
	UserID          string    `json:"user_id" db:"user_id"`
	Name            string    `json:"name" db:"name"`
	ProviderType    string    `json:"provider_type" db:"provider_type"`
	IsDefault       bool      `json:"is_default" db:"is_default"`
	BaseURL         *string   `json:"base_url,omitempty" db:"base_url"`
	APIKeyEncrypted *string   `json:"-" db:"api_key_encrypted"`
	Configuration   string    `json:"configuration" db:"configuration"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
	Version         int64     `json:"version" db:"version"`
}

type CreateEmbeddingProviderRequest struct {
	Name         string  `json:"name" validate:"required,min=1,max=255"`
	ProviderType string  `json:"provider_type" validate:"required"`
	IsDefault    *bool   `json:"is_default,omitempty"`
	BaseURL      *string `json:"base_url,omitempty" validate:"omitempty,url"`
	// #nosec G117 - Request struct field for API key input, not a hardcoded secret
	APIKey        *string                `json:"api_key,omitempty"`
	Configuration map[string]interface{} `json:"configuration,omitempty"`
}

type UpdateEmbeddingProviderRequest struct {
	Name         *string `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	ProviderType *string `json:"provider_type,omitempty"`
	IsDefault    *bool   `json:"is_default,omitempty"`
	BaseURL      *string `json:"base_url,omitempty" validate:"omitempty,url"`
	// #nosec G117 - Request struct field for API key input, not a hardcoded secret
	APIKey        *string                `json:"api_key,omitempty"`
	Configuration map[string]interface{} `json:"configuration,omitempty"`
}

type EmbeddingProviderResponse struct {
	EmbeddingProvider
	HasAPIKey bool `json:"has_api_key"`
}

type EmbeddingProviderListResponse struct {
	EmbeddingProviders []EmbeddingProviderResponse `json:"embedding_providers"`
	TotalCount         int                         `json:"total_count"`
	Page               int                         `json:"page"`
	PerPage            int                         `json:"per_page"`
	TotalPages         int                         `json:"total_pages"`
}

type ValidateEmbeddingProviderRequest struct {
	ProviderType string `json:"provider_type" validate:"required"`
	BaseURL      string `json:"base_url" validate:"required,url"`
	// #nosec G117 - Request struct field for API key input, not a hardcoded secret
	APIKey        *string                `json:"api_key,omitempty"`
	Configuration map[string]interface{} `json:"configuration,omitempty"`
}

type ValidateEmbeddingProviderResponse struct {
	IsValid bool   `json:"is_valid"`
	Message string `json:"message"`
	Details struct {
		ResponseTime int    `json:"response_time_ms,omitempty"`
		StatusCode   int    `json:"status_code,omitempty"`
		ErrorDetails string `json:"error_details,omitempty"`
	} `json:"details,omitempty"`
}
