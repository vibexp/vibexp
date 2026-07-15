package services

import (
	"fmt"
	"testing"
	"time"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/pkg/events"
	event_mocks "github.com/vibexp/vibexp/pkg/events/mocks"
)

func createTestPromptService(
	repo repositories.PromptRepository,
	projectRepo repositories.ProjectRepository,
) *PromptService {
	// Create a real mock that accepts nil since we don't have access to *testing.T here
	mockRefRepo := &mocks.MockPromptReferenceRepository{}
	mockRefRepo.On("DeleteByPromptID", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockRefRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockRefRepo.On("HasDependents", mock.Anything, mock.Anything).Return(false, nil).Maybe()
	mockRefRepo.On("GetPromptsUsingPrompt", mock.Anything, mock.Anything, mock.Anything).
		Return([]models.PromptDependencyInfo{}, nil).Maybe()
	logger := func() *slog.Logger { l, _ := logtest.New(); return l }()
	return NewPromptService(repo, mockRefRepo, nil, projectRepo, nil, allowAllAuthz{}, nil, logger, nil)
}

func createTestProjectRepo(t *testing.T) *mocks.MockProjectRepository {
	mockProjectRepo := mocks.NewMockProjectRepository(t)
	// Set up default project validation for test user
	mockProjectRepo.On("GetByID", mock.AnythingOfType("context.backgroundCtx"), "user-123", "project-123").
		Return(&models.Project{
			ID:     "project-123",
			UserID: "user-123",
			Name:   "Test Project",
			Slug:   "test-project",
		}, nil).Maybe()
	return mockProjectRepo
}

func createTestPrompt() *models.Prompt {
	now := time.Now()
	return &models.Prompt{
		ID:          "prompt-123",
		Name:        "Test Prompt",
		Slug:        "test-prompt",
		Description: "A test prompt for unit testing",
		Body:        "Hello {{name}}, this is a test prompt with placeholder.",
		UserID:      "user-123",
		ProjectID:   "project-123",
		Status:      "published",
		MCPExpose:   true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func createTestCreatePromptRequest() *models.CreatePromptRequest {
	return &models.CreatePromptRequest{
		Name:        "Test Prompt",
		Slug:        "test-prompt",
		Description: "A test prompt for unit testing",
		Body:        "Hello {{name}}, this is a test prompt.",
		Status:      "published",
		ProjectID:   "project-123",
	}
}

func createTestUpdatePromptRequest() *models.UpdatePromptRequest {
	name := "Updated Prompt"
	body := "Updated body with {{updated_placeholder}}"
	status := "draft"
	return &models.UpdatePromptRequest{
		Name:   &name,
		Body:   &body,
		Status: &status,
	}
}

func TestNewPromptService(t *testing.T) {
	mockRepo := mocks.NewMockPromptRepository(t)
	mockRefRepo := mocks.NewMockPromptReferenceRepository(t)
	mockProjectRepo := mocks.NewMockProjectRepository(t)
	logger, _ := logtest.New()
	service := NewPromptService(mockRepo, mockRefRepo, nil, mockProjectRepo, nil, allowAllAuthz{}, nil, logger, nil)

	assert.NotNil(t, service)
	assert.Equal(t, mockRepo, service.repo)
	assert.Equal(t, mockRefRepo, service.refRepo)
	assert.Equal(t, mockProjectRepo, service.projectRepo)
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestPromptService_CreatePrompt(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		request     *models.CreatePromptRequest
		setup       func(*mocks.MockPromptRepository)
		expectError bool
		errorMsg    string
	}{
		{
			name:    "Successful prompt creation",
			userID:  "user-123",
			request: createTestCreatePromptRequest(),
			setup: func(mockRepo *mocks.MockPromptRepository) {
				mockRepo.On("Create", mock.AnythingOfType("context.backgroundCtx"),
					mock.MatchedBy(func(prompt *models.Prompt) bool {
						return prompt.Name == "Test Prompt" &&
							prompt.UserID == "user-123" && prompt.Status == "published" &&
							prompt.ProjectID == "project-123"
					})).Return(nil).Run(func(args mock.Arguments) {
					prompt := args.Get(1).(*models.Prompt)
					prompt.ID = "prompt-123"
					prompt.CreatedAt = time.Now()
					prompt.UpdatedAt = time.Now()
				})
			},
			expectError: false,
		},
		{
			name:   "Default status to draft when empty",
			userID: "user-123",
			request: &models.CreatePromptRequest{
				Name:        "Test Prompt",
				Slug:        "test-prompt",
				Description: "A test prompt",
				Body:        "Test body",
				Status:      "", // Empty status should default to "draft"
				ProjectID:   "project-123",
			},
			setup: func(mockRepo *mocks.MockPromptRepository) {
				mockRepo.On("Create", mock.AnythingOfType("context.backgroundCtx"),
					mock.MatchedBy(func(prompt *models.Prompt) bool {
						return prompt.Status == "draft"
					})).Return(nil).Run(func(args mock.Arguments) {
					prompt := args.Get(1).(*models.Prompt)
					prompt.ID = "prompt-123"
					prompt.CreatedAt = time.Now()
					prompt.UpdatedAt = time.Now()
				})
			},
			expectError: false,
		},
		{
			name:    "Repository error",
			userID:  "user-123",
			request: createTestCreatePromptRequest(),
			setup: func(mockRepo *mocks.MockPromptRepository) {
				mockRepo.On("Create", mock.AnythingOfType("context.backgroundCtx"), mock.Anything).
					Return(fmt.Errorf("prompt with slug 'test-prompt' already exists for this user"))
			},
			expectError: true,
			errorMsg:    "prompt with slug 'test-prompt' already exists for this user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockPromptRepository(t)
			mockProjectRepo := createTestProjectRepo(t)
			service := createTestPromptService(mockRepo, mockProjectRepo)
			tt.setup(mockRepo)

			prompt, err := service.CreatePrompt(tt.userID, "team-123", tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, prompt)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, prompt)
				assert.Equal(t, tt.userID, prompt.UserID)
				assert.Equal(t, tt.request.Name, prompt.Name)
				assert.Equal(t, tt.request.Slug, prompt.Slug)
				assert.Equal(t, tt.request.ProjectID, prompt.ProjectID)
			}
		})
	}
}

func TestPromptService_GetPrompt(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		promptID    string
		setup       func(*mocks.MockPromptRepository)
		expectError bool
		errorMsg    string
	}{
		{
			name:     "Successful prompt retrieval",
			userID:   "user-123",
			promptID: "prompt-123",
			setup: func(mockRepo *mocks.MockPromptRepository) {
				prompt := createTestPrompt()
				mockRepo.On("GetByID", mock.AnythingOfType("context.backgroundCtx"), "user-123", mock.Anything, "prompt-123").
					Return(prompt, nil)
			},
			expectError: false,
		},
		{
			name:     "Prompt not found",
			userID:   "user-123",
			promptID: "non-existent",
			setup: func(mockRepo *mocks.MockPromptRepository) {
				mockRepo.On("GetByID", mock.AnythingOfType("context.backgroundCtx"), "user-123", mock.Anything, "non-existent").
					Return(nil, repositories.ErrPromptNotFound)
			},
			expectError: true,
			errorMsg:    "prompt not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockPromptRepository(t)
			service := createTestPromptService(mockRepo, nil)
			tt.setup(mockRepo)

			prompt, err := service.GetPrompt(tt.userID, "team-123", tt.promptID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, prompt)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, prompt)
				assert.Equal(t, tt.promptID, prompt.ID)
				assert.Equal(t, tt.userID, prompt.UserID)
			}
		})
	}
}

