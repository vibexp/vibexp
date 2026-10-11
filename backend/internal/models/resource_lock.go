package models

import "time"

// Lock history actions (resource_lock_events.action).
const (
	LockActionLock   = "lock"
	LockActionUnlock = "unlock"
)

// ResourceLock is the lock held on a prompt, blueprint or artifact (epic
// #1285). A resource carries one when it is locked and nil when it is not.
//
// It is stored on the resource row (locked_by, locked_at, lock_comment), and a
// row is locked exactly when locked_by is set: the other two columns are left
// behind, stale, when the locker's account is deleted, so the repository builds
// this value from locked_by alone.
type ResourceLock struct {
	// LockedBy is the id of the member holding the lock.
	LockedBy string `json:"locked_by"`
	// LockedByName is that member's display name, resolved by the read's join
	// on users so a client can render "locked by …" without a second request.
	LockedByName string `json:"locked_by_name"`
	// Comment is why the resource was locked, as given by the locker.
	Comment string `json:"comment"`
	// LockedAt is when the lock was taken.
	LockedAt time.Time `json:"locked_at"`
}

// ResourceLockEvent is one entry of the append-only lock history
// (resource_lock_events). The history outlives both the resource and the
// actor: ResourceID carries no foreign key, and ActorUserID is nulled when the
// account is deleted.
type ResourceLockEvent struct {
	ID     string `json:"id" db:"id"`
	TeamID string `json:"team_id" db:"team_id"`
	// ResourceType is one of the RelationResourceType* values a lock applies
	// to (see IsLockableResourceType).
	ResourceType string `json:"resource_type" db:"resource_type"`
	ResourceID   string `json:"resource_id" db:"resource_id"`
	// Action is LockActionLock or LockActionUnlock.
	Action string `json:"action" db:"action"`
	// ActorUserID is who locked or unlocked; nil once that account is deleted.
	ActorUserID *string `json:"actor_user_id" db:"actor_user_id"`
	// ActorName is the actor's display name, resolved by the list read's join
	// on users. Empty when there is no actor, and on the value Append returns
	// (an INSERT cannot produce a joined column).
	ActorName string `json:"actor_name" db:"-"`
	// Comment is the lock comment for a lock event; nil for an unlock.
	Comment   *string   `json:"comment" db:"comment"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// IsLockableResourceType reports whether t is a resource type that can be
// locked. It reuses the relation vocabulary rather than introducing a second
// one; memories are deliberately not lockable.
func IsLockableResourceType(t string) bool {
	switch t {
	case RelationResourceTypePrompt, RelationResourceTypeBlueprint, RelationResourceTypeArtifact:
		return true
	}
	return false
}

// IsValidLockAction reports whether a is a known lock history action.
func IsValidLockAction(a string) bool {
	return a == LockActionLock || a == LockActionUnlock
}
