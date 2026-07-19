package services

import (
	"context"
	"crypto/sha1" // #nosec G505 -- git blob SHAs are sha1 by definition; not used for security.
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/blueprintpath"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/utils"
)

// msgSkippedBlueprintFile is the log message emitted for each repository file skipped during blueprint import.
const msgSkippedBlueprintFile = "Skipped file during blueprint import"

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
		UpdatedItems:    []models.BlueprintImportUpdated{},
		ConflictItems:   []models.BlueprintImportConflict{},
		UpToDateItems:   []models.BlueprintImportUpToDate{},
	}

	// 5. Resolve the branch head commit SHA once per run for import provenance.
	sourceCommitSHA := s.resolveSourceCommitSHA(ctx, installation.InstallationID, repo, teamID)

	// 6-7. Scan and import each well-known path
	job := &blueprintImportJob{
		installationID:  installation.InstallationID,
		userID:          userID,
		teamID:          teamID,
		repo:            repo,
		projectID:       projectID,
		report:          report,
		sourceCommitSHA: sourceCommitSHA,
	}
	if err := s.runBlueprintScan(ctx, job); err != nil {
		return report, err
	}

	return report, nil
}

// runBlueprintScan scans every well-known AI-config path for the job and imports
// each discovered file into the job's report. It returns early only on context
// cancellation surfaced by a scan.
func (s *GitHubAppService) runBlueprintScan(ctx context.Context, job *blueprintImportJob) error {
	s.logger.With(
		"project_id", job.projectID,
		"repo_id", job.repo.ID,
		"source_commit_sha", job.sourceCommitSHA,
		"team_id", job.teamID,
	).Info("Starting blueprint import run")
	for _, scanPath := range blueprintScanPaths {
		if err := s.scanBlueprintPath(ctx, job, scanPath); err != nil {
			return err
		}
	}
	return nil
}

// resolveSourceCommitSHA resolves the repo's default-branch head commit SHA once
// per import run for provenance (#341). A failure must never fail the import —
// the commit SHA is treated as unknown (empty), exactly as an absent blob SHA is
// treated. Returns "" when the default branch is unknown or resolution fails.
func (s *GitHubAppService) resolveSourceCommitSHA(
	ctx context.Context, installationID int64, repo *models.GitHubRepository, teamID string,
) string {
	if repo.DefaultBranch == "" {
		return ""
	}
	sha, err := s.githubClient.GetBranchHeadSHA(
		ctx, installationID, repo.Owner.Login, repo.Name, repo.DefaultBranch,
	)
	if err != nil {
		s.logger.With(
			"error", err,
			"repo_id", repo.ID,
			"default_branch", repo.DefaultBranch,
			"team_id", teamID,
		).Warn("Failed to resolve branch head commit SHA; import provenance commit SHA will be empty")
		return ""
	}
	return sha
}

// blueprintScanPaths are the well-known AI-config locations scanned during a
// blueprint import (directories recursively, files directly).
var blueprintScanPaths = []blueprintScanPath{
	{".claude", true},
	{".cursor", true},
	{".codex", true},
	{".agents", true},
	{"CLAUDE.md", false},
	{"CURSOR.md", false},
	{"AGENTS.md", false},
}

// blueprintImportJob carries the fields invariant across one repository's
// blueprint import, so the per-path scan helpers stay at two parameters.
type blueprintImportJob struct {
	installationID int64
	userID         string
	teamID         string
	repo           *models.GitHubRepository
	projectID      string
	report         *models.BlueprintImportReport
	// sourceCommitSHA is the default-branch head commit SHA resolved once per
	// run; #341 persists it as blueprint provenance (source_commit_sha).
	sourceCommitSHA string
}

