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
)

func TestPromptReferenceRepository_CreateBatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewPromptReferenceRepository(&database.DB{DB: db})
	ctx := context.Background()

	t.Run("successfully creates multiple references", func(t *testing.T) {
		references := []models.PromptReference{
			{
				PromptID:           "prompt-1",
				ReferencedPromptID: "ref-1",
				CreatedAt:          time.Now(),
			},
			{
				PromptID:           "prompt-1",
				ReferencedPromptID: "ref-2",
				CreatedAt:          time.Now(),
			},
		}

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO prompt_references").
			WithArgs("prompt-1", "ref-1").
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec("INSERT INTO prompt_references").
			WithArgs("prompt-1", "ref-2").
			WillReturnResult(sqlmock.NewResult(2, 1))
		mock.ExpectCommit()

		err := repo.CreateBatch(ctx, references)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles empty references slice", func(t *testing.T) {
		err := repo.CreateBatch(ctx, []models.PromptReference{})
		assert.NoError(t, err)
	})
}

func TestPromptReferenceRepository_DeleteByPromptID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewPromptReferenceRepository(&database.DB{DB: db})
	ctx := context.Background()

	t.Run("successfully deletes references", func(t *testing.T) {
		promptID := "test-prompt-id"

		mock.ExpectExec("DELETE FROM prompt_references WHERE prompt_id").
			WithArgs(promptID).
			WillReturnResult(sqlmock.NewResult(0, 2))

		err := repo.DeleteByPromptID(ctx, promptID)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPromptReferenceRepository_GetPromptsUsingPrompt(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewPromptReferenceRepository(&database.DB{DB: db})
	ctx := context.Background()

	t.Run("returns prompts that reference the given prompt", func(t *testing.T) {
		userID := "user-123"
		promptID := "prompt-456"

		rows := sqlmock.NewRows([]string{"id", "slug", "name"}).
			AddRow("dep-1", "dependent-slug-1", "Dependent Prompt 1").
			AddRow("dep-2", "dependent-slug-2", "Dependent Prompt 2")

		mock.ExpectQuery("SELECT p.id, p.slug, p.name FROM prompt_references pr").
			WithArgs(promptID, userID).
			WillReturnRows(rows)

		results, err := repo.GetPromptsUsingPrompt(ctx, userID, promptID)
		assert.NoError(t, err)
		assert.Len(t, results, 2)
		assert.Equal(t, "dependent-slug-1", results[0].Slug)
		assert.Equal(t, "Dependent Prompt 1", results[0].Name)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns empty array when no prompts reference it", func(t *testing.T) {
		userID := "user-123"
		promptID := "prompt-456"

		rows := sqlmock.NewRows([]string{"id", "slug", "name"})

		mock.ExpectQuery("SELECT p.id, p.slug, p.name FROM prompt_references pr").
			WithArgs(promptID, userID).
			WillReturnRows(rows)

		results, err := repo.GetPromptsUsingPrompt(ctx, userID, promptID)
		assert.NoError(t, err)
		assert.NotNil(t, results)
		assert.Len(t, results, 0) // Empty array, not nil
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPromptReferenceRepository_GetPromptsUsedByPrompt(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewPromptReferenceRepository(&database.DB{DB: db})
	ctx := context.Background()

	t.Run("returns prompts that are referenced by the given prompt", func(t *testing.T) {
		userID := "user-123"
		promptID := "prompt-456"

		rows := sqlmock.NewRows([]string{"id", "slug", "name"}).
			AddRow("ref-1", "referenced-slug-1", "Referenced Prompt 1").
			AddRow("ref-2", "referenced-slug-2", "Referenced Prompt 2")

		mock.ExpectQuery("SELECT p.id, p.slug, p.name FROM prompt_references pr").
			WithArgs(promptID, userID).
			WillReturnRows(rows)

		results, err := repo.GetPromptsUsedByPrompt(ctx, userID, promptID)
		assert.NoError(t, err)
		assert.Len(t, results, 2)
		assert.Equal(t, "referenced-slug-1", results[0].Slug)
		assert.Equal(t, "Referenced Prompt 1", results[0].Name)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns empty array when prompt references nothing", func(t *testing.T) {
		userID := "user-123"
		promptID := "prompt-456"

		rows := sqlmock.NewRows([]string{"id", "slug", "name"})

		mock.ExpectQuery("SELECT p.id, p.slug, p.name FROM prompt_references pr").
			WithArgs(promptID, userID).
			WillReturnRows(rows)

		results, err := repo.GetPromptsUsedByPrompt(ctx, userID, promptID)
		assert.NoError(t, err)
		assert.NotNil(t, results)
		assert.Len(t, results, 0) // Empty array, not nil
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPromptReferenceRepository_HasDependents(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewPromptReferenceRepository(&database.DB{DB: db})
	ctx := context.Background()

	t.Run("returns true when prompt has dependents", func(t *testing.T) {
		promptID := "prompt-123"

		rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)

		mock.ExpectQuery("SELECT EXISTS").
			WithArgs(promptID).
			WillReturnRows(rows)

		hasDepend, err := repo.HasDependents(ctx, promptID)
		assert.NoError(t, err)
		assert.True(t, hasDepend)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns false when prompt has no dependents", func(t *testing.T) {
		promptID := "prompt-123"

		rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)

		mock.ExpectQuery("SELECT EXISTS").
			WithArgs(promptID).
			WillReturnRows(rows)

		hasDepend, err := repo.HasDependents(ctx, promptID)
		assert.NoError(t, err)
		assert.False(t, hasDepend)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
