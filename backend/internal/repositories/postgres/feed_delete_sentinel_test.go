package postgres_test

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/postgres"
)

const (
	feedDeleteUserID = "11111111-1111-1111-1111-111111111111"
	feedDeleteTeamID = "22222222-2222-2222-2222-222222222222"
	feedDeleteItemID = "33333333-3333-3333-3333-333333333333"
)

// checkQueryRegex matches the existence + authorization SELECT used by Delete.
// Using a regex (not a literal) lets the test tolerate whitespace formatting
// changes inside the query while still verifying the right query is run.
// The regex anchors on `feed_items` to make sure we don't accidentally match
// some other repository's existence/authorization probe.
var checkQueryRegex = regexp.MustCompile(`SELECT[\s\S]+EXISTS[\s\S]+feed_items[\s\S]+item_exists[\s\S]+authorized`)

// deleteQueryRegex matches the actual DELETE statement issued after the
// existence/authorization check. Anchored on `feed_items` for the same
// reason as checkQueryRegex above.
var deleteQueryRegex = regexp.MustCompile(`DELETE\s+FROM\s+feed_items`)

// TestFeedItemRepository_Delete_NotFound_ReturnsSentinel verifies that an absent
// item produces ErrFeedItemNotFound (404), not ErrFeedItemForbidden.
func TestFeedItemRepository_Delete_NotFound_ReturnsSentinel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close db: %v", closeErr)
		}
	}()

	repo := postgres.NewFeedItemRepository(&database.DB{DB: db})

	rows := sqlmock.NewRows([]string{"item_exists", "authorized"}).AddRow(false, false)
	mock.ExpectQuery(checkQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID, feedDeleteUserID).
		WillReturnRows(rows)

	err = repo.Delete(context.Background(), feedDeleteUserID, feedDeleteTeamID, feedDeleteItemID)

	require.Error(t, err)
	assert.True(t, errors.Is(err, repositories.ErrFeedItemNotFound),
		"Delete on absent item must wrap ErrFeedItemNotFound; got: %v", err)
	assert.False(t, errors.Is(err, repositories.ErrFeedItemForbidden),
		"absent item must NOT be reported as forbidden")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestFeedItemRepository_Delete_ExistingButUnauthorized_ReturnsForbiddenSentinel
// verifies that an existing item with a non-authorized member produces
// ErrFeedItemForbidden (403), not ErrFeedItemNotFound.
func TestFeedItemRepository_Delete_ExistingButUnauthorized_ReturnsForbiddenSentinel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close db: %v", closeErr)
		}
	}()

	repo := postgres.NewFeedItemRepository(&database.DB{DB: db})

	rows := sqlmock.NewRows([]string{"item_exists", "authorized"}).AddRow(true, false)
	mock.ExpectQuery(checkQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID, feedDeleteUserID).
		WillReturnRows(rows)

	err = repo.Delete(context.Background(), feedDeleteUserID, feedDeleteTeamID, feedDeleteItemID)

	require.Error(t, err)
	assert.True(t, errors.Is(err, repositories.ErrFeedItemForbidden),
		"Delete by non-authorized member must wrap ErrFeedItemForbidden; got: %v", err)
	assert.False(t, errors.Is(err, repositories.ErrFeedItemNotFound),
		"existing-but-unauthorized must NOT be reported as not-found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestFeedItemRepository_Delete_Authorized_Succeeds verifies that an authorized
// caller (poster or owner/admin) deletes the item with no error.
func TestFeedItemRepository_Delete_Authorized_Succeeds(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close db: %v", closeErr)
		}
	}()

	repo := postgres.NewFeedItemRepository(&database.DB{DB: db})

	rows := sqlmock.NewRows([]string{"item_exists", "authorized"}).AddRow(true, true)
	mock.ExpectQuery(checkQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID, feedDeleteUserID).
		WillReturnRows(rows)
	mock.ExpectExec(deleteQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID, feedDeleteUserID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.Delete(context.Background(), feedDeleteUserID, feedDeleteTeamID, feedDeleteItemID)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestFeedItemRepository_Delete_DBErrorOnCheck_NotSentinel verifies that a real
// DB error on the existence/authorization SELECT is NOT mapped to either of the
// two sentinels. The caller must surface a 500, not 403/404.
func TestFeedItemRepository_Delete_DBErrorOnCheck_NotSentinel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close db: %v", closeErr)
		}
	}()

	repo := postgres.NewFeedItemRepository(&database.DB{DB: db})

	mock.ExpectQuery(checkQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID, feedDeleteUserID).
		WillReturnError(errors.New("connection refused"))

	err = repo.Delete(context.Background(), feedDeleteUserID, feedDeleteTeamID, feedDeleteItemID)

	require.Error(t, err)
	assert.False(t, errors.Is(err, repositories.ErrFeedItemNotFound),
		"transient DB errors must NOT be mapped to ErrFeedItemNotFound")
	assert.False(t, errors.Is(err, repositories.ErrFeedItemForbidden),
		"transient DB errors must NOT be mapped to ErrFeedItemForbidden")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestFeedItemRepository_Delete_DBErrorOnDelete_NotSentinel verifies that a DB
