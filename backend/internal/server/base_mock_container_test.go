package server

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/vibexp/vibexp/internal/container"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/scheduler"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	"github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
	"github.com/vibexp/vibexp/pkg/events"
)

// BaseMockContainer provides default nil implementations for all Container interface methods.
// Test-specific mock containers can embed this and only override methods they need.
//
// Usage Example:
//
//	type MockPromptContainer struct {
//	    BaseMockContainer // Embed base container
//	    promptService *mocks.MockPromptServiceInterface
//	}
//
//	func (m *MockPromptContainer) PromptService() services.PromptServiceInterface {
//	    return m.promptService
//	}
//
// This pattern eliminates boilerplate by providing default nil implementations for all
// Container interface methods, allowing test files to only override what they need.
type BaseMockContainer struct{}

// Compile-time interface compliance check
var _ container.Container = (*BaseMockContainer)(nil)

// alwaysActiveUserRepository is the default UserRepository every server test
// gets. Since #454 EVERY authenticated request performs a suspension lookup
// through container.UserRepository(), and that check fails CLOSED — so a nil
// default would reject every authenticated request in every handler test for
// reasons that have nothing to do with what those tests assert.
//
// It answers GetByID with an active user and nothing else; a test that actually
// exercises suspension overrides UserRepository() with its own mock.
type alwaysActiveUserRepository struct {
	repositories.UserRepository
}

func (alwaysActiveUserRepository) GetByID(_ context.Context, userID string) (*models.User, error) {
	return &models.User{ID: userID, Status: models.UserStatusActive}, nil
}

// Repository methods - all return nil by default, except UserRepository (see above)
func (b *BaseMockContainer) UserRepository() repositories.UserRepository {
	return alwaysActiveUserRepository{}
}

func (b *BaseMockContainer) APIKeyRepository() repositories.APIKeyRepository {
	return nil
}

func (b *BaseMockContainer) PromptRepository() repositories.PromptRepository {
	return nil
}

func (b *BaseMockContainer) PromptGalleryRepository() repositories.PromptGalleryRepository {
	return nil
}

func (b *BaseMockContainer) PromptShareRepository() repositories.PromptShareRepository {
	return nil
}

func (b *BaseMockContainer) ArtifactRepository() repositories.ArtifactRepository {
	return nil
}

func (b *BaseMockContainer) BlueprintRepository() repositories.BlueprintRepository {
	return nil
}

func (b *BaseMockContainer) EmbeddingProviderRepository() repositories.EmbeddingProviderRepository {
	return nil
}

func (b *BaseMockContainer) ModelProviderRepository() repositories.ModelProviderRepository {
	return nil
}

func (b *BaseMockContainer) ActivityRepository() repositories.ActivityRepository {
	return nil
}

func (b *BaseMockContainer) ResourceAccessRepository() repositories.ResourceAccessRepository {
	return nil
}

func (b *BaseMockContainer) AgentRepository() repositories.AgentRepository {
	return nil
}

func (b *BaseMockContainer) AgentExecutionRepository() repositories.AgentExecutionRepository {
	return nil
}

func (b *BaseMockContainer) AgentExecutionEventRepository() repositories.AgentExecutionEventRepository {
	return nil
}

func (b *BaseMockContainer) MemoryRepository() repositories.MemoryRepository {
	return nil
}

func (b *BaseMockContainer) EmbeddingRepository() repositories.EmbeddingRepository {
	return nil
}

func (b *BaseMockContainer) BackofficeRepository() repositories.BackofficeRepository {
	return nil
}

func (b *BaseMockContainer) UserPreferencesRepository() repositories.UserPreferencesRepository {
	return nil
}

func (b *BaseMockContainer) TeamRepository() repositories.TeamRepository {
	return nil
}

func (b *BaseMockContainer) TeamMemberRepository() repositories.TeamMemberRepository {
	return nil
}

func (b *BaseMockContainer) ProjectRepository() repositories.ProjectRepository {
	return nil
}

func (b *BaseMockContainer) WebhookEventRepository() repositories.WebhookEventRepository {
	return nil
}

