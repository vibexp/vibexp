package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// These tests ratchet the artifact detail-GET onto spec conformance (issue
// #424, CLAUDE.md #122) as it gains the required `related` array, and pin that
// the depth-1 neighborhood is populated from RelationService with hydrated
// titles — and is [] (never null) when the resource has no edges.

const (
	relArtTeamID  = "550e8400-e29b-41d4-a716-446655440000"
	relArtProject = "550e8400-e29b-41d4-a716-446655440000"
	relArtUserID  = "user-123"
)

func newArtifactRelatedServer(
	artSvc *servicesmocks.MockArtifactServiceInterface, relSvc *servicesmocks.MockRelationServiceInterface,
) *Server {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = &MockArtifactContainer{
		ArtifactServiceMock: artSvc,
		RelationServiceMock: relSvc,
	}
	return srv
}

func getArtifactRelatedRequest() *http.Request {
	url := "/api/v1/" + relArtTeamID + "/artifacts/test-project/test-slug"
	req := createAuthenticatedRequest("GET", url, "", relArtUserID)
	return addURLParams(req, map[string]string{
		"team_id":    relArtTeamID,
		"project_id": relArtProject,
		"slug":       "test-slug",
	})
}

func sampleRelArtifact() *models.Artifact {
	return &models.Artifact{
		ID:        "art-1",
		ProjectID: relArtProject,
		Slug:      "test-slug",
		Title:     "Test Artifact",
		UserID:    relArtUserID,
		Type:      "general",
		Status:    "active",
		Metadata:  map[string]interface{}{"key": "value"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func expectGetArtifact(m *servicesmocks.MockArtifactServiceInterface) {
	m.On("GetArtifactByProjectIDAndSlugInTeam", relArtUserID, relArtProject, relArtProject, "test-slug").
		Return(sampleRelArtifact(), nil)
}

func TestHandleGetArtifact_RelatedPopulated_ConformsToSpec(t *testing.T) {
	artSvc := servicesmocks.NewMockArtifactServiceInterface(t)
	expectGetArtifact(artSvc)

	relSvc := servicesmocks.NewMockRelationServiceInterface(t)
	relSvc.EXPECT().ListByResource(
		mock.Anything, relArtUserID, relArtTeamID, models.RelationResourceTypeArtifact, "art-1", 1, relatedOnReadCap,
	).Return(&models.RelationListResponse{
		Related: []models.RelatedResource{{
			RelationID:   "770e8400-e29b-41d4-a716-446655440002",
			RelationType: models.RelationTypeGovernedBy,
			Direction:    models.RelationDirectionOutgoing,
			Origin:       models.RelationOriginHuman,
			Status:       models.RelationStatusConfirmed,
			ResourceType: models.RelationResourceTypeBlueprint,
			ResourceID:   "880e8400-e29b-41d4-a716-446655440003",
			Title:        "Go coding standards",
			CreatedAt:    time.Now(),
		}},
		TotalCount: 1, Page: 1, PerPage: 20, TotalPages: 1,
	}, nil)

	srv := newArtifactRelatedServer(artSvc, relSvc)
	req := getArtifactRelatedRequest()
	rr := httptest.NewRecorder()
	srv.handleGetArtifact(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	var resp models.Artifact
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, []models.RelatedResource(resp.Related), 1)
	assert.Equal(t, "Go coding standards", resp.Related[0].Title)
	assert.Equal(t, models.RelationResourceTypeBlueprint, resp.Related[0].ResourceType)
	assert.Equal(t, models.RelationDirectionOutgoing, resp.Related[0].Direction)
}

// A resource with zero edges must serialize related as [] — never null, never absent.
func TestHandleGetArtifact_RelatedEmpty_IsArrayNotNull(t *testing.T) {
	artSvc := servicesmocks.NewMockArtifactServiceInterface(t)
	expectGetArtifact(artSvc)

	relSvc := servicesmocks.NewMockRelationServiceInterface(t)
	relSvc.EXPECT().ListByResource(
		mock.Anything, relArtUserID, relArtTeamID, models.RelationResourceTypeArtifact, "art-1", 1, relatedOnReadCap,
	).Return(&models.RelationListResponse{
		Related: nil, TotalCount: 0, Page: 1, PerPage: 20, TotalPages: 0,
	}, nil)

	srv := newArtifactRelatedServer(artSvc, relSvc)
	req := getArtifactRelatedRequest()
	rr := httptest.NewRecorder()
	srv.handleGetArtifact(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	assert.Contains(t, rr.Body.String(), `"related":[]`)
	assert.NotContains(t, rr.Body.String(), `"related":null`)
}

// Best-effort: a RelationService failure must not fail the resource read; the
// artifact still returns 200 with related:[].
func TestHandleGetArtifact_RelatedServiceError_StillReturnsArtifact(t *testing.T) {
	artSvc := servicesmocks.NewMockArtifactServiceInterface(t)
	expectGetArtifact(artSvc)

	relSvc := servicesmocks.NewMockRelationServiceInterface(t)
	relSvc.EXPECT().ListByResource(
		mock.Anything, relArtUserID, relArtTeamID, models.RelationResourceTypeArtifact, "art-1", 1, relatedOnReadCap,
	).Return(nil, assert.AnError)

	srv := newArtifactRelatedServer(artSvc, relSvc)
	req := getArtifactRelatedRequest()
	rr := httptest.NewRecorder()
	srv.handleGetArtifact(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"related":[]`)
}
