package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"time"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/blueprintpath"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/utils"
	"github.com/vibexp/vibexp/pkg/events"
)

// ErrInvalidBlueprintPath is returned when a create/update supplies a path that
// fails traversal validation. Handlers map it to a 400.
var ErrInvalidBlueprintPath = fmt.Errorf("invalid blueprint path")

// computeContentSHA returns the lowercase-hex SHA-256 of raw, matching the
// migration backfill (encode(digest(...,'sha256'),'hex')).
func computeContentSHA(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// blueprintSubtype returns the blueprint's subtype string ("" when unset).
func blueprintSubtype(bp *models.Blueprint) string {
	if bp.Subtype != nil {
		return *bp.Subtype
	}
	return ""
}

// deriveDefaultPath returns the canonical default path for a blueprint, falling
// back to "<slug>.md" for a (type, subtype) with no canonical default — matching
// the migration's ELSE branch.
func deriveDefaultPath(bp *models.Blueprint) string {
	p, err := blueprintpath.DefaultPath(bp.Type, blueprintSubtype(bp), bp.Slug)
	if err != nil {
		return bp.Slug + ".md"
	}
	return p
}

// resolveBlueprintPath sets bp.Path/PathDerived from an optional explicit path on
// CREATE. An explicit path is traversal-validated and freezes the path
// (PathDerived false); otherwise a default is derived (PathDerived true).
func resolveBlueprintPath(bp *models.Blueprint, explicit *string) error {
	if explicit != nil && *explicit != "" {
		if err := blueprintpath.ValidateRelativePath(*explicit); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidBlueprintPath, err)
		}
		bp.Path = *explicit
		bp.PathDerived = false
		return nil
	}
	bp.Path = deriveDefaultPath(bp)
	bp.PathDerived = true
	return nil
}

// applyUpdatePath resolves the path on UPDATE. An explicit path is validated and
// freezes the path (PathDerived false). With no explicit path, a still-derived
// path is recomputed from the (possibly renamed) slug/type/subtype — idempotent
// when nothing changed — while a frozen path (imported or previously overridden)
// is left untouched.
func applyUpdatePath(bp *models.Blueprint, explicit *string) error {
	if explicit != nil && *explicit != "" {
		if err := blueprintpath.ValidateRelativePath(*explicit); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidBlueprintPath, err)
		}
		bp.Path = *explicit
		bp.PathDerived = false
		return nil
	}
	if bp.PathDerived {
		bp.Path = deriveDefaultPath(bp)
	}
	return nil
}

// isSkillBlueprint reports whether bp is a claude-code/skills blueprint, whose
// Agent-Skill frontmatter name must match its directory name.
func isSkillBlueprint(bp *models.Blueprint) bool {
	return bp.Type == blueprintpath.TypeClaudeCode && blueprintSubtype(bp) == "skills"
}

// syncSkillName silently forces a claude-code/skills blueprint's Title (the
// Agent-Skill "name") to its path's directory name, so VibeXP can never produce
// a spec-invalid skill (epic #334 decision 6). No-op for other types. Call after
// the path is resolved.
func syncSkillName(bp *models.Blueprint) {
	if isSkillBlueprint(bp) {
		bp.Title = filepath.Base(filepath.Dir(bp.Path))
	}
}

// regenerateRaw deterministically rebuilds a blueprint's raw_content from its
// parsed parts (metadata + body), forcing the Agent-Skill "name" for skills, and
// returns the raw plus its SHA-256. A metadata-less blueprint regenerates to its
// body verbatim (no spurious frontmatter block). Deterministic + stable: a no-op
// edit reproduces byte-identical raw (guaranteed by #336's serializer
// idempotency), so content_sha stays a reliable "edited in VibeXP" signal.
func (s *BlueprintService) regenerateRaw(bp *models.Blueprint) (string, string) {
	meta := make(map[string]any, len(bp.Metadata)+1)
	for k, v := range bp.Metadata {
		meta[k] = v
	}
	if isSkillBlueprint(bp) {
		meta["name"] = bp.Title // = directory name after syncSkillName
	}
	var raw string
	if len(meta) == 0 {
		raw = bp.Content
	} else {
		raw = utils.SerializeFrontMatter(meta, bp.Content)
	}
	return raw, computeContentSHA(raw)
}

