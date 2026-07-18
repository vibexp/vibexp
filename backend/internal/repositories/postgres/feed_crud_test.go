package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// CRUD tests for FeedRepository and FeedItemRepository (issue #365). The
// squirrel List shapes are covered in feed_list_squirrel_test.go and the
// FeedItemRepository.Delete contract in feed_delete_sentinel_test.go; this
// file covers the remaining write/read paths.

// feedCrudTestTime is the fixed timestamp used by the feed CRUD tests.
var feedCrudTestTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// feedCrudColumns mirrors the 7-column feed projection.
var feedCrudColumns = []string{
	"id", "team_id", "name", "description", "created_by_user_id", "created_at", "updated_at",
}

// feedCrudItemColumns mirrors the 11-column feed item projection.
var feedCrudItemColumns = []string{
	"id", "team_id", "feed_id", "project_id", "title", "content",
	"excerpt", "ai_assistant_name", "posted_by_user_id", "archived_at", "posted_at",
}

// testFeedFixture builds the feed written by Create/Update tests.
func testFeedFixture() *models.Feed {
	return &models.Feed{
		TeamID:          "team-1",
		Name:            "General",
		CreatedByUserID: "user-1",
		CreatedAt:       feedCrudTestTime,
		UpdatedAt:       feedCrudTestTime,
	}
}

// feedCrudScenario drives one feed or feed item repository call.
type feedCrudScenario struct {
	name    string
	setup   func(mock sqlmock.Sqlmock)
	wantIs  error
	wantSub string
}

func runFeedCreateScenario(t *testing.T, sc feedCrudScenario) {
	repo, mock, mockDB := setupFeedListTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	feed := testFeedFixture()
	err := repo.Create(context.Background(), feed)

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, "feed-1", feed.ID)
		assert.Equal(t, feedCrudTestTime, feed.CreatedAt)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedRepository_Create(t *testing.T) {
	expectInsert := func(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
		return mock.ExpectQuery(`INSERT INTO feeds`).
			WithArgs("team-1", "General", nil, "user-1", feedCrudTestTime, feedCrudTestTime)
	}

	scenarios := []feedCrudScenario{
		{
			name: "insert returns id and timestamps",
			setup: func(mock sqlmock.Sqlmock) {
				expectInsert(mock).WillReturnRows(
					sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
						AddRow("feed-1", feedCrudTestTime, feedCrudTestTime))
			},
		},
		{
			name: "unique violation reports the duplicate name",
			setup: func(mock sqlmock.Sqlmock) {
				expectInsert(mock).WillReturnError(&pq.Error{Code: uniqueViolationCode})
			},
			wantSub: "feed with name 'General' already exists for this team",
		},
		{
			name: "FK violation maps to team-or-user not found",
			setup: func(mock sqlmock.Sqlmock) {
				expectInsert(mock).WillReturnError(&pq.Error{Code: fkViolationCode})
			},
			wantSub: "team or user not found",
		},
		{
			name: "driver error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				expectInsert(mock).WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to create feed",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runFeedCreateScenario(t, sc) })
	}
}

func runFeedGetByIDScenario(t *testing.T, sc feedCrudScenario) {
	repo, mock, mockDB := setupFeedListTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	feed, err := repo.GetByID(context.Background(), "user-1", "team-1", "feed-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, "feed-1", feed.ID)
		assert.Equal(t, "team-1", feed.TeamID)
		assert.Equal(t, "General", feed.Name)
		assert.Equal(t, "user-1", feed.CreatedByUserID)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedRepository_GetByID(t *testing.T) {
	const getRE = `SELECT .+ FROM feeds f WHERE f\.id = \$1 AND f\.team_id = \$2`

	scenarios := []feedCrudScenario{
		{
			name: "found returns the team-scoped feed",
			setup: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows(feedCrudColumns).
					AddRow("feed-1", "team-1", "General", nil, "user-1", feedCrudTestTime, feedCrudTestTime)
				mock.ExpectQuery(getRE).WithArgs("feed-1", "team-1", "user-1").WillReturnRows(rows)
			},
		},
		{
			name: "no rows maps to the feed not-found sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getRE).WithArgs("feed-1", "team-1", "user-1").WillReturnError(sql.ErrNoRows)
			},
			wantIs: repositories.ErrFeedNotFound,
		},
		{
			name: "driver error is wrapped, not the sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getRE).WithArgs("feed-1", "team-1", "user-1").WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to get feed by ID",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runFeedGetByIDScenario(t, sc) })
	}
}

