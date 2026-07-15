package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	repoMocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// MockTeamService is a simple mock for testing team validation
type MockTeamService struct {
	mock.Mock
}

func (m *MockTeamService) IsUserMemberOfTeam(ctx context.Context, userID, teamID string) (bool, error) {
	args := m.Called(ctx, userID, teamID)
	return args.Bool(0), args.Error(1)
}

func (m *MockTeamService) GetUserDefaultTeam(ctx context.Context, userID string) (*models.Team, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Team), args.Error(1)
}

func (m *MockTeamService) CreateTeam(
	ctx context.Context, userID string, req *models.CreateTeamRequest,
) (*models.Team, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Team), args.Error(1)
}

func (m *MockTeamService) GetTeam(ctx context.Context, userID, teamID string) (*models.Team, error) {
	args := m.Called(ctx, userID, teamID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Team), args.Error(1)
}

func (m *MockTeamService) UpdateTeam(
	ctx context.Context, userID, teamID string, req *models.UpdateTeamRequest,
) (*models.Team, error) {
	args := m.Called(ctx, userID, teamID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Team), args.Error(1)
}

func (m *MockTeamService) DeleteTeam(ctx context.Context, userID, teamID string) error {
	args := m.Called(ctx, userID, teamID)
	return args.Error(0)
}

func (m *MockTeamService) ListTeams(
	ctx context.Context, userID string, page, pageSize int,
) (*models.TeamListResponse, error) {
	args := m.Called(ctx, userID, page, pageSize)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TeamListResponse), args.Error(1)
}

func (m *MockTeamService) CreateDefaultTeam(ctx context.Context, userID string) (*models.Team, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Team), args.Error(1)
}

func (m *MockTeamService) GetTeamByOwnerID(ctx context.Context, ownerID string) (*models.Team, error) {
	args := m.Called(ctx, ownerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Team), args.Error(1)
}

func (m *MockTeamService) GetTeamStats(ctx context.Context, teamID string) (*models.TeamStatsResponse, error) {
	args := m.Called(ctx, teamID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TeamStatsResponse), args.Error(1)
}

func (m *MockTeamService) GetTeamResourceCreationMetrics(
	ctx context.Context, teamID string, since time.Time,
) ([]models.TeamResourceCreationCount, error) {
	args := m.Called(ctx, teamID, since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.TeamResourceCreationCount), args.Error(1)
}

func (m *MockTeamService) GetTeamFeedCreationMetrics(
	ctx context.Context, teamID string, since time.Time,
) ([]models.TeamFeedCreationCount, error) {
	args := m.Called(ctx, teamID, since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.TeamFeedCreationCount), args.Error(1)
}

func (m *MockTeamService) GetTeamMembers(
	ctx context.Context, userID, teamID string, page, pageSize int,
) (*models.TeamMembersListResponse, error) {
	args := m.Called(ctx, userID, teamID, page, pageSize)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TeamMembersListResponse), args.Error(1)
}

func (m *MockTeamService) RemoveTeamMember(ctx context.Context, userID, teamID, memberUserID string) error {
	args := m.Called(ctx, userID, teamID, memberUserID)
	return args.Error(0)
}

func (m *MockTeamService) UpdateMemberRole(
	ctx context.Context, userID, teamID, targetUserID string, newRole models.TeamMemberRole,
) (*models.TeamMemberDetail, error) {
	args := m.Called(ctx, userID, teamID, targetUserID, newRole)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TeamMemberDetail), args.Error(1)
}

func (m *MockTeamService) TransferOwnership(
	ctx context.Context, userID, teamID, newOwnerID string,
) (*models.Team, error) {
	args := m.Called(ctx, userID, teamID, newOwnerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Team), args.Error(1)
}

