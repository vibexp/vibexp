package testutils

import (
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/auth/idp"
	idpmocks "github.com/vibexp/vibexp/internal/auth/idp/mocks"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/external"
	extmocks "github.com/vibexp/vibexp/internal/external/mocks"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// TestingInterface defines the interface needed for mock construction
type TestingInterface interface {
	Cleanup(func())
	Errorf(format string, args ...interface{})
	FailNow()
	Logf(format string, args ...interface{})
}

// MockServiceContainer holds all service mocks for easy setup in tests
type MockServiceContainer struct {
	Auth              *svcmocks.MockAuthServiceInterface
	APIKey            *svcmocks.MockAPIKeyServiceInterface
	Prompt            *svcmocks.MockPromptServiceInterface
	EmbeddingProvider *svcmocks.MockEmbeddingProviderServiceInterface
	Email             *svcmocks.MockEmailServiceInterface
	Memory            *svcmocks.MockMemoryServiceInterface
}

// NewMockServiceContainer creates all service mocks
func NewMockServiceContainer(t TestingInterface) *MockServiceContainer {
	return &MockServiceContainer{
		Auth:              svcmocks.NewMockAuthServiceInterface(t),
		APIKey:            svcmocks.NewMockAPIKeyServiceInterface(t),
		Prompt:            svcmocks.NewMockPromptServiceInterface(t),
		EmbeddingProvider: svcmocks.NewMockEmbeddingProviderServiceInterface(t),
		Email:             svcmocks.NewMockEmailServiceInterface(t),
		Memory:            svcmocks.NewMockMemoryServiceInterface(t),
	}
}

// MockRepositoryContainer holds all repository mocks for easy setup in tests
type MockRepositoryContainer struct {
	User              *repomocks.MockUserRepository
	APIKey            *repomocks.MockAPIKeyRepository
	Prompt            *repomocks.MockPromptRepository
	EmbeddingProvider *repomocks.MockEmbeddingProviderRepository
	Memory            *repomocks.MockMemoryRepository
}

// NewMockRepositoryContainer creates all repository mocks
func NewMockRepositoryContainer(t TestingInterface) *MockRepositoryContainer {
	return &MockRepositoryContainer{
		User:              repomocks.NewMockUserRepository(t),
		APIKey:            repomocks.NewMockAPIKeyRepository(t),
		Prompt:            repomocks.NewMockPromptRepository(t),
		EmbeddingProvider: repomocks.NewMockEmbeddingProviderRepository(t),
		Memory:            repomocks.NewMockMemoryRepository(t),
	}
}

// MockExternalContainer holds all external service mocks for easy setup in tests
type MockExternalContainer struct {
	IDP  *idpmocks.MockIdentityProvider
	SMTP *extmocks.MockSMTPClient
}

// NewMockExternalContainer creates all external service mocks
func NewMockExternalContainer(t TestingInterface) *MockExternalContainer {
	return &MockExternalContainer{
		IDP:  idpmocks.NewMockIdentityProvider(t),
		SMTP: extmocks.NewMockSMTPClient(t),
	}
}

// MockContainer holds all mocks for comprehensive test setup
type MockContainer struct {
	Services     *MockServiceContainer
	Repositories *MockRepositoryContainer
	External     *MockExternalContainer
}

// NewMockContainer creates all mocks in one convenient container
func NewMockContainer(t TestingInterface) *MockContainer {
	return &MockContainer{
		Services:     NewMockServiceContainer(t),
		Repositories: NewMockRepositoryContainer(t),
		External:     NewMockExternalContainer(t),
	}
}

// NewMockContainerForTest is a convenience method that works with *testing.T
func NewMockContainerForTest(t *testing.T) *MockContainer {
	return NewMockContainer(t)
}

// MockAppContainer implements the container.Container interface for testing
type MockAppContainer struct {
	mock.Mock
}

// Ensure MockAppContainer implements Container interface
var _ interface {
	MemoryService() services.MemoryServiceInterface
	ResourceUsageService() services.ResourceUsageServiceInterface
	ActivityService() activities.ActivityService
	EmbeddingService() services.EmbeddingServiceInterface
	Database() *database.DB
	Close() error
} = (*MockAppContainer)(nil)

// NewMockAppContainer creates a new mock app container
func NewMockAppContainer(t TestingInterface) *MockAppContainer {
	mockContainer := &MockAppContainer{}
	t.Cleanup(func() {
		mockContainer.AssertExpectations(t.(mock.TestingT))
	})
	return mockContainer
}

// Repository methods
func (m *MockAppContainer) UserRepository() repositories.UserRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(repositories.UserRepository)
}

