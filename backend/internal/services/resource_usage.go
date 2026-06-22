package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// ResourceUsageServiceInterface defines methods for resource usage service
type ResourceUsageServiceInterface interface {
	CheckResourceLimit(ctx context.Context, userID, resourceType string) (bool, error)
	GetResourceUsage(ctx context.Context, userID string) (*models.ResourceUsageResponse, error)
}

// ResourceUsageService implements the resource usage service
type ResourceUsageService struct {
	userRepo             repositories.UserRepository
	promptRepo           repositories.PromptRepository
	artifactRepo         repositories.ArtifactRepository
	memoryRepo           repositories.MemoryRepository
	agentRepo            repositories.AgentRepository
	agentExecRepo        repositories.AgentExecutionRepository
	claudeCodeRepo       repositories.ClaudeCodeHooksRepository
	cursorIDERepo        repositories.CursorIDEHooksRepository
	specLibraryRepo      repositories.BlueprintRepository
	teamRepo             repositories.TeamRepository
	teamMemberRepo       repositories.TeamMemberRepository
	teamSubscriptionRepo repositories.TeamSubscriptionRepository
	feedRepo             repositories.FeedRepository
	feedItemRepo         repositories.FeedItemRepository
	feedItemReplyRepo    repositories.FeedItemReplyRepository
	logger               *slog.Logger
}

// NewResourceUsageService creates a new resource usage service
func NewResourceUsageService(
	userRepo repositories.UserRepository,
	promptRepo repositories.PromptRepository,
	artifactRepo repositories.ArtifactRepository,
	memoryRepo repositories.MemoryRepository,
	agentRepo repositories.AgentRepository,
	agentExecRepo repositories.AgentExecutionRepository,
	claudeCodeRepo repositories.ClaudeCodeHooksRepository,
	cursorIDERepo repositories.CursorIDEHooksRepository,
	specLibraryRepo repositories.BlueprintRepository,
	teamRepo repositories.TeamRepository,
	teamMemberRepo repositories.TeamMemberRepository,
	teamSubscriptionRepo repositories.TeamSubscriptionRepository,
	feedRepo repositories.FeedRepository,
	feedItemRepo repositories.FeedItemRepository,
	feedItemReplyRepo repositories.FeedItemReplyRepository,
	logger *slog.Logger,
) *ResourceUsageService {
	return &ResourceUsageService{
		userRepo:             userRepo,
		promptRepo:           promptRepo,
		artifactRepo:         artifactRepo,
		memoryRepo:           memoryRepo,
		agentRepo:            agentRepo,
		agentExecRepo:        agentExecRepo,
		claudeCodeRepo:       claudeCodeRepo,
		cursorIDERepo:        cursorIDERepo,
		specLibraryRepo:      specLibraryRepo,
		teamRepo:             teamRepo,
		teamMemberRepo:       teamMemberRepo,
		teamSubscriptionRepo: teamSubscriptionRepo,
		feedRepo:             feedRepo,
		feedItemRepo:         feedItemRepo,
		feedItemReplyRepo:    feedItemReplyRepo,
		logger:               logger,
	}
}

// CheckResourceLimit reports whether the user may create another resource of the
// given type. The open-source build has no paid tiers or quotas, so every
// resource type is unlimited and this always allows the operation.
func (s *ResourceUsageService) CheckResourceLimit(_ context.Context, _, _ string) (bool, error) {
	return true, nil
}

// getResourceCount gets the count of a specific resource type for a user
// countPrompts counts all prompts (both draft and published)
func (s *ResourceUsageService) countPrompts(ctx context.Context, userID string) (int, error) {
	draftCount, err := s.promptRepo.CountByStatus(ctx, userID, "draft")
	if err != nil {
		return 0, fmt.Errorf("failed to count draft prompts: %w", err)
	}

	publishedCount, err := s.promptRepo.CountByStatus(ctx, userID, "published")
	if err != nil {
		return 0, fmt.Errorf("failed to count published prompts: %w", err)
	}

	return draftCount + publishedCount, nil
}

