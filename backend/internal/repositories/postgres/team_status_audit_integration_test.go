//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

// Integration tests for #1333: a custom status change and its
// team_settings_audit row are one transaction. The first half forces the audit
// insert itself to fail and checks that nothing of the change survives; the
// second runs the real status management service over the real repositories
// and reads back the one audit row each write leaves.

// storedStatusAudit is one team_settings_audit row as these tests read it.
type storedStatusAudit struct {
	surface           string
	actorUserID       *string
	sourceTeamID      *string
	createdResourceID *string
	detail            map[string]interface{}
}

// statusAuditRows returns every audit row of the team, oldest first.
func statusAuditRows(t *testing.T, teamID string) []storedStatusAudit {
	t.Helper()
	rows, err := integrationDB.Query(
		`SELECT surface, actor_user_id, source_team_id, created_resource_id, detail
		   FROM team_settings_audit
		  WHERE team_id = $1
		  ORDER BY created_at ASC, id ASC`, teamID)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	var entries []storedStatusAudit
	for rows.Next() {
		var entry storedStatusAudit
		var detail []byte
		require.NoError(t, rows.Scan(
			&entry.surface, &entry.actorUserID, &entry.sourceTeamID, &entry.createdResourceID, &detail))
		require.NoError(t, json.Unmarshal(detail, &entry.detail))
		entries = append(entries, entry)
	}
	require.NoError(t, rows.Err())
	return entries
}

// unstorableStatusAudit builds an entry the table rejects: its actor is a user
// that does not exist, which trips team_settings_audit's foreign key on the
// INSERT itself -- after the status change has already run in the transaction.
func unstorableStatusAudit(change repositories.TeamStatusChange) (*models.TeamSettingsAudit, error) {
	ghost := uuid.New().String()
	return &models.TeamSettingsAudit{
		TeamID:      change.Status.TeamID,
		ActorUserID: &ghost,
		Surface:     models.SettingsAuditSurfaceCustomStatuses,
	}, nil
}

func TestIntegrationTeamStatusRepository_CreateRollsBackWithoutItsAuditEntry(t *testing.T) {
	f := newTeamStatusFixture(t)
	ctx := context.Background()
	newStatus := func() *models.TeamStatus {
		return &models.TeamStatus{
			TeamID: f.teamA, ResourceType: models.StatusResourceTypeArtifact,
			Slug: "in-review", Name: "In review", CreatedBy: &f.userID,
		}
	}

	failed := newStatus()
	err := f.repo.Create(ctx, failed, unstorableStatusAudit)
	require.ErrorContains(t, err, "failed to audit team status create")
	assert.Empty(t, failed.ID, "a rolled-back insert reports no id")

	count, err := f.repo.Count(ctx, f.teamA, models.StatusResourceTypeArtifact)
	require.NoError(t, err)
	assert.Zero(t, count, "the status row must not survive its audit entry")
	assert.Empty(t, statusAuditRows(t, f.teamA))

	// Nothing is half-applied, so the very same request succeeds on a retry
	// instead of answering "already exists".
	retried := newStatus()
	require.NoError(t, f.repo.Create(ctx, retried, testStatusAudit))
	assert.NotEmpty(t, retried.ID)
	assert.Len(t, statusAuditRows(t, f.teamA), 1)
}

func TestIntegrationTeamStatusRepository_RenameRollsBackWithoutItsAuditEntry(t *testing.T) {
	f := newTeamStatusFixture(t)
	ctx := context.Background()
	created := f.create(t, f.teamA, models.StatusResourceTypeBlueprint, "in-review", "In review")
	auditsBefore := statusAuditRows(t, f.teamA)

	renamed, err := f.repo.UpdateName(ctx, f.teamA, created.ID, "Under review", unstorableStatusAudit)
	require.ErrorContains(t, err, "failed to audit team status rename")
	assert.Nil(t, renamed)

	got, err := f.repo.GetByID(ctx, f.teamA, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "In review", got.Name, "the old name must still be stored")
	assert.Equal(t, created.UpdatedAt, got.UpdatedAt)
	assert.Equal(t, auditsBefore, statusAuditRows(t, f.teamA))
}

func TestIntegrationTeamStatusRepository_DeleteRollsBackWithoutItsAuditEntry(t *testing.T) {
	ctx := context.Background()
	repo := NewTeamStatusRepository(integrationDB)

	for resourceType, table := range statusResourceTables {
		t.Run(resourceType, func(t *testing.T) {
			f := newMigration025Fixture(t, integrationDB.DB)
			status := &models.TeamStatus{
				TeamID: f.teamID, ResourceType: resourceType, Slug: "in-review", Name: "In review",
			}
			require.NoError(t, repo.Create(ctx, status, testStatusAudit))
			carrying := []string{f.insert(t, table, "in-review"), f.insert(t, table, "in-review")}
			before := make(map[string]migration025Snapshot, len(carrying))
			for _, id := range carrying {
				before[id] = f.snapshot(t, table, id)
			}
			auditsBefore := statusAuditRows(t, f.teamID)

			deleted, reassigned, err := repo.DeleteAndReassign(
				ctx, f.teamID, status.ID, models.DefaultStatusFor(resourceType), unstorableStatusAudit)
			require.ErrorContains(t, err, "failed to audit team status delete")
			assert.Nil(t, deleted)
			assert.Zero(t, reassigned)

			got, err := repo.GetByID(ctx, f.teamID, status.ID)
			require.NoError(t, err, "the status row must still exist")
			assert.Equal(t, "in-review", got.Slug)
			for _, id := range carrying {
				assert.Equal(t, before[id], f.snapshot(t, table, id),
					"a %s must keep its status when the delete is rolled back", resourceType)
			}
			assert.Equal(t, auditsBefore, statusAuditRows(t, f.teamID))
		})
	}
}

