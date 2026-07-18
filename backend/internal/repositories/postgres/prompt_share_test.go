package postgres

// sqlmock unit tests for PromptShareRepository (prompt_share.go): argument
// wiring, row scanning, sentinel mapping, optimistic locking, and the
// access-email transaction. Notable pinned contracts: GetByToken/GetByPromptID
// apply NO is_active/expiry filtering (that is the service's job), and
// AddAccessEmails with an empty slice is a full no-op (no transaction, no
// delete of the existing list).

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

// Fixed timestamps for the prompt-share fixtures. The fixture row is
// deliberately inactive AND already expired so the Get tests prove the
// repository returns it unfiltered.
var (
	promptShareTestNow     = time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC)
	promptShareTestExpired = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
)

var promptShareTestColumns = []string{
	"id", "prompt_id", "share_token", "share_type", "created_by",
	"created_at", "expires_at", "is_active", "access_count", "version",
}

func newPromptShareMockRepo(t *testing.T) (repositories.PromptShareRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, mockDB := newSquirrelMockRepo(t)
	return NewPromptShareRepository(db), mock, mockDB
}

func promptShareInactiveExpiredRows() *sqlmock.Rows {
	return sqlmock.NewRows(promptShareTestColumns).AddRow(
		"share-1", "prompt-1", "tok-1", "restricted", "user-1",
		promptShareTestNow, promptShareTestExpired, false, 7, int64(3),
	)
}

// assertPromptShareError asserts the sentinel and/or message fragment a
// prompt-share call must return; both zero values mean "no error expected".
func assertPromptShareError(t *testing.T, err error, wantIs error, wantSub string) {
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

// assertInactiveExpiredShare pins the full row mapping — including IsActive
// false and a past ExpiresAt, proving the repo returned the row unfiltered.
func assertInactiveExpiredShare(t *testing.T, got *models.PromptShare) {
	t.Helper()
	require.NotNil(t, got)
	assert.Equal(t, "share-1", got.ID)
	assert.Equal(t, "prompt-1", got.PromptID)
	assert.Equal(t, "tok-1", got.ShareToken)
	assert.Equal(t, "restricted", got.ShareType)
	assert.Equal(t, "user-1", got.CreatedBy)
	assert.Equal(t, promptShareTestNow, got.CreatedAt)
	require.NotNil(t, got.ExpiresAt)
	assert.Equal(t, promptShareTestExpired, *got.ExpiresAt)
	assert.False(t, got.IsActive, "an inactive share must still be returned")
	assert.Equal(t, 7, got.AccessCount)
	assert.Equal(t, int64(3), got.Version)
}

type promptShareCreateScenario struct {
	name     string
	queryErr error
	wantIs   error
	wantSub  string
}

func runPromptShareCreateScenario(t *testing.T, sc promptShareCreateScenario) {
	t.Helper()
	repo, mock, mockDB := newPromptShareMockRepo(t)
	defer closeMockDB(t, mockDB)

	// The caller-supplied token is inserted verbatim — the repository mints
	// nothing and applies no transformation.
	exp := mock.ExpectQuery(`INSERT INTO prompt_shares`).
		WithArgs("prompt-1", "caller-minted-token", "public", "user-1",
			promptShareTestNow, nil, true, 0)
	if sc.queryErr != nil {
		exp.WillReturnError(sc.queryErr)
	} else {
		exp.WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).
			AddRow("share-1", promptShareTestNow))
	}

	share := &models.PromptShare{
		PromptID: "prompt-1", ShareToken: "caller-minted-token", ShareType: "public",
		CreatedBy: "user-1", CreatedAt: promptShareTestNow, IsActive: true,
	}
	err := repo.Create(context.Background(), share)

	assertPromptShareError(t, err, sc.wantIs, sc.wantSub)
	if sc.queryErr == nil {
		assert.Equal(t, "share-1", share.ID)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptShareRepository_Create(t *testing.T) {
	scenarios := []promptShareCreateScenario{
		{name: "happy path inserts the caller-supplied token verbatim"},
		{
			name:     "unique violation reports share-already-exists",
			queryErr: &pq.Error{Code: uniqueViolationCode},
			wantSub:  "share already exists for this prompt",
		},
		{
			name:     "driver error is wrapped",
			queryErr: sql.ErrConnDone,
			wantIs:   sql.ErrConnDone,
			wantSub:  "failed to create prompt share",
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runPromptShareCreateScenario(t, sc) })
	}
}

