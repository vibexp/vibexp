package services

import (
	"context"
	"errors"
	"log/slog"
	"regexp"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Type validation rules and sentinels. The handler maps these to 400 responses.
var (
	// ErrTypeSlugRequired is returned when a custom type's slug is empty.
	ErrTypeSlugRequired = errors.New("slug is required")
	// ErrTypeSlugInvalid is returned when a slug contains characters outside
	// [a-z0-9-].
	ErrTypeSlugInvalid = errors.New("slug must contain only lowercase letters, numbers, and hyphens")
	// ErrTypeSlugTooLong is returned when a slug exceeds the column length.
	ErrTypeSlugTooLong = errors.New("slug must be at most 255 characters")
	// ErrTypeNameRequired is returned when a custom type's display name is empty.
	ErrTypeNameRequired = errors.New("name is required")
	// ErrTypeNameTooLong is returned when a name exceeds the column length.
	ErrTypeNameTooLong = errors.New("name must be at most 255 characters")
	// ErrTypeResourceTypeUnsupported is returned when resource_type is not one of
	// the resources that support custom types.
	ErrTypeResourceTypeUnsupported = errors.New("resource_type is not supported")
)

const typeSlugMaxLen = 255
const typeNameMaxLen = 255

// typeSlugPattern enforces URL-safe, lowercase, hyphenated slugs.
var typeSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// resourceTypeDefaultSlug maps a supported resource_type to the system-default
// slug that orphaned resources are reassigned to when a custom type is deleted.
// Adding a resource here (plus the repository's reassignment branch) is all
// that is needed for it to adopt custom types.
var resourceTypeDefaultSlug = map[string]string{
	"artifacts": "general",
}

// TypeService is the concrete TypeServiceInterface implementation.
type TypeService struct {
	repo   repositories.TypeRepository
	logger *slog.Logger
}

// NewTypeService creates a new TypeService.
func NewTypeService(repo repositories.TypeRepository, logger *slog.Logger) *TypeService {
	return &TypeService{repo: repo, logger: logger}
}

func (s *TypeService) List(ctx context.Context, teamID, resourceType string) ([]models.Type, error) {
	if _, ok := resourceTypeDefaultSlug[resourceType]; !ok {
		return nil, ErrTypeResourceTypeUnsupported
	}
	return s.repo.List(ctx, teamID, resourceType)
}

func (s *TypeService) CreateCustom(ctx context.Context, params CreateTypeParams) (*models.Type, error) {
	if _, ok := resourceTypeDefaultSlug[params.ResourceType]; !ok {
		return nil, ErrTypeResourceTypeUnsupported
	}
	if err := validateTypeSlug(params.Slug); err != nil {
		return nil, err
	}
	if err := validateTypeName(params.Name); err != nil {
		return nil, err
	}

	// Reject collisions against a global default or an existing team row up
	// front (the team partial unique index does not cover global-row slugs);
	// Create itself is the race backstop.
	if _, err := s.repo.GetBySlug(ctx, params.TeamID, params.ResourceType, params.Slug); err == nil {
		return nil, repositories.ErrTypeAlreadyExists
	} else if !errors.Is(err, repositories.ErrTypeNotFound) {
		return nil, err
	}

	t := &models.Type{
		TeamID:       params.TeamID,
		ResourceType: params.ResourceType,
		Slug:         params.Slug,
		Name:         params.Name,
		CreatedBy:    params.UserID,
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *TypeService) Delete(ctx context.Context, teamID, id string) error {
	// "general" is the artifacts default; the repository only applies the
	// fallback to the row's own resource_type, so passing the artifacts default
	// is safe even once other resources adopt types.
	return s.repo.DeleteCustom(ctx, teamID, id, resourceTypeDefaultSlug["artifacts"])
}

func (s *TypeService) ValidateType(
	ctx context.Context, teamID, resourceType, slug string,
) (bool, error) {
	_, err := s.repo.GetBySlug(ctx, teamID, resourceType, slug)
	if err != nil {
		if errors.Is(err, repositories.ErrTypeNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func validateTypeSlug(slug string) error {
	switch {
	case slug == "":
		return ErrTypeSlugRequired
	case len(slug) > typeSlugMaxLen:
		return ErrTypeSlugTooLong
	case !typeSlugPattern.MatchString(slug):
		return ErrTypeSlugInvalid
	default:
		return nil
	}
}

func validateTypeName(name string) error {
	switch {
	case name == "":
		return ErrTypeNameRequired
	case len(name) > typeNameMaxLen:
		return ErrTypeNameTooLong
	default:
		return nil
	}
}
