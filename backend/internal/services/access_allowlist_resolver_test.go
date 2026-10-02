package services

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// fakeAllowlistStore is an in-memory instance_auth_allowlist plus the shared
// auth settings version. It counts reads, so tests can prove what the resolver
// did and did not touch.
type fakeAllowlistStore struct {
	repositories.InstanceAuthAllowlistRepository

	mu           sync.Mutex
	row          *models.InstanceAuthAllowlist
	version      int64
	versionErr   error
	allowlistErr error
	versionReads int
	rowReads     int

	previewArgs  []any
	previewCount int
	previewEmail []string
	previewErr   error
}

// set replaces the stored allowlist and bumps the version, as every audited
// write does. Both lists empty removes the row.
func (f *fakeAllowlistStore) set(domains, emails []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.version++
	if len(domains) == 0 && len(emails) == 0 {
		f.row = nil
		return
	}
	f.row = &models.InstanceAuthAllowlist{Domains: domains, Emails: emails}
}

func (f *fakeAllowlistStore) fail(versionErr, allowlistErr error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.versionErr, f.allowlistErr = versionErr, allowlistErr
}

func (f *fakeAllowlistStore) reads() (versionReads, rowReads int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.versionReads, f.rowReads
}

func (f *fakeAllowlistStore) Get(context.Context) (*models.InstanceAuthAllowlist, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rowReads++
	if f.allowlistErr != nil {
		return nil, f.allowlistErr
	}
	if f.row == nil {
		return nil, repositories.ErrInstanceAuthAllowlistNotFound
	}
	return f.row, nil
}

func (f *fakeAllowlistStore) CountUsersOutside(
	_ context.Context, domains, emails, exempt []string, limit int,
) (int, []string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.previewArgs = []any{domains, emails, exempt, limit}
	return f.previewCount, f.previewEmail, f.previewErr
}

// fakeAllowlistVersions reads the store's version.
type fakeAllowlistVersions struct{ store *fakeAllowlistStore }

func (v fakeAllowlistVersions) Get(context.Context) (int64, error) {
	v.store.mu.Lock()
	defer v.store.mu.Unlock()
	v.store.versionReads++
	return v.store.version, v.store.versionErr
}

// testClock is a manually advanced clock.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// allowlistHarness is the real resolver over the in-memory store.
type allowlistHarness struct {
	resolver *accessAllowlistResolver
	store    *fakeAllowlistStore
	clock    *testClock
	logs     *logtest.Recorder
}

func newAllowlistHarness(rootAdmins ...string) *allowlistHarness {
	logger, logs := logtest.New()
	store := &fakeAllowlistStore{version: 1}
	clock := &testClock{now: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)}
	resolver := newAccessAllowlistResolver(AccessAllowlistResolverDeps{
		Allowlists: store,
		Versions:   fakeAllowlistVersions{store},
		RootAdmins: rootAdmins,
		Logger:     logger,
	})
	resolver.now = clock.Now
	return &allowlistHarness{resolver: resolver, store: store, clock: clock, logs: logs}
}

// expire moves the clock past the version probe interval, so the next decision
// probes the version again.
func (h *allowlistHarness) expire() { h.clock.Advance(allowlistVersionProbeInterval) }

func (h *allowlistHarness) logged(message string) int {
	n := 0
	for _, e := range h.logs.AllEntries() {
		if e.Message == message {
			n++
		}
	}
	return n
}

// newStaticAllowlistResolver returns the real resolver over a fixed allowlist
// of exact emails (none means open access), for tests of its consumers.
func newStaticAllowlistResolver(logger *slog.Logger, emails []string) AccessAllowlistResolver {
	store := &fakeAllowlistStore{version: 1}
	if len(emails) > 0 {
		store.row = &models.InstanceAuthAllowlist{Emails: emails}
	}
	return NewAccessAllowlistResolver(AccessAllowlistResolverDeps{
		Allowlists: store,
		Versions:   fakeAllowlistVersions{store},
		Logger:     logger,
	})
}

func mustAllow(t *testing.T, r AccessAllowlistResolver, email string) (allowed, active bool) {
	t.Helper()
	allowed, active, err := r.IsEmailAllowed(context.Background(), email)
	require.NoError(t, err)
	return allowed, active
}

