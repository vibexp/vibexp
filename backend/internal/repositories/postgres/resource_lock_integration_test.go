//go:build integration

package postgres

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Behaviour-level proof, against real Postgres, of resource lock storage
// (#1286) on all three lockable types. The sqlmock tests pin the statements;
// these pin what the database does with them: the lock round-trips through
// every read path, a second lock cannot overwrite the first even under
// concurrency, the filter and its total agree, and deleting the locker's
// account unlocks the resource with no application code.

// lockScope is one team with its owner (who is also the resources' author, so
// every membership- and ownership-scoped read reaches them) and a project.
type lockScope struct {
	userID, teamID, projectID string
}

func newLockScope(t *testing.T) lockScope {
	t.Helper()
	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	return lockScope{userID: userID, teamID: teamID, projectID: insertTestProject(t, userID, teamID)}
}

// lockRow identifies a seeded resource by every key a read path can use.
type lockRow struct {
	id, slug, path string
}

// lockReads maps a read-path name to a call returning the Lock that path
// reported for one resource.
type lockReads map[string]func(ctx context.Context) (*models.ResourceLock, error)

// lockTarget adapts one resource repository to the type-agnostic tests below.
type lockTarget struct {
	resourceType string
	table        string
	notFound     error
	insert       func(t *testing.T, s lockScope) lockRow
	setLock      func(ctx context.Context, teamID, id, lockerID, comment string) error
	clearLock    func(ctx context.Context, teamID, id string) error
	// reads returns EVERY single-row read path of the repository plus its list
	// paths (each narrowed to the one row), so a path that forgot the lock
	// columns cannot hide.
	reads func(s lockScope, row lockRow) lockReads
	// list returns the Lock of every row the team list returns for the filter,
	// keyed by id, and the total the list reported.
	list func(ctx context.Context, s lockScope, locked *bool) (map[string]*models.ResourceLock, int, error)
}

func lockTargets() []lockTarget {
	return []lockTarget{promptLockTarget(), blueprintLockTarget(), artifactLockTarget()}
}

// lockOf picks the one row's Lock out of a list result, failing the read when
// the row is missing so a list that dropped the row is not read as "unlocked".
func lockOf(locks map[string]*models.ResourceLock, id string) (*models.ResourceLock, error) {
	lock, ok := locks[id]
	if !ok {
		return nil, repositories.ErrResourceNotLocked
	}
	return lock, nil
}

func promptLockTarget() lockTarget {
	repo := NewPromptRepository(integrationDB)
	list := func(ctx context.Context, s lockScope, locked *bool) (map[string]*models.ResourceLock, int, error) {
		prompts, total, err := repo.List(ctx, s.userID, repositories.PromptFilters{
			TeamID: s.teamID, Page: 1, Limit: 100, Locked: locked,
		})
		locks := make(map[string]*models.ResourceLock, len(prompts))
		for _, p := range prompts {
			locks[p.ID] = p.Lock
		}
		return locks, total, err
	}
	return lockTarget{
		resourceType: models.RelationResourceTypePrompt,
		table:        "prompts",
		notFound:     repositories.ErrPromptNotFound,
		insert: func(t *testing.T, s lockScope) lockRow {
			id := insertTestPrompt(t, s.userID, s.teamID, s.projectID, "Lock prompt", "body", models.PromptStatusPublished)
			return lockRow{id: id, slug: "prompt-" + id[:8]}
		},
		setLock:   repo.SetLock,
		clearLock: repo.ClearLock,
		list:      list,
		reads: func(s lockScope, row lockRow) lockReads {
			one := func(p *models.Prompt, err error) (*models.ResourceLock, error) {
				if err != nil {
					return nil, err
				}
				return p.Lock, nil
			}
			return lockReads{
				"GetByID": func(ctx context.Context) (*models.ResourceLock, error) {
					return one(repo.GetByID(ctx, s.userID, s.teamID, row.id))
				},
				"GetByID (shared, no user)": func(ctx context.Context) (*models.ResourceLock, error) {
					return one(repo.GetByID(ctx, "", s.teamID, row.id))
				},
				"GetBySlug": func(ctx context.Context) (*models.ResourceLock, error) {
					return one(repo.GetBySlug(ctx, s.userID, s.teamID, row.slug))
				},
				"GetBySlugInTeam": func(ctx context.Context) (*models.ResourceLock, error) {
					return one(repo.GetBySlugInTeam(ctx, s.teamID, row.slug))
				},
				"GetByIDCrossTeam": func(ctx context.Context) (*models.ResourceLock, error) {
					return one(repo.GetByIDCrossTeam(ctx, s.userID, row.id))
				},
				"List": func(ctx context.Context) (*models.ResourceLock, error) {
					locks, _, err := list(ctx, s, nil)
					if err != nil {
						return nil, err
					}
					return lockOf(locks, row.id)
				},
				"List (is_shared join)": func(ctx context.Context) (*models.ResourceLock, error) {
					notShared := false
					prompts, _, err := repo.List(ctx, s.userID, repositories.PromptFilters{
						TeamID: s.teamID, Page: 1, Limit: 100, IsShared: &notShared,
					})
					if err != nil {
						return nil, err
					}
					locks := make(map[string]*models.ResourceLock, len(prompts))
					for _, p := range prompts {
						locks[p.ID] = p.Lock
					}
					return lockOf(locks, row.id)
				},
			}
		},
	}
}

