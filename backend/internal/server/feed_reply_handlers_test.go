package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// ─────────────────────────────────────────────────────────────────────────────
// Mock container for feed reply tests
// ─────────────────────────────────────────────────────────────────────────────

// MockFeedReplyContainer is a mock container for feed item reply handler tests.
type MockFeedReplyContainer struct {
	BaseMockContainer
	FeedItemServiceMock      services.FeedItemServiceInterface
	FeedItemReplyServiceMock services.FeedItemReplyServiceInterface
	ResourceUsageServiceMock services.ResourceUsageServiceInterface
}

func (m *MockFeedReplyContainer) FeedItemService() services.FeedItemServiceInterface {
	return m.FeedItemServiceMock
}

func (m *MockFeedReplyContainer) FeedItemReplyService() services.FeedItemReplyServiceInterface {
	return m.FeedItemReplyServiceMock
}

func (m *MockFeedReplyContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return m.ResourceUsageServiceMock
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

const (
	replyTestReplyID = "990e8400-e29b-41d4-a716-446655440004"
)

func sampleReply() *models.FeedItemReply {
	name := "claude-test"
	return &models.FeedItemReply{
		ID:              replyTestReplyID,
		TeamID:          feedTestTeamID,
		FeedItemID:      feedTestItemID,
		Content:         "This is a test reply.",
		PostedByUserID:  feedTestUserID,
		AIAssistantName: &name,
		PostedAt:        time.Now(),
	}
}

func newReplyTestServerWithContainer(t *testing.T, rc *MockFeedReplyContainer) *Server {
	t.Helper()
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = rc
	return srv
}

// ─────────────────────────────────────────────────────────────────────────────
// handleCreateFeedItemReply tests
// ─────────────────────────────────────────────────────────────────────────────

//nolint:funlen // Table-driven test with multiple cases
func TestHandleCreateFeedItemReply(t *testing.T) {
	tests := []struct {
		name       string
		itemID     string
		body       string
		setupMock  func(m *servicesmocks.MockFeedItemReplyServiceInterface)
		wantStatus int
	}{
		{
			name:   "success - creates reply and returns 201",
			itemID: feedTestItemID,
			body:   `{"content":"Great update!","ai_assistant_name":"claude-test"}`,
			setupMock: func(m *servicesmocks.MockFeedItemReplyServiceInterface) {
				m.On("CreateReply", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID,
					mock.MatchedBy(func(r *models.CreateFeedItemReplyRequest) bool {
						return r.Content == "Great update!"
					}),
				).Return(sampleReply(), nil)
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "invalid item_id UUID",
			itemID:     "not-a-uuid",
			body:       `{"content":"reply"}`,
			setupMock:  func(m *servicesmocks.MockFeedItemReplyServiceInterface) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing content",
			itemID:     feedTestItemID,
			body:       `{}`,
			setupMock:  func(m *servicesmocks.MockFeedItemReplyServiceInterface) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty content (whitespace only)",
			itemID:     feedTestItemID,
			body:       `{"content":"   "}`,
			setupMock:  func(m *servicesmocks.MockFeedItemReplyServiceInterface) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "content too long (>10000 chars)",
			itemID:     feedTestItemID,
			body:       `{"content":"` + strings.Repeat("x", 10001) + `"}`,
			setupMock:  func(m *servicesmocks.MockFeedItemReplyServiceInterface) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "ai_assistant_name too long (>30 chars)",
			itemID:     feedTestItemID,
			body:       `{"content":"reply","ai_assistant_name":"` + strings.Repeat("a", 31) + `"}`,
			setupMock:  func(m *servicesmocks.MockFeedItemReplyServiceInterface) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "bad JSON body",
			itemID:     feedTestItemID,
			body:       `{bad json}`,
			setupMock:  func(m *servicesmocks.MockFeedItemReplyServiceInterface) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "archived feed item returns 422",
			itemID: feedTestItemID,
			body:   `{"content":"reply to archived"}`,
			setupMock: func(m *servicesmocks.MockFeedItemReplyServiceInterface) {
				m.On("CreateReply", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID, mock.Anything).
					Return((*models.FeedItemReply)(nil), errors.New("feed item is archived"))
			},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:   "not team member returns 403",
			itemID: feedTestItemID,
			body:   `{"content":"reply"}`,
			setupMock: func(m *servicesmocks.MockFeedItemReplyServiceInterface) {
				m.On("CreateReply", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID, mock.Anything).
					Return((*models.FeedItemReply)(nil), errors.New("user is not a member of the specified team"))
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:   "feed item not found returns 404",
			itemID: feedTestItemID,
			body:   `{"content":"reply"}`,
			setupMock: func(m *servicesmocks.MockFeedItemReplyServiceInterface) {
				m.On("CreateReply", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID, mock.Anything).
					Return((*models.FeedItemReply)(nil), errors.New("feed item not found"))
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "internal error returns 500",
			itemID: feedTestItemID,
			body:   `{"content":"reply"}`,
			setupMock: func(m *servicesmocks.MockFeedItemReplyServiceInterface) {
				m.On("CreateReply", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID, mock.Anything).
					Return((*models.FeedItemReply)(nil), errors.New("unexpected db failure"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockReplySvc := servicesmocks.NewMockFeedItemReplyServiceInterface(t)
			tt.setupMock(mockReplySvc)

			// Resource limit check needed for any test case that passes validation
			// (i.e., has a valid body and item_id). Validation failures return before
			// the resource limit check is called, so those cases don't need it.
			var mockResourceSvc *servicesmocks.MockResourceUsageServiceInterface
			needsResourceCheck := tt.itemID == feedTestItemID && tt.wantStatus != http.StatusBadRequest
			if needsResourceCheck {
				mockResourceSvc = newAllowedResourceUsageMock(t)
			}

			rc := &MockFeedReplyContainer{
				FeedItemReplyServiceMock: mockReplySvc,
				ResourceUsageServiceMock: mockResourceSvc,
			}
			srv := newReplyTestServerWithContainer(t, rc)

			req := authenticatedFeedRequest("POST", "/api/v1/"+feedTestTeamID+"/feed-items/"+tt.itemID+"/replies",
				tt.body, feedTestUserID)
			req = addFeedURLParams(req, map[string]string{
				"team_id": feedTestTeamID,
				"item_id": tt.itemID,
			})
			rr := httptest.NewRecorder()

			srv.handleCreateFeedItemReply(rr, req)
			assert.Equal(t, tt.wantStatus, rr.Code)

			if tt.wantStatus == http.StatusCreated {
				var got models.FeedItemReply
				assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
				assert.Equal(t, replyTestReplyID, got.ID)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// handleListFeedItemReplies tests
// ─────────────────────────────────────────────────────────────────────────────

//nolint:funlen // Table-driven test with multiple cases
func TestHandleListFeedItemReplies(t *testing.T) {
	tests := []struct {
		name       string
		itemID     string
		queryStr   string
		setupMock  func(m *servicesmocks.MockFeedItemReplyServiceInterface)
		wantStatus int
	}{
		{
			name:     "success - returns paginated replies",
			itemID:   feedTestItemID,
			queryStr: "",
			setupMock: func(m *servicesmocks.MockFeedItemReplyServiceInterface) {
				m.On("ListReplies", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID, 1, 20).
					Return(&models.FeedItemReplyListResponse{
						Replies:    []models.FeedItemReply{*sampleReply()},
						TotalCount: 1,
						Page:       1,
						PerPage:    20,
						TotalPages: 1,
					}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid item_id UUID",
			itemID:     "not-a-uuid",
			queryStr:   "",
			setupMock:  func(m *servicesmocks.MockFeedItemReplyServiceInterface) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "service error returns 500",
			itemID:   feedTestItemID,
			queryStr: "",
			setupMock: func(m *servicesmocks.MockFeedItemReplyServiceInterface) {
				m.On("ListReplies", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID, 1, 20).
					Return((*models.FeedItemReplyListResponse)(nil), errors.New("db failure"))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:     "not team member returns 403",
			itemID:   feedTestItemID,
			queryStr: "",
			setupMock: func(m *servicesmocks.MockFeedItemReplyServiceInterface) {
				m.On("ListReplies", mock.Anything, feedTestUserID, feedTestTeamID, feedTestItemID, 1, 20).
					Return((*models.FeedItemReplyListResponse)(nil), errors.New("user is not a member of the specified team"))
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockReplySvc := servicesmocks.NewMockFeedItemReplyServiceInterface(t)
			tt.setupMock(mockReplySvc)

			rc := &MockFeedReplyContainer{FeedItemReplyServiceMock: mockReplySvc}
			srv := newReplyTestServerWithContainer(t, rc)

			path := "/api/v1/" + feedTestTeamID + "/feed-items/" + tt.itemID + "/replies"
			if tt.queryStr != "" {
				path += tt.queryStr
			}

			req := authenticatedFeedRequest("GET", path, "", feedTestUserID)
			req = addFeedURLParams(req, map[string]string{
				"team_id": feedTestTeamID,
				"item_id": tt.itemID,
			})
			rr := httptest.NewRecorder()

			srv.handleListFeedItemReplies(rr, req)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Unauthorized reply endpoints (routing-level tests)
// ─────────────────────────────────────────────────────────────────────────────

func TestFeedReplyHandlers_Unauthorized(t *testing.T) {
	srv := testServer()

	routes := []struct {
		name   string
		method string
		path   string
	}{
		{"Create Reply", "POST", "/api/v1/" + feedTestTeamID + "/feed-items/" + feedTestItemID + "/replies"},
		{"List Replies", "GET", "/api/v1/" + feedTestTeamID + "/feed-items/" + feedTestItemID + "/replies"},
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
