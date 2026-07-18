package services

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/utils"
)

const (
	// msgSkippedBlueprintFile is the log message emitted for each repository file skipped during blueprint import.
	msgSkippedBlueprintFile = "Skipped file during blueprint import"
	// blueprintTypeClaudeCode is the blueprint type assigned to files under .claude/.
	blueprintTypeClaudeCode = "claude-code"
)

// ImportBlueprintsFromRepository imports AI assistant configurations from a GitHub repository as blueprints.
// The project is automatically discovered by matching the repository URL. If no project exists for the
// repository, an error is returned instructing the user to import the repository as a project first.
//
//nolint:funlen,gocognit // Blueprint import requires comprehensive scanning and context checks
func (s *GitHubAppService) ImportBlueprintsFromRepository(
	ctx context.Context,
	userID, teamID string,
	repoID int64,
) (*models.BlueprintImportReport, error) {
	// 1. Get installation for team
	installation, err := s.getTeamInstallation(ctx, teamID)
	if err != nil {
		return nil, err
	}

	// 2. Get repository details
	repo, err := s.githubClient.GetRepository(ctx, installation.InstallationID, repoID)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	// Extract owner and repo name for API calls
	owner := repo.Owner.Login
	repoName := repo.Name

	// 3. Find project by repository URL.
	// Blueprint import requires a project to exist for the repository.
	project, err := s.projectRepo.GetByGitURL(ctx, teamID, userID, repo.HTMLURL)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: %s - please import this repository as a project first before importing blueprints",
			repositories.ErrProjectNotFoundForRepo,
			repo.FullName,
		)
	}

	s.logger.With(
		"project_id", project.ID,
		"project_name", project.Name,
		"repo_url", repo.HTMLURL,
		"team_id", teamID,
	).
		Info("Found project for blueprint import")

	projectID := project.ID

	// 4. Initialize report
	report := &models.BlueprintImportReport{
		SuccessfulItems: []models.BlueprintImportSuccess{},
		FailedItems:     []models.BlueprintImportFailed{},
		SkippedItems:    []models.BlueprintImportSkipped{},
	}

	// 5. Define paths to scan
	pathsToScan := []struct {
		path  string
		isDir bool
	}{
		{".claude", true},
		{".cursor", true},
		{".codex", true},
		{".agents", true},
		{"CLAUDE.md", false},
		{"CURSOR.md", false},
		{"AGENTS.md", false},
	}

	// 6. Scan and import each path
	for _, scanPath := range pathsToScan {
		select {
		case <-ctx.Done():
			return report, ctx.Err()
		default:
		}

		if scanPath.isDir {
			files, err := s.githubClient.GetDirectoryContentsRecursive(
				ctx, installation.InstallationID, owner, repoName, scanPath.path,
			)
			if err != nil {
				s.logger.With(
					"path", scanPath.path,
					"repo_id", repoID,
				).Debug("Directory not found, skipping")
				continue
			}

			for _, file := range files {
				select {
				case <-ctx.Done():
					return report, ctx.Err()
				default:
				}

				report.TotalScanned++
				s.logImportProgress(report, repo.ID, teamID)
				s.importSingleFile(ctx, userID, teamID, repo, file, projectID, report)
			}
		} else {
			file, err := s.githubClient.GetFileContent(
				ctx, installation.InstallationID, owner, repoName, scanPath.path,
			)
			if err != nil {
				s.logger.With(
					"path", scanPath.path,
					"repo_id", repoID,
				).Debug("File not found, skipping")
				continue
			}

			report.TotalScanned++
			s.logImportProgress(report, repo.ID, teamID)
			s.importSingleFile(ctx, userID, teamID, repo, file, projectID, report)
		}
	}

	return report, nil
}

// logImportProgress logs import progress every importProgressInterval files scanned.
func (s *GitHubAppService) logImportProgress(report *models.BlueprintImportReport, repoID int64, teamID string) {
	if report.TotalScanned%importProgressInterval == 0 {
		s.logger.With(
			"service", logServiceGitHubApp,
			"scanned", report.TotalScanned,
			"successful", report.TotalSuccessful,
			"failed", report.TotalFailed,
			"skipped", report.TotalSkipped,
			"repo_id", repoID,
			"team_id", teamID,
		).Info("GitHub blueprint import progress")
	}
}

