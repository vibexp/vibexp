package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
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
			got, err := parseLabelsFilter(tc.raw)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	// Over-limit input is rejected, not truncated: a silently narrowed filter
	// returns rows that do not answer the question that was asked. This mirrors
	// the sibling `metadata` parameter on the same five list operations, which
	// 400s on its own limits rather than trimming them.
	t.Run("rejects more labels than the documented cap", func(t *testing.T) {
		labels := make([]string, MaxLabelsFilterValues+1)
		for i := range labels {
			labels[i] = "l" + strconv.Itoa(i)
		}
		got, err := parseLabelsFilter(strings.Join(labels, ","))

		require.Error(t, err)
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "at most 25 labels")
	})

	t.Run("accepts exactly the documented cap", func(t *testing.T) {
		labels := make([]string, MaxLabelsFilterValues)
		for i := range labels {
			labels[i] = "l" + strconv.Itoa(i)
		}
		got, err := parseLabelsFilter(strings.Join(labels, ","))

		require.NoError(t, err)
		assert.Len(t, got, MaxLabelsFilterValues)
	})

	// A label longer than the storable maximum cannot match any stored label, so
	// silently accepting it would answer "no results" to what is really a client
	// bug.
	t.Run("rejects a label longer than a stored label can be", func(t *testing.T) {
		got, err := parseLabelsFilter(strings.Repeat("x", models.MaxLabelLength+1))

		require.Error(t, err)
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "at most 50 characters")
	})
}

// The rejection has to reach the wire as a 400 on every list operation, not just
// exist in the parser -- the three resources build their filters through three
// different functions, so the error has to be threaded through each one.
func TestListMemories_RejectsAnOverLongLabelsFilter(t *testing.T) {
	container := newMockMemoryContainer(t)
	// No ListMemories expectation: reaching the service is the failure.

	srv := createMemoryTestServer(container)
	req := makeMemoryAuthenticatedRequest("GET",
		"/api/v1/"+memoriesTestTeamID+"/memories?labels="+strings.Repeat("x", models.MaxLabelLength+1),
		nil, memoriesTestUserID)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
}

// genLabels is the construction-site guarantee for the READ path: it must never
// hand a nil slice to a generated type, whatever the model holds.
//
// The requiredArrayResponseRegistry entries for Artifact/Blueprint/Memory do NOT
// cover this. That check marshals models.Artifact{} etc., which are the live
// types for the hand-marshaled WRITE bodies only; the read operations serialize
// artifactsgen.Artifact and friends, which cannot carry the LabelList shim. So
// the [] guarantee on a read comes solely from genLabels, and the per-resource
// wire assertions below are what stand behind it.
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

func TestGetArtifact_LabelsAlwaysSerializeAsAnArray(t *testing.T) {
	const (
		team    = "550e8400-e29b-41d4-a716-446655440000"
		project = "550e8400-e29b-41d4-a716-446655440000"
		slug    = "labelled-artifact"
	)
	url := "/api/v1/" + team + "/artifacts/" + project + "/" + slug

	sample := func(labels models.LabelList) *models.Artifact {
		now := time.Date(2026, 3, 1, 12, 30, 45, 0, time.UTC)
		return &models.Artifact{
			ID: "art-1", ProjectID: project, Slug: slug, UserID: "user-123",
			Title: "Labelled", Content: "body", Description: "d",
			Type: "general", Status: "active", Labels: labels,
			Metadata:  map[string]interface{}{"key": "value"},
			CreatedAt: now, UpdatedAt: now,
		}
	}

	get := func(t *testing.T, artifact *models.Artifact) (*http.Request, *httptest.ResponseRecorder) {
		t.Helper()
		svc := servicesmocks.NewMockArtifactServiceInterface(t)
		svc.On("GetArtifactByProjectIDAndSlugInTeam", "user-123", team, project, slug).
			Return(artifact, nil)

		srv := mountArtifactReadRoutes(
			New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler)))
		srv.container = &MockArtifactContainer{ArtifactServiceMock: svc}

		req := createAuthenticatedRequest("GET", url, "", "user-123")
		rr := httptest.NewRecorder()
		srv.router.ServeHTTP(rr, req)
		return req, rr
	}

	t.Run("no labels emits [] and never null", func(t *testing.T) {
		req, rr := get(t, sample(nil))

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.Contains(t, rr.Body.String(), `"labels":[]`)
		assert.NotContains(t, rr.Body.String(), `"labels":null`)
	})

	t.Run("labels round-trip to the wire", func(t *testing.T) {
		req, rr := get(t, sample(models.LabelList{"onboarding", "api"}))

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)

		var body map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
		assert.Equal(t, []any{"onboarding", "api"}, body["labels"])
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

// The service's over-limit rejection must surface as a 400, not the generic 500.
// This pins the handler MAPPING only -- that the limits are enforced at all is
// TestValidateLabels' job, and it has to be, because the `validate:` struct tags
// on these request models are inert (nothing calls validate.Struct on them) and
// the generated binder validates neither maxItems nor maxLength. Without the
// errors.Is arm the service error falls through to the generic 500 branch.
func TestCreateMemory_MapsInvalidLabelsTo400(t *testing.T) {
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
