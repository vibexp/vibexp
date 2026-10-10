package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// Resource status on the wire, for all four resource types (#912, opened to
// team custom statuses in #1266).
//
// A status is validated in ONE place, the service, against the team's status
// catalog: the type's system statuses plus the team's own. The handlers carry
// no copy of that check, so what they owe a client is exactly three things,
// and these tests pin each on the create path, the update path and the list
// filter of every resource:
//
//   - a custom status goes through to the service untouched, and the resource
//     that comes back carrying it is a body the spec accepts -- the four status
//     schemas are open strings, so this fails the moment one is closed again;
//   - the service's rejection reaches the client as a 400 whose detail is the
//     service's message, which lists the allowed values;
//   - an unknown list filter is that same 400, never an empty page.
//
// The service is mocked here; that it really accepts the team's statuses and
// rejects another team's is TestStatusValidator and the *_CustomStatus tests in
// internal/services.

const (
	statusTestTeamID = "550e8400-e29b-41d4-a716-446655440000"
	// statusTestCustom is a status no resource type has as a system value.
	statusTestCustom = "in-review"
)

// errStatusTestRejected is the service's answer for a status the team does not
// have: the sentinel wrapped with the allowed values, system statuses first.
var errStatusTestRejected = fmt.Errorf(
	"%w: must be one of: active, draft, archived, in-review", services.ErrInvalidStatus)

const statusTestRejectedDetail = "invalid status: must be one of: active, draft, archived, in-review"

// assertStatusRejected pins the shape every one of these rejections shares.
// wantCode is VALIDATION_FAILED on the chi write handlers and BAD_REQUEST on
// the strict-server list handlers -- the two codes those two families already
// answer for every other bad input (#800).
func assertStatusRejected(t *testing.T, req *http.Request, rr *httptest.ResponseRecorder, wantCode string) {
	t.Helper()

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, wantCode, body["code"])
	assert.Equal(t, statusTestRejectedDetail, body["detail"],
		"the rejection must carry the service's message: it is the only place a "+
			"client learns which statuses the team has")
}

// assertCustomStatusOnTheWire checks a successful write: the spec accepts the
// body, and the resource in it carries the custom status.
func assertCustomStatusOnTheWire(
	t *testing.T, req *http.Request, rr *httptest.ResponseRecorder, wantCode int, path ...string,
) {
	t.Helper()

	require.Equal(t, wantCode, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)

	var body any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	for _, key := range path {
		switch node := body.(type) {
		case map[string]any:
			body = node[key]
		case []any:
			require.NotEmpty(t, node)
			body = node[0]
		}
	}
	resource, ok := body.(map[string]any)
	require.True(t, ok, "no resource object at %v in %s", path, rr.Body.String())
	assert.Equal(t, statusTestCustom, resource["status"])
}

// ---------------------------------------------------------------------------
// Prompts
// ---------------------------------------------------------------------------

func statusPromptServer(t *testing.T) (*Server, *MockPromptContainer) {
	t.Helper()
	container := newMockPromptContainer(t)
	// The prompt routes go through the router, which resolves team membership
	// before any handler runs.
	container.teamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", statusTestTeamID).
		Return(true, nil).Maybe()
	return createTestServer(container), container
}

func statusPrompt() *models.Prompt {
	prompt := samplePrompt()
	prompt.Status = statusTestCustom
	return prompt
}

func createPromptWithStatus(srv *Server, status string) (*http.Request, *httptest.ResponseRecorder) {
	req := makeAuthenticatedRequest("POST", "/api/v1/"+statusTestTeamID+"/prompts",
		map[string]any{
			"name": "n", "slug": "s", "body": "b",
			"project_id": statusTestTeamID, "status": status,
		}, "user-123")
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)
	return req, rr
}

func updatePromptWithStatus(srv *Server, status string) (*http.Request, *httptest.ResponseRecorder) {
	req := makeAuthenticatedRequest("PUT", "/api/v1/"+statusTestTeamID+"/prompts/test-slug",
		map[string]any{"status": status}, "user-123")
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)
	return req, rr
}

