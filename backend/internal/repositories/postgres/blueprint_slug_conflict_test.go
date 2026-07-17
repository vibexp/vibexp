package postgres

import (
	"context"
	"testing"

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
