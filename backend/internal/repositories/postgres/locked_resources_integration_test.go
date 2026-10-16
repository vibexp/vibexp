//go:build integration

package postgres

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

// The indirect paths around a lock (#1290), against real Postgres and on all
// three lockable types: project deletion, custom type deletion, custom status
// deletion and its single-row repair are refused while a locked resource is in
// the way and go through once it is unlocked; deleting the locker's account
// records the unlock it causes; a GitHub re-import and the locker leaving the
// team both leave the lock where it is. Project migration lives with its own
// service, in internal/services/projectmigration.

// lockedRefTitleColumn is the column a LockedResourceRef's Title is read from.
var lockedRefTitleColumn = map[string]string{"prompts": "name", "blueprints": "title", "artifacts": "title"}

// lockedRefOf builds the ref a guard must report for one seeded resource,
// reading slug and title straight off the row.
func lockedRefOf(t *testing.T, target lockTarget, id string) models.LockedResourceRef {
	t.Helper()
	ref := models.LockedResourceRef{ResourceType: target.resourceType, ID: id}
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT slug, "+lockedRefTitleColumn[target.table]+" FROM "+target.table+" WHERE id = $1", id).
		Scan(&ref.Slug, &ref.Title))
	return ref
}

// requireRefusedByLock asserts err is a guard's refusal for reason, naming
// exactly want.
func requireRefusedByLock(t *testing.T, err, reason error, want ...models.LockedResourceRef) {
	t.Helper()
	var locked *repositories.LockedResourcesError
	require.ErrorAs(t, err, &locked)
	require.ErrorIs(t, err, reason)
	assert.Equal(t, want, locked.Resources)
}

// columnOf reads one text column of one row.
func columnOf(t *testing.T, table, column, id string) string {
	t.Helper()
	var value string
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT "+column+" FROM "+table+" WHERE id = $1", id).Scan(&value))
	return value
}

// setColumn writes one text column of one fixture row.
func setColumn(t *testing.T, table, column, id, value string) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"UPDATE "+table+" SET "+column+" = $2 WHERE id = $1", id, value)
	require.NoError(t, err)
}

func newIntegrationProjectService() *services.ProjectService {
	logger := slog.New(slog.DiscardHandler)
	return services.NewProjectService(
		NewProjectRepository(integrationDB), nil,
		services.NewAuthorizationService(NewTeamMemberRepository(integrationDB), logger), nil, logger,
	)
}

// Deleting a project cascades to every resource in it. With a locked resource
// inside it is refused, naming that resource and nothing else; after the
// unlock the same call deletes the project and its resources.
func TestIntegrationProjectDelete_RefusedWhileLockedThenAllowed(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationProjectService()
	for _, target := range lockTargets() {
		t.Run(target.resourceType, func(t *testing.T) {
			s := newLockScope(t)
			insertLockTeamMember(t, s.teamID, s.userID, models.TeamMemberRoleOwner)
			slug := columnOf(t, "projects", "slug", s.projectID)

			// A second project, so the last-project guard is not what refuses,
			// holding a locked resource of its own that must not be reported.
			other := s
			other.projectID = insertTestProject(t, s.userID, s.teamID)
			elsewhere := target.insert(t, other)
			require.NoError(t, target.setLock(ctx, s.teamID, elsewhere.id, s.userID, "another project"))

			locked := target.insert(t, s)
			unlocked := target.insert(t, s)
			require.NoError(t, target.setLock(ctx, s.teamID, locked.id, s.userID, "freeze"))

			err := svc.DeleteProject(context.Background(), s.teamID, s.userID, slug)
			requireRefusedByLock(t, err, repositories.ErrProjectHasLockedResources, lockedRefOf(t, target, locked.id))
			assert.Equal(t, 1, countRows(t, integrationDB.DB, "SELECT count(*) FROM projects WHERE id = $1", s.projectID))
			assert.Equal(t, 2, countRows(t, integrationDB.DB,
				"SELECT count(*) FROM "+target.table+" WHERE project_id = $1", s.projectID),
				"a refused delete removes nothing")
			assert.True(t, lockIsHeld(t, target.table, locked.id))

			require.NoError(t, target.clearLock(ctx, s.teamID, locked.id))
			require.NoError(t, svc.DeleteProject(context.Background(), s.teamID, s.userID, slug))
			assert.Zero(t, countRows(t, integrationDB.DB, "SELECT count(*) FROM projects WHERE id = $1", s.projectID))
			assert.Zero(t, countRows(t, integrationDB.DB,
				"SELECT count(*) FROM "+target.table+" WHERE id IN ($1, $2)", locked.id, unlocked.id))
			assert.True(t, lockIsHeld(t, target.table, elsewhere.id), "the other project's resource is untouched")
		})
	}
}

