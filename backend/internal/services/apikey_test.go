package services

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// generateAPIKeyCase is one GenerateAPIKeyLegacy table case.
type generateAPIKeyCase struct {
	name        string
	userID      string
	keyName     string
	usageType   string
	setupMocks  func(*mocks.MockAPIKeyRepository)
	expectError bool
	errorMsg    string
	validateKey func(*testing.T, *models.APIKey, string)
}

// assertGeneratedAPIKey verifies one GenerateAPIKeyLegacy table case outcome.
func assertGeneratedAPIKey(t *testing.T, tt generateAPIKeyCase, apiKey *models.APIKey, fullKey string, err error) {
	t.Helper()
	if tt.expectError {
		assert.Error(t, err)
		if tt.errorMsg != "" {
			assert.Contains(t, err.Error(), tt.errorMsg)
		}
		assert.Nil(t, apiKey)
		assert.Empty(t, fullKey)
		return
	}
	assert.NoError(t, err)
	if tt.validateKey != nil {
		tt.validateKey(t, apiKey, fullKey)
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAPIKeyService_GenerateAPIKey_New(t *testing.T) {
	tests := []generateAPIKeyCase{
		{
			name:      "successful generation with everything type",
			userID:    "user-123",
			keyName:   "test-key",
			usageType: "everything",
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				mockRepo.On("GetValidIntegrationCodes", context.Background()).
					Return(models.ValidIntegrationCodes(), nil)
				mockRepo.On("Create", context.Background(), mock.MatchedBy(func(apiKey *models.APIKey) bool {
					return apiKey.UserID == "user-123" &&
						apiKey.Name == "test-key" &&
						apiKey.KeyHash != "" &&
						apiKey.KeyPrefix != "" &&
						len(apiKey.Integrations) == 3 // everything maps to all integrations
				})).Return(nil).Run(func(args mock.Arguments) {
					apiKey := args.Get(1).(*models.APIKey)
					apiKey.ID = "api-key-123"
				})
			},
			expectError: false,
			validateKey: func(t *testing.T, key *models.APIKey, fullKey string) {
				assert.NotNil(t, key)
				assert.Equal(t, "user-123", key.UserID)
				assert.Equal(t, "test-key", key.Name)
				assert.Len(t, key.Integrations, 3) // everything maps to all 3 integrations
				assert.True(t, strings.HasPrefix(fullKey, "vxk_"))
				assert.Len(t, fullKey, 68) // vxk_ + 64 hex chars (32 bytes)
				assert.True(t, strings.HasPrefix(key.KeyPrefix, "vxk_"))
				assert.Len(t, key.KeyPrefix, 10) // vxk_ + 6 chars
				assert.Equal(t, "api-key-123", key.ID)
			},
		},
		{
			name:      "successful generation with ai_tools type",
			userID:    "user-123",
			keyName:   "test-key",
			usageType: "ai_tools",
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				mockRepo.On("GetValidIntegrationCodes", context.Background()).
					Return(models.ValidIntegrationCodes(), nil)
				mockRepo.On("Create", context.Background(), mock.MatchedBy(func(apiKey *models.APIKey) bool {
					return apiKey.UserID == "user-123" &&
						apiKey.Name == "test-key" &&
						len(apiKey.Integrations) == 1 &&
						apiKey.Integrations[0] == "ai_tools"
				})).Return(nil).Run(func(args mock.Arguments) {
					apiKey := args.Get(1).(*models.APIKey)
					apiKey.ID = "api-key-123"
				})
			},
			expectError: false,
			validateKey: func(t *testing.T, key *models.APIKey, fullKey string) {
				assert.NotNil(t, key)
				assert.Equal(t, []string{"ai_tools"}, []string(key.Integrations))
				assert.True(t, strings.HasPrefix(fullKey, "vxk_"))
				assert.Len(t, fullKey, 68) // vxk_ + 64 hex chars
				assert.True(t, strings.HasPrefix(key.KeyPrefix, "vxk_"))
			},
		},
		{
			name:      "successful generation with cli type",
			userID:    "user-123",
			keyName:   "test-key",
			usageType: "cli",
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				mockRepo.On("GetValidIntegrationCodes", context.Background()).
					Return(models.ValidIntegrationCodes(), nil)
				mockRepo.On("Create", context.Background(), mock.MatchedBy(func(apiKey *models.APIKey) bool {
					return len(apiKey.Integrations) == 1 && apiKey.Integrations[0] == "cli"
				})).Return(nil).Run(func(args mock.Arguments) {
					apiKey := args.Get(1).(*models.APIKey)
					apiKey.ID = "api-key-123"
				})
			},
			expectError: false,
			validateKey: func(t *testing.T, key *models.APIKey, fullKey string) {
				assert.Equal(t, []string{"cli"}, []string(key.Integrations))
				assert.True(t, strings.HasPrefix(fullKey, "vxk_"))
				assert.Len(t, fullKey, 68) // vxk_ + 64 hex chars
				assert.True(t, strings.HasPrefix(key.KeyPrefix, "vxk_"))
			},
		},
		{
			name:      "successful generation with mcp type",
			userID:    "user-123",
			keyName:   "test-key",
			usageType: "mcp",
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				mockRepo.On("GetValidIntegrationCodes", context.Background()).
					Return(models.ValidIntegrationCodes(), nil)
				mockRepo.On("Create", context.Background(), mock.MatchedBy(func(apiKey *models.APIKey) bool {
					return len(apiKey.Integrations) == 1 && apiKey.Integrations[0] == "mcp_server"
				})).Return(nil).Run(func(args mock.Arguments) {
					apiKey := args.Get(1).(*models.APIKey)
					apiKey.ID = "api-key-123"
				})
			},
			expectError: false,
			validateKey: func(t *testing.T, key *models.APIKey, fullKey string) {
				assert.Equal(t, []string{"mcp_server"}, []string(key.Integrations))
				assert.True(t, strings.HasPrefix(fullKey, "vxk_"))
				assert.Len(t, fullKey, 68) // vxk_ + 64 hex chars
				assert.True(t, strings.HasPrefix(key.KeyPrefix, "vxk_"))
			},
		},
		{
			name:        "invalid usage type",
			userID:      "user-123",
			keyName:     "test-key",
			usageType:   "invalid",
			setupMocks:  func(mockRepo *mocks.MockAPIKeyRepository) {},
			expectError: true,
			errorMsg:    "invalid usage type",
		},
		{
			name:      "repository error",
			userID:    "user-123",
			keyName:   "test-key",
			usageType: "everything",
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				mockRepo.On("GetValidIntegrationCodes", context.Background()).
					Return(models.ValidIntegrationCodes(), nil)
				mockRepo.On("Create", context.Background(), mock.AnythingOfType("*models.APIKey")).
					Return(assert.AnError)
			},
			expectError: true,
			errorMsg:    "failed to create API key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mocks.MockAPIKeyRepository{}
			service := NewAPIKeyService(mockRepo, func() *slog.Logger { l, _ := logtest.New(); return l }())
			tt.setupMocks(mockRepo)

			ctx := context.Background()
			apiKey, fullKey, err := service.GenerateAPIKeyLegacy(ctx, tt.userID, tt.keyName, tt.usageType)

			assertGeneratedAPIKey(t, tt, apiKey, fullKey, err)

			mockRepo.AssertExpectations(t)
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAPIKeyService_GetAPIKeysByUserID_New(t *testing.T) {
	testKeys := []models.APIKey{
		{
			ID:        "key-1",
			UserID:    "user-123",
			Name:      "Test Key 1",
			KeyHash:   "hash1",
			KeyPrefix: "ak_test1",
			CreatedAt: time.Now(),
		},
		{
			ID:        "key-2",
			UserID:    "user-123",
			Name:      "Test Key 2",
			KeyHash:   "hash2",
			KeyPrefix: "ak_test2",
			CreatedAt: time.Now(),
		},
	}

	tests := []struct {
		name         string
		userID       string
		setupMocks   func(*mocks.MockAPIKeyRepository)
		expectError  bool
		expectedKeys []models.APIKey
	}{
		{
			name:   "successful retrieval",
			userID: "user-123",
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				mockRepo.On("GetByUserID", context.Background(), "user-123").Return(testKeys, nil)
			},
			expectError:  false,
			expectedKeys: testKeys,
		},
		{
			name:   "empty result",
			userID: "user-456",
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				mockRepo.On("GetByUserID", context.Background(), "user-456").Return([]models.APIKey{}, nil)
			},
			expectError:  false,
			expectedKeys: []models.APIKey{},
		},
		{
			name:   "repository error",
			userID: "user-123",
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				mockRepo.On("GetByUserID", context.Background(), "user-123").Return(nil, assert.AnError)
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mocks.MockAPIKeyRepository{}
			service := NewAPIKeyService(mockRepo, func() *slog.Logger { l, _ := logtest.New(); return l }())
			tt.setupMocks(mockRepo)

			ctx := context.Background()
			keys, err := service.GetAPIKeysByUserID(ctx, tt.userID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, keys)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedKeys, keys)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestAPIKeyService_ValidateAPIKey_New(t *testing.T) {
	testKey := &models.APIKey{
		ID:        "key-123",
		UserID:    "user-123",
		Name:      "Test Key",
		KeyHash:   "test-hash",
		KeyPrefix: "ak_test",
		CreatedAt: time.Now(),
	}

	tests := []struct {
		name        string
		key         string
		setupMocks  func(*mocks.MockAPIKeyRepository)
		expectError bool
		expectedKey *models.APIKey
	}{
		{
			name: "successful validation",
			key:  "test-key",
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				mockRepo.On("GetByKeyHash", context.Background(), mock.AnythingOfType("string")).Return(testKey, nil)
				mockRepo.On("UpdateLastUsed", context.Background(), "key-123", mock.AnythingOfType("time.Time")).Return(nil)
			},
			expectError: false,
			expectedKey: testKey,
		},
		{
			name: "key not found",
			key:  "invalid-key",
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				mockRepo.On("GetByKeyHash", context.Background(), mock.AnythingOfType("string")).Return(nil, assert.AnError)
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mocks.MockAPIKeyRepository{}
			service := NewAPIKeyService(mockRepo, func() *slog.Logger { l, _ := logtest.New(); return l }())
			tt.setupMocks(mockRepo)

			ctx := context.Background()
			key, err := service.ValidateAPIKey(ctx, tt.key)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, key)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedKey, key)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestAPIKeyService_DeleteAPIKey_New(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		keyID       string
		setupMocks  func(*mocks.MockAPIKeyRepository)
		expectError bool
	}{
		{
			name:   "successful deletion",
			userID: "user-123",
			keyID:  "key-123",
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				mockRepo.On("Delete", context.Background(), "user-123", "key-123").Return(nil)
			},
			expectError: false,
		},
		{
			name:   "repository error",
			userID: "user-123",
			keyID:  "key-123",
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				mockRepo.On("Delete", context.Background(), "user-123", "key-123").Return(assert.AnError)
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mocks.MockAPIKeyRepository{}
			service := NewAPIKeyService(mockRepo, func() *slog.Logger { l, _ := logtest.New(); return l }())
			tt.setupMocks(mockRepo)

			ctx := context.Background()
			err := service.DeleteAPIKey(ctx, tt.userID, tt.keyID)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAPIKeyService_ValidateAPIKeyForIntegration(t *testing.T) {
	testKey := &models.APIKey{
		ID:        "key-123",
		UserID:    "user-123",
		Name:      "Test Key",
		KeyHash:   "test-hash",
		KeyPrefix: "vxk_test",
		IsLegacy:  false,
		CreatedAt: time.Now(),
	}

	legacyKeyEverything := &models.APIKey{
		ID:        "legacy-key-1",
		UserID:    "user-123",
		Name:      "Legacy Everything Key",
		KeyHash:   "legacy-hash-1",
		KeyPrefix: "vxk_leg",
		IsLegacy:  true,
		UsageType: models.UsageTypeEverything,
		CreatedAt: time.Now(),
	}

	legacyKeyAITools := &models.APIKey{
		ID:        "legacy-key-2",
		UserID:    "user-123",
		Name:      "Legacy AI Tools Key",
		KeyHash:   "legacy-hash-2",
		KeyPrefix: "vxk_leg",
		IsLegacy:  true,
		UsageType: models.UsageTypeAITools,
		CreatedAt: time.Now(),
	}

	tests := []struct {
		name            string
		key             string
		integrationCode string
		setupMocks      func(*mocks.MockAPIKeyRepository)
		expectError     bool
		errorMsg        string
		expectedKey     *models.APIKey
	}{
		{
			name:            "successful validation with permission",
			key:             "test-key",
			integrationCode: models.IntegrationCodeAITools,
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				ctx := context.Background()
				mockRepo.On("GetByKeyHash", ctx, mock.AnythingOfType("string")).Return(testKey, nil)
				mockRepo.On("UpdateLastUsed", ctx, "key-123", mock.AnythingOfType("time.Time")).Return(nil)
				mockRepo.On("HasIntegrationPermission", ctx, "key-123", models.IntegrationCodeAITools).
					Return(true, nil)
			},
			expectError: false,
			expectedKey: testKey,
		},
		{
			name:            "validation fails - no permission",
			key:             "test-key",
			integrationCode: models.IntegrationCodeCLI,
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				ctx := context.Background()
				mockRepo.On("GetByKeyHash", ctx, mock.AnythingOfType("string")).Return(testKey, nil)
				mockRepo.On("UpdateLastUsed", ctx, "key-123", mock.AnythingOfType("time.Time")).Return(nil)
				mockRepo.On("HasIntegrationPermission", ctx, "key-123", models.IntegrationCodeCLI).
					Return(false, nil)
			},
			expectError: true,
			errorMsg:    "does not have permission",
		},
		{
			name:            "key not found",
			key:             "invalid-key",
			integrationCode: models.IntegrationCodeAITools,
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				mockRepo.On("GetByKeyHash", context.Background(), mock.AnythingOfType("string")).Return(nil, assert.AnError)
			},
			expectError: true,
			errorMsg:    "invalid API key",
		},
		{
			name:            "legacy key with everything type - all access",
			key:             "legacy-key",
			integrationCode: models.IntegrationCodeMCPServer,
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				mockRepo.On("GetByKeyHash", context.Background(), mock.AnythingOfType("string")).Return(legacyKeyEverything, nil)
				mockRepo.On("UpdateLastUsed", context.Background(), "legacy-key-1", mock.AnythingOfType("time.Time")).Return(nil)
			},
			expectError: false,
			expectedKey: legacyKeyEverything,
		},
		{
			name:            "legacy key with ai_tools type - matching integration",
			key:             "legacy-key-ai",
			integrationCode: models.IntegrationCodeAITools,
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				mockRepo.On("GetByKeyHash", context.Background(), mock.AnythingOfType("string")).Return(legacyKeyAITools, nil)
				mockRepo.On("UpdateLastUsed", context.Background(), "legacy-key-2", mock.AnythingOfType("time.Time")).Return(nil)
			},
			expectError: false,
			expectedKey: legacyKeyAITools,
		},
		{
			name:            "legacy key with ai_tools type - non-matching integration",
			key:             "legacy-key-ai",
			integrationCode: models.IntegrationCodeCLI,
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				mockRepo.On("GetByKeyHash", context.Background(), mock.AnythingOfType("string")).Return(legacyKeyAITools, nil)
				mockRepo.On("UpdateLastUsed", context.Background(), "legacy-key-2", mock.AnythingOfType("time.Time")).Return(nil)
			},
			expectError: true,
			errorMsg:    "legacy API key does not have permission",
		},
		{
			name:            "permission check error",
			key:             "test-key",
			integrationCode: models.IntegrationCodeAITools,
			setupMocks: func(mockRepo *mocks.MockAPIKeyRepository) {
				ctx := context.Background()
				mockRepo.On("GetByKeyHash", ctx, mock.AnythingOfType("string")).Return(testKey, nil)
				mockRepo.On("UpdateLastUsed", ctx, "key-123", mock.AnythingOfType("time.Time")).Return(nil)
				mockRepo.On("HasIntegrationPermission", ctx, "key-123", models.IntegrationCodeAITools).
					Return(false, assert.AnError)
			},
			expectError: true,
			errorMsg:    "failed to check permission",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mocks.MockAPIKeyRepository{}
			service := NewAPIKeyService(mockRepo, func() *slog.Logger { l, _ := logtest.New(); return l }())
			tt.setupMocks(mockRepo)

			ctx := context.Background()
			key, err := service.ValidateAPIKeyForIntegration(ctx, tt.key, tt.integrationCode)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, key)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedKey, key)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestAPIKeyService_NilServiceCheck(t *testing.T) {
	var service *APIKeyService

	ctx := context.Background()

	t.Run("GenerateAPIKey with nil service", func(t *testing.T) {
		key, fullKey, err := service.GenerateAPIKey(ctx, "user-123", "test-key", []string{"ai_tools"})
		assert.Error(t, err)
		assert.Nil(t, key)
		assert.Empty(t, fullKey)
		assert.Contains(t, err.Error(), "APIKeyService is nil")
	})

	t.Run("GetAPIKeysByUserID with nil service", func(t *testing.T) {
		keys, err := service.GetAPIKeysByUserID(ctx, "user-123")
		assert.Error(t, err)
		assert.Nil(t, keys)
		assert.Contains(t, err.Error(), "APIKeyService is nil")
	})

	t.Run("ValidateAPIKey with nil service", func(t *testing.T) {
		key, err := service.ValidateAPIKey(ctx, "test-key")
		assert.Error(t, err)
		assert.Nil(t, key)
		assert.Contains(t, err.Error(), "APIKeyService is nil")
	})

	t.Run("ValidateAPIKeyForIntegration with nil service", func(t *testing.T) {
		key, err := service.ValidateAPIKeyForIntegration(ctx, "test-key", models.IntegrationCodeAITools)
		assert.Error(t, err)
		assert.Nil(t, key)
		assert.Contains(t, err.Error(), "APIKeyService is nil")
	})

	t.Run("DeleteAPIKey with nil service", func(t *testing.T) {
		err := service.DeleteAPIKey(ctx, "user-123", "key-123")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "APIKeyService is nil")
	})
}

