package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// The resource permission matrix from epic #220 §4, asserted per role for prompts:
//
//	| Action                              | Owner | Admin | Member |
//	|-------------------------------------|-------|-------|--------|
//	| Create resources                    |  yes  |  yes  |  yes   |
//	| Update ANY resource (incl. others') |  yes  |  yes  |  yes   |  <- D1
//	| Delete OWN resources                |  yes  |  yes  |  yes   |
//	| Delete OTHERS' resources            |  yes  |  yes  |   no   |
//
// These drive a REAL AuthorizationService over a mocked TeamMemberRepository, so
// the decision under test is the shipped matrix, not a restatement of it.

const (
	promptRBACTeamID   = "team-rbac"
	promptRBACCaller   = "user-caller"
	promptRBACOther    = "user-other"
	promptRBACPromptID = "prompt-1"
)

func promptServiceForRole(
	t *testing.T, repo repositories.PromptRepository, refRepo repositories.PromptReferenceRepository,
	role models.TeamMemberRole,
) *PromptService {
	t.Helper()
	logger, _ := logtest.New()

	projectRepo := mocks.NewMockProjectRepository(t)
	projectRepo.EXPECT().GetByID(mock.Anything, promptRBACCaller, mock.Anything).
		Return(&models.Project{ID: "project-1", UserID: promptRBACCaller}, nil).Maybe()

	memberRepo := mocks.NewMockTeamMemberRepository(t)
	if role == "" {
		memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, promptRBACTeamID, promptRBACCaller).
			Return(nil, repositories.ErrTeamMemberNotFound).Maybe()
	} else {
		memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, promptRBACTeamID, promptRBACCaller).
			Return(&models.TeamMember{
				TeamID: promptRBACTeamID, UserID: promptRBACCaller, Role: role,
			}, nil).Maybe()
	}

	return NewPromptService(
		repo, refRepo, nil, projectRepo, nil,
		NewAuthorizationService(memberRepo, logger), nil, logger, nil,
		nil)
}

func promptOwnedBy(ownerID string) *models.Prompt {
	return &models.Prompt{
		ID:     promptRBACPromptID,
		Slug:   "p",
		Name:   "P",
		Body:   "b",
		UserID: ownerID,
		TeamID: promptRBACTeamID,
	}
}

func TestPromptService_CreatePrompt_RoleMatrix(t *testing.T) {
	// Every role that IS a member may create; a non-member may not.
	for _, tc := range []struct {
		role    models.TeamMemberRole
		allowed bool
	}{
		{models.TeamMemberRoleOwner, true},
		{models.TeamMemberRoleAdmin, true},
		{models.TeamMemberRoleMember, true},
		{"", false},
	} {
		t.Run(roleName(tc.role), func(t *testing.T) {
			repo := mocks.NewMockPromptRepository(t)
			refRepo := mocks.NewMockPromptReferenceRepository(t)
			if tc.allowed {
				repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
				// Creating also rebuilds the prompt's @slug references.
				refRepo.EXPECT().DeleteByPromptID(mock.Anything, mock.Anything).Return(nil).Maybe()
				refRepo.EXPECT().CreateBatch(mock.Anything, mock.Anything).Return(nil).Maybe()
			}

			svc := promptServiceForRole(t, repo, refRepo, tc.role)
			_, err := svc.CreatePrompt(promptRBACCaller, promptRBACTeamID, &models.CreatePromptRequest{
				Name: "P", Slug: "p", Body: "b", ProjectID: "project-1",
			})

			if tc.allowed {
				require.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, ErrPermissionDenied)
			repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		})
	}
}

// TestPromptService_UpdatePrompt_AnyMemberMayUpdateAnothers is decision D1: update
// is uniform, so a plain member may edit a prompt someone else created.
func TestPromptService_UpdatePrompt_AnyMemberMayUpdateAnothers(t *testing.T) {
	repo := mocks.NewMockPromptRepository(t)
	repo.EXPECT().GetByID(mock.Anything, promptRBACCaller, promptRBACTeamID, promptRBACPromptID).
		Return(promptOwnedBy(promptRBACOther), nil).Once()
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()

	refRepo := mocks.NewMockPromptReferenceRepository(t)
	refRepo.EXPECT().DeleteByPromptID(mock.Anything, mock.Anything).Return(nil).Maybe()
	refRepo.EXPECT().CreateBatch(mock.Anything, mock.Anything).Return(nil).Maybe()

	body := "edited by a plain member"
	svc := promptServiceForRole(t, repo, refRepo, models.TeamMemberRoleMember)
	_, err := svc.UpdatePrompt(promptRBACCaller, promptRBACTeamID, promptRBACPromptID,
		&models.UpdatePromptRequest{Body: &body})

	assert.NoError(t, err)
}

func TestPromptService_UpdatePrompt_NonMemberDenied(t *testing.T) {
	repo := mocks.NewMockPromptRepository(t)
	repo.EXPECT().GetByID(mock.Anything, promptRBACCaller, promptRBACTeamID, promptRBACPromptID).
		Return(promptOwnedBy(promptRBACOther), nil).Once()

	body := "edited"
	svc := promptServiceForRole(t, repo, mocks.NewMockPromptReferenceRepository(t), "")
	_, err := svc.UpdatePrompt(promptRBACCaller, promptRBACTeamID, promptRBACPromptID,
		&models.UpdatePromptRequest{Body: &body})

	assert.ErrorIs(t, err, ErrPermissionDenied)
	repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestPromptService_DeletePrompt_OwnVsAny(t *testing.T) {
	tests := []struct {
		name    string
		role    models.TeamMemberRole
		ownerID string
		allowed bool
	}{
		{"member deletes own", models.TeamMemberRoleMember, promptRBACCaller, true},
		{"member cannot delete another's", models.TeamMemberRoleMember, promptRBACOther, false},
		{"admin deletes another's", models.TeamMemberRoleAdmin, promptRBACOther, true},
		{"owner deletes another's", models.TeamMemberRoleOwner, promptRBACOther, true},
		{"non-member cannot delete own", "", promptRBACCaller, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewMockPromptRepository(t)
			repo.EXPECT().GetByID(mock.Anything, promptRBACCaller, promptRBACTeamID, promptRBACPromptID).
				Return(promptOwnedBy(tc.ownerID), nil).Once()

			refRepo := mocks.NewMockPromptReferenceRepository(t)
			if tc.allowed {
				refRepo.EXPECT().HasDependents(mock.Anything, promptRBACPromptID).Return(false, nil).Once()
				repo.EXPECT().Delete(mock.Anything, promptRBACCaller, promptRBACTeamID, promptRBACPromptID).
					Return(nil).Once()
			}

			svc := promptServiceForRole(t, repo, refRepo, tc.role)
			err := svc.DeletePrompt(promptRBACCaller, promptRBACTeamID, promptRBACPromptID)

			if tc.allowed {
				require.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, ErrPermissionDenied)
			repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			// Authorization precedes the dependents check.
			refRepo.AssertNotCalled(t, "HasDependents", mock.Anything, mock.Anything)
		})
	}
}