// The lookup behind the project guards: every lockable type, ordered by type
// then title, scoped to the one project, and the same answer on a transaction.
func TestIntegrationProjectRepository_ListLockedResources(t *testing.T) {
	ctx := context.Background()
	repo := NewProjectRepository(integrationDB)
	s := newLockScope(t)

	empty, err := repo.ListLockedResources(ctx, s.teamID, s.projectID)
	require.NoError(t, err)
	assert.NotNil(t, empty)
	assert.Empty(t, empty)

	byType := make(map[string]models.LockedResourceRef)
	for _, target := range lockTargets() {
		target.insert(t, s) // an unlocked sibling that must not be listed
		row := target.insert(t, s)
		require.NoError(t, target.setLock(ctx, s.teamID, row.id, s.userID, "freeze"))
		byType[target.resourceType] = lockedRefOf(t, target, row.id)
	}
	want := []models.LockedResourceRef{
		byType[models.RelationResourceTypeArtifact],
		byType[models.RelationResourceTypeBlueprint],
		byType[models.RelationResourceTypePrompt],
	}

	got, err := repo.ListLockedResources(ctx, s.teamID, s.projectID)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	inTx, err := repo.ListLockedResourcesTx(ctx, tx, s.teamID, s.projectID)
	require.NoError(t, tx.Rollback())
	require.NoError(t, err)
	assert.Equal(t, want, inTx)

	// Tenancy: the project id under another team's id finds nothing.
	foreign, err := repo.ListLockedResources(ctx, newLockScope(t).teamID, s.projectID)
	require.NoError(t, err)
	assert.Empty(t, foreign)
}

// Deleting a custom type rewrites artifacts.type, a frozen field. While a
// locked artifact uses the type the delete is refused and neither the type row
// nor any artifact changes; after the unlock it goes through.
func TestIntegrationTypeDelete_RefusedWhileLockedArtifactUsesIt(t *testing.T) {
	ctx := context.Background()
	repo := NewTypeRepository(integrationDB)
	target := artifactLockTarget()
	s := newLockScope(t)

	custom := &models.Type{TeamID: s.teamID, ResourceType: "artifacts", Slug: "runbook", Name: "Runbook"}
	require.NoError(t, repo.Create(ctx, custom))

	locked, unlocked := target.insert(t, s), target.insert(t, s)
	for _, id := range []string{locked.id, unlocked.id} {
		setColumn(t, "artifacts", "type", id, custom.Slug)
	}
	require.NoError(t, target.setLock(ctx, s.teamID, locked.id, s.userID, "freeze"))

	// Another team's locked artifact carrying the same slug is not this team's
	// business and must not be what refuses or what is reported.
	foreign := newLockScope(t)
	foreignRow := target.insert(t, foreign)
	setColumn(t, "artifacts", "type", foreignRow.id, custom.Slug)
	require.NoError(t, target.setLock(ctx, foreign.teamID, foreignRow.id, foreign.userID, "theirs"))

	before := target.rowState(t, unlocked.id)

	err := repo.DeleteCustom(ctx, s.teamID, custom.ID, "general")
	requireRefusedByLock(t, err, repositories.ErrTypeUsedByLockedResource, lockedRefOf(t, target, locked.id))
	assert.Equal(t, 1, countRows(t, integrationDB.DB, "SELECT count(*) FROM types WHERE id = $1", custom.ID),
		"the rollback restores the type row")
	for _, id := range []string{locked.id, unlocked.id} {
		assert.Equal(t, custom.Slug, columnOf(t, "artifacts", "type", id))
	}
	assert.Equal(t, before, target.rowState(t, unlocked.id), "a refused delete edits nothing")

	require.NoError(t, target.clearLock(ctx, s.teamID, locked.id))
	require.NoError(t, repo.DeleteCustom(ctx, s.teamID, custom.ID, "general"))
	assert.Zero(t, countRows(t, integrationDB.DB, "SELECT count(*) FROM types WHERE id = $1", custom.ID))
	for _, id := range []string{locked.id, unlocked.id} {
		assert.Equal(t, "general", columnOf(t, "artifacts", "type", id))
	}
	assert.Equal(t, custom.Slug, columnOf(t, "artifacts", "type", foreignRow.id))
}

