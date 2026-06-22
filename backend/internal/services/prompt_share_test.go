package services

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

func createTestPromptShareService(
	shareRepo *mocks.MockPromptShareRepository,
	promptRepo *mocks.MockPromptRepository,
) *PromptShareService {
	logger, _ := logtest.New()
	return NewPromptShareService(shareRepo, promptRepo, nil, logger)
}

func createTestPromptForSharing() *models.Prompt {
	now := time.Now()
	return &models.Prompt{
		ID:          "prompt-123",
		Name:        "Test Prompt",
		Slug:        "test-prompt",
		Description: "A test prompt",
		Body:        "Hello {{name}}, this is a test.",
		UserID:      "user-123",
		Status:      "published",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func createTestPromptShare() *models.PromptShare {
	now := time.Now()
	return &models.PromptShare{
		ID:          "share-123",
		PromptID:    "prompt-123",
		ShareToken:  "abc123xyz789",
		ShareType:   "public",
		CreatedBy:   "user-123",
		CreatedAt:   now,
		IsActive:    true,
		AccessCount: 0,
	}
}

func TestNewPromptShareService(t *testing.T) {
	shareRepo := mocks.NewMockPromptShareRepository(t)
	promptRepo := mocks.NewMockPromptRepository(t)
	logger, _ := logtest.New()

	service := NewPromptShareService(shareRepo, promptRepo, nil, logger)

	assert.NotNil(t, service)
	assert.Equal(t, shareRepo, service.shareRepo)
	assert.Equal(t, promptRepo, service.promptRepo)
}

func TestPromptShareService_generateShareToken(t *testing.T) {
	shareRepo := mocks.NewMockPromptShareRepository(t)
	promptRepo := mocks.NewMockPromptRepository(t)
	service := createTestPromptShareService(shareRepo, promptRepo)

	// Generate multiple tokens to verify uniqueness
	tokens := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := service.generateShareToken()
		assert.NoError(t, err)
		assert.NotEmpty(t, token)
		assert.Len(t, token, 43)
		assert.False(t, tokens[token], "Token should be unique")
		tokens[token] = true
	}
}

//nolint:funlen // Test function requires comprehensive test cases
func TestPromptShareService_CreateShare(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		promptSlug  string
		request     *models.CreateShareRequest
		setupMocks  func(*mocks.MockPromptShareRepository, *mocks.MockPromptRepository)
		expectError bool
		errorMsg    string
	}{
		{
			name:       "Successfully create public share",
			userID:     "user-123",
			promptSlug: "test-prompt",
			request: &models.CreateShareRequest{
				ShareType: "public",
			},
			setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
				prompt := createTestPromptForSharing()
				promptRepo.On("GetBySlugCrossTeam", mock.Anything, "user-123", "test-prompt").
					Return(prompt, nil)
				shareRepo.On("GetByPromptID", mock.Anything, "prompt-123").
					Return(nil, errors.New("share not found"))
				shareRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.PromptShare")).
					Return(nil).Run(func(args mock.Arguments) {
					share := args.Get(1).(*models.PromptShare)
					share.ID = "share-123"
					share.CreatedAt = time.Now()
				})
			},
			expectError: false,
		},
		{
			name:       "Successfully create restricted share with emails",
			userID:     "user-123",
			promptSlug: "test-prompt",
			request: &models.CreateShareRequest{
				ShareType: "restricted",
				Emails:    []string{"alice@example.com", "bob@example.com"},
			},
			setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
				prompt := createTestPromptForSharing()
				promptRepo.On("GetBySlugCrossTeam", mock.Anything, "user-123", "test-prompt").
					Return(prompt, nil)
				shareRepo.On("GetByPromptID", mock.Anything, "prompt-123").
					Return(nil, errors.New("share not found"))
				shareRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.PromptShare")).
					Return(nil).Run(func(args mock.Arguments) {
					share := args.Get(1).(*models.PromptShare)
					share.ID = "share-123"
					share.CreatedAt = time.Now()
				})
				shareRepo.On("AddAccessEmails", mock.Anything, "share-123",
					[]string{"alice@example.com", "bob@example.com"}).Return(nil)
			},
			expectError: false,
		},
		{
			name:       "Update existing public share",
			userID:     "user-123",
			promptSlug: "test-prompt",
			request: &models.CreateShareRequest{
				ShareType: "public",
			},
			setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
				prompt := createTestPromptForSharing()
				promptRepo.On("GetBySlugCrossTeam", mock.Anything, "user-123", "test-prompt").
					Return(prompt, nil)
				existingShare := createTestPromptShare()
				existingShare.ShareType = "restricted"
				shareRepo.On("GetByPromptID", mock.Anything, "prompt-123").
					Return(existingShare, nil)
				shareRepo.On("Update", mock.Anything, mock.AnythingOfType("*models.PromptShare")).
					Return(nil)
				shareRepo.On("AddAccessEmails", mock.Anything, existingShare.ID, []string{}).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:       "Update existing share to restricted with emails",
			userID:     "user-123",
			promptSlug: "test-prompt",
			request: &models.CreateShareRequest{
				ShareType: "restricted",
				Emails:    []string{"charlie@example.com"},
			},
			setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
				prompt := createTestPromptForSharing()
				promptRepo.On("GetBySlugCrossTeam", mock.Anything, "user-123", "test-prompt").
					Return(prompt, nil)
				existingShare := createTestPromptShare()
				shareRepo.On("GetByPromptID", mock.Anything, "prompt-123").
					Return(existingShare, nil)
				shareRepo.On("Update", mock.Anything, mock.AnythingOfType("*models.PromptShare")).
					Return(nil)
				shareRepo.On("AddAccessEmails", mock.Anything, existingShare.ID,
					[]string{"charlie@example.com"}).Return(nil)
				shareRepo.On("GetAccessEmails", mock.Anything, existingShare.ID).
					Return([]string{"charlie@example.com"}, nil)
			},
			expectError: false,
		},
		{
			name:       "Fail when prompt not found",
			userID:     "user-123",
			promptSlug: "nonexistent",
			request: &models.CreateShareRequest{
				ShareType: "public",
			},
			setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
				promptRepo.On("GetBySlugCrossTeam", mock.Anything, "user-123", "nonexistent").
					Return(nil, errors.New("not found"))
			},
			expectError: true,
			errorMsg:    "prompt not found",
		},
		{
			name:       "Fail when restricted share has no emails",
			userID:     "user-123",
			promptSlug: "test-prompt",
			request: &models.CreateShareRequest{
				ShareType: "restricted",
				Emails:    []string{},
			},
			setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
				prompt := createTestPromptForSharing()
				promptRepo.On("GetBySlugCrossTeam", mock.Anything, "user-123", "test-prompt").
					Return(prompt, nil)
			},
			expectError: true,
			errorMsg:    "restricted shares must specify at least one email",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shareRepo := mocks.NewMockPromptShareRepository(t)
			promptRepo := mocks.NewMockPromptRepository(t)
			tt.setupMocks(shareRepo, promptRepo)

			service := createTestPromptShareService(shareRepo, promptRepo)

			result, err := service.CreateShare(tt.userID, tt.promptSlug, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.ShareToken)
				assert.Equal(t, tt.request.ShareType, result.ShareType)
				assert.True(t, strings.HasPrefix(result.ShareURL, "/shared/prompts/"))
			}
		})
	}
}

