package services

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

type ModelProviderService struct {
	repo          repositories.ModelProviderRepository
	encryptionKey []byte
}

// Ensure ModelProviderService implements ModelProviderServiceInterface
var _ ModelProviderServiceInterface = (*ModelProviderService)(nil)

func NewModelProviderService(
	repo repositories.ModelProviderRepository, encryptionKey string,
) *ModelProviderService {
	// Use a fixed 32-byte key for AES-256
	key := make([]byte, 32)
	copy(key, []byte(encryptionKey))

	return &ModelProviderService{
		repo:          repo,
		encryptionKey: key,
	}
}

func (mps *ModelProviderService) encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(mps.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (mps *ModelProviderService) decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(mps.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, cipherData := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func (mps *ModelProviderService) prepareAPIKey(req models.CreateModelProviderRequest) (*string, error) {
	if req.APIKey == nil || *req.APIKey == "" {
		return nil, nil
	}

	encrypted, err := mps.encrypt(*req.APIKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt API key: %w", err)
	}
	return &encrypted, nil
}

func (mps *ModelProviderService) prepareConfiguration(req models.CreateModelProviderRequest) (string, error) {
	if req.Configuration == nil {
		return "{}", nil
	}

	configBytes, err := json.Marshal(req.Configuration)
	if err != nil {
		return "", fmt.Errorf("failed to marshal configuration: %w", err)
	}
	return string(configBytes), nil
}

func (mps *ModelProviderService) buildModelProvider(
	teamID, userID string,
	req models.CreateModelProviderRequest,
	encryptedAPIKey *string,
	configJSON string,
) *models.ModelProvider {
	isDefault := false
	if req.IsDefault != nil {
		isDefault = *req.IsDefault
	}

	now := time.Now()
	return &models.ModelProvider{
		UserID:          userID,
		TeamID:          &teamID,
		Name:            req.Name,
		ProviderType:    req.ProviderType,
		Model:           req.Model,
		IsDefault:       isDefault,
		BaseURL:         req.BaseURL,
		APIKeyEncrypted: encryptedAPIKey,
		Configuration:   configJSON,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func (mps *ModelProviderService) CreateModelProvider(
	ctx context.Context,
	teamID string,
	userID string,
	req models.CreateModelProviderRequest,
) (*models.ModelProvider, error) {
	if mps == nil || mps.repo == nil {
		return nil, fmt.Errorf("ModelProviderService is nil")
	}

	encryptedAPIKey, err := mps.prepareAPIKey(req)
	if err != nil {
		return nil, err
	}

	configJSON, err := mps.prepareConfiguration(req)
	if err != nil {
		return nil, err
	}

	provider := mps.buildModelProvider(teamID, userID, req, encryptedAPIKey, configJSON)

	if err := mps.repo.Create(ctx, provider); err != nil {
		// Check for duplicate/already exists errors from the database
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "already exists") ||
			strings.Contains(errStr, "duplicate") ||
			strings.Contains(errStr, "unique constraint") {
			return nil, fmt.Errorf("%w: %s", ErrModelProviderAlreadyExists, req.Name)
		}
		return nil, fmt.Errorf("failed to create model provider: %w", err)
	}

	if req.IsDefault != nil && *req.IsDefault {
		if err := mps.repo.SetDefault(ctx, teamID, provider.ID); err != nil {
			return nil, fmt.Errorf("failed to set as default: %w", err)
		}
	}

	return provider, nil
}

func (mps *ModelProviderService) GetModelProvidersByTeamID(
	ctx context.Context, teamID string,
) ([]models.ModelProviderResponse, error) {
	if mps == nil || mps.repo == nil {
		return nil, fmt.Errorf("ModelProviderService is nil")
	}

	// Use repository to list providers with no filters (get all for the team)
	filters := repositories.ModelProviderFilters{
		Page:  1,
		Limit: 1000, // Get all providers for the team
	}

	providers, _, err := mps.repo.List(ctx, teamID, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to query model providers: %w", err)
	}

	// Convert to response format
	responses := make([]models.ModelProviderResponse, 0, len(providers))
	for _, provider := range providers {
		response := models.ModelProviderResponse{
			ModelProvider: provider,
			HasAPIKey:     provider.APIKeyEncrypted != nil && *provider.APIKeyEncrypted != "",
		}

		// Clear the encrypted API key from response
		response.APIKeyEncrypted = nil

		responses = append(responses, response)
	}

	return responses, nil
}

func (mps *ModelProviderService) GetModelProvider(
	ctx context.Context, teamID, providerID string,
) (*models.ModelProviderResponse, error) {
	if mps == nil || mps.repo == nil {
		return nil, fmt.Errorf("ModelProviderService is nil")
	}

	provider, err := mps.repo.GetByID(ctx, teamID, providerID)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrModelProviderNotFound, providerID)
	}

	response := &models.ModelProviderResponse{
		ModelProvider: *provider,
		HasAPIKey:     provider.APIKeyEncrypted != nil && *provider.APIKeyEncrypted != "",
	}

	// Clear the encrypted API key from response
	response.APIKeyEncrypted = nil

	return response, nil
}

func (mps *ModelProviderService) handleDefaultProviderUpdate(
	ctx context.Context,
	teamID, providerID string,
	req models.UpdateModelProviderRequest,
	provider *models.ModelProvider,
) error {
	if req.IsDefault != nil && *req.IsDefault {
		setDefaultErr := mps.repo.SetDefault(ctx, teamID, providerID)
		if setDefaultErr != nil {
			return fmt.Errorf("failed to set as default: %w", setDefaultErr)
		}
		provider.IsDefault = true
	}
	return nil
}

func (mps *ModelProviderService) updateProviderFields(
	req models.UpdateModelProviderRequest,
	provider *models.ModelProvider,
) error {
	if req.Name != nil {
		provider.Name = *req.Name
	}
	if req.ProviderType != nil {
		provider.ProviderType = *req.ProviderType
	}
	if req.Model != nil {
		provider.Model = *req.Model
	}
	if req.IsDefault != nil {
		provider.IsDefault = *req.IsDefault
	}
	if req.BaseURL != nil {
		provider.BaseURL = req.BaseURL
	}

	if err := mps.applyAPIKeyUpdate(req, provider); err != nil {
		return err
	}

	return applyModelConfigurationUpdate(req, provider)
}

// applyAPIKeyUpdate re-encrypts and applies an API-key change from the update
// request, treating an explicit empty string as "clear the stored key". A nil
// APIKey (field omitted) preserves the stored key untouched.
func (mps *ModelProviderService) applyAPIKeyUpdate(
	req models.UpdateModelProviderRequest, provider *models.ModelProvider,
) error {
	if req.APIKey == nil {
		return nil
	}
	var encryptedAPIKey *string
	if *req.APIKey != "" {
		encrypted, err := mps.encrypt(*req.APIKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt API key: %w", err)
		}
		encryptedAPIKey = &encrypted
	}
	provider.APIKeyEncrypted = encryptedAPIKey
	return nil
}

// applyModelConfigurationUpdate marshals and applies a configuration change from
// the update request.
func applyModelConfigurationUpdate(
	req models.UpdateModelProviderRequest, provider *models.ModelProvider,
) error {
	if req.Configuration == nil {
		return nil
	}
	configBytes, err := json.Marshal(req.Configuration)
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}
	provider.Configuration = string(configBytes)
	return nil
}

