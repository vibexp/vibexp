package testutils

import (
	"time"

	"github.com/vibexp/vibexp/internal/models"
)

// TestUser generates a test user with default values
func TestUser() *models.User {
	return TestUserWithID("test-user-123")
}

// TestUserWithID generates a test user with a specific ID
func TestUserWithID(userID string) *models.User {
	now := time.Now()
	googleID := "google-" + userID
	return &models.User{
		ID:                 userID,
		GoogleID:           &googleID,
		Email:              "test@example.com",
		Name:               "Test User",
		AvatarURL:          nil,
		StripeCustomerID:   nil,
		SubscriptionStatus: "basic",
		TrialEndsAt:        nil,
		SubscriptionPlan:   &[]string{"basic"}[0],
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// TestUserWithEmail generates a test user with a specific email
func TestUserWithEmail(email string) *models.User {
	user := TestUser()
	user.Email = email
	return user
}

// TestPrompt generates a test prompt for a given user ID
func TestPrompt(userID string) *models.Prompt {
	return TestPromptWithData(userID, "test-prompt", "Test Prompt", "draft")
}

// TestPromptWithData generates a test prompt with specific data
func TestPromptWithData(userID, slug, name, status string) *models.Prompt {
	now := time.Now()
	return &models.Prompt{
		ID:          "prompt-" + slug + "-123",
		Name:        name,
		Slug:        slug,
		Description: "This is a test prompt for " + name,
		Body:        "You are a helpful assistant. {{context}}",
		UserID:      userID,
		Status:      status,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// TestPromptPublished generates a published test prompt
func TestPromptPublished(userID string) *models.Prompt {
	return TestPromptWithData(userID, "published-prompt", "Published Test Prompt", "published")
}

// TestPromptDraft generates a draft test prompt
func TestPromptDraft(userID string) *models.Prompt {
	return TestPromptWithData(userID, "draft-prompt", "Draft Test Prompt", "draft")
}

// TestAPIKey generates a test API key for a given user ID
func TestAPIKey(userID string) *models.APIKey {
	return TestAPIKeyWithName(userID, "Test API Key")
}

// TestAPIKeyWithName generates a test API key with a specific name
func TestAPIKeyWithName(userID, name string) *models.APIKey {
	now := time.Now()
	return &models.APIKey{
		ID:         "api-key-" + userID + "-123",
		UserID:     userID,
		Name:       name,
		KeyHash:    "hashed-key-value",
		KeyPrefix:  "ak_test123",
		LastUsedAt: nil,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// TestAPIKeyUsed generates a test API key that has been used recently
func TestAPIKeyUsed(userID string) *models.APIKey {
	apiKey := TestAPIKey(userID)
	now := time.Now()
	apiKey.LastUsedAt = &now
	return apiKey
}

// TestEmbeddingProvider generates a test embedding provider for a given user ID
func TestEmbeddingProvider(userID string) *models.EmbeddingProvider {
	return TestEmbeddingProviderWithData(userID, "Test Provider", "openai", false)
}

// TestEmbeddingProviderWithData generates a test embedding provider with specific data
func TestEmbeddingProviderWithData(userID, name, providerType string, isDefault bool) *models.EmbeddingProvider {
	now := time.Now()
	baseURL := "https://api.openai.com/v1"
	return &models.EmbeddingProvider{
		ID:              "provider-" + userID + "-123",
		UserID:          userID,
		Name:            name,
		ProviderType:    providerType,
		IsDefault:       isDefault,
		BaseURL:         &baseURL,
		APIKeyEncrypted: nil,
		Configuration:   "{}",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// TestEmbeddingProviderDefault generates a default test embedding provider
func TestEmbeddingProviderDefault(userID string) *models.EmbeddingProvider {
	return TestEmbeddingProviderWithData(userID, "Default Provider", "openai", true)
}

// TestEmbeddingProviderWithAPIKey generates a test embedding provider with encrypted API key
func TestEmbeddingProviderWithAPIKey(userID string) *models.EmbeddingProvider {
	provider := TestEmbeddingProvider(userID)
	encryptedKey := "encrypted-api-key-value"
	provider.APIKeyEncrypted = &encryptedKey
	return provider
}

// TestGoogleUserInfo generates test Google user info
func TestGoogleUserInfo() *models.GoogleUserInfo {
	return &models.GoogleUserInfo{
		ID:            "google-123456789",
		Email:         "test@example.com",
		VerifiedEmail: true,
		Name:          "Test User",
		GivenName:     "Test",
		FamilyName:    "User",
		Picture:       "https://example.com/avatar.jpg",
	}
}

// TestErrorResponse generates a test error response
func TestErrorResponse(errorCode, message string) *models.ErrorResponse {
	return &models.ErrorResponse{
		Error:   errorCode,
		Message: message,
	}
}

// TestPromptListResponse generates a test prompt list response
func TestPromptListResponse(prompts []models.Prompt, page, perPage int) *models.PromptListResponse {
	totalCount := len(prompts)
	totalPages := (totalCount + perPage - 1) / perPage
	return &models.PromptListResponse{
		Prompts:    prompts,
		TotalCount: totalCount,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}
}

// TestEmbeddingProviderListResponse generates a test embedding provider list response
func TestEmbeddingProviderListResponse(
	providers []models.EmbeddingProviderResponse, page, perPage int,
) *models.EmbeddingProviderListResponse {
	totalCount := len(providers)
	totalPages := (totalCount + perPage - 1) / perPage
	return &models.EmbeddingProviderListResponse{
		EmbeddingProviders: providers,
		TotalCount:         totalCount,
		Page:               page,
		PerPage:            perPage,
		TotalPages:         totalPages,
	}
}

// TestEmbeddingProviderResponse generates a test embedding provider response
func TestEmbeddingProviderResponse(
	provider *models.EmbeddingProvider, hasAPIKey bool,
) *models.EmbeddingProviderResponse {
	return &models.EmbeddingProviderResponse{
		EmbeddingProvider: *provider,
		HasAPIKey:         hasAPIKey,
	}
}

// TestCreateAPIKeyResponse generates a test create API key response
func TestCreateAPIKeyResponse(apiKey *models.APIKey, fullKey string) *models.CreateAPIKeyResponse {
	return &models.CreateAPIKeyResponse{
		APIKey:    *apiKey,
		FullKey:   fullKey,
		KeyPrefix: apiKey.KeyPrefix,
	}
}

// TestRenderPromptResponse generates a test render prompt response
func TestRenderPromptResponse(renderedBody string) *models.RenderPromptResponse {
	return &models.RenderPromptResponse{
		RenderedBody:        renderedBody,
		PlaceholdersMissing: []string{},
		ReferencesUsed:      []string{},
	}
}

// TestRenderPromptResponseWithMissing generates a test render prompt response with missing placeholders
func TestRenderPromptResponseWithMissing(renderedBody string, missing []string) *models.RenderPromptResponse {
	return &models.RenderPromptResponse{
		RenderedBody:        renderedBody,
		PlaceholdersMissing: missing,
		ReferencesUsed:      []string{},
	}
}

// TestValidateEmbeddingProviderResponse generates a test validate embedding provider response
func TestValidateEmbeddingProviderResponse(isValid bool, message string) *models.ValidateEmbeddingProviderResponse {
	return &models.ValidateEmbeddingProviderResponse{
		IsValid: isValid,
		Message: message,
	}
}
