package services

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
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

// attachmentOwnerTypeBlueprint is the attachment owner_type under which Agent
// Skill companion files are stored (matches the server's ownerTypeBlueprint and
// the registered blueprint attachment authorizer).
const attachmentOwnerTypeBlueprint = "blueprint"

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
		CompanionItems:  []models.BlueprintImportCompanion{},
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

// scanBlueprintDirectory recursively lists a repository directory and imports its
// files. Agent Skill directories (a dir directly containing SKILL.md under a
// skills-mapped prefix) import whole: the SKILL.md as a blueprint and every
// sibling as a blueprint-owned attachment (#342). Every other file imports
// individually, still markdown-only. It only returns an error on context
// cancellation; a missing directory is logged and skipped.
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

	// Every listed file counts as scanned, including skill companions.
	for range files {
		report.TotalScanned++
		s.logImportProgress(report, repo.ID, job.teamID)
	}

	units, standalone := groupSkillUnits(files)

	for _, file := range standalone {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		s.importSingleFile(ctx, job, file)
	}

	for _, unit := range units {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		s.importSkillUnit(ctx, job, unit)
	}
	return nil
}

// skillUnit is one Agent Skill directory grouped for whole-directory import: its
// SKILL.md anchor plus every companion file beneath the directory.
type skillUnit struct {
	skillDir   string
	anchor     *external.GitHubFile
	companions []*external.GitHubFile
}

// isSkillAnchor reports whether a repository path is the SKILL.md of an Agent
// Skill directory: its base name is SKILL.md and its path classifies to a
// "skills" subtype under a skills-mapped prefix (per the shared blueprintpath
// table). This is what makes ".claude/skills/foo/SKILL.md" a skill unit while a
// stray SKILL.md elsewhere stays an ordinary markdown file.
func isSkillAnchor(p string) bool {
	if path.Base(p) != "SKILL.md" {
		return false
	}
	_, subtype, ok := blueprintpath.FromPath(p)
	return ok && subtype == "skills"
}

// groupSkillUnits partitions a directory listing into Agent Skill units and
// standalone files. A file is a companion of the deepest skill directory that
// contains it (nested skills are not a real Agent Skills shape, but deepest-match
// keeps grouping unambiguous). Units are returned in the order their SKILL.md
// appears in the listing; standalone files keep their listing order.
func groupSkillUnits(files []*external.GitHubFile) (units []*skillUnit, standalone []*external.GitHubFile) {
	unitByDir := make(map[string]*skillUnit)
	var skillDirs []string
	for _, f := range files {
		if isSkillAnchor(f.Path) {
			dir := path.Dir(f.Path)
			unitByDir[dir] = &skillUnit{skillDir: dir, anchor: f}
			skillDirs = append(skillDirs, dir)
			units = append(units, unitByDir[dir])
		}
	}
	// Match a companion to the deepest (longest) skill dir that is its ancestor.
	ordered := append([]string(nil), skillDirs...)
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i]) > len(ordered[j]) })

	for _, f := range files {
		if isSkillAnchor(f.Path) {
			continue
		}
		owner := ""
		for _, dir := range ordered {
			if strings.HasPrefix(f.Path, dir+"/") {
				owner = dir
				break
			}
		}
		if owner == "" {
			standalone = append(standalone, f)
			continue
		}
		unitByDir[owner].companions = append(unitByDir[owner].companions, f)
	}
	return units, standalone
}