func TestPromptStatus_Create(t *testing.T) {
	withStatus := func(want string) any {
		return mock.MatchedBy(func(r *models.CreatePromptRequest) bool { return r.Status == want })
	}

	t.Run("custom status round-trips", func(t *testing.T) {
		srv, container := statusPromptServer(t)
		container.promptService.On("CreatePrompt", "user-123", statusTestTeamID, withStatus(statusTestCustom)).
			Return(statusPrompt(), nil).Once()

		req, rr := createPromptWithStatus(srv, statusTestCustom)

		assertCustomStatusOnTheWire(t, req, rr, http.StatusCreated)
	})

	t.Run("the service's rejection is a 400 listing the allowed values", func(t *testing.T) {
		srv, container := statusPromptServer(t)
		container.promptService.On("CreatePrompt", "user-123", statusTestTeamID, withStatus("retired")).
			Return(nil, errStatusTestRejected).Once()

		req, rr := createPromptWithStatus(srv, "retired")

		assertStatusRejected(t, req, rr, "VALIDATION_FAILED")
	})
}

func TestPromptStatus_Update(t *testing.T) {
	withStatus := func(want string) any {
		return mock.MatchedBy(func(r *models.UpdatePromptRequest) bool {
			return r.Status != nil && *r.Status == want
		})
	}

	t.Run("custom status round-trips", func(t *testing.T) {
		srv, container := statusPromptServer(t)
		container.promptService.
			On("UpdatePromptBySlug", "user-123", statusTestTeamID, "test-slug", withStatus(statusTestCustom)).
			Return(statusPrompt(), nil).Once()

		req, rr := updatePromptWithStatus(srv, statusTestCustom)

		assertCustomStatusOnTheWire(t, req, rr, http.StatusOK)
	})

	t.Run("the service's rejection is a 400 listing the allowed values", func(t *testing.T) {
		srv, container := statusPromptServer(t)
		container.promptService.
			On("UpdatePromptBySlug", "user-123", statusTestTeamID, "test-slug", withStatus("retired")).
			Return(nil, errStatusTestRejected).Once()

		req, rr := updatePromptWithStatus(srv, "retired")

		assertStatusRejected(t, req, rr, "VALIDATION_FAILED")
	})
}

func TestPromptStatus_ListFilter(t *testing.T) {
	withStatus := func(want string) any {
		return mock.MatchedBy(func(f services.PromptFilters) bool {
			return f.TeamID == statusTestTeamID && f.Status == want
		})
	}
	list := func(srv *Server, status string) (*http.Request, *httptest.ResponseRecorder) {
		req := makeAuthenticatedRequest("GET",
			"/api/v1/"+statusTestTeamID+"/prompts?status="+status, nil, "user-123")
		rr := httptest.NewRecorder()
		srv.router.ServeHTTP(rr, req)
		return req, rr
	}

	t.Run("custom status filter returns matching items", func(t *testing.T) {
		srv, container := statusPromptServer(t)
		container.promptService.On("ListPrompts", "user-123", withStatus(statusTestCustom)).
			Return(&models.PromptListResponse{
				Prompts:    models.JSONArray[models.Prompt]{*statusPrompt()},
				TotalCount: 1, Page: 1, PerPage: 20, TotalPages: 1,
			}, nil).Once()

		req, rr := list(srv, statusTestCustom)

		assertCustomStatusOnTheWire(t, req, rr, http.StatusOK, "data", "prompts", "0")
	})

	t.Run("unknown status filter is a 400, not an empty page", func(t *testing.T) {
		srv, container := statusPromptServer(t)
		container.promptService.On("ListPrompts", "user-123", withStatus("retired")).
			Return(nil, errStatusTestRejected).Once()

		req, rr := list(srv, "retired")

		assertStatusRejected(t, req, rr, "BAD_REQUEST")
	})
}

// ---------------------------------------------------------------------------
// Artifacts
// ---------------------------------------------------------------------------

func statusArtifactServer(t *testing.T) (*Server, *servicesmocks.MockArtifactServiceInterface) {
	t.Helper()
	svc := servicesmocks.NewMockArtifactServiceInterface(t)
	srv := mountArtifactReadRoutes(New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler)))
	srv.container = &MockArtifactContainer{ArtifactServiceMock: svc}
	return srv, svc
}