func blueprintLockTarget() lockTarget {
	repo := NewBlueprintRepository(integrationDB)
	list := func(ctx context.Context, s lockScope, locked *bool) (map[string]*models.ResourceLock, int, error) {
		blueprints, total, err := repo.List(ctx, s.userID, repositories.BlueprintFilters{
			TeamID: s.teamID, Page: 1, Limit: 100, Locked: locked,
		})
		locks := make(map[string]*models.ResourceLock, len(blueprints))
		for _, b := range blueprints {
			locks[b.ID] = b.Lock
		}
		return locks, total, err
	}
	return lockTarget{
		resourceType: models.RelationResourceTypeBlueprint,
		table:        "blueprints",
		notFound:     repositories.ErrBlueprintNotFound,
		insert: func(t *testing.T, s lockScope) lockRow {
			t.Helper()
			id := uuid.New().String()
			row := lockRow{id: id, slug: "blueprint-" + id[:8], path: "rules/" + id[:8] + ".md"}
			_, err := integrationDB.ExecContext(context.Background(),
				"INSERT INTO blueprints (id, user_id, team_id, project_id, slug, title, content, path, status) "+
					"VALUES ($1, $2, $3, $4, $5, 'Lock blueprint', 'content', $6, $7)",
				id, s.userID, s.teamID, s.projectID, row.slug, row.path, models.StatusActive)
			require.NoError(t, err)
			return row
		},
		setLock:   repo.SetLock,
		clearLock: repo.ClearLock,
		list:      list,
		reads: func(s lockScope, row lockRow) lockReads {
			one := func(b *models.Blueprint, err error) (*models.ResourceLock, error) {
				if err != nil {
					return nil, err
				}
				return b.Lock, nil
			}
			return lockReads{
				"GetByID": func(ctx context.Context) (*models.ResourceLock, error) {
					return one(repo.GetByID(ctx, s.userID, s.teamID, row.id))
				},
				"GetByProjectIDAndSlug": func(ctx context.Context) (*models.ResourceLock, error) {
					return one(repo.GetByProjectIDAndSlug(ctx, s.userID, s.teamID, s.projectID, row.slug))
				},
				"GetByProjectIDAndPath": func(ctx context.Context) (*models.ResourceLock, error) {
					return one(repo.GetByProjectIDAndPath(ctx, s.userID, s.teamID, s.projectID, row.path))
				},
				"GetByIDCrossTeam": func(ctx context.Context) (*models.ResourceLock, error) {
					return one(repo.GetByIDCrossTeam(ctx, s.userID, row.id))
				},
				"List": func(ctx context.Context) (*models.ResourceLock, error) {
					locks, _, err := list(ctx, s, nil)
					if err != nil {
						return nil, err
					}
					return lockOf(locks, row.id)
				},
			}
		},
	}
}

