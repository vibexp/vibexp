package channels

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services/notifications"
)

// NotificationEmailSender is a narrow interface for sending notification emails.
// It decouples the email channel from the full EmailService implementation.
type NotificationEmailSender interface {
	SendNotificationEmail(to, subject, htmlBody string) error
}

// EmailChannel delivers notifications via email.
// - "instant": sends immediately
// - "digest":  enqueues to notification_digest_queue for later batch delivery
// - otherwise: skips silently
type EmailChannel struct {
	emailSender NotificationEmailSender
	digestRepo  repositories.NotificationDigestQueueRepository
	logger      *logrus.Logger
}

// NewEmailChannel creates a new EmailChannel
func NewEmailChannel(
	emailSender NotificationEmailSender,
	digestRepo repositories.NotificationDigestQueueRepository,
	logger *logrus.Logger,
) *EmailChannel {
	return &EmailChannel{
		emailSender: emailSender,
		digestRepo:  digestRepo,
		logger:      logger,
	}
}

// Name returns the channel identifier
func (c *EmailChannel) Name() notifications.ChannelName {
	return notifications.ChannelEmail
}

// Deliver sends or enqueues the notification email based on the user's email preference.
func (c *EmailChannel) Deliver(
	ctx context.Context,
	n *notifications.Notification,
	user *models.User,
	prefs *models.NotificationTypePreference,
) notifications.DeliveryResult {
	if prefs == nil {
		return notifications.DeliveryResult{
			Status: notifications.StatusSkipped,
			Reason: "no preference found for notification type",
		}
	}

	switch prefs.Email {
	case "instant":
		return c.deliverInstant(ctx, n, user)
	case "digest":
		return c.enqueueDigest(ctx, n, user)
	default:
		return notifications.DeliveryResult{
			Status: notifications.StatusSkipped,
			Reason: fmt.Sprintf("email preference is %q", prefs.Email),
		}
	}
}

func (c *EmailChannel) deliverInstant(
	ctx context.Context,
	n *notifications.Notification,
	user *models.User,
) notifications.DeliveryResult {
	_ = ctx // used by caller for tracing; kept for interface consistency

	subject := n.RenderedEmailSubject
	if subject == "" {
		subject = n.Title
	}

	htmlBody := n.RenderedEmailHTML
	if htmlBody == "" {
		// Fallback: use plain-text body as pre-escaped HTML when no template was rendered
		htmlBody = "<p>" + n.Body + "</p>"
	}

	if err := c.emailSender.SendNotificationEmail(user.Email, subject, htmlBody); err != nil {
		if c.logger != nil {
			c.logger.WithFields(logrus.Fields{
				"notification_id":   n.ID,
				"recipient_user_id": n.RecipientUserID,
				"error":             err.Error(),
			}).Error("Failed to send instant notification email")
		}

		return notifications.DeliveryResult{
			Status: notifications.StatusFailed,
			Reason: err.Error(),
		}
	}

	return notifications.DeliveryResult{Status: notifications.StatusSent}
}

func (c *EmailChannel) enqueueDigest(
	ctx context.Context,
	n *notifications.Notification,
	user *models.User,
) notifications.DeliveryResult {
	scheduledFor := nextDigestTime()

	if err := c.digestRepo.Enqueue(ctx, user.ID, n.ID, scheduledFor); err != nil {
		if c.logger != nil {
			c.logger.WithFields(logrus.Fields{
				"notification_id":   n.ID,
				"recipient_user_id": n.RecipientUserID,
				"scheduled_for":     scheduledFor,
				"error":             fmt.Sprintf("%+v", err),
			}).Error("Failed to enqueue notification for digest")
		}

		return notifications.DeliveryResult{
			Status: notifications.StatusFailed,
			Reason: err.Error(),
		}
	}

	return notifications.DeliveryResult{
		Status: notifications.StatusQueued,
		Reason: fmt.Sprintf("scheduled for %s", scheduledFor.Format(time.RFC3339)),
	}
}

// nextDigestTime returns the next 09:00 UTC after the current time
func nextDigestTime() time.Time {
	now := time.Now().UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.UTC)

	if now.After(next) {
		next = next.Add(24 * time.Hour)
	}

	return next
}
