package services

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

type BlueprintService struct {
	repo              repositories.BlueprintRepository
	teamService       TeamServiceInterface
	eventManager      events.EventPublisher
	resourceUsageSvc  ResourceUsageServiceInterface
	contentVersionSvc ContentVersionServiceInterface
	logger            *logrus.Logger
}

// Ensure BlueprintService implements BlueprintServiceInterface
var _ BlueprintServiceInterface = (*BlueprintService)(nil)

func NewBlueprintService(
	repo repositories.BlueprintRepository,
	teamService TeamServiceInterface,
	eventManager events.EventPublisher,
	resourceUsageSvc ResourceUsageServiceInterface,
	logger *logrus.Logger,
	contentVersionSvc ContentVersionServiceInterface,
) *BlueprintService {
	return &BlueprintService{
		repo:              repo,
		teamService:       teamService,
		eventManager:      eventManager,
		resourceUsageSvc:  resourceUsageSvc,
		contentVersionSvc: contentVersionSvc,
		logger:            logger,
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

	blueprint := buildBlueprintFromRequest(userID, finalTeamID, req)

	// Validate business rules before creating
	err := validateBlueprintBusinessRules(blueprint)
	if err != nil {
		return nil, err
	}

	err = s.repo.Create(ctx, blueprint)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "blueprint",
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to create blueprint")
		return nil, err
	}

	// Publish blueprint created event
	if s.eventManager != nil {
		event := events.NewBlueprintCreatedEvent(
			blueprint.ID, blueprint.UserID, blueprint.ProjectID, blueprint.Slug,
			blueprint.Title, blueprint.Type, blueprint.Content, blueprint.CreatedAt,
		)
		if err := s.eventManager.Publish(ctx, event); err != nil {
			s.logger.WithError(err).Warn("Failed to publish blueprint created event")
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
		s.logger.WithFields(logrus.Fields{
			"service":      "blueprint",
			"user_id":      userID,
			"team_id":      teamID,
			"blueprint_id": blueprintID,
			"error":        fmt.Sprintf("%+v", err),
		}).Error("Failed to get blueprint by id (team-scoped)")
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
		s.logger.WithFields(logrus.Fields{
			"service":    "blueprint",
			"user_id":    userID,
			"project_id": projectID,
			"slug":       slug,
			"error":      fmt.Sprintf("%+v", err),
		}).Error("Failed to get blueprint")
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
		s.logger.WithFields(logrus.Fields{
			"service": "blueprint",
			"user_id": userID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to list blueprints")
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

	return s.applyAndPersistBlueprintUpdate(userID, blueprint, req, models.ActorTypeHuman, nil)
}

// applyAndPersistBlueprintUpdate applies the update request to an already-loaded blueprint,
// snapshots the pre-update content when it changed, persists the blueprint, and publishes the
// updated event. The blueprint must already have been loaded through an authorization-enforcing
// lookup. actorType and changeSummary describe the content-version snapshot: human edits pass
// (ActorTypeHuman, nil); a restore passes (ActorTypeSystem, "Restored Version N"). changeSummary
// is an internal snapshot attribute only — it is never read from UpdateBlueprintRequest, so the
// blueprint update API exposes no user-facing change-summary field (parity with artifacts).
func (s *BlueprintService) applyAndPersistBlueprintUpdate(
	userID string, blueprint *models.Blueprint, req *models.UpdateBlueprintRequest,
	actorType string, changeSummary *string,
) (*models.Blueprint, error) {
	ctx := context.Background()

	// Note: team_id cannot be changed via update (removed from UpdateBlueprintRequest)
	// Team reassignment is forbidden to prevent cross-team resource moves

	// Snapshot the prior content before the update mutates it, so a version
	// history is built whenever the content actually changes.
	oldContent := blueprint.Content

	// Apply updates
	applyBlueprintUpdates(blueprint, req)

	// Validate final merged state
	if err := validateBlueprintBusinessRules(blueprint); err != nil {
		return nil, err
	}

	// Best-effort content-version snapshot: record the pre-update content when it
	// changed. A snapshot failure must not fail the update (mirrors event publishing).
	if s.contentVersionSvc != nil && oldContent != blueprint.Content {
		if err := s.contentVersionSvc.SnapshotIfChanged(ctx, SnapshotRequest{
			ResourceType:  "blueprint",
			ResourceID:    blueprint.ID,
			TeamID:        blueprint.TeamID,
			UserID:        userID,
			OldContent:    oldContent,
			NewContent:    blueprint.Content,
			ChangeSummary: changeSummary,
			ActorType:     actorType,
		}); err != nil {
			s.logger.WithError(err).Warn("Failed to snapshot blueprint content version")
		}
	}

	if err := s.repo.Update(ctx, blueprint); err != nil {
		s.logger.WithFields(logrus.Fields{
			"service":    "blueprint",
			"user_id":    userID,
			"project_id": blueprint.ProjectID,
			"slug":       blueprint.Slug,
			"error":      fmt.Sprintf("%+v", err),
		}).Error("Failed to update blueprint")
		return nil, err
	}

	// Publish blueprint updated event
	if s.eventManager != nil {
		event := events.NewBlueprintUpdatedEvent(
			blueprint.ID, blueprint.UserID, blueprint.ProjectID, blueprint.Slug,
			blueprint.Title, blueprint.Type, blueprint.Content, blueprint.UpdatedAt,
		)
		if err := s.eventManager.Publish(ctx, event); err != nil {
			s.logger.WithError(err).Warn("Failed to publish blueprint updated event")
		}
	}

	return blueprint, nil
}

// ListBlueprintVersions returns the content-version history for a blueprint, newest-first.
// The blueprint is loaded through the authorization-enforcing cross-team lookup before its
// versions are read; the resolved blueprint's TeamID scopes the version query.
func (s *BlueprintService) ListBlueprintVersions(
	userID, projectID, slug string,
) ([]*models.ContentVersion, error) {
	blueprint, err := s.GetBlueprintByProjectIDAndSlug(userID, projectID, slug)
	if err != nil {
		return nil, err
	}
	return s.contentVersionSvc.ListVersions(context.Background(), blueprint.TeamID, "blueprint", blueprint.ID)
}

// GetBlueprintVersion returns a single content version of a blueprint.
func (s *BlueprintService) GetBlueprintVersion(
	userID, projectID, slug string, versionNumber int,
) (*models.ContentVersion, error) {
	blueprint, err := s.GetBlueprintByProjectIDAndSlug(userID, projectID, slug)
	if err != nil {
		return nil, err
	}
	return s.contentVersionSvc.GetVersion(
		context.Background(), blueprint.TeamID, "blueprint", blueprint.ID, versionNumber,
	)
}

// RestoreBlueprintVersion restores a blueprint's content to the given version by applying it
// through the shared update path, which snapshots the pre-restore content as a new version.
func (s *BlueprintService) RestoreBlueprintVersion(
	userID, projectID, slug string, versionNumber int,
) (*models.Blueprint, error) {
	blueprint, err := s.GetBlueprintByProjectIDAndSlug(userID, projectID, slug)
	if err != nil {
		return nil, err
	}

	target, err := s.contentVersionSvc.Restore(
		context.Background(), blueprint.TeamID, "blueprint", blueprint.ID, versionNumber,
	)
	if err != nil {
		return nil, err
	}

	// A restore is a system-authored edit: the pre-restore content is snapshotted with a
	// default "Restored Version N" summary so the timeline reads clearly.
	restoreSummary := fmt.Sprintf("Restored Version %d", versionNumber)
	return s.applyAndPersistBlueprintUpdate(
		userID, blueprint, &models.UpdateBlueprintRequest{Content: &target},
		models.ActorTypeSystem, &restoreSummary,
	)
}

func (s *BlueprintService) DeleteBlueprintByProjectIDAndSlug(userID, projectID, slug string) error {
	// First get the blueprint to get its ID and TeamID
	blueprint, err := s.GetBlueprintByProjectIDAndSlug(userID, projectID, slug)
	if err != nil {
		return err
	}

	ctx := context.Background()
	err = s.repo.Delete(ctx, userID, blueprint.TeamID, blueprint.ID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service":    "blueprint",
			"user_id":    userID,
			"project_id": projectID,
			"slug":       slug,
			"error":      fmt.Sprintf("%+v", err),
		}).Error("Failed to delete blueprint")
		return err
	}

	return nil
}

func (s *BlueprintService) GetBlueprintStats(userID string) (*models.BlueprintStatsResponse, error) {
	stats, err := s.repo.GetStats(context.Background(), userID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "blueprint",
			"user_id": userID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to get blueprint stats")
		return nil, err
	}

	return stats, nil
}
