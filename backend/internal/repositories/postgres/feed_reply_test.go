package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// feedReplyTestTime is the fixed timestamp used by the reply repository tests.
var feedReplyTestTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// feedReplyColumnsTest mirrors the 7-column projection of the reply queries.
var feedReplyColumnsTest = []string{
	"id", "team_id", "feed_item_id", "content", "posted_by_user_id", "ai_assistant_name", "posted_at",
}

// registerMockDBClose closes the sqlmock database when the test finishes.
func registerMockDBClose(t *testing.T, mockDB *sql.DB) {
	t.Helper()
	t.Cleanup(func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})
}

// assertWantRepoErr asserts a repository call outcome: success when neither
// expectation is set, otherwise sentinel identity (errors.Is) and/or message
// substring.
func assertWantRepoErr(t *testing.T, err error, wantIs error, wantSub string) {
	t.Helper()
	if wantIs == nil && wantSub == "" {
		require.NoError(t, err)
		return
	}
	require.Error(t, err)
	if wantIs != nil {
		assert.ErrorIs(t, err, wantIs)
	}
	if wantSub != "" {
		assert.Contains(t, err.Error(), wantSub)
	}
}

// feedReplyScenario drives one reply repository call against a mocked DB.
type feedReplyScenario struct {
	name    string
	setup   func(mock sqlmock.Sqlmock)
	wantIs  error
	wantSub string
}

func runCreateReplyScenario(t *testing.T, sc feedReplyScenario) {
	repo, mock, mockDB := setupFeedReplyRepoTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	reply := &models.FeedItemReply{
		TeamID: "team-1", FeedItemID: "item-1", Content: "Nice work", PostedByUserID: "user-1",
	}
	created, err := repo.CreateReply(context.Background(), reply)

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, "reply-9", created.ID)
		assert.Equal(t, feedReplyTestTime, created.PostedAt)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedItemReplyRepository_CreateReply(t *testing.T) {
	expectCreate := func(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
		return mock.ExpectQuery(`INSERT INTO feed_item_replies`).
			WithArgs("team-1", "item-1", "Nice work", "user-1", nil, sqlmock.AnyArg())
	}

	scenarios := []feedReplyScenario{
		{
			name: "membership-gated INSERT returns id and posted_at",
			setup: func(mock sqlmock.Sqlmock) {
				expectCreate(mock).WillReturnRows(
					sqlmock.NewRows([]string{"id", "posted_at"}).AddRow("reply-9", feedReplyTestTime))
			},
		},
		{
			name: "FK violation maps to the not-found message",
			setup: func(mock sqlmock.Sqlmock) {
				expectCreate(mock).WillReturnError(&pq.Error{Code: fkViolationCode})
			},
			wantSub: "feed item, team, or user not found",
		},
		{
			name: "no rows means the user is not a team member",
			setup: func(mock sqlmock.Sqlmock) {
				expectCreate(mock).WillReturnError(sql.ErrNoRows)
			},
			wantSub: "user is not a member of the specified team",
		},
		{
			name: "driver error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				expectCreate(mock).WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to create feed item reply",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runCreateReplyScenario(t, sc) })
	}
}

func runGetReplyScenario(t *testing.T, sc feedReplyScenario) {
	repo, mock, mockDB := setupFeedReplyRepoTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	reply, err := repo.GetReply(context.Background(), "user-1", "team-1", "reply-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, "reply-1", reply.ID)
		assert.Equal(t, "team-1", reply.TeamID)
		assert.Equal(t, "item-1", reply.FeedItemID)
		assert.Equal(t, "Great insight", reply.Content)
		assert.Equal(t, "poster-1", reply.PostedByUserID)
		assert.Equal(t, feedReplyTestTime, reply.PostedAt)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedItemReplyRepository_GetReply(t *testing.T) {
	const getReplyRE = `SELECT .+ FROM feed_item_replies WHERE team_id = \$1 AND id = \$2`

	scenarios := []feedReplyScenario{
		{
			name: "found returns the team-scoped reply",
			setup: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows(feedReplyColumnsTest).
					AddRow("reply-1", "team-1", "item-1", "Great insight", "poster-1", nil, feedReplyTestTime)
				mock.ExpectQuery(getReplyRE).WithArgs("team-1", "reply-1", "user-1").WillReturnRows(rows)
			},
		},
		{
			name: "no rows maps to the reply not-found sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getReplyRE).WithArgs("team-1", "reply-1", "user-1").WillReturnError(sql.ErrNoRows)
			},
			wantIs: repositories.ErrFeedItemReplyNotFound,
		},
		{
			name: "driver error is wrapped, not the sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getReplyRE).WithArgs("team-1", "reply-1", "user-1").WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to get feed item reply",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runGetReplyScenario(t, sc) })
	}
}

