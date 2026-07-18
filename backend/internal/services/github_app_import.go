package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// Import-related constants.
const (
	// logServiceGitHubApp is the "service" structured-log field value shared by the GitHub App import flows.
	logServiceGitHubApp = "github-app"

	// maxSlugRetries is the number of slug-suffix attempts before giving up on slug collision resolution.
	maxSlugRetries = 10

	// maxSlugLength bounds a generated slug to the projects.slug column width (VARCHAR(100)).
	// Deriving slugs from the full owner/repo name can exceed this, so slugs are truncated to fit.
	maxSlugLength = 100

	// maxFileSize is the maximum allowed blueprint file size in bytes (1 MB).
	maxFileSize = 1024 * 1024

	// maxTitleLen is the maximum blueprint title length in runes.
	maxTitleLen = 255

	// maxDescriptionLen is the maximum blueprint description length in runes.
	maxDescriptionLen = 500

	// importProgressInterval is how often (in files scanned) a progress log is emitted.
	importProgressInterval = 10
)

// ImportProjectFromRepository imports a GitHub repository as a VibeXP project
// Returns (project, created, error) where created indicates if a new project was created
func (s *GitHubAppService) ImportProjectFromRepository(
	ctx context.Context,
	userID, teamID string,
	repoID int64,
) (*models.Project, bool, error) {
	// Get installation for team
	installation, err := s.getTeamInstallation(ctx, teamID)
	if err != nil {
		return nil, false, err
	}

	// Get repository details from GitHub API
	repo, err := s.githubClient.GetRepository(ctx, installation.InstallationID, repoID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get repository: %w", err)
	}

	// Check if project with this git_url already exists
	existingProject, existingErr := s.checkExistingProject(ctx, teamID, userID, repo.HTMLURL)
	if existingErr == nil {
		return existingProject, false, nil
	}

	// Create new project from repository
	project := s.buildProjectFromRepo(userID, teamID, repo)

	created, err := s.createAndPublishProject(ctx, project, userID, teamID, repoID)
	if err != nil {
		return nil, false, err
	}

	return project, created, nil
}

// getTeamInstallation retrieves the GitHub installation for a team
func (s *GitHubAppService) getTeamInstallation(
	ctx context.Context,
	teamID string,
) (*models.GitHubInstallation, error) {
	installation, err := s.installationRepo.GetByTeamID(ctx, teamID)
	if err != nil {
		if errors.Is(err, repositories.ErrGitHubInstallationNotFound) {
			return nil, repositories.ErrGitHubInstallationNotFound
		}
		return nil, fmt.Errorf("failed to get installation: %w", err)
	}
	return installation, nil
}

// checkExistingProject checks if a project already exists for the given git URL
func (s *GitHubAppService) checkExistingProject(
	ctx context.Context,
	teamID, userID, gitURL string,
) (*models.Project, error) {
	existingProject, err := s.projectRepo.GetByGitURL(ctx, teamID, userID, gitURL)
	if err == nil {
		s.logger.With(
			"team_id", teamID,
			"git_url", gitURL,
			"project_id", existingProject.ID,
		).
			Info("Project already exists for this repository")
	}
	return existingProject, err
}

