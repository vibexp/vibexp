package notifications_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/notifications"
)

// mockChannel is a simple test double for notifications.Channel
type mockChannel struct {
	name   notifications.ChannelName
	result notifications.DeliveryResult
	calls  int
}

func (m *mockChannel) Name() notifications.ChannelName { return m.name }
func (m *mockChannel) Deliver(
	_ context.Context,
	_ *notifications.Notification,
	_ *models.User,
	_ *models.NotificationTypePreference,
) notifications.DeliveryResult {
	m.calls++
	return m.result
}

// errorChannel always returns a failure result (non-error, just failed status)
type errorChannel struct {
	name notifications.ChannelName
}

func (e *errorChannel) Name() notifications.ChannelName { return e.name }
func (e *errorChannel) Deliver(
	_ context.Context,
	_ *notifications.Notification,
	_ *models.User,
	_ *models.NotificationTypePreference,
) notifications.DeliveryResult {
	return notifications.DeliveryResult{
		Status: notifications.StatusFailed,
		Reason: "simulated channel failure",
	}
}

func newTestLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	return logger
}

func makeNotifReq(dedupeKey string) *notifications.SendRequest {
	return &notifications.SendRequest{
		RecipientUserID: "user-123",
		TeamID:          "team-456",
		Type:            "feed.item.created",
		Category:        notifications.CategoryLow,
		Title:           "Test notification",
		Body:            "Test body",
		DedupeKey:       dedupeKey,
	}
}

func TestNotificationService_Send_HappyPath(t *testing.T) {
	ctx := context.Background()

	notifRepo := new(repomocks.MockNotificationRepository)
	deliveryRepo := new(repomocks.MockNotificationDeliveryRepository)
	prefRepo := new(repomocks.MockUserPreferencesRepository)
	userRepo := new(repomocks.MockUserRepository)

	sentResult := notifications.DeliveryResult{Status: notifications.StatusSent}
	ch := &mockChannel{name: notifications.ChannelInApp, result: sentResult}

	svc := notifications.NewNotificationService(
		notifRepo, deliveryRepo, prefRepo, userRepo,
		[]notifications.Channel{ch}, nil, newTestLogger(),
	)

	req := makeNotifReq("test-dedupe-1")

	notifRepo.On("Insert", ctx, mock.MatchedBy(func(n *models.Notification) bool {
		n.ID = "notif-id-1"
		n.CreatedAt = time.Now()
		return n.RecipientUserID == "user-123"
	})).Return(nil)

	testUser := &models.User{ID: "user-123", Email: "user@example.com"}
	userRepo.On("GetByID", ctx, "user-123").Return(testUser, nil)
	prefRepo.On("GetByUserID", ctx, "user-123").Return((*models.UserPreferences)(nil), nil)

	deliveryRepo.On("Insert", ctx, mock.AnythingOfType("*models.NotificationDelivery")).Return(nil)

	err := svc.Send(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, 1, ch.calls, "channel should be called once")
	notifRepo.AssertExpectations(t)
	deliveryRepo.AssertExpectations(t)
}

func TestNotificationService_Send_Dedupe(t *testing.T) {
	ctx := context.Background()

	notifRepo := new(repomocks.MockNotificationRepository)
	deliveryRepo := new(repomocks.MockNotificationDeliveryRepository)
	prefRepo := new(repomocks.MockUserPreferencesRepository)
	userRepo := new(repomocks.MockUserRepository)

	sentResult := notifications.DeliveryResult{Status: notifications.StatusSent}
	ch := &mockChannel{name: notifications.ChannelInApp, result: sentResult}

	svc := notifications.NewNotificationService(
		notifRepo, deliveryRepo, prefRepo, userRepo,
		[]notifications.Channel{ch}, nil, newTestLogger(),
	)

	req := makeNotifReq("dedupe-key-xyz")

	// Insert returns nil but doesn't set ID (simulates ON CONFLICT DO NOTHING)
	notifRepo.On("Insert", ctx, mock.AnythingOfType("*models.Notification")).Return(nil)

	err := svc.Send(ctx, req)

	require.NoError(t, err)
	// Channel should NOT be called when ID is empty (dedupe hit)
	assert.Equal(t, 0, ch.calls, "channel should not be called on dedupe")
	// No delivery record should be inserted
	deliveryRepo.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything)
}

func TestNotificationService_Send_ChannelError_DoesNotFail(t *testing.T) {
	ctx := context.Background()

	notifRepo := new(repomocks.MockNotificationRepository)
	deliveryRepo := new(repomocks.MockNotificationDeliveryRepository)
	prefRepo := new(repomocks.MockUserPreferencesRepository)
	userRepo := new(repomocks.MockUserRepository)

	failCh := &errorChannel{name: notifications.ChannelEmail}

	svc := notifications.NewNotificationService(
		notifRepo, deliveryRepo, prefRepo, userRepo,
		[]notifications.Channel{failCh}, nil, newTestLogger(),
	)

	req := makeNotifReq("key-1")

	notifRepo.On("Insert", ctx, mock.MatchedBy(func(n *models.Notification) bool {
		n.ID = "notif-id-2"
		return true
	})).Return(nil)

	testUser := &models.User{ID: "user-123"}
	userRepo.On("GetByID", ctx, "user-123").Return(testUser, nil)
	prefRepo.On("GetByUserID", ctx, "user-123").Return((*models.UserPreferences)(nil), nil)
	deliveryRepo.On("Insert", ctx, mock.AnythingOfType("*models.NotificationDelivery")).Return(nil)

	err := svc.Send(ctx, req)

	require.NoError(t, err, "Send should not fail when channel delivery fails")
	notifRepo.AssertExpectations(t)
}