func TestPromptService_GetPromptBySlug(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		slug        string
		setup       func(*mocks.MockPromptRepository)
		expectError bool
		errorMsg    string
	}{
		{
			name:   "Successful prompt retrieval by slug",
			userID: "user-123",
			slug:   "test-prompt",
			setup: func(mockRepo *mocks.MockPromptRepository) {
				prompt := createTestPrompt()
				mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
					Return(prompt, nil)
			},
			expectError: false,
		},
		{
			name:   "Prompt not found by slug",
			userID: "user-123",
			slug:   "non-existent",
			setup: func(mockRepo *mocks.MockPromptRepository) {
				mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "non-existent").
					Return(nil, repositories.ErrPromptNotFound)
			},
			expectError: true,
			errorMsg:    "prompt not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockPromptRepository(t)
			service := createTestPromptService(mockRepo, nil)
			tt.setup(mockRepo)

			prompt, err := service.GetPromptBySlug(tt.userID, "team-123", tt.slug)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, prompt)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, prompt)
				assert.Equal(t, tt.slug, prompt.Slug)
				assert.Equal(t, tt.userID, prompt.UserID)
			}
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestPromptService_ListPrompts(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		filters     PromptFilters
		setup       func(*mocks.MockPromptRepository)
		expectError bool
		expectedLen int
	}{
		{
			name:   "Successful prompt list with default pagination",
			userID: "user-123",
			filters: PromptFilters{
				Page:  1,
				Limit: 20,
			},
			setup: func(mockRepo *mocks.MockPromptRepository) {
				prompts := []models.Prompt{
					{ID: "prompt-1", Name: "Prompt 1", UserID: "user-123"},
					{ID: "prompt-2", Name: "Prompt 2", UserID: "user-123"},
				}
				mockRepo.On("List", mock.AnythingOfType("context.backgroundCtx"), "user-123", repositories.PromptFilters{
					Status: "",
					Search: "",
					Page:   1,
					Limit:  20,
				}).Return(prompts, 2, nil)
			},
			expectError: false,
			expectedLen: 2,
		},
		{
			name:   "List with status filter",
			userID: "user-123",
			filters: PromptFilters{
				Status: "published",
				Page:   1,
				Limit:  20,
			},
			setup: func(mockRepo *mocks.MockPromptRepository) {
				prompts := []models.Prompt{
					{ID: "prompt-1", Name: "Prompt 1", UserID: "user-123", Status: "published"},
				}
				mockRepo.On("List", mock.AnythingOfType("context.backgroundCtx"), "user-123", repositories.PromptFilters{
					Status: "published",
					Search: "",
					Page:   1,
					Limit:  20,
				}).Return(prompts, 1, nil)
			},
			expectError: false,
			expectedLen: 1,
		},
		{
			name:   "Repository error",
			userID: "user-123",
			filters: PromptFilters{
				Page:  1,
				Limit: 20,
			},
			setup: func(mockRepo *mocks.MockPromptRepository) {
				mockRepo.On("List", mock.AnythingOfType("context.backgroundCtx"), "user-123", mock.Anything).
					Return(nil, 0, fmt.Errorf("database error"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockPromptRepository(t)
			service := createTestPromptService(mockRepo, nil)
			tt.setup(mockRepo)

			response, err := service.ListPrompts(tt.userID, tt.filters)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.Len(t, response.Prompts, tt.expectedLen)
				assert.Equal(t, tt.expectedLen, response.TotalCount)
			}
		})
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestPromptService_UpdatePrompt(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		promptID    string
		request     *models.UpdatePromptRequest
		setup       func(*mocks.MockPromptRepository)
		expectError bool
		errorMsg    string
	}{
		{
			name:     "Successful prompt update",
			userID:   "user-123",
			promptID: "prompt-123",
			request:  createTestUpdatePromptRequest(),
			setup: func(mockRepo *mocks.MockPromptRepository) {
				existingPrompt := createTestPrompt()
				mockRepo.On("GetByID", mock.AnythingOfType("context.backgroundCtx"), "user-123", mock.Anything, "prompt-123").
					Return(existingPrompt, nil)
				mockRepo.On("Update", mock.AnythingOfType("context.backgroundCtx"),
					mock.MatchedBy(func(prompt *models.Prompt) bool {
						return prompt.Name == "Updated Prompt" && prompt.Status == "draft"
					})).Return(nil)
			},
			expectError: false,
		},
		{
			name:     "No updates provided - return existing prompt",
			userID:   "user-123",
			promptID: "prompt-123",
			request:  &models.UpdatePromptRequest{}, // Empty request
			setup: func(mockRepo *mocks.MockPromptRepository) {
				existingPrompt := createTestPrompt()
				mockRepo.On("GetByID", mock.AnythingOfType("context.backgroundCtx"), "user-123", mock.Anything, "prompt-123").
					Return(existingPrompt, nil)
			},
			expectError: false,
		},
		{
			name:     "Prompt not found",
			userID:   "user-123",
			promptID: "non-existent",
			request:  createTestUpdatePromptRequest(),
			setup: func(mockRepo *mocks.MockPromptRepository) {
				mockRepo.On("GetByID", mock.AnythingOfType("context.backgroundCtx"), "user-123", mock.Anything, "non-existent").
					Return(nil, repositories.ErrPromptNotFound)
			},
			expectError: true,
			errorMsg:    "prompt not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockPromptRepository(t)
			service := createTestPromptService(mockRepo, nil)
			tt.setup(mockRepo)

			prompt, err := service.UpdatePrompt(tt.userID, "team-123", tt.promptID, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, prompt)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, prompt)
				assert.Equal(t, tt.promptID, prompt.ID)
				assert.Equal(t, tt.userID, prompt.UserID)
			}
		})
	}
}

func TestPromptService_UpdatePromptBySlug(t *testing.T) {
	t.Run("Successful update by slug", func(t *testing.T) {
		mockRepo := mocks.NewMockPromptRepository(t)
		service := createTestPromptService(mockRepo, nil)

		existingPrompt := createTestPrompt()
		mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
			Return(existingPrompt, nil)
		mockRepo.On("GetByID", mock.AnythingOfType("context.backgroundCtx"), "user-123", mock.Anything, "prompt-123").
			Return(existingPrompt, nil)
		mockRepo.On("Update", mock.AnythingOfType("context.backgroundCtx"), mock.MatchedBy(func(prompt *models.Prompt) bool {
			return prompt.Name == "Updated Prompt"
		})).Return(nil)

		request := createTestUpdatePromptRequest()
		prompt, err := service.UpdatePromptBySlug("user-123", "team-123", "test-prompt", request)

		assert.NoError(t, err)
		assert.NotNil(t, prompt)
		assert.Equal(t, "prompt-123", prompt.ID)
	})
}

