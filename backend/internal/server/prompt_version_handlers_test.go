package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// versionPromptSlug is the prompt slug used across the prompt version handler tests. The
// team id reuses the shared versionTeamID UUID so requests conform to the OpenAPI path.
const versionPromptSlug = "weekly-report"

func promptVersionURL(suffix string) string {
	return "/api/v1/" + versionTeamID + "/prompts/" + versionPromptSlug + "/versions" + suffix
}

func TestHandleListPromptVersions_Success(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	createdBy := "user-123"
	summary := "Tweaked the {{name}} placeholder"
	created := "Created the prompt"
	avatar := "https://example.com/a.png"
	versions := []*models.ContentVersion{
		{
			ID: "ver-2", TeamID: versionTeamID, ResourceType: "prompt", ResourceID: "prompt-1",
			VersionNumber: 2, Content: "Hello {{name}} @intro", ChangeSummary: &summary,
			ActorType: models.ActorTypeHuman, CreatedBy: &createdBy, CreatedAt: time.Now(),
			Author: &models.VersionAuthor{
				ID: createdBy, DisplayName: "Ada Lovelace", AvatarURL: &avatar, Initials: "AL",
			},
		},
		{
			ID: "ver-1", TeamID: versionTeamID, ResourceType: "prompt", ResourceID: "prompt-1",
			VersionNumber: 1, Content: "Hello {{name}}", ChangeSummary: &created,
			ActorType: models.ActorTypeSystem, CreatedBy: nil, Author: nil, CreatedAt: time.Now(),
		},
	}

	mockContainer.promptService.On(
		"ListPromptVersions", "user-123", versionTeamID, versionPromptSlug,
	).Return(versions, nil)

	srv := createTestServer(mockContainer)

	req := makeAuthenticatedRequest("GET", promptVersionURL(""), nil, "user-123")
	req = addRouteParams(req, map[string]string{"team_id": versionTeamID, "slug": versionPromptSlug})
	rr := httptest.NewRecorder()

	srv.handleListPromptVersions(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response models.PromptVersionListResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	assert.Len(t, response.Versions, 2)
	assert.Equal(t, 2, response.Versions[0].VersionNumber)
	// The versioned content is the raw template (placeholders/@refs preserved).
	assert.Equal(t, "Hello {{name}} @intro", response.Versions[0].Content)
	require.NotNil(t, response.Versions[0].Author)
	assert.Equal(t, "AL", response.Versions[0].Author.Initials)
	assert.Equal(t, models.ActorTypeSystem, response.Versions[1].ActorType)

	specconformance.AssertConformsToSpec(t, req, rr)
	mockContainer.promptService.AssertExpectations(t)
}

func TestHandleGetPromptVersion_Success(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	createdBy := "user-123"
	summary := "Tweaked the {{name}} placeholder"
	version := &models.ContentVersion{
		ID: "ver-2", TeamID: versionTeamID, ResourceType: "prompt", ResourceID: "prompt-1",
		VersionNumber: 2, Content: "Hello {{name}} @intro", ChangeSummary: &summary,
		ActorType: models.ActorTypeHuman, CreatedBy: &createdBy, CreatedAt: time.Now(),
	}

	mockContainer.promptService.On(
		"GetPromptVersion", "user-123", versionTeamID, versionPromptSlug, 2,
	).Return(version, nil)

	srv := createTestServer(mockContainer)

	req := makeAuthenticatedRequest("GET", promptVersionURL("/2"), nil, "user-123")
	req = addRouteParams(req, map[string]string{
		"team_id": versionTeamID, "slug": versionPromptSlug, "version_number": "2",
	})
	rr := httptest.NewRecorder()

	srv.handleGetPromptVersion(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var response models.ContentVersion
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	assert.Equal(t, 2, response.VersionNumber)
	assert.Equal(t, "Hello {{name}} @intro", response.Content)

	specconformance.AssertConformsToSpec(t, req, rr)
	mockContainer.promptService.AssertExpectations(t)
}

func TestHandleGetPromptVersion_NotFound(t *testing.T) {
	mockContainer := newMockPromptContainer(t)

	mockContainer.promptService.On(
		"GetPromptVersion", "user-123", versionTeamID, versionPromptSlug, 99,
	).Return((*models.ContentVersion)(nil), repositories.ErrPromptNotFound)

	srv := createTestServer(mockContainer)

	req := makeAuthenticatedRequest("GET", promptVersionURL("/99"), nil, "user-123")
	req = addRouteParams(req, map[string]string{
		"team_id": versionTeamID, "slug": versionPromptSlug, "version_number": "99",
	})
	rr := httptest.NewRecorder()

	srv.handleGetPromptVersion(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
	mockContainer.promptService.AssertExpectations(t)
}

func TestHandleGetPromptVersion_BadVersionNumber(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	srv := createTestServer(mockContainer)

	req := makeAuthenticatedRequest("GET", promptVersionURL("/abc"), nil, "user-123")
	req = addRouteParams(req, map[string]string{
		"team_id": versionTeamID, "slug": versionPromptSlug, "version_number": "abc",
	})
	rr := httptest.NewRecorder()

	srv.handleGetPromptVersion(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestHandleRestorePromptVersion_Success(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	restored := &models.Prompt{
		ID: "prompt-1", Slug: versionPromptSlug, Name: "Weekly report", TeamID: versionTeamID,
		UserID: "user-123", Body: "Hello {{name}}", Status: "published",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	mockContainer.promptService.On(
		"RestorePromptVersion", "user-123", versionTeamID, versionPromptSlug, 1,
	).Return(restored, nil)

	srv := createTestServer(mockContainer)

	req := makeAuthenticatedRequest("POST", promptVersionURL("/1/restore"), nil, "user-123")
	req = addRouteParams(req, map[string]string{
		"team_id": versionTeamID, "slug": versionPromptSlug, "version_number": "1",
	})
	rr := httptest.NewRecorder()

	srv.handleRestorePromptVersion(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var response models.Prompt
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	// Restore preserves the raw template verbatim.
	assert.Equal(t, "Hello {{name}}", response.Body)

	specconformance.AssertConformsToSpec(t, req, rr)
	mockContainer.promptService.AssertExpectations(t)
}