func runFeedUpdateScenario(t *testing.T, sc feedCrudScenario) {
	repo, mock, mockDB := setupFeedListTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	feed := testFeedFixture()
	feed.ID = "feed-1"
	err := repo.Update(context.Background(), feed)

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, feedCrudTestTime.Add(time.Hour), feed.UpdatedAt,
			"Update must adopt the RETURNING updated_at")
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedRepository_Update(t *testing.T) {
	expectUpdate := func(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
		return mock.ExpectQuery(`UPDATE feeds SET name = \$3, description = \$4, updated_at = \$5`).
			WithArgs("feed-1", "team-1", "General", nil, feedCrudTestTime, "user-1")
	}

	scenarios := []feedCrudScenario{
		{
			name: "membership-gated update returns updated_at",
			setup: func(mock sqlmock.Sqlmock) {
				expectUpdate(mock).WillReturnRows(
					sqlmock.NewRows([]string{"updated_at"}).AddRow(feedCrudTestTime.Add(time.Hour)))
			},
		},
		{
			name: "unique violation reports the duplicate name",
			setup: func(mock sqlmock.Sqlmock) {
				expectUpdate(mock).WillReturnError(&pq.Error{Code: uniqueViolationCode})
			},
			wantSub: "feed with name 'General' already exists for this team",
		},
		{
			name: "no rows maps to the feed not-found sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				expectUpdate(mock).WillReturnError(sql.ErrNoRows)
			},
			wantIs: repositories.ErrFeedNotFound,
		},
		{
			name: "driver error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				expectUpdate(mock).WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to update feed",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runFeedUpdateScenario(t, sc) })
	}
}

func runFeedDeleteScenario(t *testing.T, sc feedCrudScenario) {
	repo, mock, mockDB := setupFeedListTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	err := repo.Delete(context.Background(), "user-1", "team-1", "feed-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedRepository_Delete(t *testing.T) {
	const deleteRE = `DELETE FROM feeds WHERE id = \$1 AND team_id = \$2`

	scenarios := []feedCrudScenario{
		{
			name: "deletes the feed",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(deleteRE).WithArgs("feed-1", "team-1", "user-1").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
		},
		{
			name: "zero rows affected maps to the feed not-found sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(deleteRE).WithArgs("feed-1", "team-1", "user-1").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantIs: repositories.ErrFeedNotFound,
		},
		{
			name: "exec error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(deleteRE).WithArgs("feed-1", "team-1", "user-1").
					WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to delete feed",
		},
		{
			name: "rows-affected error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(deleteRE).WithArgs("feed-1", "team-1", "user-1").
					WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))
			},
			wantSub: "failed to get rows affected",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runFeedDeleteScenario(t, sc) })
	}
}

func runFeedCountAllScenario(t *testing.T, sc feedCrudScenario) {
	repo, mock, mockDB := setupFeedListTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	count, err := repo.CountAll(context.Background(), "user-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, 3, count)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedRepository_CountAll(t *testing.T) {
	const countAllRE = `SELECT COUNT\(DISTINCT f\.id\) FROM feeds f`

	scenarios := []feedCrudScenario{
		{
			name: "counts feeds across the user's teams",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(countAllRE).WithArgs("user-1").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
			},
		},
		{
			name: "query error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(countAllRE).WithArgs("user-1").WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to count feeds",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runFeedCountAllScenario(t, sc) })
	}
}

// testFeedItemFixture builds the feed item written by the Create test.
func testFeedItemFixture() *models.FeedItem {
	return &models.FeedItem{
		TeamID:          "team-1",
		FeedID:          "feed-1",
		Title:           "Shipped the thing",
		Content:         "Long-form body",
		Excerpt:         "Short body",
		AIAssistantName: "claude",
		PostedByUserID:  "user-1",
		PostedAt:        feedCrudTestTime,
	}
}

func runFeedItemCreateScenario(t *testing.T, sc feedCrudScenario) {
	repo, mock, mockDB := setupFeedItemRepoTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	item := testFeedItemFixture()
	err := repo.Create(context.Background(), item)

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, "item-1", item.ID)
		assert.Equal(t, feedCrudTestTime, item.PostedAt)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedItemRepository_Create(t *testing.T) {
	expectInsert := func(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
		return mock.ExpectQuery(`INSERT INTO feed_items`).
			WithArgs("team-1", "feed-1", nil, "Shipped the thing", "Long-form body",
				"Short body", "claude", "user-1", nil, feedCrudTestTime)
	}

	scenarios := []feedCrudScenario{
		{
			name: "insert returns id and posted_at",
			setup: func(mock sqlmock.Sqlmock) {
				expectInsert(mock).WillReturnRows(
					sqlmock.NewRows([]string{"id", "posted_at"}).AddRow("item-1", feedCrudTestTime))
			},
		},
		{
			name: "FK violation maps to the not-found message",
			setup: func(mock sqlmock.Sqlmock) {
				expectInsert(mock).WillReturnError(&pq.Error{Code: fkViolationCode})
			},
			wantSub: "feed, team, project, or user not found",
		},
		{
			name: "driver error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				expectInsert(mock).WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to create feed item",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runFeedItemCreateScenario(t, sc) })
	}
}