func artifactLockTarget() lockTarget {
	repo := NewArtifactRepository(integrationDB)
	collect := func(artifacts []models.Artifact) map[string]*models.ResourceLock {
		locks := make(map[string]*models.ResourceLock, len(artifacts))
		for _, a := range artifacts {
			locks[a.ID] = a.Lock
		}
		return locks
	}
	list := func(ctx context.Context, s lockScope, locked *bool) (map[string]*models.ResourceLock, int, error) {
		artifacts, total, err := repo.List(ctx, s.userID, repositories.ArtifactFilters{
			TeamID: s.teamID, Page: 1, Limit: 100, Locked: locked,
		})
		return collect(artifacts), total, err
	}
	return lockTarget{
		resourceType: models.RelationResourceTypeArtifact,
		table:        "artifacts",
		notFound:     repositories.ErrArtifactNotFound,
		insert: func(t *testing.T, s lockScope) lockRow {
			id := insertTestArtifact(t, s.userID, s.teamID, s.projectID, "Lock artifact", "content", models.StatusActive)
			return lockRow{id: id, slug: "artifact-" + id[:8]}
		},
		setLock:   repo.SetLock,
		clearLock: repo.ClearLock,
		list:      list,
		reads: func(s lockScope, row lockRow) lockReads {
			one := func(a *models.Artifact, err error) (*models.ResourceLock, error) {
				if err != nil {
					return nil, err
				}
				return a.Lock, nil
			}
			return lockReads{
				"GetByID": func(ctx context.Context) (*models.ResourceLock, error) {
					return one(repo.GetByID(ctx, s.userID, s.teamID, row.id))
				},
				"GetByProjectIDAndSlug": func(ctx context.Context) (*models.ResourceLock, error) {
					return one(repo.GetByProjectIDAndSlug(ctx, s.userID, s.teamID, s.projectID, row.slug))
				},
				"GetByIDCrossTeam": func(ctx context.Context) (*models.ResourceLock, error) {
					return one(repo.GetByIDCrossTeam(ctx, s.userID, row.id))
				},
				"GetByProjectIDAndSlugCrossTeam": func(ctx context.Context) (*models.ResourceLock, error) {
					return one(repo.GetByProjectIDAndSlugCrossTeam(ctx, s.userID, s.projectID, row.slug))
				},
				"List": func(ctx context.Context) (*models.ResourceLock, error) {
					locks, _, err := list(ctx, s, nil)
					if err != nil {
						return nil, err
					}
					return lockOf(locks, row.id)
				},
				"ListCrossTeam": func(ctx context.Context) (*models.ResourceLock, error) {
					artifacts, _, err := repo.ListCrossTeam(ctx, s.userID, repositories.ArtifactFilters{
						Page: 1, Limit: 100, ProjectID: &s.projectID,
					})
					if err != nil {
						return nil, err
					}
					return lockOf(collect(artifacts), row.id)
				},
			}
		},
	}
}

// lockRowState is the row's edit signals: neither may move on a lock write.
type lockRowState struct {
	updatedAt time.Time
	version   int64
}

func (target lockTarget) rowState(t *testing.T, id string) lockRowState {
	t.Helper()
	var state lockRowState
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT updated_at, version FROM "+target.table+" WHERE id = $1", id,
	).Scan(&state.updatedAt, &state.version))
	return state
}

// requireLockOnEveryRead asserts every read path reports want (nil = unlocked).
func requireLockOnEveryRead(t *testing.T, target lockTarget, s lockScope, row lockRow, want *models.ResourceLock) {
	t.Helper()
	for name, read := range target.reads(s, row) {
		got, err := read(context.Background())
		require.NoError(t, err, name)
		if want == nil {
			assert.Nil(t, got, "%s must report the resource as unlocked", name)
			continue
		}
		require.NotNil(t, got, "%s must report the lock", name)
		assert.Equal(t, want.LockedBy, got.LockedBy, name)
		assert.Equal(t, want.LockedByName, got.LockedByName, name)
		assert.Equal(t, want.Comment, got.Comment, name)
		assert.True(t, want.LockedAt.Equal(got.LockedAt), "%s: locked_at %s, want %s", name, got.LockedAt, want.LockedAt)
	}
}

// storedLock reads the lock straight off the row, through no repository read.
func (target lockTarget) storedLock(t *testing.T, id string) *models.ResourceLock {
	t.Helper()
	var lock models.ResourceLock
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT locked_by::text, lock_comment, locked_at FROM "+target.table+" WHERE id = $1", id,
	).Scan(&lock.LockedBy, &lock.Comment, &lock.LockedAt))
	return &lock
}

