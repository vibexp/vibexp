package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// Issue #929: the four resource detail responses carry the owning project's
// summary, so a client renders a project label without fetching the project list
// and matching the id — a lookup that silently found nothing past the list's
// 100-project cap.
//
// Two things have to hold and NEITHER is visible to AssertConformsToSpec, which
// cannot tell an absent key from a null one: the summary must be carried when
// the repository resolved it, and the key must be PRESENT and `null` when it did
// not (`project` is required+nullable in the spec, which is the only shape
// oapi-codegen emits without `omitempty`). So each case asserts the wire form
// directly, the same reasoning as the labels assertions in resource_labels_test.go.

const (
	projectSummaryTestName = "Platform"
	projectSummaryTestSlug = "platform"
)

// assertProjectSummaryIsNull fails unless the body carries `"project": null` —
// key present, value null. `body["project"]` alone cannot distinguish the two.
func assertProjectSummaryIsNull(t *testing.T, raw []byte) {
	t.Helper()
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &body))
	value, ok := body["project"]
	require.True(t, ok, "project key must be present, not omitted: %s", raw)
	assert.JSONEq(t, "null", string(value))
}

// assertProjectSummary fails unless the body carries the full summary object.
func assertProjectSummary(t *testing.T, raw []byte, wantID string) {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	project, ok := body["project"].(map[string]any)
	require.True(t, ok, "project must be an object: %s", raw)
	assert.Equal(t, wantID, project["id"])
	assert.Equal(t, projectSummaryTestName, project["name"])
	assert.Equal(t, projectSummaryTestSlug, project["slug"])
}

func TestStrictGetPrompt_CarriesTheProjectSummary(t *testing.T) {
	prompt := samplePrompt()
	prompt.Project = &models.ProjectSummary{
		ID: strictPrProject, Name: projectSummaryTestName, Slug: projectSummaryTestSlug,
	}

	srv, container := strictPromptServer(t)
	container.promptService.On("GetPromptBySlug", strictPrUserID, strictPrTeamID, strictPrSlug).
		Return(prompt, nil)

	req, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts/"+strictPrSlug)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	assertProjectSummary(t, w.Body.Bytes(), strictPrProject)
}

func TestStrictGetPrompt_AbsentProjectStaysNullNotOmitted(t *testing.T) {
	srv, container := strictPromptServer(t)
	container.promptService.On("GetPromptBySlug", strictPrUserID, strictPrTeamID, strictPrSlug).
		Return(samplePrompt(), nil)

	req, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts/"+strictPrSlug)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	assertProjectSummaryIsNull(t, w.Body.Bytes())
}

func TestStrictGetArtifact_CarriesTheProjectSummary(t *testing.T) {
	artifact := sampleStrictArtifact()
	artifact.Project = &models.ProjectSummary{
		ID: strictArtProject, Name: projectSummaryTestName, Slug: projectSummaryTestSlug,
	}

	svc := servicesmocks.NewMockArtifactServiceInterface(t)
	svc.On("GetArtifactByProjectIDAndSlugInTeam",
		strictArtUserID, strictArtTeamID, strictArtProject, strictArtSlug).
		Return(artifact, nil)

	srv := strictArtifactServer(t, svc)
	req, w := strictArtifactRequest(t, srv,
		"/api/v1/"+strictArtTeamID+"/artifacts/"+strictArtProject+"/"+strictArtSlug)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	assertProjectSummary(t, w.Body.Bytes(), strictArtProject)
}

func TestStrictGetArtifact_AbsentProjectStaysNullNotOmitted(t *testing.T) {
	svc := servicesmocks.NewMockArtifactServiceInterface(t)
	svc.On("GetArtifactByProjectIDAndSlugInTeam",
		strictArtUserID, strictArtTeamID, strictArtProject, strictArtSlug).
		Return(sampleStrictArtifact(), nil)

	srv := strictArtifactServer(t, svc)
	req, w := strictArtifactRequest(t, srv,
		"/api/v1/"+strictArtTeamID+"/artifacts/"+strictArtProject+"/"+strictArtSlug)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	assertProjectSummaryIsNull(t, w.Body.Bytes())
}

