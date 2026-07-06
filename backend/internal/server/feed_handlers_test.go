package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/container"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// ─────────────────────────────────────────────────────────────────────────────
// Mock container for feed tests
// ─────────────────────────────────────────────────────────────────────────────

// MockFeedContainer is a mock container for feed handler tests.
type MockFeedContainer struct {
	BaseMockContainer
	FeedServiceMock          services.FeedServiceInterface
	FeedItemServiceMock      services.FeedItemServiceInterface
	FeedItemReplyServiceMock services.FeedItemReplyServiceInterface
	ResourceUsageServiceMock services.ResourceUsageServiceInterface
	EmbeddingServiceMock     services.EmbeddingServiceInterface
}

func (m *MockFeedContainer) FeedService() services.FeedServiceInterface {
	return m.FeedServiceMock
}

func (m *MockFeedContainer) FeedItemService() services.FeedItemServiceInterface {
	return m.FeedItemServiceMock
}

func (m *MockFeedContainer) FeedItemReplyService() services.FeedItemReplyServiceInterface {
	return m.FeedItemReplyServiceMock
}

func (m *MockFeedContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return m.ResourceUsageServiceMock
}

func (m *MockFeedContainer) EmbeddingService() services.EmbeddingServiceInterface {
	return m.EmbeddingServiceMock
}

// Compile-time check
var _ container.Container = (*MockFeedContainer)(nil)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func newFeedTestServer(t *testing.T, fc *MockFeedContainer) *Server {
	t.Helper()
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = fc
	return srv
}

// newAllowedResourceUsageMock returns a ResourceUsageService mock that allows any resource limit check.
func newAllowedResourceUsageMock(t *testing.T) *servicesmocks.MockResourceUsageServiceInterface {
	t.Helper()
	m := servicesmocks.NewMockResourceUsageServiceInterface(t)
	m.On("CheckResourceLimit", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return(true, nil).Maybe()
	return m
}

// addFeedURLParams sets chi route params on the request context.
func addFeedURLParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// authenticatedFeedRequest creates a request with the user ID already set in context.
func authenticatedFeedRequest(method, path, body, userID string) *http.Request {
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, path, reader)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, http.NoBody)
	}
	ctx := context.WithValue(req.Context(), contextKeyUserID, userID)
	return req.WithContext(ctx)
}

const (
	feedTestTeamID    = "550e8400-e29b-41d4-a716-446655440000"
	feedTestFeedID    = "660e8400-e29b-41d4-a716-446655440001"
	feedTestItemID    = "770e8400-e29b-41d4-a716-446655440002"
	feedTestUserID    = "user-abc"
	feedTestProjectID = "880e8400-e29b-41d4-a716-446655440003"

	// Derived path constants to keep test lines within 120 chars.
	feedItemsPath = "/api/v1/" + feedTestTeamID + "/feeds/" + feedTestFeedID + "/items"
	archivePath   = "/api/v1/" + feedTestTeamID + "/feed-items/" + feedTestItemID + "/archive"
	unarchivePath = "/api/v1/" + feedTestTeamID + "/feed-items/" + feedTestItemID + "/unarchive"
)

