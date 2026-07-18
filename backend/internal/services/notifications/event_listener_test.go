package notifications_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/pkg/events"
)

// mockNotificationService is a test double for NotificationServiceInterface
type mockNotificationService struct {
	mock.Mock
}

func (m *mockNotificationService) Send(ctx context.Context, req *notifications.SendRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *mockNotificationService) ListForUser(
	ctx context.Context, userID string, f notifications.ListFilters,
) ([]*notifications.Notification, error) {
	args := m.Called(ctx, userID, f)
	return args.Get(0).([]*notifications.Notification), args.Error(1)
}

func (m *mockNotificationService) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

func (m *mockNotificationService) MarkRead(ctx context.Context, userID, notifID string) error {
	return m.Called(ctx, userID, notifID).Error(0)
}

func (m *mockNotificationService) MarkAllRead(ctx context.Context, userID string) error {
	return m.Called(ctx, userID).Error(0)
}

func (m *mockNotificationService) RunRetentionJob(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

// mockRecipientResolver for testing
type mockRecipientResolver struct {
	mock.Mock
}

func (m *mockRecipientResolver) ResolveForEvent(
	ctx context.Context, event events.Event,
) ([]string, string, error) {
	args := m.Called(ctx, event)
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// mockTemplateRenderer for testing
type mockTemplateRenderer struct {
	mock.Mock
}

func (m *mockTemplateRenderer) Render(
	notifType notifications.NotificationType,
	data map[string]interface{},
) (*notifications.RenderedContent, error) {
	args := m.Called(notifType, data)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*notifications.RenderedContent), args.Error(1)
}

// newTestListener creates a listener with mock repos and a test logger.
func newTestListener(
	svc *mockNotificationService,
	resolver *mockRecipientResolver,
	renderer *mockTemplateRenderer,
	userRepo *mocks.MockUserRepository,
	feedItemRepo *mocks.MockFeedItemRepository,
) *notifications.NotificationEventListener {
	return notifications.NewNotificationEventListener(notifications.NotificationEventListenerDeps{
		NotifSvc:        svc,
		Resolver:        resolver,
		Renderer:        renderer,
		UserRepo:        userRepo,
		FeedItemRepo:    feedItemRepo,
		FrontendBaseURL: "https://app.example.com",
		AppMetrics:      nil,
		Logger:          newTestLogger(),
	})
}

func TestNotificationEventListener_EventTypes(t *testing.T) {
	listener := notifications.NewNotificationEventListener(notifications.NotificationEventListenerDeps{
		NotifSvc:        nil,
		Resolver:        nil,
		Renderer:        nil,
		UserRepo:        nil,
		FeedItemRepo:    nil,
		FrontendBaseURL: "",
		AppMetrics:      nil,
		Logger:          nil,
	})
	assert.Equal(t, []string{events.EventTypeFeedItemCreated}, listener.EventTypes())
}

func TestNotificationEventListener_Handle_FeedItemCreated(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)
	userRepo := new(mocks.MockUserRepository)
	feedItemRepo := new(mocks.MockFeedItemRepository)

	listener := newTestListener(svc, resolver, renderer, userRepo, feedItemRepo)

	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "item-1",
		UserID:   "author-id",
		TeamID:   "team-abc",
		FeedID:   "feed-1",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})

	resolver.On("ResolveForEvent", ctx, event).
		Return([]string{"member-1", "member-2"}, "team-abc", nil)

	userRepo.On("GetByID", ctx, "author-id").
		Return(&models.User{ID: "author-id", Name: "Alice"}, nil)

	feedItemRepo.On("GetByID", ctx, "author-id", "team-abc", "item-1").
		Return(&models.FeedItem{ID: "item-1", Title: "Hello world"}, nil)

	renderer.On("Render", notifications.NotificationType("feed.item.created"), mock.Anything).
		Return(&notifications.RenderedContent{
			InAppTitle: "Alice posted to the team feed",
			InAppBody:  "Hello world",
		}, nil)

	svc.On("Send", ctx, mock.MatchedBy(func(req *notifications.SendRequest) bool {
		return req.RecipientUserID == "member-1" || req.RecipientUserID == "member-2"
	})).Return(nil)

	err := listener.Handle(ctx, event)

	require.NoError(t, err)
	svc.AssertNumberOfCalls(t, "Send", 2)
	resolver.AssertExpectations(t)
}

