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
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// MockNotificationService mocks notifications.NotificationServiceInterface
type MockNotificationService struct {
	mock.Mock
}

func (m *MockNotificationService) Send(ctx context.Context, req *notifications.SendRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *MockNotificationService) ListForUser(
	ctx context.Context, userID string, f notifications.ListFilters,
) ([]*notifications.Notification, error) {
	args := m.Called(ctx, userID, f)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*notifications.Notification), args.Error(1)
}

func (m *MockNotificationService) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

func (m *MockNotificationService) MarkRead(ctx context.Context, userID, notifID string) error {
	args := m.Called(ctx, userID, notifID)
	return args.Error(0)
}

func (m *MockNotificationService) MarkAllRead(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockNotificationService) RunRetentionJob(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// MockNotificationsContainer overrides only the notification service on the base container
type MockNotificationsContainer struct {
	BaseMockContainer
	notificationService *MockNotificationService
}

func (c *MockNotificationsContainer) NotificationService() notifications.NotificationServiceInterface {
	return c.notificationService
}

// createTestNotificationsServer mounts the real setupNotificationsRoutes (the
// generated strict-server mounting under test) on a bare router; auth
// middleware is replaced by injecting contextKeyUserID into the request.
func createTestNotificationsServer(svc *MockNotificationService) *Server {
	logger := slog.New(slog.DiscardHandler) // Suppress logs during test

	r := chi.NewRouter()
	srv := &Server{
		port:      "8080",
		container: &MockNotificationsContainer{notificationService: svc},
		logger:    logger,
		config:    &config.Config{},
		router:    r,
	}
	srv.setupNotificationsRoutes(r)
	return srv
}

// testNotificationsUserID is the authenticated user injected into every
// request; auth middleware is out of scope for these handler tests.
const testNotificationsUserID = "user-1"

func makeNotificationsRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testNotificationsUserID))
	return req
}