func TestCompiledAllowlist_Match(t *testing.T) {
	t.Run("exact emails, case- and whitespace-insensitive", func(t *testing.T) {
		list := compileAllowlist(1, nil, []string{"Test@Example.Com", "  admin@example.com  ", ""})
		require.True(t, list.active())
		for email, want := range map[string]bool{
			"test@example.com":         true,
			"TEST@EXAMPLE.COM":         true,
			"  admin@example.com  ":    true,
			"aDmIn@ExAmPlE.cOm":        true,
			"unauthorized@example.com": false,
			"":                         false,
		} {
			assert.Equal(t, want, list.match(email), "email %q", email)
		}
	})

	t.Run("domains match exactly on the part after the last @", func(t *testing.T) {
		list := compileAllowlist(1, []string{"Example.com", "  vibexp.io  "}, nil)
		for _, tc := range []struct {
			name, email string
			want        bool
		}{
			{"exact domain match", "alice@example.com", true},
			{"exact domain match, second domain", "bob@vibexp.io", true},
			{"domain match, uppercase input", "ALICE@EXAMPLE.COM", true},
			{"domain match, surrounding whitespace", "  alice@example.com  ", true},
			{"subdomain must NOT match", "a@sub.vibexp.io", false},
			{"lookalike domain must NOT match", "a@evil-vibexp.io", false},
			{"superstring domain must NOT match", "a@vibexp.io.attacker.com", false},
			{"unrelated domain rejected", "a@other.com", false},
			{"no @ matches no domain", "not-an-email", false},
			{"a bare domain is not an email at that domain", "example.com", false},
			{"domain matched on last @ (multiple @)", `"a@b"@example.com`, true},
			{"trailing @ (empty domain) denied", "alice@", false},
		} {
			assert.Equal(t, tc.want, list.match(tc.email), tc.name)
		}
	})

	t.Run("either list admits", func(t *testing.T) {
		list := compileAllowlist(1, []string{"example.com"}, []string{"special@other.com"})
		assert.True(t, list.match("anyone@example.com"))
		assert.True(t, list.match("special@other.com"))
		assert.False(t, list.match("nobody@other.com"))
	})

	t.Run("duplicates and blanks compile away", func(t *testing.T) {
		list := compileAllowlist(1, []string{"example.com", " EXAMPLE.com ", " "}, []string{"", "  "})
		assert.Len(t, list.domains, 1)
		assert.Empty(t, list.emails)
		assert.True(t, list.active())
		assert.False(t, compileAllowlist(1, []string{" "}, nil).active(), "only blanks is open access")
	})
}

func TestAccessAllowlistResolver_OpenAccess(t *testing.T) {
	h := newAllowlistHarness()

	allowed, active := mustAllow(t, h.resolver, "anyone@anywhere.test")
	assert.True(t, allowed)
	assert.False(t, active, "no row stored is open access")

	h.store.mu.Lock()
	h.store.row = &models.InstanceAuthAllowlist{Domains: []string{}, Emails: []string{}}
	h.store.version++
	h.store.mu.Unlock()
	h.expire()
	allowed, active = mustAllow(t, h.resolver, "anyone@anywhere.test")
	assert.True(t, allowed)
	assert.False(t, active, "a stored row with both lists empty is open access too")
}

func TestAccessAllowlistResolver_TighteningTakesEffectWithinTheProbeInterval(t *testing.T) {
	h := newAllowlistHarness()
	h.store.set([]string{"example.com"}, []string{"guest@other.com"})

	allowed, active := mustAllow(t, h.resolver, "guest@other.com")
	assert.True(t, allowed)
	assert.True(t, active)

	// The guest is removed. Within the probe interval this replica still serves
	// the list it compiled; after it, the next decision sees the new version.
	h.store.set([]string{"example.com"}, nil)
	allowed, _ = mustAllow(t, h.resolver, "guest@other.com")
	assert.True(t, allowed, "served from memory inside the probe interval")

	h.expire()
	allowed, active = mustAllow(t, h.resolver, "guest@other.com")
	assert.False(t, allowed, "removal is enforced once the version is probed again")
	assert.True(t, active)
	allowed, _ = mustAllow(t, h.resolver, "dev@example.com")
	assert.True(t, allowed)

	// Clearing the allowlist reopens the instance.
	h.store.set(nil, nil)
	h.expire()
	allowed, active = mustAllow(t, h.resolver, "guest@other.com")
	assert.True(t, allowed)
	assert.False(t, active)
}

