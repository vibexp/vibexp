package providers

import (
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/auth/password"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/observability/metrics"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/scheduler"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	"github.com/vibexp/vibexp/internal/services/audit"
	"github.com/vibexp/vibexp/internal/services/freshness"
	notificationsvc "github.com/vibexp/vibexp/internal/services/notifications"
	notifchannels "github.com/vibexp/vibexp/internal/services/notifications/channels"
	"github.com/vibexp/vibexp/internal/services/projectmigration"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
	"github.com/vibexp/vibexp/internal/storage"
	"github.com/vibexp/vibexp/pkg/events"
)

// AuthServiceDeps groups the dependencies of ProvideAuthService. Wire fills it
// via wire.Struct (see the container ProviderSet).
type AuthServiceDeps struct {
	UserRepo     repositories.UserRepository
	IdentityRepo repositories.UserIdentityRepository
	// AccountTokens and APIAccess are what the account-claim cleanup works
	// through: the first removes the account's one-time links and its password
	// in one transaction, the second its API keys and grants.
	AccountTokens repositories.AccountTokenRepository
	APIAccess     services.APIAccessServiceInterface
	// Notifier tells the owner of a claimed account that its password was
	// removed (#1493).
	Notifier     services.AccountNotifier
	Resolver     services.IdentityProviderResolver
	EventManager events.EventPublisher
	Logger       *slog.Logger
	Allowlist    services.AccessAllowlistResolver
	// Gate enforces the registration mode for a new account (#1434).
	Gate services.RegistrationGate
	// Audit records session revocations and identity links (#1514).
	Audit audit.Recorder
	// Methods records and reports disconnected identities (#1460).
	Methods repositories.SignInMethodRepository
}

// ProvideAuthService creates a new AuthService. An unverified account claimed
// by a verified provider sign-in loses its one-time links (#1432), its password
// (#1431) and its API access (#1449): whoever set them up need not be the
// mailbox's owner. A sign-in that would create an account goes through the
// registration gate (#1434).
func ProvideAuthService(deps AuthServiceDeps) services.AuthServiceInterface {
	return services.NewAuthService(
		deps.UserRepo, deps.IdentityRepo, deps.Resolver, deps.EventManager, deps.Logger, deps.Allowlist,
		services.WithRegistrationGate(deps.Gate),
		services.WithAuditRecorder(deps.Audit),
		services.WithDisconnectedIdentities(deps.Methods),
		services.WithUnverifiedAccountClaimHook(
			services.NewUnverifiedAccountCleanup(deps.AccountTokens, deps.APIAccess, deps.Notifier, deps.Logger)))
}

// ProvideRegistrationGate creates the gate that enforces the instance's
// registration mode when a sign-in would create a new account (#1434).
func ProvideRegistrationGate(
	policy services.AuthPolicyResolver,
	invitations repositories.TeamInvitationRepository,
	rootAdmins services.InstanceAdminResolver,
	logger *slog.Logger,
) services.RegistrationGate {
	return services.NewRegistrationGate(services.RegistrationGateDeps{
		Policy: policy, Invitations: invitations, RootAdmins: rootAdmins, Logger: logger,
	})
}

// ProvideAPIAccessService creates a new APIAccessService
func ProvideAPIAccessService(
	repo repositories.APIAccessRepository, logger *slog.Logger, recorder audit.Recorder,
) services.APIAccessServiceInterface {
	return services.NewAPIAccessService(repo, logger, services.WithAPIAccessAuditRecorder(recorder))
}

// ProvideAPIKeyService creates a new APIKeyService
func ProvideAPIKeyService(
	apiKeyRepo repositories.APIKeyRepository,
	logger *slog.Logger,
) services.APIKeyServiceInterface {
	return services.NewAPIKeyService(apiKeyRepo, logger)
}

// ProvidePromptService creates a new PromptService. Wire fills the deps struct
// via wire.Struct (see the container ProviderSet).
func ProvidePromptService(deps services.PromptServiceDeps) services.PromptServiceInterface {
	return services.NewPromptService(deps)
}

// ProvidePromptGalleryService creates a new PromptGalleryService
func ProvidePromptGalleryService(
	repo repositories.PromptGalleryRepository,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) services.PromptGalleryServiceInterface {
	return services.NewPromptGalleryService(repo, eventManager, logger)
}

// ProvidePromptShareService creates a new PromptShareService
func ProvidePromptShareService(
	shareRepo repositories.PromptShareRepository,
	promptRepo repositories.PromptRepository,
	promptService services.PromptServiceInterface,
	logger *slog.Logger,
) *services.PromptShareService {
	// Cast the interface to concrete type since we know the implementation
	concretePromptService, ok := promptService.(*services.PromptService)
	if !ok {
		// If casting fails, pass nil and the service will handle it gracefully
		concretePromptService = nil
	}
	return services.NewPromptShareService(shareRepo, promptRepo, concretePromptService, logger)
}

// ProvideContentVersionService creates a new ContentVersionService with the artifact adapter
// registered. New resource types are added by registering further adapters here. The user
// repository resolves version authors for read responses. The retention cap for every
// resource type comes from cfg.Retention.ContentVersionLimit (default 20, 0 = keep all).
func ProvideContentVersionService(
	repo repositories.ContentVersionRepository,
	users repositories.UserRepository,
	cfg *config.Config,
	logger *slog.Logger,
) services.ContentVersionServiceInterface {
	retentionCap := cfg.Retention.ContentVersionLimit
	return services.NewContentVersionService(
		repo,
		users,
		logger,
		services.ContentVersionAdapter{
			ResourceType:        "artifact",
			RetentionCap:        retentionCap,
			InitialVersionLabel: "Created the artifact",
		},
		services.ContentVersionAdapter{
			ResourceType:        "blueprint",
			RetentionCap:        retentionCap,
			InitialVersionLabel: "Created the blueprint",
		},
		services.ContentVersionAdapter{
			ResourceType:        "memory",
			RetentionCap:        retentionCap,
			InitialVersionLabel: "Created the memory",
		},
		services.ContentVersionAdapter{
			ResourceType:        "prompt",
			RetentionCap:        retentionCap,
			InitialVersionLabel: "Created the prompt",
		},
	)
}

// ProvideArtifactService creates a new ArtifactService. Wire fills the deps
// struct via wire.Struct (see the container ProviderSet).
func ProvideArtifactService(deps services.ArtifactServiceDeps) services.ArtifactServiceInterface {
	return services.NewArtifactService(deps)
}

// ProvideCommentService creates a new CommentService.
func ProvideCommentService(
	repo repositories.CommentRepository,
	teamService services.TeamServiceInterface,
	authzService services.AuthorizationServiceInterface,
	logger *slog.Logger,
) services.CommentServiceInterface {
	return services.NewCommentService(repo, teamService, authzService, logger)
}

// ProvideRelationService creates a new RelationService.
func ProvideRelationService(
	repo repositories.RelationRepository,
	teamService services.TeamServiceInterface,
	authzService services.AuthorizationServiceInterface,
	logger *slog.Logger,
) services.RelationServiceInterface {
	return services.NewRelationService(repo, teamService, authzService, logger)
}

// ProvideRelationSeedService creates a new RelationSeedService.
func ProvideRelationSeedService(
	repo repositories.RelationRepository,
	relationService services.RelationServiceInterface,
	logger *slog.Logger,
) services.RelationSeedServiceInterface {
	return services.NewRelationSeedService(repo, relationService, logger)
}

// ProvideAttachmentService creates a new AttachmentService. The object store
// may be nil (storage disabled); the service degrades to 503 in that case.
func ProvideAttachmentService(
	repo repositories.AttachmentRepository,
	store storage.ObjectStore,
	logger *slog.Logger,
) services.AttachmentServiceInterface {
	return services.NewAttachmentService(repo, store, logger)
}