//nolint:funlen // Test requires comprehensive test cases
func TestAgentService_ValidateAndResolveTeamID(t *testing.T) {
	ctx := context.Background()
	userID := "user-123"
	defaultTeamID := "team-default"
	requestedTeamID := "team-requested"

	tests := []struct {
		name            string
		requestedTeamID *string
		setupMocks      func(*MockTeamService)
		expectedTeamID  string
		expectError     bool
		errorContains   string
	}{
		{
			name:            "No team_id provided - returns default team ID",
			requestedTeamID: nil,
			setupMocks:      func(m *MockTeamService) {},
			expectedTeamID:  defaultTeamID,
			expectError:     false,
		},
		{
			name:            "Empty team_id provided - returns default team ID",
			requestedTeamID: agentTeamStringPtr(""),
			setupMocks:      func(m *MockTeamService) {},
			expectedTeamID:  defaultTeamID,
			expectError:     false,
		},
		{
			name:            "Valid team_id, user is member - returns requested team ID",
			requestedTeamID: &requestedTeamID,
			setupMocks: func(m *MockTeamService) {
				m.On("IsUserMemberOfTeam", ctx, userID, requestedTeamID).Return(true, nil)
			},
			expectedTeamID: requestedTeamID,
			expectError:    false,
		},
		{
			name:            "Valid team_id, user not member - returns error",
			requestedTeamID: &requestedTeamID,
			setupMocks: func(m *MockTeamService) {
				m.On("IsUserMemberOfTeam", ctx, userID, requestedTeamID).Return(false, nil)
			},
			expectError:   true,
			errorContains: "not a member of the specified team",
		},
		{
			name:            "Team membership check fails - returns error",
			requestedTeamID: &requestedTeamID,
			setupMocks: func(m *MockTeamService) {
				m.On("IsUserMemberOfTeam", ctx, userID, requestedTeamID).
					Return(false, errors.New("database error"))
			},
			expectError:   true,
			errorContains: "failed to validate team membership",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTeamService := &MockTeamService{}
			tt.setupMocks(mockTeamService)

			mockAgentRepo := repoMocks.NewMockAgentRepository(t)
			mockExecutionRepo := repoMocks.NewMockAgentExecutionRepository(t)
			encryptionSvc, err := NewEncryptionService("test-encryption-key-32-bytes1234")
			require.NoError(t, err)
			logger, _ := logtest.New()

			service := NewAgentService(mockAgentRepo, mockExecutionRepo, encryptionSvc, mockTeamService, allowAllAuthz{}, nil, logger)

			teamID, err := service.validateAndResolveTeamID(ctx, userID, defaultTeamID, tt.requestedTeamID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedTeamID, teamID)
			}

			mockTeamService.AssertExpectations(t)
		})
	}
}