type BlueprintService struct {
	repo              repositories.BlueprintRepository
	teamService       TeamServiceInterface
	authz             AuthorizationServiceInterface
	eventManager      events.EventPublisher
	resourceUsageSvc  ResourceUsageServiceInterface
	contentVersionSvc ContentVersionServiceInterface
	commentRepo       repositories.CommentRepository
	relationRepo      repositories.RelationRepository
	logger            *slog.Logger
}

// Ensure BlueprintService implements BlueprintServiceInterface
var _ BlueprintServiceInterface = (*BlueprintService)(nil)

// BlueprintServiceDeps groups the dependencies injected into BlueprintService.
type BlueprintServiceDeps struct {
	Repo              repositories.BlueprintRepository
	TeamService       TeamServiceInterface
	Authz             AuthorizationServiceInterface
	EventManager      events.EventPublisher
	ResourceUsageSvc  ResourceUsageServiceInterface
	Logger            *slog.Logger
	ContentVersionSvc ContentVersionServiceInterface
	CommentRepo       repositories.CommentRepository
	RelationRepo      repositories.RelationRepository
}

func NewBlueprintService(deps BlueprintServiceDeps) *BlueprintService {
	return &BlueprintService{
		repo:              deps.Repo,
		teamService:       deps.TeamService,
		authz:             deps.Authz,
		eventManager:      deps.EventManager,
		resourceUsageSvc:  deps.ResourceUsageSvc,
		contentVersionSvc: deps.ContentVersionSvc,
		commentRepo:       deps.CommentRepo,
		relationRepo:      deps.RelationRepo,
		logger:            deps.Logger,
	}
}

type BlueprintFilters struct {
	ProjectID string
	Status    string
	Type      string
	Subtype   string
	TeamID    string
	Search    string
	SortBy    string
	SortOrder string
	Metadata  map[string]string
	Page      int
	Limit     int
}