// ProvideTypeService creates a new TypeService. The authorization and audit
// services are for the cross-team copy alone (#829): it is the one operation
// whose second team never passes through teamValidationMiddleware, and the one
// that must leave an audit trail.
func ProvideTypeService(
	repo repositories.TypeRepository,
	authorizer services.AuthorizationServiceInterface,
	audit services.TeamSettingsAuditServiceInterface,
	logger *slog.Logger,
) services.TypeServiceInterface {
	return services.NewTypeService(repo, authorizer, audit, logger)
}

// ProvideStatusCatalogService creates a new StatusCatalogService.
func ProvideStatusCatalogService(
	repo repositories.TeamStatusRepository,
) services.StatusCatalogServiceInterface {
	return services.NewStatusCatalogService(repo)
}

// ProvideStatusManagementService creates a new StatusManagementService.
func ProvideStatusManagementService(
	repo repositories.TeamStatusRepository,
	catalog services.StatusCatalogServiceInterface,
	authorizer services.AuthorizationServiceInterface,
	audit services.TeamSettingsAuditServiceInterface,
	logger *slog.Logger,
) services.StatusManagementServiceInterface {
	return services.NewStatusManagementService(repo, catalog, authorizer, audit, logger)
}

// ProvideResourceLockService creates a new ResourceLockService. Wire fills the
// deps struct via wire.Struct (see the container ProviderSet).
func ProvideResourceLockService(deps services.ResourceLockServiceDeps) services.ResourceLockServiceInterface {
	return services.NewResourceLockService(deps)
}

// ProvideBlueprintService creates a new BlueprintService. Wire fills the deps
// struct via wire.Struct (see the container ProviderSet).
func ProvideBlueprintService(deps services.BlueprintServiceDeps) services.BlueprintServiceInterface {
	return services.NewBlueprintService(deps)
}

// ProvideEmbeddingProviderService creates a new EmbeddingProviderService. Secret
// encryption is delegated to the shared, fail-closed EncryptionService rather than
// a private inline AES implementation keyed off the raw config string (#294).
func ProvideEmbeddingProviderService(
	repo repositories.EmbeddingProviderRepository,
	enc services.EncryptionServiceInterface,
	cfg *config.Config,
	authzSvc services.AuthorizationServiceInterface,
	audit services.TeamSettingsAuditServiceInterface,
	coverageRepo repositories.EmbeddingBackfillRepository,
) services.EmbeddingProviderServiceInterface {
	return services.NewEmbeddingProviderService(repo, enc, cfg, authzSvc, audit, coverageRepo)
}

// ProvideLLMService creates the central completion service (#1069). The search
// summary (#1073) is its first consumer, through the services.LLMCompleter
// binding in wire.go; it has no container accessor of its own.
func ProvideLLMService(
	repo repositories.ModelProviderRepository,
	enc services.EncryptionServiceInterface,
	cfg *config.Config,
	logger *slog.Logger,
) *services.LLMService {
	return services.NewLLMService(repo, enc, cfg, logger)
}

// ProvideModelProviderService creates a new ModelProviderService. Secret
// encryption is delegated to the shared, fail-closed EncryptionService rather than
// a private inline AES implementation keyed off the raw config string (#294).
func ProvideModelProviderService(
	repo repositories.ModelProviderRepository,
	enc services.EncryptionServiceInterface,
	cfg *config.Config,
	authzSvc services.AuthorizationServiceInterface,
	audit services.TeamSettingsAuditServiceInterface,
) services.ModelProviderServiceInterface {
	return services.NewModelProviderService(repo, enc, cfg, authzSvc, audit)
}

// ProvideGitHubAppConfigService creates a new GitHubAppConfigService. The
// frontend base URL is what the webhook URL shown to the admin is built from, so
// it must be the instance's PUBLIC origin — the same value the OAuth flows use.
func ProvideGitHubAppConfigService(
	repo repositories.GitHubAppConfigRepository,
	enc services.EncryptionServiceInterface,
	cfg *config.Config,
	authzSvc services.AuthorizationServiceInterface,
	clients services.GitHubAppClientResolver,
) services.GitHubAppConfigServiceInterface {
	return services.NewGitHubAppConfigService(repo, enc, authzSvc, clients, cfg.Frontend.BaseURL)
}

// ProvideEmailService creates a new EmailService.
//
// It takes the sender RESOLVER rather than an EmailProvider: every send now picks
// its provider per team, and the instance provider reaches the service as the
// resolver's fallback. Injecting a provider directly would let a send bypass a
// team's configuration.
//
// siteInfo supplies the company branding every email shows (#1362).
func ProvideEmailService(
	resolver services.EmailSenderResolver,
	siteInfo services.InstanceSiteInfoResolver,
	cfg *config.Config,
) services.EmailServiceInterface {
	return services.NewEmailService(resolver, siteInfo, cfg)
}

// ActivityServiceDeps groups the dependencies of ProvideActivityService. Wire
// fills it via wire.Struct (see the container ProviderSet).
type ActivityServiceDeps struct {
	Repo          repositories.ActivityRepository
	ProjectRepo   repositories.ProjectRepository
	PromptRepo    repositories.PromptRepository
	ArtifactRepo  repositories.ArtifactRepository
	UserRepo      repositories.UserRepository
	AgentRepo     repositories.AgentRepository
	BlueprintRepo repositories.BlueprintRepository
	APIKeyRepo    repositories.APIKeyRepository
	MemoryRepo    repositories.MemoryRepository
	Cfg           *config.Config
}

// ProvideActivityService creates a new ActivityService
func ProvideActivityService(deps ActivityServiceDeps) activities.ActivityService {
	return activities.NewService(activities.ServiceDeps{
		Repo:          deps.Repo,
		ProjectRepo:   deps.ProjectRepo,
		PromptRepo:    deps.PromptRepo,
		ArtifactRepo:  deps.ArtifactRepo,
		UserRepo:      deps.UserRepo,
		AgentRepo:     deps.AgentRepo,
		BlueprintRepo: deps.BlueprintRepo,
		APIKeyRepo:    deps.APIKeyRepo,
		MemoryRepo:    deps.MemoryRepo,
		RetentionDays: deps.Cfg.Retention.ActivityDays,
	})
}

// resourceAccessWorkerCount is the number of worker goroutines dedicated to
// persisting resource access events off the request path.
const resourceAccessWorkerCount = 5

// ProvideResourceAccessWorkerPool constructs and starts the worker pool used to
// persist resource access events asynchronously. Start() must be called here or
// no workers consume the queue.
//
// A dedicated pool is used (rather than the shared event bus) because recording
// access is a generic off-read-path write, not a domain pub/sub event; routing it
// through the event bus would add needless coupling. Note that WorkerPool.Submit's
// queue-full fallback spawns an unbounded `go task()`, so a sustained burst can
// briefly exceed the worker count.
func ProvideResourceAccessWorkerPool() *events.WorkerPool {
	pool := events.NewWorkerPool(resourceAccessWorkerCount)
	pool.Start()
	return pool
}

// ProvideResourceAccessService creates a new ResourceAccessService
func ProvideResourceAccessService(
	repo repositories.ResourceAccessRepository,
	lastAccessed repositories.ResourceLastAccessedRepository,
	clearer *freshness.Clearer,
	pool *events.WorkerPool,
	logger *slog.Logger,
	cfg *config.Config,
) resourceaccess.ResourceAccessService {
	return resourceaccess.NewService(repo, lastAccessed, clearer, pool, logger, cfg.Retention.AccessEventDays)
}

// ProvideResourceWriteService creates a new ResourceWriteService. It shares the
// resource-access worker pool: recording a write is the same kind of
// off-request-path event write as recording a read.
func ProvideResourceWriteService(
	repo repositories.ResourceWriteEventRepository,
	pool *events.WorkerPool,
	logger *slog.Logger,
) resourceaccess.ResourceWriteService {
	return resourceaccess.NewWriteService(repo, pool, logger)
}

