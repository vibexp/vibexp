package services

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

const (
	// cardStalenessThreshold is how old an agent card may be before an
	// out-of-band refresh is triggered (never on the read path).
	cardStalenessThreshold = 24 * time.Hour
	// cardRefreshTimeout bounds a single background staleness refresh.
	cardRefreshTimeout = 30 * time.Second
)

type AgentService struct {
	agentRepo         repositories.AgentRepository
	executionRepo     repositories.AgentExecutionRepository
	cardFetcher       AgentCardFetcherInterface
	encryptionService EncryptionServiceInterface
	authenticator     *AgentAuthenticator
	teamService       TeamServiceInterface
	logger            *slog.Logger
	// refreshInFlight dedups concurrent background card refreshes per agent id so
	// a burst of reads for a stale agent spawns at most one refresh.
	refreshInFlight sync.Map
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
		authenticator:     NewAgentAuthenticator(encryptionService),
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
		authenticator:     NewAgentAuthenticator(encryptionService),
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

// cardFetchAuthHeaders derives the header-based authentication to attach when
// (re-)fetching the given agent's card, from its stored security schemes and
// decrypted credentials. A nil authenticator or a derivation error degrades to no
// auth headers so card discovery is never blocked by a credential problem.
func (s *AgentService) cardFetchAuthHeaders(agent *models.Agent) map[string]string {
	if s.authenticator == nil {
		return nil
	}
	headers, err := s.authenticator.AuthHeaders(agent)
	if err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "cardFetchAuthHeaders",
			"agent_id", agent.ID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to derive card-fetch auth headers; fetching without authentication")
		return nil
	}
	return headers
}

func (s *AgentService) CreateAgent(
	ctx context.Context, userID, teamID string, req *models.CreateAgentRequest,
) (*models.Agent, error) {
	// Team ID is already validated by middleware, use it directly

	if req.Status == "" {
		req.Status = "active"
	}

	// No stored card/credentials exist yet at create time, so there are no header
	// schemes to derive; the initial discovery fetch is unauthenticated. Cards
	// behind header auth are picked up on the re-fetch in GetAgentByID/UpdateAgent.
	agentCard, err := s.cardFetcher.FetchAgentCard(ctx, req.CardURL, nil)
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

// GetAgentByID returns the agent straight from the database — it performs no
// outbound card fetch and no write on the read path (that coupled read latency
// and availability to the remote agent, #166). Card freshness is maintained by
// create/update and by an out-of-band staleness refresh triggered here only when
// the stored card is older than cardStalenessThreshold; that refresh runs in the
// background and never blocks or fails this read.
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

	s.maybeRefreshStaleCard(agent)
	return agent, nil
}

// isCardStale reports whether an agent's stored card is old enough (older than
// cardStalenessThreshold) to warrant a background refresh. An agent with no card
// URL is never stale; one that has never been synced is always stale.
func isCardStale(agent *models.Agent) bool {
	if agent.CardURL == nil || *agent.CardURL == "" {
		return false
	}
	if agent.LastSyncedAt == nil {
		return true
	}
	return time.Since(*agent.LastSyncedAt) >= cardStalenessThreshold
}

// maybeRefreshStaleCard triggers an out-of-band card refresh when the stored
// card is stale. It never blocks the caller: the staleness check is a cheap
// timestamp comparison, and the actual fetch+persist runs in a detached
// goroutine, deduped per agent id. Fresh agents (and agents with no card URL)
// do nothing here — so the read path stays free of outbound calls and writes.
func (s *AgentService) maybeRefreshStaleCard(agent *models.Agent) {
	if !isCardStale(agent) {
		return
	}
	if _, inFlight := s.refreshInFlight.LoadOrStore(agent.ID, struct{}{}); inFlight {
		return
	}
	// Snapshot the fields the refresh needs so the background goroutine never
	// races the pointer returned to the caller.
	snapshot := *agent
	go s.refreshCardInBackground(&snapshot)
}

// refreshCardInBackground fetches the agent's card and persists the refreshed
// card + last_synced_at. It is best-effort: any failure is logged and dropped,
// never surfaced to the read that triggered it. It intentionally does NOT
// overwrite name/description (those change only via explicit create/update).
func (s *AgentService) refreshCardInBackground(agent *models.Agent) {
	defer s.refreshInFlight.Delete(agent.ID)

	ctx, cancel := context.WithTimeout(context.Background(), cardRefreshTimeout)
	defer cancel()

	agentCard, err := s.cardFetcher.FetchAgentCard(ctx, *agent.CardURL, s.cardFetchAuthHeaders(agent))
	if err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "refreshCardInBackground",
			"agent_id", agent.ID,
			"card_url", *agent.CardURL,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Background stale-card refresh failed; keeping existing card")
		return
	}

	agent.AgentCard = agentCard
	now := time.Now()
	agent.LastSyncedAt = &now

	if err := s.agentRepo.Update(ctx, agent); err != nil {
		s.logger.With(
			"service", "agent-service",
			"method", "refreshCardInBackground",
			"agent_id", agent.ID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Background stale-card refresh: failed to persist refreshed card")
		return
	}

	s.logger.With(
		"service", "agent-service",
		"method", "refreshCardInBackground",
		"agent_id", agent.ID,
		"card_url", *agent.CardURL,
	).Info("Refreshed stale agent card out of band")
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
		// Fetch new agent card, applying the agent's stored header credentials so a
		// card behind header auth can be re-discovered.
		agentCard, err := s.cardFetcher.FetchAgentCard(ctx, *req.CardURL, s.cardFetchAuthHeaders(agent))
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
