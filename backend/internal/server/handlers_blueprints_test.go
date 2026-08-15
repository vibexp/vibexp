package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// The blueprints read path converted to generated strict-server types (#778,
// epic #122), following memories (#779) and artifacts (#776). These are the
// spec-validated assertions the conversion exists to buy: every response body
// is checked against backend/openapi.yaml rather than trusted, which is the
// class of drift that crashed the SPA three times (#105 / #121 / #132).

const (
	strictBpTeamID  = "550e8400-e29b-41d4-a716-446655440030"
	strictBpProject = "550e8400-e29b-41d4-a716-446655440031"
	strictBpUserID  = "user-123"
	strictBpSlug    = "test-blueprint"
)

func strictBlueprintServer(t *testing.T, svc services.BlueprintServiceInterface) *Server {
	t.Helper()
	return createListSpecTestServer(t, svc.(*servicesmocks.MockBlueprintServiceInterface))
}

// sampleStrictBlueprint mirrors what the repository returns: note `id` and
// `user_id` are plain strings in the schema (unlike project_id), so they are
// deliberately NOT UUIDs here.
func sampleStrictBlueprint() *models.Blueprint {
	now := time.Date(2026, 3, 1, 12, 30, 45, 0, time.UTC)
	return &models.Blueprint{
		ID:        "spec-1",
		ProjectID: strictBpProject,
		Slug:      strictBpSlug,
		UserID:    strictBpUserID,
		Title:     "Test Blueprint",
		Content:   "# rules",
		Path:      ".claude/test-blueprint.md",
		Type:      "claude-code",
		Status:    "active",
		Metadata:  map[string]interface{}{"key": "value"},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func strictBlueprintRequest(t *testing.T, srv *Server, path string) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	req := createBlueprintAuthenticatedRequest("GET", path, "", strictBpUserID)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	return req, w
}

func TestStrictGetBlueprint_ConformsToSpec(t *testing.T) {
	svc := servicesmocks.NewMockBlueprintServiceInterface(t)
	svc.On("GetBlueprintByProjectIDAndSlugInTeam",
		strictBpUserID, strictBpTeamID, strictBpProject, strictBpSlug).
		Return(sampleStrictBlueprint(), nil)

	srv := strictBlueprintServer(t, svc)
	req, w := strictBlueprintRequest(t, srv,
		"/api/v1/"+strictBpTeamID+"/blueprints/"+strictBpProject+"/"+strictBpSlug)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "spec-1", body["id"])
	assert.Equal(t, ".claude/test-blueprint.md", body["path"])
}

// The detail operation answers with BlueprintDetail (`allOf: [Blueprint,
// {raw_content}]`), which oapi-codegen emits as a SEPARATE struct with its own
// enum types — so raw_content has to be carried on this path and nowhere else.
func TestStrictGetBlueprint_CarriesRawContent(t *testing.T) {
	blueprint := sampleStrictBlueprint()
	blueprint.RawContent = "---\nfrontmatter: kept\n---\n# rules"

	svc := servicesmocks.NewMockBlueprintServiceInterface(t)
	svc.On("GetBlueprintByProjectIDAndSlugInTeam",
		strictBpUserID, strictBpTeamID, strictBpProject, strictBpSlug).
		Return(blueprint, nil)

	srv := strictBlueprintServer(t, svc)
	_, w := strictBlueprintRequest(t, srv,
		"/api/v1/"+strictBpTeamID+"/blueprints/"+strictBpProject+"/"+strictBpSlug)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, blueprint.RawContent, body["raw_content"])
}

// The optional keys that carried NO omitempty on models.Blueprint must still be
// present when empty, and the ones that DID carry it must still be absent.
// Every one of them is optional in the schema, so AssertConformsToSpec cannot
// tell the difference — this is the only thing standing between the conversion
// and a silent wire change (#779).
func TestStrictGetBlueprint_OptionalKeyPresenceIsUnchanged(t *testing.T) {
	blueprint := sampleStrictBlueprint()
	blueprint.Metadata = nil // HAS omitempty on the model: must stay absent
	blueprint.Description = ""

	svc := servicesmocks.NewMockBlueprintServiceInterface(t)
	svc.On("GetBlueprintByProjectIDAndSlugInTeam",
		strictBpUserID, strictBpTeamID, strictBpProject, strictBpSlug).
		Return(blueprint, nil)

	srv := strictBlueprintServer(t, svc)
	_, w := strictBlueprintRequest(t, srv,
		"/api/v1/"+strictBpTeamID+"/blueprints/"+strictBpProject+"/"+strictBpSlug)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	body := w.Body.String()
	assert.Contains(t, body, `"description":""`, "description carries no omitempty on models.Blueprint")
	assert.Contains(t, body, `"related":[]`, "an empty neighborhood serialized as [] before the conversion")
	assert.Contains(t, body, `"similar":[]`)
	// These DO carry omitempty on the model -- emitting them would ADD keys that
	// were never there. Note metadata differs from its artifacts counterpart,
	// which has no omitempty and must always be present.
	assert.NotContains(t, body, `"metadata"`)
	assert.NotContains(t, body, `"subtype"`)
	assert.NotContains(t, body, `"content_sha"`)
	assert.NotContains(t, body, `"source"`)
	assert.NotContains(t, body, `"raw_content"`)
	assert.NotContains(t, body, `"freshness"`)
}