func TestNotificationEventListener_Handle_NilEvent(t *testing.T) {
	listener := notifications.NewNotificationEventListener(notifications.NotificationEventListenerDeps{
		NotifSvc:        nil,
		Resolver:        nil,
		Renderer:        nil,
		UserRepo:        nil,
		FeedItemRepo:    nil,
		FrontendBaseURL: "",
		AppMetrics:      nil,
		Logger:          nil,
	})
	err := listener.Handle(context.Background(), nil)
	assert.NoError(t, err)
}

func TestNotificationEventListener_Handle_ResolverError_ReturnsNil(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)

	listener := newTestListener(svc, resolver, renderer, nil, nil)

	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "item-1",
		UserID:   "author-id",
		TeamID:   "team-abc",
		FeedID:   "feed-1",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})

	resolver.On("ResolveForEvent", ctx, event).
		Return([]string{}, "", errors.New("db error"))

	// Should not return an error (avoid retry storm)
	err := listener.Handle(ctx, event)
	assert.NoError(t, err)
	svc.AssertNotCalled(t, "Send", mock.Anything, mock.Anything)
}

func TestNotificationEventListener_Handle_NoRecipients(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)

	listener := newTestListener(svc, resolver, renderer, nil, nil)

	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "item-1",
		UserID:   "author-id",
		TeamID:   "team-abc",
		FeedID:   "feed-1",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})

	resolver.On("ResolveForEvent", ctx, event).Return([]string{}, "team-abc", nil)

	err := listener.Handle(ctx, event)

	assert.NoError(t, err)
	svc.AssertNotCalled(t, "Send", mock.Anything, mock.Anything)
}

func TestNotificationEventListener_Handle_SendError_ContinuesOtherRecipients(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)
	userRepo := new(mocks.MockUserRepository)
	feedItemRepo := new(mocks.MockFeedItemRepository)

	listener := newTestListener(svc, resolver, renderer, userRepo, feedItemRepo)

	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "item-1",
		UserID:   "author-id",
		TeamID:   "team-abc",
		FeedID:   "feed-1",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})

	resolver.On("ResolveForEvent", ctx, event).
		Return([]string{"m1", "m2"}, "team-abc", nil)

	userRepo.On("GetByID", ctx, "author-id").
		Return(&models.User{ID: "author-id", Name: "Bob"}, nil)

	feedItemRepo.On("GetByID", ctx, "author-id", "team-abc", "item-1").
		Return(&models.FeedItem{ID: "item-1", Title: "A title"}, nil)

	renderer.On("Render", mock.Anything, mock.Anything).
		Return(&notifications.RenderedContent{InAppTitle: "title"}, nil)

	// First call fails, second succeeds
	svc.On("Send", ctx, mock.MatchedBy(func(r *notifications.SendRequest) bool {
		return r.RecipientUserID == "m1"
	})).Return(errors.New("send error"))

	svc.On("Send", ctx, mock.MatchedBy(func(r *notifications.SendRequest) bool {
		return r.RecipientUserID == "m2"
	})).Return(nil)

	err := listener.Handle(ctx, event)

	assert.NoError(t, err)
	svc.AssertNumberOfCalls(t, "Send", 2)
}

// --- New tests for enrichment behaviour ---

func TestRenderFeedItemTemplate_UsesRealActorName(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)
	userRepo := new(mocks.MockUserRepository)
	feedItemRepo := new(mocks.MockFeedItemRepository)

	listener := newTestListener(svc, resolver, renderer, userRepo, feedItemRepo)
	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "item-99",
		UserID:   "user-alice",
		TeamID:   "team-1",
		FeedID:   "feed-1",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})

	resolver.On("ResolveForEvent", ctx, event).
		Return([]string{"recipient-1"}, "team-1", nil)

	userRepo.On("GetByID", ctx, "user-alice").
		Return(&models.User{ID: "user-alice", Name: "Alice Smith"}, nil)

	feedItemRepo.On("GetByID", ctx, "user-alice", "team-1", "item-99").
		Return(&models.FeedItem{ID: "item-99", Title: "My real title"}, nil)

	renderer.On("Render", notifications.NotificationType("feed.item.created"),
		mock.MatchedBy(func(data map[string]interface{}) bool {
			return data["actor_name"] == "Alice Smith" && data["title"] == "My real title"
		}),
	).Return(&notifications.RenderedContent{InAppTitle: "Alice Smith posted", InAppBody: "My real title"}, nil)

	svc.On("Send", ctx, mock.Anything).Return(nil)

	err := listener.Handle(ctx, event)
	require.NoError(t, err)
	renderer.AssertExpectations(t)
}

