package services

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// ErrUnsupportedBackfillEntityType is returned when the request names an entity
// type the backfill cannot handle. Handlers map it to a 400.
var ErrUnsupportedBackfillEntityType = stderrors.New("unsupported backfill entity type")

// ErrBackfillScopeRequired is returned when a request specifies neither all nor a
// non-empty entity_types list. Handlers map it to a 400.
var ErrBackfillScopeRequired = stderrors.New("specify all=true or a non-empty entity_types")

// ErrBackfillScopeAmbiguous is returned when a request sets both all and a
// non-empty entity_types list. Handlers map it to a 400.
var ErrBackfillScopeAmbiguous = stderrors.New("all and entity_types are mutually exclusive")

// backfillPageSize bounds how many entities are read per page from the source
// tables. It caps memory per iteration while keeping the round-trip count low.
const backfillPageSize = 500

// backfillEntityTypes is the canonical, ordered set of embeddable entity types the
// backfill can republish. It matches the `.created` event types the embedding
// worker consumes, so a backfill regenerates exactly the embeddings the live
// pipeline produces.
var backfillEntityTypes = []string{
	"prompt", "artifact", "memory", "blueprint", "feed_item",
}

// EmbeddingBackfillRequest configures a backfill run. The scope is explicit: the
// caller must set either All or a non-empty EntityTypes, never both and never
// neither — Backfill returns ErrBackfillScopeRequired / ErrBackfillScopeAmbiguous
// otherwise so a missing scope can't silently fall through to a full run.
type EmbeddingBackfillRequest struct {
	// All backfills every supported entity type. Mutually exclusive with EntityTypes.
	All bool
	// EntityTypes restricts the run to a subset of the supported types. Mutually
	// exclusive with All.
	EntityTypes []string
	// MissingOnly restricts the run to entities that have no embedding row for the
	// currently configured model, so a backfill targets only the gaps a model swap
	// left behind.
	MissingOnly bool
	// DryRun counts the entities that would be republished without publishing any
	// event, so an operator can preview the blast radius. It honors MissingOnly.
	DryRun bool
	// TeamID, when set, restricts the run to that team's entities. Empty means all
	// teams. Used to re-embed a single team after its provider's model changes.
	TeamID string
}

// EmbeddingBackfillTypeResult is the per-entity-type outcome of a backfill run.
//
// Every counter here describes EVENT PUBLISHING only. Embeddings are generated
// asynchronously downstream (event bus → embedding worker → dispatcher → provider),
// so a clean result says the work was handed off, never that it landed (#755). Use
// EmbeddingCoverageGetter / the coverage endpoint for what is actually embedded.
type EmbeddingBackfillTypeResult struct {
	EntityType string `json:"entity_type"`
	// Total is the number of source entities seen for this type.
	Total int `json:"total"`
	// Published is the number of `.created` events successfully handed to the event
	// publisher. It equals Total on a clean run and Total minus Failed otherwise. On
	// a dry run it is 0 (nothing is published) while Total still reflects the
	// entities seen. An entity counted here is NOT yet embedded: generation happens
	// asynchronously and can still fail, or be lost with the in-memory job queue on
	// a restart (#820), without changing this number.
	Published int `json:"published"`
	// Failed is the number of entities whose event PUBLISH errored
	// (log-and-continue). It does not count entities that were published and then
	// failed to embed — those are invisible here by construction.
	Failed int `json:"failed"`
}

// EmbeddingBackfillResult aggregates a backfill run across all processed types.
//
// TotalPublished / TotalFailed are publish-side sums with exactly the semantics
// documented on EmbeddingBackfillTypeResult: TotalFailed == 0 means "every event
// was published", not "every entity was embedded" (#755).
type EmbeddingBackfillResult struct {
	DryRun  bool                          `json:"dry_run"`
	Results []EmbeddingBackfillTypeResult `json:"results"`
	// TotalSeen is the number of source entities the run enumerated.
	TotalSeen int `json:"total_seen"`
	// TotalPublished is the sum of per-type Published: events handed to the
	// publisher, not embeddings written.
	TotalPublished int `json:"total_published"`
	// TotalFailed is the sum of per-type Failed: publish failures only.
	TotalFailed int `json:"total_failed"`
}

