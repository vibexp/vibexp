package services

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

type AgentService struct {
	agentRepo         repositories.AgentRepository
	executionRepo     repositories.AgentExecutionRepository
	cardFetcher       AgentCardFetcherInterface
	encryptionService EncryptionServiceInterface
	teamService       TeamServiceInterface
	logger            *slog.Logger
}

// Ensure AgentService implements AgentServiceInterface
var _ AgentServiceInterface = (*AgentService)(nil)

func NewAgentService(
	agentRepo repositories.AgentRepository,
	executionRepo repositories.AgentExecutionRepository,
	encryptionService EncryptionServiceInterface,
	teamService TeamServiceInterface,
	logger *slog.Logger,
) *AgentService {
	return &AgentService{
		agentRepo:         agentRepo,
		executionRepo:     executionRepo,
		cardFetcher:       NewAgentCardFetcher(),
		encryptionService: encryptionService,
		teamService:       teamService,
		logger:            logger,
	}
}

// NewAgentServiceWithCardFetcher allows injecting a custom card fetcher (useful for testing)
func NewAgentServiceWithCardFetcher(
	agentRepo repositories.AgentRepository,
	executionRepo repositories.AgentExecutionRepository,
	cardFetcher AgentCardFetcherInterface,
	encryptionService EncryptionServiceInterface,
	teamService TeamServiceInterface,
	logger *slog.Logger,
) *AgentService {
	return &AgentService{
		agentRepo:         agentRepo,
		executionRepo:     executionRepo,
		cardFetcher:       cardFetcher,
		encryptionService: encryptionService,
		teamService:       teamService,
		logger:            logger,
	}
}

type AgentFilters struct {
	Status    string
	Search    string
	TeamID    string
	SortBy    string
	SortOrder string
	Page      int
	Limit     int
}

type AgentExecutionFilters struct {
	AgentID  *string
	Status   *string
	DateFrom *string
	DateTo   *string
	Page     int
	Limit    int
}

// validateAndResolveTeamID validates team membership and returns the final team ID to use
func (s *AgentService) validateAndResolveTeamID(
	ctx context.Context, userID, defaultTeamID string, requestedTeamID *string,
) (string, error) {
	if requestedTeamID == nil || *requestedTeamID == "" {
		return defaultTeamID, nil
	}

	if s.teamService == nil {
		return *requestedTeamID, nil
	}

	isMember, err := s.teamService.IsUserMemberOfTeam(ctx, userID, *requestedTeamID)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", *requestedTeamID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to validate team membership for agent")
		return "", fmt.Errorf("failed to validate team membership")
	}

	if !isMember {
		s.logger.With(
			"user_id", userID,
			"team_id", *requestedTeamID,
		).
			Warn("User attempted to create agent in team they are not a member of")
		return "", fmt.Errorf("user is not a member of the specified team")
	}

	return *requestedTeamID, nil
}

// validateTeamReassignment checks if user is trying to move agent to different team
func (s *AgentService) validateTeamReassignment(
	requestedTeamID *string, currentTeamID, agentID string,
) error {
	if requestedTeamID != nil && *requestedTeamID != currentTeamID {
		s.logger.With(
			"agent_id", agentID,
			"existing_team", currentTeamID,
			"requested_team", *requestedTeamID,
		).
			Warn("User attempted to reassign agent to different team")
		return fmt.Errorf("agents cannot be moved between teams once created")
	}
	return nil
}

func (s *AgentService) CreateAgent(
	ctx context.Context, userID, teamID string, req *models.CreateAgentRequest,
) (*models.Agent, error) {
	// Team ID is already validated by middleware, use it directly

	if req.Status == "" {
		req.Status = "active"
	}

	agentCard, err := s.cardFetcher.FetchAgentCard(ctx, req.CardURL)
	if err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "CreateAgent",
			"user_id", userID,
			"card_url", req.CardURL,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to fetch agent card")
		return nil, err
	}

	agent := &models.Agent{
		UserID: userID, TeamID: teamID, Name: agentCard.Name, Description: agentCard.Description,
		Status: req.Status, Config: make(map[string]interface{}), CardURL: &req.CardURL, AgentCard: agentCard,
	}

	if len(req.Credentials) > 0 {
		if err := s.encryptCredentials(agent, req.Credentials); err != nil {
			s.logger.With(
				"service", "agent-service",
				"method", "CreateAgent",
				"user_id", userID,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to encrypt credentials")
			return nil, fmt.Errorf("failed to encrypt credentials: %w", err)
		}
	}

	if err := s.agentRepo.Create(ctx, agent); err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "CreateAgent",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create agent")
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	s.logger.With(
		"service", "agent-service",
		"method", "CreateAgent",
		"user_id", userID,
		"agent_id", agent.ID,
		"card_url", req.CardURL,
		"has_card", agentCard != nil,
	).Info("Agent created successfully")

	return agent, nil
}

