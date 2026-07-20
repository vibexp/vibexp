package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// These tests exercise the error/edge branches the happy-path suite skips:
// repository failures are logged and propagated (never swallowed), and the
// concurrent-confirm race is reported as already-confirmed.

var errRelBoom = errors.New("boom")

func TestRelationService_Create_EndpointLookupErrorPropagates(t *testing.T) {
	repo := mocks.NewMockRelationRepository(t)
	repo.EXPECT().ResourceProjectID(mock.Anything, relTeamID, models.RelationResourceTypeArtifact, relFromID).
		Return("", false, errRelBoom).Once()
	svc := newRelationService(t, repo, nil, allowAllAuthz{})

	_, _, err := svc.Create(context.Background(), relCaller, relTeamID, validRelationReq())

	assert.ErrorIs(t, err, errRelBoom)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestRelationService_Create_RepoErrorPropagates(t *testing.T) {
	repo := mocks.NewMockRelationRepository(t)
	repo.EXPECT().ResourceProjectID(mock.Anything, relTeamID, models.RelationResourceTypeArtifact, relFromID).
		Return(relProject, true, nil).Once()
	repo.EXPECT().ResourceProjectID(mock.Anything, relTeamID, models.RelationResourceTypeBlueprint, relToID).
		Return(relProject, true, nil).Once()
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, false, errRelBoom).Once()
	svc := newRelationService(t, repo, nil, allowAllAuthz{})

	_, _, err := svc.Create(context.Background(), relCaller, relTeamID, validRelationReq())

	assert.ErrorIs(t, err, errRelBoom)
}

func TestRelationService_Confirm_RaceReportsAlreadyConfirmed(t *testing.T) {
	repo := mocks.NewMockRelationRepository(t)
	repo.EXPECT().GetByID(mock.Anything, relTeamID, relRelationID).
		Return(&models.Relation{ID: relRelationID, TeamID: relTeamID, Status: models.RelationStatusSuggested}, nil).Once()
	// A concurrent confirm won the race: the status='suggested' UPDATE matched
	// nothing, so the repo returns not-found.
	repo.EXPECT().Confirm(mock.Anything, relTeamID, relRelationID, relCaller).
		Return(nil, repositories.ErrRelationNotFound).Once()
	svc := newRelationService(t, repo, nil, allowAllAuthz{})

	_, err := svc.Confirm(context.Background(), relCaller, relTeamID, relRelationID)

	assert.ErrorIs(t, err, ErrRelationAlreadyConfirmed)
}

func TestRelationService_Confirm_RepoErrorPropagates(t *testing.T) {
	repo := mocks.NewMockRelationRepository(t)
	repo.EXPECT().GetByID(mock.Anything, relTeamID, relRelationID).
		Return(&models.Relation{ID: relRelationID, TeamID: relTeamID, Status: models.RelationStatusSuggested}, nil).Once()
	repo.EXPECT().Confirm(mock.Anything, relTeamID, relRelationID, relCaller).
		Return(nil, errRelBoom).Once()
	svc := newRelationService(t, repo, nil, allowAllAuthz{})

	_, err := svc.Confirm(context.Background(), relCaller, relTeamID, relRelationID)

	assert.ErrorIs(t, err, errRelBoom)
}

func TestRelationService_Delete_RepoErrorPropagates(t *testing.T) {
	owner := relCaller
	repo := mocks.NewMockRelationRepository(t)
	repo.EXPECT().GetByID(mock.Anything, relTeamID, relRelationID).
		Return(&models.Relation{ID: relRelationID, TeamID: relTeamID, CreatedBy: &owner}, nil).Once()
	repo.EXPECT().Delete(mock.Anything, relTeamID, relRelationID).Return(errRelBoom).Once()
	svc := newRelationService(t, repo, nil, authzForRole(t, models.TeamMemberRoleMember))

	err := svc.Delete(context.Background(), relCaller, relTeamID, relRelationID)

	assert.ErrorIs(t, err, errRelBoom)
}