// importSingleFile imports a single file as a blueprint
//
//nolint:funlen,gocognit,gocyclo // File processing requires multiple validation and transformation steps
func (s *GitHubAppService) importSingleFile(
	ctx context.Context,
	userID, teamID string,
	repo *models.GitHubRepository,
	file *external.GitHubFile,
	projectID string,
	report *models.BlueprintImportReport,
) {
	// Check file extension - ONLY markdown files
	if !strings.HasSuffix(strings.ToLower(file.Path), ".md") {
		s.logger.With(
			"service", logServiceGitHubApp,
			"file_path", file.Path,
			"extension", filepath.Ext(file.Path),
			"repo_id", repo.ID,
			"team_id", teamID,
			"reason", "invalid_extension",
		).Info(msgSkippedBlueprintFile)
		report.TotalSkipped++
		report.SkippedItems = append(report.SkippedItems, models.BlueprintImportSkipped{
			FilePath: file.Path,
			Reason:   "Not a markdown file",
		})
		return
	}

	// Skip if file is empty
	if len(file.Content) == 0 {
		s.logger.With(
			"service", logServiceGitHubApp,
			"file_path", file.Path,
			"repo_id", repo.ID,
			"team_id", teamID,
			"reason", "empty_content",
		).Debug("Skipped empty file during blueprint import")
		report.TotalSkipped++
		report.SkippedItems = append(report.SkippedItems, models.BlueprintImportSkipped{
			FilePath: file.Path,
			Reason:   "Empty file",
		})
		return
	}

	// Skip files larger than maxFileSize
	if len(file.Content) > maxFileSize {
		s.logger.With(
			"service", logServiceGitHubApp,
			"file_path", file.Path,
			"file_size", len(file.Content),
			"max_size", maxFileSize,
			"repo_id", repo.ID,
			"team_id", teamID,
			"reason", "file_too_large",
		).Info(msgSkippedBlueprintFile)
		report.TotalSkipped++
		report.SkippedItems = append(report.SkippedItems, models.BlueprintImportSkipped{
			FilePath: file.Path,
			Reason:   fmt.Sprintf("File too large (%d bytes, max %d bytes)", len(file.Content), maxFileSize),
		})
		return
	}

	blueprintType, subtype := s.determineTypeFromPath(file.Path)

	if blueprintType == "general" {
		s.logger.With(
			"file_path", file.Path,
			"repo_id", repo.ID,
			"team_id", teamID,
		).Warn("Unmapped file path pattern encountered during blueprint import - consider adding support for this pattern")
	}

	slug := s.generateBlueprintSlug(file.Path, repo.Name)

	filename := file.Path
	if idx := strings.LastIndex(file.Path, "/"); idx != -1 {
		filename = file.Path[idx+1:]
	}

	fm := utils.ParseFrontMatter(file.Content)

	title := fmt.Sprintf("%s from %s", filename, repo.Name)
	if v, ok := fm.Metadata["name"]; ok && v != "" {
		title = v
	} else if v, ok := fm.Metadata["title"]; ok && v != "" {
		title = v
	}
	if titleRunes := []rune(title); len(titleRunes) > maxTitleLen {
		s.logger.With(
			"file_path", file.Path,
			"title_length", len(titleRunes),
			"max_length", maxTitleLen,
		).
			Warn("Frontmatter title exceeds maximum length, truncating")
		title = string(titleRunes[:maxTitleLen])
	}

	description := fmt.Sprintf("Imported from %s", repo.FullName)
	if v, ok := fm.Metadata["description"]; ok && v != "" {
		if descRunes := []rune(v); len(descRunes) > maxDescriptionLen {
			s.logger.With(
				"file_path", file.Path,
				"description_length", len(descRunes),
				"max_length", maxDescriptionLen,
			).Warn("Frontmatter description exceeds maximum length, truncating")
			v = string(descRunes[:maxDescriptionLen])
		}
		description = v
	}

	content := file.Content
	if fm.HasFrontMatter {
		content = fm.Body
	}

	metadata := make(map[string]interface{})
	for k, v := range fm.Metadata {
		if k != "name" && k != "title" && k != "description" {
			metadata[k] = v
		}
	}

	blueprint := &models.Blueprint{
		ID:          uuid.New().String(),
		ProjectID:   projectID,
		Slug:        slug,
		UserID:      userID,
		TeamID:      teamID,
		Content:     content,
		Title:       title,
		Description: description,
		Type:        blueprintType,
		Status:      "active",
		Metadata:    metadata,
		Version:     1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if subtype != "" {
		blueprint.Subtype = &subtype
	}

	existingBlueprint, checkErr := s.blueprintRepo.GetByProjectIDAndSlug(ctx, userID, teamID, projectID, slug)
	if checkErr == nil && existingBlueprint != nil {
		s.logger.With(
			"service", logServiceGitHubApp,
			"file_path", file.Path,
			"slug", slug,
			"repo_id", repo.ID,
			"team_id", teamID,
			"reason", "existing_slug",
		).Info(msgSkippedBlueprintFile)
		report.TotalSkipped++
		report.SkippedItems = append(report.SkippedItems, models.BlueprintImportSkipped{
			FilePath: file.Path,
			Reason:   "Blueprint already exists with slug: " + slug,
		})
		return
	}

	if err := s.blueprintRepo.Create(ctx, blueprint); err != nil {
		s.logger.With("error", err).With(
			"file_path", file.Path,
			"slug", slug,
		).Error("Failed to create blueprint")
		report.TotalFailed++
		report.FailedItems = append(report.FailedItems, models.BlueprintImportFailed{
			FilePath: file.Path,
			Error:    "failed to import blueprint",
		})
		return
	}

	report.TotalSuccessful++
	subtypeStr := ""
	if blueprint.Subtype != nil {
		subtypeStr = *blueprint.Subtype
	}
	report.SuccessfulItems = append(report.SuccessfulItems, models.BlueprintImportSuccess{
		FilePath:    file.Path,
		BlueprintID: blueprint.ID,
		Title:       blueprint.Title,
		Type:        blueprint.Type,
		Subtype:     subtypeStr,
	})

	s.logger.With(
		"service", logServiceGitHubApp,
		"file_path", file.Path,
		"blueprint_id", blueprint.ID,
		"title", blueprint.Title,
		"type", blueprint.Type,
		"subtype", subtypeStr,
		"repo_id", repo.ID,
		"team_id", teamID,
	).Info("Successfully imported blueprint from GitHub")
}

// determineTypeFromPath determines blueprint type and subtype from file path
//
//nolint:gocognit,gocyclo // Path pattern matching requires sequential checks
func (s *GitHubAppService) determineTypeFromPath(path string) (string, string) {
	if strings.HasPrefix(path, ".claude/agents/") {
		return blueprintTypeClaudeCode, "sub-agents"
	}
	if strings.HasPrefix(path, ".claude/skills/") {
		return blueprintTypeClaudeCode, "skills"
	}
	if strings.HasPrefix(path, ".claude/commands/") {
		return blueprintTypeClaudeCode, "slash-commands"
	}
	if strings.HasPrefix(path, ".claude/") {
		return blueprintTypeClaudeCode, "others"
	}
	if path == "CLAUDE.md" {
		return "claude", "claude-md"
	}

	if strings.HasPrefix(path, ".cursor/skills/") {
		return "cursor", "skills"
	}
	if strings.HasPrefix(path, ".cursor/agents/") {
		return "cursor", "agents"
	}
	if strings.HasPrefix(path, ".cursor/commands/") {
		return "cursor", "commands"
	}
	if strings.HasPrefix(path, ".cursor/rules/") {
		return "cursor", "rules"
	}
	if strings.HasPrefix(path, ".cursor/") {
		return "cursor", "cursor-md"
	}
	if path == "CURSOR.md" {
		return "cursor", "cursor-md"
	}

	if path == "AGENTS.md" {
		return "codex", "agents-md"
	}

	if strings.HasPrefix(path, ".codex/rules/") {
		return "codex", "rules"
	}
	if strings.HasPrefix(path, ".codex/skills/") {
		return "codex", "skills"
	}
	if strings.HasPrefix(path, ".codex/") {
		return "codex", "others"
	}

	if strings.HasPrefix(path, ".agents/skills/") {
		return "codex", "skills"
	}
	if strings.HasPrefix(path, ".agents/") {
		return "codex", "others"
	}

	// Default fallback (will trigger warning log in caller)
	return "general", ""
}

// generateBlueprintSlug generates a URL-friendly slug from file path and repo name
func (s *GitHubAppService) generateBlueprintSlug(filePath, repoName string) string {
	// Include directory context to prevent slug collisions
	// e.g., ".claude/agents/agent.md" -> "claude-agents-agent-from-myrepo"
	slug := filePath
	slug = strings.TrimSuffix(slug, ".md")
	slug = strings.ReplaceAll(slug, "/.", "/")
	slug = strings.TrimPrefix(slug, ".")
	slug = strings.ReplaceAll(slug, "/", "-")
	slug = fmt.Sprintf("%s-from-%s", slug, repoName)
	slug = strings.ToLower(slug)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, ".", "-")

	var result strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	slug = result.String()

	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")

	if slug == "" {
		slug = "blueprint"
	}

	return slug
}
