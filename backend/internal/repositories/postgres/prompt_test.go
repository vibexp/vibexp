package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

func TestPromptRepository_Create_WithMCPExposeTrue(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	prompt := &models.Prompt{
		Name:        "Test Prompt",
		Slug:        "test-prompt",
		Description: "Test description",
		Body:        "Test body",
		UserID:      "user-123",
		TeamID:      "team-123",
		ProjectID:   "project-123",
		Status:      "published",
		MCPExpose:   true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	mock.ExpectQuery("INSERT INTO prompts").
		WithArgs(
			prompt.Name, prompt.Slug, prompt.Description, prompt.Body,
			prompt.UserID, prompt.TeamID, prompt.ProjectID, prompt.Status, prompt.MCPExpose, sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow("prompt-123", now, now))

	err = repo.Create(ctx, prompt)
	assert.NoError(t, err)
	assert.Equal(t, "prompt-123", prompt.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptRepository_Create_WithMCPExposeFalse(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	prompt := &models.Prompt{
		Name:        "Private Prompt",
		Slug:        "private-prompt",
		Description: "Private description",
		Body:        "Private body",
		UserID:      "user-123",
		TeamID:      "team-123",
		ProjectID:   "project-123",
		Status:      "published",
		MCPExpose:   false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	mock.ExpectQuery("INSERT INTO prompts").
		WithArgs(
			prompt.Name, prompt.Slug, prompt.Description, prompt.Body,
			prompt.UserID, prompt.TeamID, prompt.ProjectID, prompt.Status, prompt.MCPExpose, sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow("prompt-456", now, now))

	err = repo.Create(ctx, prompt)
	assert.NoError(t, err)
	assert.Equal(t, "prompt-456", prompt.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptRepository_GetByID_WithMCPExposeTrue(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "name", "slug", "description", "body", "user_id", "team_id", "project_id", "status", "mcp_expose",
		"labels", "created_at", "updated_at", "version", "is_shared",
	}).AddRow(
		"prompt-123", "Test Prompt", "test-prompt", "Test description",
		"Test body", "user-123", "team-123", "project-123", "published", true, "{}", now, now, 1, false,
	)

	mock.ExpectQuery("SELECT (.+) FROM prompts p(.+)EXISTS.*teams").
		WithArgs("prompt-123", "team-123", "user-123").
		WillReturnRows(rows)

	prompt, err := repo.GetByID(ctx, "user-123", "team-123", "prompt-123")
	assert.NoError(t, err)
	assert.NotNil(t, prompt)
	assert.Equal(t, "prompt-123", prompt.ID)
	assert.True(t, prompt.MCPExpose)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptRepository_GetByID_WithMCPExposeFalse(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "name", "slug", "description", "body", "user_id", "team_id", "project_id", "status", "mcp_expose",
		"labels", "created_at", "updated_at", "version", "is_shared",
	}).AddRow(
		"prompt-456", "Private Prompt", "private-prompt", "Private description",
		"Private body", "user-123", "team-123", "project-123", "published", false, "{}", now, now, 1, false,
	)

	mock.ExpectQuery("SELECT (.+) FROM prompts p(.+)EXISTS.*teams").
		WithArgs("prompt-456", "team-123", "user-123").
		WillReturnRows(rows)

	prompt, err := repo.GetByID(ctx, "user-123", "team-123", "prompt-456")
	assert.NoError(t, err)
	assert.NotNil(t, prompt)
	assert.Equal(t, "prompt-456", prompt.ID)
	assert.False(t, prompt.MCPExpose)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptRepository_Update_MCPExposeToFalse(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	prompt := &models.Prompt{
		ID:          "prompt-123",
		Name:        "Test Prompt",
		Slug:        "test-prompt",
		Description: "Test description",
		Body:        "Test body",
		UserID:      "user-123",
		TeamID:      "team-123",
		ProjectID:   "project-123",
		Status:      "published",
		MCPExpose:   false,
		UpdatedAt:   now,
		Version:     1,
	}

	// Existence-in-team check (tenancy only; role is decided in PromptService)
	mock.ExpectQuery("SELECT EXISTS\\(.*FROM prompts p.*").
		WithArgs(prompt.ID, prompt.TeamID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Args order: id, name, slug, description, body, project_id, status, mcp_expose, labels,
	// team_id, updated_at, team_id, version
	mock.ExpectQuery("UPDATE prompts.*WHERE.*").
		WithArgs(
			prompt.ID, prompt.Name, prompt.Slug, prompt.Description,
			prompt.Body, prompt.ProjectID, prompt.Status, prompt.MCPExpose, sqlmock.AnyArg(),
			prompt.TeamID, sqlmock.AnyArg(), prompt.TeamID, prompt.Version,
		).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at", "version"}).AddRow(now, 2))

	err = repo.Update(ctx, prompt)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptRepository_Update_MCPExposeToTrue(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	prompt := &models.Prompt{
		ID:          "prompt-456",
		Name:        "Private Prompt",
		Slug:        "private-prompt",
		Description: "Private description",
		Body:        "Private body",
		UserID:      "user-123",
		TeamID:      "team-123",
		ProjectID:   "project-123",
		Status:      "published",
		MCPExpose:   true,
		UpdatedAt:   now,
		Version:     1,
	}

	// Existence-in-team check (tenancy only; role is decided in PromptService)
	mock.ExpectQuery("SELECT EXISTS\\(.*FROM prompts p.*").
		WithArgs(prompt.ID, prompt.TeamID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Args order: id, name, slug, description, body, project_id, status, mcp_expose, labels,
	// team_id, updated_at, team_id, version
	mock.ExpectQuery("UPDATE prompts.*WHERE.*").
		WithArgs(
			prompt.ID, prompt.Name, prompt.Slug, prompt.Description,
			prompt.Body, prompt.ProjectID, prompt.Status, prompt.MCPExpose, sqlmock.AnyArg(),
			prompt.TeamID, sqlmock.AnyArg(), prompt.TeamID, prompt.Version,
		).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at", "version"}).AddRow(now, 2))

	err = repo.Update(ctx, prompt)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptRepository_List_FilterByMCPExposeTrue(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	mcpExposeTrue := true
	filters := repositories.PromptFilters{
		MCPExpose: &mcpExposeTrue,
		Page:      1,
		Limit:     10,
	}

	// squirrel binds each EXISTS placeholder individually, so the team/user
	// pair appears as (team, team, user, team, user) and LIMIT/OFFSET are
	// inlined literals (not bound args).
	mock.ExpectQuery("SELECT COUNT.*FROM prompts p.*EXISTS.*teams").
		WithArgs("team-123", "team-123", "user-123", "team-123", "user-123", true).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	rows := sqlmock.NewRows([]string{
		"id", "name", "slug", "description", "body", "user_id", "team_id", "project_id", "status", "mcp_expose",
		"labels", "created_at", "updated_at", "is_shared",
	}).
		AddRow(
			"prompt-1", "Prompt 1", "prompt-1", "Desc 1", "Body 1", "user-123", "team-123",
			"project-123", "published", true, "{}", now, now, false,
		).
		AddRow(
			"prompt-2", "Prompt 2", "prompt-2", "Desc 2", "Body 2", "user-123", "team-123",
			"project-123", "published", true, "{}", now, now, false,
		)

	mock.ExpectQuery("SELECT (.+) FROM prompts p.*EXISTS.*teams").
		WithArgs("team-123", "team-123", "user-123", "team-123", "user-123", true).
		WillReturnRows(rows)

	filters.TeamID = "team-123"
	prompts, total, err := repo.List(ctx, "user-123", filters)
	assert.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, prompts, 2)
	assert.True(t, prompts[0].MCPExpose)
	assert.True(t, prompts[1].MCPExpose)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptRepository_List_FilterByMCPExposeFalse(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	mcpExposeFalse := false
	filters := repositories.PromptFilters{
		MCPExpose: &mcpExposeFalse,
		Page:      1,
		Limit:     10,
	}

	mock.ExpectQuery("SELECT COUNT.*FROM prompts p.*EXISTS.*teams").
		WithArgs("team-123", "team-123", "user-123", "team-123", "user-123", false).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	rows := sqlmock.NewRows([]string{
		"id", "name", "slug", "description", "body", "user_id", "team_id", "project_id", "status", "mcp_expose",
		"labels", "created_at", "updated_at", "is_shared",
	}).
		AddRow(
			"prompt-3", "Prompt 3", "prompt-3", "Desc 3", "Body 3", "user-123", "team-123",
			"project-123", "published", false, "{}", now, now, false,
		)

	mock.ExpectQuery("SELECT (.+) FROM prompts p.*EXISTS.*teams").
		WithArgs("team-123", "team-123", "user-123", "team-123", "user-123", false).
		WillReturnRows(rows)

	filters.TeamID = "team-123"
	prompts, total, err := repo.List(ctx, "user-123", filters)
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, prompts, 1)
	assert.False(t, prompts[0].MCPExpose)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptRepository_List_WithoutMCPExposeFilter(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	filters := repositories.PromptFilters{
		Page:  1,
		Limit: 10,
	}

	mock.ExpectQuery("SELECT COUNT.*FROM prompts p.*EXISTS.*teams").
		WithArgs("team-123", "team-123", "user-123", "team-123", "user-123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	rows := sqlmock.NewRows([]string{
		"id", "name", "slug", "description", "body", "user_id", "team_id", "project_id", "status", "mcp_expose",
		"labels", "created_at", "updated_at", "is_shared",
	}).
		AddRow(
			"prompt-1", "Prompt 1", "prompt-1", "Desc 1", "Body 1", "user-123", "team-123",
			"project-123", "published", true, "{}", now, now, false,
		).
		AddRow(
			"prompt-2", "Prompt 2", "prompt-2", "Desc 2", "Body 2", "user-123", "team-123",
			"project-123", "published", true, "{}", now, now, false,
		).
		AddRow(
			"prompt-3", "Prompt 3", "prompt-3", "Desc 3", "Body 3", "user-123", "team-123",
			"project-123", "published", false, "{}", now, now, false,
		)

	mock.ExpectQuery("SELECT (.+) FROM prompts p.*EXISTS.*teams").
		WithArgs("team-123", "team-123", "user-123", "team-123", "user-123").
		WillReturnRows(rows)

	filters.TeamID = "team-123"
	prompts, total, err := repo.List(ctx, "user-123", filters)
	assert.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, prompts, 3)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptRepository_GetBySlug(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := NewPromptRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	t.Run("Get prompt by slug includes mcp_expose", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"id", "name", "slug", "description", "body", "user_id", "team_id", "project_id", "status", "mcp_expose",
			"labels", "created_at", "updated_at", "version", "is_shared",
		}).AddRow(
			"prompt-123", "Test Prompt", "test-prompt", "Test description",
			"Test body", "user-123", "team-123", "project-123", "published", true, "{}", now, now, int64(1), false,
		)

		mock.ExpectQuery("SELECT (.+) FROM prompts p.*EXISTS.*teams").
			WithArgs("test-prompt", "team-123", "user-123").
			WillReturnRows(rows)

		prompt, err := repo.GetBySlug(ctx, "user-123", "team-123", "test-prompt")
		assert.NoError(t, err)
		assert.NotNil(t, prompt)
		assert.Equal(t, "test-prompt", prompt.Slug)
		assert.True(t, prompt.MCPExpose)
		assert.Equal(t, int64(1), prompt.Version)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
