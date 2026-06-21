package testutils

import (
	"github.com/vibexp/vibexp/internal/models"
)

// CreatePromptRequestBuilder builds CreatePromptRequest with defaults and optional overrides
func CreatePromptRequestBuilder() *models.CreatePromptRequest {
	return &models.CreatePromptRequest{
		Name:        "Test Prompt",
		Slug:        "test-prompt",
		Description: "This is a test prompt for testing purposes",
		Body:        "You are a helpful assistant. {{context}}",
		Status:      "draft",
	}
}

// CreatePromptRequest builds CreatePromptRequest with defaults and applies optional overrides
func CreatePromptRequest(overrides ...func(*models.CreatePromptRequest)) *models.CreatePromptRequest {
	req := CreatePromptRequestBuilder()

	for _, override := range overrides {
		override(req)
	}

	return req
}

// WithPromptName sets the name for CreatePromptRequest
func WithPromptName(name string) func(*models.CreatePromptRequest) {
	return func(req *models.CreatePromptRequest) {
		req.Name = name
	}
}

// WithPromptSlug sets the slug for CreatePromptRequest
func WithPromptSlug(slug string) func(*models.CreatePromptRequest) {
	return func(req *models.CreatePromptRequest) {
		req.Slug = slug
	}
}

// WithPromptDescription sets the description for CreatePromptRequest
func WithPromptDescription(description string) func(*models.CreatePromptRequest) {
	return func(req *models.CreatePromptRequest) {
		req.Description = description
	}
}

// WithPromptBody sets the body for CreatePromptRequest
func WithPromptBody(body string) func(*models.CreatePromptRequest) {
	return func(req *models.CreatePromptRequest) {
		req.Body = body
	}
}

// WithPromptStatus sets the status for CreatePromptRequest
func WithPromptStatus(status string) func(*models.CreatePromptRequest) {
	return func(req *models.CreatePromptRequest) {
		req.Status = status
	}
}

// UpdatePromptRequestBuilder builds UpdatePromptRequest with defaults
func UpdatePromptRequestBuilder() *models.UpdatePromptRequest {
	name := "Updated Test Prompt"
	description := "This is an updated test prompt"
	body := "You are an updated helpful assistant. {{context}}"
	status := "published"

	return &models.UpdatePromptRequest{
		Name:        &name,
		Description: &description,
		Body:        &body,
		Status:      &status,
	}
}

// UpdatePromptRequest builds UpdatePromptRequest with defaults and applies optional overrides
func UpdatePromptRequest(overrides ...func(*models.UpdatePromptRequest)) *models.UpdatePromptRequest {
	req := UpdatePromptRequestBuilder()

	for _, override := range overrides {
		override(req)
	}

	return req
}

// WithUpdatePromptName sets the name for UpdatePromptRequest
func WithUpdatePromptName(name string) func(*models.UpdatePromptRequest) {
	return func(req *models.UpdatePromptRequest) {
		req.Name = &name
	}
}

// WithUpdatePromptSlug sets the slug for UpdatePromptRequest
func WithUpdatePromptSlug(slug string) func(*models.UpdatePromptRequest) {
	return func(req *models.UpdatePromptRequest) {
		req.Slug = &slug
	}
}

// WithUpdatePromptDescription sets the description for UpdatePromptRequest
func WithUpdatePromptDescription(description string) func(*models.UpdatePromptRequest) {
	return func(req *models.UpdatePromptRequest) {
		req.Description = &description
	}
}

// WithUpdatePromptBody sets the body for UpdatePromptRequest
func WithUpdatePromptBody(body string) func(*models.UpdatePromptRequest) {
	return func(req *models.UpdatePromptRequest) {
		req.Body = &body
	}
}

// WithUpdatePromptStatus sets the status for UpdatePromptRequest
func WithUpdatePromptStatus(status string) func(*models.UpdatePromptRequest) {
	return func(req *models.UpdatePromptRequest) {
		req.Status = &status
	}
}

// CreateAPIKeyRequestBuilder builds CreateAPIKeyRequest with defaults
func CreateAPIKeyRequestBuilder() *models.CreateAPIKeyRequest {
	return &models.CreateAPIKeyRequest{
		Name: "Test API Key",
	}
}

// CreateAPIKeyRequest builds CreateAPIKeyRequest with name
func CreateAPIKeyRequest(name string, overrides ...func(*models.CreateAPIKeyRequest)) *models.CreateAPIKeyRequest {
	req := CreateAPIKeyRequestBuilder()
	req.Name = name

	for _, override := range overrides {
		override(req)
	}

	return req
}

// CreateEmbeddingProviderRequestBuilder builds CreateEmbeddingProviderRequest with defaults
func CreateEmbeddingProviderRequestBuilder() *models.CreateEmbeddingProviderRequest {
	isDefault := false
	return &models.CreateEmbeddingProviderRequest{
		Name:         "Test Embedding Provider",
		ProviderType: "openai",
		IsDefault:    &isDefault,
	}
}

// CreateEmbeddingProviderRequest builds CreateEmbeddingProviderRequest with defaults
// and applies optional overrides
func CreateEmbeddingProviderRequest(
	overrides ...func(*models.CreateEmbeddingProviderRequest),
) *models.CreateEmbeddingProviderRequest {
	req := CreateEmbeddingProviderRequestBuilder()

	for _, override := range overrides {
		override(req)
	}

	return req
}

