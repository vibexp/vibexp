package server

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/auth/authkit"
	"github.com/vibexp/vibexp/internal/auth/oauthserver"
	"github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/container"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/observability/metrics"
	"github.com/vibexp/vibexp/internal/observability/tracing"
	"github.com/vibexp/vibexp/internal/repositories/postgres"
	"github.com/vibexp/vibexp/internal/server/gen"
	typesgen "github.com/vibexp/vibexp/internal/server/gen/types"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
)

// HTTP server timeouts harden the service against slow-client resource exhaustion
// (Slowloris and slow-body attacks). They are fixed constants rather than config
// because the values are operational invariants tuned to the Cloud Run request
// budget, not per-environment settings.
const (
	serverReadHeaderTimeout = 10 * time.Second
	serverReadTimeout       = 30 * time.Second
	serverWriteTimeout      = 60 * time.Second
	serverIdleTimeout       = 120 * time.Second
)

// rateLimitByIP applies a per-IP httprate limiter to r only when limit is positive.
// Production config (config.Load) validates every limit to be >= 1, so the limiter is
// always active in production. A non-positive limit (an uninitialized zero-value Config,
// as used in most handler unit tests) disables the limiter so shared-IP test traffic is
// not throttled. middleware.RealIP runs earlier in the chain, so the limiter keys on the
// true client IP. Limits are applied per route group, never globally, so internal Pub/Sub
// job routes, Stripe/GitHub webhooks, and the /ping & /health probes are never throttled.
func rateLimitByIP(r chi.Router, limit int) {
	if limit < 1 {
		return
	}
	r.Use(httprate.LimitByIP(limit, time.Minute))
}

// defaultMaxBodyBytes is the fallback request-body cap (1MiB) used when the configured
// limit is non-positive (e.g. a zero-value Config in tests). Production config is
// validated to be >= 1 at startup, so this only guards against an uninitialized value.
const defaultMaxBodyBytes int64 = 1 << 20

