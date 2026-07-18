package notifications

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/vibexp/vibexp/internal/observability/metrics"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// Safe fallbacks used when a lookup fails, so a notification is never dropped
// because of a missing user or feed item record.
const (
	fallbackActorName     = "A team member"
	fallbackFeedItemTitle = "new post"
)

// NotificationEventListener subscribes to domain events and dispatches notifications
// to all team members (excluding the event author) via NotificationService.Send.
type NotificationEventListener struct {
	notifSvc        NotificationServiceInterface
	resolver        RecipientResolverInterface
	renderer        TemplateRendererInterface
	userRepo        repositories.UserRepository
	feedItemRepo    repositories.FeedItemRepository
	frontendBaseURL string
	appMetrics      *metrics.Metrics
	logger          *slog.Logger
}

// NewNotificationEventListener creates a new NotificationEventListener
func NewNotificationEventListener(
	notifSvc NotificationServiceInterface,
	resolver RecipientResolverInterface,
	renderer TemplateRendererInterface,
	userRepo repositories.UserRepository,
	feedItemRepo repositories.FeedItemRepository,
	frontendBaseURL string,
	appMetrics *metrics.Metrics,
	logger *slog.Logger,
) *NotificationEventListener {
	return &NotificationEventListener{
		notifSvc:        notifSvc,
		resolver:        resolver,
		renderer:        renderer,
		userRepo:        userRepo,
		feedItemRepo:    feedItemRepo,
		frontendBaseURL: frontendBaseURL,
		appMetrics:      appMetrics,
		logger:          logger,
	}
}

// EventTypes returns the domain event types this listener handles
func (l *NotificationEventListener) EventTypes() []string {
	return []string{events.EventTypeFeedItemCreated}
}

// Handle processes a domain event and sends notifications to resolved recipients.
// Non-fatal errors (e.g. one recipient failing) are logged and not returned so
// the event bus does not retry the entire batch.
func (l *NotificationEventListener) Handle(ctx context.Context, event events.Event) error {
	if event == nil {
		return nil
	}

	// Backfill-origin events are replays of historical entities, not genuine user
	// actions. Notifying on them would re-send a user-visible notification for
	// every historical feed item, so skip them entirely.
	if events.IsBackfillOrigin(event) {
		return nil
	}

	switch event.Type() {
	case events.EventTypeFeedItemCreated:
		return l.handleFeedItemCreated(ctx, event)
	default:
		// Not our event; skip silently
		return nil
	}
}

func (l *NotificationEventListener) handleFeedItemCreated(
	ctx context.Context, event events.Event,
) error {
	recipients, teamID, err := l.resolver.ResolveForEvent(ctx, event)
	if err != nil {
		l.recordListenerError(ctx, event.Type())
		l.logError(event.Type(), event.UserID(), "Failed to resolve notification recipients", err)
		return nil
	}

	if len(recipients) == 0 {
		return nil
	}

	rendered, actionURL, ok := l.renderFeedItemTemplate(ctx, event, teamID)
	if !ok {
		return nil
	}

	l.sendToRecipients(ctx, event, recipients, teamID, rendered, actionURL)

	return nil
}

func (l *NotificationEventListener) renderFeedItemTemplate(
	ctx context.Context, event events.Event, teamID string,
) (*RenderedContent, string, bool) {
	payload, ok := event.Payload().(*events.FeedItemCreatedPayload)
	if !ok {
		return nil, "", false
	}

	actorName := l.resolveActorName(ctx, event.UserID())
	title := l.resolveFeedItemTitle(ctx, payload)
	actionURL := l.buildActionURL(payload.ItemID)

	templateData := map[string]interface{}{
		"actor_name": actorName,
		"team_id":    teamID,
		"item_id":    payload.ItemID,
		"title":      title,
		"action_url": actionURL,
	}

	rendered, err := l.renderer.Render("feed.item.created", templateData)
	if err != nil {
		l.recordListenerError(ctx, event.Type())
		l.logError(event.Type(), "", "Failed to render notification template", err)
		return nil, "", false
	}

	return rendered, actionURL, true
}

