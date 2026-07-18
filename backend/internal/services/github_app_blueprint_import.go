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

// blueprintScanPath is a repository location scanned for importable blueprint files.
type blueprintScanPath struct {
	path  string
	isDir bool
}

// ImportBlueprintsFromRepository imports AI assistant configurations from a GitHub repository as blueprints.
// The project is automatically discovered by matching the repository URL. If no project exists for the
// repository, an error is returned instructing the user to import the repository as a project first.
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
	pathsToScan := []blueprintScanPath{
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
		if err := s.scanBlueprintPath(
			ctx, installation.InstallationID, userID, teamID, repo, scanPath, projectID, report,
		); err != nil {
			return report, err
		}
	}

	return report, nil
}

// scanBlueprintPath scans one repository path (file or directory) and imports every
// discovered file into the report. It only returns an error on context cancellation;
// a missing path is logged and skipped.
func (s *GitHubAppService) scanBlueprintPath(
	ctx context.Context,
	installationID int64,
	userID, teamID string,
	repo *models.GitHubRepository,
	scanPath blueprintScanPath,
	projectID string,
	report *models.BlueprintImportReport,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if scanPath.isDir {
		return s.scanBlueprintDirectory(ctx, installationID, userID, teamID, repo, scanPath.path, projectID, report)
	}

	file, err := s.githubClient.GetFileContent(
		ctx, installationID, repo.Owner.Login, repo.Name, scanPath.path,
	)
	if err != nil {
		s.logger.With(
			"path", scanPath.path,
			"repo_id", repo.ID,
		).Debug("File not found, skipping")
		return nil
	}

	report.TotalScanned++
	s.logImportProgress(report, repo.ID, teamID)
	s.importSingleFile(ctx, userID, teamID, repo, file, projectID, report)
	return nil
}

