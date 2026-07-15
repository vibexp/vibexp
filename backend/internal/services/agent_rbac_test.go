package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repoMocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// The resource permission matrix from epic #220 §4, asserted per role for agents.
// Agents are the contentious case of decision D1: any member may update any
// agent, and agents carry encrypted provider credentials. That is an accepted,
// documented product trade-off — these tests pin it so it cannot drift silently
// in either direction.

const (
	agentRBACTeamID  = "team-rbac"
	agentRBACCaller  = "user-caller"
	agentRBACOther   = "user-other"
	agentRBACAgentID = "agent-1"
)

func agentServiceForRole(
	t *testing.T, agentRepo repositories.AgentRepository, role models.TeamMemberRole,
) *AgentService {
	t.Helper()
	logger, _ := logtest.New()

	memberRepo := repoMocks.NewMockTeamMemberRepository(t)
	if role == "" {
		memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, agentRBACTeamID, agentRBACCaller).
			Return(nil, repositories.ErrTeamMemberNotFound).Maybe()
	} else {
		memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, agentRBACTeamID, agentRBACCaller).
			Return(&models.TeamMember{
				TeamID: agentRBACTeamID, UserID: agentRBACCaller, Role: role,
			}, nil).Maybe()
	}

	encryptionSvc, err := NewEncryptionService("test-encryption-key-32-bytes1234")
	require.NoError(t, err)

	return NewAgentServiceWithCardFetcher(
		agentRepo, repoMocks.NewMockAgentExecutionRepository(t), &MockAgentCardFetcher{},
		encryptionSvc, nil, NewAuthorizationService(memberRepo, logger), logger,
	)
}

func agentOwnedBy(ownerID string) *models.Agent {
	cardURL := "https://example.com/card"
	return &models.Agent{
		ID:      agentRBACAgentID,
		Name:    "A",
		UserID:  ownerID,
		TeamID:  agentRBACTeamID,
		Status:  "active",
		CardURL: &cardURL,
	}
}

// TestAgentService_UpdateAgent_AnyMemberMayUpdateAnothers is decision D1 at its
// sharpest: a plain member may edit an agent another member created, credentials
// and all.
func TestAgentService_UpdateAgent_AnyMemberMayUpdateAnothers(t *testing.T) {
	repo := repoMocks.NewMockAgentRepository(t)
	repo.EXPECT().GetByID(mock.Anything, agentRBACCaller, agentRBACTeamID, agentRBACAgentID).
		Return(agentOwnedBy(agentRBACOther), nil).Once()
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()

	name := "renamed by a plain member"
	svc := agentServiceForRole(t, repo, models.TeamMemberRoleMember)
	_, err := svc.UpdateAgent(context.Background(), agentRBACCaller, agentRBACTeamID, agentRBACAgentID,
		&models.UpdateAgentRequest{Name: &name})

	assert.NoError(t, err)
}

func TestAgentService_UpdateAgent_NonMemberDenied(t *testing.T) {
	repo := repoMocks.NewMockAgentRepository(t)
	repo.EXPECT().GetByID(mock.Anything, agentRBACCaller, agentRBACTeamID, agentRBACAgentID).
		Return(agentOwnedBy(agentRBACOther), nil).Once()

	name := "renamed"
	svc := agentServiceForRole(t, repo, "")
	_, err := svc.UpdateAgent(context.Background(), agentRBACCaller, agentRBACTeamID, agentRBACAgentID,
		&models.UpdateAgentRequest{Name: &name})

	assert.ErrorIs(t, err, ErrPermissionDenied)
	repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestAgentService_CreateAgent_NonMemberDenied(t *testing.T) {
	repo := repoMocks.NewMockAgentRepository(t)

	svc := agentServiceForRole(t, repo, "")
	_, err := svc.CreateAgent(context.Background(), agentRBACCaller, agentRBACTeamID,
		&models.CreateAgentRequest{Name: "A", CardURL: "https://example.com/card"})

	assert.ErrorIs(t, err, ErrPermissionDenied)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAgentService_DeleteAgent_OwnVsAny(t *testing.T) {
	tests := []struct {
		name    string
		role    models.TeamMemberRole
		ownerID string
		allowed bool
	}{
		{"member deletes own", models.TeamMemberRoleMember, agentRBACCaller, true},
		{"member cannot delete another's", models.TeamMemberRoleMember, agentRBACOther, false},
		{"admin deletes another's", models.TeamMemberRoleAdmin, agentRBACOther, true},
		{"owner deletes another's", models.TeamMemberRoleOwner, agentRBACOther, true},
		{"non-member cannot delete own", "", agentRBACCaller, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := repoMocks.NewMockAgentRepository(t)
			repo.EXPECT().GetByID(mock.Anything, agentRBACCaller, agentRBACTeamID, agentRBACAgentID).
				Return(agentOwnedBy(tc.ownerID), nil).Once()
			if tc.allowed {
				repo.EXPECT().Delete(mock.Anything, agentRBACCaller, agentRBACTeamID, agentRBACAgentID).
					Return(nil).Once()
			}

			svc := agentServiceForRole(t, repo, tc.role)
			err := svc.DeleteAgent(context.Background(), agentRBACCaller, agentRBACTeamID, agentRBACAgentID)

			if tc.allowed {
				require.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, ErrPermissionDenied)
			repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

// TestAgentService_UpdateAgentCredentials_IsGated covers the fourth mutating
// method, which #236's scope did not list: writing credentials is an update, so
// it carries ResourceUpdateAny. Leaving it ungated would be a hole around the
// exact field D1 is contentious about.
func TestAgentService_UpdateAgentCredentials_IsGated(t *testing.T) {
	repo := repoMocks.NewMockAgentRepository(t)
	repo.EXPECT().GetByID(mock.Anything, agentRBACCaller, agentRBACTeamID, agentRBACAgentID).
		Return(agentOwnedBy(agentRBACOther), nil).Once()

	svc := agentServiceForRole(t, repo, "")
	err := svc.UpdateAgentCredentials(context.Background(), agentRBACCaller, agentRBACTeamID, agentRBACAgentID,
		&models.UpdateAgentCredentialsRequest{})

	assert.ErrorIs(t, err, ErrPermissionDenied)
	repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}