// Deleting a custom status rewrites the status column, a frozen field. While a
// locked resource carries the status the delete is refused: the status row is
// still there, no row's status is rewritten -- not even the unlocked ones --
// and no audit entry is stored. After the unlock every carrying row moves.
func TestIntegrationStatusDelete_RefusedWhileLockedResourceCarriesIt(t *testing.T) {
	ctx := context.Background()
	repo := NewTeamStatusRepository(integrationDB)
	for _, target := range lockTargets() {
		t.Run(target.resourceType, func(t *testing.T) {
			s := newLockScope(t)
			status := &models.TeamStatus{
				TeamID: s.teamID, ResourceType: target.resourceType, Slug: "in-review", Name: "In review",
			}
			require.NoError(t, repo.Create(ctx, status, testStatusAudit))
			auditsBefore := countRows(t, integrationDB.DB,
				"SELECT count(*) FROM team_settings_audit WHERE team_id = $1", s.teamID)

			locked, unlocked := target.insert(t, s), target.insert(t, s)
			for _, id := range []string{locked.id, unlocked.id} {
				setColumn(t, target.table, "status", id, status.Slug)
			}
			require.NoError(t, target.setLock(ctx, s.teamID, locked.id, s.userID, "freeze"))
			defaultStatus := models.DefaultStatusFor(target.resourceType)

			deleted, reassigned, err := repo.DeleteAndReassign(ctx, s.teamID, status.ID, defaultStatus, testStatusAudit)
			requireRefusedByLock(t, err, repositories.ErrTeamStatusUsedByLockedResource,
				lockedRefOf(t, target, locked.id))
			assert.Nil(t, deleted)
			assert.Zero(t, reassigned)
			_, getErr := repo.GetByID(ctx, s.teamID, status.ID)
			require.NoError(t, getErr, "the rollback restores the status row")
			for _, id := range []string{locked.id, unlocked.id} {
				assert.Equal(t, status.Slug, columnOf(t, target.table, "status", id))
			}
			assert.Equal(t, auditsBefore, countRows(t, integrationDB.DB,
				"SELECT count(*) FROM team_settings_audit WHERE team_id = $1", s.teamID),
				"a refused delete is not audited as a delete")

			require.NoError(t, target.clearLock(ctx, s.teamID, locked.id))
			_, reassigned, err = repo.DeleteAndReassign(ctx, s.teamID, status.ID, defaultStatus, testStatusAudit)
			require.NoError(t, err)
			assert.Equal(t, 2, reassigned)
			for _, id := range []string{locked.id, unlocked.id} {
				assert.Equal(t, defaultStatus, columnOf(t, target.table, "status", id))
			}
		})
	}
}

// The single-row repair of a status delete applies the same rule: a locked row
// still carrying the deleted status is left as it is and reported, an unlocked
// one moves.
func TestIntegrationStatusReassignResource_SkipsALockedRow(t *testing.T) {
	ctx := context.Background()
	repo := NewTeamStatusRepository(integrationDB)
	for _, target := range lockTargets() {
		t.Run(target.resourceType, func(t *testing.T) {
			s := newLockScope(t)
			row := target.insert(t, s)
			setColumn(t, target.table, "status", row.id, "gone")
			require.NoError(t, target.setLock(ctx, s.teamID, row.id, s.userID, "freeze"))
			defaultStatus := models.DefaultStatusFor(target.resourceType)

			err := repo.ReassignResource(ctx, s.teamID, target.resourceType, row.id, "gone", defaultStatus)
			requireRefusedByLock(t, err, repositories.ErrTeamStatusUsedByLockedResource, lockedRefOf(t, target, row.id))
			assert.Equal(t, "gone", columnOf(t, target.table, "status", row.id))

			// A locked row that no longer carries the status is not a refusal.
			require.NoError(t, repo.ReassignResource(ctx, s.teamID, target.resourceType, row.id, "other", defaultStatus))

			require.NoError(t, target.clearLock(ctx, s.teamID, row.id))
			require.NoError(t, repo.ReassignResource(ctx, s.teamID, target.resourceType, row.id, "gone", defaultStatus))
			assert.Equal(t, defaultStatus, columnOf(t, target.table, "status", row.id))
		})
	}
}

