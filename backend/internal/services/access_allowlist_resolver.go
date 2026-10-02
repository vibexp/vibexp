package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Access allowlist (#1235, epic #1230, decision 9).
//
// The allowlist restricts who may use the instance, by email domain and by
// exact email. It is stored in instance_auth_allowlist and resolved at runtime,
// so tightening it takes effect without a restart: at sign-in, at MCP consent,
// and on every authenticated request of an already signed-in user.

const (
	// allowlistVersionProbeInterval is how long a compiled allowlist is served
	// from memory before the shared auth settings version is read again. It
	// bounds how long a replica keeps enforcing a list that was just changed:
	// a removal takes effect within this interval, on every replica.
	allowlistVersionProbeInterval = 2 * time.Second
	// allowlistRefreshTimeout bounds one refresh (the version read plus, when
	// the version changed, the allowlist read).
	allowlistRefreshTimeout = 5 * time.Second
	// AllowlistImpactSampleLimit caps the emails PreviewAllowlistImpact returns.
	AllowlistImpactSampleLimit = 20
)

// AllowlistImpact is what tightening the allowlist to a candidate would do: the
// active users who would stop matching it.
type AllowlistImpact struct {
	// Count is how many active (non-suspended) users match neither list of the
	// candidate. Root instance admins are exempt from the allowlist and are
	// never counted.
	Count int
	// Sample holds up to AllowlistImpactSampleLimit of their emails, in
	// alphabetical order.
	Sample []string
	// SampleTruncated reports whether Count exceeds len(Sample).
	SampleTruncated bool
}

// AccessAllowlistResolver decides whether an email may use the instance, from
// the allowlist stored in instance_auth_allowlist.
//
// # Caching
//
// The compiled allowlist is cached by the shared auth settings version. The
// version is read at most once per allowlistVersionProbeInterval; in between,
// decisions are served from memory with no database read. An unchanged version
// reuses the compiled list without reading the allowlist table.
//
// # When the allowlist cannot be read
//
// A failed version or allowlist read is fail-CLOSED only while the last list
// this replica compiled is active: IsEmailAllowed then returns the error and
// the caller must refuse. Serving the stale list instead would keep admitting a
// user who may just have been removed. When no list was ever compiled, or the
// last one was open access, the read failure is logged and the email is allowed
// (fail-OPEN): an instance with no allowlist must not start refusing everyone
// because of a database error.
//
// # Root instance admins
//
// A root admin (auth.instance_admins) is exempt: they are allowed even when
// their email matches no entry, and even when the allowlist cannot be read, so
// an allowlist mistake can never lock out the trust root. DB-granted admins are
// not exempt.
type AccessAllowlistResolver interface {
	// IsEmailAllowed reports whether email may use the instance, and whether an
	// allowlist is active (at least one domain or email stored). With no active
	// allowlist every email is allowed. A non-nil error means the decision
	// could not be made and the caller must fail closed.
	//
	// It says nothing about whether the address was provider-verified; sign-in
	// applies that rule itself while active is true (#218).
	IsEmailAllowed(ctx context.Context, email string) (allowed, active bool, err error)
	// PreviewAllowlistImpact reports the active users who would stop matching
	// if candidate replaced the stored allowlist. candidate is validated and
	// normalized first; an invalid one returns ErrInvalidInstanceAuthAllowlist.
	// An open-access candidate affects nobody. Nothing is stored.
	PreviewAllowlistImpact(ctx context.Context, candidate models.InstanceAuthAllowlist) (*AllowlistImpact, error)
}

// AccessAllowlistResolverDeps are the collaborators of the resolver.
type AccessAllowlistResolverDeps struct {
	Allowlists repositories.InstanceAuthAllowlistRepository
	Versions   repositories.InstanceAuthSettingsVersionRepository
	// RootAdmins is auth.instance_admins, the emails exempt from the allowlist.
	RootAdmins []string
	Logger     *slog.Logger
	// ProbeInterval overrides how long a compiled allowlist is served before
	// the auth settings version is read again. Zero means
	// allowlistVersionProbeInterval; production wiring leaves it zero.
	ProbeInterval time.Duration
}

// compiledAllowlist is one immutable, matchable allowlist and the auth settings
// version it was compiled at.
type compiledAllowlist struct {
	version int64
	domains map[string]struct{}
	emails  map[string]struct{}
	// probedAt is when the version was last confirmed.
	probedAt time.Time
	// exempted records the root admins already logged as exempt from this list,
	// so the exemption is logged once per admin per compiled list, not once per
	// request.
	exempted *sync.Map
}

// compileAllowlist normalizes (trim, lower-case) and de-duplicates both lists.
// Blank entries are dropped.
func compileAllowlist(version int64, domains, emails []string) *compiledAllowlist {
	return &compiledAllowlist{
		version:  version,
		domains:  normalizedSet(domains),
		emails:   normalizedSet(emails),
		exempted: &sync.Map{},
	}
}

func normalizedSet(items []string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		if n := normalizeAllowlistEntry(item); n != "" {
			set[n] = struct{}{}
		}
	}
	return set
}

