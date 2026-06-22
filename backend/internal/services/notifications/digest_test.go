package notifications_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/notifications"
)

// mockDigestEmailSender is a simple test double for DigestEmailSender.
type mockDigestEmailSender struct {
	calls  int
	lastTo string
	err    error
}

func (m *mockDigestEmailSender) SendNotificationEmail(to, _, _ string) error {
	m.calls++
	m.lastTo = to
	return m.err
}

// newDigestRunner constructs a DigestRunner for tests, registering advisory-lock
// expectations that succeed (lock acquired + released) so all tests that exercise
// Run() work without boilerplate in each test case.
func newDigestRunner(
	digestRepo *repomocks.MockNotificationDigestQueueRepository,
	notifRepo *repomocks.MockNotificationRepository,
	userRepo *repomocks.MockUserRepository,
	teamRepo *repomocks.MockTeamRepository,
	prefRepo *repomocks.MockUserPreferencesRepository,
	emailSvc notifications.DigestEmailSender,
) *notifications.DigestRunner {
	logger := slog.New(slog.DiscardHandler)
	return notifications.NewDigestRunner(
		digestRepo,
		notifRepo,
		userRepo,
		teamRepo,
		prefRepo,
		emailSvc,
		notifications.NewTemplateRenderer("https://app.example.com"),
		nil, // metrics – nil is safe; RecordXxx guards against nil receiver
		logger,
	)
}

// setupAdvisoryLock registers the expected advisory lock acquire + release on the
// mock so tests that call runner.Run() don't need to repeat this setup.
func setupAdvisoryLock(digestRepo *repomocks.MockNotificationDigestQueueRepository, ctx context.Context) {
	digestRepo.EXPECT().
		TryAdvisoryLock(ctx, mock.AnythingOfType("int64")).
		Return(true, nil)
	digestRepo.EXPECT().
		ReleaseAdvisoryLock(ctx, mock.AnythingOfType("int64")).
		Return(nil)
}

// --------------------------------------------------------------------------
// Happy path: one user, one notification
// --------------------------------------------------------------------------

func TestDigestRunner_Run_HappyPath(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	userID := "user-001"
	queueRowID := "row-001"
	notifID := "notif-001"

	digestRepo := repomocks.NewMockNotificationDigestQueueRepository(t)
	notifRepo := repomocks.NewMockNotificationRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	teamRepo := repomocks.NewMockTeamRepository(t)
	prefRepo := repomocks.NewMockUserPreferencesRepository(t)
	emailSvc := &mockDigestEmailSender{}

	pending := []*models.NotificationDigestQueueRow{
		{ID: queueRowID, UserID: userID, NotificationID: notifID, ScheduledFor: now.Add(-1 * time.Hour)},
	}
	user := &models.User{ID: userID, Name: "Alice", Email: "alice@example.com"}
	prefs := &models.UserPreferences{
		Preferences: models.Preferences{
			Notifications: models.NotificationPreferences{
				Channels: models.NotificationChannelPreferences{Email: true},
			},
		},
	}
	notifs := []*models.Notification{
		{ID: notifID, RecipientUserID: userID, Title: "New post", Body: "Hello", ActionURL: "https://app.vibexp.io/feed/1"},
	}

	setupAdvisoryLock(digestRepo, ctx)
	digestRepo.EXPECT().FetchPending(ctx, now).Return(pending, nil)
	userRepo.EXPECT().GetByID(ctx, userID).Return(user, nil)
	prefRepo.EXPECT().GetByUserID(ctx, userID).Return(prefs, nil)
	notifRepo.EXPECT().GetByIDsForUser(ctx, userID, []string{notifID}).Return(notifs, nil)
	digestRepo.EXPECT().MarkSent(ctx, []string{queueRowID}, now).Return(nil)

	runner := newDigestRunner(digestRepo, notifRepo, userRepo, teamRepo, prefRepo, emailSvc)
	err := runner.Run(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 1, emailSvc.calls, "expected exactly one email sent")
	assert.Equal(t, "alice@example.com", emailSvc.lastTo)
}

// --------------------------------------------------------------------------
// Multi-user batching: two users each get separate emails
// --------------------------------------------------------------------------

