package models

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// auditOperationPattern is the dotted <domain>.<verb> shape every operation
// must have: lowercase words joined by underscores, at least two segments.
var auditOperationPattern = regexp.MustCompile(`^[a-z_]+(\.[a-z_]+)+$`)

func TestAllAuditOperations_AreUniqueAndDotted(t *testing.T) {
	ops := AllAuditOperations()
	require.NotEmpty(t, ops)

	seen := make(map[AuditOperation]bool, len(ops))
	for _, op := range ops {
		assert.Regexp(t, auditOperationPattern, string(op))
		assert.False(t, seen[op], "duplicate operation %q", op)
		seen[op] = true
		assert.True(t, op.IsValid(), "%q must be valid", op)
	}
}

// TestAllAuditOperations_CoversTheEpic pins the vocabulary the feature lanes
// of epic #1510 record and the admin API publishes as its enum. Removing or
// renaming one is a breaking change to that enum.
func TestAllAuditOperations_CoversTheEpic(t *testing.T) {
	want := []string{
		"prompt.create", "prompt.update", "prompt.move", "prompt.delete", "prompt.lock", "prompt.unlock",
		"blueprint.create", "blueprint.update", "blueprint.move",
		"blueprint.delete", "blueprint.lock", "blueprint.unlock",
		"artifact.create", "artifact.update", "artifact.move",
		"artifact.delete", "artifact.lock", "artifact.unlock",
		"memory.create", "memory.update", "memory.move", "memory.delete",
		"feed.create", "feed.update", "feed.delete",
		"feed_item.create", "feed_item.delete", "feed_item.archive", "feed_item.unarchive",
		"project.create", "project.update", "project.delete", "project.migrated",
		"auth.login", "auth.login_failed", "auth.logout",
		"auth.setup_session.started", "auth.sessions.revoked",
		"auth.api_key.created", "auth.api_key.revoked", "auth.api_access.revoked",
		"auth.password.set", "auth.password.changed", "auth.password.removed",
		"auth.password.reset_requested", "auth.password.reset_link_issued",
		"auth.identity.linked", "auth.identity.unlinked",
		"auth.mcp_client.registered", "auth.mcp_consent.granted",
		"auth.mcp_consent.denied", "auth.mcp_grant.revoked",
		"team.created", "team.updated", "team.deleted",
		"team.invite.sent", "team.invite.accepted", "team.invite.rejected", "team.invite.revoked",
		"team.member.added", "team.member.role_changed", "team.member.removed",
		"team.ownership.transferred", "team.settings.updated",
		"instance.settings.updated",
		"admin.user.created", "admin.user.updated", "admin.user.suspended",
		"admin.user.unsuspended", "admin.user.deleted",
		"authz.denied", "audit.export", "audit.settings.updated",
	}

	got := make([]string, 0, len(want))
	for _, op := range AllAuditOperations() {
		got = append(got, string(op))
	}
	assert.ElementsMatch(t, want, got)
}

func TestAllAuditOperations_ReturnsACopy(t *testing.T) {
	ops := AllAuditOperations()
	ops[0] = "tampered.op"

	assert.Equal(t, AuditOpPromptCreate, AllAuditOperations()[0])
}

func TestAuditEnums_IsValid(t *testing.T) {
	assert.False(t, AuditOperation("prompt.read").IsValid())
	assert.False(t, AuditOperation("").IsValid())

	for _, v := range []AuditActorType{AuditActorUser, AuditActorSystem, AuditActorAnonymous} {
		assert.True(t, v.IsValid(), v)
	}
	assert.False(t, AuditActorType("robot").IsValid())

	for _, v := range []AuditSource{AuditSourceWeb, AuditSourceCLI, AuditSourceMCP, AuditSourceAPI, AuditSourceSystem} {
		assert.True(t, v.IsValid(), v)
	}
	assert.False(t, AuditSource("unknown").IsValid())

	for _, v := range []AuditOutcome{AuditOutcomeSuccess, AuditOutcomeDenied, AuditOutcomeFailure} {
		assert.True(t, v.IsValid(), v)
	}
	assert.False(t, AuditOutcome("error").IsValid())
}
