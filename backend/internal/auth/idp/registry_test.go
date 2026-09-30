package idp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeProvider is a minimal IdentityProvider used to exercise the registry.
type fakeProvider struct {
	name ProviderName
}

func (f fakeProvider) Name() ProviderName                 { return f.name }
func (f fakeProvider) AuthorizeURL(_, _, _ string) string { return string(f.name) }
func (f fakeProvider) Refresh(context.Context, string) (*Tokens, error) {
	return nil, nil
}

func (f fakeProvider) ExchangeCode(context.Context, string, string) (*Tokens, *Claims, error) {
	return nil, nil, nil
}

func TestRegistry_GetAndLen(t *testing.T) {
	reg := NewRegistry(
		fakeProvider{name: ProviderGoogle},
		fakeProvider{name: ProviderGitHub},
	)

	assert.Equal(t, 2, reg.Len())

	got, ok := reg.Get(ProviderGoogle)
	assert.True(t, ok)
	assert.Equal(t, ProviderGoogle, got.Name())

	_, ok = reg.Get(ProviderOIDC)
	assert.False(t, ok, "unregistered provider must not be found")
}

func TestRegistry_EnabledPreservesInsertionOrder(t *testing.T) {
	// Insertion order deliberately not alphabetical: it is the admin-defined
	// sort order, which the login picker must keep.
	reg := NewRegistry(
		fakeProvider{name: ProviderOIDC},
		fakeProvider{name: ProviderGitHub},
		fakeProvider{name: ProviderGoogle},
	)

	assert.Equal(t, []ProviderName{ProviderOIDC, ProviderGitHub, ProviderGoogle}, reg.Enabled())
}

func TestRegistry_KeyedBySlugWithTypeAndDisplayName(t *testing.T) {
	// Two OIDC providers with different slugs live side by side.
	reg := NewRegistryFromEntries(
		Entry{Provider: fakeProvider{name: "corp-sso"}, Type: ProviderOIDC, DisplayName: "Corp"},
		Entry{Provider: fakeProvider{name: "partner-sso"}, Type: ProviderOIDC, DisplayName: "Partner"},
		Entry{Provider: nil, Type: ProviderGoogle},
	)

	assert.Equal(t, 2, reg.Len())
	assert.Equal(t, []ProviderName{"corp-sso", "partner-sso"}, reg.Enabled())
	e, ok := reg.Entry("partner-sso")
	assert.True(t, ok)
	assert.Equal(t, ProviderOIDC, e.Type)
	assert.Equal(t, "Partner", e.DisplayName)
	entries := reg.Entries()
	assert.Len(t, entries, 2)
	assert.Equal(t, "Corp", entries[0].DisplayName)

	_, ok = reg.Entry(ProviderOIDC)
	assert.False(t, ok, "the registry is keyed by slug, not by type")
}

func TestNewRegistry_DefaultsTypeAndDisplayName(t *testing.T) {
	reg := NewRegistry(fakeProvider{name: ProviderGitHub})
	e, ok := reg.Entry(ProviderGitHub)
	assert.True(t, ok)
	assert.Equal(t, ProviderGitHub, e.Type)
	assert.Equal(t, "GitHub", e.DisplayName)
}

func TestRegistry_Empty(t *testing.T) {
	reg := NewRegistry()
	assert.Equal(t, 0, reg.Len())
	assert.Empty(t, reg.Enabled())

	regNil := NewRegistry(nil)
	assert.Equal(t, 0, regNil.Len())
}

func TestRegistry_LastDuplicateWins(t *testing.T) {
	first := fakeProvider{name: ProviderGoogle}
	second := fakeProvider{name: ProviderGoogle}
	reg := NewRegistry(first, second)

	assert.Equal(t, 1, reg.Len())
	assert.Equal(t, []ProviderName{ProviderGoogle}, reg.Enabled())
	got, ok := reg.Get(ProviderGoogle)
	assert.True(t, ok)
	assert.Equal(t, second, got)
}