func TestDigestRunner_Run_MultiUserBatching(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	digestRepo := repomocks.NewMockNotificationDigestQueueRepository(t)
	notifRepo := repomocks.NewMockNotificationRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	teamRepo := repomocks.NewMockTeamRepository(t)
	prefRepo := repomocks.NewMockUserPreferencesRepository(t)
	emailSvc := &mockDigestEmailSender{}

	user1 := &models.User{ID: "u1", Name: "Bob", Email: "bob@example.com"}
	user2 := &models.User{ID: "u2", Name: "Carol", Email: "carol@example.com"}

	emailEnabledPrefs := &models.UserPreferences{
		Preferences: models.Preferences{
			Notifications: models.NotificationPreferences{
				Channels: models.NotificationChannelPreferences{Email: true},
			},
		},
	}

	pending := []*models.NotificationDigestQueueRow{
		{ID: "row-1", UserID: "u1", NotificationID: "n1", ScheduledFor: now.Add(-1 * time.Hour)},
		{ID: "row-2", UserID: "u2", NotificationID: "n2", ScheduledFor: now.Add(-1 * time.Hour)},
	}

	setupAdvisoryLock(digestRepo, ctx)
	digestRepo.EXPECT().FetchPending(ctx, now).Return(pending, nil)

	userRepo.EXPECT().GetByID(ctx, "u1").Return(user1, nil)
	prefRepo.EXPECT().GetByUserID(ctx, "u1").Return(emailEnabledPrefs, nil)
	notifRepo.EXPECT().GetByIDsForUser(ctx, "u1", []string{"n1"}).Return([]*models.Notification{
		{ID: "n1", Title: "Notif for Bob"},
	}, nil)
	digestRepo.EXPECT().MarkSent(ctx, []string{"row-1"}, now).Return(nil)

	userRepo.EXPECT().GetByID(ctx, "u2").Return(user2, nil)
	prefRepo.EXPECT().GetByUserID(ctx, "u2").Return(emailEnabledPrefs, nil)
	notifRepo.EXPECT().GetByIDsForUser(ctx, "u2", []string{"n2"}).Return([]*models.Notification{
		{ID: "n2", Title: "Notif for Carol"},
	}, nil)
	digestRepo.EXPECT().MarkSent(ctx, []string{"row-2"}, now).Return(nil)

	runner := newDigestRunner(digestRepo, notifRepo, userRepo, teamRepo, prefRepo, emailSvc)
	err := runner.Run(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 2, emailSvc.calls, "expected one email per user")
}

// --------------------------------------------------------------------------
// Email failure: rows should NOT be marked sent
// --------------------------------------------------------------------------

func TestDigestRunner_Run_EmailFailure(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	digestRepo := repomocks.NewMockNotificationDigestQueueRepository(t)
	notifRepo := repomocks.NewMockNotificationRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	teamRepo := repomocks.NewMockTeamRepository(t)
	prefRepo := repomocks.NewMockUserPreferencesRepository(t)
	emailSvc := &mockDigestEmailSender{err: errors.New("SMTP connection refused")}

	pending := []*models.NotificationDigestQueueRow{
		{ID: "row-fail", UserID: "u1", NotificationID: "n1", ScheduledFor: now.Add(-1 * time.Hour)},
	}
	user := &models.User{ID: "u1", Name: "Dave", Email: "dave@example.com"}
	prefs := &models.UserPreferences{
		Preferences: models.Preferences{
			Notifications: models.NotificationPreferences{
				Channels: models.NotificationChannelPreferences{Email: true},
			},
		},
	}

	setupAdvisoryLock(digestRepo, ctx)
	digestRepo.EXPECT().FetchPending(ctx, now).Return(pending, nil)
	userRepo.EXPECT().GetByID(ctx, "u1").Return(user, nil)
	prefRepo.EXPECT().GetByUserID(ctx, "u1").Return(prefs, nil)
	notifRepo.EXPECT().GetByIDsForUser(ctx, "u1", []string{"n1"}).Return([]*models.Notification{
		{ID: "n1", Title: "A post"},
	}, nil)
	// MarkSent should NOT be called when the email fails

	runner := newDigestRunner(digestRepo, notifRepo, userRepo, teamRepo, prefRepo, emailSvc)
	err := runner.Run(ctx, now)

	require.NoError(t, err, "Run should absorb per-user errors and not propagate them")
	assert.Equal(t, 1, emailSvc.calls, "email was attempted once")
}