func TestPromptService_DeletePrompt(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		promptID    string
		setup       func(*mocks.MockPromptRepository)
		expectError bool
		errorMsg    string
	}{
		{
			name:     "Successful prompt deletion",
			userID:   "user-123",
			promptID: "prompt-123",
			setup: func(mockRepo *mocks.MockPromptRepository) {
				// Delete fetches first to learn the prompt's owner: members may
				// delete only their own, Admin+ may delete anyone's.
				mockRepo.On("GetByID", mock.Anything, "user-123", mock.Anything, "prompt-123").
					Return(createTestPrompt(), nil)
				mockRepo.On("Delete", mock.AnythingOfType("context.backgroundCtx"), "user-123", mock.Anything, "prompt-123").
					Return(nil)
			},
			expectError: false,
		},
		{
			name:     "Prompt not found",
			userID:   "user-123",
			promptID: "non-existent",
			setup: func(mockRepo *mocks.MockPromptRepository) {
				// The owner-fetch is now what surfaces a missing prompt.
				mockRepo.On("GetByID", mock.Anything, "user-123", mock.Anything, "non-existent").
					Return(nil, repositories.ErrPromptNotFound)
			},
			expectError: true,
			errorMsg:    "prompt not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockPromptRepository(t)
			service := createTestPromptService(mockRepo, nil)
			tt.setup(mockRepo)

			err := service.DeletePrompt(tt.userID, "team-123", tt.promptID)

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

func TestPromptService_DeletePromptBySlug(t *testing.T) {
	t.Run("Successful delete by slug", func(t *testing.T) {
		mockRepo := mocks.NewMockPromptRepository(t)
		service := createTestPromptService(mockRepo, nil)

		existingPrompt := createTestPrompt()
		mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
			Return(existingPrompt, nil)
		// DeletePrompt re-fetches by ID to learn the owner for the own-vs-any check.
		mockRepo.On("GetByID", mock.Anything, "user-123", mock.Anything, "prompt-123").
			Return(existingPrompt, nil)
		mockRepo.On("Delete", mock.AnythingOfType("context.backgroundCtx"), "user-123", mock.Anything, "prompt-123").
			Return(nil)

		err := service.DeletePromptBySlug("user-123", "team-123", "test-prompt")

		assert.NoError(t, err)
	})
}

// Test helper to verify interface compliance
func TestPromptService_ImplementsInterface(t *testing.T) {
	mockRepo := mocks.NewMockPromptRepository(t)
	service := createTestPromptService(mockRepo, nil)

	// Verify that PromptService implements PromptServiceInterface
	var _ PromptServiceInterface = service
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

// Simple tests for the utility functions since they don't depend on repository
//
//nolint:funlen // Test function requires comprehensive setup and assertions
func TestPromptService_RenderPrompt(t *testing.T) {
	t.Run("Simple placeholder rendering", func(t *testing.T) {
		mockRepo := mocks.NewMockPromptRepository(t)
		service := createTestPromptService(mockRepo, nil)

		prompt := &models.Prompt{
			ID:     "prompt-123",
			Body:   "Hello {{name}}, you are {{age}} years old.",
			UserID: "user-123",
		}
		mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
			Return(prompt, nil)

		placeholders := map[string]string{
			"name": "John",
			"age":  "30",
		}

		response, err := service.RenderPrompt("user-123", "team-123", "test-prompt", placeholders)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, "Hello John, you are 30 years old.", response.RenderedBody)
		assert.Empty(t, response.ReferencesUsed)
	})

	t.Run("Missing placeholder - keeps placeholder as-is", func(t *testing.T) {
		mockRepo := mocks.NewMockPromptRepository(t)
		service := createTestPromptService(mockRepo, nil)

		prompt := &models.Prompt{
			ID:     "prompt-123",
			Body:   "Hello {{name}}, you are {{age}} years old.",
			UserID: "user-123",
		}
		mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
			Return(prompt, nil)

		placeholders := map[string]string{
			"name": "John",
			// Missing "age" placeholder
		}

		response, err := service.RenderPrompt("user-123", "team-123", "test-prompt", placeholders)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		// The rendered body should have "name" replaced but keep "{{age}}" as-is
		assert.Equal(t, "Hello John, you are {{age}} years old.", response.RenderedBody)
		assert.Empty(t, response.ReferencesUsed)
	})

	t.Run("No placeholders provided - keeps all placeholders as-is", func(t *testing.T) {
		mockRepo := mocks.NewMockPromptRepository(t)
		service := createTestPromptService(mockRepo, nil)

		prompt := &models.Prompt{
			ID:     "prompt-123",
			Body:   "Hello {{name}}, you are {{age}} years old.",
			UserID: "user-123",
		}
		mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
			Return(prompt, nil)

		// Empty placeholders map
		placeholders := map[string]string{}

		response, err := service.RenderPrompt("user-123", "team-123", "test-prompt", placeholders)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		// The rendered body should keep all placeholders as-is
		assert.Equal(t, "Hello {{name}}, you are {{age}} years old.", response.RenderedBody)
		assert.Empty(t, response.ReferencesUsed)
	})

	t.Run("Render with @reference and missing placeholders", func(t *testing.T) {
		mockRepo := mocks.NewMockPromptRepository(t)
		service := createTestPromptService(mockRepo, nil)

		basePrompt := &models.Prompt{
			ID:     "base-prompt-123",
			Slug:   "base-prompt",
			Body:   "Base content with {{base_var}}.",
			UserID: "user-123",
		}

		mainPrompt := &models.Prompt{
			ID:     "prompt-123",
			Body:   "Main: {{main_var}} @base-prompt End.",
			UserID: "user-123",
		}

		mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
			Return(mainPrompt, nil)
		mockRepo.On("GetBySlugCrossTeam", mock.AnythingOfType("context.backgroundCtx"), "user-123", "base-prompt").
			Return(basePrompt, nil)

		// Only provide one placeholder value
		placeholders := map[string]string{
			"main_var": "MainValue",
			// Missing "base_var" placeholder
		}

		response, err := service.RenderPrompt("user-123", "team-123", "test-prompt", placeholders)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		// The rendered body should have @reference resolved and main_var replaced,
		// but keep {{base_var}} as-is
		assert.Equal(t, "Main: MainValue Base content with {{base_var}}. End.", response.RenderedBody)
		assert.Equal(t, []string{"base-prompt"}, response.ReferencesUsed)
	})

	t.Run("Render with empty placeholder values (not nil, but empty string)", func(t *testing.T) {
		mockRepo := mocks.NewMockPromptRepository(t)
		service := createTestPromptService(mockRepo, nil)

		prompt := &models.Prompt{
			ID:     "prompt-123",
			Body:   "Hello {{name}}, you are {{age}} years old.",
			UserID: "user-123",
		}
		mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
			Return(prompt, nil)

		placeholders := map[string]string{
			"name": "", // Empty string is a valid value
			"age":  "30",
		}

		response, err := service.RenderPrompt("user-123", "team-123", "test-prompt", placeholders)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		// Empty string should replace the placeholder (it's a valid value)
		assert.Equal(t, "Hello , you are 30 years old.", response.RenderedBody)
		assert.Empty(t, response.ReferencesUsed)
	})

	t.Run("Render with escaped @@ sequence", func(t *testing.T) {
		mockRepo := mocks.NewMockPromptRepository(t)
		service := createTestPromptService(mockRepo, nil)

		prompt := &models.Prompt{
			ID:     "prompt-123",
			Body:   "Contact me at user@@example.com or @@mention me",
			UserID: "user-123",
		}
		mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
			Return(prompt, nil)

		response, err := service.RenderPrompt("user-123", "team-123", "test-prompt", map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, response)
		// @@ should be rendered as single @
		assert.Equal(t, "Contact me at user@example.com or @mention me", response.RenderedBody)
		assert.Empty(t, response.ReferencesUsed)
		assert.Empty(t, response.Warnings)
	})

	t.Run("Render with non-existent @reference", func(t *testing.T) {
		mockRepo := mocks.NewMockPromptRepository(t)
		service := createTestPromptService(mockRepo, nil)

		prompt := &models.Prompt{
			ID:     "prompt-123",
			Body:   "This references @nonexistent prompt",
			UserID: "user-123",
		}
		mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
			Return(prompt, nil)
		mockRepo.On("GetBySlugCrossTeam", mock.AnythingOfType("context.backgroundCtx"), "user-123", "nonexistent").
			Return(nil, repositories.ErrPromptNotFound)

		response, err := service.RenderPrompt("user-123", "team-123", "test-prompt", map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, response)
		// Non-existent reference should be kept as-is
		assert.Equal(t, "This references @nonexistent prompt", response.RenderedBody)
		assert.Empty(t, response.ReferencesUsed)
		// Should have a warning about the missing reference
		assert.Len(t, response.Warnings, 1)
		assert.Equal(t, "Reference not found: @nonexistent", response.Warnings[0])
	})

	t.Run("Render with mixed escaped and real references", func(t *testing.T) {
		mockRepo := mocks.NewMockPromptRepository(t)
		service := createTestPromptService(mockRepo, nil)

		basePrompt := &models.Prompt{
			ID:     "base-prompt-123",
			Slug:   "base",
			Body:   "Base content",
			UserID: "user-123",
		}

		prompt := &models.Prompt{
			ID:     "prompt-123",
			Body:   "Email: user@@example.com, Reference: @base, Missing: @missing",
			UserID: "user-123",
		}
		mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
			Return(prompt, nil)
		mockRepo.On("GetBySlugCrossTeam", mock.AnythingOfType("context.backgroundCtx"), "user-123", "base").
			Return(basePrompt, nil)
		mockRepo.On("GetBySlugCrossTeam", mock.AnythingOfType("context.backgroundCtx"), "user-123", "missing").
			Return(nil, repositories.ErrPromptNotFound)

		response, err := service.RenderPrompt("user-123", "team-123", "test-prompt", map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, response)
		// @@ becomes @, @base is resolved, @missing stays
		assert.Equal(t, "Email: user@example.com, Reference: Base content, Missing: @missing", response.RenderedBody)
		assert.Equal(t, []string{"base"}, response.ReferencesUsed)
		assert.Len(t, response.Warnings, 1)
		assert.Equal(t, "Reference not found: @missing", response.Warnings[0])
	})

	t.Run("Render with nested references and escaped sequences", func(t *testing.T) {
		mockRepo := mocks.NewMockPromptRepository(t)
		service := createTestPromptService(mockRepo, nil)

		nestedPrompt := &models.Prompt{
			ID:     "nested-123",
			Slug:   "nested",
			Body:   "Nested with email: admin@@company.com",
			UserID: "user-123",
		}

		basePrompt := &models.Prompt{
			ID:     "base-123",
			Slug:   "base",
			Body:   "Base @nested end",
			UserID: "user-123",
		}

		prompt := &models.Prompt{
			ID:     "prompt-123",
			Body:   "Start @@symbol @base finish",
			UserID: "user-123",
		}

		mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
			Return(prompt, nil)
		mockRepo.On("GetBySlugCrossTeam", mock.AnythingOfType("context.backgroundCtx"), "user-123", "base").
			Return(basePrompt, nil)
		mockRepo.On("GetBySlugCrossTeam", mock.AnythingOfType("context.backgroundCtx"), "user-123", "nested").
			Return(nestedPrompt, nil)

		response, err := service.RenderPrompt("user-123", "team-123", "test-prompt", map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, response)
		// All @@ should become @, all references resolved
		assert.Equal(t, "Start @symbol Base Nested with email: admin@company.com end finish", response.RenderedBody)
		assert.Equal(t, []string{"base", "nested"}, response.ReferencesUsed)
		assert.Empty(t, response.Warnings)
	})
}

func TestPromptService_GetPromptPlaceholders(t *testing.T) {
	t.Run("Simple placeholders extraction", func(t *testing.T) {
		mockRepo := mocks.NewMockPromptRepository(t)
		service := createTestPromptService(mockRepo, nil)

		prompt := &models.Prompt{
			ID:     "prompt-123",
			Body:   "Hello {{name}}, you are {{age}} years old. Welcome {{name}}!",
			UserID: "user-123",
		}
		mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
			Return(prompt, nil)

		placeholders, err := service.GetPromptPlaceholders("user-123", "team-123", "test-prompt")

		assert.NoError(t, err)
		assert.ElementsMatch(t, []string{"name", "age"}, placeholders)
	})
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestPromptService_PublishesPromptEvents(t *testing.T) {
	tests := []struct {
		name             string
		setupMocks       func(*mocks.MockPromptRepository, *event_mocks.MockEventPublisher)
		executeAction    func(*PromptService) error
		expectEventCalls int
		eventType        string
	}{
		{
			name: "publishes prompt.created event when creating prompt",
			setupMocks: func(mockRepo *mocks.MockPromptRepository, mockEventManager *event_mocks.MockEventPublisher) {
				mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Prompt")).
					Return(nil).Run(func(args mock.Arguments) {
					prompt := args.Get(1).(*models.Prompt)
					prompt.ID = "prompt-new-123"
				})

				// Expect event to be published exactly once
				mockEventManager.On("Publish", mock.Anything, mock.MatchedBy(func(event events.Event) bool {
					return event.Type() == events.EventTypePromptCreated
				})).Return(nil).Once()
			},
			executeAction: func(service *PromptService) error {
				req := &models.CreatePromptRequest{
					Name:        "Test Prompt",
					Slug:        "test-prompt",
					Description: "Test Description",
					Body:        "Test Body",
					Status:      "draft",
					ProjectID:   "project-123",
				}
				_, err := service.CreatePrompt("user-123", "team-123", req)
				return err
			},
			expectEventCalls: 1,
			eventType:        events.EventTypePromptCreated,
		},
		{
			name: "publishes prompt.updated event when updating prompt",
			setupMocks: func(mockRepo *mocks.MockPromptRepository, mockEventManager *event_mocks.MockEventPublisher) {
				existingPrompt := createTestPrompt()
				mockRepo.On("GetByID", mock.Anything, "user-123", mock.Anything, "prompt-123").
					Return(existingPrompt, nil)

				mockRepo.On("Update", mock.Anything, mock.AnythingOfType("*models.Prompt")).
					Return(nil)

				// Expect event to be published exactly once
				mockEventManager.On("Publish", mock.Anything, mock.MatchedBy(func(event events.Event) bool {
					return event.Type() == events.EventTypePromptUpdated
				})).Return(nil).Once()
			},
			executeAction: func(service *PromptService) error {
				name := "Updated Name"
				req := &models.UpdatePromptRequest{
					Name: &name,
				}
				_, err := service.UpdatePrompt("user-123", "team-123", "prompt-123", req)
				return err
			},
			expectEventCalls: 1,
			eventType:        events.EventTypePromptUpdated,
		},
		{
			name: "does not publish prompt.updated event when no updates provided",
			setupMocks: func(mockRepo *mocks.MockPromptRepository, mockEventManager *event_mocks.MockEventPublisher) {
				existingPrompt := createTestPrompt()
				mockRepo.On("GetByID", mock.Anything, "user-123", mock.Anything, "prompt-123").
					Return(existingPrompt, nil)

				// No event should be published
			},
			executeAction: func(service *PromptService) error {
				req := &models.UpdatePromptRequest{} // No updates
				_, err := service.UpdatePrompt("user-123", "team-123", "prompt-123", req)
				return err
			},
			expectEventCalls: 0,
			eventType:        events.EventTypePromptUpdated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mocks.MockPromptRepository{}
			mockRefRepo := &mocks.MockPromptReferenceRepository{}
			mockProjectRepo := &mocks.MockProjectRepository{}
			mockEventManager := &event_mocks.MockEventPublisher{}

			mockRefRepo.On("DeleteByPromptID", mock.Anything, mock.Anything).Return(nil).Maybe()
			mockRefRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()

			// Set up default project validation for test user
			mockProjectRepo.On("GetByID", mock.Anything, "user-123", "project-123").
				Return(&models.Project{
					ID:     "project-123",
					UserID: "user-123",
					Name:   "Test Project",
					Slug:   "test-project",
				}, nil).Maybe()

			service := NewPromptService(
				mockRepo, mockRefRepo, nil, mockProjectRepo, nil, allowAllAuthz{}, mockEventManager,
				func() *slog.Logger { l, _ := logtest.New(); return l }(), nil,
			)

			tt.setupMocks(mockRepo, mockEventManager)

			err := tt.executeAction(service)
			assert.NoError(t, err)

			mockRepo.AssertExpectations(t)
			mockEventManager.AssertExpectations(t)

			// Verify the event was published the expected number of times
			if tt.expectEventCalls > 0 {
				mockEventManager.AssertNumberOfCalls(t, "Publish", tt.expectEventCalls)
			} else {
				mockEventManager.AssertNotCalled(t, "Publish")
			}
			//nolint:funlen // Test function requires comprehensive setup and assertions
		})
		//nolint:funlen // Test function requires comprehensive setup and assertions
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

// Test that verifies rendered body (with @references resolved) is sent in events
//
//nolint:funlen // Test function requires comprehensive setup and assertions
func TestPromptService_PublishesRenderedBodyInEvents(t *testing.T) {
	t.Run("publishes rendered body with @references resolved in prompt.created event", func(t *testing.T) {
		mockRepo := &mocks.MockPromptRepository{}
		mockProjectRepo := &mocks.MockProjectRepository{}
		mockEventManager := &event_mocks.MockEventPublisher{}

		// Mock the referenced prompt that will be included
		referencedPrompt := &models.Prompt{
			ID:     "ref-prompt-123",
			Slug:   "base-prompt",
			Body:   "This is the base prompt content.",
			UserID: "user-123",
		}

		// Mock getting the referenced prompt during rendering
		mockRepo.On("GetBySlugCrossTeam", mock.Anything, "user-123", "base-prompt").
			Return(referencedPrompt, nil)

		// Mock creating the main prompt
		mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Prompt")).
			Return(nil).Run(func(args mock.Arguments) {
			prompt := args.Get(1).(*models.Prompt)
			prompt.ID = "prompt-new-123"
		})

		// Mock project validation
		mockProjectRepo.On("GetByID", mock.Anything, "user-123", "project-123").
			Return(&models.Project{
				ID:     "project-123",
				UserID: "user-123",
				Name:   "Test Project",
				Slug:   "test-project",
			}, nil)

		// Capture the event payload to verify rendered body
		var capturedEvent events.Event
		mockEventManager.On("Publish", mock.Anything, mock.MatchedBy(func(event events.Event) bool {
			if event.Type() == events.EventTypePromptCreated {
				capturedEvent = event
				return true
			}
			return false
		})).Return(nil).Once()

		mockRefRepo := &mocks.MockPromptReferenceRepository{}
		mockRefRepo.On("DeleteByPromptID", mock.Anything, mock.Anything).Return(nil).Maybe()
		mockRefRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()

		service := NewPromptService(
			mockRepo, mockRefRepo, nil, mockProjectRepo, nil, allowAllAuthz{}, mockEventManager,
			func() *slog.Logger { l, _ := logtest.New(); return l }(), nil)

		// Create a prompt with @reference
		req := &models.CreatePromptRequest{
			Name:        "Test Prompt",
			Slug:        "test-prompt",
			Description: "Test with reference",
			Body:        "Instructions: @base-prompt Use this for testing.",
			Status:      "published",
			ProjectID:   "project-123",
		}

		_, err := service.CreatePrompt("user-123", "team-123", req)
		assert.NoError(t, err)

		// Verify event was published
		mockEventManager.AssertExpectations(t)
		assert.NotNil(t, capturedEvent)

		// Extract and verify the body in the event payload
		payload, ok := capturedEvent.Payload().(*events.PromptCreatedPayload)
		assert.True(t, ok, "Event payload should be PromptCreatedPayload")

		// The body should be rendered (reference resolved)
		expectedRenderedBody := "Instructions: This is the base prompt content. Use this for testing."
		assert.Equal(t, expectedRenderedBody, payload.Body,
			"Event should contain rendered body with @references resolved, not raw body")

		// Verify it's NOT the raw body with unresolved @reference
		assert.NotContains(t, payload.Body, "@base-prompt",
			"Event body should not contain unresolved @references")

		// The description must be carried so the embedding pipeline can build the
		// per-chunk context header (issue #173).
		assert.Equal(t, "Test with reference", payload.Description,
			"Event should carry the prompt description for the embedding context header")
	})

	t.Run("publishes rendered body with @references resolved in prompt.updated event", func(t *testing.T) {
		mockRepo := &mocks.MockPromptRepository{}
		mockEventManager := &event_mocks.MockEventPublisher{}

		// Mock the existing prompt
		existingPrompt := &models.Prompt{
			ID:          "prompt-123",
			Name:        "Test Prompt",
			Slug:        "test-prompt",
			Description: "Test Description",
			Body:        "Old body",
			UserID:      "user-123",
			Status:      "published",
		}

		// Mock the referenced prompt that will be included
		referencedPrompt := &models.Prompt{
			ID:     "ref-prompt-456",
			Slug:   "footer-prompt",
			Body:   "This is the footer content.",
			UserID: "user-123",
		}

		mockRepo.On("GetByID", mock.Anything, "user-123", mock.Anything, "prompt-123").
			Return(existingPrompt, nil)

		// Mock getting the referenced prompt during rendering
		mockRepo.On("GetBySlugCrossTeam", mock.Anything, "user-123", "footer-prompt").
			Return(referencedPrompt, nil)

		mockRepo.On("Update", mock.Anything, mock.AnythingOfType("*models.Prompt")).
			Return(nil)

		// Capture the event payload to verify rendered body
		var capturedEvent events.Event
		mockEventManager.On("Publish", mock.Anything, mock.MatchedBy(func(event events.Event) bool {
			if event.Type() == events.EventTypePromptUpdated {
				capturedEvent = event
				return true
			}
			return false
		})).Return(nil).Once()

		mockRefRepo := &mocks.MockPromptReferenceRepository{}
		mockRefRepo.On("DeleteByPromptID", mock.Anything, mock.Anything).Return(nil).Maybe()
		mockRefRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()

		service := NewPromptService(
			mockRepo, mockRefRepo, nil, nil, nil, allowAllAuthz{}, mockEventManager,
			func() *slog.Logger { l, _ := logtest.New(); return l }(), nil)

		// Update the prompt with a body containing @reference
		newBody := "Main content here. @footer-prompt"
		req := &models.UpdatePromptRequest{
			Body: &newBody,
		}

		_, err := service.UpdatePrompt("user-123", "team-123", "prompt-123", req)
		assert.NoError(t, err)

		// Verify event was published
		mockEventManager.AssertExpectations(t)
		assert.NotNil(t, capturedEvent)

		// Extract and verify the body in the event payload
		payload, ok := capturedEvent.Payload().(*events.PromptUpdatedPayload)
		assert.True(t, ok, "Event payload should be PromptUpdatedPayload")

		// The body should be rendered (reference resolved)
		expectedRenderedBody := "Main content here. This is the footer content."
		assert.Equal(t, expectedRenderedBody, payload.Body,
			"Event should contain rendered body with @references resolved, not raw body")

		// Verify it's NOT the raw body with unresolved @reference
		assert.NotContains(t, payload.Body, "@footer-prompt",
			"Event body should not contain unresolved @references")

		// A body-only update must still carry the resource description, so the
		// updated embedding re-includes it in the context header (issue #173).
		assert.Equal(t, "Test Description", payload.Description,
			"Update event should carry the prompt description for the embedding context header")
	})

	t.Run("publishes raw body when prompt has no @references", func(t *testing.T) {
		mockRepo := &mocks.MockPromptRepository{}
		mockProjectRepo := &mocks.MockProjectRepository{}
		mockEventManager := &event_mocks.MockEventPublisher{}

		mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Prompt")).
			Return(nil).Run(func(args mock.Arguments) {
			prompt := args.Get(1).(*models.Prompt)
			prompt.ID = "prompt-simple-123"
		})

		// Mock project validation
		mockProjectRepo.On("GetByID", mock.Anything, "user-123", "project-123").
			Return(&models.Project{
				ID:     "project-123",
				UserID: "user-123",
				Name:   "Test Project",
				Slug:   "test-project",
			}, nil)

		// Capture the event payload to verify body
		var capturedEvent events.Event
		mockEventManager.On("Publish", mock.Anything, mock.MatchedBy(func(event events.Event) bool {
			if event.Type() == events.EventTypePromptCreated {
				capturedEvent = event
				return true
			}
			return false
		})).Return(nil).Once()

		mockRefRepo := &mocks.MockPromptReferenceRepository{}
		mockRefRepo.On("DeleteByPromptID", mock.Anything, mock.Anything).Return(nil).Maybe()
		mockRefRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()

		service := NewPromptService(
			mockRepo, mockRefRepo, nil, mockProjectRepo, nil, allowAllAuthz{}, mockEventManager,
			func() *slog.Logger { l, _ := logtest.New(); return l }(), nil)

		// Create a simple prompt without @references
		req := &models.CreatePromptRequest{
			Name:        "Simple Prompt",
			Slug:        "simple-prompt",
			Description: "No references",
			Body:        "Just plain text without any references.",
			Status:      "published",
			ProjectID:   "project-123",
		}

		_, err := service.CreatePrompt("user-123", "team-123", req)
		assert.NoError(t, err)

		// Verify event was published
		mockEventManager.AssertExpectations(t)
		assert.NotNil(t, capturedEvent)

		// Extract and verify the body in the event payload
		payload, ok := capturedEvent.Payload().(*events.PromptCreatedPayload)
		assert.True(t, ok, "Event payload should be PromptCreatedPayload")

		// The body should be the same as input (no references to resolve)
		assert.Equal(t, "Just plain text without any references.", payload.Body,
			"Event should contain the same body when there are no references")
	})
}

func TestPromptService_GetPromptDependencies(t *testing.T) {
	t.Run("successfully retrieves prompt dependencies", func(t *testing.T) {
		mockRepo := &mocks.MockPromptRepository{}
		mockRefRepo := &mocks.MockPromptReferenceRepository{}

		logger, _ := logtest.New()
		service := NewPromptService(mockRepo, mockRefRepo, nil, nil, nil, allowAllAuthz{}, nil, logger, nil)

		userID := "user-123"
		promptID := "prompt-456"

		usedByDeps := []models.PromptDependencyInfo{
			{ID: "dep-1", Slug: "dependent-1", Name: "Dependent Prompt 1"},
		}
		usesDeps := []models.PromptDependencyInfo{
			{ID: "ref-1", Slug: "referenced-1", Name: "Referenced Prompt 1"},
		}

		mockRefRepo.On("GetPromptsUsingPrompt", mock.Anything, userID, promptID).
			Return(usedByDeps, nil)
		mockRefRepo.On("GetPromptsUsedByPrompt", mock.Anything, userID, promptID).
			Return(usesDeps, nil)

		result, err := service.GetPromptDependencies(userID, "team-123", promptID)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result.UsedBy, 1)
		assert.Len(t, result.Uses, 1)
		assert.Equal(t, "dependent-1", result.UsedBy[0].Slug)
		assert.Equal(t, "referenced-1", result.Uses[0].Slug)
		mockRefRepo.AssertExpectations(t)
	})

	t.Run("returns empty arrays when no dependencies exist", func(t *testing.T) {
		mockRepo := &mocks.MockPromptRepository{}
		mockRefRepo := &mocks.MockPromptReferenceRepository{}

		logger, _ := logtest.New()
		service := NewPromptService(mockRepo, mockRefRepo, nil, nil, nil, allowAllAuthz{}, nil, logger, nil)

		userID := "user-123"
		promptID := "prompt-456"

		mockRefRepo.On("GetPromptsUsingPrompt", mock.Anything, userID, promptID).
			Return([]models.PromptDependencyInfo{}, nil)
		mockRefRepo.On("GetPromptsUsedByPrompt", mock.Anything, userID, promptID).
			Return([]models.PromptDependencyInfo{}, nil)

		result, err := service.GetPromptDependencies(userID, "team-123", promptID)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result.UsedBy, 0)
		assert.Len(t, result.Uses, 0)
		mockRefRepo.AssertExpectations(t)
	})
}

func TestPromptService_GetPromptDependenciesBySlug(t *testing.T) {
	t.Run("successfully retrieves dependencies by slug", func(t *testing.T) {
		mockRepo := &mocks.MockPromptRepository{}
		mockRefRepo := &mocks.MockPromptReferenceRepository{}

		logger, _ := logtest.New()
		service := NewPromptService(mockRepo, mockRefRepo, nil, nil, nil, allowAllAuthz{}, nil, logger, nil)

		userID := "user-123"
		slug := "test-prompt"
		promptID := "prompt-456"

		prompt := &models.Prompt{
			ID:     promptID,
			Slug:   slug,
			UserID: userID,
		}

		mockRepo.On("GetBySlug", mock.Anything, userID, "team-123", slug).Return(prompt, nil)
		mockRefRepo.On("GetPromptsUsingPrompt", mock.Anything, userID, promptID).
			Return([]models.PromptDependencyInfo{}, nil)
		mockRefRepo.On("GetPromptsUsedByPrompt", mock.Anything, userID, promptID).
			Return([]models.PromptDependencyInfo{}, nil)

		result, err := service.GetPromptDependenciesBySlug(userID, "team-123", slug)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		mockRepo.AssertExpectations(t)
		mockRefRepo.AssertExpectations(t)
	})

	t.Run("returns error when prompt not found", func(t *testing.T) {
		mockRepo := &mocks.MockPromptRepository{}
		mockRefRepo := &mocks.MockPromptReferenceRepository{}

		logger, _ := logtest.New()
		service := NewPromptService(mockRepo, mockRefRepo, nil, nil, nil, allowAllAuthz{}, nil, logger, nil)

		userID := "user-123"
		slug := "non-existent"

		mockRepo.On("GetBySlug", mock.Anything, userID, "team-123", slug).Return(nil, repositories.ErrPromptNotFound)

		result, err := service.GetPromptDependenciesBySlug(userID, "team-123", slug)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "prompt not found")
		mockRepo.AssertExpectations(t)
	})
}

func TestPromptService_DeletePrompt_WithDependencies(t *testing.T) {
	t.Run("blocks deletion when prompt has dependents", func(t *testing.T) {
		mockRepo := &mocks.MockPromptRepository{}
		mockRefRepo := &mocks.MockPromptReferenceRepository{}

		logger, _ := logtest.New()
		service := NewPromptService(mockRepo, mockRefRepo, nil, nil, nil, allowAllAuthz{}, nil, logger, nil)

		userID := "user-123"
		promptID := "prompt-456"

		dependents := []models.PromptDependencyInfo{
			{ID: "dep-1", Slug: "dependent-1", Name: "Dependent Prompt 1"},
			{ID: "dep-2", Slug: "dependent-2", Name: "Dependent Prompt 2"},
		}

		mockRepo.On("GetByID", mock.Anything, userID, mock.Anything, promptID).
			Return(createTestPrompt(), nil)
		mockRefRepo.On("HasDependents", mock.Anything, promptID).Return(true, nil)
		mockRefRepo.On("GetPromptsUsingPrompt", mock.Anything, userID, promptID).
			Return(dependents, nil)

		err := service.DeletePrompt(userID, "team-123", promptID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot delete prompt")
		assert.Contains(t, err.Error(), "2 other prompt(s)")
		assert.Contains(t, err.Error(), "Dependent Prompt 1")
		mockRefRepo.AssertExpectations(t)
		mockRepo.AssertNotCalled(t, "Delete")
	})

	t.Run("allows deletion when prompt has no dependents", func(t *testing.T) {
		mockRepo := &mocks.MockPromptRepository{}
		mockRefRepo := &mocks.MockPromptReferenceRepository{}

		logger, _ := logtest.New()
		service := NewPromptService(mockRepo, mockRefRepo, nil, nil, nil, allowAllAuthz{}, nil, logger, nil)

		userID := "user-123"
		promptID := "prompt-456"

		mockRepo.On("GetByID", mock.Anything, userID, mock.Anything, promptID).
			Return(createTestPrompt(), nil)
		mockRefRepo.On("HasDependents", mock.Anything, promptID).Return(false, nil)
		mockRepo.On("Delete", mock.Anything, userID, mock.Anything, promptID).Return(nil)

		err := service.DeletePrompt(userID, "team-123", promptID)

		assert.NoError(t, err)
		mockRefRepo.AssertExpectations(t)
		mockRepo.AssertExpectations(t)
	})
}

// Tests for MCP Expose functionality

func TestPromptService_CreatePrompt_MCPExposeTrue(t *testing.T) {
	mockRepo := new(mocks.MockPromptRepository)
	mockRefRepo := new(mocks.MockPromptReferenceRepository)
	mockProjectRepo := new(mocks.MockProjectRepository)
	mockEventManager := new(event_mocks.MockEventPublisher)
	logger, _ := logtest.New()
	service := NewPromptService(mockRepo, mockRefRepo, nil, mockProjectRepo, nil, allowAllAuthz{}, mockEventManager, logger, nil)

	userID := "user-123"
	mcpExposeTrue := true
	req := &models.CreatePromptRequest{
		Name:        "Test Prompt",
		Slug:        "test-prompt",
		Description: "Test description",
		Body:        "Test body",
		Status:      "published",
		MCPExpose:   &mcpExposeTrue,
		ProjectID:   "project-123",
	}

	mockProjectRepo.On("GetByID", mock.Anything, "user-123", "project-123").
		Return(&models.Project{ID: "project-123", UserID: "user-123"}, nil)

	mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(p *models.Prompt) bool {
		return p.Name == req.Name && p.MCPExpose
	})).Return(nil).Once()

	mockRefRepo.On("DeleteByPromptID", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockRefRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockEventManager.On("Publish", mock.Anything, mock.Anything).Return(nil).Maybe()

	prompt, err := service.CreatePrompt(userID, "team-123", req)

	assert.NoError(t, err)
	assert.NotNil(t, prompt)
	assert.True(t, prompt.MCPExpose)
	mockRepo.AssertExpectations(t)
}

func TestPromptService_CreatePrompt_MCPExposeFalse(t *testing.T) {
	mockRepo := new(mocks.MockPromptRepository)
	mockRefRepo := new(mocks.MockPromptReferenceRepository)
	mockProjectRepo := new(mocks.MockProjectRepository)
	mockEventManager := new(event_mocks.MockEventPublisher)
	logger, _ := logtest.New()
	service := NewPromptService(mockRepo, mockRefRepo, nil, mockProjectRepo, nil, allowAllAuthz{}, mockEventManager, logger, nil)

	userID := "user-123"
	mcpExposeFalse := false
	req := &models.CreatePromptRequest{
		Name:        "Private Prompt",
		Slug:        "private-prompt",
		Description: "Private description",
		Body:        "Private body",
		Status:      "published",
		MCPExpose:   &mcpExposeFalse,
		ProjectID:   "project-123",
	}

	mockProjectRepo.On("GetByID", mock.Anything, "user-123", "project-123").
		Return(&models.Project{ID: "project-123", UserID: "user-123"}, nil)

	mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(p *models.Prompt) bool {
		return p.Name == req.Name && !p.MCPExpose
	})).Return(nil).Once()

	mockRefRepo.On("DeleteByPromptID", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockRefRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockEventManager.On("Publish", mock.Anything, mock.Anything).Return(nil).Maybe()

	prompt, err := service.CreatePrompt(userID, "team-123", req)

	assert.NoError(t, err)
	assert.NotNil(t, prompt)
	assert.False(t, prompt.MCPExpose)
	mockRepo.AssertExpectations(t)
}

