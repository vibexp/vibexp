package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/repositories"
	metadatagen "github.com/vibexp/vibexp/internal/server/gen/metadata"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	testMetadataTeamID  = "550e8400-e29b-41d4-a716-446655440000"
	testMetadataUserID  = "660e8400-e29b-41d4-a716-446655440001"
	testMetadataProject = "770e8400-e29b-41d4-a716-446655440002"
	metadataKeysPath    = "/api/v1/" + testMetadataTeamID + "/metadata/keys"
	metadataValuesPath  = "/api/v1/" + testMetadataTeamID + "/metadata/values"
)

// MockMetadataContainer overrides the metadata catalog service on the base
// container.
type MockMetadataContainer struct {
	BaseMockContainer
	metadataCatalogService services.MetadataCatalogServiceInterface
}

func (c *MockMetadataContainer) MetadataCatalogService() services.MetadataCatalogServiceInterface {
	return c.metadataCatalogService
}

func createTestMetadataServer(svc services.MetadataCatalogServiceInterface) *Server {
	r := chi.NewRouter()
	srv := &Server{
		container: &MockMetadataContainer{metadataCatalogService: svc},
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
		router:    r,
	}
	strict := metadatagen.NewStrictHandlerWithOptions(
		&metadataStrictServer{s: srv},
		nil,
		metadatagen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  srv.metadataBindErrorHandler,
			ResponseErrorHandlerFunc: srv.metadataResponseErrorHandler,
		},
	)
	metadatagen.HandlerWithOptions(strict, metadatagen.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: srv.metadataBindErrorHandler,
	})
	return srv
}

func makeMetadataRequest(path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testMetadataUserID))
}

func TestGetMetadataKeys_ReturnsSpecConformantBody(t *testing.T) {
	svc := servicesmocks.NewMockMetadataCatalogServiceInterface(t)
	svc.EXPECT().Keys(mock.Anything, repositories.MetadataCatalogQuery{
		UserID:       testMetadataUserID,
		TeamID:       testMetadataTeamID,
		ResourceType: repositories.MetadataResourceBlueprints,
		Limit:        0,
	}).Return(repositories.MetadataCatalogResult{
		Entries:   []string{"env", "spec.type"},
		Truncated: false,
	}, nil)

	srv := createTestMetadataServer(svc)
	req := makeMetadataRequest(metadataKeysPath + "?resource_type=blueprints")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Keys      []string `json:"keys"`
		Truncated bool     `json:"truncated"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// A dotted key is the case the widened allowlist exists for: writable
	// before #519, but never filterable.
	assert.Equal(t, []string{"env", "spec.type"}, resp.Keys)
	assert.False(t, resp.Truncated)
}

func TestGetMetadataKeys_PassesProjectAndLimitThrough(t *testing.T) {
	projectID := testMetadataProject
	svc := servicesmocks.NewMockMetadataCatalogServiceInterface(t)
	svc.EXPECT().Keys(mock.Anything, repositories.MetadataCatalogQuery{
		UserID:       testMetadataUserID,
		TeamID:       testMetadataTeamID,
		ResourceType: repositories.MetadataResourceArtifacts,
		ProjectID:    &projectID,
		Limit:        5,
	}).Return(repositories.MetadataCatalogResult{Entries: []string{"a"}, Truncated: true}, nil)

	srv := createTestMetadataServer(svc)
	req := makeMetadataRequest(
		metadataKeysPath + "?resource_type=artifacts&project_id=" + testMetadataProject + "&limit=5",
	)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Truncated bool `json:"truncated"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// Truncation must be reported, not silently swallowed — a typeahead that
	// shows a partial list without saying so is worse than one that admits it.
	assert.True(t, resp.Truncated)
}

// TestGetMetadataKeys_EmptyCatalogSerializesAsArray guards the required-array
// invariant (#125): a generated strict-server type cannot use
// models.JSONArray, so the handler has to guarantee [] itself.
func TestGetMetadataKeys_EmptyCatalogSerializesAsArray(t *testing.T) {
	svc := servicesmocks.NewMockMetadataCatalogServiceInterface(t)
	svc.EXPECT().Keys(mock.Anything, mock.Anything).
		Return(repositories.MetadataCatalogResult{Entries: nil}, nil)

	srv := createTestMetadataServer(svc)
	req := makeMetadataRequest(metadataKeysPath + "?resource_type=memories")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"keys":[],"truncated":false}`, w.Body.String())
}

func TestGetMetadataValues_ReturnsSpecConformantBody(t *testing.T) {
	search := "pro"
	svc := servicesmocks.NewMockMetadataCatalogServiceInterface(t)
	svc.EXPECT().Values(mock.Anything, repositories.MetadataCatalogQuery{
		UserID:       testMetadataUserID,
		TeamID:       testMetadataTeamID,
		ResourceType: repositories.MetadataResourceBlueprints,
		Key:          "env",
		Search:       &search,
		Limit:        100,
	}).Return(repositories.MetadataCatalogResult{
		Entries:   []string{"prod", "production"},
		Truncated: false,
	}, nil)

	srv := createTestMetadataServer(svc)
	req := makeMetadataRequest(metadataValuesPath + "?resource_type=blueprints&key=env&q=pro&limit=100")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Values    []string `json:"values"`
		Truncated bool     `json:"truncated"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, []string{"prod", "production"}, resp.Values)
}

