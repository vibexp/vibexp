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

// The project permission matrix from epic #220 §4, asserted per role:
//
//	| Action                            | Owner | Admin | Member |
//	|-----------------------------------|-------|-------|--------|
//	| Create / update / delete projects |  yes  |  yes  |   no   |
//
// These drive a REAL AuthorizationService over a mocked TeamMemberRepository, so
// the role→permission decision under test is the shipped matrix rather than a
// restatement of it in a mock.

const (
	projRBACTeamID = "team-rbac"
	projRBACUserID = "user-caller"
	projRBACSlug   = "test-project"
)

func projectServiceForRole(
	t *testing.T, repo repositories.ProjectRepository, role models.TeamMemberRole,
) *ProjectService {
	t.Helper()
	logger, _ := logtest.New()

	memberRepo := mocks.NewMockTeamMemberRepository(t)
	if role == "" {
		// "" means: not a member of the team at all.
		memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, projRBACTeamID, projRBACUserID).
			Return(nil, repositories.ErrTeamMemberNotFound).Maybe()
	} else {
		memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, projRBACTeamID, projRBACUserID).
			Return(&models.TeamMember{TeamID: projRBACTeamID, UserID: projRBACUserID, Role: role}, nil).Maybe()
	}

	return NewProjectService(repo, nil, NewAuthorizationService(memberRepo, logger), nil, logger)
}

// projectRoleCases is the matrix: owner/admin allowed, member and non-member denied.
//
// On the "" (non-member) case: for update and delete this asserts defense in
// depth rather than the live response. Those paths fetch through
// GetProjectBySlug, whose repository predicate is tenancy-scoped, so a real
// non-member is stopped there with not-found and sees a 404 — authz is never
// reached. The case stubs the fetch as succeeding to prove that authz would
// still deny if that scoping were ever widened or bypassed. For create there is
// no prior fetch, so the denial here IS the live behavior.
var projectRoleCases = []struct {
	role    models.TeamMemberRole
	allowed bool
}{
	{models.TeamMemberRoleOwner, true},
	{models.TeamMemberRoleAdmin, true},
	{models.TeamMemberRoleMember, false},
	{"", false}, // not a member of the team at all
}

func roleName(role models.TeamMemberRole) string {
	if role == "" {
		return "non-member"
	}
	return string(role)
}

func TestProjectService_CreateProject_RoleMatrix(t *testing.T) {
	for _, tc := range projectRoleCases {
		t.Run(roleName(tc.role), func(t *testing.T) {
			repo := mocks.NewMockProjectRepository(t)
			if tc.allowed {
				repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
			}

			svc := projectServiceForRole(t, repo, tc.role)
			project, err := svc.CreateProject(projRBACUserID, projRBACTeamID, &models.CreateProjectRequest{
				Name: "New Project",
				Slug: "new-project",
			})

			if tc.allowed {
				require.NoError(t, err)
				assert.NotNil(t, project)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPermissionDenied)
			assert.Nil(t, project)
			// Denied means nothing was written.
			repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		})
	}
}

func TestProjectService_UpdateProject_RoleMatrix(t *testing.T) {
	for _, tc := range projectRoleCases {
		t.Run(roleName(tc.role), func(t *testing.T) {
			repo := mocks.NewMockProjectRepository(t)
			repo.EXPECT().GetBySlug(mock.Anything, projRBACTeamID, projRBACUserID, projRBACSlug).
				Return(&models.Project{
					ID:     "project-1",
					UserID: "user-creator",
					TeamID: projRBACTeamID,
					Slug:   projRBACSlug,
				}, nil).Once()
			if tc.allowed {
				repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()
			}

			name := "Renamed"
			svc := projectServiceForRole(t, repo, tc.role)
			project, err := svc.UpdateProject(projRBACTeamID, projRBACUserID, projRBACSlug,
				&models.UpdateProjectRequest{Name: &name})

			if tc.allowed {
				require.NoError(t, err)
				assert.NotNil(t, project)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPermissionDenied)
			assert.Nil(t, project)
			repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
		})
	}
}

