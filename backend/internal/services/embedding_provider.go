package services

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Ensure EmbeddingProviderService satisfies the narrow resolver seam consumed by
// the embedding worker and query embedder.
var _ ActiveEmbeddingProviderResolver = (*EmbeddingProviderService)(nil)

type EmbeddingProviderService struct {
	repo          repositories.EmbeddingProviderRepository
	encryptionKey []byte
}

// Ensure EmbeddingProviderService implements EmbeddingProviderServiceInterface
var _ EmbeddingProviderServiceInterface = (*EmbeddingProviderService)(nil)

func NewEmbeddingProviderService(
	repo repositories.EmbeddingProviderRepository, encryptionKey string,
) *EmbeddingProviderService {
	// Use a fixed 32-byte key for AES-256
	key := make([]byte, 32)
	copy(key, []byte(encryptionKey))

	return &EmbeddingProviderService{
		repo:          repo,
		encryptionKey: key,
	}
}

func (eps *EmbeddingProviderService) encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(eps.encryptionKey)
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

func (eps *EmbeddingProviderService) decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(eps.encryptionKey)
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

func (eps *EmbeddingProviderService) prepareAPIKey(req models.CreateEmbeddingProviderRequest) (*string, error) {
	if req.APIKey == nil || *req.APIKey == "" {
		return nil, nil
	}

	encrypted, err := eps.encrypt(*req.APIKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt API key: %w", err)
	}
	return &encrypted, nil
}

func (eps *EmbeddingProviderService) prepareConfiguration(req models.CreateEmbeddingProviderRequest) (string, error) {
	if req.Configuration == nil {
		return "{}", nil
	}

	configBytes, err := json.Marshal(req.Configuration)
	if err != nil {
		return "", fmt.Errorf("failed to marshal configuration: %w", err)
	}
	return string(configBytes), nil
}

func (eps *EmbeddingProviderService) buildEmbeddingProvider(
	userID string,
	req models.CreateEmbeddingProviderRequest,
	encryptedAPIKey *string,
	configJSON string,
) *models.EmbeddingProvider {
	isDefault := false
	if req.IsDefault != nil {
		isDefault = *req.IsDefault
	}

	now := time.Now()
	return &models.EmbeddingProvider{
		UserID:          userID,
		Name:            req.Name,
		ProviderType:    req.ProviderType,
		IsDefault:       isDefault,
		BaseURL:         req.BaseURL,
		APIKeyEncrypted: encryptedAPIKey,
		Configuration:   configJSON,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func (eps *EmbeddingProviderService) CreateEmbeddingProvider(
	ctx context.Context,
	userID string,
	req models.CreateEmbeddingProviderRequest,
) (*models.EmbeddingProvider, error) {
	if eps == nil || eps.repo == nil {
		return nil, fmt.Errorf("EmbeddingProviderService is nil")
	}

	encryptedAPIKey, err := eps.prepareAPIKey(req)
	if err != nil {
		return nil, err
	}

	configJSON, err := eps.prepareConfiguration(req)
	if err != nil {
		return nil, err
	}

	provider := eps.buildEmbeddingProvider(userID, req, encryptedAPIKey, configJSON)

	if err := eps.repo.Create(ctx, provider); err != nil {
		// Check for duplicate/already exists errors from the database
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "already exists") ||
			strings.Contains(errStr, "duplicate") ||
			strings.Contains(errStr, "unique constraint") {
			return nil, fmt.Errorf("%w: %s", ErrProviderAlreadyExists, req.Name)
		}
		return nil, fmt.Errorf("failed to create embedding provider: %w", err)
	}

	if req.IsDefault != nil && *req.IsDefault {
		if err := eps.repo.SetDefault(ctx, userID, provider.ID); err != nil {
			return nil, fmt.Errorf("failed to set as default: %w", err)
		}
	}

	return provider, nil
}

func (eps *EmbeddingProviderService) GetEmbeddingProvidersByUserID(
	ctx context.Context, userID string,
) ([]models.EmbeddingProviderResponse, error) {
	if eps == nil || eps.repo == nil {
		return nil, fmt.Errorf("EmbeddingProviderService is nil")
	}

	// Use repository to list providers with no filters (get all for user)
	filters := repositories.EmbeddingProviderFilters{
		Page:  1,
		Limit: 1000, // Get all providers for the user
	}

	providers, _, err := eps.repo.List(ctx, userID, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to query embedding providers: %w", err)
	}

	// Convert to response format
	responses := make([]models.EmbeddingProviderResponse, 0, len(providers))
	for _, provider := range providers {
		response := models.EmbeddingProviderResponse{
			EmbeddingProvider: provider,
			HasAPIKey:         provider.APIKeyEncrypted != nil && *provider.APIKeyEncrypted != "",
		}

		// Clear the encrypted API key from response
		response.APIKeyEncrypted = nil

		responses = append(responses, response)
	}

	return responses, nil
}

func (eps *EmbeddingProviderService) GetEmbeddingProvider(
	ctx context.Context, userID, providerID string,
) (*models.EmbeddingProviderResponse, error) {
	if eps == nil || eps.repo == nil {
		return nil, fmt.Errorf("EmbeddingProviderService is nil")
	}

	provider, err := eps.repo.GetByID(ctx, userID, providerID)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, providerID)
	}

	response := &models.EmbeddingProviderResponse{
		EmbeddingProvider: *provider,
		HasAPIKey:         provider.APIKeyEncrypted != nil && *provider.APIKeyEncrypted != "",
	}

	// Clear the encrypted API key from response
	response.APIKeyEncrypted = nil

	return response, nil
}

