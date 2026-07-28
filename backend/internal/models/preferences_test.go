package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultNotificationPreferences_SurvivingChannels guards the channels that
// are still real, so a future cleanup cannot quietly take in-app or email
// defaults with it.
func TestDefaultNotificationPreferences_SurvivingChannels(t *testing.T) {
	defaults := DefaultNotificationPreferences()

	assert.True(t, defaults.Channels.InApp, "in-app notifications must default on")
	assert.True(t, defaults.Channels.Email, "email notifications must default on")

	expectedEmail := map[string]string{
		"feed.item.created":  "digest",
		"feed.reply.created": "instant",
		"team.invitation":    "instant",
	}
	require.Len(t, defaults.Types, len(expectedEmail))

	for name, wantEmail := range expectedEmail {
		typePrefs, ok := defaults.Types[name]
		require.True(t, ok, "missing default for notification type %q", name)
		assert.True(t, typePrefs.InApp, "type %q must default to in-app on", name)
		assert.Equal(t, wantEmail, typePrefs.Email, "unexpected email mode for type %q", name)
	}
}