func statusArtifact() *models.Artifact {
	artifact := sampleStrictArtifact()
	artifact.TeamID = statusTestTeamID
	artifact.Status = statusTestCustom
	return artifact
}

func createArtifactWithStatus(srv *Server, status string) (*http.Request, *httptest.ResponseRecorder) {
	body := `{"project_id":"` + statusTestTeamID + `","slug":"s","title":"t",` +
		`"content":"c","status":"` + status + `"}`
	req := createAuthenticatedRequest("POST", "/api/v1/"+statusTestTeamID+"/artifacts", body, "user-123")
	req = addURLParams(req, map[string]string{"team_id": statusTestTeamID})
	rr := httptest.NewRecorder()
	srv.handleCreateArtifact(rr, req)
	return req, rr
}

func updateArtifactWithStatus(srv *Server, status string) (*http.Request, *httptest.ResponseRecorder) {
	url := "/api/v1/" + statusTestTeamID + "/artifacts/" + statusTestTeamID + "/test-slug"
	req := createAuthenticatedRequest("PUT", url, `{"status":"`+status+`"}`, "user-123")
	req = addURLParams(req, map[string]string{
		"team_id": statusTestTeamID, "project_id": statusTestTeamID, "slug": "test-slug",
	})
	rr := httptest.NewRecorder()
	srv.handleUpdateArtifact(rr, req)
	return req, rr
}

func TestArtifactStatus_Create(t *testing.T) {
	withStatus := func(want string) any {
		return mock.MatchedBy(func(r *models.CreateArtifactRequest) bool { return r.Status == want })
	}

	t.Run("custom status round-trips", func(t *testing.T) {
		srv, svc := statusArtifactServer(t)
		svc.On("CreateArtifact", "user-123", statusTestTeamID, withStatus(statusTestCustom)).
			Return(statusArtifact(), nil).Once()

		req, rr := createArtifactWithStatus(srv, statusTestCustom)

		assertCustomStatusOnTheWire(t, req, rr, http.StatusCreated)
	})

	t.Run("the service's rejection is a 400 listing the allowed values", func(t *testing.T) {
		srv, svc := statusArtifactServer(t)
		svc.On("CreateArtifact", "user-123", statusTestTeamID, withStatus("retired")).
			Return(nil, errStatusTestRejected).Once()

		req, rr := createArtifactWithStatus(srv, "retired")

		assertStatusRejected(t, req, rr, "VALIDATION_FAILED")
	})
}

func TestArtifactStatus_Update(t *testing.T) {
	withStatus := func(want string) any {
		return mock.MatchedBy(func(r *models.UpdateArtifactRequest) bool {
			return r.Status != nil && *r.Status == want
		})
	}

	t.Run("custom status round-trips", func(t *testing.T) {
		srv, svc := statusArtifactServer(t)
		svc.On("UpdateArtifactByProjectIDAndSlugInTeam",
			"user-123", statusTestTeamID, statusTestTeamID, "test-slug", withStatus(statusTestCustom)).
			Return(statusArtifact(), nil).Once()

		req, rr := updateArtifactWithStatus(srv, statusTestCustom)

		assertCustomStatusOnTheWire(t, req, rr, http.StatusOK)
	})

	t.Run("the service's rejection is a 400 listing the allowed values", func(t *testing.T) {
		srv, svc := statusArtifactServer(t)
		svc.On("UpdateArtifactByProjectIDAndSlugInTeam",
			"user-123", statusTestTeamID, statusTestTeamID, "test-slug", withStatus("retired")).
			Return(nil, errStatusTestRejected).Once()

		req, rr := updateArtifactWithStatus(srv, "retired")

		assertStatusRejected(t, req, rr, "VALIDATION_FAILED")
	})
}