func TestRelationService_ListByResource_ErrorBranches(t *testing.T) {
	const rtype = models.RelationResourceTypeArtifact

	t.Run("membership lookup error propagates", func(t *testing.T) {
		repo := mocks.NewMockRelationRepository(t)
		svc := newRelationService(t, repo, membershipStub{err: errRelBoom}, allowAllAuthz{})

		_, err := svc.ListByResource(context.Background(), relCaller, relTeamID, rtype, relFromID, 1, 5)

		require.Error(t, err)
		repo.AssertNotCalled(t, "ListByResource",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		repo := mocks.NewMockRelationRepository(t)
		repo.EXPECT().ListByResource(mock.Anything, relTeamID, rtype, relFromID, 1, 20).
			Return(nil, 0, errRelBoom).Once()
		svc := newRelationService(t, repo, membershipStub{isMember: true}, allowAllAuthz{})

		_, err := svc.ListByResource(context.Background(), relCaller, relTeamID, rtype, relFromID, 1, 20)

		assert.ErrorIs(t, err, errRelBoom)
	})
}

// The cascade is best-effort: a relation-repo failure is logged but must NOT
// fail the already-completed resource delete.
func TestMemoryService_Delete_RelationCascadeErrorIsSwallowed(t *testing.T) {
	const memoryID = "memory-1"
	logger, _ := logtest.New()
	repo := mocks.NewMockMemoryRepository(t)
	relationRepo := mocks.NewMockRelationRepository(t)

	repo.EXPECT().GetByID(mock.Anything, resRBACCaller, resRBACTeamID, memoryID).
		Return(&models.Memory{ID: memoryID, UserID: resRBACCaller, TeamID: resRBACTeamID}, nil).Once()
	repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, memoryID).Return(nil).Once()
	relationRepo.EXPECT().DeleteByResource(mock.Anything, resRBACTeamID, models.RelationResourceTypeMemory, memoryID).
		Return(int64(0), errRelBoom).Once()

	svc := NewMemoryService(repo, nil, authzForRole(t, models.TeamMemberRoleMember), nil, logger, nil, nil, relationRepo)
	require.NoError(t, svc.DeleteMemory(resRBACCaller, resRBACTeamID, memoryID))
}

func TestArtifactService_Delete_RelationCascadeErrorIsSwallowed(t *testing.T) {
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
		Return(int64(0), errRelBoom).Once()

	svc := NewArtifactService(ArtifactServiceDeps{
		Repo:         repo,
		Authz:        authzForRole(t, models.TeamMemberRoleMember),
		Logger:       logger,
		RelationRepo: relationRepo,
	})
	require.NoError(t, svc.DeleteArtifactByProjectIDAndSlug(resRBACCaller, resRBACTeamID, projectID, slug))
}

func TestBlueprintService_Delete_RelationCascadeErrorIsSwallowed(t *testing.T) {
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
		Return(int64(0), errRelBoom).Once()

	svc := NewBlueprintService(BlueprintServiceDeps{
		Repo:         repo,
		Authz:        authzForRole(t, models.TeamMemberRoleMember),
		Logger:       logger,
		RelationRepo: relationRepo,
	})
	require.NoError(t, svc.DeleteBlueprintByProjectIDAndSlug(resRBACCaller, resRBACTeamID, projectID, slug))
}

func TestPromptService_Delete_RelationCascadeErrorIsSwallowed(t *testing.T) {
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
		Return(int64(0), errRelBoom).Once()

	svc := NewPromptService(PromptServiceDeps{
		Repo:         repo,
		RefRepo:      refRepo,
		Authz:        authzForRole(t, models.TeamMemberRoleMember),
		Logger:       logger,
		RelationRepo: relationRepo,
	})
	require.NoError(t, svc.DeletePrompt(resRBACCaller, resRBACTeamID, promptID))
}
