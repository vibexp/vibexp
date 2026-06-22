package providers

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"cloud.google.com/go/pubsub" //nolint:staticcheck // v2 has breaking changes, will upgrade when stable

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/observability/metrics"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/crm"
	notificationsvc "github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/pkg/events"
)

// EventSystemDeps holds the event system dependencies
type EventSystemDeps struct {
	EventManager    *events.EventManager
	PubSubClient    *pubsub.Client
	PubSubForwarder *events.PubSubForwarderListener
}

// ProvideEventManager creates and starts the event manager
func ProvideEventManager(
	cfg *config.Config,
	logger *slog.Logger,
	metricsRecorder *metrics.Metrics,
) *events.EventManager {
	logger.Info(
		"Initializing event manager",
		"service", "vibexp-api",
		"component", "event-manager",
		"worker_count", cfg.EventBus.WorkerCount,
		"buffer_size", cfg.EventBus.BufferSize,
		"max_retries", cfg.EventBus.MaxRetries,
		"retry_backoff", cfg.EventBus.RetryBackoff,
		"retry_jitter", cfg.EventBus.RetryJitter,
	)

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

// ProvideEventSystemDeps creates the complete event system with all listeners
func ProvideEventSystemDeps(
	eventManager *events.EventManager,
	cfg *config.Config,
	logger *slog.Logger,
	embeddingHandlers events.EmbeddingHandlers,
	teamService services.TeamServiceInterface,
	projectService services.ProjectServiceInterface,
	notifSvc notificationsvc.NotificationServiceInterface,
	teamMemberRepo repositories.TeamMemberRepository,
	userRepo repositories.UserRepository,
	feedItemRepo repositories.FeedItemRepository,
	appMetrics *metrics.Metrics,
) *EventSystemDeps {
	// Register event listeners
	registerEventListeners(
		eventManager, cfg, logger, teamService, projectService,
		notifSvc, teamMemberRepo, userRepo, feedItemRepo, appMetrics,
	)

	// Initialize Pub/Sub forwarder if configured
	var pubsubClient *pubsub.Client
	var pubsubForwarder *events.PubSubForwarderListener

	if shouldInitializePubSubForwarder(cfg, logger) {
		eventTypes := cfg.GetPubSubForwardedEventTypes()
		logPubSubInitialization(cfg, eventTypes, logger)

		pubsubClient = createPubSubClient(cfg, logger)
		if pubsubClient != nil {
			pubsubForwarder = createPubSubForwarder(pubsubClient, cfg, eventTypes, logger)
			if pubsubForwarder != nil {
				subscribePubSubForwarder(eventManager, pubsubForwarder, eventTypes, logger)
			}
		}
	}

	// Initialize HTTP sync listener if Pub/Sub is disabled
	if shouldInitializeHTTPSyncListener(cfg, logger) {
		eventTypes := getHTTPSyncEventTypes()
		logHTTPSyncInitialization(eventTypes, logger)

		listener := createHTTPSyncListener(cfg, eventTypes, embeddingHandlers, logger)
		if listener != nil {
			subscribeHTTPSyncListener(eventManager, listener, eventTypes, logger)
		}
	}

	return &EventSystemDeps{
		EventManager:    eventManager,
		PubSubClient:    pubsubClient,
		PubSubForwarder: pubsubForwarder,
	}
}

// registerEventListeners registers all event listeners to the event manager
func registerEventListeners(
	eventManager *events.EventManager,
	cfg *config.Config,
	logger *slog.Logger,
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
	registerHubSpotCRMListener(eventManager, cfg, logger)
	registerNotificationEventListener(
		eventManager, notifSvc, teamMemberRepo,
		userRepo, feedItemRepo, cfg.FrontendBaseURL, appMetrics, logger,
	)
	registerNoOpListener(eventManager, logger)
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

	logger.Info(
		"Team creation listener registered successfully",
		"service", "vibexp-api",
		"component", "team-creation-listener",
		"event_types", listener.EventTypes(),
	)
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

// registerHubSpotCRMListener registers the HubSpot CRM listener if configured
func registerHubSpotCRMListener(eventManager *events.EventManager, cfg *config.Config, logger *slog.Logger) {
	if cfg.HubSpotCRMAccessKey == "" {
		logger.Info(
			"HubSpot CRM access key not configured, skipping listener registration",
			"service", "vibexp-api",
			"component", "hubspot-crm-listener",
		)
		return
	}

	hubspotService := crm.NewHubSpotService(cfg.HubSpotCRMAccessKey, logger)
	hubspotListener := events.NewHubSpotCRMListener(hubspotService, logger)

	if err := eventManager.Subscribe(hubspotListener); err != nil {
		logger.Error(
			"Failed to subscribe HubSpot CRM listener",
			"service", "vibexp-api",
			"component", "event-manager",
			"error", fmt.Sprintf("%+v", err),
		)
	} else {
		logger.Info(
			"HubSpot CRM listener registered successfully",
			"service", "vibexp-api",
			"component", "hubspot-crm-listener",
			"event_types", hubspotListener.EventTypes(),
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

	logger.Info(
		"Notification event listener registered successfully",
		"service", "vibexp-api",
		"component", "notification-event-listener",
		"event_types", listener.EventTypes(),
	)
}

// registerNoOpListener registers the no-op listener for events without specific handlers
func registerNoOpListener(eventManager *events.EventManager, logger *slog.Logger) {
	noOpListener := events.NewNoOpListener(
		events.EventTypePromptCreated,
		events.EventTypePromptUpdated,
		events.EventTypeArtifactCreated,
		events.EventTypeArtifactUpdated,
		events.EventTypeMemoryCreated,
		events.EventTypeMemoryUpdated,
		events.EventTypeBlueprintCreated,
		events.EventTypeBlueprintUpdated,
		events.EventTypeFeedItemCreated,
		events.EventTypeFeedItemReplyCreated,
	)

	if err := eventManager.Subscribe(noOpListener); err != nil {
		logger.Error(
			"Failed to subscribe no-op listener",
			"service", "vibexp-api",
			"component", "event-manager",
			"error", fmt.Sprintf("%+v", err),
		)
	}
}

// shouldInitializePubSubForwarder checks if Pub/Sub forwarder should be initialized
func shouldInitializePubSubForwarder(cfg *config.Config, logger *slog.Logger) bool {
	if cfg.EventBackendMode() != config.EventBackendPubSub {
		logger.Info(
			"Pub/Sub event backend not selected, skipping forwarder initialization",
			"service", "vibexp-api",
			"component", "pubsub-forwarder",
			"event_backend", cfg.EventBackendMode(),
		)
		return false
	}

	if cfg.GCPProjectID == "" {
		logger.Warn(
			"Pub/Sub forwarding enabled but GCP_PROJECT_ID not configured, skipping",
			"service", "vibexp-api",
			"component", "pubsub-forwarder",
		)
		return false
	}

	return true
}

// logPubSubInitialization logs Pub/Sub initialization details
func logPubSubInitialization(cfg *config.Config, eventTypes []string, logger *slog.Logger) {
	logger.Info(
		"Initializing Pub/Sub forwarder",
		"service", "vibexp-api",
		"component", "pubsub-forwarder",
		"project_id", cfg.GCPProjectID,
		"topic", cfg.PubSubEventsTopicName,
		"event_types", eventTypes,
	)
}

// createPubSubClient creates and returns a Pub/Sub client
func createPubSubClient(cfg *config.Config, logger *slog.Logger) *pubsub.Client {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, cfg.GCPProjectID)
	if err != nil {
		logger.Error(
			"Failed to create Pub/Sub client, forwarding disabled",
			"service", "vibexp-api",
			"component", "pubsub-forwarder",
			"error", fmt.Sprintf("%+v", err),
		)
		return nil
	}
	return client
}

// createPubSubForwarder creates and returns a Pub/Sub forwarder listener
func createPubSubForwarder(
	client *pubsub.Client, cfg *config.Config, eventTypes []string, logger *slog.Logger,
) *events.PubSubForwarderListener {
	forwarder, err := events.NewPubSubForwarderListener(events.PubSubForwarderConfig{
		Client:       client,
		TopicName:    cfg.PubSubEventsTopicName,
		EventTypes:   eventTypes,
		Logger:       logger,
		PublishAsync: false,
	})
	if err != nil {
		logger.Error(
			"Failed to create Pub/Sub forwarder",
			"service", "vibexp-api",
			"component", "pubsub-forwarder",
			"error", fmt.Sprintf("%+v", err),
		)
		return nil
	}
	return forwarder
}

// subscribePubSubForwarder subscribes the forwarder to the event manager
func subscribePubSubForwarder(
	eventManager *events.EventManager, forwarder *events.PubSubForwarderListener,
	eventTypes []string, logger *slog.Logger,
) {
	if err := eventManager.Subscribe(forwarder); err != nil {
		logger.Error(
			"Failed to subscribe Pub/Sub forwarder",
			"service", "vibexp-api",
			"component", "pubsub-forwarder",
			"error", fmt.Sprintf("%+v", err),
		)
		return
	}

	logger.Info(
		"Pub/Sub forwarder initialized successfully",
		"service", "vibexp-api",
		"component", "pubsub-forwarder",
		"event_types", eventTypes,
	)
}

// shouldInitializeHTTPSyncListener checks if the in-process (sync) HTTP listener
// should be initialized. This is the broker-free embedding path: the default
// "sync" event backend POSTs events straight to the AI service so self-hosters
// get semantic search without Pub/Sub.
func shouldInitializeHTTPSyncListener(cfg *config.Config, logger *slog.Logger) bool {
	if cfg.EventBackendMode() != config.EventBackendSync {
		logger.Info(
			"Sync event backend not selected, skipping HTTP sync listener",
			"service", "vibexp-api",
			"component", "http-sync-listener",
			"event_backend", cfg.EventBackendMode(),
		)
		return false
	}

	if cfg.AIServiceURL == "" {
		logger.Info(
			"AI_SERVICE_URL not configured, skipping HTTP sync listener",
			"service", "vibexp-api",
			"component", "http-sync-listener",
		)
		return false
	}

	return true
}

// getHTTPSyncEventTypes returns the list of event types to forward via HTTP
func getHTTPSyncEventTypes() []string {
	return []string{
		events.EventTypePromptCreated,
		events.EventTypePromptUpdated,
		events.EventTypeArtifactCreated,
		events.EventTypeArtifactUpdated,
		events.EventTypeMemoryCreated,
		events.EventTypeMemoryUpdated,
		events.EventTypeBlueprintCreated,
		events.EventTypeBlueprintUpdated,
		events.EventTypeFeedItemCreated,
		events.EventTypeFeedItemReplyCreated,
	}
}

// logHTTPSyncInitialization logs HTTP sync listener initialization details
func logHTTPSyncInitialization(eventTypes []string, logger *slog.Logger) {
	logger.Info(
		"Initializing HTTP sync listener",
		"service", "vibexp-api",
		"component", "http-sync-listener",
		"event_types", eventTypes,
	)
}

// createHTTPSyncListener creates and returns an HTTP sync listener
func createHTTPSyncListener(
	cfg *config.Config, eventTypes []string, embeddingHandlers events.EmbeddingHandlers, logger *slog.Logger,
) *events.HTTPSyncListener {
	listener, err := events.NewHTTPSyncListener(events.HTTPSyncListenerConfig{
		AIServiceURL:      cfg.AIServiceURL,
		EventTypes:        eventTypes,
		Logger:            logger,
		EmbeddingHandlers: embeddingHandlers,
	})
	if err != nil {
		logger.Error(
			"Failed to create HTTP sync listener",
			"service", "vibexp-api",
			"component", "http-sync-listener",
			"error", fmt.Sprintf("%+v", err),
		)
		return nil
	}
	return listener
}

// subscribeHTTPSyncListener subscribes the HTTP sync listener to the event manager
func subscribeHTTPSyncListener(
	eventManager *events.EventManager, listener *events.HTTPSyncListener,
	eventTypes []string, logger *slog.Logger,
) {
	if err := eventManager.Subscribe(listener); err != nil {
		logger.Error(
			"Failed to subscribe HTTP sync listener",
			"service", "vibexp-api",
			"component", "http-sync-listener",
			"error", fmt.Sprintf("%+v", err),
		)
		return
	}

	logger.Info(
		"HTTP sync listener initialized successfully",
		"service", "vibexp-api",
		"component", "http-sync-listener",
		"event_types", eventTypes,
	)
}

// ProvideEmbeddingHandlerAdapter creates the events.EmbeddingHandlers implementation used by
// the HTTPSyncListener (local-dev / non-Pub/Sub path). The adapter is defined in the services
// package so it can be shared across the call graph without introducing import cycles.
func ProvideEmbeddingHandlerAdapter(
	embeddingService services.EmbeddingServiceInterface,
	logger *slog.Logger,
) events.EmbeddingHandlers {
	return services.NewEmbeddingHandlerAdapter(embeddingService, logger)
}
