package services

import (
	"context"
	"math"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
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
	// GetDashboardOverview returns instance-wide totals, status/type breakdowns
	// and system health, stamped with the supplied app version.
	GetDashboardOverview(ctx context.Context, version string) (models.AdminDashboardOverview, error)
	// GetDashboardTimeseries returns gap-filled growth, sign-in and
	// access-by-source series for the requested range. An invalid range or
	// granularity yields *ErrAdminTimeseriesRange, which the handler maps to 400.
	GetDashboardTimeseries(
		ctx context.Context, q AdminTimeseriesQuery, window models.AdminDataWindow,
	) (models.AdminTimeseries, error)
	// SuspendUser blocks an account at every auth entry point. Returns
	// (nil, nil) for an unknown id (handler: 404); *ErrAdminSuspendSelf or
	// *ErrAdminSuspendInstanceAdmin for the two lockout guards (handler: 409).
	SuspendUser(
		ctx context.Context, actingAdminID, targetID string, isInstanceAdmin InstanceAdminPredicate,
	) (*models.AdminUserDetail, error)
	// ReactivateUser restores a suspended account. Returns (nil, nil) for an
	// unknown id (handler: 404).
	ReactivateUser(ctx context.Context, targetID string) (*models.AdminUserDetail, error)
	// UpdateUserName changes the only admin-editable user field and returns the
	// refreshed detail. (nil, nil) for an unknown id (handler: 404).
	UpdateUserName(ctx context.Context, targetID, name string) (*models.AdminUserDetail, error)
	// DeleteUser hard-deletes a user after the self / config-admin / owned-shared-
	// team guards. Returns (false, nil) for an unknown id (handler: 404), and
	// *ErrAdminDeleteSelf, *ErrAdminDeleteInstanceAdmin or *ErrAdminDeleteBlocked
	// (handler: 409) when refused — in every refusal case nothing is deleted.
	DeleteUser(
		ctx context.Context, actingAdminID, targetID string, isInstanceAdmin InstanceAdminPredicate,
	) (deletedEmail string, deleted bool, err error)
	// CreateUser creates a user and publishes `user.created` so the personal
	// workspace and default project are provisioned by the same listener
	// self-signup uses. Returns *ErrAdminUserEmailTaken (handler: 409) when the
	// email is already registered.
	CreateUser(ctx context.Context, req AdminUserCreateRequest) (*models.AdminUserDetail, error)
	// ListProjects returns a page of projects matching the filters with pagination
	// metadata over the filtered set. filters.Page/Limit are clamped.
	ListProjects(
		ctx context.Context, filters repositories.AdminProjectFilters,
	) (models.AdminProjectList, error)
	// GetProjectDetail returns one project with its team, owner and resource
	// counts, or (nil, nil) when no project with that id exists (handler: 404).
	GetProjectDetail(ctx context.Context, id string) (*models.AdminProjectDetail, error)
}

// AdminService implements AdminServiceInterface.
//
// userRepo and eventPublisher exist only for user CREATION (#462): a new user
// must be provisioned by the same `user.created` listener self-signup uses, so
// this service needs to write the users row and publish that event. Everything
// else goes through adminRepo. Both may be nil in a read-only wiring; CreateUser
// reports a clear error rather than panicking.
type AdminService struct {
	adminRepo      repositories.AdminRepository
	userRepo       repositories.UserRepository
	eventPublisher events.EventPublisher
}

// NewAdminService creates a new AdminService.
func NewAdminService(
	adminRepo repositories.AdminRepository,
	userRepo repositories.UserRepository,
	eventPublisher events.EventPublisher,
) AdminServiceInterface {
	return &AdminService{
		adminRepo:      adminRepo,
		userRepo:       userRepo,
		eventPublisher: eventPublisher,
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