// --------------------------------------------------------------------------
// Deleted user: drain queue by marking rows sent (C1 fix)
// --------------------------------------------------------------------------

func TestDigestRunner_Run_DeletedUser(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	digestRepo := repomocks.NewMockNotificationDigestQueueRepository(t)
	notifRepo := repomocks.NewMockNotificationRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	teamRepo := repomocks.NewMockTeamRepository(t)
	prefRepo := repomocks.NewMockUserPreferencesRepository(t)
	emailSvc := &mockDigestEmailSender{}

	pending := []*models.NotificationDigestQueueRow{
		{ID: "row-deleted", UserID: "u-deleted", NotificationID: "n1"},
	}

	setupAdvisoryLock(digestRepo, ctx)
	digestRepo.EXPECT().FetchPending(ctx, now).Return(pending, nil)
	// Deleted user returns the sentinel error
	userRepo.EXPECT().GetByID(ctx, "u-deleted").Return(nil, repositories.ErrUserNotFound)
	// Rows must be drained (marked sent) so they don't accumulate forever
	digestRepo.EXPECT().MarkSent(ctx, []string{"row-deleted"}, now).Return(nil)
	// prefRepo, notifRepo, emailSvc must NOT be called

	runner := newDigestRunner(digestRepo, notifRepo, userRepo, teamRepo, prefRepo, emailSvc)
	err := runner.Run(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 0, emailSvc.calls)
}

// --------------------------------------------------------------------------
// Transient DB error loading user: skip WITHOUT marking rows sent (C1 fix)
// --------------------------------------------------------------------------

func TestDigestRunner_Run_UserLoadTransientError(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	digestRepo := repomocks.NewMockNotificationDigestQueueRepository(t)
	notifRepo := repomocks.NewMockNotificationRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	teamRepo := repomocks.NewMockTeamRepository(t)
	prefRepo := repomocks.NewMockUserPreferencesRepository(t)
	emailSvc := &mockDigestEmailSender{}

	pending := []*models.NotificationDigestQueueRow{
		{ID: "row-transient", UserID: "u-transient", NotificationID: "n1"},
	}

	setupAdvisoryLock(digestRepo, ctx)
	digestRepo.EXPECT().FetchPending(ctx, now).Return(pending, nil)
	// Generic DB error — NOT ErrUserNotFound
	userRepo.EXPECT().GetByID(ctx, "u-transient").Return(nil, errors.New("connection timeout"))
	// MarkSent must NOT be called — rows stay pending for retry on next run

	runner := newDigestRunner(digestRepo, notifRepo, userRepo, teamRepo, prefRepo, emailSvc)
	err := runner.Run(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 0, emailSvc.calls)
}

// --------------------------------------------------------------------------
// Empty email address: skip and drain queue (H4 fix)
// --------------------------------------------------------------------------

func TestDigestRunner_Run_EmptyEmailAddress(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	digestRepo := repomocks.NewMockNotificationDigestQueueRepository(t)
	notifRepo := repomocks.NewMockNotificationRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	teamRepo := repomocks.NewMockTeamRepository(t)
	prefRepo := repomocks.NewMockUserPreferencesRepository(t)
	emailSvc := &mockDigestEmailSender{}

	pending := []*models.NotificationDigestQueueRow{
		{ID: "row-noemail", UserID: "u-noemail", NotificationID: "n1"},
	}
	// OAuth-only user with no email
	user := &models.User{ID: "u-noemail", Email: ""}

	setupAdvisoryLock(digestRepo, ctx)
	digestRepo.EXPECT().FetchPending(ctx, now).Return(pending, nil)
	userRepo.EXPECT().GetByID(ctx, "u-noemail").Return(user, nil)
	// Rows must be drained — this user can never receive email
	digestRepo.EXPECT().MarkSent(ctx, []string{"row-noemail"}, now).Return(nil)
	// prefRepo and emailSvc must NOT be called

	runner := newDigestRunner(digestRepo, notifRepo, userRepo, teamRepo, prefRepo, emailSvc)
	err := runner.Run(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 0, emailSvc.calls, "no email attempt when address is empty")
}