// Blueprints answer with BlueprintDetail (`allOf: [Blueprint, {raw_content}]`),
// which oapi-codegen emits as a SEPARATE struct whose fields are copied across
// one by one — so the summary can be dropped on this path alone.
func TestStrictGetBlueprint_CarriesTheProjectSummary(t *testing.T) {
	blueprint := sampleStrictBlueprint()
	blueprint.Project = &models.ProjectSummary{
		ID: strictBpProject, Name: projectSummaryTestName, Slug: projectSummaryTestSlug,
	}

	svc := servicesmocks.NewMockBlueprintServiceInterface(t)
	svc.On("GetBlueprintByProjectIDAndSlugInTeam",
		strictBpUserID, strictBpTeamID, strictBpProject, strictBpSlug).
		Return(blueprint, nil)

	srv := strictBlueprintServer(t, svc)
	req, w := strictBlueprintRequest(t, srv,
		"/api/v1/"+strictBpTeamID+"/blueprints/"+strictBpProject+"/"+strictBpSlug)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	assertProjectSummary(t, w.Body.Bytes(), strictBpProject)
}

func TestStrictGetBlueprint_AbsentProjectStaysNullNotOmitted(t *testing.T) {
	svc := servicesmocks.NewMockBlueprintServiceInterface(t)
	svc.On("GetBlueprintByProjectIDAndSlugInTeam",
		strictBpUserID, strictBpTeamID, strictBpProject, strictBpSlug).
		Return(sampleStrictBlueprint(), nil)

	srv := strictBlueprintServer(t, svc)
	req, w := strictBlueprintRequest(t, srv,
		"/api/v1/"+strictBpTeamID+"/blueprints/"+strictBpProject+"/"+strictBpSlug)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	assertProjectSummaryIsNull(t, w.Body.Bytes())
}

func TestGetMemory_CarriesTheProjectSummary(t *testing.T) {
	memory := sampleStrictMemory()
	memory.Project = &models.ProjectSummary{
		ID: memoriesTestProjectID, Name: projectSummaryTestName, Slug: projectSummaryTestSlug,
	}

	container := newMockMemoryContainer(t)
	container.memoryService.On("GetMemory", memoriesTestUserID, memoriesTestTeamID, memoriesTestMemoryID).
		Return(memory, nil)

	srv := createMemoryTestServer(container)
	req, w := getMemoryRequest(t, srv, memoriesTestMemoryID)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	assertProjectSummary(t, w.Body.Bytes(), memoriesTestProjectID)
}

func TestGetMemory_AbsentProjectStaysNullNotOmitted(t *testing.T) {
	container := newMockMemoryContainer(t)
	container.memoryService.On("GetMemory", memoriesTestUserID, memoriesTestTeamID, memoriesTestMemoryID).
		Return(sampleStrictMemory(), nil)

	srv := createMemoryTestServer(container)
	req, w := getMemoryRequest(t, srv, memoriesTestMemoryID)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	assertProjectSummaryIsNull(t, w.Body.Bytes())
}

// A project id that is not a UUID cannot be rendered as the spec's uuid-typed
// `ProjectSummary.id`; the converter must surface that rather than emit a
// zero-valued object the client would render as a real project.
func TestToGenProjectSummary_RejectsANonUUIDProjectID(t *testing.T) {
	bad := &models.ProjectSummary{ID: "not-a-uuid", Name: "X", Slug: "x"}

	_, err := toGenPromptProjectSummary(bad)
	assert.Error(t, err)
	_, err = toGenArtifactProjectSummary(bad)
	assert.Error(t, err)
	_, err = toGenBlueprintProjectSummary(bad)
	assert.Error(t, err)
	_, err = toGenMemoryProjectSummary(bad)
	assert.Error(t, err)

	// nil in, nil out — the branch every list read takes.
	got, err := toGenPromptProjectSummary(nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}
