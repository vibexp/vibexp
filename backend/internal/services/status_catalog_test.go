package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

const testStatusTeamID = "team-1"

func newStatusCatalogService(t *testing.T) (*StatusCatalogService, *repomocks.MockTeamStatusRepository) {
	t.Helper()
	repo := repomocks.NewMockTeamStatusRepository(t)
	return NewStatusCatalogService(repo), repo
}

var (
	testStatusCreatedAt = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	testStatusUpdatedAt = time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
)

func customStatus(id, resourceType, slug, name string) models.TeamStatus {
	return models.TeamStatus{
		ID: id, TeamID: testStatusTeamID, ResourceType: resourceType, Slug: slug, Name: name,
		CreatedAt: testStatusCreatedAt, UpdatedAt: testStatusUpdatedAt,
	}
}

func TestStatusCatalogService_List_SystemThenCustom(t *testing.T) {
	svc, repo := newStatusCatalogService(t)
	repo.EXPECT().List(mock.Anything, testStatusTeamID, models.StatusResourceTypeArtifact).
		Return([]models.TeamStatus{
			customStatus("status-1", models.StatusResourceTypeArtifact, "in-review", "In review"),
			customStatus("status-2", models.StatusResourceTypeArtifact, "shipped", "Shipped"),
		}, nil)

	got, err := svc.List(context.Background(), testStatusTeamID, models.StatusResourceTypeArtifact)
	require.NoError(t, err)

	id1, id2 := "status-1", "status-2"
	created, renamed := testStatusCreatedAt, testStatusUpdatedAt
	assert.Equal(t, []CatalogStatus{
		{Slug: "active", Name: "Active", IsSystem: true, IsLive: true},
		{Slug: "draft", Name: "Draft", IsSystem: true, IsLive: false},
		{Slug: "archived", Name: "Archived", IsSystem: true, IsLive: false},
		{ID: &id1, Slug: "in-review", Name: "In review", IsLive: true, CreatedAt: &created, UpdatedAt: &renamed},
		{ID: &id2, Slug: "shipped", Name: "Shipped", IsLive: true, CreatedAt: &created, UpdatedAt: &renamed},
	}, got)
}

// With no custom rows the catalog is exactly the system subset, in the order
// models declares it, for every supported resource type.
func TestStatusCatalogService_List_SystemSubsetPerResourceType(t *testing.T) {
	for _, resourceType := range models.StatusResourceTypes {
		t.Run(resourceType, func(t *testing.T) {
			svc, repo := newStatusCatalogService(t)
			repo.EXPECT().List(mock.Anything, testStatusTeamID, resourceType).
				Return([]models.TeamStatus{}, nil)

			got, err := svc.List(context.Background(), testStatusTeamID, resourceType)
			require.NoError(t, err)

			system := models.SystemStatusesFor(resourceType)
			require.Len(t, got, len(system))
			for i, entry := range got {
				assert.Equal(t, system[i], entry.Slug)
				assert.True(t, entry.IsSystem)
				assert.Nil(t, entry.ID, "a system status has no row")
				assert.NotEmpty(t, entry.Name)
				assert.Equal(t, models.IsLiveStatus(resourceType, entry.Slug), entry.IsLive)
			}
		})
	}
}

// A custom row carrying a system slug can only have been written around the
// service; the catalog must still list that slug once, as the system status.
func TestStatusCatalogService_List_SystemDefinitionWinsASlugCollision(t *testing.T) {
	svc, repo := newStatusCatalogService(t)
	repo.EXPECT().List(mock.Anything, testStatusTeamID, models.StatusResourceTypeBlueprint).
		Return([]models.TeamStatus{
			customStatus("status-1", models.StatusResourceTypeBlueprint, "expired", "Shadow"),
			customStatus("status-2", models.StatusResourceTypeBlueprint, "proposed", "Proposed"),
		}, nil)

	got, err := svc.AllowedSlugs(context.Background(), testStatusTeamID, models.StatusResourceTypeBlueprint)
	require.NoError(t, err)
	assert.Equal(t, []string{"active", "expired", "proposed"}, got)
}

