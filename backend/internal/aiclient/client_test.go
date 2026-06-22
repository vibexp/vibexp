package aiclient

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Without service-account ADC (the usual test/local environment), idtoken cannot
// mint tokens, so New falls back to a plain client. Either way the returned client
// must be usable and carry the configured timeout (New sets Timeout on both the
// OIDC and the fallback client, so this holds regardless of the environment).
func TestNew_ReturnsClientWithConfiguredTimeout(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	client := New(context.Background(), "https://ai-service.example.run.app", 7*time.Second, logger)

	require.NotNil(t, client)
	assert.Equal(t, 7*time.Second, client.Timeout)
}

func TestNew_NilLoggerIsSafe(t *testing.T) {
	client := New(context.Background(), "https://ai-service.example.run.app/", 5*time.Second, nil)

	require.NotNil(t, client)
	assert.Equal(t, 5*time.Second, client.Timeout)
}
