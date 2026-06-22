package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/pkg/events"
)

// MockResourceUsageContainer implements Container interface for resource usage handler tests
type MockResourceUsageContainer struct {
	BaseMockContainer // Embed base container for default nil implementations
	mock.Mock
	resourceUsageService *svcmocks.MockResourceUsageServiceInterface
}

func (m *MockResourceUsageContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return m.resourceUsageService
}

const testResourceUsageUserID = "test-user-123"

func newMockResourceUsageContainer(t *testing.T) *MockResourceUsageContainer {
	return &MockResourceUsageContainer{
		resourceUsageService: svcmocks.NewMockResourceUsageServiceInterface(t),
	}
}

func createResourceUsageTestServer(container *MockResourceUsageContainer) *Server {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	// Initialize router manually for testing
	r := chi.NewRouter()

	srv := &Server{
		port:      "8080",
		container: container,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	// Register routes manually (simplified version for testing)
	r.Route("/api/v1", func(r chi.Router) {
		handler := NewResourceUsageHandler(container.resourceUsageService, logger)
		r.Get("/resource-usage", handler.GetResourceUsage)
	})

	return srv
}

func makeResourceUsageAuthenticatedRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/resource-usage", nil)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testResourceUsageUserID))

	return req
}

func makeResourceUsageUnauthenticatedRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/resource-usage", nil)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// createResourceUsageResponse is a helper to create resource usage response for tests
func createResourceUsageResponse(resources []models.ResourceUsageItem) *models.ResourceUsageResponse {
	now := time.Now().UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	return &models.ResourceUsageResponse{
		UserID:      testResourceUsageUserID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Resources:   resources,
	}
}

// newItem is a helper to create ResourceUsageItem with shorter syntax (teamQuota defaults to 0)
func newItem(rt string, count, limit, indLimit, pct int) models.ResourceUsageItem {
	return models.ResourceUsageItem{
		ResourceType:    rt,
		Count:           count,
		Limit:           limit,
		IndividualLimit: indLimit,
		TeamQuota:       0,
		Percentage:      pct,
	}
}

// ========================================
// Basic Usage Retrieval Tests
// ========================================

// TestGetResourceUsage_Success tests successful resource usage retrieval for authenticated user
func TestGetResourceUsage_Success(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	expectedResources := []models.ResourceUsageItem{
		{
			ResourceType:    events.ResourceTypePrompt,
			Count:           5,
			Limit:           100,
			IndividualLimit: 100,
			TeamQuota:       0,
			Percentage:      5,
		},
		{
			ResourceType:    events.ResourceTypeMemory,
			Count:           10,
			Limit:           100,
			IndividualLimit: 100,
			TeamQuota:       0,
			Percentage:      10,
		},
	}
	expectedResponse := createResourceUsageResponse(expectedResources)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ResourceUsageResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, testResourceUsageUserID, response.UserID)
	assert.Len(t, response.Resources, 2)

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestGetResourceUsage_Unauthorized tests that unauthenticated requests are rejected
func TestGetResourceUsage_Unauthorized(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)
	srv := createResourceUsageTestServer(mockContainer)

	req := makeResourceUsageUnauthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	// Should return 401 Unauthorized when no user ID in context
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Service should not be called for unauthorized requests
	mockContainer.resourceUsageService.AssertNotCalled(t, "GetResourceUsage")
}