func TestStatusCatalogService_UnsupportedResourceType(t *testing.T) {
	// The repository mock has no expectations: an unsupported type must be
	// rejected before any query.
	for _, resourceType := range []string{"", "artifacts", "feed"} {
		t.Run(resourceType, func(t *testing.T) {
			svc, _ := newStatusCatalogService(t)
			ctx := context.Background()

			_, err := svc.List(ctx, testStatusTeamID, resourceType)
			require.ErrorIs(t, err, ErrStatusResourceTypeUnsupported)

			_, err = svc.AllowedSlugs(ctx, testStatusTeamID, resourceType)
			require.ErrorIs(t, err, ErrStatusResourceTypeUnsupported)

			allowed, err := svc.IsAllowed(ctx, testStatusTeamID, resourceType, "active")
			require.ErrorIs(t, err, ErrStatusResourceTypeUnsupported)
			assert.False(t, allowed)
		})
	}
}

func TestStatusCatalogService_RepositoryError(t *testing.T) {
	boom := errors.New("connection reset")
	ctx := context.Background()

	t.Run("List", func(t *testing.T) {
		svc, repo := newStatusCatalogService(t)
		repo.EXPECT().List(mock.Anything, testStatusTeamID, models.StatusResourceTypeMemory).Return(nil, boom)
		_, err := svc.List(ctx, testStatusTeamID, models.StatusResourceTypeMemory)
		require.ErrorIs(t, err, boom)
	})

	t.Run("AllowedSlugs", func(t *testing.T) {
		svc, repo := newStatusCatalogService(t)
		repo.EXPECT().List(mock.Anything, testStatusTeamID, models.StatusResourceTypeMemory).Return(nil, boom)
		_, err := svc.AllowedSlugs(ctx, testStatusTeamID, models.StatusResourceTypeMemory)
		require.ErrorIs(t, err, boom)
	})

	t.Run("IsAllowed fails closed", func(t *testing.T) {
		svc, repo := newStatusCatalogService(t)
		repo.EXPECT().List(mock.Anything, testStatusTeamID, models.StatusResourceTypeMemory).Return(nil, boom)
		allowed, err := svc.IsAllowed(ctx, testStatusTeamID, models.StatusResourceTypeMemory, "in-review")
		require.ErrorIs(t, err, boom)
		assert.False(t, allowed)
	})
}

func TestStatusCatalogService_IsAllowed_SystemStatusNeedsNoQuery(t *testing.T) {
	// No repository expectation: a List call would fail the test.
	svc, _ := newStatusCatalogService(t)
	ctx := context.Background()

	for _, resourceType := range models.StatusResourceTypes {
		for _, status := range models.SystemStatusesFor(resourceType) {
			allowed, err := svc.IsAllowed(ctx, testStatusTeamID, resourceType, status)
			require.NoError(t, err)
			assert.True(t, allowed, "%s/%s", resourceType, status)
		}
	}

	allowed, err := svc.IsAllowed(ctx, testStatusTeamID, models.StatusResourceTypePrompt, "")
	require.NoError(t, err)
	assert.False(t, allowed, "an empty status is never allowed")
}

func TestStatusCatalogService_IsAllowed_CustomAndForeignStatuses(t *testing.T) {
	cases := []struct {
		name   string
		status string
		want   bool
	}{
		{"the team's custom status", "in-review", true},
		{"a status nobody defined", "shipped", false},
		// `published` is a system status of prompts only.
		{"another type's system status", models.StatusPublished, false},
		{"slugs are case-sensitive", "In-Review", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newStatusCatalogService(t)
			repo.EXPECT().List(mock.Anything, testStatusTeamID, models.StatusResourceTypeArtifact).
				Return([]models.TeamStatus{
					customStatus("status-1", models.StatusResourceTypeArtifact, "in-review", "In review"),
				}, nil)

			allowed, err := svc.IsAllowed(
				context.Background(), testStatusTeamID, models.StatusResourceTypeArtifact, tc.status)
			require.NoError(t, err)
			assert.Equal(t, tc.want, allowed)
		})
	}
}