// error on the actual DELETE statement (after authorization passed) propagates
// without being mapped to a sentinel.
func TestFeedItemRepository_Delete_DBErrorOnDelete_NotSentinel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close db: %v", closeErr)
		}
	}()

	repo := postgres.NewFeedItemRepository(&database.DB{DB: db})

	rows := sqlmock.NewRows([]string{"item_exists", "authorized"}).AddRow(true, true)
	mock.ExpectQuery(checkQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID, feedDeleteUserID).
		WillReturnRows(rows)
	mock.ExpectExec(deleteQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID, feedDeleteUserID).
		WillReturnError(errors.New("query timeout"))

	err = repo.Delete(context.Background(), feedDeleteUserID, feedDeleteTeamID, feedDeleteItemID)

	require.Error(t, err)
	assert.False(t, errors.Is(err, repositories.ErrFeedItemNotFound),
		"DELETE-statement errors must NOT be mapped to ErrFeedItemNotFound")
	assert.False(t, errors.Is(err, repositories.ErrFeedItemForbidden),
		"DELETE-statement errors must NOT be mapped to ErrFeedItemForbidden")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestFeedItemRepository_Delete_AuthorizedButZeroRowsAffected_ReturnsNotFound
// verifies the SELECT→DELETE race window: the existence/authorization SELECT
// passes, but by the time the DELETE runs the row is gone (concurrent
// deletion) or the caller's authorization was revoked (membership/role change),
// so the auth-predicated DELETE affects 0 rows. Returning nil here would
// produce a silent 204 no-op; the repository must instead surface
// ErrFeedItemNotFound so the handler returns a deterministic 404.
func TestFeedItemRepository_Delete_AuthorizedButZeroRowsAffected_ReturnsNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close db: %v", closeErr)
		}
	}()

	repo := postgres.NewFeedItemRepository(&database.DB{DB: db})

	rows := sqlmock.NewRows([]string{"item_exists", "authorized"}).AddRow(true, true)
	mock.ExpectQuery(checkQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID, feedDeleteUserID).
		WillReturnRows(rows)
	mock.ExpectExec(deleteQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID, feedDeleteUserID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.Delete(context.Background(), feedDeleteUserID, feedDeleteTeamID, feedDeleteItemID)

	require.Error(t, err)
	assert.True(t, errors.Is(err, repositories.ErrFeedItemNotFound),
		"zero rows affected after authorization passed must wrap ErrFeedItemNotFound; got: %v", err)
	assert.False(t, errors.Is(err, repositories.ErrFeedItemForbidden),
		"zero rows affected must NOT be reported as forbidden")
	assert.NoError(t, mock.ExpectationsWereMet())
}