// maxBodySize returns a middleware that caps every request body at limit bytes by
// wrapping r.Body with http.MaxBytesReader. Reads beyond the limit fail with
// "http: request body too large", which handlers surface as 413. This is a
// defense-in-depth backstop; webhook handlers apply their own tighter
// per-route caps.
func maxBodySize(limit int64) func(http.Handler) http.Handler {
	if limit < 1 {
		limit = defaultMaxBodyBytes
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

type Server struct {
	router                *chi.Mux
	port                  string
	apiKey                string
	config                *config.Config
	container             container.Container
	activityService       activities.ActivityService
	resourceAccessService resourceaccess.ResourceAccessService
	metrics               *metrics.Metrics
	tracer                *tracing.Tracer
	logger                *slog.Logger
	sessionManager        *session.Manager
	// apiTokenVerifier validates AuthKit bearer JWTs on /api/v1/*. Nil when
	// API_OAUTH_ISSUER is unset, which disables the JWT branch in the auth
	// middleware (non-API-key bearer tokens then 401 as before).
	apiTokenVerifier *authkit.Verifier
	// attachmentAuthorizers is the allowlist of attachable owner types for the
	// universal /attachments endpoint. Each registered authorizer enforces the
	// owning resource's existing access boundary; adding a new attachable
	// resource type is exactly one Register call (see setupAttachmentAuthorizers).
	attachmentAuthorizers *services.AttachmentAuthorizerRegistry
	// oauthAS is the embedded OAuth 2.1 Authorization Server (issue #31). Nil when
	// OAUTH_AS_ISSUER_URL is unset, which leaves its routes unmounted.
	oauthAS *oauthserver.Service
	// refreshLocks serializes concurrent refresh-token rotations per user.
	// Many identity providers invalidate a refresh token on use; without this,
	// parallel requests from the same user with an expired access token would all
	// race on RefreshTokens and most would 401. See middleware.go:authenticateWithSession.
	refreshLocks sync.Map // map[string]*sync.Mutex keyed by user_id
	// reembedInFlight guards background team re-embeds so a rapid provider
	// change or a repeated reprocess click never stacks duplicate fan-outs for
	// the same team. Keyed by team_id; the value is present while a run is live
	// (see enqueueTeamReembed). Its zero value is ready to use.
	reembedInFlight sync.Map // map[string]struct{} keyed by team_id
	// spaFS is the embedded frontend build (contents of frontend/dist), served by
	// the SPA catch-all (handleSPA). It is nil in the default dev/CI build (the
	// frontend is NOT embedded, so the backend compiles and runs without a built
	// frontend/dist); release builds set it via the `embedfrontend` build tag.
	// See spa.go / spa_embed.go / spa_noembed.go.
	spaFS fs.FS
}

// initializeMetrics sets up OpenTelemetry metrics with the provided configuration
func initializeMetrics(cfg *config.Config, logger *slog.Logger) *metrics.Metrics {
	// Service version should be set via build-time variable: -ldflags "-X main.version=1.0.0"
	serviceVersion := "dev"
	if v := cfg.Server.ServiceVersion; v != "" {
		serviceVersion = v
	}

	appMetrics, err := metrics.New(
		serviceVersion,
		metrics.WithConfig(cfg),
		metrics.WithOTelEndpoint(cfg.OTel.Endpoint),
		metrics.WithExportInterval(cfg.OTel.ExportInterval),
		metrics.WithLogger(logger),
	)
	if err != nil {
		logger.With(
			"service", "vibexp-api",
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to initialize metrics, continuing without metrics")
		return nil
	}

	logger.With(
		"service", "vibexp-api",
		"version", serviceVersion,
		"otel_endpoint", cfg.OTel.Endpoint,
		"export_interval", cfg.OTel.ExportInterval.String(),
	).Info("OpenTelemetry metrics initialized successfully")

	return appMetrics
}

// initializeTracing sets up OpenTelemetry tracing with the provided configuration
func initializeTracing(cfg *config.Config, logger *slog.Logger) *tracing.Tracer {
	// Check if tracing is enabled
	if !cfg.OTel.TracingEnabled {
		logger.Info("OpenTelemetry tracing is disabled")
		return nil
	}

	// Service version should be set via build-time variable: -ldflags "-X main.version=1.0.0"
	serviceVersion := "dev"
	if v := cfg.Server.ServiceVersion; v != "" {
		serviceVersion = v
	}

	appTracer, err := tracing.New(
		serviceVersion,
		tracing.WithConfig(cfg),
		tracing.WithOTelEndpoint(cfg.OTel.Endpoint),
		tracing.WithSampleRatio(cfg.OTel.TraceSampleRatio),
	)
	if err != nil {
		logger.With(
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to initialize tracing, continuing without tracing")
		return nil
	}

	logger.With(
		"version", serviceVersion,
		"otel_endpoint", cfg.OTel.Endpoint,
		"sample_ratio", cfg.OTel.TraceSampleRatio,
	).Info("OpenTelemetry tracing initialized successfully")

	return appTracer
}

//nolint:funlen // Server constructor initialises many subsystems; factoring them out reduces readability
func New(port string, db *database.DB, apiKey string, cfg *config.Config, logger *slog.Logger) *Server {
	r := chi.NewRouter()

	// Initialize OpenTelemetry metrics
	appMetrics := initializeMetrics(cfg, logger)

	// Initialize OpenTelemetry tracing
	appTracer := initializeTracing(cfg, logger)

	// Outermost panic recovery: must wrap every inner middleware so panics in OTel, RequestID,
	// and other outer-ring middleware also produce a single structured ERROR entry.
	// When panicLoggerMiddleware fires before RequestIDMiddleware, contextkeys.GetLoggerFromContext
	// falls through to the package fallback — still structured JSON, just without request_id/trace.
	r.Use(panicLoggerMiddleware(logger))

	// CORS middleware
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.Server.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	}))

	// Security headers on every response (after CORS so preflight is unaffected).
	r.Use(securityHeadersMiddleware)

	// Fix url.scheme for Cloud Run: TLS is terminated at the Cloud Run proxy and
	// forwarded to the container over plain HTTP. Correct the scheme before the
	// OTel middleware reads it so that url.scheme spans show "https" instead of "http".
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" || proto == "http" {
				r.URL.Scheme = proto
			}
			next.ServeHTTP(w, r)
		})
	})

	// OpenTelemetry tracing middleware (must be before other middleware for accurate instrumentation)
	r.Use(tracing.HTTPMiddleware(appTracer))

	// OpenTelemetry metrics middleware (must be before other middleware to capture all requests)
	if appMetrics != nil {
		r.Use(metrics.MetricsMiddleware(appMetrics))
	}

	// Other middleware
	r.Use(RequestIDMiddleware(logger))     // Request ID (honors inbound X-Request-ID) + request-scoped logger
	r.Use(structuredRequestLogger(logger)) // Structured request-completion log (replaces middleware.Logger)
	// middleware.RealIP trusts X-Forwarded-For/X-Real-IP; only safe behind a
	// trusted reverse proxy (chi deprecation GHSA-3fxj-6jh8-hvhx).
	r.Use(middleware.RealIP) //nolint:staticcheck // safe only behind a trusted proxy
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(maxBodySize(cfg.Server.MaxBodySizeBytes)) // Global request-body cap (configurable, 1MiB default)

	c, err := container.InitializeContainer(db, cfg, logger)
	if err != nil {
		logger.With(
			"service", "vibexp-api",
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to initialize container")
		os.Exit(1)
	}

	// Build the session manager. IsDevelopment uses FrontendBaseURL heuristic;
	// when SESSION_ENCRYPTION_KEY is empty (test/stub environments) the manager
	// is left nil and the handlers degrade gracefully.
	var sessMgr *session.Manager
	if cfg.Auth.SessionEncryptionKey != "" {
		envSvc := c.EnvironmentService()
		isLocal := envSvc != nil && envSvc.IsDevelopment()
		sessMgr, err = session.NewManager(cfg.Auth.SessionEncryptionKey, isLocal)
		if err != nil {
			logger.With(
				"service", "vibexp-api",
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to initialize session manager")
			os.Exit(1)
		}
		logger.Info("Session manager initialized")
	} else {
		logger.Warn("SESSION_ENCRYPTION_KEY not set; session cookie auth disabled (stub/test mode)")
	}

	s := &Server{
		router:                r,
		port:                  port,
		apiKey:                apiKey,
		config:                cfg,
		container:             c,
		activityService:       c.ActivityService(),
		resourceAccessService: c.ResourceAccessService(),
		metrics:               appMetrics,
		tracer:                appTracer,
		logger:                logger,
		sessionManager:        sessMgr,
		apiTokenVerifier:      newAPITokenVerifier(cfg, c, logger),
		attachmentAuthorizers: setupAttachmentAuthorizers(c),
		oauthAS:               newOAuthAuthorizationServer(cfg, db, logger),
		spaFS:                 embeddedSPAFS(),
	}

	s.setupRoutes()
	return s
}

// newOAuthAuthorizationServer builds the embedded OAuth 2.1 Authorization Server
// (issue #31). It returns nil when OAUTH_AS_ISSUER_URL is unset, leaving the AS
// disabled and its routes unmounted. The AS never authenticates anyone itself: it
// stashes the authorize request as a user-less login session and the SPA binds the
// logged-in app user via the authenticated /api/v1/oauth/consent/attach endpoint
// (issue #54).
func newOAuthAuthorizationServer(
	cfg *config.Config, db *database.DB, logger *slog.Logger,
) *oauthserver.Service {
	if cfg.Auth.OAuthAS.IssuerURL == "" {
		return nil
	}
	svc := oauthserver.NewService(
		oauthserver.Config{
			Issuer:              cfg.Auth.OAuthAS.IssuerURL,
			ResourceURI:         cfg.MCP.ResourceURI,
			FrontendBaseURL:     cfg.Frontend.BaseURL,
			AccessTokenTTL:      cfg.Auth.OAuthAS.AccessTokenTTL,
			RefreshTokenTTL:     cfg.Auth.OAuthAS.RefreshTokenTTL,
			AuthCodeTTL:         cfg.Auth.OAuthAS.AuthCodeTTL,
			KeyRotationInterval: cfg.Auth.OAuthAS.KeyRotationInterval,
			CleanupInterval:     cfg.Auth.OAuthAS.CleanupInterval,
		},
		[]byte(cfg.Security.EncryptionKey),
		postgres.NewOAuthClientRepository(db),
		postgres.NewOAuthCodeRepository(db),
		postgres.NewOAuthAccessTokenRepository(db),
		postgres.NewOAuthRefreshTokenRepository(db),
		postgres.NewOAuthPKCERepository(db),
		postgres.NewOAuthSigningKeyRepository(db),
		postgres.NewOAuthLoginSessionRepository(db),
		logger,
	)
	logger.Info("Embedded OAuth 2.1 Authorization Server enabled", "issuer", cfg.Auth.OAuthAS.IssuerURL)
	return svc
}

// setupAttachmentAuthorizers builds the owner-authorizer registry for the
// universal attachments endpoint. Registering an owner_type here is the only
// wiring a new attachable resource type needs — the universal routes, handlers,
// and AttachmentService stay untouched.
func setupAttachmentAuthorizers(c container.Container) *services.AttachmentAuthorizerRegistry {
	reg := services.NewAttachmentAuthorizerRegistry()
	reg.Register(ownerTypeArtifact, services.NewArtifactAttachmentAuthorizer(c.ArtifactService()))
	reg.Register(ownerTypePrompt, services.NewPromptAttachmentAuthorizer(c.PromptService()))
	reg.Register(ownerTypeBlueprint, services.NewBlueprintAttachmentAuthorizer(c.BlueprintService()))
	return reg
}

// newAPITokenVerifier builds the API-surface AuthKit JWT verifier. When
// API_OAUTH_ISSUER is empty it returns nil and the auth middleware keeps
// rejecting non-API-key bearer tokens, preserving pre-mobile behavior.
func newAPITokenVerifier(cfg *config.Config, c container.Container, logger *slog.Logger) *authkit.Verifier {
	if cfg.Auth.APIAuth.Issuer == "" {
		return nil
	}

	verifier, err := authkit.New(
		context.Background(),
		cfg.Auth.APIAuth.Issuer,
		apiAudiencePolicy(cfg.Auth.APIAuth.Audiences, cfg.MCP.ResourceURI),
		userResolverAdapter{users: c.UserRepository()},
	)
	if err != nil {
		logger.With(
			"service", "vibexp-api",
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to initialize API OAuth token verifier")
		os.Exit(1)
	}
	logger.Info("API OAuth bearer JWT authentication enabled")
	return verifier
}

// apiAudiencePolicy selects the audience policy for the API surface. A
// configured allow-list (at least one non-empty, trimmed entry — envconfig
// splits on commas without trimming, and stray commas yield empty entries)
// requires membership. Otherwise plain AuthKit PKCE tokens carry no aud claim,
// so the default accepts any audience EXCEPT the MCP resource URI — an MCP
// client's audience-bound token must not double as a full API credential.
func apiAudiencePolicy(raw []string, mcpResourceURI string) authkit.AudiencePolicy {
	audiences := make([]string, 0, len(raw))
	for _, a := range raw {
		if t := strings.TrimSpace(a); t != "" {
			audiences = append(audiences, t)
		}
	}
	if len(audiences) > 0 {
		return authkit.RequireAnyAudience(audiences)
	}
	return authkit.AllowAnyAudienceExcept(mcpResourceURI)
}

// Container returns the dependency injection container
// This is needed for graceful shutdown to close event manager and other resources
func (s *Server) Container() container.Container {
	return s.container
}

// ServeHTTP implements http.Handler interface, allowing the server to be used in tests
// This avoids exposing the internal router while still allowing testing
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) setupResourceUsageRoutes(r chi.Router) {
	resourceUsageService := s.container.ResourceUsageService()
	// Skip if resource usage service is not available
	if resourceUsageService == nil {
		s.logger.Warn("Resource usage service not available, skipping route setup")
		return
	}

	handler := NewResourceUsageHandler(resourceUsageService, s.logger)
	r.Get("/resource-usage", handler.GetResourceUsage)
}

func (s *Server) setupRoutes() {
	s.setupPublicRoutes()
	s.setupBackofficeRoutes()
	s.setupAuthRoutes()
	s.setupProtectedRoutes()
	s.setupFlexibleAuthRoutes()
	// The SPA catch-all is registered LAST as the router's NotFound handler so it
	// can never shadow an API/MCP/OAuth route: chi only falls through to NotFound
	// when no route matched. Registering it via NotFound (rather than a GET "/*"
	// route) also keeps it out of the OpenAPI drift/payload-coverage gates, which
	// walk only the route tree. See spa.go.
	s.router.NotFound(s.handleSPA)
}

func (s *Server) setupBackofficeRoutes() {
	s.router.Route("/bo/v1", func(r chi.Router) {
		r.Use(s.backofficeAuthMiddleware)
		r.Route("/reports", func(r chi.Router) {
			r.Get("/usage-and-growth", s.handleBackofficeUsageAndGrowth)
		})
	})
}

func (s *Server) setupPublicRoutes() {
	s.router.Get("/ping", s.handlePing)
	s.router.Get("/health", s.handleHealth)
	s.router.Get("/favicon.ico", s.handleFavicon)
	// Public, unauthenticated OpenAPI spec (#139): the fully-bundled,
	// self-contained schema served from the embedded artifact (openapispec) so
	// external tooling can fetch the API contract from any running instance.
	// Intentionally undocumented-by-convention (allowlisted in the drift gate),
	// like /favicon.ico.
	s.router.Get("/openapi.yaml", s.handleOpenAPISpecYAML)
	s.router.Get("/openapi.json", s.handleOpenAPISpecJSON)
	// OAuth 2.1 resource-server discovery for the MCP endpoint. Both are public
	// (no auth): clients fetch them before they hold a token. RFC 9728 (PRM) and
	// the legacy AS-metadata probe path that older MCP clients hit.
	//
	// The PRM document is advertised ONLY when MCP auth is actually configured.
	// With no resource URI there is nothing valid to publish: serving a document
	// with empty `resource`/`authorization_servers` (as it once did) makes a client
	// fail with an opaque "Invalid OAuth error response" instead of a clear signal,
	// so the route is left unregistered and discovery 404s — the honest answer when
	// MCP auth is off. The AS-metadata probe path already 404s when unconfigured.
	if s.config.MCP.ResourceURI != "" {
		mcpMetadataPath, _ := deriveMCPMetadata(s.config.MCP.ResourceURI)
		s.router.Handle(mcpMetadataPath, s.mcpProtectedResourceMetadataHandler())
	} else {
		s.logger.Warn("MCP auth not configured (MCP_RESOURCE_URI empty); " +
			"protected-resource metadata not advertised and the MCP endpoint rejects all tokens")
	}
	s.router.Get(mcpAuthorizationServerMetadataPath, s.handleMCPAuthorizationServerMetadata)
	s.setupTestRoutes()
	s.router.Post("/api/v1/webhooks/github", s.handleGitHubWebhook)
	// Redirect legacy/misconfigured webhook path to the correct public endpoint.
	// GitHub App may be configured with the wrong URL; this 308 redirect ensures
	// webhook events are not silently dropped while the configuration is corrected.
	s.router.Post("/api/v1/integrations/github/webhook", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/v1/webhooks/github", http.StatusPermanentRedirect)
	})
	s.router.With(s.pubSubOIDCMiddleware).Post("/internal/jobs/notifications/retention", s.handleNotificationRetentionJob)
	s.router.With(s.pubSubOIDCMiddleware).Post("/internal/jobs/notifications/digest", s.handleNotificationDigestJob)
	s.router.With(s.pubSubOIDCMiddleware).Post("/internal/jobs/activities/retention", s.handleActivityRetentionJob)
	s.router.With(s.pubSubOIDCMiddleware).Post("/internal/jobs/access-events/retention", s.handleAccessEventsRetentionJob)
	s.setupOAuthASRoutes()
}

