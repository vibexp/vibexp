package channels

import (
	"context"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/notifications"
)

// InAppChannel delivers notifications by persisting them to the database.
// The notification is already inserted by NotificationService.Send, so
// this channel is a no-op transport that simply records delivery as sent.
type InAppChannel struct{}

// NewInAppChannel creates a new InAppChannel
func NewInAppChannel() *InAppChannel {
	return &InAppChannel{}
}

// Name returns the channel identifier
func (c *InAppChannel) Name() notifications.ChannelName {
	return notifications.ChannelInApp
}

// Deliver marks the notification as delivered in-app.
// The actual storage was performed by NotificationService.Send before
// channels are dispatched, so no additional action is required here.
func (c *InAppChannel) Deliver(
	_ context.Context,
	_ *notifications.Notification,
	_ *models.User,
	prefs *models.NotificationTypePreference,
) notifications.DeliveryResult {
	if prefs == nil || !prefs.InApp {
		return notifications.DeliveryResult{
			Status: notifications.StatusSkipped,
			Reason: "in_app preference disabled",
		}
	}

	return notifications.DeliveryResult{Status: notifications.StatusSent}
}
