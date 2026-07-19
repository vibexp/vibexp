package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

// TestBlueprintSlugConflictError_NamesTheScope pins the #282 fix: a slug
// collision must name the scope that actually collided. Blueprints carry both a
// team-wide (slug, team_id) key and a stricter (project_id, slug) key, so "for
// this project" was wrong whenever the team-wide key fired for a slug that was
// free in the target project.
func TestBlueprintSlugConflictError_NamesTheScope(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		want       string
	}{
		{
			name:       "team-wide key names the team",
			constraint: "blueprints_slug_team_id_key",
			want:       "blueprint with slug 'dup' already exists in this team",
		},
		{
			name:       "per-project key names the project",
			constraint: "blueprints_project_id_slug_unique",
			want:       "blueprint with slug 'dup' already exists in this project",
		},
		{
			name:       "unknown constraint falls back to a scope-neutral message",
			constraint: "some_other_key",
			want:       "blueprint with slug 'dup' already exists",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := blueprintSlugConflictError(&pq.Error{Constraint: tc.constraint}, "dup")
			require.Error(t, err)
			assert.EqualError(t, err, tc.want)
		})
	}
}

// TestBlueprintRepository_Create_SlugConflict_NamesScope proves the create call
// site is wired to the scope-aware mapping — a team-wide unique violation from
// the DB surfaces as "in this team", not the old "for this project".
func TestBlueprintRepository_Create_SlugConflict_NamesScope(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewBlueprintRepository(&database.DB{DB: db})

	mock.ExpectQuery("INSERT INTO blueprints").
		WillReturnError(&pq.Error{Code: "23505", Constraint: "blueprints_slug_team_id_key"})

	err = repo.Create(context.Background(), &models.Blueprint{
		ProjectID: "proj-1", Slug: "dup", UserID: "user-1", TeamID: "team-1",
		Title: "T", Content: "C", Status: "active", Type: "general",
	})

	assert.EqualError(t, err, "blueprint with slug 'dup' already exists in this team")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBlueprintRepository_Update_SlugConflict_NamesScope proves the update call
// site is wired to the same scope-aware mapping — here the per-project key fires.
func TestBlueprintRepository_Update_SlugConflict_NamesScope(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewBlueprintRepository(&database.DB{DB: db})

	// Update first validates the blueprint belongs to the team, then runs the UPDATE.
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("UPDATE blueprints").
		WillReturnError(&pq.Error{Code: "23505", Constraint: "blueprints_project_id_slug_unique"})

	err = repo.Update(context.Background(), &models.Blueprint{
		ID: "bp-1", ProjectID: "proj-1", Slug: "dup", UserID: "user-1", TeamID: "team-1",
		Title: "T", Content: "C", Status: "active", Type: "general", Version: 1,
	})

	assert.EqualError(t, err, "blueprint with slug 'dup' already exists in this project")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestNullableString covers both branches of the NULL-mapping helper.
func TestNullableString(t *testing.T) {
	assert.Nil(t, nullableString(""))
	assert.Equal(t, "x", nullableString("x"))
}

// TestBlueprintUniqueConflictError_PathScope covers the (project_id, path)
// conflict branch (#339): a path unique violation names the path scope; anything
// else falls through to the slug-scoped messages.
func TestBlueprintUniqueConflictError_PathScope(t *testing.T) {
	bp := &models.Blueprint{Slug: "dup", Path: ".claude/x.md"}
	pathErr := blueprintUniqueConflictError(&pq.Error{Constraint: "blueprints_project_id_path_unique"}, bp)
	assert.EqualError(t, pathErr, "blueprint with path '.claude/x.md' already exists in this project")

	slugErr := blueprintUniqueConflictError(&pq.Error{Constraint: "blueprints_slug_team_id_key"}, bp)
	assert.EqualError(t, slugErr, "blueprint with slug 'dup' already exists in this team")
}

// TestBlueprintRepository_Create_PathConflict_NamesScope proves the create call
// site surfaces the (project_id, path) violation with a path-scoped message.
func TestBlueprintRepository_Create_PathConflict_NamesScope(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("failed to close db: %v", closeErr)
		}
	}()

	repo := NewBlueprintRepository(&database.DB{DB: db})
	mock.ExpectQuery("INSERT INTO blueprints").
		WillReturnError(&pq.Error{Code: "23505", Constraint: "blueprints_project_id_path_unique"})

	err = repo.Create(context.Background(), &models.Blueprint{
		ProjectID: "proj-1", Slug: "s", UserID: "u", TeamID: "team-1",
		Title: "T", Content: "C", Status: "active", Type: "general", Path: "dup.md",
	})
	assert.EqualError(t, err, "blueprint with path 'dup.md' already exists in this project")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBlueprintRepository_Create_PersistsProvenance covers the provenance INSERT
// path: a blueprint carrying a Source writes non-NULL source_* / imported_at.
func TestBlueprintRepository_Create_PersistsProvenance(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("failed to close db: %v", closeErr)
		}
	}()

	repo := NewBlueprintRepository(&database.DB{DB: db})
	now := time.Now()
	// Args: 13 base cols, then path, path_derived, raw_content, content_sha,
	// source_repo, source_commit_sha, source_blob_sha, source_content_sha, imported_at.
	mock.ExpectQuery("INSERT INTO blueprints").
		WithArgs(
			"proj-1", "s", "u", "team-1", "T", "D", "raw body",
			"active", "general", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			"CLAUDE.md", false, "raw body", "sha-1",
			"https://github.com/o/r", "commit-1", "blob-1", nil, now,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow("bp-1", now, now))

	err = repo.Create(context.Background(), &models.Blueprint{
		ProjectID: "proj-1", Slug: "s", UserID: "u", TeamID: "team-1",
		Title: "T", Description: "D", Content: "raw body", Status: "active", Type: "general",
		Path: "CLAUDE.md", PathDerived: false, RawContent: "raw body", ContentSHA: "sha-1",
		Source: &models.BlueprintSource{
			Repo: "https://github.com/o/r", CommitSHA: "commit-1", BlobSHA: "blob-1", ImportedAt: &now,
		},
	})
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBlueprintRepository_GetByID_AssemblesSource covers the detail-read
// provenance assembly: non-NULL source_* / imported_at columns become a Source
// object; raw_content is returned.
func TestBlueprintRepository_GetByID_AssemblesSource(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("failed to close db: %v", closeErr)
		}
	}()

	repo := NewBlueprintRepository(&database.DB{DB: db})
	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "project_id", "slug", "user_id", "team_id", "title", "description",
		"content", "status", "type", "subtype", "metadata", "created_at", "updated_at", "version",
		"path", "path_derived", "raw_content", "content_sha",
		"source_repo", "source_commit_sha", "source_blob_sha", "source_content_sha", "imported_at",
	}).AddRow(
		"bp-1", "proj-1", "s", "u", "team-1", "T", "D",
		"body", "active", "general", nil, nil, now, now, 1,
		"CLAUDE.md", false, "raw bytes", "sha-1",
		"https://github.com/o/r", "commit-1", "blob-1", "import-sha", now,
	)
	mock.ExpectQuery("SELECT (.+) FROM blueprints s.*").
		WithArgs("bp-1", "team-1", "u").WillReturnRows(rows)

	bp, err := repo.GetByID(context.Background(), "u", "team-1", "bp-1")
	require.NoError(t, err)
	assert.Equal(t, "CLAUDE.md", bp.Path)
	assert.Equal(t, "raw bytes", bp.RawContent)
	assert.Equal(t, "sha-1", bp.ContentSHA)
	require.NotNil(t, bp.Source)
	assert.Equal(t, "https://github.com/o/r", bp.Source.Repo)
	assert.Equal(t, "commit-1", bp.Source.CommitSHA)
	assert.Equal(t, "blob-1", bp.Source.BlobSHA)
	require.NotNil(t, bp.Source.ImportedAt)
	assert.NoError(t, mock.ExpectationsWereMet())
}
