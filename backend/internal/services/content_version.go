package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ContentVersionAdapter registers a resource type with the content-versioning core.
// A resource type must be registered before it can be snapshotted, listed, or restored,
// which is what keeps the polymorphic core honest: ResourceType identifies the family of
// rows in content_versions, and RetentionCap bounds how many snapshots are kept.
type ContentVersionAdapter struct {
	ResourceType string
	RetentionCap int
	// InitialVersionLabel is the change summary rendered for the resource's first
	// version (version 1) when none was explicitly recorded. Under the snapshot-on-write
	// model version 1 always holds the originally-created content, so this labels the
	// "creation" entry (e.g. "Created the artifact"). Empty means no default.
	InitialVersionLabel string
}

// SnapshotRequest carries everything the core needs to record one version. Grouping the
// fields keeps the resource-agnostic contract readable as it grows (content, attribution,
// and the human-readable change metadata).
type SnapshotRequest struct {
	ResourceType string
	ResourceID   string
	TeamID       string
	UserID       string
	OldContent   string
	NewContent   string
	// ChangeSummary is the optional human-readable description of this change.
	ChangeSummary *string
	// ActorType is ActorTypeHuman or ActorTypeSystem; empty defaults to human.
	ActorType string
}

// ContentVersionServiceInterface is the business contract for the generic
// content-versioning core. It is resource-agnostic: callers pass a resourceType that
// must correspond to a registered adapter.
type ContentVersionServiceInterface interface {
	// SnapshotIfChanged records the prior content as the next version for the resource
	// when it differs from the new content, then prunes to the adapter's retention cap.
	// It is a no-op when the content is unchanged.
	SnapshotIfChanged(ctx context.Context, req SnapshotRequest) error
	// ListVersions returns the resource's versions newest-first, with authors resolved.
	ListVersions(ctx context.Context, teamID, resourceType, resourceID string) ([]*models.ContentVersion, error)
	// GetVersion returns a single version of the resource, with its author resolved.
	GetVersion(
		ctx context.Context, teamID, resourceType, resourceID string, versionNumber int,
	) (*models.ContentVersion, error)
	// Restore returns the content of the given version so the caller can apply it through
	// its normal update path (which snapshots the pre-restore content).
	Restore(
		ctx context.Context, teamID, resourceType, resourceID string, versionNumber int,
	) (string, error)
}

// ContentVersionService is the resource-agnostic content-versioning core. It holds a
// registry of adapters keyed by resource type; a resource is versionable iff its type
// is registered, which is the single extension point — new resource types add an adapter,
// not a code path.
type ContentVersionService struct {
	repo     repositories.ContentVersionRepository
	users    repositories.UserRepository
	logger   *slog.Logger
	registry map[string]ContentVersionAdapter
}

var _ ContentVersionServiceInterface = (*ContentVersionService)(nil)

// NewContentVersionService builds the service and its adapter registry from the given adapters.
// users resolves version authors for read responses; it may be nil (author enrichment is then skipped).
func NewContentVersionService(
	repo repositories.ContentVersionRepository,
	users repositories.UserRepository,
	logger *slog.Logger,
	adapters ...ContentVersionAdapter,
) *ContentVersionService {
	registry := make(map[string]ContentVersionAdapter, len(adapters))
	for _, a := range adapters {
		registry[a.ResourceType] = a
	}
	return &ContentVersionService{
		repo:     repo,
		users:    users,
		logger:   logger,
		registry: registry,
	}
}

func (s *ContentVersionService) adapterFor(resourceType string) (ContentVersionAdapter, error) {
	adapter, ok := s.registry[resourceType]
	if !ok {
		return ContentVersionAdapter{}, fmt.Errorf("unregistered resource type for versioning: %s", resourceType)
	}
	return adapter, nil
}

// SnapshotIfChanged snapshots the prior content (req.OldContent) as a new version and prunes
// to the adapter's retention cap. It no-ops when the content has not changed.
func (s *ContentVersionService) SnapshotIfChanged(ctx context.Context, req SnapshotRequest) error {
	if req.OldContent == req.NewContent {
		return nil
	}

	adapter, err := s.adapterFor(req.ResourceType)
	if err != nil {
		return err
	}

	var createdBy *string
	if req.UserID != "" {
		createdBy = &req.UserID
	}

	actorType := req.ActorType
	if actorType == "" {
		actorType = models.ActorTypeHuman
	}

	version := &models.ContentVersion{
		TeamID:        req.TeamID,
		ResourceType:  req.ResourceType,
		ResourceID:    req.ResourceID,
		Content:       req.OldContent,
		ChangeSummary: req.ChangeSummary,
		ActorType:     actorType,
		CreatedBy:     createdBy,
	}
	if err := s.repo.Create(ctx, version); err != nil {
		return fmt.Errorf("ContentVersionService.SnapshotIfChanged: %w", err)
	}

	if err := s.repo.PruneToCap(ctx, req.ResourceType, req.ResourceID, adapter.RetentionCap); err != nil {
		return fmt.Errorf("ContentVersionService.SnapshotIfChanged: prune: %w", err)
	}

	return nil
}

