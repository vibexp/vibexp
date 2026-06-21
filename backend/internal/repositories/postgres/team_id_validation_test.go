package postgres

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

// TestPromptRepository_List_EmptyTeamID tests that List returns an error when TeamID is empty
func TestPromptRepository_List_EmptyTeamID(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()

	filters := repositories.PromptFilters{
		TeamID: "", // Empty TeamID
		Page:   1,
		Limit:  10,
	}

	prompts, count, err := repo.List(ctx, "user-123", filters)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TeamID is required but was empty")
	assert.Nil(t, prompts)
	assert.Equal(t, 0, count)
}

// TestArtifactRepository_List_EmptyTeamID tests that List returns an error when TeamID is empty
func TestArtifactRepository_List_EmptyTeamID(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewArtifactRepository(&database.DB{DB: db})
	ctx := context.Background()

	filters := repositories.ArtifactFilters{
		TeamID: "", // Empty TeamID
		Page:   1,
		Limit:  10,
	}

	artifacts, count, err := repo.List(ctx, "user-123", filters)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TeamID is required but was empty")
	assert.Nil(t, artifacts)
	assert.Equal(t, 0, count)
}

// TestMemoryRepository_List_EmptyTeamID tests that List returns an error when TeamID is empty
func TestMemoryRepository_List_EmptyTeamID(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewMemoryRepository(&database.DB{DB: db})
	ctx := context.Background()

	filters := repositories.MemoryFilters{
		TeamID: "", // Empty TeamID
		Page:   1,
		Limit:  10,
	}

	memories, count, err := repo.List(ctx, "user-123", filters)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TeamID is required but was empty")
	assert.Nil(t, memories)
	assert.Equal(t, 0, count)
}

// TestAgentRepository_List_EmptyTeamID tests that List returns an error when TeamID is empty
func TestAgentRepository_List_EmptyTeamID(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewAgentRepository(&database.DB{DB: db})
	ctx := context.Background()

	filters := repositories.AgentFilters{
		TeamID: "", // Empty TeamID
		Page:   1,
		Limit:  10,
	}

	agents, count, err := repo.List(ctx, "user-123", filters)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TeamID is required but was empty")
	assert.Nil(t, agents)
	assert.Equal(t, 0, count)
}

// TestBlueprintRepository_List_EmptyTeamID tests that List returns an error when TeamID is empty
func TestBlueprintRepository_List_EmptyTeamID(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewBlueprintRepository(&database.DB{DB: db})
	ctx := context.Background()

	filters := repositories.BlueprintFilters{
		TeamID: "", // Empty TeamID
		Page:   1,
		Limit:  10,
	}

	specLibraries, count, err := repo.List(ctx, "user-123", filters)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TeamID is required but was empty")
	assert.Nil(t, specLibraries)
	assert.Equal(t, 0, count)
}

// TestProjectRepository_List_EmptyTeamID tests that List returns an error when TeamID is empty
func TestProjectRepository_List_EmptyTeamID(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewProjectRepository(&database.DB{DB: db})
	ctx := context.Background()

	filters := repositories.ProjectListFilters{
		TeamID: "", // Empty TeamID
		Page:   1,
		Limit:  10,
	}

	projects, count, err := repo.List(ctx, "user-123", filters)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TeamID is required but was empty")
	assert.Nil(t, projects)
	assert.Equal(t, 0, count)
}
