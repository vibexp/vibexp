package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// MockFeedQuotaContainer is a mock container for feed quota integration tests.
// It embeds BaseMockContainer and only overrides the services it needs.
type MockFeedQuotaContainer struct {
	BaseMockContainer
	FeedServiceMock          services.FeedServiceInterface
	FeedItemServiceMock      services.FeedItemServiceInterface
	FeedItemReplyServiceMock services.FeedItemReplyServiceInterface
	ResourceUsageServiceMock services.ResourceUsageServiceInterface
	AuthServiceMock          services.AuthServiceInterface
	APIKeyServiceMock        services.APIKeyServiceInterface
	TeamServiceMock          services.TeamServiceInterface
}

func (m *MockFeedQuotaContainer) FeedService() services.FeedServiceInterface {
	return m.FeedServiceMock
}

func (m *MockFeedQuotaContainer) FeedItemService() services.FeedItemServiceInterface {
	return m.FeedItemServiceMock
}

func (m *MockFeedQuotaContainer) FeedItemReplyService() services.FeedItemReplyServiceInterface {
	return m.FeedItemReplyServiceMock
}

func (m *MockFeedQuotaContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return m.ResourceUsageServiceMock
}

func (m *MockFeedQuotaContainer) AuthService() services.AuthServiceInterface {
	return m.AuthServiceMock
}

func (m *MockFeedQuotaContainer) APIKeyService() services.APIKeyServiceInterface {
	return m.APIKeyServiceMock
}

func (m *MockFeedQuotaContainer) TeamService() services.TeamServiceInterface {
	return m.TeamServiceMock
}

// setupFeedQuotaTestServer creates a test server wired with the given mocks.
func setupFeedQuotaTestServer(
	t *testing.T,
	resourceSvc *servicesmocks.MockResourceUsageServiceInterface,
	feedSvc *servicesmocks.MockFeedServiceInterface,
	feedItemSvc *servicesmocks.MockFeedItemServiceInterface,
	feedItemReplySvc *servicesmocks.MockFeedItemReplyServiceInterface,
) *Server {
	t.Helper()

	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)
	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_feed_quota_key").
		Return(&models.APIKey{
			ID:     "api-key-quota",
			UserID: "user-quota",
		}, nil)

	// Mock team membership validation
	mockTeamService.On(
		"IsUserMemberOfTeam", mock.Anything, "user-quota", feedTestTeamID,
	).Return(true, nil).Maybe()

	mockContainer := &MockFeedQuotaContainer{
		FeedServiceMock:          feedSvc,
		FeedItemServiceMock:      feedItemSvc,
		FeedItemReplyServiceMock: feedItemReplySvc,
		ResourceUsageServiceMock: resourceSvc,
		APIKeyServiceMock:        mockAPIKeyService,
		TeamServiceMock:          mockTeamService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer
	return srv
}

