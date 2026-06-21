package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/observability/metrics"
	"github.com/vibexp/vibexp/internal/repositories"
)

const (
	retentionDays    = 90
	defaultListLimit = 20
	maxListLimit     = 100
)

// NotificationServiceInterface defines the public contract for the notification service
type NotificationServiceInterface interface {
	Send(ctx context.Context, req *SendRequest) error
	ListForUser(ctx context.Context, userID string, f ListFilters) ([]*Notification, error)
	GetUnreadCount(ctx context.Context, userID string) (int, error)
	MarkRead(ctx context.Context, userID, notifID string) error
	MarkAllRead(ctx context.Context, userID string) error
	RunRetentionJob(ctx context.Context) error
}

// NotificationService orchestrates notification creation and multi-channel delivery
type NotificationService struct {
	notifRepo    repositories.NotificationRepository
	deliveryRepo repositories.NotificationDeliveryRepository
	prefRepo     repositories.UserPreferencesRepository
	userRepo     repositories.UserRepository
	channels     []Channel
	appMetrics   *metrics.Metrics
	logger       *logrus.Logger
}

// Ensure NotificationService implements NotificationServiceInterface
var _ NotificationServiceInterface = (*NotificationService)(nil)

// NewNotificationService creates a new NotificationService
func NewNotificationService(
	notifRepo repositories.NotificationRepository,
	deliveryRepo repositories.NotificationDeliveryRepository,
	prefRepo repositories.UserPreferencesRepository,
	userRepo repositories.UserRepository,
	channels []Channel,
	appMetrics *metrics.Metrics,
	logger *logrus.Logger,
) *NotificationService {
	return &NotificationService{
		notifRepo:    notifRepo,
		deliveryRepo: deliveryRepo,
		prefRepo:     prefRepo,
		userRepo:     userRepo,
		channels:     channels,
		appMetrics:   appMetrics,
		logger:       logger,
	}
}

// Send persists a notification and dispatches it to all registered channels.
// If the dedupe_key already exists for the recipient, the call is silently
// ignored (idempotent). Channel delivery failures are recorded but do not
// cause Send to return an error.
func (s *NotificationService) Send(ctx context.Context, req *SendRequest) error {
	var entityRefJSON json.RawMessage
	if len(req.EntityRef) > 0 {
		b, err := json.Marshal(req.EntityRef)
		if err != nil {
			return fmt.Errorf("marshal entity_ref: %w", err)
		}
		entityRefJSON = b
	}

	n := &models.Notification{
		RecipientUserID: req.RecipientUserID,
		TeamID:          req.TeamID,
		Type:            string(req.Type),
		Category:        string(req.Category),
		Title:           req.Title,
		Body:            req.Body,
		ActionURL:       req.ActionURL,
		EntityRef:       entityRefJSON,
		DedupeKey:       req.DedupeKey,
	}

	if err := s.notifRepo.Insert(ctx, n); err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}

	// If Insert returned without a new ID (dedupe hit), stop here
	if n.ID == "" {
		return nil
	}

	domainNotif := modelToDomain(n)
	domainNotif.RenderedEmailSubject = req.RenderedEmailSubject
	domainNotif.RenderedEmailHTML = req.RenderedEmailHTML
	s.dispatchChannels(ctx, domainNotif, req)

	return nil
}

// dispatchChannels delivers the notification to all registered channels.
// Aborts silently when the user cannot be loaded — delivery cannot proceed
// without a valid user (e.g. no email address for the email channel).
func (s *NotificationService) dispatchChannels(
	ctx context.Context,
	n *Notification,
	req *SendRequest,
) {
	user, userPrefs := s.loadUserAndPrefs(ctx, req.RecipientUserID)
	if user == nil {
		// User load failed; already logged in loadUserAndPrefs. Skip all channels.
		return
	}

	for _, ch := range s.channels {
		// Respect the global per-channel master switch before per-type preferences
		if !channelEnabledGlobally(userPrefs, ch.Name()) {
			s.recordDelivery(ctx, n.ID, ch, DeliveryResult{
				Status: StatusSkipped,
				Reason: "channel disabled globally",
			}, 0)
			continue
		}

		start := time.Now()
		typePref := resolveTypePreference(userPrefs, string(req.Type))

		result := ch.Deliver(ctx, n, user, typePref)
		duration := time.Since(start)

		s.recordDelivery(ctx, n.ID, ch, result, duration)
	}
}

// channelEnabledGlobally returns true if the user's global channel preference
// allows delivery on the given channel. Defaults to true when prefs are nil.
func channelEnabledGlobally(prefs *models.Preferences, ch ChannelName) bool {
	if prefs == nil {
		return true
	}
	switch ch {
	case ChannelInApp:
		return prefs.Notifications.Channels.InApp
	case ChannelEmail:
		return prefs.Notifications.Channels.Email
	case ChannelWebPush:
		return prefs.Notifications.Channels.WebPush
	default:
		return true
	}
}

// loadUserAndPrefs retrieves the user and their notification preferences.
// Returns nil for both on any error; channels handle nil prefs defensively.
func (s *NotificationService) loadUserAndPrefs(
	ctx context.Context,
	userID string,
) (*models.User, *models.Preferences) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if s.logger != nil {
			s.logger.WithFields(logrus.Fields{
				"user_id": userID,
				"error":   fmt.Sprintf("%+v", err),
			}).Warn("Failed to load user for notification delivery")
		}

		return nil, nil
	}

	prefs, err := s.prefRepo.GetByUserID(ctx, userID)
	if err != nil || prefs == nil {
		defaults := models.DefaultPreferences()
		return user, &defaults
	}

	p := prefs.Preferences
	return user, &p
}

