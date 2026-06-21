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

// versionMemoryID is the memory id used across the memory version handler tests. The
// team id reuses the shared versionTeamID UUID so requests conform to the OpenAPI path.
const versionMemoryID = "mem-1"

func memoryVersionURL(suffix string) string {
	return "/api/v1/" + versionTeamID + "/memories/" + versionMemoryID + "/versions" + suffix
}

func TestHandleListMemoryVersions_Success(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)
	createdBy := "user-123"
	summary := "Reworded the note"
	created := "Created the memory"
	avatar := "https://example.com/a.png"
	versions := []*models.ContentVersion{
		{
			ID: "ver-2", TeamID: versionTeamID, ResourceType: "memory", ResourceID: versionMemoryID,
			VersionNumber: 2, Content: "v2", ChangeSummary: &summary, ActorType: models.ActorTypeHuman,
			CreatedBy: &createdBy, CreatedAt: time.Now(),
			Author: &models.VersionAuthor{
				ID: createdBy, DisplayName: "Ada Lovelace", AvatarURL: &avatar, Initials: "AL",
			},
		},
		{
			ID: "ver-1", TeamID: versionTeamID, ResourceType: "memory", ResourceID: versionMemoryID,
			VersionNumber: 1, Content: "v1", ChangeSummary: &created, ActorType: models.ActorTypeSystem,
			CreatedBy: nil, Author: nil, CreatedAt: time.Now(),
		},
	}

	mockContainer.memoryService.On(
		"ListMemoryVersions", "user-123", versionTeamID, versionMemoryID,
	).Return(versions, nil)

	srv := createMemoryTestServer(mockContainer)

	req := makeMemoryAuthenticatedRequest("GET", memoryVersionURL(""), nil, "user-123")
	req = addRouteParams(req, map[string]string{"team_id": versionTeamID, "id": versionMemoryID})
	rr := httptest.NewRecorder()

	srv.handleListMemoryVersions(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response models.MemoryVersionListResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	assert.Len(t, response.Versions, 2)
	assert.Equal(t, 2, response.Versions[0].VersionNumber)
	require.NotNil(t, response.Versions[0].ChangeSummary)
	assert.Equal(t, "Reworded the note", *response.Versions[0].ChangeSummary)
	assert.Equal(t, models.ActorTypeHuman, response.Versions[0].ActorType)
	require.NotNil(t, response.Versions[0].Author)
	assert.Equal(t, "AL", response.Versions[0].Author.Initials)
	assert.Equal(t, models.ActorTypeSystem, response.Versions[1].ActorType)
	assert.Nil(t, response.Versions[1].Author)

	specconformance.AssertConformsToSpec(t, req, rr)
	mockContainer.memoryService.AssertExpectations(t)
}

func TestHandleGetMemoryVersion_Success(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)
	createdBy := "user-123"
	summary := "Reworded the note"
	avatar := "https://example.com/a.png"
	version := &models.ContentVersion{
		ID: "ver-2", TeamID: versionTeamID, ResourceType: "memory", ResourceID: versionMemoryID,
		VersionNumber: 2, Content: "v2", ChangeSummary: &summary, ActorType: models.ActorTypeHuman,
		CreatedBy: &createdBy, CreatedAt: time.Now(),
		Author: &models.VersionAuthor{
			ID: createdBy, DisplayName: "Ada Lovelace", AvatarURL: &avatar, Initials: "AL",
		},
	}

	mockContainer.memoryService.On(
		"GetMemoryVersion", "user-123", versionTeamID, versionMemoryID, 2,
	).Return(version, nil)

	srv := createMemoryTestServer(mockContainer)

	req := makeMemoryAuthenticatedRequest("GET", memoryVersionURL("/2"), nil, "user-123")
	req = addRouteParams(req, map[string]string{
		"team_id": versionTeamID, "id": versionMemoryID, "version_number": "2",
	})
	rr := httptest.NewRecorder()

	srv.handleGetMemoryVersion(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var response models.ContentVersion
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	assert.Equal(t, 2, response.VersionNumber)

	specconformance.AssertConformsToSpec(t, req, rr)
	mockContainer.memoryService.AssertExpectations(t)
}

func TestHandleGetMemoryVersion_NotFound(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	mockContainer.memoryService.On(
		"GetMemoryVersion", "user-123", versionTeamID, versionMemoryID, 99,
	).Return((*models.ContentVersion)(nil), repositories.ErrContentVersionNotFound)

	srv := createMemoryTestServer(mockContainer)

	req := makeMemoryAuthenticatedRequest("GET", memoryVersionURL("/99"), nil, "user-123")
	req = addRouteParams(req, map[string]string{
		"team_id": versionTeamID, "id": versionMemoryID, "version_number": "99",
	})
	rr := httptest.NewRecorder()

	srv.handleGetMemoryVersion(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
	mockContainer.memoryService.AssertExpectations(t)
}

func TestHandleGetMemoryVersion_BadVersionNumber(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)
	srv := createMemoryTestServer(mockContainer)

	req := makeMemoryAuthenticatedRequest("GET", memoryVersionURL("/abc"), nil, "user-123")
	req = addRouteParams(req, map[string]string{
		"team_id": versionTeamID, "id": versionMemoryID, "version_number": "abc",
	})
	rr := httptest.NewRecorder()

	srv.handleGetMemoryVersion(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestHandleRestoreMemoryVersion_Success(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)
	restored := &models.Memory{
		ID: versionMemoryID, ProjectID: testHandlerProjectID, TeamID: versionTeamID,
		UserID: "user-123", Text: "v1", Metadata: map[string]interface{}{},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	mockContainer.memoryService.On(
		"RestoreMemoryVersion", "user-123", versionTeamID, versionMemoryID, 1,
	).Return(restored, nil)

	srv := createMemoryTestServer(mockContainer)

	req := makeMemoryAuthenticatedRequest("POST", memoryVersionURL("/1/restore"), nil, "user-123")
	req = addRouteParams(req, map[string]string{
		"team_id": versionTeamID, "id": versionMemoryID, "version_number": "1",
	})
	rr := httptest.NewRecorder()

	srv.handleRestoreMemoryVersion(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var response models.Memory
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	assert.Equal(t, "v1", response.Text)

	specconformance.AssertConformsToSpec(t, req, rr)
	mockContainer.memoryService.AssertExpectations(t)
}
