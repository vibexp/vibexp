package services

import (
	"context"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
)

// allowAllAuthz authorizes every permission. It is shared by the domain service
// tests (project, prompt, agent) that predate RBAC and cover resource mechanics
// rather than authorization: they run as a caller who is allowed, and the matrix
// itself is asserted in the per-domain *_rbac_test.go files.
//
// Authorize is left unimplemented on purpose — no service reaches for it through
// this double today, so an unexpected call should fail loudly rather than
// silently allow.
type allowAllAuthz struct{}

func (allowAllAuthz) Can(_ context.Context, _, _ string, _ authz.Permission) error { return nil }

func (allowAllAuthz) CanActOnResource(
	_ context.Context, _, _, _ string, _, _ authz.Permission,
) error {
	return nil
}

func (allowAllAuthz) Authorize(
	_ context.Context, _, _ string, _ authz.Permission,
) (models.TeamMemberRole, error) {
	panic("allowAllAuthz: unexpected Authorize call — give this test an explicit authz double")
}
