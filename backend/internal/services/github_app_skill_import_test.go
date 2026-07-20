package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// fakeAttachmentRepo is an in-memory repositories.AttachmentRepository for the
// multi-file skill import tests, so companion reconciliation runs against real
// list/create/delete/sum behavior (paired with the real AttachmentService and an
// in-memory object store).
type fakeAttachmentRepo struct {
	rows map[string]models.Attachment // id -> attachment
}

func newFakeAttachmentRepo() *fakeAttachmentRepo {
	return &fakeAttachmentRepo{rows: make(map[string]models.Attachment)}
}

func (f *fakeAttachmentRepo) Create(_ context.Context, att *models.Attachment) error {
	// Enforce the partial-unique (owner_type, owner_id, relative_path) index so
	// the test catches an upload-before-delete regression in companion reconcile.
	if att.RelativePath != "" {
		for _, existing := range f.rows {
			if existing.OwnerType == att.OwnerType &&
				existing.OwnerID == att.OwnerID &&
				existing.RelativePath == att.RelativePath {
				return repositories.ErrAttachmentRelativePathConflict
			}
		}
	}
	if att.ID == "" {
		att.ID = uuid.NewString()
	}
	att.CreatedAt = time.Unix(int64(len(f.rows)+1), 0)
	f.rows[att.ID] = *att
	return nil
}

func (f *fakeAttachmentRepo) GetByID(_ context.Context, ownerType, ownerID, id string) (*models.Attachment, error) {
	att, ok := f.rows[id]
	if !ok || att.OwnerType != ownerType || att.OwnerID != ownerID {
		return nil, repositories.ErrAttachmentNotFound
	}
	return &att, nil
}

func (f *fakeAttachmentRepo) GetByIDInTeam(_ context.Context, teamID, id string) (*models.Attachment, error) {
	att, ok := f.rows[id]
	if !ok || att.TeamID != teamID {
		return nil, repositories.ErrAttachmentNotFound
	}
	return &att, nil
}

func (f *fakeAttachmentRepo) ListByOwner(_ context.Context, ownerType, ownerID string) ([]models.Attachment, error) {
	var out []models.Attachment
	for _, att := range f.rows {
		if att.OwnerType == ownerType && att.OwnerID == ownerID {
			out = append(out, att)
		}
	}
	return out, nil
}

func (f *fakeAttachmentRepo) SumSizeByOwner(_ context.Context, ownerType, ownerID string) (int64, error) {
	var total int64
	for _, att := range f.rows {
		if att.OwnerType == ownerType && att.OwnerID == ownerID {
			total += att.SizeBytes
		}
	}
	return total, nil
}

func (f *fakeAttachmentRepo) Delete(_ context.Context, ownerType, ownerID, id string) error {
	att, ok := f.rows[id]
	if !ok || att.OwnerType != ownerType || att.OwnerID != ownerID {
		return repositories.ErrAttachmentNotFound
	}
	delete(f.rows, id)
	return nil
}

func (f *fakeAttachmentRepo) DeleteByOwner(_ context.Context, ownerType, ownerID string) ([]models.Attachment, error) {
	var deleted []models.Attachment
	for id, att := range f.rows {
		if att.OwnerType == ownerType && att.OwnerID == ownerID {
			deleted = append(deleted, att)
			delete(f.rows, id)
		}
	}
	return deleted, nil
}

// skillImportFixture wires a GitHubAppService around a mock GitHub client, a mock
// blueprint repository, and a real AttachmentService backed by in-memory fakes.
type skillImportFixture struct {
	svc     *GitHubAppService
	gh      *MockGitHubAppClient
	bpRepo  *MockBlueprintRepository
	attRepo *fakeAttachmentRepo
	store   *fakeObjectStore
	job     *blueprintImportJob
}