func TestPromptService_CreatePrompt_MCPExposeDefaultsToTrue(t *testing.T) {
	mockRepo := new(mocks.MockPromptRepository)
	mockRefRepo := new(mocks.MockPromptReferenceRepository)
	mockProjectRepo := new(mocks.MockProjectRepository)
	mockEventManager := new(event_mocks.MockEventPublisher)
	logger, _ := logtest.New()
	service := NewPromptService(mockRepo, mockRefRepo, nil, mockProjectRepo, nil, allowAllAuthz{}, mockEventManager, logger, nil)

	userID := "user-123"
	req := &models.CreatePromptRequest{
		Name:        "Default Prompt",
		Slug:        "default-prompt",
		Description: "Default description",
		Body:        "Default body",
		Status:      "published",
		MCPExpose:   nil,
		ProjectID:   "project-123",
	}

	mockProjectRepo.On("GetByID", mock.Anything, "user-123", "project-123").
		Return(&models.Project{ID: "project-123", UserID: "user-123"}, nil)

	mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(p *models.Prompt) bool {
		return p.Name == req.Name && p.MCPExpose
	})).Return(nil).Once()

	mockRefRepo.On("DeleteByPromptID", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockRefRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockEventManager.On("Publish", mock.Anything, mock.Anything).Return(nil).Maybe()

	prompt, err := service.CreatePrompt(userID, "team-123", req)

	assert.NoError(t, err)
	assert.NotNil(t, prompt)
	assert.True(t, prompt.MCPExpose)
	mockRepo.AssertExpectations(t)
}