// setupOAuthASRoutes mounts the embedded OAuth 2.1 Authorization Server endpoints
// (issue #31) when it is enabled. Most are public: OAuth clients reach them before
// they hold a VibeXP token, and the protocol enforces its own authentication
// (PKCE, client/redirect validation, the consent CSRF token). Rate-limited per IP
// like the auth routes.
//
// The consent step is served as JSON under /api/v1/oauth/consent (issue #52): the
// browser is redirected to the SPA consent page, which renders it with the design
// system and calls these endpoints (same-origin in prod via the frontend /api
// proxy). All issuance/CSRF/redirect-validation stays server-side.
//
// The AS never authenticates anyone itself (issue #54): /authorize creates a
// user-less login session, and the SPA binds the logged-in app user via
// /api/v1/oauth/consent/attach — the ONE AS route behind the standard /api auth
// middleware (vx_session), so signing out gates MCP auth.
//
// Like the rest of the AS, these routes are mounted only when the AS is enabled,
// so they are absent (and thus undocumented-by-design, invisible to the OpenAPI
// drift/payload-coverage gates) when it is off.
func (s *Server) setupOAuthASRoutes() {
	if s.oauthAS == nil {
		return
	}
	// HTTPS is a MUST for the Authorization Server (issue #34). Enforce it on
	// every AS endpoint, exempting only local development (where the dev loop
	// and e2e stack serve plain HTTP on localhost).
	httpsOnly := requireHTTPSMiddleware(s.config.IsLocalDevelopment())
	s.router.With(httpsOnly).Get(oauthserver.MetadataPath, s.oauthAS.Metadata)
	s.router.Group(func(r chi.Router) {
		r.Use(httpsOnly)
		rateLimitByIP(r, s.config.RateLimit.AuthPerMinute)
		r.Get(oauthserver.AuthorizePath, s.oauthAS.Authorize)
		r.Get(oauthserver.ConsentAPIPath, s.oauthAS.ConsentDetails)
		r.Post(oauthserver.ConsentAPIPath, s.oauthAS.ConsentDecision)
		// The attach endpoint binds the authenticated app user to the login session;
		// it requires a vx_session (standard /api auth middleware), unlike the rest
		// of the protocol-authenticated AS routes.
		r.With(s.flexibleAuthMiddleware).Post(oauthserver.ConsentAttachPath, s.oauthAS.ConsentAttach)
		r.Post(oauthserver.TokenPath, s.oauthAS.Token)
		r.Post(oauthserver.RegisterPath, s.oauthAS.Register)
		r.Get(oauthserver.JWKSPath, s.oauthAS.JWKS)
	})
}