// ProvideAuditClientInfoCapture creates the switch that decides whether audit
// events store the client's IP and user agent: the instance's capture_ip_ua
// setting (#1512), cached and invalidated by the settings service. wire.Bind in
// wire.go maps it to audit.ClientInfoCapturer for the recorder.
func ProvideAuditClientInfoCapture(
	repo repositories.InstanceAuditSettingsRepository,
	logger *slog.Logger,
) *services.AuditClientInfoCapture {
	return services.NewAuditClientInfoCapture(repo, logger)
}

// ProvideAuditPurgeService creates the audit retention purge (#1512): it
// deletes the audit events older than the configured retention.
func ProvideAuditPurgeService(
	settings repositories.InstanceAuditSettingsRepository,
	events repositories.AuditEventRepository,
	logger *slog.Logger,
) services.AuditPurger {
	return services.NewAuditPurgeService(settings, events, logger)
}

// ProvideAdminAuditService creates the instance admin's read side of the audit
// log (#1516): the paged listing and the CSV export.
func ProvideAdminAuditService(events repositories.AuditEventRepository) services.AdminAuditServiceInterface {
	return services.NewAdminAuditService(events)
}

// ProvideAuditRecorder creates the instance audit log recorder. It shares the
// resource-access worker pool: every audit write is off the request path.
func ProvideAuditRecorder(deps audit.RecorderDeps) audit.Recorder {
	return audit.NewRecorder(deps)
}

// ProvideEncryptionService creates a new EncryptionService
func ProvideEncryptionService(cfg *config.Config) (services.EncryptionServiceInterface, error) {
	if cfg.Security.EncryptionKey == "" {
		// Return nil for tests and environments without encryption key
		return nil, nil
	}
	return services.NewEncryptionService(cfg.Security.EncryptionKey)
}

// ProvideAgentService creates a new AgentService
func ProvideAgentService(
	agentRepo repositories.AgentRepository,
	executionRepo repositories.AgentExecutionRepository,
	encryptionService services.EncryptionServiceInterface,
	teamService services.TeamServiceInterface,
	authzService services.AuthorizationServiceInterface,
	cfg *config.Config,
	logger *slog.Logger,
) services.AgentServiceInterface {
	return services.NewAgentService(
		agentRepo, executionRepo, encryptionService, teamService, authzService, cfg, logger,
	)
}

// ProvideAgentCardFetcher creates a new AgentCardFetcher. cfg drives the SSRF
// policy: loopback/private agent card hosts are reachable only in local development.
func ProvideAgentCardFetcher(cfg *config.Config) services.CardFetcher {
	return services.NewAgentCardFetcher(cfg)
}

// ProvideAgentAuthenticator creates a new AgentAuthenticator
func ProvideAgentAuthenticator(
	encryptionService services.EncryptionServiceInterface,
) *services.AgentAuthenticator {
	return services.NewAgentAuthenticator(encryptionService)
}

// ProvideA2AHTTPClient creates a new A2AHTTPClient
func ProvideA2AHTTPClient(
	authenticator *services.AgentAuthenticator,
	cfg *config.Config,
) services.A2AHTTPClientInterface {
	return services.NewA2AHTTPClient(authenticator, cfg)
}

// ProvideA2AStreamProcessor creates a new A2AStreamProcessor
func ProvideA2AStreamProcessor(
	eventRepo repositories.AgentExecutionEventRepository,
	executionRepo repositories.AgentExecutionRepository,
	logger *slog.Logger,
) services.StreamProcessor {
	return services.NewA2AStreamProcessor(eventRepo, executionRepo, logger)
}

// ProvideAgentInvocationService creates a new AgentInvocationService
func ProvideAgentInvocationService(
	agentRepo repositories.AgentRepository,
	executionRepo repositories.AgentExecutionRepository,
	eventRepo repositories.AgentExecutionEventRepository,
	a2aClient services.A2AHTTPClientInterface,
	streamProcessor services.StreamProcessor,
	logger *slog.Logger,
) services.AgentInvocationServiceInterface {
	return services.NewAgentInvocationService(agentRepo, executionRepo, eventRepo, a2aClient, streamProcessor, logger)
}

// ProvideMemoryService creates a new MemoryService. Wire fills the deps struct
// via wire.Struct (see the container ProviderSet).
func ProvideMemoryService(deps services.MemoryServiceDeps) services.MemoryServiceInterface {
	return services.NewMemoryService(deps)
}

// EmbeddingServiceDeps groups the dependencies of ProvideEmbeddingService. Wire
// fills it via wire.Struct (see the container ProviderSet).
type EmbeddingServiceDeps struct {
	Repo              repositories.EmbeddingRepository
	PromptRepo        repositories.PromptRepository
	ArtifactRepo      repositories.ArtifactRepository
	MemoryRepo        repositories.MemoryRepository
	BlueprintRepo     repositories.BlueprintRepository
	FeedItemRepo      repositories.FeedItemRepository
	FeedItemReplyRepo repositories.FeedItemReplyRepository
	Logger            *slog.Logger
}

// ProvideEmbeddingService creates a new EmbeddingService
func ProvideEmbeddingService(deps EmbeddingServiceDeps) services.EmbeddingServiceInterface {
	return services.NewEmbeddingService(services.EmbeddingServiceDeps{
		Repo:              deps.Repo,
		PromptRepo:        deps.PromptRepo,
		ArtifactRepo:      deps.ArtifactRepo,
		MemoryRepo:        deps.MemoryRepo,
		BlueprintRepo:     deps.BlueprintRepo,
		FeedItemRepo:      deps.FeedItemRepo,
		FeedItemReplyRepo: deps.FeedItemReplyRepo,
		Dimensions:        services.EmbeddingVectorDimensions,
		Logger:            deps.Logger,
	})
}

// ProvideQueryEmbedder creates a QueryEmbedder backed by the active system-wide
// embedding provider, so search queries embed with the same provider, model, and
// dimension as documents.
func ProvideQueryEmbedder(
	providerSvc services.EmbeddingProviderServiceInterface,
	logger *slog.Logger,
) services.QueryEmbedder {
	return services.NewProviderQueryEmbedder(providerSvc, logger)
}

// ProvideEmbeddingProcessor creates the events.EmbeddingProcessor the embedding
// worker uses to generate and persist embeddings off the event bus. The
// generation engine resolves the active provider, chunks entity text in Go,
// embeds it, and saves the chunks; the EmbeddingDispatcher wraps it to bound
// fan-out per provider (sized by each provider's concurrency) and keep generation
// off the bus's shared, unbounded worker pool (#142). Retry and resolve-stage
// sizing reuse the event_bus.* knobs so operators tune one set of values.
//
// The durable queue (#820) is wired unconditionally: it is the system of record
// for outstanding work, so an accepted event is persisted before ProcessEvent
// returns and a restart resumes rather than discarding its backlog. Only the
// drain knobs are configurable (embedding.queue.*), never durability itself.
func ProvideEmbeddingProcessor(
	providerSvc services.EmbeddingProviderServiceInterface,
	embeddingService services.EmbeddingServiceInterface,
	jobRepo repositories.EmbeddingJobRepository,
	cfg *config.Config,
	logger *slog.Logger,
) events.EmbeddingProcessor {
	engine := services.NewEmbeddingGenerationProcessor(providerSvc, embeddingService, logger)
	return services.NewEmbeddingDispatcher(
		engine,
		cfg.EventBus.WorkerCount,
		services.EmbeddingRetryConfig{
			MaxRetries:  cfg.EventBus.MaxRetries,
			BaseBackoff: cfg.EventBus.RetryBackoff,
			Jitter:      cfg.EventBus.RetryJitter,
		},
		logger,
		services.WithDurableQueue(jobRepo, services.EmbeddingQueueConfig{
			LeaseDuration: cfg.Embedding.Queue.LeaseDuration,
			MaxAttempts:   int(cfg.Embedding.Queue.MaxAttempts),
			BatchSize:     int(cfg.Embedding.Queue.BatchSize),
			PollInterval:  cfg.Embedding.Queue.PollInterval,
			RetryBackoff:  cfg.Embedding.Queue.RetryBackoff,
		}),
	)
}

