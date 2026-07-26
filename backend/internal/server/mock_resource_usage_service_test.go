package server

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
)

// MockResourceUsageServiceForHandlers is a mock implementation for testing handlers
type MockResourceUsageServiceForHandlers struct {
	mock.Mock
}

func (m *MockResourceUsageServiceForHandlers) GetResourceUsage(
	ctx context.Context, userID string,
) (*models.ResourceUsageResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ResourceUsageResponse), args.Error(1)
}
