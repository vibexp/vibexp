//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// A resource write racing a status delete (#1332). Validation happens before
// the write and holds no lock, so a write can commit a custom status after the
// delete's reassignment has already run. These tests hold the delete's
// transaction open by hand to pin both orders: either the write is committed
// before the delete starts and the delete moves it, or ConfirmSlug waits the
// delete out, answers false, and ReassignResource moves the straggler.

// orphanedStatusCount counts the team's rows in table whose status is neither
// a system status of resourceType nor one of the team's custom statuses for
// it: the state this issue is about, and the one no interleaving may leave.
func orphanedStatusCount(t *testing.T, table, resourceType, teamID string) int {
	t.Helper()
	return countRows(t, integrationDB.DB,
		`SELECT count(*) FROM `+table+` r
		  WHERE r.team_id = $1
		    AND NOT (r.status = ANY($2))
		    AND NOT EXISTS (
		        SELECT 1 FROM team_statuses s
		         WHERE s.team_id = r.team_id AND s.resource_type = $3 AND s.slug = r.status)`,
		teamID, pq.Array(models.SystemStatusesFor(resourceType)), resourceType)
}

// confirmSlugAsync runs ConfirmSlug on its own connection and reports the
// answer, so a test can observe that it has not answered yet.
type confirmSlugResult struct {
	confirmed bool
	err       error
}

func confirmSlugAsync(ctx context.Context, teamID, resourceType, slug string) <-chan confirmSlugResult {
	done := make(chan confirmSlugResult, 1)
	go func() {
		confirmed, err := NewTeamStatusRepository(integrationDB).ConfirmSlug(ctx, teamID, resourceType, slug)
		done <- confirmSlugResult{confirmed: confirmed, err: err}
	}()
	return done
}

// statusDeleteHeldOpen runs DeleteAndReassign's own delete and reassignment
// statements for the status inside a transaction it leaves open, and returns
// the commit. It writes no audit entry: the locks under test are the status
// row's, which the audit insert does not touch.
func statusDeleteHeldOpen(
	t *testing.T, teamID, statusID, resourceType string,
) (commit func()) {
	t.Helper()
	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	deleted, err := scanTeamStatus(tx.QueryRowContext(ctx, teamStatusDeleteReturningSQL, teamID, statusID))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, teamStatusPreserveUpdatedAtSQL)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, teamStatusResourceTables[resourceType].reassign,
		teamID, models.DefaultStatusFor(resourceType), deleted.Slug)
	require.NoError(t, err)

	return func() { require.NoError(t, tx.Commit()) }
}

func TestIntegrationTeamStatusRepository_ConfirmSlugWaitsOutADelete(t *testing.T) {
	ctx := context.Background()
	repo := NewTeamStatusRepository(integrationDB)

	for resourceType, table := range statusResourceTables {
		t.Run(resourceType, func(t *testing.T) {
			f := newTeamStatusesFixture(t, integrationDB.DB)
			defaultStatus := models.DefaultStatusFor(resourceType)
			status := &models.TeamStatus{
				TeamID: f.teamID, ResourceType: resourceType, Slug: "in-review", Name: "In review",
			}
			require.NoError(t, repo.Create(ctx, status, testStatusAudit))
			early := f.insert(t, table, "in-review")

			// The delete has removed the row and reassigned what it could see,
			// and has not committed.
			commit := statusDeleteHeldOpen(t, f.teamID, status.ID, resourceType)

			// The racing write: it validated "in-review" before the delete, and
			// commits it now, too late for the reassignment above.
			late := f.insert(t, table, "in-review")

			done := confirmSlugAsync(ctx, f.teamID, resourceType, "in-review")
			select {
			case got := <-done:
				t.Fatalf("ConfirmSlug answered (%v, %v) while the delete was uncommitted: "+
					"it must wait for the row lock", got.confirmed, got.err)
			case <-time.After(300 * time.Millisecond):
			}

			commit()

			select {
			case got := <-done:
				require.NoError(t, got.err)
				assert.False(t, got.confirmed, "the status is gone once the delete commits")
			case <-time.After(10 * time.Second):
				t.Fatal("ConfirmSlug did not return after the delete committed")
			}

			// This is the state the issue describes, before the repair.
			assert.Equal(t, defaultStatus, f.snapshot(t, table, early).status)
			assert.Equal(t, "in-review", f.snapshot(t, table, late).status)
			assert.Equal(t, 1, orphanedStatusCount(t, table, resourceType, f.teamID))

			require.NoError(t, repo.ReassignResource(ctx, f.teamID, resourceType, late, "in-review", defaultStatus))

			assert.Equal(t, defaultStatus, f.snapshot(t, table, late).status)
			assert.Zero(t, orphanedStatusCount(t, table, resourceType, f.teamID))
		})
	}
}

