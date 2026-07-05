package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

func createTestEmbeddingProviderService(repo repositories.EmbeddingProviderRepository) *EmbeddingProviderService {
	return NewEmbeddingProviderService(repo, "test-encryption-key-32-bytes-12345")
}

func createTestCreateEmbeddingProviderRequest() models.CreateEmbeddingProviderRequest {
	return models.CreateEmbeddingProviderRequest{
		Name:         "Test Provider",
		ProviderType: "openai_compatible",
		Model:        "text-embedding-3-small",
		IsDefault:    boolPtr(true),
		BaseURL:      stringPtr("https://api.openai.com/v1/embeddings"),
		APIKey:       stringPtr("sk-test-key-123"),
		Configuration: map[string]interface{}{
			"model": "text-embedding-ada-002",
		},
	}
}

// TestEmbeddingProviderService_buildEmbeddingProvider_defaults verifies the model
// is persisted and chunk sizing falls back to the package defaults when the
// request omits it (issue #79).
func TestEmbeddingProviderService_buildEmbeddingProvider_defaults(t *testing.T) {
	mockRepo := mocks.NewMockEmbeddingProviderRepository(t)
	service := createTestEmbeddingProviderService(mockRepo)

	t.Run("defaults applied when chunk sizing omitted", func(t *testing.T) {
		p := service.buildEmbeddingProvider("user-1",
			models.CreateEmbeddingProviderRequest{
				Name: "P", ProviderType: "openai_compatible", Model: "m",
			}, nil, "{}")
		assert.Equal(t, "m", p.Model)
		assert.Equal(t, defaultEmbeddingChunkSize, p.ChunkSize)
		assert.Equal(t, defaultEmbeddingChunkOverlap, p.ChunkOverlap)
	})

	t.Run("explicit chunk sizing is preserved", func(t *testing.T) {
		size, overlap := 512, 64
		p := service.buildEmbeddingProvider("user-1",
			models.CreateEmbeddingProviderRequest{
				Name: "P", ProviderType: "openai_compatible", Model: "m",
				ChunkSize: &size, ChunkOverlap: &overlap,
			}, nil, "{}")
		assert.Equal(t, 512, p.ChunkSize)
		assert.Equal(t, 64, p.ChunkOverlap)
	})
}

func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func TestNewEmbeddingProviderService(t *testing.T) {
	mockRepo := mocks.NewMockEmbeddingProviderRepository(t)
	service := NewEmbeddingProviderService(mockRepo, "test-encryption-key")

	assert.NotNil(t, service)
	assert.Equal(t, mockRepo, service.repo)
	assert.Len(t, service.encryptionKey, 32) // AES-256 requires 32-byte key
}

func TestEmbeddingProviderService_encrypt_decrypt(t *testing.T) {
	mockRepo := mocks.NewMockEmbeddingProviderRepository(t)
	service := createTestEmbeddingProviderService(mockRepo)

	tests := []struct {
		name        string
		plaintext   string
		expectError bool
	}{
		{
			name:        "Valid API key encryption",
			plaintext:   "sk-test-key-123456789",
			expectError: false,
		},
		{
			name:        "Empty string",
			plaintext:   "",
			expectError: false,
		},
		{
			name:        "Long API key",
			plaintext:   strings.Repeat("a", 1000),
			expectError: false,
		},
		{
			name:        "Special characters",
			plaintext:   "sk-test!@#$%^&*()_+-={}[]|\\:;\"'<>?,./'",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test encryption
			encrypted, err := service.encrypt(tt.plaintext)
			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			if tt.plaintext == "" {
				assert.Equal(t, "", encrypted)
				return
			}

			assert.NotEqual(t, tt.plaintext, encrypted)
			assert.NotEmpty(t, encrypted)

			// Test decryption
			decrypted, err := service.decrypt(encrypted)
			assert.NoError(t, err)
			assert.Equal(t, tt.plaintext, decrypted)
		})
	}
}

