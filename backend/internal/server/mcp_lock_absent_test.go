package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Lock state is deliberately absent from every MCP read result (issue #1288):
// an agent meets locks through the MCP lock tool (#1296), not as a field on
// what it reads. The models now emit `lock` for the REST API, and get_resource
// returns the full model, so each MCP read is pinned here against a LOCKED
// resource -- both the JSON text an agent reads and the structured content.

func assertNoLockAnywhere(t *testing.T, result *mcp.CallToolResult, structured any) {
	t.Helper()
	require.NotNil(t, result)
	require.False(t, result.IsError, extractText(t, result))

	text := extractText(t, result)
	assert.NotContains(t, text, `"lock"`)
	assert.NotContains(t, text, heldLock().Comment)
	assert.NotContains(t, text, lockTestLockerName)

	raw, err := json.Marshal(structured)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"lock"`)
	assert.NotContains(t, string(raw), heldLock().Comment)
}

func TestMCPGetResource_CarriesNoLock(t *testing.T) {
	t.Run(resourceTypeArtifact, func(t *testing.T) {
		srv, m := newDeleteResourceTestServer(t)
		m.artifact.On("GetArtifactByProjectIDAndSlugInTeam", testMemberUserID, testTeamUUID, testProjectID, "art").
			Return(&models.Artifact{
				ID: "art-1", Slug: "art", Title: "Locked artifact", Content: "full body",
				TeamID: testTeamUUID, Lock: heldLock(),
			}, nil)

		result, structured, err := srv.getResource(context.Background(), nil, &GetResourceParams{
			TeamID: testTeamUUID, ResourceType: resourceTypeArtifact, ProjectID: testProjectID, Slug: "art",
		}, testMemberUserID)

		require.NoError(t, err)
		assertNoLockAnywhere(t, result, structured)
		// The rest of the model is still there: only the lock is withheld.
		assert.Contains(t, extractText(t, result), "full body")
	})

	t.Run(resourceTypeBlueprint, func(t *testing.T) {
		srv, m := newDeleteResourceTestServer(t)
		m.blueprint.On("GetBlueprintByProjectIDAndSlugInTeam", testMemberUserID, testTeamUUID, testProjectID, "bp").
			Return(&models.Blueprint{
				ID: "bp-1", Slug: "bp", Title: "Locked blueprint", Content: "full body",
				TeamID: testTeamUUID, Lock: heldLock(),
			}, nil)

		result, structured, err := srv.getResource(context.Background(), nil, &GetResourceParams{
			TeamID: testTeamUUID, ResourceType: resourceTypeBlueprint, ProjectID: testProjectID, Slug: "bp",
		}, testMemberUserID)

		require.NoError(t, err)
		assertNoLockAnywhere(t, result, structured)
		assert.Contains(t, extractText(t, result), "full body")
	})
}

func TestMCPListResources_CarriesNoLock(t *testing.T) {
	list := func(t *testing.T, srv *Server, resourceType string) {
		t.Helper()
		result, structured, err := srv.listResources(context.Background(), nil, &ListResourcesParams{
			TeamID: testTeamUUID, ResourceType: resourceType, ProjectID: testProjectID,
		}, testMemberUserID)
		require.NoError(t, err)
		assertNoLockAnywhere(t, result, structured)
		assert.Contains(t, extractText(t, result), "Locked resource", "the locked item itself is listed")
	}

	t.Run(resourceTypeArtifact, func(t *testing.T) {
		srv, m := newDeleteResourceTestServer(t)
		m.artifact.On("ListArtifactsByProject", testMemberUserID, testProjectID, mock.Anything).
			Return(&models.ArtifactListResponse{
				Artifacts: []models.Artifact{{ID: "art-1", Slug: "art", Title: "Locked resource", Lock: heldLock()}},
				Page:      1, PerPage: 10, TotalCount: 1,
			}, nil)
		list(t, srv, resourceTypeArtifact)
	})

	t.Run(resourceTypeBlueprint, func(t *testing.T) {
		srv, m := newDeleteResourceTestServer(t)
		m.blueprint.On("ListBlueprintsByProject", testMemberUserID, testProjectID, mock.Anything).
			Return(&models.BlueprintListResponse{
				Blueprints: []models.Blueprint{{ID: "bp-1", Slug: "bp", Title: "Locked resource", Lock: heldLock()}},
				Page:       1, PerPage: 10, TotalCount: 1,
			}, nil)
		list(t, srv, resourceTypeBlueprint)
	})
}

// The shadowing wrapper only works while the model field keeps the JSON name
// `lock`: prove the model on its own DOES emit the lock, so the assertions
// above cannot pass against a model that never had one.
func TestMCPLockAbsence_TheModelItselfEmitsTheLock(t *testing.T) {
	for name, value := range map[string]any{
		"artifact":  &models.Artifact{Lock: heldLock()},
		"blueprint": &models.Blueprint{Lock: heldLock()},
	} {
		raw, err := json.Marshal(value)
		require.NoError(t, err, name)
		assert.Contains(t, string(raw), `"lock":{`, name)
	}
}