// recordDelivery persists the delivery outcome and emits observability signals
func (s *NotificationService) recordDelivery(
	ctx context.Context,
	notifID string,
	ch Channel,
	result DeliveryResult,
	duration time.Duration,
) {
	delivery := &models.NotificationDelivery{
		NotificationID: notifID,
		Channel:        string(ch.Name()),
		Status:         string(result.Status),
		Reason:         result.Reason,
		Attempts:       1,
	}

	if result.Status == StatusSent {
		now := time.Now()
		delivery.DeliveredAt = &now
	}

	if err := s.deliveryRepo.Insert(ctx, delivery); err != nil {
		if s.logger != nil {
			s.logger.WithFields(logrus.Fields{
				"notification_id": notifID,
				"channel":         ch.Name(),
				"error":           fmt.Sprintf("%+v", err),
			}).Error("Failed to record notification delivery")
		}
	}

	s.recordDeliveryMetrics(ctx, ch.Name(), result, duration)
}

// recordDeliveryMetrics emits OTel metrics for notification delivery
func (s *NotificationService) recordDeliveryMetrics(
	ctx context.Context,
	channel ChannelName,
	result DeliveryResult,
	duration time.Duration,
) {
	if s.appMetrics == nil {
		return
	}

	if s.appMetrics.NotificationsSentTotal != nil {
		s.appMetrics.NotificationsSentTotal.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("channel", string(channel)),
				attribute.String("status", string(result.Status)),
			),
		)
	}

	if s.appMetrics.NotificationsDeliveryDur != nil {
		s.appMetrics.NotificationsDeliveryDur.Record(ctx, duration.Seconds(),
			metric.WithAttributes(
				attribute.String("channel", string(channel)),
			),
		)
	}
}

// resolveTypePreference returns the per-type preference for the notification type.
// Returns nil when no preference is configured for the type (channels use defaults).
func resolveTypePreference(
	prefs *models.Preferences,
	notifType string,
) *models.NotificationTypePreference {
	if prefs == nil {
		return nil
	}

	if prefs.Notifications.Types == nil {
		defaults := models.DefaultNotificationPreferences()
		if pref, ok := defaults.Types[notifType]; ok {
			return &pref
		}

		return nil
	}

	pref, ok := prefs.Notifications.Types[notifType]
	if !ok {
		return nil
	}

	return &pref
}

// ListForUser returns paginated notifications for the user
func (s *NotificationService) ListForUser(
	ctx context.Context, userID string, f ListFilters,
) ([]*Notification, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	repoFilters := repositories.NotificationListFilters{
		UnreadOnly: f.UnreadOnly,
		Limit:      limit,
		Offset:     f.Offset,
	}

	rows, err := s.notifRepo.ListForUser(ctx, userID, repoFilters)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}

	result := make([]*Notification, 0, len(rows))
	for _, r := range rows {
		result = append(result, modelToDomain(r))
	}

	return result, nil
}

// GetUnreadCount returns the count of unread notifications for the user
func (s *NotificationService) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	count, err := s.notifRepo.GetUnreadCount(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("get unread count: %w", err)
	}

	return count, nil
}

// MarkRead marks a single notification as read for the user
func (s *NotificationService) MarkRead(ctx context.Context, userID, notifID string) error {
	if err := s.notifRepo.MarkRead(ctx, userID, notifID); err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}

	return nil
}

// MarkAllRead marks all notifications as read for the user
func (s *NotificationService) MarkAllRead(ctx context.Context, userID string) error {
	if err := s.notifRepo.MarkAllRead(ctx, userID); err != nil {
		return fmt.Errorf("mark all notifications read: %w", err)
	}

	return nil
}

// RunRetentionJob deletes notifications older than 90 days.
// Called via HTTP from Cloud Scheduler.
func (s *NotificationService) RunRetentionJob(ctx context.Context) error {
	before := time.Now().UTC().Add(-retentionDays * 24 * time.Hour)

	count, err := s.notifRepo.DeleteOlderThan(ctx, before)
	if err != nil {
		return fmt.Errorf("run notification retention job: %w", err)
	}

	if s.logger != nil {
		s.logger.WithFields(logrus.Fields{
			"deleted_count":  count,
			"older_than":     before.Format(time.RFC3339),
			"retention_days": retentionDays,
		}).Info("Notification retention job completed")
	}

	return nil
}

// modelToDomain converts a models.Notification to the domain Notification type
func modelToDomain(n *models.Notification) *Notification {
	return &Notification{
		ID:              n.ID,
		RecipientUserID: n.RecipientUserID,
		TeamID:          n.TeamID,
		Type:            NotificationType(n.Type),
		Category:        Category(n.Category),
		Title:           n.Title,
		Body:            n.Body,
		ActionURL:       n.ActionURL,
		DedupeKey:       n.DedupeKey,
		ReadAt:          n.ReadAt,
		DismissedAt:     n.DismissedAt,
		CreatedAt:       n.CreatedAt,
	}
}