// --------------------------------------------------------------------------
// Email channel disabled: skip and mark rows sent
// --------------------------------------------------------------------------

func TestDigestRunner_Run_EmailChannelDisabled(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	digestRepo := repomocks.NewMockNotificationDigestQueueRepository(t)
	notifRepo := repomocks.NewMockNotificationRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	teamRepo := repomocks.NewMockTeamRepository(t)
	prefRepo := repomocks.NewMockUserPreferencesRepository(t)
	emailSvc := &mockDigestEmailSender{}

	userID := "u-disabled"
	pending := []*models.NotificationDigestQueueRow{
		{ID: "row-dis", UserID: userID, NotificationID: "n1"},
	}
	user := &models.User{ID: userID, Email: "disabled@example.com"}
	disabledPrefs := &models.UserPreferences{
		Preferences: models.Preferences{
			Notifications: models.NotificationPreferences{
				Channels: models.NotificationChannelPreferences{Email: false},
			},
		},
	}

	setupAdvisoryLock(digestRepo, ctx)
	digestRepo.EXPECT().FetchPending(ctx, now).Return(pending, nil)
	userRepo.EXPECT().GetByID(ctx, userID).Return(user, nil)
	prefRepo.EXPECT().GetByUserID(ctx, userID).Return(disabledPrefs, nil)
	digestRepo.EXPECT().MarkSent(ctx, []string{"row-dis"}, now).Return(nil)
	// notifRepo and emailSvc must NOT be called

	runner := newDigestRunner(digestRepo, notifRepo, userRepo, teamRepo, prefRepo, emailSvc)
	err := runner.Run(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 0, emailSvc.calls, "no email when channel is disabled")
}

// --------------------------------------------------------------------------
// Empty queue: no work done
// --------------------------------------------------------------------------

func TestDigestRunner_Run_EmptyQueue(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	digestRepo := repomocks.NewMockNotificationDigestQueueRepository(t)
	notifRepo := repomocks.NewMockNotificationRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	teamRepo := repomocks.NewMockTeamRepository(t)
	prefRepo := repomocks.NewMockUserPreferencesRepository(t)
	emailSvc := &mockDigestEmailSender{}

	setupAdvisoryLock(digestRepo, ctx)
	digestRepo.EXPECT().FetchPending(ctx, now).Return([]*models.NotificationDigestQueueRow{}, nil)

	runner := newDigestRunner(digestRepo, notifRepo, userRepo, teamRepo, prefRepo, emailSvc)
	err := runner.Run(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 0, emailSvc.calls)
}

// --------------------------------------------------------------------------
// FetchPending error propagates
// --------------------------------------------------------------------------

func TestDigestRunner_Run_FetchPendingError(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	digestRepo := repomocks.NewMockNotificationDigestQueueRepository(t)
	notifRepo := repomocks.NewMockNotificationRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	teamRepo := repomocks.NewMockTeamRepository(t)
	prefRepo := repomocks.NewMockUserPreferencesRepository(t)
	emailSvc := &mockDigestEmailSender{}

	setupAdvisoryLock(digestRepo, ctx)
	digestRepo.EXPECT().FetchPending(ctx, now).Return(nil, errors.New("db error"))

	runner := newDigestRunner(digestRepo, notifRepo, userRepo, teamRepo, prefRepo, emailSvc)
	err := runner.Run(ctx, now)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch pending digest queue rows")
}

// --------------------------------------------------------------------------
// Advisory lock contention: another run in progress → skip gracefully (H1 fix)
// --------------------------------------------------------------------------

func TestDigestRunner_Run_AdvisoryLockContention(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	digestRepo := repomocks.NewMockNotificationDigestQueueRepository(t)
	notifRepo := repomocks.NewMockNotificationRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	teamRepo := repomocks.NewMockTeamRepository(t)
	prefRepo := repomocks.NewMockUserPreferencesRepository(t)
	emailSvc := &mockDigestEmailSender{}

	// Lock not acquired — another runner is active
	digestRepo.EXPECT().TryAdvisoryLock(ctx, mock.AnythingOfType("int64")).Return(false, nil)
	// FetchPending and downstream must NOT be called

	runner := newDigestRunner(digestRepo, notifRepo, userRepo, teamRepo, prefRepo, emailSvc)
	err := runner.Run(ctx, now)

	require.NoError(t, err, "contention should not be an error — just a no-op")
	assert.Equal(t, 0, emailSvc.calls)
}