func TestRenderFeedItemTemplate_UsesRealFeedItemTitle(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)
	userRepo := new(mocks.MockUserRepository)
	feedItemRepo := new(mocks.MockFeedItemRepository)

	listener := newTestListener(svc, resolver, renderer, userRepo, feedItemRepo)
	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "item-42",
		UserID:   "user-bob",
		TeamID:   "team-2",
		FeedID:   "feed-2",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})

	resolver.On("ResolveForEvent", ctx, event).
		Return([]string{"recipient-1"}, "team-2", nil)

	userRepo.On("GetByID", ctx, "user-bob").
		Return(&models.User{ID: "user-bob", Name: "Bob"}, nil)

	feedItemRepo.On("GetByID", ctx, "user-bob", "team-2", "item-42").
		Return(&models.FeedItem{ID: "item-42", Title: "A real feed title"}, nil)

	renderer.On("Render", notifications.NotificationType("feed.item.created"),
		mock.MatchedBy(func(data map[string]interface{}) bool {
			return data["title"] == "A real feed title"
		}),
	).Return(&notifications.RenderedContent{InAppTitle: "Bob posted", InAppBody: "A real feed title"}, nil)

	svc.On("Send", ctx, mock.Anything).Return(nil)

	err := listener.Handle(ctx, event)
	require.NoError(t, err)
	renderer.AssertExpectations(t)
}

func TestRenderFeedItemTemplate_UserRepoError_UsesFallback_NotDropped(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)
	userRepo := new(mocks.MockUserRepository)
	feedItemRepo := new(mocks.MockFeedItemRepository)

	listener := newTestListener(svc, resolver, renderer, userRepo, feedItemRepo)
	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "item-1",
		UserID:   "missing-user",
		TeamID:   "team-1",
		FeedID:   "feed-1",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})

	resolver.On("ResolveForEvent", ctx, event).
		Return([]string{"recipient-1"}, "team-1", nil)

	userRepo.On("GetByID", ctx, "missing-user").
		Return((*models.User)(nil), errors.New("user not found"))

	feedItemRepo.On("GetByID", ctx, "missing-user", "team-1", "item-1").
		Return(&models.FeedItem{ID: "item-1", Title: "Some title"}, nil)

	// Expect fallback actor name "A team member"
	renderer.On("Render", notifications.NotificationType("feed.item.created"),
		mock.MatchedBy(func(data map[string]interface{}) bool {
			return data["actor_name"] == "A team member"
		}),
	).Return(&notifications.RenderedContent{InAppTitle: "A team member posted"}, nil)

	svc.On("Send", ctx, mock.Anything).Return(nil)

	err := listener.Handle(ctx, event)

	// Must not drop the notification — no error returned
	require.NoError(t, err)
	svc.AssertNumberOfCalls(t, "Send", 1)
	renderer.AssertExpectations(t)
}

func TestRenderFeedItemTemplate_FeedItemRepoError_UsesFallback_NotDropped(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)
	userRepo := new(mocks.MockUserRepository)
	feedItemRepo := new(mocks.MockFeedItemRepository)

	listener := newTestListener(svc, resolver, renderer, userRepo, feedItemRepo)
	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "missing-item",
		UserID:   "user-1",
		TeamID:   "team-1",
		FeedID:   "feed-1",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})

	resolver.On("ResolveForEvent", ctx, event).
		Return([]string{"recipient-1"}, "team-1", nil)

	userRepo.On("GetByID", ctx, "user-1").
		Return(&models.User{ID: "user-1", Name: "Carol"}, nil)

	feedItemRepo.On("GetByID", ctx, "user-1", "team-1", "missing-item").
		Return((*models.FeedItem)(nil), errors.New("item not found"))

	// Expect fallback title "new post"
	renderer.On("Render", notifications.NotificationType("feed.item.created"),
		mock.MatchedBy(func(data map[string]interface{}) bool {
			return data["title"] == "new post"
		}),
	).Return(&notifications.RenderedContent{InAppTitle: "Carol posted"}, nil)

	svc.On("Send", ctx, mock.Anything).Return(nil)

	err := listener.Handle(ctx, event)

	// Must not drop the notification — no error returned
	require.NoError(t, err)
	svc.AssertNumberOfCalls(t, "Send", 1)
	renderer.AssertExpectations(t)
}