func TestEmbeddingProviderService_decrypt_errors(t *testing.T) {
	mockRepo := mocks.NewMockEmbeddingProviderRepository(t)
	service := createTestEmbeddingProviderService(mockRepo)

	tests := []struct {
		name        string
		ciphertext  string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Invalid base64",
			ciphertext:  "invalid-base64!@#",
			expectError: true,
		},
		{
			name:        "Too short ciphertext",
			ciphertext:  "YWJj", // "abc" in base64, too short for nonce
			expectError: true,
			errorMsg:    "ciphertext too short",
		},
		{
			name:        "Empty string returns empty",
			ciphertext:  "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decrypted, err := service.decrypt(tt.ciphertext)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				if tt.ciphertext == "" {
					assert.Equal(t, "", decrypted)
				}
			}
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestEmbeddingProviderService_CreateEmbeddingProvider(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		request     models.CreateEmbeddingProviderRequest
		setup       func(*mocks.MockEmbeddingProviderRepository)
		expectError bool
		errorMsg    string
	}{
		{
			name:    "Successful provider creation",
			userID:  "user-123",
			request: createTestCreateEmbeddingProviderRequest(),
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				// Mock provider creation
				mockRepo.EXPECT().
					Create(context.Background(), mock.AnythingOfType("*models.EmbeddingProvider")).
					RunAndReturn(func(ctx context.Context, provider *models.EmbeddingProvider) error {
						// Set ID and timestamps to simulate database behavior
						provider.ID = "provider-123"
						provider.CreatedAt = time.Now()
						provider.UpdatedAt = time.Now()
						return nil
					})

				// Mock setting as default after creation
				mockRepo.EXPECT().SetDefault(context.Background(), "user-123", "provider-123").Return(nil)
			},
			expectError: false,
		},
		{
			name:   "Provider without default flag",
			userID: "user-123",
			request: models.CreateEmbeddingProviderRequest{
				Name:         "Non-default Provider",
				ProviderType: "openai_compatible",
				BaseURL:      stringPtr("https://api.openai.com/v1/embeddings"),
			},
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				// Should not call UnsetAllDefaults since IsDefault is nil

				// Mock provider creation
				mockRepo.EXPECT().
					Create(context.Background(), mock.AnythingOfType("*models.EmbeddingProvider")).
					RunAndReturn(func(ctx context.Context, provider *models.EmbeddingProvider) error {
						// Set ID and timestamps to simulate database behavior
						provider.ID = "provider-123"
						provider.CreatedAt = time.Now()
						provider.UpdatedAt = time.Now()
						return nil
					})
			},
			expectError: false,
		},
		{
			name:    "Error on unsetting default",
			userID:  "user-123",
			request: createTestCreateEmbeddingProviderRequest(),
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				// Mock provider creation first
				mockRepo.EXPECT().
					Create(context.Background(), mock.AnythingOfType("*models.EmbeddingProvider")).
					RunAndReturn(func(ctx context.Context, provider *models.EmbeddingProvider) error {
						// Set ID and timestamps to simulate database behavior
						provider.ID = "provider-123"
						provider.CreatedAt = time.Now()
						provider.UpdatedAt = time.Now()
						return nil
					})

				// Mock error on setting default
				mockRepo.EXPECT().
					SetDefault(context.Background(), "user-123", "provider-123").
					Return(errors.New("database error"))
			},
			expectError: true,
			errorMsg:    "failed to set as default",
		},
		{
			name:   "Error on creation",
			userID: "user-123",
			request: models.CreateEmbeddingProviderRequest{
				Name:         "Test Provider",
				ProviderType: "openai_compatible",
			},
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				mockRepo.EXPECT().
					Create(context.Background(), mock.AnythingOfType("*models.EmbeddingProvider")).
					Return(errors.New("database error"))
			},
			expectError: true,
			errorMsg:    "failed to create embedding provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockEmbeddingProviderRepository(t)
			service := createTestEmbeddingProviderService(mockRepo)

			tt.setup(mockRepo)

			provider, err := service.CreateEmbeddingProvider(context.Background(), tt.userID, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, provider)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
				assert.Equal(t, tt.userID, provider.UserID)
				assert.Equal(t, tt.request.Name, provider.Name)
				assert.Equal(t, tt.request.ProviderType, provider.ProviderType)
			}
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestEmbeddingProviderService_GetEmbeddingProvidersByUserID(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		setup       func(*mocks.MockEmbeddingProviderRepository)
		expectError bool
		expectedLen int
	}{
		{
			name:   "Successful retrieval of multiple providers",
			userID: "user-123",
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				providers := []models.EmbeddingProvider{
					{
						ID:              "provider-1",
						UserID:          "user-123",
						Name:            "Default Provider",
						ProviderType:    "openai_compatible",
						IsDefault:       true,
						BaseURL:         stringPtr("https://api.openai.com/v1/embeddings"),
						APIKeyEncrypted: stringPtr("encrypted-key"),
						Configuration:   `{"model":"ada-002"}`,
						CreatedAt:       time.Now(),
						UpdatedAt:       time.Now(),
					},
					{
						ID:              "provider-2",
						UserID:          "user-123",
						Name:            "Secondary Provider",
						ProviderType:    "openai_compatible",
						IsDefault:       false,
						BaseURL:         stringPtr("https://api.custom.com/v1/embeddings"),
						APIKeyEncrypted: nil,
						Configuration:   `{}`,
						CreatedAt:       time.Now(),
						UpdatedAt:       time.Now(),
					},
				}
				filters := repositories.EmbeddingProviderFilters{Page: 1, Limit: 1000}
				mockRepo.EXPECT().
					List(context.Background(), "user-123", filters).
					Return(providers, 2, nil)
			},
			expectError: false,
			expectedLen: 2,
		},
		{
			name:   "No providers found",
			userID: "user-123",
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				filters := repositories.EmbeddingProviderFilters{Page: 1, Limit: 1000}
				mockRepo.EXPECT().
					List(context.Background(), "user-123", filters).
					Return([]models.EmbeddingProvider{}, 0, nil)
			},
			expectError: false,
			expectedLen: 0,
		},
		{
			name:   "Database error",
			userID: "user-123",
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				filters := repositories.EmbeddingProviderFilters{Page: 1, Limit: 1000}
				mockRepo.EXPECT().
					List(context.Background(), "user-123", filters).
					Return(nil, 0, errors.New("database error"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockEmbeddingProviderRepository(t)
			service := createTestEmbeddingProviderService(mockRepo)

			tt.setup(mockRepo)

			providers, err := service.GetEmbeddingProvidersByUserID(context.Background(), tt.userID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, providers)
			} else {
				assert.NoError(t, err)
				assert.Len(t, providers, tt.expectedLen)

				// Verify that API keys are not exposed
				for _, provider := range providers {
					assert.Nil(t, provider.APIKeyEncrypted)
					// HasAPIKey should be set correctly
					if tt.expectedLen > 0 && provider.Name == "Default Provider" {
						assert.True(t, provider.HasAPIKey)
					}
				}
			}
		})
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestEmbeddingProviderService_GetEmbeddingProvider(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		providerID  string
		setup       func(*mocks.MockEmbeddingProviderRepository)
		expectError bool
		errorMsg    string
	}{
		{
			name:       "Successful provider retrieval",
			userID:     "user-123",
			providerID: "provider-123",
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				provider := &models.EmbeddingProvider{
					ID:              "provider-123",
					UserID:          "user-123",
					Name:            "Test Provider",
					ProviderType:    "openai_compatible",
					IsDefault:       true,
					BaseURL:         stringPtr("https://api.openai.com/v1/embeddings"),
					APIKeyEncrypted: stringPtr("encrypted-key"),
					Configuration:   `{"model":"ada-002"}`,
					CreatedAt:       time.Now(),
					UpdatedAt:       time.Now(),
				}
				mockRepo.EXPECT().GetByID(context.Background(), "user-123", "provider-123").Return(provider, nil)
			},
			expectError: false,
		},
		{
			name:       "Provider not found",
			userID:     "user-123",
			providerID: "non-existent",
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				mockRepo.EXPECT().
					GetByID(context.Background(), "user-123", "non-existent").
					Return(nil, errors.New("embedding provider not found"))
			},
			expectError: true,
			errorMsg:    "embedding provider not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockEmbeddingProviderRepository(t)
			service := createTestEmbeddingProviderService(mockRepo)

			tt.setup(mockRepo)

			provider, err := service.GetEmbeddingProvider(context.Background(), tt.userID, tt.providerID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, provider)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
				assert.Equal(t, tt.providerID, provider.ID)
				assert.Equal(t, tt.userID, provider.UserID)
				assert.Nil(t, provider.APIKeyEncrypted) // Should be cleared
				assert.True(t, provider.HasAPIKey)      // Should be set correctly
			}
		})
		//nolint:funlen // Test function requires comprehensive setup and assertions
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestEmbeddingProviderService_UpdateEmbeddingProvider(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		providerID  string
		request     models.UpdateEmbeddingProviderRequest
		setup       func(*mocks.MockEmbeddingProviderRepository)
		expectError bool
		errorMsg    string
	}{
		{
			name:       "Successful update with name only",
			userID:     "user-123",
			providerID: "provider-123",
			request: models.UpdateEmbeddingProviderRequest{
				Name: stringPtr("Updated Provider Name"),
			},
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				existingProvider := &models.EmbeddingProvider{
					ID:              "provider-123",
					UserID:          "user-123",
					Name:            "Old Name",
					ProviderType:    "openai_compatible",
					IsDefault:       false,
					BaseURL:         stringPtr("https://api.openai.com/v1/embeddings"),
					APIKeyEncrypted: stringPtr("encrypted-key"),
					Configuration:   `{"model":"ada-002"}`,
					CreatedAt:       time.Now(),
					UpdatedAt:       time.Now(),
				}
				mockRepo.EXPECT().
					GetByID(context.Background(), "user-123", "provider-123").
					Return(existingProvider, nil)
				mockRepo.EXPECT().
					Update(context.Background(), mock.AnythingOfType("*models.EmbeddingProvider")).
					RunAndReturn(func(ctx context.Context, provider *models.EmbeddingProvider) error {
						assert.Equal(t, "Updated Provider Name", provider.Name)
						return nil
					})
			},
			expectError: false,
		},
		{
			name:       "Update with default flag true",
			userID:     "user-123",
			providerID: "provider-123",
			request: models.UpdateEmbeddingProviderRequest{
				IsDefault: boolPtr(true),
			},
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				existingProvider := &models.EmbeddingProvider{
					ID:              "provider-123",
					UserID:          "user-123",
					Name:            "Test Provider",
					ProviderType:    "openai_compatible",
					IsDefault:       false,
					BaseURL:         stringPtr("https://api.openai.com/v1/embeddings"),
					APIKeyEncrypted: stringPtr("encrypted-key"),
					Configuration:   `{"model":"ada-002"}`,
					CreatedAt:       time.Now(),
					UpdatedAt:       time.Now(),
				}
				mockRepo.EXPECT().
					GetByID(context.Background(), "user-123", "provider-123").
					Return(existingProvider, nil)
				mockRepo.EXPECT().
					SetDefault(context.Background(), "user-123", "provider-123").
					Return(nil)
				mockRepo.EXPECT().
					Update(context.Background(), mock.AnythingOfType("*models.EmbeddingProvider")).
					RunAndReturn(func(ctx context.Context, provider *models.EmbeddingProvider) error {
						assert.True(t, provider.IsDefault)
						return nil
					})
			},
			expectError: false,
		},
		{
			name:       "Provider not found",
			userID:     "user-123",
			providerID: "non-existent",
			request: models.UpdateEmbeddingProviderRequest{
				Name: stringPtr("Updated Name"),
			},
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				mockRepo.EXPECT().
					GetByID(context.Background(), "user-123", "non-existent").
					Return(nil, errors.New("embedding provider not found"))
			},
			expectError: true,
			errorMsg:    "embedding provider not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockEmbeddingProviderRepository(t)
			service := createTestEmbeddingProviderService(mockRepo)

			tt.setup(mockRepo)

			provider, err := service.UpdateEmbeddingProvider(context.Background(), tt.userID, tt.providerID, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, provider)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
				assert.Equal(t, tt.providerID, provider.ID)
				assert.Equal(t, tt.userID, provider.UserID)
				if tt.request.Name != nil {
					assert.Equal(t, *tt.request.Name, provider.Name)
				}
				if tt.request.IsDefault != nil {
					assert.Equal(t, *tt.request.IsDefault, provider.IsDefault)
				}
			}
			//nolint:funlen // Test function requires comprehensive setup and assertions
		})
		//nolint:funlen // Test function requires comprehensive setup and assertions
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestEmbeddingProviderService_DeleteEmbeddingProvider(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		providerID  string
		setup       func(*mocks.MockEmbeddingProviderRepository)
		expectError bool
		errorMsg    string
	}{
		{
			name:       "Successful deletion",
			userID:     "user-123",
			providerID: "provider-123",
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				// Mock ownership verification
				provider := &models.EmbeddingProvider{ID: "provider-123", UserID: "user-123"}
				mockRepo.EXPECT().
					GetByID(context.Background(), "user-123", "provider-123").
					Return(provider, nil)
				// Mock count query returning more than 1
				mockRepo.EXPECT().Count(context.Background(), "user-123").Return(3, nil)
				mockRepo.EXPECT().Delete(context.Background(), "user-123", "provider-123").Return(nil)
			},
			expectError: false,
		},
		{
			name:       "Cannot delete last provider",
			userID:     "user-123",
			providerID: "provider-123",
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				// Mock ownership verification
				provider := &models.EmbeddingProvider{ID: "provider-123", UserID: "user-123"}
				mockRepo.EXPECT().
					GetByID(context.Background(), "user-123", "provider-123").
					Return(provider, nil)
				// Mock count query returning 1
				mockRepo.EXPECT().Count(context.Background(), "user-123").Return(1, nil)
			},
			expectError: true,
			errorMsg:    "cannot delete the last embedding provider",
		},
		{
			name:       "Database error on count",
			userID:     "user-123",
			providerID: "provider-123",
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				// Mock ownership verification
				provider := &models.EmbeddingProvider{ID: "provider-123", UserID: "user-123"}
				mockRepo.EXPECT().
					GetByID(context.Background(), "user-123", "provider-123").
					Return(provider, nil)
				mockRepo.EXPECT().
					Count(context.Background(), "user-123").
					Return(0, errors.New("database error"))
			},
			expectError: true,
			errorMsg:    "failed to count embedding providers",
		},
		{
			name:       "Error on deletion",
			userID:     "user-123",
			providerID: "provider-123",
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				// Mock ownership verification
				provider := &models.EmbeddingProvider{ID: "provider-123", UserID: "user-123"}
				mockRepo.EXPECT().
					GetByID(context.Background(), "user-123", "provider-123").
					Return(provider, nil)
				mockRepo.EXPECT().Count(context.Background(), "user-123").Return(2, nil)
				mockRepo.EXPECT().
					Delete(context.Background(), "user-123", "provider-123").
					Return(errors.New("embedding provider not found"))
			},
			expectError: true,
			errorMsg:    "failed to delete embedding provider",
		},
		{
			name:       "Provider not found or access denied",
			userID:     "user-123",
			providerID: "provider-456",
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				// Mock ownership verification failure
				mockRepo.EXPECT().
					GetByID(context.Background(), "user-123", "provider-456").
					Return(nil, errors.New("provider not found"))
			},
			expectError: true,
			errorMsg:    "embedding provider not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockEmbeddingProviderRepository(t)
			service := createTestEmbeddingProviderService(mockRepo)

			tt.setup(mockRepo)

			err := service.DeleteEmbeddingProvider(context.Background(), tt.userID, tt.providerID)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				//nolint:funlen // Test function requires comprehensive setup and assertions
			}
			//nolint:funlen // Test function requires comprehensive setup and assertions
		})
		//nolint:funlen // Test function requires comprehensive setup and assertions
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestEmbeddingProviderService_GetDefaultEmbeddingProvider(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		setup       func(*mocks.MockEmbeddingProviderRepository)
		expectError bool
		errorMsg    string
	}{
		{
			name:   "Successful default provider retrieval",
			userID: "user-123",
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				provider := &models.EmbeddingProvider{
					ID:              "provider-123",
					UserID:          "user-123",
					Name:            "Default Provider",
					ProviderType:    "openai_compatible",
					IsDefault:       true,
					BaseURL:         stringPtr("https://api.openai.com/v1/embeddings"),
					APIKeyEncrypted: stringPtr("encrypted-key"),
					Configuration:   `{"model":"ada-002"}`,
					CreatedAt:       time.Now(),
					UpdatedAt:       time.Now(),
				}
				mockRepo.EXPECT().GetDefault(context.Background(), "user-123").Return(provider, nil)
			},
			expectError: false,
		},
		{
			name:   "No default provider found",
			userID: "user-123",
			setup: func(mockRepo *mocks.MockEmbeddingProviderRepository) {
				mockRepo.EXPECT().
					GetDefault(context.Background(), "user-123").
					Return(nil, errors.New("no default embedding provider found"))
			},
			expectError: true,
			errorMsg:    "no default embedding provider found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockEmbeddingProviderRepository(t)
			service := createTestEmbeddingProviderService(mockRepo)

			tt.setup(mockRepo)

			provider, err := service.GetDefaultEmbeddingProvider(context.Background(), tt.userID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, provider)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
				assert.Equal(t, tt.userID, provider.UserID)
				assert.True(t, provider.IsDefault)
			}
		})
	}
}

