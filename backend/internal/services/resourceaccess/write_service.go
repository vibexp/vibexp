package resourceaccess

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// Resource-type keys of the creation metrics. They are the plural keys the
// per-day series already uses, not the singular resource_type of an event.
const (
	creationKeyPrompts    = "prompts"
	creationKeyArtifacts  = "artifacts"
	creationKeyBlueprints = "blueprints"
	creationKeyMemories   = "memories"
)

// SourceCreationCounts is how many resources of each type were created through
// one channel. Total is the sum of the four.
type SourceCreationCounts struct {
	Prompts    int
	Artifacts  int
	Blueprints int
	Memories   int
	Total      int
}

// CreationSources is the per-channel view of a creation-metrics window.
//
// Created always has the four write channels plus SourceUnknown, and for each
// resource type the five values sum to that type's creation total over the
// same window. Writes always has the four write channels, counting write
// events (creates and updates alike), plus SourceUnknown: the writes of the
// window with no event (#1418). Those are the creations Created reports under
// SourceUnknown and the content changes whose version snapshot has no source,
// so the five values sum to every write of the window the database has a
// record of.
type CreationSources struct {
	Created map[string]SourceCreationCounts
	Writes  map[string]int
}

// ResourceWriteService records resource write events (#1392) and reports them
// per channel for the creation metrics. It is the write-side twin of
// ResourceAccessService.
type ResourceWriteService interface {
	// RecordWrite records a resource write event asynchronously. It is
	// fire-and-forget: it never blocks the caller, never returns an error, and
	// never panics the caller even if persistence fails, so recording can never
	// fail the write it describes.
	//
	// The event is persisted on another goroutine, so callers must pass a
	// freshly-allocated event and must not mutate or reuse it afterwards.
	RecordWrite(event *models.ResourceWriteEvent)

	// GetTeamCreationSources returns the per-channel breakdown of the team's
	// resources created at or after since, and of its write events over the
	// same window. Team membership is authorized by the caller.
	GetTeamCreationSources(ctx context.Context, teamID string, since time.Time) (*CreationSources, error)

	// GetProjectCreationSources is GetTeamCreationSources for one project of
	// the team, identified by its slug. The caller resolves and authorizes the
	// project first.
	GetProjectCreationSources(
		ctx context.Context, teamID, projectSlug string, since time.Time,
	) (*CreationSources, error)
}

// WriteService implements ResourceWriteService.
type WriteService struct {
	repo      repositories.ResourceWriteEventRepository
	submitter taskSubmitter
	logger    *slog.Logger
}

var _ ResourceWriteService = (*WriteService)(nil)

// NewWriteService creates a new resource write service. It shares the access
// service's worker pool: both are off-request-path event writes.
func NewWriteService(
	repo repositories.ResourceWriteEventRepository,
	pool *events.WorkerPool,
	logger *slog.Logger,
) *WriteService {
	return &WriteService{repo: repo, submitter: pool, logger: logger}
}

// RecordWrite records a resource write event asynchronously. An event without a
// write channel is dropped rather than stored under a guessed one.
func (s *WriteService) RecordWrite(event *models.ResourceWriteEvent) {
	if event == nil || !IsWriteSource(event.Source) {
		return
	}

	s.submitter.Submit(func() {
		s.persistWrite(event)
	})
}

// persistWrite writes a single event using a fresh, time-bounded context. It is
// panic-safe and logs (Warn) on failure rather than propagating.
func (s *WriteService) persistWrite(event *models.ResourceWriteEvent) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.With("panic", r).Warn("recovered from panic while recording resource write event")
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), recordTimeout)
	defer cancel()

	if err := s.repo.Create(ctx, event); err != nil {
		s.logger.With("error", err).With(
			"team_id", event.TeamID,
			"resource_type", event.ResourceType,
			"resource_id", event.ResourceID,
			"action", event.Action,
			"source", event.Source,
		).Warn("failed to record resource write event")
	}
}

