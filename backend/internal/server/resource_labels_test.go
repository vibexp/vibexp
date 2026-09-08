package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// The shared `labels` taxonomy across artifacts, blueprints and memories
// (issue #910, epic #899). `labels` is a REQUIRED array in all three response
// schemas, but the generated strict-server types serving the read path cannot
// use the models.JSONArray/LabelList shim -- the [] guarantee has to come from
// the converter. AssertConformsToSpec does NOT catch a required array
// serialized as null, so the wire form is asserted explicitly here; that second
// assertion is the one that actually bites.

func TestParseLabelsFilter(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty means no filter", raw: "", want: nil},
		{name: "single label", raw: "onboarding", want: []string{"onboarding"}},
		{name: "comma separated", raw: "onboarding,api", want: []string{"onboarding", "api"}},
		{name: "trims whitespace around entries", raw: " onboarding , api ", want: []string{"onboarding", "api"}},
		{name: "drops empty entries", raw: "onboarding,,api", want: []string{"onboarding", "api"}},
		{name: "drops a trailing separator", raw: "onboarding,", want: []string{"onboarding"}},
		{name: "all separators means no filter", raw: ",,,", want: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseLabelsFilter(tc.raw))
		})
	}
}

// genLabels is the construction-site guarantee behind the three
// adHocRequiredArrayAllowlist entries: it must never hand a nil slice to a
// generated type, whatever the model holds.
func TestGenLabelsNeverReturnsNil(t *testing.T) {
	assert.NotNil(t, genLabels(nil))
	assert.Empty(t, genLabels(nil))
	assert.Equal(t, []string{"api"}, genLabels(models.LabelList{"api"}))
}

func TestGetMemory_LabelsAlwaysSerializeAsAnArray(t *testing.T) {
	t.Run("no labels emits [] and never null", func(t *testing.T) {
		container := newMockMemoryContainer(t)
		memory := sampleStrictMemory()
		memory.Labels = nil
		container.memoryService.On("GetMemory", memoriesTestUserID, memoriesTestTeamID, memoriesTestMemoryID).
			Return(memory, nil)

		srv := createMemoryTestServer(container)
		req, w := getMemoryRequest(t, srv, memoriesTestMemoryID)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		specconformance.AssertConformsToSpec(t, req, w)
		assert.Contains(t, w.Body.String(), `"labels":[]`)
		assert.NotContains(t, w.Body.String(), `"labels":null`)
	})

	t.Run("labels round-trip to the wire", func(t *testing.T) {
		container := newMockMemoryContainer(t)
		memory := sampleStrictMemory()
		memory.Labels = models.LabelList{"onboarding", "api"}
		container.memoryService.On("GetMemory", memoriesTestUserID, memoriesTestTeamID, memoriesTestMemoryID).
			Return(memory, nil)

		srv := createMemoryTestServer(container)
		req, w := getMemoryRequest(t, srv, memoriesTestMemoryID)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		specconformance.AssertConformsToSpec(t, req, w)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, []any{"onboarding", "api"}, body["labels"])
	})
}

func TestGetBlueprint_LabelsAlwaysSerializeAsAnArray(t *testing.T) {
	t.Run("no labels emits [] and never null", func(t *testing.T) {
		svc := servicesmocks.NewMockBlueprintServiceInterface(t)
		blueprint := sampleStrictBlueprint()
		blueprint.Labels = nil
		svc.On("GetBlueprintByProjectIDAndSlugInTeam",
			strictBpUserID, strictBpTeamID, strictBpProject, strictBpSlug).
			Return(blueprint, nil)

		srv := strictBlueprintServer(t, svc)
		req, w := strictBlueprintRequest(t, srv,
			"/api/v1/"+strictBpTeamID+"/blueprints/"+strictBpProject+"/"+strictBpSlug)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		specconformance.AssertConformsToSpec(t, req, w)
		assert.Contains(t, w.Body.String(), `"labels":[]`)
		assert.NotContains(t, w.Body.String(), `"labels":null`)
	})

	// The detail operation answers with BlueprintDetail (`allOf: [Blueprint,
	// {raw_content}]`), which oapi-codegen emits as a SEPARATE struct -- so the
	// field has to be carried across from the base converter, not just set once.
	t.Run("labels reach the detail representation", func(t *testing.T) {
		svc := servicesmocks.NewMockBlueprintServiceInterface(t)
		blueprint := sampleStrictBlueprint()
		blueprint.Labels = models.LabelList{"onboarding"}
		svc.On("GetBlueprintByProjectIDAndSlugInTeam",
			strictBpUserID, strictBpTeamID, strictBpProject, strictBpSlug).
			Return(blueprint, nil)

		srv := strictBlueprintServer(t, svc)
		req, w := strictBlueprintRequest(t, srv,
			"/api/v1/"+strictBpTeamID+"/blueprints/"+strictBpProject+"/"+strictBpSlug)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		specconformance.AssertConformsToSpec(t, req, w)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, []any{"onboarding"}, body["labels"])
	})
}

func TestListMemories_LabelsFilterReachesTheService(t *testing.T) {
	container := newMockMemoryContainer(t)
	container.memoryService.On("ListMemories", memoriesTestUserID,
		mock.MatchedBy(func(f services.MemoryFilters) bool {
			return len(f.Labels) == 2 && f.Labels[0] == "onboarding" && f.Labels[1] == "api"
		}),
	).Return(&models.MemoryListResponse{Page: 1, PerPage: 20}, nil)

	srv := createMemoryTestServer(container)
	req := makeMemoryAuthenticatedRequest("GET",
		"/api/v1/"+memoriesTestTeamID+"/memories?labels=onboarding,api", nil, memoriesTestUserID)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	container.memoryService.AssertExpectations(t)
}

// An over-limit label list must be a 400, not a 500 and not a silently
// truncated write. The `validate:` struct tags on these request models are
// inert -- nothing calls validate.Struct on them -- and the generated binder
// validates neither maxItems nor maxLength, so the service-side check and this
// handler mapping are the ONLY enforcement. Without the errors.Is arm the
// service error falls through to the generic 500 branch.
func TestCreateMemory_RejectsOverLimitLabels(t *testing.T) {
	container := newMockMemoryContainer(t)
	container.projectRepository.On("GetByID", mock.Anything, memoriesTestUserID, testHandlerProjectID).
		Return(&models.Project{ID: testHandlerProjectID, UserID: memoriesTestUserID, TeamID: memoriesTestTeamID}, nil)
	container.memoryService.On("CreateMemory", memoriesTestUserID, memoriesTestTeamID, mock.Anything).
		Return(nil, services.ErrInvalidLabels)

	srv := createMemoryTestServer(container)
	req := makeMemoryAuthenticatedRequest("POST",
		"/api/v1/"+memoriesTestTeamID+"/memories",
		map[string]any{"project_id": testHandlerProjectID, "text": "hi", "labels": []string{"a"}},
		memoriesTestUserID)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "VALIDATION_FAILED")
	assert.Contains(t, w.Body.String(), "invalid labels")
}