// The other order: the write is committed, and confirmed, before the delete
// starts. The confirmation holds no lock once it has answered, so the delete
// is not blocked, and its reassignment sees the write.
func TestIntegrationTeamStatusRepository_ConfirmSlugBeforeADelete(t *testing.T) {
	ctx := context.Background()
	repo := NewTeamStatusRepository(integrationDB)

	for resourceType, table := range statusResourceTables {
		t.Run(resourceType, func(t *testing.T) {
			f := newTeamStatusesFixture(t, integrationDB.DB)
			defaultStatus := models.DefaultStatusFor(resourceType)
			status := &models.TeamStatus{
				TeamID: f.teamID, ResourceType: resourceType, Slug: "in-review", Name: "In review",
			}
			require.NoError(t, repo.Create(ctx, status, testStatusAudit))

			written := f.insert(t, table, "in-review")
			confirmed, err := repo.ConfirmSlug(ctx, f.teamID, resourceType, "in-review")
			require.NoError(t, err)
			require.True(t, confirmed)

			_, reassigned, err := repo.DeleteAndReassign(ctx, f.teamID, status.ID, defaultStatus, testStatusAudit)
			require.NoError(t, err)
			assert.Equal(t, 1, reassigned)
			assert.Equal(t, defaultStatus, f.snapshot(t, table, written).status)
			assert.Zero(t, orphanedStatusCount(t, table, resourceType, f.teamID))
		})
	}
}

// The wait ends with the caller's context. The service gives every
// confirmation a deadline (statusConfirmTimeout), so a delete that never
// commits fails the confirmation instead of holding the write, and its
// connection, for as long as the delete stays open.
func TestIntegrationTeamStatusRepository_ConfirmSlugHonoursTheContext(t *testing.T) {
	repo := NewTeamStatusRepository(integrationDB)
	f := newTeamStatusesFixture(t, integrationDB.DB)
	status := &models.TeamStatus{
		TeamID: f.teamID, ResourceType: models.StatusResourceTypeArtifact, Slug: "in-review", Name: "In review",
	}
	require.NoError(t, repo.Create(context.Background(), status, testStatusAudit))
	statusDeleteHeldOpen(t, f.teamID, status.ID, models.StatusResourceTypeArtifact)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	confirmed, err := repo.ConfirmSlug(ctx, f.teamID, models.StatusResourceTypeArtifact, "in-review")

	require.Error(t, err)
	assert.False(t, confirmed)
}

func TestIntegrationTeamStatusRepository_ConfirmSlugScope(t *testing.T) {
	ctx := context.Background()
	f := newTeamStatusFixture(t)
	f.create(t, f.teamA, models.StatusResourceTypeArtifact, "in-review", "In review")

	for name, tc := range map[string]struct {
		teamID, resourceType, slug string
		want                       bool
	}{
		"the team's own status":             {f.teamA, models.StatusResourceTypeArtifact, "in-review", true},
		"another team":                      {f.teamOther, models.StatusResourceTypeArtifact, "in-review", false},
		"another resource type":             {f.teamA, models.StatusResourceTypeMemory, "in-review", false},
		"a slug the team never had":         {f.teamA, models.StatusResourceTypeArtifact, "shipped", false},
		"a system status, which has no row": {f.teamA, models.StatusResourceTypeArtifact, models.StatusActive, false},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := f.repo.ConfirmSlug(ctx, tc.teamID, tc.resourceType, tc.slug)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestIntegrationTeamStatusRepository_ReassignResource(t *testing.T) {
	ctx := context.Background()
	repo := NewTeamStatusRepository(integrationDB)

	for resourceType, table := range statusResourceTables {
		t.Run(resourceType+" moves only the named row", func(t *testing.T) {
			mine := newTeamStatusesFixture(t, integrationDB.DB)
			other := newTeamStatusesFixture(t, integrationDB.DB)
			defaultStatus := models.DefaultStatusFor(resourceType)

			target := mine.insert(t, table, "in-review")
			sibling := mine.insert(t, table, "in-review")
			foreign := other.insert(t, table, "in-review")
			siblingBefore := mine.snapshot(t, table, sibling)
			foreignBefore := other.snapshot(t, table, foreign)

			require.NoError(t, repo.ReassignResource(ctx, mine.teamID, resourceType, target, "in-review", defaultStatus))

			assert.Equal(t, defaultStatus, mine.snapshot(t, table, target).status)
			assert.Equal(t, siblingBefore, mine.snapshot(t, table, sibling))

			// Another team's id is out of reach even when named outright.
			require.NoError(t, repo.ReassignResource(ctx, mine.teamID, resourceType, foreign, "in-review", defaultStatus))
			assert.Equal(t, foreignBefore, other.snapshot(t, table, foreign))
		})
	}

	// A resource that has moved on since -- to a status somebody chose -- is
	// not dragged back to the default.
	t.Run("a row that no longer carries the status is left alone", func(t *testing.T) {
		f := newTeamStatusesFixture(t, integrationDB.DB)
		row := f.insert(t, "artifacts", models.StatusArchived)
		before := f.snapshot(t, "artifacts", row)

		require.NoError(t, repo.ReassignResource(
			ctx, f.teamID, models.StatusResourceTypeArtifact, row, "in-review", models.StatusActive))

		assert.Equal(t, before, f.snapshot(t, "artifacts", row))
	})
}
