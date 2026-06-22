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
	typesgen "github.com/vibexp/vibexp/internal/server/gen/types"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	testTypesTeamID = "660e8400-e29b-41d4-a716-446655440001"
	testTypesUserID = "user-1"
)

// MockTypesContainer overrides only the type service on the base container.
type MockTypesContainer struct {
	BaseMockContainer
	typeService services.TypeServiceInterface
}

func (c *MockTypesContainer) TypeService() services.TypeServiceInterface {
	return c.typeService
}

// createTestTypesServer mounts the generated Types strict-server handler on a
// bare router. The team-membership/subscription middleware that setupTypesRoutes
// adds is exercised elsewhere; here the authenticated user is injected per
// request so the handler logic and spec wire shape are tested in isolation.
func createTestTypesServer(svc services.TypeServiceInterface) *Server {
	logger := slog.New(slog.DiscardHandler)

	r := chi.NewRouter()
	srv := &Server{
		container: &MockTypesContainer{typeService: svc},
		logger:    logger,
		config:    &config.Config{},
		router:    r,
	}
	strict := typesgen.NewStrictHandlerWithOptions(
		&typesStrictServer{s: srv},
		nil,
		typesgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  srv.typesBindErrorHandler,
			ResponseErrorHandlerFunc: srv.typesResponseErrorHandler,
		},
	)
	typesgen.HandlerWithOptions(strict, typesgen.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: srv.typesBindErrorHandler,
	})
	return srv
}

func makeTypesRequest(method, path, body string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testTypesUserID))
}

func assertTypesProblem(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
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

func TestListTypes_Success(t *testing.T) {
	svc := servicesmocks.NewMockTypeServiceInterface(t)
	system := models.Type{
		ID:           "550e8400-e29b-41d4-a716-446655440000",
		ResourceType: "artifacts",
		Slug:         "general",
		Name:         "General",
		IsSystem:     true,
		CreatedAt:    time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC),
	}
	custom := models.Type{
		ID:           "770e8400-e29b-41d4-a716-446655440002",
		TeamID:       testTypesTeamID,
		ResourceType: "artifacts",
		Slug:         "bug-report",
		Name:         "Bug report",
		IsSystem:     false,
		CreatedBy:    testTypesUserID,
		CreatedAt:    time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC),
	}
	svc.EXPECT().List(mock.Anything, testTypesTeamID, "artifacts").
		Return([]models.Type{system, custom}, nil)

	srv := createTestTypesServer(svc)
	req := makeTypesRequest("GET", "/api/v1/"+testTypesTeamID+"/types?resource_type=artifacts", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.EqualValues(t, 2, resp["total_count"])

	items := resp["types"].([]interface{})
	require.Len(t, items, 2)

	first := items[0].(map[string]interface{})
	assert.Equal(t, "general", first["slug"])
	assert.Equal(t, true, first["is_system"])
	// team_id is omitted for global system defaults.
	_, hasTeam := first["team_id"]
	assert.False(t, hasTeam, "system default must omit team_id")

	second := items[1].(map[string]interface{})
	assert.Equal(t, "bug-report", second["slug"])
	assert.Equal(t, testTypesTeamID, second["team_id"])
}

