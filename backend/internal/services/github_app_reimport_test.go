package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
)

// TestGitBlobSHA pins the Git blob hashing against a known reference
// (`printf 'hello' | git hash-object --stdin`).
func TestGitBlobSHA(t *testing.T) {
	assert.Equal(t, "b6fc4c620b67d95f953a5c1c1230aaab5db5a1b0", gitBlobSHA("hello"))
	// Empty blob SHA is the well-known git constant.
	assert.Equal(t, "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391", gitBlobSHA(""))
}

// TestReimportOutcome covers the update/up-to-date/conflict decision matrix.
func TestReimportOutcome(t *testing.T) {
	rawAtImport := "---\nname: X\n---\nbody"
	blob := gitBlobSHA(rawAtImport)

	tests := []struct {
		name        string
		existingRaw string
		source      *models.BlueprintSource
		fileBlobSHA string
		want        reimportDecision
	}{
		{
			name:        "repo file unchanged -> up-to-date",
			existingRaw: rawAtImport,
			source:      &models.BlueprintSource{BlobSHA: blob},
			fileBlobSHA: blob, // same blob as stored
			want:        reimportUpToDate,
		},
		{
			name:        "repo changed + blueprint unedited -> update",
			existingRaw: rawAtImport, // still byte-identical to the imported bytes
			source:      &models.BlueprintSource{BlobSHA: blob},
			fileBlobSHA: "newblobsha",
			want:        reimportUpdate,
		},
		{
			name:        "repo changed + blueprint edited in VibeXP -> conflict",
			existingRaw: "---\nname: X\n---\nEDITED", // raw diverged from import
			source:      &models.BlueprintSource{BlobSHA: blob},
			fileBlobSHA: "newblobsha",
			want:        reimportConflict,
		},
		{
			name:        "no provenance -> conflict (cannot confirm unedited)",
			existingRaw: rawAtImport,
			source:      nil,
			fileBlobSHA: "newblobsha",
			want:        reimportConflict,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			existing := &models.Blueprint{RawContent: tc.existingRaw, Source: tc.source}
			got := reimportOutcome(existing, &external.GitHubFile{BlobSHA: tc.fileBlobSHA})
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestBuildImportedBlueprint_Provenance verifies an imported blueprint carries
// verbatim path + raw + full provenance (repo/commit/blob/imported-at).
func TestBuildImportedBlueprint_Provenance(t *testing.T) {
	svc, _ := newSanitizeTestService()
	job := &blueprintImportJob{
		userID: "u", teamID: "t", projectID: "p",
		repo: &models.GitHubRepository{
			Name: "r", FullName: "o/r", HTMLURL: "https://github.com/o/r",
		},
		sourceCommitSHA: "commit-abc",
	}
	file := &external.GitHubFile{Path: ".claude/agents/x.md", Content: "# body", BlobSHA: "blob-xyz"}

	bp := svc.buildImportedBlueprint(job, file, "claude-code", "sub-agents")

	assert.Equal(t, ".claude/agents/x.md", bp.Path)
	assert.False(t, bp.PathDerived)
	assert.Equal(t, "# body", bp.RawContent)
	assert.Equal(t, computeContentSHA("# body"), bp.ContentSHA)
	require.NotNil(t, bp.Source)
	assert.Equal(t, "https://github.com/o/r", bp.Source.Repo)
	assert.Equal(t, "commit-abc", bp.Source.CommitSHA)
	assert.Equal(t, "blob-xyz", bp.Source.BlobSHA)
	require.NotNil(t, bp.Source.ImportedAt)
}

// TestReconcileReimport_Update verifies a changed repo file for an unedited
// blueprint refreshes it via UpdateOnReimport and reports "updated".
func TestReconcileReimport_Update(t *testing.T) {
	svc, repo := newSanitizeTestService()
	report := &models.BlueprintImportReport{}
	rawAtImport := "old raw"
	existing := &models.Blueprint{
		ID: "bp-1", Slug: "s", RawContent: rawAtImport,
		Source: &models.BlueprintSource{BlobSHA: gitBlobSHA(rawAtImport)},
	}
	imported := &models.Blueprint{
		Content: "new content", RawContent: "new raw", ContentSHA: "new-sha",
		Title: "New", Type: "claude-code", Path: ".claude/agents/x.md",
		Source: &models.BlueprintSource{BlobSHA: "new-blob"},
	}
	var saved *models.Blueprint
	repo.On("UpdateOnReimport", mock.Anything, mock.MatchedBy(func(bp *models.Blueprint) bool {
		saved = bp
		return true
	})).Return(nil)

	job := &blueprintImportJob{teamID: "t", report: report}
	svc.reconcileReimport(context.Background(), job, &external.GitHubFile{Path: ".claude/agents/x.md", BlobSHA: "new-blob"}, existing, imported)

	assert.Equal(t, 1, report.TotalUpdated)
	require.Len(t, report.UpdatedItems, 1)
	assert.Equal(t, "bp-1", report.UpdatedItems[0].BlueprintID)
	// Existing was refreshed in place (identity preserved, content/raw/provenance updated).
	assert.Equal(t, "bp-1", saved.ID)
	assert.Equal(t, "new raw", saved.RawContent)
	assert.Equal(t, "new-blob", saved.Source.BlobSHA)
	repo.AssertExpectations(t)
}

// TestReconcileReimport_UpToDate verifies an unchanged repo file is a no-op.
func TestReconcileReimport_UpToDate(t *testing.T) {
	svc, repo := newSanitizeTestService()
	report := &models.BlueprintImportReport{}
	existing := &models.Blueprint{ID: "bp-1", Source: &models.BlueprintSource{BlobSHA: "same"}}
	job := &blueprintImportJob{teamID: "t", report: report}

	svc.reconcileReimport(context.Background(), job,
		&external.GitHubFile{Path: "p", BlobSHA: "same"}, existing, &models.Blueprint{})

	assert.Equal(t, 1, report.TotalUpToDate)
	require.Len(t, report.UpToDateItems, 1)
	assert.Equal(t, "bp-1", report.UpToDateItems[0].BlueprintID)
	repo.AssertNotCalled(t, "UpdateOnReimport")
}

// TestReconcileReimport_Conflict verifies a VibeXP-edited blueprint is untouched.
func TestReconcileReimport_Conflict(t *testing.T) {
	svc, repo := newSanitizeTestService()
	report := &models.BlueprintImportReport{}
	existing := &models.Blueprint{
		ID: "bp-1", RawContent: "edited in vibexp",
		Source: &models.BlueprintSource{BlobSHA: gitBlobSHA("original raw")},
	}
	job := &blueprintImportJob{teamID: "t", report: report}

	svc.reconcileReimport(context.Background(), job,
		&external.GitHubFile{Path: "p", BlobSHA: "changed"}, existing, &models.Blueprint{})

	assert.Equal(t, 1, report.TotalConflicts)
	require.Len(t, report.ConflictItems, 1)
	assert.Equal(t, "bp-1", report.ConflictItems[0].BlueprintID)
	assert.NotEmpty(t, report.ConflictItems[0].Reason)
	repo.AssertNotCalled(t, "UpdateOnReimport")
}

// TestFindExistingForReimport_PathThenSlug verifies path match wins and slug is
// the fallback.
func TestFindExistingForReimport_PathThenSlug(t *testing.T) {
	t.Run("path match wins (slug not consulted)", func(t *testing.T) {
		svc, repo := newSanitizeTestService()
		byPath := &models.Blueprint{ID: "by-path"}
		repo.On("GetByProjectIDAndPath", mock.Anything, "u", "t", "p", "path.md").Return(byPath, nil)
		got := svc.findExistingForReimport(context.Background(), "u", "t", "p", "path.md", "slug")
		require.NotNil(t, got)
		assert.Equal(t, "by-path", got.ID)
		repo.AssertNotCalled(t, "GetByProjectIDAndSlug")
	})

	t.Run("slug fallback when no path match", func(t *testing.T) {
		svc, repo := newSanitizeTestService()
		bySlug := &models.Blueprint{ID: "by-slug"}
		repo.On("GetByProjectIDAndPath", mock.Anything, "u", "t", "p", "path.md").
			Return(nil, errors.New("not found"))
		repo.On("GetByProjectIDAndSlug", mock.Anything, "u", "t", "p", "slug").Return(bySlug, nil)
		got := svc.findExistingForReimport(context.Background(), "u", "t", "p", "path.md", "slug")
		require.NotNil(t, got)
		assert.Equal(t, "by-slug", got.ID)
	})

	t.Run("nil when neither matches", func(t *testing.T) {
		svc, repo := newSanitizeTestService()
		repo.On("GetByProjectIDAndPath", mock.Anything, "u", "t", "p", "path.md").
			Return(nil, errors.New("not found"))
		repo.On("GetByProjectIDAndSlug", mock.Anything, "u", "t", "p", "slug").
			Return(nil, errors.New("not found"))
		assert.Nil(t, svc.findExistingForReimport(context.Background(), "u", "t", "p", "path.md", "slug"))
	})
}
