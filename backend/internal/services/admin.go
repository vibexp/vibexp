package services

import (
	"context"
	"math"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

const (
	adminDefaultListLimit = 20
	adminMaxListLimit     = 100
)

// AdminServiceInterface exposes instance-level administrative reads. It backs
// the /api/v1/admin surface (guarded by instance-admin auth at the transport
// layer) and is intentionally separate from the team-scoped services and from
// the machine-only BackofficeService.
type AdminServiceInterface interface {
	// GetInstanceCounts returns instance-wide totals for the top-level entities.
	GetInstanceCounts(ctx context.Context) (models.InstanceCounts, error)
	// ListUsers returns a page of users matching the filters with team counts and
	// pagination metadata over the filtered set. filters.Page/Limit are clamped
	// (page>=1, limit in [1, 100], default 20); every other filter is passed
	// through to the repository unchanged.
	ListUsers(ctx context.Context, filters repositories.AdminUserFilters) (models.AdminUserList, error)
	// GetUserDetail returns one user with team memberships, or (nil, nil) when no
	// user with that id exists (the handler maps that to 404).
	GetUserDetail(ctx context.Context, id string) (*models.AdminUserDetail, error)
	// ListTeams returns a page of teams matching the filters with owner and member
	// count plus pagination metadata over the filtered set. filters.Page/Limit are
	// clamped (page>=1, limit in [1,100], default 20).
	ListTeams(ctx context.Context, filters repositories.AdminTeamFilters) (models.AdminTeamList, error)
	// GetTeamDetail returns one team with owner and member list, or (nil, nil)
	// when no team with that id exists (the handler maps that to 404).
	GetTeamDetail(ctx context.Context, id string) (*models.AdminTeamDetail, error)
}

// AdminService implements AdminServiceInterface.
type AdminService struct {
	adminRepo repositories.AdminRepository
}

// NewAdminService creates a new AdminService.
func NewAdminService(adminRepo repositories.AdminRepository) AdminServiceInterface {
	return &AdminService{
		adminRepo: adminRepo,
	}
}

// GetInstanceCounts returns instance-wide entity totals from the repository.
func (s *AdminService) GetInstanceCounts(ctx context.Context) (models.InstanceCounts, error) {
	return s.adminRepo.GetInstanceCounts(ctx)
}

// clampAdminPage normalizes page/limit to safe bounds (page>=1, limit in
// [1, adminMaxListLimit], defaulting to adminDefaultListLimit).
func clampAdminPage(page, limit int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = adminDefaultListLimit
	} else if limit > adminMaxListLimit {
		limit = adminMaxListLimit
	}
	return page, limit
}

// ListUsers returns a page of filtered users with team counts and pagination
// metadata computed over the filtered total.
func (s *AdminService) ListUsers(
	ctx context.Context, filters repositories.AdminUserFilters,
) (models.AdminUserList, error) {
	filters.Page, filters.Limit = clampAdminPage(filters.Page, filters.Limit)

	users, totalCount, err := s.adminRepo.ListUsers(ctx, filters)
	if err != nil {
		return models.AdminUserList{}, err
	}

	return models.AdminUserList{
		Users:      users,
		TotalCount: totalCount,
		Page:       filters.Page,
		PerPage:    filters.Limit,
		TotalPages: int(math.Ceil(float64(totalCount) / float64(filters.Limit))),
	}, nil
}

// GetUserDetail returns one user with team memberships, or (nil, nil) when the
// user does not exist.
func (s *AdminService) GetUserDetail(ctx context.Context, id string) (*models.AdminUserDetail, error) {
	return s.adminRepo.GetUserDetail(ctx, id)
}

// ListTeams returns a page of filtered teams with owner and member count plus
// pagination metadata computed over the filtered total.
func (s *AdminService) ListTeams(
	ctx context.Context, filters repositories.AdminTeamFilters,
) (models.AdminTeamList, error) {
	filters.Page, filters.Limit = clampAdminPage(filters.Page, filters.Limit)

	teams, totalCount, err := s.adminRepo.ListTeams(ctx, filters)
	if err != nil {
		return models.AdminTeamList{}, err
	}

	return models.AdminTeamList{
		Teams:      teams,
		TotalCount: totalCount,
		Page:       filters.Page,
		PerPage:    filters.Limit,
		TotalPages: int(math.Ceil(float64(totalCount) / float64(filters.Limit))),
	}, nil
}

// GetTeamDetail returns one team with owner and member list, or (nil, nil) when
// the team does not exist.
func (s *AdminService) GetTeamDetail(ctx context.Context, id string) (*models.AdminTeamDetail, error) {
	return s.adminRepo.GetTeamDetail(ctx, id)
}
