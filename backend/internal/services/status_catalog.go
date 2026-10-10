package services

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ErrStatusResourceTypeUnsupported is returned by the status catalog when
// resource_type is not one of models.StatusResourceTypes.
var ErrStatusResourceTypeUnsupported = errors.New("resource_type has no status catalog")

// CatalogStatus is one entry of a team's status catalog for a resource type:
// either a system status, defined in Go, or one of the team's custom rows.
type CatalogStatus struct {
	// ID is the team_statuses row id of a custom status, and nil for a system
	// status, which has no row.
	ID *string
	// Slug is the value stored in the resource's status column.
	Slug string
	Name string
	// IsSystem is true for the statuses every team has and none can edit.
	IsSystem bool
	// IsLive is models.IsLiveStatus for this slug; always true for a custom
	// status.
	IsLive bool
	// CreatedAt and UpdatedAt are the custom row's timestamps, and nil for a
	// system status.
	CreatedAt *time.Time
	UpdatedAt *time.Time
}

// StatusCatalogService is the concrete StatusCatalogServiceInterface. It holds
// no cache: each call is one query on the (team_id, resource_type, slug)
// unique index, or none at all when the answer is a system status.
type StatusCatalogService struct {
	repo repositories.TeamStatusRepository
}

// NewStatusCatalogService creates a new StatusCatalogService.
func NewStatusCatalogService(repo repositories.TeamStatusRepository) *StatusCatalogService {
	return &StatusCatalogService{repo: repo}
}

// List returns the system statuses of resourceType in their declared order,
// then the team's custom statuses in the repository's order.
func (s *StatusCatalogService) List(
	ctx context.Context, teamID, resourceType string,
) ([]CatalogStatus, error) {
	system, err := systemStatusesFor(resourceType)
	if err != nil {
		return nil, err
	}
	custom, err := s.repo.List(ctx, teamID, resourceType)
	if err != nil {
		return nil, fmt.Errorf("failed to list team statuses: %w", err)
	}

	catalog := make([]CatalogStatus, 0, len(system)+len(custom))
	for _, slug := range system {
		catalog = append(catalog, CatalogStatus{
			Slug:     slug,
			Name:     systemStatusName(slug),
			IsSystem: true,
			IsLive:   models.IsLiveStatus(resourceType, slug),
		})
	}
	for i := range custom {
		row := &custom[i]
		// The system statuses are reserved slugs, so a custom row can only
		// carry one if it was written around the service. A slug must appear in
		// the catalog once, and the system definition is the one that holds.
		if slices.Contains(system, row.Slug) {
			continue
		}
		catalog = append(catalog, CatalogStatus{
			ID:        &row.ID,
			Slug:      row.Slug,
			Name:      row.Name,
			IsLive:    models.IsLiveStatus(resourceType, row.Slug),
			CreatedAt: &row.CreatedAt,
			UpdatedAt: &row.UpdatedAt,
		})
	}
	return catalog, nil
}

// IsAllowed reports whether status is a system status of resourceType or one
// of the team's custom statuses for it. A system status is answered without a
// query.
func (s *StatusCatalogService) IsAllowed(
	ctx context.Context, teamID, resourceType, status string,
) (bool, error) {
	system, err := systemStatusesFor(resourceType)
	if err != nil {
		return false, err
	}
	if status == "" {
		return false, nil
	}
	if slices.Contains(system, status) {
		return true, nil
	}
	custom, err := s.repo.List(ctx, teamID, resourceType)
	if err != nil {
		return false, fmt.Errorf("failed to list team statuses: %w", err)
	}
	return slices.ContainsFunc(custom, func(row models.TeamStatus) bool {
		return row.Slug == status
	}), nil
}

// AllowedSlugs returns the slugs of List, in the same order.
func (s *StatusCatalogService) AllowedSlugs(
	ctx context.Context, teamID, resourceType string,
) ([]string, error) {
	catalog, err := s.List(ctx, teamID, resourceType)
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(catalog))
	for i := range catalog {
		slugs = append(slugs, catalog[i].Slug)
	}
	return slugs, nil
}

// systemStatusesFor returns the system statuses of a supported resource type,
// or ErrStatusResourceTypeUnsupported.
func systemStatusesFor(resourceType string) ([]string, error) {
	if !models.IsStatusResourceType(resourceType) {
		return nil, fmt.Errorf("%w: %q", ErrStatusResourceTypeUnsupported, resourceType)
	}
	return models.SystemStatusesFor(resourceType), nil
}

// systemStatusName is the display name of a system status: its slug with the
// first letter capitalised ("active" -> "Active"). The system slugs are single
// lowercase ASCII words, so there is nothing else to title-case.
func systemStatusName(slug string) string {
	if slug == "" {
		return ""
	}
	return strings.ToUpper(slug[:1]) + slug[1:]
}