// ProvideTeamEmailProviderService creates the team email provider service.
// Secret encryption is delegated to the shared, fail-closed EncryptionService
// (#294); when Security.EncryptionKey is unset ProvideEncryptionService returns
// nil and the service refuses to store or read a credential rather than handling
// one in plaintext.
func ProvideTeamEmailProviderService(
	repo repositories.TeamEmailProviderRepository,
	instanceRepo repositories.InstanceEmailProviderRepository,
	userRepo repositories.UserRepository,
	enc services.EncryptionServiceInterface,
	cfg *config.Config,
	authzService services.AuthorizationServiceInterface,
	logger *slog.Logger,
) services.TeamEmailProviderServiceInterface {
	return services.NewTeamEmailProviderService(repo, instanceRepo, userRepo, enc, cfg, authzService, logger)
}

// ProvideInstanceEmailProviderService creates the instance email provider
// service (#1188). Like the team service, it refuses to store or read a
// credential when no encryption service is configured.
func ProvideInstanceEmailProviderService(
	repo repositories.InstanceEmailProviderRepository,
	audit repositories.InstanceSettingsAuditRepository,
	userRepo repositories.UserRepository,
	enc services.EncryptionServiceInterface,
	logger *slog.Logger,
) services.InstanceEmailProviderServiceInterface {
	return services.NewInstanceEmailProviderService(repo, audit, userRepo, enc, logger)
}

// ProvideInstanceAdminResolver creates the instance admin resolver (#1233):
// root admins come from auth.instance_admins, DB admins from instance_admins.
func ProvideInstanceAdminResolver(
	cfg *config.Config,
	grants repositories.InstanceAdminRepository,
	userRepo repositories.UserRepository,
	logger *slog.Logger,
) services.InstanceAdminResolver {
	return services.NewInstanceAdminService(cfg.Auth.InstanceAdmins, grants, userRepo, logger)
}

// ProvideSetupModeService creates the authentication setup-mode service
// (#1236). Setup mode is active while no sign-in method works (AuthWayIn,
// #1438) unless auth.recovery_mode forces it on (#1237).
func ProvideSetupModeService(
	setup repositories.InstanceAuthSetupRepository,
	wayIn services.AuthWayIn,
	admins services.InstanceAdminResolver,
	cfg *config.Config,
	logger *slog.Logger,
) services.SetupModeService {
	return services.NewSetupModeService(services.SetupModeDeps{
		Setup:        setup,
		WayIn:        wayIn,
		IsRootAdmin:  admins.IsRootAdmin,
		BaseURL:      cfg.Frontend.BaseURL,
		RecoveryMode: bool(cfg.Auth.RecoveryMode),
		Logger:       logger,
	})
}

// ProvideAuthWayIn creates the check setup mode and the lockout guard share
// (#1438): an identity provider is enabled, or password login is on and an
// instance admin has a usable password.
func ProvideAuthWayIn(
	providerRepo repositories.InstanceAuthProviderRepository,
	policies repositories.InstanceAuthPolicyRepository,
	credentials repositories.UserCredentialRepository,
	admins services.InstanceAdminResolver,
) services.AuthWayIn {
	return services.NewAuthWayIn(services.AuthWayInDeps{
		Providers:       providerRepo,
		Policies:        policies,
		Credentials:     credentials,
		RootAdminEmails: admins.RootAdminEmails,
	})
}

// ProvideRootAdminPasswordService creates the service behind the setup
// session's password bootstrap (#1438). It publishes user.created for an
// account it creates, so the account gets its workspace like any other.
func ProvideRootAdminPasswordService(
	users repositories.UserRepository,
	credentials repositories.UserCredentialRepository,
	policies repositories.InstanceAuthPolicyRepository,
	passwords services.PasswordAuthServiceInterface,
	admins services.InstanceAdminResolver,
	caches *services.AuthSettingsInvalidator,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) *services.RootAdminPasswordService {
	return services.NewRootAdminPasswordService(services.RootAdminPasswordDeps{
		Users:       users,
		Credentials: credentials,
		Policies:    policies,
		Passwords:   passwords,
		IsRootAdmin: admins.IsRootAdmin,
		Caches:      caches,
		Events:      eventManager,
		Logger:      logger,
	})
}

// ProvideAuthSettingsInvalidator creates what in-process auth settings writers
// call after a write, so the policy and allowlist caches serve it at once
// (#1485). The identity provider resolver reads the settings version on every
// call and needs no invalidation.
//
// Both resolvers must be invalidatable: a resolver wrapped in something that
// is not would make every write wait out the probe again with no test noticing,
// so it fails the start instead.
func ProvideAuthSettingsInvalidator(
	policy services.AuthPolicyResolver,
	allowlist services.AccessAllowlistResolver,
) (*services.AuthSettingsInvalidator, error) {
	policyCache, ok := policy.(services.AuthSettingsCache)
	if !ok {
		return nil, fmt.Errorf("the auth policy resolver %T cannot be invalidated", policy)
	}
	allowlistCache, ok := allowlist.(services.AuthSettingsCache)
	if !ok {
		return nil, fmt.Errorf("the access allowlist resolver %T cannot be invalidated", allowlist)
	}
	return services.NewAuthSettingsInvalidator(policyCache, allowlistCache), nil
}

// ProvideIdentityProviderResolver creates the runtime sign-in provider resolver
// (#1234): providers are read from instance_auth_providers per sign-in, cached
// by the auth settings version, and built with the derived callback URL.
func ProvideIdentityProviderResolver(
	repo repositories.InstanceAuthProviderRepository,
	versions repositories.InstanceAuthSettingsVersionRepository,
	enc services.EncryptionServiceInterface,
	cfg *config.Config,
	logger *slog.Logger,
) services.IdentityProviderResolver {
	services.WarnIgnoredLegacyRedirectURIs(cfg.Auth, cfg.AuthCallbackURL(), logger)
	return services.NewIdentityProviderResolver(services.IdentityProviderResolverDeps{
		Providers:   repo,
		Versions:    versions,
		Enc:         enc,
		CallbackURL: cfg.AuthCallbackURL(),
		Logger:      logger,
	})
}

// ProvideEmailSenderResolver creates the send-time sender resolver. Both the
// team provider and the INSTANCE FALLBACK are read from the database per send
// (#1188), so nothing about the instance provider is fixed at wire time.
func ProvideEmailSenderResolver(
	repo repositories.TeamEmailProviderRepository,
	instanceRepo repositories.InstanceEmailProviderRepository,
	enc services.EncryptionServiceInterface,
	logger *slog.Logger,
) services.EmailSenderResolver {
	return services.NewEmailSenderResolver(repo, instanceRepo, enc, logger)
}

// ProvideMetadataCatalogService creates the metadata catalog service, which
// backs the metadata key/value discovery endpoints (epic #519).
func ProvideMetadataCatalogService(
	repo repositories.MetadataCatalogRepository,
	logger *slog.Logger,
) services.MetadataCatalogServiceInterface {
	return services.NewMetadataCatalogService(repo, logger)
}

// ProvideFreshnessService creates the freshness rules + settings service
// (epic #726). The repositories landed in #729; this is the first consumer.
// Wire fills the deps struct via wire.Struct (see the container ProviderSet).
func ProvideFreshnessService(deps services.FreshnessServiceDeps) services.FreshnessServiceInterface {
	return services.NewFreshnessService(deps)
}

