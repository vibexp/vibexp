package services

import (
	"context"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// BackofficeService implements the BackofficeServiceInterface
type BackofficeService struct {
	backofficeRepo repositories.BackofficeRepository
}

// NewBackofficeService creates a new BackofficeService
func NewBackofficeService(backofficeRepo repositories.BackofficeRepository) BackofficeServiceInterface {
	return &BackofficeService{
		backofficeRepo: backofficeRepo,
	}
}

// GetUsageAndGrowth retrieves usage and growth data for the platform
func (s *BackofficeService) GetUsageAndGrowth(
	ctx context.Context,
	fromDate, toDate *time.Time,
) (*models.UsageAndGrowthResponse, error) {
	// Get usage metrics
	usageMetrics, err := s.backofficeRepo.GetUsageMetrics(ctx, fromDate, toDate)
	if err != nil {
		return nil, err
	}

	// Get user activities
	userActivities, err := s.backofficeRepo.GetUserActivities(ctx)
	if err != nil {
		return nil, err
	}

	return &models.UsageAndGrowthResponse{
		Usage:             usageMetrics,
		ActivitiesPerUser: userActivities,
	}, nil
}