func runFeedItemGetByIDScenario(t *testing.T, sc feedCrudScenario) {
	repo, mock, mockDB := setupFeedItemRepoTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	item, err := repo.GetByID(context.Background(), "user-1", "team-1", "item-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, "item-1", item.ID)
		assert.Equal(t, "team-1", item.TeamID)
		assert.Equal(t, "feed-1", item.FeedID)
		assert.Equal(t, "Shipped the thing", item.Title)
		assert.Nil(t, item.ArchivedAt)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedItemRepository_GetByID(t *testing.T) {
	const getRE = `SELECT .+ FROM feed_items fi WHERE fi\.id = \$1 AND fi\.team_id = \$2`

	scenarios := []feedCrudScenario{
		{
			name: "found returns the membership-scoped item",
			setup: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows(feedCrudItemColumns).
					AddRow("item-1", "team-1", "feed-1", nil, "Shipped the thing", "Long-form body",
						"Short body", "claude", "user-1", nil, feedCrudTestTime)
				mock.ExpectQuery(getRE).WithArgs("item-1", "team-1", "user-1").WillReturnRows(rows)
			},
		},
		{
			name: "no rows maps to the feed item not-found sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getRE).WithArgs("item-1", "team-1", "user-1").WillReturnError(sql.ErrNoRows)
			},
			wantIs: repositories.ErrFeedItemNotFound,
		},
		{
			name: "driver error is wrapped, not the sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getRE).WithArgs("item-1", "team-1", "user-1").WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to get feed item by ID",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runFeedItemGetByIDScenario(t, sc) })
	}
}

func runFeedItemArchiveScenario(t *testing.T, sc feedCrudScenario) {
	repo, mock, mockDB := setupFeedItemRepoTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	err := repo.Archive(context.Background(), "user-1", "team-1", "item-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedItemRepository_Archive(t *testing.T) {
	const archiveRE = `UPDATE feed_items SET archived_at = NOW\(\) WHERE id = \$1 AND team_id = \$2 ` +
		`AND archived_at IS NULL`

	scenarios := []feedCrudScenario{
		{
			name: "archives an active item",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(archiveRE).WithArgs("item-1", "team-1", "user-1").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
		},
		{
			name: "zero rows means not found or already archived",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(archiveRE).WithArgs("item-1", "team-1", "user-1").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantSub: "feed item not found or already archived",
		},
		{
			name: "exec error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(archiveRE).WithArgs("item-1", "team-1", "user-1").
					WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to archive feed item",
		},
		{
			name: "rows-affected error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(archiveRE).WithArgs("item-1", "team-1", "user-1").
					WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))
			},
			wantSub: "failed to get rows affected",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runFeedItemArchiveScenario(t, sc) })
	}
}

func runFeedItemUnarchiveScenario(t *testing.T, sc feedCrudScenario) {
	repo, mock, mockDB := setupFeedItemRepoTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	err := repo.Unarchive(context.Background(), "user-1", "team-1", "item-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedItemRepository_Unarchive(t *testing.T) {
	const unarchiveRE = `UPDATE feed_items SET archived_at = NULL WHERE id = \$1 AND team_id = \$2 ` +
		`AND archived_at IS NOT NULL`

	scenarios := []feedCrudScenario{
		{
			name: "unarchives an archived item",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(unarchiveRE).WithArgs("item-1", "team-1", "user-1").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
		},
		{
			name: "zero rows means not found or not archived",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(unarchiveRE).WithArgs("item-1", "team-1", "user-1").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantSub: "feed item not found or not archived",
		},
		{
			name: "exec error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(unarchiveRE).WithArgs("item-1", "team-1", "user-1").
					WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to unarchive feed item",
		},
		{
			name: "rows-affected error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(unarchiveRE).WithArgs("item-1", "team-1", "user-1").
					WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))
			},
			wantSub: "failed to get rows affected",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runFeedItemUnarchiveScenario(t, sc) })
	}
}

func runFeedItemCountAllScenario(t *testing.T, sc feedCrudScenario) {
	repo, mock, mockDB := setupFeedItemRepoTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	count, err := repo.CountAll(context.Background(), "user-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, 9, count)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedItemRepository_CountAll(t *testing.T) {
	const countAllRE = `SELECT COUNT\(DISTINCT fi\.id\) FROM feed_items fi`

	scenarios := []feedCrudScenario{
		{
			name: "counts items across the user's teams including archived",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(countAllRE).WithArgs("user-1").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(9))
			},
		},
		{
			name: "query error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(countAllRE).WithArgs("user-1").WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to count feed items",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runFeedItemCountAllScenario(t, sc) })
	}
}