// ProvideFreshnessClearer creates the freshness reversal helper (#733), shared
// by the async access path and the four resource update services.
func ProvideFreshnessClearer(
	settings repositories.TeamFreshnessSettingsRepository,
	state repositories.ResourceFreshnessRepository,
	audit repositories.FreshnessAuditRepository,
	rules repositories.FreshnessRuleRepository,
	logger *slog.Logger,
) *freshness.Clearer {
	return freshness.NewClearer(settings, state, audit, rules, logger)
}

// ProvideFreshnessEvaluator creates the freshness rule engine (#732), the
// handler behind the scheduler's freshness_evaluate job.
func ProvideFreshnessEvaluator(
	rules repositories.FreshnessRuleRepository,
	candidates repositories.FreshnessCandidateRepository,
	state repositories.ResourceFreshnessRepository,
	audit repositories.FreshnessAuditRepository,
	logger *slog.Logger,
) *freshness.Evaluator {
	return freshness.NewEvaluator(rules, candidates, state, audit, logger)
}

// ProvideInstanceSearchSettingsService creates the instance search settings
// service (#1198): the per-request resolver of the instance ranking defaults
// (instance_search_settings row, else the built-in defaults) and the surface an
// instance admin edits them through.
//
// It returns the CONCRETE type because the service satisfies three interfaces —
// InstanceSearchSettingsResolver (the fail-open read the team resolver builds
// on), InstanceSearchSettingsReader (both reads, for the team settings service)
// and InstanceSearchSettingsServiceInterface (the admin read + writes).
// wire.Bind in wire.go maps it to all three.
func ProvideInstanceSearchSettingsService(
	repo repositories.InstanceSearchSettingsRepository,
	logger *slog.Logger,
) *services.InstanceSearchSettingsService {
	return services.NewInstanceSearchSettingsService(repo, logger)
}

// ProvideInstanceSiteInfoService creates the instance site info service
// (#1353): the fail-open resolver of the instance's company branding and the
// surface an instance admin edits it through.
//
// It returns the CONCRETE type because the service satisfies two interfaces,
// InstanceSiteInfoResolver (the fail-open read) and
// InstanceSiteInfoServiceInterface (the admin read + writes). wire.Bind in
// wire.go maps it to both.
func ProvideInstanceSiteInfoService(
	repo repositories.InstanceSiteInfoRepository,
	logger *slog.Logger,
) *services.InstanceSiteInfoService {
	return services.NewInstanceSiteInfoService(repo, logger)
}

// ProvideInstanceAuditSettingsService creates the instance audit-log settings
// service (#1512): the surface an instance admin edits the retention and the
// client-info capture switch through. It drops capture's cache after every
// write and records each change through recorder.
func ProvideInstanceAuditSettingsService(
	repo repositories.InstanceAuditSettingsRepository,
	capture *services.AuditClientInfoCapture,
	recorder audit.Recorder,
	logger *slog.Logger,
) *services.InstanceAuditSettingsService {
	return services.NewInstanceAuditSettingsService(repo, capture, recorder, logger)
}

// ProvideTeamSearchSettingsService creates the team search settings service.
// The instance reader supplies, per request, both the fallback for a team with
// no stored profile and the instance_defaults reported on every read; the
// service reads them fail-closed, so an unreadable instance row is an error.
func ProvideTeamSearchSettingsService(
	repo repositories.TeamSearchSettingsRepository,
	authzService services.AuthorizationServiceInterface,
	instance services.InstanceSearchSettingsReader,
	logger *slog.Logger,
) services.TeamSearchSettingsServiceInterface {
	return services.NewTeamSearchSettingsService(repo, authzService, instance, logger)
}

// ProvideInstanceAISummarySettingsService creates the instance AI summary
// settings service (#1199): the per-request resolver of the instance AI summary
// defaults and budgets (instance_ai_summary_settings row, else the built-in
// defaults) and the surface an instance admin edits them through.
//
// It returns the CONCRETE type because the service satisfies three interfaces —
// InstanceAISummarySettingsResolver (the fail-open read the search summary
// service builds on), InstanceAISummarySettingsReader (both reads, for the team
// settings service) and InstanceAISummarySettingsServiceInterface (the admin
// read + writes). wire.Bind in wire.go maps it to all three.
func ProvideInstanceAISummarySettingsService(
	repo repositories.InstanceAISummarySettingsRepository,
	logger *slog.Logger,
) *services.InstanceAISummarySettingsService {
	return services.NewInstanceAISummarySettingsService(repo, logger)
}

// ProvideTeamAISummarySettingsService creates the team AI summary settings
// service (#1071). The instance reader supplies, per request, both the
// fallback for a team with no stored profile and the instance_defaults reported
// on every read.
// The model provider repository is the tenancy check on a submitted
// model_provider_id: the column's FK proves existence, not ownership.
//
// It returns the CONCRETE type because the service satisfies two interfaces —
// TeamAISummarySettingsServiceInterface (the settings API's read + writes) and
// AISummarySettingsResolver (the generator's fail-open read). wire.Bind in
// wire.go maps it to both, so the two consumers stay typed against the surface
// each is allowed to use rather than sharing one that offers both reads.
func ProvideTeamAISummarySettingsService(
	repo repositories.TeamAISummarySettingsRepository,
	modelProviders repositories.ModelProviderRepository,
	authzService services.AuthorizationServiceInterface,
	instance services.InstanceAISummarySettingsReader,
	logger *slog.Logger,
) *services.TeamAISummarySettingsService {
	return services.NewTeamAISummarySettingsService(
		repo, modelProviders, authzService, instance, logger)
}

// ProvideTeamSettingsAuditService creates the team settings audit log service
// (epic #827). The authorization service is used by the READ path only (#832,
// authz.TeamSettingsUpdate): entries are WRITTEN from inside an
// already-authorized cross-team copy, which re-checking could only re-derive.
// The two name repositories back the read path's batched actor and source-team
// display-name resolution.
func ProvideTeamSettingsAuditService(
	repo repositories.TeamSettingsAuditRepository,
	authzService services.AuthorizationServiceInterface,
	users repositories.UserRepository,
	teams repositories.TeamRepository,
	logger *slog.Logger,
) services.TeamSettingsAuditServiceInterface {
	return services.NewTeamSettingsAuditService(repo, authzService, users, teams, logger)
}

// ProvideSearchSettingsResolver creates the per-team ranking resolver. The
// instance resolver supplies the INSTANCE DEFAULTS per request, returned for
// any team that has not stored an override of its own, so a change to them
// applies to the next search without a restart.
func ProvideSearchSettingsResolver(
	repo repositories.TeamSearchSettingsRepository,
	instance services.InstanceSearchSettingsResolver,
	logger *slog.Logger,
) services.SearchSettingsResolver {
	return services.NewTeamSearchSettingsResolver(repo, instance, logger)
}

// ProvideSearchService creates a new SearchService. Ranking is no longer baked
// in at wire time: the service resolves it per search through the resolver, so
// a team's stored profile takes effect without a restart.
//
// It returns the concrete type so one instance backs both services.Searcher
// (the search API) and services.SourceDocumentSearcher (the search summary,
// #1073); wire.go binds each interface to it.
func ProvideSearchService(
	repo repositories.SearchRepository,
	embedder services.QueryEmbedder,
	logger *slog.Logger,
	settings services.SearchSettingsResolver,
) *services.SearchService {
	return services.NewSearchService(repo, embedder, logger, settings)
}

// ProvideSearchSummaryService creates the search summary service (#1073). The
// context budgets and request timeout come from the instance resolver and
// everything a team may tune from the team resolver, both per request.
func ProvideSearchSummaryService(
	search services.SourceDocumentSearcher,
	llm services.LLMCompleter,
	settings services.AISummarySettingsResolver,
	instance services.InstanceAISummarySettingsResolver,
	logger *slog.Logger,
) services.SearchSummaryServiceInterface {
	return services.NewSearchSummaryService(search, llm, settings, instance, logger)
}

