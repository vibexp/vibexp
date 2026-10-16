package server

// The 423 an indirect delete or move answers with while locked resources stand
// in its way (issue #1290): delete project, migrate project, delete type and
// delete status. Every response is validated against the spec.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/services/projectmigration"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	lockedTestArtifactID = "990e8400-e29b-41d4-a716-446655440009"
	lockedTestPromptID   = "990e8400-e29b-41d4-a716-446655440010"
)

// lockedTestRefs is what the guards in these tests report as locked.
func lockedTestRefs() []models.LockedResourceRef {
	return []models.LockedResourceRef{
		{ResourceType: models.RelationResourceTypeArtifact, ID: lockedTestArtifactID, Slug: "runbook", Title: "Runbook"},
		{ResourceType: models.RelationResourceTypePrompt, ID: lockedTestPromptID, Slug: "triage", Title: "Triage"},
	}
}

// lockedTestError is the error a guard raises for reason.
func lockedTestError(reason error) error {
	return &repositories.LockedResourcesError{Reason: reason, Resources: lockedTestRefs()}
}

// requireLockedResourcesProblem asserts w is the documented 423: status, media
// type, code, the detail sentence, and the locked resources in metadata exactly
// as the guard reported them.
func requireLockedResourcesProblem(t *testing.T, w *httptest.ResponseRecorder, wantDetail string) {
	t.Helper()
	require.Equal(t, http.StatusLocked, w.Code, w.Body.String())
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var problem struct {
		Code     string `json:"code"`
		Status   int    `json:"status"`
		Detail   string `json:"detail"`
		Metadata struct {
			LockedResources []models.LockedResourceRef `json:"locked_resources"`
		} `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
	assert.Equal(t, "HAS_LOCKED_RESOURCES", problem.Code)
	assert.Equal(t, http.StatusLocked, problem.Status)
	assert.Equal(t, wantDetail, problem.Detail)
	assert.Equal(t, lockedTestRefs(), problem.Metadata.LockedResources)
}

func TestLockedResourcesProblem(t *testing.T) {
	t.Run("each reason gets its own sentence", func(t *testing.T) {
		for reason, want := range map[error]string{
			repositories.ErrProjectHasLockedResources:      lockedMsgProject,
			repositories.ErrTypeUsedByLockedResource:       lockedMsgType,
			repositories.ErrTeamStatusUsedByLockedResource: lockedMsgStatus,
			errors.New("some future guard"):                lockedMsgDefault,
		} {
			apiErr := lockedResourcesProblem(lockedTestError(reason))
			require.NotNil(t, apiErr, reason)
			assert.Equal(t, http.StatusLocked, apiErr.Status, reason)
			assert.Equal(t, apierrors.CodeHasLockedResources, apiErr.Code, reason)
			assert.Equal(t, want, apiErr.Detail, reason)
		}
	})

	t.Run("a wrapped refusal is still found", func(t *testing.T) {
		wrapped := fmt.Errorf("Migrate: %w", lockedTestError(repositories.ErrProjectHasLockedResources))
		require.NotNil(t, lockedResourcesProblem(wrapped))
	})

	t.Run("any other error is not this problem", func(t *testing.T) {
		assert.Nil(t, lockedResourcesProblem(errors.New("connection reset")))
		assert.Nil(t, lockedResourcesProblem(repositories.ErrTypeUsedByLockedResource),
			"a bare reason carries no resources to list")
		assert.False(t, writeLockedResources(httptest.NewRecorder(), nil, errors.New("connection reset")))
	})
}

const (
	lockedProjTeamID = "550e8400-e29b-41d4-a716-446655440000"
	lockedProjUserID = "user-test-123"
	lockedProjSlug   = "release-notes"
)

// makeDeleteProjectRequest builds DELETE /api/v1/{team_id}/projects/{slug} as
// the test user, with the chi params a direct handler call needs.
func makeDeleteProjectRequest() *http.Request {
	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/"+lockedProjTeamID+"/projects/"+lockedProjSlug, nil)
	ctx := context.WithValue(req.Context(), contextKeyUserID, lockedProjUserID)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("team_id", lockedProjTeamID)
	rctx.URLParams.Add("slug", lockedProjSlug)
	return req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
}

func TestHandleDeleteProject_LockedResources_Returns423(t *testing.T) {
	container := newProjectGitHubTestContainer(t)
	container.projectSvc.EXPECT().
		DeleteProject(mock.Anything, lockedProjTeamID, lockedProjUserID, lockedProjSlug).
		Return(lockedTestError(repositories.ErrProjectHasLockedResources)).Once()

	req := makeDeleteProjectRequest()
	w := httptest.NewRecorder()
	createProjectGitHubServer(container).handleDeleteProject(w, req)

	requireLockedResourcesProblem(t, w, lockedMsgProject)
	specconformance.AssertConformsToSpec(t, req, w)
}

// The same request succeeds once nothing in the project is locked.
func TestHandleDeleteProject_Unlocked_Returns204(t *testing.T) {
	container := newProjectGitHubTestContainer(t)
	container.projectSvc.EXPECT().
		DeleteProject(mock.Anything, lockedProjTeamID, lockedProjUserID, lockedProjSlug).Return(nil).Once()

	req := makeDeleteProjectRequest()
	w := httptest.NewRecorder()
	createProjectGitHubServer(container).handleDeleteProject(w, req)

	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
}

// makeMigrateProjectRequest builds POST
// /api/v1/{team_id}/projects/{project_id}/migration as the test user.
func makeMigrateProjectRequest(t *testing.T) *http.Request {
	t.Helper()
	body, err := json.Marshal(projectmigration.MigrationRequest{
		DestinationProjectID: migrationTestDstProjID,
		Resources: projectmigration.ResourceSelections{
			Artifacts: projectmigration.ResourceSelection{All: true},
		},
		ConflictPolicy: projectmigration.ConflictPolicySkip,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/"+migrationTestTeamID+"/projects/"+migrationTestSrcProjID+"/migration", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-test-123"))
	return withMigrationChiParams(req, migrationTestTeamID, migrationTestSrcProjID)
}

// The service wraps the refusal ("Migrate: ..."), and the handler must still
// answer 423 rather than fall through to its string-matching 500.
func TestHandleMigrateProject_LockedResources_Returns423(t *testing.T) {
	svc := new(mockProjectMigrationService)
	svc.On("Migrate", mock.Anything, "user-test-123", migrationTestTeamID, migrationTestSrcProjID, mock.Anything).
		Return(nil, fmt.Errorf("Migrate: %w", lockedTestError(repositories.ErrProjectHasLockedResources)))

	req := makeMigrateProjectRequest(t)
	w := httptest.NewRecorder()
	newMigrationServer(t, svc).handleMigrateProject(w, req)

	requireLockedResourcesProblem(t, w, lockedMsgProject)
	specconformance.AssertConformsToSpec(t, req, w)
	svc.AssertExpectations(t)
}

func TestHandleMigrateProject_Success_ConformsToSpec(t *testing.T) {
	svc := new(mockProjectMigrationService)
	svc.On("Migrate", mock.Anything, "user-test-123", migrationTestTeamID, migrationTestSrcProjID, mock.Anything).
		Return(&projectmigration.MigrationResult{
			Migrated: projectmigration.ResourceMigrationCounts{Artifacts: 1},
		}, nil)

	req := makeMigrateProjectRequest(t)
	w := httptest.NewRecorder()
	newMigrationServer(t, svc).handleMigrateProject(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
}

func TestDeleteType_UsedByLockedArtifact_Returns423(t *testing.T) {
	typeID := "770e8400-e29b-41d4-a716-446655440002"
	svc := servicesmocks.NewMockTypeServiceInterface(t)
	svc.EXPECT().Delete(mock.Anything, testTypesTeamID, typeID).
		Return(lockedTestError(repositories.ErrTypeUsedByLockedResource))

	req := makeTypesRequest(http.MethodDelete, "/api/v1/"+testTypesTeamID+"/types/"+typeID, "")
	w := httptest.NewRecorder()
	createTestTypesServer(svc).ServeHTTP(w, req)

	requireLockedResourcesProblem(t, w, lockedMsgType)
	specconformance.AssertConformsToSpec(t, req, w)
}

func TestDeleteStatus_CarriedByLockedResource_Returns423(t *testing.T) {
	svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
	svc.EXPECT().Delete(mock.Anything, mock.Anything).
		Return(nil, lockedTestError(repositories.ErrTeamStatusUsedByLockedResource))

	// serveStatuses validates the response against the spec.
	w := serveStatuses(t, newStatusesTestServer(svc), http.MethodDelete, testStatusesByID, "")

	requireLockedResourcesProblem(t, w, lockedMsgStatus)
}