func TestIntegrationResourceLock_RoundTripsThroughEveryReadPath(t *testing.T) {
	ctx := context.Background()
	for _, target := range lockTargets() {
		t.Run(target.resourceType, func(t *testing.T) {
			s := newLockScope(t)
			row := target.insert(t, s)
			requireLockOnEveryRead(t, target, s, row, nil)

			before := time.Now().Add(-time.Minute)
			require.NoError(t, target.setLock(ctx, s.teamID, row.id, s.userID, "freeze for release"))

			stored := target.storedLock(t, row.id)
			assert.Equal(t, s.userID, stored.LockedBy)
			assert.Equal(t, "freeze for release", stored.Comment)
			assert.True(t, stored.LockedAt.After(before), "locked_at is set by the database at lock time")
			requireLockOnEveryRead(t, target, s, row, &models.ResourceLock{
				LockedBy: s.userID, LockedByName: "Integration Test User",
				Comment: "freeze for release", LockedAt: stored.LockedAt,
			})

			require.NoError(t, target.clearLock(ctx, s.teamID, row.id))
			requireLockOnEveryRead(t, target, s, row, nil)
			assert.Equal(t, 1, countRows(t, integrationDB.DB,
				"SELECT count(*) FROM "+target.table+
					" WHERE id = $1 AND locked_by IS NULL AND locked_at IS NULL AND lock_comment IS NULL", row.id),
				"an explicit unlock clears all three columns")
		})
	}
}

func TestIntegrationResourceLock_SecondLockFailsWithoutOverwriting(t *testing.T) {
	ctx := context.Background()
	for _, target := range lockTargets() {
		t.Run(target.resourceType, func(t *testing.T) {
			s := newLockScope(t)
			row := target.insert(t, s)
			rival := insertTestUser(t)

			require.NoError(t, target.setLock(ctx, s.teamID, row.id, s.userID, "first"))
			first := target.storedLock(t, row.id)

			err := target.setLock(ctx, s.teamID, row.id, rival, "second")
			require.ErrorIs(t, err, repositories.ErrResourceAlreadyLocked)
			// Re-locking by the holder is refused too: it would move locked_at
			// and rewrite the comment.
			err = target.setLock(ctx, s.teamID, row.id, s.userID, "again")
			require.ErrorIs(t, err, repositories.ErrResourceAlreadyLocked)

			after := target.storedLock(t, row.id)
			assert.Equal(t, first.LockedBy, after.LockedBy)
			assert.Equal(t, "first", after.Comment)
			assert.True(t, first.LockedAt.Equal(after.LockedAt), "locked_at must not move")
		})
	}
}

// Two goroutines race for the same unlocked row. The conditional UPDATE is the
// only thing between them, so exactly one must win and the stored lock must be
// the winner's, whole.
func TestIntegrationResourceLock_ConcurrentLockHasExactlyOneWinner(t *testing.T) {
	ctx := context.Background()
	for _, target := range lockTargets() {
		t.Run(target.resourceType, func(t *testing.T) {
			s := newLockScope(t)
			lockers := []string{s.userID, insertTestUser(t)}

			// Repeated on fresh rows: a single race can be won by luck.
			for round := 0; round < 20; round++ {
				row := target.insert(t, s)
				errs := make([]error, len(lockers))
				start := make(chan struct{})
				var wg sync.WaitGroup
				for i, locker := range lockers {
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-start
						errs[i] = target.setLock(ctx, s.teamID, row.id, locker, "by "+locker)
					}()
				}
				close(start)
				wg.Wait()

				winner := -1
				for i, err := range errs {
					if err == nil {
						require.Equal(t, -1, winner, "round %d: both lockers won", round)
						winner = i
						continue
					}
					require.ErrorIs(t, err, repositories.ErrResourceAlreadyLocked, "round %d", round)
				}
				require.NotEqual(t, -1, winner, "round %d: nobody won", round)

				stored := target.storedLock(t, row.id)
				assert.Equal(t, lockers[winner], stored.LockedBy, "round %d", round)
				assert.Equal(t, "by "+lockers[winner], stored.Comment, "round %d: locker and comment must match", round)
			}
		})
	}
}