func normalizeAllowlistEntry(entry string) string {
	return strings.ToLower(strings.TrimSpace(entry))
}

// active reports whether the list restricts access. With both lists empty the
// instance is open and every email is allowed.
func (c *compiledAllowlist) active() bool {
	return len(c.domains) > 0 || len(c.emails) > 0
}

// match reports whether email is admitted by an ACTIVE list: its normalized
// form is a listed email, or its domain, the part after the LAST "@", is a
// listed domain. Matching is exact: no subdomain or substring matching, so
// "a@sub.example.com" and "a@evil-example.com" do not match domain
// "example.com". An input without an "@" matches no domain.
func (c *compiledAllowlist) match(email string) bool {
	email = normalizeAllowlistEntry(email)
	if _, ok := c.emails[email]; ok {
		return true
	}
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	_, ok := c.domains[email[at+1:]]
	return ok
}

// accessAllowlistResolver implements AccessAllowlistResolver.
type accessAllowlistResolver struct {
	allowlists repositories.InstanceAuthAllowlistRepository
	versions   repositories.InstanceAuthSettingsVersionRepository
	rootAdmins map[string]struct{}
	logger     *slog.Logger

	// Injectable for tests.
	now           func() time.Time
	probeInterval time.Duration

	snap  atomic.Pointer[compiledAllowlist]
	group singleflight.Group
}

var _ AccessAllowlistResolver = (*accessAllowlistResolver)(nil)

// NewAccessAllowlistResolver creates the runtime access allowlist resolver.
func NewAccessAllowlistResolver(deps AccessAllowlistResolverDeps) AccessAllowlistResolver {
	return newAccessAllowlistResolver(deps)
}

func newAccessAllowlistResolver(deps AccessAllowlistResolverDeps) *accessAllowlistResolver {
	probeInterval := deps.ProbeInterval
	if probeInterval == 0 {
		probeInterval = allowlistVersionProbeInterval
	}
	return &accessAllowlistResolver{
		allowlists:    deps.Allowlists,
		versions:      deps.Versions,
		rootAdmins:    normalizedSet(deps.RootAdmins),
		logger:        deps.Logger,
		now:           time.Now,
		probeInterval: probeInterval,
	}
}

func (r *accessAllowlistResolver) isRootAdmin(email string) bool {
	_, ok := r.rootAdmins[normalizeAllowlistEntry(email)]
	return ok
}

// IsEmailAllowed implements AccessAllowlistResolver.
func (r *accessAllowlistResolver) IsEmailAllowed(ctx context.Context, email string) (allowed, active bool, err error) {
	list, err := r.current(ctx)
	if err != nil {
		// current returns an error only while the last known list is active.
		if r.isRootAdmin(email) {
			r.logger.With("email", normalizeAllowlistEntry(email), "error", err.Error()).
				Info("Root instance admin is exempt from the access allowlist, which could not be read")
			return true, true, nil
		}
		return false, true, err
	}
	if !list.active() {
		return true, false, nil
	}
	if list.match(email) {
		return true, true, nil
	}
	if r.isRootAdmin(email) {
		normalized := normalizeAllowlistEntry(email)
		if _, logged := list.exempted.LoadOrStore(normalized, struct{}{}); !logged {
			r.logger.With("email", normalized).
				Info("Root instance admin is exempt from the access allowlist their email does not match")
		}
		return true, true, nil
	}
	return false, true, nil
}