func TestRenderFeedItemTemplate_ActionURLSetInSendRequest(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)
	userRepo := new(mocks.MockUserRepository)
	feedItemRepo := new(mocks.MockFeedItemRepository)

	listener := newTestListener(svc, resolver, renderer, userRepo, feedItemRepo)
	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "item-55",
		UserID:   "user-1",
		TeamID:   "team-1",
		FeedID:   "feed-1",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})

	resolver.On("ResolveForEvent", ctx, event).
		Return([]string{"recipient-1"}, "team-1", nil)

	userRepo.On("GetByID", ctx, "user-1").
		Return(&models.User{ID: "user-1", Name: "Dave"}, nil)

	feedItemRepo.On("GetByID", ctx, "user-1", "team-1", "item-55").
		Return(&models.FeedItem{ID: "item-55", Title: "My post"}, nil)

	renderer.On("Render", mock.Anything, mock.Anything).
		Return(&notifications.RenderedContent{InAppTitle: "Dave posted", EntityID: "item-55"}, nil)

	// Verify ActionURL is set in the send request
	svc.On("Send", ctx, mock.MatchedBy(func(req *notifications.SendRequest) bool {
		return req.ActionURL == "https://app.example.com/feed-items/item-55"
	})).Return(nil)

	err := listener.Handle(ctx, event)
	require.NoError(t, err)
	svc.AssertExpectations(t)
}

func TestResolveActorName_EmailFallback(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)
	userRepo := new(mocks.MockUserRepository)
	feedItemRepo := new(mocks.MockFeedItemRepository)

	listener := newTestListener(svc, resolver, renderer, userRepo, feedItemRepo)
	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "item-1",
		UserID:   "user-nname",
		TeamID:   "team-1",
		FeedID:   "feed-1",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})

	resolver.On("ResolveForEvent", ctx, event).
		Return([]string{"recipient-1"}, "team-1", nil)

	// User has no Name but has an email
	userRepo.On("GetByID", ctx, "user-nname").
		Return(&models.User{ID: "user-nname", Name: "", Email: "jane@example.com"}, nil)

	feedItemRepo.On("GetByID", ctx, "user-nname", "team-1", "item-1").
		Return(&models.FeedItem{ID: "item-1", Title: "A title"}, nil)

	// actor_name should be the email local-part "jane"
	renderer.On("Render", notifications.NotificationType("feed.item.created"),
		mock.MatchedBy(func(data map[string]interface{}) bool {
			return data["actor_name"] == "jane"
		}),
	).Return(&notifications.RenderedContent{InAppTitle: "jane posted"}, nil)

	svc.On("Send", ctx, mock.Anything).Return(nil)

	err := listener.Handle(ctx, event)
	require.NoError(t, err)
	renderer.AssertExpectations(t)
}

func TestRenderFeedItemTemplate_RendererError_NotDropped(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)
	userRepo := new(mocks.MockUserRepository)
	feedItemRepo := new(mocks.MockFeedItemRepository)

	listener := newTestListener(svc, resolver, renderer, userRepo, feedItemRepo)
	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "item-1",
		UserID:   "user-1",
		TeamID:   "team-1",
		FeedID:   "feed-1",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})

	resolver.On("ResolveForEvent", ctx, event).
		Return([]string{"recipient-1"}, "team-1", nil)

	userRepo.On("GetByID", ctx, "user-1").
		Return(&models.User{ID: "user-1", Name: "Frank"}, nil)

	feedItemRepo.On("GetByID", ctx, "user-1", "team-1", "item-1").
		Return(&models.FeedItem{ID: "item-1", Title: "A title"}, nil)

	renderer.On("Render", mock.Anything, mock.Anything).
		Return((*notifications.RenderedContent)(nil), errors.New("template error"))

	// Renderer failure must not cause Send to be called
	err := listener.Handle(ctx, event)
	require.NoError(t, err)
	svc.AssertNotCalled(t, "Send", mock.Anything, mock.Anything)
}