// scanBlueprintDirectory recursively lists a repository directory and imports each file.
// It only returns an error on context cancellation; a missing directory is logged and skipped.
func (s *GitHubAppService) scanBlueprintDirectory(
	ctx context.Context,
	installationID int64,
	userID, teamID string,
	repo *models.GitHubRepository,
	dirPath string,
	projectID string,
	report *models.BlueprintImportReport,
) error {
	files, err := s.githubClient.GetDirectoryContentsRecursive(
		ctx, installationID, repo.Owner.Login, repo.Name, dirPath,
	)
	if err != nil {
		s.logger.With(
			"path", dirPath,
			"repo_id", repo.ID,
		).Debug("Directory not found, skipping")
		return nil
	}

	for _, file := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		report.TotalScanned++
		s.logImportProgress(report, repo.ID, teamID)
		s.importSingleFile(ctx, userID, teamID, repo, file, projectID, report)
	}
	return nil
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
func (s *GitHubAppService) importSingleFile(
	ctx context.Context,
	userID, teamID string,
	repo *models.GitHubRepository,
	file *external.GitHubFile,
	projectID string,
	report *models.BlueprintImportReport,
) {
	if s.shouldSkipImportFile(file, repo, teamID, report) {
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

	blueprint := s.buildImportedBlueprint(userID, teamID, projectID, repo, file, blueprintType, subtype)

	existingBlueprint, checkErr := s.blueprintRepo.GetByProjectIDAndSlug(ctx, userID, teamID, projectID, blueprint.Slug)
	if checkErr == nil && existingBlueprint != nil {
		s.logger.With(
			"service", logServiceGitHubApp,
			"file_path", file.Path,
			"slug", blueprint.Slug,
			"repo_id", repo.ID,
			"team_id", teamID,
			"reason", "existing_slug",
		).Info(msgSkippedBlueprintFile)
		recordSkippedImportFile(report, file.Path, "Blueprint already exists with slug: "+blueprint.Slug)
		return
	}

	if err := s.blueprintRepo.Create(ctx, blueprint); err != nil {
		s.logger.With("error", err).With(
			"file_path", file.Path,
			"slug", blueprint.Slug,
		).Error("Failed to create blueprint")
		report.TotalFailed++
		report.FailedItems = append(report.FailedItems, models.BlueprintImportFailed{
			FilePath: file.Path,
			Error:    "failed to import blueprint",
		})
		return
	}

	s.recordImportedBlueprint(file, repo, teamID, blueprint, report)
}

// buildImportedBlueprint assembles the blueprint model for an imported repository
// file, deriving slug, title, description, content, and metadata from the file.
func (s *GitHubAppService) buildImportedBlueprint(
	userID, teamID, projectID string,
	repo *models.GitHubRepository,
	file *external.GitHubFile,
	blueprintType, subtype string,
) *models.Blueprint {
	slug := s.generateBlueprintSlug(file.Path, repo.Name)

	filename := file.Path
	if idx := strings.LastIndex(file.Path, "/"); idx != -1 {
		filename = file.Path[idx+1:]
	}

	fm := utils.ParseFrontMatter(file.Content)

	content := file.Content
	if fm.HasFrontMatter {
		content = fm.Body
	}

	blueprint := &models.Blueprint{
		ID:          uuid.New().String(),
		ProjectID:   projectID,
		Slug:        slug,
		UserID:      userID,
		TeamID:      teamID,
		Content:     content,
		Title:       s.blueprintTitleFromFrontMatter(fm, file.Path, filename, repo.Name),
		Description: s.blueprintDescriptionFromFrontMatter(fm, file.Path, repo.FullName),
		Type:        blueprintType,
		Status:      "active",
		Metadata:    blueprintMetadataFromFrontMatter(fm),
		Version:     1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if subtype != "" {
		blueprint.Subtype = &subtype
	}

	return blueprint
}

// recordImportedBlueprint records a successful import in the report and logs it.
func (s *GitHubAppService) recordImportedBlueprint(
	file *external.GitHubFile,
	repo *models.GitHubRepository,
	teamID string,
	blueprint *models.Blueprint,
	report *models.BlueprintImportReport,
) {
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

// shouldSkipImportFile checks the import guards (markdown-only, non-empty, size cap)
// and records a skip in the report when one fails. It returns true when the file
// must not be imported.
func (s *GitHubAppService) shouldSkipImportFile(
	file *external.GitHubFile,
	repo *models.GitHubRepository,
	teamID string,
	report *models.BlueprintImportReport,
) bool {
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
		recordSkippedImportFile(report, file.Path, "Not a markdown file")
		return true
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
		recordSkippedImportFile(report, file.Path, "Empty file")
		return true
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
		recordSkippedImportFile(
			report, file.Path,
			fmt.Sprintf("File too large (%d bytes, max %d bytes)", len(file.Content), maxFileSize),
		)
		return true
	}

	return false
}

// recordSkippedImportFile appends a skipped entry to the import report.
func recordSkippedImportFile(report *models.BlueprintImportReport, filePath, reason string) {
	report.TotalSkipped++
	report.SkippedItems = append(report.SkippedItems, models.BlueprintImportSkipped{
		FilePath: filePath,
		Reason:   reason,
	})
}

// blueprintTitleFromFrontMatter derives the blueprint title, preferring the
// frontmatter "name" then "title" over the default, truncated to maxTitleLen runes.
func (s *GitHubAppService) blueprintTitleFromFrontMatter(
	fm utils.FrontMatterResult, filePath, filename, repoName string,
) string {
	title := fmt.Sprintf("%s from %s", filename, repoName)
	if v, ok := fm.Metadata["name"]; ok && v != "" {
		title = v
	} else if v, ok := fm.Metadata["title"]; ok && v != "" {
		title = v
	}
	if titleRunes := []rune(title); len(titleRunes) > maxTitleLen {
		s.logger.With(
			"file_path", filePath,
			"title_length", len(titleRunes),
			"max_length", maxTitleLen,
		).
			Warn("Frontmatter title exceeds maximum length, truncating")
		title = string(titleRunes[:maxTitleLen])
	}
	return title
}

// blueprintDescriptionFromFrontMatter derives the blueprint description, preferring
// the frontmatter "description" over the default, truncated to maxDescriptionLen runes.
func (s *GitHubAppService) blueprintDescriptionFromFrontMatter(
	fm utils.FrontMatterResult, filePath, repoFullName string,
) string {
	description := fmt.Sprintf("Imported from %s", repoFullName)
	if v, ok := fm.Metadata["description"]; ok && v != "" {
		if descRunes := []rune(v); len(descRunes) > maxDescriptionLen {
			s.logger.With(
				"file_path", filePath,
				"description_length", len(descRunes),
				"max_length", maxDescriptionLen,
			).Warn("Frontmatter description exceeds maximum length, truncating")
			v = string(descRunes[:maxDescriptionLen])
		}
		description = v
	}
	return description
}

// blueprintMetadataFromFrontMatter copies the frontmatter metadata minus the keys
// already mapped to dedicated blueprint fields (name/title/description).
func blueprintMetadataFromFrontMatter(fm utils.FrontMatterResult) map[string]interface{} {
	metadata := make(map[string]interface{})
	for k, v := range fm.Metadata {
		if k != "name" && k != "title" && k != "description" {
			metadata[k] = v
		}
	}
	return metadata
}

// blueprintPathType maps a repository path prefix to a blueprint type and subtype.
type blueprintPathType struct {
	prefix  string
	typ     string
	subtype string
}

// blueprintExactPathTypes maps exact root-level file paths to blueprint types.
var blueprintExactPathTypes = map[string]blueprintPathType{
	"CLAUDE.md": {typ: "claude", subtype: "claude-md"},
	"CURSOR.md": {typ: "cursor", subtype: "cursor-md"},
	"AGENTS.md": {typ: "codex", subtype: "agents-md"},
}

// blueprintPrefixPathTypes maps path prefixes to blueprint types, most specific first
// (order matters: e.g. ".claude/agents/" must match before ".claude/").
var blueprintPrefixPathTypes = []blueprintPathType{
	{prefix: ".claude/agents/", typ: blueprintTypeClaudeCode, subtype: "sub-agents"},
	{prefix: ".claude/skills/", typ: blueprintTypeClaudeCode, subtype: "skills"},
	{prefix: ".claude/commands/", typ: blueprintTypeClaudeCode, subtype: "slash-commands"},
	{prefix: ".claude/", typ: blueprintTypeClaudeCode, subtype: "others"},
	{prefix: ".cursor/skills/", typ: "cursor", subtype: "skills"},
	{prefix: ".cursor/agents/", typ: "cursor", subtype: "agents"},
	{prefix: ".cursor/commands/", typ: "cursor", subtype: "commands"},
	{prefix: ".cursor/rules/", typ: "cursor", subtype: "rules"},
	{prefix: ".cursor/", typ: "cursor", subtype: "cursor-md"},
	{prefix: ".codex/rules/", typ: "codex", subtype: "rules"},
	{prefix: ".codex/skills/", typ: "codex", subtype: "skills"},
	{prefix: ".codex/", typ: "codex", subtype: "others"},
	{prefix: ".agents/skills/", typ: "codex", subtype: "skills"},
	{prefix: ".agents/", typ: "codex", subtype: "others"},
}

// determineTypeFromPath determines blueprint type and subtype from file path
func (s *GitHubAppService) determineTypeFromPath(path string) (string, string) {
	if match, ok := blueprintExactPathTypes[path]; ok {
		return match.typ, match.subtype
	}
	for _, match := range blueprintPrefixPathTypes {
		if strings.HasPrefix(path, match.prefix) {
			return match.typ, match.subtype
		}
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
