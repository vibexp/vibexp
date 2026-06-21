package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

func TestPromptGalleryRepository_GetCategories(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("Failed to close database: %v", err)
		}
	}()

	repo := NewPromptGalleryRepository(&database.DB{DB: db})
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"category", "count"}).
			AddRow("Engineering", 2).
			AddRow("Marketing", 1)

		mock.ExpectQuery("SELECT category, COUNT").WillReturnRows(rows)

		categories, err := repo.GetCategories(ctx)
		require.NoError(t, err)
		assert.Len(t, categories, 2)
		assert.Equal(t, "Engineering", categories[0].Category)
		assert.Equal(t, 2, categories[0].Count)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Database Error", func(t *testing.T) {
		mock.ExpectQuery("SELECT category, COUNT").WillReturnError(errors.New("db error"))

		categories, err := repo.GetCategories(ctx)
		assert.Error(t, err)
		assert.Nil(t, categories)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

//nolint:funlen // Test function with multiple subtests
func TestPromptGalleryRepository_List(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("Failed to close database: %v", err)
		}
	}()

	repo := NewPromptGalleryRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	t.Run("List with no filters", func(t *testing.T) {
		// Count query
		mock.ExpectQuery("SELECT COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

		// List query - tags is JSONB so use []byte for JSON array
		cols := []string{
			"id", "title", "description", "content", "category", "tags",
			"metadata", "created_at", "updated_at",
		}
		rows := sqlmock.NewRows(cols).
			AddRow(
				"123", "Test 1", "Desc 1", "Content 1", "Engineering",
				[]byte(`["tag1"]`), []byte(`{}`), now, now,
			).
			AddRow(
				"456", "Test 2", "Desc 2", "Content 2", "Marketing",
				[]byte(`["tag2"]`), []byte(`{}`), now, now,
			)

		mock.ExpectQuery("SELECT id, title, description, content, category").
			WillReturnRows(rows)

		prompts, total, err := repo.List(ctx, repositories.PromptGalleryFilters{
			Page:  1,
			Limit: 10,
		})

		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, prompts, 2)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("List with category filter", func(t *testing.T) {
		mock.ExpectQuery(`SELECT COUNT.*WHERE \(category`).
			WithArgs("Engineering").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		cols := []string{
			"id", "title", "description", "content", "category", "tags",
			"metadata", "created_at", "updated_at",
		}
		rows := sqlmock.NewRows(cols).
			AddRow(
				"123", "Test 1", "Desc 1", "Content 1", "Engineering",
				[]byte(`["tag1"]`), []byte(`{}`), now, now,
			)

		// squirrel inlines LIMIT/OFFSET as literals, so the only bound arg is the
		// category value.
		mock.ExpectQuery(`SELECT id, title, description.*WHERE \(category`).
			WithArgs("Engineering").
			WillReturnRows(rows)

		prompts, total, err := repo.List(ctx, repositories.PromptGalleryFilters{
			Category: "Engineering",
			Page:     1,
			Limit:    10,
		})

		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, prompts, 1)
		assert.Equal(t, "Engineering", prompts[0].Category)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("List with search filter", func(t *testing.T) {
		// squirrel binds the title and description ILIKE patterns as two separate
		// placeholders (the prior hand-built query reused a single placeholder).
		// The two patterns are identical, so the rows matched are unchanged.
		mock.ExpectQuery(`SELECT COUNT.*WHERE.*ILIKE`).
			WithArgs("%test%", "%test%").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		cols := []string{
			"id", "title", "description", "content", "category", "tags",
			"metadata", "created_at", "updated_at",
		}
		rows := sqlmock.NewRows(cols).
			AddRow(
				"123", "Test 1", "Desc 1", "Content 1", "Engineering",
				[]byte(`["tag1"]`), []byte(`{}`), now, now,
			)

		// squirrel inlines LIMIT/OFFSET as literals; both ILIKE patterns are bound.
		mock.ExpectQuery(`SELECT id, title, description.*WHERE.*ILIKE`).
			WithArgs("%test%", "%test%").
			WillReturnRows(rows)

		prompts, total, err := repo.List(ctx, repositories.PromptGalleryFilters{
			Search: "test",
			Page:   1,
			Limit:  10,
		})

		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, prompts, 1)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPromptGalleryRepository_GetByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("Failed to close database: %v", err)
		}
	}()

	repo := NewPromptGalleryRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	t.Run("Success", func(t *testing.T) {
		cols := []string{
			"id", "title", "description", "content", "category", "tags",
			"metadata", "created_at", "updated_at",
		}
		rows := sqlmock.NewRows(cols).
			AddRow(
				"123", "Test Prompt", "Test Desc", "Test Content", "Engineering",
				[]byte(`["tag1"]`), []byte(`{"key":"value"}`), now, now,
			)

		query := "SELECT id, title, description, content, category, " +
			"tags, metadata, created_at, updated_at FROM prompt_gallery_templates WHERE id"
		mock.ExpectQuery(query).
			WithArgs("123").
			WillReturnRows(rows)

		prompt, err := repo.GetByID(ctx, "123")
		require.NoError(t, err)
		assert.NotNil(t, prompt)
		assert.Equal(t, "123", prompt.ID)
		assert.Equal(t, "Test Prompt", prompt.Title)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Prompt Not Found", func(t *testing.T) {
		query := "SELECT id, title, description, content, category, " +
			"tags, metadata, created_at, updated_at FROM prompt_gallery_templates WHERE id"
		mock.ExpectQuery(query).
			WithArgs("999").
			WillReturnError(errors.New("sql: no rows in result set"))

		prompt, err := repo.GetByID(ctx, "999")
		assert.Error(t, err)
		assert.Nil(t, prompt)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