// ProvideEnvironmentService creates a new EnvironmentService
func ProvideEnvironmentService(cfg *config.Config) *services.EnvironmentService {
	return services.NewEnvironmentService(cfg)
}

// InstanceAuthSettingsServiceDeps groups the dependencies of
// ProvideInstanceAuthSettingsService. Wire fills it via wire.Struct (see the
// container ProviderSet).
type InstanceAuthSettingsServiceDeps struct {
	ProviderRepo      repositories.InstanceAuthProviderRepository
	Allowlists        repositories.InstanceAuthAllowlistRepository
	Policies          repositories.InstanceAuthPolicyRepository
	Versions          repositories.InstanceAuthSettingsVersionRepository
	Grants            repositories.InstanceAdminRepository
	UserRepo          repositories.UserRepository
	Resolver          services.IdentityProviderResolver
	AllowlistResolver services.AccessAllowlistResolver
	Admins            services.InstanceAdminResolver
	WayIn             services.AuthWayIn
	Credentials       repositories.UserCredentialRepository
	RootPasswords     *services.RootAdminPasswordService
	Caches            *services.AuthSettingsInvalidator
	Enc               services.EncryptionServiceInterface
	Cfg               *config.Config
	Logger            *slog.Logger
}

// ProvideInstanceAuthSettingsService creates the service behind the instance
// authentication settings admin API (#1238). It shares the identity provider
// and allowlist resolvers with sign-in, so a provider's reported health is the
// one sign-in acts on.
func ProvideInstanceAuthSettingsService(
	deps InstanceAuthSettingsServiceDeps,
) services.InstanceAuthSettingsServiceInterface {
	return services.NewInstanceAuthSettingsService(services.InstanceAuthSettingsDeps{
		Providers:         deps.ProviderRepo,
		Allowlists:        deps.Allowlists,
		Policies:          deps.Policies,
		Versions:          deps.Versions,
		Grants:            deps.Grants,
		Users:             deps.UserRepo,
		Resolver:          deps.Resolver,
		AllowlistResolver: deps.AllowlistResolver,
		Admins:            deps.Admins,
		WayIn:             deps.WayIn,
		Credentials:       deps.Credentials,
		RootPasswords:     deps.RootPasswords,
		Caches:            deps.Caches,
		Enc:               deps.Enc,
		CallbackURL:       deps.Cfg.AuthCallbackURL(),
		Logger:            deps.Logger,
	})
}

// ProvideAccessAllowlistResolver creates the runtime access allowlist resolver
// (#1235): the allowlist is read from instance_auth_allowlist, cached by the
// auth settings version, and enforced at sign-in, at MCP consent and on every
// authenticated request. Root admins, as answered by admins, are exempt.
func ProvideAccessAllowlistResolver(
	allowlists repositories.InstanceAuthAllowlistRepository,
	versions repositories.InstanceAuthSettingsVersionRepository,
	admins services.InstanceAdminResolver,
	logger *slog.Logger,
) services.AccessAllowlistResolver {
	return services.NewAccessAllowlistResolver(services.AccessAllowlistResolverDeps{
		Allowlists: allowlists,
		Versions:   versions,
		RootAdmins: admins,
		Logger:     logger,
	})
}

// ProvideAuthPolicyResolver creates the runtime authentication policy resolver
// (#1429): the policy is read from instance_auth_policy and cached by the auth
// settings version, and the password reset channel is read from the stored
// instance email provider.
func ProvideAuthPolicyResolver(
	policies repositories.InstanceAuthPolicyRepository,
	versions repositories.InstanceAuthSettingsVersionRepository,
	emailProviders repositories.InstanceEmailProviderRepository,
	logger *slog.Logger,
) services.AuthPolicyResolver {
	return services.NewAuthPolicyResolver(services.AuthPolicyResolverDeps{
		Policies:       policies,
		Versions:       versions,
		EmailProviders: emailProviders,
		Logger:         logger,
	})
}

// AccountServiceDeps groups the dependencies of ProvideAccountService. Wire
// fills it via wire.Struct (see the container ProviderSet).
type AccountServiceDeps struct {
	Users      repositories.UserRepository
	Identities repositories.UserIdentityRepository
	Methods    repositories.SignInMethodRepository
	Passwords  services.PasswordAuthServiceInterface
	Policy     services.AuthPolicyResolver
	Providers  services.IdentityProviderResolver
	APIAccess  services.APIAccessServiceInterface
	Notifier   services.AccountNotifier
	Logger     *slog.Logger
}

// ProvideAccountService creates the self-service account service (#1437). It
// shares the password service, the policy resolver and the provider resolver
// with sign-in, so "which methods can this account use" is answered from the
// same sources sign-in itself reads. The notifier sends the password-changed
// email (#1433).
func ProvideAccountService(deps AccountServiceDeps) services.AccountServiceInterface {
	return services.NewAccountService(services.AccountServiceDeps{
		Users:      deps.Users,
		Identities: deps.Identities,
		Methods:    deps.Methods,
		Passwords:  deps.Passwords,
		Policy:     deps.Policy,
		Providers:  deps.Providers,
		APIAccess:  deps.APIAccess,
		Notifier:   deps.Notifier,
		Logger:     deps.Logger,
	})
}

// ProvidePasswordAuthService creates the email and password sign-in service
// (#1431) with the production argon2id parameters and lockout schedule. It
// shares the policy and allowlist resolvers with every other sign-in path, so
// they cannot disagree on whether password login is on or who is admitted.
func ProvidePasswordAuthService(
	users repositories.UserRepository,
	credentials repositories.UserCredentialRepository,
	policy services.AuthPolicyResolver,
	allowlist services.AccessAllowlistResolver,
	logger *slog.Logger,
) services.PasswordAuthServiceInterface {
	return services.NewPasswordAuthService(services.PasswordAuthDeps{
		Users:       users,
		Credentials: credentials,
		Policy:      policy,
		Allowlist:   allowlist,
		Hasher:      password.NewHasher(password.DefaultParams),
		Lockout:     models.DefaultPasswordLockout,
		Logger:      logger,
	})
}

// AccountTokenServiceDeps groups the dependencies of
// ProvideAccountTokenService. Wire fills it via wire.Struct (see the container
// ProviderSet).
type AccountTokenServiceDeps struct {
	Cfg       *config.Config
	Tokens    repositories.AccountTokenRepository
	Users     repositories.UserRepository
	Passwords services.PasswordAuthServiceInterface
	APIAccess services.APIAccessServiceInterface
	Allowlist services.AccessAllowlistResolver
	Notifier  services.AccountNotifier
	Logger    *slog.Logger
}

// ProvideAccountTokenService creates the one-time account link service
// (#1432). Passwords are stored through the password auth service only, and the
// allowlist resolver is the one every sign-in path shares.
func ProvideAccountTokenService(deps AccountTokenServiceDeps) services.AccountTokenServiceInterface {
	return services.NewAccountTokenService(services.AccountTokenDeps{
		Tokens:    deps.Tokens,
		Users:     deps.Users,
		Passwords: deps.Passwords,
		APIAccess: deps.APIAccess,
		Allowlist: deps.Allowlist,
		Notifier:  deps.Notifier,
		BaseURL:   deps.Cfg.Frontend.BaseURL,
		Logger:    deps.Logger,
	})
}

// InvitationRegistrationDeps groups the dependencies of
// ProvideInvitationRegistrationService. Wire fills it via wire.Struct (see the
// container ProviderSet).
type InvitationRegistrationDeps struct {
	Invitations repositories.TeamInvitationRepository
	Teams       repositories.TeamRepository
	Users       repositories.UserRepository
	Passwords   services.PasswordAuthServiceInterface
	Policy      services.AuthPolicyResolver
	Allowlist   services.AccessAllowlistResolver
	Admins      services.InstanceAdminResolver
	Events      events.EventPublisher
	Logger      *slog.Logger
}

