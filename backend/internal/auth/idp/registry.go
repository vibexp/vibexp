package idp

// Entry is one provider held in a Registry, with the metadata the login UI and
// the auth service need beside the provider itself.
type Entry struct {
	// Provider is the built identity provider. Its Name() is the registry key:
	// the provider's slug for a DB-managed provider (#1234).
	Provider IdentityProvider
	// Type is the provider's kind (ProviderGoogle, ProviderGitHub or
	// ProviderOIDC). Type-specific logic, such as the legacy google_id lookup,
	// reads it rather than Name(), because the slug of a DB-managed provider is
	// admin-chosen.
	Type ProviderName
	// DisplayName is the label the login UI shows.
	DisplayName string
}

// Registry is an immutable snapshot of the identity providers enabled for a
// deployment, keyed by each provider's Name() (its slug) and kept in the order
// they were added (the admin-defined sort order). Several providers of the same
// type may be enabled side by side as long as their names differ. A Registry is
// never mutated after construction, so it is safe for concurrent reads.
type Registry struct {
	entries map[ProviderName]Entry
	order   []ProviderName
}

// NewRegistry builds a Registry from the given providers, keyed by each
// provider's Name() and in argument order. Each provider's Type is its Name()
// and its DisplayName is DefaultDisplayName(Name()). A nil or empty input yields
// an empty registry (web login disabled).
func NewRegistry(providers ...IdentityProvider) *Registry {
	entries := make([]Entry, 0, len(providers))
	for _, p := range providers {
		if p == nil {
			continue
		}
		entries = append(entries, Entry{Provider: p, Type: p.Name(), DisplayName: DefaultDisplayName(p.Name())})
	}
	return NewRegistryFromEntries(entries...)
}

// NewRegistryFromEntries builds a Registry from entries, keyed by each
// provider's Name() and in argument order. Entries with a nil provider are
// skipped. If two entries report the same Name() the later one replaces the
// earlier one in place; callers should not register duplicates.
func NewRegistryFromEntries(entries ...Entry) *Registry {
	r := &Registry{
		entries: make(map[ProviderName]Entry, len(entries)),
		order:   make([]ProviderName, 0, len(entries)),
	}
	for _, e := range entries {
		if e.Provider == nil {
			continue
		}
		name := e.Provider.Name()
		if _, dup := r.entries[name]; !dup {
			r.order = append(r.order, name)
		}
		r.entries[name] = e
	}
	return r
}

// Get returns the provider registered under name and whether it exists.
func (r *Registry) Get(name ProviderName) (IdentityProvider, bool) {
	e, ok := r.entries[name]
	return e.Provider, ok
}

// Entry returns the entry registered under name and whether it exists.
func (r *Registry) Entry(name ProviderName) (Entry, bool) {
	e, ok := r.entries[name]
	return e, ok
}

// Enabled returns the names of all enabled providers in registration order.
func (r *Registry) Enabled() []ProviderName {
	return append([]ProviderName(nil), r.order...)
}

// Entries returns every entry in registration order.
func (r *Registry) Entries() []Entry {
	out := make([]Entry, len(r.order))
	for i, name := range r.order {
		out[i] = r.entries[name]
	}
	return out
}

// Len reports how many providers are enabled.
func (r *Registry) Len() int { return len(r.order) }