func TestListTypes_UnsupportedResourceType(t *testing.T) {
	svc := servicesmocks.NewMockTypeServiceInterface(t)
	svc.EXPECT().List(mock.Anything, testTypesTeamID, "prompts").
		Return(nil, services.ErrTypeResourceTypeUnsupported)

	srv := createTestTypesServer(svc)
	req := makeTypesRequest("GET", "/api/v1/"+testTypesTeamID+"/types?resource_type=prompts", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertTypesProblem(t, w, http.StatusBadRequest, "BAD_REQUEST")
}

func TestCreateType_Success(t *testing.T) {
	svc := servicesmocks.NewMockTypeServiceInterface(t)
	created := &models.Type{
		ID:           "770e8400-e29b-41d4-a716-446655440002",
		TeamID:       testTypesTeamID,
		ResourceType: "artifacts",
		Slug:         "bug-report",
		Name:         "Bug report",
		IsSystem:     false,
		CreatedBy:    testTypesUserID,
		CreatedAt:    time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC),
	}
	svc.EXPECT().CreateCustom(mock.Anything, services.CreateTypeParams{
		TeamID:       testTypesTeamID,
		UserID:       testTypesUserID,
		ResourceType: "artifacts",
		Slug:         "bug-report",
		Name:         "Bug report",
	}).Return(created, nil)

	srv := createTestTypesServer(svc)
	body := `{"resource_type":"artifacts","slug":"bug-report","name":"Bug report"}`
	req := makeTypesRequest("POST", "/api/v1/"+testTypesTeamID+"/types", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "bug-report", resp["slug"])
	assert.Equal(t, "Bug report", resp["name"])
	assert.Equal(t, false, resp["is_system"])
	assert.Equal(t, testTypesTeamID, resp["team_id"])
}

func TestCreateType_ValidationError(t *testing.T) {
	svc := servicesmocks.NewMockTypeServiceInterface(t)
	svc.EXPECT().CreateCustom(mock.Anything, mock.AnythingOfType("services.CreateTypeParams")).
		Return(nil, services.ErrTypeSlugInvalid)

	srv := createTestTypesServer(svc)
	body := `{"resource_type":"artifacts","slug":"Bad Slug","name":"X"}`
	req := makeTypesRequest("POST", "/api/v1/"+testTypesTeamID+"/types", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertTypesProblem(t, w, http.StatusBadRequest, "BAD_REQUEST")
}

func TestCreateType_Conflict(t *testing.T) {
	svc := servicesmocks.NewMockTypeServiceInterface(t)
	svc.EXPECT().CreateCustom(mock.Anything, mock.AnythingOfType("services.CreateTypeParams")).
		Return(nil, repositories.ErrTypeAlreadyExists)

	srv := createTestTypesServer(svc)
	body := `{"resource_type":"artifacts","slug":"general","name":"General"}`
	req := makeTypesRequest("POST", "/api/v1/"+testTypesTeamID+"/types", body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertTypesProblem(t, w, http.StatusConflict, "RESOURCE_EXISTS")
}

func TestDeleteType_Success(t *testing.T) {
	typeID := "770e8400-e29b-41d4-a716-446655440002"
	svc := servicesmocks.NewMockTypeServiceInterface(t)
	svc.EXPECT().Delete(mock.Anything, testTypesTeamID, typeID).Return(nil)

	srv := createTestTypesServer(svc)
	req := makeTypesRequest("DELETE", "/api/v1/"+testTypesTeamID+"/types/"+typeID, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestDeleteType_NotFound(t *testing.T) {
	typeID := "770e8400-e29b-41d4-a716-446655440002"
	svc := servicesmocks.NewMockTypeServiceInterface(t)
	svc.EXPECT().Delete(mock.Anything, testTypesTeamID, typeID).
		Return(repositories.ErrTypeNotFound)

	srv := createTestTypesServer(svc)
	req := makeTypesRequest("DELETE", "/api/v1/"+testTypesTeamID+"/types/"+typeID, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertTypesProblem(t, w, http.StatusNotFound, "RESOURCE_NOT_FOUND")
}

func TestDeleteType_InvalidUUID(t *testing.T) {
	// The generated binder rejects a non-UUID id before the handler runs; the
	// bind error handler maps it to a 400 problem+json.
	svc := servicesmocks.NewMockTypeServiceInterface(t)
	srv := createTestTypesServer(svc)
	req := makeTypesRequest("DELETE", "/api/v1/"+testTypesTeamID+"/types/not-a-uuid", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertTypesProblem(t, w, http.StatusBadRequest, "BAD_REQUEST")
}
