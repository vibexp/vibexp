package notifications_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/notifications"
)

// TestNotificationService_DistinctDedupeKeys verifies that two distinct events
// from the same author to the same recipient each produce a notification.
// This is the regression test for the dedupe-key bug where event.UserID() was
// used instead of the event/entity ID, causing all posts from one author to
// one recipient to collapse into a single notification.
func TestNotificationService_DistinctDedupeKeys(t *testing.T) {
	ctx := context.Background()

	notifRepo := new(repomocks.MockNotificationRepository)
	deliveryRepo := new(repomocks.MockNotificationDeliveryRepository)
	prefRepo := new(repomocks.MockUserPreferencesRepository)
	userRepo := new(repomocks.MockUserRepository)

	ch := &mockChannel{
		name:   notifications.ChannelInApp,
		result: notifications.DeliveryResult{Status: notifications.StatusSent},
	}

	svc := notifications.NewNotificationService(
		notifRepo, deliveryRepo, prefRepo, userRepo,
		[]notifications.Channel{ch}, nil, newTestLogger(),
	)

	testUser := &models.User{ID: "user-456"}
	userRepo.On("GetByID", ctx, "user-456").Return(testUser, nil)
	prefRepo.On("GetByUserID", ctx, "user-456").Return((*models.UserPreferences)(nil), nil)
	deliveryRepo.On("Insert", ctx, mock.AnythingOfType("*models.NotificationDelivery")).Return(nil)

	// Two events from same author, different item IDs
	for i, itemID := range []string{"item-aaa", "item-bbb"} {
		req := &notifications.SendRequest{
			RecipientUserID: "user-456",
			Type:            "feed.item.created",
			Category:        notifications.CategoryLow,
			Title:           "Post " + itemID,
			DedupeKey:       "feed.item.created:" + itemID + ":user-456",
		}

		notifID := "notif-" + itemID
		notifRepo.On("Insert", ctx, mock.MatchedBy(func(n *models.Notification) bool {
			if n.DedupeKey == req.DedupeKey {
				n.ID = notifID
				return true
			}
			return false
		})).Return(nil).Once()

		ch.calls = 0
		err := svc.Send(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, 1, ch.calls, "channel must be called for event %d (item %s)", i+1, itemID)
	}
}

// TestNotificationService_Idempotency verifies that sending the same dedupe_key twice
// does not result in duplicate notifications.
// The second Insert call (simulated by not setting ID) should cause Send to stop
// processing without dispatching channels again.
func TestNotificationService_Idempotency(t *testing.T) {
	ctx := context.Background()

	notifRepo := new(repomocks.MockNotificationRepository)
	deliveryRepo := new(repomocks.MockNotificationDeliveryRepository)
	prefRepo := new(repomocks.MockUserPreferencesRepository)
	userRepo := new(repomocks.MockUserRepository)

	var callCount int
	ch := &mockChannel{
		name:   notifications.ChannelInApp,
		result: notifications.DeliveryResult{Status: notifications.StatusSent},
	}

	svc := notifications.NewNotificationService(
		notifRepo, deliveryRepo, prefRepo, userRepo,
		[]notifications.Channel{ch}, nil, newTestLogger(),
	)

	req := &notifications.SendRequest{
		RecipientUserID: "user-123",
		TeamID:          "team-456",
		Type:            "feed.item.created",
		Category:        notifications.CategoryLow,
		Title:           "Idempotency test",
		DedupeKey:       "event-abc:user-123",
	}

	// First call: Insert sets the ID (new notification)
	firstCall := true
	notifRepo.On("Insert", ctx, mock.AnythingOfType("*models.Notification")).
		Run(func(args mock.Arguments) {
			n := args.Get(1).(*models.Notification)
			callCount++
			if firstCall {
				n.ID = "notif-id-first"
				firstCall = false
			}
			// On subsequent calls, don't set ID (simulates ON CONFLICT DO NOTHING)
		}).Return(nil)

	userRepo.On("GetByID", ctx, "user-123").Return(&models.User{ID: "user-123"}, nil)
	prefRepo.On("GetByUserID", ctx, "user-123").Return((*models.UserPreferences)(nil), nil)
	deliveryRepo.On("Insert", ctx, mock.AnythingOfType("*models.NotificationDelivery")).Return(nil)

	// First send should succeed
	err := svc.Send(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 1, ch.calls, "channel called once on first send")

	// Second send with same dedupe_key: Insert doesn't set ID → channels NOT dispatched
	ch.calls = 0
	err = svc.Send(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 0, ch.calls, "channel should not be called on duplicate send")
}
