package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
)

const testModelID = "gemini-embedding-001"

// backfillColumns mirrors the 13 columns every backfill query projects, in order.
var backfillColumns = []string{
	"entity_id", "user_id", "team_id", "project_name", "feed_id", "slug",
	"title", "description", "body", "type", "email", "excerpt", "created_at",
}

func TestEmbeddingBackfillRepository_ListEntities_Prompt(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	repo := NewEmbeddingBackfillRepository(&database.DB{DB: mockDB})
	now := time.Now()

	rows := sqlmock.NewRows(backfillColumns).AddRow(
		"p1", "user-1", "team-1", "proj-1", "", "my-prompt",
		"Prompt Name", "prompt summary", "prompt body", "", "user@example.com", "", now,
	)
	mock.ExpectQuery("FROM prompts").
		WithArgs(500, 0).
		WillReturnRows(rows)

	got, err := repo.ListEntities(context.Background(), "prompt", testModelID, "", false, 500, 0)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "prompt", got[0].EntityType)
	assert.Equal(t, "p1", got[0].EntityID)
	assert.Equal(t, "user@example.com", got[0].Email)
	assert.Equal(t, "Prompt Name", got[0].Title)
	assert.Equal(t, "prompt summary", got[0].Description)
	assert.Equal(t, "prompt body", got[0].Body)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEmbeddingBackfillRepository_ListEntities_FeedItem(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	repo := NewEmbeddingBackfillRepository(&database.DB{DB: mockDB})
	now := time.Now()

	rows := sqlmock.NewRows(backfillColumns).AddRow(
		"f1", "poster-1", "team-9", "", "feed-1", "",
		"Item Title", "", "item content", "", "", "short excerpt", now,
	)
	mock.ExpectQuery("FROM feed_items").
		WithArgs(500, 0).
		WillReturnRows(rows)

	got, err := repo.ListEntities(context.Background(), "feed_item", testModelID, "", false, 500, 0)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "feed-1", got[0].FeedID)
	assert.Equal(t, "short excerpt", got[0].Excerpt)
	assert.Equal(t, "poster-1", got[0].UserID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEmbeddingBackfillRepository_ListEntities_MissingOnly_FiltersAndBindsModel(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	repo := NewEmbeddingBackfillRepository(&database.DB{DB: mockDB})
	now := time.Now()

	rows := sqlmock.NewRows(backfillColumns).AddRow(
		"m1", "user-1", "team-1", "proj-1", "", "",
		"", "", "memory text", "", "", "", now,
	)
	// The missing-only query must add the NOT EXISTS embeddings filter keyed by the
	// configured model id (bound as the third arg).
	mock.ExpectQuery(regexp.QuoteMeta("NOT EXISTS")).
		WithArgs(500, 0, testModelID).
		WillReturnRows(rows)

	got, err := repo.ListEntities(context.Background(), "memory", testModelID, "", true, 500, 0)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "m1", got[0].EntityID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEmbeddingBackfillRepository_ListEntities_UnsupportedType(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	repo := NewEmbeddingBackfillRepository(&database.DB{DB: mockDB})

	_, err = repo.ListEntities(context.Background(), "widget", testModelID, "", false, 500, 0)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported backfill entity type")
}

func TestEmbeddingBackfillRepository_ListEntities_FeedItemReply_NoLongerSupported(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	repo := NewEmbeddingBackfillRepository(&database.DB{DB: mockDB})

	_, err = repo.ListEntities(context.Background(), "feed_item_reply", testModelID, "", false, 500, 0)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported backfill entity type")
}