func TestNotificationService_Send_InsertError(t *testing.T) {
	ctx := context.Background()

	notifRepo := new(repomocks.MockNotificationRepository)
	deliveryRepo := new(repomocks.MockNotificationDeliveryRepository)
	prefRepo := new(repomocks.MockUserPreferencesRepository)
	userRepo := new(repomocks.MockUserRepository)

	svc := notifications.NewNotificationService(
		notifRepo, deliveryRepo, prefRepo, userRepo,
		nil, nil, newTestLogger(),
	)

	req := makeNotifReq("")

	notifRepo.On("Insert", ctx, mock.AnythingOfType("*models.Notification")).
		Return(errors.New("db error"))

	err := svc.Send(ctx, req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insert notification")
}

func TestNotificationService_GetUnreadCount(t *testing.T) {
	ctx := context.Background()
	notifRepo := new(repomocks.MockNotificationRepository)

	svc := notifications.NewNotificationService(
		notifRepo, nil, nil, nil, nil, nil, newTestLogger(),
	)

	notifRepo.On("GetUnreadCount", ctx, "user-123").Return(5, nil)

	count, err := svc.GetUnreadCount(ctx, "user-123")

	require.NoError(t, err)
	assert.Equal(t, 5, count)
}

func TestNotificationService_MarkRead(t *testing.T) {
	ctx := context.Background()
	notifRepo := new(repomocks.MockNotificationRepository)

	svc := notifications.NewNotificationService(
		notifRepo, nil, nil, nil, nil, nil, newTestLogger(),
	)

	notifRepo.On("MarkRead", ctx, "user-123", "notif-456").Return(nil)

	err := svc.MarkRead(ctx, "user-123", "notif-456")
	require.NoError(t, err)
}

func TestNotificationService_MarkAllRead(t *testing.T) {
	ctx := context.Background()
	notifRepo := new(repomocks.MockNotificationRepository)

	svc := notifications.NewNotificationService(
		notifRepo, nil, nil, nil, nil, nil, newTestLogger(),
	)

	notifRepo.On("MarkAllRead", ctx, "user-123").Return(nil)

	err := svc.MarkAllRead(ctx, "user-123")
	require.NoError(t, err)
}

func TestNotificationService_RunRetentionJob(t *testing.T) {
	ctx := context.Background()
	notifRepo := new(repomocks.MockNotificationRepository)

	svc := notifications.NewNotificationService(
		notifRepo, nil, nil, nil, nil, nil, newTestLogger(),
	)

	notifRepo.On("DeleteOlderThan", ctx, mock.AnythingOfType("time.Time")).Return(int64(42), nil)

	err := svc.RunRetentionJob(ctx)
	require.NoError(t, err)
	notifRepo.AssertExpectations(t)
}

// TestNotificationService_WebPushChannelGloballyDisabled verifies that when
// the user's global web_push channel preference is false the channel is skipped.
func TestNotificationService_WebPushChannelGloballyDisabled(t *testing.T) {
	ctx := context.Background()

	notifRepo := new(repomocks.MockNotificationRepository)
	deliveryRepo := new(repomocks.MockNotificationDeliveryRepository)
	prefRepo := new(repomocks.MockUserPreferencesRepository)
	userRepo := new(repomocks.MockUserRepository)

	ch := &mockChannel{name: notifications.ChannelWebPush, result: notifications.DeliveryResult{
		Status: notifications.StatusSent,
	}}

	svc := notifications.NewNotificationService(
		notifRepo, deliveryRepo, prefRepo, userRepo,
		[]notifications.Channel{ch}, nil, newTestLogger(),
	)

	req := makeNotifReq("web-push-disabled-test")

	notifRepo.On("Insert", ctx, mock.MatchedBy(func(n *models.Notification) bool {
		n.ID = "notif-wp-1"
		n.CreatedAt = time.Now()
		return true
	})).Return(nil)

	testUser := &models.User{ID: "user-123", Email: "user@example.com"}
	userRepo.On("GetByID", ctx, "user-123").Return(testUser, nil)

	// WebPush globally off in channel preferences
	prefs := &models.UserPreferences{
		Preferences: models.Preferences{
			Notifications: models.NotificationPreferences{
				Channels: models.NotificationChannelPreferences{
					InApp:   true,
					Email:   true,
					WebPush: false,
				},
			},
		},
	}
	prefRepo.On("GetByUserID", ctx, "user-123").Return(prefs, nil)

	// The channel is skipped — delivery is recorded with StatusSkipped
	deliveryRepo.On("Insert", ctx, mock.MatchedBy(func(d *models.NotificationDelivery) bool {
		return d.Status == string(notifications.StatusSkipped)
	})).Return(nil)

	err := svc.Send(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, 0, ch.calls, "web_push channel should not be called when globally disabled")
}

func TestNotificationService_ListForUser(t *testing.T) {
	ctx := context.Background()
	notifRepo := new(repomocks.MockNotificationRepository)

	svc := notifications.NewNotificationService(
		notifRepo, nil, nil, nil, nil, nil, newTestLogger(),
	)

	expectedNotifs := []*models.Notification{
		{ID: "n1", RecipientUserID: "user-123", Title: "Test", CreatedAt: time.Now()},
	}

	notifRepo.On("ListForUser", ctx, "user-123", mock.AnythingOfType("repositories.NotificationListFilters")).
		Return(expectedNotifs, nil)

	items, err := svc.ListForUser(ctx, "user-123", notifications.ListFilters{Limit: 10})

	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, "n1", items[0].ID)
}