func (s *AgentService) GetAgentByID(ctx context.Context, userID, teamID, agentID string) (*models.Agent, error) {
	agent, err := s.agentRepo.GetByID(ctx, userID, teamID, agentID)
	if err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "GetAgentByID",
			"user_id", userID,
			"team_id", teamID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get agent by ID")
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	// Re-fetch agent card if card_url exists
	if agent.CardURL != nil && *agent.CardURL != "" {
		agentCard, err := s.cardFetcher.FetchAgentCard(ctx, *agent.CardURL)
		if err != nil {
			s.logger.With(
				"service", "agent-service",
				"method", "GetAgentByID",
				"user_id", userID,
				"agent_id", agentID,
				"card_url", *agent.CardURL,
				"error", fmt.Sprintf("%+v", err),
			).Warn("Failed to re-fetch agent card, returning existing data")
			// Return agent without updating card - don't fail the request
			return agent, nil
		}

		// Update agent card and last_synced_at
		agent.AgentCard = agentCard
		now := time.Now()
		agent.LastSyncedAt = &now

		// Update in database
		if err := s.agentRepo.Update(ctx, agent); err != nil {
			s.logger.With(
				"service", "agent-service",
				"method", "GetAgentByID",
				"user_id", userID,
				"agent_id", agentID,
				"error", fmt.Sprintf("%+v", err),
			).Warn("Failed to update agent with re-fetched card")

			// Return agent anyway - we have the updated card in memory
		}

		s.logger.With(
			"service", "agent-service",
			"method", "GetAgentByID",
			"user_id", userID,
			"agent_id", agentID,
			"card_url", *agent.CardURL,
		).Info("Agent card re-fetched and updated successfully")
	}

	return agent, nil
}

func (s *AgentService) ListAgents(
	ctx context.Context, userID string, filters AgentFilters,
) (*models.AgentListResponse, error) {
	repoFilters := repositories.AgentFilters{
		Status:    filters.Status,
		Search:    filters.Search,
		TeamID:    filters.TeamID,
		SortBy:    filters.SortBy,
		SortOrder: filters.SortOrder,
		Page:      filters.Page,
		Limit:     filters.Limit,
	}

	if repoFilters.Limit <= 0 {
		repoFilters.Limit = 10
	}
	if repoFilters.Page <= 0 {
		repoFilters.Page = 1
	}

	agents, totalCount, err := s.agentRepo.List(ctx, userID, repoFilters)
	if err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "ListAgents",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list agents")
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	totalPages := int(math.Ceil(float64(totalCount) / float64(repoFilters.Limit)))

	response := &models.AgentListResponse{
		Agents:     agents,
		TotalCount: totalCount,
		Page:       repoFilters.Page,
		PerPage:    repoFilters.Limit,
		TotalPages: totalPages,
	}

	return response, nil
}