// ConfirmCustom is asked after a write (#1332). A system status needs no row,
// so it is confirmed without reaching the repository; anything else is the
// repository's locking read, whatever it answers.
func TestStatusCatalogService_ConfirmCustom(t *testing.T) {
	ctx := context.Background()

	t.Run("a system status is confirmed without a query", func(t *testing.T) {
		svc, _ := newStatusCatalogService(t)
		for _, resourceType := range models.StatusResourceTypes {
			for _, status := range models.SystemStatusesFor(resourceType) {
				confirmed, err := svc.ConfirmCustom(ctx, testStatusTeamID, resourceType, status)
				require.NoError(t, err)
				assert.True(t, confirmed, "%s %s", resourceType, status)
			}
		}
	})

	for _, stillThere := range []bool{true, false} {
		t.Run("a custom status is the repository's answer", func(t *testing.T) {
			svc, repo := newStatusCatalogService(t)
			repo.EXPECT().
				ConfirmSlug(mock.Anything, testStatusTeamID, models.StatusResourceTypeArtifact, "in-review").
				Return(stillThere, nil).Once()

			confirmed, err := svc.ConfirmCustom(ctx, testStatusTeamID, models.StatusResourceTypeArtifact, "in-review")
			require.NoError(t, err)
			assert.Equal(t, stillThere, confirmed)
		})
	}

	// Another type's system status is not this type's: `published` is a
	// prompt status only, so on an artifact it is a custom slug to look up.
	t.Run("another type's system status is looked up", func(t *testing.T) {
		svc, repo := newStatusCatalogService(t)
		repo.EXPECT().
			ConfirmSlug(mock.Anything, testStatusTeamID, models.StatusResourceTypeArtifact, "published").
			Return(false, nil).Once()

		confirmed, err := svc.ConfirmCustom(ctx, testStatusTeamID, models.StatusResourceTypeArtifact, "published")
		require.NoError(t, err)
		assert.False(t, confirmed)
	})

	t.Run("a repository error is wrapped", func(t *testing.T) {
		svc, repo := newStatusCatalogService(t)
		boom := errors.New("connection reset")
		repo.EXPECT().ConfirmSlug(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(false, boom).Once()

		_, err := svc.ConfirmCustom(ctx, testStatusTeamID, models.StatusResourceTypeArtifact, "in-review")
		require.ErrorIs(t, err, boom)
	})

	t.Run("an unsupported resource type", func(t *testing.T) {
		svc, _ := newStatusCatalogService(t)
		_, err := svc.ConfirmCustom(ctx, testStatusTeamID, "feeds", "in-review")
		require.ErrorIs(t, err, ErrStatusResourceTypeUnsupported)
	})
}

// ReassignDeleted moves the one resource to the default of its type -- `draft`
// for a prompt, `active` for the rest -- and reports that default.
func TestStatusCatalogService_ReassignDeleted(t *testing.T) {
	ctx := context.Background()

	for _, resourceType := range models.StatusResourceTypes {
		t.Run(resourceType, func(t *testing.T) {
			svc, repo := newStatusCatalogService(t)
			fallback := models.DefaultStatusFor(resourceType)
			repo.EXPECT().
				ReassignResource(mock.Anything, testStatusTeamID, resourceType, "resource-1", "in-review", fallback).
				Return(nil).Once()

			got, err := svc.ReassignDeleted(ctx, testStatusTeamID, resourceType, "resource-1", "in-review")
			require.NoError(t, err)
			assert.Equal(t, fallback, got)
		})
	}

	t.Run("a repository error is returned", func(t *testing.T) {
		svc, repo := newStatusCatalogService(t)
		boom := errors.New("connection reset")
		repo.EXPECT().
			ReassignResource(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(boom).Once()

		_, err := svc.ReassignDeleted(ctx, testStatusTeamID, models.StatusResourceTypeMemory, "resource-1", "in-review")
		require.ErrorIs(t, err, boom)
	})

	t.Run("an unsupported resource type", func(t *testing.T) {
		svc, _ := newStatusCatalogService(t)
		_, err := svc.ReassignDeleted(ctx, testStatusTeamID, "feeds", "resource-1", "in-review")
		require.ErrorIs(t, err, ErrStatusResourceTypeUnsupported)
	})
}
