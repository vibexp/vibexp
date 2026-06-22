package notifications

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/observability/metrics"
	"github.com/vibexp/vibexp/internal/repositories"
)

// digestEmailSubject is the subject line used for all daily digest emails.
const digestEmailSubject = "Your vibexp daily digest"

// DigestEmailSender is a narrow interface for sending digest notification emails.
// services.EmailService satisfies this interface.
type DigestEmailSender interface {
	SendNotificationEmail(to, subject, htmlBody string) error
}

// DigestRunner flushes the notification_digest_queue by grouping pending rows
// by user, rendering one summary email per user, sending it, and marking the
// rows sent. It is invoked via HTTP POST /internal/jobs/notifications/digest,
// protected by Cloud Scheduler + OIDC (same pattern as the retention job).
type DigestRunner struct {
	digestRepo repositories.NotificationDigestQueueRepository
	notifRepo  repositories.NotificationRepository
	userRepo   repositories.UserRepository
	teamRepo   repositories.TeamRepository
	prefRepo   repositories.UserPreferencesRepository
	emailSvc   DigestEmailSender
	renderer   *TemplateRenderer
	appMetrics *metrics.Metrics
	logger     *slog.Logger
}

// NewDigestRunner creates a new DigestRunner.
func NewDigestRunner(
	digestRepo repositories.NotificationDigestQueueRepository,
	notifRepo repositories.NotificationRepository,
	userRepo repositories.UserRepository,
	teamRepo repositories.TeamRepository,
	prefRepo repositories.UserPreferencesRepository,
	emailSvc DigestEmailSender,
	renderer *TemplateRenderer,
	appMetrics *metrics.Metrics,
	logger *slog.Logger,
) *DigestRunner {
	return &DigestRunner{
		digestRepo: digestRepo,
		notifRepo:  notifRepo,
		userRepo:   userRepo,
		teamRepo:   teamRepo,
		prefRepo:   prefRepo,
		emailSvc:   emailSvc,
		renderer:   renderer,
		appMetrics: appMetrics,
		logger:     logger,
	}
}

// digestAdvisoryLockKey is the PostgreSQL advisory lock key for the digest runner.
// Only one Run invocation may proceed at a time, preventing concurrent schedulers from
// double-sending emails. The value must remain stable across application versions.
const digestAdvisoryLockKey int64 = 7_481_293_001

// Run processes all pending digest queue rows whose scheduled_for < now.
// Each user gets at most one email per run, containing all their pending notifications.
// An advisory lock is held for the duration of the run to prevent concurrent executions.
func (d *DigestRunner) Run(ctx context.Context, now time.Time) error {
	start := time.Now()

	// Acquire a Postgres advisory lock to ensure only one digest run executes at a time.
	// pg_try_advisory_lock is non-blocking: if the lock is held by another session the
	// function returns false immediately rather than waiting.
	locked, err := d.digestRepo.TryAdvisoryLock(ctx, digestAdvisoryLockKey)
	if err != nil {
		return fmt.Errorf("acquire digest advisory lock: %w", err)
	}
	if !locked {
		d.logger.Info("digest: another run is in progress, skipping")
		return nil
	}
	defer func() {
		if releaseErr := d.digestRepo.ReleaseAdvisoryLock(ctx, digestAdvisoryLockKey); releaseErr != nil {
			d.logger.With("error", releaseErr.Error()).Warn("digest: failed to release advisory lock")
		}
	}()

	pending, err := d.digestRepo.FetchPending(ctx, now)
	if err != nil {
		return fmt.Errorf("fetch pending digest queue rows: %w", err)
	}

	d.appMetrics.RecordDigestQueueDepth(ctx, len(pending))

	if len(pending) == 0 {
		d.appMetrics.RecordDigestRunnerDuration(ctx, time.Since(start))
		return nil
	}

	byUser := groupQueueRowsByUser(pending)
	// Sort user IDs for deterministic processing order, matching the ORDER BY user_id
	// used in FetchPending and making test behaviour predictable.
	userIDs := make([]string, 0, len(byUser))
	for uid := range byUser {
		userIDs = append(userIDs, uid)
	}
	sort.Strings(userIDs)
	for _, userID := range userIDs {
		d.processUserDigest(ctx, userID, byUser[userID], now)
	}

	d.appMetrics.RecordDigestRunnerDuration(ctx, time.Since(start))
	return nil
}

// processUserDigest handles digest delivery for a single user.
// Any error is logged but does not abort other users.
//
// Retry semantics: rows are only marked sent after a successful email send or an
// explicit skip (channel disabled, empty email, empty notification list). If an
// error occurs (DB transient, SMTP failure) rows remain pending so the next run
// retries them. Template wording is deliberately neutral ("recent activity") to
// avoid implying a specific time window, since retries may include older items.
func (d *DigestRunner) processUserDigest(
	ctx context.Context,
	userID string,
	items []*models.NotificationDigestQueueRow,
	now time.Time,
) {
	user, err := d.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repositories.ErrUserNotFound) {
			// User was deleted — drain the queue so rows don't accumulate forever.
			d.logger.With("user_id", userID).Info("digest: user not found, marking rows sent to drain queue")
			d.markSent(ctx, rowIDs(items), now)
		} else {
			// Transient DB error — skip without marking sent so next run retries.
			d.logger.With(
				"user_id", userID,
				"error", err.Error(),
			).Warn("digest: transient error loading user, will retry next run")
		}
		return
	}

	// Guard against legacy or OAuth-only users with no email address.
	// Mark rows sent to drain the queue; they will never be deliverable.
	if user.Email == "" {
		d.logger.With("user_id", userID).Info("digest: user has no email address, skipping and draining queue")
		d.markSent(ctx, rowIDs(items), now)
		d.appMetrics.RecordDigestEmailSent(ctx, "skipped")
		return
	}

	if !d.emailChannelEnabled(ctx, userID) {
		d.markSent(ctx, rowIDs(items), now)
		d.appMetrics.RecordDigestEmailSent(ctx, "skipped")
		return
	}

	notifs, ok := d.loadNotifications(ctx, userID, items)
	if !ok {
		return
	}

	if len(notifs) == 0 {
		d.markSent(ctx, rowIDs(items), now)
		return
	}

	d.sendDigestEmail(ctx, user, items, notifs, now)
}