func (b *BaseMockContainer) GitHubInstallationRepository() repositories.GitHubInstallationRepository {
	return nil
}

func (b *BaseMockContainer) FeedRepository() repositories.FeedRepository {
	return nil
}

func (b *BaseMockContainer) FeedItemRepository() repositories.FeedItemRepository {
	return nil
}

// Service methods - all return nil by default
func (b *BaseMockContainer) AuthService() services.AuthServiceInterface {
	return nil
}

func (b *BaseMockContainer) APIKeyService() services.APIKeyServiceInterface {
	return nil
}

func (b *BaseMockContainer) PromptService() services.PromptServiceInterface {
	return nil
}

func (b *BaseMockContainer) PromptGalleryService() services.PromptGalleryServiceInterface {
	return nil
}

func (b *BaseMockContainer) PromptShareService() services.PromptShareServiceInterface {
	return nil
}

func (b *BaseMockContainer) ArtifactService() services.ArtifactServiceInterface {
	return nil
}

func (b *BaseMockContainer) AttachmentService() services.AttachmentServiceInterface {
	return nil
}

func (b *BaseMockContainer) CommentService() services.CommentServiceInterface {
	return nil
}

func (b *BaseMockContainer) RelationService() services.RelationServiceInterface {
	return nil
}

func (b *BaseMockContainer) RelationSeedService() services.RelationSeedServiceInterface {
	return nil
}

func (b *BaseMockContainer) TypeService() services.TypeServiceInterface {
	return nil
}

func (b *BaseMockContainer) BlueprintService() services.BlueprintServiceInterface {
	return nil
}

func (b *BaseMockContainer) EmbeddingProviderService() services.EmbeddingProviderServiceInterface {
	return nil
}

func (b *BaseMockContainer) ModelProviderService() services.ModelProviderServiceInterface {
	return nil
}

func (b *BaseMockContainer) GitHubAppConfigService() services.GitHubAppConfigServiceInterface {
	return nil
}

func (b *BaseMockContainer) EmailService() services.EmailServiceInterface {
	return nil
}

func (b *BaseMockContainer) ResourceAccessService() resourceaccess.ResourceAccessService {
	return nil
}

func (b *BaseMockContainer) ActivityService() activities.ActivityService {
	return nil
}

func (b *BaseMockContainer) AgentService() services.AgentServiceInterface {
	return nil
}

func (b *BaseMockContainer) AgentCardFetcher() services.CardFetcher {
	return nil
}

func (b *BaseMockContainer) AgentInvocationService() services.AgentInvocationServiceInterface {
	return nil
}

func (b *BaseMockContainer) MemoryService() services.MemoryServiceInterface {
	return nil
}

func (b *BaseMockContainer) EmbeddingService() services.EmbeddingServiceInterface {
	return nil
}

func (b *BaseMockContainer) SearchService() services.Searcher {
	return nil
}

func (b *BaseMockContainer) SearchSummaryService() services.SearchSummaryServiceInterface {
	return nil
}

func (b *BaseMockContainer) AISummaryAvailability() services.AISummaryAvailabilityResolver {
	return nil
}

func (b *BaseMockContainer) TeamSearchSettingsService() services.TeamSearchSettingsServiceInterface {
	return nil
}

// TeamAISummarySettingsService returns nil; suites that exercise it install their own.
func (b *BaseMockContainer) TeamAISummarySettingsService() services.TeamAISummarySettingsServiceInterface {
	return nil
}

// TeamSettingsAuditRepository returns nil; suites that exercise it install their own.
func (b *BaseMockContainer) TeamSettingsAuditRepository() repositories.TeamSettingsAuditRepository {
	return nil
}

// noInstanceAdminGrants is an InstanceAdminRepository holding no DB grants.
type noInstanceAdminGrants struct {
	repositories.InstanceAdminRepository
}

func (noInstanceAdminGrants) IsGranted(context.Context, string) (bool, error) { return false, nil }

