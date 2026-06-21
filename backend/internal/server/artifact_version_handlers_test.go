package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	versionTeamID    = "550e8400-e29b-41d4-a716-446655440000"
	versionProjectID = "550e8400-e29b-41d4-a716-446655440000"
	versionSlug      = "test-slug"
)

func newVersionTestServer(mockArtifactService *servicesmocks.MockArtifactServiceInterface) *Server {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = &MockArtifactContainer{ArtifactServiceMock: mockArtifactService}
	return srv
}

func TestHandleListArtifactVersions_Success(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)
	createdBy := "user-123"
	summary := "Tightened the wording"
	created := "Created the artifact"
	avatar := "https://example.com/a.png"
	versions := []*models.ContentVersion{
		{
			ID: "ver-2", TeamID: versionTeamID, ResourceType: "artifact", ResourceID: "art-1",
			VersionNumber: 2, Content: "v2", ChangeSummary: &summary, ActorType: models.ActorTypeHuman,
			CreatedBy: &createdBy, CreatedAt: time.Now(),
			Author: &models.VersionAuthor{
				ID: createdBy, DisplayName: "Ada Lovelace", AvatarURL: &avatar, Initials: "AL",
			},
		},
		{
			ID: "ver-1", TeamID: versionTeamID, ResourceType: "artifact", ResourceID: "art-1",
			VersionNumber: 1, Content: "v1", ChangeSummary: &created, ActorType: models.ActorTypeSystem,
			CreatedBy: nil, Author: nil, CreatedAt: time.Now(),
		},
	}

	mockArtifactService.On(
		"ListArtifactVersionsInTeam", "user-123", versionTeamID, versionProjectID, versionSlug,
	).Return(versions, nil)

	srv := newVersionTestServer(mockArtifactService)

	url := "/api/v1/" + versionTeamID + "/artifacts/" + versionProjectID + "/" + versionSlug + "/versions"
	req := createAuthenticatedRequest("GET", url, "", "user-123")
	req = addURLParams(req, map[string]string{
		"team_id": versionTeamID, "project_id": versionProjectID, "slug": versionSlug,
	})
	rr := httptest.NewRecorder()

	srv.handleListArtifactVersions(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response models.ArtifactVersionListResponse
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
	mockArtifactService.AssertExpectations(t)
}

func TestHandleGetArtifactVersion_Success(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)
	createdBy := "user-123"
	summary := "Tightened the wording"
	avatar := "https://example.com/a.png"
	version := &models.ContentVersion{
		ID: "ver-2", TeamID: versionTeamID, ResourceType: "artifact", ResourceID: "art-1",
		VersionNumber: 2, Content: "v2", ChangeSummary: &summary, ActorType: models.ActorTypeHuman,
		CreatedBy: &createdBy, CreatedAt: time.Now(),
		Author: &models.VersionAuthor{
			ID: createdBy, DisplayName: "Ada Lovelace", AvatarURL: &avatar, Initials: "AL",
		},
	}

	mockArtifactService.On(
		"GetArtifactVersionInTeam", "user-123", versionTeamID, versionProjectID, versionSlug, 2,
	).Return(version, nil)

	srv := newVersionTestServer(mockArtifactService)

	url := "/api/v1/" + versionTeamID + "/artifacts/" + versionProjectID + "/" + versionSlug + "/versions/2"
	req := createAuthenticatedRequest("GET", url, "", "user-123")
	req = addURLParams(req, map[string]string{
		"team_id": versionTeamID, "project_id": versionProjectID, "slug": versionSlug, "version_number": "2",
	})
	rr := httptest.NewRecorder()

	srv.handleGetArtifactVersion(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var response models.ContentVersion
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	assert.Equal(t, 2, response.VersionNumber)

	specconformance.AssertConformsToSpec(t, req, rr)
	mockArtifactService.AssertExpectations(t)
}

func TestHandleGetArtifactVersion_NotFound(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)

	mockArtifactService.On(
		"GetArtifactVersionInTeam", "user-123", versionTeamID, versionProjectID, versionSlug, 99,
	).Return((*models.ContentVersion)(nil), repositories.ErrContentVersionNotFound)

	srv := newVersionTestServer(mockArtifactService)

	url := "/api/v1/" + versionTeamID + "/artifacts/" + versionProjectID + "/" + versionSlug + "/versions/99"
	req := createAuthenticatedRequest("GET", url, "", "user-123")
	req = addURLParams(req, map[string]string{
		"team_id": versionTeamID, "project_id": versionProjectID, "slug": versionSlug, "version_number": "99",
	})
	rr := httptest.NewRecorder()

	srv.handleGetArtifactVersion(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
	mockArtifactService.AssertExpectations(t)
}

func TestHandleGetArtifactVersion_BadVersionNumber(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)
	srv := newVersionTestServer(mockArtifactService)

	url := "/api/v1/" + versionTeamID + "/artifacts/" + versionProjectID + "/" + versionSlug + "/versions/abc"
	req := createAuthenticatedRequest("GET", url, "", "user-123")
	req = addURLParams(req, map[string]string{
		"team_id": versionTeamID, "project_id": versionProjectID, "slug": versionSlug, "version_number": "abc",
	})
	rr := httptest.NewRecorder()

	srv.handleGetArtifactVersion(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestHandleRestoreArtifactVersion_Success(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)
	restored := &models.Artifact{
		ID: "art-1", ProjectID: versionProjectID, Slug: versionSlug, TeamID: versionTeamID,
		UserID: "user-123", Title: "Doc", Content: "v1", Type: "general", Status: "active",
		Metadata: map[string]interface{}{}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	mockArtifactService.On(
		"RestoreArtifactVersionInTeam", "user-123", versionTeamID, versionProjectID, versionSlug, 1,
	).Return(restored, nil)

	srv := newVersionTestServer(mockArtifactService)

	url := "/api/v1/" + versionTeamID + "/artifacts/" + versionProjectID + "/" + versionSlug + "/versions/1/restore"
	req := createAuthenticatedRequest("POST", url, "", "user-123")
	req = addURLParams(req, map[string]string{
		"team_id": versionTeamID, "project_id": versionProjectID, "slug": versionSlug, "version_number": "1",
	})
	rr := httptest.NewRecorder()

	srv.handleRestoreArtifactVersion(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var response models.Artifact
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	assert.Equal(t, "v1", response.Content)

	specconformance.AssertConformsToSpec(t, req, rr)
	mockArtifactService.AssertExpectations(t)
}
