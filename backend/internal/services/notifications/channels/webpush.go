package channels

import (
	"context"
	"fmt"
	"log/slog"

	firebase "firebase.google.com/go/v4/messaging"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services/notifications"
)

// WebPushChannel delivers notifications via Firebase Cloud Messaging to registered browsers / devices.
type WebPushChannel struct {
	fcm       *firebase.Client
	tokenRepo repositories.DeviceTokenRepository
	logger    *slog.Logger
}

// NewWebPushChannel creates a new WebPushChannel.
// If fcm is nil (FCM_ENABLED=false) the channel will always
// return StatusSkipped rather than panicking.
func NewWebPushChannel(
	fcm *firebase.Client,
	tokenRepo repositories.DeviceTokenRepository,
	logger *slog.Logger,
) *WebPushChannel {
	return &WebPushChannel{
		fcm:       fcm,
		tokenRepo: tokenRepo,
		logger:    logger,
	}
}

// Name returns the channel identifier.
func (c *WebPushChannel) Name() notifications.ChannelName {
	return notifications.ChannelWebPush
}

// Deliver sends a multicast FCM push to all registered device tokens for the user.
// Expired / invalid tokens discovered during delivery are pruned automatically.
func (c *WebPushChannel) Deliver(
	ctx context.Context,
	n *notifications.Notification,
	user *models.User,
	prefs *models.NotificationTypePreference,
) notifications.DeliveryResult {
	if c.fcm == nil {
		return notifications.DeliveryResult{
			Status: notifications.StatusSkipped,
			Reason: "FCM client not configured",
		}
	}

	if prefs == nil || !prefs.WebPush {
		return notifications.DeliveryResult{
			Status: notifications.StatusSkipped,
			Reason: "web_push disabled for this notification type",
		}
	}

	tokenStrings, result, ok := c.resolveTokens(ctx, user.ID)
	if !ok {
		return result
	}

	return c.sendMulticast(ctx, n, tokenStrings)
}

// resolveTokens fetches registered tokens for the user and converts them to a string slice.
// Returns (nil, result, false) when delivery should be aborted.
func (c *WebPushChannel) resolveTokens(
	ctx context.Context,
	userID string,
) ([]string, notifications.DeliveryResult, bool) {
	tokens, err := c.tokenRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, notifications.DeliveryResult{
			Status: notifications.StatusFailed,
			Reason: fmt.Errorf("list device tokens: %w", err).Error(),
		}, false
	}

	if len(tokens) == 0 {
		return nil, notifications.DeliveryResult{
			Status: notifications.StatusSkipped,
			Reason: "no registered devices",
		}, false
	}

	tokenStrings := make([]string, len(tokens))
	for i, t := range tokens {
		tokenStrings[i] = t.Token
	}

	return tokenStrings, notifications.DeliveryResult{}, true
}

// sendMulticast dispatches the FCM multicast message and prunes expired tokens.
func (c *WebPushChannel) sendMulticast(
	ctx context.Context,
	n *notifications.Notification,
	tokenStrings []string,
) notifications.DeliveryResult {
	resp, err := c.fcm.SendEachForMulticast(ctx, &firebase.MulticastMessage{
		Tokens: tokenStrings,
		Notification: &firebase.Notification{
			Title: n.Title,
			Body:  n.Body,
		},
		Data: map[string]string{
			"notification_id": n.ID,
			"type":            string(n.Type),
			"action_url":      n.ActionURL,
		},
		Webpush: &firebase.WebpushConfig{
			FCMOptions: &firebase.WebpushFCMOptions{
				Link: n.ActionURL,
			},
		},
	})
	if err != nil {
		return notifications.DeliveryResult{
			Status: notifications.StatusFailed,
			Reason: fmt.Errorf("fcm multicast: %w", err).Error(),
		}
	}

	c.pruneExpiredTokens(ctx, resp, tokenStrings)

	return notifications.DeliveryResult{Status: notifications.StatusSent}
}

// pruneExpiredTokens deletes FCM registration tokens that have become invalid or
// belong to a different sender, to keep the token table tidy.
func (c *WebPushChannel) pruneExpiredTokens(
	ctx context.Context,
	resp *firebase.BatchResponse,
	tokenStrings []string,
) {
	var expiredTokens []string

	for i, r := range resp.Responses {
		if r.Success {
			continue
		}

		if firebase.IsUnregistered(r.Error) || firebase.IsSenderIDMismatch(r.Error) {
			expiredTokens = append(expiredTokens, tokenStrings[i])
		}
	}

	if len(expiredTokens) == 0 {
		return
	}

	if deleteErr := c.tokenRepo.DeleteByTokens(ctx, expiredTokens); deleteErr != nil {
		if c.logger != nil {
			c.logger.With("error", deleteErr).Warn("failed to delete expired FCM tokens")
		}
	}
}
