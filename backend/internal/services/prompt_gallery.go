package services

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

type PromptGalleryService struct {
	repo         repositories.PromptGalleryRepository
	eventManager events.EventPublisher
	logger       *slog.Logger
}

// Ensure PromptGalleryService implements PromptGalleryServiceInterface
var _ PromptGalleryServiceInterface = (*PromptGalleryService)(nil)

func NewPromptGalleryService(
	repo repositories.PromptGalleryRepository,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) *PromptGalleryService {
	return &PromptGalleryService{
		repo:         repo,
		eventManager: eventManager,
		logger:       logger,
	}
}

func (s *PromptGalleryService) GetCategories() ([]models.PromptGalleryCategory, error) {
	ctx := context.Background()
	categories, err := s.repo.GetCategories(ctx)
	if err != nil {
		s.logger.With("error", err).Error("Failed to get prompt gallery categories")
		return nil, fmt.Errorf("failed to get categories: %w", err)
	}

	return categories, nil
}

func (s *PromptGalleryService) ListPrompts(
	category, search string, tags []string, page, limit int,
) (*models.PromptGalleryListResponse, error) {
	ctx := context.Background()

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	filters := repositories.PromptGalleryFilters{
		Category: category,
		Search:   search,
		Tags:     tags,
		Page:     page,
		Limit:    limit,
	}

	prompts, total, err := s.repo.List(ctx, filters)
	if err != nil {
		s.logger.With("error", err).Error("Failed to list prompt gallery templates")
		return nil, fmt.Errorf("failed to list prompts: %w", err)
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	return &models.PromptGalleryListResponse{
		Prompts:    prompts,
		TotalCount: total,
		Page:       page,
		PerPage:    limit,
		TotalPages: totalPages,
	}, nil
}

func (s *PromptGalleryService) GetPromptByID(promptID string) (*models.PromptGalleryTemplate, error) {
	ctx := context.Background()
	prompt, err := s.repo.GetByID(ctx, promptID)
	if err != nil {
		s.logger.With("error", err).With("prompt_id", promptID).Error("Failed to get prompt gallery template")
		return nil, fmt.Errorf("failed to get prompt: %w", err)
	}

	return prompt, nil
}

func (s *PromptGalleryService) TrackPromptUsage(userID string, req *models.PromptGalleryUsageRequest) error {
	ctx := context.Background()

	// Verify the prompt exists
	_, err := s.repo.GetByID(ctx, req.PromptID)
	if err != nil {
		s.logger.With("error", err).With("prompt_id", req.PromptID).Error("Failed to verify prompt for usage tracking")
		return fmt.Errorf("failed to verify prompt: %w", err)
	}

	// Log the usage for analytics
	s.logger.With(
		"event_type", "prompt_gallery_prompt_used",
		"user_id", userID,
		"prompt_id", req.PromptID,
	).
		Info("Prompt gallery usage tracked")

	return nil
}
