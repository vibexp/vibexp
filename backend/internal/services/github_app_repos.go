package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// maxRepoPages is the safety cap on the number of pages iterated when fetching
// all accessible repositories. At 30 repos/page this covers ~3,000 repositories.
const maxRepoPages = 100

// GetAccessibleRepoURLs returns a set of normalized HTML URLs for all repositories
// accessible through the team's GitHub installation.
// Returns an empty map (not an error) when no installation exists or the installation is suspended.
func (s *GitHubAppService) GetAccessibleRepoURLs(ctx context.Context, teamID string) (map[string]bool, error) {
	installation, err := s.installationRepo.GetByTeamID(ctx, teamID)
	if err != nil {
		if errors.Is(err, repositories.ErrGitHubInstallationNotFound) {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("failed to get installation: %w", err)
	}

	if installation.SuspendedAt != nil {
		return map[string]bool{}, nil
	}

	return s.fetchAllRepoURLs(ctx, teamID, installation.InstallationID)
}

// fetchAllRepoURLs pages through all accessible repositories for an installation and
// returns a set of normalized HTML URLs. A safety cap of maxRepoPages pages prevents
// unbounded iteration if the API behaves unexpectedly.
func (s *GitHubAppService) fetchAllRepoURLs(
	ctx context.Context, teamID string, installationID int64,
) (map[string]bool, error) {
	repoURLs := make(map[string]bool)
	for page := 1; page <= maxRepoPages; page++ {
		repos, _, err := s.githubClient.GetInstallationRepositories(ctx, installationID, page)
		if err != nil {
			if errors.Is(err, external.ErrGitHubInstallationGone) {
				s.removeGoneInstallation(ctx, teamID, installationID)
				return map[string]bool{}, nil
			}
			return nil, fmt.Errorf("failed to get installation repositories: %w", err)
		}
		if len(repos) == 0 {
			break
		}
		for _, repo := range repos {
			if normalized := normalizeRepoURL(repo.HTMLURL); normalized != "" {
				repoURLs[normalized] = true
			}
		}
		if page == maxRepoPages {
			s.logger.With(
				"service", "github-app-service",
				"team_id", teamID,
				"installation_id", installationID,
				"pages_fetched", maxRepoPages,
			).Warn("fetchAllRepoURLs: reached max page limit; some repositories may not be reflected")
		}
	}
	return repoURLs, nil
}

// normalizeRepoURL normalizes a GitHub repository HTML URL for consistent comparison.
// It lowercases, trims trailing slashes, and removes .git suffix.
func normalizeRepoURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u := strings.ToLower(rawURL)
	u = strings.TrimSuffix(u, "/")
	u = strings.TrimSuffix(u, ".git")
	u = strings.TrimSuffix(u, "/")
	return u
}

// GetRepositories retrieves repositories accessible by the installation, enriched
// with the slug of any already-imported VibeXP project that matches each repo's
// HTML URL within the team. When the project lookup fails, the call still returns
// the repositories with empty slugs so a transient DB hiccup does not break the
// integration page.
func (s *GitHubAppService) GetRepositories(
	ctx context.Context,
	teamID, userID string,
	page int,
) (*models.GitHubRepositoriesResponse, error) {
	installation, err := s.installationRepo.GetByTeamID(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get installation: %w", err)
	}

	repos, totalCount, err := s.githubClient.GetInstallationRepositories(ctx, installation.InstallationID, page)
	if err != nil {
		if errors.Is(err, external.ErrGitHubInstallationGone) {
			s.removeGoneInstallation(ctx, teamID, installation.InstallationID)
		}
		return nil, fmt.Errorf("failed to get repositories: %w", err)
	}

	enriched := s.enrichRepositoriesWithProjectSlugs(ctx, teamID, userID, repos)

	return &models.GitHubRepositoriesResponse{
		Repositories: enriched,
		TotalCount:   totalCount,
	}, nil
}

// enrichRepositoriesWithProjectSlugs sets ImportedProjectSlug on each repo whose
// HTMLURL matches an existing project's git_url within teamID. A project lookup
// error is logged and the repositories are returned unchanged so the GitHub page
// degrades gracefully on a transient DB error.
func (s *GitHubAppService) enrichRepositoriesWithProjectSlugs(
	ctx context.Context,
	teamID, userID string,
	repos []models.GitHubRepository,
) []models.GitHubRepository {
	if len(repos) == 0 {
		return repos
	}

	gitURLToSlug, err := s.projectRepo.ListGitURLToSlugByTeam(ctx, teamID, userID)
	if err != nil {
		s.logger.With("error", err).
			With(
				"service", "github-app-service",
				"team_id", teamID,
			).
			Warn("failed to load project git_url->slug map; returning repositories without slugs")
		return repos
	}

	for i := range repos {
		if slug, ok := gitURLToSlug[repos[i].HTMLURL]; ok {
			repos[i].ImportedProjectSlug = slug
		}
	}
	return repos
}
