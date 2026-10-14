package services

import (
	"strconv"
	"sync/atomic"
)

// AuthSettingsCache is a cache of the instance auth settings that a write in
// this process can invalidate (#1485). The policy and allowlist resolvers
// serve a snapshot confirmed by the shared auth settings version at most every
// probe interval (2 s), so without this a sign-in right after an in-process
// write (the setup page's bootstrap, an admin enabling password sign-in) still
// sees the old settings. Other replicas converge within the probe interval.
type AuthSettingsCache interface {
	// Invalidate makes the next read reload, even one that joins or follows a
	// refresh that started before the call.
	Invalidate()
}

// settingsGeneration is the invalidation counter a resolver embeds. A snapshot
// records the generation its refresh started under and is fresh only while
// that is still the current one; the single-flight key carries it too, so a
// refresh that started before an Invalidate is neither served nor joined
// afterwards.
type settingsGeneration struct{ n atomic.Uint64 }

// Invalidate implements AuthSettingsCache.
func (g *settingsGeneration) Invalidate() { g.n.Add(1) }

func (g *settingsGeneration) generation() uint64 { return g.n.Load() }

// refreshKey is the single-flight key for a refresh started under gen.
func refreshKey(gen uint64) string { return "refresh-" + strconv.FormatUint(gen, 10) }

// AuthSettingsInvalidator invalidates every auth settings cache of this
// process. Writers call it after their write commits.
type AuthSettingsInvalidator struct {
	caches []AuthSettingsCache
}

// NewAuthSettingsInvalidator invalidates caches; nil entries are skipped.
func NewAuthSettingsInvalidator(caches ...AuthSettingsCache) *AuthSettingsInvalidator {
	inv := &AuthSettingsInvalidator{}
	for _, c := range caches {
		if c != nil {
			inv.caches = append(inv.caches, c)
		}
	}
	return inv
}

// Invalidate invalidates every cache. A nil invalidator does nothing, so
// callers without one (the recovery CLI) need no special case.
func (inv *AuthSettingsInvalidator) Invalidate() {
	if inv == nil {
		return
	}
	for _, c := range inv.caches {
		c.Invalidate()
	}
}