// The required `blueprints` array must serialize as [] and never null.
func TestStrictListBlueprints_EmptyPageSerializesAnArray(t *testing.T) {
	svc := servicesmocks.NewMockBlueprintServiceInterface(t)
	svc.On("ListBlueprints", strictBpUserID, mock.Anything).
		Return(&models.BlueprintListResponse{
			Blueprints: nil, // the shape a repository returns for an empty page
			Page:       1, PerPage: 20, TotalCount: 0, TotalPages: 0,
		}, nil)

	srv := strictBlueprintServer(t, svc)
	req, w := strictBlueprintRequest(t, srv, "/api/v1/"+strictBpTeamID+"/blueprints")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	assert.Contains(t, w.Body.String(), `"blueprints":[]`, "a nil slice must marshal as [], not null")
}

func TestStrictListBlueprints_ConformsToSpec(t *testing.T) {
	svc := servicesmocks.NewMockBlueprintServiceInterface(t)
	svc.On("ListBlueprints", strictBpUserID, mock.Anything).
		Return(&models.BlueprintListResponse{
			Blueprints: models.JSONArray[models.Blueprint]{*sampleStrictBlueprint()},
			Page:       1, PerPage: 20, TotalCount: 1, TotalPages: 1,
		}, nil)

	srv := strictBlueprintServer(t, svc)
	req, w := strictBlueprintRequest(t, srv, "/api/v1/"+strictBpTeamID+"/blueprints")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.EqualValues(t, 1, body["total_count"])
	require.Len(t, body["blueprints"], 1)
}

// The by-project operation narrows the same service call by a PATH parameter.
func TestStrictListBlueprintsByProject_ConformsAndFiltersByPath(t *testing.T) {
	svc := servicesmocks.NewMockBlueprintServiceInterface(t)
	svc.On("ListBlueprints", strictBpUserID,
		mock.MatchedBy(func(f services.BlueprintFilters) bool {
			return f.ProjectID == strictBpProject && f.TeamID == strictBpTeamID
		}),
	).Return(&models.BlueprintListResponse{Page: 1, PerPage: 20}, nil)

	srv := strictBlueprintServer(t, svc)
	req, w := strictBlueprintRequest(t, srv,
		"/api/v1/"+strictBpTeamID+"/blueprints/"+strictBpProject)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	svc.AssertExpectations(t)
}

// Every filter must actually reach the service. mock.Anything on the filters
// argument makes this untestable, which is how a dropped filter ships: the
// endpoint answers 200 with the FULL list, which reads as a legitimate answer
// (#779). Note `subtype`, which the other three domains do not have.
func TestStrictListBlueprints_EveryFilterReachesTheService(t *testing.T) {
	svc := servicesmocks.NewMockBlueprintServiceInterface(t)
	svc.On("ListBlueprints", strictBpUserID,
		mock.MatchedBy(func(f services.BlueprintFilters) bool {
			return f.TeamID == strictBpTeamID &&
				f.ProjectID == strictBpProject &&
				f.Status == "active" &&
				f.Type == "claude-code" &&
				f.Subtype == "agent" &&
				f.Search == "rules" &&
				f.Freshness == services.FreshnessFilterStale &&
				f.SortBy == "created_at" &&
				f.SortOrder == "asc" &&
				f.Page == 2 && f.Limit == 5 &&
				len(f.MetadataFilter) == 1
		}),
	).Return(&models.BlueprintListResponse{Page: 2, PerPage: 5}, nil)

	srv := strictBlueprintServer(t, svc)
	_, w := strictBlueprintRequest(t, srv,
		"/api/v1/"+strictBpTeamID+"/blueprints"+
			"?project_id="+strictBpProject+"&status=active&type=claude-code&subtype=agent"+
			"&search=rules&freshness=stale&sort_by=created_at&sort_order=asc&page=2&limit=5"+
			"&metadata="+url.QueryEscape(`{"env":["prod"]}`))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	svc.AssertExpectations(t)
}

