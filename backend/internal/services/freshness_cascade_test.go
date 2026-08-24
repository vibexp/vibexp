package services

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// These tests pin the app-level freshness cascade (#762): deleting a resource
// clears its resource_freshness row, since resource_id carries no cascading FK
// (one column cannot reference four resource tables). The cascade is
// best-effort — a failure must not fail the completed delete, and an absent
// dependency must not panic — but the happy path must invoke it, with the
// resource-type string the freshness domain uses.

func TestArtifactService_Delete_CascadesFreshness(t *testing.T) {
	const (
		projectID  = "project-1"
		slug       = "a"
		artifactID = "artifact-1"
	)
	logger, _ := logtest.New()
	repo := mocks.NewMockArtifactRepository(t)
	freshnessRepo := mocks.NewMockResourceFreshnessRepository(t)

	repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, resRBACCaller, resRBACTeamID, projectID, slug).
		Return(&models.Artifact{ID: artifactID, UserID: resRBACCaller, TeamID: resRBACTeamID, Slug: slug}, nil).Once()
	repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, artifactID).Return(nil).Once()
	freshnessRepo.EXPECT().DeleteByResource(mock.Anything, "artifact", artifactID).
		Return(true, nil).Once()

	svc := NewArtifactService(ArtifactServiceDeps{
		Repo:          repo,
		Authz:         authzForRole(t, models.TeamMemberRoleMember),
		Logger:        logger,
		FreshnessRepo: freshnessRepo,
	})
	require.NoError(t, svc.DeleteArtifactByProjectIDAndSlug(resRBACCaller, resRBACTeamID, projectID, slug))
}

func TestBlueprintService_Delete_CascadesFreshness(t *testing.T) {
	const (
		projectID   = "project-1"
		slug        = "b"
		blueprintID = "blueprint-1"
	)
	logger, _ := logtest.New()
	repo := mocks.NewMockBlueprintRepository(t)
	freshnessRepo := mocks.NewMockResourceFreshnessRepository(t)

	repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, resRBACCaller, resRBACTeamID, projectID, slug).
		Return(&models.Blueprint{ID: blueprintID, UserID: resRBACCaller, TeamID: resRBACTeamID, Slug: slug}, nil).Once()
	repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, blueprintID).Return(nil).Once()
	freshnessRepo.EXPECT().DeleteByResource(mock.Anything, "blueprint", blueprintID).
		Return(true, nil).Once()

	svc := NewBlueprintService(BlueprintServiceDeps{
		Repo:          repo,
		Authz:         authzForRole(t, models.TeamMemberRoleMember),
		Logger:        logger,
		FreshnessRepo: freshnessRepo,
	})
	require.NoError(t, svc.DeleteBlueprintByProjectIDAndSlug(resRBACCaller, resRBACTeamID, projectID, slug))
}

func TestMemoryService_Delete_CascadesFreshness(t *testing.T) {
	const memoryID = "memory-1"
	logger, _ := logtest.New()
	repo := mocks.NewMockMemoryRepository(t)
	freshnessRepo := mocks.NewMockResourceFreshnessRepository(t)

	repo.EXPECT().GetByID(mock.Anything, resRBACCaller, resRBACTeamID, memoryID).
		Return(&models.Memory{ID: memoryID, UserID: resRBACCaller, TeamID: resRBACTeamID}, nil).Once()
	repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, memoryID).Return(nil).Once()
	freshnessRepo.EXPECT().DeleteByResource(mock.Anything, "memory", memoryID).
		Return(true, nil).Once()

	svc := NewMemoryService(
		repo, nil, authzForRole(t, models.TeamMemberRoleMember), nil, logger, nil, nil, nil, nil, freshnessRepo,
	)
	require.NoError(t, svc.DeleteMemory(resRBACCaller, resRBACTeamID, memoryID))
}