// WithEmbeddingProviderName sets the name for CreateEmbeddingProviderRequest
func WithEmbeddingProviderName(name string) func(*models.CreateEmbeddingProviderRequest) {
	return func(req *models.CreateEmbeddingProviderRequest) {
		req.Name = name
	}
}

// WithEmbeddingProviderType sets the provider type for CreateEmbeddingProviderRequest
func WithEmbeddingProviderType(providerType string) func(*models.CreateEmbeddingProviderRequest) {
	return func(req *models.CreateEmbeddingProviderRequest) {
		req.ProviderType = providerType
	}
}

// WithEmbeddingProviderDefault sets the default flag for CreateEmbeddingProviderRequest
func WithEmbeddingProviderDefault(isDefault bool) func(*models.CreateEmbeddingProviderRequest) {
	return func(req *models.CreateEmbeddingProviderRequest) {
		req.IsDefault = &isDefault
	}
}

// WithEmbeddingProviderBaseURL sets the base URL for CreateEmbeddingProviderRequest
func WithEmbeddingProviderBaseURL(baseURL string) func(*models.CreateEmbeddingProviderRequest) {
	return func(req *models.CreateEmbeddingProviderRequest) {
		req.BaseURL = &baseURL
	}
}

// WithEmbeddingProviderAPIKey sets the API key for CreateEmbeddingProviderRequest
func WithEmbeddingProviderAPIKey(apiKey string) func(*models.CreateEmbeddingProviderRequest) {
	return func(req *models.CreateEmbeddingProviderRequest) {
		req.APIKey = &apiKey
	}
}

// WithEmbeddingProviderConfiguration sets the configuration for CreateEmbeddingProviderRequest
func WithEmbeddingProviderConfiguration(config map[string]interface{}) func(*models.CreateEmbeddingProviderRequest) {
	return func(req *models.CreateEmbeddingProviderRequest) {
		req.Configuration = config
	}
}

// CreateSubscriptionRequestBuilder builds CreateSubscriptionRequest with defaults
func CreateSubscriptionRequestBuilder() *models.CreateSubscriptionRequest {
	return &models.CreateSubscriptionRequest{
		PriceID: "price_test_123",
	}
}

// CreateSubscriptionRequest builds CreateSubscriptionRequest with price ID
func CreateSubscriptionRequest(
	priceID string, overrides ...func(*models.CreateSubscriptionRequest),
) *models.CreateSubscriptionRequest {
	req := CreateSubscriptionRequestBuilder()
	req.PriceID = priceID

	for _, override := range overrides {
		override(req)
	}

	return req
}

// RenderPromptRequestBuilder builds RenderPromptRequest with defaults
func RenderPromptRequestBuilder() *models.RenderPromptRequest {
	return &models.RenderPromptRequest{
		Placeholders: map[string]string{
			"context": "This is test context",
			"name":    "Test User",
		},
	}
}

// RenderPromptRequest builds RenderPromptRequest with defaults and applies optional overrides
func RenderPromptRequest(overrides ...func(*models.RenderPromptRequest)) *models.RenderPromptRequest {
	req := RenderPromptRequestBuilder()

	for _, override := range overrides {
		override(req)
	}

	return req
}

// WithRenderPromptPlaceholders sets the placeholders for RenderPromptRequest
func WithRenderPromptPlaceholders(placeholders map[string]string) func(*models.RenderPromptRequest) {
	return func(req *models.RenderPromptRequest) {
		req.Placeholders = placeholders
	}
}

// ValidateEmbeddingProviderRequestBuilder builds ValidateEmbeddingProviderRequest with defaults
func ValidateEmbeddingProviderRequestBuilder() *models.ValidateEmbeddingProviderRequest {
	return &models.ValidateEmbeddingProviderRequest{
		ProviderType: "openai",
		BaseURL:      "https://api.openai.com/v1",
	}
}

// ValidateEmbeddingProviderRequest builds ValidateEmbeddingProviderRequest with defaults
// and applies optional overrides
func ValidateEmbeddingProviderRequest(
	overrides ...func(*models.ValidateEmbeddingProviderRequest),
) *models.ValidateEmbeddingProviderRequest {
	req := ValidateEmbeddingProviderRequestBuilder()

	for _, override := range overrides {
		override(req)
	}

	return req
}

// WithValidateProviderType sets the provider type for ValidateEmbeddingProviderRequest
func WithValidateProviderType(providerType string) func(*models.ValidateEmbeddingProviderRequest) {
	return func(req *models.ValidateEmbeddingProviderRequest) {
		req.ProviderType = providerType
	}
}

// WithValidateBaseURL sets the base URL for ValidateEmbeddingProviderRequest
func WithValidateBaseURL(baseURL string) func(*models.ValidateEmbeddingProviderRequest) {
	return func(req *models.ValidateEmbeddingProviderRequest) {
		req.BaseURL = baseURL
	}
}

// WithValidateAPIKey sets the API key for ValidateEmbeddingProviderRequest
func WithValidateAPIKey(apiKey string) func(*models.ValidateEmbeddingProviderRequest) {
	return func(req *models.ValidateEmbeddingProviderRequest) {
		req.APIKey = &apiKey
	}
}

// WithValidateConfiguration sets the configuration for ValidateEmbeddingProviderRequest
func WithValidateConfiguration(config map[string]interface{}) func(*models.ValidateEmbeddingProviderRequest) {
	return func(req *models.ValidateEmbeddingProviderRequest) {
		req.Configuration = config
	}
}