// lockEvent is one resource_lock_events row as these tests read it.
type lockEvent struct {
	teamID, resourceType, action string
	hasActor, hasComment         bool
}

func lockEventsOf(t *testing.T, resourceID string) []lockEvent {
	t.Helper()
	rows, err := integrationDB.QueryContext(context.Background(),
		`SELECT team_id::text, resource_type, action, actor_user_id IS NOT NULL, comment IS NOT NULL
		   FROM resource_lock_events WHERE resource_id = $1 ORDER BY created_at, id`, resourceID)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	var events []lockEvent
	for rows.Next() {
		var e lockEvent
		require.NoError(t, rows.Scan(&e.teamID, &e.resourceType, &e.action, &e.hasActor, &e.hasComment))
		events = append(events, e)
	}
	require.NoError(t, rows.Err())
	return events
}

// Deleting an account releases every lock it holds (the foreign key), and the
// admin delete records each release as an unlock event with no actor -- one
// per lock, in the resource's own team, and none for a lock somebody else
// holds.
func TestIntegrationAdminDeleteUser_RecordsAnUnlockPerLockHeld(t *testing.T) {
	ctx := context.Background()
	admin := NewAdminRepository(integrationDB)
	s := newLockScope(t)
	locker := insertTestUser(t)

	type seeded struct {
		target       lockTarget
		mine, theirs lockRow
	}
	var seededRows []seeded
	for _, target := range lockTargets() {
		row := seeded{target: target, mine: target.insert(t, s), theirs: target.insert(t, s)}
		require.NoError(t, target.setLock(ctx, s.teamID, row.mine.id, locker, "the locker's"))
		require.NoError(t, target.setLock(ctx, s.teamID, row.theirs.id, s.userID, "the owner's"))
		seededRows = append(seededRows, row)
	}

	blockers, found, err := admin.DeleteUserIfUnblocked(ctx, locker)
	require.NoError(t, err)
	require.True(t, found)
	require.Empty(t, blockers)

	for _, row := range seededRows {
		assert.False(t, lockIsHeld(t, row.target.table, row.mine.id), row.target.resourceType)
		assert.Equal(t, []lockEvent{{
			teamID: s.teamID, resourceType: row.target.resourceType, action: models.LockActionUnlock,
		}}, lockEventsOf(t, row.mine.id), "%s: exactly one unlock event, with no actor and no comment",
			row.target.resourceType)

		assert.True(t, lockIsHeld(t, row.target.table, row.theirs.id), row.target.resourceType)
		assert.Empty(t, lockEventsOf(t, row.theirs.id), row.target.resourceType)
	}
}

// A refused account delete -- the user still owns a team with other members --
// writes no unlock event and releases nothing.
func TestIntegrationAdminDeleteUser_RefusedDeleteRecordsNothing(t *testing.T) {
	ctx := context.Background()
	admin := NewAdminRepository(integrationDB)
	s := newLockScope(t)
	insertLockTeamMember(t, s.teamID, s.userID, models.TeamMemberRoleOwner)
	insertLockTeamMember(t, s.teamID, insertTestUser(t), models.TeamMemberRoleMember)

	target := artifactLockTarget()
	row := target.insert(t, s)
	require.NoError(t, target.setLock(ctx, s.teamID, row.id, s.userID, "freeze"))

	blockers, found, err := admin.DeleteUserIfUnblocked(ctx, s.userID)
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, blockers, "fixture: the delete must be refused")

	assert.True(t, lockIsHeld(t, target.table, row.id))
	assert.Empty(t, lockEventsOf(t, row.id))
}