func newSkillImportFixture(t *testing.T, storageConfigured bool) *skillImportFixture {
	t.Helper()
	gh := new(MockGitHubAppClient)
	bpRepo := new(MockBlueprintRepository)
	attRepo := newFakeAttachmentRepo()
	store := newFakeObjectStore()

	// When storage is unconfigured, pass an untyped nil so AttachmentService sees
	// a nil interface (a typed nil would defeat its s.store == nil guard).
	var attSvc *AttachmentService
	if storageConfigured {
		attSvc = NewAttachmentService(attRepo, store, newTestLogger())
	} else {
		attSvc = NewAttachmentService(attRepo, nil, newTestLogger())
	}

	svc := &GitHubAppService{
		blueprintRepo: bpRepo,
		githubClient:  gh,
		attachmentSvc: attSvc,
		logger:        newTestLogger(),
	}

	report := &models.BlueprintImportReport{
		SuccessfulItems: []models.BlueprintImportSuccess{},
		FailedItems:     []models.BlueprintImportFailed{},
		SkippedItems:    []models.BlueprintImportSkipped{},
		UpdatedItems:    []models.BlueprintImportUpdated{},
		ConflictItems:   []models.BlueprintImportConflict{},
		UpToDateItems:   []models.BlueprintImportUpToDate{},
		CompanionItems:  []models.BlueprintImportCompanion{},
	}
	job := &blueprintImportJob{
		installationID: 1,
		userID:         "user-1",
		teamID:         "team-1",
		projectID:      "proj-1",
		report:         report,
		repo: &models.GitHubRepository{
			ID:       1,
			Name:     "repo",
			FullName: "org/repo",
			HTMLURL:  "https://github.com/org/repo",
			Owner:    models.GitHubRepositoryOwner{Login: "org"},
		},
	}
	return &skillImportFixture{svc: svc, gh: gh, bpRepo: bpRepo, attRepo: attRepo, store: store, job: job}
}

// expectFreshImport makes the blueprint repo treat every SKILL.md as new.
func (f *skillImportFixture) expectFreshImport() {
	f.bpRepo.On("GetByProjectIDAndPath", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, repositories.ErrBlueprintNotFound)
	f.bpRepo.On("GetByProjectIDAndSlug", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, repositories.ErrBlueprintNotFound)
	f.bpRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
}

// scan drives scanBlueprintDirectory over the ".claude" tree with the given
// listing.
func (f *skillImportFixture) scan(t *testing.T, files []*external.GitHubFile) {
	t.Helper()
	const dir = ".claude"
	f.gh.On("GetDirectoryContentsRecursive", mock.Anything, int64(1), "org", "repo", dir).
		Return(files, nil)
	require.NoError(t, f.svc.scanBlueprintDirectory(context.Background(), f.job, dir))
}

func companionByPath(report *models.BlueprintImportReport, relPath string) (models.BlueprintImportCompanion, bool) {
	for _, c := range report.CompanionItems {
		if c.RelativePath == relPath {
			return c, true
		}
	}
	return models.BlueprintImportCompanion{}, false
}

func skillFile(path, content string) *external.GitHubFile {
	return &external.GitHubFile{Path: path, Content: content, BlobSHA: "sha-" + path}
}

// A skill directory imports as a blueprint (SKILL.md) plus companion attachments
// carrying correct skill-relative paths; non-.md companions of allowed types are
// stored, and companion outcomes are reported distinctly from the blueprint.
func TestScanBlueprintDirectory_SkillUnit_ImportsCompanions(t *testing.T) {
	f := newSkillImportFixture(t, true)
	f.expectFreshImport()

	files := []*external.GitHubFile{
		skillFile(".claude/skills/deploy/SKILL.md", "---\nname: Deploy\n---\nbody"),
		skillFile(".claude/skills/deploy/reference/data.json", `{"k":"v"}`),
		skillFile(".claude/skills/deploy/templates/note.txt", "hello template"),
	}
	f.scan(t, files)

	report := f.job.report
	// One blueprint (SKILL.md), two companions.
	assert.Equal(t, 1, report.TotalSuccessful, "SKILL.md imports as a blueprint")
	assert.Equal(t, 2, report.TotalCompanionsImported)
	assert.Equal(t, 0, report.TotalCompanionsSkipped)
	require.Len(t, report.SuccessfulItems, 1)

	blueprintID := report.SuccessfulItems[0].BlueprintID
	stored, err := f.attRepo.ListByOwner(context.Background(), attachmentOwnerTypeBlueprint, blueprintID)
	require.NoError(t, err)
	require.Len(t, stored, 2)

	paths := map[string]bool{}
	for _, att := range stored {
		paths[att.RelativePath] = true
		assert.NotEmpty(t, att.RelativePath)
	}
	assert.True(t, paths["reference/data.json"], "companion keeps its skill-relative subpath")
	assert.True(t, paths["templates/note.txt"])

	// Companion outcomes are recorded distinctly from the blueprint outcome.
	c, ok := companionByPath(report, "reference/data.json")
	require.True(t, ok)
	assert.Equal(t, companionImported, c.Outcome)
	assert.Equal(t, blueprintID, c.BlueprintID)
}