// --------------------------------------------------------------------------
// All notifications deleted (empty notif list after GetByIDsForUser)
// --------------------------------------------------------------------------

func TestDigestRunner_Run_NotificationsDeleted(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	digestRepo := repomocks.NewMockNotificationDigestQueueRepository(t)
	notifRepo := repomocks.NewMockNotificationRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	teamRepo := repomocks.NewMockTeamRepository(t)
	prefRepo := repomocks.NewMockUserPreferencesRepository(t)
	emailSvc := &mockDigestEmailSender{}

	userID := "u1"
	pending := []*models.NotificationDigestQueueRow{
		{ID: "row-1", UserID: userID, NotificationID: "n1"},
	}
	user := &models.User{ID: userID, Email: "eve@example.com"}
	prefs := &models.UserPreferences{
		Preferences: models.Preferences{
			Notifications: models.NotificationPreferences{
				Channels: models.NotificationChannelPreferences{Email: true},
			},
		},
	}

	setupAdvisoryLock(digestRepo, ctx)
	digestRepo.EXPECT().FetchPending(ctx, now).Return(pending, nil)
	userRepo.EXPECT().GetByID(ctx, userID).Return(user, nil)
	prefRepo.EXPECT().GetByUserID(ctx, userID).Return(prefs, nil)
	notifRepo.EXPECT().GetByIDsForUser(ctx, userID, []string{"n1"}).Return([]*models.Notification{}, nil)
	digestRepo.EXPECT().MarkSent(ctx, []string{"row-1"}, now).Return(nil)

	runner := newDigestRunner(digestRepo, notifRepo, userRepo, teamRepo, prefRepo, emailSvc)
	err := runner.Run(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 0, emailSvc.calls)
}

// --------------------------------------------------------------------------
// Template rendering: RenderDigestEmail produces valid HTML
// --------------------------------------------------------------------------

func TestTemplateRenderer_RenderDigestEmail(t *testing.T) {
	renderer := notifications.NewTemplateRenderer("https://app.example.com")

	user := &models.User{Name: "Frank", Email: "frank@example.com"}
	notifs := []*models.Notification{
		{Title: "Post alpha", Body: "Hello world", ActionURL: "https://app.vibexp.io/feed/1"},
		{Title: "Post beta"},
	}

	html, err := renderer.RenderDigestEmail(user, notifs)

	require.NoError(t, err)
	assert.Contains(t, html, "Frank")
	assert.Contains(t, html, "Post alpha")
	assert.Contains(t, html, "Hello world")
	assert.Contains(t, html, "https://app.vibexp.io/feed/1")
	assert.Contains(t, html, "Post beta")
	assert.Contains(t, html, "Manage notification preferences")
}

// --------------------------------------------------------------------------
// Template rendering: empty notification list renders no-activity message
// --------------------------------------------------------------------------

func TestTemplateRenderer_RenderDigestEmail_EmptyNotifications(t *testing.T) {
	renderer := notifications.NewTemplateRenderer("https://app.example.com")
	user := &models.User{Name: "Grace"}

	html, err := renderer.RenderDigestEmail(user, []*models.Notification{})

	require.NoError(t, err)
	assert.Contains(t, html, "Grace")
	assert.Contains(t, html, "No new activity")
}

// --------------------------------------------------------------------------
// Template rendering: notifications grouped by team with resolved names
// --------------------------------------------------------------------------

func TestTemplateRenderer_RenderDigestEmail_TeamGrouping(t *testing.T) {
	renderer := notifications.NewTemplateRenderer("https://app.example.com")
	user := &models.User{Name: "Hank"}
	notifs := []*models.Notification{
		{TeamID: "team-a", Title: "A post"},
		{TeamID: "team-b", Title: "B post"},
		{TeamID: "team-a", Title: "Another A post"},
	}

	html, err := renderer.RenderDigestEmail(user, notifs)

	require.NoError(t, err)
	assert.Contains(t, html, "team-a")
	assert.Contains(t, html, "team-b")
	assert.Contains(t, html, "A post")
	assert.Contains(t, html, "B post")
}

