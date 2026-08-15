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
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// The `metadata` query parameter (epic #519) is shared by the artifact,
// blueprint and memory list handlers. These tests cover the two things the
// per-domain handler tests do not: that the parsed filter actually reaches the
// service, and that a malformed one is rejected with a 400 before any query
// runs.

const metadataParamTeamID = "550e8400-e29b-41d4-a716-446655440000"

func metadataParamServer(t *testing.T, svc services.ArtifactServiceInterface) *Server {
	t.Helper()
	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
	srv.container = &MockArtifactContainer{ArtifactServiceMock: svc}
	return mountArtifactReadRoutes(srv)
}

func TestListArtifacts_MetadataParamReachesTheService(t *testing.T) {
	svc := servicesmocks.NewMockArtifactServiceInterface(t)
	svc.On("ListArtifacts", "user-123", mock.MatchedBy(func(filters services.ArtifactFilters) bool {
		return assert.ObjectsAreEqual(repositories.MetadataFilter{
			"env":  {"prod", "staging"},
			"team": {"core"},
		}, filters.MetadataFilter)
	})).Return(&models.ArtifactListResponse{Artifacts: []models.Artifact{}}, nil)

	srv := metadataParamServer(t, svc)

	req := createAuthenticatedRequest("GET",
		`/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?metadata=%7B%22env%22%3A%5B%22prod%22%2C%22staging%22%5D%2C%22team%22%3A%5B%22core%22%5D%7D`,
		"", "user-123")
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestListArtifacts_AbsentMetadataParamIsANoOpFilter(t *testing.T) {
	svc := servicesmocks.NewMockArtifactServiceInterface(t)
	svc.On("ListArtifacts", "user-123", mock.MatchedBy(func(filters services.ArtifactFilters) bool {
		return filters.MetadataFilter == nil
	})).Return(&models.ArtifactListResponse{Artifacts: []models.Artifact{}}, nil)

	srv := metadataParamServer(t, svc)

	req := createAuthenticatedRequest("GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts", "", "user-123")
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

// TestListArtifacts_MalformedMetadataParamIs400 asserts the request is refused
// before the service is consulted — the mock has no expectations, so any call
// fails the test.
func TestListArtifacts_MalformedMetadataParamIs400(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "not JSON", raw: "%7B"},
		{name: "a JSON array", raw: "%5B%22env%22%5D"},
		{name: "scalar value", raw: `%7B%22env%22%3A%22prod%22%7D`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := servicesmocks.NewMockArtifactServiceInterface(t)
			srv := metadataParamServer(t, svc)

			req := createAuthenticatedRequest("GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?metadata="+tt.raw, "", "user-123")
			req = addURLParams(req, map[string]string{"team_id": metadataParamTeamID})
			rr := httptest.NewRecorder()
			srv.router.ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Equal(t, "application/problem+json", rr.Header().Get("Content-Type"))

			var problem struct {
				Detail string `json:"detail"`
			}
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &problem))
			assert.Contains(t, problem.Detail, "invalid metadata filter")
		})
	}
}

func TestListArtifactsByProject_MalformedMetadataParamIs400(t *testing.T) {
	svc := servicesmocks.NewMockArtifactServiceInterface(t)
	srv := metadataParamServer(t, svc)

	projectID := "660e8400-e29b-41d4-a716-446655440001"
	req := createAuthenticatedRequest("GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/"+projectID+"?metadata=%7B", "", "user-123")
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