// The name a rename's audit entry records as replaced is read under the row
// lock, in the rename's own transaction. A rename that starts while another is
// still uncommitted therefore waits, and then records the name the first one
// stored -- not the one it would have read before the first committed.
func TestIntegrationTeamStatusRepository_RenameReadsThePreviousNameUnderTheLock(t *testing.T) {
	f := newTeamStatusFixture(t)
	ctx := context.Background()
	created := f.create(t, f.teamA, models.StatusResourceTypeArtifact, "in-review", "First")

	// An earlier rename to "Second", run with the repository's own statements
	// and left uncommitted.
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	var locked string
	require.NoError(t, tx.QueryRowContext(ctx, teamStatusLockNameSQL, f.teamA, created.ID).Scan(&locked))
	require.Equal(t, "First", locked)
	_, err = scanTeamStatus(tx.QueryRowContext(ctx, teamStatusUpdateNameSQL, f.teamA, created.ID, "Second"))
	require.NoError(t, err)

	type renameResult struct {
		previous string
		err      error
	}
	done := make(chan renameResult, 1)
	go func() {
		var result renameResult
		waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		_, result.err = f.repo.UpdateName(waitCtx, f.teamA, created.ID, "Third",
			func(change repositories.TeamStatusChange) (*models.TeamSettingsAudit, error) {
				result.previous = change.PreviousName
				return testStatusAudit(change)
			})
		done <- result
	}()

	select {
	case result := <-done:
		t.Fatalf("the second rename did not wait for the first: previous=%q err=%v", result.previous, result.err)
	case <-time.After(300 * time.Millisecond):
	}
	require.NoError(t, tx.Commit())

	result := <-done
	require.NoError(t, result.err)
	assert.Equal(t, "Second", result.previous, "the entry names the name that was actually replaced")

	got, err := f.repo.GetByID(ctx, f.teamA, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "Third", got.Name)
}

// The real service over the real repositories: each successful write leaves
// exactly one custom_statuses audit row, with the detail the audit page reads.
func TestIntegrationStatusManagement_EachWriteStoresExactlyOneAuditRow(t *testing.T) {
	ctx := context.Background()
	f := newMigration025Fixture(t, integrationDB.DB)
	_, err := integrationDB.Exec(
		"INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)",
		f.teamID, f.userID, models.TeamMemberRoleOwner)
	require.NoError(t, err)

	logger := slog.New(slog.DiscardHandler)
	statuses := NewTeamStatusRepository(integrationDB)
	authorizer := services.NewAuthorizationService(NewTeamMemberRepository(integrationDB), logger)
	audit := services.NewTeamSettingsAuditService(
		NewTeamSettingsAuditRepository(integrationDB), authorizer,
		NewUserRepository(integrationDB), NewTeamRepository(integrationDB), logger)
	svc := services.NewStatusManagementService(
		statuses, services.NewStatusCatalogService(statuses), authorizer, audit, logger)

	// A resource already carries the slug before the status exists (the state
	// #1332 closes): the create response still reports 0 without counting.
	carrying := f.insert(t, "artifacts", "in-review")

	created, err := svc.Create(ctx, services.CreateStatusParams{
		TeamID: f.teamID, UserID: f.userID,
		ResourceType: models.StatusResourceTypeArtifact, Slug: "in-review", Name: "In review",
	})
	require.NoError(t, err)
	require.NotNil(t, created.ID)
	assert.Zero(t, created.UsageCount)

	renamed, err := svc.Rename(ctx, services.RenameStatusParams{
		TeamID: f.teamID, UserID: f.userID, ID: *created.ID, Name: "Under review",
	})
	require.NoError(t, err)
	assert.Equal(t, "Under review", renamed.Name)
	assert.Equal(t, 1, renamed.UsageCount)

	deletion, err := svc.Delete(ctx, services.DeleteStatusParams{
		TeamID: f.teamID, UserID: f.userID, ID: *created.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, &services.StatusDeletion{ReassignedCount: 1, ReassignedTo: models.StatusActive}, deletion)
	assert.Equal(t, models.StatusActive, f.snapshot(t, "artifacts", carrying).status)

	entries := statusAuditRows(t, f.teamID)
	require.Len(t, entries, 3, "one audit row per write, no more")
	for _, entry := range entries {
		assert.Equal(t, models.SettingsAuditSurfaceCustomStatuses, entry.surface)
		require.NotNil(t, entry.actorUserID)
		assert.Equal(t, f.userID, *entry.actorUserID)
		assert.Nil(t, entry.sourceTeamID)
	}

	assert.Equal(t, map[string]interface{}{
		"action": "created", "resource_type": "artifact", "slug": "in-review", "name": "In review",
	}, entries[0].detail)
	require.NotNil(t, entries[0].createdResourceID)
	assert.Equal(t, *created.ID, *entries[0].createdResourceID)

	assert.Equal(t, map[string]interface{}{
		"action": "renamed", "resource_type": "artifact", "slug": "in-review",
		"name": "Under review", "previous_name": "In review",
	}, entries[1].detail)
	assert.Nil(t, entries[1].createdResourceID)

	assert.Equal(t, map[string]interface{}{
		"action": "deleted", "resource_type": "artifact", "slug": "in-review", "name": "Under review",
		"reassigned_count": float64(1), "reassigned_to": models.StatusActive,
	}, entries[2].detail)
	assert.Nil(t, entries[2].createdResourceID)
}
