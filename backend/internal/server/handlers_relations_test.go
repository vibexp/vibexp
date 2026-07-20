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
	relationsgen "github.com/vibexp/vibexp/internal/server/gen/relations"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	testRelationsTeamID     = "660e8400-e29b-41d4-a716-446655440001"
	testRelationsUserID     = "880e8400-e29b-41d4-a716-446655440003"
	testRelationsFromID     = "770e8400-e29b-41d4-a716-446655440002"
	testRelationsToID       = "990e8400-e29b-41d4-a716-446655440004"
	testRelationsRelationID = "550e8400-e29b-41d4-a716-446655440000"
	testRelationsProjectID  = "aa0e8400-e29b-41d4-a716-446655440005"
)

// MockRelationsContainer overrides the relation, relation-seed, and
// authorization services on the base container.
type MockRelationsContainer struct {
	BaseMockContainer
	relationService     services.RelationServiceInterface
	relationSeedService services.RelationSeedServiceInterface
	authzService        services.AuthorizationServiceInterface
}

func (c *MockRelationsContainer) RelationService() services.RelationServiceInterface {
	return c.relationService
}

func (c *MockRelationsContainer) RelationSeedService() services.RelationSeedServiceInterface {
	return c.relationSeedService
}

func (c *MockRelationsContainer) AuthorizationService() services.AuthorizationServiceInterface {
	return c.authzService
}

func createTestRelationsServer(svc services.RelationServiceInterface) *Server {
	logger := slog.New(slog.DiscardHandler)

	r := chi.NewRouter()
	srv := &Server{
		container: &MockRelationsContainer{relationService: svc},
		logger:    logger,
		config:    &config.Config{},
		router:    r,
	}
	strict := relationsgen.NewStrictHandlerWithOptions(
		&relationsStrictServer{s: srv},
		nil,
		relationsgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  srv.relationsBindErrorHandler,
			ResponseErrorHandlerFunc: srv.relationsResponseErrorHandler,
		},
	)
	relationsgen.HandlerWithOptions(strict, relationsgen.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: srv.relationsBindErrorHandler,
	})
	return srv
}

func makeRelationsRequest(method, path, body string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testRelationsUserID))
}

func sampleRelation() models.Relation {
	createdBy := testRelationsUserID
	return models.Relation{
		ID:           testRelationsRelationID,
		TeamID:       testRelationsTeamID,
		ProjectID:    testRelationsProjectID,
		FromType:     models.RelationResourceTypeArtifact,
		FromID:       testRelationsFromID,
		ToType:       models.RelationResourceTypeBlueprint,
		ToID:         testRelationsToID,
		RelationType: models.RelationTypeGovernedBy,
		Origin:       models.RelationOriginHuman,
		Status:       models.RelationStatusConfirmed,
		CreatedBy:    &createdBy,
		CreatedAt:    time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC),
	}
}

func sampleRelatedResource() models.RelatedResource {
	projectID := testRelationsProjectID
	slug := "go-coding-standards"
	return models.RelatedResource{
		RelationID:   testRelationsRelationID,
		RelationType: models.RelationTypeGovernedBy,
		Direction:    models.RelationDirectionOutgoing,
		Origin:       models.RelationOriginHuman,
		Status:       models.RelationStatusConfirmed,
		ResourceType: models.RelationResourceTypeBlueprint,
		ResourceID:   testRelationsToID,
		Title:        "Go coding standards",
		ProjectID:    &projectID,
		Slug:         &slug,
		CreatedAt:    time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC),
	}
}

func createRelationBody() string {
	return `{"from_type":"artifact","from_id":"` + testRelationsFromID +
		`","to_type":"blueprint","to_id":"` + testRelationsToID +
		`","relation_type":"governed-by","origin":"human"}`
}

func TestListRelations_Success(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	svc.EXPECT().ListByResource(
		mock.Anything, testRelationsUserID, testRelationsTeamID,
		models.RelationResourceTypeArtifact, testRelationsFromID, mock.Anything, mock.Anything,
	).Return(&models.RelationListResponse{
		Related:    []models.RelatedResource{sampleRelatedResource()},
		TotalCount: 1,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}, nil)

	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("GET",
		"/api/v1/"+testRelationsTeamID+"/relations?resource_type=artifact&resource_id="+testRelationsFromID, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.EqualValues(t, 1, resp["total_count"])
	require.Len(t, resp["relations"].([]interface{}), 1)
}

func TestListRelations_NonMemberForbidden(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	svc.EXPECT().ListByResource(
		mock.Anything, testRelationsUserID, testRelationsTeamID,
		models.RelationResourceTypeArtifact, testRelationsFromID, mock.Anything, mock.Anything,
	).Return(nil, assertErrNotMember())

	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("GET",
		"/api/v1/"+testRelationsTeamID+"/relations?resource_type=artifact&resource_id="+testRelationsFromID, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusForbidden, "FORBIDDEN")
}

func TestCreateRelation_Created(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	created := sampleRelation()
	svc.EXPECT().Create(mock.Anything, testRelationsUserID, testRelationsTeamID, mock.MatchedBy(
		func(req *models.CreateRelationRequest) bool {
			return req.FromType == models.RelationResourceTypeArtifact &&
				req.ToType == models.RelationResourceTypeBlueprint &&
				req.RelationType == models.RelationTypeGovernedBy && req.Origin == models.RelationOriginHuman
		},
	)).Return(&created, true, nil)

	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("POST", "/api/v1/"+testRelationsTeamID+"/relations", createRelationBody())
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "governed-by", resp["relation_type"])
	assert.Equal(t, "confirmed", resp["status"])
}

