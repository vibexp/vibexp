package services

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/freshness"
)

// These tests pin the app-level freshness cascade (#762): deleting a resource
// clears its resource_freshness row, since resource_id carries no cascading FK
// (one column cannot reference four resource tables). The cascade is
// best-effort — a failure must not fail the completed delete, and an absent
// dependency must not panic — but the happy path must invoke it.

const (
	cascadeArtifactID  = "artifact-1"
	cascadeBlueprintID = "blueprint-1"
	cascadeMemoryID    = "memory-1"
	cascadePromptID    = "prompt-1"
	cascadeProjectID   = "project-1"
	cascadeSlug        = "s"
)

// freshnessCascadeCase drives one resource service's delete path. wantType is
// the resource-type string that service must pass to DeleteByResource.
type freshnessCascadeCase struct {
	name       string
	wantType   string
	resourceID string
	// del wires the service with freshnessRepo and performs the delete. A nil
	// freshnessRepo constructs the service WITHOUT the dependency -- it must be
	// an untyped nil, since a typed nil mock would satisfy the interface and
	// slip past the helper's nil guard.
	del func(t *testing.T, freshnessRepo repositories.ResourceFreshnessRepository) error
}

func freshnessCascadeCases() []freshnessCascadeCase {
	logger, _ := logtest.New()

	return []freshnessCascadeCase{
		{
			name:       "artifact",
			wantType:   "artifact",
			resourceID: cascadeArtifactID,
			del: func(t *testing.T, freshnessRepo repositories.ResourceFreshnessRepository) error {
				t.Helper()
				repo := mocks.NewMockArtifactRepository(t)
				repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, resRBACCaller, resRBACTeamID, cascadeProjectID, cascadeSlug).
					Return(&models.Artifact{
						ID: cascadeArtifactID, UserID: resRBACCaller, TeamID: resRBACTeamID, Slug: cascadeSlug,
					}, nil).Once()
				repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, cascadeArtifactID).Return(nil).Once()

				svc := NewArtifactService(ArtifactServiceDeps{
					Repo:          repo,
					Authz:         authzForRole(t, models.TeamMemberRoleMember),
					Logger:        logger,
					FreshnessRepo: freshnessRepo,
				})
				return svc.DeleteArtifactByProjectIDAndSlug(resRBACCaller, resRBACTeamID, cascadeProjectID, cascadeSlug)
			},
		},
		{
			name:       "blueprint",
			wantType:   "blueprint",
			resourceID: cascadeBlueprintID,
			del: func(t *testing.T, freshnessRepo repositories.ResourceFreshnessRepository) error {
				t.Helper()
				repo := mocks.NewMockBlueprintRepository(t)
				repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, resRBACCaller, resRBACTeamID, cascadeProjectID, cascadeSlug).
					Return(&models.Blueprint{
						ID: cascadeBlueprintID, UserID: resRBACCaller, TeamID: resRBACTeamID, Slug: cascadeSlug,
					}, nil).Once()
				repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, cascadeBlueprintID).Return(nil).Once()

				svc := NewBlueprintService(BlueprintServiceDeps{
					Repo:          repo,
					Authz:         authzForRole(t, models.TeamMemberRoleMember),
					Logger:        logger,
					FreshnessRepo: freshnessRepo,
				})
				return svc.DeleteBlueprintByProjectIDAndSlug(resRBACCaller, resRBACTeamID, cascadeProjectID, cascadeSlug)
			},
		},
		{
			name:       "memory",
			wantType:   "memory",
			resourceID: cascadeMemoryID,
			del: func(t *testing.T, freshnessRepo repositories.ResourceFreshnessRepository) error {
				t.Helper()
				repo := mocks.NewMockMemoryRepository(t)
				repo.EXPECT().GetByID(mock.Anything, resRBACCaller, resRBACTeamID, cascadeMemoryID).
					Return(&models.Memory{ID: cascadeMemoryID, UserID: resRBACCaller, TeamID: resRBACTeamID}, nil).Once()
				repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, cascadeMemoryID).Return(nil).Once()

				svc := NewMemoryService(
					repo, nil, authzForRole(t, models.TeamMemberRoleMember), nil, logger, nil, nil, nil, nil,
					freshnessRepo,
				)
				return svc.DeleteMemory(resRBACCaller, resRBACTeamID, cascadeMemoryID)
			},
		},
		{
			name:       "prompt",
			wantType:   "prompt",
			resourceID: cascadePromptID,
			del: func(t *testing.T, freshnessRepo repositories.ResourceFreshnessRepository) error {
				t.Helper()
				repo := mocks.NewMockPromptRepository(t)
				refRepo := mocks.NewMockPromptReferenceRepository(t)
				repo.EXPECT().GetByID(mock.Anything, resRBACCaller, resRBACTeamID, cascadePromptID).
					Return(&models.Prompt{ID: cascadePromptID, UserID: resRBACCaller, TeamID: resRBACTeamID}, nil).Once()
				refRepo.EXPECT().HasDependents(mock.Anything, cascadePromptID).Return(false, nil).Once()
				repo.EXPECT().Delete(mock.Anything, resRBACCaller, resRBACTeamID, cascadePromptID).Return(nil).Once()

				svc := NewPromptService(PromptServiceDeps{
					Repo:          repo,
					RefRepo:       refRepo,
					Authz:         authzForRole(t, models.TeamMemberRoleMember),
					Logger:        logger,
					FreshnessRepo: freshnessRepo,
				})
				return svc.DeletePrompt(resRBACCaller, resRBACTeamID, cascadePromptID)
			},
		},
	}
}