func (s *Server) setupSupportRoutes(r chi.Router) {
	r.Route("/api/v1/support", func(r chi.Router) {
		r.Post("/message", s.handleSupportMessage)
	})
}

func (s *Server) setupTestRoutes() {
	s.router.HandleFunc("/api/v1/prompts-invalid", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
}

func (s *Server) setupAuthRoutes() {
	s.router.Route("/api/v1/auth", func(r chi.Router) {
		// Strict per-IP rate limit: these are unauthenticated, abuse-prone endpoints.
		rateLimitByIP(r, s.config.RateLimit.AuthPerMinute)

		// Identity-provider OAuth login routes
		r.Get("/providers", s.handleListProviders)
		r.Get("/login", s.handleLogin)
		r.Get("/callback", s.handleCallback)
		r.Post("/logout", s.handleLogout)

		// Dev login (development environment only)
		r.Post("/dev/login", s.handleDevLogin)
	})

	// /auth/me endpoint with flexible auth (supports both cookie session and API keys)
	s.router.Group(func(r chi.Router) {
		r.Use(s.flexibleAuthMiddleware)
		r.Get("/api/v1/auth/me", s.handleGetMe)
	})
}

func (s *Server) setupProtectedRoutes() {
	// Cookie/API-key protected routes (web app: cookie session; CLI: API key)
	s.router.Group(func(r chi.Router) {
		// Moderate per-IP rate limit on the authenticated API surface. Applied here
		// (not globally) so webhooks, Pub/Sub job routes, and health/ping probes in
		// setupPublicRoutes are never throttled.
		rateLimitByIP(r, s.config.RateLimit.APIPerMinute)
		r.Use(s.flexibleAuthMiddleware)
		s.setupAPIKeysRoutes(r)
		s.setupUserRoutes(r)
		s.setupSettingsRoutes(r)
		s.setupAIToolsRoutes(r)
		s.setupSupportRoutes(r)
		// Non-CLI resource routes
		s.setupPromptGalleryRoutes(r)
		s.setupActivitiesRoutes(r)
		s.setupAgentsRoutes(r)
		s.setupGitHubIntegrationRoutes(r)
		// CLI-accessible resources (previously in flexible auth group)
		s.setupPromptsRoutes(r)
		s.setupArtifactsRoutes(r)
		s.setupAttachmentsRoutes(r)
		s.setupTypesRoutes(r)
		s.setupBlueprintRoutes(r)
		s.setupMemoriesRoutes(r)
		s.setupProjectsRoutes(r)
		s.setupSearchRoutes(r)
		// Resource access analytics (free-tier accessible)
		s.setupResourceAccessMetricsRoutes(r)
		// Teams routes (web app and CLI)
		s.setupTeamsRoutes(r)
		// AI Feed routes (free-tier accessible)
		s.setupFeedsRoutes(r)
		// Notification routes
		s.setupNotificationsRoutes(r)
		// Device token routes are intentionally free-tier — push notification
		// registration is a UX feature, not a gated resource.
		s.setupDeviceTokensRoutes(r)
	})
}

func (s *Server) setupAPIKeysRoutes(r chi.Router) {
	r.Route("/api/v1/api-keys", func(r chi.Router) {
		r.Post("/", s.handleCreateAPIKey)
		r.Get("/", s.handleListAPIKeys)
		r.Delete("/{id}", s.handleDeleteAPIKey)
	})
}

func (s *Server) setupUserRoutes(r chi.Router) {
	r.Route("/api/v1/user", func(r chi.Router) {
		r.Post("/onboarding/complete", s.handleMarkOnboardingCompleted)
	})
}

func (s *Server) setupSettingsRoutes(r chi.Router) {
	r.Route("/api/v1/settings", func(r chi.Router) {
		r.Route("/api-keys", func(r chi.Router) {
			r.Post("/", s.handleCreateAPIKey)
			r.Get("/", s.handleListAPIKeys)
			r.Delete("/{id}", s.handleDeleteAPIKey)
		})
	})
	// Embedding providers are scoped to a team (issue #79). Both the settings and
	// bare route groups validate team membership from the {team_id} path segment.
	r.Route("/api/v1/{team_id}/settings/embedding-providers", func(r chi.Router) {
		r.Use(s.teamValidationMiddleware())
		s.setupEmbeddingProvidersRoutes(r)
		// Clear-all-embeddings is a destructive maintenance action surfaced only
		// in the embedding settings UI, so it is registered on the settings mount
		// alone — not in the shared setupEmbeddingProvidersRoutes (which serves
		// both the settings and bare groups). The static "/embeddings" segment
		// sits beside the "/{id}" routes; chi matches the literal path first
		// (issue #182).
		r.Delete("/embeddings", s.handleClearEmbeddings)
	})
	r.Route("/api/v1/{team_id}/embedding-providers", func(r chi.Router) {
		r.Use(s.teamValidationMiddleware())
		s.setupEmbeddingProvidersRoutes(r)
	})
	// Model providers are scoped to a team (issue #110). Both the settings and
	// bare route groups validate team membership from the {team_id} path segment.
	r.Route("/api/v1/{team_id}/settings/model-providers", func(r chi.Router) {
		r.Use(s.teamValidationMiddleware())
		s.setupModelProvidersRoutes(r)
	})
	r.Route("/api/v1/{team_id}/model-providers", func(r chi.Router) {
		r.Use(s.teamValidationMiddleware())
		s.setupModelProvidersRoutes(r)
	})
	r.Route("/api/v1/preferences", s.setupPreferencesRoutes)
}

func (s *Server) setupPreferencesRoutes(r chi.Router) {
	r.Get("/", s.handleGetPreferences)
	r.Put("/", s.handleUpdatePreferences)
}

func (s *Server) setupEmbeddingProvidersRoutes(r chi.Router) {
	r.Post("/", s.handleCreateEmbeddingProvider)
	r.Get("/", s.handleListEmbeddingProviders)
	r.Get("/coverage", s.handleGetEmbeddingCoverage)
	r.Get("/{id}", s.handleGetEmbeddingProvider)
	r.Put("/{id}", s.handleUpdateEmbeddingProvider)
	r.Delete("/{id}", s.handleDeleteEmbeddingProvider)
	r.Post("/{id}/reprocess", s.handleReprocessEmbeddingProvider)
	r.Post("/validate", s.handleValidateEmbeddingProvider)
}

func (s *Server) setupModelProvidersRoutes(r chi.Router) {
	r.Post("/", s.handleCreateModelProvider)
	r.Get("/", s.handleListModelProviders)
	r.Get("/{id}", s.handleGetModelProvider)
	r.Put("/{id}", s.handleUpdateModelProvider)
	r.Delete("/{id}", s.handleDeleteModelProvider)
	r.Post("/validate", s.handleValidateModelProvider)
}

// setupResourceRoutes is now split between JWT-only and flexible auth groups
// This function is kept for reference but routes are set up directly in setupProtectedRoutes

func (s *Server) setupPromptsRoutes(r chi.Router) {
	r.Route("/api/v1/{team_id}/prompts", func(r chi.Router) {
		r.Use(s.teamValidationMiddleware()) // Validate team_id from URL and team access
		r.Post("/", s.handleCreatePrompt)
		r.Get("/", s.handleListPrompts)
		r.Get("/labels", s.handleGetPromptLabels)
		r.With(s.recordResourceAccess(resourceTypePrompt)).Get("/{slug}", s.handleGetPrompt)
		r.Put("/{slug}", s.handleUpdatePrompt)
		r.Delete("/{slug}", s.handleDeletePrompt)
		// Prompt content versions (generic content-versioning core, resource_type=prompt).
		r.Get("/{slug}/versions", s.handleListPromptVersions)
		r.Get("/{slug}/versions/{version_number}", s.handleGetPromptVersion)
		r.Post("/{slug}/versions/{version_number}/restore", s.handleRestorePromptVersion)
		r.Get("/{slug}/placeholders", s.handleGetPromptPlaceholders)
		r.Get("/{slug}/dependencies", s.handleGetPromptDependencies)
		r.Post("/{slug}/render", s.handleRenderPrompt)
		r.Post("/{slug}/share", s.handleCreatePromptShare)
		r.Get("/{slug}/share", s.handleGetPromptShare)
		r.Delete("/{slug}/share", s.handleDeletePromptShare)
	})

	// Public endpoint for shared prompts (optional auth)
	s.router.Route("/api/v1/shared/prompts", func(r chi.Router) {
		r.Use(s.optionalAuthMiddleware) // Allow both authenticated and unauthenticated access
		r.Get("/{token}", s.handleGetSharedPrompt)
	})
}

func (s *Server) setupPromptGalleryRoutes(r chi.Router) {
	r.Route("/api/v1/prompt-gallery", func(r chi.Router) {
		// Public GET endpoints - gallery browsing doesn't require authentication
		r.Group(func(r chi.Router) {
			r.Use(s.optionalAuthMiddleware)
			r.Get("/categories", s.handleGetPromptGalleryCategories)
			r.Get("/prompts", s.handleListPromptGalleryPrompts)
			r.Get("/prompts/{id}", s.handleGetPromptGalleryPrompt)
		})

		// Protected POST endpoints - usage tracking requires authentication
		r.Group(func(r chi.Router) {
			r.Post("/prompts/{id}/use", s.handleTrackPromptGalleryUsage)
		})
	})
}

func (s *Server) setupArtifactsRoutes(r chi.Router) {
	r.Route("/api/v1/{team_id}/artifacts", func(r chi.Router) {
		r.Use(s.artifactTeamValidationMiddleware()) // Validate team_id from URL and team access
		r.Post("/", s.handleCreateArtifact)
		r.Get("/", s.handleListArtifacts)
		r.Get("/stats", s.handleGetArtifactStats)
		r.Get("/{project_id}", s.handleListArtifactsByProject)
		r.With(s.recordResourceAccess(resourceTypeArtifact)).Get("/{project_id}/{slug}", s.handleGetArtifact)
		r.Put("/{project_id}/{slug}", s.handleUpdateArtifact)
		r.Delete("/{project_id}/{slug}", s.handleDeleteArtifact)
		// Artifact file attachments — DEPRECATED aliases of the universal
		// /api/v1/{team_id}/attachments endpoint (see setupAttachmentsRoutes).
		// Kept for one release so the frontend can cut over safely; remove once no
		// client uses the artifact-nested paths.
		r.Post("/{project_id}/{slug}/attachments", s.handleUploadArtifactAttachment)
		r.Get("/{project_id}/{slug}/attachments", s.handleListArtifactAttachments)
		r.Get("/{project_id}/{slug}/attachments/{id}", s.handleDownloadArtifactAttachment)
		r.Delete("/{project_id}/{slug}/attachments/{id}", s.handleDeleteArtifactAttachment)
		// Artifact content versions (generic content-versioning core, resource_type=artifact).
		r.Get("/{project_id}/{slug}/versions", s.handleListArtifactVersions)
		r.Get("/{project_id}/{slug}/versions/{version_number}", s.handleGetArtifactVersion)
		r.Post("/{project_id}/{slug}/versions/{version_number}/restore", s.handleRestoreArtifactVersion)
	})
}

// setupAttachmentsRoutes registers the universal attachments endpoint. owner_type
// and owner_id are a collection filter (list) / creation attribute (upload) — never
// part of an item's identity — so get/delete are keyed only by the attachment id and
// authorized against the owner stored on the row. Same team-validation middleware
// as the artifact routes.
func (s *Server) setupAttachmentsRoutes(r chi.Router) {
	r.Route("/api/v1/{team_id}/attachments", func(r chi.Router) {
		r.Use(s.teamValidationMiddleware()) // Validate team_id from URL and team access
		r.Post("/", s.handleUploadAttachment)
		r.Get("/", s.handleListAttachments)
		r.Get("/{id}", s.handleDownloadAttachment)
		r.Delete("/{id}", s.handleDeleteAttachment)
	})
}

// setupTypesRoutes registers the resource-type-agnostic, team-customizable type
// (category) CRUD endpoints (#1846) under /api/v1/{team_id}/types. Like
// Notifications (#1713) the operations are served through oapi-codegen
// strict-server bindings — here in their own package (internal/server/gen/types)
// so this domain mounts independently of Notifications — wrapped in a group that
// applies the same team-scoped middleware (team membership) the
// hand-written team-scoped resources use.
func (s *Server) setupTypesRoutes(r chi.Router) {
	strict := typesgen.NewStrictHandlerWithOptions(
		&typesStrictServer{s: s},
		nil,
		typesgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  s.typesBindErrorHandler,
			ResponseErrorHandlerFunc: s.typesResponseErrorHandler,
		},
	)
	r.Group(func(gr chi.Router) {
		gr.Use(s.teamValidationMiddleware()) // Validate team_id from URL and team access
		typesgen.HandlerWithOptions(strict, typesgen.ChiServerOptions{
			BaseRouter:       gr,
			ErrorHandlerFunc: s.typesBindErrorHandler,
		})
	})
}

