package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

type APIKeyService struct {
	apiKeyRepo repositories.APIKeyRepository
	logger     *logrus.Logger
}

// Ensure APIKeyService implements APIKeyServiceInterface
var _ APIKeyServiceInterface = (*APIKeyService)(nil)

func NewAPIKeyService(apiKeyRepo repositories.APIKeyRepository, logger *logrus.Logger) *APIKeyService {
	return &APIKeyService{
		apiKeyRepo: apiKeyRepo,
		logger:     logger,
	}
}

// GenerateAPIKey creates a new API key with multi-integration support
func (aks *APIKeyService) GenerateAPIKey(
	ctx context.Context, userID, name string, integrationCodes []string,
) (*models.APIKey, string, error) {
	// Check if the service is nil
	if aks == nil {
		return nil, "", fmt.Errorf("APIKeyService is nil")
	}
	if aks.apiKeyRepo == nil {
		return nil, "", fmt.Errorf("apiKeyRepo is nil")
	}

	// Validate integration codes exist
	validIntegrations, err := aks.apiKeyRepo.GetValidIntegrationCodes(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get valid integrations: %w", err)
	}

	// Check each provided integration code is valid
	for _, code := range integrationCodes {
		valid := false
		for _, validCode := range validIntegrations {
			if code == validCode {
				valid = true
				break
			}
		}
		if !valid {
			return nil, "", fmt.Errorf("invalid integration code: %s", code)
		}
	}

	// Generate a 32-byte random key
	keyBytes := make([]byte, 32)
	if _, readErr := rand.Read(keyBytes); readErr != nil {
		return nil, "", fmt.Errorf("failed to generate random key: %w", readErr)
	}

	// Use unified prefix for all new keys
	fullKey := models.PrefixVibeXPKey + hex.EncodeToString(keyBytes)

	// Create prefix (first 10 characters)
	keyPrefix := fullKey[:10]

	// Hash the full key for storage
	hash := sha256.Sum256([]byte(fullKey))
	keyHash := hex.EncodeToString(hash[:])

	// Create API key model with integrations
	apiKey := &models.APIKey{
		UserID:       userID,
		Name:         name,
		KeyHash:      keyHash,
		KeyPrefix:    keyPrefix,
		Integrations: integrationCodes,
		IsLegacy:     false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Create using repository (will handle integration permissions in transaction)
	err = aks.apiKeyRepo.Create(ctx, apiKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create API key: %w", err)
	}

	return apiKey, fullKey, nil
}

// GenerateAPIKeyLegacy is the legacy method for backward compatibility
//
// Deprecated: Use GenerateAPIKey with integrationCodes instead
func (aks *APIKeyService) GenerateAPIKeyLegacy(
	ctx context.Context, userID, name, usageType string,
) (*models.APIKey, string, error) {
	// Map legacy usage type to integration codes
	var integrationCodes []string
	switch usageType {
	case models.UsageTypeAITools:
		integrationCodes = []string{models.IntegrationCodeAITools}
	case models.UsageTypeCLI:
		integrationCodes = []string{models.IntegrationCodeCLI}
	case models.UsageTypeMCP:
		integrationCodes = []string{models.IntegrationCodeMCPServer}
	case models.UsageTypeEverything:
		// Grant all integrations
		integrationCodes = models.ValidIntegrationCodes()
	default:
		return nil, "", fmt.Errorf("invalid usage type: %s", usageType)
	}

	return aks.GenerateAPIKey(ctx, userID, name, integrationCodes)
}

func (aks *APIKeyService) GetAPIKeysByUserID(ctx context.Context, userID string) ([]models.APIKey, error) {
	// Check if the service is nil
	if aks == nil {
		return nil, fmt.Errorf("APIKeyService is nil")
	}
	if aks.apiKeyRepo == nil {
		return nil, fmt.Errorf("apiKeyRepo is nil")
	}

	return aks.apiKeyRepo.GetByUserID(ctx, userID)
}

func (aks *APIKeyService) ValidateAPIKey(ctx context.Context, key string) (*models.APIKey, error) {
	// Check if the service is nil
	if aks == nil {
		return nil, fmt.Errorf("APIKeyService is nil")
	}
	if aks.apiKeyRepo == nil {
		return nil, fmt.Errorf("apiKeyRepo is nil")
	}

	// Hash the provided key
	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])

	// Look up the API key by hash
	apiKey, err := aks.apiKeyRepo.GetByKeyHash(ctx, keyHash)
	if err != nil {
		return nil, fmt.Errorf("invalid API key: %w", err)
	}

	// Update last used timestamp
	err = aks.apiKeyRepo.UpdateLastUsed(ctx, apiKey.ID, time.Now())
	if err != nil {
		// Log but don't fail the validation for this
		aks.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"method":     "ValidateAPIKey",
			"api_key_id": apiKey.ID,
			"error":      fmt.Sprintf("%+v", err),
		}).Warn("Failed to update last_used_at for API key")
	}

	return apiKey, nil
}

// ValidateAPIKeyForIntegration validates an API key and checks integration permission
func (aks *APIKeyService) ValidateAPIKeyForIntegration(
	ctx context.Context, key, integrationCode string,
) (*models.APIKey, error) {
	// Check if the service is nil
	if aks == nil {
		return nil, fmt.Errorf("APIKeyService is nil")
	}
	if aks.apiKeyRepo == nil {
		return nil, fmt.Errorf("apiKeyRepo is nil")
	}

	// Validate key exists
	apiKey, err := aks.ValidateAPIKey(ctx, key)
	if err != nil {
		return nil, err
	}

	// For legacy keys, check old usage_type field
	if apiKey.IsLegacy && apiKey.UsageType != "" {
		// Map legacy usage types to integration codes
		legacyMapping := map[string]string{
			models.UsageTypeAITools: models.IntegrationCodeAITools,
			models.UsageTypeCLI:     models.IntegrationCodeCLI,
			models.UsageTypeMCP:     models.IntegrationCodeMCPServer,
		}

		if apiKey.UsageType == models.UsageTypeEverything {
			// "everything" keys have access to all integrations
			return apiKey, nil
		}

		if mappedCode, ok := legacyMapping[apiKey.UsageType]; ok && mappedCode == integrationCode {
			return apiKey, nil
		}

		return nil, fmt.Errorf("legacy API key does not have permission for integration: %s", integrationCode)
	}

	// For new keys, check integration permissions
	hasPermission, err := aks.apiKeyRepo.HasIntegrationPermission(ctx, apiKey.ID, integrationCode)
	if err != nil {
		return nil, fmt.Errorf("failed to check permission: %w", err)
	}

	if !hasPermission {
		return nil, fmt.Errorf("API key does not have permission for integration: %s", integrationCode)
	}

	return apiKey, nil
}

func (aks *APIKeyService) DeleteAPIKey(ctx context.Context, userID, apiKeyID string) error {
	// Check if the service is nil
	if aks == nil {
		return fmt.Errorf("APIKeyService is nil")
	}
	if aks.apiKeyRepo == nil {
		return fmt.Errorf("apiKeyRepo is nil")
	}

	return aks.apiKeyRepo.Delete(ctx, userID, apiKeyID)
}
