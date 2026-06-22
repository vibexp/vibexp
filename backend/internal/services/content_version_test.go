package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

func newTestContentVersionService(
	repo repositories.ContentVersionRepository,
	users repositories.UserRepository,
	adapters ...ContentVersionAdapter,
) *ContentVersionService {
	logger, _ := logtest.New()
	return NewContentVersionService(repo, users, logger, adapters...)
}

func artifactAdapter() ContentVersionAdapter {
	return ContentVersionAdapter{ResourceType: "artifact", RetentionCap: 5, InitialVersionLabel: "Created the artifact"}
}

func TestContentVersionService_SnapshotIfChanged_NoChange(t *testing.T) {
	repo := repomocks.NewMockContentVersionRepository(t)
	svc := newTestContentVersionService(repo, nil, artifactAdapter())

	err := svc.SnapshotIfChanged(context.Background(), SnapshotRequest{
		ResourceType: "artifact", ResourceID: "res-1", TeamID: "team-1", UserID: "user-1",
		OldContent: "same", NewContent: "same",
	})

	require.NoError(t, err)
	// No repo interaction expected; mockery asserts no unexpected calls on cleanup.
}

func TestContentVersionService_SnapshotIfChanged_SnapshotsOldContentAndPrunes(t *testing.T) {
	repo := repomocks.NewMockContentVersionRepository(t)
	svc := newTestContentVersionService(repo, nil, artifactAdapter())

	summary := "Tightened the wording"
	repo.EXPECT().
		Create(mock.Anything, mock.MatchedBy(func(v *models.ContentVersion) bool {
			return v.ResourceType == "artifact" &&
				v.ResourceID == "res-1" &&
				v.TeamID == "team-1" &&
				v.Content == "old" && // snapshots the PRIOR content, not the new one
				v.CreatedBy != nil && *v.CreatedBy == "user-1" &&
				v.ChangeSummary != nil && *v.ChangeSummary == summary &&
				v.ActorType == models.ActorTypeHuman // empty actor defaults to human
		})).
		Return(nil).
		Once()
	// PruneToCap must be called with the adapter's retention cap (5), not a hardcoded value.
	repo.EXPECT().PruneToCap(mock.Anything, "artifact", "res-1", 5).Return(nil).Once()

	err := svc.SnapshotIfChanged(context.Background(), SnapshotRequest{
		ResourceType: "artifact", ResourceID: "res-1", TeamID: "team-1", UserID: "user-1",
		OldContent: "old", NewContent: "new", ChangeSummary: &summary,
	})

	require.NoError(t, err)
}

func TestContentVersionService_SnapshotIfChanged_SystemActorPreserved(t *testing.T) {
	repo := repomocks.NewMockContentVersionRepository(t)
	svc := newTestContentVersionService(repo, nil, artifactAdapter())

	repo.EXPECT().
		Create(mock.Anything, mock.MatchedBy(func(v *models.ContentVersion) bool {
			return v.ActorType == models.ActorTypeSystem
		})).
		Return(nil).
		Once()
	repo.EXPECT().PruneToCap(mock.Anything, "artifact", "res-1", 5).Return(nil).Once()

	err := svc.SnapshotIfChanged(context.Background(), SnapshotRequest{
		ResourceType: "artifact", ResourceID: "res-1", TeamID: "team-1", UserID: "user-1",
		OldContent: "old", NewContent: "new", ActorType: models.ActorTypeSystem,
	})

	require.NoError(t, err)
}

func TestContentVersionService_SnapshotIfChanged_EmptyUserIsNilCreatedBy(t *testing.T) {
	repo := repomocks.NewMockContentVersionRepository(t)
	svc := newTestContentVersionService(repo, nil, artifactAdapter())

	repo.EXPECT().
		Create(mock.Anything, mock.MatchedBy(func(v *models.ContentVersion) bool {
			return v.CreatedBy == nil
		})).
		Return(nil).
		Once()
	repo.EXPECT().PruneToCap(mock.Anything, "artifact", "res-1", 5).Return(nil).Once()

	err := svc.SnapshotIfChanged(context.Background(), SnapshotRequest{
		ResourceType: "artifact", ResourceID: "res-1", TeamID: "team-1", UserID: "",
		OldContent: "old", NewContent: "new",
	})

	require.NoError(t, err)
}