// buildProjectFromRepo builds a project model from a GitHub repository
func (s *GitHubAppService) buildProjectFromRepo(
	userID, teamID string,
	repo *models.GitHubRepository,
) *models.Project {
	description := ""
	if repo.Description != nil {
		description = *repo.Description
	}

	slug := generateSlugFromName(repo.FullName)

	return &models.Project{
		UserID:      userID,
		TeamID:      teamID,
		Name:        repo.FullName,
		Slug:        slug,
		Description: description,
		GitURL:      repo.HTMLURL,
		Homepage:    "",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// createAndPublishProject creates the project and publishes the event.
// Returns (created, error) where created reports whether a new project was persisted;
// a git_url race that resolves to an existing project returns created=false.
func (s *GitHubAppService) createAndPublishProject(
	ctx context.Context,
	project *models.Project,
	userID, teamID string,
	repoID int64,
) (bool, error) {
	err := s.projectRepo.Create(ctx, project)
	if err != nil {
		handled, wasCreated, handledErr := s.handleConstraintViolation(ctx, project, teamID, userID, repoID, err)
		if !handled {
			return false, s.logAndReturnCreateError(userID, teamID, repoID, project.GitURL, err)
		}
		if handledErr != nil {
			return false, handledErr
		}
		if !wasCreated {
			s.logger.With(
				"service", logServiceGitHubApp,
				"team_id", teamID,
				"project_id", project.ID,
				"git_url", project.GitURL,
			).Info("Project already exists - no event published")
			return false, nil
		}
	}

	s.publishProjectCreatedEvent(ctx, project)

	s.logger.With(
		"service", logServiceGitHubApp,
		"user_id", userID,
		"team_id", teamID,
		"repo_id", repoID,
		"project_id", project.ID,
		"git_url", project.GitURL,
	).Info("Successfully created project from GitHub repository")

	return true, nil
}

// handleConstraintViolation handles unique constraint violations for git_url and slug.
// Returns (handled, wasCreated, error).
// wasCreated=false means existing project was found (git_url violation).
// wasCreated=true means project was created after retry (slug collision resolved).
//
// Routing relies on ProjectRepository.Create/Update wrapping every Postgres 23505 into
// one of these domain sentinels (see mapProjectUniqueViolation). A raw *pq.Error that
// bypasses that mapping is treated as "not handled" and surfaces as a hard create error.
func (s *GitHubAppService) handleConstraintViolation(
	ctx context.Context,
	project *models.Project,
	teamID, userID string,
	repoID int64,
	err error,
) (bool, bool, error) {
	if errors.Is(err, repositories.ErrProjectGitURLExists) {
		return true, false, s.handleGitURLConstraintViolation(ctx, project, teamID, userID)
	}
	if errors.Is(err, repositories.ErrProjectSlugExists) {
		return true, true, s.handleSlugConstraintViolation(ctx, project, teamID, userID, repoID)
	}
	return false, false, nil
}

// handleGitURLConstraintViolation handles duplicate git_url by fetching existing project
func (s *GitHubAppService) handleGitURLConstraintViolation(
	ctx context.Context,
	project *models.Project,
	teamID, userID string,
) error {
	s.logger.With(
		"service", logServiceGitHubApp,
		"team_id", teamID,
		"git_url", project.GitURL,
	).
		Info("Unique constraint violation on git_url - project already exists")

	existingProject, queryErr := s.projectRepo.GetByGitURL(ctx, teamID, userID, project.GitURL)
	if queryErr == nil {
		project.ID = existingProject.ID
		project.CreatedAt = existingProject.CreatedAt
		project.UpdatedAt = existingProject.UpdatedAt
		return nil
	}

	s.logger.With("error", queryErr).Error("Failed to query existing project after constraint violation")
	return fmt.Errorf("constraint violation on git_url but failed to query existing project: %w", queryErr)
}

// handleSlugConstraintViolation handles slug collision by appending numeric suffix
func (s *GitHubAppService) handleSlugConstraintViolation(
	ctx context.Context,
	project *models.Project,
	teamID, userID string,
	repoID int64,
) error {
	s.logger.With(
		"service", logServiceGitHubApp,
		"team_id", teamID,
		"original_slug", project.Slug,
	).
		Info("Slug collision detected - generating unique slug with suffix")

	baseSlug := project.Slug
	var err error

	for i := 2; i <= maxSlugRetries+1; i++ {
		project.Slug = buildSuffixedSlug(baseSlug, i)
		err = s.projectRepo.Create(ctx, project)
		if err == nil {
			return nil
		}
		if !errors.Is(err, repositories.ErrProjectSlugExists) {
			break
		}
	}

	s.logger.With(
		"service", logServiceGitHubApp,
		"user_id", userID,
		"team_id", teamID,
		"repo_id", repoID,
		"slug", project.Slug,
		"error", fmt.Sprintf("%+v", err),
	).Error(fmt.Sprintf("Failed to create project with unique slug after %d attempts", maxSlugRetries))
	return fmt.Errorf("failed to create project with unique slug: %w", err)
}

// logAndReturnCreateError logs and returns a generic project creation error
func (s *GitHubAppService) logAndReturnCreateError(
	userID, teamID string,
	repoID int64,
	gitURL string,
	err error,
) error {
	s.logger.With(
		"service", logServiceGitHubApp,
		"user_id", userID,
		"team_id", teamID,
		"repo_id", repoID,
		"git_url", gitURL,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to create project from repository")
	return fmt.Errorf("failed to create project: %w", err)
}

// publishProjectCreatedEvent publishes a project created event
func (s *GitHubAppService) publishProjectCreatedEvent(
	ctx context.Context,
	project *models.Project,
) {
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
}

// generateSlugFromName creates a URL-friendly slug from a name.
// Separators that carry meaning in a repository full name (' ', '/', '.')
// become hyphens so an org-qualified name like "my-org/my.repo"
// yields "my-org-my-repo" rather than merging the segments.
func generateSlugFromName(name string) string {
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "/", "-")
	slug = strings.ReplaceAll(slug, ".", "-")
	// Remove special characters, keep only alphanumeric and hyphens
	var result strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	slug = result.String()
	// Remove consecutive hyphens
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	// Keep the slug within the projects.slug column width; the sanitized slug is
	// ASCII-only here, so byte and rune lengths match. Re-trim in case the cut
	// landed on a hyphen.
	if len(slug) > maxSlugLength {
		slug = strings.Trim(slug[:maxSlugLength], "-")
	}
	if slug == "" {
		slug = "project"
	}
	return slug
}

// buildSuffixedSlug appends a numeric collision suffix to baseSlug, truncating the
// base when necessary so the combined slug stays within maxSlugLength (the
// projects.slug column width). baseSlug is the ASCII output of generateSlugFromName,
// so byte slicing is safe.
func buildSuffixedSlug(baseSlug string, attempt int) string {
	suffix := fmt.Sprintf("-%d", attempt)
	if len(baseSlug)+len(suffix) > maxSlugLength {
		baseSlug = strings.TrimRight(baseSlug[:maxSlugLength-len(suffix)], "-")
	}
	return baseSlug + suffix
}