// resolveActorName looks up the display name for a user ID.
// On lookup failure it logs a warning and returns a safe fallback so the
// notification is never dropped due to a missing user record.
func (l *NotificationEventListener) resolveActorName(ctx context.Context, userID string) string {
	if l.userRepo == nil {
		return fallbackActorName
	}

	user, err := l.userRepo.GetByID(ctx, userID)
	if err != nil {
		l.logWarn("resolve actor name", userID, "Failed to look up actor for notification; using fallback", err)
		return fallbackActorName
	}

	if user.Name != "" {
		return user.Name
	}

	if idx := strings.Index(user.Email, "@"); idx > 0 {
		return user.Email[:idx]
	}

	return fallbackActorName
}

// resolveFeedItemTitle looks up the real title of a feed item.
// On lookup failure it logs a warning and returns a safe fallback so the
// notification is never dropped due to a missing feed item record.
// Note: GetByID enforces team-membership using payload.UserID (the post author).
// This works because the author is always a team member at event time; if the author
// is later removed from the team, the lookup may return not-found and we fall back.
func (l *NotificationEventListener) resolveFeedItemTitle(
	ctx context.Context, payload *events.FeedItemCreatedPayload,
) string {
	if l.feedItemRepo == nil {
		return fallbackFeedItemTitle
	}

	item, err := l.feedItemRepo.GetByID(ctx, payload.UserID, payload.TeamID, payload.ItemID)
	if err != nil {
		l.logWarn("resolve feed item title", payload.ItemID,
			"Failed to look up feed item for notification; using fallback", err)
		return fallbackFeedItemTitle
	}

	if item.Title != "" {
		return item.Title
	}

	if item.Excerpt != "" {
		return item.Excerpt
	}

	if item.Content != "" {
		return runeAwareTruncate(item.Content, 80)
	}

	return fallbackFeedItemTitle
}

// buildActionURL constructs the deep-link URL for the feed item.
// Returns an empty string when frontendBaseURL is not configured rather than
// producing a relative URL that would be unclickable in email clients.
func (l *NotificationEventListener) buildActionURL(itemID string) string {
	base := strings.TrimRight(l.frontendBaseURL, "/")
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s/feed-items/%s", base, itemID)
}

// runeAwareTruncate truncates s to at most maxRunes runes, appending "…" when
// truncated. Byte-slicing a UTF-8 string can split multi-byte characters, so
// we operate on runes instead.
func runeAwareTruncate(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…"
}

func (l *NotificationEventListener) sendToRecipients(
	ctx context.Context, event events.Event, recipients []string,
	teamID string, rendered *RenderedContent, actionURL string,
) {
	for _, recipientID := range recipients {
		req := &SendRequest{
			RecipientUserID:      recipientID,
			TeamID:               teamID,
			Type:                 "feed.item.created",
			Category:             CategoryLow,
			Title:                rendered.InAppTitle,
			Body:                 rendered.InAppBody,
			ActionURL:            actionURL,
			RenderedEmailSubject: rendered.EmailSubject,
			RenderedEmailHTML:    rendered.EmailBodyHTML,
			DedupeKey:            fmt.Sprintf("feed.item.created:%s:%s", rendered.EntityID, recipientID),
		}

		if err := l.notifSvc.Send(ctx, req); err != nil {
			l.recordListenerError(ctx, event.Type())
			l.logError(event.Type(), recipientID, "Failed to send notification for event", err)
		}
	}
}

func (l *NotificationEventListener) logError(eventType, contextID, msg string, err error) {
	if l.logger == nil {
		return
	}
	l.logger.With(
		"event_type", eventType,
		"context_id", contextID,
		"error", fmt.Sprintf("%+v", err),
	).Error(msg)
}

func (l *NotificationEventListener) logWarn(operation, contextID, msg string, err error) {
	if l.logger == nil {
		return
	}
	l.logger.With("error", err).With(
		"operation", operation,
		"context_id", contextID,
	).Warn(msg)
}

// recordListenerError increments the listener error counter
func (l *NotificationEventListener) recordListenerError(ctx context.Context, eventType string) {
	if l.appMetrics == nil || l.appMetrics.NotificationsListenerErrs == nil {
		return
	}

	l.appMetrics.NotificationsListenerErrs.Add(ctx, 1,
		metric.WithAttributes(attribute.String("event_type", eventType)),
	)
}