// Both artifact list operations -- the team-wide one and the per-project one --
// share one service call, and each must map its rejection.
func TestArtifactStatus_ListFilter(t *testing.T) {
	withStatus := func(want string) any {
		return mock.MatchedBy(func(f services.ArtifactFilters) bool {
			return f.TeamID == strictArtTeamID && f.Status == want
		})
	}
	paths := map[string]string{
		"team":    "/api/v1/" + strictArtTeamID + "/artifacts",
		"project": "/api/v1/" + strictArtTeamID + "/artifacts/" + strictArtProject,
	}

	for name, path := range paths {
		t.Run(name+": custom status filter returns matching items", func(t *testing.T) {
			svc := servicesmocks.NewMockArtifactServiceInterface(t)
			svc.On("ListArtifacts", strictArtUserID, withStatus(statusTestCustom)).
				Return(&models.ArtifactListResponse{
					Artifacts:  []models.Artifact{*statusArtifact()},
					TotalCount: 1, Page: 1, PerPage: 20, TotalPages: 1,
				}, nil).Once()

			req, rr := strictArtifactRequest(t, strictArtifactServer(t, svc), path+"?status="+statusTestCustom)

			assertCustomStatusOnTheWire(t, req, rr, http.StatusOK, "artifacts", "0")
		})

		t.Run(name+": unknown status filter is a 400, not an empty page", func(t *testing.T) {
			svc := servicesmocks.NewMockArtifactServiceInterface(t)
			svc.On("ListArtifacts", strictArtUserID, withStatus("retired")).
				Return(nil, errStatusTestRejected).Once()

			req, rr := strictArtifactRequest(t, strictArtifactServer(t, svc), path+"?status=retired")

			assertStatusRejected(t, req, rr, "BAD_REQUEST")
		})
	}
}

// ---------------------------------------------------------------------------
// Blueprints
// ---------------------------------------------------------------------------

func statusBlueprint() *models.Blueprint {
	blueprint := sampleStrictBlueprint()
	blueprint.Status = statusTestCustom
	return blueprint
}

func TestBlueprintStatus_Create(t *testing.T) {
	withStatus := func(want string) any {
		return mock.MatchedBy(func(r *models.CreateBlueprintRequest) bool { return r.Status == want })
	}
	create := func(srv *Server, status string) (*http.Request, *httptest.ResponseRecorder) {
		body := `{"project_id":"` + statusTestTeamID + `","slug":"s","title":"t",` +
			`"content":"c","status":"` + status + `"}`
		req := createBlueprintAuthenticatedRequest("POST", "/api/v1/"+statusTestTeamID+"/blueprints", body, "user-123")
		rr := httptest.NewRecorder()
		srv.router.ServeHTTP(rr, req)
		return req, rr
	}

	t.Run("custom status round-trips", func(t *testing.T) {
		srv, svc := setupTestServerForBlueprint(t, nil)
		svc.On("CreateBlueprint", "user-123", statusTestTeamID, withStatus(statusTestCustom)).
			Return(statusBlueprint(), nil).Once()

		req, rr := create(srv, statusTestCustom)

		assertCustomStatusOnTheWire(t, req, rr, http.StatusCreated)
	})

	t.Run("the service's rejection is a 400 listing the allowed values", func(t *testing.T) {
		srv, svc := setupTestServerForBlueprint(t, nil)
		svc.On("CreateBlueprint", "user-123", statusTestTeamID, withStatus("retired")).
			Return(nil, errStatusTestRejected).Once()

		req, rr := create(srv, "retired")

		assertStatusRejected(t, req, rr, "VALIDATION_FAILED")
	})
}

func TestBlueprintStatus_Update(t *testing.T) {
	withStatus := func(want string) any {
		return mock.MatchedBy(func(r *models.UpdateBlueprintRequest) bool {
			return r.Status != nil && *r.Status == want
		})
	}
	update := func(srv *Server, status string) (*http.Request, *httptest.ResponseRecorder) {
		url := "/api/v1/" + statusTestTeamID + "/blueprints/" + statusTestTeamID + "/test-spec"
		req := createBlueprintAuthenticatedRequest("PUT", url, `{"status":"`+status+`"}`, "user-123")
		rr := httptest.NewRecorder()
		srv.router.ServeHTTP(rr, req)
		return req, rr
	}

	t.Run("custom status round-trips", func(t *testing.T) {
		srv, svc := setupTestServerForBlueprint(t, nil)
		svc.On("UpdateBlueprintByProjectIDAndSlugInTeam",
			"user-123", statusTestTeamID, statusTestTeamID, "test-spec", withStatus(statusTestCustom)).
			Return(statusBlueprint(), nil).Once()

		req, rr := update(srv, statusTestCustom)

		assertCustomStatusOnTheWire(t, req, rr, http.StatusOK)
	})

	t.Run("the service's rejection is a 400 listing the allowed values", func(t *testing.T) {
		srv, svc := setupTestServerForBlueprint(t, nil)
		svc.On("UpdateBlueprintByProjectIDAndSlugInTeam",
			"user-123", statusTestTeamID, statusTestTeamID, "test-spec", withStatus("retired")).
			Return(nil, errStatusTestRejected).Once()

		req, rr := update(srv, "retired")

		assertStatusRejected(t, req, rr, "VALIDATION_FAILED")
	})
}

