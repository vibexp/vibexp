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
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

const (
	relTeamID     = resRBACTeamID
	relCaller     = resRBACCaller
	relRelationID = "relation-1"
	relProject    = "project-1"
	relFromID     = "art-1"
	relToID       = "bp-1"
)

func newRelationService(
	t *testing.T, repo repositories.RelationRepository, team TeamServiceInterface, authz AuthorizationServiceInterface,
) *RelationService {
	t.Helper()
	logger, _ := logtest.New()
	return NewRelationService(repo, team, authz, logger)
}

// validRelationReq is a governed-by edge (artifact -> blueprint), human origin,
// which the matrix accepts.
func validRelationReq() *models.CreateRelationRequest {
	return &models.CreateRelationRequest{
		FromType:     models.RelationResourceTypeArtifact,
		FromID:       relFromID,
		ToType:       models.RelationResourceTypeBlueprint,
		ToID:         relToID,
		RelationType: models.RelationTypeGovernedBy,
		Origin:       models.RelationOriginHuman,
	}
}

// Validation that happens before any repo/authz work: bad enums, self-links,
// and matrix violations each get a distinct error and never touch the repo.
func TestRelationService_Create_RejectsBeforeRepo(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(r *models.CreateRelationRequest)
		wantIs error
	}{
		{"invalid from type", func(r *models.CreateRelationRequest) { r.FromType = "project" }, nil},
		{"invalid relation type", func(r *models.CreateRelationRequest) { r.RelationType = "relates-to" }, nil},
		{"invalid origin", func(r *models.CreateRelationRequest) { r.Origin = "system" }, nil},
		{"self link", func(r *models.CreateRelationRequest) {
			r.RelationType = models.RelationTypeSupersedes
			r.ToType = models.RelationResourceTypeArtifact
			r.ToID = r.FromID
		}, ErrRelationSelfLink},
		{"governed-by object must be blueprint", func(r *models.CreateRelationRequest) {
			r.ToType = models.RelationResourceTypePrompt
		}, ErrRelationInvalidType},
		{"built-from object must be prompt", func(r *models.CreateRelationRequest) {
			r.RelationType = models.RelationTypeBuiltFrom
			r.ToType = models.RelationResourceTypeBlueprint
		}, ErrRelationInvalidType},
		{"supersedes must be same type", func(r *models.CreateRelationRequest) {
			r.RelationType = models.RelationTypeSupersedes
			r.ToType = models.RelationResourceTypePrompt
		}, ErrRelationInvalidType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewMockRelationRepository(t)
			svc := newRelationService(t, repo, nil, allowAllAuthz{})

			req := validRelationReq()
			tc.mutate(req)
			_, err := svc.Create(context.Background(), relCaller, relTeamID, req)

			require.Error(t, err)
			if tc.wantIs != nil {
				assert.ErrorIs(t, err, tc.wantIs)
			}
			repo.AssertNotCalled(t, "ResourceProjectID", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		})
	}
}

