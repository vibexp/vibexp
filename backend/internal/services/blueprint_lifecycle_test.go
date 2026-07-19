package services

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
)

func lifecycleStr(s string) *string { return &s }

// TestApplyUpdatePath covers the #340 path lifecycle on update: derived paths
// recompute on rename, frozen paths are untouched, an explicit path freezes, and
// a traversal-invalid path is rejected.
func TestApplyUpdatePath(t *testing.T) {
	t.Run("derived path recomputes on rename", func(t *testing.T) {
		bp := &models.Blueprint{
			Type: "claude-code", Subtype: lifecycleStr("others"),
			Slug: "renamed", Path: ".claude/old.md", PathDerived: true,
		}
		require.NoError(t, applyUpdatePath(bp, nil))
		assert.Equal(t, ".claude/renamed.md", bp.Path)
		assert.True(t, bp.PathDerived)
	})

	t.Run("frozen path is untouched", func(t *testing.T) {
		bp := &models.Blueprint{
			Type: "claude-code", Subtype: lifecycleStr("others"),
			Slug: "renamed", Path: "frozen/keep.md", PathDerived: false,
		}
		require.NoError(t, applyUpdatePath(bp, nil))
		assert.Equal(t, "frozen/keep.md", bp.Path)
		assert.False(t, bp.PathDerived)
	})

	t.Run("explicit path freezes", func(t *testing.T) {
		bp := &models.Blueprint{Slug: "s", Path: "s.md", PathDerived: true, Type: "general"}
		require.NoError(t, applyUpdatePath(bp, lifecycleStr("custom/dir.md")))
		assert.Equal(t, "custom/dir.md", bp.Path)
		assert.False(t, bp.PathDerived)
	})

	t.Run("traversal-invalid path rejected", func(t *testing.T) {
		bp := &models.Blueprint{Slug: "s", Type: "general", PathDerived: true}
		assert.ErrorIs(t, applyUpdatePath(bp, lifecycleStr("../escape.md")), ErrInvalidBlueprintPath)
	})
}

// TestSyncSkillName covers the silent skill-name auto-sync: a claude-code/skills
// blueprint's Title becomes its path directory name; other types are untouched.
func TestSyncSkillName(t *testing.T) {
	skill := &models.Blueprint{
		Type: "claude-code", Subtype: lifecycleStr("skills"),
		Path: ".claude/skills/deploy/SKILL.md", Title: "whatever the user typed",
	}
	syncSkillName(skill)
	assert.Equal(t, "deploy", skill.Title)

	other := &models.Blueprint{Type: "general", Title: "keep me", Path: "notes.md"}
	syncSkillName(other)
	assert.Equal(t, "keep me", other.Title)
}

// TestRegenerateRaw covers deterministic raw regeneration: metadata-less content
// stays verbatim, metadata produces a stable frontmatter block, skills inject the
// name, and repeated calls are byte-identical (no churn) with a matching sha.
func TestRegenerateRaw(t *testing.T) {
	s := &BlueprintService{}

	t.Run("metadata-less content regenerates to body verbatim", func(t *testing.T) {
		bp := &models.Blueprint{Type: "general", Content: "# Plain markdown\n\nno frontmatter"}
		raw, sha := s.regenerateRaw(bp)
		assert.Equal(t, "# Plain markdown\n\nno frontmatter", raw)
		assert.Equal(t, computeContentSHA(raw), sha)
	})

	t.Run("metadata produces a frontmatter block; stable across calls", func(t *testing.T) {
		bp := &models.Blueprint{Type: "general", Content: "body", Metadata: map[string]any{"model": "sonnet", "n": 2}}
		raw1, sha1 := s.regenerateRaw(bp)
		raw2, sha2 := s.regenerateRaw(bp)
		assert.Equal(t, raw1, raw2, "regeneration must be deterministic (no churn)")
		assert.Equal(t, sha1, sha2)
		assert.True(t, strings.HasPrefix(raw1, "---\n"))
		assert.Contains(t, raw1, "model: sonnet")
		assert.True(t, strings.HasSuffix(raw1, "---\nbody"))
	})

	t.Run("skills inject name = title (directory)", func(t *testing.T) {
		bp := &models.Blueprint{
			Type: "claude-code", Subtype: lifecycleStr("skills"),
			Path: ".claude/skills/deploy/SKILL.md", Title: "deploy", Content: "Skill body",
		}
		raw, _ := s.regenerateRaw(bp)
		assert.Contains(t, raw, "name: deploy")
	})
}

