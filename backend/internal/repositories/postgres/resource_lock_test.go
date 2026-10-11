package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

func TestResourceLockScan_Value(t *testing.T) {
	at := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)

	t.Run("a NULL locked_by is unlocked, whatever the other columns hold", func(t *testing.T) {
		// The state a row is left in when the locker's account is deleted:
		// ON DELETE SET NULL clears locked_by and leaves the rest behind.
		ghost := resourceLockScan{
			lockedAt: sql.NullTime{Time: at, Valid: true},
			comment:  sql.NullString{String: "freeze for release", Valid: true},
		}
		assert.Nil(t, ghost.value())
		assert.Nil(t, (&resourceLockScan{}).value())
	})

	t.Run("a set locked_by yields the full lock", func(t *testing.T) {
		scan := resourceLockScan{
			lockedBy:   sql.NullString{String: "user-1", Valid: true},
			lockedAt:   sql.NullTime{Time: at, Valid: true},
			comment:    sql.NullString{String: "freeze for release", Valid: true},
			lockerName: sql.NullString{String: "Ada", Valid: true},
		}
		lock := scan.value()
		require.NotNil(t, lock)
		assert.Equal(t, "user-1", lock.LockedBy)
		assert.Equal(t, "Ada", lock.LockedByName)
		assert.Equal(t, "freeze for release", lock.Comment)
		assert.Equal(t, at, lock.LockedAt)
	})
}

func TestApplyLockedFilter(t *testing.T) {
	locked, unlocked := true, false
	base := squirrel.And{squirrel.Eq{"p.team_id": "team-1"}}

	t.Run("nil adds no predicate", func(t *testing.T) {
		sqlText, args, err := applyLockedFilter(base, "p", nil).ToSql()
		require.NoError(t, err)
		assert.Equal(t, "(p.team_id = ?)", sqlText)
		assert.Equal(t, []any{"team-1"}, args)
	})

	t.Run("true keeps locked rows", func(t *testing.T) {
		sqlText, args, err := applyLockedFilter(base, "p", &locked).ToSql()
		require.NoError(t, err)
		assert.Equal(t, "(p.team_id = ? AND p.locked_by IS NOT NULL)", sqlText)
		assert.Len(t, args, 1, "the filter binds no argument")
	})

	t.Run("false keeps unlocked rows", func(t *testing.T) {
		sqlText, _, err := applyLockedFilter(base, "p", &unlocked).ToSql()
		require.NoError(t, err)
		assert.Equal(t, "(p.team_id = ? AND p.locked_by IS NULL)", sqlText)
	})
}

// lockWriter is the lock half of the three resource repositories.
type lockWriter interface {
	SetLock(ctx context.Context, teamID, resourceID, lockerID, comment string) error
	ClearLock(ctx context.Context, teamID, resourceID string) error
}

// lockWriterCase names one repository and the table and not-found sentinel its
// lock statements must use.
type lockWriterCase struct {
	table    string
	notFound error
	build    func(db *database.DB) lockWriter
}

func lockWriterCases() []lockWriterCase {
	return []lockWriterCase{
		{"prompts", repositories.ErrPromptNotFound, func(db *database.DB) lockWriter {
			return NewPromptRepository(db).(*PromptRepository)
		}},
		{"blueprints", repositories.ErrBlueprintNotFound, func(db *database.DB) lockWriter {
			return NewBlueprintRepository(db).(*BlueprintRepository)
		}},
		{"artifacts", repositories.ErrArtifactNotFound, func(db *database.DB) lockWriter {
			return NewArtifactRepository(db).(*ArtifactRepository)
		}},
	}
}

// The lock statements are pinned whole: they must be scoped by id AND team,
// conditional on the current lock state, and must not write updated_at or
// version.
func setLockPattern(table string) string {
	return `^\s*UPDATE ` + table + `\s+SET locked_by = \$3, locked_at = now\(\), lock_comment = \$4` +
		`\s+WHERE id = \$1 AND team_id = \$2 AND locked_by IS NULL$`
}

func clearLockPattern(table string) string {
	return `^\s*UPDATE ` + table + `\s+SET locked_by = NULL, locked_at = NULL, lock_comment = NULL` +
		`\s+WHERE id = \$1 AND team_id = \$2 AND locked_by IS NOT NULL$`
}

func existsPattern(table string) string {
	return `SELECT EXISTS\(SELECT 1 FROM ` + table + ` \w WHERE \w\.id = \$1 AND \w\.team_id = \$2\)`
}

