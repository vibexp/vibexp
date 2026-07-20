package services

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// These tests pin the app-level relation cascade: deleting a resource removes
// every edge it appears on (either endpoint), since from_id/to_id carry no
// cascading FK. The cascade is best-effort, but the happy path must invoke it.

func TestArtifactService_Delete_CascadesRelations(t *testing.T) {
	const (
		projectID  = "project-1"
		slug       = "a"
		artifactID = "artifact-1"
	)
	logger, _ := logtest.New()
	repo := mocks.NewMockArtifactRepository(t)
	relationRepo := mocks.NewMockRelationRepository(t)

	repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, resRBACCaller, resRBACTeamID, projectID, slug).
		Return(&models.Artifact{ID: artifactID, UserID: resRBACCaller, TeamID: resRBACTeamID, Slug: slug}, nil).Once()
	repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, artifactID).Return(nil).Once()
	relationRepo.EXPECT().DeleteByResource(mock.Anything, resRBACTeamID, models.RelationResourceTypeArtifact, artifactID).
		Return(int64(2), nil).Once()

	svc := NewArtifactService(ArtifactServiceDeps{
		Repo:         repo,
		Authz:        authzForRole(t, models.TeamMemberRoleMember),
		Logger:       logger,
		RelationRepo: relationRepo,
	})
	require.NoError(t, svc.DeleteArtifactByProjectIDAndSlug(resRBACCaller, resRBACTeamID, projectID, slug))
}

func TestBlueprintService_Delete_CascadesRelations(t *testing.T) {
	const (
		projectID   = "project-1"
		slug        = "b"
		blueprintID = "blueprint-1"
	)
	logger, _ := logtest.New()
	repo := mocks.NewMockBlueprintRepository(t)
	relationRepo := mocks.NewMockRelationRepository(t)

	repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, resRBACCaller, resRBACTeamID, projectID, slug).
		Return(&models.Blueprint{ID: blueprintID, UserID: resRBACCaller, TeamID: resRBACTeamID, Slug: slug}, nil).Once()
	repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, blueprintID).Return(nil).Once()
	relationRepo.EXPECT().DeleteByResource(mock.Anything, resRBACTeamID, models.RelationResourceTypeBlueprint, blueprintID).
		Return(int64(1), nil).Once()

	svc := NewBlueprintService(BlueprintServiceDeps{
		Repo:         repo,
		Authz:        authzForRole(t, models.TeamMemberRoleMember),
		Logger:       logger,
		RelationRepo: relationRepo,
	})
	require.NoError(t, svc.DeleteBlueprintByProjectIDAndSlug(resRBACCaller, resRBACTeamID, projectID, slug))
}

func TestMemoryService_Delete_CascadesRelations(t *testing.T) {
	const memoryID = "memory-1"
	logger, _ := logtest.New()
	repo := mocks.NewMockMemoryRepository(t)
	relationRepo := mocks.NewMockRelationRepository(t)

	repo.EXPECT().GetByID(mock.Anything, resRBACCaller, resRBACTeamID, memoryID).
		Return(&models.Memory{ID: memoryID, UserID: resRBACCaller, TeamID: resRBACTeamID}, nil).Once()
	repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, memoryID).Return(nil).Once()
	relationRepo.EXPECT().DeleteByResource(mock.Anything, resRBACTeamID, models.RelationResourceTypeMemory, memoryID).
		Return(int64(1), nil).Once()

	svc := NewMemoryService(repo, nil, authzForRole(t, models.TeamMemberRoleMember), nil, logger, nil, nil, relationRepo)
	require.NoError(t, svc.DeleteMemory(resRBACCaller, resRBACTeamID, memoryID))
}

func TestPromptService_Delete_CascadesRelations(t *testing.T) {
	const promptID = "prompt-1"
	logger, _ := logtest.New()
	repo := mocks.NewMockPromptRepository(t)
	refRepo := mocks.NewMockPromptReferenceRepository(t)
	relationRepo := mocks.NewMockRelationRepository(t)

	repo.EXPECT().GetByID(mock.Anything, resRBACCaller, resRBACTeamID, promptID).
		Return(&models.Prompt{ID: promptID, UserID: resRBACCaller, TeamID: resRBACTeamID}, nil).Once()
	refRepo.EXPECT().HasDependents(mock.Anything, promptID).Return(false, nil).Once()
	repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, promptID).Return(nil).Once()
	relationRepo.EXPECT().DeleteByResource(mock.Anything, resRBACTeamID, models.RelationResourceTypePrompt, promptID).
		Return(int64(1), nil).Once()

	svc := NewPromptService(PromptServiceDeps{
		Repo:         repo,
		RefRepo:      refRepo,
		Authz:        authzForRole(t, models.TeamMemberRoleMember),
		Logger:       logger,
		RelationRepo: relationRepo,
	})
	require.NoError(t, svc.DeletePrompt(resRBACCaller, resRBACTeamID, promptID))
}