// ProvideInvitationRegistrationService creates the public side of invitations
// (#1435). It hashes passwords with the password auth service's hasher, shares
// the policy and allowlist resolvers with sign-in, and publishes user.created
// for the account it creates, so that account gets its workspace like any
// other.
func ProvideInvitationRegistrationService(
	deps InvitationRegistrationDeps,
) services.InvitationRegistrationServiceInterface {
	return services.NewInvitationRegistrationService(services.InvitationRegistrationDeps{
		Invitations: deps.Invitations,
		Teams:       deps.Teams,
		Users:       deps.Users,
		Passwords:   deps.Passwords,
		Policy:      deps.Policy,
		Allowlist:   deps.Allowlist,
		IsRootAdmin: deps.Admins.IsRootAdmin,
		Events:      deps.Events,
		Logger:      deps.Logger,
	})
}

// ProvideAccountEmailSender creates the sender of the account emails (#1433).
// It is the email service under a narrower interface, built from the same
// resolver and branding so account mail goes out exactly like other instance
// mail.
func ProvideAccountEmailSender(
	resolver services.EmailSenderResolver,
	siteInfo services.InstanceSiteInfoResolver,
	cfg *config.Config,
) services.AccountEmailSender {
	return services.NewEmailService(resolver, siteInfo, cfg)
}

// ProvideAccountEmailRunners creates the two bounded background runners of the
// account emails (#1433): one for forgot-password requests, one for
// password-changed notices, so a flood of the first cannot crowd out the
// second. The container shuts them down.
func ProvideAccountEmailRunners(logger *slog.Logger) *services.AccountEmailRunners {
	return services.NewAccountEmailRunners(logger)
}

// ProvideAccountNotifier creates the notifier that emails an account's owner
// when its password changes (#1433). It depends on nothing that depends on
// it, so both the account service and the link service can take it.
func ProvideAccountNotifier(
	users repositories.UserRepository,
	policy services.AuthPolicyResolver,
	email services.AccountEmailSender,
	runners *services.AccountEmailRunners,
	logger *slog.Logger,
) services.AccountNotifier {
	return services.NewAccountEmailNotifier(services.AccountEmailNotifierDeps{
		Users: users, Policy: policy, Email: email, Runner: runners.Notices, Logger: logger,
	})
}

// AccountLinkMailerDeps groups the dependencies of ProvideAccountLinkMailer.
// Wire fills it via wire.Struct (see the container ProviderSet).
type AccountLinkMailerDeps struct {
	Tokens     services.AccountTokenServiceInterface
	Users      repositories.UserRepository
	Identities repositories.UserIdentityRepository
	Passwords  services.PasswordAuthServiceInterface
	Policy     services.AuthPolicyResolver
	Allowlist  services.AccessAllowlistResolver
	Email      services.AccountEmailSender
	Runners    *services.AccountEmailRunners
	Logger     *slog.Logger
}

// ProvideAccountLinkMailer creates the service that emails one-time account
// links: the forgot-password request and the setup link (#1433).
func ProvideAccountLinkMailer(deps AccountLinkMailerDeps) services.AccountLinkMailerInterface {
	return services.NewAccountLinkMailer(services.AccountLinkMailerDeps{
		Tokens: deps.Tokens, Users: deps.Users, Identities: deps.Identities, Passwords: deps.Passwords,
		Policy: deps.Policy, Allowlist: deps.Allowlist, Email: deps.Email, Runner: deps.Runners.Requests,
		Logger: deps.Logger,
	})
}

// ProvideBackofficeService creates a new BackofficeService
func ProvideBackofficeService(
	backofficeRepo repositories.BackofficeRepository,
) services.UsageAndGrowthGetter {
	return services.NewBackofficeService(backofficeRepo)
}

// ProvideAdminService creates a new AdminService
// It takes the user repository and the event publisher because admin user
// creation (#462) must publish the same `user.created` event as self-signup, so
// the personal workspace and default project are provisioned by one listener
// rather than by two code paths that can drift. The freshness service backs the
// read-only project configuration (#1145).
func ProvideAdminService(
	adminRepo repositories.AdminRepository,
	userRepo repositories.UserRepository,
	eventManager events.EventPublisher,
	freshness services.FreshnessServiceInterface,
	allowlist services.AccessAllowlistResolver,
	teams repositories.TeamRepository,
	members repositories.TeamMemberRepository,
) services.AdminServiceInterface {
	return services.NewAdminService(adminRepo, userRepo, eventManager, freshness,
		services.WithAdminAccessAllowlist(allowlist),
		services.WithAdminTeamAssignment(teams, members))
}

// AdminAccountServiceDeps groups the dependencies of ProvideAdminAccountService.
// Wire fills it via wire.Struct (see the container ProviderSet).
type AdminAccountServiceDeps struct {
	Admin      repositories.AdminRepository
	Users      repositories.UserRepository
	Policy     services.AuthPolicyResolver
	Tokens     services.AccountTokenServiceInterface
	Mailer     services.AccountLinkMailerInterface
	Store      repositories.AccountTokenRepository
	Accounts   services.AccountServiceInterface
	Notifier   services.AccountNotifier
	RootAdmins services.InstanceAdminResolver
	Logger     *slog.Logger
}

// ProvideAdminAccountService creates the service through which instance admins
// give accounts their way in (#1436).
func ProvideAdminAccountService(deps AdminAccountServiceDeps) services.AdminAccountServiceInterface {
	return services.NewAdminAccountService(services.AdminAccountDeps{
		Admin: deps.Admin, Users: deps.Users, Policy: deps.Policy, Tokens: deps.Tokens,
		Mailer: deps.Mailer, Store: deps.Store, Accounts: deps.Accounts, Notifier: deps.Notifier,
		RootAdmins: deps.RootAdmins, Logger: deps.Logger,
	})
}

// ProvideEmbeddingBackfillService creates a new EmbeddingBackfillService that
// republishes `.created` events to regenerate embeddings after a model swap. The
// coverage getter lets a team-scoped run end its log with what is actually embedded,
// since the run's own counters only describe event publishing (#755).
func ProvideEmbeddingBackfillService(
	repo repositories.EmbeddingBackfillRepository,
	publisher events.EventPublisher,
	promptService services.PromptServiceInterface,
	coverage services.EmbeddingCoverageGetter,
	logger *slog.Logger,
) services.EmbeddingBackfiller {
	return services.NewEmbeddingBackfillService(repo, publisher, promptService, coverage, logger)
}

// ProvideEmbeddingStatusService creates a new EmbeddingStatusService that derives
// per-entity-type embedding coverage from a team's active provider model.
func ProvideEmbeddingStatusService(
	providerRepo repositories.EmbeddingProviderRepository,
	coverageRepo repositories.EmbeddingBackfillRepository,
	logger *slog.Logger,
) services.EmbeddingCoverageGetter {
	return services.NewEmbeddingStatusService(providerRepo, coverageRepo, logger)
}

// ProvideUserPreferencesService creates a new UserPreferencesService
func ProvideUserPreferencesService(
	repo repositories.UserPreferencesRepository,
) services.UserPreferencesServiceInterface {
	return services.NewUserPreferencesService(repo)
}

// ProvideAuthorizationService creates a new AuthorizationService
func ProvideAuthorizationService(
	teamMemberRepo repositories.TeamMemberRepository,
	logger *slog.Logger,
) services.AuthorizationServiceInterface {
	return services.NewAuthorizationService(teamMemberRepo, logger)
}

// ProvideTeamService creates a new TeamService
func ProvideTeamService(
	teamRepo repositories.TeamRepository,
	teamMemberRepo repositories.TeamMemberRepository,
	userRepo repositories.UserRepository,
	authzService services.AuthorizationServiceInterface,
	logger *slog.Logger,
	commentRepo repositories.CommentRepository,
) services.TeamServiceInterface {
	return services.NewTeamService(teamRepo, teamMemberRepo, userRepo, authzService, logger, commentRepo)
}

