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

// ProjectService implements the ProjectServiceInterface
type ProjectService struct {
	repo         repositories.ProjectRepository
	teamService  TeamServiceInterface
	authz        AuthorizationServiceInterface
	eventManager events.EventPublisher
	logger       *slog.Logger
}

// Ensure ProjectService implements ProjectServiceInterface
var _ ProjectServiceInterface = (*ProjectService)(nil)

// NewProjectService creates a new ProjectService
func NewProjectService(
	repo repositories.ProjectRepository,
	teamService TeamServiceInterface,
	authzService AuthorizationServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) *ProjectService {
	return &ProjectService{
		repo:         repo,
		teamService:  teamService,
		authz:        authzService,
		eventManager: eventManager,
		logger:       logger,
	}
}

// validateAndResolveTeamID validates team membership and returns the final team ID to use
func (s *ProjectService) validateAndResolveTeamID(
	ctx context.Context, userID, defaultTeamID string, requestedTeamID *string,
) (string, error) {
	// If no team_id provided in request, use default team
	if requestedTeamID == nil || *requestedTeamID == "" {
		return defaultTeamID, nil
	}

	// If teamService not available, accept the requested team
	if s.teamService == nil {
		return *requestedTeamID, nil
	}

	// Validate user is member of the requested team
	isMember, err := s.teamService.IsUserMemberOfTeam(ctx, userID, *requestedTeamID)
	if err != nil {
		s.logger.With(
			"service", "project-service",
			"user_id", userID,
			"team_id", *requestedTeamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to validate team membership")
		return "", fmt.Errorf("failed to validate team membership")
	}

	if !isMember {
		s.logger.With(
			"service", "project-service",
			"user_id", userID,
			"team_id", *requestedTeamID,
		).
			Warn("User attempted to create project in team they are not a member of")
		return "", fmt.Errorf("user is not a member of the specified team")
	}

	return *requestedTeamID, nil
}

// validateTeamReassignment checks if user is trying to move resource to different team
func (s *ProjectService) validateTeamReassignment(requestedTeamID *string, currentTeamID, projectID string) error {
	if requestedTeamID != nil && *requestedTeamID != currentTeamID {
		s.logger.With(
			"service", "project-service",
			"project_id", projectID,
			"existing_team", currentTeamID,
			"requested_team", *requestedTeamID,
		).Warn("User attempted to reassign project to different team")
		return fmt.Errorf("resources cannot be moved between teams once created")
	}
	return nil
}

// CreateProject creates a new project
func (s *ProjectService) CreateProject(
	userID, teamID string, req *models.CreateProjectRequest,
) (*models.Project, error) {
	ctx := context.Background()

	// Validate and resolve team ID
	finalTeamID, err := s.validateAndResolveTeamID(ctx, userID, teamID, req.TeamID)
	if err != nil {
		return nil, err
	}

	// Creating a project is Admin+ (epic #220 decision D2: projects are
	// read-only containers for members). Authorize against the RESOLVED team —
	// req.TeamID can redirect the create away from the URL's team, and the
	// caller's role in that team is what matters.
	if authzErr := s.authz.Can(ctx, userID, finalTeamID, authz.ProjectCreate); authzErr != nil {
		return nil, authzErr
	}

	project := &models.Project{
		UserID:      userID,
		TeamID:      finalTeamID,
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		GitURL:      req.GitURL,
		Homepage:    req.Homepage,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err = s.repo.Create(ctx, project)
	if err != nil {
		s.logger.With(
			"service", "project",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to create project")
		return nil, err
	}

	// Publish project created event if event manager is available
	if s.eventManager != nil {
		event := events.NewProjectCreatedEvent(
			project.ID,
			project.UserID,
			project.Name,
			project.Slug,
			project.Description,
			project.GitURL,
			project.Homepage,
			project.CreatedAt,
		)
		if err := s.eventManager.Publish(ctx, event); err != nil {
			s.logger.With("error", err).Warn("Failed to publish project created event")
		}
	}

	return project, nil
}

// GetProjectBySlug retrieves a project by slug
func (s *ProjectService) GetProjectBySlug(teamID, userID, slug string) (*models.Project, error) {
	project, err := s.repo.GetBySlug(context.Background(), teamID, userID, slug)
	if err != nil {
		s.logger.With(
			"service", "project",
			"team_id", teamID,
			"user_id", userID,
			"slug", slug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get project")
		return nil, err
	}

	return project, nil
}

// ListProjects retrieves projects with filtering and pagination
func (s *ProjectService) ListProjects(userID string, filters ProjectFilters) (*models.ProjectListResponse, error) {
	repoFilters := repositories.ProjectListFilters{
		Search:    filters.Search,
		SortBy:    filters.SortBy,
		SortOrder: filters.SortOrder,
		TeamID:    filters.TeamID,
		Page:      filters.Page,
		Limit:     filters.Limit,
	}

	projects, totalCount, err := s.repo.List(context.Background(), userID, repoFilters)
	if err != nil {
		s.logger.With(
			"service", "project",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to list projects")
		return nil, err
	}

	// Calculate pagination
	totalPages := int(math.Ceil(float64(totalCount) / float64(filters.Limit)))

	// Wrap each project in a ProjectResponse (GitHubConnected is false by default)
	projectResponses := make([]models.ProjectResponse, len(projects))
	for i, p := range projects {
		projectResponses[i] = models.ProjectResponse{Project: p}
	}

	return &models.ProjectListResponse{
		Projects:   projectResponses,
		TotalCount: totalCount,
		Page:       filters.Page,
		PerPage:    filters.Limit,
		TotalPages: totalPages,
	}, nil
}

// applyProjectUpdates copies the provided fields onto the project. TeamID is
// deliberately absent: it is immutable, and validateTeamReassignment has already
// rejected any attempt to change it.
func applyProjectUpdates(project *models.Project, req *models.UpdateProjectRequest) {
	if req.Name != nil {
		project.Name = *req.Name
	}
	if req.Slug != nil {
		project.Slug = *req.Slug
	}
	if req.Description != nil {
		project.Description = *req.Description
	}
	if req.GitURL != nil {
		project.GitURL = *req.GitURL
	}
	if req.Homepage != nil {
		project.Homepage = *req.Homepage
	}
	project.UpdatedAt = time.Now()
}

// UpdateProject updates an existing project
func (s *ProjectService) UpdateProject(
	teamID, userID, slug string, req *models.UpdateProjectRequest,
) (*models.Project, error) {
	// First check if the project exists and get current data
	existingProject, err := s.GetProjectBySlug(teamID, userID, slug)
	if err != nil {
		return nil, err
	}

	// CRITICAL: Validate team_id is not being changed (team reassignment forbidden)
	err = s.validateTeamReassignment(req.TeamID, existingProject.TeamID, existingProject.ID)
	if err != nil {
		return nil, err
	}

	// Updating a project is Admin+ (epic #220 D2). Authorize against the
	// project's own team, which reassignment has just proven is unchanged.
	authzErr := s.authz.Can(context.Background(), userID, existingProject.TeamID, authz.ProjectUpdate)
	if authzErr != nil {
		return nil, authzErr
	}

	applyProjectUpdates(existingProject, req)

	ctx := context.Background()
	err = s.repo.Update(ctx, existingProject)
	if err != nil {
		s.logger.With(
			"service", "project",
			"user_id", userID,
			"slug", slug,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to update project")
		return nil, err
	}

	// Publish project updated event if event manager is available
	if s.eventManager != nil {
		event := events.NewProjectUpdatedEvent(
			existingProject.ID,
			existingProject.UserID,
			existingProject.Name,
			existingProject.Slug,
			existingProject.Description,
			existingProject.GitURL,
			existingProject.Homepage,
			existingProject.UpdatedAt,
		)
		if err := s.eventManager.Publish(ctx, event); err != nil {
			s.logger.With("error", err).Warn("Failed to publish project updated event")
		}
	}

	return existingProject, nil
}

// GetProjectStats returns resource counts for the project identified by teamID + slug.
func (s *ProjectService) GetProjectStats(teamID, userID, slug string) (*models.ProjectStatsResponse, error) {
	stats, err := s.repo.GetProjectStats(context.Background(), teamID, userID, slug)
	if err != nil {
		s.logger.With(
			"service", "project",
			"team_id", teamID,
			"user_id", userID,
			"slug", slug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get project stats")
		return nil, err
	}
	return stats, nil
}

// GetProjectResourceCreationMetrics returns sparse per-day creation counts per
// resource type for the project identified by teamID + slug, counting rows
// created at or after `since`. It is a thin passthrough to the repository; the
// handler owns the date window and zero-fills the result into a continuous series.
func (s *ProjectService) GetProjectResourceCreationMetrics(
	teamID, userID, slug string, since time.Time,
) ([]models.ProjectResourceCreationCount, error) {
	counts, err := s.repo.GetProjectResourceCreationMetrics(context.Background(), teamID, userID, slug, since)
	if err != nil {
		s.logger.With(
			"service", "project",
			"team_id", teamID,
			"user_id", userID,
			"slug", slug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get project resource creation metrics")
		return nil, err
	}
	return counts, nil
}

// DeleteProject deletes a project by team ID, user ID, and slug
func (s *ProjectService) DeleteProject(teamID, userID, slug string) error {
	// First get the project to get its ID for event publishing
	project, err := s.GetProjectBySlug(teamID, userID, slug)
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Deleting a project is Admin+ (epic #220 D2), uniformly — there is no
	// own-vs-any split, because members hold no project permission at all.
	// Authorized before the last-project guard so an unauthorized caller cannot
	// probe how many projects a team has.
	if authzErr := s.authz.Can(ctx, userID, teamID, authz.ProjectDelete); authzErr != nil {
		return authzErr
	}

	// Check if this is the last project in the team
	projectCount, err := s.repo.CountByTeamID(ctx, teamID)
	if err != nil {
		s.logger.With(
			"service", "project",
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to count projects")
		return fmt.Errorf("failed to verify project count: %w", err)
	}

	if projectCount <= 1 {
		return NewCannotDeleteLastProjectError(teamID, slug)
	}

	err = s.repo.Delete(ctx, teamID, userID, slug)
	if err != nil {
		s.logger.With(
			"service", "project",
			"team_id", teamID,
			"user_id", userID,
			"slug", slug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to delete project")
		return err
	}

	// Publish project deleted event if event manager is available
	if s.eventManager != nil {
		event := events.NewProjectDeletedEvent(
			project.ID,
			project.UserID,
			project.Slug,
			time.Now(),
		)
		if err := s.eventManager.Publish(ctx, event); err != nil {
			s.logger.With("error", err).Warn("Failed to publish project deleted event")
		}
	}

	return nil
}
