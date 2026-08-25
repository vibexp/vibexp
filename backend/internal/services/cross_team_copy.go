package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

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

// --- Copy naming (shared by the provider copy endpoints) ---------------------

// copyNameSuffix is appended to a source provider's name when the destination
// team already holds it: " (copy)", then " (copy 2)", " (copy 3)", …
const copyNameSuffix = "copy"

// copyNameMaxAttempts bounds the disambiguation loop. The candidates come from
// a snapshot of the destination's names, so the loop is finite by construction;
// the bound only stops a pathological team (hundreds of literal "X (copy N)"
// rows) from spinning, and failing loudly beats inventing "X (copy 741)".
const copyNameMaxAttempts = 100

// providerNameMaxLen mirrors model_providers.name and embedding_providers.name —
// both varchar(255). A suffix pushing past it would be truncated by Postgres
// into a name that no longer matches what the collision check cleared, so the
// BASE is trimmed instead and the suffix always survives intact.
//
// Postgres counts varchar(n) in CHARACTERS, not bytes, so the trimming below
// works in runes: measuring in bytes would both over-trim a non-ASCII name and,
// worse, cut mid-rune into invalid UTF-8 that Postgres rejects outright.
const providerNameMaxLen = 255

// ErrCopyNameExhausted reports that neither the source name nor any of the
// generated variants of it is free in the destination team. Each surface wraps
// it in its own already-exists sentinel so the handler keeps mapping to 409.
var ErrCopyNameExhausted = errors.New("no free name for the copy")

// resolveCopyName decides what a copied provider is called, given a snapshot of
// the destination team's existing names.
//
// A caller-supplied override wins outright and is used verbatim — the caller has
// seen the destination's providers and made a choice, so silently renaming it
// would be worse than the 409 a collision earns. Only a name INHERITED from the
// source is disambiguated, because the caller never chose it.
//
// Pure over its inputs so the numbering and the varchar trimming are testable
// without a repository, and shared so the two provider surfaces cannot drift.
func resolveCopyName(override *string, sourceName string, existing []string) (string, error) {
	if override != nil {
		return *override, nil
	}

	taken := make(map[string]struct{}, len(existing))
	for _, name := range existing {
		taken[name] = struct{}{}
	}

	if _, clash := taken[sourceName]; !clash {
		return sourceName, nil
	}

	for attempt := 1; attempt <= copyNameMaxAttempts; attempt++ {
		candidate := copyNameCandidate(sourceName, attempt)
		if _, clash := taken[candidate]; !clash {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("%w: %s (and %d generated variants of it)",
		ErrCopyNameExhausted, sourceName, copyNameMaxAttempts)
}

// copyNameCandidate builds the nth disambiguated name: attempt 1 is
// "<base> (copy)", attempt 2 "<base> (copy 2)", and so on — the numbering a
// reader expects, where the unnumbered form IS the first copy.
func copyNameCandidate(base string, attempt int) string {
	suffix := " (" + copyNameSuffix + ")"
	if attempt > 1 {
		suffix = " (" + copyNameSuffix + " " + strconv.Itoa(attempt) + ")"
	}

	runes := []rune(base)
	if overflow := len(runes) + len([]rune(suffix)) - providerNameMaxLen; overflow > 0 {
		base = strings.TrimRight(string(runes[:max(len(runes)-overflow, 0)]), " ")
	}
	return base + suffix
}