// assertNotificationsProblem asserts the RFC 9457 problem+json wire shape the
// notifications domain has always emitted for errors.
func assertNotificationsProblem(t *testing.T, w *httptest.ResponseRecorder, status int, code, detail string) {
	t.Helper()
	assert.Equal(t, status, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
		Status int    `json:"status"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
	assert.Equal(t, code, problem.Code)
	assert.Equal(t, detail, problem.Detail)
	assert.Equal(t, status, problem.Status)
}

func TestListNotifications_DefaultsAndWireShape(t *testing.T) {
	svc := &MockNotificationService{}
	readAt := time.Date(2026, 6, 9, 10, 15, 30, 0, time.UTC)
	full := &notifications.Notification{
		ID:        "550e8400-e29b-41d4-a716-446655440000",
		TeamID:    "660e8400-e29b-41d4-a716-446655440001",
		Type:      "feed.item.created",
		Category:  "low",
		Title:     "New feed item",
		Body:      "A new item was posted.",
		ActionURL: "https://app.vibexp.io/feeds/abc",
		EntityRef: map[string]interface{}{"entity_type": "feed_item"},
		ReadAt:    &readAt,
		CreatedAt: time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC),
	}
	minimal := &notifications.Notification{
		ID:        "770e8400-e29b-41d4-a716-446655440002",
		Type:      "team.invite",
		Category:  "high",
		Title:     "You were invited",
		CreatedAt: time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC),
	}
	svc.On("ListForUser", mock.Anything, "user-1",
		notifications.ListFilters{Limit: 20, Offset: 0, UnreadOnly: false}).
		Return([]*notifications.Notification{full, minimal}, nil)

	srv := createTestNotificationsServer(svc)
	req := makeNotificationsRequest("GET", "/api/v1/notifications")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.EqualValues(t, 2, resp["count"])
	assert.EqualValues(t, 20, resp["limit"])
	assert.EqualValues(t, 0, resp["offset"])

	items := resp["notifications"].([]interface{})
	require.Len(t, items, 2)
	first := items[0].(map[string]interface{})
	assert.Equal(t, full.ID, first["id"])
	assert.Equal(t, full.TeamID, first["team_id"])
	assert.Equal(t, "A new item was posted.", first["body"])
	assert.Equal(t, "2026-06-09T10:15:30Z", first["read_at"])

	// omitempty parity with the previous hand-written shape: optional fields
	// absent on the wire when empty, not "" / {} / null.
	second := items[1].(map[string]interface{})
	for _, key := range []string{"team_id", "body", "action_url", "entity_ref", "read_at", "dismissed_at"} {
		_, present := second[key]
		assert.False(t, present, "optional field %q must be omitted when empty", key)
	}

	svc.AssertExpectations(t)
}

func TestListNotifications_ExplicitParams(t *testing.T) {
	svc := &MockNotificationService{}
	svc.On("ListForUser", mock.Anything, "user-1",
		notifications.ListFilters{Limit: 50, Offset: 10, UnreadOnly: true}).
		Return([]*notifications.Notification{}, nil)

	srv := createTestNotificationsServer(svc)
	req := makeNotificationsRequest("GET", "/api/v1/notifications?limit=50&offset=10&unread=true")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.EqualValues(t, 0, resp["count"])
	assert.EqualValues(t, 50, resp["limit"])
	assert.EqualValues(t, 10, resp["offset"])
	svc.AssertExpectations(t)
}

func TestListNotifications_InvalidParams(t *testing.T) {
	cases := []struct {
		name   string
		query  string
		detail string
	}{
		{"limit zero", "limit=0", "limit must be an integer between 1 and 100"},
		{"limit over max", "limit=101", "limit must be an integer between 1 and 100"},
		{"limit not an integer", "limit=abc", "limit must be an integer between 1 and 100"},
		{"offset negative", "offset=-1", "offset must be a non-negative integer"},
		{"offset not an integer", "offset=abc", "offset must be a non-negative integer"},
		{"unread not a boolean", "unread=notabool", "unread must be a boolean"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &MockNotificationService{} // service must never be called
			srv := createTestNotificationsServer(svc)
			req := makeNotificationsRequest("GET", "/api/v1/notifications?"+tc.query)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)
			specconformance.AssertConformsToSpec(t, req, w)

			assertNotificationsProblem(t, w, http.StatusBadRequest, apierrors.CodeBadRequest, tc.detail)
			svc.AssertExpectations(t)
		})
	}
}

func TestListNotifications_ConversionError(t *testing.T) {
	svc := &MockNotificationService{}
	corrupt := &notifications.Notification{
		ID:        "not-a-uuid", // cannot happen with DB-generated IDs; pins the defensive 500
		Type:      "feed.item.created",
		Category:  "low",
		Title:     "Corrupt",
		CreatedAt: time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC),
	}
	svc.On("ListForUser", mock.Anything, "user-1", mock.Anything).
		Return([]*notifications.Notification{corrupt}, nil)

	srv := createTestNotificationsServer(svc)
	req := makeNotificationsRequest("GET", "/api/v1/notifications")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assertNotificationsProblem(t, w, http.StatusInternalServerError,
		apierrors.CodeInternalError, "Failed to list notifications")
	svc.AssertExpectations(t)
}

func TestListNotifications_ServiceError(t *testing.T) {
	svc := &MockNotificationService{}
	svc.On("ListForUser", mock.Anything, "user-1", mock.Anything).
		Return(nil, errors.New("db down"))

	srv := createTestNotificationsServer(svc)
	req := makeNotificationsRequest("GET", "/api/v1/notifications")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assertNotificationsProblem(t, w, http.StatusInternalServerError,
		apierrors.CodeInternalError, "Failed to list notifications")
	svc.AssertExpectations(t)
}

func TestGetUnreadNotificationCount(t *testing.T) {
	svc := &MockNotificationService{}
	svc.On("GetUnreadCount", mock.Anything, "user-1").Return(3, nil)

	srv := createTestNotificationsServer(svc)
	req := makeNotificationsRequest("GET", "/api/v1/notifications/unread-count")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.EqualValues(t, 3, resp["unread_count"])
	svc.AssertExpectations(t)
}

func TestGetUnreadNotificationCount_ServiceError(t *testing.T) {
	svc := &MockNotificationService{}
	svc.On("GetUnreadCount", mock.Anything, "user-1").Return(0, errors.New("db down"))

	srv := createTestNotificationsServer(svc)
	req := makeNotificationsRequest("GET", "/api/v1/notifications/unread-count")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assertNotificationsProblem(t, w, http.StatusInternalServerError,
		apierrors.CodeInternalError, "Failed to get unread notification count")
	svc.AssertExpectations(t)
}

func TestMarkNotificationRead(t *testing.T) {
	const notifID = "550e8400-e29b-41d4-a716-446655440000"
	svc := &MockNotificationService{}
	svc.On("MarkRead", mock.Anything, "user-1", notifID).Return(nil)

	srv := createTestNotificationsServer(svc)
	req := makeNotificationsRequest("PATCH", "/api/v1/notifications/"+notifID+"/read")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.Bytes())
	svc.AssertExpectations(t)
}

func TestMarkNotificationRead_InvalidUUID(t *testing.T) {
	svc := &MockNotificationService{} // service must never be called
	srv := createTestNotificationsServer(svc)
	req := makeNotificationsRequest("PATCH", "/api/v1/notifications/not-a-uuid/read")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assertNotificationsProblem(t, w, http.StatusBadRequest,
		apierrors.CodeBadRequest, "notification id must be a valid UUID")
	svc.AssertExpectations(t)
}

func TestMarkNotificationRead_ServiceError(t *testing.T) {
	const notifID = "550e8400-e29b-41d4-a716-446655440000"
	svc := &MockNotificationService{}
	svc.On("MarkRead", mock.Anything, "user-1", notifID).Return(errors.New("db down"))

	srv := createTestNotificationsServer(svc)
	req := makeNotificationsRequest("PATCH", "/api/v1/notifications/"+notifID+"/read")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assertNotificationsProblem(t, w, http.StatusInternalServerError,
		apierrors.CodeInternalError, "Failed to mark notification as read")
	svc.AssertExpectations(t)
}

func TestMarkAllNotificationsRead(t *testing.T) {
	svc := &MockNotificationService{}
	svc.On("MarkAllRead", mock.Anything, "user-1").Return(nil)

	srv := createTestNotificationsServer(svc)
	req := makeNotificationsRequest("PATCH", "/api/v1/notifications/read-all")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.Bytes())
	svc.AssertExpectations(t)
}

func TestMarkAllNotificationsRead_ServiceError(t *testing.T) {
	svc := &MockNotificationService{}
	svc.On("MarkAllRead", mock.Anything, "user-1").Return(errors.New("db down"))

	srv := createTestNotificationsServer(svc)
	req := makeNotificationsRequest("PATCH", "/api/v1/notifications/read-all")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assertNotificationsProblem(t, w, http.StatusInternalServerError,
		apierrors.CodeInternalError, "Failed to mark all notifications as read")
	svc.AssertExpectations(t)
}
