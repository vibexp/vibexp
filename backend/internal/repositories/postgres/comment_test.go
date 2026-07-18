package postgres

// sqlmock unit tests for CommentRepository (comment.go). The integration file
// (comment_integration_test.go) covers real-database semantics; these tests pin
// the unit surface: argument wiring, row scanning, sentinel mapping, and error
// wrapping.

import (
	"context"
	"database/sql"
	"database/sql/driver"
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

// Fixed timestamp shared by the comment fixtures and their assertions.
var commentTestNow = time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)

var commentTestColumns = []string{
	"id", "team_id", "resource_type", "resource_id", "user_id", "content", "created_at", "updated_at",
}

func newCommentMockRepo(t *testing.T) (repositories.CommentRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, mockDB := newSquirrelMockRepo(t)
	return NewCommentRepository(db), mock, mockDB
}

func commentFixtureRows() *sqlmock.Rows {
	return sqlmock.NewRows(commentTestColumns).AddRow(
		"comment-1", "team-1", "artifact", "res-1", "user-1", "hello", commentTestNow, commentTestNow,
	)
}

// assertCommentError asserts the sentinel and/or message fragment a
// comment-repo call must return; both zero values mean "no error expected".
func assertCommentError(t *testing.T, err error, wantIs error, wantSub string) {
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

// assertHappyComment pins the full field mapping of the fixture row.
func assertHappyComment(t *testing.T, got *models.Comment) {
	t.Helper()
	require.NotNil(t, got)
	assert.Equal(t, "comment-1", got.ID)
	assert.Equal(t, "team-1", got.TeamID)
	assert.Equal(t, "artifact", got.ResourceType)
	assert.Equal(t, "res-1", got.ResourceID)
	assert.Equal(t, "user-1", got.UserID)
	assert.Equal(t, "hello", got.Content)
	assert.Equal(t, commentTestNow, got.CreatedAt)
	assert.Equal(t, commentTestNow, got.UpdatedAt)
}

type commentCreateScenario struct {
	name     string
	queryErr error
	wantSub  string
}

func runCommentCreateScenario(t *testing.T, sc commentCreateScenario) {
	t.Helper()
	repo, mock, mockDB := newCommentMockRepo(t)
	defer closeMockDB(t, mockDB)

	exp := mock.ExpectQuery(`INSERT INTO comments`).
		WithArgs("team-1", "artifact", "res-1", "user-1", "hello")
	if sc.queryErr != nil {
		exp.WillReturnError(sc.queryErr)
	} else {
		exp.WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow("comment-1", commentTestNow, commentTestNow))
	}

	comment := &models.Comment{
		TeamID: "team-1", ResourceType: "artifact", ResourceID: "res-1",
		UserID: "user-1", Content: "hello",
	}
	err := repo.Create(context.Background(), comment)

	assertCommentError(t, err, sc.queryErr, sc.wantSub)
	if sc.queryErr == nil {
		assert.Equal(t, "comment-1", comment.ID)
		assert.Equal(t, commentTestNow, comment.CreatedAt)
		assert.Equal(t, commentTestNow, comment.UpdatedAt)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCommentRepository_Create(t *testing.T) {
	scenarios := []commentCreateScenario{
		{name: "happy path scans generated id and timestamps"},
		{
			name:     "FK violation maps to team-or-user-not-found",
			queryErr: &pq.Error{Code: fkViolationCode},
			wantSub:  "team or user not found for comment",
		},
		{name: "driver error is wrapped", queryErr: sql.ErrConnDone, wantSub: "failed to create comment"},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runCommentCreateScenario(t, sc) })
	}
}

// commentRowMethod describes a single-row method (GetByID / UpdateContent):
// both are team-scoped, scan the same column list, and map no-rows to
// ErrCommentNotFound, so they share one scenario matrix.
type commentRowMethod struct {
	name    string
	pattern string
	args    []driver.Value
	call    func(ctx context.Context, repo repositories.CommentRepository) (*models.Comment, error)
}