var getShareTests = []struct {
	name        string
	userID      string
	promptSlug  string
	setupMocks  func(*mocks.MockPromptShareRepository, *mocks.MockPromptRepository)
	expectError bool
	errorMsg    string
}{
	{
		name:       "Successfully get public share",
		userID:     "user-123",
		promptSlug: "test-prompt",
		setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
			prompt := createTestPromptForSharing()
			promptRepo.On("GetBySlugCrossTeam", mock.Anything, "user-123", "test-prompt").
				Return(prompt, nil)
			share := createTestPromptShare()
			shareRepo.On("GetByPromptID", mock.Anything, "prompt-123").
				Return(share, nil)
		},
		expectError: false,
	},
	{
		name:       "Successfully get restricted share with emails",
		userID:     "user-123",
		promptSlug: "test-prompt",
		setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
			prompt := createTestPromptForSharing()
			promptRepo.On("GetBySlugCrossTeam", mock.Anything, "user-123", "test-prompt").
				Return(prompt, nil)
			share := createTestPromptShare()
			share.ShareType = "restricted"
			shareRepo.On("GetByPromptID", mock.Anything, "prompt-123").
				Return(share, nil)
			shareRepo.On("GetAccessEmails", mock.Anything, share.ID).
				Return([]string{"alice@example.com"}, nil)
		},
		expectError: false,
	},
	{
		name:       "Fail when prompt not found",
		userID:     "user-123",
		promptSlug: "nonexistent",
		setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
			promptRepo.On("GetBySlugCrossTeam", mock.Anything, "user-123", "nonexistent").
				Return(nil, errors.New("not found"))
		},
		expectError: true,
		errorMsg:    "prompt not found",
	},
	{
		name:       "Fail when share not found",
		userID:     "user-123",
		promptSlug: "test-prompt",
		setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
			prompt := createTestPromptForSharing()
			promptRepo.On("GetBySlugCrossTeam", mock.Anything, "user-123", "test-prompt").
				Return(prompt, nil)
			shareRepo.On("GetByPromptID", mock.Anything, "prompt-123").
				Return(nil, errors.New("not found"))
		},
		expectError: true,
		errorMsg:    "share not found",
	},
}

