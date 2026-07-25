package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/repositories"
)

// ErrInvalidMetadataCatalogQuery is returned when a catalog lookup is asked for
// something outside the closed set of resource types, or with a key that
// cannot identify anything. Handlers map it to 400.
var ErrInvalidMetadataCatalogQuery = errors.New("invalid metadata catalog query")

// MetadataCatalogServiceInterface exposes the discovery half of metadata
// filtering: which keys a team uses, and which values sit under one of them.
//
// Membership is enforced by the tenancy middleware and, in depth, by the
// repository's read-access predicate; there is no role check, because reading
// the catalog is exactly as privileged as reading the list it describes.
type MetadataCatalogServiceInterface interface {
	Keys(ctx context.Context, query repositories.MetadataCatalogQuery) (repositories.MetadataCatalogResult, error)
	Values(ctx context.Context, query repositories.MetadataCatalogQuery) (repositories.MetadataCatalogResult, error)
}

// MetadataCatalogService is the default MetadataCatalogServiceInterface.
type MetadataCatalogService struct {
	repo   repositories.MetadataCatalogRepository
	logger *slog.Logger
}

var _ MetadataCatalogServiceInterface = (*MetadataCatalogService)(nil)

// NewMetadataCatalogService creates a new MetadataCatalogService.
func NewMetadataCatalogService(
	repo repositories.MetadataCatalogRepository, logger *slog.Logger,
) *MetadataCatalogService {
	return &MetadataCatalogService{repo: repo, logger: logger}
}

// Keys returns the distinct metadata keys in use for the requested resource type.
func (s *MetadataCatalogService) Keys(
	ctx context.Context, query repositories.MetadataCatalogQuery,
) (repositories.MetadataCatalogResult, error) {
	if err := validateMetadataCatalogQuery(query); err != nil {
		return repositories.MetadataCatalogResult{}, err
	}
	return s.repo.Keys(ctx, query)
}

// Values returns the distinct values stored under query.Key.
func (s *MetadataCatalogService) Values(
	ctx context.Context, query repositories.MetadataCatalogQuery,
) (repositories.MetadataCatalogResult, error) {
	if err := validateMetadataCatalogQuery(query); err != nil {
		return repositories.MetadataCatalogResult{}, err
	}
	if query.Key == "" {
		return repositories.MetadataCatalogResult{}, fmt.Errorf("%w: key is required", ErrInvalidMetadataCatalogQuery)
	}
	if len(query.Key) > repositories.MaxMetadataFilterKeyLength {
		return repositories.MetadataCatalogResult{}, fmt.Errorf(
			"%w: key length must be at most %d characters",
			ErrInvalidMetadataCatalogQuery, repositories.MaxMetadataFilterKeyLength,
		)
	}
	return s.repo.Values(ctx, query)
}

// validateMetadataCatalogQuery rejects anything the repository's closed table
// map could not resolve, so an unknown resource type never reaches SQL
// construction.
func validateMetadataCatalogQuery(query repositories.MetadataCatalogQuery) error {
	if _, ok := repositories.ParseMetadataResourceType(string(query.ResourceType)); !ok {
		return fmt.Errorf("%w: unknown resource_type %q", ErrInvalidMetadataCatalogQuery, query.ResourceType)
	}
	if query.TeamID == "" {
		return fmt.Errorf("%w: team_id is required", ErrInvalidMetadataCatalogQuery)
	}
	return nil
}
