package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// A status outside the resource type's documented subset must answer 400 with an
// ErrorResponse on the create AND update path of all four resource types
// (issue #912). Each subset is a $ref to its own component schema now, so
// "documented" and "enforced" have a single source; these tests are the wire
// half of that -- the spec half is TestResourceStatusSubsetsAreDrawnFromVocabulary
// and TestSpecEnumsMatchServiceAllowlists (internal/services).
//
// The value chosen for each resource is deliberately a status that is VALID for
// one of the other three: "unknown value rejected" is the easy half, and a
// vocabulary shared across four types makes the interesting failure a value
// leaking between subsets.
//
// No service call is expected in any of these: reaching the service is itself
// the failure, since the request never described a state a resource can hold.

const statusTestTeamID = "550e8400-e29b-41d4-a716-446655440000"

// assertStatusRejected pins the shape every one of these rejections shares.
func assertStatusRejected(t *testing.T, req *http.Request, rr *httptest.ResponseRecorder) {
	t.Helper()

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "VALIDATION_FAILED", body["code"])

	detail, ok := body["detail"].(string)
	require.True(t, ok, "the error body must carry a detail string")
	assert.Contains(t, strings.ToLower(detail), "status",
		"the rejection must name the field so a client can act on it")
}

func TestCreatePrompt_RejectsStatusOutsideThePromptSubset(t *testing.T) {
	container := newMockPromptContainer(t)
	// The prompt routes go through the router, which resolves team membership
	// before any handler runs.
	container.teamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", statusTestTeamID).
		Return(true, nil)
	srv := createTestServer(container)

	// "active" is a valid ArtifactStatus/MemoryStatus/BlueprintStatus.
	req := makeAuthenticatedRequest("POST", "/api/v1/"+statusTestTeamID+"/prompts",
		map[string]any{
			"name": "n", "slug": "s", "body": "b",
			"project_id": statusTestTeamID, "status": "active",
		}, "user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assertStatusRejected(t, req, rr)
}

func TestUpdatePrompt_RejectsStatusOutsideThePromptSubset(t *testing.T) {
	container := newMockPromptContainer(t)
	// The prompt routes go through the router, which resolves team membership
	// before any handler runs.
	container.teamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", statusTestTeamID).
		Return(true, nil)
	srv := createTestServer(container)

	req := makeAuthenticatedRequest("PUT", "/api/v1/"+statusTestTeamID+"/prompts/test-slug",
		map[string]any{"status": "archived"}, "user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assertStatusRejected(t, req, rr)
}

func statusArtifactServer(t *testing.T) *Server {
	t.Helper()
	srv := mountArtifactReadRoutes(New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler)))
	srv.container = &MockArtifactContainer{
		ArtifactServiceMock: servicesmocks.NewMockArtifactServiceInterface(t),
	}
	return srv
}

func TestCreateArtifact_RejectsStatusOutsideTheArtifactSubset(t *testing.T) {
	srv := statusArtifactServer(t)

	// "published" is a valid PromptStatus.
	body := `{"project_id":"` + statusTestTeamID + `","slug":"s","title":"t",` +
		`"content":"c","status":"published"}`
	req := createAuthenticatedRequest("POST", "/api/v1/"+statusTestTeamID+"/artifacts", body, "user-123")
	req = addURLParams(req, map[string]string{"team_id": statusTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleCreateArtifact(rr, req)

	assertStatusRejected(t, req, rr)
}

func TestUpdateArtifact_RejectsStatusOutsideTheArtifactSubset(t *testing.T) {
	srv := statusArtifactServer(t)

	// "expired" is a valid BlueprintStatus.
	url := "/api/v1/" + statusTestTeamID + "/artifacts/" + statusTestTeamID + "/test-slug"
	req := createAuthenticatedRequest("PUT", url, `{"status":"expired"}`, "user-123")
	req = addURLParams(req, map[string]string{
		"team_id": statusTestTeamID, "project_id": statusTestTeamID, "slug": "test-slug",
	})
	rr := httptest.NewRecorder()

	srv.handleUpdateArtifact(rr, req)

	assertStatusRejected(t, req, rr)
}

func TestCreateBlueprint_RejectsStatusOutsideTheBlueprintSubset(t *testing.T) {
	srv, _ := setupTestServerForBlueprint(t, nil)

	// "draft" is a valid ArtifactStatus/MemoryStatus/PromptStatus.
	body := `{"project_id":"` + statusTestTeamID + `","slug":"s","title":"t",` +
		`"content":"c","status":"draft"}`
	req := createBlueprintAuthenticatedRequest("POST", "/api/v1/"+statusTestTeamID+"/blueprints", body, "user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assertStatusRejected(t, req, rr)
}

func TestUpdateBlueprint_RejectsStatusOutsideTheBlueprintSubset(t *testing.T) {
	srv, _ := setupTestServerForBlueprint(t, nil)

	url := "/api/v1/" + statusTestTeamID + "/blueprints/" + statusTestTeamID + "/test-spec"
	req := createBlueprintAuthenticatedRequest("PUT", url, `{"status":"archived"}`, "user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assertStatusRejected(t, req, rr)
}

func statusMemoryRequest(method, url, body string) *http.Request {
	req := httptest.NewRequest(method, url, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "test-user-123"))
}

func TestCreateMemory_RejectsStatusOutsideTheMemorySubset(t *testing.T) {
	srv := createMemoryTestServer(newMockMemoryContainer(t))

	// "published" is a valid PromptStatus.
	req := statusMemoryRequest("POST", "/api/v1/"+statusTestTeamID+"/memories",
		`{"project_id":"`+statusTestTeamID+`","text":"t","status":"published"}`)
	req = addRouteParams(req, map[string]string{"team_id": statusTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleCreateMemory(rr, req)

	assertStatusRejected(t, req, rr)
}

func TestUpdateMemory_RejectsStatusOutsideTheMemorySubset(t *testing.T) {
	srv := createMemoryTestServer(newMockMemoryContainer(t))

	// "expired" is a valid BlueprintStatus.
	req := statusMemoryRequest("PUT", "/api/v1/"+statusTestTeamID+"/memories/memory-1",
		`{"status":"expired"}`)
	req = addRouteParams(req, map[string]string{"team_id": statusTestTeamID, "id": "memory-1"})
	rr := httptest.NewRecorder()

	srv.handleUpdateMemory(rr, req)

	assertStatusRejected(t, req, rr)
}
