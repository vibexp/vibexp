package channels_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/internal/services/notifications/channels"
)

// mockEmailSender implements NotificationEmailSender for testing
type mockEmailSender struct {
	mock.Mock
}

func (m *mockEmailSender) SendNotificationEmail(
	ctx context.Context, teamID, to, subject, htmlBody string,
) error {
	args := m.Called(ctx, teamID, to, subject, htmlBody)
	return args.Error(0)
}

func TestEmailChannel_Name(t *testing.T) {
	ch := channels.NewEmailChannel(nil, nil, nil)
	assert.Equal(t, notifications.ChannelEmail, ch.Name())
}

func TestEmailChannel_Deliver_Instant_Sends(t *testing.T) {
	ctx := context.Background()

	sender := new(mockEmailSender)
	digestRepo := new(repomocks.MockNotificationDigestQueueRepository)

	ch := channels.NewEmailChannel(sender, digestRepo, nil)

	n := &notifications.Notification{
		ID:                   "n1",
		Title:                "Subject",
		Body:                 "Body text",
		RenderedEmailSubject: "Rendered Subject",
		RenderedEmailHTML:    "<p>Rendered HTML</p>",
	}
	user := &models.User{ID: "user-1", Email: "user@example.com"}
	prefs := &models.NotificationTypePreference{Email: "instant"}

	// Should use RenderedEmailSubject and RenderedEmailHTML, not Title/Body
	sender.On("SendNotificationEmail", mock.Anything, mock.Anything,
		"user@example.com", "Rendered Subject", "<p>Rendered HTML</p>").Return(nil)

	result := ch.Deliver(ctx, n, user, prefs)

	assert.Equal(t, notifications.StatusSent, result.Status)
	sender.AssertExpectations(t)
}

func TestEmailChannel_Deliver_Instant_FallbackToTitleBody(t *testing.T) {
	ctx := context.Background()

	sender := new(mockEmailSender)
	ch := channels.NewEmailChannel(sender, nil, nil)

	// No RenderedEmailSubject/HTML — fallback to Title and escaped Body
	n := &notifications.Notification{ID: "n1", Title: "Subject", Body: "Body text"}
	user := &models.User{ID: "user-1", Email: "user@example.com"}
	prefs := &models.NotificationTypePreference{Email: "instant"}

	sender.On("SendNotificationEmail", mock.Anything, mock.Anything,
		"user@example.com", "Subject", "<p>Body text</p>").Return(nil)

	result := ch.Deliver(ctx, n, user, prefs)

	assert.Equal(t, notifications.StatusSent, result.Status)
	sender.AssertExpectations(t)
}

func TestEmailChannel_Deliver_Instant_SendError(t *testing.T) {
	ctx := context.Background()

	sender := new(mockEmailSender)
	ch := channels.NewEmailChannel(sender, nil, nil)

	n := &notifications.Notification{ID: "n1", Title: "Subject", Body: "Body"}
	user := &models.User{Email: "user@example.com"}
	prefs := &models.NotificationTypePreference{Email: "instant"}

	sender.On("SendNotificationEmail", mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything).
		Return(errors.New("smtp error"))

	result := ch.Deliver(ctx, n, user, prefs)

	assert.Equal(t, notifications.StatusFailed, result.Status)
	assert.Contains(t, result.Reason, "smtp error")
}

func TestEmailChannel_Deliver_Digest_Enqueues(t *testing.T) {
	ctx := context.Background()

	digestRepo := new(repomocks.MockNotificationDigestQueueRepository)
	ch := channels.NewEmailChannel(nil, digestRepo, nil)

	n := &notifications.Notification{ID: "n1"}
	user := &models.User{ID: "user-1", Email: "user@example.com"}
	prefs := &models.NotificationTypePreference{Email: "digest"}

	digestRepo.On("Enqueue", ctx, "user-1", "n1", mock.AnythingOfType("time.Time")).Return(nil)

	result := ch.Deliver(ctx, n, user, prefs)

	assert.Equal(t, notifications.StatusQueued, result.Status)
	digestRepo.AssertExpectations(t)
}

func TestEmailChannel_Deliver_Digest_EnqueueError(t *testing.T) {
	ctx := context.Background()

	digestRepo := new(repomocks.MockNotificationDigestQueueRepository)
	ch := channels.NewEmailChannel(nil, digestRepo, nil)

	n := &notifications.Notification{ID: "n1"}
	user := &models.User{ID: "user-1"}
	prefs := &models.NotificationTypePreference{Email: "digest"}

	digestRepo.On("Enqueue", ctx, mock.Anything, mock.Anything, mock.AnythingOfType("time.Time")).
		Return(errors.New("db error"))

	result := ch.Deliver(ctx, n, user, prefs)

	assert.Equal(t, notifications.StatusFailed, result.Status)
}

func TestEmailChannel_Deliver_None_Skips(t *testing.T) {
	ctx := context.Background()

	ch := channels.NewEmailChannel(nil, nil, nil)
	n := &notifications.Notification{ID: "n1"}
	user := &models.User{Email: "user@example.com"}
	prefs := &models.NotificationTypePreference{Email: "none"}

	result := ch.Deliver(ctx, n, user, prefs)

	assert.Equal(t, notifications.StatusSkipped, result.Status)
}

