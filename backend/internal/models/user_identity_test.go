package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeEmail(t *testing.T) {
	for in, want := range map[string]string{
		"alice@example.com":     "alice@example.com",
		"Alice@Example.COM":     "alice@example.com",
		"  Alice@Example.com\t": "alice@example.com",
		"":                      "",
	} {
		assert.Equal(t, want, NormalizeEmail(in), "input %q", in)
	}
}