func TestContentVersionService_UnregisteredResourceType(t *testing.T) {
	repo := repomocks.NewMockContentVersionRepository(t)
	svc := newTestContentVersionService(repo, nil, artifactAdapter())

	err := svc.SnapshotIfChanged(context.Background(), SnapshotRequest{
		ResourceType: "prompt", ResourceID: "res-1", TeamID: "team-1", UserID: "user-1",
		OldContent: "old", NewContent: "new",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unregistered resource type")

	_, listErr := svc.ListVersions(context.Background(), "team-1", "prompt", "res-1")
	require.Error(t, listErr)
	assert.Contains(t, listErr.Error(), "unregistered resource type")
}

func TestContentVersionService_Restore_ReturnsTargetContent(t *testing.T) {
	repo := repomocks.NewMockContentVersionRepository(t)
	svc := newTestContentVersionService(repo, nil, artifactAdapter())

	repo.EXPECT().
		GetByVersionNumber(mock.Anything, "team-1", "artifact", "res-1", 2).
		Return(&models.ContentVersion{VersionNumber: 2, Content: "v2-content"}, nil).
		Once()

	content, err := svc.Restore(context.Background(), "team-1", "artifact", "res-1", 2)

	require.NoError(t, err)
	assert.Equal(t, "v2-content", content)
}

func TestContentVersionService_GetVersion_NotFoundPropagates(t *testing.T) {
	repo := repomocks.NewMockContentVersionRepository(t)
	svc := newTestContentVersionService(repo, nil, artifactAdapter())

	repo.EXPECT().
		GetByVersionNumber(mock.Anything, "team-1", "artifact", "res-1", 99).
		Return(nil, repositories.ErrContentVersionNotFound).
		Once()

	_, err := svc.GetVersion(context.Background(), "team-1", "artifact", "res-1", 99)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repositories.ErrContentVersionNotFound))
}

func TestContentVersionService_ListVersions_ResolvesAuthorsAndDedups(t *testing.T) {
	repo := repomocks.NewMockContentVersionRepository(t)
	users := repomocks.NewMockUserRepository(t)
	svc := newTestContentVersionService(repo, users, artifactAdapter())

	userA := "user-a"
	avatar := "https://example.com/a.png"
	repo.EXPECT().
		ListByResource(mock.Anything, "team-1", "artifact", "art-1").
		Return([]*models.ContentVersion{
			{VersionNumber: 3, Content: "c3", CreatedBy: &userA},
			{VersionNumber: 2, Content: "c2", CreatedBy: &userA}, // same author -> single lookup
			{VersionNumber: 1, Content: "c1", CreatedBy: nil},    // no author
		}, nil).
		Once()
	// Author resolved exactly once for the two versions sharing user-a.
	users.EXPECT().
		GetByID(mock.Anything, "user-a").
		Return(&models.User{ID: "user-a", Name: "Ada Lovelace", AvatarURL: &avatar}, nil).
		Once()

	versions, err := svc.ListVersions(context.Background(), "team-1", "artifact", "art-1")
	require.NoError(t, err)
	require.Len(t, versions, 3)

	require.NotNil(t, versions[0].Author)
	assert.Equal(t, "Ada Lovelace", versions[0].Author.DisplayName)
	assert.Equal(t, "AL", versions[0].Author.Initials)
	assert.Equal(t, &avatar, versions[0].Author.AvatarURL)
	require.NotNil(t, versions[1].Author)
	assert.Equal(t, "user-a", versions[1].Author.ID)
	assert.Nil(t, versions[2].Author) // CreatedBy nil -> no author resolution
}

func TestContentVersionService_ListVersions_InitialVersionLabel(t *testing.T) {
	repo := repomocks.NewMockContentVersionRepository(t)
	svc := newTestContentVersionService(repo, nil, artifactAdapter())

	explicit := "Explicit summary"
	repo.EXPECT().
		ListByResource(mock.Anything, "team-1", "artifact", "art-1").
		Return([]*models.ContentVersion{
			{VersionNumber: 2, Content: "c2", ChangeSummary: nil},       // not version 1 -> stays nil
			{VersionNumber: 1, Content: "c1", ChangeSummary: &explicit}, // explicit summary not overridden
		}, nil).
		Once()

	versions, err := svc.ListVersions(context.Background(), "team-1", "artifact", "art-1")
	require.NoError(t, err)

	assert.Nil(t, versions[0].ChangeSummary)
	require.NotNil(t, versions[1].ChangeSummary)
	assert.Equal(t, "Explicit summary", *versions[1].ChangeSummary) // explicit wins over default
}