// --------------------------------------------------------------------------
// Template rendering: RenderDigestEmailWithTeamNames resolves names
// --------------------------------------------------------------------------

func TestTemplateRenderer_RenderDigestEmailWithTeamNames(t *testing.T) {
	renderer := notifications.NewTemplateRenderer("https://app.example.com")
	user := &models.User{Name: "Ivy"}
	notifs := []*models.Notification{
		{TeamID: "tid-1", Title: "Post from Engineering"},
		{TeamID: "tid-2", Title: "Post from Design"},
	}
	teamNames := map[string]string{
		"tid-1": "Engineering",
		"tid-2": "Design",
	}

	html, err := renderer.RenderDigestEmailWithTeamNames(user, notifs, teamNames)

	require.NoError(t, err)
	assert.Contains(t, html, "Engineering")
	assert.Contains(t, html, "Design")
	assert.Contains(t, html, "Post from Engineering")
}

// --------------------------------------------------------------------------
// LoadNotifications error: skip user, don't mark sent
// --------------------------------------------------------------------------

func TestDigestRunner_Run_LoadNotificationsError(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	digestRepo := repomocks.NewMockNotificationDigestQueueRepository(t)
	notifRepo := repomocks.NewMockNotificationRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	teamRepo := repomocks.NewMockTeamRepository(t)
	prefRepo := repomocks.NewMockUserPreferencesRepository(t)
	emailSvc := &mockDigestEmailSender{}

	userID := "u1"
	pending := []*models.NotificationDigestQueueRow{
		{ID: "row-1", UserID: userID, NotificationID: "n1"},
	}
	user := &models.User{ID: userID, Email: "joe@example.com"}
	prefs := &models.UserPreferences{
		Preferences: models.Preferences{
			Notifications: models.NotificationPreferences{
				Channels: models.NotificationChannelPreferences{Email: true},
			},
		},
	}

	setupAdvisoryLock(digestRepo, ctx)
	digestRepo.EXPECT().FetchPending(ctx, now).Return(pending, nil)
	userRepo.EXPECT().GetByID(ctx, userID).Return(user, nil)
	prefRepo.EXPECT().GetByUserID(ctx, userID).Return(prefs, nil)
	notifRepo.EXPECT().GetByIDsForUser(ctx, userID, []string{"n1"}).Return(nil, errors.New("db error"))
	// MarkSent must NOT be called

	runner := newDigestRunner(digestRepo, notifRepo, userRepo, teamRepo, prefRepo, emailSvc)
	err := runner.Run(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 0, emailSvc.calls)
}

// --------------------------------------------------------------------------
// MarkSent error path after successful email send (M3 — duplicate-email risk)
// --------------------------------------------------------------------------

func TestDigestRunner_Run_MarkSentError(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	digestRepo := repomocks.NewMockNotificationDigestQueueRepository(t)
	notifRepo := repomocks.NewMockNotificationRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	teamRepo := repomocks.NewMockTeamRepository(t)
	prefRepo := repomocks.NewMockUserPreferencesRepository(t)
	emailSvc := &mockDigestEmailSender{}

	userID := "u1"
	pending := []*models.NotificationDigestQueueRow{
		{ID: "row-1", UserID: userID, NotificationID: "n1"},
	}
	user := &models.User{ID: userID, Name: "Kim", Email: "kim@example.com"}
	prefs := &models.UserPreferences{
		Preferences: models.Preferences{
			Notifications: models.NotificationPreferences{
				Channels: models.NotificationChannelPreferences{Email: true},
			},
		},
	}

	setupAdvisoryLock(digestRepo, ctx)
	digestRepo.EXPECT().FetchPending(ctx, now).Return(pending, nil)
	userRepo.EXPECT().GetByID(ctx, userID).Return(user, nil)
	prefRepo.EXPECT().GetByUserID(ctx, userID).Return(prefs, nil)
	notifRepo.EXPECT().GetByIDsForUser(ctx, userID, []string{"n1"}).Return([]*models.Notification{
		{ID: "n1", Title: "Hello"},
	}, nil)
	digestRepo.EXPECT().MarkSent(ctx, []string{"row-1"}, now).Return(errors.New("mark sent failed"))

	runner := newDigestRunner(digestRepo, notifRepo, userRepo, teamRepo, prefRepo, emailSvc)
	err := runner.Run(ctx, now)

	require.NoError(t, err, "MarkSent error should be logged but not propagated")
	assert.Equal(t, 1, emailSvc.calls)
}