func TestEmbeddingProviderService_ValidateEmbeddingProvider(t *testing.T) {
	// This test focuses on the validation logic without external HTTP calls
	mockRepo := mocks.NewMockEmbeddingProviderRepository(t)
	service := createTestEmbeddingProviderService(mockRepo)

	tests := []struct {
		name     string
		request  models.ValidateEmbeddingProviderRequest
		expected string
	}{
		{
			name: "Unsupported provider type",
			request: models.ValidateEmbeddingProviderRequest{
				ProviderType: "unsupported_type",
				BaseURL:      "https://api.example.com",
			},
			expected: "Unsupported provider type: unsupported_type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := service.ValidateEmbeddingProvider(context.Background(), tt.request)

			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.False(t, resp.IsValid)
			assert.Equal(t, tt.expected, resp.Message)
		})
	}
}

func TestEmbeddingProviderService_validateOpenAICompatibleProvider(t *testing.T) {
	// Note: This test is complex because it requires mocking HTTP client
	// In a real implementation, we would inject the HTTP client as a dependency
	// For now, we'll test the basic structure and error handling

	mockRepo := mocks.NewMockEmbeddingProviderRepository(t)
	service := createTestEmbeddingProviderService(mockRepo)

	// Test the request preparation logic
	req := models.ValidateEmbeddingProviderRequest{
		ProviderType: "openai_compatible",
		BaseURL:      "https://api.openai.com/v1/embeddings",
		APIKey:       stringPtr("sk-test-key"),
		Configuration: map[string]interface{}{
			"model": "custom-model",
		},
	}

	// This will fail because we can't mock the HTTP client without dependency injection
	// But we can verify the basic structure
	resp, err := service.validateOpenAICompatibleProvider(context.Background(), req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	// The response will indicate failure due to network error, which is expected
	assert.False(t, resp.IsValid)
}

func TestEmbeddingProviderService_nilService(t *testing.T) {
	// Test nil service scenarios
	var service *EmbeddingProviderService

	ctx := context.Background()
	userID := "user-123"

	_, err := service.CreateEmbeddingProvider(ctx, userID, models.CreateEmbeddingProviderRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "EmbeddingProviderService is nil")

	_, err = service.GetEmbeddingProvidersByUserID(ctx, userID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "EmbeddingProviderService is nil")

	_, err = service.GetEmbeddingProvider(ctx, userID, "provider-123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "EmbeddingProviderService is nil")

	_, err = service.UpdateEmbeddingProvider(ctx, userID, "provider-123", models.UpdateEmbeddingProviderRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "EmbeddingProviderService is nil")

	err = service.DeleteEmbeddingProvider(ctx, userID, "provider-123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "EmbeddingProviderService is nil")

	_, err = service.GetDefaultEmbeddingProvider(ctx, userID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "EmbeddingProviderService is nil")

	_, err = service.ValidateEmbeddingProvider(ctx, models.ValidateEmbeddingProviderRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "EmbeddingProviderService is nil")
}

// Test helper to verify interface compliance
func TestEmbeddingProviderService_ImplementsInterface(t *testing.T) {
	mockRepo := mocks.NewMockEmbeddingProviderRepository(t)
	service := createTestEmbeddingProviderService(mockRepo)

	// Verify that EmbeddingProviderService implements EmbeddingProviderServiceInterface
	var _ EmbeddingProviderServiceInterface = service
}

// Benchmark tests for encryption operations
func BenchmarkEmbeddingProviderService_encrypt(b *testing.B) {
	// Create a simple test interface implementation for benchmark
	testingInterface := &testingTInterface{b}
	mockRepo := mocks.NewMockEmbeddingProviderRepository(testingInterface)
	service := createTestEmbeddingProviderService(mockRepo)
	testKey := "sk-test-key-1234567890123456789012345678901234567890"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.encrypt(testKey)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEmbeddingProviderService_decrypt(b *testing.B) {
	// Create a simple test interface implementation for benchmark
	testingInterface := &testingTInterface{b}
	mockRepo := mocks.NewMockEmbeddingProviderRepository(testingInterface)
	service := createTestEmbeddingProviderService(mockRepo)
	testKey := "sk-test-key-1234567890123456789012345678901234567890"

	// Encrypt once to get encrypted value
	encrypted, err := service.encrypt(testKey)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.decrypt(encrypted)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Edge case tests
// Simple testing.T interface implementation for benchmarks
type testingTInterface struct {
	*testing.B
}

func (t *testingTInterface) Cleanup(f func()) {
	t.B.Cleanup(f)
}

func TestEmbeddingProviderService_EdgeCases(t *testing.T) {
	t.Run("Encryption with very long keys", func(t *testing.T) {
		mockRepo := mocks.NewMockEmbeddingProviderRepository(t)
		service := createTestEmbeddingProviderService(mockRepo)

		longKey := strings.Repeat("a", 10000)
		encrypted, err := service.encrypt(longKey)
		assert.NoError(t, err)
		assert.NotEmpty(t, encrypted)

		decrypted, err := service.decrypt(encrypted)
		assert.NoError(t, err)
		assert.Equal(t, longKey, decrypted)
	})

	t.Run("Multiple encrypt/decrypt cycles", func(t *testing.T) {
		mockRepo := mocks.NewMockEmbeddingProviderRepository(t)
		service := createTestEmbeddingProviderService(mockRepo)

		original := "sk-test-key-123"
		current := original

		// Multiple encryption/decryption cycles
		for i := 0; i < 10; i++ {
			encrypted, err := service.encrypt(current)
			assert.NoError(t, err)

			decrypted, err := service.decrypt(encrypted)
			assert.NoError(t, err)
			assert.Equal(t, current, decrypted)

			// Each encryption should produce different ciphertext due to random nonce
			encrypted2, err := service.encrypt(current)
			assert.NoError(t, err)
			assert.NotEqual(t, encrypted, encrypted2)
		}
	})
}

// TestEmbeddingProviderService_validateOpenAICompatibleProvider_DimensionEnforcement
// verifies validation runs the real generation path and accepts a provider only
// when it returns exactly EmbeddingVectorDimensions-wide vectors (issue #79).
func TestEmbeddingProviderService_validateOpenAICompatibleProvider_DimensionEnforcement(t *testing.T) {
	mockRepo := mocks.NewMockEmbeddingProviderRepository(t)
	service := createTestEmbeddingProviderService(mockRepo)

	makeServer := func(status, dim int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if status != http.StatusOK {
				w.WriteHeader(status)
				return
			}
			resp := map[string]any{
				"data": []map[string]any{{"index": 0, "embedding": make([]float32, dim)}},
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Errorf("encode probe response: %v", err)
			}
		}))
	}

	validate := func(baseURL string) *models.ValidateEmbeddingProviderResponse {
		resp, err := service.validateOpenAICompatibleProvider(context.Background(),
			models.ValidateEmbeddingProviderRequest{
				ProviderType: ProviderTypeOpenAICompatible,
				Model:        "test-model",
				BaseURL:      baseURL,
			})
		require.NoError(t, err)
		require.NotNil(t, resp)
		return resp
	}

	t.Run("accepts a provider returning the required dimension", func(t *testing.T) {
		srv := makeServer(http.StatusOK, EmbeddingVectorDimensions)
		defer srv.Close()
		resp := validate(srv.URL)
		assert.True(t, resp.IsValid)
		assert.Equal(t, EmbeddingVectorDimensions, resp.Details.Dimension)
	})

	t.Run("rejects a provider returning a different dimension", func(t *testing.T) {
		srv := makeServer(http.StatusOK, 768)
		defer srv.Close()
		resp := validate(srv.URL)
		assert.False(t, resp.IsValid)
		assert.Contains(t, resp.Message, "1024")
	})

	t.Run("rejects an unauthenticated provider", func(t *testing.T) {
		srv := makeServer(http.StatusUnauthorized, 0)
		defer srv.Close()
		resp := validate(srv.URL)
		assert.False(t, resp.IsValid)
	})
}
