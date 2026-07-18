package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
)

// adminMsgInternalError is the generic problem detail for unexpected failures
// in the admin strict handlers.
const adminMsgInternalError = "Internal server error"

// adminStrictServer implements the generated Admin StrictServerInterface. The
// /api/v1/admin surface is guarded by instanceAdminMiddleware, so every request
// reaching these methods is already an authenticated instance admin.
type adminStrictServer struct {
	s *Server
}

var _ admingen.StrictServerInterface = (*adminStrictServer)(nil)

// GetAdminStats returns instance-wide entity counts plus the running app version.
func (a *adminStrictServer) GetAdminStats(
	ctx context.Context, _ admingen.GetAdminStatsRequestObject,
) (admingen.GetAdminStatsResponseObject, error) {
	counts, err := a.s.container.AdminService().GetInstanceCounts(ctx)
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName,
			"handler", "GetAdminStats",
			"error", err,
		).Error("Failed to get instance counts")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}

	version := a.s.config.Server.ServiceVersion
	if version == "" {
		version = "dev"
	}

	return admingen.GetAdminStats200JSONResponse(admingen.AdminStatsResponse{
		Counts: admingen.AdminInstanceCounts{
			Users:     counts.Users,
			Teams:     counts.Teams,
			Prompts:   counts.Prompts,
			Artifacts: counts.Artifacts,
			Memories:  counts.Memories,
		},
		Version: version,
	}), nil
}

// ListAdminUsers returns a paginated, instance-wide user listing with team counts.
func (a *adminStrictServer) ListAdminUsers(
	ctx context.Context, request admingen.ListAdminUsersRequestObject,
) (admingen.ListAdminUsersResponseObject, error) {
	page, limit := derefPageLimit(request.Params.Page, request.Params.Limit)

	list, err := a.s.container.AdminService().ListUsers(ctx, page, limit)
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "ListAdminUsers", "error", err,
		).Error("Failed to list admin users")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}

	genResp, convErr := toGenAdminUserList(list)
	if convErr != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "ListAdminUsers", "error", convErr,
		).Error("Failed to convert admin user list")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	return admingen.ListAdminUsers200JSONResponse(genResp), nil
}

// GetAdminUser returns one user with their team memberships; an unknown id 404s.
func (a *adminStrictServer) GetAdminUser(
	ctx context.Context, request admingen.GetAdminUserRequestObject,
) (admingen.GetAdminUserResponseObject, error) {
	detail, err := a.s.container.AdminService().GetUserDetail(ctx, request.Id.String())
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "GetAdminUser", "error", err,
		).Error("Failed to get admin user")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	if detail == nil {
		return nil, apierrors.NewResourceNotFoundError("user", "User not found")
	}

	genDetail, convErr := toGenAdminUserDetail(detail)
	if convErr != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "GetAdminUser", "error", convErr,
		).Error("Failed to convert admin user detail")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	return admingen.GetAdminUser200JSONResponse(genDetail), nil
}

// toGenAdminUserListItem converts a domain user-list row to the generated type,
// parsing the string id into a UUID.
func toGenAdminUserListItem(u models.AdminUserListItem) (admingen.AdminUserListItem, error) {
	id, err := uuid.Parse(u.ID)
	if err != nil {
		return admingen.AdminUserListItem{}, fmt.Errorf("user id %q is not a UUID: %w", u.ID, err)
	}
	return admingen.AdminUserListItem{
		Id:          id,
		Email:       openapi_types.Email(u.Email),
		Name:        u.Name,
		IdpProvider: u.IDPProvider,
		CreatedAt:   u.CreatedAt,
		TeamCount:   u.TeamCount,
	}, nil
}

// toGenAdminUserList converts a domain user page to the generated response. The
// users slice is always non-nil so the required array serializes as [], not null.
func toGenAdminUserList(l models.AdminUserList) (admingen.AdminUserListResponse, error) {
	users := make([]admingen.AdminUserListItem, 0, len(l.Users))
	for _, u := range l.Users {
		gu, err := toGenAdminUserListItem(u)
		if err != nil {
			return admingen.AdminUserListResponse{}, err
		}
		users = append(users, gu)
	}
	return admingen.AdminUserListResponse{
		Users:      users,
		TotalCount: l.TotalCount,
		Page:       l.Page,
		PerPage:    l.PerPage,
		TotalPages: l.TotalPages,
	}, nil
}

// toGenAdminUserDetail converts a domain user detail to the generated type. The
// memberships slice is always non-nil so the required array serializes as [].
func toGenAdminUserDetail(d *models.AdminUserDetail) (admingen.AdminUserDetail, error) {
	id, err := uuid.Parse(d.ID)
	if err != nil {
		return admingen.AdminUserDetail{}, fmt.Errorf("user id %q is not a UUID: %w", d.ID, err)
	}
	memberships := make([]admingen.AdminTeamMembership, 0, len(d.Memberships))
	for _, m := range d.Memberships {
		teamID, parseErr := uuid.Parse(m.TeamID)
		if parseErr != nil {
			return admingen.AdminUserDetail{}, fmt.Errorf("team id %q is not a UUID: %w", m.TeamID, parseErr)
		}
		memberships = append(memberships, admingen.AdminTeamMembership{
			TeamId:   teamID,
			TeamName: m.TeamName,
			Role:     m.Role,
		})
	}
	return admingen.AdminUserDetail{
		Id:          id,
		Email:       openapi_types.Email(d.Email),
		Name:        d.Name,
		IdpProvider: d.IDPProvider,
		CreatedAt:   d.CreatedAt,
		Memberships: memberships,
	}, nil
}

