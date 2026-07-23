package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

// boolPtr returns a pointer to b, for exercising the tri-state Archived filter.
func boolPtr(b bool) *bool { return &b }

// feedListColumnsTest mirrors the 7 columns scanned by FeedRepository.List.
var feedListColumnsTest = []string{
	"id", "team_id", "name", "description", "created_by_user_id", "created_at", "updated_at",
}

// feedWithLastPostColumnsTest mirrors the 8 columns scanned by ListWithLastPost.
var feedWithLastPostColumnsTest = []string{
	"id", "team_id", "name", "description", "created_by_user_id",
	"created_at", "updated_at", "last_post_at",
}

// feedItemListColumnsTest mirrors the 11 columns scanned by FeedItemRepository.List.
var feedItemListColumnsTest = []string{
	"id", "team_id", "feed_id", "project_id", "title", "content",
	"excerpt", "ai_assistant_name", "posted_by_user_id", "archived_at", "posted_at",
}

// feedListBaseArgs is the base argument set squirrel binds for every team-scoped
// feed/feed-item List query: team_id, then the team/user pair repeated per EXISTS
// clause.
func feedListBaseArgs() []driver.Value {
	return []driver.Value{"team-123", "team-123", "user-123", "team-123", "user-123"}
}

func setupFeedListTest(t *testing.T) (*FeedRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewFeedRepository(&database.DB{DB: mockDB}).(*FeedRepository)
	return repo, mock, mockDB
}

func setupFeedItemListTest(t *testing.T) (*FeedItemRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewFeedItemRepository(&database.DB{DB: mockDB}).(*FeedItemRepository)
	return repo, mock, mockDB
}

func feedListOneRow(now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(feedListColumnsTest).AddRow(
		"feed-1", "team-123", "Releases", "desc", "user-123", now, now,
	)
}

func feedItemListOneRow(now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(feedItemListColumnsTest).AddRow(
		"item-1", "team-123", "feed-1", "project-1", "Title", "Content",
		"Excerpt", "claude", "user-123", nil, now,
	)
}

// ---------------------------------------------------------------------------
// FeedRepository.List
// ---------------------------------------------------------------------------

