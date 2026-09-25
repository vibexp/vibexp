//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// The admin CSV exports (#1149) must yield exactly the listing's filtered set,
// in the listing's order. Each test pages the list one row at a time to the
// end and compares the ordered ids with the stream over the same filters,
// scoped by the fixtures' search tokens like the list tests.

// pageAdminIDs pages a listing one row per page until an empty page, returning
// the ordered ids and the reported total.
func pageAdminIDs(t *testing.T, page func(n int) (ids []string, total int)) ([]string, int) {
	t.Helper()
	var ids []string
	total := -1
	for n := 1; n <= 20; n++ {
		pageIDs, pageTotal := page(n)
		total = pageTotal
		if len(pageIDs) == 0 {
			return ids, total
		}
		ids = append(ids, pageIDs...)
	}
	t.Fatal("paging did not reach an empty page")
	return nil, 0
}

// streamAdminIDs collects the ids a Stream* call yields.
func streamAdminIDs[T any](
	t *testing.T, stream func(fn func(T) error) error, id func(T) string,
) []string {
	t.Helper()
	var ids []string
	require.NoError(t, stream(func(item T) error {
		ids = append(ids, id(item))
		return nil
	}))
	return ids
}

func TestAdminUserExport_StreamEqualsPagedList(t *testing.T) {
	f := seedAdminListFixture(t)
	repo := NewAdminRepository(integrationDB)
	ctx := context.Background()

	filters := repositories.AdminUserFilters{
		Search:             &f.token,
		PromptCount:        repositories.AdminCountRange{Min: int64p(1)},
		TotalResourceCount: repositories.AdminCountRange{Max: int64p(100)},
		SortBy:             "total_resource_count",
		SortOrder:          "asc",
	}

	paged, total := pageAdminIDs(t, func(n int) ([]string, int) {
		pf := filters
		pf.Page, pf.Limit = n, 1
		users, pageTotal, err := repo.ListUsers(ctx, pf)
		require.NoError(t, err)
		return adminListIDs(users), pageTotal
	})
	streamed := streamAdminIDs(t, func(fn func(models.AdminUserListItem) error) error {
		return repo.StreamUsers(ctx, filters, 100, fn)
	}, func(u models.AdminUserListItem) string { return u.ID })

	assert.Equal(t, []string{f.light, f.heavy}, paged, "the fixture's filtered order")
	assert.Equal(t, paged, streamed)
	count, err := repo.CountUsers(ctx, filters)
	require.NoError(t, err)
	assert.Equal(t, total, count)

	capped := streamAdminIDs(t, func(fn func(models.AdminUserListItem) error) error {
		return repo.StreamUsers(ctx, filters, 1, fn)
	}, func(u models.AdminUserListItem) string { return u.ID })
	assert.Equal(t, paged[:1], capped, "limit caps the stream")
}

func TestAdminTeamExport_StreamEqualsPagedList(t *testing.T) {
	f := seedAdminTeamListFixture(t)
	repo := NewAdminRepository(integrationDB)
	ctx := context.Background()

	filters := repositories.AdminTeamFilters{
		Search:       &f.token,
		MemberCount:  repositories.AdminCountRange{Min: int64p(1)},
		ProjectCount: repositories.AdminCountRange{Min: int64p(1)},
		PromptCount:  repositories.AdminCountRange{Max: int64p(100)},
		SortBy:       "admin_count",
		SortOrder:    "asc",
	}

	paged, total := pageAdminIDs(t, func(n int) ([]string, int) {
		pf := filters
		pf.Page, pf.Limit = n, 1
		teams, pageTotal, err := repo.ListTeams(ctx, pf)
		require.NoError(t, err)
		return adminTeamIDs(teams), pageTotal
	})
	streamed := streamAdminIDs(t, func(fn func(models.AdminTeamListItem) error) error {
		return repo.StreamTeams(ctx, filters, 100, fn)
	}, func(tm models.AdminTeamListItem) string { return tm.ID })

	assert.Len(t, paged, 2, "heavy and light have members and a project; empty has neither")
	assert.Equal(t, paged, streamed)
	count, err := repo.CountTeams(ctx, filters)
	require.NoError(t, err)
	assert.Equal(t, total, count)
}

func TestAdminProjectExport_StreamEqualsPagedList(t *testing.T) {
	f := seedAdminProjectListFixture(t)
	repo := NewAdminRepository(integrationDB)
	ctx := context.Background()

	filters := repositories.AdminProjectFilters{
		Search:             &f.token,
		TotalResourceCount: repositories.AdminCountRange{Min: int64p(1)},
		PromptCount:        repositories.AdminCountRange{Min: int64p(1)},
		FeedItemCount:      repositories.AdminCountRange{Max: int64p(100)},
		SortBy:             "feed_item_count",
		SortOrder:          "asc",
	}

	paged, total := pageAdminIDs(t, func(n int) ([]string, int) {
		pf := filters
		pf.Page, pf.Limit = n, 1
		projects, pageTotal, err := repo.ListProjects(ctx, pf)
		require.NoError(t, err)
		return adminProjectIDs(projects), pageTotal
	})
	streamed := streamAdminIDs(t, func(fn func(models.AdminProjectListItem) error) error {
		return repo.StreamProjects(ctx, filters, 100, fn)
	}, func(p models.AdminProjectListItem) string { return p.ID })

	assert.Equal(t, []string{f.light, f.heavy}, paged, "the fixture's filtered order")
	assert.Equal(t, paged, streamed)
	count, err := repo.CountProjects(ctx, filters)
	require.NoError(t, err)
	assert.Equal(t, total, count)
}

// TestAdminExport_CallbackErrorStopsStream pins that the first callback error
// ends the stream and comes back unchanged (a closed pipe on client disconnect).
func TestAdminExport_CallbackErrorStopsStream(t *testing.T) {
	f := seedAdminProjectListFixture(t)
	repo := NewAdminRepository(integrationDB)
	stop := errors.New("client went away")

	calls := 0
	err := repo.StreamProjects(context.Background(), repositories.AdminProjectFilters{Search: &f.token}, 100,
		func(models.AdminProjectListItem) error {
			calls++
			return stop
		})
	require.ErrorIs(t, err, stop)
	assert.Equal(t, 1, calls)
}

// TestAdminExport_CancelledContextFails pins that a cancelled request context
// fails the stream loudly rather than yielding an empty, clean result.
func TestAdminExport_CancelledContextFails(t *testing.T) {
	f := seedAdminListFixture(t)
	repo := NewAdminRepository(integrationDB)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := repo.StreamUsers(ctx, repositories.AdminUserFilters{Search: &f.token}, 100,
		func(models.AdminUserListItem) error { return nil })
	require.Error(t, err)
}