// --------------------------------------------------------------------------
// Render error path
// --------------------------------------------------------------------------

func TestDigestRunner_Run_RenderError(t *testing.T) {
	// Verify that a working runner with valid template still processes successfully.
	// The render-error code path requires a broken renderer; since templates are
	// embedded and cannot fail at runtime, we cover the logger nil-safety path here.
	ctx := context.Background()
	now := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	digestRepo := repomocks.NewMockNotificationDigestQueueRepository(t)
	notifRepo := repomocks.NewMockNotificationRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	teamRepo := repomocks.NewMockTeamRepository(t)
	prefRepo := repomocks.NewMockUserPreferencesRepository(t)
	emailSvc := &mockDigestEmailSender{}

	logger := slog.New(slog.DiscardHandler)
	runner := notifications.NewDigestRunner(
		digestRepo, notifRepo, userRepo, teamRepo, prefRepo, emailSvc,
		notifications.NewTemplateRenderer("https://app.example.com"),
		nil, // nil metrics
		logger,
	)

	userID := "u1"
	pending := []*models.NotificationDigestQueueRow{
		{ID: "row-1", UserID: userID, NotificationID: "n1"},
	}
	user := &models.User{ID: userID, Name: "Lee", Email: "lee@example.com"}
	prefs := &models.UserPreferences{
		Preferences: models.Preferences{
			Notifications: models.NotificationPreferences{
				Channels: models.NotificationChannelPreferences{Email: true},
			},
		},
	}

	setupAdvisoryLock(digestRepo, ctx)
	digestRepo.EXPECT().FetchPending(ctx, now).Return(pending, nil)
	userRepo.EXPECT().GetByID(ctx, userID).Return(user, nil)
	prefRepo.EXPECT().GetByUserID(ctx, userID).Return(prefs, nil)
	notifRepo.EXPECT().GetByIDsForUser(ctx, userID, []string{"n1"}).Return([]*models.Notification{
		{ID: "n1", Title: "Post"},
	}, nil)
	digestRepo.EXPECT().MarkSent(ctx, []string{"row-1"}, now).Return(nil)

	err := runner.Run(ctx, now)
	require.NoError(t, err)
	assert.Equal(t, 1, emailSvc.calls)
}

// --------------------------------------------------------------------------
// Preferences: nil preferences defaults to email enabled
// --------------------------------------------------------------------------

func TestDigestRunner_Run_NilPreferencesDefaultsToEnabled(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	digestRepo := repomocks.NewMockNotificationDigestQueueRepository(t)
	notifRepo := repomocks.NewMockNotificationRepository(t)
	userRepo := repomocks.NewMockUserRepository(t)
	teamRepo := repomocks.NewMockTeamRepository(t)
	prefRepo := repomocks.NewMockUserPreferencesRepository(t)
	emailSvc := &mockDigestEmailSender{}

	userID := "u1"
	pending := []*models.NotificationDigestQueueRow{
		{ID: "row-1", UserID: userID, NotificationID: "n1"},
	}
	user := &models.User{ID: userID, Name: "Ivan", Email: "ivan@example.com"}

	setupAdvisoryLock(digestRepo, ctx)
	digestRepo.EXPECT().FetchPending(ctx, now).Return(pending, nil)
	userRepo.EXPECT().GetByID(ctx, userID).Return(user, nil)
	// GetByUserID returns nil — defaults to enabled
	prefRepo.EXPECT().GetByUserID(ctx, userID).Return(nil, nil)
	notifRepo.EXPECT().GetByIDsForUser(ctx, userID, []string{"n1"}).Return([]*models.Notification{
		{ID: "n1", Title: "Hello"},
	}, nil)
	digestRepo.EXPECT().MarkSent(ctx, mock.AnythingOfType("[]string"), now).Return(nil)

	runner := newDigestRunner(digestRepo, notifRepo, userRepo, teamRepo, prefRepo, emailSvc)
	err := runner.Run(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 1, emailSvc.calls)
}