// loadNotifications fetches notification details for the given queue rows.
// Returns (notifications, true) on success; (nil, false) on error (already logged).
// On error rows remain pending — the next run will retry them along with new ones.
// Template wording intentionally avoids "since your last digest" to stay accurate on retries.
func (d *DigestRunner) loadNotifications(
	ctx context.Context,
	userID string,
	items []*models.NotificationDigestQueueRow,
) ([]*models.Notification, bool) {
	notifs, err := d.notifRepo.GetByIDsForUser(ctx, userID, notificationIDs(items))
	if err != nil {
		d.logger.With(
			"user_id", userID,
			"error", err.Error(),
		).Error("digest: failed to load notifications for user")
		return nil, false
	}
	return notifs, true
}

// resolveTeamNames builds a map from team ID to display name, used by the digest template.
// Teams that cannot be fetched (deleted, transient error) fall back to the team ID string.
func (d *DigestRunner) resolveTeamNames(ctx context.Context, notifs []*models.Notification) map[string]string {
	names := make(map[string]string)
	for _, n := range notifs {
		if n.TeamID == "" || names[n.TeamID] != "" {
			continue
		}
		team, err := d.teamRepo.GetByID(ctx, n.TeamID)
		if err != nil {
			// Fall back to the raw ID so the email is still rendered.
			d.logger.With(
				"team_id", n.TeamID,
				"error", err.Error(),
			).Warn("digest: could not resolve team name, using team ID as fallback")
			names[n.TeamID] = n.TeamID
			continue
		}
		names[n.TeamID] = team.Name
	}
	return names
}

// sendDigestEmail renders and sends the digest email, then marks rows sent.
// If MarkSent fails after a successful send, a warning is logged so ops can detect
// potential duplicate-email risk on the next run. The outbox pattern would eliminate
// this race but is deferred as over-engineering for current scale.
func (d *DigestRunner) sendDigestEmail(
	ctx context.Context,
	user *models.User,
	items []*models.NotificationDigestQueueRow,
	notifs []*models.Notification,
	now time.Time,
) {
	teamNames := d.resolveTeamNames(ctx, notifs)
	htmlBody, err := d.renderer.RenderDigestEmailWithTeamNames(user, notifs, teamNames)
	if err != nil {
		d.logger.With(
			"user_id", user.ID,
			"error", err.Error(),
		).Error("digest: failed to render digest email")
		d.appMetrics.RecordDigestEmailSent(ctx, "failed")
		return
	}

	if sendErr := d.emailSvc.SendNotificationEmail(user.Email, digestEmailSubject, htmlBody); sendErr != nil {
		d.logger.With(
			"user_id", user.ID,
			"error", sendErr.Error(),
		).Warn("digest: send failed")
		d.appMetrics.RecordDigestEmailSent(ctx, "failed")
		return
	}

	// Email sent successfully — mark rows sent.
	// If MarkSent fails here, the same rows will be re-fetched on the next run,
	// producing a duplicate email. Log at Warn so this is visible in ops dashboards.
	if markErr := d.digestRepo.MarkSent(ctx, rowIDs(items), now); markErr != nil {
		d.logger.With(
			"user_id", user.ID,
			"row_count", len(items),
			"error", markErr.Error(),
		).Warn("digest: email sent but failed to mark rows sent — duplicate email risk on next run")
		d.appMetrics.RecordDigestEmailSent(ctx, "sent")
		return
	}

	d.appMetrics.RecordDigestEmailSent(ctx, "sent")
}

// emailChannelEnabled returns true when the user's global email channel toggle is on.
// Defaults to true when preferences cannot be loaded.
func (d *DigestRunner) emailChannelEnabled(ctx context.Context, userID string) bool {
	prefs, err := d.prefRepo.GetByUserID(ctx, userID)
	if err != nil || prefs == nil {
		return true
	}
	return prefs.Preferences.Notifications.Channels.Email
}

// markSent updates sent_at for the given row IDs, logging on error.
func (d *DigestRunner) markSent(ctx context.Context, ids []string, now time.Time) {
	if err := d.digestRepo.MarkSent(ctx, ids, now); err != nil {
		d.logger.With(
			"row_count", len(ids),
			"error", err.Error(),
		).Error("digest: failed to mark rows sent")
	}
}

// groupQueueRowsByUser groups digest queue rows by their UserID.
func groupQueueRowsByUser(
	rows []*models.NotificationDigestQueueRow,
) map[string][]*models.NotificationDigestQueueRow {
	groups := make(map[string][]*models.NotificationDigestQueueRow, len(rows))
	for _, r := range rows {
		groups[r.UserID] = append(groups[r.UserID], r)
	}
	return groups
}

// notificationIDs extracts unique NotificationID values from queue rows.
func notificationIDs(rows []*models.NotificationDigestQueueRow) []string {
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.NotificationID)
	}
	return ids
}

// rowIDs extracts the queue row IDs.
func rowIDs(rows []*models.NotificationDigestQueueRow) []string {
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return ids
}
