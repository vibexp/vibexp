package services

import (
	"context"
	"math"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Instance-wide project reads for the admin surface (#453). Same shape as
// ListUsers/ListTeams: the service clamps pagination and computes the envelope
// from the FILTERED total; every other filter passes through untouched.

// ListProjects returns a page of filtered projects with pagination metadata
// computed over the filtered total.
func (s *AdminService) ListProjects(
	ctx context.Context, filters repositories.AdminProjectFilters,
) (models.AdminProjectList, error) {
	filters.Page, filters.Limit = clampAdminPage(filters.Page, filters.Limit)

	projects, totalCount, err := s.adminRepo.ListProjects(ctx, filters)
	if err != nil {
		return models.AdminProjectList{}, err
	}

	return models.AdminProjectList{
		Projects:   projects,
		TotalCount: totalCount,
		Page:       filters.Page,
		PerPage:    filters.Limit,
		TotalPages: int(math.Ceil(float64(totalCount) / float64(filters.Limit))),
	}, nil
}

// GetProjectDetail returns one project with team, owner and resource counts, or
// (nil, nil) when the project does not exist.
func (s *AdminService) GetProjectDetail(
	ctx context.Context, id string,
) (*models.AdminProjectDetail, error) {
	return s.adminRepo.GetProjectDetail(ctx, id)
}
