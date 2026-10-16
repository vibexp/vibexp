package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

// Lock enforcement over MCP (issue #1289): the update tools and
// delete_resource share the service methods the REST handlers call, so a
// locked resource is refused there too. The tool error has to say why: it
// carries the frozen field, the locker and the lock comment.

const mcpLockComment = "Used by the nightly release agent"

func mcpLockRefusal(resourceType, field string) *services.ResourceLockedError {
	return &services.ResourceLockedError{
		ResourceType: resourceType, Field: field,
		LockedBy: "user-locker", LockedByName: "Ada Lovelace", Comment: mcpLockComment,
	}
}

func TestUpdatePrompt_LockedSurfacesTheLockComment(t *testing.T) {
	srv, promptService := newPromptTestServer(t)
	promptService.On("UpdatePromptBySlug", mock.Anything, testMemberUserID, testTeamUUID, "my-prompt",
		mock.AnythingOfType("*models.UpdatePromptRequest"),
	).Return(nil, mcpLockRefusal(models.RelationResourceTypePrompt, "status"))

	params := &UpdatePromptParams{TeamID: testTeamUUID, Slug: "my-prompt", Status: "draft"}
	result, structured, err := srv.updatePrompt(context.Background(), nil, params, testMemberUserID)

	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Nil(t, structured)
	assert.Equal(t,
		`Failed to update prompt: cannot change status: this prompt is locked by Ada Lovelace: "`+mcpLockComment+`"`,
		extractText(t, result))
}

func TestUpdateBlueprint_LockedSurfacesTheLockComment(t *testing.T) {
	srv, blueprintService := newBlueprintTestServer(t)
	blueprintService.On("UpdateBlueprintByProjectIDAndSlugInTeam", mock.Anything,
		testMemberUserID, testTeamUUID, testBlueprintProjectID, "bp",
		mock.AnythingOfType("*models.UpdateBlueprintRequest"),
	).Return(nil, mcpLockRefusal(models.RelationResourceTypeBlueprint, "type"))

	params := &UpdateBlueprintParams{TeamID: testTeamUUID, ProjectID: testBlueprintProjectID, Slug: "bp", Title: "x"}
	result, structured, err := srv.updateBlueprint(context.Background(), nil, params, testMemberUserID)

	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Nil(t, structured)
	text := extractText(t, result)
	assert.Contains(t, text, "cannot change type: this blueprint is locked by Ada Lovelace")
	assert.Contains(t, text, mcpLockComment)
}

func TestUpdateArtifact_LockedSurfacesTheLockComment(t *testing.T) {
	srv, artifactService := newArtifactTestServer(t)
	artifactService.On("UpdateArtifactByProjectIDAndSlugInTeam", mock.Anything,
		testMemberUserID, testTeamUUID, testArtifactProjectID, "art",
		mock.AnythingOfType("*models.UpdateArtifactRequest"),
	).Return(nil, mcpLockRefusal(models.RelationResourceTypeArtifact, "labels"))

	params := &UpdateArtifactParams{TeamID: testTeamUUID, ProjectID: testArtifactProjectID, Slug: "art", Title: "x"}
	result, structured, err := srv.updateArtifact(context.Background(), nil, params, testMemberUserID)

	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Nil(t, structured)
	text := extractText(t, result)
	assert.Contains(t, text, "cannot change labels: this artifact is locked by Ada Lovelace")
	assert.Contains(t, text, mcpLockComment)
}

// delete_resource on a locked resource fails with the lock comment and skips
// the embedding cleanup that follows a real delete.
func TestDeleteResource_LockedSurfacesTheLockComment(t *testing.T) {
	tests := []struct {
		resourceType string
		params       *DeleteResourceParams
		expect       func(m *deleteResourceMocks, refusal error)
	}{
		{
			resourceType: resourceTypePrompt,
			params:       &DeleteResourceParams{TeamID: testTeamUUID, ResourceType: "prompt", Slug: "p"},
			expect: func(m *deleteResourceMocks, refusal error) {
				m.prompt.On("GetPromptBySlug", testMemberUserID, testTeamUUID, "p").
					Return(&models.Prompt{ID: "prompt-1"}, nil)
				m.prompt.On("DeletePromptBySlug", testMemberUserID, testTeamUUID, "p").Return(refusal)
			},
		},
		{
			resourceType: resourceTypeArtifact,
			params: &DeleteResourceParams{
				TeamID: testTeamUUID, ResourceType: "artifact", ProjectID: testProjectID, Slug: "a",
			},
			expect: func(m *deleteResourceMocks, refusal error) {
				m.artifact.On("GetArtifactByProjectIDAndSlugInTeam", testMemberUserID, testTeamUUID, testProjectID, "a").
					Return(&models.Artifact{ID: "artifact-1"}, nil)
				m.artifact.On("DeleteArtifactByProjectIDAndSlug", mock.Anything, testMemberUserID, testTeamUUID, testProjectID, "a").
					Return(refusal)
			},
		},
		{
			resourceType: resourceTypeBlueprint,
			params: &DeleteResourceParams{
				TeamID: testTeamUUID, ResourceType: "blueprint", ProjectID: testProjectID, Slug: "b",
			},
			expect: func(m *deleteResourceMocks, refusal error) {
				m.blueprint.On("GetBlueprintByProjectIDAndSlugInTeam", testMemberUserID, testTeamUUID, testProjectID, "b").
					Return(&models.Blueprint{ID: "blueprint-1"}, nil)
				m.blueprint.On("DeleteBlueprintByProjectIDAndSlug", mock.Anything, testMemberUserID, testTeamUUID, testProjectID, "b").
					Return(refusal)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.resourceType, func(t *testing.T) {
			srv, m := newDeleteResourceTestServer(t)
			tt.expect(m, mcpLockRefusal(tt.resourceType, ""))

			result, structured, err := srv.deleteResource(context.Background(), nil, tt.params, testMemberUserID)

			assertDeleteIsError(t, result, structured, err)
			assert.Equal(t,
				"Failed to delete "+tt.resourceType+": cannot delete: this "+tt.resourceType+
					` is locked by Ada Lovelace: "`+mcpLockComment+`"`,
				extractText(t, result))
			m.embedding.AssertNotCalled(t, "DeleteEmbeddingsByEntity")
		})
	}
}