func TestAccessAllowlistResolver_Cache(t *testing.T) {
	h := newAllowlistHarness()
	h.store.set([]string{"example.com"}, nil)

	for range 50 {
		mustAllow(t, h.resolver, "dev@example.com")
	}
	versionReads, rowReads := h.store.reads()
	assert.Equal(t, 1, versionReads, "decisions inside the probe interval read nothing")
	assert.Equal(t, 1, rowReads)

	// An unchanged version is confirmed with one version read and no recompile:
	// the allowlist table is not read again.
	for range 3 {
		h.expire()
		mustAllow(t, h.resolver, "dev@example.com")
	}
	versionReads, rowReads = h.store.reads()
	assert.Equal(t, 4, versionReads)
	assert.Equal(t, 1, rowReads, "an unchanged version must not re-read the allowlist")
	assert.Equal(t, 1, h.logged("Compiled the access allowlist"))

	// A changed version recompiles exactly once.
	h.store.set([]string{"example.org"}, nil)
	h.expire()
	mustAllow(t, h.resolver, "dev@example.com")
	mustAllow(t, h.resolver, "dev@example.com")
	versionReads, rowReads = h.store.reads()
	assert.Equal(t, 5, versionReads)
	assert.Equal(t, 2, rowReads)
	assert.Equal(t, 2, h.logged("Compiled the access allowlist"))
}

func TestAccessAllowlistResolver_ConcurrentCallersShareOneRefresh(t *testing.T) {
	h := newAllowlistHarness()
	h.store.set([]string{"example.com"}, nil)

	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			allowed, _, err := h.resolver.IsEmailAllowed(context.Background(), "dev@example.com")
			assert.NoError(t, err)
			assert.True(t, allowed)
		})
	}
	wg.Wait()
	_, rowReads := h.store.reads()
	assert.Equal(t, 1, rowReads)
}

// TestAccessAllowlistResolver_ReadFailure pins the fail-closed/fail-open split
// documented on AccessAllowlistResolver.
func TestAccessAllowlistResolver_ReadFailure(t *testing.T) {
	dbDown := errors.New("db down")
	failures := map[string]func(*fakeAllowlistStore){
		"version read fails":   func(s *fakeAllowlistStore) { s.fail(dbDown, nil) },
		"allowlist read fails": func(s *fakeAllowlistStore) { s.set([]string{"changed.example"}, nil); s.fail(nil, dbDown) },
	}

	for name, breakStore := range failures {
		t.Run(name+": fails closed while the last known list is active", func(t *testing.T) {
			h := newAllowlistHarness()
			h.store.set([]string{"example.com"}, nil)
			mustAllow(t, h.resolver, "dev@example.com") // compile the active list

			breakStore(h.store)
			h.expire()
			allowed, active, err := h.resolver.IsEmailAllowed(context.Background(), "dev@example.com")
			require.ErrorIs(t, err, dbDown)
			assert.False(t, allowed, "even an email the stale list admits is refused")
			assert.True(t, active)

			// It recovers as soon as the store does.
			h.store.fail(nil, nil)
			h.store.set([]string{"example.com"}, nil)
			allowed, _ = mustAllow(t, h.resolver, "dev@example.com")
			assert.True(t, allowed)
		})

		t.Run(name+": fails open while the last known list is open", func(t *testing.T) {
			h := newAllowlistHarness()
			mustAllow(t, h.resolver, "anyone@anywhere.test") // compile the open list

			breakStore(h.store)
			h.expire()
			allowed, active := mustAllow(t, h.resolver, "anyone@anywhere.test")
			assert.True(t, allowed)
			assert.False(t, active)
			assert.Positive(t, h.logged(
				"Access allowlist could not be read; no allowlist is known to be active, allowing access"))
		})
	}

	t.Run("fails open when no list was ever compiled", func(t *testing.T) {
		h := newAllowlistHarness()
		h.store.set([]string{"example.com"}, nil)
		h.store.fail(dbDown, nil)

		allowed, active := mustAllow(t, h.resolver, "outsider@other.com")
		assert.True(t, allowed)
		assert.False(t, active)
	})

	t.Run("a cancelled caller is refused while the list is active", func(t *testing.T) {
		h := newAllowlistHarness()
		h.store.set([]string{"example.com"}, nil)
		mustAllow(t, h.resolver, "dev@example.com")
		h.store.fail(dbDown, nil)
		h.expire()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, err := h.resolver.IsEmailAllowed(ctx, "dev@example.com")
		require.Error(t, err)
	})
}