// buildBlueprintFromRequest constructs a Blueprint from a create request with defaults
func buildBlueprintFromRequest(userID, teamID string, req *models.CreateBlueprintRequest) *models.Blueprint {
	status := req.Status
	if status == "" {
		status = "active"
	}

	blueprintType := req.Type
	if blueprintType == "" {
		blueprintType = "general"
	}

	metadata := req.Metadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	return &models.Blueprint{
		ProjectID:   req.ProjectID,
		Slug:        req.Slug,
		UserID:      userID,
		TeamID:      teamID,
		Content:     req.Content,
		Title:       req.Title,
		Description: req.Description,
		Status:      status,
		Type:        blueprintType,
		Subtype:     req.Subtype,
		Metadata:    metadata,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func (s *BlueprintService) CreateBlueprint(
	userID, teamID string, req *models.CreateBlueprintRequest,
) (*models.Blueprint, error) {
	ctx := context.Background()

	// Team ID comes from URL path and is already validated by middleware
	finalTeamID := teamID

	// Creating a resource is open to any team member (epic #220), but the caller
	// must still BE one — the middleware proves tenancy, this proves the role.
	if authzErr := s.authz.Can(ctx, userID, finalTeamID, authz.ResourceCreate); authzErr != nil {
		return nil, authzErr
	}

	blueprint := buildBlueprintFromRequest(userID, finalTeamID, req)

	// Resolve the canonical path (epic #334): explicit path is validated and
	// frozen; otherwise a default is derived from (type, subtype, slug).
	if err := resolveBlueprintPath(blueprint, req.Path); err != nil {
		return nil, err
	}
	// Skills force their frontmatter name to the directory name, then raw_content
	// is regenerated deterministically from the parsed parts.
	syncSkillName(blueprint)
	blueprint.RawContent, blueprint.ContentSHA = s.regenerateRaw(blueprint)

	// Validate business rules before creating
	err := validateBlueprintBusinessRules(blueprint)
	if err != nil {
		return nil, err
	}

	err = s.repo.Create(ctx, blueprint)
	if err != nil {
		s.logger.With(
			"service", "blueprint",
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create blueprint")
		return nil, err
	}

	// Publish blueprint created event
	if s.eventManager != nil {
		event := events.NewBlueprintCreatedEvent(events.BlueprintCreatedPayload{
			BlueprintID: blueprint.ID,
			UserID:      blueprint.UserID,
			ProjectName: blueprint.ProjectID,
			Slug:        blueprint.Slug,
			Title:       blueprint.Title,
			Description: blueprint.Description,
			Type:        blueprint.Type,
			Body:        blueprint.Content,
			CreatedAt:   blueprint.CreatedAt,
		})
		if err := s.eventManager.Publish(ctx, event); err != nil {
			s.logger.With("error", err).Warn("Failed to publish blueprint created event")
		}
	}

	return blueprint, nil
}

// GetBlueprintByIDInTeam retrieves a blueprint by its id, scoped to a single team the
// user belongs to. It backs the attachment owner authorizer for owner_type="blueprint":
// the universal attachments endpoint carries the blueprint id as owner_id, so access is
// verified by the same team-membership boundary the blueprint repo already enforces.
func (s *BlueprintService) GetBlueprintByIDInTeam(
	userID, teamID, blueprintID string,
) (*models.Blueprint, error) {
	blueprint, err := s.repo.GetByID(context.Background(), userID, teamID, blueprintID)
	if err != nil {
		s.logger.With(
			"service", "blueprint",
			"user_id", userID,
			"team_id", teamID,
			"blueprint_id", blueprintID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get blueprint by id (team-scoped)")
		return nil, err
	}

	return blueprint, nil
}

func (s *BlueprintService) GetBlueprintByProjectIDAndSlug(
	userID, projectID, slug string,
) (*models.Blueprint, error) {
	// Search across all user's teams
	blueprint, err := s.repo.GetByProjectIDAndSlugCrossTeam(context.Background(), userID, projectID, slug)
	if err != nil {
		s.logger.With(
			"service", "blueprint",
			"user_id", userID,
			"project_id", projectID,
			"slug", slug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get blueprint")
		return nil, err
	}

	return blueprint, nil
}

// GetBlueprintByProjectIDAndSlugInTeam retrieves a blueprint scoped to a single team the
// user belongs to. Unlike GetBlueprintByProjectIDAndSlug (which spans all of the user's
// teams by creator user_id), this enforces that the blueprint lives in teamID and is
// visible to any member of that team — so a non-creator member can open it (#258) — while
// a caller outside the team still gets not-found (tenancy preserved). Mirrors
// ArtifactService.GetArtifactByProjectIDAndSlugInTeam.
func (s *BlueprintService) GetBlueprintByProjectIDAndSlugInTeam(
	userID, teamID, projectID, slug string,
) (*models.Blueprint, error) {
	blueprint, err := s.repo.GetByProjectIDAndSlug(context.Background(), userID, teamID, projectID, slug)
	if err != nil {
		s.logger.With(
			"service", "blueprint",
			"user_id", userID,
			"team_id", teamID,
			"project_id", projectID,
			"slug", slug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get blueprint (team-scoped)")
		return nil, err
	}

	return blueprint, nil
}

func (s *BlueprintService) ListBlueprints(
	userID string, filters BlueprintFilters,
) (*models.BlueprintListResponse, error) {
	// Convert service filters to repository filters
	var projectID *string
	if filters.ProjectID != "" {
		projectID = &filters.ProjectID
	}

	var status *string
	if filters.Status != "" {
		status = &filters.Status
	}

	var blueprintType *string
	if filters.Type != "" {
		blueprintType = &filters.Type
	}

	var subtype *string
	if filters.Subtype != "" {
		subtype = &filters.Subtype
	}

	repoFilters := repositories.BlueprintFilters{
		ProjectID: projectID,
		Status:    status,
		Type:      blueprintType,
		Subtype:   subtype,
		TeamID:    filters.TeamID,
		Search:    filters.Search,
		SortBy:    filters.SortBy,
		SortOrder: filters.SortOrder,
		Metadata:  filters.Metadata,
		Page:      filters.Page,
		Limit:     filters.Limit,
	}

	blueprints, totalCount, err := s.repo.List(context.Background(), userID, repoFilters)
	if err != nil {
		s.logger.With(
			"service", "blueprint",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to list blueprints")
		return nil, err
	}

	// Calculate pagination
	totalPages := int(math.Ceil(float64(totalCount) / float64(filters.Limit)))

	return &models.BlueprintListResponse{
		Blueprints: blueprints,
		TotalCount: totalCount,
		Page:       filters.Page,
		PerPage:    filters.Limit,
		TotalPages: totalPages,
	}, nil
}

func (s *BlueprintService) ListBlueprintsByProject(
	userID, projectID string, filters BlueprintFilters,
) (*models.BlueprintListResponse, error) {
	filters.ProjectID = projectID
	return s.ListBlueprints(userID, filters)
}

// applyBlueprintUpdates applies the update request fields to the blueprint
func applyBlueprintUpdates(blueprint *models.Blueprint, req *models.UpdateBlueprintRequest) {
	if req.ProjectID != nil {
		blueprint.ProjectID = *req.ProjectID
	}
	if req.Slug != nil {
		blueprint.Slug = *req.Slug
	}
	if req.Title != nil {
		blueprint.Title = *req.Title
	}
	if req.Description != nil {
		blueprint.Description = *req.Description
	}
	if req.Content != nil {
		blueprint.Content = *req.Content
	}
	if req.Status != nil {
		blueprint.Status = *req.Status
	}
	if req.Type != nil {
		blueprint.Type = *req.Type
	}
	if req.Subtype != nil {
		blueprint.Subtype = req.Subtype
	}
	if req.Metadata != nil {
		blueprint.Metadata = req.Metadata
	}
	blueprint.UpdatedAt = time.Now()
}

// validateBlueprintBusinessRules validates business rules on the final merged blueprint
func validateBlueprintBusinessRules(blueprint *models.Blueprint) error {
	// Rule: sub-agents subtype requires model metadata
	if blueprint.Subtype != nil && *blueprint.Subtype == "sub-agents" {
		if blueprint.Metadata == nil {
			return fmt.Errorf("sub-agents subtype requires metadata with 'model' field")
		}
		modelVal, ok := blueprint.Metadata["model"].(string)
		if !ok || modelVal == "" {
			return fmt.Errorf("sub-agents subtype requires valid 'model' metadata value")
		}
	}
	return nil
}

func (s *BlueprintService) UpdateBlueprintByProjectIDAndSlug(
	userID, projectID, slug string, req *models.UpdateBlueprintRequest,
) (*models.Blueprint, error) {
	// First check if the blueprint exists and get current data
	blueprint, err := s.GetBlueprintByProjectIDAndSlug(userID, projectID, slug)
	if err != nil {
		return nil, err
	}

	return s.applyAndPersistBlueprintUpdate(userID, blueprint, req, models.ActorTypeHuman, nil, nil)
}

// UpdateBlueprintByProjectIDAndSlugInTeam updates a blueprint scoped to a single team the
// user belongs to. Unlike UpdateBlueprintByProjectIDAndSlug (which spans all of the user's
// teams by creator user_id), this resolves by team membership so resource.update.any (D1 —
// every role may update any team member's resource) reaches the update path instead of 404ing
// for a non-creator member (#258). Mirrors ArtifactService.UpdateArtifactByProjectIDAndSlugInTeam.
func (s *BlueprintService) UpdateBlueprintByProjectIDAndSlugInTeam(
	userID, teamID, projectID, slug string, req *models.UpdateBlueprintRequest,
) (*models.Blueprint, error) {
	blueprint, err := s.GetBlueprintByProjectIDAndSlugInTeam(userID, teamID, projectID, slug)
	if err != nil {
		return nil, err
	}

	return s.applyAndPersistBlueprintUpdate(userID, blueprint, req, models.ActorTypeHuman, nil, nil)
}

// applyAndPersistBlueprintUpdate applies the update request to an already-loaded blueprint,
// snapshots the pre-update content when it changed, persists the blueprint, and publishes the
// updated event. The blueprint must already have been loaded through an authorization-enforcing
// lookup. actorType and changeSummary describe the content-version snapshot: human edits pass
// (ActorTypeHuman, nil); a restore passes (ActorTypeSystem, "Restored Version N"). changeSummary
// is an internal snapshot attribute only — it is never read from UpdateBlueprintRequest, so the
// blueprint update API exposes no user-facing change-summary field (parity with artifacts).
// snapshotBlueprintContent records a best-effort content-version snapshot of the
// pre-update content when it changed. A snapshot failure must not fail the
// update (mirrors event publishing).
func (s *BlueprintService) snapshotBlueprintContent(
	ctx context.Context, userID string, blueprint *models.Blueprint,
	oldContent, oldRawContent, actorType string, changeSummary *string,
) {
	if s.contentVersionSvc == nil || oldContent == blueprint.Content {
		return
	}
	if err := s.contentVersionSvc.SnapshotIfChanged(ctx, SnapshotRequest{
		ResourceType:  "blueprint",
		ResourceID:    blueprint.ID,
		TeamID:        blueprint.TeamID,
		UserID:        userID,
		OldContent:    oldContent,
		NewContent:    blueprint.Content,
		OldRawContent: oldRawContent,
		ChangeSummary: changeSummary,
		ActorType:     actorType,
	}); err != nil {
		s.logger.With("error", err).Warn("Failed to snapshot blueprint content version")
	}
}

func (s *BlueprintService) applyAndPersistBlueprintUpdate(
	userID string, blueprint *models.Blueprint, req *models.UpdateBlueprintRequest,
	actorType string, changeSummary *string, rawOverride *string,
) (*models.Blueprint, error) {
	// Any member may update any blueprint, including another member's (epic #220
	// decision D1). Gated in the shared helper so the entry point and version
	// restore both funnel through one check.
	if authzErr := s.authz.Can(
		context.Background(), userID, blueprint.TeamID, authz.ResourceUpdateAny,
	); authzErr != nil {
		return nil, authzErr
	}

	ctx := context.Background()

	// Note: team_id cannot be changed via update (removed from UpdateBlueprintRequest)
	// Team reassignment is forbidden to prevent cross-team resource moves

	// Snapshot the prior content + raw before the update mutates them, so a
	// version history is built whenever the content actually changes.
	oldContent := blueprint.Content
	oldRawContent := blueprint.RawContent

	// Apply updates
	applyBlueprintUpdates(blueprint, req)

	// Resolve the path: explicit override freezes it; a still-derived path
	// recomputes from a renamed slug/type/subtype; a frozen path is untouched.
	if err := applyUpdatePath(blueprint, req.Path); err != nil {
		return nil, err
	}
	// Skills force their frontmatter name to the (possibly recomputed) directory.
	syncSkillName(blueprint)

	// Validate final merged state
	if err := validateBlueprintBusinessRules(blueprint); err != nil {
		return nil, err
	}

	// Regenerate raw_content deterministically from the edited parts, unless a
	// raw override is supplied (version restore reinstates the snapshotted raw
	// exactly rather than regenerating it).
	if rawOverride != nil {
		blueprint.RawContent = *rawOverride
		blueprint.ContentSHA = computeContentSHA(*rawOverride)
	} else {
		blueprint.RawContent, blueprint.ContentSHA = s.regenerateRaw(blueprint)
	}

	s.snapshotBlueprintContent(ctx, userID, blueprint, oldContent, oldRawContent, actorType, changeSummary)

	if err := s.repo.Update(ctx, blueprint); err != nil {
		s.logger.With(
			"service", "blueprint",
			"user_id", userID,
			"project_id", blueprint.ProjectID,
			"slug", blueprint.Slug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to update blueprint")
		return nil, err
	}

	s.publishBlueprintUpdatedEvent(ctx, blueprint)

	return blueprint, nil
}

// publishBlueprintUpdatedEvent emits the blueprint-updated event (best-effort;
// a publish failure is logged, never fatal).
func (s *BlueprintService) publishBlueprintUpdatedEvent(ctx context.Context, blueprint *models.Blueprint) {
	if s.eventManager == nil {
		return
	}
	event := events.NewBlueprintUpdatedEvent(events.BlueprintUpdatedPayload{
		BlueprintID: blueprint.ID,
		UserID:      blueprint.UserID,
		ProjectName: blueprint.ProjectID,
		Slug:        blueprint.Slug,
		Title:       blueprint.Title,
		Description: blueprint.Description,
		Type:        blueprint.Type,
		Body:        blueprint.Content,
		UpdatedAt:   blueprint.UpdatedAt,
	})
	if err := s.eventManager.Publish(ctx, event); err != nil {
		s.logger.With("error", err).Warn("Failed to publish blueprint updated event")
	}
}

// ListBlueprintVersions returns the content-version history for a blueprint, newest-first.
// The blueprint is loaded through the authorization-enforcing cross-team lookup before its
// versions are read; the resolved blueprint's TeamID scopes the version query.
// ListBlueprintVersionsInTeam returns the content-version history for a team-scoped blueprint,
// newest-first. The blueprint is loaded through the team-membership lookup so any member (not
// only the creator) can read its history (#258). Mirrors ArtifactService.ListArtifactVersionsInTeam.
func (s *BlueprintService) ListBlueprintVersionsInTeam(
	userID, teamID, projectID, slug string,
) ([]*models.ContentVersion, error) {
	blueprint, err := s.GetBlueprintByProjectIDAndSlugInTeam(userID, teamID, projectID, slug)
	if err != nil {
		return nil, err
	}
	return s.contentVersionSvc.ListVersions(context.Background(), blueprint.TeamID, "blueprint", blueprint.ID)
}

// GetBlueprintVersionInTeam returns a single content version of a team-scoped blueprint.
func (s *BlueprintService) GetBlueprintVersionInTeam(
	userID, teamID, projectID, slug string, versionNumber int,
) (*models.ContentVersion, error) {
	blueprint, err := s.GetBlueprintByProjectIDAndSlugInTeam(userID, teamID, projectID, slug)
	if err != nil {
		return nil, err
	}
	return s.contentVersionSvc.GetVersion(
		context.Background(), blueprint.TeamID, "blueprint", blueprint.ID, versionNumber,
	)
}

// RestoreBlueprintVersionInTeam restores a team-scoped blueprint's content to the given version
// by applying it through the shared update path, which snapshots the pre-restore content as a
// new version. Resolves by team membership so a non-creator member can restore (#258).
func (s *BlueprintService) RestoreBlueprintVersionInTeam(
	userID, teamID, projectID, slug string, versionNumber int,
) (*models.Blueprint, error) {
	blueprint, err := s.GetBlueprintByProjectIDAndSlugInTeam(userID, teamID, projectID, slug)
	if err != nil {
		return nil, err
	}

	// Fetch the full target version so its snapshotted raw_content can be
	// reinstated exactly (the normal update path regenerates raw; restore must
	// reproduce the old raw byte-for-byte).
	target, err := s.contentVersionSvc.GetVersion(
		context.Background(), blueprint.TeamID, "blueprint", blueprint.ID, versionNumber,
	)
	if err != nil {
		return nil, err
	}

	// A restore is a system-authored edit: the pre-restore content is snapshotted with a
	// default "Restored Version N" summary so the timeline reads clearly.
	restoreSummary := fmt.Sprintf("Restored Version %d", versionNumber)
	rawOverride := target.RawContent
	return s.applyAndPersistBlueprintUpdate(
		userID, blueprint, &models.UpdateBlueprintRequest{Content: &target.Content},
		models.ActorTypeSystem, &restoreSummary, &rawOverride,
	)
}

func (s *BlueprintService) DeleteBlueprintByProjectIDAndSlug(userID, teamID, projectID, slug string) error {
	// Resolve team-scoped (by membership, not creator user_id) so an Admin/Owner can reach
	// another member's blueprint and the delete.own/delete.any authorization below actually
	// runs — the owner-scoped getter returned 404 first, making delete.any dead (#258).
	blueprint, err := s.GetBlueprintByProjectIDAndSlugInTeam(userID, teamID, projectID, slug)
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Delete is own-vs-any: members delete only what they authored, Admin+ delete
	// anyone's. The repository no longer carries that decision in its SQL.
	if authzErr := s.authz.CanActOnResource(
		ctx, userID, blueprint.TeamID, blueprint.UserID,
		authz.ResourceDeleteOwn, authz.ResourceDeleteAny,
	); authzErr != nil {
		return authzErr
	}

	err = s.repo.Delete(ctx, userID, blueprint.TeamID, blueprint.ID)
	if err != nil {
		s.logger.With(
			"service", "blueprint",
			"user_id", userID,
			"project_id", projectID,
			"slug", slug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to delete blueprint")
		return err
	}

	s.deleteBlueprintComments(ctx, blueprint.TeamID, blueprint.ID)
	s.deleteBlueprintRelations(ctx, blueprint.TeamID, blueprint.ID)

	return nil
}

// deleteBlueprintComments removes a blueprint's comments after it is deleted.
// Best-effort: a failure is logged but does not fail the completed delete.
func (s *BlueprintService) deleteBlueprintComments(ctx context.Context, teamID, blueprintID string) {
	if s.commentRepo == nil {
		return
	}
	if _, err := s.commentRepo.DeleteByResource(
		ctx, teamID, models.CommentResourceTypeBlueprint, blueprintID,
	); err != nil {
		s.logger.With(
			"service", "blueprint",
			"team_id", teamID,
			"blueprint_id", blueprintID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to delete comments for deleted blueprint")
	}
}

// deleteBlueprintRelations removes every relation the blueprint appears on
// (either endpoint) after it is deleted. Best-effort, like the comment cascade.
func (s *BlueprintService) deleteBlueprintRelations(ctx context.Context, teamID, blueprintID string) {
	if s.relationRepo == nil {
		return
	}
	if _, err := s.relationRepo.DeleteByResource(
		ctx, teamID, models.RelationResourceTypeBlueprint, blueprintID,
	); err != nil {
		s.logger.With(
			"service", "blueprint",
			"team_id", teamID,
			"blueprint_id", blueprintID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to delete relations for deleted blueprint")
	}
}

func (s *BlueprintService) GetBlueprintStats(userID string) (*models.BlueprintStatsResponse, error) {
	stats, err := s.repo.GetStats(context.Background(), userID)
	if err != nil {
		s.logger.With(
			"service", "blueprint",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to get blueprint stats")
		return nil, err
	}

	return stats, nil
}
