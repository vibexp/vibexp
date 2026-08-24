package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/vibexp/vibexp/internal/authz"
)

// ErrCopySourceRequired is returned when a cross-team copy names no source
// team. The handler maps it to 400.
var ErrCopySourceRequired = errors.New("source team is required")

// ErrCopySourceIsDestination is returned when a cross-team copy names the
// destination as its own source. The handler maps it to 400: the request is
// well-formed but meaningless — every slug would collide with itself and the
// whole set would be reported as skipped, which reads like a bug rather than a
// no-op.
var ErrCopySourceIsDestination = errors.New("source team must differ from the destination team")

// CrossTeamCopyMembershipOnly is the sentinel passed to AuthorizeCrossTeamCopy
// for a surface whose create path authorizes on team membership alone, with no
// role dimension — custom types today (#829). It is deliberately the zero
// Permission: no permission constant exists for "is a member", because
// membership is not a cell in the authz matrix (see internal/authz/matrix.go,
// "Viewing the team ... carries no role dimension"), and inventing one would
// widen the published team `permissions` array, which is API surface pinned by
// TestTeamPermissionsEnumMatchesAuthzConstants.
const CrossTeamCopyMembershipOnly authz.Permission = ""

// AuthorizeCrossTeamCopy is the single authorization point for every
// settings-copy endpoint in epic #827 — custom types (#829), model providers
// (#830) and embedding providers (#831).
//
// It is the first operation in the codebase that touches two teams in one
// request, and the rules it encodes are the reason it is shared rather than
// re-derived per surface:
//
//   - Whatever the destination's own create path requires is required on BOTH
//     teams. A copy moves a configuration's use into a different set of
//     members, so "may read the source" is not the bar — "may configure the
//     source" is.
//   - The DESTINATION is evaluated first. ErrPermissionDenied does not
//     distinguish non-membership from insufficient role, so both teams deny
//     identically and a caller learns nothing about a team they do not belong
//     to — including whether it exists.
//   - perm == CrossTeamCopyMembershipOnly checks membership on both teams
//     without consulting the matrix, for surfaces that authorize on membership
//     alone.
//
// A denial wraps ErrPermissionDenied; a storage failure is propagated
// unchanged, so an unreachable database surfaces as 500 rather than 403.
func AuthorizeCrossTeamCopy(
	ctx context.Context,
	authorizer AuthorizationServiceInterface,
	userID, destinationTeamID, sourceTeamID string,
	perm authz.Permission,
) error {
	if sourceTeamID == "" {
		return ErrCopySourceRequired
	}
	if sourceTeamID == destinationTeamID {
		return ErrCopySourceIsDestination
	}

	check := func(teamID string) error {
		if perm == CrossTeamCopyMembershipOnly {
			return authorizer.IsMember(ctx, userID, teamID)
		}
		return authorizer.Can(ctx, userID, teamID, perm)
	}

	// Destination first — see the doc comment. Both branches return the same
	// sentinel, and neither names the team, so the two are indistinguishable to
	// the caller.
	if err := check(destinationTeamID); err != nil {
		if errors.Is(err, ErrPermissionDenied) {
			return fmt.Errorf("%w: cross-team copy requires access to both teams", ErrPermissionDenied)
		}
		return err
	}
	if err := check(sourceTeamID); err != nil {
		if errors.Is(err, ErrPermissionDenied) {
			return fmt.Errorf("%w: cross-team copy requires access to both teams", ErrPermissionDenied)
		}
		return err
	}
	return nil
}
