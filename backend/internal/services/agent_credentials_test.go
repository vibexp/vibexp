package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	repoMocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// failingEncryption is an EncryptionServiceInterface whose operations always
// fail, simulating a broken/misconfigured encryption key.
type failingEncryption struct{ err error }

func (f failingEncryption) Encrypt(string) (string, error) { return "", f.err }
func (f failingEncryption) Decrypt(string) (string, error) { return "", f.err }

// newAgentServiceWithEncryption builds an AgentService around an arbitrary
// EncryptionServiceInterface so credential encryption failures can be injected.
func newAgentServiceWithEncryption(
	agentRepo *repoMocks.MockAgentRepository,
	cardFetcher CardFetcher,
	encryption EncryptionServiceInterface,
) *AgentService {
	logger, _ := logtest.New()
	return NewAgentServiceWithCardFetcher(
		agentRepo, nil, cardFetcher, encryption, nil, allowAllAuthz{}, logger,
	)
}

func testCredentials() map[string]models.CredentialRequest {
	return map[string]models.CredentialRequest{
		"api_key": {Type: "apiKey", Value: "super-secret"},
	}
}

// TestAgentService_CreateAgent_EncryptionFailureAbortsCreate pins that a
// credential that cannot be encrypted aborts the create BEFORE the repository
// write: no agent row may exist whose credentials were never secured.
func TestAgentService_CreateAgent_EncryptionFailureAbortsCreate(t *testing.T) {
	agentRepo := repoMocks.NewMockAgentRepository(t)
	cardFetcher := &MockAgentCardFetcher{}
	encryption := failingEncryption{err: fmt.Errorf("bad encryption key")}
	service := newAgentServiceWithEncryption(agentRepo, cardFetcher, encryption)

	cardFetcher.On("FetchAgentCard", mock.Anything, mock.Anything, mock.Anything).
		Return(&models.AgentCard{Name: "Test Agent"}, nil)

	req := createTestCreateAgentRequest()
	req.Credentials = testCredentials()

	agent, err := service.CreateAgent(context.Background(), "user-123", "team-123", req)

	require.Error(t, err)
	assert.Nil(t, agent)
	assert.Contains(t, err.Error(), "failed to encrypt credentials")
	agentRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// TestAgentService_UpdateAgent_EncryptionFailureAbortsUpdate pins the same
// decision on the update path (updateAgentCredentials): encryption failure
// surfaces to the caller and nothing is persisted.
func TestAgentService_UpdateAgent_EncryptionFailureAbortsUpdate(t *testing.T) {
	agentRepo := repoMocks.NewMockAgentRepository(t)
	encryption := failingEncryption{err: fmt.Errorf("bad encryption key")}
	service := newAgentServiceWithEncryption(agentRepo, &MockAgentCardFetcher{}, encryption)

	agentRepo.On("GetByID", mock.Anything, "user-123", "team-123", "agent-123").
		Return(createTestAgent(), nil)

	req := &models.UpdateAgentRequest{Credentials: testCredentials()}

	agent, err := service.UpdateAgent(context.Background(), "user-123", "team-123", "agent-123", req)

	require.Error(t, err)
	assert.Nil(t, agent)
	assert.Contains(t, err.Error(), "failed to encrypt credentials")
	agentRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

// TestAgentService_UpdateAgentCredentials_EncryptionFailure covers the
// credentials-only endpoint: encryption failure aborts before the repo write.
func TestAgentService_UpdateAgentCredentials_EncryptionFailure(t *testing.T) {
	agentRepo := repoMocks.NewMockAgentRepository(t)
	encryption := failingEncryption{err: fmt.Errorf("cipher init failed")}
	service := newAgentServiceWithEncryption(agentRepo, &MockAgentCardFetcher{}, encryption)

	agentRepo.On("GetByID", mock.Anything, "user-123", "team-123", "agent-123").
		Return(createTestAgent(), nil)

	err := service.UpdateAgentCredentials(
		context.Background(), "user-123", "team-123", "agent-123",
		&models.UpdateAgentCredentialsRequest{Credentials: testCredentials()},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to encrypt credentials")
	agentRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

// TestAgentService_UpdateAgentCredentials_RepoUpdateFailure pins that a
// repository failure after successful encryption is propagated as the
// credentials-specific error, not swallowed.
func TestAgentService_UpdateAgentCredentials_RepoUpdateFailure(t *testing.T) {
	agentRepo := repoMocks.NewMockAgentRepository(t)
	cardFetcher := &MockAgentCardFetcher{}
	service := createTestAgentService(agentRepo, nil, cardFetcher) // real encryption

	agentRepo.On("GetByID", mock.Anything, "user-123", "team-123", "agent-123").
		Return(createTestAgent(), nil)
	agentRepo.On("Update", mock.Anything, mock.Anything).Return(fmt.Errorf("db down"))

	err := service.UpdateAgentCredentials(
		context.Background(), "user-123", "team-123", "agent-123",
		&models.UpdateAgentCredentialsRequest{Credentials: testCredentials()},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update agent credentials")
}

// TestAgentService_UpdateAgentCredentials_AgentLookupFailure covers the first
// error branch: an unknown agent aborts before any encryption or write.
func TestAgentService_UpdateAgentCredentials_AgentLookupFailure(t *testing.T) {
	agentRepo := repoMocks.NewMockAgentRepository(t)
	service := createTestAgentService(agentRepo, nil, &MockAgentCardFetcher{})

	agentRepo.On("GetByID", mock.Anything, "user-123", "team-123", "missing").
		Return(nil, fmt.Errorf("agent not found"))

	err := service.UpdateAgentCredentials(
		context.Background(), "user-123", "team-123", "missing",
		&models.UpdateAgentCredentialsRequest{Credentials: testCredentials()},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get agent")
	agentRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

// TestEncryptCredentials_StoresCiphertextAndMergesExisting pins the real
// behavioral contract of encryptCredentials with the real AES-GCM service:
// values are stored encrypted (never plaintext), decrypt back to the original,
// the credential type is preserved, a nil credentials map is initialized, and
// existing credentials survive an update that adds a new one.
func TestEncryptCredentials_StoresCiphertextAndMergesExisting(t *testing.T) {
	service := createTestAgentService(
		repoMocks.NewMockAgentRepository(t), nil, &MockAgentCardFetcher{},
	) // real encryption service

	// Nil map is initialized on first use.
	agent := &models.Agent{ID: "agent-123"}
	require.NoError(t, service.encryptCredentials(agent, testCredentials()))
	require.NotNil(t, agent.Credentials)

	stored, ok := (*agent.Credentials)["api_key"]
	require.True(t, ok)
	assert.Equal(t, "apiKey", stored.Type)
	assert.NotEqual(t, "super-secret", stored.Value, "credential must not be stored in plaintext")
	decrypted, err := service.encryptionService.Decrypt(stored.Value)
	require.NoError(t, err)
	assert.Equal(t, "super-secret", decrypted)

	// Adding a second credential merges; the first one is preserved untouched.
	require.NoError(t, service.encryptCredentials(agent, map[string]models.CredentialRequest{
		"bearer": {Type: "http", Value: "token-2"},
	}))
	assert.Len(t, *agent.Credentials, 2)
	assert.Equal(t, stored, (*agent.Credentials)["api_key"], "existing credential must survive a merge")
	decrypted2, err := service.encryptionService.Decrypt((*agent.Credentials)["bearer"].Value)
	require.NoError(t, err)
	assert.Equal(t, "token-2", decrypted2)
}

// TestAgentService_UpdateAgent_CredentialsEncryptedAndPersisted pins the
// success branch of updateAgentCredentials through UpdateAgent: the agent
// handed to the repository carries the credential encrypted, never plaintext.
func TestAgentService_UpdateAgent_CredentialsEncryptedAndPersisted(t *testing.T) {
	agentRepo := repoMocks.NewMockAgentRepository(t)
	service := createTestAgentService(agentRepo, nil, &MockAgentCardFetcher{}) // real encryption

	agentRepo.On("GetByID", mock.Anything, "user-123", "team-123", "agent-123").
		Return(createTestAgent(), nil)
	agentRepo.On("Update", mock.Anything, mock.MatchedBy(func(a *models.Agent) bool {
		if a.Credentials == nil {
			return false
		}
		cred, ok := (*a.Credentials)["api_key"]
		return ok && cred.Type == "apiKey" && cred.Value != "" && cred.Value != "super-secret"
	})).Return(nil).Once()

	req := &models.UpdateAgentRequest{Credentials: testCredentials()}
	agent, err := service.UpdateAgent(context.Background(), "user-123", "team-123", "agent-123", req)

	require.NoError(t, err)
	require.NotNil(t, agent)
	agentRepo.AssertExpectations(t)
}
