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
)

// These tests cover the poster-scoped lookups and reply-poster enumeration added for
// feed embedding wiring (#1361). They assert the SQL filters on posted_by_user_id and
// that sql.ErrNoRows maps to the entity-specific "not found" message.

func setupFeedItemRepoTest(t *testing.T) (*FeedItemRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	repo := NewFeedItemRepository(&database.DB{DB: mockDB}).(*FeedItemRepository)
	return repo, mock, mockDB
}

func setupFeedReplyRepoTest(t *testing.T) (*FeedItemReplyRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	repo := NewFeedItemReplyRepository(&database.DB{DB: mockDB}).(*FeedItemReplyRepository)
	return repo, mock, mockDB
}

func TestFeedItemRepository_GetByIDForPoster(t *testing.T) {
	repo, mock, mockDB := setupFeedItemRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	t.Run("returns item for poster", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"id", "team_id", "feed_id", "project_id", "title", "content",
			"excerpt", "ai_assistant_name", "posted_by_user_id", "archived_at", "posted_at",
		}).AddRow("item-1", "team-1", "feed-1", nil, "Title", "Body", "Excerpt", "claude", "poster-1", nil, now)

		mock.ExpectQuery("SELECT .* FROM feed_items fi\\s+WHERE fi.id = \\$1 AND fi.posted_by_user_id = \\$2").
			WithArgs("item-1", "poster-1").
			WillReturnRows(rows)

		item, err := repo.GetByIDForPoster(ctx, "poster-1", "item-1")
		require.NoError(t, err)
		assert.Equal(t, "item-1", item.ID)
		assert.Equal(t, "poster-1", item.PostedByUserID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found maps to feed item not found", func(t *testing.T) {
		mock.ExpectQuery("SELECT .* FROM feed_items fi\\s+WHERE fi.id = \\$1 AND fi.posted_by_user_id = \\$2").
			WithArgs("item-x", "forged").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByIDForPoster(ctx, "forged", "item-x")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "feed item not found")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestFeedItemReplyRepository_GetReplyForPoster(t *testing.T) {
	repo, mock, mockDB := setupFeedReplyRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()

	t.Run("returns reply for poster", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"id", "team_id", "feed_item_id", "content", "posted_by_user_id", "ai_assistant_name", "posted_at",
		}).AddRow("reply-1", "team-1", "item-1", "Body", "poster-1", nil, now)

		mock.ExpectQuery("SELECT .* FROM feed_item_replies\\s+WHERE id = \\$1 AND posted_by_user_id = \\$2").
			WithArgs("reply-1", "poster-1").
			WillReturnRows(rows)

		reply, err := repo.GetReplyForPoster(ctx, "poster-1", "reply-1")
		require.NoError(t, err)
		assert.Equal(t, "reply-1", reply.ID)
		assert.Equal(t, "poster-1", reply.PostedByUserID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found maps to feed item reply not found", func(t *testing.T) {
		mock.ExpectQuery("SELECT .* FROM feed_item_replies\\s+WHERE id = \\$1 AND posted_by_user_id = \\$2").
			WithArgs("reply-x", "forged").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetReplyForPoster(ctx, "forged", "reply-x")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "feed item reply not found")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestFeedItemReplyRepository_ListReplyPostersByItemID(t *testing.T) {
	repo, mock, mockDB := setupFeedReplyRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"id", "posted_by_user_id"}).
		AddRow("reply-1", "poster-a").
		AddRow("reply-2", "poster-b")

	const wantSQL = "SELECT id, posted_by_user_id\\s+FROM feed_item_replies\\s+" +
		"WHERE team_id = \\$1 AND feed_item_id = \\$2"
	mock.ExpectQuery(wantSQL).
		WithArgs("team-1", "item-1").
		WillReturnRows(rows)

	posters, err := repo.ListReplyPostersByItemID(ctx, "team-1", "item-1")
	require.NoError(t, err)
	require.Len(t, posters, 2)
	assert.Equal(t, "reply-1", posters[0].ReplyID)
	assert.Equal(t, "poster-a", posters[0].PostedByUserID)
	assert.Equal(t, "reply-2", posters[1].ReplyID)
	assert.Equal(t, "poster-b", posters[1].PostedByUserID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedItemReplyRepository_ListReplyPostersByItemID_QueryError(t *testing.T) {
	repo, mock, mockDB := setupFeedReplyRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery("SELECT id, posted_by_user_id\\s+FROM feed_item_replies").
		WithArgs("team-1", "item-1").
		WillReturnError(sql.ErrConnDone)

	_, err := repo.ListReplyPostersByItemID(context.Background(), "team-1", "item-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list reply posters by item ID")
	assert.NoError(t, mock.ExpectationsWereMet())
}