// An empty query value meant "no filter" to the chi parser this replaced, and
// still does — dropEmptyQueryValues strips them ahead of the binder (#779).
func TestStrictListBlueprints_EmptyQueryValuesAreIgnoredNotRejected(t *testing.T) {
	svc := servicesmocks.NewMockBlueprintServiceInterface(t)
	svc.On("ListBlueprints", strictBpUserID,
		mock.MatchedBy(func(f services.BlueprintFilters) bool {
			return f.Status == "" && f.ProjectID == "" && f.Freshness == "" &&
				f.Type == "" && f.Subtype == "" && f.Search == "" && f.Page == 1
		}),
	).Return(&models.BlueprintListResponse{Page: 1, PerPage: 20}, nil)

	srv := strictBlueprintServer(t, svc)
	_, w := strictBlueprintRequest(t, srv,
		"/api/v1/"+strictBpTeamID+"/blueprints?status=&project_id=&type=&subtype=&search=&page=&freshness=")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	svc.AssertExpectations(t)
}

// freshness is the one enum the handler still rejects itself: oapi-codegen binds
// enums without validating their values, and an ignored freshness filter returns
// the full list, which reads as a legitimate answer.
func TestStrictListBlueprints_RejectsUnknownFreshnessValue(t *testing.T) {
	// The service mock has no ListBlueprints expectation: reaching it is itself
	// the failure this test exists to catch.
	srv := strictBlueprintServer(t, servicesmocks.NewMockBlueprintServiceInterface(t))
	_, w := strictBlueprintRequest(t, srv, "/api/v1/"+strictBpTeamID+"/blueprints?freshness=stail")

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "freshness must be stale")
}

// The PATH project_id is a plain string in this spec, NOT format: uuid (unlike
// the artifacts equivalent), so the binder does not reject a malformed one and
// the handler's own check is what keeps the 400 this endpoint has always
// returned. Without it the value reaches the repository and becomes a 500.
func TestStrictBlueprints_RejectsNonUUIDPathProjectID(t *testing.T) {
	srv := strictBlueprintServer(t, servicesmocks.NewMockBlueprintServiceInterface(t))

	for _, path := range []string{
		"/api/v1/" + strictBpTeamID + "/blueprints/not-a-uuid",
		"/api/v1/" + strictBpTeamID + "/blueprints/not-a-uuid/some-slug",
	} {
		_, w := strictBlueprintRequest(t, srv, path)
		assert.Equal(t, http.StatusBadRequest, w.Code, path)
		assert.Contains(t, w.Body.String(), "Invalid project_id format", path)
	}
}

// A list failure must never be reported as a 404: only the detail operation
// documents that status, and the not-found arm matches a broad string fragment.
// The 500 body also keeps the per-operation wording the chi handler used.
func TestStrictListBlueprints_ServiceErrorIsNotA404(t *testing.T) {
	svc := servicesmocks.NewMockBlueprintServiceInterface(t)
	svc.On("ListBlueprints", strictBpUserID, mock.Anything).
		Return(nil, errors.New("project not found while listing"))

	srv := strictBlueprintServer(t, svc)
	_, w := strictBlueprintRequest(t, srv, "/api/v1/"+strictBpTeamID+"/blueprints")

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Failed to list blueprints")
}

// The detail read must still record a resource-access event through the new
// mount. recordResourceAccess reports nothing unless a handler sets the id, so
// this is unpinned unless asserted.
func TestStrictGetBlueprint_RecordsAnAccessEvent(t *testing.T) {
	svc := servicesmocks.NewMockBlueprintServiceInterface(t)
	svc.On("GetBlueprintByProjectIDAndSlugInTeam",
		strictBpUserID, strictBpTeamID, strictBpProject, strictBpSlug).
		Return(sampleStrictBlueprint(), nil)

	access := &spyResourceAccessService{}
	srv := strictBlueprintServer(t, svc)
	srv.container.(*MockBlueprintContainer).ResourceAccessServiceMock = access

	_, w := strictBlueprintRequest(t, srv,
		"/api/v1/"+strictBpTeamID+"/blueprints/"+strictBpProject+"/"+strictBpSlug)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Len(t, access.events, 1, "the detail read must record exactly one access event")
	assert.Equal(t, "spec-1", access.events[0].ResourceID)
	assert.Equal(t, resourceTypeBlueprint, access.events[0].ResourceType)
}