func (m *MockAppContainer) APIKeyRepository() repositories.APIKeyRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(repositories.APIKeyRepository)
}

func (m *MockAppContainer) PromptRepository() repositories.PromptRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(repositories.PromptRepository)
}

func (m *MockAppContainer) ArtifactRepository() repositories.ArtifactRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(repositories.ArtifactRepository)
}

func (m *MockAppContainer) EmbeddingProviderRepository() repositories.EmbeddingProviderRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(repositories.EmbeddingProviderRepository)
}

func (m *MockAppContainer) ActivityRepository() repositories.ActivityRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(repositories.ActivityRepository)
}

func (m *MockAppContainer) ClaudeCodeHooksRepository() repositories.ClaudeCodeHooksRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(repositories.ClaudeCodeHooksRepository)
}

func (m *MockAppContainer) CursorIDEHooksRepository() repositories.CursorIDEHooksRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(repositories.CursorIDEHooksRepository)
}

func (m *MockAppContainer) AgentRepository() repositories.AgentRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(repositories.AgentRepository)
}

func (m *MockAppContainer) AgentExecutionRepository() repositories.AgentExecutionRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(repositories.AgentExecutionRepository)
}

func (m *MockAppContainer) AgentExecutionEventRepository() repositories.AgentExecutionEventRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(repositories.AgentExecutionEventRepository)
}

func (m *MockAppContainer) MemoryRepository() repositories.MemoryRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(repositories.MemoryRepository)
}

func (m *MockAppContainer) EmbeddingRepository() repositories.EmbeddingRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(repositories.EmbeddingRepository)
}

// Service methods
func (m *MockAppContainer) AuthService() services.AuthServiceInterface {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(services.AuthServiceInterface)
}

func (m *MockAppContainer) APIKeyService() services.APIKeyServiceInterface {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(services.APIKeyServiceInterface)
}

func (m *MockAppContainer) PromptService() services.PromptServiceInterface {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(services.PromptServiceInterface)
}

func (m *MockAppContainer) ArtifactService() services.ArtifactServiceInterface {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(services.ArtifactServiceInterface)
}

func (m *MockAppContainer) EmbeddingProviderService() services.EmbeddingProviderServiceInterface {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(services.EmbeddingProviderServiceInterface)
}

func (m *MockAppContainer) EmailService() services.EmailServiceInterface {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(services.EmailServiceInterface)
}

func (m *MockAppContainer) ActivityService() activities.ActivityService {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(activities.ActivityService)
}

func (m *MockAppContainer) AgentService() services.AgentServiceInterface {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(services.AgentServiceInterface)
}

func (m *MockAppContainer) AgentCardFetcher() services.AgentCardFetcherInterface {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(services.AgentCardFetcherInterface)
}

func (m *MockAppContainer) AgentInvocationService() services.AgentInvocationServiceInterface {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(services.AgentInvocationServiceInterface)
}

func (m *MockAppContainer) MemoryService() services.MemoryServiceInterface {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(services.MemoryServiceInterface)
}

func (m *MockAppContainer) EmbeddingService() services.EmbeddingServiceInterface {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(services.EmbeddingServiceInterface)
}

func (m *MockAppContainer) EnvironmentService() *services.EnvironmentService {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*services.EnvironmentService)
}

func (m *MockAppContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(services.ResourceUsageServiceInterface)
}

// External methods
func (m *MockAppContainer) IdentityProvider() idp.IdentityProvider {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(idp.IdentityProvider)
}

func (m *MockAppContainer) SMTPClient() external.SMTPClient {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(external.SMTPClient)
}

func (m *MockAppContainer) Database() *database.DB {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*database.DB)
}

func (m *MockAppContainer) Close() error {
	args := m.Called()
	return args.Error(0)
}