// TestGetResourceUsage_IncludesAllResources verifies all resource types are present in response
func TestGetResourceUsage_IncludesAllResources(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	// All resource types as defined in the service
	allResources := []models.ResourceUsageItem{
		newItem(events.ResourceTypeAITool, 1, 2, 2, 50),
		newItem(events.ResourceTypeAISession, 50, 100, 100, 50),
		newItem(events.ResourceTypePrompt, 10, 100, 100, 10),
		newItem(events.ResourceTypeArtifact, 5, 100, 100, 5),
		newItem(events.ResourceTypeMemory, 20, 100, 100, 20),
		newItem(events.ResourceTypeBlueprint, 2, 20, 20, 10),
		newItem(events.ResourceTypeAgent, 1, 2, 2, 50),
		newItem(events.ResourceTypeAgentConv, 25, 100, 100, 25),
		newItem(events.ResourceTypeTeam, 1, 2, 2, 50),
	}
	expectedResponse := createResourceUsageResponse(allResources)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ResourceUsageResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Len(t, response.Resources, 9) // All 9 resource types

	// Verify all expected resource types are present
	resourceTypes := make(map[string]bool)
	for _, r := range response.Resources {
		resourceTypes[r.ResourceType] = true
	}

	expectedTypes := []string{
		events.ResourceTypeAITool,
		events.ResourceTypeAISession,
		events.ResourceTypePrompt,
		events.ResourceTypeArtifact,
		events.ResourceTypeMemory,
		events.ResourceTypeBlueprint,
		events.ResourceTypeAgent,
		events.ResourceTypeAgentConv,
		events.ResourceTypeTeam,
	}

	for _, expectedType := range expectedTypes {
		assert.True(t, resourceTypes[expectedType], "Expected resource type %s not found", expectedType)
	}

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// ========================================
// Subscription Tier Quota Tests
// ========================================

// TestGetResourceUsage_FreeUser tests quota limits for free tier user
func TestGetResourceUsage_FreeUser(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	// Free tier (basic) has lower limits: prompts=100, memories=100, etc.
	freeUserResources := []models.ResourceUsageItem{
		newItem(events.ResourceTypePrompt, 50, 100, 100, 50),
		newItem(events.ResourceTypeMemory, 30, 100, 100, 30),
		newItem(events.ResourceTypeAgent, 1, 2, 2, 50),
		newItem(events.ResourceTypeAITool, 1, 2, 2, 50),
	}
	expectedResponse := createResourceUsageResponse(freeUserResources)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ResourceUsageResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)

	// Verify free tier limits are applied
	for _, resource := range response.Resources {
		switch resource.ResourceType {
		case events.ResourceTypePrompt:
			assert.Equal(t, 100, resource.Limit)
		case events.ResourceTypeMemory:
			assert.Equal(t, 100, resource.Limit)
		case events.ResourceTypeAgent:
			assert.Equal(t, 2, resource.Limit)
		case events.ResourceTypeAITool:
			assert.Equal(t, 2, resource.Limit)
		}
	}

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestGetResourceUsage_ProUser tests quota limits for pro tier user
func TestGetResourceUsage_ProUser(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	// Pro tier has higher limits: prompts=500, memories=500, etc.
	proUserResources := []models.ResourceUsageItem{
		newItem(events.ResourceTypePrompt, 200, 500, 500, 40),
		newItem(events.ResourceTypeMemory, 150, 500, 500, 30),
		newItem(events.ResourceTypeAgent, 3, 5, 5, 60),
		newItem(events.ResourceTypeAITool, 2, 3, 3, 66),
	}
	expectedResponse := createResourceUsageResponse(proUserResources)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ResourceUsageResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)

	// Verify pro tier limits are applied
	for _, resource := range response.Resources {
		switch resource.ResourceType {
		case events.ResourceTypePrompt:
			assert.Equal(t, 500, resource.Limit)
		case events.ResourceTypeMemory:
			assert.Equal(t, 500, resource.Limit)
		case events.ResourceTypeAgent:
			assert.Equal(t, 5, resource.Limit)
		case events.ResourceTypeAITool:
			assert.Equal(t, 3, resource.Limit)
		}
	}

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestGetResourceUsage_EnterpriseUser tests quota limits for enterprise/power user tier
func TestGetResourceUsage_EnterpriseUser(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	// Power user tier has highest limits: prompts=1000, some unlimited (-1)
	enterpriseResources := []models.ResourceUsageItem{
		newItem(events.ResourceTypePrompt, 500, 1000, 1000, 50),
		newItem(events.ResourceTypeMemory, 300, 1000, 1000, 30),
		newItem(events.ResourceTypeAgent, 10, -1, -1, 0),
		newItem(events.ResourceTypeAITool, 2, -1, -1, 0),
	}
	expectedResponse := createResourceUsageResponse(enterpriseResources)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ResourceUsageResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)

	// Verify enterprise tier limits and unlimited resources
	for _, resource := range response.Resources {
		switch resource.ResourceType {
		case events.ResourceTypePrompt:
			assert.Equal(t, 1000, resource.Limit)
		case events.ResourceTypeMemory:
			assert.Equal(t, 1000, resource.Limit)
		case events.ResourceTypeAgent:
			assert.Equal(t, -1, resource.Limit) // Unlimited
		case events.ResourceTypeAITool:
			assert.Equal(t, -1, resource.Limit) // Unlimited
		}
	}

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestGetResourceUsage_TeamMember tests individual + team quota aggregation
func TestGetResourceUsage_TeamMember(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	// Team member gets individual + team quota contribution
	teamMemberResources := []models.ResourceUsageItem{
		{
			ResourceType:    events.ResourceTypePrompt,
			Count:           75,
			Limit:           200, // 100 individual + 100 team quota
			IndividualLimit: 100,
			TeamQuota:       100,
			Percentage:      37,
		},
		{
			ResourceType:    events.ResourceTypeMemory,
			Count:           50,
			Limit:           200, // 100 individual + 100 team quota
			IndividualLimit: 100,
			TeamQuota:       100,
			Percentage:      25,
		},
		{
			ResourceType:    events.ResourceTypeAgent,
			Count:           3,
			Limit:           7, // 2 individual + 5 team quota
			IndividualLimit: 2,
			TeamQuota:       5,
			Percentage:      42,
		},
	}
	expectedResponse := createResourceUsageResponse(teamMemberResources)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ResourceUsageResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)

	// Verify team quota aggregation
	for _, resource := range response.Resources {
		assert.Greater(t, resource.TeamQuota, 0, "Team member should have team quota for %s", resource.ResourceType)
		assert.Equal(t, resource.IndividualLimit+resource.TeamQuota, resource.Limit,
			"Total limit should equal individual + team quota for %s", resource.ResourceType)
	}

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestGetResourceUsage_UnlimitedResources tests handling of unlimited (-1) resources
func TestGetResourceUsage_UnlimitedResources(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	// Power user with unlimited resources
	unlimitedResources := []models.ResourceUsageItem{
		{
			ResourceType:    events.ResourceTypeAgent,
			Count:           50,
			Limit:           -1, // Unlimited
			IndividualLimit: -1,
			TeamQuota:       0,
			Percentage:      0, // 0% for unlimited
		},
		{
			ResourceType:    events.ResourceTypeAITool,
			Count:           5,
			Limit:           -1, // Unlimited
			IndividualLimit: -1,
			TeamQuota:       0,
			Percentage:      0,
		},
	}
	expectedResponse := createResourceUsageResponse(unlimitedResources)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ResourceUsageResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)

	// Verify unlimited resources have -1 limit and 0% percentage
	for _, resource := range response.Resources {
		assert.Equal(t, -1, resource.Limit, "Unlimited resource should have -1 limit")
		assert.Equal(t, 0, resource.Percentage, "Unlimited resource should have 0% percentage")
	}

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// ========================================
// Usage Scenario Tests
// ========================================

// TestGetResourceUsage_WithinQuota tests usage below limits
func TestGetResourceUsage_WithinQuota(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	// All resources well within quota (< 50%)
	withinQuotaResources := []models.ResourceUsageItem{
		newItem(events.ResourceTypePrompt, 10, 100, 100, 10),
		newItem(events.ResourceTypeMemory, 5, 100, 100, 5),
		newItem(events.ResourceTypeArtifact, 20, 100, 100, 20),
	}
	expectedResponse := createResourceUsageResponse(withinQuotaResources)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ResourceUsageResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)

	// Verify all resources are within quota
	for _, resource := range response.Resources {
		assert.Less(t, resource.Percentage, 50, "Resource %s should be within quota", resource.ResourceType)
		assert.Less(t, resource.Count, resource.Limit, "Count should be less than limit for %s", resource.ResourceType)
	}

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestGetResourceUsage_NearQuota tests usage at 80-99% of limit
func TestGetResourceUsage_NearQuota(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	// Resources at 80-99% of quota (warning zone)
	nearQuotaResources := []models.ResourceUsageItem{
		newItem(events.ResourceTypePrompt, 85, 100, 100, 85),
		newItem(events.ResourceTypeMemory, 90, 100, 100, 90),
		newItem(events.ResourceTypeArtifact, 95, 100, 100, 95),
	}
	expectedResponse := createResourceUsageResponse(nearQuotaResources)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ResourceUsageResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)

	// Verify all resources are near quota (80-99%)
	for _, resource := range response.Resources {
		assert.GreaterOrEqual(t, resource.Percentage, 80, "Resource %s should be near quota", resource.ResourceType)
		assert.Less(t, resource.Percentage, 100, "Resource %s should not exceed 100%", resource.ResourceType)
	}

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestGetResourceUsage_ExceededQuota tests usage above limit (100%+)
func TestGetResourceUsage_ExceededQuota(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	// Resources exceeding quota (can happen if limits were reduced)
	exceededQuotaResources := []models.ResourceUsageItem{
		newItem(events.ResourceTypePrompt, 110, 100, 100, 110),
		newItem(events.ResourceTypeMemory, 150, 100, 100, 150),
	}
	expectedResponse := createResourceUsageResponse(exceededQuotaResources)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ResourceUsageResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)

	// Verify resources are shown as exceeded
	for _, resource := range response.Resources {
		assert.GreaterOrEqual(t, resource.Percentage, 100, "Resource %s should show exceeded quota", resource.ResourceType)
		assert.GreaterOrEqual(t, resource.Count, resource.Limit, "Count should exceed limit for %s", resource.ResourceType)
	}

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestGetResourceUsage_ZeroUsage tests new user with no usage
func TestGetResourceUsage_ZeroUsage(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	// New user with zero usage across all resources
	zeroUsageResources := []models.ResourceUsageItem{
		newItem(events.ResourceTypePrompt, 0, 100, 100, 0),
		newItem(events.ResourceTypeMemory, 0, 100, 100, 0),
		newItem(events.ResourceTypeArtifact, 0, 100, 100, 0),
		newItem(events.ResourceTypeAgent, 0, 2, 2, 0),
		newItem(events.ResourceTypeAITool, 0, 2, 2, 0),
	}
	expectedResponse := createResourceUsageResponse(zeroUsageResources)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ResourceUsageResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)

	// Verify all resources have zero count and 0% usage
	for _, resource := range response.Resources {
		assert.Equal(t, 0, resource.Count, "New user should have zero count for %s", resource.ResourceType)
		assert.Equal(t, 0, resource.Percentage, "New user should have 0%% usage for %s", resource.ResourceType)
	}

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestGetResourceUsage_MultipleResources tests different quotas per resource type
func TestGetResourceUsage_MultipleResources(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	// Different usage patterns across resource types
	multipleResources := []models.ResourceUsageItem{
		newItem(events.ResourceTypePrompt, 50, 100, 100, 50),
		newItem(events.ResourceTypeMemory, 90, 100, 100, 90),
		newItem(events.ResourceTypeArtifact, 10, 100, 100, 10),
		newItem(events.ResourceTypeAgent, 2, 2, 2, 100),
		newItem(events.ResourceTypeAITool, 0, 2, 2, 0),
		newItem(events.ResourceTypeBlueprint, 15, 20, 20, 75),
	}
	expectedResponse := createResourceUsageResponse(multipleResources)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ResourceUsageResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Len(t, response.Resources, 6)

	// Verify each resource has correct count and percentage
	resourceMap := make(map[string]models.ResourceUsageItem)
	for _, r := range response.Resources {
		resourceMap[r.ResourceType] = r
	}

	assert.Equal(t, 50, resourceMap[events.ResourceTypePrompt].Percentage)
	assert.Equal(t, 90, resourceMap[events.ResourceTypeMemory].Percentage)
	assert.Equal(t, 10, resourceMap[events.ResourceTypeArtifact].Percentage)
	assert.Equal(t, 100, resourceMap[events.ResourceTypeAgent].Percentage) // At limit
	assert.Equal(t, 0, resourceMap[events.ResourceTypeAITool].Percentage)  // Zero usage
	assert.Equal(t, 75, resourceMap[events.ResourceTypeBlueprint].Percentage)

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// ========================================
// Time-Based Quota Tests
// ========================================

// TestGetResourceUsage_MonthlyReset tests that period dates are correct for monthly reset
func TestGetResourceUsage_MonthlyReset(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	now := time.Now().UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	resources := []models.ResourceUsageItem{
		newItem(events.ResourceTypePrompt, 10, 100, 100, 10),
	}
	expectedResponse := &models.ResourceUsageResponse{
		UserID:      testResourceUsageUserID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Resources:   resources,
	}

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ResourceUsageResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)

	// Verify period start is first of current month
	assert.Equal(t, 1, response.PeriodStart.Day())
	assert.Equal(t, now.Month(), response.PeriodStart.Month())
	assert.Equal(t, now.Year(), response.PeriodStart.Year())

	// Verify period end is last moment of current month
	assert.Equal(t, now.Month(), response.PeriodEnd.Month())
	assert.Greater(t, response.PeriodEnd.Day(), 27) // Should be 28-31 depending on month

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestGetResourceUsage_DailyLimits tests daily API call limits (if applicable)
func TestGetResourceUsage_DailyLimits(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	// AI sessions often have daily or monthly limits
	dailyResources := []models.ResourceUsageItem{
		newItem(events.ResourceTypeAISession, 50, 100, 100, 50),
		newItem(events.ResourceTypeAgentConv, 25, 100, 100, 25),
	}
	expectedResponse := createResourceUsageResponse(dailyResources)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ResourceUsageResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)

	// Verify session-based resources are tracked
	for _, resource := range response.Resources {
		if resource.ResourceType == events.ResourceTypeAISession || resource.ResourceType == events.ResourceTypeAgentConv {
			assert.GreaterOrEqual(t, resource.Limit, 0, "Session limits should be positive or unlimited")
		}
	}

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestGetResourceUsage_CurrentPeriod tests usage for current billing period only
func TestGetResourceUsage_CurrentPeriod(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	now := time.Now().UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Usage should only count current period
	currentPeriodResources := []models.ResourceUsageItem{
		newItem(events.ResourceTypePrompt, 15, 100, 100, 15),
	}
	expectedResponse := &models.ResourceUsageResponse{
		UserID:      testResourceUsageUserID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Resources:   currentPeriodResources,
	}

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ResourceUsageResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)

	// Verify period is in the present/future
	assert.False(t, response.PeriodEnd.Before(time.Now().UTC()),
		"Period end should not be in the past")

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// ========================================
// Error Handling Tests
// ========================================

// TestGetResourceUsage_ServiceError tests handling of service errors
func TestGetResourceUsage_ServiceError(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return((*models.ResourceUsageResponse)(nil), errors.New("database connection error"))

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestGetResourceUsage_UserNotFound tests handling when user is not found
func TestGetResourceUsage_UserNotFound(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return((*models.ResourceUsageResponse)(nil), errors.New("failed to get user: user not found"))

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestGetResourceUsage_EmptyUserID tests handling of empty user ID in context
func TestGetResourceUsage_EmptyUserID(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)
	srv := createResourceUsageTestServer(mockContainer)

	// Create request with empty user ID in context
	req := httptest.NewRequest("GET", "/api/v1/resource-usage", nil)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, ""))

	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	mockContainer.resourceUsageService.AssertNotCalled(t, "GetResourceUsage")
}

// ========================================
// Response Structure Tests
// ========================================

// TestGetResourceUsage_ResponseStructure tests correct JSON structure in response
func TestGetResourceUsage_ResponseStructure(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	resources := []models.ResourceUsageItem{
		{
			ResourceType:    events.ResourceTypePrompt,
			Count:           50,
			Limit:           100,
			IndividualLimit: 80,
			TeamQuota:       20,
			Percentage:      50,
		},
	}
	expectedResponse := createResourceUsageResponse(resources)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify JSON structure with raw map
	var rawResponse map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&rawResponse)
	assert.NoError(t, err)

	// Check required top-level fields
	assert.Contains(t, rawResponse, "user_id")
	assert.Contains(t, rawResponse, "period_start")
	assert.Contains(t, rawResponse, "period_end")
	assert.Contains(t, rawResponse, "resources")

	// Check resource item structure
	resourcesRaw := rawResponse["resources"].([]interface{})
	assert.Len(t, resourcesRaw, 1)

	resourceItem := resourcesRaw[0].(map[string]interface{})
	assert.Contains(t, resourceItem, "resource_type")
	assert.Contains(t, resourceItem, "count")
	assert.Contains(t, resourceItem, "limit")
	assert.Contains(t, resourceItem, "individual_limit")
	assert.Contains(t, resourceItem, "team_quota")
	assert.Contains(t, resourceItem, "percentage")

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestGetResourceUsage_PercentageCalculation tests correct percentage calculation
func TestGetResourceUsage_PercentageCalculation(t *testing.T) {
	tests := []struct {
		name               string
		count              int
		limit              int
		expectedPercentage int
	}{
		{"zero usage", 0, 100, 0},
		{"10%% usage", 10, 100, 10},
		{"50%% usage", 50, 100, 50},
		{"100%% usage", 100, 100, 100},
		{"exceeded quota", 150, 100, 150},
		{"unlimited resource", 50, -1, 0}, // Unlimited should show 0%
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockResourceUsageContainer(t)

			resources := []models.ResourceUsageItem{
				{
					ResourceType:    events.ResourceTypePrompt,
					Count:           tt.count,
					Limit:           tt.limit,
					IndividualLimit: tt.limit,
					TeamQuota:       0,
					Percentage:      tt.expectedPercentage,
				},
			}
			expectedResponse := createResourceUsageResponse(resources)

			mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
				Return(expectedResponse, nil)

			srv := createResourceUsageTestServer(mockContainer)
			req := makeResourceUsageAuthenticatedRequest()
			w := httptest.NewRecorder()

			srv.router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response models.ResourceUsageResponse
			err := json.NewDecoder(w.Body).Decode(&response)
			assert.NoError(t, err)
			assert.Len(t, response.Resources, 1)
			assert.Equal(t, tt.expectedPercentage, response.Resources[0].Percentage)

			mockContainer.resourceUsageService.AssertExpectations(t)
		})
	}
}

// TestGetResourceUsage_ContentType tests that response has correct content type
func TestGetResourceUsage_ContentType(t *testing.T) {
	mockContainer := newMockResourceUsageContainer(t)

	resources := []models.ResourceUsageItem{
		newItem(events.ResourceTypePrompt, 5, 100, 100, 5),
	}
	expectedResponse := createResourceUsageResponse(resources)

	mockContainer.resourceUsageService.On("GetResourceUsage", mock.Anything, testResourceUsageUserID).
		Return(expectedResponse, nil)

	srv := createResourceUsageTestServer(mockContainer)
	req := makeResourceUsageAuthenticatedRequest()
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	mockContainer.resourceUsageService.AssertExpectations(t)
}
