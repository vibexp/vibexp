package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	getProjTeamID    = "5d0c2f7e-2a4b-4b8e-9f61-0c1d2e3f4a5b"
	getProjUserID    = "7e1f3a2b-4c5d-4e6f-8a9b-0c1d2e3f4a5b"
	getProjProjectID = "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d"
)

// makeGetProjectRequest builds GET /api/v1/{team_id}/projects/{ref} with the
// user and chi URL params injected, so handleGetProject runs without middleware.
func makeGetProjectRequest(ref string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/"+getProjTeamID+"/projects/"+ref, nil)
	ctx := context.WithValue(req.Context(), contextKeyUserID, getProjUserID)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("team_id", getProjTeamID)
	rctx.URLParams.Add("slug", ref)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	return req.WithContext(ctx)
}

// A persisted project ID must resolve through getProject (issue #957), and the
// response must satisfy the spec.
func TestHandleGetProject_ResolvesProjectID_ConformsToSpec(t *testing.T) {
	container := newProjectGitHubTestContainer(t)
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	container.projectSvc.EXPECT().
		GetProjectBySlugOrID(getProjTeamID, getProjUserID, getProjProjectID).
		Return(&models.Project{
			ID:        getProjProjectID,
			UserID:    getProjUserID,
			TeamID:    getProjTeamID,
			Name:      "Project 101",
			Slug:      "project-101",
			CreatedAt: now,
			UpdatedAt: now,
			Version:   1,
		}, nil).Once()

	req := makeGetProjectRequest(getProjProjectID)
	w := httptest.NewRecorder()
	createProjectGitHubServer(container).handleGetProject(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, getProjProjectID, body["id"])
}

// An ID the service cannot resolve in this team (deleted, or another team's)
// is a 404, which is what lets a client clear a stale persisted selection.
func TestHandleGetProject_UnresolvedRef_Returns404(t *testing.T) {
	container := newProjectGitHubTestContainer(t)
	container.projectSvc.EXPECT().
		GetProjectBySlugOrID(getProjTeamID, getProjUserID, getProjProjectID).
		Return(nil, fmt.Errorf("%w: id=%s", repositories.ErrProjectNotFoundForRepo, getProjProjectID)).Once()

	req := makeGetProjectRequest(getProjProjectID)
	w := httptest.NewRecorder()
	createProjectGitHubServer(container).handleGetProject(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
}