func TestResourceRepositories_SetLock(t *testing.T) {
	ctx := context.Background()
	errDB := errors.New("connection reset")

	for _, tc := range lockWriterCases() {
		t.Run(tc.table, func(t *testing.T) {
			newRepo := func(t *testing.T) (lockWriter, sqlmock.Sqlmock) {
				t.Helper()
				mockDB, mock, err := sqlmock.New()
				require.NoError(t, err)
				t.Cleanup(func() { closeMockDB(t, mockDB) })
				return tc.build(&database.DB{DB: mockDB}), mock
			}
			expectSet := func(mock sqlmock.Sqlmock) *sqlmock.ExpectedExec {
				return mock.ExpectExec(setLockPattern(tc.table)).
					WithArgs("res-1", "team-1", "user-1", "freeze")
			}

			t.Run("locks an unlocked resource with one statement", func(t *testing.T) {
				repo, mock := newRepo(t)
				expectSet(mock).WillReturnResult(sqlmock.NewResult(0, 1))

				require.NoError(t, repo.SetLock(ctx, "team-1", "res-1", "user-1", "freeze"))
				assert.NoError(t, mock.ExpectationsWereMet())
			})

			t.Run("an existing resource that matched no row is already locked", func(t *testing.T) {
				repo, mock := newRepo(t)
				expectSet(mock).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectQuery(existsPattern(tc.table)).WithArgs("res-1", "team-1").
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

				err := repo.SetLock(ctx, "team-1", "res-1", "user-1", "freeze")
				assert.ErrorIs(t, err, repositories.ErrResourceAlreadyLocked)
				assert.NoError(t, mock.ExpectationsWereMet())
			})

			t.Run("a resource outside the team is not found", func(t *testing.T) {
				repo, mock := newRepo(t)
				expectSet(mock).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectQuery(existsPattern(tc.table)).WithArgs("res-1", "team-1").
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

				err := repo.SetLock(ctx, "team-1", "res-1", "user-1", "freeze")
				assert.ErrorIs(t, err, tc.notFound)
				assert.NotErrorIs(t, err, repositories.ErrResourceAlreadyLocked)
			})

			t.Run("a failed write is wrapped", func(t *testing.T) {
				repo, mock := newRepo(t)
				expectSet(mock).WillReturnError(errDB)

				err := repo.SetLock(ctx, "team-1", "res-1", "user-1", "freeze")
				assert.ErrorIs(t, err, errDB)
				assert.ErrorContains(t, err, "failed to lock resource")
			})

			t.Run("an unreadable row count is wrapped", func(t *testing.T) {
				repo, mock := newRepo(t)
				expectSet(mock).WillReturnResult(sqlmock.NewErrorResult(errDB))

				err := repo.SetLock(ctx, "team-1", "res-1", "user-1", "freeze")
				assert.ErrorIs(t, err, errDB)
				assert.ErrorContains(t, err, "failed to get rows affected")
			})

			t.Run("a failed existence check is surfaced, not reported as a lock state", func(t *testing.T) {
				repo, mock := newRepo(t)
				expectSet(mock).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectQuery(existsPattern(tc.table)).WillReturnError(errDB)

				err := repo.SetLock(ctx, "team-1", "res-1", "user-1", "freeze")
				assert.ErrorIs(t, err, errDB)
				assert.NotErrorIs(t, err, repositories.ErrResourceAlreadyLocked)
			})
		})
	}
}

func TestResourceRepositories_ClearLock(t *testing.T) {
	ctx := context.Background()
	errDB := errors.New("connection reset")

	for _, tc := range lockWriterCases() {
		t.Run(tc.table, func(t *testing.T) {
			newRepo := func(t *testing.T) (lockWriter, sqlmock.Sqlmock) {
				t.Helper()
				mockDB, mock, err := sqlmock.New()
				require.NoError(t, err)
				t.Cleanup(func() { closeMockDB(t, mockDB) })
				return tc.build(&database.DB{DB: mockDB}), mock
			}
			expectClear := func(mock sqlmock.Sqlmock) *sqlmock.ExpectedExec {
				return mock.ExpectExec(clearLockPattern(tc.table)).WithArgs("res-1", "team-1")
			}

			t.Run("unlocks a locked resource with one statement", func(t *testing.T) {
				repo, mock := newRepo(t)
				expectClear(mock).WillReturnResult(sqlmock.NewResult(0, 1))

				require.NoError(t, repo.ClearLock(ctx, "team-1", "res-1"))
				assert.NoError(t, mock.ExpectationsWereMet())
			})

			t.Run("an existing resource that matched no row is not locked", func(t *testing.T) {
				repo, mock := newRepo(t)
				expectClear(mock).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectQuery(existsPattern(tc.table)).WithArgs("res-1", "team-1").
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

				err := repo.ClearLock(ctx, "team-1", "res-1")
				assert.ErrorIs(t, err, repositories.ErrResourceNotLocked)
				assert.NoError(t, mock.ExpectationsWereMet())
			})

			t.Run("a resource outside the team is not found", func(t *testing.T) {
				repo, mock := newRepo(t)
				expectClear(mock).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectQuery(existsPattern(tc.table)).WithArgs("res-1", "team-1").
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

				err := repo.ClearLock(ctx, "team-1", "res-1")
				assert.ErrorIs(t, err, tc.notFound)
				assert.NotErrorIs(t, err, repositories.ErrResourceNotLocked)
			})

			t.Run("a failed write is wrapped", func(t *testing.T) {
				repo, mock := newRepo(t)
				expectClear(mock).WillReturnError(errDB)

				err := repo.ClearLock(ctx, "team-1", "res-1")
				assert.ErrorIs(t, err, errDB)
				assert.ErrorContains(t, err, "failed to unlock resource")
			})
		})
	}
}
