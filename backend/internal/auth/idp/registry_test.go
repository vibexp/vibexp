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

func TestRegistry_EnabledIsSorted(t *testing.T) {
	// Insertion order deliberately not alphabetical.
	reg := NewRegistry(
		fakeProvider{name: ProviderOIDC},
		fakeProvider{name: ProviderGitHub},
		fakeProvider{name: ProviderGoogle},
	)

	assert.Equal(t, []ProviderName{ProviderGitHub, ProviderGoogle, ProviderOIDC}, reg.Enabled())
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
	got, ok := reg.Get(ProviderGoogle)
	assert.True(t, ok)
	assert.Equal(t, second, got)
}