// promptShareGetMethod describes GetByToken / GetByPromptID, which share one
// scenario matrix. Both patterns are $-anchored to pin the contract that the
// WHERE clause holds a single predicate — no is_active or expiry filtering
// happens in the repository.
type promptShareGetMethod struct {
	name    string
	pattern string
	arg     driver.Value
	call    func(ctx context.Context, repo repositories.PromptShareRepository) (*models.PromptShare, error)
}

type promptShareGetScenario struct {
	name     string
	queryErr error
	wantIs   error
	wantSub  string
}

func runPromptShareGetScenario(t *testing.T, m promptShareGetMethod, sc promptShareGetScenario) {
	t.Helper()
	repo, mock, mockDB := newPromptShareMockRepo(t)
	defer closeMockDB(t, mockDB)

	exp := mock.ExpectQuery(m.pattern).WithArgs(m.arg)
	if sc.queryErr != nil {
		exp.WillReturnError(sc.queryErr)
	} else {
		exp.WillReturnRows(promptShareInactiveExpiredRows())
	}

	got, err := m.call(context.Background(), repo)
	if sc.queryErr == nil {
		require.NoError(t, err)
		assertInactiveExpiredShare(t, got)
	} else {
		assert.Nil(t, got)
		assertPromptShareError(t, err, sc.wantIs, sc.wantSub)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptShareRepository_Gets(t *testing.T) {
	methods := []promptShareGetMethod{
		{
			name:    "GetByToken",
			pattern: `FROM prompt_shares WHERE share_token = \$1$`,
			arg:     "tok-1",
			call: func(ctx context.Context, repo repositories.PromptShareRepository) (*models.PromptShare, error) {
				return repo.GetByToken(ctx, "tok-1")
			},
		},
		{
			name:    "GetByPromptID",
			pattern: `FROM prompt_shares WHERE prompt_id = \$1$`,
			arg:     "prompt-1",
			call: func(ctx context.Context, repo repositories.PromptShareRepository) (*models.PromptShare, error) {
				return repo.GetByPromptID(ctx, "prompt-1")
			},
		},
	}
	scenarios := []promptShareGetScenario{
		{name: "returns an inactive, expired share unfiltered"},
		{
			name:     "no rows maps to the not-found sentinel",
			queryErr: sql.ErrNoRows,
			wantIs:   repositories.ErrPromptShareNotFound,
		},
		{
			name:     "driver error is wrapped",
			queryErr: sql.ErrConnDone,
			wantIs:   sql.ErrConnDone,
			wantSub:  "failed to get share",
		},
	}
	for _, m := range methods {
		for _, sc := range scenarios {
			t.Run(m.name+"/"+sc.name, func(t *testing.T) { runPromptShareGetScenario(t, m, sc) })
		}
	}
}

// promptShareUpdatePattern pins the optimistic-locking contract: the version
// bumps in SQL and the WHERE clause matches both id and the caller's version.
const promptShareUpdatePattern = `UPDATE prompt_shares SET .+ version = version \+ 1 ` +
	`WHERE id = \$4 AND version = \$5 RETURNING version`

type promptShareUpdateScenario struct {
	name        string
	queryErr    error
	wantIs      error
	wantSub     string
	wantVersion int64
}

func runPromptShareUpdateScenario(t *testing.T, sc promptShareUpdateScenario) {
	t.Helper()
	repo, mock, mockDB := newPromptShareMockRepo(t)
	defer closeMockDB(t, mockDB)

	exp := mock.ExpectQuery(promptShareUpdatePattern).
		WithArgs("restricted", nil, false, "share-1", int64(3))
	if sc.queryErr != nil {
		exp.WillReturnError(sc.queryErr)
	} else {
		exp.WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(int64(4)))
	}

	share := &models.PromptShare{ID: "share-1", ShareType: "restricted", IsActive: false, Version: 3}
	err := repo.Update(context.Background(), share)

	assertPromptShareError(t, err, sc.wantIs, sc.wantSub)
	assert.Equal(t, sc.wantVersion, share.Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptShareRepository_Update(t *testing.T) {
	scenarios := []promptShareUpdateScenario{
		{name: "happy path writes the bumped version back", wantVersion: 4},
		{
			name:        "version mismatch (no rows) reports not-found-or-mismatch",
			queryErr:    sql.ErrNoRows,
			wantSub:     "share not found or version mismatch",
			wantVersion: 3,
		},
		{
			name:        "driver error is wrapped",
			queryErr:    sql.ErrConnDone,
			wantIs:      sql.ErrConnDone,
			wantSub:     "failed to update prompt share",
			wantVersion: 3,
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runPromptShareUpdateScenario(t, sc) })
	}
}

type promptShareDeleteScenario struct {
	name    string
	execErr error
	result  driver.Result
	wantIs  error
	wantSub string
}

func runPromptShareDeleteScenario(t *testing.T, sc promptShareDeleteScenario) {
	t.Helper()
	repo, mock, mockDB := newPromptShareMockRepo(t)
	defer closeMockDB(t, mockDB)

	exp := mock.ExpectExec(`DELETE FROM prompt_shares WHERE id = \$1`).WithArgs("share-1")
	if sc.execErr != nil {
		exp.WillReturnError(sc.execErr)
	} else {
		exp.WillReturnResult(sc.result)
	}

	err := repo.Delete(context.Background(), "share-1")
	assertPromptShareError(t, err, sc.wantIs, sc.wantSub)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptShareRepository_Delete(t *testing.T) {
	scenarios := []promptShareDeleteScenario{
		{name: "happy path deletes one row", result: sqlmock.NewResult(0, 1)},
		{
			name:   "zero rows affected maps to the not-found sentinel",
			result: sqlmock.NewResult(0, 0),
			wantIs: repositories.ErrPromptShareNotFound,
		},
		{
			name:    "exec error is wrapped",
			execErr: sql.ErrConnDone,
			wantIs:  sql.ErrConnDone,
			wantSub: "failed to delete prompt share",
		},
		{
			name:    "rows-affected error is wrapped",
			result:  sqlmock.NewErrorResult(errors.New("boom")),
			wantSub: "failed to check rows affected",
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runPromptShareDeleteScenario(t, sc) })
	}
}

// promptShareExecMethod describes a plain fire-and-forget exec
// (IncrementAccessCount / RemoveAccessEmail); both share one scenario matrix.
type promptShareExecMethod struct {
	name    string
	pattern string
	args    []driver.Value
	wrapSub string
	call    func(ctx context.Context, repo repositories.PromptShareRepository) error
}

type promptShareExecScenario struct {
	name    string
	execErr error
}

func runPromptShareExecScenario(t *testing.T, m promptShareExecMethod, sc promptShareExecScenario) {
	t.Helper()
	repo, mock, mockDB := newPromptShareMockRepo(t)
	defer closeMockDB(t, mockDB)

	exp := mock.ExpectExec(m.pattern).WithArgs(m.args...)
	if sc.execErr != nil {
		exp.WillReturnError(sc.execErr)
	} else {
		exp.WillReturnResult(sqlmock.NewResult(0, 1))
	}

	err := m.call(context.Background(), repo)
	wantSub := ""
	if sc.execErr != nil {
		wantSub = m.wrapSub
	}
	assertPromptShareError(t, err, sc.execErr, wantSub)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptShareRepository_IncrementAndRemoveEmail(t *testing.T) {
	methods := []promptShareExecMethod{
		{
			name:    "IncrementAccessCount",
			pattern: `UPDATE prompt_shares SET access_count = access_count \+ 1 WHERE id = \$1`,
			args:    []driver.Value{"share-1"},
			wrapSub: "failed to increment access count",
			call: func(ctx context.Context, repo repositories.PromptShareRepository) error {
				return repo.IncrementAccessCount(ctx, "share-1")
			},
		},
		{
			name:    "RemoveAccessEmail",
			pattern: `DELETE FROM prompt_share_access WHERE share_id = \$1 AND email = \$2`,
			args:    []driver.Value{"share-1", "a@example.com"},
			wrapSub: "failed to remove access email",
			call: func(ctx context.Context, repo repositories.PromptShareRepository) error {
				return repo.RemoveAccessEmail(ctx, "share-1", "a@example.com")
			},
		},
	}
	scenarios := []promptShareExecScenario{
		{name: "happy path"},
		{name: "exec error is wrapped", execErr: sql.ErrConnDone},
	}
	for _, m := range methods {
		for _, sc := range scenarios {
			t.Run(m.name+"/"+sc.name, func(t *testing.T) { runPromptShareExecScenario(t, m, sc) })
		}
	}
}

const (
	promptShareDeleteAccessPattern = `DELETE FROM prompt_share_access WHERE share_id = \$1$`
	promptShareInsertAccessPattern = `INSERT INTO prompt_share_access .+ ON CONFLICT \(share_id, email\) DO NOTHING`
)

func expectAccessDelete(mock sqlmock.Sqlmock) {
	mock.ExpectExec(promptShareDeleteAccessPattern).
		WithArgs("share-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectAccessInsert(mock sqlmock.Sqlmock, email string) {
	mock.ExpectExec(promptShareInsertAccessPattern).
		WithArgs("share-1", email).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

type promptShareEmailsScenario struct {
	name    string
	emails  []string
	setup   func(mock sqlmock.Sqlmock)
	wantIs  error
	wantSub string
}

func runPromptShareEmailsScenario(t *testing.T, sc promptShareEmailsScenario) {
	t.Helper()
	repo, mock, mockDB := newPromptShareMockRepo(t)
	defer closeMockDB(t, mockDB)
	sc.setup(mock)

	err := repo.AddAccessEmails(context.Background(), "share-1", sc.emails)
	assertPromptShareError(t, err, sc.wantIs, sc.wantSub)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptShareRepository_AddAccessEmails(t *testing.T) {
	scenarios := []promptShareEmailsScenario{
		{
			// Contract: the empty slice returns before BeginTx, so the
			// existing access list is NOT deleted. Zero mock expectations
			// prove no DB interaction at all.
			name:   "empty slice is a full no-op (keeps the existing list)",
			emails: nil,
			setup:  func(sqlmock.Sqlmock) {},
		},
		{
			name:   "happy path replaces the list in one transaction",
			emails: []string{"a@example.com", "b@example.com"},
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				expectAccessDelete(mock)
				expectAccessInsert(mock, "a@example.com")
				expectAccessInsert(mock, "b@example.com")
				mock.ExpectCommit()
			},
		},
		{
			name:   "begin error is wrapped",
			emails: []string{"a@example.com"},
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin().WillReturnError(sql.ErrConnDone)
			},
			wantIs:  sql.ErrConnDone,
			wantSub: "failed to begin transaction",
		},
		{
			name:   "delete error rolls the transaction back",
			emails: []string{"a@example.com"},
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(promptShareDeleteAccessPattern).
					WithArgs("share-1").
					WillReturnError(sql.ErrConnDone)
				mock.ExpectRollback()
			},
			wantIs:  sql.ErrConnDone,
			wantSub: "failed to delete existing access entries",
		},
		{
			name:   "insert error rolls the transaction back",
			emails: []string{"a@example.com", "b@example.com"},
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				expectAccessDelete(mock)
				mock.ExpectExec(promptShareInsertAccessPattern).
					WithArgs("share-1", "a@example.com").
					WillReturnError(sql.ErrConnDone)
				mock.ExpectRollback()
			},
			wantIs:  sql.ErrConnDone,
			wantSub: "failed to add access email",
		},
		{
			name:   "commit error is wrapped",
			emails: []string{"a@example.com"},
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				expectAccessDelete(mock)
				expectAccessInsert(mock, "a@example.com")
				mock.ExpectCommit().WillReturnError(sql.ErrConnDone)
			},
			wantIs:  sql.ErrConnDone,
			wantSub: "failed to commit transaction",
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runPromptShareEmailsScenario(t, sc) })
	}
}

