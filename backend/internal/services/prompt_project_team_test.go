package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// A prompt may use any project of its own team, whoever created the project,
// and no project of another team (#1540). The project repository is
// tenancy-scoped, so it also returns projects of the caller's other teams; the
// team match is what keeps a prompt in its team.

const (
	promptProjectTeammate     = "project-of-teammate"
	promptProjectOtherTeam    = "project-of-other-team"
	promptProjectMissing      = "project-missing"
	promptProjectOtherTeamID  = "team-other"
	promptProjectTeammateUser = "user-teammate"
)

func promptServiceWithProjects(t *testing.T, repo repositories.PromptRepository) *PromptService {
	t.Helper()
	logger, _ := logtest.New()

	projectRepo := mocks.NewMockProjectRepository(t)
	projectRepo.EXPECT().GetByID(mock.Anything, promptRBACCaller, promptProjectTeammate).
		Return(&models.Project{
			ID: promptProjectTeammate, TeamID: promptRBACTeamID, UserID: promptProjectTeammateUser,
		}, nil).Maybe()
	projectRepo.EXPECT().GetByID(mock.Anything, promptRBACCaller, promptProjectOtherTeam).
		Return(&models.Project{
			ID: promptProjectOtherTeam, TeamID: promptProjectOtherTeamID, UserID: promptRBACCaller,
		}, nil).Maybe()
	projectRepo.EXPECT().GetByID(mock.Anything, promptRBACCaller, promptProjectMissing).
		Return(nil, fmt.Errorf("%w: id=%s", repositories.ErrProjectNotFoundForRepo, promptProjectMissing)).Maybe()

	memberRepo := mocks.NewMockTeamMemberRepository(t)
	memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, promptRBACTeamID, promptRBACCaller).
		Return(&models.TeamMember{
			TeamID: promptRBACTeamID, UserID: promptRBACCaller, Role: models.TeamMemberRoleMember,
		}, nil).Maybe()

	refRepo := mocks.NewMockPromptReferenceRepository(t)
	refRepo.EXPECT().DeleteByPromptID(mock.Anything, mock.Anything).Return(nil).Maybe()
	refRepo.EXPECT().CreateBatch(mock.Anything, mock.Anything).Return(nil).Maybe()

	return NewPromptService(PromptServiceDeps{
		Repo:        repo,
		RefRepo:     refRepo,
		ProjectRepo: projectRepo,
		Authz:       NewAuthorizationService(memberRepo, logger),
		Logger:      logger,
	})
}

func TestPromptService_CreatePrompt_ProjectTeam(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		wantErr   string
	}{
		{"member uses a teammate's project", promptProjectTeammate, ""},
		{"project of another team", promptProjectOtherTeam, "project does not belong to the specified team"},
		{"project not found", promptProjectMissing, "project not found"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewMockPromptRepository(t)
			if tc.wantErr == "" {
				repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(p *models.Prompt) bool {
					return p.ProjectID == tc.projectID && p.TeamID == promptRBACTeamID
				})).Return(nil).Once()
			}

			svc := promptServiceWithProjects(t, repo)
			_, err := svc.CreatePrompt(context.Background(), promptRBACCaller, promptRBACTeamID,
				&models.CreatePromptRequest{Name: "P", Slug: "p", Body: "b", ProjectID: tc.projectID})

			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		})
	}
}

func TestPromptService_UpdatePrompt_ProjectTeam(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		wantErr   string
	}{
		{"member moves a prompt into a teammate's project", promptProjectTeammate, ""},
		{"project of another team", promptProjectOtherTeam, "project does not belong to the specified team"},
		{"project not found", promptProjectMissing, "project not found"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewMockPromptRepository(t)
			repo.EXPECT().GetByID(mock.Anything, promptRBACCaller, promptRBACTeamID, promptRBACPromptID).
				Return(promptOwnedBy(promptRBACOther), nil).Once()
			if tc.wantErr == "" {
				repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(p *models.Prompt) bool {
					return p.ProjectID == tc.projectID && p.TeamID == promptRBACTeamID
				})).Return(nil).Once()
			}

			projectID := tc.projectID
			svc := promptServiceWithProjects(t, repo)
			_, err := svc.UpdatePrompt(context.Background(), promptRBACCaller, promptRBACTeamID, promptRBACPromptID,
				&models.UpdatePromptRequest{ProjectID: &projectID})

			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
		})
	}
}
