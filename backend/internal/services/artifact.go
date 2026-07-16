package services

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

type ArtifactService struct {
	repo              repositories.ArtifactRepository
	teamService       TeamServiceInterface
	authz             AuthorizationServiceInterface
	eventManager      events.EventPublisher
	resourceUsageSvc  ResourceUsageServiceInterface
	contentVersionSvc ContentVersionServiceInterface
	logger            *slog.Logger
}

// Ensure ArtifactService implements ArtifactServiceInterface
var _ ArtifactServiceInterface = (*ArtifactService)(nil)

func NewArtifactService(
	repo repositories.ArtifactRepository,
	teamService TeamServiceInterface,
	authzService AuthorizationServiceInterface,
	eventManager events.EventPublisher,
	resourceUsageSvc ResourceUsageServiceInterface,
	logger *slog.Logger,
	contentVersionSvc ContentVersionServiceInterface,
) *ArtifactService {
	return &ArtifactService{
		repo:              repo,
		teamService:       teamService,
		authz:             authzService,
		eventManager:      eventManager,
		resourceUsageSvc:  resourceUsageSvc,
		contentVersionSvc: contentVersionSvc,
		logger:            logger,
	}
}

type ArtifactFilters struct {
	ProjectID string
	Status    string
	Type      string
	TeamID    string
	Search    string
	SortBy    string
	SortOrder string
	Metadata  map[string]string
	Page      int
	Limit     int
}

// validateAndResolveTeamID validates team membership and returns the final team ID to use
func (s *ArtifactService) validateAndResolveTeamID(
	ctx context.Context, userID, defaultTeamID string, requestedTeamID *string,
) (string, error) {
	if requestedTeamID == nil || *requestedTeamID == "" {
		return defaultTeamID, nil
	}

	if s.teamService == nil {
		return *requestedTeamID, nil
	}

	isMember, err := s.teamService.IsUserMemberOfTeam(ctx, userID, *requestedTeamID)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", *requestedTeamID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to validate team membership")
		return "", fmt.Errorf("failed to validate team membership")
	}

	if !isMember {
		s.logger.With(
			"user_id", userID,
			"team_id", *requestedTeamID,
		).
			Warn("User attempted to create artifact in team they are not a member of")
		return "", fmt.Errorf("user is not a member of the specified team")
	}

	return *requestedTeamID, nil
}

