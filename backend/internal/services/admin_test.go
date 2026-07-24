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
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

func TestAdminService_GetInstanceCounts(t *testing.T) {
	want := models.InstanceCounts{Users: 5, Teams: 2, Prompts: 9, Artifacts: 4, Memories: 7}
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("GetInstanceCounts", mock.Anything).Return(want, nil)

	got, err := NewAdminService(repo).GetInstanceCounts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestAdminService_GetInstanceCounts_Error(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("GetInstanceCounts", mock.Anything).Return(models.InstanceCounts{}, errors.New("boom"))

	_, err := NewAdminService(repo).GetInstanceCounts(context.Background())
	require.Error(t, err)
}

func TestAdminService_ListUsers_ClampsAndComputesPages(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	// page 0 -> 1, limit 0 -> default 20.
	repo.On("ListUsers", mock.Anything, repositories.AdminUserFilters{Page: 1, Limit: 20}).
		Return([]models.AdminUserListItem{{ID: "u1"}}, 45, nil)

	got, err := NewAdminService(repo).ListUsers(context.Background(), repositories.AdminUserFilters{})
	require.NoError(t, err)
	assert.Equal(t, 1, got.Page)
	assert.Equal(t, 20, got.PerPage)
	assert.Equal(t, 45, got.TotalCount)
	assert.Equal(t, 3, got.TotalPages) // ceil(45/20)
	assert.Len(t, got.Users, 1)
}

// TestAdminService_ListUsers_ForwardsFiltersAndPagesFilteredTotal pins the two
// service-layer responsibilities: everything except page/limit reaches the
// repository untouched, and the envelope is computed from the FILTERED total.
func TestAdminService_ListUsers_ForwardsFiltersAndPagesFilteredTotal(t *testing.T) {
	search := "alice"
	idp := "google"
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	// Only Page/Limit are clamped; the filter fields must arrive unchanged.
	want := repositories.AdminUserFilters{
		Search: &search, IDPProvider: &idp, CreatedFrom: &from, CreatedTo: &to,
		SortBy: "email", SortOrder: "asc", Page: 1, Limit: 20,
	}
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("ListUsers", mock.Anything, want).
		Return([]models.AdminUserListItem{{ID: "u1"}}, 3, nil)

	got, err := NewAdminService(repo).ListUsers(context.Background(), repositories.AdminUserFilters{
		Search: &search, IDPProvider: &idp, CreatedFrom: &from, CreatedTo: &to,
		SortBy: "email", SortOrder: "asc", Page: 0, Limit: 0,
	})
	require.NoError(t, err)
	// 3 filtered matches, not the instance-wide user count.
	assert.Equal(t, 3, got.TotalCount)
	assert.Equal(t, 1, got.TotalPages)
}

// TestAdminService_ListTeams_ForwardsFilters mirrors the user case for teams.
func TestAdminService_ListTeams_ForwardsFilters(t *testing.T) {
	search := "acme"
	isPersonal := false

	want := repositories.AdminTeamFilters{
		Search: &search, IsPersonal: &isPersonal,
		SortBy: "member_count", SortOrder: "desc", Page: 1, Limit: 20,
	}
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("ListTeams", mock.Anything, want).
		Return([]models.AdminTeamListItem{{ID: "t1"}}, 1, nil)

	got, err := NewAdminService(repo).ListTeams(context.Background(), repositories.AdminTeamFilters{
		Search: &search, IsPersonal: &isPersonal,
		SortBy: "member_count", SortOrder: "desc",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, got.TotalCount)
	assert.Equal(t, 1, got.TotalPages)
}

func TestAdminService_ListUsers_LimitCapped(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	// limit 500 -> capped to 100; empty result -> 0 pages.
	repo.On("ListUsers", mock.Anything, repositories.AdminUserFilters{Page: 2, Limit: 100}).
		Return([]models.AdminUserListItem{}, 0, nil)

	got, err := NewAdminService(repo).ListUsers(context.Background(), repositories.AdminUserFilters{Page: 2, Limit: 500})
	require.NoError(t, err)
	assert.Equal(t, 100, got.PerPage)
	assert.Equal(t, 0, got.TotalPages)
}

func TestAdminService_ListUsers_Error(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("ListUsers", mock.Anything, repositories.AdminUserFilters{Page: 1, Limit: 20}).
		Return(nil, 0, errors.New("boom"))

	_, err := NewAdminService(repo).ListUsers(context.Background(), repositories.AdminUserFilters{Page: 1, Limit: 20})
	require.Error(t, err)
}

func TestAdminService_GetUserDetail(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	want := &models.AdminUserDetail{ID: "u1", Memberships: []models.AdminTeamMembership{}}
	repo.On("GetUserDetail", mock.Anything, "u1").Return(want, nil)

	got, err := NewAdminService(repo).GetUserDetail(context.Background(), "u1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestAdminService_ListTeams_ClampsAndComputesPages(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("ListTeams", mock.Anything, repositories.AdminTeamFilters{Page: 1, Limit: 20}).
		Return([]models.AdminTeamListItem{{ID: "t1"}}, 21, nil)

	got, err := NewAdminService(repo).ListTeams(context.Background(), repositories.AdminTeamFilters{})
	require.NoError(t, err)
	assert.Equal(t, 1, got.Page)
	assert.Equal(t, 20, got.PerPage)
	assert.Equal(t, 21, got.TotalCount)
	assert.Equal(t, 2, got.TotalPages) // ceil(21/20)
	assert.Len(t, got.Teams, 1)
}

func TestAdminService_ListTeams_Error(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("ListTeams", mock.Anything, repositories.AdminTeamFilters{Page: 1, Limit: 20}).
		Return(nil, 0, errors.New("boom"))

	_, err := NewAdminService(repo).ListTeams(context.Background(), repositories.AdminTeamFilters{Page: 1, Limit: 20})
	require.Error(t, err)
}

func TestAdminService_GetTeamDetail(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	want := &models.AdminTeamDetail{ID: "t1", Members: []models.AdminTeamMember{}}
	repo.On("GetTeamDetail", mock.Anything, "t1").Return(want, nil)

	got, err := NewAdminService(repo).GetTeamDetail(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