//nolint:funlen // Complex business logic for agent updates
func (s *AgentService) UpdateAgent(
	ctx context.Context, userID, teamID, agentID string, req *models.UpdateAgentRequest,
) (*models.Agent, error) {
	agent, err := s.agentRepo.GetByID(ctx, userID, teamID, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	// Note: team_id cannot be changed via update (removed from UpdateAgentRequest)
	// Team reassignment is forbidden to prevent cross-team resource moves

	// Handle agent card URL update
	if req.CardURL != nil && *req.CardURL != "" {
		// Fetch new agent card
		agentCard, err := s.cardFetcher.FetchAgentCard(ctx, *req.CardURL)
		if err != nil {
			s.logger.With(
				"service", "agent-service",
				"method", "UpdateAgent",
				"user_id", userID,
				"agent_id", agentID,
				"card_url", *req.CardURL,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to fetch agent card during update")
			// Return the detailed error message from the card fetcher directly
			return nil, err
		}

		// Update agent with card data
		agent.CardURL = req.CardURL
		agent.AgentCard = agentCard
		agent.Name = agentCard.Name
		agent.Description = agentCard.Description

		// Update last_synced_at when card is updated
		now := time.Now()
		agent.LastSyncedAt = &now
	} else {
		// Update fields if provided (only if not using card)
		if req.Name != nil {
			agent.Name = *req.Name
		}
		if req.Description != nil {
			agent.Description = *req.Description
		}
	}

	if req.Status != nil {
		agent.Status = *req.Status
	}

	// Handle credentials update if provided
	if len(req.Credentials) > 0 {
		if err := s.encryptCredentials(agent, req.Credentials); err != nil {
			s.logger.With(
				"service", "agent-service",
				"method", "UpdateAgent",
				"user_id", userID,
				"agent_id", agentID,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to encrypt credentials")
			return nil, fmt.Errorf("failed to encrypt credentials: %w", err)
		}
	}

	if err := s.agentRepo.Update(ctx, agent); err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "UpdateAgent",
			"user_id", userID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to update agent")
		return nil, fmt.Errorf("failed to update agent: %w", err)
	}

	s.logger.With(
		"service", "agent-service",
		"method", "UpdateAgent",
		"user_id", userID,
		"agent_id", agentID,
		"updated_card", req.CardURL != nil,
	).Info("Agent updated successfully")

	return agent, nil
}

func (s *AgentService) DeleteAgent(ctx context.Context, userID, teamID, agentID string) error {
	if err := s.agentRepo.Delete(ctx, userID, teamID, agentID); err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "DeleteAgent",
			"user_id", userID,
			"team_id", teamID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to delete agent")
		return fmt.Errorf("failed to delete agent: %w", err)
	}

	s.logger.With(
		"service", "agent-service",
		"method", "DeleteAgent",
		"user_id", userID,
		"team_id", teamID,
		"agent_id", agentID,
	).Info("Agent deleted successfully")

	return nil
}

func (s *AgentService) GetAgentStats(ctx context.Context, userID, teamID string) (*models.AgentStatsResponse, error) {
	stats, err := s.agentRepo.GetStats(ctx, userID, teamID)
	if err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "GetAgentStats",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get agent stats")
		return nil, fmt.Errorf("failed to get agent stats: %w", err)
	}

	return stats, nil
}

func (s *AgentService) StartExecution(
	ctx context.Context, userID, teamID, agentID string, req *models.CreateAgentExecutionRequest,
) (*models.AgentExecution, error) {
	// Verify agent exists and belongs to user
	_, err := s.agentRepo.GetByID(ctx, userID, teamID, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	execution := &models.AgentExecution{
		AgentID: agentID,
		UserID:  userID,
		Status:  "running",
		Input:   req.Input,
	}

	if execution.Input == nil {
		execution.Input = make(map[string]interface{})
	}

	if err := s.executionRepo.Create(ctx, execution); err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "StartExecution",
			"user_id", userID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create agent execution")
		return nil, fmt.Errorf("failed to start execution: %w", err)
	}

	s.logger.With(
		"service", "agent-service",
		"method", "StartExecution",
		"user_id", userID,
		"agent_id", agentID,
		"execution_id", execution.ID,
	).Info("Agent execution started successfully")

	return execution, nil
}