// countArtifacts counts all artifacts for a user across all teams
func (s *ResourceUsageService) countArtifacts(ctx context.Context, userID string) (int, error) {
	count, err := s.artifactRepo.CountAll(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to count artifacts: %w", err)
	}
	return count, nil
}

// countMemories counts all memories for a user
func (s *ResourceUsageService) countMemories(ctx context.Context, userID string) (int, error) {
	count, err := s.memoryRepo.CountAll(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to count memories: %w", err)
	}
	return count, nil
}

// countSpecLibraries counts all blueprints for a user
func (s *ResourceUsageService) countSpecLibraries(ctx context.Context, userID string) (int, error) {
	stats, err := s.specLibraryRepo.GetStats(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get blueprint stats: %w", err)
	}
	return stats.TotalBlueprints, nil
}

// countAgents counts all agents for a user across all teams
func (s *ResourceUsageService) countAgents(ctx context.Context, userID string) (int, error) {
	// Pass empty teamID to count agents across all teams
	stats, err := s.agentRepo.GetStats(ctx, userID, "")
	if err != nil {
		return 0, fmt.Errorf("failed to get agent stats: %w", err)
	}
	return stats.TotalAgents, nil
}

// countAgentConversations counts all agent conversations using pagination
func (s *ResourceUsageService) countAgentConversations(ctx context.Context, userID string) (int, error) {
	totalCount := 0
	page := 1
	limit := 100

	for {
		conversations, _, err := s.agentExecRepo.ListConversations(ctx, userID, "", page, limit)
		if err != nil {
			return 0, fmt.Errorf("failed to list agent conversations: %w", err)
		}

		totalCount += len(conversations)

		if len(conversations) < limit {
			break
		}
		page++
	}

	return totalCount, nil
}

// countConnectedAITools counts unique AI tools with at least one session
func (s *ResourceUsageService) countConnectedAITools(ctx context.Context, userID string) (int, error) {
	claudeCount, cursorCount, err := s.getAIToolSessionCounts(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get AI tool session counts: %w", err)
	}

	count := 0
	if claudeCount > 0 {
		count++
	}
	if cursorCount > 0 {
		count++
	}
	return count, nil
}

// countTotalAISessions counts total AI sessions across all tools
func (s *ResourceUsageService) countTotalAISessions(ctx context.Context, userID string) (int, error) {
	claudeCount, cursorCount, err := s.getAIToolSessionCounts(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get AI session counts: %w", err)
	}
	return claudeCount + cursorCount, nil
}

// countTeams counts all teams owned by a user
func (s *ResourceUsageService) countTeams(ctx context.Context, userID string) (int, error) {
	return s.teamRepo.CountByOwnerID(ctx, userID)
}

// countFeeds counts all feeds accessible to the user across all their teams
func (s *ResourceUsageService) countFeeds(ctx context.Context, userID string) (int, error) {
	count, err := s.feedRepo.CountAll(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to count feeds: %w", err)
	}
	return count, nil
}

// countFeedItems counts all feed items plus replies accessible to the user across all their teams
func (s *ResourceUsageService) countFeedItems(ctx context.Context, userID string) (int, error) {
	items, err := s.feedItemRepo.CountAll(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to count feed items: %w", err)
	}

	replies, err := s.feedItemReplyRepo.CountAll(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to count feed item replies: %w", err)
	}

	return items + replies, nil
}

func (s *ResourceUsageService) getResourceCount(ctx context.Context, userID, resourceType string) (int, error) {
	switch resourceType {
	case events.ResourceTypePrompt:
		return s.countPrompts(ctx, userID)
	case events.ResourceTypeArtifact:
		return s.countArtifacts(ctx, userID)
	case events.ResourceTypeMemory:
		return s.countMemories(ctx, userID)
	case events.ResourceTypeBlueprint:
		return s.countSpecLibraries(ctx, userID)
	case events.ResourceTypeAgent:
		return s.countAgents(ctx, userID)
	case events.ResourceTypeAgentConv:
		return s.countAgentConversations(ctx, userID)
	case events.ResourceTypeAITool:
		return s.countConnectedAITools(ctx, userID)
	case events.ResourceTypeAISession:
		return s.countTotalAISessions(ctx, userID)
	case events.ResourceTypeTeam:
		return s.countTeams(ctx, userID)
	case events.ResourceTypeFeed:
		return s.countFeeds(ctx, userID)
	case events.ResourceTypeFeedItem:
		return s.countFeedItems(ctx, userID)
	default:
		s.logger.With("resource_type", resourceType).Warn("Unknown resource type for count check")
		return 0, nil
	}
}