// artifactTeamValidationMiddleware validates team_id from URL path and team access for artifact operations
// This middleware reuses the same validation logic as prompts
func (s *Server) artifactTeamValidationMiddleware() func(http.Handler) http.Handler {
	return s.teamValidationMiddleware()
}

func (s *Server) setupBlueprintRoutes(r chi.Router) {
	r.Route("/api/v1/{team_id}/blueprints", func(r chi.Router) {
		r.Use(s.teamValidationMiddleware()) // Validate team_id from URL and team access
		r.Post("/", s.handleCreateBlueprint)
		r.Get("/", s.handleListBlueprints)
		r.Get("/stats", s.handleGetBlueprintStats)
		r.Get("/{project_id}", s.handleListBlueprintsByProject)
		r.With(s.recordResourceAccess(resourceTypeBlueprint)).Get("/{project_id}/{slug}", s.handleGetBlueprint)
		r.Put("/{project_id}/{slug}", s.handleUpdateBlueprint)
		r.Delete("/{project_id}/{slug}", s.handleDeleteBlueprint)
		// Blueprint content versions
		r.Get("/{project_id}/{slug}/versions", s.handleListBlueprintVersions)
		r.Get("/{project_id}/{slug}/versions/{version_number}", s.handleGetBlueprintVersion)
		r.Post("/{project_id}/{slug}/versions/{version_number}/restore", s.handleRestoreBlueprintVersion)
	})
}