func TestIntegrationResourceLock_WritesAreNotEdits(t *testing.T) {
	ctx := context.Background()
	for _, target := range lockTargets() {
		t.Run(target.resourceType, func(t *testing.T) {
			s := newLockScope(t)
			row := target.insert(t, s)
			// Backdate the row so a write that touched updated_at could not hide
			// inside the same clock tick. None of the three tables has an
			// updated_at trigger, so a plain UPDATE is a faithful fixture.
			_, err := integrationDB.ExecContext(ctx,
				"UPDATE "+target.table+" SET updated_at = $1 WHERE id = $2",
				time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), row.id)
			require.NoError(t, err)
			before := target.rowState(t, row.id)

			require.ErrorIs(t, target.clearLock(ctx, s.teamID, row.id), repositories.ErrResourceNotLocked)
			assert.Equal(t, before, target.rowState(t, row.id), "a refused unlock")

			require.NoError(t, target.setLock(ctx, s.teamID, row.id, s.userID, "freeze"))
			assert.Equal(t, before, target.rowState(t, row.id), "lock")

			require.ErrorIs(t, target.setLock(ctx, s.teamID, row.id, s.userID, "x"), repositories.ErrResourceAlreadyLocked)
			assert.Equal(t, before, target.rowState(t, row.id), "a refused lock")

			require.NoError(t, target.clearLock(ctx, s.teamID, row.id))
			assert.Equal(t, before, target.rowState(t, row.id), "unlock")
		})
	}
}

func TestIntegrationResourceLock_MissingResourceIsNotFound(t *testing.T) {
	ctx := context.Background()
	for _, target := range lockTargets() {
		t.Run(target.resourceType, func(t *testing.T) {
			s := newLockScope(t)
			missing := uuid.New().String()

			assert.ErrorIs(t, target.setLock(ctx, s.teamID, missing, s.userID, "freeze"), target.notFound)
			assert.ErrorIs(t, target.clearLock(ctx, s.teamID, missing), target.notFound)
		})
	}
}

func TestIntegrationResourceLock_LockedFilterAndTotalAgree(t *testing.T) {
	ctx := context.Background()
	locked, unlocked := true, false
	for _, target := range lockTargets() {
		t.Run(target.resourceType, func(t *testing.T) {
			s := newLockScope(t)
			var lockedIDs, unlockedIDs []string
			for i := 0; i < 2; i++ {
				row := target.insert(t, s)
				require.NoError(t, target.setLock(ctx, s.teamID, row.id, s.userID, "freeze"))
				lockedIDs = append(lockedIDs, row.id)
			}
			for i := 0; i < 3; i++ {
				unlockedIDs = append(unlockedIDs, target.insert(t, s).id)
			}

			ids := func(filter *bool) ([]string, int) {
				locks, total, err := target.list(ctx, s, filter)
				require.NoError(t, err)
				out := make([]string, 0, len(locks))
				for id, lock := range locks {
					out = append(out, id)
					if filter != nil {
						assert.Equal(t, *filter, lock != nil, "row %s: the filter and the reported Lock disagree", id)
					}
				}
				sort.Strings(out)
				return out, total
			}
			sorted := func(in ...[]string) []string {
				var out []string
				for _, part := range in {
					out = append(out, part...)
				}
				sort.Strings(out)
				return out
			}

			got, total := ids(&locked)
			assert.Equal(t, sorted(lockedIDs), got, "locked=true")
			assert.Equal(t, 2, total, "locked=true total")

			got, total = ids(&unlocked)
			assert.Equal(t, sorted(unlockedIDs), got, "locked=false")
			assert.Equal(t, 3, total, "locked=false total")

			got, total = ids(nil)
			assert.Equal(t, sorted(lockedIDs, unlockedIDs), got, "no filter returns every row")
			assert.Equal(t, 5, total, "no filter total")
		})
	}
}

