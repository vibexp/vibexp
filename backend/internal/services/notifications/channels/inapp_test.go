package channels_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/internal/services/notifications/channels"
)

func TestInAppChannel_Name(t *testing.T) {
	ch := channels.NewInAppChannel()
	assert.Equal(t, notifications.ChannelInApp, ch.Name())
}

func TestInAppChannel_Deliver_InAppEnabled(t *testing.T) {
	ch := channels.NewInAppChannel()
	ctx := context.Background()

	n := &notifications.Notification{ID: "n1", Title: "Test"}
	user := &models.User{ID: "user-1"}
	prefs := &models.NotificationTypePreference{InApp: true}

	result := ch.Deliver(ctx, n, user, prefs)

	assert.Equal(t, notifications.StatusSent, result.Status)
}

func TestInAppChannel_Deliver_InAppDisabled(t *testing.T) {
	ch := channels.NewInAppChannel()
	ctx := context.Background()

	n := &notifications.Notification{ID: "n1"}
	user := &models.User{ID: "user-1"}
	prefs := &models.NotificationTypePreference{InApp: false}

	result := ch.Deliver(ctx, n, user, prefs)

	assert.Equal(t, notifications.StatusSkipped, result.Status)
}

func TestInAppChannel_Deliver_NilPrefs_Skipped(t *testing.T) {
	ch := channels.NewInAppChannel()
	ctx := context.Background()

	result := ch.Deliver(ctx, &notifications.Notification{}, &models.User{}, nil)

	assert.Equal(t, notifications.StatusSkipped, result.Status)
}