// newLifecycleSvc builds a BlueprintService with a permissive authz + the given repo.
func newLifecycleSvc(repo *MockBlueprintRepository) *BlueprintService {
	return NewBlueprintService(BlueprintServiceDeps{
		Repo:   repo,
		Authz:  allowAllAuthz{},
		Logger: func() *slog.Logger { l, _ := logtest.New(); return l }(),
	})
}

// TestUpdateBlueprint_RegeneratesRawAndRecomputesPath drives the full update path
// and asserts raw is regenerated, content_sha updated, and a derived path
// recomputed on a slug rename.
func TestUpdateBlueprint_RegeneratesRawAndRecomputesPath(t *testing.T) {
	repo := &MockBlueprintRepository{}
	existing := &models.Blueprint{
		ID: "bp-1", ProjectID: "p1", Slug: "old", UserID: "u1", TeamID: "t1",
		Type: "claude-code", Subtype: lifecycleStr("others"), Title: "T", Content: "old body",
		Path: ".claude/old.md", PathDerived: true, RawContent: "old body", ContentSHA: "stale",
	}
	repo.On("GetByProjectIDAndSlugCrossTeam", mock.Anything, "u1", "p1", "old").Return(existing, nil)
	var saved *models.Blueprint
	repo.On("Update", mock.Anything, mock.MatchedBy(func(bp *models.Blueprint) bool {
		saved = bp
		return true
	})).Return(nil)

	updated, err := newLifecycleSvc(repo).UpdateBlueprintByProjectIDAndSlug(
		"u1", "p1", "old",
		&models.UpdateBlueprintRequest{Slug: lifecycleStr("new"), Content: lifecycleStr("new body")},
	)
	require.NoError(t, err)
	// Derived path recomputed from the renamed slug.
	assert.Equal(t, ".claude/new.md", updated.Path)
	// Raw regenerated (metadata-less -> body verbatim) + content_sha refreshed.
	assert.Equal(t, "new body", saved.RawContent)
	assert.Equal(t, computeContentSHA("new body"), saved.ContentSHA)
	assert.NotEqual(t, "stale", saved.ContentSHA)
}

// TestRestoreBlueprint_RoundTripsRaw verifies restore reinstates the snapshotted
// raw_content exactly (not a regeneration of the restored content).
func TestRestoreBlueprint_RoundTripsRaw(t *testing.T) {
	repo := &MockBlueprintRepository{}
	cvs := &lifecycleContentVersionStub{
		version: &models.ContentVersion{Content: "v1 content", RawContent: "---\nname: original\n---\nv1 content"},
	}
	existing := &models.Blueprint{
		ID: "bp-1", ProjectID: "p1", Slug: "s", UserID: "u1", TeamID: "t1",
		Type: "general", Title: "T", Content: "current", Path: "s.md", RawContent: "current",
	}
	repo.On("GetByProjectIDAndSlug", mock.Anything, "u1", "t1", "p1", "s").Return(existing, nil)
	var saved *models.Blueprint
	repo.On("Update", mock.Anything, mock.MatchedBy(func(bp *models.Blueprint) bool {
		saved = bp
		return true
	})).Return(nil)

	svc := NewBlueprintService(BlueprintServiceDeps{
		Repo: repo, Authz: allowAllAuthz{}, ContentVersionSvc: cvs,
		Logger: func() *slog.Logger { l, _ := logtest.New(); return l }(),
	})
	_, err := svc.RestoreBlueprintVersionInTeam("u1", "t1", "p1", "s", 1)
	require.NoError(t, err)
	// The exact snapshotted raw is reinstated, not regenerated from the content.
	assert.Equal(t, "---\nname: original\n---\nv1 content", saved.RawContent)
	assert.Equal(t, computeContentSHA("---\nname: original\n---\nv1 content"), saved.ContentSHA)
}

// lifecycleContentVersionStub is a minimal ContentVersionServiceInterface: GetVersion
// returns a fixed version, SnapshotIfChanged is a no-op.
type lifecycleContentVersionStub struct{ version *models.ContentVersion }

func (s *lifecycleContentVersionStub) SnapshotIfChanged(context.Context, SnapshotRequest) error {
	return nil
}

func (s *lifecycleContentVersionStub) ListVersions(
	context.Context, string, string, string,
) ([]*models.ContentVersion, error) {
	return nil, nil
}

func (s *lifecycleContentVersionStub) GetVersion(
	_ context.Context, _, _, _ string, _ int,
) (*models.ContentVersion, error) {
	return s.version, nil
}

func (s *lifecycleContentVersionStub) Restore(
	_ context.Context, _, _, _ string, _ int,
) (string, error) {
	return s.version.Content, nil
}
