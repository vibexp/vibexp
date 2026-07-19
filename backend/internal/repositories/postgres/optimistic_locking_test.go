package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

// TestArtifactRepository_OptimisticLocking tests that version conflicts are properly detected
func TestArtifactRepository_OptimisticLocking(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			t.Logf("failed to close db: %v", closeErr)
		}
	}()

	db := &database.DB{DB: sqlDB}
	repo := &ArtifactRepository{db: db}

	ctx := context.Background()
	artifact := &models.Artifact{
		ID:          "artifact-123",
		ProjectID:   "550e8400-e29b-41d4-a716-446655440000",
		Slug:        "test-slug",
		UserID:      "user-123",
		TeamID:      "team-123",
		Title:       "Updated Title",
		Description: "Updated Description",
		Content:     "Updated Content",
		Status:      "active",
		Type:        "general",
		Metadata:    map[string]interface{}{"key": "value"},
		UpdatedAt:   time.Now(),
		Version:     1, // Current version
	}

	// Existence-in-team check (tenancy only; role is decided in the service)
	mock.ExpectQuery("SELECT EXISTS\\(.*FROM artifacts a.*").
		WithArgs(artifact.ID, artifact.TeamID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Simulate version mismatch (no rows affected)
	// Args order: id, project_id, slug, title, description, content, status, type, metadata,
	// team_id, updated_at, team_id, version
	mock.ExpectQuery("UPDATE artifacts.*WHERE.*").
		WithArgs(
			artifact.ID, artifact.ProjectID, artifact.Slug,
			artifact.Title, artifact.Description, artifact.Content,
			artifact.Status, artifact.Type, sqlmock.AnyArg(),
			artifact.TeamID, sqlmock.AnyArg(), artifact.TeamID, artifact.Version,
		).
		WillReturnError(sql.ErrNoRows)

	err = repo.Update(ctx, artifact)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "version conflict")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMemoryRepository_OptimisticLocking tests optimistic locking for memories
func TestMemoryRepository_OptimisticLocking(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			t.Logf("failed to close db: %v", closeErr)
		}
	}()

	db := &database.DB{DB: sqlDB}
	repo := &MemoryRepository{db: db}

	ctx := context.Background()
	memory := &models.Memory{
		ID:        "memory-123",
		UserID:    "user-123",
		TeamID:    "team-123",
		Text:      "Updated memory text",
		Metadata:  map[string]interface{}{"key": "value"},
		UpdatedAt: time.Now(),
		Version:   1, // Current version
	}

	// Expect ownership validation query first with team membership check
	mock.ExpectQuery("SELECT EXISTS\\(.*FROM memories m.*").
		WithArgs(memory.ID, memory.TeamID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Simulate version mismatch (UPDATE fails because version doesn't match)
	// Args order: id, text, status, metadata, project_id, team_id, updated_at, team_id, version, user_id.
	// The memory carries no status, so the repository defaults it to "active".
	mock.ExpectQuery("UPDATE memories.*WHERE.*").
		WithArgs(
			memory.ID, memory.Text, "active", sqlmock.AnyArg(), memory.ProjectID,
			memory.TeamID, sqlmock.AnyArg(), memory.TeamID, memory.Version,
		).
		WillReturnError(sql.ErrNoRows)

	err = repo.Update(ctx, memory)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "version conflict")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBlueprintRepository_OptimisticLocking tests optimistic locking for spec library
func TestBlueprintRepository_OptimisticLocking(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			t.Logf("failed to close db: %v", closeErr)
		}
	}()

	db := &database.DB{DB: sqlDB}
	repo := &BlueprintRepository{db: db}

	ctx := context.Background()
	specLibrary := &models.Blueprint{
		ID:          "spec-123",
		ProjectID:   "550e8400-e29b-41d4-a716-446655440000",
		Slug:        "test-slug",
		UserID:      "user-123",
		TeamID:      "team-123",
		Title:       "Updated Title",
		Description: "Updated Description",
		Content:     "Updated Content",
		Status:      "active",
		Type:        "general",
		Metadata:    map[string]interface{}{"key": "value"},
		UpdatedAt:   time.Now(),
		Version:     1, // Current version
	}

	// Expect ownership validation query first with team membership check
	ownershipQuery := "SELECT EXISTS\\(.*FROM blueprints s.*"
	mock.ExpectQuery(ownershipQuery).
		WithArgs(specLibrary.ID, specLibrary.TeamID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Simulate version mismatch (UPDATE fails because version doesn't match)
	// Args order: id, project_id, slug, title, description, content, status, type, subtype,
	// metadata, team_id, updated_at, path, path_derived, raw_content, content_sha, team_id, version
	mock.ExpectQuery("UPDATE blueprints.*WHERE.*").
		WithArgs(
			specLibrary.ID, specLibrary.ProjectID, specLibrary.Slug,
			specLibrary.Title, specLibrary.Description, specLibrary.Content,
			specLibrary.Status, specLibrary.Type, specLibrary.Subtype, sqlmock.AnyArg(),
			specLibrary.TeamID, sqlmock.AnyArg(),
			specLibrary.Path, specLibrary.PathDerived, sqlmock.AnyArg(), sqlmock.AnyArg(),
			specLibrary.TeamID, specLibrary.Version,
		).
		WillReturnError(sql.ErrNoRows)

	err = repo.Update(ctx, specLibrary)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "version conflict")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestArtifactRepository_SuccessfulUpdate tests that successful updates increment version
func TestArtifactRepository_SuccessfulUpdate(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			t.Logf("failed to close db: %v", closeErr)
		}
	}()

	db := &database.DB{DB: sqlDB}
	repo := &ArtifactRepository{db: db}

	ctx := context.Background()
	now := time.Now()
	artifact := &models.Artifact{
		ID:          "artifact-123",
		ProjectID:   "550e8400-e29b-41d4-a716-446655440000",
		Slug:        "test-slug",
		UserID:      "user-123",
		TeamID:      "team-123",
		Title:       "Updated Title",
		Description: "Updated Description",
		Content:     "Updated Content",
		Status:      "active",
		Type:        "general",
		Metadata:    map[string]interface{}{"key": "value"},
		UpdatedAt:   now,
		Version:     1, // Current version
	}

	// Existence-in-team check (tenancy only; role is decided in the service)
	mock.ExpectQuery("SELECT EXISTS\\(.*FROM artifacts a.*").
		WithArgs(artifact.ID, artifact.TeamID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Simulate successful update returning new version
	rows := sqlmock.NewRows([]string{"updated_at", "version"}).
		AddRow(now, 2) // Version incremented to 2

	// Args order: id, project_id, slug, title, description, content, status, type, metadata,
	// team_id, updated_at, team_id, version
	mock.ExpectQuery("UPDATE artifacts.*WHERE.*").
		WithArgs(
			artifact.ID, artifact.ProjectID, artifact.Slug,
			artifact.Title, artifact.Description, artifact.Content,
			artifact.Status, artifact.Type, sqlmock.AnyArg(),
			artifact.TeamID, sqlmock.AnyArg(), artifact.TeamID, artifact.Version,
		).
		WillReturnRows(rows)

	err = repo.Update(ctx, artifact)

	assert.NoError(t, err)
	assert.Equal(t, int64(2), artifact.Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}
