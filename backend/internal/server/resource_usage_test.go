package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/pkg/events"
)

// MockResourceUsageService is a mock implementation of ResourceUsageServiceInterface
type MockResourceUsageService struct {
	mock.Mock
}

func (m *MockResourceUsageService) CheckResourceLimit(
	ctx context.Context,
	userID, resourceType string,
) (bool, error) {
	args := m.Called(ctx, userID, resourceType)
	return args.Bool(0), args.Error(1)
}

func (m *MockResourceUsageService) TrackResourceCreation(
	ctx context.Context,
	userID, resourceType, resourceID string,
) error {
	args := m.Called(ctx, userID, resourceType, resourceID)
	return args.Error(0)
}

func (m *MockResourceUsageService) TrackResourceDeletion(
	ctx context.Context,
	userID, resourceType, resourceID string,
) error {
	args := m.Called(ctx, userID, resourceType, resourceID)
	return args.Error(0)
}

func (m *MockResourceUsageService) GetResourceUsage(
	ctx context.Context,
	userID string,
) (*models.ResourceUsageResponse, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(*models.ResourceUsageResponse), args.Error(1)
}

func TestGetResourceUsage(t *testing.T) {
	// Create mock service
	mockService := new(MockResourceUsageService)

	// Create test data
	userID := "test-user-id"
	now := time.Now().UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	responseData := &models.ResourceUsageResponse{
		UserID:      userID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Resources: []models.ResourceUsageItem{
			{
				ResourceType: events.ResourceTypePrompt,
				Count:        5,
				Limit:        20,
				Percentage:   25,
			},
			{
				ResourceType: events.ResourceTypeMemory,
				Count:        8,
				Limit:        20,
				Percentage:   40,
			},
		},
	}

	// Setup expected calls
	mockService.On("GetResourceUsage", mock.Anything, userID).Return(responseData, nil)

	// Create handler
	logger := logrus.New()
	handler := NewResourceUsageHandler(mockService, logger)

	// Create request
	req, err := http.NewRequest("GET", "/api/v1/resource-usage", nil)
	require.NoError(t, err)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, userID))

	// Create response recorder
	w := httptest.NewRecorder()

	// Call handler
	handler.GetResourceUsage(w, req)

	// Assert status code
	assert.Equal(t, http.StatusOK, w.Code)

	// Decode response
	var response models.ResourceUsageResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Assert response data
	assert.Equal(t, userID, response.UserID)
	assert.Equal(t, 2, len(response.Resources))

	// Verify expected calls
	mockService.AssertExpectations(t)
}