// A GitHub re-import is exempt from the lock by decision: it goes through
// UpdateOnReimport, which rewrites every synced field of a locked blueprint
// and leaves the lock exactly as it was. The test stops a later change from
// routing the re-import through the lock guard, or from clearing the lock.
func TestIntegrationBlueprintReimport_UpdatesALockedBlueprintAndKeepsTheLock(t *testing.T) {
	ctx := context.Background()
	repo := NewBlueprintRepository(integrationDB)
	target := blueprintLockTarget()
	s := newLockScope(t)
	row := target.insert(t, s)
	require.NoError(t, target.setLock(ctx, s.teamID, row.id, s.userID, "freeze"))
	lockBefore := target.storedLock(t, row.id)

	blueprint, err := repo.GetByID(ctx, s.userID, s.teamID, row.id)
	require.NoError(t, err)
	versionBefore := blueprint.Version

	blueprint.Title = "Re-imported title"
	blueprint.Description = "Re-imported description"
	blueprint.Content = "re-imported content"
	blueprint.Type = "cursor"
	subtype := "rules"
	blueprint.Subtype = &subtype
	blueprint.Metadata = map[string]interface{}{"origin": "reimport"}
	blueprint.Path = "skills/reimported/SKILL.md"
	blueprint.RawContent = "---\nname: reimported\n---\nre-imported content"
	blueprint.ContentSHA = "content-sha-2"
	blueprint.SourceContentSHA = "source-content-sha-2"
	blueprint.UpdatedAt = time.Now().UTC()
	require.NoError(t, repo.UpdateOnReimport(ctx, blueprint))

	got, err := repo.GetByID(ctx, s.userID, s.teamID, row.id)
	require.NoError(t, err)
	assert.Equal(t, "Re-imported title", got.Title)
	assert.Equal(t, "Re-imported description", got.Description)
	assert.Equal(t, "re-imported content", got.Content)
	assert.Equal(t, "cursor", got.Type)
	require.NotNil(t, got.Subtype)
	assert.Equal(t, "rules", *got.Subtype)
	assert.Equal(t, "reimport", got.Metadata["origin"])
	assert.Equal(t, "skills/reimported/SKILL.md", got.Path)
	assert.Equal(t, blueprint.RawContent, got.RawContent)
	assert.Equal(t, "content-sha-2", got.ContentSHA)
	assert.Equal(t, "source-content-sha-2", got.SourceContentSHA)
	assert.Equal(t, versionBefore+1, got.Version)

	require.NotNil(t, got.Lock, "the re-import must leave the lock in place")
	assert.Equal(t, lockBefore, target.storedLock(t, row.id))
}

// A lock belongs to the resource, not to the locker's membership: removing the
// locker from the team leaves the lock in place, the former member can no
// longer release it, and an admin can.
func TestIntegrationResourceLock_SurvivesTheLockerLeavingTheTeam(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationLockService()
	for _, target := range lockTargets() {
		t.Run(target.resourceType, func(t *testing.T) {
			s := newLockScope(t)
			insertLockTeamMember(t, s.teamID, s.userID, models.TeamMemberRoleOwner)
			locker, admin := insertTestUser(t), insertTestUser(t)
			insertLockTeamMember(t, s.teamID, locker, models.TeamMemberRoleMember)
			insertLockTeamMember(t, s.teamID, admin, models.TeamMemberRoleAdmin)

			row := target.insert(t, s)
			require.NoError(t, svc.Lock(ctx, locker, s.teamID, target.resourceType, row.id, "freeze"))

			_, err := integrationDB.ExecContext(ctx,
				"DELETE FROM team_members WHERE team_id = $1 AND user_id = $2", s.teamID, locker)
			require.NoError(t, err)

			require.True(t, lockIsHeld(t, target.table, row.id), "removing the locker must not unlock")
			assert.Equal(t, locker, target.storedLock(t, row.id).LockedBy)
			assert.ErrorIs(t, svc.Unlock(ctx, locker, s.teamID, target.resourceType, row.id),
				services.ErrPermissionDenied, "a former member may not release the lock")

			require.NoError(t, svc.Unlock(ctx, admin, s.teamID, target.resourceType, row.id))
			assert.False(t, lockIsHeld(t, target.table, row.id))
			assert.Equal(t, []string{models.LockActionUnlock, models.LockActionLock},
				lockHistoryActions(t, svc, admin, s, target.resourceType, row.id))
		})
	}
}
