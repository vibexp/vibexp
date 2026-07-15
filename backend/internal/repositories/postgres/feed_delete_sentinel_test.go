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

// FeedItemRepository.Delete's contract after epic #220 decision D3.
//
// The repository used to decide authorization itself: its check returned both
// `item_exists` AND `authorized`, and it owned an ErrFeedItemForbidden sentinel.
// That decision now lives in FeedItemService via the authz matrix (the poster may
// delete their own; Owner/Admin may delete anyone's as moderation), so:
//
//   - the check is existence-only — it survives because the caller still needs to
//     tell a missing item (404) apart from a denial;
//   - the DELETE carries TENANCY only;
//   - ErrFeedItemForbidden is retired. Nothing produces it, so nothing may match
//     it — a stale sentinel match is what silently 500s a denial (see PR #233).
//
// The role decision is asserted where it now lives: internal/services (feed RBAC
// tests) and the handler 403 mapping.

const (
	feedDeleteUserID = "11111111-1111-1111-1111-111111111111"
	feedDeleteTeamID = "22222222-2222-2222-2222-222222222222"
	feedDeleteItemID = "33333333-3333-3333-3333-333333333333"
)

// existsQueryRegex matches the existence SELECT used by Delete. A regex (not a
// literal) tolerates whitespace formatting changes while still verifying the
// right query runs; anchored on `feed_items` so it cannot match another
// repository's existence probe.
var existsQueryRegex = regexp.MustCompile(`SELECT[\s\S]+EXISTS[\s\S]+feed_items`)

// deleteQueryRegex matches the DELETE issued after the existence check.
var deleteQueryRegex = regexp.MustCompile(`DELETE\s+FROM\s+feed_items`)

func newFeedItemRepoForDelete(t *testing.T) (repositories.FeedItemRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close db: %v", closeErr)
		}
	})
	return postgres.NewFeedItemRepository(&database.DB{DB: db}), mock
}

// TestFeedItemRepository_Delete_NotFound_ReturnsSentinel: an absent item produces
// ErrFeedItemNotFound, and the DELETE is never attempted.
func TestFeedItemRepository_Delete_NotFound_ReturnsSentinel(t *testing.T) {
	repo, mock := newFeedItemRepoForDelete(t)

	// One column now, not two — `authorized` went with D3, and so did its arg.
	mock.ExpectQuery(existsQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	err := repo.Delete(context.Background(), feedDeleteUserID, feedDeleteTeamID, feedDeleteItemID)

	assert.True(t, errors.Is(err, repositories.ErrFeedItemNotFound),
		"absent item must return ErrFeedItemNotFound, got %v", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestFeedItemRepository_Delete_ExistingItem_Deletes: the repository no longer
// asks who the caller is beyond team membership — it deletes. The role decision
// happened in the service before this point.
func TestFeedItemRepository_Delete_ExistingItem_Deletes(t *testing.T) {
	repo, mock := newFeedItemRepoForDelete(t)

	mock.ExpectQuery(existsQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(deleteQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID, feedDeleteUserID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Delete(context.Background(), feedDeleteUserID, feedDeleteTeamID, feedDeleteItemID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestFeedItemRepository_Delete_DBErrorOnCheck_NotSentinel: a database failure
// must not masquerade as not-found.
func TestFeedItemRepository_Delete_DBErrorOnCheck_NotSentinel(t *testing.T) {
	repo, mock := newFeedItemRepoForDelete(t)

	mock.ExpectQuery(existsQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID).
		WillReturnError(errors.New("connection refused"))

	err := repo.Delete(context.Background(), feedDeleteUserID, feedDeleteTeamID, feedDeleteItemID)

	require.Error(t, err)
	assert.False(t, errors.Is(err, repositories.ErrFeedItemNotFound),
		"a DB error must not be reported as not-found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestFeedItemRepository_Delete_DBErrorOnDelete_NotSentinel: same for the DELETE.
func TestFeedItemRepository_Delete_DBErrorOnDelete_NotSentinel(t *testing.T) {
	repo, mock := newFeedItemRepoForDelete(t)

	mock.ExpectQuery(existsQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(deleteQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID, feedDeleteUserID).
		WillReturnError(errors.New("connection refused"))

	err := repo.Delete(context.Background(), feedDeleteUserID, feedDeleteTeamID, feedDeleteItemID)

	require.Error(t, err)
	assert.False(t, errors.Is(err, repositories.ErrFeedItemNotFound))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestFeedItemRepository_Delete_ZeroRowsAffected_ReturnsNotFound covers the race
// the existence check cannot close: the row vanished (or the caller's membership
// was revoked) between the SELECT and the DELETE. The caller gets a deterministic
// not-found rather than a silent 204 no-op.
func TestFeedItemRepository_Delete_ZeroRowsAffected_ReturnsNotFound(t *testing.T) {
	repo, mock := newFeedItemRepoForDelete(t)

	mock.ExpectQuery(existsQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(deleteQueryRegex.String()).
		WithArgs(feedDeleteItemID, feedDeleteTeamID, feedDeleteUserID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.Delete(context.Background(), feedDeleteUserID, feedDeleteTeamID, feedDeleteItemID)

	assert.True(t, errors.Is(err, repositories.ErrFeedItemNotFound),
		"a concurrent change must surface as not-found, got %v", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
