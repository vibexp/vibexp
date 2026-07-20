package services

import (
	"context"
	"errors"
	"io"
	"path"
	"sort"
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

// errFixtureNotFound stands in for a missing repo path so the blueprint scan
// skips that well-known location (mirrors the real client's not-found).
var errFixtureNotFound = errors.New("fixture path not found")

// ---------------------------------------------------------------------------
// Dry-run materializer (#344)
//
// A test-only, deliberately-unshipped reconstruction of a repository file tree
// from imported blueprints + their skill companions. It is defined in a _test.go
// file so it can never leak into the API surface. It is the epic's proof harness:
// import a repo, change nothing, materialize, and assert the bytes are identical
// to what was imported.
// ---------------------------------------------------------------------------

// materialize maps a project's blueprints (+ their skill companions) back to a
// deterministic {repo-relative path -> bytes} file tree: each blueprint's
// raw_content lands at its canonical path, and each blueprint-owned attachment
// carrying a relative_path lands at dir(path)/relative_path. A path collision is
// a hard error (two sources writing the same file). Inputs are processed in
// sorted order so a collision is reported deterministically.
func materialize(
	ctx context.Context, blueprints []*models.Blueprint, attSvc AttachmentServiceInterface,
) (map[string][]byte, error) {
	files := make(map[string][]byte)

	sorted := append([]*models.Blueprint(nil), blueprints...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	for _, bp := range sorted {
		if _, dup := files[bp.Path]; dup {
			return nil, collisionErr(bp.Path)
		}
		files[bp.Path] = []byte(bp.RawContent)

		companions, err := companionFiles(ctx, attSvc, bp)
		if err != nil {
			return nil, err
		}
		for _, c := range companions {
			if _, dup := files[c.path]; dup {
				return nil, collisionErr(c.path)
			}
			files[c.path] = c.bytes
		}
	}
	return files, nil
}

type materializedCompanion struct {
	path  string
	bytes []byte
}

// companionFiles resolves one blueprint's skill companions to their repo paths +
// bytes, in relative_path order.
func companionFiles(
	ctx context.Context, attSvc AttachmentServiceInterface, bp *models.Blueprint,
) ([]materializedCompanion, error) {
	list, err := attSvc.List(ctx, "blueprint", bp.ID)
	if err != nil {
		return nil, err
	}
	dir := path.Dir(bp.Path)
	out := make([]materializedCompanion, 0, len(list.Attachments))
	for i := range list.Attachments {
		att := list.Attachments[i]
		if att.RelativePath == "" {
			continue
		}
		rc, dlErr := attSvc.Download(ctx, &att)
		if dlErr != nil {
			return nil, dlErr
		}
		b, readErr := io.ReadAll(rc)
		if closeErr := rc.Close(); closeErr != nil && readErr == nil {
			readErr = closeErr
		}
		if readErr != nil {
			return nil, readErr
		}
		out = append(out, materializedCompanion{path: path.Join(dir, att.RelativePath), bytes: b})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out, nil
}

func collisionErr(p string) error {
	return &materializeCollision{path: p}
}

type materializeCollision struct{ path string }

func (e *materializeCollision) Error() string {
	return "materialize: path collision at " + e.path
}

// ---------------------------------------------------------------------------
// In-memory repository fakes (no DB) — enough to run the real import service and
// the real BlueprintService + ContentVersionService end to end.
// ---------------------------------------------------------------------------

// inMemoryBlueprintRepo is an in-memory repositories.BlueprintRepository. Only
// Create / GetByProjectIDAndSlug / GetByProjectIDAndPath / Update carry real
// behavior (the import + update/restore paths); the rest are stubs.
type inMemoryBlueprintRepo struct {
	byID map[string]*models.Blueprint
}

func newInMemoryBlueprintRepo() *inMemoryBlueprintRepo {
	return &inMemoryBlueprintRepo{byID: make(map[string]*models.Blueprint)}
}

func (r *inMemoryBlueprintRepo) store(bp *models.Blueprint) {
	cp := *bp
	r.byID[bp.ID] = &cp
}

func (r *inMemoryBlueprintRepo) Create(_ context.Context, bp *models.Blueprint) error {
	r.store(bp)
	return nil
}

func (r *inMemoryBlueprintRepo) Update(_ context.Context, bp *models.Blueprint) error {
	if _, ok := r.byID[bp.ID]; !ok {
		return repositories.ErrBlueprintNotFound
	}
	r.store(bp)
	return nil
}

func (r *inMemoryBlueprintRepo) UpdateOnReimport(ctx context.Context, bp *models.Blueprint) error {
	return r.Update(ctx, bp)
}

func (r *inMemoryBlueprintRepo) GetByProjectIDAndSlug(
	_ context.Context, _, _, projectID, slug string,
) (*models.Blueprint, error) {
	for _, bp := range r.byID {
		if bp.ProjectID == projectID && bp.Slug == slug {
			cp := *bp
			return &cp, nil
		}
	}
	return nil, repositories.ErrBlueprintNotFound
}

func (r *inMemoryBlueprintRepo) GetByProjectIDAndPath(
	_ context.Context, _, _, projectID, p string,
) (*models.Blueprint, error) {
	for _, bp := range r.byID {
		if bp.ProjectID == projectID && bp.Path == p {
			cp := *bp
			return &cp, nil
		}
	}
	return nil, repositories.ErrBlueprintNotFound
}

// forProject returns every stored blueprint of the test project (materializer
// input).
func (r *inMemoryBlueprintRepo) forProject() []*models.Blueprint {
	var out []*models.Blueprint
	for _, bp := range r.byID {
		if bp.ProjectID == mzProjectID {
			cp := *bp
			out = append(out, &cp)
		}
	}
	return out
}

// Unused-on-this-path stubs.
func (r *inMemoryBlueprintRepo) GetByID(context.Context, string, string, string) (*models.Blueprint, error) {
	return nil, repositories.ErrBlueprintNotFound
}

func (r *inMemoryBlueprintRepo) GetByIDCrossTeam(context.Context, string, string) (*models.Blueprint, error) {
	return nil, repositories.ErrBlueprintNotFound
}

func (r *inMemoryBlueprintRepo) GetByProjectIDAndSlugCrossTeam(
	context.Context, string, string, string,
) (*models.Blueprint, error) {
	return nil, repositories.ErrBlueprintNotFound
}

func (r *inMemoryBlueprintRepo) List(
	context.Context, string, repositories.BlueprintFilters,
) ([]models.Blueprint, int, error) {
	return nil, 0, nil
}

func (r *inMemoryBlueprintRepo) Delete(context.Context, string, string, string) error { return nil }

func (r *inMemoryBlueprintRepo) GetStats(context.Context, string) (*models.BlueprintStatsResponse, error) {
	return &models.BlueprintStatsResponse{}, nil
}

func (r *inMemoryBlueprintRepo) GetNamesByIDsCrossTeam(
	context.Context, string, []string,
) (map[string]string, error) {
	return map[string]string{}, nil
}

// inMemoryContentVersionRepo is an in-memory repositories.ContentVersionRepository:
// Create assigns the next version_number per (resourceType, resourceID) and
// preserves RawContent so a restore reproduces the snapshotted raw byte-for-byte.
type inMemoryContentVersionRepo struct {
	versions map[string][]*models.ContentVersion // key: resourceType/resourceID
}

func newInMemoryContentVersionRepo() *inMemoryContentVersionRepo {
	return &inMemoryContentVersionRepo{versions: make(map[string][]*models.ContentVersion)}
}

func cvKey(resourceType, resourceID string) string { return resourceType + "/" + resourceID }

func (r *inMemoryContentVersionRepo) Create(_ context.Context, v *models.ContentVersion) error {
	key := cvKey(v.ResourceType, v.ResourceID)
	v.ID = uuid.NewString()
	v.VersionNumber = len(r.versions[key]) + 1
	v.CreatedAt = time.Unix(int64(v.VersionNumber), 0)
	cp := *v
	r.versions[key] = append(r.versions[key], &cp)
	return nil
}

func (r *inMemoryContentVersionRepo) ListByResource(
	_ context.Context, _, resourceType, resourceID string,
) ([]*models.ContentVersion, error) {
	src := r.versions[cvKey(resourceType, resourceID)]
	out := make([]*models.ContentVersion, len(src))
	for i, v := range src { // newest-first
		cp := *v
		out[len(src)-1-i] = &cp
	}
	return out, nil
}

func (r *inMemoryContentVersionRepo) GetByVersionNumber(
	_ context.Context, _, resourceType, resourceID string, versionNumber int,
) (*models.ContentVersion, error) {
	for _, v := range r.versions[cvKey(resourceType, resourceID)] {
		if v.VersionNumber == versionNumber {
			cp := *v
			return &cp, nil
		}
	}
	return nil, repositories.ErrContentVersionNotFound
}

func (r *inMemoryContentVersionRepo) PruneToCap(_ context.Context, _, _ string, _ int) error {
	return nil // retention is irrelevant to a fidelity test
}

// ---------------------------------------------------------------------------
// Fixture + harness
// ---------------------------------------------------------------------------

const (
	mzUserID    = "user-1"
	mzTeamID    = "team-1"
	mzProjectID = "proj-1"
)

// fixtureFiles is the source-of-truth repo tree: rules, commands, CLAUDE.md, a
// nested-YAML-frontmatter agent, and one multi-file skill (SKILL.md + non-md
// companions). Authored byte-exact; compared as []byte.
func fixtureFiles() map[string]string {
	return map[string]string{
		"CLAUDE.md":                  "# Project guidance\n\nUse tabs, not spaces.\n",
		".claude/commands/deploy.md": "---\ndescription: Deploy the app\n---\nRun the deploy script.\n",
		".codex/rules/style.md":      "# Style rules\n\nPrefer composition.\n",
		// Nested YAML frontmatter — proves import preserves the raw bytes verbatim
		// (the #336 parser reads it into Metadata, but raw_content stays original).
		".claude/agents/complex.md": "---\nname: Complex Agent\nconfig:\n  model: sonnet\n  tools:\n    - read\n    - write\nnested:\n  deep:\n    value: 42\n---\n# Complex\n\nBody content.\n",
		// Multi-file skill: SKILL.md + companions of allowed types.
		".claude/skills/deploy/SKILL.md":            "---\nname: deploy\n---\nDeploy skill instructions.\n",
		".claude/skills/deploy/reference/data.json": "{\n  \"key\": \"value\"\n}\n",
		".claude/skills/deploy/templates/note.txt":  "A companion template.\n",
	}
}

// fixtureBytes is the expected materialized tree as bytes.
func fixtureBytes() map[string][]byte {
	out := make(map[string][]byte)
	for p, c := range fixtureFiles() {
		out[p] = []byte(c)
	}
	return out
}

// materializerHarness wires the real import service + real BlueprintService +
// real ContentVersionService against in-memory fakes and a fixture-backed GitHub
// client.
type materializerHarness struct {
	importSvc    *GitHubAppService
	blueprintSvc *BlueprintService
	bpRepo       *inMemoryBlueprintRepo
	attSvc       AttachmentServiceInterface
	job          *blueprintImportJob
}

func newMaterializerHarness(t *testing.T) *materializerHarness {
	t.Helper()
	gh := new(MockGitHubAppClient)
	setupFixtureGitHubClient(gh, fixtureFiles())

	bpRepo := newInMemoryBlueprintRepo()
	attSvc := NewAttachmentService(newFakeAttachmentRepo(), newFakeObjectStore(), newTestLogger())

	importSvc := &GitHubAppService{
		blueprintRepo: bpRepo,
		githubClient:  gh,
		attachmentSvc: attSvc,
		logger:        newTestLogger(),
	}

	cvs := NewContentVersionService(
		newInMemoryContentVersionRepo(), nil, newTestLogger(),
		ContentVersionAdapter{ResourceType: "blueprint", RetentionCap: 50},
	)
	blueprintSvc := NewBlueprintService(BlueprintServiceDeps{
		Repo:              bpRepo,
		Authz:             allowAllAuthz{},
		ContentVersionSvc: cvs,
		Logger:            newTestLogger(),
	})

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
		installationID: 1, userID: mzUserID, teamID: mzTeamID, projectID: mzProjectID, report: report,
		repo: &models.GitHubRepository{
			ID: 1, Name: "repo", FullName: "org/repo", HTMLURL: "https://github.com/org/repo",
			Owner: models.GitHubRepositoryOwner{Login: "org"},
		},
	}
	return &materializerHarness{
		importSvc: importSvc, blueprintSvc: blueprintSvc, bpRepo: bpRepo, attSvc: attSvc, job: job,
	}
}

// setupFixtureGitHubClient wires the mock GitHub client to serve the fixture: the
// blueprint scan lists each well-known directory recursively and reads the three
// root files, so absent locations return an error (scan skips them).
func setupFixtureGitHubClient(gh *MockGitHubAppClient, files map[string]string) {
	byDir := map[string][]*external.GitHubFile{}
	rootFile := map[string]*external.GitHubFile{}
	for p, content := range files {
		gf := &external.GitHubFile{Path: p, Content: content, BlobSHA: "blob-" + p}
		if p == "CLAUDE.md" || p == "CURSOR.md" || p == "AGENTS.md" {
			rootFile[p] = gf
			continue
		}
		top := strings.SplitN(p, "/", 2)[0] // top-level dir, e.g. ".claude"
		byDir[top] = append(byDir[top], gf)
	}
	for _, dir := range []string{".claude", ".cursor", ".codex", ".agents"} {
		if fs, ok := byDir[dir]; ok {
			gh.On("GetDirectoryContentsRecursive", mock.Anything, int64(1), "org", "repo", dir).Return(fs, nil)
		} else {
			gh.On("GetDirectoryContentsRecursive", mock.Anything, int64(1), "org", "repo", dir).
				Return(nil, errFixtureNotFound)
		}
	}
	for _, name := range []string{"CLAUDE.md", "CURSOR.md", "AGENTS.md"} {
		if gf, ok := rootFile[name]; ok {
			gh.On("GetFileContent", mock.Anything, int64(1), "org", "repo", name).Return(gf, nil)
		} else {
			gh.On("GetFileContent", mock.Anything, int64(1), "org", "repo", name).Return(nil, errFixtureNotFound)
		}
	}
}

// TestMaterializer_ImportRoundTripByteIdentical is the epic's proof: import a
// fixture repo (rules, commands, CLAUDE.md, nested-YAML frontmatter, a multi-file
// skill), change nothing, materialize, and assert the reconstructed tree is
// byte-identical to the fixture — including the skill companions at their
// skill-relative paths.
func TestMaterializer_ImportRoundTripByteIdentical(t *testing.T) {
	ctx := context.Background()
	h := newMaterializerHarness(t)

	require.NoError(t, h.importSvc.runBlueprintScan(ctx, h.job))
	// Every fixture file imported as a blueprint or a companion (nothing skipped).
	assert.Zero(t, h.job.report.TotalSkipped, "no fixture file should be skipped")
	assert.Zero(t, h.job.report.TotalCompanionsSkipped)

	got, err := materialize(ctx, h.bpRepo.forProject(), h.attSvc)
	require.NoError(t, err)

	assertFileTreeEqual(t, fixtureBytes(), got)
}

// TestMaterializer_EditRestoreRoundTrip proves the version round-trip: after
// editing a blueprint (raw regenerated) and restoring the prior version, the
// materialized file returns to its original bytes.
func TestMaterializer_EditRestoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	h := newMaterializerHarness(t)
	require.NoError(t, h.importSvc.runBlueprintScan(ctx, h.job))

	// Pick the simple codex rule (no frontmatter, no companions).
	const targetPath = ".codex/rules/style.md"
	target := blueprintAtPath(t, h.bpRepo, targetPath)
	originalRaw := target.RawContent
	require.Equal(t, fixtureFiles()[targetPath], originalRaw)

	// Edit its content -> raw regenerated, prior (imported) raw snapshotted as v1.
	edited := "# Style rules\n\nEDITED: prefer inheritance.\n"
	_, err := h.blueprintSvc.UpdateBlueprintByProjectIDAndSlugInTeam(
		mzUserID, mzTeamID, mzProjectID, target.Slug,
		&models.UpdateBlueprintRequest{Content: &edited},
	)
	require.NoError(t, err)

	afterEdit, err := materialize(ctx, h.bpRepo.forProject(), h.attSvc)
	require.NoError(t, err)
	assert.NotEqual(t, originalRaw, string(afterEdit[targetPath]), "edit must change the materialized bytes")

	// Restore version 1 -> raw reinstated byte-for-byte.
	_, err = h.blueprintSvc.RestoreBlueprintVersionInTeam(mzUserID, mzTeamID, mzProjectID, target.Slug, 1)
	require.NoError(t, err)

	afterRestore, err := materialize(ctx, h.bpRepo.forProject(), h.attSvc)
	require.NoError(t, err)
	assert.Equal(t, originalRaw, string(afterRestore[targetPath]),
		"restore must reproduce the original raw bytes")
	// The whole tree is byte-identical to the fixture again.
	assertFileTreeEqual(t, fixtureBytes(), afterRestore)
}

func blueprintAtPath(t *testing.T, repo *inMemoryBlueprintRepo, p string) *models.Blueprint {
	t.Helper()
	for _, bp := range repo.forProject() {
		if bp.Path == p {
			return bp
		}
	}
	t.Fatalf("no imported blueprint at path %q", p)
	return nil
}

func assertFileTreeEqual(t *testing.T, want, got map[string][]byte) {
	t.Helper()
	wantPaths := make([]string, 0, len(want))
	for p := range want {
		wantPaths = append(wantPaths, p)
	}
	sort.Strings(wantPaths)
	gotPaths := make([]string, 0, len(got))
	for p := range got {
		gotPaths = append(gotPaths, p)
	}
	sort.Strings(gotPaths)
	require.Equal(t, wantPaths, gotPaths, "materialized path set must match the fixture exactly")
	for _, p := range wantPaths {
		assert.Equal(t, want[p], got[p], "byte mismatch at %s", p)
	}
}