// createFeedQuotaRequest creates an authenticated POST HTTP request for feed quota tests.
func createFeedQuotaRequest(path, body string) *http.Request {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req := httptest.NewRequest(http.MethodPost, path, bodyReader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer vxk_test_feed_quota_key")
	return req
}

// ─────────────────────────────────────────────────────────────────────────────
// TestHandleCreateFeed_ResourceLimitExceeded
// ─────────────────────────────────────────────────────────────────────────────

// TestHandleCreateFeed_ResourceLimitExceeded verifies that a user who has reached
// the feed limit receives HTTP 403 with resource_limit_exceeded error code.
func TestHandleCreateFeed_ResourceLimitExceeded(t *testing.T) {
	mockResourceSvc := servicesmocks.NewMockResourceUsageServiceInterface(t)
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	mockFeedItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockFeedItemReplySvc := servicesmocks.NewMockFeedItemReplyServiceInterface(t)

	// Resource limit check returns false (limit exceeded) for feed
	mockResourceSvc.On("CheckResourceLimit", mock.Anything, "user-quota", "feed").
		Return(false, nil)

	srv := setupFeedQuotaTestServer(t, mockResourceSvc, mockFeedSvc, mockFeedItemSvc, mockFeedItemReplySvc)

	reqBody := `{"name": "My Second Feed", "description": "A feed that exceeds the limit"}`
	req := createFeedQuotaRequest(
		"/api/v1/"+feedTestTeamID+"/feeds",
		reqBody,
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "RESOURCE_LIMIT_EXCEEDED")

	mockResourceSvc.AssertExpectations(t)
	// CreateFeed must NOT be called when limit is exceeded
	mockFeedSvc.AssertNotCalled(t, "CreateFeed")
}

// TestHandleCreateFeed_ResourceLimitCheckError verifies that when the resource limit
// check returns an error, the handler returns HTTP 500.
func TestHandleCreateFeed_ResourceLimitCheckError(t *testing.T) {
	mockResourceSvc := servicesmocks.NewMockResourceUsageServiceInterface(t)
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	mockFeedItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockFeedItemReplySvc := servicesmocks.NewMockFeedItemReplyServiceInterface(t)

	mockResourceSvc.On("CheckResourceLimit", mock.Anything, "user-quota", "feed").
		Return(false, assert.AnError)

	srv := setupFeedQuotaTestServer(t, mockResourceSvc, mockFeedSvc, mockFeedItemSvc, mockFeedItemReplySvc)

	reqBody := `{"name": "My Feed"}`
	req := createFeedQuotaRequest(
		"/api/v1/"+feedTestTeamID+"/feeds",
		reqBody,
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "INTERNAL_ERROR")

	mockResourceSvc.AssertExpectations(t)
	mockFeedSvc.AssertNotCalled(t, "CreateFeed")
}

// ─────────────────────────────────────────────────────────────────────────────
// TestHandleCreateFeedItem_ResourceLimitExceeded
// ─────────────────────────────────────────────────────────────────────────────

// TestHandleCreateFeedItem_ResourceLimitExceeded verifies that a user who has reached
// the feed_item limit receives HTTP 403 with resource_limit_exceeded error code.
func TestHandleCreateFeedItem_ResourceLimitExceeded(t *testing.T) {
	mockResourceSvc := servicesmocks.NewMockResourceUsageServiceInterface(t)
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	mockFeedItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockFeedItemReplySvc := servicesmocks.NewMockFeedItemReplyServiceInterface(t)

	// Resource limit check returns false (limit exceeded) for feed_item
	mockResourceSvc.On("CheckResourceLimit", mock.Anything, "user-quota", "feed_item").
		Return(false, nil)

	srv := setupFeedQuotaTestServer(t, mockResourceSvc, mockFeedSvc, mockFeedItemSvc, mockFeedItemReplySvc)

	reqBody := `{
		"title": "My Feed Item",
		"content": "Some content for the feed item",
		"ai_assistant_name": "Claude"
	}`
	req := createFeedQuotaRequest(
		"/api/v1/"+feedTestTeamID+"/feeds/"+feedTestFeedID+"/items",
		reqBody,
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "RESOURCE_LIMIT_EXCEEDED")

	mockResourceSvc.AssertExpectations(t)
	// CreateFeedItem must NOT be called when limit is exceeded
	mockFeedItemSvc.AssertNotCalled(t, "CreateFeedItem")
}

// TestHandleCreateFeedItem_ResourceLimitCheckError verifies that when the resource limit
// check returns an error, the handler returns HTTP 500.
func TestHandleCreateFeedItem_ResourceLimitCheckError(t *testing.T) {
	mockResourceSvc := servicesmocks.NewMockResourceUsageServiceInterface(t)
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	mockFeedItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockFeedItemReplySvc := servicesmocks.NewMockFeedItemReplyServiceInterface(t)

	mockResourceSvc.On("CheckResourceLimit", mock.Anything, "user-quota", "feed_item").
		Return(false, assert.AnError)

	srv := setupFeedQuotaTestServer(t, mockResourceSvc, mockFeedSvc, mockFeedItemSvc, mockFeedItemReplySvc)

	reqBody := `{
		"title": "My Feed Item",
		"content": "Some content",
		"ai_assistant_name": "Claude"
	}`
	req := createFeedQuotaRequest(
		"/api/v1/"+feedTestTeamID+"/feeds/"+feedTestFeedID+"/items",
		reqBody,
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "INTERNAL_ERROR")

	mockResourceSvc.AssertExpectations(t)
	mockFeedItemSvc.AssertNotCalled(t, "CreateFeedItem")
}

// ─────────────────────────────────────────────────────────────────────────────
// TestHandleCreateFeedItemReply_ResourceLimitExceeded
// ─────────────────────────────────────────────────────────────────────────────

// TestHandleCreateFeedItemReply_ResourceLimitExceeded verifies that a user who has
// reached the feed_item limit receives HTTP 403 when posting a reply.
// Replies count against the same feed_item quota.
func TestHandleCreateFeedItemReply_ResourceLimitExceeded(t *testing.T) {
	mockResourceSvc := servicesmocks.NewMockResourceUsageServiceInterface(t)
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	mockFeedItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockFeedItemReplySvc := servicesmocks.NewMockFeedItemReplyServiceInterface(t)

	// Resource limit check returns false (limit exceeded) for feed_item
	mockResourceSvc.On("CheckResourceLimit", mock.Anything, "user-quota", "feed_item").
		Return(false, nil)

	srv := setupFeedQuotaTestServer(t, mockResourceSvc, mockFeedSvc, mockFeedItemSvc, mockFeedItemReplySvc)

	reqBody := `{"content": "A reply to a feed item"}`
	req := createFeedQuotaRequest(
		"/api/v1/"+feedTestTeamID+"/feed-items/"+feedTestItemID+"/replies",
		reqBody,
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "RESOURCE_LIMIT_EXCEEDED")

	mockResourceSvc.AssertExpectations(t)
	// CreateReply must NOT be called when limit is exceeded
	mockFeedItemReplySvc.AssertNotCalled(t, "CreateReply")
}

// TestHandleCreateFeedItemReply_ResourceLimitCheckError verifies that when the resource
// limit check returns an error while creating a reply, the handler returns HTTP 500.
func TestHandleCreateFeedItemReply_ResourceLimitCheckError(t *testing.T) {
	mockResourceSvc := servicesmocks.NewMockResourceUsageServiceInterface(t)
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	mockFeedItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockFeedItemReplySvc := servicesmocks.NewMockFeedItemReplyServiceInterface(t)

	mockResourceSvc.On("CheckResourceLimit", mock.Anything, "user-quota", "feed_item").
		Return(false, assert.AnError)

	srv := setupFeedQuotaTestServer(t, mockResourceSvc, mockFeedSvc, mockFeedItemSvc, mockFeedItemReplySvc)

	reqBody := `{"content": "A reply"}`
	req := createFeedQuotaRequest(
		"/api/v1/"+feedTestTeamID+"/feed-items/"+feedTestItemID+"/replies",
		reqBody,
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "INTERNAL_ERROR")

	mockResourceSvc.AssertExpectations(t)
	mockFeedItemReplySvc.AssertNotCalled(t, "CreateReply")
}
