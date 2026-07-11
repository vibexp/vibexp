package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repoMocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// MockAgentCardFetcher is a mock implementation of AgentCardFetcherInterface
type MockAgentCardFetcher struct {
	mock.Mock
}

func (m *MockAgentCardFetcher) FetchAgentCard(ctx context.Context, cardURL string) (*models.AgentCard, error) {
	args := m.Called(ctx, cardURL)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.AgentCard), args.Error(1)
}

func createTestAgentService(
	agentRepo repositories.AgentRepository,
	executionRepo repositories.AgentExecutionRepository,
	cardFetcher AgentCardFetcherInterface,
) *AgentService {
	encryptionSvc, err := NewEncryptionService("test-encryption-key-32-bytes1234")
	if err != nil {
		panic(fmt.Sprintf("Failed to create encryption service for tests: %v", err))
	}
	logger, _ := logtest.New()
	return NewAgentServiceWithCardFetcher(agentRepo, executionRepo, cardFetcher, encryptionSvc, nil, logger)
}

func createTestAgent() *models.Agent {
	now := time.Now()
	return &models.Agent{
		ID:          "agent-123",
		UserID:      "user-123",
		Name:        "Test Agent",
		Description: "A test agent for unit testing",
		Status:      "active",
		CardURL:     agentStringPtr("http://localhost:8000/.well-known/agent-card.json"),
		AgentCard: &models.AgentCard{
			Name:        "Test Agent",
			Description: "A test agent for unit testing",
			SupportedInterfaces: []*a2a.AgentInterface{
				{URL: "http://localhost:8000", ProtocolBinding: a2a.TransportProtocolHTTPJSON},
			},
			Version:            "1.0.0",
			Capabilities:       a2a.AgentCapabilities{},
			DefaultInputModes:  []string{"text/plain"},
			DefaultOutputModes: []string{"text/plain"},
			Skills: []a2a.AgentSkill{
				{
					ID:          "test-skill",
					Name:        "Test Skill",
					Description: "A test skill",
					Tags:        []string{"test"},
				},
			},
		},
		TotalRuns:   5,
		SuccessRate: 80.0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func createTestCreateAgentRequest() *models.CreateAgentRequest {
	return &models.CreateAgentRequest{
		CardURL: "http://localhost:8000/.well-known/agent-card.json",
		Status:  "active",
	}
}

func createTestUpdateAgentRequest() *models.UpdateAgentRequest {
	status := "paused"
	cardURL := "http://localhost:8000/.well-known/updated-agent-card.json"
	return &models.UpdateAgentRequest{
		Status:  &status,
		CardURL: &cardURL,
	}
}

func createTestAgentExecution() *models.AgentExecution {
	now := time.Now()
	duration := 1500
	return &models.AgentExecution{
		ID:        "execution-123",
		AgentID:   "agent-123",
		UserID:    "user-123",
		Status:    "success",
		Input:     map[string]interface{}{"message": "test input"},
		StartedAt: now.Add(-2 * time.Minute),
		EndedAt:   &now,
		Duration:  &duration,
	}
}

func createTestCreateAgentExecutionRequest() *models.CreateAgentExecutionRequest {
	return &models.CreateAgentExecutionRequest{
		Input: map[string]interface{}{"message": "test input"},
	}
}

func createTestUpdateAgentExecutionRequest() *models.UpdateAgentExecutionRequest {
	return &models.UpdateAgentExecutionRequest{
		Status: "success",
	}
}

// Helper function for string pointers
func agentStringPtr(s string) *string {
	return &s
}

func TestNewAgentService(t *testing.T) {
	mockAgentRepo := repoMocks.NewMockAgentRepository(t)
	mockExecutionRepo := repoMocks.NewMockAgentExecutionRepository(t)
	encryptionSvc, err := NewEncryptionService("test-encryption-key-32-bytes1234")
	require.NoError(t, err)
	logger, _ := logtest.New()
	service := NewAgentService(mockAgentRepo, mockExecutionRepo, encryptionSvc, nil, logger)

	assert.NotNil(t, service)
	assert.Equal(t, mockAgentRepo, service.agentRepo)
	assert.Equal(t, mockExecutionRepo, service.executionRepo)
	assert.NotNil(t, service.cardFetcher)
	assert.NotNil(t, service.encryptionService)
	assert.NotNil(t, service.logger)
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAgentService_CreateAgent(t *testing.T) {
	tests := []struct {
		name    string
		userID  string
		request *models.CreateAgentRequest
		setup   func(
			*repoMocks.MockAgentRepository,
			*repoMocks.MockAgentExecutionRepository,
			*MockAgentCardFetcher,
		)
		expectError bool
		errorMsg    string
	}{
		{
			name:    "Successful agent creation with card fetching",
			userID:  "user-123",
			request: createTestCreateAgentRequest(),
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				// Mock card fetcher returning a valid agent card
				testAgentCard := &models.AgentCard{
					Name:        "Test Agent",
					Description: "A test agent for unit testing",
					SupportedInterfaces: []*a2a.AgentInterface{
						{URL: "http://localhost:8000", ProtocolBinding: a2a.TransportProtocolHTTPJSON},
					},
					Version:            "1.0.0",
					Capabilities:       a2a.AgentCapabilities{},
					DefaultInputModes:  []string{"text/plain"},
					DefaultOutputModes: []string{"text/plain"},
					Skills: []a2a.AgentSkill{
						{
							ID:          "test-skill",
							Name:        "Test Skill",
							Description: "A test skill",
							Tags:        []string{"test"},
						},
					},
				}
				mockCardFetcher.On("FetchAgentCard", mock.Anything, "http://localhost:8000/.well-known/agent-card.json").
					Return(testAgentCard, nil)

				// Mock successful agent creation
				mockAgentRepo.On("Create", mock.Anything, mock.MatchedBy(func(agent *models.Agent) bool {
					return agent.UserID == "user-123" &&
						agent.Status == "active" &&
						agent.CardURL != nil &&
						*agent.CardURL == "http://localhost:8000/.well-known/agent-card.json" &&
						agent.Name == "Test Agent" &&
						agent.Description == "A test agent for unit testing"
				})).Return(nil).Run(func(args mock.Arguments) {
					agent := args.Get(1).(*models.Agent)
					agent.ID = "agent-123"
					agent.CreatedAt = time.Now()
					agent.UpdatedAt = time.Now()
				})
			},
			expectError: false,
		},
		{
			name:   "Default status to active when empty",
			userID: "user-123",
			request: &models.CreateAgentRequest{
				CardURL: "http://localhost:8000/.well-known/agent-card.json",
				Status:  "", // Empty status should default to "active"
			},
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				// Mock card fetcher returning a valid agent card
				testAgentCard := &models.AgentCard{
					Name:        "Test Agent",
					Description: "A test agent for unit testing",
					SupportedInterfaces: []*a2a.AgentInterface{
						{URL: "http://localhost:8000", ProtocolBinding: a2a.TransportProtocolHTTPJSON},
					},
					Version:            "1.0.0",
					Capabilities:       a2a.AgentCapabilities{},
					DefaultInputModes:  []string{"text/plain"},
					DefaultOutputModes: []string{"text/plain"},
					Skills:             []a2a.AgentSkill{},
				}
				mockCardFetcher.On("FetchAgentCard", mock.Anything, "http://localhost:8000/.well-known/agent-card.json").
					Return(testAgentCard, nil)

				mockAgentRepo.On("Create", mock.Anything, mock.MatchedBy(func(agent *models.Agent) bool {
					return agent.Status == "active"
				})).Return(nil).Run(func(args mock.Arguments) {
					agent := args.Get(1).(*models.Agent)
					agent.ID = "agent-123"
					agent.CreatedAt = time.Now()
					agent.UpdatedAt = time.Now()
				})
			},
			expectError: false,
		},
		{
			name:    "Repository error",
			userID:  "user-123",
			request: createTestCreateAgentRequest(),
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				// Mock card fetcher returning a valid agent card
				testAgentCard := &models.AgentCard{
					Name:        "Test Agent",
					Description: "A test agent for unit testing",
					SupportedInterfaces: []*a2a.AgentInterface{
						{URL: "http://localhost:8000", ProtocolBinding: a2a.TransportProtocolHTTPJSON},
					},
					Version:            "1.0.0",
					Capabilities:       a2a.AgentCapabilities{},
					DefaultInputModes:  []string{"text/plain"},
					DefaultOutputModes: []string{"text/plain"},
					Skills:             []a2a.AgentSkill{},
				}
				mockCardFetcher.On("FetchAgentCard", mock.Anything, "http://localhost:8000/.well-known/agent-card.json").
					Return(testAgentCard, nil)

				mockAgentRepo.On("Create", mock.Anything, mock.Anything).
					Return(fmt.Errorf("agent with name 'Test Agent' already exists for this user"))
			},
			expectError: true,
			errorMsg:    "agent with name 'Test Agent' already exists for this user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAgentRepo := repoMocks.NewMockAgentRepository(t)
			mockExecutionRepo := repoMocks.NewMockAgentExecutionRepository(t)
			mockCardFetcher := &MockAgentCardFetcher{}
			service := createTestAgentService(mockAgentRepo, mockExecutionRepo, mockCardFetcher)
			tt.setup(mockAgentRepo, mockExecutionRepo, mockCardFetcher)

			agent, err := service.CreateAgent(context.Background(), tt.userID, "team-123", tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, agent)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, agent)
				assert.Equal(t, tt.userID, agent.UserID)
				assert.NotEmpty(t, agent.Name)
				assert.NotEmpty(t, agent.Description)
			}
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAgentService_GetAgentByID(t *testing.T) {
	tests := []struct {
		name    string
		userID  string
		agentID string
		setup   func(
			*repoMocks.MockAgentRepository,
			*repoMocks.MockAgentExecutionRepository,
			*MockAgentCardFetcher,
		)
		expectError bool
		errorMsg    string
		validate    func(*testing.T, *models.Agent)
	}{
		{
			name:    "Successful agent retrieval with card sync",
			userID:  "user-123",
			agentID: "agent-123",
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				agent := createTestAgent()
				mockAgentRepo.On("GetByID", mock.Anything, "user-123", "team-123", "agent-123").
					Return(agent, nil)

				// Mock card fetcher to return updated agent card
				updatedCard := &models.AgentCard{
					Name:        "Updated Test Agent",
					Description: "Updated test agent",
					Version:     "1.1.0",
					SupportedInterfaces: []*a2a.AgentInterface{
						{URL: "http://localhost:8000", ProtocolBinding: a2a.TransportProtocolHTTPJSON},
					},
					Capabilities:       a2a.AgentCapabilities{},
					DefaultInputModes:  []string{"text/plain"},
					DefaultOutputModes: []string{"text/plain"},
				}
				mockCardFetcher.On("FetchAgentCard", mock.Anything, "http://localhost:8000/.well-known/agent-card.json").
					Return(updatedCard, nil)

				// Mock update call to save the re-fetched card
				mockAgentRepo.On("Update", mock.Anything, mock.AnythingOfType("*models.Agent")).
					Return(nil)
			},
			expectError: false,
			validate: func(t *testing.T, agent *models.Agent) {
				assert.Equal(t, "Updated Test Agent", agent.AgentCard.Name)
				assert.NotNil(t, agent.LastSyncedAt)
			},
		},
		{
			name:    "Successful agent retrieval without card URL",
			userID:  "user-123",
			agentID: "agent-456",
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				agent := createTestAgent()
				agent.ID = "agent-456"
				agent.CardURL = nil // No card URL
				mockAgentRepo.On("GetByID", mock.Anything, "user-123", "team-123", "agent-456").
					Return(agent, nil)
			},
			expectError: false,
			validate: func(t *testing.T, agent *models.Agent) {
				assert.Nil(t, agent.CardURL)
			},
		},
		{
			name:    "Agent retrieval with card fetch failure - returns existing data",
			userID:  "user-123",
			agentID: "agent-789",
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				agent := createTestAgent()
				agent.ID = "agent-789"
				mockAgentRepo.On("GetByID", mock.Anything, "user-123", "team-123", "agent-789").
					Return(agent, nil)

				// Mock card fetcher to fail
				mockCardFetcher.On("FetchAgentCard", mock.Anything, "http://localhost:8000/.well-known/agent-card.json").
					Return((*models.AgentCard)(nil), fmt.Errorf("network error"))
			},
			expectError: false,
			validate: func(t *testing.T, agent *models.Agent) {
				// Should return agent with existing card data
				assert.Equal(t, "Test Agent", agent.AgentCard.Name)
				assert.Equal(t, "agent-789", agent.ID)
			},
		},
		{
			name:    "Agent not found",
			userID:  "user-123",
			agentID: "non-existent",
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				mockAgentRepo.On("GetByID", mock.Anything, "user-123", "team-123", "non-existent").
					Return(nil, fmt.Errorf("agent not found"))
			},
			expectError: true,
			errorMsg:    "agent not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAgentRepo := repoMocks.NewMockAgentRepository(t)
			mockExecutionRepo := repoMocks.NewMockAgentExecutionRepository(t)
			mockCardFetcher := &MockAgentCardFetcher{}
			service := createTestAgentService(mockAgentRepo, mockExecutionRepo, mockCardFetcher)
			tt.setup(mockAgentRepo, mockExecutionRepo, mockCardFetcher)

			agent, err := service.GetAgentByID(context.Background(), tt.userID, "team-123", tt.agentID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, agent)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, agent)
				assert.Equal(t, tt.agentID, agent.ID)
				assert.Equal(t, tt.userID, agent.UserID)

				if tt.validate != nil {
					tt.validate(t, agent)
				}
			}
		})
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAgentService_ListAgents(t *testing.T) {
	tests := []struct {
		name    string
		userID  string
		filters AgentFilters
		setup   func(
			*repoMocks.MockAgentRepository,
			*repoMocks.MockAgentExecutionRepository,
			*MockAgentCardFetcher,
		)
		expectError bool
		expectedLen int
	}{
		{
			name:   "Successful agent list with default pagination",
			userID: "user-123",
			filters: AgentFilters{
				Page:  1,
				Limit: 10,
			},
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				agents := []models.Agent{
					{ID: "agent-1", Name: "Agent 1", UserID: "user-123"},
					{ID: "agent-2", Name: "Agent 2", UserID: "user-123"},
				}
				mockAgentRepo.On("List", mock.Anything, "user-123", repositories.AgentFilters{
					Status: "",
					Search: "",
					Page:   1,
					Limit:  10,
				}).Return(agents, 2, nil)
			},
			expectError: false,
			expectedLen: 2,
		},
		{
			name:   "List with status filter",
			userID: "user-123",
			filters: AgentFilters{
				Status: "active",
				Page:   1,
				Limit:  10,
			},
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				agents := []models.Agent{
					{ID: "agent-1", Name: "Agent 1", UserID: "user-123", Status: "active"},
				}
				mockAgentRepo.On("List", mock.Anything, "user-123", repositories.AgentFilters{
					Status: "active",
					Search: "",
					Page:   1,
					Limit:  10,
				}).Return(agents, 1, nil)
			},
			expectError: false,
			expectedLen: 1,
		},
		{
			name:   "Repository error",
			userID: "user-123",
			filters: AgentFilters{
				Page:  1,
				Limit: 10,
			},
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				mockAgentRepo.On("List", mock.Anything, "user-123", mock.Anything).
					Return(nil, 0, fmt.Errorf("database error"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAgentRepo := repoMocks.NewMockAgentRepository(t)
			mockExecutionRepo := repoMocks.NewMockAgentExecutionRepository(t)
			mockCardFetcher := &MockAgentCardFetcher{}
			service := createTestAgentService(mockAgentRepo, mockExecutionRepo, mockCardFetcher)
			tt.setup(mockAgentRepo, mockExecutionRepo, mockCardFetcher)

			response, err := service.ListAgents(context.Background(), tt.userID, tt.filters)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.Len(t, response.Agents, tt.expectedLen)
				assert.Equal(t, tt.expectedLen, response.TotalCount)
			}
		})
		//nolint:funlen // Test function requires comprehensive setup and assertions
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen,gocognit // Test function requires comprehensive setup and assertions
func TestAgentService_UpdateAgent(t *testing.T) {
	tests := []struct {
		name    string
		userID  string
		agentID string
		request *models.UpdateAgentRequest
		setup   func(
			*repoMocks.MockAgentRepository,
			*repoMocks.MockAgentExecutionRepository,
			*MockAgentCardFetcher,
		)
		expectError bool
		errorMsg    string
		validate    func(*testing.T, *models.Agent)
	}{
		{
			name:    "Successful agent update with card URL change - sets last_synced_at",
			userID:  "user-123",
			agentID: "agent-123",
			request: createTestUpdateAgentRequest(),
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				existingAgent := createTestAgent()
				mockAgentRepo.On("GetByID", mock.Anything, "user-123", "team-123", "agent-123").
					Return(existingAgent, nil)

				// Mock card fetcher for the updated card URL
				updatedAgentCard := &models.AgentCard{
					Name:        "Updated Test Agent",
					Description: "An updated test agent",
					SupportedInterfaces: []*a2a.AgentInterface{
						{URL: "http://localhost:8000", ProtocolBinding: a2a.TransportProtocolHTTPJSON},
					},
					Version:            "1.1.0",
					Capabilities:       a2a.AgentCapabilities{},
					DefaultInputModes:  []string{"text/plain"},
					DefaultOutputModes: []string{"text/plain"},
					Skills:             []a2a.AgentSkill{},
				}
				mockCardFetcher.On("FetchAgentCard", mock.Anything, "http://localhost:8000/.well-known/updated-agent-card.json").
					Return(updatedAgentCard, nil)

				mockAgentRepo.On("Update", mock.Anything, mock.MatchedBy(func(agent *models.Agent) bool {
					return agent.Status == "paused" && agent.Name == "Updated Test Agent" && agent.LastSyncedAt != nil
				})).Return(nil)
			},
			expectError: false,
			validate: func(t *testing.T, agent *models.Agent) {
				assert.Equal(t, "Updated Test Agent", agent.Name)
				assert.NotNil(t, agent.LastSyncedAt, "last_synced_at should be set when card_url is updated")
			},
		},
		{
			name:    "Update agent status only - does not set last_synced_at",
			userID:  "user-456",
			agentID: "agent-456",
			request: func() *models.UpdateAgentRequest {
				status := "paused"
				return &models.UpdateAgentRequest{
					Status: &status,
				}
			}(),
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				existingAgent := createTestAgent()
				existingAgent.ID = "agent-456"
				existingAgent.UserID = "user-456"
				existingAgent.TeamID = "team-456"
				mockAgentRepo.On("GetByID", mock.Anything, "user-456", "team-456", "agent-456").
					Return(existingAgent, nil)

				mockAgentRepo.On("Update", mock.Anything, mock.MatchedBy(func(agent *models.Agent) bool {
					return agent.Status == "paused"
				})).Return(nil)
			},
			expectError: false,
			validate: func(t *testing.T, agent *models.Agent) {
				assert.Equal(t, "paused", agent.Status)
			},
		},
		{
			name:    "Agent not found",
			userID:  "user-123",
			agentID: "non-existent",
			request: createTestUpdateAgentRequest(),
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				mockAgentRepo.On("GetByID", mock.Anything, "user-123", "team-123", "non-existent").
					Return(nil, fmt.Errorf("agent not found"))
			},
			expectError: true,
			errorMsg:    "agent not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAgentRepo := repoMocks.NewMockAgentRepository(t)
			mockExecutionRepo := repoMocks.NewMockAgentExecutionRepository(t)
			mockCardFetcher := &MockAgentCardFetcher{}
			service := createTestAgentService(mockAgentRepo, mockExecutionRepo, mockCardFetcher)
			tt.setup(mockAgentRepo, mockExecutionRepo, mockCardFetcher)

			// Use team-456 for user-456, team-123 for others
			teamID := "team-123"
			if tt.userID == "user-456" {
				teamID = "team-456"
			}
			agent, err := service.UpdateAgent(context.Background(), tt.userID, teamID, tt.agentID, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, agent)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, agent)
				assert.Equal(t, tt.agentID, agent.ID)
				assert.Equal(t, tt.userID, agent.UserID)

				if tt.validate != nil {
					tt.validate(t, agent)
				}
			}
			//nolint:funlen // Test function requires comprehensive setup and assertions
		})
		//nolint:funlen // Test function requires comprehensive setup and assertions
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAgentService_DeleteAgent(t *testing.T) {
	tests := []struct {
		name    string
		userID  string
		agentID string
		setup   func(
			*repoMocks.MockAgentRepository,
			*repoMocks.MockAgentExecutionRepository,
			*MockAgentCardFetcher,
		)
		expectError bool
		errorMsg    string
	}{
		{
			name:    "Successful agent deletion",
			userID:  "user-123",
			agentID: "agent-123",
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				mockAgentRepo.On("Delete", mock.Anything, "user-123", "team-123", "agent-123").
					Return(nil)
			},
			expectError: false,
		},
		{
			name:    "Agent not found",
			userID:  "user-123",
			agentID: "non-existent",
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				mockAgentRepo.On("Delete", mock.Anything, "user-123", "team-123", "non-existent").
					Return(fmt.Errorf("agent not found"))
			},
			expectError: true,
			errorMsg:    "agent not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAgentRepo := repoMocks.NewMockAgentRepository(t)
			mockExecutionRepo := repoMocks.NewMockAgentExecutionRepository(t)
			mockCardFetcher := &MockAgentCardFetcher{}
			service := createTestAgentService(mockAgentRepo, mockExecutionRepo, mockCardFetcher)
			tt.setup(mockAgentRepo, mockExecutionRepo, mockCardFetcher)

			err := service.DeleteAgent(context.Background(), tt.userID, "team-123", tt.agentID)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				//nolint:funlen // Test function requires comprehensive setup and assertions
			}
			//nolint:funlen // Test function requires comprehensive setup and assertions
		})
		//nolint:funlen // Test function requires comprehensive setup and assertions
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAgentService_GetAgentStats(t *testing.T) {
	tests := []struct {
		name   string
		userID string
		setup  func(
			*repoMocks.MockAgentRepository,
			*repoMocks.MockAgentExecutionRepository,
			*MockAgentCardFetcher,
		)
		expectError bool
	}{
		{
			name:   "Successful stats retrieval",
			userID: "user-123",
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				stats := &models.AgentStatsResponse{
					TotalAgents:    5,
					ActiveAgents:   3,
					PausedAgents:   1,
					ErrorAgents:    1,
					TotalRuns:      100,
					AvgSuccessRate: 85.5,
				}
				mockAgentRepo.On("GetStats", mock.Anything, "user-123", "test-team-id").
					Return(stats, nil)
			},
			expectError: false,
		},
		{
			name:   "Repository error",
			userID: "user-123",
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				mockAgentRepo.On("GetStats", mock.Anything, "user-123", "test-team-id").
					Return(nil, fmt.Errorf("database error"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAgentRepo := repoMocks.NewMockAgentRepository(t)
			mockExecutionRepo := repoMocks.NewMockAgentExecutionRepository(t)
			mockCardFetcher := &MockAgentCardFetcher{}
			service := createTestAgentService(mockAgentRepo, mockExecutionRepo, mockCardFetcher)
			tt.setup(mockAgentRepo, mockExecutionRepo, mockCardFetcher)

			stats, err := service.GetAgentStats(context.Background(), tt.userID, "test-team-id")

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, stats)
			} else {
				assert.NoError(t, err)
				//nolint:funlen // Test function requires comprehensive setup and assertions
				assert.NotNil(t, stats)
				//nolint:funlen // Test function requires comprehensive setup and assertions
			}
			//nolint:funlen // Test function requires comprehensive setup and assertions
		})
		//nolint:funlen // Test function requires comprehensive setup and assertions
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAgentService_StartExecution(t *testing.T) {
	tests := []struct {
		name    string
		userID  string
		agentID string
		request *models.CreateAgentExecutionRequest
		setup   func(
			*repoMocks.MockAgentRepository,
			*repoMocks.MockAgentExecutionRepository,
			*MockAgentCardFetcher,
		)
		expectError bool
	}{
		{
			name:    "Successful execution start",
			userID:  "user-123",
			agentID: "agent-123",
			request: createTestCreateAgentExecutionRequest(),
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				agent := createTestAgent()
				mockAgentRepo.On("GetByID", mock.Anything, "user-123", "team-123", "agent-123").
					Return(agent, nil)
				mockExecutionRepo.On("Create", mock.Anything, mock.MatchedBy(func(execution *models.AgentExecution) bool {
					return execution.AgentID == "agent-123" && execution.UserID == "user-123" && execution.Status == "running"
				})).Return(nil).Run(func(args mock.Arguments) {
					execution := args.Get(1).(*models.AgentExecution)
					execution.ID = "execution-123"
					execution.StartedAt = time.Now()
				})
			},
			expectError: false,
		},
		{
			name:    "Agent not found",
			userID:  "user-123",
			agentID: "non-existent",
			request: createTestCreateAgentExecutionRequest(),
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				mockAgentRepo.On("GetByID", mock.Anything, "user-123", "team-123", "non-existent").
					Return(nil, fmt.Errorf("agent not found"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAgentRepo := repoMocks.NewMockAgentRepository(t)
			mockExecutionRepo := repoMocks.NewMockAgentExecutionRepository(t)
			mockCardFetcher := &MockAgentCardFetcher{}
			service := createTestAgentService(mockAgentRepo, mockExecutionRepo, mockCardFetcher)
			tt.setup(mockAgentRepo, mockExecutionRepo, mockCardFetcher)

			execution, err := service.StartExecution(context.Background(), tt.userID, "team-123", tt.agentID, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, execution)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, execution)
				assert.Equal(t, tt.agentID, execution.AgentID)
				//nolint:funlen // Test function requires comprehensive setup and assertions
				assert.Equal(t, tt.userID, execution.UserID)
				//nolint:funlen // Test function requires comprehensive setup and assertions
				assert.Equal(t, "running", execution.Status)
				//nolint:funlen // Test function requires comprehensive setup and assertions
			}
			//nolint:funlen // Test function requires comprehensive setup and assertions
		})
		//nolint:funlen // Test function requires comprehensive setup and assertions
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAgentService_CompleteExecution(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		executionID string
		request     *models.UpdateAgentExecutionRequest
		setup       func(
			*repoMocks.MockAgentRepository,
			*repoMocks.MockAgentExecutionRepository,
			*MockAgentCardFetcher,
		)
		expectError bool
	}{
		{
			name:        "Successful execution completion",
			userID:      "user-123",
			executionID: "execution-123",
			request:     createTestUpdateAgentExecutionRequest(),
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				execution := createTestAgentExecution()
				execution.Status = "running" // Set initial status
				mockExecutionRepo.On("GetByID", mock.Anything, "user-123", "execution-123").
					Return(execution, nil)
				mockExecutionRepo.On("Update", mock.Anything, mock.MatchedBy(func(exec *models.AgentExecution) bool {
					return exec.Status == "success" && exec.EndedAt != nil && exec.Duration != nil
				})).Return(nil)
				mockAgentRepo.On("UpdateExecutionStats", mock.Anything, "agent-123", true, mock.AnythingOfType("int")).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:        "Execution not found",
			userID:      "user-123",
			executionID: "non-existent",
			request:     createTestUpdateAgentExecutionRequest(),
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				mockExecutionRepo.On("GetByID", mock.Anything, "user-123", "non-existent").
					Return(nil, fmt.Errorf("agent execution not found"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAgentRepo := repoMocks.NewMockAgentRepository(t)
			mockExecutionRepo := repoMocks.NewMockAgentExecutionRepository(t)
			mockCardFetcher := &MockAgentCardFetcher{}
			service := createTestAgentService(mockAgentRepo, mockExecutionRepo, mockCardFetcher)
			tt.setup(mockAgentRepo, mockExecutionRepo, mockCardFetcher)

			execution, err := service.CompleteExecution(context.Background(), tt.userID, tt.executionID, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, execution)
			} else {
				//nolint:funlen // Test function requires comprehensive setup and assertions
				assert.NoError(t, err)
				//nolint:funlen // Test function requires comprehensive setup and assertions
				assert.NotNil(t, execution)
				//nolint:funlen // Test function requires comprehensive setup and assertions
				assert.Equal(t, "success", execution.Status)
				//nolint:funlen // Test function requires comprehensive setup and assertions
			}
			//nolint:funlen // Test function requires comprehensive setup and assertions
		})
		//nolint:funlen // Test function requires comprehensive setup and assertions
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAgentService_GetExecution(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		executionID string
		setup       func(
			*repoMocks.MockAgentRepository,
			*repoMocks.MockAgentExecutionRepository,
			*MockAgentCardFetcher,
		)
		expectError bool
	}{
		{
			name:        "Successful execution retrieval",
			userID:      "user-123",
			executionID: "execution-123",
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				execution := createTestAgentExecution()
				mockExecutionRepo.On("GetByID", mock.Anything, "user-123", "execution-123").
					Return(execution, nil)
			},
			expectError: false,
		},
		{
			name:        "Execution not found",
			userID:      "user-123",
			executionID: "non-existent",
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				mockExecutionRepo.On("GetByID", mock.Anything, "user-123", "non-existent").
					Return(nil, fmt.Errorf("agent execution not found"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAgentRepo := repoMocks.NewMockAgentRepository(t)
			mockExecutionRepo := repoMocks.NewMockAgentExecutionRepository(t)
			mockCardFetcher := &MockAgentCardFetcher{}
			service := createTestAgentService(mockAgentRepo, mockExecutionRepo, mockCardFetcher)
			tt.setup(mockAgentRepo, mockExecutionRepo, mockCardFetcher)

			execution, err := service.GetExecution(context.Background(), tt.userID, tt.executionID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, execution)
			} else {
				//nolint:funlen // Test function requires comprehensive setup and assertions
				assert.NoError(t, err)
				//nolint:funlen // Test function requires comprehensive setup and assertions
				assert.NotNil(t, execution)
				//nolint:funlen // Test function requires comprehensive setup and assertions
				assert.Equal(t, tt.executionID, execution.ID)
				//nolint:funlen // Test function requires comprehensive setup and assertions
				assert.Equal(t, tt.userID, execution.UserID)
				//nolint:funlen // Test function requires comprehensive setup and assertions
			}
			//nolint:funlen // Test function requires comprehensive setup and assertions
		})
		//nolint:funlen // Test function requires comprehensive setup and assertions
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAgentService_ListExecutions(t *testing.T) {
	tests := []struct {
		name    string
		userID  string
		filters AgentExecutionFilters
		setup   func(
			*repoMocks.MockAgentRepository,
			*repoMocks.MockAgentExecutionRepository,
			*MockAgentCardFetcher,
		)
		expectError bool
		expectedLen int
	}{
		{
			name:   "Successful execution list",
			userID: "user-123",
			filters: AgentExecutionFilters{
				Page:  1,
				Limit: 10,
			},
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				executions := []models.AgentExecution{
					{ID: "execution-1", UserID: "user-123"},
					{ID: "execution-2", UserID: "user-123"},
				}
				mockExecutionRepo.On("List", mock.Anything, "user-123", repositories.AgentExecutionFilters{
					Page:  1,
					Limit: 10,
				}).Return(executions, 2, nil)
			},
			expectError: false,
			expectedLen: 2,
		},
		{
			name:   "Repository error",
			userID: "user-123",
			filters: AgentExecutionFilters{
				Page:  1,
				Limit: 10,
			},
			setup: func(
				mockAgentRepo *repoMocks.MockAgentRepository,
				mockExecutionRepo *repoMocks.MockAgentExecutionRepository,
				mockCardFetcher *MockAgentCardFetcher,
			) {
				mockExecutionRepo.On("List", mock.Anything, "user-123", mock.Anything).
					Return(nil, 0, fmt.Errorf("database error"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAgentRepo := repoMocks.NewMockAgentRepository(t)
			mockExecutionRepo := repoMocks.NewMockAgentExecutionRepository(t)
			mockCardFetcher := &MockAgentCardFetcher{}
			service := createTestAgentService(mockAgentRepo, mockExecutionRepo, mockCardFetcher)
			tt.setup(mockAgentRepo, mockExecutionRepo, mockCardFetcher)

			executions, totalCount, err := service.ListExecutions(context.Background(), tt.userID, tt.filters)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, executions)
				assert.Equal(t, 0, totalCount)
			} else {
				assert.NoError(t, err)
				assert.Len(t, executions, tt.expectedLen)
				assert.Equal(t, tt.expectedLen, totalCount)
			}
		})
	}
}

// Test helper to verify interface compliance
func TestAgentService_ImplementsInterface(t *testing.T) {
	mockAgentRepo := repoMocks.NewMockAgentRepository(t)
	mockExecutionRepo := repoMocks.NewMockAgentExecutionRepository(t)
	mockCardFetcher := &MockAgentCardFetcher{}
	service := createTestAgentService(mockAgentRepo, mockExecutionRepo, mockCardFetcher)

	// Verify that AgentService implements AgentServiceInterface
	var _ AgentServiceInterface = service
}