func TestPromptService_UpdatePrompt_MCPExposeToFalse(t *testing.T) {
	mockRepo := new(mocks.MockPromptRepository)
	mockRefRepo := new(mocks.MockPromptReferenceRepository)
	mockEventManager := new(event_mocks.MockEventPublisher)
	logger, _ := logtest.New()
	service := NewPromptService(mockRepo, mockRefRepo, nil, nil, nil, allowAllAuthz{}, mockEventManager, logger, nil)

	userID := "user-123"
	promptID := "prompt-123"
	existingPrompt := createTestPrompt()
	existingPrompt.MCPExpose = true

	mcpExposeFalse := false
	req := &models.UpdatePromptRequest{
		MCPExpose: &mcpExposeFalse,
	}

	mockRepo.On("GetByID", mock.Anything, userID, mock.Anything, promptID).Return(existingPrompt, nil).Once()
	mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(p *models.Prompt) bool {
		return p.ID == promptID && !p.MCPExpose
	})).Return(nil).Once()

	mockEventManager.On("Publish", mock.Anything, mock.Anything).Return(nil).Maybe()

	prompt, err := service.UpdatePrompt(userID, "team-123", promptID, req)

	assert.NoError(t, err)
	assert.NotNil(t, prompt)
	assert.False(t, prompt.MCPExpose)
	mockRepo.AssertExpectations(t)
}