// getAIToolSessionCounts gets session counts for both Claude Code and Cursor IDE using efficient COUNT queries
// Returns (claudeCodeCount, cursorIDECount, error)
func (s *ResourceUsageService) getAIToolSessionCounts(ctx context.Context, userID string) (int, int, error) {
	// Use CountUniqueSessions for efficient counting without pagination overhead
	claudeCount, err := s.claudeCodeRepo.CountUniqueSessions(ctx, userID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count Claude Code sessions: %w", err)
	}

	cursorCount, err := s.cursorIDERepo.CountUniqueSessions(ctx, userID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count Cursor IDE sessions: %w", err)
	}

	return claudeCount, cursorCount, nil
}

// getTeamQuotaContribution calculates total quota contribution from all team subscriptions
// It aggregates base quotas and per-seat bonuses from all active team subscriptions
// Personal workspaces and inactive subscriptions are excluded
//
//nolint:gocognit,gocyclo,funlen // Complex business logic for team quota calculation
func (s *ResourceUsageService) getTeamQuotaContribution(ctx context.Context, userID, resourceType string) (int, error) {
	// Get all team memberships for the user
	memberships, err := s.teamMemberRepo.GetByUserID(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get user team memberships: %w", err)
	}

	totalTeamQuota := 0

	// Iterate through each team membership
	for _, membership := range memberships {
		// Get team details to check if it's a personal workspace
		team, err := s.teamRepo.GetByID(ctx, membership.TeamID)
		if err != nil {
			s.logger.With("error", err).
				With(
					"user_id", userID,
					"team_id", membership.TeamID,
				).
				Warn("Failed to get team details for quota calculation")
			continue
		}

		// Skip personal workspaces (they don't contribute to team quotas)
		if team.IsPersonal {
			continue
		}

		// Get team subscription
		teamSub, err := s.teamSubscriptionRepo.GetByTeamID(ctx, membership.TeamID)
		if err != nil {
			// Team might not have a subscription yet, skip gracefully
			s.logger.With(
				"user_id", userID,
				"team_id", membership.TeamID,
			).
				Debug("Team has no subscription, skipping quota contribution")
			continue
		}

		// Only count active subscriptions (active or trialing)
		if !teamSub.IsActiveForQuotas() {
			continue
		}

		// Map team tier to plan constant
		var planName string
		switch teamSub.Tier {
		case models.TeamTierStarter:
			planName = models.PlanTeamsStarter
		case models.TeamTierProfessional:
			planName = models.PlanTeamsProfessional
		case models.TeamTierEnterprise:
			planName = models.PlanTeamsEnterprise
		default:
			s.logger.With(
				"user_id", userID,
				"team_id", membership.TeamID,
				"tier", teamSub.Tier,
			).
				Warn("Unknown team tier, skipping quota contribution")
			continue
		}

		// Calculate team quota: base + (per-seat bonus × seats_purchased)
		// Use shared quota calculation from models.GetTeamResourceQuota
		baseQuota, perSeatBonus := models.GetTeamResourceQuota(teamSub.Tier, resourceType)

		// HIGH-02: Validate no invalid negative values (except -1 unlimited)
		if baseQuota < -1 || perSeatBonus < 0 {
			s.logger.With(
				"user_id", userID,
				"team_id", membership.TeamID,
				"base_quota", baseQuota,
				"per_seat_bonus", perSeatBonus,
			).Error("Invalid negative quota values detected")
			continue
		}

		// Handle unlimited (-1) quotas before calculation
		if baseQuota == -1 {
			return -1, nil // If any team has unlimited, user gets unlimited
		}

		// CRITICAL-01: Check for integer overflow in per-seat calculation
		if teamSub.SeatCount > 0 && perSeatBonus > 0 {
			// Prevent overflow: check if multiplication would exceed max int
			const maxInt = int(^uint(0) >> 1)
			if perSeatBonus > (maxInt / teamSub.SeatCount) {
				s.logger.With(
					"user_id", userID,
					"team_id", membership.TeamID,
					"per_seat_bonus", perSeatBonus,
					"seat_count", teamSub.SeatCount,
				).Error("Integer overflow detected in per-seat quota calculation")
				return 0, fmt.Errorf("quota calculation would overflow for team %s", membership.TeamID)
			}
		}

		teamQuota := baseQuota + (perSeatBonus * teamSub.SeatCount)

		// HIGH-03: Comprehensive unlimited quota detection
		if teamQuota == -1 {
			s.logger.With(
				"user_id", userID,
				"team_id", membership.TeamID,
				"team_quota", teamQuota,
			).
				Debug("Team has unlimited quota for resource type")
			return -1, nil
		}

		// HIGH-02: Validate calculated quota is non-negative
		if teamQuota < 0 {
			s.logger.With(
				"user_id", userID,
				"team_id", membership.TeamID,
				"team_quota", teamQuota,
			).
				Error("Calculated team quota is negative")
			continue
		}

		// CRITICAL-01: Check for overflow when accumulating total
		if totalTeamQuota > 0 && teamQuota > 0 {
			const maxInt = int(^uint(0) >> 1)
			if teamQuota > (maxInt - totalTeamQuota) {
				s.logger.With(
					"user_id", userID,
					"total_team_quota", totalTeamQuota,
					"new_team_quota", teamQuota,
				).
					Error("Integer overflow detected in total team quota accumulation")
				// Return current total, skip this team to prevent overflow
				return totalTeamQuota, nil
			}
		}

		totalTeamQuota += teamQuota

		s.logger.With(
			"user_id", userID,
			"team_id", membership.TeamID,
			"resource_type", resourceType,
			"plan", planName,
			"base_quota", baseQuota,
			"per_seat_bonus", perSeatBonus,
			"seat_count", teamSub.SeatCount,
			"team_quota", teamQuota,
		).Debug("Calculated team quota contribution")
	}

	return totalTeamQuota, nil
}

