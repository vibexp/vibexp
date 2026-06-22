package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
)

// MockResourceAccessService is a mock implementation of resourceaccess.ResourceAccessService.
type MockResourceAccessService struct {
	mock.Mock
}

func (m *MockResourceAccessService) RecordAccess(event *models.ResourceAccessEvent) {
	m.Called(event)
}

func (m *MockResourceAccessService) GetMetrics(
	ctx context.Context, teamID, resourceType, resourceID string, rangeDays int,
) (*resourceaccess.MetricsResult, error) {
	args := m.Called(ctx, teamID, resourceType, resourceID, rangeDays)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*resourceaccess.MetricsResult), args.Error(1)
}

func (m *MockResourceAccessService) GetTeamMetrics(
	ctx context.Context, teamID string, rangeDays int,
) (*resourceaccess.MetricsResult, error) {
	args := m.Called(ctx, teamID, rangeDays)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*resourceaccess.MetricsResult), args.Error(1)
}

func (m *MockResourceAccessService) GetTopAccessedResources(
	ctx context.Context, teamID string, rangeDays int, source string, limit int,
) ([]models.TopAccessedResource, error) {
	args := m.Called(ctx, teamID, rangeDays, source, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.TopAccessedResource), args.Error(1)
}

func (m *MockResourceAccessService) RunRetentionJob(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// MockAccessEventsContainer implements Container for access-events handler tests.
type MockAccessEventsContainer struct {
	BaseMockContainer     // Embed base container for default nil implementations
	resourceAccessService *MockResourceAccessService
}

func (m *MockAccessEventsContainer) ResourceAccessService() resourceaccess.ResourceAccessService {
	return m.resourceAccessService
}

func newMockAccessEventsContainer() *MockAccessEventsContainer {
	return &MockAccessEventsContainer{
		resourceAccessService: &MockResourceAccessService{},
	}
}

func createTestAccessEventsServer(container *MockAccessEventsContainer) *Server {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	r := chi.NewRouter()

	srv := &Server{
		port:                  "8080",
		container:             container,
		resourceAccessService: container.resourceAccessService,
		logger:                logger,
		config:                cfg,
		router:                r,
	}

	// Register internal job route without OIDC middleware (middleware is bypassed in unit tests).
	r.Post("/internal/jobs/access-events/retention", srv.handleAccessEventsRetentionJob)

	return srv
}

// TestHandleAccessEventsRetentionJob_Success verifies 200 response when RunRetentionJob succeeds.
func TestHandleAccessEventsRetentionJob_Success(t *testing.T) {
	container := newMockAccessEventsContainer()
	container.resourceAccessService.On("RunRetentionJob", mock.Anything).Return(nil).Once()

	srv := createTestAccessEventsServer(container)
	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/access-events/retention", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK", w.Body.String())
	container.resourceAccessService.AssertExpectations(t)
}

// TestHandleAccessEventsRetentionJob_ServiceError verifies 500 response when RunRetentionJob fails.
func TestHandleAccessEventsRetentionJob_ServiceError(t *testing.T) {
	container := newMockAccessEventsContainer()
	container.resourceAccessService.On("RunRetentionJob", mock.Anything).
		Return(errors.New("db unavailable")).Once()

	srv := createTestAccessEventsServer(container)
	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/access-events/retention", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	// Lock down the 500 contract: WriteJSONError emits a JSON body carrying the
	// "Retention job failed" detail (never the raw underlying error).
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	var errBody map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errBody))
	assert.Contains(t, w.Body.String(), "Retention job failed")
	assert.NotContains(t, w.Body.String(), "db unavailable")
	container.resourceAccessService.AssertExpectations(t)
}

// TestHandleAccessEventsRetentionJob_InvokedOnce verifies RunRetentionJob is called exactly once.
func TestHandleAccessEventsRetentionJob_InvokedOnce(t *testing.T) {
	container := newMockAccessEventsContainer()
	container.resourceAccessService.On("RunRetentionJob", mock.Anything).Return(nil).Once()

	srv := createTestAccessEventsServer(container)
	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/access-events/retention", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	container.resourceAccessService.AssertNumberOfCalls(t, "RunRetentionJob", 1)
	assert.Equal(t, http.StatusOK, w.Code)
}