// TestProjectService_UpdateProject_MemberCannotUpdateOwnProject is the sharp
// edge of decision D2: creating a project no longer buys you the right to
// update it, because members hold no project permission at all.
func TestProjectService_UpdateProject_MemberCannotUpdateOwnProject(t *testing.T) {
	repo := mocks.NewMockProjectRepository(t)
	repo.EXPECT().GetBySlug(mock.Anything, projRBACTeamID, projRBACUserID, projRBACSlug).
		Return(&models.Project{
			ID:     "project-1",
			UserID: projRBACUserID, // the caller created it
			TeamID: projRBACTeamID,
			Slug:   projRBACSlug,
		}, nil).Once()

	name := "Renamed"
	svc := projectServiceForRole(t, repo, models.TeamMemberRoleMember)
	project, err := svc.UpdateProject(projRBACTeamID, projRBACUserID, projRBACSlug,
		&models.UpdateProjectRequest{Name: &name})

	assert.ErrorIs(t, err, ErrPermissionDenied)
	assert.Nil(t, project)
	repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestProjectService_DeleteProject_RoleMatrix(t *testing.T) {
	for _, tc := range projectRoleCases {
		t.Run(roleName(tc.role), func(t *testing.T) {
			repo := mocks.NewMockProjectRepository(t)
			repo.EXPECT().GetBySlug(mock.Anything, projRBACTeamID, projRBACUserID, projRBACSlug).
				Return(&models.Project{
					ID:     "project-1",
					UserID: "user-creator",
					TeamID: projRBACTeamID,
					Slug:   projRBACSlug,
				}, nil).Once()
			if tc.allowed {
				// Two projects, so the last-project guard does not fire.
				repo.EXPECT().CountByTeamID(mock.Anything, projRBACTeamID).Return(2, nil).Once()
				repo.EXPECT().Delete(mock.Anything, projRBACTeamID, projRBACUserID, projRBACSlug).
					Return(nil).Once()
			}

			svc := projectServiceForRole(t, repo, tc.role)
			err := svc.DeleteProject(projRBACTeamID, projRBACUserID, projRBACSlug)

			if tc.allowed {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPermissionDenied)
			repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			// Authorization precedes the last-project guard, so an unauthorized
			// caller cannot probe the team's project count.
			repo.AssertNotCalled(t, "CountByTeamID", mock.Anything, mock.Anything)
		})
	}
}

// TestProjectService_DeleteProject_MemberCannotDeleteOwnProject: same D2 edge as
// update — the creator branch that used to live in the DELETE's SQL predicate is
// gone, so a member cannot delete even a project they created.
func TestProjectService_DeleteProject_MemberCannotDeleteOwnProject(t *testing.T) {
	repo := mocks.NewMockProjectRepository(t)
	repo.EXPECT().GetBySlug(mock.Anything, projRBACTeamID, projRBACUserID, projRBACSlug).
		Return(&models.Project{
			ID:     "project-1",
			UserID: projRBACUserID, // the caller created it
			TeamID: projRBACTeamID,
			Slug:   projRBACSlug,
		}, nil).Once()

	svc := projectServiceForRole(t, repo, models.TeamMemberRoleMember)
	err := svc.DeleteProject(projRBACTeamID, projRBACUserID, projRBACSlug)

	assert.ErrorIs(t, err, ErrPermissionDenied)
	repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestProjectService_CreateProject_AuthorizesResolvedTeam pins that the create is
// authorized against the team the project actually lands in. req.TeamID can
// redirect the create away from the URL's team, so authorizing the URL team would
// let a caller create in a team where they are only a member (or not a member).
func TestProjectService_CreateProject_AuthorizesResolvedTeam(t *testing.T) {
	const urlTeamID = "team-url"
	const targetTeamID = projRBACTeamID

	repo := mocks.NewMockProjectRepository(t)
	logger, _ := logtest.New()

	// The caller is a plain member of the team they are redirecting INTO.
	memberRepo := mocks.NewMockTeamMemberRepository(t)
	memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, targetTeamID, projRBACUserID).
		Return(&models.TeamMember{
			TeamID: targetTeamID, UserID: projRBACUserID, Role: models.TeamMemberRoleMember,
		}, nil).Once()

	// teamService validates membership in the requested team.
	teamSvc := &MockTeamService{}
	teamSvc.On("IsUserMemberOfTeam", mock.Anything, projRBACUserID, targetTeamID).Return(true, nil)

	svc := NewProjectService(repo, teamSvc, NewAuthorizationService(memberRepo, logger), nil, logger)

	requested := targetTeamID
	project, err := svc.CreateProject(projRBACUserID, urlTeamID, &models.CreateProjectRequest{
		Name:   "Redirected",
		Slug:   "redirected",
		TeamID: &requested,
	})

	assert.ErrorIs(t, err, ErrPermissionDenied, "must authorize the resolved team, not the URL team")
	assert.Nil(t, project)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}
