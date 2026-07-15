// Package authz is the single source of truth for VibeXP's team permission
// matrix (owner / admin / member).
//
// This package IS the permission spec: the table in matrix.go is a literal
// transcription of the matrix in epic #220, and matrix_test.go asserts every
// cell of it. Changing who may do what is therefore a one-line diff here plus
// a one-line change in the test — never a hunt through services, handlers or
// SQL predicates.
//
// The package is pure: it performs no I/O and knows nothing about the
// database. Resolving a user's role and returning a typed error is the job of
// services.AuthorizationService, which is the only intended caller.
package authz

// Permission identifies a single action a team member may attempt.
//
// The string values are dot-namespaced and stable: they are surfaced verbatim
// to clients through the team `permissions` API field, so renaming one is a
// breaking API change.
type Permission string

const (
	// TeamUpdate covers changing team details (name, slug, description).
	TeamUpdate Permission = "team.update"
	// TeamDelete covers deleting the team itself.
	TeamDelete Permission = "team.delete"
	// OwnershipTransfer covers transferring team ownership to another member.
	OwnershipTransfer Permission = "team.transfer"

	// MemberInvite covers inviting new members (as member or admin).
	MemberInvite Permission = "member.invite"
	// MemberRemove covers removing members and admins from the team.
	MemberRemove Permission = "member.remove"
	// MemberRoleUpdate covers changing a member's role between member and admin.
	MemberRoleUpdate Permission = "member.role.update"

	// ProjectCreate covers creating a project within the team.
	ProjectCreate Permission = "project.create"
	// ProjectUpdate covers updating any project within the team.
	ProjectUpdate Permission = "project.update"
	// ProjectDelete covers deleting any project within the team.
	ProjectDelete Permission = "project.delete"

	// ResourceCreate covers creating a resource (prompt, memory, artifact,
	// blueprint, agent) in an existing project.
	ResourceCreate Permission = "resource.create"
	// ResourceUpdateAny covers updating any resource, including other members'.
	ResourceUpdateAny Permission = "resource.update.any"
	// ResourceDeleteOwn covers deleting a resource the caller created.
	ResourceDeleteOwn Permission = "resource.delete.own"
	// ResourceDeleteAny covers deleting a resource created by someone else.
	ResourceDeleteAny Permission = "resource.delete.any"

	// FeedItemDeleteAny covers deleting another member's feed post or reply
	// (moderation). Deleting one's own is always allowed to the author.
	FeedItemDeleteAny Permission = "feed.delete.any"
)

// all lists every permission in declaration order. It fixes the order in which
// RolePermissions reports a role's grants, and lets the matrix test prove that
// no permission escapes assertion.
var all = []Permission{
	TeamUpdate,
	TeamDelete,
	OwnershipTransfer,
	MemberInvite,
	MemberRemove,
	MemberRoleUpdate,
	ProjectCreate,
	ProjectUpdate,
	ProjectDelete,
	ResourceCreate,
	ResourceUpdateAny,
	ResourceDeleteOwn,
	ResourceDeleteAny,
	FeedItemDeleteAny,
}

// All returns every known permission, in declaration order.
func All() []Permission {
	out := make([]Permission, len(all))
	copy(out, all)
	return out
}

// String returns the wire representation of the permission.
func (p Permission) String() string {
	return string(p)
}