// buildArtifactFromRequest assembles a new artifact, applying the defaults the
// API leaves optional (status, type, metadata).
func buildArtifactFromRequest(
	userID, teamID string, req *models.CreateArtifactRequest,
) *models.Artifact {
	status := req.Status
	if status == "" {
		status = models.ArtifactStatusActive
	}

	artifactType := req.Type
	if artifactType == "" {
		artifactType = "general"
	}

	metadata := req.Metadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	now := time.Now()
	return &models.Artifact{
		ProjectID:   req.ProjectID,
		Slug:        req.Slug,
		UserID:      userID,
		TeamID:      teamID,
		Content:     req.Content,
		Title:       req.Title,
		Description: req.Description,
		Status:      status,
		Type:        artifactType,
		Metadata:    metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func (s *ArtifactService) CreateArtifact(
	userID, teamID string, req *models.CreateArtifactRequest,
) (*models.Artifact, error) {
	ctx := context.Background()

	// Validate and resolve team ID
	finalTeamID, err := s.validateAndResolveTeamID(ctx, userID, teamID, nil)
	if err != nil {
		return nil, err
	}

	// Creating a resource is open to any team member (epic #220). Authorize the
	// RESOLVED team — that is where the artifact actually lands.
	if authzErr := s.authz.Can(ctx, userID, finalTeamID, authz.ResourceCreate); authzErr != nil {
		return nil, authzErr
	}

	artifact := buildArtifactFromRequest(userID, finalTeamID, req)

	err = s.repo.Create(ctx, artifact)
	if err != nil {
		s.logger.With(
			"service", "artifact",
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create artifact")
		return nil, err
	}

	// Publish artifact created event
	if s.eventManager != nil {
		event := events.NewArtifactCreatedEvent(
			artifact.ID, artifact.UserID, artifact.ProjectID, artifact.Slug,
			artifact.Title, artifact.Description, artifact.Type, artifact.Content, artifact.CreatedAt,
		)
		if err := s.eventManager.Publish(ctx, event); err != nil {
			s.logger.With("error", err).Warn("Failed to publish artifact created event")
		}
	}

	return artifact, nil
}

func (s *ArtifactService) GetArtifactByProjectIDAndSlug(userID, projectID, slug string) (*models.Artifact, error) {
	// Search across all user's teams
	artifact, err := s.repo.GetByProjectIDAndSlugCrossTeam(context.Background(), userID, projectID, slug)
	if err != nil {
		s.logger.With(
			"service", "artifact",
			"user_id", userID,
			"project_id", projectID,
			"slug", slug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get artifact")
		return nil, err
	}

	return artifact, nil
}

// GetArtifactByProjectIDAndSlugInTeam retrieves an artifact scoped to a single team the
// user belongs to. Unlike GetArtifactByProjectIDAndSlug (which spans all of the user's
// teams), this enforces that the artifact lives in teamID, so a caller cannot reach an
// artifact in another of their teams by supplying a different team_id.
func (s *ArtifactService) GetArtifactByProjectIDAndSlugInTeam(
	userID, teamID, projectID, slug string,
) (*models.Artifact, error) {
	artifact, err := s.repo.GetByProjectIDAndSlug(context.Background(), userID, teamID, projectID, slug)
	if err != nil {
		s.logger.With(
			"service", "artifact",
			"user_id", userID,
			"team_id", teamID,
			"project_id", projectID,
			"slug", slug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get artifact (team-scoped)")
		return nil, err
	}

	return artifact, nil
}

// GetArtifactByIDInTeam retrieves an artifact by its id, scoped to a single team the
// user belongs to. It backs the attachment owner authorizer for owner_type="artifact":
// the universal attachments endpoint carries the artifact id as owner_id, so access is
// verified by the same team-membership boundary as GetArtifactByProjectIDAndSlugInTeam.
func (s *ArtifactService) GetArtifactByIDInTeam(
	userID, teamID, artifactID string,
) (*models.Artifact, error) {
	artifact, err := s.repo.GetByID(context.Background(), userID, teamID, artifactID)
	if err != nil {
		s.logger.With(
			"service", "artifact",
			"user_id", userID,
			"team_id", teamID,
			"artifact_id", artifactID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get artifact by id (team-scoped)")
		return nil, err
	}

	return artifact, nil
}

func (s *ArtifactService) ListArtifacts(userID string, filters ArtifactFilters) (*models.ArtifactListResponse, error) {
	// Convert service filters to repository filters
	var projectID *string
	if filters.ProjectID != "" {
		projectID = &filters.ProjectID
	}

	var status *string
	if filters.Status != "" {
		status = &filters.Status
	}

	var artifactType *string
	if filters.Type != "" {
		artifactType = &filters.Type
	}

	repoFilters := repositories.ArtifactFilters{
		ProjectID: projectID,
		Status:    status,
		Type:      artifactType,
		TeamID:    filters.TeamID,
		Search:    filters.Search,
		SortBy:    filters.SortBy,
		SortOrder: filters.SortOrder,
		Metadata:  filters.Metadata,
		Page:      filters.Page,
		Limit:     filters.Limit,
	}

	artifacts, totalCount, err := s.repo.List(context.Background(), userID, repoFilters)
	if err != nil {
		s.logger.With(
			"service", "artifact",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to list artifacts")
		return nil, err
	}

	// Calculate pagination
	totalPages := int(math.Ceil(float64(totalCount) / float64(filters.Limit)))

	return &models.ArtifactListResponse{
		Artifacts:  artifacts,
		TotalCount: totalCount,
		Page:       filters.Page,
		PerPage:    filters.Limit,
		TotalPages: totalPages,
	}, nil
}

func (s *ArtifactService) ListArtifactsByProject(
	userID, projectID string, filters ArtifactFilters,
) (*models.ArtifactListResponse, error) {
	filters.ProjectID = projectID
	return s.ListArtifacts(userID, filters)
}

// ListArtifactsByProjectCrossTeam lists artifacts for a project across all teams the user owns.
// No TeamID is required — uses user_id ownership (mirrors GetArtifactByProjectIDAndSlug semantics).
func (s *ArtifactService) ListArtifactsByProjectCrossTeam(
	userID, projectID string, filters ArtifactFilters,
) (*models.ArtifactListResponse, error) {
	var projectIDPtr *string
	if projectID != "" {
		projectIDPtr = &projectID
	}

	var status *string
	if filters.Status != "" {
		status = &filters.Status
	}

	var artifactType *string
	if filters.Type != "" {
		artifactType = &filters.Type
	}

	repoFilters := repositories.ArtifactFilters{
		ProjectID: projectIDPtr,
		Status:    status,
		Type:      artifactType,
		Search:    filters.Search,
		SortBy:    filters.SortBy,
		SortOrder: filters.SortOrder,
		Metadata:  filters.Metadata,
		Page:      filters.Page,
		Limit:     filters.Limit,
	}

	artifacts, totalCount, err := s.repo.ListCrossTeam(context.Background(), userID, repoFilters)
	if err != nil {
		s.logger.With(
			"service", "artifact",
			"user_id", userID,
			"project_id", projectID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list artifacts (cross-team)")
		return nil, err
	}

	totalPages := int(math.Ceil(float64(totalCount) / float64(filters.Limit)))

	return &models.ArtifactListResponse{
		Artifacts:  artifacts,
		TotalCount: totalCount,
		Page:       filters.Page,
		PerPage:    filters.Limit,
		TotalPages: totalPages,
	}, nil
}

// applyArtifactUpdates applies update request fields to the artifact
func applyArtifactUpdates(artifact *models.Artifact, req *models.UpdateArtifactRequest) {
	if req.ProjectID != nil {
		artifact.ProjectID = *req.ProjectID
	}
	if req.Slug != nil {
		artifact.Slug = *req.Slug
	}
	if req.Title != nil {
		artifact.Title = *req.Title
	}
	if req.Description != nil {
		artifact.Description = *req.Description
	}
	if req.Content != nil {
		artifact.Content = *req.Content
	}
	if req.Status != nil {
		artifact.Status = *req.Status
	}
	if req.Type != nil {
		artifact.Type = *req.Type
	}
	if req.Metadata != nil {
		artifact.Metadata = req.Metadata
	}
	artifact.UpdatedAt = time.Now()
}

//nolint:gocyclo // Artifact update requires validation of multiple optional fields
func (s *ArtifactService) UpdateArtifactByProjectIDAndSlug(
	userID, projectID, slug string, req *models.UpdateArtifactRequest,
) (*models.Artifact, error) {
	// First check if the artifact exists and get current data (across all user's teams)
	artifact, err := s.GetArtifactByProjectIDAndSlug(userID, projectID, slug)
	if err != nil {
		return nil, err
	}

	return s.applyAndPersistArtifactUpdate(userID, artifact, req, models.ActorTypeHuman)
}

// UpdateArtifactByProjectIDAndSlugInTeam updates an artifact scoped to a single team the
// user belongs to. Unlike UpdateArtifactByProjectIDAndSlug (which spans all of the user's
// teams), this enforces that the artifact lives in teamID before applying the update.
func (s *ArtifactService) UpdateArtifactByProjectIDAndSlugInTeam(
	userID, teamID, projectID, slug string, req *models.UpdateArtifactRequest,
) (*models.Artifact, error) {
	artifact, err := s.GetArtifactByProjectIDAndSlugInTeam(userID, teamID, projectID, slug)
	if err != nil {
		return nil, err
	}

	return s.applyAndPersistArtifactUpdate(userID, artifact, req, models.ActorTypeHuman)
}

// applyAndPersistArtifactUpdate applies the update request to an already-loaded artifact,
// persists it, and publishes the updated event. The artifact must already have been loaded
// through an authorization-enforcing lookup.
func (s *ArtifactService) applyAndPersistArtifactUpdate(
	userID string, artifact *models.Artifact, req *models.UpdateArtifactRequest, actorType string,
) (*models.Artifact, error) {
	// Any member may update any artifact, including another member's (epic #220
	// decision D1). Gated here rather than at the entry points so both the
	// team-scoped and cross-team lookups, and version restore, all funnel through
	// one check. artifact.TeamID is the artifact's own team.
	if authzErr := s.authz.Can(
		context.Background(), userID, artifact.TeamID, authz.ResourceUpdateAny,
	); authzErr != nil {
		return nil, authzErr
	}

	ctx := context.Background()

	// Note: team_id cannot be changed via update (removed from UpdateArtifactRequest)
	// Team reassignment is forbidden to prevent cross-team resource moves

	// Snapshot the prior content before the update mutates it, so a version
	// history is built whenever the content actually changes.
	oldContent := artifact.Content

	// Apply updates
	applyArtifactUpdates(artifact, req)

	// Best-effort content-version snapshot: record the pre-update content when it
	// changed. A snapshot failure must not fail the update (mirrors event publishing).
	if s.contentVersionSvc != nil && oldContent != artifact.Content {
		if err := s.contentVersionSvc.SnapshotIfChanged(ctx, SnapshotRequest{
			ResourceType:  "artifact",
			ResourceID:    artifact.ID,
			TeamID:        artifact.TeamID,
			UserID:        userID,
			OldContent:    oldContent,
			NewContent:    artifact.Content,
			ChangeSummary: req.ChangeSummary,
			ActorType:     actorType,
		}); err != nil {
			s.logger.With("error", err).Warn("Failed to snapshot artifact content version")
		}
	}

	err := s.repo.Update(ctx, artifact)
	if err != nil {
		s.logger.With(
			"service", "artifact",
			"user_id", userID,
			"project_id", artifact.ProjectID,
			"slug", artifact.Slug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to update artifact")
		return nil, err
	}

	// Publish artifact updated event
	if s.eventManager != nil {
		event := events.NewArtifactUpdatedEvent(
			artifact.ID, artifact.UserID, artifact.ProjectID, artifact.Slug,
			artifact.Title, artifact.Description, artifact.Type, artifact.Content, artifact.UpdatedAt,
		)
		if err := s.eventManager.Publish(ctx, event); err != nil {
			s.logger.With("error", err).Warn("Failed to publish artifact updated event")
		}
	}

	return artifact, nil
}

func (s *ArtifactService) DeleteArtifactByProjectIDAndSlug(userID, teamID, projectID, slug string) error {
	// Resolve team-scoped (by membership, not creator user_id) so an Admin/Owner can
	// reach another member's artifact and the delete.own/delete.any authorization below
	// actually runs — the owner-scoped getter returned 404 first, making delete.any dead (#258).
	artifact, err := s.GetArtifactByProjectIDAndSlugInTeam(userID, teamID, projectID, slug)
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Delete is own-vs-any: members delete only what they authored, Admin+ delete
	// anyone's. The repository no longer carries that decision in its SQL.
	if authzErr := s.authz.CanActOnResource(
		ctx, userID, artifact.TeamID, artifact.UserID,
		authz.ResourceDeleteOwn, authz.ResourceDeleteAny,
	); authzErr != nil {
		return authzErr
	}

	err = s.repo.Delete(ctx, userID, artifact.TeamID, artifact.ID)
	if err != nil {
		s.logger.With(
			"service", "artifact",
			"user_id", userID,
			"project_id", projectID,
			"slug", slug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to delete artifact")
		return err
	}

	return nil
}

// ListArtifactVersionsInTeam returns the content-version history for a team-scoped
// artifact, newest-first. The artifact is loaded through the authorization-enforcing
// team-scoped lookup before its versions are read.
func (s *ArtifactService) ListArtifactVersionsInTeam(
	userID, teamID, projectID, slug string,
) ([]*models.ContentVersion, error) {
	artifact, err := s.GetArtifactByProjectIDAndSlugInTeam(userID, teamID, projectID, slug)
	if err != nil {
		return nil, err
	}
	return s.contentVersionSvc.ListVersions(context.Background(), teamID, "artifact", artifact.ID)
}

// GetArtifactVersionInTeam returns a single content version of a team-scoped artifact.
func (s *ArtifactService) GetArtifactVersionInTeam(
	userID, teamID, projectID, slug string, versionNumber int,
) (*models.ContentVersion, error) {
	artifact, err := s.GetArtifactByProjectIDAndSlugInTeam(userID, teamID, projectID, slug)
	if err != nil {
		return nil, err
	}
	return s.contentVersionSvc.GetVersion(context.Background(), teamID, "artifact", artifact.ID, versionNumber)
}

// RestoreArtifactVersionInTeam restores a team-scoped artifact's content to the given
// version by applying it through the normal update path, which snapshots the pre-restore
// content as a new version.
func (s *ArtifactService) RestoreArtifactVersionInTeam(
	userID, teamID, projectID, slug string, versionNumber int,
) (*models.Artifact, error) {
	artifact, err := s.GetArtifactByProjectIDAndSlugInTeam(userID, teamID, projectID, slug)
	if err != nil {
		return nil, err
	}

	target, err := s.contentVersionSvc.Restore(context.Background(), teamID, "artifact", artifact.ID, versionNumber)
	if err != nil {
		return nil, err
	}

	// A restore is a system-authored edit: the pre-restore content is snapshotted with a
	// default "Restored Version N" summary so the timeline reads clearly.
	restoreSummary := fmt.Sprintf("Restored Version %d", versionNumber)
	return s.applyAndPersistArtifactUpdate(userID, artifact, &models.UpdateArtifactRequest{
		Content:       &target,
		ChangeSummary: &restoreSummary,
	}, models.ActorTypeSystem)
}

func (s *ArtifactService) GetArtifactStats(userID, teamID string) (*models.ArtifactStatsResponse, error) {
	stats, err := s.repo.GetStats(context.Background(), userID, teamID)
	if err != nil {
		s.logger.With(
			"service", "artifact",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to get artifact stats")
		return nil, err
	}

	return stats, nil
}
