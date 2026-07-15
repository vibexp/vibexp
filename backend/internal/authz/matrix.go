package authz

import "github.com/vibexp/vibexp/internal/models"

// rolePermissions IS the permission matrix from epic #220 — a literal
// transcription of the PRD table. Every grant is listed explicitly; a role
// holds exactly the permissions named under it and nothing else.
//
// Read a column of the PRD table by reading one entry here. Deliberate
// omissions worth noting, because they are policy rather than oversight:
//
//   - Admin has neither TeamDelete nor OwnershipTransfer: those stay owner-only.
//   - Member has no project permissions at all: projects are read-only
//     containers for members (epic #220 decision D2, a breaking change).
//   - Member has ResourceUpdateAny — any member may update any resource,
//     including agents carrying encrypted credentials (decision D1).
//   - Member has ResourceDeleteOwn but not ResourceDeleteAny, and no
//     FeedItemDeleteAny: members delete only what they authored.
//
// Viewing the team, its members and its stats is not represented here: it
// carries no role dimension (every member may view), and tenancy is enforced
// by teamValidationMiddleware.
var rolePermissions = map[models.TeamMemberRole]map[Permission]bool{
	models.TeamMemberRoleOwner: {
		TeamUpdate:        true,
		TeamDelete:        true,
		OwnershipTransfer: true,
		MemberInvite:      true,
		MemberRemove:      true,
		MemberRoleUpdate:  true,
		ProjectCreate:     true,
		ProjectUpdate:     true,
		ProjectDelete:     true,
		ResourceCreate:    true,
		ResourceUpdateAny: true,
		ResourceDeleteOwn: true,
		ResourceDeleteAny: true,
		FeedItemDeleteAny: true,
	},
	models.TeamMemberRoleAdmin: {
		TeamUpdate:        true,
		MemberInvite:      true,
		MemberRemove:      true,
		MemberRoleUpdate:  true,
		ProjectCreate:     true,
		ProjectUpdate:     true,
		ProjectDelete:     true,
		ResourceCreate:    true,
		ResourceUpdateAny: true,
		ResourceDeleteOwn: true,
		ResourceDeleteAny: true,
		FeedItemDeleteAny: true,
	},
	models.TeamMemberRoleMember: {
		ResourceCreate:    true,
		ResourceUpdateAny: true,
		ResourceDeleteOwn: true,
	},
}

// Allowed reports whether role may perform perm.
//
// An unknown or empty role holds no permissions, so Allowed fails closed.
func Allowed(role models.TeamMemberRole, perm Permission) bool {
	return rolePermissions[role][perm]
}

// RolePermissions returns every permission granted to role, in the declaration
// order of All. The result is always non-nil — an unknown role yields an empty
// slice — so callers may serialize it directly as a JSON array.
func RolePermissions(role models.TeamMemberRole) []Permission {
	granted := rolePermissions[role]
	out := make([]Permission, 0, len(granted))
	for _, perm := range all {
		if granted[perm] {
			out = append(out, perm)
		}
	}
	return out
}