// current returns the compiled allowlist to decide with. It serves the cached
// list while its version was confirmed within probeInterval and refreshes it
// otherwise. A failed refresh returns an error only when the cached list is
// active; see the AccessAllowlistResolver doc comment.
func (r *accessAllowlistResolver) current(ctx context.Context) (*compiledAllowlist, error) {
	if s := r.snap.Load(); r.fresh(s) {
		return s, nil
	}
	// One refresh at a time, shared by every caller that arrives meanwhile. It
	// runs detached from the caller's cancellation, so one aborted request
	// cannot fail the refresh for everyone waiting on it.
	ch := r.group.DoChan("refresh", func() (any, error) {
		if s := r.snap.Load(); r.fresh(s) {
			return s, nil
		}
		refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), allowlistRefreshTimeout)
		defer cancel()
		return r.refresh(refreshCtx)
	})
	var refreshErr error
	select {
	case <-ctx.Done():
		refreshErr = ctx.Err()
	case res := <-ch:
		if res.Err == nil {
			return res.Val.(*compiledAllowlist), nil
		}
		refreshErr = res.Err
	}
	return r.afterFailedRefresh(refreshErr)
}

// fresh reports whether s can be served without probing the version.
func (r *accessAllowlistResolver) fresh(s *compiledAllowlist) bool {
	return s != nil && r.now().Before(s.probedAt.Add(r.probeInterval))
}

// refresh reads the auth settings version and recompiles the allowlist only
// when it changed. An unchanged version re-stamps the cached list and reads
// nothing else.
func (r *accessAllowlistResolver) refresh(ctx context.Context) (*compiledAllowlist, error) {
	version, err := r.versions.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("read auth settings version: %w", err)
	}
	if s := r.snap.Load(); s != nil && s.version == version {
		confirmed := *s
		confirmed.probedAt = r.now()
		r.snap.Store(&confirmed)
		return &confirmed, nil
	}

	var domains, emails []string
	stored, err := r.allowlists.Get(ctx)
	switch {
	case errors.Is(err, repositories.ErrInstanceAuthAllowlistNotFound):
		// None stored: open access.
	case err != nil:
		return nil, fmt.Errorf("read access allowlist: %w", err)
	default:
		domains, emails = stored.Domains, stored.Emails
	}
	compiled := compileAllowlist(version, domains, emails)
	compiled.probedAt = r.now()
	r.snap.Store(compiled)
	r.logger.With(
		"version", version, "domains", len(compiled.domains), "emails", len(compiled.emails),
	).Info("Compiled the access allowlist")
	return compiled, nil
}

// afterFailedRefresh applies the fail-closed/fail-open split to a refresh that
// failed with err.
func (r *accessAllowlistResolver) afterFailedRefresh(err error) (*compiledAllowlist, error) {
	if s := r.snap.Load(); s != nil && s.active() {
		return nil, fmt.Errorf("access allowlist is active and could not be re-read: %w", err)
	}
	r.logger.With("error", err.Error()).
		Warn("Access allowlist could not be read; no allowlist is known to be active, allowing access")
	return compileAllowlist(0, nil, nil), nil
}

// PreviewAllowlistImpact implements AccessAllowlistResolver.
func (r *accessAllowlistResolver) PreviewAllowlistImpact(
	ctx context.Context, candidate models.InstanceAuthAllowlist,
) (*AllowlistImpact, error) {
	domains, emails, err := ValidateInstanceAuthAllowlist(candidate.Domains, candidate.Emails)
	if err != nil {
		return nil, err
	}
	if len(domains) == 0 && len(emails) == 0 {
		// Open access admits everyone.
		return &AllowlistImpact{Sample: []string{}}, nil
	}
	exempt := make([]string, 0, len(r.rootAdmins))
	for email := range r.rootAdmins {
		exempt = append(exempt, email)
	}
	count, sample, err := r.allowlists.CountUsersOutside(ctx, domains, emails, exempt, AllowlistImpactSampleLimit)
	if err != nil {
		return nil, fmt.Errorf("preview access allowlist impact: %w", err)
	}
	if sample == nil {
		sample = []string{}
	}
	return &AllowlistImpact{Count: count, Sample: sample, SampleTruncated: count > len(sample)}, nil
}