type commentRowScenario struct {
	name     string
	queryErr error
	wantIs   error
	wantSub  string
}

func runCommentRowScenario(t *testing.T, m commentRowMethod, sc commentRowScenario) {
	t.Helper()
	repo, mock, mockDB := newCommentMockRepo(t)
	defer closeMockDB(t, mockDB)

	exp := mock.ExpectQuery(m.pattern).WithArgs(m.args...)
	if sc.queryErr != nil {
		exp.WillReturnError(sc.queryErr)
	} else {
		exp.WillReturnRows(commentFixtureRows())
	}

	got, err := m.call(context.Background(), repo)
	if sc.queryErr == nil {
		require.NoError(t, err)
		assertHappyComment(t, got)
	} else {
		assert.Nil(t, got)
		assertCommentError(t, err, sc.wantIs, sc.wantSub)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCommentRepository_GetByIDAndUpdateContent(t *testing.T) {
	methods := []commentRowMethod{
		{
			name:    "GetByID",
			pattern: `FROM comments WHERE id = \$1 AND team_id = \$2`,
			args:    []driver.Value{"comment-1", "team-1"},
			call: func(ctx context.Context, repo repositories.CommentRepository) (*models.Comment, error) {
				return repo.GetByID(ctx, "team-1", "comment-1")
			},
		},
		{
			name:    "UpdateContent",
			pattern: `UPDATE comments SET content = \$1, updated_at = now\(\) WHERE id = \$2 AND team_id = \$3`,
			args:    []driver.Value{"hello", "comment-1", "team-1"},
			call: func(ctx context.Context, repo repositories.CommentRepository) (*models.Comment, error) {
				return repo.UpdateContent(ctx, "team-1", "comment-1", "hello")
			},
		},
	}
	scenarios := []commentRowScenario{
		{name: "happy path scans the full row"},
		{
			name:     "no rows maps to the not-found sentinel",
			queryErr: sql.ErrNoRows,
			wantIs:   repositories.ErrCommentNotFound,
		},
		{name: "driver error is wrapped", queryErr: sql.ErrConnDone, wantIs: sql.ErrConnDone, wantSub: "failed to"},
	}
	for _, m := range methods {
		for _, sc := range scenarios {
			t.Run(m.name+"/"+sc.name, func(t *testing.T) { runCommentRowScenario(t, m, sc) })
		}
	}
}

const (
	commentCountPattern = `SELECT COUNT\(\*\) FROM comments`
	commentListPattern  = `FROM comments WHERE team_id = \$1 AND resource_type = \$2 AND resource_id = \$3 ` +
		`ORDER BY created_at DESC`
)

func expectCommentCount(mock sqlmock.Sqlmock, total int) {
	mock.ExpectQuery(commentCountPattern).
		WithArgs("team-1", "artifact", "res-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(total))
}

type commentListScenario struct {
	name      string
	setup     func(mock sqlmock.Sqlmock)
	wantLen   int
	wantTotal int
	wantSub   string
}

// runCommentListScenario calls ListByResource with page=2, limit=10; every
// non-error scenario therefore pins the offset math offset=(page-1)*limit=10
// via the expected query arguments.
func runCommentListScenario(t *testing.T, sc commentListScenario) {
	t.Helper()
	repo, mock, mockDB := newCommentMockRepo(t)
	defer closeMockDB(t, mockDB)
	sc.setup(mock)

	comments, total, err := repo.ListByResource(context.Background(), "team-1", "artifact", "res-1", 2, 10)
	if sc.wantSub != "" {
		require.Error(t, err)
		assert.Contains(t, err.Error(), sc.wantSub)
	} else {
		require.NoError(t, err)
		require.NotNil(t, comments, "empty result must be a non-nil slice")
		assert.Len(t, comments, sc.wantLen)
		assert.Equal(t, sc.wantTotal, total)
	}
	if sc.wantLen > 0 {
		assertHappyComment(t, &comments[0])
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCommentRepository_ListByResource(t *testing.T) {
	scenarios := []commentListScenario{
		{
			name: "happy path pins offset math and returns rows with total",
			setup: func(mock sqlmock.Sqlmock) {
				expectCommentCount(mock, 5)
				mock.ExpectQuery(commentListPattern).
					WithArgs("team-1", "artifact", "res-1", 10, 10).
					WillReturnRows(commentFixtureRows().AddRow(
						"comment-2", "team-1", "artifact", "res-1", "user-2", "second",
						commentTestNow, commentTestNow))
			},
			wantLen:   2,
			wantTotal: 5,
		},
		{
			name: "empty page returns a non-nil empty slice",
			setup: func(mock sqlmock.Sqlmock) {
				expectCommentCount(mock, 0)
				mock.ExpectQuery(commentListPattern).
					WithArgs("team-1", "artifact", "res-1", 10, 10).
					WillReturnRows(sqlmock.NewRows(commentTestColumns))
			},
		},
		{
			name: "count error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(commentCountPattern).
					WithArgs("team-1", "artifact", "res-1").
					WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to count comments",
		},
		{
			name: "list query error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				expectCommentCount(mock, 5)
				mock.ExpectQuery(commentListPattern).
					WithArgs("team-1", "artifact", "res-1", 10, 10).
					WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to list comments",
		},
		{
			name: "scan error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				expectCommentCount(mock, 1)
				mock.ExpectQuery(commentListPattern).
					WithArgs("team-1", "artifact", "res-1", 10, 10).
					WillReturnRows(sqlmock.NewRows(commentTestColumns).AddRow(
						"comment-1", "team-1", "artifact", "res-1", "user-1", "hello",
						"not-a-time", commentTestNow))
			},
			wantSub: "failed to scan comment",
		},
		{
			name: "row iteration error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				expectCommentCount(mock, 1)
				mock.ExpectQuery(commentListPattern).
					WithArgs("team-1", "artifact", "res-1", 10, 10).
					WillReturnRows(commentFixtureRows().RowError(0, sql.ErrConnDone))
			},
			wantSub: "failed to iterate comments",
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runCommentListScenario(t, sc) })
	}
}

var commentActivityColumns = []string{
	"id", "team_id", "resource_type", "resource_id", "user_id", "content",
	"created_at", "updated_at", "resource_title", "project_id", "slug",
}

type commentRecentScenario struct {
	name     string
	queryErr error
	rows     func() *sqlmock.Rows
	wantSub  string
	check    func(t *testing.T, acts []models.CommentActivity)
}

func runCommentRecentScenario(t *testing.T, sc commentRecentScenario) {
	t.Helper()
	repo, mock, mockDB := newCommentMockRepo(t)
	defer closeMockDB(t, mockDB)

	exp := mock.ExpectQuery(`FROM comments c LEFT JOIN artifacts`).WithArgs("team-1", 20)
	if sc.queryErr != nil {
		exp.WillReturnError(sc.queryErr)
	} else {
		exp.WillReturnRows(sc.rows())
	}

	acts, err := repo.ListRecentByTeam(context.Background(), "team-1", 20)
	if sc.wantSub != "" {
		require.Error(t, err)
		assert.Contains(t, err.Error(), sc.wantSub)
	} else {
		require.NoError(t, err)
		require.NotNil(t, acts, "empty result must be a non-nil slice")
		sc.check(t, acts)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCommentRepository_ListRecentByTeam(t *testing.T) {
	scenarios := []commentRecentScenario{
		{
			name: "happy path maps nullable project_id and slug per row",
			rows: func() *sqlmock.Rows {
				return sqlmock.NewRows(commentActivityColumns).
					AddRow("comment-1", "team-1", "artifact", "res-1", "user-1", "hello",
						commentTestNow, commentTestNow, "Doc title", "proj-1", "doc-title").
					AddRow("comment-2", "team-1", "memory", "res-2", "user-2", "second",
						commentTestNow, commentTestNow, "memory excerpt", "proj-2", nil)
			},
			check: func(t *testing.T, acts []models.CommentActivity) {
				t.Helper()
				require.Len(t, acts, 2)
				assert.Equal(t, "comment-1", acts[0].ID)
				assert.Equal(t, "Doc title", acts[0].ResourceTitle)
				require.NotNil(t, acts[0].ProjectID)
				assert.Equal(t, "proj-1", *acts[0].ProjectID)
				require.NotNil(t, acts[0].Slug)
				assert.Equal(t, "doc-title", *acts[0].Slug)
				assert.Equal(t, "memory excerpt", acts[1].ResourceTitle)
				require.NotNil(t, acts[1].ProjectID)
				assert.Equal(t, "proj-2", *acts[1].ProjectID)
				assert.Nil(t, acts[1].Slug, "a memory has no slug: NULL must map to nil")
			},
		},
		{
			name: "no activity returns a non-nil empty slice",
			rows: func() *sqlmock.Rows { return sqlmock.NewRows(commentActivityColumns) },
			check: func(t *testing.T, acts []models.CommentActivity) {
				t.Helper()
				assert.Empty(t, acts)
			},
		},
		{name: "query error is wrapped", queryErr: sql.ErrConnDone, wantSub: "failed to list recent comments"},
		{
			name: "scan error is wrapped",
			rows: func() *sqlmock.Rows {
				return sqlmock.NewRows(commentActivityColumns).
					AddRow("comment-1", "team-1", "artifact", "res-1", "user-1", "hello",
						"not-a-time", commentTestNow, "Doc title", "proj-1", "doc-title")
			},
			wantSub: "failed to scan recent comment",
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runCommentRecentScenario(t, sc) })
	}
}

type commentDeleteScenario struct {
	name    string
	execErr error
	result  driver.Result
	wantIs  error
	wantSub string
}

func runCommentDeleteScenario(t *testing.T, sc commentDeleteScenario) {
	t.Helper()
	repo, mock, mockDB := newCommentMockRepo(t)
	defer closeMockDB(t, mockDB)

	exp := mock.ExpectExec(`DELETE FROM comments WHERE id = \$1 AND team_id = \$2`).
		WithArgs("comment-1", "team-1")
	if sc.execErr != nil {
		exp.WillReturnError(sc.execErr)
	} else {
		exp.WillReturnResult(sc.result)
	}

	err := repo.Delete(context.Background(), "team-1", "comment-1")
	assertCommentError(t, err, sc.wantIs, sc.wantSub)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCommentRepository_Delete(t *testing.T) {
	scenarios := []commentDeleteScenario{
		{name: "happy path deletes one row", result: sqlmock.NewResult(0, 1)},
		{
			name:   "zero rows affected maps to the not-found sentinel",
			result: sqlmock.NewResult(0, 0),
			wantIs: repositories.ErrCommentNotFound,
		},
		{
			name:    "exec error is wrapped",
			execErr: sql.ErrConnDone,
			wantIs:  sql.ErrConnDone,
			wantSub: "failed to delete comment",
		},
		{
			name:    "rows-affected error is wrapped",
			result:  sqlmock.NewErrorResult(errors.New("boom")),
			wantSub: "failed to read delete result",
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runCommentDeleteScenario(t, sc) })
	}
}

// commentBulkDeleteMethod describes a count-returning bulk delete
// (DeleteByResource / DeleteByUser); both share one scenario matrix.
type commentBulkDeleteMethod struct {
	name    string
	pattern string
	args    []driver.Value
	call    func(ctx context.Context, repo repositories.CommentRepository) (int64, error)
}

type commentBulkDeleteScenario struct {
	name    string
	execErr error
	result  driver.Result
	want    int64
	wantSub string
}

func runCommentBulkDeleteScenario(t *testing.T, m commentBulkDeleteMethod, sc commentBulkDeleteScenario) {
	t.Helper()
	repo, mock, mockDB := newCommentMockRepo(t)
	defer closeMockDB(t, mockDB)

	exp := mock.ExpectExec(m.pattern).WithArgs(m.args...)
	if sc.execErr != nil {
		exp.WillReturnError(sc.execErr)
	} else {
		exp.WillReturnResult(sc.result)
	}

	got, err := m.call(context.Background(), repo)
	assertCommentError(t, err, sc.execErr, sc.wantSub)
	assert.Equal(t, sc.want, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCommentRepository_BulkDeletes(t *testing.T) {
	methods := []commentBulkDeleteMethod{
		{
			name:    "DeleteByResource",
			pattern: `DELETE FROM comments WHERE team_id = \$1 AND resource_type = \$2 AND resource_id = \$3`,
			args:    []driver.Value{"team-1", "artifact", "res-1"},
			call: func(ctx context.Context, repo repositories.CommentRepository) (int64, error) {
				return repo.DeleteByResource(ctx, "team-1", "artifact", "res-1")
			},
		},
		{
			name:    "DeleteByUser",
			pattern: `DELETE FROM comments WHERE team_id = \$1 AND user_id = \$2`,
			args:    []driver.Value{"team-1", "user-1"},
			call: func(ctx context.Context, repo repositories.CommentRepository) (int64, error) {
				return repo.DeleteByUser(ctx, "team-1", "user-1")
			},
		},
	}
	scenarios := []commentBulkDeleteScenario{
		{name: "happy path returns the affected count", result: sqlmock.NewResult(0, 3), want: 3},
		{
			name:    "exec error is wrapped",
			execErr: sql.ErrConnDone,
			wantSub: "failed to delete comments for",
		},
		{
			name:    "rows-affected error is wrapped",
			result:  sqlmock.NewErrorResult(errors.New("boom")),
			wantSub: "failed to read delete result",
		},
	}
	for _, m := range methods {
		for _, sc := range scenarios {
			t.Run(m.name+"/"+sc.name, func(t *testing.T) { runCommentBulkDeleteScenario(t, m, sc) })
		}
	}
}

type commentExistsScenario struct {
	name         string
	resourceType string
	pattern      string
	exists       bool
	queryErr     error
	wantSub      string
}

func runCommentExistsScenario(t *testing.T, sc commentExistsScenario) {
	t.Helper()
	repo, mock, mockDB := newCommentMockRepo(t)
	defer closeMockDB(t, mockDB)

	if sc.pattern != "" {
		exp := mock.ExpectQuery(sc.pattern).WithArgs("res-1", "team-1")
		if sc.queryErr != nil {
			exp.WillReturnError(sc.queryErr)
		} else {
			exp.WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(sc.exists))
		}
	}

	got, err := repo.ResourceExists(context.Background(), "team-1", sc.resourceType, "res-1")
	assertCommentError(t, err, sc.queryErr, sc.wantSub)
	if sc.wantSub == "" {
		assert.Equal(t, sc.exists, got)
	} else {
		assert.False(t, got)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCommentRepository_ResourceExists(t *testing.T) {
	scenarios := []commentExistsScenario{
		{
			name:         "artifact existence check hits the artifacts table",
			resourceType: models.CommentResourceTypeArtifact,
			pattern:      `SELECT EXISTS\(SELECT 1 FROM artifacts WHERE id = \$1 AND team_id = \$2\)`,
			exists:       true,
		},
		{
			name:         "missing prompt reports false",
			resourceType: models.CommentResourceTypePrompt,
			pattern:      `SELECT EXISTS\(SELECT 1 FROM prompts WHERE id = \$1 AND team_id = \$2\)`,
			exists:       false,
		},
		{
			name:         "unknown resource type is rejected without touching the DB",
			resourceType: "widget",
			wantSub:      "unknown resource type",
		},
		{
			name:         "driver error is wrapped",
			resourceType: models.CommentResourceTypeMemory,
			pattern:      `SELECT EXISTS\(SELECT 1 FROM memories WHERE id = \$1 AND team_id = \$2\)`,
			queryErr:     sql.ErrConnDone,
			wantSub:      "failed to check resource existence",
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runCommentExistsScenario(t, sc) })
	}
}
