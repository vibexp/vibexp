package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultNotificationPreferences_WebPushIsOffEverywhere pins the invariant
// established in #688: web push was removed, so the retained `web_push` field
// must never default to enabled.
//
// The field survives only so consumers of the currently published
// @vibexp/api-client keep compiling — it is `required` in the schema and is
// marked `deprecated: true` there. Nothing in Go reads it. Defaulting it to true
// would write "this channel is on" into every new user's stored preferences for
// a channel that no longer exists, and would contradict the schema's own
// `example: false`. It is removed entirely in the follow-up (#690).
func TestDefaultNotificationPreferences_WebPushIsOffEverywhere(t *testing.T) {
	defaults := DefaultNotificationPreferences()

	assert.False(t, defaults.Channels.WebPush,
		"the global web_push channel must default to off — the channel was removed in #688")

	require.NotEmpty(t, defaults.Types, "there must be per-type defaults to assert against")
	for name, typePrefs := range defaults.Types {
		assert.False(t, typePrefs.WebPush,
			"notification type %q must default web_push to off — the channel was removed in #688", name)
	}
}

// TestDefaultNotificationPreferences_SurvivingChannels guards the channels that
// are still real, so a future cleanup of the dead web_push field cannot quietly
// take in-app or email defaults with it.
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

// TestDefaultPreferences_MarshalsWebPushAsFalse checks the serialized shape
// rather than only the struct: `web_push` is still part of the published
// contract, so it must appear in the JSON (omitting it would break clients
// typed against the current api-client) and it must appear as false.
func TestDefaultPreferences_MarshalsWebPushAsFalse(t *testing.T) {
	raw, err := json.Marshal(DefaultPreferences().Notifications)
	require.NoError(t, err)

	var decoded struct {
		Channels map[string]any            `json:"channels"`
		Types    map[string]map[string]any `json:"types"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))

	webPush, ok := decoded.Channels["web_push"]
	require.True(t, ok, "web_push must still be present in the channels payload")
	assert.Equal(t, false, webPush)

	for name, typePrefs := range decoded.Types {
		value, ok := typePrefs["web_push"]
		require.True(t, ok, "web_push must still be present in the payload for type %q", name)
		assert.Equal(t, false, value, "web_push must serialize as false for type %q", name)
	}
}