// TestResourceServices_Delete_CascadesFreshness pins two things at once.
//
// Per resource type: the delete path calls DeleteByResource with that type and
// the resource's id.
//
// Across types: the set of types the four cascades pass is EXACTLY
// freshness.EvaluableResourceTypes — the vocabulary the evaluator marks rows
// with. Without that assertion the per-type expectations only pin the strings
// against themselves, so a rename on the freshness side would leave every
// cascade a silent no-op (DeleteByResource matches nothing and returns
// (false, nil), never an error). It is also the guard against a fifth
// evaluable resource type shipping without a cascade.
func TestResourceServices_Delete_CascadesFreshness(t *testing.T) {
	cases := freshnessCascadeCases()
	cascaded := make([]string, 0, len(cases))

	for _, c := range cases {
		cascaded = append(cascaded, c.wantType)

		t.Run(c.name, func(t *testing.T) {
			freshnessRepo := mocks.NewMockResourceFreshnessRepository(t)
			freshnessRepo.EXPECT().DeleteByResource(mock.Anything, c.wantType, c.resourceID).
				Return(true, nil).Once()

			require.NoError(t, c.del(t, freshnessRepo))
		})
	}

	assert.ElementsMatch(t, freshness.EvaluableResourceTypes, cascaded,
		"every evaluable resource type must clear its freshness state on delete, and only those")
}

// A freshness clear that fails must not fail the delete: the resource row is
// already gone and the caller got what they asked for. The next evaluation run
// cannot repair this one (the resource no longer exists to be evaluated), so
// the warning log is the only signal — but failing the request would be worse.
func TestResourceServices_Delete_FreshnessClearFailureDoesNotFailDelete(t *testing.T) {
	for _, c := range freshnessCascadeCases() {
		t.Run(c.name, func(t *testing.T) {
			freshnessRepo := mocks.NewMockResourceFreshnessRepository(t)
			freshnessRepo.EXPECT().DeleteByResource(mock.Anything, c.wantType, c.resourceID).
				Return(false, errors.New("boom")).Once()

			require.NoError(t, c.del(t, freshnessRepo))
		})
	}
}

// Freshness is optional wiring, exactly like the comment and relation repos: a
// service constructed without it must delete without panicking.
func TestResourceServices_Delete_WithoutFreshnessRepoDoesNotPanic(t *testing.T) {
	for _, c := range freshnessCascadeCases() {
		t.Run(c.name, func(t *testing.T) {
			require.NoError(t, c.del(t, nil))
		})
	}
}