func TestAgentService_ValidateTeamReassignment(t *testing.T) {
	agentID := "agent-123"
	currentTeamID := "team-current"
	differentTeamID := "team-different"

	tests := []struct {
		name            string
		requestedTeamID *string
		expectError     bool
		errorContains   string
	}{
		{
			name:            "No team_id in update - no error",
			requestedTeamID: nil,
			expectError:     false,
		},
		{
			name:            "Same team_id in update - no error",
			requestedTeamID: &currentTeamID,
			expectError:     false,
		},
		{
			name:            "Different team_id in update - returns error",
			requestedTeamID: &differentTeamID,
			expectError:     true,
			errorContains:   "cannot be moved between teams",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAgentRepo := repoMocks.NewMockAgentRepository(t)
			mockExecutionRepo := repoMocks.NewMockAgentExecutionRepository(t)
			encryptionSvc, err := NewEncryptionService("test-encryption-key-32-bytes1234")
			require.NoError(t, err)
			logger, _ := logtest.New()

			service := NewAgentService(mockAgentRepo, mockExecutionRepo, encryptionSvc, nil, allowAllAuthz{}, nil, logger)

			err = service.validateTeamReassignment(tt.requestedTeamID, currentTeamID, agentID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

//nolint:funlen // Test requires comprehensive test cases
func TestAgentService_CreateAgent_WithTeamID(t *testing.T) {
	ctx := context.Background()
	userID := "user-123"
	defaultTeamID := "team-default"
	requestedTeamID := "team-requested"

	tests := []struct {
		name          string
		request       *models.CreateAgentRequest
		setupMocks    func(*MockTeamService, *repoMocks.MockAgentRepository, *MockAgentCardFetcher)
		expectError   bool
		errorContains string
		expectedTeam  string
	}{
		{
			name: "Create with team_id passed to service - success",
			request: &models.CreateAgentRequest{
				CardURL: "http://localhost:8000/.well-known/agent-card.json",
				Status:  "active",
			},
			setupMocks: func(ts *MockTeamService, ar *repoMocks.MockAgentRepository, cf *MockAgentCardFetcher) {
				// Team validation is now done by middleware, not service
				cf.On("FetchAgentCard", ctx, "http://localhost:8000/.well-known/agent-card.json", mock.Anything).
					Return(&models.AgentCard{
						Name:        "Test Agent",
						Description: "Test Description",
					}, nil)
				ar.On("Create", ctx, mock.AnythingOfType("*models.Agent")).Return(nil)
			},
			expectError:  false,
			expectedTeam: requestedTeamID,
		},
		{
			name: "Create with default team_id - success",
			request: &models.CreateAgentRequest{
				CardURL: "http://localhost:8000/.well-known/agent-card.json",
				Status:  "active",
			},
			setupMocks: func(ts *MockTeamService, ar *repoMocks.MockAgentRepository, cf *MockAgentCardFetcher) {
				cf.On("FetchAgentCard", ctx, "http://localhost:8000/.well-known/agent-card.json", mock.Anything).
					Return(&models.AgentCard{
						Name:        "Test Agent",
						Description: "Test Description",
					}, nil)
				ar.On("Create", ctx, mock.AnythingOfType("*models.Agent")).Return(nil)
			},
			expectError:  false,
			expectedTeam: defaultTeamID,
		},
		// Removed "user not member" test - team validation is now done by middleware, not service
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTeamService := &MockTeamService{}
			mockAgentRepo := repoMocks.NewMockAgentRepository(t)
			mockExecutionRepo := repoMocks.NewMockAgentExecutionRepository(t)
			mockCardFetcher := &MockAgentCardFetcher{}
			encryptionSvc, err := NewEncryptionService("test-encryption-key-32-bytes1234")
			require.NoError(t, err)
			logger, _ := logtest.New()

			tt.setupMocks(mockTeamService, mockAgentRepo, mockCardFetcher)

			service := NewAgentServiceWithCardFetcher(
				mockAgentRepo, mockExecutionRepo, mockCardFetcher, encryptionSvc, mockTeamService, allowAllAuthz{}, logger,
			)

			// Use the expected team ID based on test case
			teamIDToUse := tt.expectedTeam
			agent, err := service.CreateAgent(ctx, userID, teamIDToUse, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				assert.Nil(t, agent)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, agent)
				assert.Equal(t, tt.expectedTeam, agent.TeamID)
			}

			mockTeamService.AssertExpectations(t)
			mockAgentRepo.AssertExpectations(t)
			mockCardFetcher.AssertExpectations(t)
		})
	}
}

//nolint:funlen // Test requires comprehensive test cases
func TestAgentService_UpdateAgent_TeamReassignment(t *testing.T) {
	ctx := context.Background()
	userID := "user-123"
	agentID := "agent-123"
	currentTeamID := "team-current"

	tests := []struct {
		name          string
		request       *models.UpdateAgentRequest
		setupMocks    func(*repoMocks.MockAgentRepository)
		expectError   bool
		errorContains string
	}{
		{
			name: "Update without team_id - success",
			request: &models.UpdateAgentRequest{
				Status: agentTeamStringPtr("paused"),
			},
			setupMocks: func(ar *repoMocks.MockAgentRepository) {
				ar.On("GetByID", ctx, userID, currentTeamID, agentID).Return(&models.Agent{
					ID:     agentID,
					UserID: userID,
					TeamID: currentTeamID,
					Name:   "Test Agent",
				}, nil)
				ar.On("Update", ctx, mock.AnythingOfType("*models.Agent")).Return(nil)
			},
			expectError: false,
		},
		{
			name: "Update with same team_id - success",
			request: &models.UpdateAgentRequest{
				Status: agentTeamStringPtr("paused"),
			},
			setupMocks: func(ar *repoMocks.MockAgentRepository) {
				ar.On("GetByID", ctx, userID, currentTeamID, agentID).Return(&models.Agent{
					ID:     agentID,
					UserID: userID,
					TeamID: currentTeamID,
					Name:   "Test Agent",
				}, nil)
				ar.On("Update", ctx, mock.AnythingOfType("*models.Agent")).Return(nil)
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAgentRepo := repoMocks.NewMockAgentRepository(t)
			mockExecutionRepo := repoMocks.NewMockAgentExecutionRepository(t)
			encryptionSvc, err := NewEncryptionService("test-encryption-key-32-bytes1234")
			require.NoError(t, err)
			logger, _ := logtest.New()

			tt.setupMocks(mockAgentRepo)

			service := NewAgentService(mockAgentRepo, mockExecutionRepo, encryptionSvc, nil, allowAllAuthz{}, nil, logger)

			agent, err := service.UpdateAgent(ctx, userID, currentTeamID, agentID, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				assert.Nil(t, agent)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, agent)
			}

			mockAgentRepo.AssertExpectations(t)
		})
	}
}

func agentTeamStringPtr(s string) *string {
	return &s
}