// The acceptance criterion with no code behind it: locked_by is ON DELETE SET
// NULL, so deleting the locker's users row must leave the resource reading as
// unlocked on every path -- even though locked_at and lock_comment are still
// on the row.
func TestIntegrationResourceLock_DeletingTheLockerUnlocks(t *testing.T) {
	ctx := context.Background()
	locked, unlocked := true, false
	for _, target := range lockTargets() {
		t.Run(target.resourceType, func(t *testing.T) {
			s := newLockScope(t)
			row := target.insert(t, s)
			locker := insertTestUser(t)
			require.NoError(t, target.setLock(ctx, s.teamID, row.id, locker, "freeze"))
			before := target.rowState(t, row.id)

			_, err := integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", locker)
			require.NoError(t, err)

			require.Equal(t, 1, countRows(t, integrationDB.DB,
				"SELECT count(*) FROM "+target.table+
					" WHERE id = $1 AND locked_by IS NULL AND locked_at IS NOT NULL AND lock_comment = 'freeze'", row.id),
				"fixture: the stale timestamp and comment are still on the row")

			requireLockOnEveryRead(t, target, s, row, nil)

			locks, total, err := target.list(ctx, s, &locked)
			require.NoError(t, err)
			assert.Empty(t, locks, "locked=true must not return a row nobody holds")
			assert.Zero(t, total)
			locks, total, err = target.list(ctx, s, &unlocked)
			require.NoError(t, err)
			assert.Contains(t, locks, row.id)
			assert.Equal(t, 1, total)

			assert.Equal(t, before, target.rowState(t, row.id), "the unlock-by-deletion is not an edit")

			// The row is genuinely free again, not just reported so.
			assert.ErrorIs(t, target.clearLock(ctx, s.teamID, row.id), repositories.ErrResourceNotLocked)
			require.NoError(t, target.setLock(ctx, s.teamID, row.id, s.userID, "relock"))
			assert.Equal(t, "relock", target.storedLock(t, row.id).Comment)
		})
	}
}

func TestIntegrationResourceLock_TeamIsolation(t *testing.T) {
	ctx := context.Background()
	locked := true
	for _, target := range lockTargets() {
		t.Run(target.resourceType, func(t *testing.T) {
			owner := newLockScope(t)
			other := newLockScope(t)
			row := target.insert(t, owner)

			// Another team can neither set ...
			require.ErrorIs(t, target.setLock(ctx, other.teamID, row.id, other.userID, "hijack"), target.notFound)
			requireLockOnEveryRead(t, target, owner, row, nil)

			// ... nor clear ...
			require.NoError(t, target.setLock(ctx, owner.teamID, row.id, owner.userID, "freeze"))
			require.ErrorIs(t, target.clearLock(ctx, other.teamID, row.id), target.notFound)
			assert.Equal(t, "freeze", target.storedLock(t, row.id).Comment)

			// ... nor see the lock.
			locks, total, err := target.list(ctx, other, &locked)
			require.NoError(t, err)
			assert.Empty(t, locks)
			assert.Zero(t, total)
		})
	}
}

func TestIntegrationResourceLockEvents_AppendAndListNewestFirst(t *testing.T) {
	ctx := context.Background()
	repo := NewResourceLockEventRepository(integrationDB)
	s := newLockScope(t)
	resourceID := uuid.New().String()
	comment := "freeze for release"

	lockEvent := &models.ResourceLockEvent{
		TeamID: s.teamID, ResourceType: models.RelationResourceTypePrompt, ResourceID: resourceID,
		Action: models.LockActionLock, ActorUserID: &s.userID, Comment: &comment,
	}
	require.NoError(t, repo.Append(ctx, lockEvent))
	require.NotEmpty(t, lockEvent.ID)
	require.False(t, lockEvent.CreatedAt.IsZero())

	unlockEvent := &models.ResourceLockEvent{
		TeamID: s.teamID, ResourceType: models.RelationResourceTypePrompt, ResourceID: resourceID,
		Action: models.LockActionUnlock, ActorUserID: &s.userID,
	}
	require.NoError(t, repo.Append(ctx, unlockEvent))

	// Noise the read must not return: same resource id under another type,
	// another resource, and the same resource id in another team.
	other := newLockScope(t)
	for _, noise := range []*models.ResourceLockEvent{
		{TeamID: s.teamID, ResourceType: models.RelationResourceTypeArtifact, ResourceID: resourceID, Action: models.LockActionLock},
		{TeamID: s.teamID, ResourceType: models.RelationResourceTypePrompt, ResourceID: uuid.New().String(), Action: models.LockActionLock},
		{TeamID: other.teamID, ResourceType: models.RelationResourceTypePrompt, ResourceID: resourceID, Action: models.LockActionLock},
	} {
		require.NoError(t, repo.Append(ctx, noise))
	}

	events, err := repo.ListByResource(ctx, s.teamID, models.RelationResourceTypePrompt, resourceID, 0)
	require.NoError(t, err)
	require.Len(t, events, 2)

	assert.Equal(t, unlockEvent.ID, events[0].ID, "newest first")
	assert.Equal(t, models.LockActionUnlock, events[0].Action)
	assert.Nil(t, events[0].Comment, "an unlock carries no comment")

	assert.Equal(t, lockEvent.ID, events[1].ID)
	assert.Equal(t, models.LockActionLock, events[1].Action)
	require.NotNil(t, events[1].Comment)
	assert.Equal(t, comment, *events[1].Comment)
	require.NotNil(t, events[1].ActorUserID)
	assert.Equal(t, s.userID, *events[1].ActorUserID)
	assert.Equal(t, "Integration Test User", events[1].ActorName)
	assert.Equal(t, s.teamID, events[1].TeamID)
	assert.Equal(t, resourceID, events[1].ResourceID)

	limited, err := repo.ListByResource(ctx, s.teamID, models.RelationResourceTypePrompt, resourceID, 1)
	require.NoError(t, err)
	require.Len(t, limited, 1)
	assert.Equal(t, unlockEvent.ID, limited[0].ID, "the limit keeps the newest")

	fromOtherTeam, err := repo.ListByResource(ctx, other.teamID, models.RelationResourceTypePrompt, resourceID, 0)
	require.NoError(t, err)
	require.Len(t, fromOtherTeam, 1, "another team sees only its own events for that id")

	none, err := repo.ListByResource(ctx, s.teamID, models.RelationResourceTypeBlueprint, resourceID, 0)
	require.NoError(t, err)
	assert.NotNil(t, none)
	assert.Empty(t, none)
}