func TestGetMetadataValues_EmptyCatalogSerializesAsArray(t *testing.T) {
	svc := servicesmocks.NewMockMetadataCatalogServiceInterface(t)
	svc.EXPECT().Values(mock.Anything, mock.Anything).
		Return(repositories.MetadataCatalogResult{Entries: nil}, nil)

	srv := createTestMetadataServer(svc)
	req := makeMetadataRequest(metadataValuesPath + "?resource_type=artifacts&key=env")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"values":[],"truncated":false}`, w.Body.String())
}

// TestMetadataCatalog_RejectsUnknownResourceType covers the trap that
// oapi-codegen generates a Valid() helper for an enum query param and never
// calls it: without the explicit check the value would bind as a raw string
// and reach the repository.
func TestMetadataCatalog_RejectsUnknownResourceType(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "keys", path: metadataKeysPath + "?resource_type=prompts"},
		{name: "values", path: metadataValuesPath + "?resource_type=prompts&key=env"},
		{
			name: "keys with an injection attempt",
			path: metadataKeysPath + "?resource_type=artifacts%3B+DROP+TABLE+artifacts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No EXPECT calls: the service must never be reached.
			svc := servicesmocks.NewMockMetadataCatalogServiceInterface(t)

			srv := createTestMetadataServer(svc)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, makeMetadataRequest(tt.path))

			assertCommentsProblem(t, w, http.StatusBadRequest, "BAD_REQUEST")
			assert.Contains(t, w.Body.String(), "resource_type must be one of")
		})
	}
}

func TestMetadataCatalog_RejectsMissingRequiredParams(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "keys without resource_type", path: metadataKeysPath},
		{name: "values without resource_type", path: metadataValuesPath + "?key=env"},
		{name: "values without key", path: metadataValuesPath + "?resource_type=artifacts"},
		{name: "keys with a non-uuid project_id", path: metadataKeysPath + "?resource_type=artifacts&project_id=nope"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := servicesmocks.NewMockMetadataCatalogServiceInterface(t)

			srv := createTestMetadataServer(svc)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, makeMetadataRequest(tt.path))

			assertCommentsProblem(t, w, http.StatusBadRequest, "BAD_REQUEST")
		})
	}
}

func TestMetadataCatalog_InvalidTeamIDIsBadRequest(t *testing.T) {
	svc := servicesmocks.NewMockMetadataCatalogServiceInterface(t)

	srv := createTestMetadataServer(svc)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, makeMetadataRequest("/api/v1/not-a-uuid/metadata/keys?resource_type=artifacts"))

	assertCommentsProblem(t, w, http.StatusBadRequest, "BAD_REQUEST")
	assert.Contains(t, w.Body.String(), "team_id must be a valid UUID")
}

// TestMetadataCatalog_UnauthenticatedIsRejected covers the missing-user path:
// the generated layer knows nothing about auth, so the handler must not assume
// the middleware ran.
func TestMetadataCatalog_UnauthenticatedIsRejected(t *testing.T) {
	svc := servicesmocks.NewMockMetadataCatalogServiceInterface(t)

	srv := createTestMetadataServer(svc)
	w := httptest.NewRecorder()
	// Deliberately no contextKeyUserID on the request context.
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, metadataKeysPath+"?resource_type=artifacts", nil))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMetadataCatalog_ServiceRejectionIsBadRequest(t *testing.T) {
	svc := servicesmocks.NewMockMetadataCatalogServiceInterface(t)
	svc.EXPECT().Values(mock.Anything, mock.Anything).Return(
		repositories.MetadataCatalogResult{},
		fmt.Errorf("%w: key is required", services.ErrInvalidMetadataCatalogQuery),
	)

	srv := createTestMetadataServer(svc)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, makeMetadataRequest(metadataValuesPath+"?resource_type=artifacts&key=env"))

	assertCommentsProblem(t, w, http.StatusBadRequest, "BAD_REQUEST")
}

func TestMetadataCatalog_RepositoryFailureIsInternalError(t *testing.T) {
	svc := servicesmocks.NewMockMetadataCatalogServiceInterface(t)
	svc.EXPECT().Keys(mock.Anything, mock.Anything).
		Return(repositories.MetadataCatalogResult{}, errors.New("connection refused"))

	srv := createTestMetadataServer(svc)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, makeMetadataRequest(metadataKeysPath+"?resource_type=artifacts"))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	// The database error text must not leak to the caller.
	assert.NotContains(t, w.Body.String(), "connection refused")
}

// TestMetadataResponseErrorHandler_NonAPIErrorIs500 covers the defensive arm of
// the strict response handler, which no request path can reach.
func TestMetadataResponseErrorHandler_NonAPIErrorIs500(t *testing.T) {
	srv := createTestMetadataServer(servicesmocks.NewMockMetadataCatalogServiceInterface(t))

	w := httptest.NewRecorder()
	req := makeMetadataRequest(metadataKeysPath)
	srv.metadataResponseErrorHandler(w, req, errors.New("boom"))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "boom")
}