// importSkillUnit imports one Agent Skill directory: its SKILL.md as a blueprint,
// then (only when the blueprint was created or refreshed from an unedited skill)
// its companion files as blueprint-owned attachments, reconciled by
// relative_path. A skipped/failed SKILL.md, an up-to-date skill, or a
// VibeXP-edited (conflict) skill leaves companions untouched.
func (s *GitHubAppService) importSkillUnit(ctx context.Context, job *blueprintImportJob, unit *skillUnit) {
	blueprint, outcome := s.importSingleFileWithOutcome(ctx, job, unit.anchor)
	if blueprint == nil {
		return // skipped or failed — never touch companions
	}
	switch outcome {
	case outcomeCreated, outcomeUpdated:
		s.reconcileCompanions(ctx, job, blueprint.ID, unit)
	default: // outcomeUpToDate, outcomeConflict — leave companions untouched
	}
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

// importOutcome is the result of importing one blueprint file, so a skill unit
// can decide whether to touch companions (only for a created or unedited-refresh
// SKILL.md).
type importOutcome int

const (
	outcomeSkipped  importOutcome = iota // guard rejected the file (non-.md, empty, too large)
	outcomeFailed                        // create/update persistence failed
	outcomeCreated                       // a brand-new blueprint was persisted
	outcomeUpdated                       // an unedited blueprint was refreshed from a changed repo file
	outcomeUpToDate                      // repo file unchanged since import (no-op)
	outcomeConflict                      // blueprint edited in VibeXP (left untouched)
)

// importSingleFile imports a single repository file as a blueprint, or reconciles
// it against an existing blueprint (update-aware re-import, #341).
func (s *GitHubAppService) importSingleFile(
	ctx context.Context, job *blueprintImportJob, file *external.GitHubFile,
) {
	s.importSingleFileWithOutcome(ctx, job, file)
}

// importSingleFileWithOutcome imports one repository file as a blueprint and
// reports both the resulting blueprint (nil when skipped or failed) and the
// outcome, so multi-file skill import can gate companion handling on it (#342).
func (s *GitHubAppService) importSingleFileWithOutcome(
	ctx context.Context, job *blueprintImportJob, file *external.GitHubFile,
) (*models.Blueprint, importOutcome) {
	userID, teamID, projectID, repo, report := job.userID, job.teamID, job.projectID, job.repo, job.report
	if s.shouldSkipImportFile(file, repo, teamID, report) {
		return nil, outcomeSkipped
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
		if err := s.blueprintRepo.Create(ctx, blueprint); err != nil {
			s.recordFailedImport(job.report, file.Path, blueprint.Slug, err)
			return nil, outcomeFailed
		}
		s.recordImportedBlueprint(file, job.repo, job.teamID, blueprint, job.report)
		return blueprint, outcomeCreated
	}
	return s.reconcileReimport(ctx, job, file, existing, blueprint)
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

// reconcileReimport applies the update-aware re-import outcome for an existing
// blueprint: unchanged repo file -> up-to-date (no-op); changed file with a
// VibeXP-edited blueprint -> conflict (untouched); changed file with an unedited
// blueprint -> update raw + parsed content + provenance. It returns the existing
// blueprint and the mapped import outcome so callers (e.g. skill-unit import)
// can decide follow-up work.
func (s *GitHubAppService) reconcileReimport(
	ctx context.Context, job *blueprintImportJob,
	file *external.GitHubFile, existing, imported *models.Blueprint,
) (*models.Blueprint, importOutcome) {
	switch reimportOutcome(existing, file) {
	case reimportUpToDate:
		s.recordUpToDate(job.report, file.Path, existing)
		return existing, outcomeUpToDate
	case reimportConflict:
		s.recordConflict(job.report, file.Path, existing)
		return existing, outcomeConflict
	default: // reimportUpdate
		applyReimportUpdate(existing, imported)
		if err := s.blueprintRepo.UpdateOnReimport(ctx, existing); err != nil {
			s.recordFailedImport(job.report, file.Path, existing.Slug, err)
			return nil, outcomeFailed
		}
		s.recordUpdated(file, job.teamID, existing, job.report)
		return existing, outcomeUpdated
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
		// Immutable fingerprint of the imported bytes (== ContentSHA at import),
		// preserved across VibeXP edits so re-import can detect them (#341).
		SourceContentSHA: computeContentSHA(file.Content),
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
//   - repo file changed AND the blueprint is still unedited (its current
//     content_sha still equals the content_sha captured at import,
//     source_content_sha) -> update;
//   - otherwise (edited in VibeXP, or provenance is unknown so we cannot confirm
//     it is unedited) -> conflict, never overwriting local changes.
//
// The unedited test uses SHA-256 fingerprints only: source_content_sha is the
// import-time content_sha, immutable across edits, while content_sha is
// regenerated on every VibeXP edit (#340), so equality means "raw unchanged
// since import".
func reimportOutcome(existing *models.Blueprint, file *external.GitHubFile) reimportDecision {
	importedBlob := ""
	if existing.Source != nil {
		importedBlob = existing.Source.BlobSHA
	}
	if file.BlobSHA != "" && importedBlob != "" && file.BlobSHA == importedBlob {
		return reimportUpToDate
	}
	if existing.SourceContentSHA != "" && existing.ContentSHA == existing.SourceContentSHA {
		return reimportUpdate
	}
	return reimportConflict
}

// applyReimportUpdate refreshes the existing blueprint in place from a re-imported
// file: parsed + raw content, lifted title/description, type/subtype, the frozen
// verbatim path, and fresh provenance. Identity (id/slug/created_at) is preserved.
func applyReimportUpdate(existing, imported *models.Blueprint) {
	existing.Content = imported.Content
	existing.RawContent = imported.RawContent
	existing.ContentSHA = imported.ContentSHA
	existing.SourceContentSHA = imported.SourceContentSHA
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

// Companion outcome labels for the import report (models.BlueprintImportCompanion.Outcome).
const (
	companionImported = "imported"
	companionUpdated  = "updated"
	companionRemoved  = "removed"
	companionSkipped  = "skipped"
)

// reconcileCompanions stores an Agent Skill's companion files as blueprint-owned
// attachments and reconciles them by relative_path against what is already
// stored (#342): a companion new to the skill is added, one whose bytes changed
// replaces the stored copy, and one absent from the re-imported skill is
// removed. The attachment service is the authority on per-file/per-owner size
// limits, the safe-type allowlist, and storage-unconfigured degradation — each
// of its rejections becomes a per-companion skip in the report, and the SKILL.md
// blueprint (already imported) is never affected.
func (s *GitHubAppService) reconcileCompanions(
	ctx context.Context, job *blueprintImportJob, blueprintID string, unit *skillUnit,
) {
	existing := s.listCompanions(ctx, blueprintID)
	seen := make(map[string]struct{}, len(unit.companions))

	for _, file := range unit.companions {
		relPath := strings.TrimPrefix(file.Path, unit.skillDir+"/")
		seen[relPath] = struct{}{}
		s.storeCompanion(ctx, job, blueprintID, relPath, file, existing[relPath])
	}

	// Remove companions that are no longer part of the re-imported skill.
	relPaths := make([]string, 0, len(existing))
	for relPath := range existing {
		relPaths = append(relPaths, relPath)
	}
	sort.Strings(relPaths)
	for _, relPath := range relPaths {
		if _, kept := seen[relPath]; kept {
			continue
		}
		att := existing[relPath]
		if err := s.attachmentSvc.Delete(ctx, attachmentOwnerTypeBlueprint, blueprintID, att.ID); err != nil {
			s.logger.With("error", err, "blueprint_id", blueprintID, "relative_path", relPath).
				Warn("Failed to remove stale skill companion during re-import")
			continue
		}
		s.recordCompanion(job.report, blueprintID, relPath, companionRemoved, "")
	}
}

// listCompanions returns the blueprint's currently stored companions keyed by
// relative_path. A companion stored without a relative_path (not produced by
// skill import) is ignored so reconciliation only ever touches skill companions.
// A listing error (e.g. storage unconfigured) degrades to an empty set so a
// fresh import still proceeds.
func (s *GitHubAppService) listCompanions(ctx context.Context, blueprintID string) map[string]models.Attachment {
	byPath := make(map[string]models.Attachment)
	list, err := s.attachmentSvc.List(ctx, attachmentOwnerTypeBlueprint, blueprintID)
	if err != nil || list == nil {
		return byPath
	}
	for i := range list.Attachments {
		att := list.Attachments[i]
		if att.RelativePath == "" {
			continue
		}
		byPath[att.RelativePath] = att
	}
	return byPath
}

// storeCompanion uploads one companion file, replacing a stored copy at the same
// relative_path when present. Size/type/budget enforcement and the
// storage-unconfigured case are delegated to the attachment service; any
// rejection is recorded as a per-companion skip and never disturbs the already
// stored copy (the new bytes are validated by Upload before the old copy is
// removed).
func (s *GitHubAppService) storeCompanion(
	ctx context.Context, job *blueprintImportJob,
	blueprintID, relPath string, file *external.GitHubFile, existing models.Attachment,
) {
	oldAtt, hasExisting := existingAttachment(existing)

	// A companion whose byte length matches the stored copy is treated as
	// unchanged and left in place, so a re-import does not churn every companion.
	// Change-detection is size-based because attachments carry no content hash; a
	// same-size edit is therefore not detected — an accepted v1 limitation
	// (companion files are not version-tracked). Companion change-detection is
	// also keyed off the SKILL.md: a companion edit under an unchanged SKILL.md is
	// only reconciled on the next import that also changes the SKILL.md.
	if hasExisting && oldAtt.SizeBytes == int64(len(file.Content)) {
		return
	}

	// Replacing a changed companion: the partial-unique (owner_type, owner_id,
	// relative_path) index forbids two rows at the same path, so the stored copy
	// must be removed before the replacement is uploaded. Pre-validate size first
	// so an oversized replacement never destroys the good stored copy (a same-path
	// replacement of an already-allowed file can only newly fail on size).
	if hasExisting {
		if reason, skip := companionPreflightSkip(file); skip {
			s.recordCompanion(job.report, blueprintID, relPath, companionSkipped, reason)
			return
		}
		if delErr := s.attachmentSvc.Delete(ctx, attachmentOwnerTypeBlueprint, blueprintID, oldAtt.ID); delErr != nil {
			s.logger.With("error", delErr, "blueprint_id", blueprintID, "relative_path", relPath).
				Warn("Failed to remove superseded skill companion before re-upload during re-import")
		}
	}

	if _, err := s.attachmentSvc.Upload(ctx, UploadAttachmentParams{
		TeamID:       job.teamID,
		UserID:       job.userID,
		OwnerType:    attachmentOwnerTypeBlueprint,
		OwnerID:      blueprintID,
		FileName:     path.Base(relPath),
		RelativePath: relPath,
		DeclaredSize: int64(len(file.Content)),
		File:         strings.NewReader(file.Content),
	}); err != nil {
		s.logger.With(
			"error", err,
			"blueprint_id", blueprintID,
			"relative_path", relPath,
			"repo_id", job.repo.ID,
			"team_id", job.teamID,
		).Info("Skipped skill companion during blueprint import")
		s.recordCompanion(job.report, blueprintID, relPath, companionSkipped, companionSkipReason(err))
		return
	}

	outcome := companionImported
	if hasExisting {
		outcome = companionUpdated
	}
	s.recordCompanion(job.report, blueprintID, relPath, outcome, "")
}

// companionPreflightSkip reports the reason a companion cannot be stored at all
// (empty or over the per-file limit), used to protect a stored copy before it is
// deleted for replacement. Other rejections are left to the attachment service.
func companionPreflightSkip(file *external.GitHubFile) (string, bool) {
	switch {
	case len(file.Content) == 0:
		return "Empty file", true
	case int64(len(file.Content)) > MaxAttachmentFileSize:
		return "File exceeds the 5 MB per-file limit", true
	default:
		return "", false
	}
}

// existingAttachment reports whether a companion lookup found a stored row (its
// zero value has an empty ID).
func existingAttachment(att models.Attachment) (models.Attachment, bool) {
	return att, att.ID != ""
}

// companionSkipReason maps an attachment-service error to a human-readable skip
// reason for the import report.
func companionSkipReason(err error) string {
	switch {
	case errors.Is(err, ErrAttachmentStorageNotConfigured):
		return "Attachment storage is not configured"
	case errors.Is(err, ErrAttachmentTooLarge):
		return "File exceeds the 5 MB per-file limit"
	case errors.Is(err, ErrAttachmentTotalSizeExceeded):
		return "Companions exceed the 10 MB total limit for this skill"
	case errors.Is(err, ErrAttachmentDisallowedType):
		return "File type is not allowed"
	case errors.Is(err, ErrAttachmentEmpty):
		return "Empty file"
	case errors.Is(err, ErrInvalidAttachmentRelativePath):
		return "Invalid companion path"
	default:
		return "Failed to store companion file"
	}
}

// recordCompanion appends a companion outcome to the import report and bumps the
// matching counter.
func (s *GitHubAppService) recordCompanion(
	report *models.BlueprintImportReport, blueprintID, relPath, outcome, reason string,
) {
	switch outcome {
	case companionImported, companionUpdated:
		report.TotalCompanionsImported++
	case companionRemoved:
		report.TotalCompanionsRemoved++
	case companionSkipped:
		report.TotalCompanionsSkipped++
	}
	report.CompanionItems = append(report.CompanionItems, models.BlueprintImportCompanion{
		BlueprintID:  blueprintID,
		RelativePath: relPath,
		Outcome:      outcome,
		Reason:       reason,
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