// GetResourceUsage gets resource usage information for a user
//
//nolint:funlen // Multiple resource types require sequential processing
func (s *ResourceUsageService) GetResourceUsage(
	ctx context.Context, userID string,
) (*models.ResourceUsageResponse, error) {
	// Get user to determine subscription plan
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Create response
	response := &models.ResourceUsageResponse{
		UserID:    userID,
		Resources: []models.ResourceUsageItem{},
	}

	// Define resource types to check
	resourceTypes := []string{
		events.ResourceTypeAITool,
		events.ResourceTypeAISession,
		events.ResourceTypePrompt,
		events.ResourceTypeArtifact,
		events.ResourceTypeMemory,
		events.ResourceTypeBlueprint,
		events.ResourceTypeAgent,
		events.ResourceTypeAgentConv,
		events.ResourceTypeTeam,
		events.ResourceTypeFeed,
		events.ResourceTypeFeedItem,
	}

	// Get usage for each resource type
	for _, resourceType := range resourceTypes {
		count, err := s.getResourceCount(ctx, userID, resourceType)
		if err != nil {
			s.logger.With("error", err).
				With(
					"user_id", userID,
					"resource_type", resourceType,
				).
				Error("Failed to get resource count")

			// Continue with other resource types even if one fails
			count = 0
		}

		// Get individual subscription limit
		individualLimit := s.getResourceLimit(resourceType, user.SubscriptionPlan)

		// Get team quota contribution
		teamQuota, err := s.getTeamQuotaContribution(ctx, userID, resourceType)
		if err != nil {
			s.logger.With("error", err).
				With(
					"user_id", userID,
					"resource_type", resourceType,
				).
				Warn("Failed to get team quota contribution")
			teamQuota = 0
		}

		// Calculate total limit
		totalLimit := 0
		if individualLimit == -1 || teamQuota == -1 {
			totalLimit = -1
		} else {
			totalLimit = individualLimit + teamQuota
		}

		// Calculate percentage
		percentage := 0
		if totalLimit > 0 {
			percentage = int(float64(count) / float64(totalLimit) * 100)
		}

		response.Resources = append(response.Resources, models.ResourceUsageItem{
			ResourceType:    resourceType,
			Count:           count,
			Limit:           totalLimit,
			IndividualLimit: individualLimit,
			TeamQuota:       teamQuota,
			Percentage:      percentage,
		})
	}

	return response, nil
}