// GetTeamCreationSources returns the team's per-channel creation and write
// counts since the given time, zero-filled.
func (s *WriteService) GetTeamCreationSources(
	ctx context.Context, teamID string, since time.Time,
) (*CreationSources, error) {
	created, err := s.repo.GetTeamCreationCountsBySource(ctx, teamID, since)
	if err != nil {
		return nil, fmt.Errorf("ResourceWriteService.GetTeamCreationSources: %w", err)
	}
	writes, err := s.repo.GetTeamWriteCountsBySource(ctx, teamID, since)
	if err != nil {
		return nil, fmt.Errorf("ResourceWriteService.GetTeamCreationSources: writes: %w", err)
	}
	unsourcedUpdates, err := s.repo.GetTeamUnsourcedUpdateCount(ctx, teamID, since)
	if err != nil {
		return nil, fmt.Errorf("ResourceWriteService.GetTeamCreationSources: unsourced updates: %w", err)
	}
	return buildCreationSources(created, writes, unsourcedUpdates), nil
}

// GetProjectCreationSources returns one project's per-channel creation and
// write counts since the given time, zero-filled.
func (s *WriteService) GetProjectCreationSources(
	ctx context.Context, teamID, projectSlug string, since time.Time,
) (*CreationSources, error) {
	created, err := s.repo.GetProjectCreationCountsBySource(ctx, teamID, projectSlug, since)
	if err != nil {
		return nil, fmt.Errorf("ResourceWriteService.GetProjectCreationSources: %w", err)
	}
	writes, err := s.repo.GetProjectWriteCountsBySource(ctx, teamID, projectSlug, since)
	if err != nil {
		return nil, fmt.Errorf("ResourceWriteService.GetProjectCreationSources: writes: %w", err)
	}
	unsourcedUpdates, err := s.repo.GetProjectUnsourcedUpdateCount(ctx, teamID, projectSlug, since)
	if err != nil {
		return nil, fmt.Errorf("ResourceWriteService.GetProjectCreationSources: unsourced updates: %w", err)
	}
	return buildCreationSources(created, writes, unsourcedUpdates), nil
}

// buildCreationSources zero-fills the sparse repository rows so every channel
// is present even when its count is zero.
//
// A creation row whose source is not a write channel is folded into
// SourceUnknown rather than dropped: dropping it would break the one promise
// this breakdown makes, that a type's sources sum to its total. The table's
// CHECK constraint means no such row should exist.
//
// Writes[SourceUnknown] is the window's writes with no event: the creations
// counted under Created[SourceUnknown], plus unsourcedUpdates. A write event
// whose source is not a write channel is still dropped, not folded in: if it
// is a create event, its resource is already in Created[SourceUnknown], and
// folding the event in as well would count that one write twice.
func buildCreationSources(
	created []models.ResourceCreationSourceCount, writes []models.ResourceWriteSourceCount, unsourcedUpdates int,
) *CreationSources {
	result := &CreationSources{
		Created: make(map[string]SourceCreationCounts, len(writeSources)+1),
		Writes:  make(map[string]int, len(writeSources)+1),
	}
	for _, source := range writeSources {
		result.Created[source] = SourceCreationCounts{}
		result.Writes[source] = 0
	}
	result.Created[SourceUnknown] = SourceCreationCounts{}

	for _, row := range created {
		source := row.Source
		if !IsWriteSource(source) {
			source = SourceUnknown
		}
		counts := result.Created[source]
		switch row.ResourceType {
		case creationKeyPrompts:
			counts.Prompts += row.Count
		case creationKeyArtifacts:
			counts.Artifacts += row.Count
		case creationKeyBlueprints:
			counts.Blueprints += row.Count
		case creationKeyMemories:
			counts.Memories += row.Count
		default:
			continue
		}
		counts.Total += row.Count
		result.Created[source] = counts
	}

	result.Writes[SourceUnknown] = result.Created[SourceUnknown].Total + unsourcedUpdates
	for _, row := range writes {
		if IsWriteSource(row.Source) {
			result.Writes[row.Source] += row.Count
		}
	}

	return result
}