func (eps *EmbeddingProviderService) handleDefaultProviderUpdate(
	ctx context.Context,
	userID, providerID string,
	req models.UpdateEmbeddingProviderRequest,
	provider *models.EmbeddingProvider,
) error {
	if req.IsDefault != nil && *req.IsDefault {
		setDefaultErr := eps.repo.SetDefault(ctx, userID, providerID)
		if setDefaultErr != nil {
			return fmt.Errorf("failed to set as default: %w", setDefaultErr)
		}
		provider.IsDefault = true
	}
	return nil
}

func (eps *EmbeddingProviderService) updateProviderFields(
	req models.UpdateEmbeddingProviderRequest,
	provider *models.EmbeddingProvider,
) error {
	if req.Name != nil {
		provider.Name = *req.Name
	}
	if req.ProviderType != nil {
		provider.ProviderType = *req.ProviderType
	}
	if req.IsDefault != nil {
		provider.IsDefault = *req.IsDefault
	}
	if req.BaseURL != nil {
		provider.BaseURL = req.BaseURL
	}

	if req.APIKey != nil {
		var encryptedAPIKey *string
		if *req.APIKey != "" {
			encrypted, encryptErr := eps.encrypt(*req.APIKey)
			if encryptErr != nil {
				return fmt.Errorf("failed to encrypt API key: %w", encryptErr)
			}
			encryptedAPIKey = &encrypted
		}
		provider.APIKeyEncrypted = encryptedAPIKey
	}

	if req.Configuration != nil {
		configBytes, marshalErr := json.Marshal(req.Configuration)
		if marshalErr != nil {
			return fmt.Errorf("failed to marshal configuration: %w", marshalErr)
		}
		provider.Configuration = string(configBytes)
	}

	return nil
}

func (eps *EmbeddingProviderService) UpdateEmbeddingProvider(
	ctx context.Context,
	userID, providerID string,
	req models.UpdateEmbeddingProviderRequest,
) (*models.EmbeddingProvider, error) {
	if eps == nil || eps.repo == nil {
		return nil, fmt.Errorf("EmbeddingProviderService is nil")
	}

	provider, err := eps.repo.GetByID(ctx, userID, providerID)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, providerID)
	}

	err = eps.handleDefaultProviderUpdate(ctx, userID, providerID, req, provider)
	if err != nil {
		return nil, err
	}

	err = eps.updateProviderFields(req, provider)
	if err != nil {
		return nil, err
	}

	provider.UpdatedAt = time.Now()

	err = eps.repo.Update(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to update embedding provider: %w", err)
	}

	return provider, nil
}

func (eps *EmbeddingProviderService) DeleteEmbeddingProvider(ctx context.Context, userID, providerID string) error {
	if eps == nil || eps.repo == nil {
		return fmt.Errorf("EmbeddingProviderService is nil")
	}

	// First, verify the provider exists and belongs to the user (security check)
	_, err := eps.repo.GetByID(ctx, userID, providerID)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrProviderNotFound, providerID)
	}

	// Check if this is the last provider using the efficient Count method
	count, err := eps.repo.Count(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to count embedding providers: %w", err)
	}

	if count <= 1 {
		return ErrLastProviderDelete
	}

	// Proceed with deletion
	err = eps.repo.Delete(ctx, userID, providerID)
	if err != nil {
		return fmt.Errorf("failed to delete embedding provider: %w", err)
	}

	return nil
}

func (eps *EmbeddingProviderService) GetDefaultEmbeddingProvider(
	ctx context.Context, userID string,
) (*models.EmbeddingProvider, error) {
	if eps == nil || eps.repo == nil {
		return nil, fmt.Errorf("EmbeddingProviderService is nil")
	}

	provider, err := eps.repo.GetDefault(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("no default embedding provider found: %w", err)
	}

	return provider, nil
}

