package services

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

func TestPromptGalleryService_GetCategories(t *testing.T) {
	mockRepo := new(mocks.MockPromptGalleryRepository)
	logger, _ := logtest.New()

	service := NewPromptGalleryService(mockRepo, nil, logger)

	t.Run("Success", func(t *testing.T) {
		expectedCategories := []models.PromptGalleryCategory{
			{Category: "Engineering", Count: 5},
			{Category: "Marketing", Count: 3},
		}

		mockRepo.On("GetCategories", mock.Anything).Return(expectedCategories, nil).Once()

		categories, err := service.GetCategories()
		require.NoError(t, err)
		assert.Equal(t, expectedCategories, categories)

		mockRepo.AssertExpectations(t)
	})

	t.Run("Repository Error", func(t *testing.T) {
		mockRepo.On("GetCategories", mock.Anything).Return(nil, errors.New("db error")).Once()

		categories, err := service.GetCategories()
		assert.Error(t, err)
		assert.Nil(t, categories)
		assert.Contains(t, err.Error(), "failed to get categories")

		mockRepo.AssertExpectations(t)
	})
}

//nolint:funlen // Test function with multiple subtests
func TestPromptGalleryService_ListPrompts(t *testing.T) {
	mockRepo := new(mocks.MockPromptGalleryRepository)
	logger, _ := logtest.New()

	service := NewPromptGalleryService(mockRepo, nil, logger)

	t.Run("Success with filters", func(t *testing.T) {
		prompts := []models.PromptGalleryTemplate{
			{
				ID:          "123",
				Title:       "Test Prompt",
				Description: "Test Description",
				Content:     "Test Content",
				Category:    "Engineering",
			},
		}

		mockRepo.On("List", mock.Anything, mock.MatchedBy(func(f repositories.PromptGalleryFilters) bool {
			return f.Category == "Engineering" &&
				f.Search == "test" &&
				len(f.Tags) == 1 &&
				f.Tags[0] == "code-review" &&
				f.Page == 1 &&
				f.Limit == 20
		})).Return(prompts, 1, nil).Once()

		result, err := service.ListPrompts("Engineering", "test", []string{"code-review"}, 1, 20)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result.Prompts, 1)
		assert.Equal(t, 1, result.TotalCount)
		assert.Equal(t, 1, result.Page)
		assert.Equal(t, 20, result.PerPage)
		assert.Equal(t, 1, result.TotalPages)

		mockRepo.AssertExpectations(t)
	})

	t.Run("Default pagination values", func(t *testing.T) {
		mockRepo.On("List", mock.Anything, mock.MatchedBy(func(f repositories.PromptGalleryFilters) bool {
			return f.Page == 1 && f.Limit == 20
		})).Return([]models.PromptGalleryTemplate{}, 0, nil).Once()

		result, err := service.ListPrompts("", "", nil, 0, 0)
		require.NoError(t, err)
		assert.NotNil(t, result)

		mockRepo.AssertExpectations(t)
	})

	t.Run("Limit exceeds maximum", func(t *testing.T) {
		mockRepo.On("List", mock.Anything, mock.MatchedBy(func(f repositories.PromptGalleryFilters) bool {
			return f.Limit == 20 // Should cap at 20
		})).Return([]models.PromptGalleryTemplate{}, 0, nil).Once()

		result, err := service.ListPrompts("", "", nil, 1, 200)
		require.NoError(t, err)
		assert.NotNil(t, result)

		mockRepo.AssertExpectations(t)
	})

	t.Run("Calculate total pages correctly", func(t *testing.T) {
		mockRepo.On("List", mock.Anything, mock.Anything).Return([]models.PromptGalleryTemplate{}, 25, nil).Once()

		result, err := service.ListPrompts("", "", nil, 1, 10)
		require.NoError(t, err)
		assert.Equal(t, 25, result.TotalCount)
		assert.Equal(t, 3, result.TotalPages) // 25 items / 10 per page = 3 pages

		mockRepo.AssertExpectations(t)
	})

	t.Run("Repository Error", func(t *testing.T) {
		mockRepo.On("List", mock.Anything, mock.Anything).Return(nil, 0, errors.New("db error")).Once()

		result, err := service.ListPrompts("", "", nil, 1, 10)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to list prompts")

		mockRepo.AssertExpectations(t)
	})
}

func TestPromptGalleryService_GetPromptByID(t *testing.T) {
	mockRepo := new(mocks.MockPromptGalleryRepository)
	logger, _ := logtest.New()

	service := NewPromptGalleryService(mockRepo, nil, logger)

	t.Run("Success", func(t *testing.T) {
		expectedPrompt := &models.PromptGalleryTemplate{
			ID:          "123",
			Title:       "Test Prompt",
			Description: "Test Description",
			Content:     "Test Content",
			Category:    "Engineering",
		}

		mockRepo.On("GetByID", mock.Anything, "123").Return(expectedPrompt, nil).Once()

		prompt, err := service.GetPromptByID("123")
		require.NoError(t, err)
		assert.Equal(t, expectedPrompt, prompt)

		mockRepo.AssertExpectations(t)
	})

	t.Run("Prompt Not Found", func(t *testing.T) {
		mockRepo.On("GetByID", mock.Anything, "999").Return(nil, repositories.ErrPromptNotFound).Once()

		prompt, err := service.GetPromptByID("999")
		assert.Error(t, err)
		assert.Nil(t, prompt)

		mockRepo.AssertExpectations(t)
	})
}

func TestPromptGalleryService_TrackPromptUsage(t *testing.T) {
	mockRepo := new(mocks.MockPromptGalleryRepository)
	logger, _ := logtest.New()

	service := NewPromptGalleryService(mockRepo, nil, logger)

	t.Run("Success", func(t *testing.T) {
		prompt := &models.PromptGalleryTemplate{
			ID:       "123",
			Title:    "Test Prompt",
			Category: "Engineering",
		}

		mockRepo.On("GetByID", mock.Anything, "123").Return(prompt, nil).Once()

		req := &models.PromptGalleryUsageRequest{
			PromptID: "123",
		}

		err := service.TrackPromptUsage("user-123", req)
		require.NoError(t, err)

		mockRepo.AssertExpectations(t)
	})

	t.Run("Prompt Not Found", func(t *testing.T) {
		mockRepo.On("GetByID", mock.Anything, "999").Return(nil, repositories.ErrPromptNotFound).Once()

		req := &models.PromptGalleryUsageRequest{
			PromptID: "999",
		}

		err := service.TrackPromptUsage("user-123", req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "prompt not found")

		mockRepo.AssertExpectations(t)
	})
}
