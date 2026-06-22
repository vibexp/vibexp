package events

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/logging"
	"github.com/vibexp/vibexp/internal/models"
)

// TeamCreatorServiceInterface defines the interface for team creation operations
// This is defined here to avoid import cycles with the services package
type TeamCreatorServiceInterface interface {
	CreateDefaultTeam(ctx context.Context, userID string) (*models.Team, error)
}

// ProjectCreatorServiceInterface defines the interface for project creation operations
// This is defined here to avoid import cycles with the services package
type ProjectCreatorServiceInterface interface {
	CreateProject(userID, teamID string, req *models.CreateProjectRequest) (*models.Project, error)
}

// TeamCreationListener handles user.created events and creates default teams and projects
type TeamCreationListener struct {
	teamService    TeamCreatorServiceInterface
	projectService ProjectCreatorServiceInterface
	logger         *slog.Logger
}

// NewTeamCreationListener creates a new TeamCreationListener
func NewTeamCreationListener(
	teamService TeamCreatorServiceInterface,
	projectService ProjectCreatorServiceInterface,
	logger *slog.Logger,
) *TeamCreationListener {
	if logger == nil {
		logger = logging.New(logging.Config{})
	}
	return &TeamCreationListener{
		teamService:    teamService,
		projectService: projectService,
		logger:         logger,
	}
}

// Handle processes the user.created event to create a default team
func (l *TeamCreationListener) Handle(ctx context.Context, event Event) error {
	l.logger.With(
		"service", "vibexp-api",
		"component", "team-creation-listener",
		"event_type", event.Type(),
	).Debug("Received event for team creation")

	if event.Type() != EventTypeUserCreated {
		l.logger.With(
			"service", "vibexp-api",
			"component", "team-creation-listener",
			"event_type", event.Type(),
		).Warn("Unexpected event type received")
		return nil
	}

	payload, ok := event.Payload().(*UserCreatedPayload)
	if !ok {
		l.logger.With(
			"service", "vibexp-api",
			"component", "team-creation-listener",
			"event_type", event.Type(),
		).Error("Failed to cast payload to UserCreatedPayload")
		// Don't return error to avoid retry storms
		return nil
	}

	l.logger.With(
		"service", "vibexp-api",
		"component", "team-creation-listener",
		"user_id", payload.UserID,
		"email", payload.Email,
	).Info("Creating default team for new user")

	team, err := l.teamService.CreateDefaultTeam(ctx, payload.UserID)
	if err != nil {
		l.logger.With(
			"service", "vibexp-api",
			"component", "team-creation-listener",
			"user_id", payload.UserID,
			"email", payload.Email,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create default team")
		// Don't return error to avoid blocking user operations
		return nil
	}

	l.logger.With(
		"service", "vibexp-api",
		"component", "team-creation-listener",
		"user_id", payload.UserID,
		"team_id", team.ID,
	).Info("Default team created successfully")

	// Create default project in the private workspace
	l.createDefaultProject(payload.UserID, team.ID)

	return nil
}

// createDefaultProject creates a default "Project 1" project for the new user
// This is non-blocking - if it fails, we log the error but don't block user signup
func (l *TeamCreationListener) createDefaultProject(userID, teamID string) {
	if l.projectService == nil {
		l.logger.With(
			"service", "vibexp-api",
			"component", "team-creation-listener",
			"user_id", userID,
			"team_id", teamID,
		).Warn("Project service not configured, skipping default project creation")
		return
	}

	l.logger.With(
		"service", "vibexp-api",
		"component", "team-creation-listener",
		"user_id", userID,
		"team_id", teamID,
	).Info("Creating default project for new user")

	project, err := l.projectService.CreateProject(userID, teamID, models.DefaultProjectRequest())
	if err != nil {
		l.logger.With(
			"service", "vibexp-api",
			"component", "team-creation-listener",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create default project")
		// Don't return error to avoid blocking user operations
		// User still has workspace, can create project manually
		return
	}

	l.logger.With(
		"service", "vibexp-api",
		"component", "team-creation-listener",
		"user_id", userID,
		"team_id", teamID,
		"project_id", project.ID,
		"slug", project.Slug,
	).Info("Default project created successfully")
}

// EventTypes returns the event types this listener handles
func (l *TeamCreationListener) EventTypes() []string {
	return []string{EventTypeUserCreated}
}
