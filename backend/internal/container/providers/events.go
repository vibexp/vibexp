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

// Structured-logging field values shared across the event-system providers.
const (
	logServiceName           = "vibexp-api"
	logComponentEventManager = "event-manager"
)

// EventSystemDeps holds the event system dependencies.
type EventSystemDeps struct {
	EventManager *events.EventManager

	// embeddingProcessor is retained so shutdown can drain a concurrency-managed
	// processor (e.g. the EmbeddingDispatcher, #142) after the bus has stopped
	// producing new events. See ShutdownListeners.
	embeddingProcessor events.EmbeddingProcessor
}

// ShutdownListeners best-effort drains listeners that own worker goroutines
// (their generation runs off the bus). Call it after EventManager.Stop so no new
// events are produced while draining. It is safe to call when nothing needs it.
func (d *EventSystemDeps) ShutdownListeners() {
	if d == nil {
		return
	}
	if s, ok := d.embeddingProcessor.(interface{ Stop() }); ok {
		s.Stop()
	}
}

// ProvideEventManager creates and starts the event manager
func ProvideEventManager(
	cfg *config.Config,
	logger *slog.Logger,
	metricsRecorder *metrics.Metrics,
) *events.EventManager {
	logger.With(
		"service", logServiceName,
		"component", logComponentEventManager,
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
			"service", logServiceName,
			"component", logComponentEventManager,
			"error", fmt.Sprintf("%+v", err),
		)
		os.Exit(1)
	}

	return eventManager
}

// EventListenerDeps groups everything the event-listener registrations need.
// Wire fills it via wire.Struct (see the container ProviderSet).
type EventListenerDeps struct {
	EventManager       *events.EventManager
	Cfg                *config.Config
	Logger             *slog.Logger
	EmbeddingProcessor events.EmbeddingProcessor
	TeamService        services.TeamServiceInterface
	ProjectService     services.ProjectServiceInterface
	NotifSvc           notificationsvc.NotificationServiceInterface
	TeamMemberRepo     repositories.TeamMemberRepository
	UserRepo           repositories.UserRepository
	FeedItemRepo       repositories.FeedItemRepository
	AppMetrics         *metrics.Metrics
}

// ProvideEventSystemDeps creates the complete event system with all listeners,
// including the in-process embedding worker that generates embeddings off the bus.
// Embedding delivery is broker-free: there is no Pub/Sub forwarder or sync HTTP
// listener — the EmbeddingWorker subscribes to entity created/updated events and
// embeds them asynchronously via the bus's worker pool.
func ProvideEventSystemDeps(deps EventListenerDeps) *EventSystemDeps {
	registerEventListeners(deps)

	return &EventSystemDeps{
		EventManager:       deps.EventManager,
		embeddingProcessor: deps.EmbeddingProcessor,
	}
}

// registerEventListeners registers all event listeners to the event manager
func registerEventListeners(deps EventListenerDeps) {
	registerUserCreatedListener(deps.EventManager, deps.Logger)
	registerTeamCreationListener(deps.EventManager, deps.TeamService, deps.ProjectService, deps.Logger)
	registerNotificationEventListener(deps)
	registerEmbeddingWorker(deps.EventManager, deps.EmbeddingProcessor, deps.Logger)
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
			"service", logServiceName,
			"component", logComponentEventManager,
			"error", fmt.Sprintf("%+v", err),
		)
		return
	}

	logger.With(
		"service", logServiceName,
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
			"service", logServiceName,
			"component", logComponentEventManager,
			"error", fmt.Sprintf("%+v", err),
		)
	}
}

// registerNotificationEventListener registers the notification event listener
func registerNotificationEventListener(deps EventListenerDeps) {
	frontendBaseURL := deps.Cfg.Frontend.BaseURL
	logger := deps.Logger
	resolver := notificationsvc.NewRecipientResolver(deps.TeamMemberRepo)
	renderer := notificationsvc.NewTemplateRenderer(frontendBaseURL)
	listener := notificationsvc.NewNotificationEventListener(notificationsvc.NotificationEventListenerDeps{
		NotifSvc:        deps.NotifSvc,
		Resolver:        resolver,
		Renderer:        renderer,
		UserRepo:        deps.UserRepo,
		FeedItemRepo:    deps.FeedItemRepo,
		FrontendBaseURL: frontendBaseURL,
		AppMetrics:      deps.AppMetrics,
		Logger:          logger,
	})

	if err := deps.EventManager.Subscribe(listener); err != nil {
		logger.Error(
			"Failed to subscribe notification event listener",
			"service", logServiceName,
			"component", "notification-event-listener",
			"error", fmt.Sprintf("%+v", err),
		)
		return
	}

	logger.With(
		"service", logServiceName,
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
			"service", logServiceName,
			"component", "embedding-worker",
			"error", fmt.Sprintf("%+v", err),
		)
		return
	}

	logger.With(
		"service", logServiceName,
		"component", "embedding-worker",
		"event_types", worker.EventTypes(),
	).Info("Embedding worker registered successfully")
}