func (s *Server) setupFeedsRoutes(r chi.Router) {
	// Feeds are free-tier accessible — intentionally no subscriptionMiddleware (issue #1128).
	// Feed CRUD — team-scoped
	r.Route("/api/v1/{team_id}/feeds", func(r chi.Router) {
		r.Use(s.teamValidationMiddleware())
		r.Post("/", s.handleCreateFeed)
		r.Get("/", s.handleListFeeds)
		r.Get("/{feed_id}", s.handleGetFeed)
		r.Put("/{feed_id}", s.handleUpdateFeed)
		r.Delete("/{feed_id}", s.handleDeleteFeed)

		// Items nested under a specific feed
		r.Post("/{feed_id}/items", s.handleCreateFeedItem)
		r.Get("/{feed_id}/items", s.handleListFeedItemsByFeed)
	})

	// Cross-feed item list — team-scoped
	r.Route("/api/v1/{team_id}/feed-items", func(r chi.Router) {
		r.Use(s.teamValidationMiddleware())
		r.Get("/", s.handleListFeedItems)
		r.Get("/{item_id}", s.handleGetFeedItem)
		r.Post("/{item_id}/archive", s.handleArchiveFeedItem)
		r.Post("/{item_id}/unarchive", s.handleUnarchiveFeedItem)
		r.Delete("/{item_id}", s.handleDeleteFeedItem)
		// Threaded replies
		r.Post("/{item_id}/replies", s.handleCreateFeedItemReply)
		r.Get("/{item_id}/replies", s.handleListFeedItemReplies)
	})
}

func (s *Server) setupNotificationsRoutes(r chi.Router) {
	// #1713 PoC: the four REST notification operations are mounted through
	// the oapi-codegen strict-server bindings (internal/server/gen) instead
	// of direct registrations — handler signatures are generated from
	// openapi.yaml, so payload-level spec drift is a compile error for this
	// domain. Middleware (rate limit, flexible auth) is inherited from the
	// protected group exactly as before.
	strict := gen.NewStrictHandlerWithOptions(
		&notificationsStrictServer{s: s},
		nil,
		gen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  s.notificationsBindErrorHandler,
			ResponseErrorHandlerFunc: s.notificationsResponseErrorHandler,
		},
	)
	gen.HandlerWithOptions(strict, gen.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: s.notificationsBindErrorHandler,
	})
}

func (s *Server) setupDeviceTokensRoutes(r chi.Router) {
	r.Route("/api/v1/device-tokens", func(r chi.Router) {
		r.Post("/", s.handleRegisterDeviceToken)
		r.Delete("/", s.handleDeleteDeviceToken)
	})
}

func (s *Server) setupActivitiesRoutes(r chi.Router) {
	r.Route("/api/v1/activities", func(r chi.Router) {
		r.Get("/", s.handleActivitiesGet)
		r.Get("/stats", s.handleActivitiesStatsGet)
		r.Get("/types", s.handleActivitiesTypesGet)
		r.Get("/entity-types", s.handleActivitiesEntityTypesGet)
		r.Get("/{id}", s.handleActivityGet)
		r.Post("/", s.handleActivityPost)
	})
}