func TestResolveActorName_NoAtSignEmail_FallbackToTeamMember(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)
	userRepo := new(mocks.MockUserRepository)
	feedItemRepo := new(mocks.MockFeedItemRepository)

	listener := newTestListener(svc, resolver, renderer, userRepo, feedItemRepo)
	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "item-1",
		UserID:   "user-noemail",
		TeamID:   "team-1",
		FeedID:   "feed-1",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})

	resolver.On("ResolveForEvent", ctx, event).
		Return([]string{"recipient-1"}, "team-1", nil)

	// User has no Name and email without '@'
	userRepo.On("GetByID", ctx, "user-noemail").
		Return(&models.User{ID: "user-noemail", Name: "", Email: "nodomain"}, nil)

	feedItemRepo.On("GetByID", ctx, "user-noemail", "team-1", "item-1").
		Return(&models.FeedItem{ID: "item-1", Title: "A title"}, nil)

	renderer.On("Render", notifications.NotificationType("feed.item.created"),
		mock.MatchedBy(func(data map[string]interface{}) bool {
			return data["actor_name"] == "A team member"
		}),
	).Return(&notifications.RenderedContent{InAppTitle: "A team member posted"}, nil)

	svc.On("Send", ctx, mock.Anything).Return(nil)

	err := listener.Handle(ctx, event)
	require.NoError(t, err)
	renderer.AssertExpectations(t)
}

func TestResolveFeedItemTitle_ContentExcerptFallback(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)
	userRepo := new(mocks.MockUserRepository)
	feedItemRepo := new(mocks.MockFeedItemRepository)

	listener := newTestListener(svc, resolver, renderer, userRepo, feedItemRepo)
	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "item-1",
		UserID:   "user-1",
		TeamID:   "team-1",
		FeedID:   "feed-1",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})

	resolver.On("ResolveForEvent", ctx, event).
		Return([]string{"recipient-1"}, "team-1", nil)

	userRepo.On("GetByID", ctx, "user-1").
		Return(&models.User{ID: "user-1", Name: "Eve"}, nil)

	longContent := "This is a very long content string that exceeds eighty characters " +
		"in total length to trigger the truncation path"
	// FeedItem with no title but with long content — should get a truncated excerpt
	feedItemRepo.On("GetByID", ctx, "user-1", "team-1", "item-1").
		Return(&models.FeedItem{ID: "item-1", Title: "", Content: longContent}, nil)

	renderer.On("Render", notifications.NotificationType("feed.item.created"),
		mock.MatchedBy(func(data map[string]interface{}) bool {
			title, _ := data["title"].(string)
			// Truncated at 80 runes plus an ellipsis character; must not exceed 81 runes
			runes := []rune(title)
			return len(runes) <= 81 && strings.HasSuffix(title, "…")
		}),
	).Return(&notifications.RenderedContent{InAppTitle: "Eve posted"}, nil)

	svc.On("Send", ctx, mock.Anything).Return(nil)

	err := listener.Handle(ctx, event)
	require.NoError(t, err)
	renderer.AssertExpectations(t)
}

func TestResolveFeedItemTitle_ExcerptPreferredOverContent(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)
	userRepo := new(mocks.MockUserRepository)
	feedItemRepo := new(mocks.MockFeedItemRepository)

	listener := newTestListener(svc, resolver, renderer, userRepo, feedItemRepo)
	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "item-e",
		UserID:   "user-1",
		TeamID:   "team-1",
		FeedID:   "feed-1",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})

	resolver.On("ResolveForEvent", ctx, event).
		Return([]string{"recipient-1"}, "team-1", nil)

	userRepo.On("GetByID", ctx, "user-1").
		Return(&models.User{ID: "user-1", Name: "Frank"}, nil)

	// FeedItem has no Title but has both Excerpt and Content — Excerpt wins
	feedItemRepo.On("GetByID", ctx, "user-1", "team-1", "item-e").
		Return(&models.FeedItem{
			ID:      "item-e",
			Title:   "",
			Excerpt: "Short curated excerpt",
			Content: "Much longer full content that should not be used",
		}, nil)

	renderer.On("Render", notifications.NotificationType("feed.item.created"),
		mock.MatchedBy(func(data map[string]interface{}) bool {
			return data["title"] == "Short curated excerpt"
		}),
	).Return(&notifications.RenderedContent{InAppTitle: "Frank posted"}, nil)

	svc.On("Send", ctx, mock.Anything).Return(nil)

	err := listener.Handle(ctx, event)
	require.NoError(t, err)
	renderer.AssertExpectations(t)
}