// EmbeddingBackfiller regenerates embeddings for every embeddable
// entity by republishing each entity's `.created` event, letting the in-process
// embedding worker (chunk in Go → embed via the active provider → SaveEmbeddingChunks)
// rebuild the vectors. It is a permanent operational tool for model/dimension swaps.
type EmbeddingBackfiller interface {
	Backfill(ctx context.Context, req EmbeddingBackfillRequest) (*EmbeddingBackfillResult, error)
}

// PromptBodyRenderer resolves the @references and {{placeholders}} in a prompt
// body, mirroring what the live create path embeds. The backfill needs it so
// reference-using prompts are republished with their rendered content rather
// than the raw `{{...}}` template, keeping backfilled embeddings identical to
// the live pipeline's.
type PromptBodyRenderer interface {
	RenderPromptBody(userID, body string) (string, error)
}

// EmbeddingBackfillService implements EmbeddingBackfiller.
type EmbeddingBackfillService struct {
	repo           repositories.EmbeddingBackfillRepository
	publisher      events.EventPublisher
	promptRenderer PromptBodyRenderer
	// coverage, when set, is read once after a team-scoped run so the run's log
	// ends with what is actually embedded rather than only what was published
	// (#755). Optional: a nil coverage getter simply skips the snapshot, which
	// keeps the constructor usable in tests that do not exercise it.
	coverage EmbeddingCoverageGetter
	// modelID keys the missing_only NOT EXISTS filter. Embedding models are now
	// per-team (issue #79), so there is no single configured model: it is left
	// empty, which makes missing_only re-embed every entity (no model_id matches an
	// empty string). Re-embedding is idempotent (delete-then-insert), so a backfill
	// safely regenerates through each entity's team provider.
	modelID string
	logger  *slog.Logger
}

var _ EmbeddingBackfiller = (*EmbeddingBackfillService)(nil)

// NewEmbeddingBackfillService creates a new EmbeddingBackfillService. coverage is
// optional (may be nil): when supplied, a team-scoped run logs an embedding-coverage
// snapshot once it finishes publishing.
func NewEmbeddingBackfillService(
	repo repositories.EmbeddingBackfillRepository,
	publisher events.EventPublisher,
	promptRenderer PromptBodyRenderer,
	coverage EmbeddingCoverageGetter,
	logger *slog.Logger,
) *EmbeddingBackfillService {
	return &EmbeddingBackfillService{
		repo:           repo,
		publisher:      publisher,
		promptRenderer: promptRenderer,
		coverage:       coverage,
		modelID:        "",
		logger:         logger,
	}
}

// Backfill pages through every requested entity type and republishes each entity's
// `.created` event. Publish failures are logged and counted but never abort the run
// (matching the live services' log-and-continue semantics), so a single poison row
// cannot stall a multi-thousand-entity regeneration.
func (s *EmbeddingBackfillService) Backfill(
	ctx context.Context, req EmbeddingBackfillRequest,
) (*EmbeddingBackfillResult, error) {
	types, err := resolveBackfillTypes(req.All, req.EntityTypes)
	if err != nil {
		return nil, err
	}

	result := &EmbeddingBackfillResult{
		DryRun:  req.DryRun,
		Results: make([]EmbeddingBackfillTypeResult, 0, len(types)),
	}

	for _, entityType := range types {
		typeResult, err := s.backfillType(ctx, entityType, req.TeamID, req.MissingOnly, req.DryRun)
		if err != nil {
			return nil, fmt.Errorf("backfill of %s failed: %w", entityType, err)
		}
		result.Results = append(result.Results, typeResult)
		result.TotalSeen += typeResult.Total
		result.TotalPublished += typeResult.Published
		result.TotalFailed += typeResult.Failed
	}

	// Field names are unchanged so existing greps/alerts keep working; only the
	// human-readable message moves, because "completed" reads as an end-to-end
	// guarantee this run cannot make — total_failed counts publish failures, and
	// generation has barely started by the time this line is written (#755).
	s.logger.With(
		"dry_run", result.DryRun,
		"total_seen", result.TotalSeen,
		"total_published", result.TotalPublished,
		"total_failed", result.TotalFailed,
	).Info("Embedding backfill: events published (generation is async; counts are publishes, not embeddings)")

	s.logCoverageSnapshot(ctx, req)

	return result, nil
}

