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

// The resource permission matrix from epic #220 §4, asserted per role for
// memories, artifacts and blueprints:
//
//	| Action                              | Owner | Admin | Member |
//	|-------------------------------------|-------|-------|--------|
//	| Create resources                    |  yes  |  yes  |  yes   |
//	| Update ANY resource (incl. others') |  yes  |  yes  |  yes   |
//	| Delete OWN resources                |  yes  |  yes  |  yes   |
//	| Delete OTHERS' resources            |  yes  |  yes  |   no   |
//
// Update is unchanged for these three domains (they already allowed any member);
// the delete own/any split is the only behavior movement, and it tightens.
//
// These drive a REAL AuthorizationService over a mocked TeamMemberRepository, so
// the decision under test is the shipped matrix rather than a restatement of it.

const (
	resRBACTeamID = "team-rbac"
	resRBACCaller = "user-caller"
	resRBACOther  = "user-other"
)

// memberRepoForRole stubs the caller's membership lookup AuthorizationService performs.
func memberRepoForRole(t *testing.T, role models.TeamMemberRole) *mocks.MockTeamMemberRepository {
	t.Helper()
	repo := mocks.NewMockTeamMemberRepository(t)
	if role == "" {
		repo.EXPECT().GetByTeamAndUser(mock.Anything, resRBACTeamID, resRBACCaller).
			Return(nil, repositories.ErrTeamMemberNotFound).Maybe()
	} else {
		repo.EXPECT().GetByTeamAndUser(mock.Anything, resRBACTeamID, resRBACCaller).
			Return(&models.TeamMember{TeamID: resRBACTeamID, UserID: resRBACCaller, Role: role}, nil).Maybe()
	}
	return repo
}

func authzForRole(t *testing.T, role models.TeamMemberRole) AuthorizationServiceInterface {
	t.Helper()
	logger, _ := logtest.New()
	return NewAuthorizationService(memberRepoForRole(t, role), logger)
}

// deleteOwnVsAnyCases is the shared delete matrix for all three domains.
var deleteOwnVsAnyCases = []struct {
	name    string
	role    models.TeamMemberRole
	ownerID string
	allowed bool
}{
	{"member deletes own", models.TeamMemberRoleMember, resRBACCaller, true},
	{"member cannot delete another's", models.TeamMemberRoleMember, resRBACOther, false},
	{"admin deletes another's", models.TeamMemberRoleAdmin, resRBACOther, true},
	{"owner deletes another's", models.TeamMemberRoleOwner, resRBACOther, true},
	{"non-member cannot delete own", "", resRBACCaller, false},
}

func TestMemoryService_DeleteMemory_OwnVsAny(t *testing.T) {
	const memoryID = "memory-1"

	for _, tc := range deleteOwnVsAnyCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewMockMemoryRepository(t)
			logger, _ := logtest.New()
			repo.EXPECT().GetByID(mock.Anything, resRBACCaller, resRBACTeamID, memoryID).
				Return(&models.Memory{ID: memoryID, UserID: tc.ownerID, TeamID: resRBACTeamID}, nil).Once()
			if tc.allowed {
				repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, memoryID).Return(nil).Once()
			}

			svc := NewMemoryService(repo, nil, authzForRole(t, tc.role), nil, logger, nil)
			err := svc.DeleteMemory(resRBACCaller, resRBACTeamID, memoryID)

			if tc.allowed {
				require.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, ErrPermissionDenied)
			repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestMemoryService_CreateMemory_NonMemberDenied(t *testing.T) {
	repo := mocks.NewMockMemoryRepository(t)
	logger, _ := logtest.New()

	svc := NewMemoryService(repo, nil, authzForRole(t, ""), nil, logger, nil)
	_, err := svc.CreateMemory(resRBACCaller, resRBACTeamID, &models.CreateMemoryRequest{Text: "t"})

	assert.ErrorIs(t, err, ErrPermissionDenied)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// TestMemoryService_UpdateMemory_AnyMemberMayUpdateAnothers: update is unchanged
// for this domain — any member already could, and still can.
func TestMemoryService_UpdateMemory_AnyMemberMayUpdateAnothers(t *testing.T) {
	const memoryID = "memory-1"
	repo := mocks.NewMockMemoryRepository(t)
	logger, _ := logtest.New()

	repo.EXPECT().GetByID(mock.Anything, resRBACCaller, resRBACTeamID, memoryID).
		Return(&models.Memory{ID: memoryID, UserID: resRBACOther, TeamID: resRBACTeamID}, nil).Once()
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()

	text := "edited by a plain member"
	svc := NewMemoryService(repo, nil, authzForRole(t, models.TeamMemberRoleMember), nil, logger, nil)
	_, err := svc.UpdateMemory(resRBACCaller, resRBACTeamID, memoryID, &models.UpdateMemoryRequest{Text: &text})

	assert.NoError(t, err)
}

func TestArtifactService_Delete_OwnVsAny(t *testing.T) {
	const (
		projectID  = "project-1"
		slug       = "a"
		artifactID = "artifact-1"
	)

	for _, tc := range deleteOwnVsAnyCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewMockArtifactRepository(t)
			logger, _ := logtest.New()
			// Resolution is now team-scoped (by membership), so delete.any can reach a
			// non-creator member's artifact — the owner-scoped CrossTeam getter used to 404
			// first, making the any branch dead (#258).
			repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, resRBACCaller, resRBACTeamID, projectID, slug).
				Return(&models.Artifact{
					ID: artifactID, UserID: tc.ownerID, TeamID: resRBACTeamID, Slug: slug,
				}, nil).Once()
			if tc.allowed {
				repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, artifactID).Return(nil).Once()
			}

			svc := NewArtifactService(repo, nil, authzForRole(t, tc.role), nil, nil, logger, nil)
			err := svc.DeleteArtifactByProjectIDAndSlug(resRBACCaller, resRBACTeamID, projectID, slug)

			if tc.allowed {
				require.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, ErrPermissionDenied)
			repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestBlueprintService_Delete_OwnVsAny(t *testing.T) {
	const (
		projectID   = "project-1"
		slug        = "b"
		blueprintID = "blueprint-1"
	)

	for _, tc := range deleteOwnVsAnyCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewMockBlueprintRepository(t)
			logger, _ := logtest.New()
			// Resolution is now team-scoped (by membership), so delete.any can reach a
			// non-creator member's blueprint — the owner-scoped CrossTeam getter used to 404
			// first, making the any branch dead (#258).
			repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, resRBACCaller, resRBACTeamID, projectID, slug).
				Return(&models.Blueprint{
					ID: blueprintID, UserID: tc.ownerID, TeamID: resRBACTeamID, Slug: slug,
				}, nil).Once()
			if tc.allowed {
				repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, blueprintID).Return(nil).Once()
			}

			svc := NewBlueprintService(repo, nil, authzForRole(t, tc.role), nil, nil, logger, nil)
			err := svc.DeleteBlueprintByProjectIDAndSlug(resRBACCaller, resRBACTeamID, projectID, slug)

			if tc.allowed {
				require.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, ErrPermissionDenied)
			repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}