//nolint:funlen // table-driven test with multiple filter and pagination scenarios
func TestFeedRepository_ListSquirrel(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name        string
		filters     repositories.FeedFilters
		setupMock   func(mock sqlmock.Sqlmock)
		expectTotal int
		expectCount int
	}{
		{
			name:    "baseline binds team-scoped base args and default ordering",
			filters: repositories.FeedFilters{TeamID: "team-123", Page: 1, Limit: 10},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feeds f`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feeds f .* ORDER BY f\.created_at DESC LIMIT 10 OFFSET 0`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(feedListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:    "search binds ILIKE term twice after base args",
			filters: repositories.FeedFilters{TeamID: "team-123", Search: "alpha", Page: 1, Limit: 10},
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(feedListBaseArgs(), "%alpha%", "%alpha%")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feeds f .*f\.name ILIKE .* OR f\.description ILIKE`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feeds f .*f\.name ILIKE .* OR f\.description ILIKE`).
					WithArgs(args...).
					WillReturnRows(feedListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:    "clamps non-positive page and limit to LIMIT 0 OFFSET 0",
			filters: repositories.FeedFilters{TeamID: "team-123", Page: 0, Limit: -5},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feeds f`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
				mock.ExpectQuery(`FROM feeds f .* LIMIT 0 OFFSET 0`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows(feedListColumnsTest))
			},
			expectTotal: 3,
			expectCount: 0,
		},
		{
			name:    "computes offset from page and limit",
			filters: repositories.FeedFilters{TeamID: "team-123", Page: 3, Limit: 5},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feeds f`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(20))
				// offset = (3-1)*5 = 10
				mock.ExpectQuery(`FROM feeds f .* LIMIT 5 OFFSET 10`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(feedListOneRow(now))
			},
			expectTotal: 20,
			expectCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupFeedListTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			tt.setupMock(mock)

			got, total, err := repo.List(ctx, "user-123", tt.filters)

			assert.NoError(t, err)
			assert.NotNil(t, got, "list must return a non-nil empty slice, never nil")
			assert.Len(t, got, tt.expectCount)
			assert.Equal(t, tt.expectTotal, total)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestFeedRepository_List_ExplicitProjection pins the full 7-column projection so
// column drift is caught.
func TestFeedRepository_List_ExplicitProjection(t *testing.T) {
	repo, mock, mockDB := setupFeedListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	filters := repositories.FeedFilters{TeamID: "team-123", Page: 1, Limit: 10}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feeds f`).
		WithArgs(feedListBaseArgs()...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(
		`SELECT f\.id, f\.team_id, f\.name, f\.description, ` +
			`f\.created_by_user_id, f\.created_at, f\.updated_at FROM feeds f WHERE`,
	).
		WithArgs(feedListBaseArgs()...).
		WillReturnRows(feedListOneRow(now))

	feeds, total, err := repo.List(ctx, "user-123", filters)
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, feeds, 1)
	assert.Equal(t, "feed-1", feeds[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestFeedRepository_List_RequiresTeamID verifies the required-TeamID guard
// short-circuits before any query is issued.
func TestFeedRepository_List_RequiresTeamID(t *testing.T) {
	repo, mock, mockDB := setupFeedListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	feeds, total, err := repo.List(
		context.Background(), "user-123", repositories.FeedFilters{Page: 1, Limit: 10},
	)

	require.Error(t, err)
	assert.EqualError(t, err, "TeamID is required but was empty")
	assert.Nil(t, feeds)
	assert.Zero(t, total)
	assert.NoError(t, mock.ExpectationsWereMet())
}

//nolint:funlen // table-driven error-path test
func TestFeedRepository_ListSquirrel_ErrorPaths(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "count error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feeds f`).
					WithArgs(feedListBaseArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to count feeds",
		},
		{
			name: "list query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feeds f`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feeds f`).
					WithArgs(feedListBaseArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to list feeds",
		},
		{
			name: "scan error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feeds f`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feeds f`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("feed-1"))
			},
			wantErr: "failed to scan feed",
		},
		{
			name: "iterate error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feeds f`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feeds f`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(feedListOneRow(now).RowError(0, sql.ErrConnDone))
			},
			wantErr: "failed to iterate feeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupFeedListTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			tt.setupMock(mock)

			_, _, err := repo.List(context.Background(), "user-123",
				repositories.FeedFilters{TeamID: "team-123", Page: 1, Limit: 10})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// ---------------------------------------------------------------------------
// FeedRepository.ListWithLastPost
// ---------------------------------------------------------------------------

// TestFeedRepository_ListWithLastPost_Baseline asserts the LEFT JOIN, MAX
// aggregate, GROUP BY and ORDER BY appear in the emitted SQL, that no count query
// is issued, and that the 8th column (last_post_at) scans for both NULL and
// populated values.
func TestFeedRepository_ListWithLastPost_Baseline(t *testing.T) {
	repo, mock, mockDB := setupFeedListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	filters := repositories.FeedFilters{TeamID: "team-123", Page: 1, Limit: 10}

	// One query only — no count query expected for ListWithLastPost.
	mock.ExpectQuery(
		`MAX\(fi\.posted_at\) AS last_post_at FROM feeds f ` +
			`LEFT JOIN feed_items fi ON fi\.feed_id = f\.id AND fi\.archived_at IS NULL ` +
			`WHERE .* GROUP BY f\.id, f\.team_id, f\.name, f\.description, ` +
			`f\.created_by_user_id, f\.created_at, f\.updated_at ` +
			`ORDER BY f\.created_at DESC LIMIT 10 OFFSET 0`,
	).
		WithArgs(feedListBaseArgs()...).
		WillReturnRows(sqlmock.NewRows(feedWithLastPostColumnsTest).
			AddRow("feed-1", "team-123", "Releases", "desc", "user-123", now, now, now).
			AddRow("feed-2", "team-123", "Empty", "desc", "user-123", now, now, nil))

	feeds, err := repo.ListWithLastPost(ctx, "user-123", filters)
	require.NoError(t, err)
	require.Len(t, feeds, 2)
	require.NotNil(t, feeds[0].LastPostAt, "populated last_post_at must scan to a set pointer")
	assert.WithinDuration(t, now, *feeds[0].LastPostAt, time.Second)
	assert.Nil(t, feeds[1].LastPostAt, "NULL last_post_at must scan to nil")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedRepository_ListWithLastPost_Search(t *testing.T) {
	repo, mock, mockDB := setupFeedListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	filters := repositories.FeedFilters{TeamID: "team-123", Search: "alpha", Page: 1, Limit: 10}

	args := append(feedListBaseArgs(), "%alpha%", "%alpha%")
	mock.ExpectQuery(`FROM feeds f LEFT JOIN feed_items fi .*f\.name ILIKE .* OR f\.description ILIKE`).
		WithArgs(args...).
		WillReturnRows(sqlmock.NewRows(feedWithLastPostColumnsTest).
			AddRow("feed-1", "team-123", "Alpha", "desc", "user-123", now, now, now))

	feeds, err := repo.ListWithLastPost(ctx, "user-123", filters)
	require.NoError(t, err)
	require.Len(t, feeds, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedRepository_ListWithLastPost_RequiresTeamID(t *testing.T) {
	repo, mock, mockDB := setupFeedListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	feeds, err := repo.ListWithLastPost(
		context.Background(), "user-123", repositories.FeedFilters{Page: 1, Limit: 10},
	)

	require.Error(t, err)
	assert.EqualError(t, err, "TeamID is required but was empty")
	assert.Nil(t, feeds)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedRepository_ListWithLastPost_ErrorPaths(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "list query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM feeds f LEFT JOIN feed_items fi`).
					WithArgs(feedListBaseArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to list feeds with last post",
		},
		{
			name: "scan error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM feeds f LEFT JOIN feed_items fi`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("feed-1"))
			},
			wantErr: "failed to scan feed with last post",
		},
		{
			name: "iterate error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM feeds f LEFT JOIN feed_items fi`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows(feedWithLastPostColumnsTest).
						AddRow("feed-1", "team-123", "Releases", "desc", "user-123", now, now, now).
						RowError(0, sql.ErrConnDone))
			},
			wantErr: "failed to iterate feeds with last post",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupFeedListTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			tt.setupMock(mock)

			_, err := repo.ListWithLastPost(context.Background(), "user-123",
				repositories.FeedFilters{TeamID: "team-123", Page: 1, Limit: 10})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// ---------------------------------------------------------------------------
// FeedItemRepository.List
// ---------------------------------------------------------------------------

//nolint:funlen // table-driven test covering tri-state Archived and every pointer filter
func TestFeedItemRepository_ListSquirrel(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	feedID := "feed-x"
	projectID := "project-x"
	assistant := "claude"

	tests := []struct {
		name        string
		filters     repositories.FeedItemFilters
		setupMock   func(mock sqlmock.Sqlmock)
		expectTotal int
		expectCount int
	}{
		{
			name:    "baseline defaults to archived_at IS NULL with base args only",
			filters: repositories.FeedItemFilters{TeamID: "team-123", Page: 1, Limit: 10},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feed_items fi .*fi\.archived_at IS NULL`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feed_items fi .*fi\.archived_at IS NULL.* ` +
					`ORDER BY fi\.posted_at DESC LIMIT 10 OFFSET 0`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(feedItemListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "Archived=false yields IS NULL (active only)",
			filters: repositories.FeedItemFilters{
				TeamID: "team-123", Archived: boolPtr(false), Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feed_items fi .*fi\.archived_at IS NULL`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feed_items fi .*fi\.archived_at IS NULL`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(feedItemListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "Archived=true yields IS NOT NULL (archived only)",
			filters: repositories.FeedItemFilters{
				TeamID: "team-123", Archived: boolPtr(true), Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feed_items fi .*fi\.archived_at IS NOT NULL`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feed_items fi .*fi\.archived_at IS NOT NULL`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(feedItemListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "FeedID filter binds after base args",
			filters: repositories.FeedItemFilters{
				TeamID: "team-123", FeedID: &feedID, Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(feedListBaseArgs(), "feed-x")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feed_items fi .*fi\.feed_id = `).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feed_items fi .*fi\.feed_id = `).
					WithArgs(args...).
					WillReturnRows(feedItemListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "ProjectID filter binds after base args",
			filters: repositories.FeedItemFilters{
				TeamID: "team-123", ProjectID: &projectID, Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(feedListBaseArgs(), "project-x")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feed_items fi .*fi\.project_id = `).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feed_items fi .*fi\.project_id = `).
					WithArgs(args...).
					WillReturnRows(feedItemListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "AIAssistantName filter binds after base args",
			filters: repositories.FeedItemFilters{
				TeamID: "team-123", AIAssistantName: &assistant, Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(feedListBaseArgs(), "claude")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feed_items fi .*fi\.ai_assistant_name = `).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feed_items fi .*fi\.ai_assistant_name = `).
					WithArgs(args...).
					WillReturnRows(feedItemListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "search binds ILIKE term twice after base args",
			filters: repositories.FeedItemFilters{
				TeamID: "team-123", Search: "alpha", Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(feedListBaseArgs(), "%alpha%", "%alpha%")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feed_items fi .*fi\.title ILIKE .* OR fi\.content ILIKE`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feed_items fi .*fi\.title ILIKE .* OR fi\.content ILIKE`).
					WithArgs(args...).
					WillReturnRows(feedItemListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "combined filters bind in deterministic order then archived predicate",
			filters: repositories.FeedItemFilters{
				TeamID: "team-123", FeedID: &feedID, ProjectID: &projectID,
				AIAssistantName: &assistant, Archived: boolPtr(true), Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(feedListBaseArgs(), "feed-x", "project-x", "claude")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feed_items fi .*fi\.feed_id = .*fi\.project_id = ` +
					`.*fi\.ai_assistant_name = .*fi\.archived_at IS NOT NULL`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feed_items fi .*fi\.feed_id = .*fi\.project_id = ` +
					`.*fi\.ai_assistant_name = .*fi\.archived_at IS NOT NULL`).
					WithArgs(args...).
					WillReturnRows(feedItemListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "combined filters with search bind search args after equality filters",
			filters: repositories.FeedItemFilters{
				TeamID: "team-123", FeedID: &feedID, ProjectID: &projectID,
				AIAssistantName: &assistant, Search: "alpha", Archived: boolPtr(true), Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				args := append(feedListBaseArgs(), "feed-x", "project-x", "claude", "%alpha%", "%alpha%")
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feed_items fi .*fi\.feed_id = .*fi\.project_id = ` +
					`.*fi\.ai_assistant_name = .*fi\.title ILIKE .* OR fi\.content ILIKE .*fi\.archived_at IS NOT NULL`).
					WithArgs(args...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feed_items fi .*fi\.feed_id = .*fi\.project_id = ` +
					`.*fi\.ai_assistant_name = .*fi\.title ILIKE .* OR fi\.content ILIKE .*fi\.archived_at IS NOT NULL`).
					WithArgs(args...).
					WillReturnRows(feedItemListOneRow(now))
			},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:    "clamps non-positive page and limit to LIMIT 0 OFFSET 0",
			filters: repositories.FeedItemFilters{TeamID: "team-123", Page: 0, Limit: -5},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feed_items fi`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
				mock.ExpectQuery(`FROM feed_items fi .* LIMIT 0 OFFSET 0`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows(feedItemListColumnsTest))
			},
			expectTotal: 3,
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupFeedItemListTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			tt.setupMock(mock)

			got, total, err := repo.List(ctx, "user-123", tt.filters)

			assert.NoError(t, err)
			assert.NotNil(t, got, "list must return a non-nil empty slice, never nil")
			assert.Len(t, got, tt.expectCount)
			assert.Equal(t, tt.expectTotal, total)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestFeedItemRepository_List_ExplicitProjection pins the full 11-column
// projection so column drift is caught.
func TestFeedItemRepository_List_ExplicitProjection(t *testing.T) {
	repo, mock, mockDB := setupFeedItemListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	filters := repositories.FeedItemFilters{TeamID: "team-123", Page: 1, Limit: 10}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feed_items fi`).
		WithArgs(feedListBaseArgs()...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(
		`SELECT fi\.id, fi\.team_id, fi\.feed_id, fi\.project_id, fi\.title, fi\.content, ` +
			`fi\.excerpt, fi\.ai_assistant_name, fi\.posted_by_user_id, fi\.archived_at, fi\.posted_at ` +
			`FROM feed_items fi WHERE`,
	).
		WithArgs(feedListBaseArgs()...).
		WillReturnRows(feedItemListOneRow(now))

	items, total, err := repo.List(ctx, "user-123", filters)
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "item-1", items[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestFeedItemRepository_List_RequiresTeamID verifies the required-TeamID guard
// short-circuits before any query is issued.
func TestFeedItemRepository_List_RequiresTeamID(t *testing.T) {
	repo, mock, mockDB := setupFeedItemListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	items, total, err := repo.List(
		context.Background(), "user-123", repositories.FeedItemFilters{Page: 1, Limit: 10},
	)

	require.Error(t, err)
	assert.EqualError(t, err, "TeamID is required but was empty")
	assert.Nil(t, items)
	assert.Zero(t, total)
	assert.NoError(t, mock.ExpectationsWereMet())
}

//nolint:funlen // table-driven error-path test
func TestFeedItemRepository_ListSquirrel_ErrorPaths(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "count error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feed_items fi`).
					WithArgs(feedListBaseArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to count feed items",
		},
		{
			name: "list query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feed_items fi`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feed_items fi`).
					WithArgs(feedListBaseArgs()...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to list feed items",
		},
		{
			name: "scan error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feed_items fi`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feed_items fi`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("item-1"))
			},
			wantErr: "failed to scan feed item",
		},
		{
			name: "iterate error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feed_items fi`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM feed_items fi`).
					WithArgs(feedListBaseArgs()...).
					WillReturnRows(feedItemListOneRow(now).RowError(0, sql.ErrConnDone))
			},
			wantErr: "failed to iterate feed items",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupFeedItemListTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			tt.setupMock(mock)

			_, _, err := repo.List(context.Background(), "user-123",
				repositories.FeedItemFilters{TeamID: "team-123", Page: 1, Limit: 10})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
