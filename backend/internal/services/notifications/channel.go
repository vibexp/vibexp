package notifications

import (
	"context"

	"github.com/vibexp/vibexp/internal/models"
)

// Channel is the abstraction all notification delivery backends implement.
// Each call to Deliver should be idempotent: the notification is already
// stored in the database; the channel is responsible only for the
// transport-level delivery.
type Channel interface {
	// Name returns the canonical identifier for this channel.
	Name() ChannelName

	// Deliver dispatches the notification to the recipient.
	// prefs contains the per-type preferences for the recipient user.
	// Implementations must not return an error for business-logic skips
	// (e.g. user preference = "none"); use DeliveryResult{Status: StatusSkipped} instead.
	Deliver(
		ctx context.Context,
		n *Notification,
		user *models.User,
		prefs *models.NotificationTypePreference,
	) DeliveryResult
}
