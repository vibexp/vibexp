package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type countingCache struct{ invalidated int }

func (c *countingCache) Invalidate() { c.invalidated++ }

func TestAuthSettingsInvalidator(t *testing.T) {
	a, b := &countingCache{}, &countingCache{}
	inv := NewAuthSettingsInvalidator(a, nil, b)

	inv.Invalidate()
	inv.Invalidate()

	assert.Equal(t, 2, a.invalidated)
	assert.Equal(t, 2, b.invalidated, "a nil cache is skipped")

	var none *AuthSettingsInvalidator
	assert.NotPanics(t, none.Invalidate, "a nil invalidator (the recovery CLI) does nothing")
}