func TestEmailChannel_Deliver_NilPrefs_Skips(t *testing.T) {
	ctx := context.Background()

	ch := channels.NewEmailChannel(nil, nil, nil)

	result := ch.Deliver(ctx, &notifications.Notification{}, &models.User{}, nil)

	assert.Equal(t, notifications.StatusSkipped, result.Status)
}

func TestEmailChannel_DigestScheduledForNextMorning(t *testing.T) {
	ctx := context.Background()

	digestRepo := new(repomocks.MockNotificationDigestQueueRepository)
	ch := channels.NewEmailChannel(nil, digestRepo, nil)

	n := &notifications.Notification{ID: "n1"}
	user := &models.User{ID: "u1"}
	prefs := &models.NotificationTypePreference{Email: "digest"}

	var capturedTime time.Time
	digestRepo.On("Enqueue", ctx, "u1", "n1", mock.MatchedBy(func(t time.Time) bool {
		capturedTime = t
		return true
	})).Return(nil)

	result := ch.Deliver(ctx, n, user, prefs)
	require.Equal(t, notifications.StatusQueued, result.Status)

	// Verify scheduled_for is in the future and is at 09:00 UTC
	assert.True(t, capturedTime.After(time.Now().UTC()))
	assert.Equal(t, 9, capturedTime.Hour())
	assert.Equal(t, 0, capturedTime.Minute())
}

// The notification's TeamID must reach the email service so the message goes out
// as the owning team. It was previously discarded because the signature had
// nowhere to put it.
func TestEmailChannel_Deliver_PassesTheNotificationTeamID(t *testing.T) {
	ctx := context.Background()
	sender := new(mockEmailSender)
	digestRepo := new(repomocks.MockNotificationDigestQueueRepository)

	ch := channels.NewEmailChannel(sender, digestRepo, nil)

	n := &notifications.Notification{
		ID:                   "n1",
		TeamID:               "team-99",
		Title:                "Subject",
		Body:                 "Body text",
		RenderedEmailSubject: "Rendered Subject",
		RenderedEmailHTML:    "<p>Rendered HTML</p>",
	}
	user := &models.User{ID: "user-1", Email: "user@example.com"}
	prefs := &models.NotificationTypePreference{Email: "instant"}

	sender.On("SendNotificationEmail", mock.Anything, "team-99",
		"user@example.com", "Rendered Subject", "<p>Rendered HTML</p>").Return(nil)

	result := ch.Deliver(ctx, n, user, prefs)

	assert.Equal(t, notifications.StatusSent, result.Status)
	sender.AssertExpectations(t)
}

// A notification with no team resolves to the instance sender rather than being
// misattributed.
func TestEmailChannel_Deliver_EmptyTeamIDResolvesInstance(t *testing.T) {
	ctx := context.Background()
	sender := new(mockEmailSender)
	digestRepo := new(repomocks.MockNotificationDigestQueueRepository)

	ch := channels.NewEmailChannel(sender, digestRepo, nil)

	n := &notifications.Notification{
		ID:                   "n1",
		TeamID:               "",
		RenderedEmailSubject: "Rendered Subject",
		RenderedEmailHTML:    "<p>Rendered HTML</p>",
	}
	user := &models.User{ID: "user-1", Email: "user@example.com"}
	prefs := &models.NotificationTypePreference{Email: "instant"}

	sender.On("SendNotificationEmail", mock.Anything, "",
		"user@example.com", "Rendered Subject", "<p>Rendered HTML</p>").Return(nil)

	result := ch.Deliver(ctx, n, user, prefs)

	assert.Equal(t, notifications.StatusSent, result.Status)
	sender.AssertExpectations(t)
}

// The caller's context reaches the email service, so a cancelled delivery is no
// longer detached.
func TestEmailChannel_Deliver_PropagatesContext(t *testing.T) {
	type ctxKey string
	const marker ctxKey = "marker"

	sender := new(mockEmailSender)
	digestRepo := new(repomocks.MockNotificationDigestQueueRepository)
	ch := channels.NewEmailChannel(sender, digestRepo, nil)

	n := &notifications.Notification{
		ID:                   "n1",
		TeamID:               "team-1",
		RenderedEmailSubject: "S",
		RenderedEmailHTML:    "<p>b</p>",
	}
	user := &models.User{ID: "user-1", Email: "user@example.com"}
	prefs := &models.NotificationTypePreference{Email: "instant"}

	sender.On("SendNotificationEmail", mock.MatchedBy(func(ctx context.Context) bool {
		return ctx.Value(marker) == "yes"
	}), "team-1", "user@example.com", "S", "<p>b</p>").Return(nil)

	ctx := context.WithValue(context.Background(), marker, "yes")
	result := ch.Deliver(ctx, n, user, prefs)

	assert.Equal(t, notifications.StatusSent, result.Status)
	sender.AssertExpectations(t)
}
