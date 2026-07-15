package authz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// matrixSpec is the permission matrix from epic #220 §4, transcribed cell for
// cell. It is the executable copy of the PRD table: if a product decision
// changes who may do what, exactly one bool changes here and exactly one bool
// changes in matrix.go.
//
//	| Action                                   | Owner | Admin | Member |
//	|------------------------------------------|-------|-------|--------|
//	| Update team details                      |  yes  |  yes  |   no   |
//	| Delete team                              |  yes  |   no  |   no   |
//	| Transfer ownership                       |  yes  |   no  |   no   |
//	| Invite members (as member/admin)         |  yes  |  yes  |   no   |
//	| Remove members/admins                    |  yes  |  yes  |   no   |
//	| Change member<->admin roles              |  yes  |  yes  |   no   |
//	| Create / update / delete projects        |  yes  |  yes  |   no   |
//	| Create resources in existing projects    |  yes  |  yes  |  yes   |
//	| Update any resource (incl. others')      |  yes  |  yes  |  yes   |
//	| Delete own resources                     |  yes  |  yes  |  yes   |
//	| Delete others' resources                 |  yes  |  yes  |   no   |
//	| Feed posts/replies: delete anyone's      |  yes  |  yes  |   no   |
var matrixSpec = []struct {
	perm   Permission
	owner  bool
	admin  bool
	member bool
}{
	// Permission        Owner  Admin  Member
	{TeamUpdate, true, true, false},
	{TeamDelete, true, false, false},
	{OwnershipTransfer, true, false, false},
	{MemberInvite, true, true, false},
	{MemberRemove, true, true, false},
	{MemberRoleUpdate, true, true, false},
	{ProjectCreate, true, true, false},
	{ProjectUpdate, true, true, false},
	{ProjectDelete, true, true, false},
	{ResourceCreate, true, true, true},
	{ResourceUpdateAny, true, true, true},
	{ResourceDeleteOwn, true, true, true},
	{ResourceDeleteAny, true, true, false},
	{FeedItemDeleteAny, true, true, false},
}

func TestAllowed_MatchesPRDMatrix(t *testing.T) {
	for _, tc := range matrixSpec {
		t.Run(tc.perm.String(), func(t *testing.T) {
			assert.Equal(t, tc.owner, Allowed(models.TeamMemberRoleOwner, tc.perm), "owner")
			assert.Equal(t, tc.admin, Allowed(models.TeamMemberRoleAdmin, tc.perm), "admin")
			assert.Equal(t, tc.member, Allowed(models.TeamMemberRoleMember, tc.perm), "member")
		})
	}
}

// TestMatrixSpec_CoversEveryPermission keeps the table above honest: a new
// permission that nobody transcribed into matrixSpec fails here rather than
// shipping unasserted.
func TestMatrixSpec_CoversEveryPermission(t *testing.T) {
	asserted := make(map[Permission]bool, len(matrixSpec))
	for _, tc := range matrixSpec {
		require.False(t, asserted[tc.perm], "permission %q asserted twice in matrixSpec", tc.perm)
		asserted[tc.perm] = true
	}

	for _, perm := range All() {
		assert.True(t, asserted[perm], "permission %q is missing from matrixSpec", perm)
	}
	assert.Len(t, matrixSpec, len(All()), "matrixSpec asserts a permission that All() does not list")
}

// TestAllowed_UnknownRoleFailsClosed pins the fail-closed contract: anything
// that is not one of the three known roles holds nothing.
func TestAllowed_UnknownRoleFailsClosed(t *testing.T) {
	for _, role := range []models.TeamMemberRole{"", "root", "Owner", "OWNER"} {
		t.Run(string(role), func(t *testing.T) {
			for _, perm := range All() {
				assert.False(t, Allowed(role, perm), "role %q must not hold %q", role, perm)
			}
			assert.Empty(t, RolePermissions(role))
			assert.NotNil(t, RolePermissions(role), "must be an empty slice, never nil")
		})
	}
}

func TestRolePermissions_MatchesMatrixAndIsOrdered(t *testing.T) {
	tests := []struct {
		role models.TeamMemberRole
		want []Permission
	}{
		{
			role: models.TeamMemberRoleOwner,
			want: []Permission{
				TeamUpdate, TeamDelete, OwnershipTransfer,
				MemberInvite, MemberRemove, MemberRoleUpdate,
				ProjectCreate, ProjectUpdate, ProjectDelete,
				ResourceCreate, ResourceUpdateAny, ResourceDeleteOwn, ResourceDeleteAny,
				FeedItemDeleteAny,
			},
		},
		{
			role: models.TeamMemberRoleAdmin,
			want: []Permission{
				TeamUpdate,
				MemberInvite, MemberRemove, MemberRoleUpdate,
				ProjectCreate, ProjectUpdate, ProjectDelete,
				ResourceCreate, ResourceUpdateAny, ResourceDeleteOwn, ResourceDeleteAny,
				FeedItemDeleteAny,
			},
		},
		{
			role: models.TeamMemberRoleMember,
			want: []Permission{
				ResourceCreate, ResourceUpdateAny, ResourceDeleteOwn,
			},
		},
	}

	for _, tc := range tests {
		t.Run(string(tc.role), func(t *testing.T) {
			// Exact and ordered: the order is API-visible via the team
			// `permissions` field, so it must be deterministic.
			assert.Equal(t, tc.want, RolePermissions(tc.role))
		})
	}
}

// TestRolePermissionStrings_MirrorsRolePermissions pins the wire rendering of
// a role's grants (#224): same membership, same order, just strings. Clients
// gate their UI on these exact values, so this is the mapping that must not
// drift; that they are the *documented* values is asserted against the
// OpenAPI enum in internal/server.
func TestRolePermissionStrings_MirrorsRolePermissions(t *testing.T) {
	for _, role := range []models.TeamMemberRole{
		models.TeamMemberRoleOwner,
		models.TeamMemberRoleAdmin,
		models.TeamMemberRoleMember,
	} {
		t.Run(string(role), func(t *testing.T) {
			perms := RolePermissions(role)
			want := make([]string, 0, len(perms))
			for _, perm := range perms {
				want = append(want, string(perm))
			}
			require.NotEmpty(t, want, "every real role grants something")

			assert.Equal(t, want, RolePermissionStrings(role))
		})
	}
}

// TestRolePermissionStrings_UnknownRoleIsEmptyNotNil carries RolePermissions'
// fail-closed, never-nil contract through the []string conversion: the field is
// a *required* array, so a nil here would serialize as null and break the
// documented payload (#125 Layer C).
func TestRolePermissionStrings_UnknownRoleIsEmptyNotNil(t *testing.T) {
	for _, role := range []models.TeamMemberRole{"", "root", "Owner", "OWNER"} {
		t.Run(string(role), func(t *testing.T) {
			got := RolePermissionStrings(role)

			assert.Empty(t, got, "an unknown role grants nothing")
			assert.NotNil(t, got, "must be an empty slice, never nil")
		})
	}
}

// TestAll_IsACopy guards the exported catalog against mutation by a caller.
func TestAll_IsACopy(t *testing.T) {
	first := All()
	require.NotEmpty(t, first)
	first[0] = "mutated"

	assert.NotEqual(t, Permission("mutated"), All()[0])
}