// getResourceLimit returns the resource limit for a subscription plan
var resourceLimits = map[string]map[string]int{
	events.ResourceTypeAITool: {
		models.PlanBasic:     2,
		models.PlanStarter:   2,
		models.PlanPro:       3,
		models.PlanPowerUser: -1, // Unlimited
	},
	events.ResourceTypeAISession: {
		models.PlanBasic:     100,
		models.PlanStarter:   500,
		models.PlanPro:       1000,
		models.PlanPowerUser: 2000,
	},
	events.ResourceTypePrompt: {
		models.PlanBasic:     100,
		models.PlanStarter:   200,
		models.PlanPro:       500,
		models.PlanPowerUser: 1000,
	},
	events.ResourceTypeArtifact: {
		models.PlanBasic:     100,
		models.PlanStarter:   200,
		models.PlanPro:       500,
		models.PlanPowerUser: 1000,
	},
	events.ResourceTypeMemory: {
		models.PlanBasic:     100,
		models.PlanStarter:   200,
		models.PlanPro:       500,
		models.PlanPowerUser: 1000,
	},
	events.ResourceTypeBlueprint: {
		models.PlanBasic:     20,
		models.PlanStarter:   100,
		models.PlanPro:       200,
		models.PlanPowerUser: 1000,
	},
	events.ResourceTypeAgent: {
		models.PlanBasic:     2,
		models.PlanStarter:   3,
		models.PlanPro:       5,
		models.PlanPowerUser: -1, // Unlimited
	},
	events.ResourceTypeAgentConv: {
		models.PlanBasic:     100,
		models.PlanStarter:   300,
		models.PlanPro:       600,
		models.PlanPowerUser: 1500,
	},
	events.ResourceTypeTeam: {
		models.PlanBasic:     2,
		models.PlanStarter:   4,
		models.PlanPro:       8,
		models.PlanPowerUser: 20,
	},
	events.ResourceTypeFeed: {
		models.PlanBasic:     1,
		models.PlanStarter:   2,
		models.PlanPro:       5,
		models.PlanPowerUser: -1, // Unlimited
	},
	events.ResourceTypeFeedItem: {
		models.PlanBasic:     100,
		models.PlanStarter:   500,
		models.PlanPro:       1000,
		models.PlanPowerUser: 10000,
	},
}

// Team quota calculations now use shared constants from models.GetTeamResourceQuota
// This ensures consistency between ResourceUsageService and TeamSubscriptionService
// See: backend-api/internal/models/team_quota_constants.go

func (s *ResourceUsageService) getResourceLimit(resourceType string, plan *string) int {
	// If no plan, use basic tier limits
	planName := models.PlanBasic
	if plan != nil {
		planName = models.NormalizePlanName(*plan)
	}

	// Look up limit in resource limits map
	if limits, ok := resourceLimits[resourceType]; ok {
		if limit, ok := limits[planName]; ok {
			return limit
		}
		// If plan not found, return basic tier limit for this resource
		if limit, ok := limits[models.PlanBasic]; ok {
			s.logger.With(
				"resource_type", resourceType,
				"plan_name", planName,
			).
				Warn("Plan not found in resource limits, falling back to basic tier")
			return limit
		}
	}

	s.logger.With("resource_type", resourceType).Warn("Unknown resource type for limit check")
	return -1 // Unlimited for unknown types
}
