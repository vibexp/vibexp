package resourceaccess

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

func TestIsWriteSource(t *testing.T) {
	t.Parallel()

	for _, source := range []string{SourceWeb, SourceCLI, SourceMCP, SourceAPI} {
		assert.True(t, IsWriteSource(source), source)
	}
	// "unknown" is a reporting key only; it must never be accepted as the
	// source of an event.
	for _, source := range []string{SourceUnknown, "", "WEB", "browser"} {
		assert.False(t, IsWriteSource(source), source)
	}
}

func TestWriteOriginFromContext_NoneSet(t *testing.T) {
	t.Parallel()

	assert.Nil(t, WriteOriginFromContext(context.Background()))
}

func TestWithWriteOrigin_RoundTrips(t *testing.T) {
	t.Parallel()

	ctx := WithWriteOrigin(context.Background(), SourceMCP, "  claude-code ")

	origin := WriteOriginFromContext(ctx)
	require.NotNil(t, origin)
	assert.Equal(t, SourceMCP, origin.Source)
	require.NotNil(t, origin.AssistantName)
	assert.Equal(t, "claude-code", *origin.AssistantName)
}

func TestWithWriteOrigin_EmptyAssistantNameIsNil(t *testing.T) {
	t.Parallel()

	origin := WriteOriginFromContext(WithWriteOrigin(context.Background(), SourceWeb, ""))

	require.NotNil(t, origin)
	assert.Equal(t, SourceWeb, origin.Source)
	assert.Nil(t, origin.AssistantName)
}

func TestWithWriteOrigin_CapsAssistantName(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", models.MaxAssistantNameLength*2)
	origin := WriteOriginFromContext(WithWriteOrigin(context.Background(), SourceMCP, long))

	require.NotNil(t, origin)
	require.NotNil(t, origin.AssistantName)
	assert.Len(t, *origin.AssistantName, models.MaxAssistantNameLength)
}

// A source that is not a write channel must leave the context unlabelled: a
// write is recorded under a channel the metrics know, or not at all.
func TestWithWriteOrigin_RejectsUnknownSource(t *testing.T) {
	t.Parallel()

	for _, source := range []string{"", SourceUnknown, "browser"} {
		ctx := WithWriteOrigin(context.Background(), source, "claude-code")
		assert.Nil(t, WriteOriginFromContext(ctx), source)
	}
}

// The value returned is a copy: a caller that edits it cannot change what the
// next reader of the same context sees.
func TestWriteOriginFromContext_ReturnsCopy(t *testing.T) {
	t.Parallel()

	ctx := WithWriteOrigin(context.Background(), SourceCLI, "")

	first := WriteOriginFromContext(ctx)
	require.NotNil(t, first)
	first.Source = SourceWeb

	second := WriteOriginFromContext(ctx)
	require.NotNil(t, second)
	assert.Equal(t, SourceCLI, second.Source)
}
