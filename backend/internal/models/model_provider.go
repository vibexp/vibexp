package models

import (
	"time"
)

type ModelProvider struct {
	ID              string    `json:"id" db:"id"`
	UserID          string    `json:"user_id" db:"user_id"`
	TeamID          *string   `json:"team_id,omitempty" db:"team_id"`
	Name            string    `json:"name" db:"name"`
	ProviderType    string    `json:"provider_type" db:"provider_type"`
	Model           string    `json:"model" db:"model"`
	IsDefault       bool      `json:"is_default" db:"is_default"`
	BaseURL         *string   `json:"base_url,omitempty" db:"base_url"`
	APIKeyEncrypted *string   `json:"-" db:"api_key_encrypted"`
	Configuration   string    `json:"configuration" db:"configuration"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
	Version         int64     `json:"version" db:"version"`
}

type CreateModelProviderRequest struct {
	Name         string  `json:"name" validate:"required,min=1,max=255"`
	ProviderType string  `json:"provider_type" validate:"required"`
	Model        string  `json:"model" validate:"required,min=1,max=255"`
	IsDefault    *bool   `json:"is_default,omitempty"`
	BaseURL      *string `json:"base_url,omitempty" validate:"omitempty,url"`
	// #nosec G117 - Request struct field for API key input, not a hardcoded secret
	APIKey        *string                `json:"api_key,omitempty"`
	Configuration map[string]interface{} `json:"configuration,omitempty"`
}

type UpdateModelProviderRequest struct {
	Name         *string `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	ProviderType *string `json:"provider_type,omitempty"`
	Model        *string `json:"model,omitempty" validate:"omitempty,min=1,max=255"`
	IsDefault    *bool   `json:"is_default,omitempty"`
	BaseURL      *string `json:"base_url,omitempty" validate:"omitempty,url"`
	// #nosec G117 - Request struct field for API key input, not a hardcoded secret
	APIKey        *string                `json:"api_key,omitempty"`
	Configuration map[string]interface{} `json:"configuration,omitempty"`
}

type ModelProviderResponse struct {
	ModelProvider
	HasAPIKey bool `json:"has_api_key"`
}

type ModelProviderListResponse struct {
	ModelProviders JSONArray[ModelProviderResponse] `json:"model_providers"`
	TotalCount     int                              `json:"total_count"`
	Page           int                              `json:"page"`
	PerPage        int                              `json:"per_page"`
	TotalPages     int                              `json:"total_pages"`
}

type ValidateModelProviderRequest struct {
	ProviderType string `json:"provider_type" validate:"required"`
	Model        string `json:"model" validate:"required,min=1,max=255"`
	BaseURL      string `json:"base_url" validate:"required,url"`
	// #nosec G117 - Request struct field for API key input, not a hardcoded secret
	APIKey        *string                `json:"api_key,omitempty"`
	Configuration map[string]interface{} `json:"configuration,omitempty"`
}

type ValidateModelProviderResponse struct {
	IsValid bool                         `json:"is_valid"`
	Message string                       `json:"message"`
	Details ValidateModelProviderDetails `json:"details,omitempty"`
}

// ValidateModelProviderDetails carries diagnostic details from a validation
// probe: how long the probe took, the HTTP status the provider returned, and any
// error detail. Unlike embedding validation there is no dimension assertion — a
// model provider is accepted on reachability + auth alone.
type ValidateModelProviderDetails struct {
	ResponseTime int    `json:"response_time_ms,omitempty"`
	StatusCode   int    `json:"status_code,omitempty"`
	ErrorDetails string `json:"error_details,omitempty"`
}