// logCoverageSnapshot logs the team's embedding coverage right after a run finishes
// publishing, so an operator draining a backlog gets one line saying what actually
// landed instead of only what was handed off (#755).
//
// It is deliberately a SNAPSHOT, not a completion assertion: generation is async, so
// this is read while the just-published events are still draining and will normally
// show a pending count. Its value is the trend across runs — a pending count that
// does not move between two reprocesses is the publish→generate gap this issue
// exists to surface.
//
// Skipped when no coverage getter is wired, on a dry run (nothing was published), or
// for an all-teams run (coverage is per-team and there is no team to report on). A
// coverage read failure is logged and swallowed: the backfill itself succeeded, and
// failing it over a diagnostic would be a strictly worse outcome.
func (s *EmbeddingBackfillService) logCoverageSnapshot(ctx context.Context, req EmbeddingBackfillRequest) {
	if s.coverage == nil || req.DryRun || req.TeamID == "" {
		return
	}

	coverage, err := s.coverage.GetCoverage(ctx, req.TeamID)
	if err != nil {
		s.logger.With(
			"team_id", req.TeamID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to read embedding coverage after backfill")
		return
	}

	logger := s.logger.With(
		"team_id", req.TeamID,
		"has_active_provider", coverage.HasActiveProvider,
	)
	for _, item := range coverage.Coverage {
		logger = logger.With(
			item.EntityType+"_embedded", item.Embedded,
			item.EntityType+"_total", item.Total,
			item.EntityType+"_pending", item.Pending,
		)
	}
	logger.Info("Embedding backfill: coverage after run (snapshot; generation is async)")
}

// backfillType pages through one entity type and republishes its `.created` events.
func (s *EmbeddingBackfillService) backfillType(
	ctx context.Context, entityType, teamID string, missingOnly, dryRun bool,
) (EmbeddingBackfillTypeResult, error) {
	res := EmbeddingBackfillTypeResult{EntityType: entityType}

	for offset := 0; ; offset += backfillPageSize {
		entities, err := s.repo.ListEntities(ctx, entityType, s.modelID, teamID, missingOnly, backfillPageSize, offset)
		if err != nil {
			return res, err
		}

		s.processPage(ctx, entities, dryRun, &res)

		if len(entities) < backfillPageSize {
			break
		}
	}

	return res, nil
}

// processPage tallies and (unless dryRun) republishes the `.created` event for each
// entity in one page, accumulating into res.
func (s *EmbeddingBackfillService) processPage(
	ctx context.Context, entities []models.BackfillEntity, dryRun bool, res *EmbeddingBackfillTypeResult,
) {
	for i := range entities {
		res.Total++
		if dryRun {
			continue
		}
		if s.publishEntity(ctx, &entities[i]) {
			res.Published++
		} else {
			res.Failed++
		}
	}
}

// publishEntity builds and publishes the entity's `.created` event, returning true
// on success. A publish error is logged and swallowed so the run continues.
func (s *EmbeddingBackfillService) publishEntity(ctx context.Context, e *models.BackfillEntity) bool {
	event, err := s.buildCreatedEvent(e)
	if err != nil {
		s.logger.With("error", err).
			With(
				"entity_type", e.EntityType,
				"entity_id", e.EntityID,
			).
			Warn("Failed to build created event during embedding backfill")
		return false
	}
	// Tag the event as backfill-origin so user-facing side-effect listeners
	// (notifications) skip it. The embedding forwarder routes by event type and is
	// unaffected, so regeneration still happens for every entity.
	event = events.MarkBackfillOrigin(event)
	if err := s.publisher.Publish(ctx, event); err != nil {
		s.logger.With("error", err).
			With(
				"entity_type", e.EntityType,
				"entity_id", e.EntityID,
			).
			Warn("Failed to republish created event during embedding backfill")
		return false
	}
	return true
}

// resolveBackfillTypes validates the explicit scope and normalizes the requested
// entity types. The scope must be exactly one of all=true or a non-empty
// entity_types list, never both and never neither, so a missing scope can't
// silently trigger a full backfill on this destructive endpoint.
func resolveBackfillTypes(all bool, requested []string) ([]string, error) {
	if all && len(requested) > 0 {
		return nil, ErrBackfillScopeAmbiguous
	}
	if !all && len(requested) == 0 {
		return nil, ErrBackfillScopeRequired
	}
	if all {
		return backfillEntityTypes, nil
	}

	supported := make(map[string]bool, len(backfillEntityTypes))
	for _, t := range backfillEntityTypes {
		supported[t] = true
	}

	// Preserve the canonical order and reject duplicates/unknowns up front so the
	// caller gets a clear 400 instead of a partially-applied run.
	seen := make(map[string]bool, len(requested))
	for _, t := range requested {
		if !supported[t] {
			return nil, fmt.Errorf("%w: %s", ErrUnsupportedBackfillEntityType, t)
		}
		seen[t] = true
	}

	resolved := make([]string, 0, len(seen))
	for _, t := range backfillEntityTypes {
		if seen[t] {
			resolved = append(resolved, t)
		}
	}
	return resolved, nil
}

// buildCreatedEvent reconstructs an entity's `.created` event from the stored
// fields, mirroring the argument shape each live service uses (e.g. project_id is
// passed as the "project name" argument). The entity type is one of the validated
// supported types; an unsupported type returns ErrUnsupportedBackfillEntityType.
func (s *EmbeddingBackfillService) buildCreatedEvent(e *models.BackfillEntity) (events.Event, error) {
	switch e.EntityType {
	case "prompt":
		return events.NewPromptCreatedEvent(events.PromptCreatedPayload{
			PromptID:    e.EntityID,
			UserID:      e.UserID,
			Email:       e.Email,
			ProjectName: e.ProjectName,
			Slug:        e.Slug,
			Title:       e.Title,
			Description: e.Description,
			Body:        s.renderPromptBody(e),
			CreatedAt:   e.CreatedAt,
		}), nil
	case "artifact":
		return events.NewArtifactCreatedEvent(events.ArtifactCreatedPayload{
			ArtifactID:  e.EntityID,
			UserID:      e.UserID,
			ProjectName: e.ProjectName,
			Slug:        e.Slug,
			Title:       e.Title,
			Description: e.Description,
			Type:        e.Type,
			Body:        e.Body,
			CreatedAt:   e.CreatedAt,
		}), nil
	case "memory":
		return events.NewMemoryCreatedEvent(e.EntityID, e.UserID, e.ProjectName, e.Body, e.CreatedAt), nil
	case "blueprint":
		return events.NewBlueprintCreatedEvent(events.BlueprintCreatedPayload{
			BlueprintID: e.EntityID,
			UserID:      e.UserID,
			ProjectName: e.ProjectName,
			Slug:        e.Slug,
			Title:       e.Title,
			Description: e.Description,
			Type:        e.Type,
			Body:        e.Body,
			CreatedAt:   e.CreatedAt,
		}), nil
	case "feed_item":
		return events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
			ItemID:   e.EntityID,
			UserID:   e.UserID,
			TeamID:   e.TeamID,
			FeedID:   e.FeedID,
			Title:    e.Title,
			Content:  e.Body,
			Excerpt:  e.Excerpt,
			PostedAt: e.CreatedAt,
		}), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedBackfillEntityType, e.EntityType)
	}
}

// renderPromptBody resolves a prompt's @references and {{placeholders}} so the
// backfill embeds the same rendered text the live create path does. A render
// failure falls back to the raw body (matching the live path's fallback), so a
// single unresolvable reference never aborts the run.
func (s *EmbeddingBackfillService) renderPromptBody(e *models.BackfillEntity) string {
	rendered, err := s.promptRenderer.RenderPromptBody(e.UserID, e.Body)
	if err != nil {
		s.logger.With("error", err).
			With(
				"entity_type", e.EntityType,
				"entity_id", e.EntityID,
			).
			Warn("Failed to render prompt body during backfill, using raw body instead")
		return e.Body
	}
	return rendered
}
