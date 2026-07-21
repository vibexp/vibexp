package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// These ratchet the computed `similar` array (issue #427) onto the artifact
// detail GET: it is populated from embedding similarity, and degrades to [] (not
// null, not an error) when the resource has no stored embedding.

func newArtifactSimilarServer(
	artSvc *servicesmocks.MockArtifactServiceInterface, embRepo *repomocks.MockEmbeddingRepository,
) *Server {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = &MockArtifactContainer{
		ArtifactServiceMock:     artSvc,
		EmbeddingRepositoryMock: embRepo,
	}
	return srv
}

func TestHandleGetArtifact_SimilarPopulated_ConformsToSpec(t *testing.T) {
	artSvc := servicesmocks.NewMockArtifactServiceInterface(t)
	expectGetArtifact(artSvc)

	embRepo := repomocks.NewMockEmbeddingRepository(t)
	embRepo.EXPECT().FindSimilarInTeam(
		mock.Anything, relArtTeamID, models.RelationResourceTypeArtifact, "art-1", similarOnReadCap,
	).Return([]models.SimilarResource{{
		Type: models.RelationResourceTypeMemory, ID: "990e8400-e29b-41d4-a716-446655440004",
		Title: "Why we chose pgvector", Score: 0.82,
	}}, nil)

	srv := newArtifactSimilarServer(artSvc, embRepo)
	req := getArtifactRelatedRequest()
	rr := httptest.NewRecorder()
	srv.handleGetArtifact(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	var resp models.Artifact
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, []models.SimilarResource(resp.Similar), 1)
	assert.Equal(t, "Why we chose pgvector", resp.Similar[0].Title)
	assert.InDelta(t, 0.82, resp.Similar[0].Score, 1e-9)
	// `related` and `similar` are distinct and never mixed.
	assert.Empty(t, []models.RelatedResource(resp.Related))
}

// No stored embedding → similar:[] (never null), 200, no error.
func TestHandleGetArtifact_SimilarNoEmbedding_IsArrayNotNull(t *testing.T) {
	artSvc := servicesmocks.NewMockArtifactServiceInterface(t)
	expectGetArtifact(artSvc)

	embRepo := repomocks.NewMockEmbeddingRepository(t)
	embRepo.EXPECT().FindSimilarInTeam(
		mock.Anything, relArtTeamID, models.RelationResourceTypeArtifact, "art-1", similarOnReadCap,
	).Return([]models.SimilarResource{}, nil)

	srv := newArtifactSimilarServer(artSvc, embRepo)
	req := getArtifactRelatedRequest()
	rr := httptest.NewRecorder()
	srv.handleGetArtifact(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
	assert.Contains(t, rr.Body.String(), `"similar":[]`)
	assert.NotContains(t, rr.Body.String(), `"similar":null`)
}

// A similarity-load failure must not fail the read.
func TestHandleGetArtifact_SimilarServiceError_StillReturnsArtifact(t *testing.T) {
	artSvc := servicesmocks.NewMockArtifactServiceInterface(t)
	expectGetArtifact(artSvc)

	embRepo := repomocks.NewMockEmbeddingRepository(t)
	embRepo.EXPECT().FindSimilarInTeam(
		mock.Anything, relArtTeamID, models.RelationResourceTypeArtifact, "art-1", similarOnReadCap,
	).Return(nil, assert.AnError)

	srv := newArtifactSimilarServer(artSvc, embRepo)
	req := getArtifactRelatedRequest()
	rr := httptest.NewRecorder()
	srv.handleGetArtifact(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"similar":[]`)
}