func TestContentVersionService_ListVersions_InitialVersionLabelDefaultsWhenNil(t *testing.T) {
	repo := repomocks.NewMockContentVersionRepository(t)
	svc := newTestContentVersionService(repo, nil, artifactAdapter())

	repo.EXPECT().
		ListByResource(mock.Anything, "team-1", "artifact", "art-1").
		Return([]*models.ContentVersion{
			{VersionNumber: 1, Content: "c1", ChangeSummary: nil},
		}, nil).
		Once()

	versions, err := svc.ListVersions(context.Background(), "team-1", "artifact", "art-1")
	require.NoError(t, err)
	require.NotNil(t, versions[0].ChangeSummary)
	assert.Equal(t, "Created the artifact", *versions[0].ChangeSummary)
}

func TestContentVersionService_GetVersion_AuthorResolutionIsBestEffort(t *testing.T) {
	repo := repomocks.NewMockContentVersionRepository(t)
	users := repomocks.NewMockUserRepository(t)
	svc := newTestContentVersionService(repo, users, artifactAdapter())

	userA := "user-a"
	repo.EXPECT().
		GetByVersionNumber(mock.Anything, "team-1", "artifact", "art-1", 2).
		Return(&models.ContentVersion{VersionNumber: 2, Content: "c2", CreatedBy: &userA}, nil).
		Once()
	users.EXPECT().
		GetByID(mock.Anything, "user-a").
		Return(nil, errors.New("user gone")).
		Once()

	version, err := svc.GetVersion(context.Background(), "team-1", "artifact", "art-1", 2)
	require.NoError(t, err)       // a failed author lookup must not fail the read
	assert.Nil(t, version.Author) // unresolved author -> nil, not an error
}

// TestContentVersionService_Reusability_Polymorphic is the acceptance-criteria smoke
// test: the core works for a brand-new resource type ("prompt") with no schema change,
// proving the versioning core is polymorphic. It registers both an artifact and a prompt
// adapter and exercises snapshot -> list -> restore against "prompt".
func TestContentVersionService_Reusability_Polymorphic(t *testing.T) {
	repo := repomocks.NewMockContentVersionRepository(t)
	svc := newTestContentVersionService(
		repo,
		nil,
		ContentVersionAdapter{ResourceType: "artifact", RetentionCap: 5},
		ContentVersionAdapter{ResourceType: "prompt", RetentionCap: 3},
	)

	ctx := context.Background()

	// snapshot a prompt
	repo.EXPECT().
		Create(mock.Anything, mock.MatchedBy(func(v *models.ContentVersion) bool {
			return v.ResourceType == "prompt" && v.Content == "old-prompt"
		})).
		Return(nil).
		Once()
	// prune uses the prompt adapter's cap (3)
	repo.EXPECT().PruneToCap(mock.Anything, "prompt", "p-1", 3).Return(nil).Once()
	require.NoError(t, svc.SnapshotIfChanged(ctx, SnapshotRequest{
		ResourceType: "prompt", ResourceID: "p-1", TeamID: "team-1", UserID: "user-1",
		OldContent: "old-prompt", NewContent: "new-prompt",
	}))

	// list prompt versions
	repo.EXPECT().
		ListByResource(mock.Anything, "team-1", "prompt", "p-1").
		Return([]*models.ContentVersion{{VersionNumber: 1, Content: "old-prompt"}}, nil).
		Once()
	versions, err := svc.ListVersions(ctx, "team-1", "prompt", "p-1")
	require.NoError(t, err)
	require.Len(t, versions, 1)
	// prompt adapter has no InitialVersionLabel -> version 1 summary stays nil
	assert.Nil(t, versions[0].ChangeSummary)

	// restore a prompt version
	repo.EXPECT().
		GetByVersionNumber(mock.Anything, "team-1", "prompt", "p-1", 1).
		Return(&models.ContentVersion{VersionNumber: 1, Content: "old-prompt"}, nil).
		Once()
	content, err := svc.Restore(ctx, "team-1", "prompt", "p-1", 1)
	require.NoError(t, err)
	assert.Equal(t, "old-prompt", content)
}

func TestInitials(t *testing.T) {
	cases := map[string]string{
		"Ada Lovelace":        "AL",
		"madonna":             "M",
		"  spaced  out  ":     "SO",
		"":                    "?",
		"!@#":                 "?",
		"Jean-Luc Picard CMD": "JP",
	}
	for name, want := range cases {
		assert.Equal(t, want, initials(name), "initials(%q)", name)
	}
}