// Both blueprint list operations share one service call, and each must map its
// rejection.
func TestBlueprintStatus_ListFilter(t *testing.T) {
	withStatus := func(want string) any {
		return mock.MatchedBy(func(f services.BlueprintFilters) bool {
			return f.TeamID == strictBpTeamID && f.Status == want
		})
	}
	paths := map[string]string{
		"team":    "/api/v1/" + strictBpTeamID + "/blueprints",
		"project": "/api/v1/" + strictBpTeamID + "/blueprints/" + strictBpProject,
	}

	for name, path := range paths {
		t.Run(name+": custom status filter returns matching items", func(t *testing.T) {
			svc := servicesmocks.NewMockBlueprintServiceInterface(t)
			svc.On("ListBlueprints", strictBpUserID, withStatus(statusTestCustom)).
				Return(&models.BlueprintListResponse{
					Blueprints: []models.Blueprint{*statusBlueprint()},
					TotalCount: 1, Page: 1, PerPage: 20, TotalPages: 1,
				}, nil).Once()

			req, rr := strictBlueprintRequest(t, strictBlueprintServer(t, svc), path+"?status="+statusTestCustom)

			assertCustomStatusOnTheWire(t, req, rr, http.StatusOK, "blueprints", "0")
		})

		t.Run(name+": unknown status filter is a 400, not an empty page", func(t *testing.T) {
			svc := servicesmocks.NewMockBlueprintServiceInterface(t)
			svc.On("ListBlueprints", strictBpUserID, withStatus("retired")).
				Return(nil, errStatusTestRejected).Once()

			req, rr := strictBlueprintRequest(t, strictBlueprintServer(t, svc), path+"?status=retired")

			assertStatusRejected(t, req, rr, "BAD_REQUEST")
		})
	}
}

// ---------------------------------------------------------------------------
// Memories
// ---------------------------------------------------------------------------

func statusMemoryRequest(method, url, body string) *http.Request {
	req := httptest.NewRequest(method, url, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, memoriesTestUserID))
}

func statusMemory() *models.Memory {
	memory := sampleStrictMemory()
	memory.Status = statusTestCustom
	return memory
}

func TestMemoryStatus_Create(t *testing.T) {
	withStatus := func(want string) any {
		return mock.MatchedBy(func(r *models.CreateMemoryRequest) bool {
			return r.Status != nil && *r.Status == want
		})
	}
	create := func(
		t *testing.T, container *MockMemoryContainer, status string,
	) (*http.Request, *httptest.ResponseRecorder) {
		t.Helper()
		container.projectRepository.On("GetByID", mock.Anything, memoriesTestUserID, memoriesTestProjectID).
			Return(&models.Project{ID: memoriesTestProjectID, TeamID: memoriesTestTeamID}, nil).Once()
		srv := createMemoryTestServer(container)
		req := statusMemoryRequest("POST", "/api/v1/"+memoriesTestTeamID+"/memories",
			`{"project_id":"`+memoriesTestProjectID+`","text":"t","status":"`+status+`"}`)
		req = addRouteParams(req, map[string]string{"team_id": memoriesTestTeamID})
		rr := httptest.NewRecorder()
		srv.handleCreateMemory(rr, req)
		return req, rr
	}

	t.Run("custom status round-trips", func(t *testing.T) {
		container := newMockMemoryContainer(t)
		container.memoryService.On("CreateMemory", memoriesTestUserID, memoriesTestTeamID, withStatus(statusTestCustom)).
			Return(statusMemory(), nil).Once()

		req, rr := create(t, container, statusTestCustom)

		assertCustomStatusOnTheWire(t, req, rr, http.StatusCreated)
	})

	t.Run("the service's rejection is a 400 listing the allowed values", func(t *testing.T) {
		container := newMockMemoryContainer(t)
		container.memoryService.On("CreateMemory", memoriesTestUserID, memoriesTestTeamID, withStatus("retired")).
			Return(nil, errStatusTestRejected).Once()

		req, rr := create(t, container, "retired")

		assertStatusRejected(t, req, rr, "VALIDATION_FAILED")
	})
}