func (s *AgentService) CompleteExecution(
	ctx context.Context, userID, executionID string, req *models.UpdateAgentExecutionRequest,
) (*models.AgentExecution, error) {
	execution, err := s.executionRepo.GetByID(ctx, userID, executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}

	// Update execution
	execution.Status = req.Status
	if req.Error != nil {
		execution.Error = req.Error
	}

	// Set end time if execution is completed
	if req.Status == "success" || req.Status == "error" {
		now := time.Now()
		execution.EndedAt = &now
		duration := int(now.Sub(execution.StartedAt).Milliseconds())
		execution.Duration = &duration
	}

	if err := s.executionRepo.Update(ctx, execution); err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "CompleteExecution",
			"user_id", userID,
			"execution_id", executionID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to update agent execution")
		return nil, fmt.Errorf("failed to complete execution: %w", err)
	}

	// Update agent statistics if execution is completed
	if req.Status == "success" || req.Status == "error" {
		success := req.Status == "success"
		duration := 0
		if execution.Duration != nil {
			duration = *execution.Duration
		}

		if err := s.agentRepo.UpdateExecutionStats(ctx, execution.AgentID, success, duration); err != nil {
			s.logger.With(
				"service", "agent-service",
				"method", "CompleteExecution",
				"user_id", userID,
				"agent_id", execution.AgentID,
				"execution_id", executionID,
				"error", fmt.Sprintf("%+v", err),
			).Warn("Failed to update agent execution stats")
		}
	}

	s.logger.With(
		"service", "agent-service",
		"method", "CompleteExecution",
		"user_id", userID,
		"execution_id", executionID,
		"status", req.Status,
	).Info("Agent execution completed successfully")

	return execution, nil
}

func (s *AgentService) GetExecution(ctx context.Context, userID, executionID string) (*models.AgentExecution, error) {
	execution, err := s.executionRepo.GetByID(ctx, userID, executionID)
	if err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "GetExecution",
			"user_id", userID,
			"execution_id", executionID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get agent execution")
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}

	return execution, nil
}

func (s *AgentService) ListExecutions(
	ctx context.Context, userID string, filters AgentExecutionFilters,
) ([]models.AgentExecution, int, error) {
	repoFilters := repositories.AgentExecutionFilters{
		AgentID:  filters.AgentID,
		Status:   filters.Status,
		DateFrom: filters.DateFrom,
		DateTo:   filters.DateTo,
		Page:     filters.Page,
		Limit:    filters.Limit,
	}

	if repoFilters.Limit <= 0 {
		repoFilters.Limit = 10
	}
	if repoFilters.Page <= 0 {
		repoFilters.Page = 1
	}

	executions, totalCount, err := s.executionRepo.List(ctx, userID, repoFilters)
	if err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "ListExecutions",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list agent executions")
		return nil, 0, fmt.Errorf("failed to list executions: %w", err)
	}

	return executions, totalCount, nil
}

// encryptCredentials encrypts the credentials and stores them in the agent
func (s *AgentService) encryptCredentials(agent *models.Agent, credentials map[string]models.CredentialRequest) error {
	if agent.Credentials == nil {
		agentCreds := make(models.AgentCredentials)
		agent.Credentials = &agentCreds
	}

	for name, cred := range credentials {
		// Encrypt the credential value
		encrypted, err := s.encryptionService.Encrypt(cred.Value)
		if err != nil {
			return fmt.Errorf("failed to encrypt credential %s: %w", name, err)
		}

		(*agent.Credentials)[name] = models.AgentCredential{
			Type:  cred.Type,
			Value: encrypted,
		}
	}

	return nil
}

// UpdateAgentCredentials updates only the credentials for an agent
func (s *AgentService) UpdateAgentCredentials(
	ctx context.Context, userID, teamID, agentID string, req *models.UpdateAgentCredentialsRequest,
) error {
	agent, err := s.agentRepo.GetByID(ctx, userID, teamID, agentID)
	if err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "UpdateAgentCredentials",
			"user_id", userID,
			"team_id", teamID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get agent")
		return fmt.Errorf("failed to get agent: %w", err)
	}

	// Encrypt and update credentials
	if err := s.encryptCredentials(agent, req.Credentials); err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "UpdateAgentCredentials",
			"user_id", userID,
			"team_id", teamID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to encrypt credentials")
		return fmt.Errorf("failed to encrypt credentials: %w", err)
	}

	// Update agent in database
	if err := s.agentRepo.Update(ctx, agent); err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "UpdateAgentCredentials",
			"user_id", userID,
			"team_id", teamID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to update agent credentials")
		return fmt.Errorf("failed to update agent credentials: %w", err)
	}

	s.logger.With(
		"service", "agent-service",
		"method", "UpdateAgentCredentials",
		"user_id", userID,
		"agent_id", agentID,
	).Info("Agent credentials updated successfully")

	return nil
}