// ResolveActiveProvider resolves the single system-wide embedding provider into a
// ready-to-use EmbeddingProvider, decrypting its stored API key. model is
// EMBEDDING_MODEL and dimensions is the fixed EmbeddingVectorDimensions constant,
// so document and query embeddings share one model and one vector width. It
// returns (nil, nil) when no provider is configured, signalling the embedding
// pipeline to no-op so entity writes still succeed.
func (eps *EmbeddingProviderService) ResolveActiveProvider(
	ctx context.Context, model string, dimensions int,
) (EmbeddingProvider, error) {
	if eps == nil || eps.repo == nil {
		return nil, fmt.Errorf("EmbeddingProviderService is nil")
	}

	row, err := eps.repo.GetActiveProvider(ctx)
	if err != nil {
		if errors.Is(err, repositories.ErrNoActiveEmbeddingProvider) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to resolve active embedding provider: %w", err)
	}

	apiKey := ""
	if row.APIKeyEncrypted != nil {
		apiKey, err = eps.decrypt(*row.APIKeyEncrypted)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt embedding provider API key: %w", err)
		}
	}

	return NewGenerationProvider(row, apiKey, model, dimensions, generateEmbeddingsTimeout)
}

func (eps *EmbeddingProviderService) ValidateEmbeddingProvider(
	ctx context.Context, req models.ValidateEmbeddingProviderRequest,
) (*models.ValidateEmbeddingProviderResponse, error) {
	if eps == nil {
		return nil, fmt.Errorf("EmbeddingProviderService is nil")
	}

	response := &models.ValidateEmbeddingProviderResponse{
		IsValid: false,
		Message: "Validation failed",
	}

	switch req.ProviderType {
	case "openai_compatible":
		return eps.validateOpenAICompatibleProvider(ctx, req)
	default:
		response.Message = fmt.Sprintf("Unsupported provider type: %s", req.ProviderType)
		return response, nil
	}
}

// embeddingValidationProbeText is the sample input sent to a provider during
// validation. It is short and neutral; only the returned vector's shape matters.
const embeddingValidationProbeText = "VibeXP embedding provider validation probe."

// validateOpenAICompatibleProvider validates a provider by running the exact
// generation path used for real embeddings: it builds the provider and requests a
// probe embedding. Reusing NewGenerationProvider means the provider is accepted
// only if it is reachable, authenticates, AND returns a vector of exactly
// EmbeddingVectorDimensions -- the fixed width the vector(N) column requires -- so
// a model that emits a different dimension is rejected before any resource is
// embedded with it.
func (eps *EmbeddingProviderService) validateOpenAICompatibleProvider(
	ctx context.Context,
	req models.ValidateEmbeddingProviderRequest,
) (*models.ValidateEmbeddingProviderResponse, error) {
	response := &models.ValidateEmbeddingProviderResponse{
		IsValid: false,
		Message: "Validation failed",
	}

	baseURL := req.BaseURL
	row := &models.EmbeddingProvider{
		ProviderType: req.ProviderType,
		BaseURL:      &baseURL,
	}

	apiKey := ""
	if req.APIKey != nil {
		apiKey = *req.APIKey
	}

	provider, err := NewGenerationProvider(
		row, apiKey, req.Model, EmbeddingVectorDimensions, generateEmbeddingsTimeout,
	)
	if err != nil {
		response.Message = "Unsupported or misconfigured provider"
		response.Details.ErrorDetails = err.Error()
		return response, nil
	}

	startTime := time.Now()
	vectors, err := provider.GenerateEmbeddings(ctx, []string{embeddingValidationProbeText})
	response.Details.ResponseTime = int(time.Since(startTime).Milliseconds())
	if err != nil {
		response.Message = describeEmbeddingValidationError(err)
		response.Details.ErrorDetails = err.Error()
		return response, nil
	}
	// GenerateEmbeddings already guarantees exactly one vector of
	// EmbeddingVectorDimensions on a nil error (it errors otherwise, handled
	// above); the dimension mismatch surfaces as an error and is reported by
	// describeEmbeddingValidationError. This guard only avoids indexing an empty
	// slice if that contract ever changes.
	if len(vectors) == 0 {
		response.Message = "Provider returned no embedding vector"
		return response, nil
	}

	response.IsValid = true
	response.Message = "Embedding provider validation successful"
	response.Details.Dimension = len(vectors[0])
	return response, nil
}

// describeEmbeddingValidationError maps a generation error to a concise,
// user-facing validation message without leaking internals.
func describeEmbeddingValidationError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "status 401"):
		return "Authentication failed - please check your API key"
	case strings.Contains(msg, "status 404"):
		return "Embeddings endpoint not found - please check your base URL"
	case strings.Contains(msg, "expected") && strings.Contains(msg, "length"):
		return fmt.Sprintf("Provider must return %d-dimensional embeddings", EmbeddingVectorDimensions)
	default:
		return "Failed to validate embedding provider"
	}
}
