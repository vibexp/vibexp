package idp

import "sort"

// Registry holds the set of identity providers enabled for a deployment,
// keyed by their canonical ProviderName. A deployment may enable one or
// several providers at once (e.g. Google + GitHub). The registry is built
// once at startup and is safe for concurrent reads thereafter.
type Registry struct {
	providers map[ProviderName]IdentityProvider
}

// NewRegistry builds a Registry from the given providers, keyed by each
// provider's Name(). A nil or empty input yields an empty registry (web
// login disabled). If two providers report the same Name() the last one
// wins; callers should not register duplicates.
func NewRegistry(providers ...IdentityProvider) *Registry {
	m := make(map[ProviderName]IdentityProvider, len(providers))
	for _, p := range providers {
		if p == nil {
			continue
		}
		m[p.Name()] = p
	}
	return &Registry{providers: m}
}

// Get returns the provider registered under name and whether it exists.
func (r *Registry) Get(name ProviderName) (IdentityProvider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// Enabled returns the canonical names of all enabled providers,
// stable-sorted for deterministic ordering (e.g. for UI provider pickers
// and logs).
func (r *Registry) Enabled() []ProviderName {
	names := make([]ProviderName, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}

// Len reports how many providers are enabled.
func (r *Registry) Len() int { return len(r.providers) }