func (s *Server) setupAgentsRoutes(r chi.Router) {
	r.Route("/api/v1/{team_id}/agents", func(r chi.Router) {
		r.Use(s.teamValidationMiddleware()) // Validate team_id from URL and team access
		r.Post("/", s.handleCreateAgent)
		r.Post("/preview-card", s.handlePreviewAgentCard)
		r.Get("/", s.handleListAgents)
		r.Get("/stats", s.handleGetAgentStats)
		r.With(s.recordResourceAccess(resourceTypeAgent)).Get("/{id}", s.handleGetAgent)
		r.Put("/{id}", s.handleUpdateAgent)
		r.Put("/{id}/credentials", s.handleUpdateAgentCredentials)
		r.Delete("/{id}", s.handleDeleteAgent)
		r.Post("/{id}/execute", s.handleExecuteAgent)
		r.Post("/{id}/executions", s.handleStartAgentExecution)
		r.Get("/{id}/executions", s.handleListAgentExecutions)
		r.Get("/{id}/conversations", s.handleListAgentConversations)
		r.Put("/executions/{execution_id}", s.handleCompleteAgentExecution)
		r.Get("/executions/{execution_id}", s.handleGetAgentExecution)
		r.Post("/executions/{execution_id}/cancel", s.handleCancelExecution)
		r.Get("/executions/{id}/status", s.handleGetExecutionStatus)
		r.Get("/executions/{id}/events", s.handleGetExecutionEvents)
		r.Get("/conversations/{conversation_id}/executions", s.handleGetConversationExecutions)
	})
}

// agentTeamValidationMiddleware validates team_id from URL path and team access for agent operations
// This middleware reuses the same validation logic as prompts
func (s *Server) setupMemoriesRoutes(r chi.Router) {
	r.Route("/api/v1/{team_id}/memories", func(r chi.Router) {
		r.Use(s.memoryTeamValidationMiddleware()) // Validate team_id from URL and team access
		r.Post("/", s.handleCreateMemory)
		r.Get("/", s.handleListMemories)
		r.Get("/search", s.handleSearchMemoriesByMetadata)
		r.With(s.recordResourceAccess(resourceTypeMemory)).Get("/{id}", s.handleGetMemory)
		r.Put("/{id}", s.handleUpdateMemory)
		r.Delete("/{id}", s.handleDeleteMemory)
		// Memory content versions
		r.Get("/{id}/versions", s.handleListMemoryVersions)
		r.Get("/{id}/versions/{version_number}", s.handleGetMemoryVersion)
		r.Post("/{id}/versions/{version_number}/restore", s.handleRestoreMemoryVersion)
	})
}

// memoryTeamValidationMiddleware validates team_id from URL path and team access for memory operations
// This middleware reuses the same validation logic as prompts
func (s *Server) memoryTeamValidationMiddleware() func(http.Handler) http.Handler {
	return s.teamValidationMiddleware()
}

func (s *Server) setupSearchRoutes(r chi.Router) {
	r.Route("/api/v1/{team_id}/search", func(r chi.Router) {
		r.Use(s.teamValidationMiddleware()) // Validate team_id from URL and team access
		r.Post("/", s.handleSearch)
	})
}

// setupResourceAccessMetricsRoutes registers the read-only resource access
// analytics endpoint. It is intentionally free-tier (no subscriptionMiddleware),
// mirroring feeds — read analytics stays accessible to all team members.
func (s *Server) setupResourceAccessMetricsRoutes(r chi.Router) {
	r.Route("/api/v1/{team_id}/resource-access-metrics", func(r chi.Router) {
		r.Use(s.teamValidationMiddleware()) // Validate team_id from URL and team access
		r.Get("/", s.handleGetResourceAccessMetrics)
	})
}

func (s *Server) setupProjectsRoutes(r chi.Router) {
	r.Route("/api/v1/{team_id}/projects", func(r chi.Router) {
		r.Use(s.teamValidationMiddleware()) // Validate team_id from URL and team access
		r.Post("/", s.handleCreateProject)
		r.Get("/", s.handleListProjects)
		r.Get("/{slug}/stats", s.handleGetProjectStats)
		r.Get("/{slug}/resource-creation-metrics", s.handleGetProjectResourceCreationMetrics)
		r.With(s.recordResourceAccess(resourceTypeProject)).Get("/{slug}", s.handleGetProject)
		r.Put("/{slug}", s.handleUpdateProject)
		r.Delete("/{slug}", s.handleDeleteProject)

		// Project migration endpoints.
		r.Get("/{project_id}/migration/inventory", s.handleGetMigrationInventory)
		r.Post("/{project_id}/migration", s.handleMigrateProject)
	})
}

func (s *Server) setupTeamsRoutes(r chi.Router) {
	r.Route("/api/v1/teams", func(r chi.Router) {
		r.Post("/", s.handleCreateTeam)
		r.Get("/", s.handleListTeams)
		r.Get("/{id}", s.handleGetTeam)
		r.Put("/{id}", s.handleUpdateTeam)
		r.Delete("/{id}", s.handleDeleteTeam)

		// Team analytics endpoints (read-only). Authorization is performed in the
		// handlers via validateTeamAccess rather than teamValidationMiddleware,
		// because these routes share the {id} path param and the middleware reads
		// {team_id} (chi forbids two wildcard names at the same position).
		r.Get("/{id}/stats", s.handleGetTeamStats)
		r.Get("/{id}/resource-creation-metrics", s.handleGetTeamResourceCreationMetrics)
		r.Get("/{id}/resource-access-metrics", s.handleGetTeamResourceAccessMetrics)
		r.Get("/{id}/feed-creation-metrics", s.handleGetTeamFeedCreationMetrics)
		r.Get("/{id}/top-accessed-resources", s.handleGetTeamTopAccessedResources)

		// Team members endpoints
		r.Route("/{id}/members", func(r chi.Router) {
			r.Get("/", s.handleGetTeamMembers)
			r.Delete("/{userId}", s.handleRemoveTeamMember)
		})

		// Team invitation endpoints
		r.Route("/{id}/invitations", func(r chi.Router) {
			r.Post("/", s.handleSendTeamInvitations)
			r.Get("/", s.handleListTeamInvitations)
			r.Delete("/{invitationId}", s.handleRevokeInvitation)
		})

	})

	// Global invitation endpoints (no team ID required)
	r.Route("/api/v1/invitations", func(r chi.Router) {
		r.Get("/pending", s.handleGetPendingInvitations)
		// {token} routes registered AFTER fixed paths so chi matches "pending" first.
		r.Get("/{token}", s.handleGetInvitationByToken)
		r.Post("/{token}/accept", s.handleAcceptInvitation)
		r.Post("/{token}/reject", s.handleRejectInvitation)
	})
}

func (s *Server) setupGitHubIntegrationRoutes(r chi.Router) {
	r.Route("/api/v1/{team_id}/integrations/github", func(r chi.Router) {
		r.Use(s.teamValidationMiddleware())
		r.Get("/status", s.handleGitHubStatus)
		r.Get("/install-url", s.handleGitHubInstallURL)
		r.Post("/callback", s.handleGitHubCallback)
		r.Get("/repositories", s.handleGitHubRepositories)
		r.Post("/repositories/{repo_id}/import-project", s.handleGitHubImportProject)
		r.Post("/import-blueprints", s.handleGitHubImportBlueprints)
		r.Delete("/disconnect", s.handleGitHubDisconnect)
	})
}