func TestPromptShareService_GetShare(t *testing.T) {
	for _, tt := range getShareTests {
		t.Run(tt.name, func(t *testing.T) {
			shareRepo := mocks.NewMockPromptShareRepository(t)
			promptRepo := mocks.NewMockPromptRepository(t)
			tt.setupMocks(shareRepo, promptRepo)

			service := createTestPromptShareService(shareRepo, promptRepo)

			result, err := service.GetShare(tt.userID, tt.promptSlug)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.ShareToken)
				assert.NotEmpty(t, result.ShareURL)
			}
		})
	}
}

var deleteShareTests = []struct {
	name        string
	userID      string
	promptSlug  string
	setupMocks  func(*mocks.MockPromptShareRepository, *mocks.MockPromptRepository)
	expectError bool
	errorMsg    string
}{
	{
		name:       "Successfully delete share",
		userID:     "user-123",
		promptSlug: "test-prompt",
		setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
			prompt := createTestPromptForSharing()
			promptRepo.On("GetBySlugCrossTeam", mock.Anything, "user-123", "test-prompt").
				Return(prompt, nil)
			share := createTestPromptShare()
			shareRepo.On("GetByPromptID", mock.Anything, "prompt-123").
				Return(share, nil)
			shareRepo.On("Delete", mock.Anything, share.ID).Return(nil)
		},
		expectError: false,
	},
	{
		name:       "Fail when prompt not found",
		userID:     "user-123",
		promptSlug: "nonexistent",
		setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
			promptRepo.On("GetBySlugCrossTeam", mock.Anything, "user-123", "nonexistent").
				Return(nil, errors.New("not found"))
		},
		expectError: true,
		errorMsg:    "prompt not found",
	},
	{
		name:       "Fail when share not found",
		userID:     "user-123",
		promptSlug: "test-prompt",
		setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
			prompt := createTestPromptForSharing()
			promptRepo.On("GetBySlugCrossTeam", mock.Anything, "user-123", "test-prompt").
				Return(prompt, nil)
			shareRepo.On("GetByPromptID", mock.Anything, "prompt-123").
				Return(nil, errors.New("not found"))
		},
		expectError: true,
		errorMsg:    "share not found",
	},
}

