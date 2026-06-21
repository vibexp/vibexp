package mocks

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ActivityRepositoryMock is a mock implementation of ActivityRepository
type ActivityRepositoryMock struct {
	mock.Mock
}

// Create mocks the Create method
func (m *ActivityRepositoryMock) Create(ctx context.Context, activity *models.Activity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

// GetByID mocks the GetByID method
func (m *ActivityRepositoryMock) GetByID(ctx context.Context, userID, activityID string) (*models.Activity, error) {
	args := m.Called(ctx, userID, activityID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Activity), args.Error(1)
}

// List mocks the List method
func (m *ActivityRepositoryMock) List(ctx context.Context, filters repositories.ActivityFilters) (*models.ActivityListResponse, error) {
	args := m.Called(ctx, filters)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ActivityListResponse), args.Error(1)
}

// GetStats mocks the GetStats method
func (m *ActivityRepositoryMock) GetStats(ctx context.Context, userID string) (*models.ActivityStatsResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ActivityStatsResponse), args.Error(1)
}

// Delete mocks the Delete method
func (m *ActivityRepositoryMock) Delete(ctx context.Context, activityID string) error {
	args := m.Called(ctx, activityID)
	return args.Error(0)
}

// DeleteOlderThan mocks the DeleteOlderThan method
func (m *ActivityRepositoryMock) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	args := m.Called(ctx, before)
	return args.Get(0).(int64), args.Error(1)
}
