package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// The `locked` list filter (issue #1288) crosses the same service ->
// repository mappings the metadata filter does (see
// metadata_filter_passthrough_test.go), and nothing else exercises that hop:
// the handler tests stop at services.*Filters and the repository tests start
// from repositories.*Filters. Dropping the field from one mapping would
// silently disable the filter for a whole resource type, so each mapping is
// asserted here for true, false and no filter.

func lockedFilterCases() map[string]*bool {
	yes, no := true, false
	return map[string]*bool{"locked": &yes, "unlocked": &no, "no filter": nil}
}

func TestListPrompts_PassesLockedFilterToRepository(t *testing.T) {
	for name, locked := range lockedFilterCases() {
		t.Run(name, func(t *testing.T) {
			var got *bool
			repo := mocks.NewMockPromptRepository(t)
			repo.EXPECT().List(mock.Anything, "user-123", mock.MatchedBy(func(f repositories.PromptFilters) bool {
				got = f.Locked
				return true
			})).Return([]models.Prompt{}, 0, nil).Once()

			_, err := createTestPromptService(repo, nil).ListPrompts("user-123", PromptFilters{
				TeamID: "team-123", Locked: locked, Page: 1, Limit: 10,
			})

			require.NoError(t, err)
			assert.Equal(t, locked, got)
		})
	}
}

func TestListArtifacts_PassesLockedFilterToRepository(t *testing.T) {
	for name, locked := range lockedFilterCases() {
		t.Run(name, func(t *testing.T) {
			var got *bool
			repo := mocks.NewMockArtifactRepository(t)
			repo.EXPECT().List(mock.Anything, "user-123", mock.MatchedBy(func(f repositories.ArtifactFilters) bool {
				got = f.Locked
				return true
			})).Return([]models.Artifact{}, 0, nil).Once()
			svc := NewArtifactService(ArtifactServiceDeps{Repo: repo, Authz: allowAllAuthz{}, Logger: testLogger()})

			_, err := svc.ListArtifacts("user-123", ArtifactFilters{
				TeamID: "team-123", Locked: locked, Page: 1, Limit: 10,
			})

			require.NoError(t, err)
			assert.Equal(t, locked, got)
		})
	}
}

func TestListArtifactsByProjectCrossTeam_PassesLockedFilterToRepository(t *testing.T) {
	for name, locked := range lockedFilterCases() {
		t.Run(name, func(t *testing.T) {
			var got *bool
			repo := mocks.NewMockArtifactRepository(t)
			repo.EXPECT().ListCrossTeam(mock.Anything, "user-123",
				mock.MatchedBy(func(f repositories.ArtifactFilters) bool {
					got = f.Locked
					return true
				})).Return([]models.Artifact{}, 0, nil).Once()
			svc := NewArtifactService(ArtifactServiceDeps{Repo: repo, Authz: allowAllAuthz{}, Logger: testLogger()})

			_, err := svc.ListArtifactsByProjectCrossTeam("user-123", "", ArtifactFilters{
				Locked: locked, Page: 1, Limit: 10,
			})

			require.NoError(t, err)
			assert.Equal(t, locked, got)
		})
	}
}

func TestListBlueprints_PassesLockedFilterToRepository(t *testing.T) {
	for name, locked := range lockedFilterCases() {
		t.Run(name, func(t *testing.T) {
			var got *bool
			repo := mocks.NewMockBlueprintRepository(t)
			repo.EXPECT().List(mock.Anything, "user-123", mock.MatchedBy(func(f repositories.BlueprintFilters) bool {
				got = f.Locked
				return true
			})).Return([]models.Blueprint{}, 0, nil).Once()
			svc := NewBlueprintService(BlueprintServiceDeps{Repo: repo, Authz: allowAllAuthz{}, Logger: testLogger()})

			_, err := svc.ListBlueprints("user-123", BlueprintFilters{
				TeamID: "team-123", Locked: locked, Page: 1, Limit: 10,
			})

			require.NoError(t, err)
			assert.Equal(t, locked, got)
		})
	}
}