func TestPromptShareService_DeleteShare(t *testing.T) {
	for _, tt := range deleteShareTests {
		t.Run(tt.name, func(t *testing.T) {
			shareRepo := mocks.NewMockPromptShareRepository(t)
			promptRepo := mocks.NewMockPromptRepository(t)
			tt.setupMocks(shareRepo, promptRepo)

			service := createTestPromptShareService(shareRepo, promptRepo)

			err := service.DeleteShare(tt.userID, tt.promptSlug)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

//nolint:funlen // Test function requires comprehensive test cases
func TestPromptShareService_GetSharedPrompt(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		userEmail   *string
		setupMocks  func(*mocks.MockPromptShareRepository, *mocks.MockPromptRepository)
		expectError bool
		errorMsg    string
	}{
		{
			name:      "Successfully get public shared prompt",
			token:     "abc123xyz789",
			userEmail: nil,
			setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
				share := createTestPromptShare()
				shareRepo.On("GetByToken", mock.Anything, "abc123xyz789").
					Return(share, nil)
				shareRepo.On("IncrementAccessCount", mock.Anything, share.ID).
					Return(nil).Maybe()
				prompt := createTestPromptForSharing()
				promptRepo.On("GetByID", mock.Anything, "", "", "prompt-123").
					Return(prompt, nil)
			},
			expectError: false,
		},
		{
			name:      "Successfully get restricted prompt with valid email",
			token:     "abc123xyz789",
			userEmail: shareStringPtr("alice@example.com"),
			setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
				share := createTestPromptShare()
				share.ShareType = "restricted"
				shareRepo.On("GetByToken", mock.Anything, "abc123xyz789").
					Return(share, nil)
				shareRepo.On("HasAccess", mock.Anything, share.ID, "alice@example.com").
					Return(true, nil)
				shareRepo.On("IncrementAccessCount", mock.Anything, share.ID).
					Return(nil).Maybe()
				prompt := createTestPromptForSharing()
				promptRepo.On("GetByID", mock.Anything, "", "", "prompt-123").
					Return(prompt, nil)
			},
			expectError: false,
		},
		{
			name:      "Fail when share not found",
			token:     "invalid-token",
			userEmail: nil,
			setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
				shareRepo.On("GetByToken", mock.Anything, "invalid-token").
					Return(nil, errors.New("not found"))
			},
			expectError: true,
			errorMsg:    "shared prompt not found",
		},
		{
			name:      "Fail when share is inactive",
			token:     "abc123xyz789",
			userEmail: nil,
			setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
				share := createTestPromptShare()
				share.IsActive = false
				shareRepo.On("GetByToken", mock.Anything, "abc123xyz789").
					Return(share, nil)
			},
			expectError: true,
			errorMsg:    "share has been disabled",
		},
		{
			name:      "Fail when share is expired",
			token:     "abc123xyz789",
			userEmail: nil,
			setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
				share := createTestPromptShare()
				expired := time.Now().Add(-24 * time.Hour)
				share.ExpiresAt = &expired
				shareRepo.On("GetByToken", mock.Anything, "abc123xyz789").
					Return(share, nil)
			},
			expectError: true,
			errorMsg:    "share has expired",
		},
		{
			name:      "Fail when restricted share accessed without authentication",
			token:     "abc123xyz789",
			userEmail: nil,
			setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
				share := createTestPromptShare()
				share.ShareType = "restricted"
				shareRepo.On("GetByToken", mock.Anything, "abc123xyz789").
					Return(share, nil)
			},
			expectError: true,
			errorMsg:    "authentication required",
		},
		{
			name:      "Fail when restricted share accessed by unauthorized email",
			token:     "abc123xyz789",
			userEmail: shareStringPtr("unauthorized@example.com"),
			setupMocks: func(shareRepo *mocks.MockPromptShareRepository, promptRepo *mocks.MockPromptRepository) {
				share := createTestPromptShare()
				share.ShareType = "restricted"
				shareRepo.On("GetByToken", mock.Anything, "abc123xyz789").
					Return(share, nil)
				shareRepo.On("HasAccess", mock.Anything, share.ID, "unauthorized@example.com").
					Return(false, nil)
			},
			expectError: true,
			errorMsg:    "access denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shareRepo := mocks.NewMockPromptShareRepository(t)
			promptRepo := mocks.NewMockPromptRepository(t)
			tt.setupMocks(shareRepo, promptRepo)

			service := createTestPromptShareService(shareRepo, promptRepo)

			result, err := service.GetSharedPrompt(tt.token, tt.userEmail)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Prompt.ID)
				assert.NotEmpty(t, result.ShareType)
				assert.NotEmpty(t, result.RenderedBody)
			}
		})
	}
}

func shareStringPtr(s string) *string {
	return &s
}