// listRepliesScenario drives ListReplies with explicit paging inputs.
type listRepliesScenario struct {
	name      string
	page      int
	limit     int
	setup     func(mock sqlmock.Sqlmock)
	wantSub   string
	wantLen   int
	wantTotal int
}

func runListRepliesScenario(t *testing.T, sc listRepliesScenario) {
	repo, mock, mockDB := setupFeedReplyRepoTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	replies, total, err := repo.ListReplies(context.Background(), "team-1", "item-1", sc.page, sc.limit)

	if sc.wantSub != "" {
		require.Error(t, err)
		assert.Contains(t, err.Error(), sc.wantSub)
	} else {
		require.NoError(t, err)
		require.NotNil(t, replies, "ListReplies must return a non-nil slice, never nil")
		assert.Len(t, replies, sc.wantLen)
		assert.Equal(t, sc.wantTotal, total)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedItemReplyRepository_ListReplies(t *testing.T) {
	const countRE = `SELECT COUNT\(\*\) FROM feed_item_replies WHERE team_id = \$1 AND feed_item_id = \$2`
	const pageRE = `SELECT .+ FROM feed_item_replies WHERE team_id = \$1 AND feed_item_id = \$2 ` +
		`ORDER BY posted_at DESC LIMIT \$3 OFFSET \$4`

	expectCount := func(mock sqlmock.Sqlmock, total int) {
		mock.ExpectQuery(countRE).WithArgs("team-1", "item-1").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(total))
	}

	scenarios := []listRepliesScenario{
		{
			name: "second page computes offset from page and limit",
			page: 2, limit: 2,
			setup: func(mock sqlmock.Sqlmock) {
				expectCount(mock, 5)
				rows := sqlmock.NewRows(feedReplyColumnsTest).
					AddRow("reply-3", "team-1", "item-1", "third", "poster-1", nil, feedReplyTestTime).
					AddRow("reply-4", "team-1", "item-1", "fourth", "poster-2", nil, feedReplyTestTime)
				// offset = (2-1)*2 = 2
				mock.ExpectQuery(pageRE).WithArgs("team-1", "item-1", 2, 2).WillReturnRows(rows)
			},
			wantLen: 2, wantTotal: 5,
		},
		{
			name: "empty result is a non-nil empty slice",
			page: 1, limit: 10,
			setup: func(mock sqlmock.Sqlmock) {
				expectCount(mock, 0)
				mock.ExpectQuery(pageRE).WithArgs("team-1", "item-1", 10, 0).
					WillReturnRows(sqlmock.NewRows(feedReplyColumnsTest))
			},
			wantLen: 0, wantTotal: 0,
		},
		{
			name: "count query error propagates wrapped",
			page: 1, limit: 10,
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(countRE).WithArgs("team-1", "item-1").WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to count feed item replies",
		},
		{
			name: "page query error propagates wrapped",
			page: 1, limit: 10,
			setup: func(mock sqlmock.Sqlmock) {
				expectCount(mock, 3)
				mock.ExpectQuery(pageRE).WithArgs("team-1", "item-1", 10, 0).WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to list feed item replies",
		},
		{
			name: "scan error propagates wrapped",
			page: 1, limit: 10,
			setup: func(mock sqlmock.Sqlmock) {
				expectCount(mock, 1)
				// One fewer column than the scan expects forces a scan error.
				mock.ExpectQuery(pageRE).WithArgs("team-1", "item-1", 10, 0).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("reply-1"))
			},
			wantSub: "failed to scan feed item reply",
		},
		{
			name: "row iteration error propagates wrapped",
			page: 1, limit: 10,
			setup: func(mock sqlmock.Sqlmock) {
				expectCount(mock, 1)
				rows := sqlmock.NewRows(feedReplyColumnsTest).
					AddRow("reply-1", "team-1", "item-1", "text", "poster-1", nil, feedReplyTestTime).
					RowError(0, sql.ErrConnDone)
				mock.ExpectQuery(pageRE).WithArgs("team-1", "item-1", 10, 0).WillReturnRows(rows)
			},
			wantSub: "failed to iterate feed item replies",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runListRepliesScenario(t, sc) })
	}
}