func TestPromptService_UpdatePrompt_MCPExposeToTrue(t *testing.T) {
	mockRepo := new(mocks.MockPromptRepository)
	mockRefRepo := new(mocks.MockPromptReferenceRepository)
	mockEventManager := new(event_mocks.MockEventPublisher)
	logger, _ := logtest.New()
	service := NewPromptService(mockRepo, mockRefRepo, nil, nil, nil, allowAllAuthz{}, mockEventManager, logger, nil)

	userID := "user-123"
	promptID := "prompt-123"
	existingPrompt := createTestPrompt()
	existingPrompt.MCPExpose = false

	mcpExposeTrue := true
	req := &models.UpdatePromptRequest{
		MCPExpose: &mcpExposeTrue,
	}

	mockRepo.On("GetByID", mock.Anything, userID, mock.Anything, promptID).Return(existingPrompt, nil).Once()
	mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(p *models.Prompt) bool {
		return p.ID == promptID && p.MCPExpose
	})).Return(nil).Once()

	mockEventManager.On("Publish", mock.Anything, mock.Anything).Return(nil).Maybe()

	prompt, err := service.UpdatePrompt(userID, "team-123", promptID, req)

	assert.NoError(t, err)
	assert.NotNil(t, prompt)
	assert.True(t, prompt.MCPExpose)
	mockRepo.AssertExpectations(t)
}