// InstanceAdminResolver returns a resolver with no root admins and no DB
// grants, so nobody is an instance admin. Admin suites install one built from
// their config (see newAdminTestServer).
func (b *BaseMockContainer) InstanceAdminResolver() services.InstanceAdminResolver {
	return services.NewInstanceAdminService(nil, noInstanceAdminGrants{}, alwaysActiveUserRepository{},
		slog.New(slog.DiscardHandler))
}

// memAccessAllowlist is an in-memory instance_auth_allowlist plus the shared
// auth settings version, so the real allowlist resolver can be driven end to
// end without a database. The zero value stores no allowlist: open access.
type memAccessAllowlist struct {
	repositories.InstanceAuthAllowlistRepository

	mu      sync.Mutex
	row     *models.InstanceAuthAllowlist
	version int64
	err     error
}

// set replaces the stored allowlist and bumps the version, as every audited
// write does. Both lists empty removes the row.
func (m *memAccessAllowlist) set(domains, emails []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.version++
	m.row = nil
	if len(domains) > 0 || len(emails) > 0 {
		m.row = &models.InstanceAuthAllowlist{Domains: domains, Emails: emails}
	}
}

// fail makes every read return err until it is cleared with nil.
func (m *memAccessAllowlist) fail(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

func (m *memAccessAllowlist) Get(context.Context) (*models.InstanceAuthAllowlist, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	if m.row == nil {
		return nil, repositories.ErrInstanceAuthAllowlistNotFound
	}
	return m.row, nil
}

// memAccessAllowlistVersions reads the store's version.
type memAccessAllowlistVersions struct{ store *memAccessAllowlist }

func (v memAccessAllowlistVersions) Get(context.Context) (int64, error) {
	v.store.mu.Lock()
	defer v.store.mu.Unlock()
	return v.store.version, v.store.err
}

// newMemAllowlistResolver is the real allowlist resolver over store. It probes
// the version on every decision, so a change to store is enforced by the very
// next call instead of after the production probe interval.
func newMemAllowlistResolver(
	logger *slog.Logger, store *memAccessAllowlist, rootAdmins ...string,
) services.AccessAllowlistResolver {
	return services.NewAccessAllowlistResolver(services.AccessAllowlistResolverDeps{
		Allowlists:    store,
		Versions:      memAccessAllowlistVersions{store},
		RootAdmins:    services.NewInstanceAdminService(rootAdmins, nil, nil, logger),
		Logger:        logger,
		ProbeInterval: time.Nanosecond,
	})
}

// staticAllowlistResolver is the real resolver over a fixed allowlist.
func staticAllowlistResolver(domains, emails []string) services.AccessAllowlistResolver {
	store := &memAccessAllowlist{}
	store.set(domains, emails)
	return newMemAllowlistResolver(slog.New(slog.DiscardHandler), store)
}

// AccessAllowlistResolver returns the real resolver with no allowlist stored,
// so everyone is allowed. Since #1235 EVERY authenticated request consults it
// and a nil one fails closed; suites that exercise the allowlist install their
// own.
func (b *BaseMockContainer) AccessAllowlistResolver() services.AccessAllowlistResolver {
	return staticAllowlistResolver(nil, nil)
}

// InstanceSettingsAuditRepository returns nil; suites that exercise it install their own.
func (b *BaseMockContainer) InstanceSettingsAuditRepository() repositories.InstanceSettingsAuditRepository {
	return nil
}

// TeamSettingsAuditService returns nil; suites that exercise it install their own.
func (b *BaseMockContainer) TeamSettingsAuditService() services.TeamSettingsAuditServiceInterface {
	return nil
}

// FreshnessService returns nil; suites that exercise it install their own.
func (b *BaseMockContainer) FreshnessService() services.FreshnessServiceInterface {
	return nil
}

func (b *BaseMockContainer) MetadataCatalogService() services.MetadataCatalogServiceInterface {
	return nil
}

func (b *BaseMockContainer) TeamEmailProviderService() services.TeamEmailProviderServiceInterface {
	return nil
}

// InstanceEmailProviderService returns nil; suites that exercise it install their own.
func (b *BaseMockContainer) InstanceEmailProviderService() services.InstanceEmailProviderServiceInterface {
	return nil
}

// InstanceSearchSettingsService returns nil; suites that exercise it install their own.
func (b *BaseMockContainer) InstanceSearchSettingsService() services.InstanceSearchSettingsServiceInterface {
	return nil
}

// InstanceAISummarySettingsService returns nil; suites that exercise it install their own.
func (b *BaseMockContainer) InstanceAISummarySettingsService() services.InstanceAISummarySettingsServiceInterface {
	return nil
}

func (b *BaseMockContainer) EmailSenderResolver() services.EmailSenderResolver {
	return nil
}

func (b *BaseMockContainer) EnvironmentService() *services.EnvironmentService {
	return nil
}

func (b *BaseMockContainer) BackofficeService() services.UsageAndGrowthGetter {
	return nil
}

func (b *BaseMockContainer) AdminService() services.AdminServiceInterface {
	return nil
}

func (b *BaseMockContainer) EmbeddingStatusService() services.EmbeddingCoverageGetter {
	return nil
}

func (b *BaseMockContainer) EmbeddingBackfillService() services.EmbeddingBackfiller {
	return nil
}

func (b *BaseMockContainer) UserPreferencesService() services.UserPreferencesServiceInterface {
	return nil
}

func (b *BaseMockContainer) AuthorizationService() services.AuthorizationServiceInterface {
	return nil
}

func (b *BaseMockContainer) TeamService() services.TeamServiceInterface {
	return nil
}

func (b *BaseMockContainer) TeamInvitationService() *services.TeamInvitationService {
	return nil
}

func (b *BaseMockContainer) ProjectService() services.ProjectServiceInterface {
	return nil
}

func (b *BaseMockContainer) ProjectMigrationService() services.ProjectMigrationServiceInterface {
	return nil
}

func (b *BaseMockContainer) GitHubAppService() services.GitHubAppServiceInterface {
	return nil
}

func (b *BaseMockContainer) FeedService() services.FeedServiceInterface {
	return nil
}

func (b *BaseMockContainer) FeedItemService() services.FeedItemServiceInterface {
	return nil
}

func (b *BaseMockContainer) FeedItemReplyService() services.FeedItemReplyServiceInterface {
	return nil
}

func (b *BaseMockContainer) FeedItemReplyRepository() repositories.FeedItemReplyRepository {
	return nil
}

// Notification repositories
func (b *BaseMockContainer) NotificationRepository() repositories.NotificationRepository {
	return nil
}

func (b *BaseMockContainer) NotificationDeliveryRepository() repositories.NotificationDeliveryRepository {
	return nil
}

func (b *BaseMockContainer) NotificationDigestQueueRepository() repositories.NotificationDigestQueueRepository {
	return nil
}

// Notification service
func (b *BaseMockContainer) NotificationService() notifications.NotificationServiceInterface {
	return nil
}

// DigestRunner returns a nil DigestRunner (not used in most tests)
func (b *BaseMockContainer) DigestRunner() *notifications.DigestRunner {
	return nil
}

// Scheduler returns a nil Scheduler (not used in handler tests)
func (b *BaseMockContainer) Scheduler() *scheduler.Scheduler {
	return nil
}

// Event system
func (b *BaseMockContainer) EventManager() events.EventPublisher {
	return nil
}

// External dependencies
func (b *BaseMockContainer) IdentityProviderResolver() services.IdentityProviderResolver {
	return nil
}

func (b *BaseMockContainer) EmailSender() external.EmailSender {
	return nil
}

func (b *BaseMockContainer) GitHubAppClient() external.GitHubAppClient {
	return nil
}

// Legacy method for database access
func (b *BaseMockContainer) Database() *database.DB {
	return nil
}

// StartEventListeners is a no-op: a mock container owns no background loops.
func (b *BaseMockContainer) StartEventListeners() {}

// RunStartupImports is a no-op: a mock container has no database to import into.
func (b *BaseMockContainer) RunStartupImports(context.Context) {}

// Cleanup resources
func (b *BaseMockContainer) Close() error {
	return nil
}