// A disallowed companion type (a script) is skipped and reported, while the
// SKILL.md and allowed companions still import.
func TestScanBlueprintDirectory_SkillUnit_DisallowedCompanionSkipped(t *testing.T) {
	f := newSkillImportFixture(t, true)
	f.expectFreshImport()

	files := []*external.GitHubFile{
		skillFile(".claude/skills/deploy/SKILL.md", "body"),
		skillFile(".claude/skills/deploy/scripts/helper.py", "print('x')"),
		skillFile(".claude/skills/deploy/notes.txt", "notes"),
	}
	f.scan(t, files)

	report := f.job.report
	assert.Equal(t, 1, report.TotalSuccessful, "SKILL.md always imports")
	assert.Equal(t, 1, report.TotalCompanionsImported, "the .txt companion imports")
	assert.Equal(t, 1, report.TotalCompanionsSkipped, "the .py script is rejected by the allowlist")

	c, ok := companionByPath(report, "scripts/helper.py")
	require.True(t, ok)
	assert.Equal(t, companionSkipped, c.Outcome)
	assert.Equal(t, "File type is not allowed", c.Reason)
}

// Non-.md files OUTSIDE a skill directory stay markdown-only (skipped), proving
// the filter is lifted only within skill units.
func TestScanBlueprintDirectory_StandaloneNonMarkdownStillSkipped(t *testing.T) {
	f := newSkillImportFixture(t, true)
	f.expectFreshImport()

	files := []*external.GitHubFile{
		skillFile(".claude/agents/reviewer.md", "reviewer"),
		skillFile(".claude/agents/helper.py", "print('x')"), // standalone, not under a skill dir
	}
	f.scan(t, files)

	report := f.job.report
	assert.Equal(t, 1, report.TotalSuccessful, "the .md agent imports")
	assert.Equal(t, 1, report.TotalSkipped, "the standalone .py is skipped as non-markdown")
	assert.Equal(t, 0, report.TotalCompanionsImported)
	require.Len(t, report.SkippedItems, 1)
	assert.Equal(t, "Not a markdown file", report.SkippedItems[0].Reason)
}

// An oversized companion is skipped and reported per file; the SKILL.md still
// imports.
func TestScanBlueprintDirectory_SkillUnit_OversizedCompanionSkipped(t *testing.T) {
	f := newSkillImportFixture(t, true)
	f.expectFreshImport()

	big := strings.Repeat("a", int(MaxAttachmentFileSize)+1)
	files := []*external.GitHubFile{
		skillFile(".claude/skills/deploy/SKILL.md", "body"),
		skillFile(".claude/skills/deploy/big.txt", big),
	}
	f.scan(t, files)

	report := f.job.report
	assert.Equal(t, 1, report.TotalSuccessful)
	assert.Equal(t, 0, report.TotalCompanionsImported)
	assert.Equal(t, 1, report.TotalCompanionsSkipped)

	c, ok := companionByPath(report, "big.txt")
	require.True(t, ok)
	assert.Equal(t, companionSkipped, c.Outcome)
	assert.Contains(t, c.Reason, "5 MB")
}

// With object storage unconfigured, the SKILL.md still imports and every
// companion is skipped with a storage-not-configured reason.
func TestScanBlueprintDirectory_SkillUnit_StorageUnconfigured(t *testing.T) {
	f := newSkillImportFixture(t, false) // storage disabled
	f.expectFreshImport()

	files := []*external.GitHubFile{
		skillFile(".claude/skills/deploy/SKILL.md", "body"),
		skillFile(".claude/skills/deploy/notes.txt", "notes"),
	}
	f.scan(t, files)

	report := f.job.report
	assert.Equal(t, 1, report.TotalSuccessful, "SKILL.md imports even without storage")
	assert.Equal(t, 0, report.TotalCompanionsImported)
	assert.Equal(t, 1, report.TotalCompanionsSkipped)

	c, ok := companionByPath(report, "notes.txt")
	require.True(t, ok)
	assert.Equal(t, companionSkipped, c.Outcome)
	assert.Equal(t, "Attachment storage is not configured", c.Reason)
}

