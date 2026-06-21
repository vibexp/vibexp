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
		SubscriptionStatus: models.SubscriptionStatusBasic,
		TrialEndsAt:        nil,
		SubscriptionPlan:   &[]string{models.PlanBasic}[0],
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

// TestUserWithSubscription generates a test user with a specific subscription status and plan
func TestUserWithSubscription(status, plan string) *models.User {
	user := TestUser()
	user.SubscriptionStatus = status
	user.SubscriptionPlan = &plan
	return user
}

// TestUserPro generates a test user with pro subscription
func TestUserPro() *models.User {
	user := TestUser()
	user.SubscriptionStatus = models.SubscriptionStatusActive
	user.SubscriptionPlan = &[]string{models.PlanPro}[0]
	return user
}

// TestUserWithStripe generates a test user with Stripe customer ID
func TestUserWithStripe(stripeCustomerID string) *models.User {
	user := TestUser()
	user.StripeCustomerID = &stripeCustomerID
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

// TestSubscription generates a test subscription for a given user ID
func TestSubscription(userID string) *models.Subscription {
	return TestSubscriptionWithStatus(userID, models.SubscriptionStatusBasic)
}

// TestSubscriptionWithStatus generates a test subscription with specific status
func TestSubscriptionWithStatus(userID, status string) *models.Subscription {
	now := time.Now()
	return &models.Subscription{
		ID:                   "sub-" + userID + "-123",
		UserID:               userID,
		StripeSubscriptionID: nil,
		StripeCustomerID:     nil,
		Status:               status,
		PlanName:             nil,
		CurrentPeriodStart:   nil,
		CurrentPeriodEnd:     nil,
		TrialEnd:             nil,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

// TestSubscriptionActive generates an active test subscription
func TestSubscriptionActive(userID string) *models.Subscription {
	sub := TestSubscriptionWithStatus(userID, models.SubscriptionStatusActive)
	stripeSubID := "sub_stripe_123"
	stripeCustID := "cus_stripe_123"
	planName := models.PlanPro
	now := time.Now()
	periodStart := now.Add(-30 * 24 * time.Hour) // Started 30 days ago
	periodEnd := now.Add(30 * 24 * time.Hour)    // Ends in 30 days

	sub.StripeSubscriptionID = &stripeSubID
	sub.StripeCustomerID = &stripeCustID
	sub.PlanName = &planName
	sub.CurrentPeriodStart = &periodStart
	sub.CurrentPeriodEnd = &periodEnd
	return sub
}

// TestSubscriptionTrial generates a trial test subscription
func TestSubscriptionTrial(userID string) *models.Subscription {
	sub := TestSubscriptionWithStatus(userID, models.SubscriptionStatusTrialActive)
	trialEnd := time.Now().Add(7 * 24 * time.Hour) // Trial ends in 7 days
	sub.TrialEnd = &trialEnd
	return sub
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

// TestCreateSubscriptionResponse generates a test create subscription response
func TestCreateSubscriptionResponse(checkoutURL, sessionID string) *models.CreateSubscriptionResponse {
	return &models.CreateSubscriptionResponse{
		CheckoutURL: checkoutURL,
		SessionID:   sessionID,
	}
}

// TestSubscriptionStatusResponse generates a test subscription status response
func TestSubscriptionStatusResponse(status string, canAccess bool) *models.SubscriptionStatusResponse {
	return &models.SubscriptionStatusResponse{
		Status:           status,
		IsTrialActive:    status == models.SubscriptionStatusTrialActive,
		CanAccessService: canAccess,
	}
}

// TestCreatePortalSessionResponse generates a test create portal session response
func TestCreatePortalSessionResponse(url string) *models.CreatePortalSessionResponse {
	return &models.CreatePortalSessionResponse{
		URL: url,
	}
}

// TestProductConfiguration generates a test product configuration
func TestProductConfiguration() *models.ProductConfiguration {
	return &models.ProductConfiguration{
		ID:       "prod_test_123",
		Name:     "Test Plan",
		PriceID:  "price_test_123",
		Currency: "usd",
		Amount:   999,
		Popular:  false,
		MarketingFeatures: []string{
			"Feature 1",
			"Feature 2",
			"Feature 3",
		},
	}
}

// TestProductConfigurationResponse generates a test product configuration response
func TestProductConfigurationResponse(products []models.ProductConfiguration) *models.ProductConfigurationResponse {
	return &models.ProductConfigurationResponse{
		Products: products,
	}
}