func (s *Server) setupAIToolsRoutes(r chi.Router) {
	r.Route("/api/v1/ai-tools/claude-code", func(r chi.Router) {
		r.Get("/hooks", s.handleClaudeCodeHooksGet)
		r.Get("/sessions", s.handleClaudeCodeSessionsGet)
		r.Get("/session-counts", s.handleClaudeCodeSessionCountsGet)
		r.Get("/overview-stats", s.handleClaudeCodeOverviewStatsGet)
		r.Get("/recent-activities", s.handleClaudeCodeRecentActivitiesGet)
		r.Delete("/sessions/{session_id}", s.handleClaudeCodeSessionDelete)
	})
	r.Route("/api/v1/ai-tools/cursor-ide", func(r chi.Router) {
		r.Get("/hooks", s.handleCursorIDEHooksGet)
		r.Get("/sessions", s.handleCursorIDESessionsGet)
		r.Get("/session-counts", s.handleCursorIDESessionCountsGet)
		r.Get("/overview-stats", s.handleCursorIDEOverviewStatsGet)
		r.Get("/recent-activities", s.handleCursorIDERecentActivitiesGet)
		r.Delete("/sessions/{session_id}", s.handleCursorIDESessionDelete)
	})
}

// setupFlexibleAuthRoutes registers endpoints that accept either a cookie session
// or an API key.
//
// These routes are intentionally NOT IP-rate-limited. They serve high-frequency
// authenticated automation clients: IDE hook endpoints (claude-code / cursor-ide)
// fire once per tool invocation during an active coding session, and the MCP mount
// opens a long-lived SSE stream plus rapid tool calls. A per-IP request-count limit
// tuned for the human-facing API (1000/min) would throttle this legitimate traffic.
// Abuse here is bounded by authentication (a valid API key or session is required)
// rather than by IP rate limiting.
func (s *Server) setupFlexibleAuthRoutes() {
	s.router.Group(func(r chi.Router) {
		r.Use(s.flexibleAuthMiddleware)
		r.Post("/api/v1/claude-code/hooks", s.handleClaudeCodeHooksPost)
	})

	// Cursor IDE hooks endpoints with flexible auth (supports both JWT and API keys)
	s.router.Group(func(r chi.Router) {
		r.Use(s.flexibleAuthMiddleware)

		// POST endpoints that support both JWT and API key authentication
		r.Post("/api/v1/cursor-ide/hooks", s.handleCursorIDEHooksPost)
	})

	// MCP server endpoint - an OAuth 2.1 resource server (MCP authorization spec
	// 2025-06-18). It accepts only AuthKit-issued bearer JWTs minted for the MCP
	// resource audience; the legacy ?api_key query path and header API keys are not
	// honored here. This is a single user-scoped endpoint: there is no team in the
	// URL. Team-scoped tools take a required team_id parameter validated per call.
	// NOTE: the "/mcp/" route prefix is also referenced by the metrics middleware's
	// streaming-detection fallback (metrics.isStreamingResponse / mcpStreamRoutePrefix).
	// A GET here opens a long-lived SSE stream excluded from the latency histogram;
	// if this mount path changes, update mcpStreamRoutePrefix to match.
	s.setupMCPRoutes()

	// Resource usage routes - protected with flexible auth (cookie session or API key)
	s.router.Group(func(r chi.Router) {
		r.Use(s.flexibleAuthMiddleware)
		r.Route("/api/v1", func(r chi.Router) {
			s.setupResourceUsageRoutes(r)
		})
	})
}

// teamValidationMiddleware validates team_id from URL path and team access
// This middleware extracts team_id from chi URL parameters, validates it's a valid UUID,
// checks team access, and passes the validated team_id to downstream handlers
func (s *Server) teamValidationMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := r.Context().Value(contextKeyUserID).(string)
			teamID := chi.URLParam(r, "team_id")

			// Validate UUID format
			if _, err := uuid.Parse(teamID); err != nil {
				writeErrorResponse(w, r, "bad_request", "team_id must be a valid UUID", http.StatusBadRequest)
				return
			}

			// Validate team access
			if err := s.validateTeamAccess(r.Context(), userID, teamID); err != nil {
				writeErrorResponse(w, r, "access_denied", "Access denied", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	logger := contextkeys.GetLoggerFromContext(r.Context())
	logger.With(
		"handler", "handlePing",
		"user_agent", r.Header.Get("User-Agent"),
		"remote_ip", r.RemoteAddr,
	).Info("Ping endpoint accessed")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("pong")); err != nil {
		logger.With("error", err).Error("Failed to write response")
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	logger := contextkeys.GetLoggerFromContext(r.Context())
	logger.With(
		"handler", "handleHealth",
		"user_agent", r.Header.Get("User-Agent"),
		"remote_ip", r.RemoteAddr,
	).Info("Health endpoint accessed")

	sha := s.config.Server.ReleaseSHA
	if len(sha) > 8 {
		sha = sha[:8]
	}

	resp := struct {
		Status string `json:"status"`
		SHA    string `json:"sha"`
	}{Status: "healthy", SHA: sha}

	writeOK(w, resp, s.logger)
}

func (s *Server) Start(ctx context.Context) error {
	// Ensure an active OAuth AS signing key exists and start periodic rotation,
	// tied to the server's shutdown context.
	if s.oauthAS != nil {
		if err := s.oauthAS.Start(ctx); err != nil {
			return fmt.Errorf("failed to start OAuth authorization server: %w", err)
		}
	}

	srv := &http.Server{
		Addr:              ":" + s.port,
		Handler:           s.router,
		ReadHeaderTimeout: serverReadHeaderTimeout, // Prevent Slowloris attacks
		ReadTimeout:       serverReadTimeout,       // Bound total time to read the request
		WriteTimeout:      serverWriteTimeout,      // Bound total time to write the response
		IdleTimeout:       serverIdleTimeout,       // Bound keep-alive idle connections
	}

	go func() { // #nosec G118 -- ctx is already done when this runs; Background is correct for shutdown
		<-ctx.Done()
		s.logger.Info("Shutting down server...")

		// 8 seconds allows in-flight requests to complete and publish events before Container.Close() drains the event bus
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		// Shutdown tracer first to flush any pending traces
		if s.tracer != nil {
			s.logger.Info("Shutting down tracer provider...")
			if err := s.tracer.Shutdown(shutdownCtx); err != nil {
				s.logger.Error("Tracer shutdown error", "error", err)
			}
		}

		// Shutdown metrics to flush any pending metrics
		if s.metrics != nil {
			s.logger.Info("Shutting down metrics provider...")
			if err := s.metrics.Shutdown(shutdownCtx); err != nil {
				s.logger.Error("Metrics shutdown error", "error", err)
			}
		}

		if err := srv.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("Server shutdown error", "error", err)
		}
	}()

	s.logger.With("port", s.port).Info("Starting server")
	return srv.ListenAndServe()
}
