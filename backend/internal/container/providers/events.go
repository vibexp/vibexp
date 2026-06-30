package providers

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/observability/metrics"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	notificationsvc "github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/pkg/events"
)

// EventSystemDeps holds the event system dependencies.
type EventSystemDeps struct {
	EventManager *events.EventManager
}

// ProvideEventManager creates and starts the event manager
func ProvideEventManager(
	cfg *config.Config,
	logger *slog.Logger,
	metricsRecorder *metrics.Metrics,
) *events.EventManager {
	logger.With(
		"service", "vibexp-api",
		"component", "event-manager",
		"worker_count", cfg.EventBus.WorkerCount,
		"buffer_size", cfg.EventBus.BufferSize,
		"max_retries", cfg.EventBus.MaxRetries,
		"retry_backoff", cfg.EventBus.RetryBackoff,
		"retry_jitter", cfg.EventBus.RetryJitter,
	).Info("Initializing event manager")

	eventBusConfig := events.EventBusConfig{
		Config:  cfg.EventBus, // Embedded config - no manual field copying needed
		Logger:  logger,
		Metrics: metricsRecorder,
	}
	eventManager := events.NewEventManager(eventBusConfig)

	if err := eventManager.Start(); err != nil {
		logger.Error(
			"Failed to start event manager",
			"service", "vibexp-api",
			"component", "event-manager",
			"error", fmt.Sprintf("%+v", err),
		)
		os.Exit(1)
	}

	return eventManager
}

// ProvideEventSystemDeps creates the complete event system with all listeners,
// including the in-process embedding worker that generates embeddings off the bus.
// Embedding delivery is broker-free: there is no Pub/Sub forwarder or sync HTTP
// listener — the EmbeddingWorker subscribes to entity created/updated events and
// embeds them asynchronously via the bus's worker pool.
func ProvideEventSystemDeps(
	eventManager *events.EventManager,
	cfg *config.Config,
	logger *slog.Logger,
	embeddingProcessor events.EmbeddingProcessor,
	teamService services.TeamServiceInterface,
	projectService services.ProjectServiceInterface,
	notifSvc notificationsvc.NotificationServiceInterface,
	teamMemberRepo repositories.TeamMemberRepository,
	userRepo repositories.UserRepository,
	feedItemRepo repositories.FeedItemRepository,
	appMetrics *metrics.Metrics,
) *EventSystemDeps {
	registerEventListeners(
		eventManager, cfg, logger, embeddingProcessor, teamService, projectService,
		notifSvc, teamMemberRepo, userRepo, feedItemRepo, appMetrics,
	)

	return &EventSystemDeps{
		EventManager: eventManager,
	}
}

// registerEventListeners registers all event listeners to the event manager
func registerEventListeners(
	eventManager *events.EventManager,
	cfg *config.Config,
	logger *slog.Logger,
	embeddingProcessor events.EmbeddingProcessor,
	teamService services.TeamServiceInterface,
	projectService services.ProjectServiceInterface,
	notifSvc notificationsvc.NotificationServiceInterface,
	teamMemberRepo repositories.TeamMemberRepository,
	userRepo repositories.UserRepository,
	feedItemRepo repositories.FeedItemRepository,
	appMetrics *metrics.Metrics,
) {
	registerUserCreatedListener(eventManager, logger)
	registerTeamCreationListener(eventManager, teamService, projectService, logger)
	registerNotificationEventListener(
		eventManager, notifSvc, teamMemberRepo,
		userRepo, feedItemRepo, cfg.Frontend.BaseURL, appMetrics, logger,
	)
	registerEmbeddingWorker(eventManager, embeddingProcessor, logger)
}

// registerTeamCreationListener registers the team creation listener for new users
func registerTeamCreationListener(
	eventManager *events.EventManager,
	teamService services.TeamServiceInterface,
	projectService services.ProjectServiceInterface,
	logger *slog.Logger,
) {
	listener := events.NewTeamCreationListener(teamService, projectService, logger)

	if err := eventManager.Subscribe(listener); err != nil {
		logger.Error(
			"Failed to subscribe team creation listener",
			"service", "vibexp-api",
			"component", "event-manager",
			"error", fmt.Sprintf("%+v", err),
		)
		return
	}

	logger.With(
		"service", "vibexp-api",
		"component", "team-creation-listener",
		"event_types", listener.EventTypes(),
	).Info("Team creation listener registered successfully")
}

// registerUserCreatedListener registers the user created event listener
func registerUserCreatedListener(eventManager *events.EventManager, logger *slog.Logger) {
	userCreatedListener := events.NewUserCreatedListener(logger)
	if err := eventManager.Subscribe(userCreatedListener); err != nil {
		logger.Error(
			"Failed to subscribe user.created listener",
			"service", "vibexp-api",
			"component", "event-manager",
			"error", fmt.Sprintf("%+v", err),
		)
	}
}

// registerNotificationEventListener registers the notification event listener
func registerNotificationEventListener(
	eventManager *events.EventManager,
	notifSvc notificationsvc.NotificationServiceInterface,
	teamMemberRepo repositories.TeamMemberRepository,
	userRepo repositories.UserRepository,
	feedItemRepo repositories.FeedItemRepository,
	frontendBaseURL string,
	appMetrics *metrics.Metrics,
	logger *slog.Logger,
) {
	resolver := notificationsvc.NewRecipientResolver(teamMemberRepo)
	renderer := notificationsvc.NewTemplateRenderer(frontendBaseURL)
	listener := notificationsvc.NewNotificationEventListener(
		notifSvc, resolver, renderer, userRepo, feedItemRepo, frontendBaseURL, appMetrics, logger,
	)

	if err := eventManager.Subscribe(listener); err != nil {
		logger.Error(
			"Failed to subscribe notification event listener",
			"service", "vibexp-api",
			"component", "notification-event-listener",
			"error", fmt.Sprintf("%+v", err),
		)
		return
	}

	logger.With(
		"service", "vibexp-api",
		"component", "notification-event-listener",
		"event_types", listener.EventTypes(),
	).Info("Notification event listener registered successfully")
}

// registerEmbeddingWorker registers the in-process async embedding worker. It
// subscribes to entity created/updated events and generates embeddings via the
// active system-wide provider. When no provider is configured the worker no-ops,
// so entity writes still succeed without embeddings.
func registerEmbeddingWorker(
	eventManager *events.EventManager, processor events.EmbeddingProcessor, logger *slog.Logger,
) {
	worker := events.NewEmbeddingWorker(processor, logger)

	if err := eventManager.Subscribe(worker); err != nil {
		logger.Error(
			"Failed to subscribe embedding worker",
			"service", "vibexp-api",
			"component", "embedding-worker",
			"error", fmt.Sprintf("%+v", err),
		)
		return
	}

	logger.With(
		"service", "vibexp-api",
		"component", "embedding-worker",
		"event_types", worker.EventTypes(),
	).Info("Embedding worker registered successfully")
}
