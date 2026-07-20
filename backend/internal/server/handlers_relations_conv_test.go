package server

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	relationsgen "github.com/vibexp/vibexp/internal/server/gen/relations"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// These unit tests exercise the conversion and error-mapping branches the
// spec-conformance handler tests skip (malformed UUIDs from the service layer,
// binding-error parameter arms, and unmapped errors surfacing as 500).

func TestToGenRelation_ConfirmedByIsMapped(t *testing.T) {
	rel := sampleRelation()
	confirmedBy := testRelationsUserID
	rel.ConfirmedBy = &confirmedBy

	got, err := toGenRelation(rel)
	require.NoError(t, err)
	require.NotNil(t, got.ConfirmedBy)
	assert.Equal(t, testRelationsUserID, got.ConfirmedBy.String())
}

func TestToGenRelation_RejectsMalformedUUIDs(t *testing.T) {
	base := sampleRelation()
	bad := "not-a-uuid"
	cases := map[string]func(r *models.Relation){
		"id":           func(r *models.Relation) { r.ID = bad },
		"team_id":      func(r *models.Relation) { r.TeamID = bad },
		"project_id":   func(r *models.Relation) { r.ProjectID = bad },
		"from_id":      func(r *models.Relation) { r.FromID = bad },
		"to_id":        func(r *models.Relation) { r.ToID = bad },
		"created_by":   func(r *models.Relation) { r.CreatedBy = &bad },
		"confirmed_by": func(r *models.Relation) { r.ConfirmedBy = &bad },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			rel := base
			mutate(&rel)
			_, err := toGenRelation(rel)
			require.Error(t, err)
		})
	}
}

func TestToGenRelatedResource_RejectsMalformedUUIDs(t *testing.T) {
	base := sampleRelatedResource()
	bad := "not-a-uuid"
	cases := map[string]func(r *models.RelatedResource){
		"relation_id": func(r *models.RelatedResource) { r.RelationID = bad },
		"resource_id": func(r *models.RelatedResource) { r.ResourceID = bad },
		"project_id":  func(r *models.RelatedResource) { r.ProjectID = &bad },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			rr := base
			mutate(&rr)
			_, err := toGenRelatedResource(rr)
			require.Error(t, err)
		})
	}
}

// A malformed UUID returned by the service surfaces as a 500 through the
// conversion-error path (create and list both convert).
func TestCreateRelation_ConversionErrorIs500(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	bad := sampleRelation()
	bad.ID = "not-a-uuid"
	svc.EXPECT().Create(mock.Anything, testRelationsUserID, testRelationsTeamID, mock.Anything).
		Return(&bad, true, nil)

	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("POST", "/api/v1/"+testRelationsTeamID+"/relations", createRelationBody())
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusInternalServerError, "INTERNAL_ERROR")
}

func TestListRelations_ConversionErrorIs500(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	badRow := sampleRelatedResource()
	badRow.ResourceID = "not-a-uuid"
	svc.EXPECT().ListByResource(
		mock.Anything, testRelationsUserID, testRelationsTeamID,
		models.RelationResourceTypeArtifact, testRelationsFromID, mock.Anything, mock.Anything,
	).Return(&models.RelationListResponse{
		Related: []models.RelatedResource{badRow}, TotalCount: 1, Page: 1, PerPage: 20, TotalPages: 1,
	}, nil)

	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("GET",
		"/api/v1/"+testRelationsTeamID+"/relations?resource_type=artifact&resource_id="+testRelationsFromID, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusInternalServerError, "INTERNAL_ERROR")
}

// An unmapped service error surfaces as a generic 500 (relationError's fallback).
func TestDeleteRelation_UnmappedErrorIs500(t *testing.T) {
	svc := servicesmocks.NewMockRelationServiceInterface(t)
	svc.EXPECT().Delete(mock.Anything, testRelationsUserID, testRelationsTeamID, testRelationsRelationID).
		Return(errors.New("some unexpected failure"))

	srv := createTestRelationsServer(svc)
	req := makeRelationsRequest("DELETE", "/api/v1/"+testRelationsTeamID+"/relations/"+testRelationsRelationID, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusInternalServerError, "INTERNAL_ERROR")
}

func TestMapRelationServiceError_AllArms(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"permission", services.ErrPermissionDenied, http.StatusForbidden},
		{"self-link", services.ErrRelationSelfLink, http.StatusBadRequest},
		{"invalid-type", services.ErrRelationInvalidType, http.StatusBadRequest},
		{"cross-project", services.ErrRelationCrossProject, http.StatusBadRequest},
		{"endpoint-not-found", services.ErrRelationResourceNotFound, http.StatusNotFound},
		{"already-confirmed", services.ErrRelationAlreadyConfirmed, http.StatusConflict},
		{"not-a-member", errors.New("user is not a member of the specified team"), http.StatusForbidden},
		{"invalid-relation-type-string", errors.New(`invalid relation type: "x"`), http.StatusBadRequest},
		{"invalid-origin-string", errors.New(`invalid origin: "x"`), http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			apiErr := mapRelationServiceError(tc.err)
			require.NotNil(t, apiErr)
			assert.Equal(t, tc.status, apiErr.Status)
		})
	}

	assert.Nil(t, mapRelationServiceError(errors.New("something else entirely")),
		"an unmapped error returns nil so the caller emits a 500")
}

func TestRelationsBindErrorHandler_ParamMessages(t *testing.T) {
	params := []string{"team_id", "relation_id", "resource_id", "resource_type", "page", "limit"}
	srv := &Server{}
	for _, p := range params {
		t.Run(p, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)
			srv.relationsBindErrorHandler(w, r, &relationsgen.InvalidParamFormatError{
				ParamName: p, Err: errors.New("bad"),
			})
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestRelationsResponseErrorHandler_NonAPIErrorIs500(t *testing.T) {
	srv := &Server{logger: slog.New(slog.DiscardHandler)}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	srv.relationsResponseErrorHandler(w, r, errors.New("boom"))
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// An APIError passes through with its own status.
	w2 := httptest.NewRecorder()
	srv.relationsResponseErrorHandler(w2, r, apierrors.NewForbiddenError("nope"))
	assert.Equal(t, http.StatusForbidden, w2.Code)
}