// ListAdminTeams returns a paginated, instance-wide team listing with owner and
// member counts.
func (a *adminStrictServer) ListAdminTeams(
	ctx context.Context, request admingen.ListAdminTeamsRequestObject,
) (admingen.ListAdminTeamsResponseObject, error) {
	page, limit := derefPageLimit(request.Params.Page, request.Params.Limit)

	list, err := a.s.container.AdminService().ListTeams(ctx, page, limit)
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "ListAdminTeams", "error", err,
		).Error("Failed to list admin teams")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}

	genResp, convErr := toGenAdminTeamList(list)
	if convErr != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "ListAdminTeams", "error", convErr,
		).Error("Failed to convert admin team list")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	return admingen.ListAdminTeams200JSONResponse(genResp), nil
}

// GetAdminTeam returns one team with owner and member list; an unknown id 404s.
func (a *adminStrictServer) GetAdminTeam(
	ctx context.Context, request admingen.GetAdminTeamRequestObject,
) (admingen.GetAdminTeamResponseObject, error) {
	detail, err := a.s.container.AdminService().GetTeamDetail(ctx, request.Id.String())
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "GetAdminTeam", "error", err,
		).Error("Failed to get admin team")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	if detail == nil {
		return nil, apierrors.NewResourceNotFoundError("team", "Team not found")
	}

	genDetail, convErr := toGenAdminTeamDetail(detail)
	if convErr != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "GetAdminTeam", "error", convErr,
		).Error("Failed to convert admin team detail")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	return admingen.GetAdminTeam200JSONResponse(genDetail), nil
}

// toGenAdminTeamOwner converts a domain team owner to the generated type.
func toGenAdminTeamOwner(o models.AdminTeamOwner) (admingen.AdminTeamOwner, error) {
	id, err := uuid.Parse(o.ID)
	if err != nil {
		return admingen.AdminTeamOwner{}, fmt.Errorf("team owner id %q is not a UUID: %w", o.ID, err)
	}
	return admingen.AdminTeamOwner{Id: id, Email: openapi_types.Email(o.Email), Name: o.Name}, nil
}

// toGenAdminTeamListItem converts a domain team-list row to the generated type.
func toGenAdminTeamListItem(t models.AdminTeamListItem) (admingen.AdminTeamListItem, error) {
	id, err := uuid.Parse(t.ID)
	if err != nil {
		return admingen.AdminTeamListItem{}, fmt.Errorf("team id %q is not a UUID: %w", t.ID, err)
	}
	owner, err := toGenAdminTeamOwner(t.Owner)
	if err != nil {
		return admingen.AdminTeamListItem{}, err
	}
	return admingen.AdminTeamListItem{
		Id:          id,
		Name:        t.Name,
		Owner:       owner,
		MemberCount: t.MemberCount,
		CreatedAt:   t.CreatedAt,
	}, nil
}

// toGenAdminTeamList converts a domain team page to the generated response. The
// teams slice is always non-nil so the required array serializes as [], not null.
func toGenAdminTeamList(l models.AdminTeamList) (admingen.AdminTeamListResponse, error) {
	teams := make([]admingen.AdminTeamListItem, 0, len(l.Teams))
	for _, t := range l.Teams {
		gt, err := toGenAdminTeamListItem(t)
		if err != nil {
			return admingen.AdminTeamListResponse{}, err
		}
		teams = append(teams, gt)
	}
	return admingen.AdminTeamListResponse{
		Teams:      teams,
		TotalCount: l.TotalCount,
		Page:       l.Page,
		PerPage:    l.PerPage,
		TotalPages: l.TotalPages,
	}, nil
}

// toGenAdminTeamMember converts a domain team member to the generated type.
func toGenAdminTeamMember(m models.AdminTeamMember) (admingen.AdminTeamMember, error) {
	uid, err := uuid.Parse(m.UserID)
	if err != nil {
		return admingen.AdminTeamMember{}, fmt.Errorf("team member user id %q is not a UUID: %w", m.UserID, err)
	}
	return admingen.AdminTeamMember{
		UserId:   uid,
		Email:    openapi_types.Email(m.Email),
		Name:     m.Name,
		Role:     m.Role,
		JoinedAt: m.JoinedAt,
	}, nil
}

// toGenAdminTeamDetail converts a domain team detail to the generated type. The
// members slice is always non-nil so the required array serializes as [].
func toGenAdminTeamDetail(d *models.AdminTeamDetail) (admingen.AdminTeamDetail, error) {
	id, err := uuid.Parse(d.ID)
	if err != nil {
		return admingen.AdminTeamDetail{}, fmt.Errorf("team id %q is not a UUID: %w", d.ID, err)
	}
	owner, err := toGenAdminTeamOwner(d.Owner)
	if err != nil {
		return admingen.AdminTeamDetail{}, err
	}
	members := make([]admingen.AdminTeamMember, 0, len(d.Members))
	for _, m := range d.Members {
		gm, memErr := toGenAdminTeamMember(m)
		if memErr != nil {
			return admingen.AdminTeamDetail{}, memErr
		}
		members = append(members, gm)
	}
	return admingen.AdminTeamDetail{
		Id:        id,
		Name:      d.Name,
		Owner:     owner,
		CreatedAt: d.CreatedAt,
		Members:   members,
	}, nil
}

// adminBindErrorHandler translates parameter-binding failures from the generated
// layer into this domain's RFC 9457 400 responses.
func (s *Server) adminBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(err.Error()))
}

// adminResponseErrorHandler writes errors returned by the strict handler
// implementations. *apierrors.APIError carries the intended RFC 9457 error;
// anything else is defensive and maps to a generic 500.
func (s *Server) adminResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.With("error", err).Error("Admin strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(adminMsgInternalError))
}