func TestMemoryStatus_Update(t *testing.T) {
	withStatus := func(want string) any {
		return mock.MatchedBy(func(r *models.UpdateMemoryRequest) bool {
			return r.Status != nil && *r.Status == want
		})
	}
	update := func(container *MockMemoryContainer, status string) (*http.Request, *httptest.ResponseRecorder) {
		srv := createMemoryTestServer(container)
		req := statusMemoryRequest("PUT", "/api/v1/"+memoriesTestTeamID+"/memories/"+memoriesTestMemoryID,
			`{"status":"`+status+`"}`)
		req = addRouteParams(req, map[string]string{"team_id": memoriesTestTeamID, "id": memoriesTestMemoryID})
		rr := httptest.NewRecorder()
		srv.handleUpdateMemory(rr, req)
		return req, rr
	}

	t.Run("custom status round-trips", func(t *testing.T) {
		container := newMockMemoryContainer(t)
		container.memoryService.
			On("UpdateMemory", memoriesTestUserID, memoriesTestTeamID, memoriesTestMemoryID, withStatus(statusTestCustom)).
			Return(statusMemory(), nil).Once()

		req, rr := update(container, statusTestCustom)

		assertCustomStatusOnTheWire(t, req, rr, http.StatusOK)
	})

	t.Run("the service's rejection is a 400 listing the allowed values", func(t *testing.T) {
		container := newMockMemoryContainer(t)
		container.memoryService.
			On("UpdateMemory", memoriesTestUserID, memoriesTestTeamID, memoriesTestMemoryID, withStatus("retired")).
			Return(nil, errStatusTestRejected).Once()

		req, rr := update(container, "retired")

		assertStatusRejected(t, req, rr, "VALIDATION_FAILED")
	})
}

func TestMemoryStatus_ListFilter(t *testing.T) {
	withStatus := func(want string) any {
		return mock.MatchedBy(func(f services.MemoryFilters) bool {
			return f.TeamID == memoriesTestTeamID && f.Status != nil && *f.Status == want
		})
	}
	list := func(container *MockMemoryContainer, status string) (*http.Request, *httptest.ResponseRecorder) {
		srv := createMemoryTestServer(container)
		req := makeMemoryAuthenticatedRequest("GET",
			"/api/v1/"+memoriesTestTeamID+"/memories?status="+status, nil, memoriesTestUserID)
		rr := httptest.NewRecorder()
		srv.router.ServeHTTP(rr, req)
		return req, rr
	}

	t.Run("custom status filter returns matching items", func(t *testing.T) {
		container := newMockMemoryContainer(t)
		container.memoryService.On("ListMemories", memoriesTestUserID, withStatus(statusTestCustom)).
			Return(&models.MemoryListResponse{
				Memories:   []models.Memory{*statusMemory()},
				TotalCount: 1, Page: 1, PerPage: 20, TotalPages: 1,
			}, nil).Once()

		req, rr := list(container, statusTestCustom)

		assertCustomStatusOnTheWire(t, req, rr, http.StatusOK, "memories", "0")
	})

	t.Run("unknown status filter is a 400, not an empty page", func(t *testing.T) {
		container := newMockMemoryContainer(t)
		container.memoryService.On("ListMemories", memoriesTestUserID, withStatus("retired")).
			Return(nil, errStatusTestRejected).Once()

		req, rr := list(container, "retired")

		assertStatusRejected(t, req, rr, "BAD_REQUEST")
	})
}