func TestBuildActionURL_TrailingSlashNormalized(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)
	userRepo := new(mocks.MockUserRepository)
	feedItemRepo := new(mocks.MockFeedItemRepository)

	// Create listener with a trailing-slash base URL
	listener := notifications.NewNotificationEventListener(notifications.NotificationEventListenerDeps{
		NotifSvc:        svc,
		Resolver:        resolver,
		Renderer:        renderer,
		UserRepo:        userRepo,
		FeedItemRepo:    feedItemRepo,
		FrontendBaseURL: "https://app.example.com/",
		AppMetrics:      nil,
		Logger:          newTestLogger(),
	})
	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "item-ts",
		UserID:   "user-1",
		TeamID:   "team-1",
		FeedID:   "feed-1",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})

	resolver.On("ResolveForEvent", ctx, event).
		Return([]string{"recipient-1"}, "team-1", nil)

	userRepo.On("GetByID", ctx, "user-1").
		Return(&models.User{ID: "user-1", Name: "Grace"}, nil)

	feedItemRepo.On("GetByID", ctx, "user-1", "team-1", "item-ts").
		Return(&models.FeedItem{ID: "item-ts", Title: "A post"}, nil)

	renderer.On("Render", mock.Anything, mock.MatchedBy(func(data map[string]interface{}) bool {
		u, _ := data["action_url"].(string)
		// Must be exactly one slash, not double
		return u == "https://app.example.com/feed-items/item-ts"
	})).Return(&notifications.RenderedContent{InAppTitle: "Grace posted"}, nil)

	svc.On("Send", ctx, mock.MatchedBy(func(req *notifications.SendRequest) bool {
		return req.ActionURL == "https://app.example.com/feed-items/item-ts"
	})).Return(nil)

	err := listener.Handle(ctx, event)
	require.NoError(t, err)
	svc.AssertExpectations(t)
	renderer.AssertExpectations(t)
}

func TestBuildActionURL_EmptyBase_ProducesEmptyURL(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)
	userRepo := new(mocks.MockUserRepository)
	feedItemRepo := new(mocks.MockFeedItemRepository)

	// Create listener with empty frontendBaseURL
	listener := notifications.NewNotificationEventListener(notifications.NotificationEventListenerDeps{
		NotifSvc:        svc,
		Resolver:        resolver,
		Renderer:        renderer,
		UserRepo:        userRepo,
		FeedItemRepo:    feedItemRepo,
		FrontendBaseURL: "",
		AppMetrics:      nil,
		Logger:          newTestLogger(),
	})
	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "item-em",
		UserID:   "user-1",
		TeamID:   "team-1",
		FeedID:   "feed-1",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})

	resolver.On("ResolveForEvent", ctx, event).
		Return([]string{"recipient-1"}, "team-1", nil)

	userRepo.On("GetByID", ctx, "user-1").
		Return(&models.User{ID: "user-1", Name: "Hank"}, nil)

	feedItemRepo.On("GetByID", ctx, "user-1", "team-1", "item-em").
		Return(&models.FeedItem{ID: "item-em", Title: "A post"}, nil)

	renderer.On("Render", mock.Anything, mock.MatchedBy(func(data map[string]interface{}) bool {
		u, _ := data["action_url"].(string)
		// Empty base → empty URL (not a relative path like "/feed-items/...")
		return u == ""
	})).Return(&notifications.RenderedContent{InAppTitle: "Hank posted"}, nil)

	svc.On("Send", ctx, mock.MatchedBy(func(req *notifications.SendRequest) bool {
		return req.ActionURL == ""
	})).Return(nil)

	err := listener.Handle(ctx, event)
	require.NoError(t, err)
	svc.AssertExpectations(t)
	renderer.AssertExpectations(t)
}

func TestNotificationEventListener_Handle_BackfillOrigin_Skips(t *testing.T) {
	ctx := context.Background()

	svc := new(mockNotificationService)
	resolver := new(mockRecipientResolver)
	renderer := new(mockTemplateRenderer)

	listener := newTestListener(svc, resolver, renderer, nil, nil)

	event := events.NewFeedItemCreatedEvent(events.FeedItemCreatedPayload{
		ItemID:   "item-1",
		UserID:   "author-id",
		TeamID:   "team-abc",
		FeedID:   "feed-1",
		Title:    "",
		Content:  "",
		Excerpt:  "",
		PostedAt: time.Now(),
	})
	events.MarkBackfillOrigin(event)

	err := listener.Handle(ctx, event)

	require.NoError(t, err)
	// A backfill-origin event must short-circuit before recipient resolution and
	// dispatch, so no user-visible notifications are sent for historical items.
	resolver.AssertNotCalled(t, "ResolveForEvent", mock.Anything, mock.Anything)
	svc.AssertNotCalled(t, "Send", mock.Anything, mock.Anything)
}
