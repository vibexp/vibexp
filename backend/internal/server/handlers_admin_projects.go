package server

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
)

// Admin project handlers (#453). Read-only; same shape as the users/teams
// listings.

// toAdminProjectFilters maps the generated query params onto the repository
// filter struct, validating the sort enums and clamping page/limit.
//
// The enum validation is explicit for the same reason it is on every other admin
// list: oapi-codegen binds an enum query param as a raw string and never calls
// the Valid() helper it generates, so an out-of-enum value would otherwise fall
// through to the default ordering instead of returning 400.
func toAdminProjectFilters(p admingen.ListAdminProjectsParams) (repositories.AdminProjectFilters, error) {
	page, limit := derefPageLimit(p.Page, p.Limit)
	filters := repositories.AdminProjectFilters{
		Search:      p.Search,
		CreatedFrom: p.CreatedFrom,
		CreatedTo:   p.CreatedTo,
		Page:        page,
		Limit:       limit,
	}

	if p.TeamId != nil {
		teamID := p.TeamId.String()
		filters.TeamID = &teamID
	}
	if p.SortBy != nil {
		if err := validateAdminSortEnum("sort_by", string(*p.SortBy), p.SortBy.Valid()); err != nil {
			return repositories.AdminProjectFilters{}, err
		}
		filters.SortBy = string(*p.SortBy)
	}
	if p.SortOrder != nil {
		if err := validateAdminSortEnum("sort_order", string(*p.SortOrder), p.SortOrder.Valid()); err != nil {
			return repositories.AdminProjectFilters{}, err
		}
		filters.SortOrder = string(*p.SortOrder)
	}

	return filters, nil
}

// ListAdminProjects returns a paginated, filtered, instance-wide project listing.
func (a *adminStrictServer) ListAdminProjects(
	ctx context.Context, request admingen.ListAdminProjectsRequestObject,
) (admingen.ListAdminProjectsResponseObject, error) {
	filters, err := toAdminProjectFilters(request.Params)
	if err != nil {
		return nil, err
	}

	list, err := a.s.container.AdminService().ListProjects(ctx, filters)
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "ListAdminProjects", "error", err,
		).Error("Failed to list admin projects")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}

	genResp, convErr := toGenAdminProjectList(list)
	if convErr != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "ListAdminProjects", "error", convErr,
		).Error("Failed to convert admin project list")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	return admingen.ListAdminProjects200JSONResponse(genResp), nil
}

// GetAdminProject returns one project with team, owner and resource counts; an
// unknown id 404s.
func (a *adminStrictServer) GetAdminProject(
	ctx context.Context, request admingen.GetAdminProjectRequestObject,
) (admingen.GetAdminProjectResponseObject, error) {
	detail, err := a.s.container.AdminService().GetProjectDetail(ctx, request.Id.String())
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "GetAdminProject", "error", err,
		).Error("Failed to get admin project")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	if detail == nil {
		return nil, apierrors.NewResourceNotFoundError("project", "Project not found")
	}

	genDetail, convErr := toGenAdminProjectDetail(detail)
	if convErr != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "GetAdminProject", "error", convErr,
		).Error("Failed to convert admin project detail")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	return admingen.GetAdminProject200JSONResponse(genDetail), nil
}

// toGenAdminProjectTeam converts the owning team.
func toGenAdminProjectTeam(t models.AdminProjectTeam) (admingen.AdminProjectTeam, error) {
	id, err := uuid.Parse(t.ID)
	if err != nil {
		return admingen.AdminProjectTeam{}, fmt.Errorf("project team id %q is not a UUID: %w", t.ID, err)
	}
	return admingen.AdminProjectTeam{Id: id, Name: t.Name, Slug: t.Slug}, nil
}

// toGenAdminProjectListItem converts one project-list row.
func toGenAdminProjectListItem(p models.AdminProjectListItem) (admingen.AdminProjectListItem, error) {
	id, err := uuid.Parse(p.ID)
	if err != nil {
		return admingen.AdminProjectListItem{}, fmt.Errorf("project id %q is not a UUID: %w", p.ID, err)
	}
	team, err := toGenAdminProjectTeam(p.Team)
	if err != nil {
		return admingen.AdminProjectListItem{}, err
	}
	owner, err := toGenAdminTeamOwner(p.Owner)
	if err != nil {
		return admingen.AdminProjectListItem{}, err
	}
	return admingen.AdminProjectListItem{
		Id:        id,
		Name:      p.Name,
		Slug:      p.Slug,
		Team:      team,
		Owner:     owner,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}, nil
}

// toGenAdminProjectList converts a project page. The projects slice is always
// non-nil so the required array serializes as [], never null (#125).
func toGenAdminProjectList(l models.AdminProjectList) (admingen.AdminProjectListResponse, error) {
	projects := make([]admingen.AdminProjectListItem, 0, len(l.Projects))
	for _, p := range l.Projects {
		gp, err := toGenAdminProjectListItem(p)
		if err != nil {
			return admingen.AdminProjectListResponse{}, err
		}
		projects = append(projects, gp)
	}
	return admingen.AdminProjectListResponse{
		Projects:   projects,
		TotalCount: l.TotalCount,
		Page:       l.Page,
		PerPage:    l.PerPage,
		TotalPages: l.TotalPages,
	}, nil
}

// toGenAdminProjectDetail converts a project detail.
func toGenAdminProjectDetail(d *models.AdminProjectDetail) (admingen.AdminProjectDetail, error) {
	id, err := uuid.Parse(d.ID)
	if err != nil {
		return admingen.AdminProjectDetail{}, fmt.Errorf("project id %q is not a UUID: %w", d.ID, err)
	}
	team, err := toGenAdminProjectTeam(d.Team)
	if err != nil {
		return admingen.AdminProjectDetail{}, err
	}
	owner, err := toGenAdminTeamOwner(d.Owner)
	if err != nil {
		return admingen.AdminProjectDetail{}, err
	}
	return admingen.AdminProjectDetail{
		Id:          id,
		Name:        d.Name,
		Slug:        d.Slug,
		Description: d.Description,
		GitUrl:      d.GitURL,
		Homepage:    d.Homepage,
		Team:        team,
		Owner:       owner,
		ResourceCounts: admingen.AdminProjectResourceCounts{
			Prompts:    d.ResourceCounts.Prompts,
			Artifacts:  d.ResourceCounts.Artifacts,
			Memories:   d.ResourceCounts.Memories,
			Blueprints: d.ResourceCounts.Blueprints,
		},
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	}, nil
}