// ProvideTeamInvitationService creates a new TeamInvitationService. Wire fills
// the deps struct via wire.Struct (see the container ProviderSet).
func ProvideTeamInvitationService(deps services.TeamInvitationServiceDeps) *services.TeamInvitationService {
	return services.NewTeamInvitationService(deps)
}

// ProvideProjectService creates a new ProjectService
func ProvideProjectService(
	repo repositories.ProjectRepository,
	teamService services.TeamServiceInterface,
	authzService services.AuthorizationServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) services.ProjectServiceInterface {
	return services.NewProjectService(repo, teamService, authzService, eventManager, logger)
}

// ProvideFeedService creates a new FeedService
func ProvideFeedService(
	feedRepo repositories.FeedRepository,
	teamService services.TeamServiceInterface,
	authzService services.AuthorizationServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) services.FeedServiceInterface {
	return services.NewFeedService(feedRepo, teamService, authzService, eventManager, logger)
}

// ProvideFeedItemService creates a new FeedItemService
func ProvideFeedItemService(
	feedItemRepo repositories.FeedItemRepository,
	replyRepo repositories.FeedItemReplyRepository,
	projectRepo repositories.ProjectRepository,
	teamService services.TeamServiceInterface,
	authzService services.AuthorizationServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) services.FeedItemServiceInterface {
	return services.NewFeedItemService(
		feedItemRepo, replyRepo, projectRepo, teamService, authzService, eventManager, logger,
	)
}

// ProvideFeedItemReplyService creates a new FeedItemReplyService
func ProvideFeedItemReplyService(
	replyRepo repositories.FeedItemReplyRepository,
	feedItemRepo repositories.FeedItemRepository,
	teamService services.TeamServiceInterface,
	authzService services.AuthorizationServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) services.FeedItemReplyServiceInterface {
	return services.NewFeedItemReplyService(
		replyRepo, feedItemRepo, teamService, authzService, eventManager, logger,
	)
}

// NotificationServiceDeps groups the dependencies of ProvideNotificationService.
// Wire fills it via wire.Struct (see the container ProviderSet).
type NotificationServiceDeps struct {
	NotifRepo    repositories.NotificationRepository
	DeliveryRepo repositories.NotificationDeliveryRepository
	PrefRepo     repositories.UserPreferencesRepository
	UserRepo     repositories.UserRepository
	DigestRepo   repositories.NotificationDigestQueueRepository
	EmailSvc     services.EmailServiceInterface
	AppMetrics   *metrics.Metrics
	Logger       *slog.Logger
}

// ProvideNotificationService creates a new NotificationService with all registered channels.
func ProvideNotificationService(deps NotificationServiceDeps) *notificationsvc.NotificationService {
	inAppCh := notifchannels.NewInAppChannel()
	emailCh := notifchannels.NewEmailChannel(deps.EmailSvc, deps.DigestRepo, deps.Logger)

	channels := []notificationsvc.Channel{inAppCh, emailCh}

	return notificationsvc.NewNotificationService(
		deps.NotifRepo,
		deps.DeliveryRepo,
		deps.PrefRepo,
		deps.UserRepo,
		channels,
		deps.AppMetrics,
		deps.Logger,
	)
}

// DigestRunnerDeps groups the dependencies of ProvideDigestRunner. Wire fills
// it via wire.Struct (see the container ProviderSet).
type DigestRunnerDeps struct {
	Cfg        *config.Config
	SiteInfo   services.InstanceSiteInfoResolver
	DigestRepo repositories.NotificationDigestQueueRepository
	NotifRepo  repositories.NotificationRepository
	UserRepo   repositories.UserRepository
	TeamRepo   repositories.TeamRepository
	PrefRepo   repositories.UserPreferencesRepository
	EmailSvc   services.EmailServiceInterface
	AppMetrics *metrics.Metrics
	Logger     *slog.Logger
}

// ProvideDigestRunner creates a new DigestRunner for the daily notification digest job.
// EmailServiceInterface satisfies notifications.DigestEmailSender via Go structural typing
// (both declare SendNotificationEmail(to, subject, htmlBody string) error).
func ProvideDigestRunner(deps DigestRunnerDeps) *notificationsvc.DigestRunner {
	renderer := notificationsvc.NewTemplateRenderer(deps.Cfg.Frontend.BaseURL, deps.SiteInfo)
	return notificationsvc.NewDigestRunner(notificationsvc.DigestRunnerDeps{
		DigestRepo: deps.DigestRepo,
		NotifRepo:  deps.NotifRepo,
		UserRepo:   deps.UserRepo,
		TeamRepo:   deps.TeamRepo,
		PrefRepo:   deps.PrefRepo,
		EmailSvc:   deps.EmailSvc,
		Renderer:   renderer,
		AppMetrics: deps.AppMetrics,
		Logger:     deps.Logger,
	})
}

// ProvideProjectMigrationService creates a new ProjectMigrationService
func ProvideProjectMigrationService(
	db *database.DB,
	projectRepo repositories.ProjectRepository,
	logger *slog.Logger,
) services.ProjectMigrationServiceInterface {
	return projectmigration.NewService(db, projectRepo, logger)
}

// ProvideGitHubAppClientResolver creates the per-team GitHub App client
// resolver (#480). It reads credentials from the team's github_app_configs row
// rather than instance config, so it takes no *config.Config.
func ProvideGitHubAppClientResolver(
	repo repositories.GitHubAppConfigRepository,
	enc services.EncryptionServiceInterface,
	logger *slog.Logger,
) services.GitHubAppClientResolver {
	return services.NewGitHubAppClientResolver(repo, enc, logger)
}

// ProvideGitHubAppService creates a new GitHubAppService. Wire fills the deps
// struct via wire.Struct (see the container ProviderSet).
func ProvideGitHubAppService(deps services.GitHubAppServiceDeps) services.GitHubAppServiceInterface {
	return services.NewGitHubAppService(deps)
}

// ProvideSchedulerRegistry creates the scheduler's job registry and registers
// every job on it. Registration happens here, at wire time, rather than at
// startup: Registry.Register is documented as startup-only because it takes no
// lock, and a registry that is fully populated the moment it is constructed
// cannot race the run loop no matter when the scheduler is started.
func ProvideSchedulerRegistry(freshnessEvaluator *freshness.Evaluator) *scheduler.Registry {
	registry := scheduler.NewRegistry()
	registry.Register(models.JobTypeFreshnessEvaluate, freshnessEvaluator.Evaluate)
	return registry
}

// ProvideSchedulerReconcilers collects every job that can repair its own
// `schedules` rows (#768), the same way ProvideSchedulerRegistry collects every
// job handler: the feature-specific knowledge stops here, so ProvideScheduler
// below stays feature-agnostic and the engine never learns what freshness is.
// A second job with the same shape is one more argument and one more element.
func ProvideSchedulerReconcilers(
	freshnessService services.FreshnessServiceInterface,
) []scheduler.Reconciler {
	return []scheduler.Reconciler{freshnessService}
}

// ProvideScheduler creates the in-process scheduler engine. It is started and
// stopped with the container (see WireContainer.Close); cfg.Scheduler.Enabled
// gates whether the run loop AND the reconcile sweep run at all.
func ProvideScheduler(
	cfg *config.Config,
	repo repositories.ScheduleRepository,
	db *database.DB,
	registry *scheduler.Registry,
	reconcilers []scheduler.Reconciler,
	logger *slog.Logger,
) *scheduler.Scheduler {
	return scheduler.New(repo, db, registry, scheduler.Config{
		TickInterval: cfg.Scheduler.TickInterval,
		JobTimeout:   cfg.Scheduler.JobTimeout,
		DueLimit:     int(cfg.Scheduler.DueLimit),
	}, logger, reconcilers...)
}
