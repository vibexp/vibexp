package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

func TestNewBackofficeService(t *testing.T) {
	mockRepo := mocks.NewMockBackofficeRepository(t)
	service := NewBackofficeService(mockRepo)

	assert.NotNil(t, service)
}

func TestBackofficeService_GetUsageAndGrowth_Success(t *testing.T) {
	mockRepo := mocks.NewMockBackofficeRepository(t)
	service := NewBackofficeService(mockRepo)

	fromDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	toDate := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

	expectedUsageMetrics := []models.UsageMetricsRow{
		{
			WeekStart:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			NewUsers:     10,
			NewPrompts:   100,
			NewArtifacts: 50,
			NewMemories:  25,
		},
		{
			WeekStart:    time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),
			NewUsers:     15,
			NewPrompts:   120,
			NewArtifacts: 60,
			NewMemories:  30,
		},
	}

	userCreatedAt := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	expectedUserActivities := []models.UserActivityRow{
		{
			UserID:        "user-1",
			Email:         "user1@example.com",
			Name:          "User One",
			UserCreatedAt: userCreatedAt,
			TotalPrompts:  50,
		},
		{
			UserID:        "user-2",
			Email:         "user2@example.com",
			Name:          "User Two",
			UserCreatedAt: userCreatedAt,
			TotalPrompts:  30,
		},
	}

	mockRepo.EXPECT().GetUsageMetrics(mock.Anything, &fromDate, &toDate).Return(expectedUsageMetrics, nil)
	mockRepo.EXPECT().GetUserActivities(mock.Anything).Return(expectedUserActivities, nil)

	response, err := service.GetUsageAndGrowth(context.Background(), &fromDate, &toDate)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, expectedUsageMetrics, response.Usage)
	assert.Equal(t, expectedUserActivities, response.ActivitiesPerUser)
}

func TestBackofficeService_GetUsageAndGrowth_UsageMetricsError(t *testing.T) {
	mockRepo := mocks.NewMockBackofficeRepository(t)
	service := NewBackofficeService(mockRepo)

	fromDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	toDate := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

	mockRepo.EXPECT().GetUsageMetrics(mock.Anything, &fromDate, &toDate).Return(nil, assert.AnError)

	response, err := service.GetUsageAndGrowth(context.Background(), &fromDate, &toDate)

	assert.Error(t, err)
	assert.Nil(t, response)
}

func TestBackofficeService_GetUsageAndGrowth_UserActivitiesError(t *testing.T) {
	mockRepo := mocks.NewMockBackofficeRepository(t)
	service := NewBackofficeService(mockRepo)

	fromDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	toDate := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

	expectedUsageMetrics := []models.UsageMetricsRow{
		{
			WeekStart:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			NewPrompts: 100,
		},
	}

	mockRepo.EXPECT().GetUsageMetrics(mock.Anything, &fromDate, &toDate).Return(expectedUsageMetrics, nil)
	mockRepo.EXPECT().GetUserActivities(mock.Anything).Return(nil, assert.AnError)

	response, err := service.GetUsageAndGrowth(context.Background(), &fromDate, &toDate)

	assert.Error(t, err)
	assert.Nil(t, response)
}

func TestBackofficeService_GetUsageAndGrowth_NilDates(t *testing.T) {
	mockRepo := mocks.NewMockBackofficeRepository(t)
	service := NewBackofficeService(mockRepo)

	expectedUsageMetrics := []models.UsageMetricsRow{}
	expectedUserActivities := []models.UserActivityRow{}

	mockRepo.EXPECT().
		GetUsageMetrics(mock.Anything, (*time.Time)(nil), (*time.Time)(nil)).
		Return(expectedUsageMetrics, nil)
	mockRepo.EXPECT().GetUserActivities(mock.Anything).Return(expectedUserActivities, nil)

	response, err := service.GetUsageAndGrowth(context.Background(), nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Empty(t, response.Usage)
	assert.Empty(t, response.ActivitiesPerUser)
}

func TestBackofficeService_GetUsageAndGrowth_EmptyResults(t *testing.T) {
	mockRepo := mocks.NewMockBackofficeRepository(t)
	service := NewBackofficeService(mockRepo)

	fromDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	toDate := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

	mockRepo.EXPECT().GetUsageMetrics(mock.Anything, &fromDate, &toDate).Return([]models.UsageMetricsRow{}, nil)
	mockRepo.EXPECT().GetUserActivities(mock.Anything).Return([]models.UserActivityRow{}, nil)

	response, err := service.GetUsageAndGrowth(context.Background(), &fromDate, &toDate)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Empty(t, response.Usage)
	assert.Empty(t, response.ActivitiesPerUser)
}