func TestPromptService_Delete_CascadesFreshness(t *testing.T) {
	const promptID = "prompt-1"
	logger, _ := logtest.New()
	repo := mocks.NewMockPromptRepository(t)
	refRepo := mocks.NewMockPromptReferenceRepository(t)
	freshnessRepo := mocks.NewMockResourceFreshnessRepository(t)

	repo.EXPECT().GetByID(mock.Anything, resRBACCaller, resRBACTeamID, promptID).
		Return(&models.Prompt{ID: promptID, UserID: resRBACCaller, TeamID: resRBACTeamID}, nil).Once()
	refRepo.EXPECT().HasDependents(mock.Anything, promptID).Return(false, nil).Once()
	repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, promptID).Return(nil).Once()
	freshnessRepo.EXPECT().DeleteByResource(mock.Anything, "prompt", promptID).
		Return(true, nil).Once()

	svc := NewPromptService(PromptServiceDeps{
		Repo:          repo,
		RefRepo:       refRepo,
		Authz:         authzForRole(t, models.TeamMemberRoleMember),
		Logger:        logger,
		FreshnessRepo: freshnessRepo,
	})
	require.NoError(t, svc.DeletePrompt(resRBACCaller, resRBACTeamID, promptID))
}

// A freshness clear that fails must not fail the delete: the resource row is
// already gone and the caller got what they asked for. The next evaluation run
// cannot repair this one (the resource no longer exists to be evaluated), so
// the warning log is the only signal — but failing the request would be worse.
func TestArtifactService_Delete_FreshnessClearFailureDoesNotFailDelete(t *testing.T) {
	const (
		projectID  = "project-1"
		slug       = "a"
		artifactID = "artifact-1"
	)
	logger, _ := logtest.New()
	repo := mocks.NewMockArtifactRepository(t)
	freshnessRepo := mocks.NewMockResourceFreshnessRepository(t)

	repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, resRBACCaller, resRBACTeamID, projectID, slug).
		Return(&models.Artifact{ID: artifactID, UserID: resRBACCaller, TeamID: resRBACTeamID, Slug: slug}, nil).Once()
	repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, artifactID).Return(nil).Once()
	freshnessRepo.EXPECT().DeleteByResource(mock.Anything, "artifact", artifactID).
		Return(false, errors.New("boom")).Once()

	svc := NewArtifactService(ArtifactServiceDeps{
		Repo:          repo,
		Authz:         authzForRole(t, models.TeamMemberRoleMember),
		Logger:        logger,
		FreshnessRepo: freshnessRepo,
	})
	require.NoError(t, svc.DeleteArtifactByProjectIDAndSlug(resRBACCaller, resRBACTeamID, projectID, slug))
}

// Freshness is optional wiring, exactly like the comment and relation repos: a
// service constructed without it must delete without panicking.
func TestResourceServices_Delete_WithoutFreshnessRepoDoesNotPanic(t *testing.T) {
	const (
		projectID  = "project-1"
		slug       = "a"
		artifactID = "artifact-1"
		promptID   = "prompt-1"
	)
	logger, _ := logtest.New()

	t.Run("artifact", func(t *testing.T) {
		repo := mocks.NewMockArtifactRepository(t)
		repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, resRBACCaller, resRBACTeamID, projectID, slug).
			Return(&models.Artifact{ID: artifactID, UserID: resRBACCaller, TeamID: resRBACTeamID, Slug: slug}, nil).Once()
		repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, artifactID).Return(nil).Once()

		svc := NewArtifactService(ArtifactServiceDeps{
			Repo:   repo,
			Authz:  authzForRole(t, models.TeamMemberRoleMember),
			Logger: logger,
		})
		require.NoError(t, svc.DeleteArtifactByProjectIDAndSlug(resRBACCaller, resRBACTeamID, projectID, slug))
	})

	t.Run("prompt", func(t *testing.T) {
		repo := mocks.NewMockPromptRepository(t)
		refRepo := mocks.NewMockPromptReferenceRepository(t)
		repo.EXPECT().GetByID(mock.Anything, resRBACCaller, resRBACTeamID, promptID).
			Return(&models.Prompt{ID: promptID, UserID: resRBACCaller, TeamID: resRBACTeamID}, nil).Once()
		refRepo.EXPECT().HasDependents(mock.Anything, promptID).Return(false, nil).Once()
		repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, promptID).Return(nil).Once()

		svc := NewPromptService(PromptServiceDeps{
			Repo:    repo,
			RefRepo: refRepo,
			Authz:   authzForRole(t, models.TeamMemberRoleMember),
			Logger:  logger,
		})
		require.NoError(t, svc.DeletePrompt(resRBACCaller, resRBACTeamID, promptID))
	})
}