// ListVersions returns the resource's versions newest-first, with authors resolved and the
// initial-version label applied.
func (s *ContentVersionService) ListVersions(
	ctx context.Context, teamID, resourceType, resourceID string,
) ([]*models.ContentVersion, error) {
	adapter, err := s.adapterFor(resourceType)
	if err != nil {
		return nil, err
	}
	versions, err := s.repo.ListByResource(ctx, teamID, resourceType, resourceID)
	if err != nil {
		return nil, fmt.Errorf("ContentVersionService.ListVersions: %w", err)
	}
	s.enrich(ctx, adapter, versions)
	return versions, nil
}

// GetVersion returns a single version of the resource, with its author resolved.
func (s *ContentVersionService) GetVersion(
	ctx context.Context, teamID, resourceType, resourceID string, versionNumber int,
) (*models.ContentVersion, error) {
	adapter, err := s.adapterFor(resourceType)
	if err != nil {
		return nil, err
	}
	version, err := s.repo.GetByVersionNumber(ctx, teamID, resourceType, resourceID, versionNumber)
	if err != nil {
		return nil, fmt.Errorf("ContentVersionService.GetVersion: %w", err)
	}
	s.enrich(ctx, adapter, []*models.ContentVersion{version})
	return version, nil
}

// Restore returns the content of the target version. Applying it is the caller's job,
// done through the resource's normal update path so the pre-restore content is snapshotted.
func (s *ContentVersionService) Restore(
	ctx context.Context, teamID, resourceType, resourceID string, versionNumber int,
) (string, error) {
	if _, err := s.adapterFor(resourceType); err != nil {
		return "", err
	}
	version, err := s.repo.GetByVersionNumber(ctx, teamID, resourceType, resourceID, versionNumber)
	if err != nil {
		return "", fmt.Errorf("ContentVersionService.Restore: %w", err)
	}
	return version.Content, nil
}

// enrich decorates versions in place: it defaults the first version's change summary to
// the adapter's initial-version label and resolves each distinct author into Author. It is
// best-effort — a failed user lookup leaves Author nil rather than failing the read.
func (s *ContentVersionService) enrich(
	ctx context.Context, adapter ContentVersionAdapter, versions []*models.ContentVersion,
) {
	authors := make(map[string]*models.VersionAuthor)
	for _, v := range versions {
		if v == nil {
			continue
		}
		if v.ChangeSummary == nil && v.VersionNumber == 1 && adapter.InitialVersionLabel != "" {
			label := adapter.InitialVersionLabel
			v.ChangeSummary = &label
		}
		if v.CreatedBy != nil {
			v.Author = s.resolveAuthor(ctx, authors, *v.CreatedBy)
		}
	}
}

// resolveAuthor returns the render-ready author for userID, memoizing lookups (including
// negative ones) in the per-request cache so a list of N versions never issues more user
// fetches than there are distinct authors.
func (s *ContentVersionService) resolveAuthor(
	ctx context.Context, cache map[string]*models.VersionAuthor, userID string,
) *models.VersionAuthor {
	if s.users == nil {
		return nil
	}
	if author, ok := cache[userID]; ok {
		return author
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		s.logger.With("error", err).With("user_id", userID).
			Debug("content version author could not be resolved")
		cache[userID] = nil
		return nil
	}

	author := &models.VersionAuthor{
		ID:          user.ID,
		DisplayName: user.Name,
		AvatarURL:   user.AvatarURL,
		Initials:    initials(user.Name),
	}
	cache[userID] = author
	return author
}

// initials derives up to two uppercase initials from a display name, falling back to "?"
// when the name has no usable letters.
func initials(name string) string {
	fields := strings.Fields(name)
	var b strings.Builder
	for _, f := range fields {
		for _, r := range f {
			if unicode.IsLetter(r) || unicode.IsNumber(r) {
				b.WriteRune(unicode.ToUpper(r))
				break
			}
		}
		if b.Len() >= 2 {
			break
		}
	}
	if b.Len() == 0 {
		return "?"
	}
	return b.String()
}