// countRepliesScenario drives CountRepliesByItemIDs with an explicit ID batch.
type countRepliesScenario struct {
	name       string
	itemIDs    []string
	setup      func(mock sqlmock.Sqlmock)
	wantSub    string
	wantCounts map[string]int
}

func runCountRepliesScenario(t *testing.T, sc countRepliesScenario) {
	repo, mock, mockDB := setupFeedReplyRepoTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	counts, err := repo.CountRepliesByItemIDs(context.Background(), "team-1", sc.itemIDs)

	if sc.wantSub != "" {
		require.Error(t, err)
		assert.Contains(t, err.Error(), sc.wantSub)
	} else {
		require.NoError(t, err)
		assert.Equal(t, sc.wantCounts, counts)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedItemReplyRepository_CountRepliesByItemIDs(t *testing.T) {
	const batchRE = `SELECT feed_item_id, COUNT\(\*\) FROM feed_item_replies ` +
		`WHERE team_id = \$1 AND feed_item_id = ANY\(\$2\) GROUP BY feed_item_id`

	scenarios := []countRepliesScenario{
		{
			name:    "batch of two items builds the count map",
			itemIDs: []string{"item-1", "item-2"},
			setup: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"feed_item_id", "count"}).
					AddRow("item-1", 2).
					AddRow("item-2", 5)
				mock.ExpectQuery(batchRE).
					WithArgs("team-1", pq.Array([]string{"item-1", "item-2"})).
					WillReturnRows(rows)
			},
			wantCounts: map[string]int{"item-1": 2, "item-2": 5},
		},
		{
			name:    "empty ID slice short-circuits without querying",
			itemIDs: []string{},
			// No expectations: ExpectationsWereMet proves no query was issued.
			setup:      func(_ sqlmock.Sqlmock) {},
			wantCounts: map[string]int{},
		},
		{
			name:    "query error propagates wrapped",
			itemIDs: []string{"item-1"},
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(batchRE).
					WithArgs("team-1", pq.Array([]string{"item-1"})).
					WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to count replies by item IDs",
		},
		{
			name:    "scan error propagates wrapped",
			itemIDs: []string{"item-1"},
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(batchRE).
					WithArgs("team-1", pq.Array([]string{"item-1"})).
					WillReturnRows(sqlmock.NewRows([]string{"feed_item_id"}).AddRow("item-1"))
			},
			wantSub: "failed to scan reply count",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runCountRepliesScenario(t, sc) })
	}
}

// runListReplyPostersErrScenario covers the row-level error paths of
// ListReplyPostersByItemID (happy path and query error live in
// feed_embedding_lookup_test.go).
func runListReplyPostersErrScenario(t *testing.T, sc feedReplyScenario) {
	repo, mock, mockDB := setupFeedReplyRepoTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	_, err := repo.ListReplyPostersByItemID(context.Background(), "team-1", "item-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedItemReplyRepository_ListReplyPostersByItemID_RowErrors(t *testing.T) {
	const postersRE = `SELECT id, posted_by_user_id FROM feed_item_replies ` +
		`WHERE team_id = \$1 AND feed_item_id = \$2`

	scenarios := []feedReplyScenario{
		{
			name: "scan error propagates wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(postersRE).WithArgs("team-1", "item-1").
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("reply-1"))
			},
			wantSub: "failed to scan reply poster",
		},
		{
			name: "row iteration error propagates wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "posted_by_user_id"}).
					AddRow("reply-1", "poster-a").
					RowError(0, sql.ErrConnDone)
				mock.ExpectQuery(postersRE).WithArgs("team-1", "item-1").WillReturnRows(rows)
			},
			wantSub: "failed to iterate reply posters",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runListReplyPostersErrScenario(t, sc) })
	}
}

func runReplyCountAllScenario(t *testing.T, sc feedReplyScenario) {
	repo, mock, mockDB := setupFeedReplyRepoTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	count, err := repo.CountAll(context.Background(), "user-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, 7, count)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedItemReplyRepository_CountAll(t *testing.T) {
	const countAllRE = `SELECT COUNT\(DISTINCT fir\.id\) FROM feed_item_replies fir`

	scenarios := []feedReplyScenario{
		{
			name: "counts replies across the user's teams",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(countAllRE).WithArgs("user-1").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7))
			},
		},
		{
			name: "query error propagates wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(countAllRE).WithArgs("user-1").WillReturnError(errors.New("boom"))
			},
			wantSub: "failed to count feed item replies",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runReplyCountAllScenario(t, sc) })
	}
}