// Re-import of an unedited skill reconciles the companion set by relative_path:
// a new companion is added, a changed one is replaced, and an absent one removed.
func TestScanBlueprintDirectory_SkillUnit_ReimportReconciles(t *testing.T) {
	f := newSkillImportFixture(t, true)

	// Existing blueprint (unedited: content_sha == source_content_sha) whose repo
	// file changed (blob SHA differs) -> update path drives companion reconcile.
	existing := &models.Blueprint{
		ID:               "bp-1",
		ProjectID:        "proj-1",
		Slug:             "claude-skills-deploy-from-repo",
		Path:             ".claude/skills/deploy/SKILL.md",
		ContentSHA:       "old",
		SourceContentSHA: "old",
		Source:           &models.BlueprintSource{BlobSHA: "old-blob"},
	}
	f.bpRepo.On("GetByProjectIDAndPath", mock.Anything, mock.Anything, mock.Anything, "proj-1", ".claude/skills/deploy/SKILL.md").
		Return(existing, nil)
	f.bpRepo.On("UpdateOnReimport", mock.Anything, mock.Anything).Return(nil)

	// Seed the currently-stored companion set for bp-1: keep.txt (unchanged),
	// change.txt (will change), gone.txt (will be removed).
	seed := func(rel, content string) {
		_, err := f.svc.attachmentSvc.Upload(context.Background(), UploadAttachmentParams{
			TeamID: "team-1", UserID: "user-1", OwnerType: attachmentOwnerTypeBlueprint, OwnerID: "bp-1",
			FileName: rel, RelativePath: rel, DeclaredSize: int64(len(content)), File: strings.NewReader(content),
		})
		require.NoError(t, err)
	}
	seed("keep.txt", "keep")
	seed("change.txt", "old-content")
	seed("gone.txt", "gone")
	require.Len(t, f.attRepo.rows, 3)

	files := []*external.GitHubFile{
		skillFile(".claude/skills/deploy/SKILL.md", "new body"),
		skillFile(".claude/skills/deploy/keep.txt", "keep"),           // unchanged (same size)
		skillFile(".claude/skills/deploy/change.txt", "new-content2"), // changed (different size)
		skillFile(".claude/skills/deploy/added.txt", "added"),         // new
	}
	f.scan(t, files)

	report := f.job.report
	assert.Equal(t, 1, report.TotalUpdated, "SKILL.md refreshed")
	assert.Equal(t, 1, report.TotalCompanionsRemoved, "gone.txt removed")

	// Final stored set: keep.txt, change.txt (replaced), added.txt.
	stored, err := f.attRepo.ListByOwner(context.Background(), attachmentOwnerTypeBlueprint, "bp-1")
	require.NoError(t, err)
	got := map[string]int64{}
	for _, att := range stored {
		got[att.RelativePath] = att.SizeBytes
	}
	require.Len(t, got, 3)
	assert.Contains(t, got, "keep.txt")
	assert.Contains(t, got, "added.txt")
	assert.Equal(t, int64(len("new-content2")), got["change.txt"], "change.txt replaced with new bytes")
	assert.NotContains(t, got, "gone.txt")

	// added.txt reported imported, change.txt reported updated, gone.txt removed.
	added, ok := companionByPath(report, "added.txt")
	require.True(t, ok)
	assert.Equal(t, companionImported, added.Outcome)
	changed, ok := companionByPath(report, "change.txt")
	require.True(t, ok)
	assert.Equal(t, companionUpdated, changed.Outcome)
	removed, ok := companionByPath(report, "gone.txt")
	require.True(t, ok)
	assert.Equal(t, companionRemoved, removed.Outcome)
}

// A VibeXP-edited skill (conflict) leaves its companions untouched.
func TestScanBlueprintDirectory_SkillUnit_ConflictLeavesCompanionsUntouched(t *testing.T) {
	f := newSkillImportFixture(t, true)

	existing := &models.Blueprint{
		ID:               "bp-1",
		ProjectID:        "proj-1",
		Slug:             "claude-skills-deploy-from-repo",
		Path:             ".claude/skills/deploy/SKILL.md",
		ContentSHA:       "edited", // != source -> VibeXP-edited
		SourceContentSHA: "imported",
		Source:           &models.BlueprintSource{BlobSHA: "old-blob"},
	}
	f.bpRepo.On("GetByProjectIDAndPath", mock.Anything, mock.Anything, mock.Anything, "proj-1", ".claude/skills/deploy/SKILL.md").
		Return(existing, nil)

	_, err := f.svc.attachmentSvc.Upload(context.Background(), UploadAttachmentParams{
		TeamID: "team-1", UserID: "user-1", OwnerType: attachmentOwnerTypeBlueprint, OwnerID: "bp-1",
		FileName: "keep.txt", RelativePath: "keep.txt", DeclaredSize: 4, File: strings.NewReader("keep"),
	})
	require.NoError(t, err)

	files := []*external.GitHubFile{
		skillFile(".claude/skills/deploy/SKILL.md", "changed upstream"),
		skillFile(".claude/skills/deploy/added.txt", "added"),
	}
	f.scan(t, files)

	report := f.job.report
	assert.Equal(t, 1, report.TotalConflicts, "SKILL.md left untouched as a conflict")
	assert.Equal(t, 0, report.TotalCompanionsImported, "companions untouched on conflict")
	assert.Empty(t, report.CompanionItems)

	stored, err := f.attRepo.ListByOwner(context.Background(), attachmentOwnerTypeBlueprint, "bp-1")
	require.NoError(t, err)
	require.Len(t, stored, 1, "the pre-existing companion is left as-is")
	assert.Equal(t, "keep.txt", stored[0].RelativePath)
}
