package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// newBlueprintVersionTestServer builds a server whose container exposes only the mocked
// BlueprintService, so version handlers can be exercised directly (middleware/routing bypassed).
func newBlueprintVersionTestServer(mockBlueprintService *servicesmocks.MockBlueprintServiceInterface) *Server {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = &MockBlueprintContainer{BlueprintServiceMock: mockBlueprintService}
	return srv
}

func TestHandleListBlueprintVersions_Success(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)
	createdBy := "user-123"
	summary := "Tightened the wording"
	created := "Created the blueprint"
	avatar := "https://example.com/a.png"
	versions := []*models.ContentVersion{
		{
			ID: "ver-2", TeamID: versionTeamID, ResourceType: "blueprint", ResourceID: "bp-1",
			VersionNumber: 2, Content: "v2", ChangeSummary: &summary, ActorType: models.ActorTypeHuman,
			CreatedBy: &createdBy, CreatedAt: time.Now(),
			Author: &models.VersionAuthor{
				ID: createdBy, DisplayName: "Ada Lovelace", AvatarURL: &avatar, Initials: "AL",
			},
		},
		{
			ID: "ver-1", TeamID: versionTeamID, ResourceType: "blueprint", ResourceID: "bp-1",
			VersionNumber: 1, Content: "v1", ChangeSummary: &created, ActorType: models.ActorTypeSystem,
			CreatedBy: nil, Author: nil, CreatedAt: time.Now(),
		},
	}

	mockBlueprintService.On(
		"ListBlueprintVersions", "user-123", versionProjectID, versionSlug,
	).Return(versions, nil)

	srv := newBlueprintVersionTestServer(mockBlueprintService)

	url := "/api/v1/" + versionTeamID + "/blueprints/" + versionProjectID + "/" + versionSlug + "/versions"
	req := createAuthenticatedRequest("GET", url, "", "user-123")
	req = addURLParams(req, map[string]string{
		"team_id": versionTeamID, "project_id": versionProjectID, "slug": versionSlug,
	})
	rr := httptest.NewRecorder()

	srv.handleListBlueprintVersions(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response models.BlueprintVersionListResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	assert.Len(t, response.Versions, 2)
	assert.Equal(t, 2, response.Versions[0].VersionNumber)
	require.NotNil(t, response.Versions[0].ChangeSummary)
	assert.Equal(t, "Tightened the wording", *response.Versions[0].ChangeSummary)
	assert.Equal(t, models.ActorTypeHuman, response.Versions[0].ActorType)
	require.NotNil(t, response.Versions[0].Author)
	assert.Equal(t, "AL", response.Versions[0].Author.Initials)
	assert.Equal(t, models.ActorTypeSystem, response.Versions[1].ActorType)
	assert.Nil(t, response.Versions[1].Author)

	specconformance.AssertConformsToSpec(t, req, rr)
	mockBlueprintService.AssertExpectations(t)
}

func TestHandleGetBlueprintVersion_Success(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)
	createdBy := "user-123"
	summary := "Tightened the wording"
	avatar := "https://example.com/a.png"
	version := &models.ContentVersion{
		ID: "ver-2", TeamID: versionTeamID, ResourceType: "blueprint", ResourceID: "bp-1",
		VersionNumber: 2, Content: "v2", ChangeSummary: &summary, ActorType: models.ActorTypeHuman,
		CreatedBy: &createdBy, CreatedAt: time.Now(),
		Author: &models.VersionAuthor{
			ID: createdBy, DisplayName: "Ada Lovelace", AvatarURL: &avatar, Initials: "AL",
		},
	}

	mockBlueprintService.On(
		"GetBlueprintVersion", "user-123", versionProjectID, versionSlug, 2,
	).Return(version, nil)

	srv := newBlueprintVersionTestServer(mockBlueprintService)

	url := "/api/v1/" + versionTeamID + "/blueprints/" + versionProjectID + "/" + versionSlug + "/versions/2"
	req := createAuthenticatedRequest("GET", url, "", "user-123")
	req = addURLParams(req, map[string]string{
		"team_id": versionTeamID, "project_id": versionProjectID, "slug": versionSlug, "version_number": "2",
	})
	rr := httptest.NewRecorder()

	srv.handleGetBlueprintVersion(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var response models.ContentVersion
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	assert.Equal(t, 2, response.VersionNumber)

	specconformance.AssertConformsToSpec(t, req, rr)
	mockBlueprintService.AssertExpectations(t)
}

func TestHandleGetBlueprintVersion_NotFound(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)

	mockBlueprintService.On(
		"GetBlueprintVersion", "user-123", versionProjectID, versionSlug, 99,
	).Return((*models.ContentVersion)(nil), repositories.ErrContentVersionNotFound)

	srv := newBlueprintVersionTestServer(mockBlueprintService)

	url := "/api/v1/" + versionTeamID + "/blueprints/" + versionProjectID + "/" + versionSlug + "/versions/99"
	req := createAuthenticatedRequest("GET", url, "", "user-123")
	req = addURLParams(req, map[string]string{
		"team_id": versionTeamID, "project_id": versionProjectID, "slug": versionSlug, "version_number": "99",
	})
	rr := httptest.NewRecorder()

	srv.handleGetBlueprintVersion(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
	mockBlueprintService.AssertExpectations(t)
}

func TestHandleGetBlueprintVersion_BadVersionNumber(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)
	srv := newBlueprintVersionTestServer(mockBlueprintService)

	url := "/api/v1/" + versionTeamID + "/blueprints/" + versionProjectID + "/" + versionSlug + "/versions/abc"
	req := createAuthenticatedRequest("GET", url, "", "user-123")
	req = addURLParams(req, map[string]string{
		"team_id": versionTeamID, "project_id": versionProjectID, "slug": versionSlug, "version_number": "abc",
	})
	rr := httptest.NewRecorder()

	srv.handleGetBlueprintVersion(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestHandleRestoreBlueprintVersion_Success(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)
	restored := &models.Blueprint{
		ID: "bp-1", ProjectID: versionProjectID, Slug: versionSlug, TeamID: versionTeamID,
		UserID: "user-123", Title: "Doc", Content: "v1", Type: "general", Status: "active",
		Metadata: map[string]interface{}{}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	mockBlueprintService.On(
		"RestoreBlueprintVersion", "user-123", versionProjectID, versionSlug, 1,
	).Return(restored, nil)

	srv := newBlueprintVersionTestServer(mockBlueprintService)

	url := "/api/v1/" + versionTeamID + "/blueprints/" + versionProjectID + "/" + versionSlug + "/versions/1/restore"
	req := createAuthenticatedRequest("POST", url, "", "user-123")
	req = addURLParams(req, map[string]string{
		"team_id": versionTeamID, "project_id": versionProjectID, "slug": versionSlug, "version_number": "1",
	})
	rr := httptest.NewRecorder()

	srv.handleRestoreBlueprintVersion(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var response models.Blueprint
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	assert.Equal(t, "v1", response.Content)

	specconformance.AssertConformsToSpec(t, req, rr)
	mockBlueprintService.AssertExpectations(t)
}
