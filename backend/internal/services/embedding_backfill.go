package services

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/sirupsen/logrus"

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
// backfill can republish. It matches the `.created` event types forwarded to
// Pub/Sub (config.GetPubSubForwardedEventTypes) so a backfill regenerates exactly
// the embeddings the live pipeline produces.
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
}

// EmbeddingBackfillTypeResult is the per-entity-type outcome of a backfill run.
type EmbeddingBackfillTypeResult struct {
	EntityType string `json:"entity_type"`
	// Total is the number of source entities seen for this type.
	Total int `json:"total"`
	// Published is the number of `.created` events successfully republished. It
	// equals Total on a clean run and Total minus Failed otherwise. On a dry run it
	// is 0 (nothing is published) while Total still reflects the entities seen.
	Published int `json:"published"`
	// Failed is the number of entities whose event publish errored (log-and-continue).
	Failed int `json:"failed"`
}

// EmbeddingBackfillResult aggregates a backfill run across all processed types.
type EmbeddingBackfillResult struct {
	DryRun         bool                          `json:"dry_run"`
	Results        []EmbeddingBackfillTypeResult `json:"results"`
	TotalSeen      int                           `json:"total_seen"`
	TotalPublished int                           `json:"total_published"`
	TotalFailed    int                           `json:"total_failed"`
}

// EmbeddingBackfillServiceInterface regenerates embeddings for every embeddable
// entity by republishing each entity's `.created` event, letting the existing
// pipeline (ai-service chunking → `<entity>.embedding.generated` → SaveEmbeddingChunks)
// rebuild the vectors. It is a permanent operational tool for model/dimension swaps.
type EmbeddingBackfillServiceInterface interface {
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

// EmbeddingBackfillService implements EmbeddingBackfillServiceInterface.
type EmbeddingBackfillService struct {
	repo           repositories.EmbeddingBackfillRepository
	publisher      events.EventPublisher
	promptRenderer PromptBodyRenderer
	// modelID is the currently configured embedding model. It keys the missing_only
	// NOT EXISTS filter so a backfill targets entities lacking an embedding for the
	// model the live pipeline now writes.
	modelID string
	logger  *logrus.Logger
}

var _ EmbeddingBackfillServiceInterface = (*EmbeddingBackfillService)(nil)

// NewEmbeddingBackfillService creates a new EmbeddingBackfillService. modelID is the
// configured embedding model id (config.EmbeddingModel) used to key the missing_only
// filter.
func NewEmbeddingBackfillService(
	repo repositories.EmbeddingBackfillRepository,
	publisher events.EventPublisher,
	promptRenderer PromptBodyRenderer,
	modelID string,
	logger *logrus.Logger,
) *EmbeddingBackfillService {
	return &EmbeddingBackfillService{
		repo:           repo,
		publisher:      publisher,
		promptRenderer: promptRenderer,
		modelID:        modelID,
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
		typeResult, err := s.backfillType(ctx, entityType, req.MissingOnly, req.DryRun)
		if err != nil {
			return nil, fmt.Errorf("backfill of %s failed: %w", entityType, err)
		}
		result.Results = append(result.Results, typeResult)
		result.TotalSeen += typeResult.Total
		result.TotalPublished += typeResult.Published
		result.TotalFailed += typeResult.Failed
	}

	s.logger.WithFields(logrus.Fields{
		"dry_run":         result.DryRun,
		"total_seen":      result.TotalSeen,
		"total_published": result.TotalPublished,
		"total_failed":    result.TotalFailed,
	}).Info("Embedding backfill completed")

	return result, nil
}

// backfillType pages through one entity type and republishes its `.created` events.
func (s *EmbeddingBackfillService) backfillType(
	ctx context.Context, entityType string, missingOnly, dryRun bool,
) (EmbeddingBackfillTypeResult, error) {
	res := EmbeddingBackfillTypeResult{EntityType: entityType}

	for offset := 0; ; offset += backfillPageSize {
		entities, err := s.repo.ListEntities(ctx, entityType, s.modelID, missingOnly, backfillPageSize, offset)
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
		s.logger.WithError(err).WithFields(logrus.Fields{
			"entity_type": e.EntityType,
			"entity_id":   e.EntityID,
		}).Warn("Failed to build created event during embedding backfill")
		return false
	}
	// Tag the event as backfill-origin so user-facing side-effect listeners (CRM,
	// notifications) skip it. The embedding forwarder routes by event type and is
	// unaffected, so regeneration still happens for every entity.
	event = events.MarkBackfillOrigin(event)
	if err := s.publisher.Publish(ctx, event); err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"entity_type": e.EntityType,
			"entity_id":   e.EntityID,
		}).Warn("Failed to republish created event during embedding backfill")
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
		return events.NewPromptCreatedEvent(
			e.EntityID, e.UserID, e.Email, e.ProjectName, e.Slug, e.Title,
			s.renderPromptBody(e), e.CreatedAt,
		), nil
	case "artifact":
		return events.NewArtifactCreatedEvent(
			e.EntityID, e.UserID, e.ProjectName, e.Slug, e.Title, e.Type, e.Body, e.CreatedAt,
		), nil
	case "memory":
		return events.NewMemoryCreatedEvent(e.EntityID, e.UserID, e.ProjectName, e.Body, e.CreatedAt), nil
	case "blueprint":
		return events.NewBlueprintCreatedEvent(
			e.EntityID, e.UserID, e.ProjectName, e.Slug, e.Title, e.Type, e.Body, e.CreatedAt,
		), nil
	case "feed_item":
		return events.NewFeedItemCreatedEvent(
			e.EntityID, e.UserID, e.TeamID, e.FeedID, e.Title, e.Body, e.Excerpt, e.CreatedAt,
		), nil
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
		s.logger.WithError(err).WithFields(logrus.Fields{
			"entity_type": e.EntityType,
			"entity_id":   e.EntityID,
		}).Warn("Failed to render prompt body during backfill, using raw body instead")
		return e.Body
	}
	return rendered
}