type promptShareEmailListScenario struct {
	name     string
	queryErr error
	rows     func() *sqlmock.Rows
	want     []string
	wantSub  string
}

func runPromptShareEmailListScenario(t *testing.T, sc promptShareEmailListScenario) {
	t.Helper()
	repo, mock, mockDB := newPromptShareMockRepo(t)
	defer closeMockDB(t, mockDB)

	exp := mock.ExpectQuery(`SELECT email FROM prompt_share_access WHERE share_id = \$1 ORDER BY email`).
		WithArgs("share-1")
	if sc.queryErr != nil {
		exp.WillReturnError(sc.queryErr)
	} else {
		exp.WillReturnRows(sc.rows())
	}

	got, err := repo.GetAccessEmails(context.Background(), "share-1")
	if sc.wantSub != "" {
		require.Error(t, err)
		assert.Contains(t, err.Error(), sc.wantSub)
		assert.Nil(t, got)
	} else {
		require.NoError(t, err)
		assert.Equal(t, sc.want, got)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptShareRepository_GetAccessEmails(t *testing.T) {
	scenarios := []promptShareEmailListScenario{
		{
			name: "happy path preserves the ordered rows",
			rows: func() *sqlmock.Rows {
				return sqlmock.NewRows([]string{"email"}).
					AddRow("a@example.com").
					AddRow("b@example.com")
			},
			want: []string{"a@example.com", "b@example.com"},
		},
		{
			name: "no entries returns empty",
			rows: func() *sqlmock.Rows { return sqlmock.NewRows([]string{"email"}) },
		},
		{name: "query error is wrapped", queryErr: sql.ErrConnDone, wantSub: "failed to get access emails"},
		{
			name: "row iteration error is wrapped",
			rows: func() *sqlmock.Rows {
				return sqlmock.NewRows([]string{"email"}).
					AddRow("a@example.com").
					RowError(0, sql.ErrConnDone)
			},
			wantSub: "error iterating over emails",
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runPromptShareEmailListScenario(t, sc) })
	}
}

type promptShareHasAccessScenario struct {
	name     string
	exists   bool
	queryErr error
	wantSub  string
}

func runPromptShareHasAccessScenario(t *testing.T, sc promptShareHasAccessScenario) {
	t.Helper()
	repo, mock, mockDB := newPromptShareMockRepo(t)
	defer closeMockDB(t, mockDB)

	exp := mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM prompt_share_access WHERE share_id = \$1 AND email = \$2\)`).
		WithArgs("share-1", "a@example.com")
	if sc.queryErr != nil {
		exp.WillReturnError(sc.queryErr)
	} else {
		exp.WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(sc.exists))
	}

	got, err := repo.HasAccess(context.Background(), "share-1", "a@example.com")
	assertPromptShareError(t, err, sc.queryErr, sc.wantSub)
	if sc.wantSub == "" {
		assert.Equal(t, sc.exists, got)
	} else {
		assert.False(t, got)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPromptShareRepository_HasAccess(t *testing.T) {
	scenarios := []promptShareHasAccessScenario{
		{name: "email with access reports true", exists: true},
		{name: "email without access reports false", exists: false},
		{name: "driver error is wrapped", queryErr: sql.ErrConnDone, wantSub: "failed to check access"},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runPromptShareHasAccessScenario(t, sc) })
	}
}