func TestPromptService_UpdatePrompt_PreservesMCPExpose(t *testing.T) {
	mockRepo := new(mocks.MockPromptRepository)
	mockRefRepo := new(mocks.MockPromptReferenceRepository)
	mockEventManager := new(event_mocks.MockEventPublisher)
	logger, _ := logtest.New()
	service := NewPromptService(mockRepo, mockRefRepo, nil, nil, nil, allowAllAuthz{}, mockEventManager, logger, nil)

	userID := "user-123"
	promptID := "prompt-123"
	existingPrompt := createTestPrompt()
	existingPrompt.MCPExpose = true

	name := "Updated Name"
	req := &models.UpdatePromptRequest{
		Name: &name,
	}

	mockRepo.On("GetByID", mock.Anything, userID, mock.Anything, promptID).Return(existingPrompt, nil).Once()
	mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(p *models.Prompt) bool {
		return p.ID == promptID && p.MCPExpose && p.Name == "Updated Name"
	})).Return(nil).Once()

	mockRefRepo.On("DeleteByPromptID", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockRefRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockEventManager.On("Publish", mock.Anything, mock.Anything).Return(nil).Maybe()

	prompt, err := service.UpdatePrompt(userID, "team-123", promptID, req)

	assert.NoError(t, err)
	assert.NotNil(t, prompt)
	assert.True(t, prompt.MCPExpose)
	assert.Equal(t, "Updated Name", prompt.Name)
	mockRepo.AssertExpectations(t)
}