func TestRelationService_Create_NonMemberDenied(t *testing.T) {
	repo := mocks.NewMockRelationRepository(t)
	svc := newRelationService(t, repo, nil, authzForRole(t, ""))

	_, err := svc.Create(context.Background(), relCaller, relTeamID, validRelationReq())

	assert.ErrorIs(t, err, ErrPermissionDenied)
	repo.AssertNotCalled(t, "ResourceProjectID", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestRelationService_Create_EndpointExistenceAndProject(t *testing.T) {
	t.Run("from resource missing", func(t *testing.T) {
		repo := mocks.NewMockRelationRepository(t)
		repo.EXPECT().ResourceProjectID(mock.Anything, relTeamID, models.RelationResourceTypeArtifact, relFromID).
			Return("", false, nil).Once()
		svc := newRelationService(t, repo, nil, allowAllAuthz{})

		_, err := svc.Create(context.Background(), relCaller, relTeamID, validRelationReq())

		assert.ErrorIs(t, err, ErrRelationResourceNotFound)
		repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("to resource missing", func(t *testing.T) {
		repo := mocks.NewMockRelationRepository(t)
		repo.EXPECT().ResourceProjectID(mock.Anything, relTeamID, models.RelationResourceTypeArtifact, relFromID).
			Return(relProject, true, nil).Once()
		repo.EXPECT().ResourceProjectID(mock.Anything, relTeamID, models.RelationResourceTypeBlueprint, relToID).
			Return("", false, nil).Once()
		svc := newRelationService(t, repo, nil, allowAllAuthz{})

		_, err := svc.Create(context.Background(), relCaller, relTeamID, validRelationReq())

		assert.ErrorIs(t, err, ErrRelationResourceNotFound)
		repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("endpoints in different projects", func(t *testing.T) {
		repo := mocks.NewMockRelationRepository(t)
		repo.EXPECT().ResourceProjectID(mock.Anything, relTeamID, models.RelationResourceTypeArtifact, relFromID).
			Return(relProject, true, nil).Once()
		repo.EXPECT().ResourceProjectID(mock.Anything, relTeamID, models.RelationResourceTypeBlueprint, relToID).
			Return("project-2", true, nil).Once()
		svc := newRelationService(t, repo, nil, allowAllAuthz{})

		_, err := svc.Create(context.Background(), relCaller, relTeamID, validRelationReq())

		assert.ErrorIs(t, err, ErrRelationCrossProject)
		repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})
}

// The tiered-trust initial status must be applied at the service boundary, and
// the persisted edge must carry the resolved project id and the caller as
// created_by.
func TestRelationService_Create_TieredStatusAndPersistedFields(t *testing.T) {
	cases := []struct {
		name         string
		relationType string
		fromType     string
		toType       string
		origin       string
		wantStatus   string
	}{
		{"human governed-by -> confirmed", models.RelationTypeGovernedBy,
			models.RelationResourceTypeArtifact, models.RelationResourceTypeBlueprint, models.RelationOriginHuman, models.RelationStatusConfirmed},
		{"ai governed-by -> suggested", models.RelationTypeGovernedBy,
			models.RelationResourceTypeArtifact, models.RelationResourceTypeBlueprint, models.RelationOriginAI, models.RelationStatusSuggested},
		{"ai built-from -> confirmed", models.RelationTypeBuiltFrom,
			models.RelationResourceTypeArtifact, models.RelationResourceTypePrompt, models.RelationOriginAI, models.RelationStatusConfirmed},
		{"ai supersedes -> suggested", models.RelationTypeSupersedes,
			models.RelationResourceTypePrompt, models.RelationResourceTypePrompt, models.RelationOriginAI, models.RelationStatusSuggested},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewMockRelationRepository(t)
			repo.EXPECT().ResourceProjectID(mock.Anything, relTeamID, tc.fromType, relFromID).
				Return(relProject, true, nil).Once()
			repo.EXPECT().ResourceProjectID(mock.Anything, relTeamID, tc.toType, relToID).
				Return(relProject, true, nil).Once()
			repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(r *models.Relation) bool {
				return r.TeamID == relTeamID && r.ProjectID == relProject &&
					r.Status == tc.wantStatus && r.Origin == tc.origin &&
					r.CreatedBy != nil && *r.CreatedBy == relCaller
			})).RunAndReturn(func(_ context.Context, r *models.Relation) (*models.Relation, error) {
				r.ID = relRelationID
				return r, nil
			}).Once()
			svc := newRelationService(t, repo, nil, allowAllAuthz{})

			out, err := svc.Create(context.Background(), relCaller, relTeamID, &models.CreateRelationRequest{
				FromType: tc.fromType, FromID: relFromID, ToType: tc.toType, ToID: relToID,
				RelationType: tc.relationType, Origin: tc.origin,
			})

			require.NoError(t, err)
			assert.Equal(t, relRelationID, out.ID)
			assert.Equal(t, tc.wantStatus, out.Status)
		})
	}
}

// Create is idempotent: the service returns whatever the repo returns, which for
// a duplicate is the pre-existing row.
func TestRelationService_Create_ReturnsExistingOnDuplicate(t *testing.T) {
	existing := &models.Relation{ID: "pre-existing", TeamID: relTeamID, Status: models.RelationStatusConfirmed}
	repo := mocks.NewMockRelationRepository(t)
	repo.EXPECT().ResourceProjectID(mock.Anything, relTeamID, models.RelationResourceTypeArtifact, relFromID).
		Return(relProject, true, nil).Once()
	repo.EXPECT().ResourceProjectID(mock.Anything, relTeamID, models.RelationResourceTypeBlueprint, relToID).
		Return(relProject, true, nil).Once()
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(existing, nil).Once()
	svc := newRelationService(t, repo, nil, allowAllAuthz{})

	out, err := svc.Create(context.Background(), relCaller, relTeamID, validRelationReq())

	require.NoError(t, err)
	assert.Equal(t, "pre-existing", out.ID)
}

func TestRelationService_Confirm(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRelationRepository(t)
		repo.EXPECT().GetByID(mock.Anything, relTeamID, relRelationID).
			Return(nil, repositories.ErrRelationNotFound).Once()
		svc := newRelationService(t, repo, nil, allowAllAuthz{})

		_, err := svc.Confirm(context.Background(), relCaller, relTeamID, relRelationID)

		assert.ErrorIs(t, err, repositories.ErrRelationNotFound)
		repo.AssertNotCalled(t, "Confirm", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("non-member denied", func(t *testing.T) {
		repo := mocks.NewMockRelationRepository(t)
		repo.EXPECT().GetByID(mock.Anything, relTeamID, relRelationID).
			Return(&models.Relation{ID: relRelationID, TeamID: relTeamID, Status: models.RelationStatusSuggested}, nil).Once()
		svc := newRelationService(t, repo, nil, authzForRole(t, ""))

		_, err := svc.Confirm(context.Background(), relCaller, relTeamID, relRelationID)

		assert.ErrorIs(t, err, ErrPermissionDenied)
		repo.AssertNotCalled(t, "Confirm", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("already confirmed", func(t *testing.T) {
		repo := mocks.NewMockRelationRepository(t)
		repo.EXPECT().GetByID(mock.Anything, relTeamID, relRelationID).
			Return(&models.Relation{ID: relRelationID, TeamID: relTeamID, Status: models.RelationStatusConfirmed}, nil).Once()
		svc := newRelationService(t, repo, nil, allowAllAuthz{})

		_, err := svc.Confirm(context.Background(), relCaller, relTeamID, relRelationID)

		assert.ErrorIs(t, err, ErrRelationAlreadyConfirmed)
		repo.AssertNotCalled(t, "Confirm", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("happy flips suggested and records confirmer", func(t *testing.T) {
		repo := mocks.NewMockRelationRepository(t)
		repo.EXPECT().GetByID(mock.Anything, relTeamID, relRelationID).
			Return(&models.Relation{ID: relRelationID, TeamID: relTeamID, Status: models.RelationStatusSuggested}, nil).Once()
		repo.EXPECT().Confirm(mock.Anything, relTeamID, relRelationID, relCaller).
			Return(&models.Relation{ID: relRelationID, TeamID: relTeamID, Status: models.RelationStatusConfirmed, ConfirmedBy: &[]string{relCaller}[0]}, nil).Once()
		svc := newRelationService(t, repo, nil, authzForRole(t, models.TeamMemberRoleMember))

		out, err := svc.Confirm(context.Background(), relCaller, relTeamID, relRelationID)

		require.NoError(t, err)
		assert.Equal(t, models.RelationStatusConfirmed, out.Status)
		require.NotNil(t, out.ConfirmedBy)
		assert.Equal(t, relCaller, *out.ConfirmedBy)
	})
}

// Delete is own-vs-any, exactly like the other resource domains.
func TestRelationService_Delete_OwnVsAny(t *testing.T) {
	for _, tc := range deleteOwnVsAnyCases {
		t.Run(tc.name, func(t *testing.T) {
			owner := tc.ownerID
			repo := mocks.NewMockRelationRepository(t)
			repo.EXPECT().GetByID(mock.Anything, relTeamID, relRelationID).
				Return(&models.Relation{ID: relRelationID, TeamID: relTeamID, CreatedBy: &owner}, nil).Once()
			if tc.allowed {
				repo.EXPECT().Delete(mock.Anything, relTeamID, relRelationID).Return(nil).Once()
			}
			svc := newRelationService(t, repo, nil, authzForRole(t, tc.role))

			err := svc.Delete(context.Background(), relCaller, relTeamID, relRelationID)

			if tc.allowed {
				require.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, ErrPermissionDenied)
			repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

// An orphaned edge (created_by NULL) is deletable only by Admin/Owner.
func TestRelationService_Delete_OrphanedRequiresDeleteAny(t *testing.T) {
	t.Run("member denied", func(t *testing.T) {
		repo := mocks.NewMockRelationRepository(t)
		repo.EXPECT().GetByID(mock.Anything, relTeamID, relRelationID).
			Return(&models.Relation{ID: relRelationID, TeamID: relTeamID, CreatedBy: nil}, nil).Once()
		svc := newRelationService(t, repo, nil, authzForRole(t, models.TeamMemberRoleMember))

		err := svc.Delete(context.Background(), relCaller, relTeamID, relRelationID)

		assert.ErrorIs(t, err, ErrPermissionDenied)
		repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("admin allowed", func(t *testing.T) {
		repo := mocks.NewMockRelationRepository(t)
		repo.EXPECT().GetByID(mock.Anything, relTeamID, relRelationID).
			Return(&models.Relation{ID: relRelationID, TeamID: relTeamID, CreatedBy: nil}, nil).Once()
		repo.EXPECT().Delete(mock.Anything, relTeamID, relRelationID).Return(nil).Once()
		svc := newRelationService(t, repo, nil, authzForRole(t, models.TeamMemberRoleAdmin))

		require.NoError(t, svc.Delete(context.Background(), relCaller, relTeamID, relRelationID))
	})
}

func TestRelationService_ListByResource(t *testing.T) {
	const rtype = models.RelationResourceTypeArtifact

	t.Run("non-member denied", func(t *testing.T) {
		repo := mocks.NewMockRelationRepository(t)
		svc := newRelationService(t, repo, membershipStub{isMember: false}, allowAllAuthz{})

		_, err := svc.ListByResource(context.Background(), relCaller, relTeamID, rtype, relFromID, 1, 5)

		require.Error(t, err)
		repo.AssertNotCalled(t, "ListByResource",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("invalid resource type", func(t *testing.T) {
		repo := mocks.NewMockRelationRepository(t)
		svc := newRelationService(t, repo, membershipStub{isMember: true}, allowAllAuthz{})

		_, err := svc.ListByResource(context.Background(), relCaller, relTeamID, "project", relFromID, 1, 5)

		require.Error(t, err)
	})

	t.Run("clamps page and limit", func(t *testing.T) {
		repo := mocks.NewMockRelationRepository(t)
		repo.EXPECT().ListByResource(mock.Anything, relTeamID, rtype, relFromID, 1, 100).
			Return([]models.RelatedResource{{RelationID: relRelationID}}, 1, nil).Once()
		svc := newRelationService(t, repo, membershipStub{isMember: true}, allowAllAuthz{})

		resp, err := svc.ListByResource(context.Background(), relCaller, relTeamID, rtype, relFromID, 0, 500)

		require.NoError(t, err)
		assert.Equal(t, 1, resp.Page)
		assert.Equal(t, 100, resp.PerPage)
		assert.Equal(t, 1, resp.TotalCount)
		assert.Equal(t, 1, resp.TotalPages)
		assert.Len(t, []models.RelatedResource(resp.Related), 1)
	})
}