func TestAccessAllowlistResolver_RootAdminExemption(t *testing.T) {
	const root = "Root@Corp.example"
	exemptionLog := "Root instance admin is exempt from the access allowlist their email does not match"

	t.Run("a root admin matching no entry is allowed, and logged once per compiled list", func(t *testing.T) {
		h := newAllowlistHarness(root, " ")
		h.store.set([]string{"example.com"}, nil)

		for range 5 {
			allowed, active := mustAllow(t, h.resolver, "root@corp.example")
			assert.True(t, allowed)
			assert.True(t, active, "the exemption does not hide that an allowlist is active")
		}
		assert.Equal(t, 1, h.logged(exemptionLog))

		// A recompiled list logs it again.
		h.store.set([]string{"example.org"}, nil)
		h.expire()
		mustAllow(t, h.resolver, "ROOT@corp.example")
		assert.Equal(t, 2, h.logged(exemptionLog))
	})

	t.Run("a root admin the list admits is not logged as exempt", func(t *testing.T) {
		h := newAllowlistHarness(root)
		h.store.set([]string{"corp.example"}, nil)
		allowed, _ := mustAllow(t, h.resolver, root)
		assert.True(t, allowed)
		assert.Zero(t, h.logged(exemptionLog))
	})

	t.Run("anyone else is not exempt, and a blank email is never root", func(t *testing.T) {
		h := newAllowlistHarness(root, "")
		h.store.set([]string{"example.com"}, nil)
		for _, email := range []string{"granted-admin@corp.example", ""} {
			allowed, active := mustAllow(t, h.resolver, email)
			assert.False(t, allowed, "email %q", email)
			assert.True(t, active)
		}
	})

	t.Run("a root admin is allowed even when the active list cannot be read", func(t *testing.T) {
		h := newAllowlistHarness(root)
		h.store.set([]string{"example.com"}, nil)
		mustAllow(t, h.resolver, "dev@example.com")
		h.store.fail(errors.New("db down"), nil)
		h.expire()

		allowed, active := mustAllow(t, h.resolver, root)
		assert.True(t, allowed)
		assert.True(t, active)
		_, _, err := h.resolver.IsEmailAllowed(context.Background(), "dev@example.com")
		require.Error(t, err, "everyone else still fails closed")
	})
}

func TestAccessAllowlistResolver_PreviewAllowlistImpact(t *testing.T) {
	ctx := context.Background()

	t.Run("normalizes the candidate and passes the root admins as exempt", func(t *testing.T) {
		h := newAllowlistHarness("Root@Corp.example")
		h.store.previewCount = 2
		h.store.previewEmail = []string{"a@other.com", "b@other.com"}

		impact, err := h.resolver.PreviewAllowlistImpact(ctx, models.InstanceAuthAllowlist{
			Domains: []string{" Example.com ", "example.com"},
			Emails:  []string{"Guest@Other.com"},
		})
		require.NoError(t, err)
		assert.Equal(t, &AllowlistImpact{Count: 2, Sample: []string{"a@other.com", "b@other.com"}}, impact)
		assert.Equal(t, []any{
			[]string{"example.com"}, []string{"guest@other.com"}, []string{"root@corp.example"},
			AllowlistImpactSampleLimit,
		}, h.store.previewArgs)
	})

	t.Run("reports a truncated sample", func(t *testing.T) {
		h := newAllowlistHarness()
		h.store.previewCount = 45
		h.store.previewEmail = make([]string, AllowlistImpactSampleLimit)

		impact, err := h.resolver.PreviewAllowlistImpact(ctx, models.InstanceAuthAllowlist{Domains: []string{"example.com"}})
		require.NoError(t, err)
		assert.Equal(t, 45, impact.Count)
		assert.Len(t, impact.Sample, AllowlistImpactSampleLimit)
		assert.True(t, impact.SampleTruncated)
	})

	t.Run("an open-access candidate affects nobody and reads nothing", func(t *testing.T) {
		h := newAllowlistHarness()
		h.store.previewCount = 9

		impact, err := h.resolver.PreviewAllowlistImpact(ctx, models.InstanceAuthAllowlist{Emails: []string{" "}})
		require.NoError(t, err)
		assert.Equal(t, &AllowlistImpact{Sample: []string{}}, impact)
		assert.Nil(t, h.store.previewArgs)
	})

	t.Run("nobody affected yields an empty, non-nil sample", func(t *testing.T) {
		h := newAllowlistHarness()
		impact, err := h.resolver.PreviewAllowlistImpact(ctx, models.InstanceAuthAllowlist{Domains: []string{"example.com"}})
		require.NoError(t, err)
		assert.Equal(t, &AllowlistImpact{Sample: []string{}}, impact)
	})

	t.Run("rejects an invalid candidate", func(t *testing.T) {
		h := newAllowlistHarness()
		_, err := h.resolver.PreviewAllowlistImpact(ctx, models.InstanceAuthAllowlist{Domains: []string{"not a domain"}})
		require.ErrorIs(t, err, ErrInvalidInstanceAuthAllowlist)
		assert.Nil(t, h.store.previewArgs)
	})

	t.Run("wraps a repository failure", func(t *testing.T) {
		h := newAllowlistHarness()
		h.store.previewErr = errors.New("db down")
		_, err := h.resolver.PreviewAllowlistImpact(ctx, models.InstanceAuthAllowlist{Domains: []string{"example.com"}})
		require.ErrorIs(t, err, h.store.previewErr)
	})
}