// scanBlueprintPath scans one repository path (file or directory) and imports every
// discovered file into the report. It only returns an error on context cancellation;
// a missing path is logged and skipped.
func (s *GitHubAppService) scanBlueprintPath(
	ctx context.Context, job *blueprintImportJob, scanPath blueprintScanPath,
) error {
	installationID, repo, report := job.installationID, job.repo, job.report
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if scanPath.isDir {
		return s.scanBlueprintDirectory(ctx, job, scanPath.path)
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
	s.logImportProgress(report, repo.ID, job.teamID)
	s.importSingleFile(ctx, job, file)
	return nil
}

// scanBlueprintDirectory recursively lists a repository directory and imports each file.
// It only returns an error on context cancellation; a missing directory is logged and skipped.
func (s *GitHubAppService) scanBlueprintDirectory(
	ctx context.Context, job *blueprintImportJob, dirPath string,
) error {
	installationID, repo, report := job.installationID, job.repo, job.report
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
		s.logImportProgress(report, repo.ID, job.teamID)
		s.importSingleFile(ctx, job, file)
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

// importSingleFile imports a single repository file as a blueprint, or reconciles
// it against an existing blueprint (update-aware re-import, #341).
func (s *GitHubAppService) importSingleFile(
	ctx context.Context, job *blueprintImportJob, file *external.GitHubFile,
) {
	userID, teamID, projectID, repo, report := job.userID, job.teamID, job.projectID, job.repo, job.report
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

	blueprint := s.buildImportedBlueprint(job, file, blueprintType, subtype)

	// Match an existing blueprint by (project_id, path) first, then slug.
	existing := s.findExistingForReimport(ctx, userID, teamID, projectID, file.Path, blueprint.Slug)
	if existing == nil {
		s.createImportedBlueprint(ctx, job, file, blueprint)
		return
	}
	s.reconcileReimport(ctx, job, file, existing, blueprint)
}

// findExistingForReimport returns the blueprint an imported file maps to, matching
// by canonical (project_id, path) first and slug second, or nil when neither
// matches.
func (s *GitHubAppService) findExistingForReimport(
	ctx context.Context, userID, teamID, projectID, path, slug string,
) *models.Blueprint {
	if bp, err := s.blueprintRepo.GetByProjectIDAndPath(ctx, userID, teamID, projectID, path); err == nil && bp != nil {
		return bp
	}
	if bp, err := s.blueprintRepo.GetByProjectIDAndSlug(ctx, userID, teamID, projectID, slug); err == nil && bp != nil {
		return bp
	}
	return nil
}

// createImportedBlueprint persists a brand-new imported blueprint.
func (s *GitHubAppService) createImportedBlueprint(
	ctx context.Context, job *blueprintImportJob, file *external.GitHubFile, blueprint *models.Blueprint,
) {
	if err := s.blueprintRepo.Create(ctx, blueprint); err != nil {
		s.recordFailedImport(job.report, file.Path, blueprint.Slug, err)
		return
	}
	s.recordImportedBlueprint(file, job.repo, job.teamID, blueprint, job.report)
}

// reconcileReimport applies the update-aware re-import outcome for an existing
// blueprint: unchanged repo file -> up-to-date (no-op); changed file with a
// VibeXP-edited blueprint -> conflict (untouched); changed file with an unedited
// blueprint -> update raw + parsed content + provenance.
func (s *GitHubAppService) reconcileReimport(
	ctx context.Context, job *blueprintImportJob,
	file *external.GitHubFile, existing, imported *models.Blueprint,
) {
	switch reimportOutcome(existing, file) {
	case reimportUpToDate:
		s.recordUpToDate(job.report, file.Path, existing)
	case reimportConflict:
		s.recordConflict(job.report, file.Path, existing)
	default: // reimportUpdate
		applyReimportUpdate(existing, imported)
		if err := s.blueprintRepo.UpdateOnReimport(ctx, existing); err != nil {
			s.recordFailedImport(job.report, file.Path, existing.Slug, err)
			return
		}
		s.recordUpdated(file, job.teamID, existing, job.report)
	}
}

// buildImportedBlueprint assembles the blueprint model for an imported repository
// file, deriving slug, title, description, content, and metadata from the file,
// and recording full provenance (verbatim frozen path, raw bytes, content_sha,
// source repo/commit/blob SHAs, imported-at).
func (s *GitHubAppService) buildImportedBlueprint(
	job *blueprintImportJob, file *external.GitHubFile, blueprintType, subtype string,
) *models.Blueprint {
	repo := job.repo
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

	now := time.Now()
	blueprint := &models.Blueprint{
		ID:          uuid.New().String(),
		ProjectID:   job.projectID,
		Slug:        slug,
		UserID:      job.userID,
		TeamID:      job.teamID,
		Content:     content,
		Title:       s.blueprintTitleFromFrontMatter(fm, file.Path, filename, repo.Name),
		Description: s.blueprintDescriptionFromFrontMatter(fm, file.Path, repo.FullName),
		Type:        blueprintType,
		Status:      "active",
		Metadata:    blueprintMetadataFromFrontMatter(fm),
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
		// Imported blueprints carry the verbatim source path (frozen), the raw
		// file bytes + its SHA, and full import provenance (#341).
		Path:        file.Path,
		PathDerived: false,
		RawContent:  file.Content,
		ContentSHA:  computeContentSHA(file.Content),
		Source: &models.BlueprintSource{
			Repo:       repo.HTMLURL,
			CommitSHA:  job.sourceCommitSHA,
			BlobSHA:    file.BlobSHA,
			ImportedAt: &now,
		},
	}

	if subtype != "" {
		blueprint.Subtype = &subtype
	}

	return blueprint
}

// reimportDecision is the outcome of reconciling a re-imported file with an
// existing blueprint.
type reimportDecision int

const (
	reimportUpdate   reimportDecision = iota // repo file changed, blueprint unedited -> refresh
	reimportUpToDate                         // repo file unchanged -> no-op
	reimportConflict                         // blueprint edited in VibeXP -> leave untouched
)

// reimportOutcome decides how to reconcile a re-imported file with an existing
// blueprint:
//   - repo file unchanged (its blob SHA equals the stored source_blob_sha) -> up-to-date;
//   - repo file changed AND the blueprint is still byte-identical to what was
//     imported (git blob SHA of its raw_content equals source_blob_sha) -> update;
//   - otherwise (edited in VibeXP, or provenance is unknown so we cannot confirm
//     it is unedited) -> conflict, never overwriting local changes.
func reimportOutcome(existing *models.Blueprint, file *external.GitHubFile) reimportDecision {
	importedBlob := ""
	if existing.Source != nil {
		importedBlob = existing.Source.BlobSHA
	}
	if file.BlobSHA != "" && importedBlob != "" && file.BlobSHA == importedBlob {
		return reimportUpToDate
	}
	if importedBlob != "" && gitBlobSHA(existing.RawContent) == importedBlob {
		return reimportUpdate
	}
	return reimportConflict
}

// gitBlobSHA returns the Git blob SHA-1 of content ("blob <len>\0<content>"), so
// it can be compared against the source_blob_sha captured at import to tell an
// unedited blueprint (raw byte-identical to the source file) from a VibeXP-edited
// one. sha1 is git's blob-hash algorithm, not a security primitive here.
func gitBlobSHA(content string) string {
	data := append([]byte(fmt.Sprintf("blob %d\x00", len(content))), content...)
	// SHA-1 is git's content-addressing algorithm by definition: the result is
	// compared only against the git blob SHAs GitHub itself provides
	// (source_blob_sha), never used as a security primitive. Suppressed for both
	// gosec (#nosec G401) and SonarCloud (S4790 weak-hash) accordingly.
	sum := sha1.Sum(data) // #nosec G401 // NOSONAR: git blob hashing, not a security context
	return hex.EncodeToString(sum[:])
}

// applyReimportUpdate refreshes the existing blueprint in place from a re-imported
// file: parsed + raw content, lifted title/description, type/subtype, the frozen
// verbatim path, and fresh provenance. Identity (id/slug/created_at) is preserved.
func applyReimportUpdate(existing, imported *models.Blueprint) {
	existing.Content = imported.Content
	existing.RawContent = imported.RawContent
	existing.ContentSHA = imported.ContentSHA
	existing.Metadata = imported.Metadata
	existing.Title = imported.Title
	existing.Description = imported.Description
	existing.Type = imported.Type
	existing.Subtype = imported.Subtype
	existing.Path = imported.Path
	existing.PathDerived = false
	existing.Source = imported.Source
	existing.UpdatedAt = time.Now()
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

// recordFailedImport records a create/update failure in the report.
func (s *GitHubAppService) recordFailedImport(
	report *models.BlueprintImportReport, filePath, slug string, err error,
) {
	s.logger.With("error", err).With("file_path", filePath, "slug", slug).Error("Failed to import blueprint")
	report.TotalFailed++
	report.FailedItems = append(report.FailedItems, models.BlueprintImportFailed{
		FilePath: filePath,
		Error:    "failed to import blueprint",
	})
}

// recordUpToDate records a re-import no-op (repo file unchanged).
func (s *GitHubAppService) recordUpToDate(
	report *models.BlueprintImportReport, filePath string, bp *models.Blueprint,
) {
	report.TotalUpToDate++
	report.UpToDateItems = append(report.UpToDateItems, models.BlueprintImportUpToDate{
		FilePath:    filePath,
		BlueprintID: bp.ID,
	})
}

// recordConflict records a re-import left untouched because the blueprint was
// edited in VibeXP.
func (s *GitHubAppService) recordConflict(
	report *models.BlueprintImportReport, filePath string, bp *models.Blueprint,
) {
	report.TotalConflicts++
	report.ConflictItems = append(report.ConflictItems, models.BlueprintImportConflict{
		FilePath:    filePath,
		BlueprintID: bp.ID,
		Reason:      "Blueprint was edited in VibeXP; re-import skipped to avoid overwriting local changes",
	})
}

// recordUpdated records a blueprint refreshed from a changed repo file.
func (s *GitHubAppService) recordUpdated(
	file *external.GitHubFile, teamID string, bp *models.Blueprint, report *models.BlueprintImportReport,
) {
	report.TotalUpdated++
	subtypeStr := ""
	if bp.Subtype != nil {
		subtypeStr = *bp.Subtype
	}
	report.UpdatedItems = append(report.UpdatedItems, models.BlueprintImportUpdated{
		FilePath:    file.Path,
		BlueprintID: bp.ID,
		Title:       bp.Title,
		Type:        bp.Type,
		Subtype:     subtypeStr,
	})
	s.logger.With(
		"service", logServiceGitHubApp,
		"file_path", file.Path,
		"blueprint_id", bp.ID,
		"team_id", teamID,
	).Info("Refreshed blueprint from changed repo file during re-import")
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

// frontMatterString returns the frontmatter value for key when it is a string,
// otherwise "". Frontmatter values are now typed (map[string]any), so the
// name/title/description lifting only applies when the author wrote a scalar
// string — a nested/typed value for one of these keys is ignored, as before.
func frontMatterString(fm utils.FrontMatterResult, key string) string {
	if v, ok := fm.Metadata[key].(string); ok {
		return v
	}
	return ""
}

// blueprintTitleFromFrontMatter derives the blueprint title, preferring the
// frontmatter "name" then "title" over the default, truncated to maxTitleLen runes.
func (s *GitHubAppService) blueprintTitleFromFrontMatter(
	fm utils.FrontMatterResult, filePath, filename, repoName string,
) string {
	title := fmt.Sprintf("%s from %s", filename, repoName)
	if v := frontMatterString(fm, "name"); v != "" {
		title = v
	} else if v := frontMatterString(fm, "title"); v != "" {
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
	if v := frontMatterString(fm, "description"); v != "" {
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

// determineTypeFromPath determines blueprint type and subtype from a file path.
// The mapping now lives in the shared, bidirectional blueprintpath package so
// import and the future export/materializer can never drift; unmapped paths
// fall back to ("general", ""), which triggers the warning log in the caller.
func (s *GitHubAppService) determineTypeFromPath(path string) (string, string) {
	typ, subtype, _ := blueprintpath.FromPath(path)
	return typ, subtype
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
