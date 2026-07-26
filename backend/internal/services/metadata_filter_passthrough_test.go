package services

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// The `metadata` filter (epic #519) crosses four service -> repository mappings:
// ListArtifacts, ListArtifactsByProjectCrossTeam, the blueprint list, and
// ListMemories.
// Nothing else exercises that hop — the squirrel tests build
// repositories.*Filters directly and the handler tests stop at services.*Filters
// — so dropping one of those struct fields would silently disable filtering for
// a whole resource type with every other test still green. These tests are the
// guard: each asserts the filter arrives at the repository intact.

func passthroughFilter() repositories.MetadataFilter {
	return repositories.MetadataFilter{"env": {"prod", "staging"}, "team": {"core"}}
}

func testLogger() *slog.Logger {
	l, _ := logtest.New()
	return l
}

func TestListMemories_PassesMetadataFilterToRepository(t *testing.T) {
	repo := mocks.NewMockMemoryRepository(t)
	repo.EXPECT().List(mock.Anything, "user-123", mock.MatchedBy(func(f repositories.MemoryFilters) bool {
		return f.MetadataFilter != nil && len(f.MetadataFilter["env"]) == 2 && len(f.MetadataFilter["team"]) == 1
	})).Return([]models.Memory{}, 0, nil).Once()

	svc := createTestMemoryService(repo)

	_, err := svc.ListMemories("user-123", MemoryFilters{
		TeamID:         "team-123",
		MetadataFilter: passthroughFilter(),
		Page:           1,
		Limit:          10,
	})

	require.NoError(t, err)
}

func TestListArtifacts_PassesMetadataFilterToRepository(t *testing.T) {
	repo := mocks.NewMockArtifactRepository(t)
	repo.EXPECT().List(mock.Anything, "user-123", mock.MatchedBy(func(f repositories.ArtifactFilters) bool {
		return f.MetadataFilter != nil && len(f.MetadataFilter["env"]) == 2 && len(f.MetadataFilter["team"]) == 1
	})).Return([]models.Artifact{}, 0, nil).Once()

	svc := NewArtifactService(ArtifactServiceDeps{
		Repo:   repo,
		Authz:  allowAllAuthz{},
		Logger: testLogger(),
	})

	_, err := svc.ListArtifacts("user-123", ArtifactFilters{
		TeamID:         "team-123",
		MetadataFilter: passthroughFilter(),
		Page:           1,
		Limit:          10,
	})

	require.NoError(t, err)
}

func TestListArtifactsByProjectCrossTeam_PassesMetadataFilterToRepository(t *testing.T) {
	repo := mocks.NewMockArtifactRepository(t)
	repo.EXPECT().ListCrossTeam(mock.Anything, "user-123", mock.MatchedBy(func(f repositories.ArtifactFilters) bool {
		return f.MetadataFilter != nil && len(f.MetadataFilter["env"]) == 2
	})).Return([]models.Artifact{}, 0, nil).Once()

	svc := NewArtifactService(ArtifactServiceDeps{
		Repo:   repo,
		Authz:  allowAllAuthz{},
		Logger: testLogger(),
	})

	_, err := svc.ListArtifactsByProjectCrossTeam("user-123", "", ArtifactFilters{
		MetadataFilter: passthroughFilter(),
		Page:           1,
		Limit:          10,
	})

	require.NoError(t, err)
}

func TestListBlueprints_PassesMetadataFilterToRepository(t *testing.T) {
	repo := mocks.NewMockBlueprintRepository(t)
	repo.EXPECT().List(mock.Anything, "user-123", mock.MatchedBy(func(f repositories.BlueprintFilters) bool {
		return f.MetadataFilter != nil && len(f.MetadataFilter["env"]) == 2 && len(f.MetadataFilter["team"]) == 1
	})).Return([]models.Blueprint{}, 0, nil).Once()

	svc := NewBlueprintService(BlueprintServiceDeps{
		Repo:   repo,
		Authz:  allowAllAuthz{},
		Logger: testLogger(),
	})

	_, err := svc.ListBlueprints("user-123", BlueprintFilters{
		TeamID:         "team-123",
		MetadataFilter: passthroughFilter(),
		Page:           1,
		Limit:          10,
	})

	require.NoError(t, err)
}