func TestAPIKeyService_NilRepoCheck(t *testing.T) {
	logger, _ := logtest.New()
	service := NewAPIKeyService(nil, logger)

	ctx := context.Background()

	t.Run("GenerateAPIKey with nil repo", func(t *testing.T) {
		key, fullKey, err := service.GenerateAPIKey(ctx, "user-123", "test-key", []string{"ai_tools"})
		assert.Error(t, err)
		assert.Nil(t, key)
		assert.Empty(t, fullKey)
		assert.Contains(t, err.Error(), "apiKeyRepo is nil")
	})

	t.Run("GetAPIKeysByUserID with nil repo", func(t *testing.T) {
		keys, err := service.GetAPIKeysByUserID(ctx, "user-123")
		assert.Error(t, err)
		assert.Nil(t, keys)
		assert.Contains(t, err.Error(), "apiKeyRepo is nil")
	})

	t.Run("ValidateAPIKey with nil repo", func(t *testing.T) {
		key, err := service.ValidateAPIKey(ctx, "test-key")
		assert.Error(t, err)
		assert.Nil(t, key)
		assert.Contains(t, err.Error(), "apiKeyRepo is nil")
	})

	t.Run("ValidateAPIKeyForIntegration with nil repo", func(t *testing.T) {
		key, err := service.ValidateAPIKeyForIntegration(ctx, "test-key", models.IntegrationCodeAITools)
		assert.Error(t, err)
		assert.Nil(t, key)
		assert.Contains(t, err.Error(), "apiKeyRepo is nil")
	})

	t.Run("DeleteAPIKey with nil repo", func(t *testing.T) {
		err := service.DeleteAPIKey(ctx, "user-123", "key-123")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "apiKeyRepo is nil")
	})
}