func sampleFeed() *models.Feed {
	desc := "Test feed description"
	return &models.Feed{
		ID:              feedTestFeedID,
		TeamID:          feedTestTeamID,
		Name:            "Test Feed",
		Description:     &desc,
		CreatedByUserID: feedTestUserID,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

func sampleFeedItem() *models.FeedItem {
	projID := "880e8400-e29b-41d4-a716-446655440003"
	return &models.FeedItem{
		ID:              feedTestItemID,
		TeamID:          feedTestTeamID,
		FeedID:          feedTestFeedID,
		ProjectID:       &projID,
		Title:           "Test Item",
		Content:         "This is test content for the feed item.",
		Excerpt:         "This is test content for the feed item.",
		AIAssistantName: "claude-test",
		PostedByUserID:  feedTestUserID,
		PostedAt:        time.Now(),
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Unauthorized access (no auth header) — full server routing
// ─────────────────────────────────────────────────────────────────────────────

//nolint:funlen // Comprehensive unauthorized route coverage
func TestFeedHandlers_Unauthorized(t *testing.T) {
	srv := testServer()

	routes := []struct {
		name   string
		method string
		path   string
	}{
		{"Create Feed", "POST", "/api/v1/" + feedTestTeamID + "/feeds"},
		{"List Feeds", "GET", "/api/v1/" + feedTestTeamID + "/feeds"},
		{"Get Feed", "GET", "/api/v1/" + feedTestTeamID + "/feeds/" + feedTestFeedID},
		{"Update Feed", "PUT", "/api/v1/" + feedTestTeamID + "/feeds/" + feedTestFeedID},
		{"Delete Feed", "DELETE", "/api/v1/" + feedTestTeamID + "/feeds/" + feedTestFeedID},
		{"Create Feed Item", "POST", "/api/v1/" + feedTestTeamID + "/feeds/" + feedTestFeedID + "/items"},
		{"List Items by Feed", "GET", "/api/v1/" + feedTestTeamID + "/feeds/" + feedTestFeedID + "/items"},
		{"List Feed Items", "GET", "/api/v1/" + feedTestTeamID + "/feed-items"},
		{"Get Feed Item", "GET", "/api/v1/" + feedTestTeamID + "/feed-items/" + feedTestItemID},
		{"Archive Feed Item", "POST", "/api/v1/" + feedTestTeamID + "/feed-items/" + feedTestItemID + "/archive"},
		{"Unarchive Feed Item", "POST", "/api/v1/" + feedTestTeamID + "/feed-items/" + feedTestItemID + "/unarchive"},
		{"Delete Feed Item", "DELETE", "/api/v1/" + feedTestTeamID + "/feed-items/" + feedTestItemID},
	}

	for _, tt := range routes {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, http.NoBody)
			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusUnauthorized, rr.Code)
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// handleCreateFeed
// ─────────────────────────────────────────────────────────────────────────────

func TestHandleCreateFeed_Success(t *testing.T) {
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	mockResourceSvc := newAllowedResourceUsageMock(t)
	feed := sampleFeed()

	mockFeedSvc.On("CreateFeed", mock.Anything, feedTestUserID, feedTestTeamID,
		mock.MatchedBy(func(r *models.CreateFeedRequest) bool {
			return r.Name == "Test Feed"
		}),
	).Return(feed, nil)

	fc := &MockFeedContainer{FeedServiceMock: mockFeedSvc, ResourceUsageServiceMock: mockResourceSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("POST", "/api/v1/"+feedTestTeamID+"/feeds",
		`{"name":"Test Feed","description":"Test feed description"}`, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeed(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	var got models.Feed
	assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, feedTestFeedID, got.ID)
}

func TestHandleCreateFeed_MissingName(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("POST", "/api/v1/"+feedTestTeamID+"/feeds",
		`{"description":"No name here"}`, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeed(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleCreateFeed_EmptyName(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("POST", "/api/v1/"+feedTestTeamID+"/feeds",
		`{"name":"  "}`, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeed(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleCreateFeed_NameTooLong(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	longName := strings.Repeat("a", 256)
	req := authenticatedFeedRequest("POST", "/api/v1/"+feedTestTeamID+"/feeds",
		`{"name":"`+longName+`"}`, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeed(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleCreateFeed_DescriptionTooLong(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	longDesc := strings.Repeat("b", 1001)
	req := authenticatedFeedRequest("POST", "/api/v1/"+feedTestTeamID+"/feeds",
		`{"name":"Valid Name","description":"`+longDesc+`"}`, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeed(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleCreateFeed_DuplicateName_Returns409(t *testing.T) {
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	mockResourceSvc := newAllowedResourceUsageMock(t)
	mockFeedSvc.On("CreateFeed", mock.Anything, feedTestUserID, feedTestTeamID, mock.Anything).
		Return((*models.Feed)(nil), errors.New("feed already exists"))

	fc := &MockFeedContainer{FeedServiceMock: mockFeedSvc, ResourceUsageServiceMock: mockResourceSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("POST", "/api/v1/"+feedTestTeamID+"/feeds",
		`{"name":"Duplicate Feed"}`, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeed(rr, req)
	assert.Equal(t, http.StatusConflict, rr.Code)
}

func TestHandleCreateFeed_NotTeamMember_Returns403(t *testing.T) {
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	mockResourceSvc := newAllowedResourceUsageMock(t)
	mockFeedSvc.On("CreateFeed", mock.Anything, feedTestUserID, feedTestTeamID, mock.Anything).
		Return((*models.Feed)(nil), errors.New("user is not a member of the specified team"))

	fc := &MockFeedContainer{FeedServiceMock: mockFeedSvc, ResourceUsageServiceMock: mockResourceSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("POST", "/api/v1/"+feedTestTeamID+"/feeds",
		`{"name":"My Feed"}`, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeed(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestHandleCreateFeed_BadJSON(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("POST", "/api/v1/"+feedTestTeamID+"/feeds",
		`{"name": bad}`, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeed(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// ─────────────────────────────────────────────────────────────────────────────
// handleGetFeed
// ─────────────────────────────────────────────────────────────────────────────

func TestHandleGetFeed_Success(t *testing.T) {
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	feed := sampleFeed()

	mockFeedSvc.On("GetFeed", mock.Anything, feedTestUserID, feedTestTeamID, feedTestFeedID).
		Return(feed, nil)

	fc := &MockFeedContainer{FeedServiceMock: mockFeedSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("GET", "/api/v1/"+feedTestTeamID+"/feeds/"+feedTestFeedID, "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleGetFeed(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var got models.Feed
	assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, feedTestFeedID, got.ID)
}

func TestHandleGetFeed_NotFound(t *testing.T) {
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	mockFeedSvc.On("GetFeed", mock.Anything, feedTestUserID, feedTestTeamID, feedTestFeedID).
		Return((*models.Feed)(nil), errors.New("feed not found"))

	fc := &MockFeedContainer{FeedServiceMock: mockFeedSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("GET", "/api/v1/"+feedTestTeamID+"/feeds/"+feedTestFeedID, "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleGetFeed(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandleGetFeed_InvalidUUID(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("GET", "/api/v1/"+feedTestTeamID+"/feeds/not-a-uuid", "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": "not-a-uuid"})
	rr := httptest.NewRecorder()

	srv.handleGetFeed(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// ─────────────────────────────────────────────────────────────────────────────
// handleListFeeds
// ─────────────────────────────────────────────────────────────────────────────

func TestHandleListFeeds_Success(t *testing.T) {
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)

	expected := &models.FeedListResponse{
		Feeds:      []models.Feed{*sampleFeed()},
		TotalCount: 1,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}
	// Feed handlers default to limit=20 (per OpenAPI spec)
	mockFeedSvc.On("ListFeeds", mock.Anything, feedTestUserID,
		mock.MatchedBy(func(f services.FeedFilters) bool {
			return f.TeamID == feedTestTeamID && f.Page == 1 && f.Limit == 20
		}),
	).Return(expected, nil)

	fc := &MockFeedContainer{FeedServiceMock: mockFeedSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("GET", "/api/v1/"+feedTestTeamID+"/feeds", "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleListFeeds(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var got models.FeedListResponse
	assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, 1, got.TotalCount)
}

func TestHandleListFeeds_Pagination(t *testing.T) {
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)

	// 5 feeds, page 1, limit 2 => 3 pages
	expected := &models.FeedListResponse{
		Feeds:      []models.Feed{*sampleFeed(), *sampleFeed()},
		TotalCount: 5,
		Page:       1,
		PerPage:    2,
		TotalPages: 3,
	}
	mockFeedSvc.On("ListFeeds", mock.Anything, feedTestUserID,
		mock.MatchedBy(func(f services.FeedFilters) bool {
			return f.TeamID == feedTestTeamID && f.Page == 1 && f.Limit == 2
		}),
	).Return(expected, nil)

	fc := &MockFeedContainer{FeedServiceMock: mockFeedSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("GET", "/api/v1/"+feedTestTeamID+"/feeds?page=1&limit=2", "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleListFeeds(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var got models.FeedListResponse
	assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, 5, got.TotalCount)
	assert.Equal(t, 3, got.TotalPages)
	assert.Equal(t, 2, got.PerPage)
}

func TestHandleListFeeds_ServiceError(t *testing.T) {
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	mockFeedSvc.On("ListFeeds", mock.Anything, feedTestUserID, mock.Anything).
		Return((*models.FeedListResponse)(nil), errors.New("db error"))

	fc := &MockFeedContainer{FeedServiceMock: mockFeedSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("GET", "/api/v1/"+feedTestTeamID+"/feeds", "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleListFeeds(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// ─────────────────────────────────────────────────────────────────────────────
// handleUpdateFeed
// ─────────────────────────────────────────────────────────────────────────────

func TestHandleUpdateFeed_Success(t *testing.T) {
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	updated := sampleFeed()
	updated.Name = "Updated Name"

	mockFeedSvc.On("UpdateFeed", mock.Anything, feedTestUserID, feedTestTeamID, feedTestFeedID, mock.Anything).
		Return(updated, nil)

	fc := &MockFeedContainer{FeedServiceMock: mockFeedSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("PUT", "/api/v1/"+feedTestTeamID+"/feeds/"+feedTestFeedID,
		`{"name":"Updated Name"}`, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleUpdateFeed(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var got models.Feed
	assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, "Updated Name", got.Name)
}

func TestHandleUpdateFeed_EmptyNameRejected(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("PUT", "/api/v1/"+feedTestTeamID+"/feeds/"+feedTestFeedID,
		`{"name":"  "}`, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleUpdateFeed(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleUpdateFeed_NotFound(t *testing.T) {
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	mockFeedSvc.On("UpdateFeed", mock.Anything, feedTestUserID, feedTestTeamID, feedTestFeedID, mock.Anything).
		Return((*models.Feed)(nil), errors.New("feed not found"))

	fc := &MockFeedContainer{FeedServiceMock: mockFeedSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("PUT", "/api/v1/"+feedTestTeamID+"/feeds/"+feedTestFeedID,
		`{"name":"New Name"}`, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleUpdateFeed(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandleUpdateFeed_DuplicateNameConflict(t *testing.T) {
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	mockFeedSvc.On("UpdateFeed", mock.Anything, feedTestUserID, feedTestTeamID, feedTestFeedID, mock.Anything).
		Return((*models.Feed)(nil), errors.New("feed already exists with that name"))

	fc := &MockFeedContainer{FeedServiceMock: mockFeedSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("PUT", "/api/v1/"+feedTestTeamID+"/feeds/"+feedTestFeedID,
		`{"name":"Taken Name"}`, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleUpdateFeed(rr, req)
	assert.Equal(t, http.StatusConflict, rr.Code)
}

// ─────────────────────────────────────────────────────────────────────────────
// handleDeleteFeed (with cascade semantics test)
// ─────────────────────────────────────────────────────────────────────────────

func TestHandleDeleteFeed_Success(t *testing.T) {
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	mockFeedSvc.On("DeleteFeed", mock.Anything, feedTestUserID, feedTestTeamID, feedTestFeedID).
		Return(nil)

	fc := &MockFeedContainer{FeedServiceMock: mockFeedSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("DELETE", "/api/v1/"+feedTestTeamID+"/feeds/"+feedTestFeedID, "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleDeleteFeed(rr, req)
	assert.Equal(t, http.StatusNoContent, rr.Code)
}

// TestHandleDeleteFeed_CascadesItems verifies that deleting a feed (service returns no error)
// results in 204 — the cascade is enforced at the DB level (not tested here) but the handler
// must forward the call and return the correct status.
func TestHandleDeleteFeed_CascadesItems(t *testing.T) {
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	mockFeedSvc.On("DeleteFeed", mock.Anything, feedTestUserID, feedTestTeamID, feedTestFeedID).
		Return(nil)

	fc := &MockFeedContainer{FeedServiceMock: mockFeedSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("DELETE", "/api/v1/"+feedTestTeamID+"/feeds/"+feedTestFeedID, "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleDeleteFeed(rr, req)
	assert.Equal(t, http.StatusNoContent, rr.Code)
	// Service must have been called exactly once
	mockFeedSvc.AssertExpectations(t)
}

func TestHandleDeleteFeed_NotFound(t *testing.T) {
	mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
	mockFeedSvc.On("DeleteFeed", mock.Anything, feedTestUserID, feedTestTeamID, feedTestFeedID).
		Return(errors.New("feed not found"))

	fc := &MockFeedContainer{FeedServiceMock: mockFeedSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("DELETE", "/api/v1/"+feedTestTeamID+"/feeds/"+feedTestFeedID, "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleDeleteFeed(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// ─────────────────────────────────────────────────────────────────────────────
// handleCreateFeedItem
// ─────────────────────────────────────────────────────────────────────────────

func TestHandleCreateFeedItem_Success(t *testing.T) {
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockResourceSvc := newAllowedResourceUsageMock(t)
	item := sampleFeedItem()

	mockItemSvc.On("CreateFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestFeedID,
		mock.MatchedBy(func(r *models.CreateFeedItemRequest) bool {
			return r.Title == "Test Item" && r.AIAssistantName == "claude-test"
		}),
	).Return(item, nil)

	fc := &MockFeedContainer{FeedItemServiceMock: mockItemSvc, ResourceUsageServiceMock: mockResourceSvc}
	srv := newFeedTestServer(t, fc)

	body := `{"title":"Test Item","content":"This is test content.","ai_assistant_name":"claude-test"}`
	req := authenticatedFeedRequest("POST", feedItemsPath, body, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeedItem(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	var got models.FeedItem
	assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, feedTestItemID, got.ID)
}

func TestHandleCreateFeedItem_MissingTitle(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	path := feedItemsPath
	body := `{"content":"Some content","ai_assistant_name":"claude"}`
	req := authenticatedFeedRequest("POST", path, body, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeedItem(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleCreateFeedItem_MissingContent(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	path := feedItemsPath
	body := `{"title":"Title","ai_assistant_name":"claude"}`
	req := authenticatedFeedRequest("POST", path, body, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeedItem(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleCreateFeedItem_MissingAIAssistantName(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	path := feedItemsPath
	body := `{"title":"Title","content":"Some content"}`
	req := authenticatedFeedRequest("POST", path, body, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeedItem(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleCreateFeedItem_AIAssistantNameTooLong(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	longName := strings.Repeat("x", 31)
	body := `{"title":"Title","content":"Some content","ai_assistant_name":"` + longName + `"}`
	path := feedItemsPath
	req := authenticatedFeedRequest("POST", path, body, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeedItem(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleCreateFeedItem_ContentExceedsMaxSize(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	// 200 KB + 1 byte
	bigContent := strings.Repeat("x", 204801)
	body := `{"title":"Title","content":"` + bigContent + `","ai_assistant_name":"claude"}`
	path := feedItemsPath
	req := authenticatedFeedRequest("POST", path, body, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeedItem(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleCreateFeedItem_InvalidProjectID(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	body := `{"title":"Title","content":"Content","ai_assistant_name":"claude","project_id":"not-uuid"}`
	path := feedItemsPath
	req := authenticatedFeedRequest("POST", path, body, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeedItem(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleCreateFeedItem_CrossTeamProject_Returns403(t *testing.T) {
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockResourceSvc := newAllowedResourceUsageMock(t)
	mockItemSvc.On("CreateFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestFeedID, mock.Anything).
		Return((*models.FeedItem)(nil), errors.New("project does not belong to the specified team"))

	fc := &MockFeedContainer{FeedItemServiceMock: mockItemSvc, ResourceUsageServiceMock: mockResourceSvc}
	srv := newFeedTestServer(t, fc)

	body := `{"title":"T","content":"C","ai_assistant_name":"ai","project_id":"` + feedTestProjectID + `"}`
	path := feedItemsPath
	req := authenticatedFeedRequest("POST", path, body, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeedItem(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestHandleCreateFeedItem_ProjectNotFound_Returns400(t *testing.T) {
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockResourceSvc := newAllowedResourceUsageMock(t)
	mockItemSvc.On("CreateFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestFeedID, mock.Anything).
		Return((*models.FeedItem)(nil), errors.New("project not found"))

	fc := &MockFeedContainer{FeedItemServiceMock: mockItemSvc, ResourceUsageServiceMock: mockResourceSvc}
	srv := newFeedTestServer(t, fc)

	body := `{"title":"T","content":"C","ai_assistant_name":"ai","project_id":"` + feedTestProjectID + `"}`
	path := feedItemsPath
	req := authenticatedFeedRequest("POST", path, body, feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleCreateFeedItem(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// ─────────────────────────────────────────────────────────────────────────────
// handleListFeedItems and handleListFeedItemsByFeed — filters
// ─────────────────────────────────────────────────────────────────────────────

func TestHandleListFeedItems_Success(t *testing.T) {
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)

	expected := &models.FeedItemListResponse{
		Items:      []models.FeedItem{*sampleFeedItem()},
		TotalCount: 1,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}

	// Default archived=false filter; feed handlers default to limit=20
	f := false
	mockItemSvc.On("ListFeedItems", mock.Anything, feedTestUserID,
		mock.MatchedBy(func(fi services.FeedItemFilters) bool {
			return fi.TeamID == feedTestTeamID && fi.Page == 1 && fi.Limit == 20 &&
				fi.Archived != nil && !*fi.Archived
		}),
	).Return(expected, nil)
	mockItemSvc.On("EnrichWithReplyCounts", mock.Anything, feedTestTeamID, expected.Items).
		Return(expected.Items, nil)

	fc := &MockFeedContainer{FeedItemServiceMock: mockItemSvc}
	srv := newFeedTestServer(t, fc)
	_ = f // suppress unused warning

	req := authenticatedFeedRequest("GET", "/api/v1/"+feedTestTeamID+"/feed-items", "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleListFeedItems(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var got models.FeedItemListResponse
	assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, 1, got.TotalCount)
}

func TestHandleListFeedItems_AllFilters(t *testing.T) {
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)

	expected := &models.FeedItemListResponse{
		Items:      []models.FeedItem{},
		TotalCount: 0,
		Page:       1,
		PerPage:    10,
		TotalPages: 0,
	}

	projID := "880e8400-e29b-41d4-a716-446655440003"
	assistantName := "claude-sonnet"
	trueVal := true
	mockItemSvc.On("ListFeedItems", mock.Anything, feedTestUserID,
		mock.MatchedBy(func(fi services.FeedItemFilters) bool {
			return fi.TeamID == feedTestTeamID &&
				fi.FeedID != nil && *fi.FeedID == feedTestFeedID &&
				fi.ProjectID != nil && *fi.ProjectID == projID &&
				fi.AIAssistantName != nil && *fi.AIAssistantName == assistantName &&
				fi.Archived != nil && *fi.Archived == true
		}),
	).Return(expected, nil)
	mockItemSvc.On("EnrichWithReplyCounts", mock.Anything, feedTestTeamID, expected.Items).
		Return(expected.Items, nil)

	fc := &MockFeedContainer{FeedItemServiceMock: mockItemSvc}
	srv := newFeedTestServer(t, fc)
	_ = trueVal

	url := "/api/v1/" + feedTestTeamID + "/feed-items?feed_id=" + feedTestFeedID +
		"&project_id=" + projID +
		"&ai_assistant_name=" + assistantName +
		"&archived=true&page=1&limit=10"
	req := authenticatedFeedRequest("GET", url, "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleListFeedItems(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleListFeedItems_ArchivedAll(t *testing.T) {
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)

	expected := &models.FeedItemListResponse{Items: []models.FeedItem{}, TotalCount: 0, Page: 1, PerPage: 20}
	// "all" => Archived is nil
	mockItemSvc.On("ListFeedItems", mock.Anything, feedTestUserID,
		mock.MatchedBy(func(fi services.FeedItemFilters) bool {
			return fi.TeamID == feedTestTeamID && fi.Archived == nil
		}),
	).Return(expected, nil)
	mockItemSvc.On("EnrichWithReplyCounts", mock.Anything, feedTestTeamID, expected.Items).
		Return(expected.Items, nil)

	fc := &MockFeedContainer{FeedItemServiceMock: mockItemSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("GET", "/api/v1/"+feedTestTeamID+"/feed-items?archived=all", "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleListFeedItems(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleListFeedItemsByFeed_Success(t *testing.T) {
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)

	expected := &models.FeedItemListResponse{
		Items:      []models.FeedItem{*sampleFeedItem()},
		TotalCount: 1,
		Page:       1,
		PerPage:    10,
		TotalPages: 1,
	}

	mockItemSvc.On("ListFeedItems", mock.Anything, feedTestUserID,
		mock.MatchedBy(func(fi services.FeedItemFilters) bool {
			return fi.TeamID == feedTestTeamID && fi.FeedID != nil && *fi.FeedID == feedTestFeedID
		}),
	).Return(expected, nil)
	mockItemSvc.On("EnrichWithReplyCounts", mock.Anything, feedTestTeamID, expected.Items).
		Return(expected.Items, nil)

	fc := &MockFeedContainer{FeedItemServiceMock: mockItemSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("GET", "/api/v1/"+feedTestTeamID+"/feeds/"+feedTestFeedID+"/items", "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleListFeedItemsByFeed(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var got models.FeedItemListResponse
	assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, 1, got.TotalCount)
}

// ─────────────────────────────────────────────────────────────────────────────
// handleGetFeedItem
// ─────────────────────────────────────────────────────────────────────────────

func TestHandleGetFeedItem_Success(t *testing.T) {
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	item := sampleFeedItem()
	// The service enriches the single item with its reply count (#101); the
	// handler must carry it through to the response.
	item.ReplyCount = 3

	mockItemSvc.On("GetFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return(item, nil)

	fc := &MockFeedContainer{FeedItemServiceMock: mockItemSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("GET", "/api/v1/"+feedTestTeamID+"/feed-items/"+feedTestItemID, "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "item_id": feedTestItemID})
	rr := httptest.NewRecorder()

	srv.handleGetFeedItem(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var got models.FeedItem
	assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, feedTestItemID, got.ID)
	assert.Equal(t, 3, got.ReplyCount)
}

func TestHandleGetFeedItem_NotFound(t *testing.T) {
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockItemSvc.On("GetFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return((*models.FeedItem)(nil), errors.New("feed item not found"))

	fc := &MockFeedContainer{FeedItemServiceMock: mockItemSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("GET", "/api/v1/"+feedTestTeamID+"/feed-items/"+feedTestItemID, "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "item_id": feedTestItemID})
	rr := httptest.NewRecorder()

	srv.handleGetFeedItem(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandleGetFeedItem_InvalidUUID(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("GET", "/api/v1/"+feedTestTeamID+"/feed-items/bad-id", "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "item_id": "bad-id"})
	rr := httptest.NewRecorder()

	srv.handleGetFeedItem(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// ─────────────────────────────────────────────────────────────────────────────
// Archive / Unarchive toggle
// ─────────────────────────────────────────────────────────────────────────────

func TestHandleArchiveFeedItem_Success(t *testing.T) {
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockItemSvc.On("ArchiveFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return(nil)

	fc := &MockFeedContainer{FeedItemServiceMock: mockItemSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("POST",
		archivePath, "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "item_id": feedTestItemID})
	rr := httptest.NewRecorder()

	srv.handleArchiveFeedItem(rr, req)
	assert.Equal(t, http.StatusNoContent, rr.Code)
}

// TestHandleArchiveFeedItem_KeepsEmbeddings asserts archiving a feed item never deletes
// any embedding rows — archive is a soft delete and content remains retrievable (#1361).
func TestHandleArchiveFeedItem_KeepsEmbeddings(t *testing.T) {
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockItemSvc.On("ArchiveFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).Return(nil)

	mockEmbedding := servicesmocks.NewMockEmbeddingServiceInterface(t)

	fc := &MockFeedContainer{FeedItemServiceMock: mockItemSvc, EmbeddingServiceMock: mockEmbedding}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("POST", archivePath, "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "item_id": feedTestItemID})
	rr := httptest.NewRecorder()

	srv.handleArchiveFeedItem(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	mockEmbedding.AssertNotCalled(t, "DeleteEmbeddingsByEntity", mock.Anything, mock.Anything)
}

func TestHandleUnarchiveFeedItem_Success(t *testing.T) {
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockItemSvc.On("UnarchiveFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return(nil)

	fc := &MockFeedContainer{FeedItemServiceMock: mockItemSvc}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("POST",
		unarchivePath, "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "item_id": feedTestItemID})
	rr := httptest.NewRecorder()

	srv.handleUnarchiveFeedItem(rr, req)
	assert.Equal(t, http.StatusNoContent, rr.Code)
}

// TestArchiveThenUnarchiveThenList simulates the archive toggle flow:
// archive -> unarchive -> list with archived=false should not include it.
func TestArchiveThenUnarchiveThenList(t *testing.T) {
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)

	// Step 1: archive
	mockItemSvc.On("ArchiveFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return(nil).Once()

	// Step 2: unarchive
	mockItemSvc.On("UnarchiveFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return(nil).Once()

	// Step 3: list archived=false — item is back to active, so 1 result
	fFalse := false
	listResponse := &models.FeedItemListResponse{
		Items: []models.FeedItem{*sampleFeedItem()}, TotalCount: 1, Page: 1, PerPage: 20, TotalPages: 1,
	}
	mockItemSvc.On("ListFeedItems", mock.Anything, feedTestUserID,
		mock.MatchedBy(func(fi services.FeedItemFilters) bool {
			return fi.Archived != nil && !*fi.Archived
		}),
	).Return(listResponse, nil).Once()
	mockItemSvc.On("EnrichWithReplyCounts", mock.Anything, feedTestTeamID, listResponse.Items).
		Return(listResponse.Items, nil).Once()
	_ = fFalse

	fc := &MockFeedContainer{FeedItemServiceMock: mockItemSvc}
	srv := newFeedTestServer(t, fc)

	// Archive
	req1 := authenticatedFeedRequest("POST", archivePath, "", feedTestUserID)
	req1 = addFeedURLParams(req1, map[string]string{"team_id": feedTestTeamID, "item_id": feedTestItemID})
	rr1 := httptest.NewRecorder()
	srv.handleArchiveFeedItem(rr1, req1)
	assert.Equal(t, http.StatusNoContent, rr1.Code)

	// Unarchive
	req2 := authenticatedFeedRequest("POST", unarchivePath, "", feedTestUserID)
	req2 = addFeedURLParams(req2, map[string]string{"team_id": feedTestTeamID, "item_id": feedTestItemID})
	rr2 := httptest.NewRecorder()
	srv.handleUnarchiveFeedItem(rr2, req2)
	assert.Equal(t, http.StatusNoContent, rr2.Code)

	// List active (archived=false, default)
	req3 := authenticatedFeedRequest("GET", "/api/v1/"+feedTestTeamID+"/feed-items", "", feedTestUserID)
	req3 = addFeedURLParams(req3, map[string]string{"team_id": feedTestTeamID})
	rr3 := httptest.NewRecorder()
	srv.handleListFeedItems(rr3, req3)
	assert.Equal(t, http.StatusOK, rr3.Code)
	var resp models.FeedItemListResponse
	assert.NoError(t, json.Unmarshal(rr3.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.TotalCount)

	mockItemSvc.AssertExpectations(t)
}

// ─────────────────────────────────────────────────────────────────────────────
// handleDeleteFeedItem
// ─────────────────────────────────────────────────────────────────────────────

// newDeleteFeedItemRequest builds the DELETE request with the item_id route param set.
func newDeleteFeedItemRequest() *http.Request {
	req := authenticatedFeedRequest("DELETE", "/api/v1/"+feedTestTeamID+"/feed-items/"+feedTestItemID, "", feedTestUserID)
	return addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "item_id": feedTestItemID})
}

func TestHandleDeleteFeedItem_Success_RemovesItemAndReplyEmbeddings(t *testing.T) {
	item := sampleFeedItem() // PostedByUserID == feedTestUserID
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockItemSvc.On("GetFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).Return(item, nil)
	mockItemSvc.On("DeleteFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).Return(nil)

	replyPosters := []repositories.FeedItemReplyPoster{
		{ReplyID: "reply-1", PostedByUserID: "poster-a"},
		{ReplyID: "reply-2", PostedByUserID: "poster-b"},
	}
	mockReplySvc := servicesmocks.NewMockFeedItemReplyServiceInterface(t)
	mockReplySvc.On("ListReplyPosters", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return(replyPosters, nil)

	mockEmbedding := servicesmocks.NewMockEmbeddingServiceInterface(t)
	// Item embedding removed keyed solely by the entity.
	mockEmbedding.On("DeleteEmbeddingsByEntity", "feed_item", feedTestItemID).Return(nil)
	// Each reply embedding removed keyed solely by the reply entity.
	mockEmbedding.On("DeleteEmbeddingsByEntity", "feed_item_reply", "reply-1").Return(nil)
	mockEmbedding.On("DeleteEmbeddingsByEntity", "feed_item_reply", "reply-2").Return(nil)

	fc := &MockFeedContainer{
		FeedItemServiceMock:      mockItemSvc,
		FeedItemReplyServiceMock: mockReplySvc,
		EmbeddingServiceMock:     mockEmbedding,
	}
	srv := newFeedTestServer(t, fc)

	rr := httptest.NewRecorder()
	srv.handleDeleteFeedItem(rr, newDeleteFeedItemRequest())

	assert.Equal(t, http.StatusNoContent, rr.Code)
	mockEmbedding.AssertExpectations(t)
}

func TestHandleDeleteFeedItem_Success_NoReplies(t *testing.T) {
	item := sampleFeedItem()
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockItemSvc.On("GetFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).Return(item, nil)
	mockItemSvc.On("DeleteFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).Return(nil)

	mockReplySvc := servicesmocks.NewMockFeedItemReplyServiceInterface(t)
	mockReplySvc.On("ListReplyPosters", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return([]repositories.FeedItemReplyPoster{}, nil)

	mockEmbedding := servicesmocks.NewMockEmbeddingServiceInterface(t)
	mockEmbedding.On("DeleteEmbeddingsByEntity", "feed_item", feedTestItemID).Return(nil)

	fc := &MockFeedContainer{
		FeedItemServiceMock:      mockItemSvc,
		FeedItemReplyServiceMock: mockReplySvc,
		EmbeddingServiceMock:     mockEmbedding,
	}
	srv := newFeedTestServer(t, fc)

	rr := httptest.NewRecorder()
	srv.handleDeleteFeedItem(rr, newDeleteFeedItemRequest())

	assert.Equal(t, http.StatusNoContent, rr.Code)
	mockEmbedding.AssertExpectations(t)
	// No reply embedding deletions expected.
	mockEmbedding.AssertNotCalled(t, "DeleteEmbeddingsByEntity", "feed_item_reply", mock.Anything)
}

func TestHandleDeleteFeedItem_NonFatalEmbeddingError(t *testing.T) {
	item := sampleFeedItem()
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockItemSvc.On("GetFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).Return(item, nil)
	mockItemSvc.On("DeleteFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).Return(nil)

	mockReplySvc := servicesmocks.NewMockFeedItemReplyServiceInterface(t)
	mockReplySvc.On("ListReplyPosters", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return([]repositories.FeedItemReplyPoster{}, nil)

	// Embedding cleanup failure must not change the 204 response (item already deleted).
	mockEmbedding := servicesmocks.NewMockEmbeddingServiceInterface(t)
	mockEmbedding.On("DeleteEmbeddingsByEntity", "feed_item", feedTestItemID).
		Return(errors.New("embedding store unavailable"))

	fc := &MockFeedContainer{
		FeedItemServiceMock:      mockItemSvc,
		FeedItemReplyServiceMock: mockReplySvc,
		EmbeddingServiceMock:     mockEmbedding,
	}
	srv := newFeedTestServer(t, fc)

	rr := httptest.NewRecorder()
	srv.handleDeleteFeedItem(rr, newDeleteFeedItemRequest())

	assert.Equal(t, http.StatusNoContent, rr.Code)
}

// A transient failure listing reply posters (best-effort cleanup data) must not block the
// user's delete: the item is still deleted, its embedding is still cleaned, and no reply
// embedding deletions are attempted (we have no poster ids to key them by).
func TestHandleDeleteFeedItem_ListRepliesError_StillDeletes(t *testing.T) {
	item := sampleFeedItem()
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockItemSvc.On("GetFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).Return(item, nil)
	mockItemSvc.On("DeleteFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).Return(nil)

	mockReplySvc := servicesmocks.NewMockFeedItemReplyServiceInterface(t)
	mockReplySvc.On("ListReplyPosters", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return(([]repositories.FeedItemReplyPoster)(nil), errors.New("transient db error"))

	mockEmbedding := servicesmocks.NewMockEmbeddingServiceInterface(t)
	mockEmbedding.On("DeleteEmbeddingsByEntity", "feed_item", feedTestItemID).Return(nil)

	fc := &MockFeedContainer{
		FeedItemServiceMock:      mockItemSvc,
		FeedItemReplyServiceMock: mockReplySvc,
		EmbeddingServiceMock:     mockEmbedding,
	}
	srv := newFeedTestServer(t, fc)

	rr := httptest.NewRecorder()
	srv.handleDeleteFeedItem(rr, newDeleteFeedItemRequest())

	// The delete still succeeds despite the reply-poster lookup failing.
	assert.Equal(t, http.StatusNoContent, rr.Code)
	mockItemSvc.AssertCalled(t, "DeleteFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID)
	mockEmbedding.AssertExpectations(t)
	mockEmbedding.AssertNotCalled(t, "DeleteEmbeddingsByEntity", "feed_item_reply", mock.Anything)
}

func TestHandleDeleteFeedItem_NotFound(t *testing.T) {
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockItemSvc.On("GetFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return((*models.FeedItem)(nil), errors.New("feed item not found"))

	fc := &MockFeedContainer{FeedItemServiceMock: mockItemSvc}
	srv := newFeedTestServer(t, fc)

	rr := httptest.NewRecorder()
	srv.handleDeleteFeedItem(rr, newDeleteFeedItemRequest())

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// TestHandleDeleteFeedItem_DeleteForbidden_Returns403 covers the new sentinel
// path: GetFeedItem succeeds (the caller is a team member who can read the item)
// but DeleteFeedItem returns ErrFeedItemForbidden because the caller is neither
// the poster nor an owner/admin. The handler must map this to 403, not 404.
func TestHandleDeleteFeedItem_DeleteForbidden_Returns403(t *testing.T) {
	item := sampleFeedItem()
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockItemSvc.On("GetFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return(item, nil)
	forbiddenErr := fmt.Errorf("%w: user=%s item=%s",
		repositories.ErrFeedItemForbidden, feedTestUserID, feedTestItemID)
	mockItemSvc.On("DeleteFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return(forbiddenErr)

	mockReplySvc := servicesmocks.NewMockFeedItemReplyServiceInterface(t)
	mockReplySvc.On("ListReplyPosters", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return([]repositories.FeedItemReplyPoster{}, nil)

	fc := &MockFeedContainer{
		FeedItemServiceMock:      mockItemSvc,
		FeedItemReplyServiceMock: mockReplySvc,
	}
	srv := newFeedTestServer(t, fc)

	rr := httptest.NewRecorder()
	srv.handleDeleteFeedItem(rr, newDeleteFeedItemRequest())

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

// TestHandleDeleteFeedItem_DeleteNotFoundSentinel_Returns404 covers the case
// where the item vanished between the GET and the DELETE (race), surfacing
// ErrFeedItemNotFound. The handler must still map that to 404.
func TestHandleDeleteFeedItem_DeleteNotFoundSentinel_Returns404(t *testing.T) {
	item := sampleFeedItem()
	mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
	mockItemSvc.On("GetFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return(item, nil)
	notFoundErr := fmt.Errorf("%w: id=%s team=%s",
		repositories.ErrFeedItemNotFound, feedTestItemID, feedTestTeamID)
	mockItemSvc.On("DeleteFeedItem", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return(notFoundErr)

	mockReplySvc := servicesmocks.NewMockFeedItemReplyServiceInterface(t)
	mockReplySvc.On("ListReplyPosters", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID).
		Return([]repositories.FeedItemReplyPoster{}, nil)

	fc := &MockFeedContainer{
		FeedItemServiceMock:      mockItemSvc,
		FeedItemReplyServiceMock: mockReplySvc,
	}
	srv := newFeedTestServer(t, fc)

	rr := httptest.NewRecorder()
	srv.handleDeleteFeedItem(rr, newDeleteFeedItemRequest())

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// ─────────────────────────────────────────────────────────────────────────────
// Cross-team isolation: Team A user cannot access Team B resources
// ─────────────────────────────────────────────────────────────────────────────

// ─────────────────────────────────────────────────────────────────────────────
// UUID validation for feed_id / project_id query params (Fix 3)
// ─────────────────────────────────────────────────────────────────────────────

func TestHandleListFeedItems_InvalidFeedIDQueryParam_Returns400(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("GET",
		"/api/v1/"+feedTestTeamID+"/feed-items?feed_id=not-a-uuid", "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleListFeedItems(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleListFeedItems_InvalidProjectIDQueryParam_Returns400(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("GET",
		"/api/v1/"+feedTestTeamID+"/feed-items?project_id=not-a-uuid", "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleListFeedItems(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleListFeedItemsByFeed_InvalidFeedIDQueryParam_Returns400(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	// feedIDOverride is set from URL param (already validated), but ?feed_id in query
	// for the flat list endpoint should still fail.
	req := authenticatedFeedRequest("GET",
		"/api/v1/"+feedTestTeamID+"/feed-items?feed_id=not-a-uuid", "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	// Call the flat list endpoint — feedIDOverride is "" so query param is used
	srv.handleListFeedItems(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// ─────────────────────────────────────────────────────────────────────────────
// archived query param value validation (Fix 5)
// ─────────────────────────────────────────────────────────────────────────────

func TestHandleListFeedItems_InvalidArchivedValue_Returns400(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("GET",
		"/api/v1/"+feedTestTeamID+"/feed-items?archived=maybe", "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleListFeedItems(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleListFeedItemsByFeed_InvalidArchivedValue_Returns400(t *testing.T) {
	fc := &MockFeedContainer{}
	srv := newFeedTestServer(t, fc)

	req := authenticatedFeedRequest("GET", feedItemsPath+"?archived=invalid", "", feedTestUserID)
	req = addFeedURLParams(req, map[string]string{"team_id": feedTestTeamID, "feed_id": feedTestFeedID})
	rr := httptest.NewRecorder()

	srv.handleListFeedItemsByFeed(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// ─────────────────────────────────────────────────────────────────────────────
// Full-router cross-team 403 test via teamValidationMiddleware (Fix 4)
// ─────────────────────────────────────────────────────────────────────────────

// feedCrossTeamRouterContainer wires up the services needed for the full-router
// cross-team test: API key auth + a team service that denies membership for Team B.
type feedCrossTeamRouterContainer struct {
	BaseMockContainer
	apiKeySvc   services.APIKeyServiceInterface
	teamSvc     services.TeamServiceInterface
	feedSvc     services.FeedServiceInterface
	feedItemSvc services.FeedItemServiceInterface
}

func (c *feedCrossTeamRouterContainer) APIKeyService() services.APIKeyServiceInterface {
	return c.apiKeySvc
}

func (c *feedCrossTeamRouterContainer) TeamService() services.TeamServiceInterface {
	return c.teamSvc
}

func (c *feedCrossTeamRouterContainer) FeedService() services.FeedServiceInterface {
	return c.feedSvc
}

func (c *feedCrossTeamRouterContainer) FeedItemService() services.FeedItemServiceInterface {
	return c.feedItemSvc
}

// TestFeedCrossTeamIsolation_FullRouter verifies that teamValidationMiddleware blocks
// requests from a user who is not a member of the requested team, returning 403.
// This uses srv.ServeHTTP (full routing pipeline) rather than calling handlers directly,
// so teamValidationMiddleware is exercised end-to-end.
//
//nolint:funlen // Multiple endpoints must be covered for complete cross-team security validation
func TestFeedCrossTeamIsolation_FullRouter(t *testing.T) {
	const userInTeamA = "user-team-a-only"
	const apiKeyToken = "vxk_cross-team-test-key" // #nosec G101 - test credential
	teamBID := "bb0e8400-e29b-41d4-a716-446655440000"

	routes := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "GET /feeds returns 403 for non-member",
			method: "GET",
			path:   "/api/v1/" + teamBID + "/feeds",
		},
		{
			name:   "POST /feeds returns 403 for non-member",
			method: "POST",
			path:   "/api/v1/" + teamBID + "/feeds",
			body:   `{"name":"Unauthorized Feed"}`,
		},
		{
			name:   "GET /feed-items returns 403 for non-member",
			method: "GET",
			path:   "/api/v1/" + teamBID + "/feed-items",
		},
		{
			name:   "DELETE /feeds/{id} returns 403 for non-member",
			method: "DELETE",
			path:   "/api/v1/" + teamBID + "/feeds/" + feedTestFeedID,
		},
	}

	mockAPIKeySvc := servicesmocks.NewMockAPIKeyServiceInterface(t)
	mockAPIKeySvc.On("ValidateAPIKey", mock.Anything, apiKeyToken).
		Return(&models.APIKey{UserID: userInTeamA}, nil)

	mockTeamSvc := servicesmocks.NewMockTeamServiceInterface(t)
	// User is NOT a member of teamB — middleware should reject with 403
	mockTeamSvc.On("IsUserMemberOfTeam", mock.Anything, userInTeamA, teamBID).
		Return(false, nil)

	ctr := &feedCrossTeamRouterContainer{
		apiKeySvc: mockAPIKeySvc,
		teamSvc:   mockTeamSvc,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = ctr

	for _, tt := range routes {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, http.NoBody)
			}
			req.Header.Set("Authorization", "Bearer "+apiKeyToken)
			rr := httptest.NewRecorder()

			srv.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusForbidden, rr.Code,
				"expected 403 Forbidden for %s %s, got %d", tt.method, tt.path, rr.Code)
		})
	}
}

// TestFeedCrossTeamIsolation verifies that a user in Team A receives 404 when
// the service returns "not found" for a resource in Team B.
// In production the service layer + DB enforce team isolation; the handler
// must propagate the error correctly without leaking Team B existence.
//
//nolint:funlen // Comprehensive cross-team security coverage requires multiple sub-cases
func TestFeedCrossTeamIsolation(t *testing.T) {
	teamBID := "aa0e8400-e29b-41d4-a716-446655440000"
	userInTeamA := "user-team-a"

	tests := []struct {
		name string
		run  func(srv *Server) *httptest.ResponseRecorder
	}{
		{
			name: "GetFeed cross-team returns 404",
			run: func(srv *Server) *httptest.ResponseRecorder {
				mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
				mockFeedSvc.On("GetFeed", mock.Anything, userInTeamA, teamBID, feedTestFeedID).
					Return((*models.Feed)(nil), errors.New("feed not found"))
				srv.container = &MockFeedContainer{FeedServiceMock: mockFeedSvc}
				req := authenticatedFeedRequest("GET", "/", "", userInTeamA)
				req = addFeedURLParams(req, map[string]string{"team_id": teamBID, "feed_id": feedTestFeedID})
				rr := httptest.NewRecorder()
				srv.handleGetFeed(rr, req)
				return rr
			},
		},
		{
			name: "ListFeedItems cross-team returns 200 with empty list",
			run: func(srv *Server) *httptest.ResponseRecorder {
				mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
				emptyItems := []models.FeedItem{}
				mockItemSvc.On("ListFeedItems", mock.Anything, userInTeamA,
					mock.MatchedBy(func(fi services.FeedItemFilters) bool {
						return fi.TeamID == teamBID
					}),
				).Return(&models.FeedItemListResponse{
					Items: emptyItems, TotalCount: 0, Page: 1, PerPage: 20, TotalPages: 0,
				}, nil)
				mockItemSvc.On("EnrichWithReplyCounts", mock.Anything, teamBID, emptyItems).
					Return(emptyItems, nil)
				srv.container = &MockFeedContainer{FeedItemServiceMock: mockItemSvc}
				req := authenticatedFeedRequest("GET", "/", "", userInTeamA)
				req = addFeedURLParams(req, map[string]string{"team_id": teamBID})
				rr := httptest.NewRecorder()
				srv.handleListFeedItems(rr, req)
				return rr
			},
		},
		{
			name: "DeleteFeed cross-team returns 404",
			run: func(srv *Server) *httptest.ResponseRecorder {
				mockFeedSvc := servicesmocks.NewMockFeedServiceInterface(t)
				mockFeedSvc.On("DeleteFeed", mock.Anything, userInTeamA, teamBID, feedTestFeedID).
					Return(errors.New("feed not found"))
				srv.container = &MockFeedContainer{FeedServiceMock: mockFeedSvc}
				req := authenticatedFeedRequest("DELETE", "/", "", userInTeamA)
				req = addFeedURLParams(req, map[string]string{"team_id": teamBID, "feed_id": feedTestFeedID})
				rr := httptest.NewRecorder()
				srv.handleDeleteFeed(rr, req)
				return rr
			},
		},
		{
			name: "CreateFeedItem cross-team returns 403",
			run: func(srv *Server) *httptest.ResponseRecorder {
				mockItemSvc := servicesmocks.NewMockFeedItemServiceInterface(t)
				mockResourceSvc := newAllowedResourceUsageMock(t)
				mockItemSvc.On("CreateFeedItem", mock.Anything, userInTeamA, teamBID, feedTestFeedID, mock.Anything).
					Return((*models.FeedItem)(nil), errors.New("user is not a member of the specified team"))
				srv.container = &MockFeedContainer{
					FeedItemServiceMock:      mockItemSvc,
					ResourceUsageServiceMock: mockResourceSvc,
				}
				body := `{"title":"T","content":"C","ai_assistant_name":"ai"}`
				req := authenticatedFeedRequest("POST", "/", body, userInTeamA)
				req = addFeedURLParams(req, map[string]string{"team_id": teamBID, "feed_id": feedTestFeedID})
				rr := httptest.NewRecorder()
				srv.handleCreateFeedItem(rr, req)
				return rr
			},
		},
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := tt.run(srv)
			// Cross-team access should either 403 or 404 (never 200 with data)
			assert.True(t, rr.Code == http.StatusNotFound ||
				rr.Code == http.StatusForbidden ||
				(rr.Code == http.StatusOK && json.Valid(rr.Body.Bytes())),
				"unexpected status %d", rr.Code)
		})
	}
}
