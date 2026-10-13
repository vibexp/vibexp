package contextkeys

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetAuthMethodAndAuthenticatedAt(t *testing.T) {
	t.Run("absent values read as empty and zero", func(t *testing.T) {
		ctx := context.Background()

		assert.Empty(t, GetAuthMethod(ctx))
		assert.True(t, GetAuthenticatedAt(ctx).IsZero())
	})

	t.Run("recorded values are returned", func(t *testing.T) {
		at := time.Now()
		ctx := context.WithValue(context.Background(), AuthMethod, "password")
		ctx = context.WithValue(ctx, AuthenticatedAt, at)

		assert.Equal(t, "password", GetAuthMethod(ctx))
		assert.True(t, at.Equal(GetAuthenticatedAt(ctx)))
	})

	t.Run("a value of the wrong type reads as absent", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), AuthMethod, 42)
		ctx = context.WithValue(ctx, AuthenticatedAt, "yesterday")

		assert.Empty(t, GetAuthMethod(ctx))
		assert.True(t, GetAuthenticatedAt(ctx).IsZero())
	})
}