// TestPromptService_UpdatePromptBySlug_WithVersion tests that version field is properly
// retrieved by GetBySlug and used in update - this is a regression test for issue #486
func TestPromptService_UpdatePromptBySlug_WithVersion(t *testing.T) {
	mockRepo := mocks.NewMockPromptRepository(t)
	service := createTestPromptService(mockRepo, nil)

	// Create prompt with version 1
	existingPrompt := createTestPrompt()
	existingPrompt.Version = 1

	// Mock GetBySlug to return prompt with version
	mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
		Return(existingPrompt, nil)

	// Mock GetByID (called internally by UpdatePrompt)
	mockRepo.On("GetByID", mock.AnythingOfType("context.backgroundCtx"), "user-123", mock.Anything, "prompt-123").
		Return(existingPrompt, nil)

	// Mock Update - verify that version is passed correctly
	mockRepo.On("Update", mock.AnythingOfType("context.backgroundCtx"), mock.MatchedBy(func(prompt *models.Prompt) bool {
		// CRITICAL: Verify that version is not 0 (default value)
		// This would have caught the bug where GetBySlug didn't retrieve version
		return prompt.ID == "prompt-123" &&
			prompt.Version == int64(1) &&
			prompt.Name == "Updated Prompt"
	})).Return(nil)

	name := "Updated Prompt"
	request := &models.UpdatePromptRequest{
		Name: &name,
	}

	prompt, err := service.UpdatePromptBySlug("user-123", "team-123", "test-prompt", request)

	assert.NoError(t, err)
	assert.NotNil(t, prompt)
	assert.Equal(t, "prompt-123", prompt.ID)
	assert.Equal(t, int64(1), prompt.Version, "Version should be preserved from GetBySlug")
	mockRepo.AssertExpectations(t)
}

// TestPromptService_UpdatePromptBySlug_VersionZeroCausesFailure tests that when
// version is 0 (default value), the update fails - demonstrating the bug scenario
func TestPromptService_UpdatePromptBySlug_VersionZeroCausesFailure(t *testing.T) {
	mockRepo := mocks.NewMockPromptRepository(t)
	service := createTestPromptService(mockRepo, nil)

	// Simulate the bug: GetBySlug returns prompt WITHOUT version (defaults to 0)
	existingPromptWithoutVersion := createTestPrompt()
	existingPromptWithoutVersion.Version = 0 // This simulates the bug

	mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
		Return(existingPromptWithoutVersion, nil)

	mockRepo.On("GetByID", mock.AnythingOfType("context.backgroundCtx"), "user-123", mock.Anything, "prompt-123").
		Return(existingPromptWithoutVersion, nil)

	// Mock Update to fail with version mismatch (simulating database behavior)
	mockRepo.On("Update", mock.AnythingOfType("context.backgroundCtx"), mock.MatchedBy(func(prompt *models.Prompt) bool {
		return prompt.Version == int64(0)
	})).Return(fmt.Errorf("prompt not found or version mismatch"))

	name := "Updated Prompt"
	request := &models.UpdatePromptRequest{
		Name: &name,
	}

	_, err := service.UpdatePromptBySlug("user-123", "team-123", "test-prompt", request)

	// This should fail because version is 0
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "version mismatch")
	mockRepo.AssertExpectations(t)
}

// TestPromptService_GetPromptBySlug_ReturnsVersion tests that GetPromptBySlug
// returns all fields including version
func TestPromptService_GetPromptBySlug_ReturnsVersion(t *testing.T) {
	mockRepo := mocks.NewMockPromptRepository(t)
	service := createTestPromptService(mockRepo, nil)

	expectedPrompt := createTestPrompt()
	expectedPrompt.Version = 5 // Set a non-zero version

	mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
		Return(expectedPrompt, nil)

	prompt, err := service.GetPromptBySlug("user-123", "team-123", "test-prompt")

	assert.NoError(t, err)
	assert.NotNil(t, prompt)
	assert.Equal(t, int64(5), prompt.Version, "Version field must be retrieved by GetPromptBySlug")
	mockRepo.AssertExpectations(t)
}

// TestPromptService_GetPromptBySlug_NotFoundError tests that GetPromptBySlug
// logs at WARN level with slug context when prompt is not found
func TestPromptService_GetPromptBySlug_NotFoundError(t *testing.T) {
	mockRepo := mocks.NewMockPromptRepository(t)
	mockRefRepo := &mocks.MockPromptReferenceRepository{}
	mockRefRepo.On("DeleteByPromptID", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockRefRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()

	// Create logger with hook to capture log entries
	logger, hook := logtest.New()
	service := NewPromptService(mockRepo, mockRefRepo, nil, nil, nil, allowAllAuthz{}, nil, logger, nil)

	// Mock repository to return "prompt not found" error
	mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "non-existent-slug").
		Return(nil, repositories.ErrPromptNotFound)

	prompt, err := service.GetPromptBySlug("user-123", "team-123", "non-existent-slug")

	// Verify error is returned
	assert.Error(t, err)
	assert.Nil(t, prompt)

	// Verify error message is "prompt not found" for handler compatibility
	assert.Equal(t, "prompt not found", err.Error())

	// Verify logging occurred at WARN level (not ERROR) with slug context
	assert.NotEmpty(t, hook.Entries())
	logEntry := hook.LastEntry()
	assert.Equal(t, slog.LevelWarn, logEntry.Level, "Expected WARN level for not found error")
	assert.Equal(t, "Prompt not found by slug", logEntry.Message)
	assert.Equal(t, "user-123", logEntry.Data["user_id"])
	assert.Equal(t, "non-existent-slug", logEntry.Data["slug"])

	mockRepo.AssertExpectations(t)
}

// TestPromptService_GetPromptBySlug_DatabaseError tests that GetPromptBySlug
// logs at ERROR level for non-not-found database errors
func TestPromptService_GetPromptBySlug_DatabaseError(t *testing.T) {
	mockRepo := mocks.NewMockPromptRepository(t)
	mockRefRepo := &mocks.MockPromptReferenceRepository{}
	mockRefRepo.On("DeleteByPromptID", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockRefRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()

	// Create logger with hook to capture log entries
	logger, hook := logtest.New()
	service := NewPromptService(mockRepo, mockRefRepo, nil, nil, nil, allowAllAuthz{}, nil, logger, nil)

	// Mock repository to return a database error (not "prompt not found")
	mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-slug").
		Return(nil, fmt.Errorf("failed to get prompt by slug: database connection error"))

	prompt, err := service.GetPromptBySlug("user-123", "team-123", "test-slug")

	// Verify error is returned
	assert.Error(t, err)
	assert.Nil(t, prompt)

	// Verify logging occurred at ERROR level (not WARN)
	assert.NotEmpty(t, hook.Entries())
	logEntry := hook.LastEntry()
	assert.Equal(t, slog.LevelError, logEntry.Level, "Expected ERROR level for database errors")
	assert.Equal(t, "Failed to get prompt by slug", logEntry.Message)
	assert.Equal(t, "user-123", logEntry.Data["user_id"])
	assert.Equal(t, "test-slug", logEntry.Data["slug"])

	mockRepo.AssertExpectations(t)
}

// TestPromptService_UpdatePrompt_PreservesTeamID tests that team_id is preserved during update
func TestPromptService_UpdatePrompt_PreservesTeamID(t *testing.T) {
	mockRepo := mocks.NewMockPromptRepository(t)
	service := createTestPromptService(mockRepo, nil)

	// Create existing prompt with team_id
	existingPrompt := createTestPrompt()
	existingPrompt.TeamID = "team-456"

	mockRepo.On("GetByID", mock.AnythingOfType("context.backgroundCtx"), "user-123", mock.Anything, "prompt-123").
		Return(existingPrompt, nil)

	// Verify that Update is called with team_id preserved
	mockRepo.On("Update", mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(prompt *models.Prompt) bool {
			return prompt.ID == "prompt-123" &&
				prompt.TeamID == "team-456" && // TeamID must be preserved
				prompt.Name == "Updated Prompt"
		})).Return(nil)

	name := "Updated Prompt"
	request := &models.UpdatePromptRequest{
		Name: &name,
	}

	prompt, err := service.UpdatePrompt("user-123", "team-123", "prompt-123", request)

	assert.NoError(t, err)
	assert.NotNil(t, prompt)
	assert.Equal(t, "team-456", prompt.TeamID, "TeamID should be preserved during update")
	mockRepo.AssertExpectations(t)
}
