package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	commentsgen "github.com/vibexp/vibexp/internal/server/gen/comments"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	testCommentsTeamID     = "660e8400-e29b-41d4-a716-446655440001"
	testCommentsUserID     = "880e8400-e29b-41d4-a716-446655440003"
	testCommentsResourceID = "770e8400-e29b-41d4-a716-446655440002"
	testCommentsCommentID  = "550e8400-e29b-41d4-a716-446655440000"
	testCommentsProjectID  = "990e8400-e29b-41d4-a716-446655440004"
)

// MockCommentsContainer overrides only the comment service on the base container.
type MockCommentsContainer struct {
	BaseMockContainer
	commentService services.CommentServiceInterface
}

func (c *MockCommentsContainer) CommentService() services.CommentServiceInterface {
	return c.commentService
}

func createTestCommentsServer(svc services.CommentServiceInterface) *Server {
	logger := slog.New(slog.DiscardHandler)

	r := chi.NewRouter()
	srv := &Server{
		container: &MockCommentsContainer{commentService: svc},
		logger:    logger,
		config:    &config.Config{},
		router:    r,
	}
	strict := commentsgen.NewStrictHandlerWithOptions(
		&commentsStrictServer{s: srv},
		nil,
		commentsgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  srv.commentsBindErrorHandler,
			ResponseErrorHandlerFunc: srv.commentsResponseErrorHandler,
		},
	)
	commentsgen.HandlerWithOptions(strict, commentsgen.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: srv.commentsBindErrorHandler,
	})
	return srv
}

func makeCommentsRequest(method, path, body string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testCommentsUserID))
}