func (mps *ModelProviderService) UpdateModelProvider(
	ctx context.Context,
	teamID, providerID string,
	req models.UpdateModelProviderRequest,
) (*models.ModelProvider, error) {
	if mps == nil || mps.repo == nil {
		return nil, fmt.Errorf("ModelProviderService is nil")
	}

	provider, err := mps.repo.GetByID(ctx, teamID, providerID)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrModelProviderNotFound, providerID)
	}

	err = mps.handleDefaultProviderUpdate(ctx, teamID, providerID, req, provider)
	if err != nil {
		return nil, err
	}

	err = mps.updateProviderFields(req, provider)
	if err != nil {
		return nil, err
	}

	provider.UpdatedAt = time.Now()

	err = mps.repo.Update(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to update model provider: %w", err)
	}

	return provider, nil
}

func (mps *ModelProviderService) DeleteModelProvider(ctx context.Context, teamID, providerID string) error {
	if mps == nil || mps.repo == nil {
		return fmt.Errorf("ModelProviderService is nil")
	}

	// First, verify the provider exists and belongs to the team (security check)
	_, err := mps.repo.GetByID(ctx, teamID, providerID)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrModelProviderNotFound, providerID)
	}

	// Check if this is the last provider using the efficient Count method
	count, err := mps.repo.Count(ctx, teamID)
	if err != nil {
		return fmt.Errorf("failed to count model providers: %w", err)
	}

	if count <= 1 {
		return ErrLastModelProviderDelete
	}

	// Proceed with deletion
	err = mps.repo.Delete(ctx, teamID, providerID)
	if err != nil {
		return fmt.Errorf("failed to delete model provider: %w", err)
	}

	return nil
}

func (mps *ModelProviderService) GetDefaultModelProvider(
	ctx context.Context, teamID string,
) (*models.ModelProvider, error) {
	if mps == nil || mps.repo == nil {
		return nil, fmt.Errorf("ModelProviderService is nil")
	}

	provider, err := mps.repo.GetDefault(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("no default model provider found: %w", err)
	}

	return provider, nil
}

func (mps *ModelProviderService) ValidateModelProvider(
	ctx context.Context, req models.ValidateModelProviderRequest,
) (*models.ValidateModelProviderResponse, error) {
	if mps == nil {
		return nil, fmt.Errorf("ModelProviderService is nil")
	}

	response := &models.ValidateModelProviderResponse{
		IsValid: false,
		Message: "Validation failed",
	}

	switch req.ProviderType {
	case ProviderTypeOpenAICompatible:
		return mps.validateOpenAICompatibleProvider(ctx, req)
	default:
		response.Message = fmt.Sprintf("Unsupported provider type: %s", req.ProviderType)
		return response, nil
	}
}

// validateOpenAICompatibleProvider builds a ModelProvider from the request and
// runs its connectivity + auth probe. Reusing NewModelProvider means validation
// exercises the exact seam a future runtime consumer resolves against. A
// misconfigured (unreachable/unauthorized) provider is reported in the response
// body, never as an error.
func (mps *ModelProviderService) validateOpenAICompatibleProvider(
	ctx context.Context,
	req models.ValidateModelProviderRequest,
) (*models.ValidateModelProviderResponse, error) {
	response := &models.ValidateModelProviderResponse{
		IsValid: false,
		Message: "Validation failed",
	}

	baseURL := req.BaseURL
	row := &models.ModelProvider{
		ProviderType: req.ProviderType,
		BaseURL:      &baseURL,
	}

	apiKey := ""
	if req.APIKey != nil {
		apiKey = *req.APIKey
	}

	provider, err := NewModelProvider(row, apiKey, req.Model, validateModelProviderTimeout)
	if err != nil {
		response.Message = "Unsupported or misconfigured provider"
		response.Details.ErrorDetails = err.Error()
		return response, nil
	}

	return provider.Validate(ctx)
}
