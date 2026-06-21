package testutils

import (
	"testing"
)

func TestCreatePromptRequest(t *testing.T) {
	// Test with default values
	req := CreatePromptRequest()
	if req.Name != "Test Prompt" {
		t.Errorf("Expected Name 'Test Prompt', got %s", req.Name)
	}
	if req.Slug != "test-prompt" {
		t.Errorf("Expected Slug 'test-prompt', got %s", req.Slug)
	}
	if req.Status != "draft" {
		t.Errorf("Expected Status 'draft', got %s", req.Status)
	}

	// Test with overrides
	customName := "Custom Prompt Name"
	customSlug := "custom-slug"
	req = CreatePromptRequest(
		WithPromptName(customName),
		WithPromptSlug(customSlug),
		WithPromptStatus("published"),
	)

	if req.Name != customName {
		t.Errorf("Expected Name '%s', got %s", customName, req.Name)
	}
	if req.Slug != customSlug {
		t.Errorf("Expected Slug '%s', got %s", customSlug, req.Slug)
	}
	if req.Status != "published" {
		t.Errorf("Expected Status 'published', got %s", req.Status)
	}
}

func TestUpdatePromptRequest(t *testing.T) {
	req := UpdatePromptRequest()

	if req.Name == nil || *req.Name != "Updated Test Prompt" {
		t.Error("Expected Name to be 'Updated Test Prompt'")
	}
	if req.Status == nil || *req.Status != "published" {
		t.Error("Expected Status to be 'published'")
	}

	// Test with overrides
	customName := "Custom Updated Name"
	req = UpdatePromptRequest(
		WithUpdatePromptName(customName),
	)

	if req.Name == nil || *req.Name != customName {
		t.Errorf("Expected Name '%s', got %v", customName, req.Name)
	}
}

func TestCreateAPIKeyRequest(t *testing.T) {
	keyName := "Test Key"
	req := CreateAPIKeyRequest(keyName)

	if req.Name != keyName {
		t.Errorf("Expected Name '%s', got %s", keyName, req.Name)
	}
}

//nolint:gocyclo // Test function requires validation of multiple embedding provider request scenarios
func TestCreateEmbeddingProviderRequest(t *testing.T) {
	req := CreateEmbeddingProviderRequest()
	if req.Name != "Test Embedding Provider" {
		t.Errorf("Expected Name 'Test Embedding Provider', got %s", req.Name)
	}
	if req.ProviderType != "openai" {
		t.Errorf("Expected ProviderType 'openai', got %s", req.ProviderType)
	}
	if req.IsDefault == nil || *req.IsDefault != false {
		t.Error("Expected IsDefault to be false")
	}

	// Test with overrides
	customName := "Custom Provider"
	customType := "anthropic"
	isDefault := true
	baseURL := "https://api.anthropic.com"
	apiKey := "test-api-key"
	config := map[string]interface{}{
		"model": "claude-3",
	}

	req = CreateEmbeddingProviderRequest(
		WithEmbeddingProviderName(customName),
		WithEmbeddingProviderType(customType),
		WithEmbeddingProviderDefault(isDefault),
		WithEmbeddingProviderBaseURL(baseURL),
		WithEmbeddingProviderAPIKey(apiKey),
		WithEmbeddingProviderConfiguration(config),
	)

	if req.Name != customName {
		t.Errorf("Expected Name '%s', got %s", customName, req.Name)
	}
	if req.ProviderType != customType {
		t.Errorf("Expected ProviderType '%s', got %s", customType, req.ProviderType)
	}
	if req.IsDefault == nil || *req.IsDefault != isDefault {
		t.Errorf("Expected IsDefault %v, got %v", isDefault, req.IsDefault)
	}
	if req.BaseURL == nil || *req.BaseURL != baseURL {
		t.Errorf("Expected BaseURL '%s', got %v", baseURL, req.BaseURL)
	}
	if req.APIKey == nil || *req.APIKey != apiKey {
		t.Errorf("Expected APIKey '%s', got %v", apiKey, req.APIKey)
	}
	if req.Configuration == nil {
		t.Error("Expected Configuration to not be nil")
	}
}

func TestCreateSubscriptionRequest(t *testing.T) {
	priceID := "price_test_456"
	req := CreateSubscriptionRequest(priceID)

	if req.PriceID != priceID {
		t.Errorf("Expected PriceID '%s', got %s", priceID, req.PriceID)
	}
}

func TestRenderPromptRequest(t *testing.T) {
	req := RenderPromptRequest()
	if req.Placeholders == nil {
		t.Error("Expected Placeholders to not be nil")
	}
	if req.Placeholders["context"] != "This is test context" {
		t.Error("Expected default context placeholder")
	}
	if req.Placeholders["name"] != "Test User" {
		t.Error("Expected default name placeholder")
	}

	// Test with custom placeholders
	customPlaceholders := map[string]string{
		"custom": "custom value",
		"test":   "test value",
	}

	req = RenderPromptRequest(
		WithRenderPromptPlaceholders(customPlaceholders),
	)

	if req.Placeholders["custom"] != "custom value" {
		t.Error("Expected custom placeholder to be set")
	}
	if req.Placeholders["test"] != "test value" {
		t.Error("Expected test placeholder to be set")
	}
}

func TestValidateEmbeddingProviderRequest(t *testing.T) {
	req := ValidateEmbeddingProviderRequest()
	if req.ProviderType != "openai" {
		t.Errorf("Expected ProviderType 'openai', got %s", req.ProviderType)
	}
	if req.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("Expected BaseURL 'https://api.openai.com/v1', got %s", req.BaseURL)
	}

	// Test with overrides
	customType := "anthropic"
	customURL := "https://api.anthropic.com/v1"
	apiKey := "test-key"
	config := map[string]interface{}{
		"model": "claude-3",
	}

	req = ValidateEmbeddingProviderRequest(
		WithValidateProviderType(customType),
		WithValidateBaseURL(customURL),
		WithValidateAPIKey(apiKey),
		WithValidateConfiguration(config),
	)

	if req.ProviderType != customType {
		t.Errorf("Expected ProviderType '%s', got %s", customType, req.ProviderType)
	}
	if req.BaseURL != customURL {
		t.Errorf("Expected BaseURL '%s', got %s", customURL, req.BaseURL)
	}
	if req.APIKey == nil || *req.APIKey != apiKey {
		t.Errorf("Expected APIKey '%s', got %v", apiKey, req.APIKey)
	}
	if req.Configuration == nil {
		t.Error("Expected Configuration to not be nil")
	}
}