// Events written in one transaction share a created_at (now() is
// transaction-start time), so the order must fall back to id, descending, and
// stay the same from one read to the next.
func TestIntegrationResourceLockEvents_TiesBreakOnID(t *testing.T) {
	ctx := context.Background()
	repo := NewResourceLockEventRepository(integrationDB)
	s := newLockScope(t)
	resourceID := uuid.New().String()
	at := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)

	ids := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		id := uuid.New().String()
		_, err := integrationDB.ExecContext(ctx,
			`INSERT INTO resource_lock_events (id, team_id, resource_type, resource_id, action, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			id, s.teamID, models.RelationResourceTypeArtifact, resourceID, models.LockActionLock, at)
		require.NoError(t, err)
		ids = append(ids, id)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))

	for read := 0; read < 3; read++ {
		events, err := repo.ListByResource(ctx, s.teamID, models.RelationResourceTypeArtifact, resourceID, 0)
		require.NoError(t, err)
		got := make([]string, 0, len(events))
		for _, event := range events {
			got = append(got, event.ID)
		}
		assert.Equal(t, ids, got, "read %d", read)
	}
}

func TestIntegrationResourceLockEvents_OutliveTheActor(t *testing.T) {
	ctx := context.Background()
	repo := NewResourceLockEventRepository(integrationDB)
	s := newLockScope(t)
	actor := insertTestUser(t)
	resourceID := uuid.New().String()

	require.NoError(t, repo.Append(ctx, &models.ResourceLockEvent{
		TeamID: s.teamID, ResourceType: models.RelationResourceTypeBlueprint, ResourceID: resourceID,
		Action: models.LockActionLock, ActorUserID: &actor,
	}))
	_, err := integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", actor)
	require.NoError(t, err)

	events, err := repo.ListByResource(ctx, s.teamID, models.RelationResourceTypeBlueprint, resourceID, 0)
	require.NoError(t, err)
	require.Len(t, events, 1, "the entry must survive its actor")
	assert.Nil(t, events[0].ActorUserID)
	assert.Empty(t, events[0].ActorName)
}

func TestIntegrationResourceLockEvents_RejectsInvalidEventsBeforeTheDatabase(t *testing.T) {
	ctx := context.Background()
	repo := NewResourceLockEventRepository(integrationDB)
	s := newLockScope(t)
	resourceID := uuid.New().String()

	err := repo.Append(ctx, &models.ResourceLockEvent{
		TeamID: s.teamID, ResourceType: models.RelationResourceTypeMemory, ResourceID: resourceID,
		Action: models.LockActionLock,
	})
	require.ErrorIs(t, err, repositories.ErrInvalidResourceLockEvent)

	err = repo.Append(ctx, &models.ResourceLockEvent{
		TeamID: s.teamID, ResourceType: models.RelationResourceTypePrompt, ResourceID: resourceID,
		Action: "relock",
	})
	require.ErrorIs(t, err, repositories.ErrInvalidResourceLockEvent)

	assert.Equal(t, 0, countRows(t, integrationDB.DB,
		"SELECT count(*) FROM resource_lock_events WHERE team_id = $1", s.teamID))
}
