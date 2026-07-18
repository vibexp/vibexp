package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// These tests pin the app-level cascade: deleting a resource (or removing a team
// member) also removes the associated comments, since resource_id/user_id carry
// no cascading FK on that path in the resource services. The cascade is
// best-effort, but the happy path must invoke it.

func TestArtifactService_Delete_CascadesComments(t *testing.T) {
	const (
		projectID  = "project-1"
		slug       = "a"
		artifactID = "artifact-1"
	)
	logger, _ := logtest.New()
	repo := mocks.NewMockArtifactRepository(t)
	commentRepo := mocks.NewMockCommentRepository(t)

	repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, commentCaller, commentTeamID, projectID, slug).
		Return(&models.Artifact{ID: artifactID, UserID: commentCaller, TeamID: commentTeamID, Slug: slug}, nil).Once()
	repo.EXPECT().Delete(mock.Anything, commentCaller, commentTeamID, artifactID).Return(nil).Once()
	commentRepo.EXPECT().DeleteByResource(mock.Anything, commentTeamID, models.CommentResourceTypeArtifact, artifactID).
		Return(int64(2), nil).Once()

	svc := NewArtifactService(ArtifactServiceDeps{
		Repo:              repo,
		TeamService:       nil,
		Authz:             authzForRole(t, models.TeamMemberRoleMember),
		EventManager:      nil,
		ResourceUsageSvc:  nil,
		Logger:            logger,
		ContentVersionSvc: nil,
		CommentRepo:       commentRepo,
	})
	require.NoError(t, svc.DeleteArtifactByProjectIDAndSlug(commentCaller, commentTeamID, projectID, slug))
}

func TestBlueprintService_Delete_CascadesComments(t *testing.T) {
	const (
		projectID   = "project-1"
		slug        = "b"
		blueprintID = "blueprint-1"
	)
	logger, _ := logtest.New()
	repo := mocks.NewMockBlueprintRepository(t)
	commentRepo := mocks.NewMockCommentRepository(t)

	repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, commentCaller, commentTeamID, projectID, slug).
		Return(&models.Blueprint{ID: blueprintID, UserID: commentCaller, TeamID: commentTeamID, Slug: slug}, nil).Once()
	repo.EXPECT().Delete(mock.Anything, commentCaller, commentTeamID, blueprintID).Return(nil).Once()
	commentRepo.EXPECT().DeleteByResource(mock.Anything, commentTeamID, models.CommentResourceTypeBlueprint, blueprintID).
		Return(int64(1), nil).Once()

	svc := NewBlueprintService(BlueprintServiceDeps{
		Repo:              repo,
		TeamService:       nil,
		Authz:             authzForRole(t, models.TeamMemberRoleMember),
		EventManager:      nil,
		ResourceUsageSvc:  nil,
		Logger:            logger,
		ContentVersionSvc: nil,
		CommentRepo:       commentRepo,
	})
	require.NoError(t, svc.DeleteBlueprintByProjectIDAndSlug(commentCaller, commentTeamID, projectID, slug))
}

func TestMemoryService_Delete_CascadesComments(t *testing.T) {
	const memoryID = "memory-1"
	logger, _ := logtest.New()
	repo := mocks.NewMockMemoryRepository(t)
	commentRepo := mocks.NewMockCommentRepository(t)

	repo.EXPECT().GetByID(mock.Anything, commentCaller, commentTeamID, memoryID).
		Return(&models.Memory{ID: memoryID, UserID: commentCaller, TeamID: commentTeamID}, nil).Once()
	repo.EXPECT().Delete(mock.Anything, commentCaller, commentTeamID, memoryID).Return(nil).Once()
	commentRepo.EXPECT().DeleteByResource(mock.Anything, commentTeamID, models.CommentResourceTypeMemory, memoryID).
		Return(int64(1), nil).Once()

	svc := NewMemoryService(repo, nil, authzForRole(t, models.TeamMemberRoleMember), nil, logger, nil, commentRepo)
	require.NoError(t, svc.DeleteMemory(commentCaller, commentTeamID, memoryID))
}

func TestPromptService_Delete_CascadesComments(t *testing.T) {
	const promptID = "prompt-1"
	logger, _ := logtest.New()
	repo := mocks.NewMockPromptRepository(t)
	refRepo := mocks.NewMockPromptReferenceRepository(t)
	commentRepo := mocks.NewMockCommentRepository(t)

	repo.EXPECT().GetByID(mock.Anything, commentCaller, commentTeamID, promptID).
		Return(&models.Prompt{ID: promptID, UserID: commentCaller, TeamID: commentTeamID}, nil).Once()
	refRepo.EXPECT().HasDependents(mock.Anything, promptID).Return(false, nil).Once()
	repo.EXPECT().Delete(mock.Anything, commentCaller, commentTeamID, promptID).Return(nil).Once()
	commentRepo.EXPECT().DeleteByResource(mock.Anything, commentTeamID, models.CommentResourceTypePrompt, promptID).
		Return(int64(1), nil).Once()

	svc := NewPromptService(PromptServiceDeps{
		Repo:              repo,
		RefRepo:           refRepo,
		UserRepo:          nil,
		ProjectRepo:       nil,
		TeamService:       nil,
		Authz:             authzForRole(t, models.TeamMemberRoleMember),
		EventManager:      nil,
		Logger:            logger,
		ContentVersionSvc: nil,
		CommentRepo:       commentRepo,
	})
	require.NoError(t, svc.DeletePrompt(commentCaller, commentTeamID, promptID))
}

func TestTeamService_RemoveTeamMember_CascadesComments(t *testing.T) {
	logger, _ := logtest.New()
	teamRepo := mocks.NewMockTeamRepository(t)
	teamMemberRepo := mocks.NewMockTeamMemberRepository(t)
	commentRepo := mocks.NewMockCommentRepository(t)

	// The caller (owner) removes commentOther from the team.
	teamRepo.EXPECT().GetByID(mock.Anything, commentTeamID).
		Return(&models.Team{ID: commentTeamID, OwnerID: commentCaller}, nil).Once()
	teamMemberRepo.EXPECT().Delete(mock.Anything, commentTeamID, commentOther).Return(nil).Once()
	commentRepo.EXPECT().DeleteByUser(mock.Anything, commentTeamID, commentOther).Return(int64(3), nil).Once()

	svc := NewTeamService(teamRepo, teamMemberRepo, nil, authzForRole(t, models.TeamMemberRoleOwner), logger, commentRepo)
	require.NoError(t, svc.RemoveTeamMember(context.Background(), commentCaller, commentTeamID, commentOther))
}