func assertCommentsProblem(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	assert.Equal(t, status, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var problem struct {
		Code   string `json:"code"`
		Status int    `json:"status"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
	assert.Equal(t, code, problem.Code)
	assert.Equal(t, status, problem.Status)
}

func sampleComment() models.Comment {
	return models.Comment{
		ID:           testCommentsCommentID,
		TeamID:       testCommentsTeamID,
		ResourceType: models.CommentResourceTypeArtifact,
		ResourceID:   testCommentsResourceID,
		UserID:       testCommentsUserID,
		Content:      "a useful note",
		CreatedAt:    time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC),
	}
}

func TestListComments_Success(t *testing.T) {
	svc := servicesmocks.NewMockCommentServiceInterface(t)
	svc.EXPECT().ListByResource(
		mock.Anything, testCommentsUserID, testCommentsTeamID,
		models.CommentResourceTypeArtifact, testCommentsResourceID, mock.Anything, mock.Anything,
	).Return(&models.CommentListResponse{
		Comments:   []models.Comment{sampleComment()},
		TotalCount: 1,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}, nil)

	srv := createTestCommentsServer(svc)
	req := makeCommentsRequest("GET",
		"/api/v1/"+testCommentsTeamID+"/comments?resource_type=artifact&resource_id="+testCommentsResourceID, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.EqualValues(t, 1, resp["total_count"])
	require.Len(t, resp["comments"].([]interface{}), 1)
}

func TestListComments_NonMemberForbidden(t *testing.T) {
	svc := servicesmocks.NewMockCommentServiceInterface(t)
	svc.EXPECT().ListByResource(
		mock.Anything, testCommentsUserID, testCommentsTeamID,
		models.CommentResourceTypeArtifact, testCommentsResourceID, mock.Anything, mock.Anything,
	).Return(nil, assertErrNotMember())

	srv := createTestCommentsServer(svc)
	req := makeCommentsRequest("GET",
		"/api/v1/"+testCommentsTeamID+"/comments?resource_type=artifact&resource_id="+testCommentsResourceID, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusForbidden, "FORBIDDEN")
}

func TestCreateComment_Success(t *testing.T) {
	svc := servicesmocks.NewMockCommentServiceInterface(t)
	created := sampleComment()
	svc.EXPECT().Create(mock.Anything, testCommentsUserID, testCommentsTeamID, mock.MatchedBy(
		func(req *models.CreateCommentRequest) bool {
			return req.ResourceType == models.CommentResourceTypeArtifact &&
				req.ResourceID == testCommentsResourceID && req.Content == "a useful note"
		},
	)).Return(&created, nil)

	srv := createTestCommentsServer(svc)
	body := `{"resource_type":"artifact","resource_id":"` + testCommentsResourceID + `","content":"a useful note"}`
	req := makeCommentsRequest("POST", "/api/v1/"+testCommentsTeamID+"/comments", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "a useful note", resp["content"])
	assert.Equal(t, "artifact", resp["resource_type"])
}

func TestCreateComment_ResourceNotFound(t *testing.T) {
	svc := servicesmocks.NewMockCommentServiceInterface(t)
	svc.EXPECT().Create(mock.Anything, testCommentsUserID, testCommentsTeamID, mock.Anything).
		Return(nil, assertErrResourceNotFound())

	srv := createTestCommentsServer(svc)
	body := `{"resource_type":"artifact","resource_id":"` + testCommentsResourceID + `","content":"x"}`
	req := makeCommentsRequest("POST", "/api/v1/"+testCommentsTeamID+"/comments", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusNotFound, "RESOURCE_NOT_FOUND")
}

func TestCreateComment_Forbidden(t *testing.T) {
	svc := servicesmocks.NewMockCommentServiceInterface(t)
	svc.EXPECT().Create(mock.Anything, testCommentsUserID, testCommentsTeamID, mock.Anything).
		Return(nil, services.ErrPermissionDenied)

	srv := createTestCommentsServer(svc)
	body := `{"resource_type":"artifact","resource_id":"` + testCommentsResourceID + `","content":"x"}`
	req := makeCommentsRequest("POST", "/api/v1/"+testCommentsTeamID+"/comments", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusForbidden, "FORBIDDEN")
}

func TestUpdateComment_Success(t *testing.T) {
	svc := servicesmocks.NewMockCommentServiceInterface(t)
	updated := sampleComment()
	updated.Content = "edited"
	updated.UpdatedAt = updated.CreatedAt.Add(time.Hour)
	svc.EXPECT().Update(mock.Anything, testCommentsUserID, testCommentsTeamID, testCommentsCommentID,
		mock.MatchedBy(func(req *models.UpdateCommentRequest) bool { return req.Content == "edited" }),
	).Return(&updated, nil)

	srv := createTestCommentsServer(svc)
	req := makeCommentsRequest("PATCH", "/api/v1/"+testCommentsTeamID+"/comments/"+testCommentsCommentID,
		`{"content":"edited"}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "edited", resp["content"])
}

func TestUpdateComment_AuthorOnlyForbidden(t *testing.T) {
	svc := servicesmocks.NewMockCommentServiceInterface(t)
	svc.EXPECT().Update(mock.Anything, testCommentsUserID, testCommentsTeamID, testCommentsCommentID, mock.Anything).
		Return(nil, services.ErrPermissionDenied)

	srv := createTestCommentsServer(svc)
	req := makeCommentsRequest("PATCH", "/api/v1/"+testCommentsTeamID+"/comments/"+testCommentsCommentID,
		`{"content":"edited"}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusForbidden, "FORBIDDEN")
}

func TestUpdateComment_NotFound(t *testing.T) {
	svc := servicesmocks.NewMockCommentServiceInterface(t)
	svc.EXPECT().Update(mock.Anything, testCommentsUserID, testCommentsTeamID, testCommentsCommentID, mock.Anything).
		Return(nil, repositories.ErrCommentNotFound)

	srv := createTestCommentsServer(svc)
	req := makeCommentsRequest("PATCH", "/api/v1/"+testCommentsTeamID+"/comments/"+testCommentsCommentID,
		`{"content":"edited"}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusNotFound, "RESOURCE_NOT_FOUND")
}

func TestDeleteComment_Success(t *testing.T) {
	svc := servicesmocks.NewMockCommentServiceInterface(t)
	svc.EXPECT().Delete(mock.Anything, testCommentsUserID, testCommentsTeamID, testCommentsCommentID).Return(nil)

	srv := createTestCommentsServer(svc)
	req := makeCommentsRequest("DELETE", "/api/v1/"+testCommentsTeamID+"/comments/"+testCommentsCommentID, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestDeleteComment_Forbidden(t *testing.T) {
	svc := servicesmocks.NewMockCommentServiceInterface(t)
	svc.EXPECT().Delete(mock.Anything, testCommentsUserID, testCommentsTeamID, testCommentsCommentID).
		Return(services.ErrPermissionDenied)

	srv := createTestCommentsServer(svc)
	req := makeCommentsRequest("DELETE", "/api/v1/"+testCommentsTeamID+"/comments/"+testCommentsCommentID, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusForbidden, "FORBIDDEN")
}

func TestDeleteComment_NotFound(t *testing.T) {
	svc := servicesmocks.NewMockCommentServiceInterface(t)
	svc.EXPECT().Delete(mock.Anything, testCommentsUserID, testCommentsTeamID, testCommentsCommentID).
		Return(repositories.ErrCommentNotFound)

	srv := createTestCommentsServer(svc)
	req := makeCommentsRequest("DELETE", "/api/v1/"+testCommentsTeamID+"/comments/"+testCommentsCommentID, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusNotFound, "RESOURCE_NOT_FOUND")
}

func TestDeleteComment_InvalidUUID(t *testing.T) {
	// The generated binder rejects a non-UUID comment_id before the handler runs.
	svc := servicesmocks.NewMockCommentServiceInterface(t)
	srv := createTestCommentsServer(svc)
	req := makeCommentsRequest("DELETE", "/api/v1/"+testCommentsTeamID+"/comments/not-a-uuid", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusBadRequest, "BAD_REQUEST")
}

func TestListRecentComments_Success(t *testing.T) {
	svc := servicesmocks.NewMockCommentServiceInterface(t)
	projectID := testCommentsProjectID
	slug := "q3-revenue"
	artifactRow := models.CommentActivity{
		Comment:       sampleComment(),
		ResourceTitle: "Q3 revenue analysis",
		ProjectID:     &projectID,
		Slug:          &slug,
	}
	memoryRow := models.CommentActivity{
		Comment: models.Comment{
			ID:           "551e8400-e29b-41d4-a716-446655440000",
			TeamID:       testCommentsTeamID,
			ResourceType: models.CommentResourceTypeMemory,
			ResourceID:   "552e8400-e29b-41d4-a716-446655440000",
			UserID:       testCommentsUserID,
			Content:      "note",
			CreatedAt:    time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC),
			UpdatedAt:    time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC),
		},
		ResourceTitle: "a memory title label",
		ProjectID:     &projectID,
		Slug:          nil, // memory has no slug
	}
	svc.EXPECT().ListRecentByTeam(mock.Anything, testCommentsUserID, testCommentsTeamID, mock.Anything).
		Return([]models.CommentActivity{artifactRow, memoryRow}, nil)

	srv := createTestCommentsServer(svc)
	req := makeCommentsRequest("GET", "/api/v1/"+testCommentsTeamID+"/comments/recent", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.EqualValues(t, 2, resp["total_count"])
	rows := resp["comments"].([]interface{})
	require.Len(t, rows, 2)
	// The memory row omits slug; the artifact row includes it.
	art := rows[0].(map[string]interface{})
	assert.Equal(t, "q3-revenue", art["slug"])
	mem := rows[1].(map[string]interface{})
	_, hasSlug := mem["slug"]
	assert.False(t, hasSlug, "memory row must omit slug")
}

// assertErrNotMember / assertErrResourceNotFound build the string errors the
// CommentService returns for those cases (it uses fmt.Errorf, not sentinels).
func assertErrNotMember() error {
	return errString("user is not a member of the specified team")
}

func assertErrResourceNotFound() error {
	return errString("resource not found")
}

type errString string

func (e errString) Error() string { return string(e) }