// Idempotent create: a pre-existing edge (created=false) is 200, not 201.
func TestCreateRelation_ExistingReturns200(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	existing := sampleRelation()
	svc.EXPECT().Create(mock.Anything, testRelationsUserID, testRelationsTeamID, mock.Anything).
		Return(&existing, false, nil)

	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("POST", "/api/v1/"+testRelationsTeamID+"/relations", createRelationBody())
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestCreateRelation_MatrixViolation(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	svc.EXPECT().Create(mock.Anything, testRelationsUserID, testRelationsTeamID, mock.Anything).
		Return(nil, false, services.ErrRelationInvalidType)

	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("POST", "/api/v1/"+testRelationsTeamID+"/relations", createRelationBody())
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusBadRequest, "BAD_REQUEST")
}

func TestCreateRelation_EndpointNotFound(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	svc.EXPECT().Create(mock.Anything, testRelationsUserID, testRelationsTeamID, mock.Anything).
		Return(nil, false, services.ErrRelationResourceNotFound)

	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("POST", "/api/v1/"+testRelationsTeamID+"/relations", createRelationBody())
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusNotFound, "RESOURCE_NOT_FOUND")
}

func TestCreateRelation_Forbidden(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	svc.EXPECT().Create(mock.Anything, testRelationsUserID, testRelationsTeamID, mock.Anything).
		Return(nil, false, services.ErrPermissionDenied)

	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("POST", "/api/v1/"+testRelationsTeamID+"/relations", createRelationBody())
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusForbidden, "FORBIDDEN")
}

func TestConfirmRelation_Success(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	confirmed := sampleRelation()
	svc.EXPECT().Confirm(mock.Anything, testRelationsUserID, testRelationsTeamID, testRelationsRelationID).
		Return(&confirmed, nil)

	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("POST",
		"/api/v1/"+testRelationsTeamID+"/relations/"+testRelationsRelationID+"/confirm", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "confirmed", resp["status"])
}

func TestConfirmRelation_AlreadyConfirmed(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	svc.EXPECT().Confirm(mock.Anything, testRelationsUserID, testRelationsTeamID, testRelationsRelationID).
		Return(nil, services.ErrRelationAlreadyConfirmed)

	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("POST",
		"/api/v1/"+testRelationsTeamID+"/relations/"+testRelationsRelationID+"/confirm", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusConflict, "RESOURCE_CONFLICT")
}

func TestConfirmRelation_NotFound(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	svc.EXPECT().Confirm(mock.Anything, testRelationsUserID, testRelationsTeamID, testRelationsRelationID).
		Return(nil, repositories.ErrRelationNotFound)

	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("POST",
		"/api/v1/"+testRelationsTeamID+"/relations/"+testRelationsRelationID+"/confirm", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusNotFound, "RESOURCE_NOT_FOUND")
}

func TestDeleteRelation_Success(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	svc.EXPECT().Delete(mock.Anything, testRelationsUserID, testRelationsTeamID, testRelationsRelationID).
		Return(nil)

	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("DELETE", "/api/v1/"+testRelationsTeamID+"/relations/"+testRelationsRelationID, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestDeleteRelation_Forbidden(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	svc.EXPECT().Delete(mock.Anything, testRelationsUserID, testRelationsTeamID, testRelationsRelationID).
		Return(services.ErrPermissionDenied)

	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("DELETE", "/api/v1/"+testRelationsTeamID+"/relations/"+testRelationsRelationID, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusForbidden, "FORBIDDEN")
}

func TestDeleteRelation_NotFound(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	svc.EXPECT().Delete(mock.Anything, testRelationsUserID, testRelationsTeamID, testRelationsRelationID).
		Return(repositories.ErrRelationNotFound)

	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("DELETE", "/api/v1/"+testRelationsTeamID+"/relations/"+testRelationsRelationID, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusNotFound, "RESOURCE_NOT_FOUND")
}

// A malformed relation_id must be a 400 from the bind-error handler, before the service.
func TestDeleteRelation_InvalidUUID(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("DELETE", "/api/v1/"+testRelationsTeamID+"/relations/not-a-uuid", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusBadRequest, "BAD_REQUEST")
}
