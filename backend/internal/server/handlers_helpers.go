package server

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-playground/validator/v10"
)

// validate is the shared struct validator used across request handlers
// (support, search, agent, MCP). It lives here, not in any one feature's
// handler file, so removing a single feature never strands the shared singleton.
var validate = validator.New()

// getUserDefaultTeamID retrieves the user's default team ID for resource creation
// All user-created resources are linked to their default team for future team collaboration features
// This function validates that:
// 1. The user exists
// 2. The user has a default team
// 3. The user is still a member of that team
// 4. The team exists and is active
func (s *Server) getUserDefaultTeamID(ctx context.Context, userID string) (string, error) {
	user, err := s.container.AuthService().GetUserByID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	if user.DefaultTeamID == nil || *user.DefaultTeamID == "" {
		return "", fmt.Errorf("user has no default team")
	}

	teamID := *user.DefaultTeamID

	// Validate team membership
	isMember, err := s.container.TeamService().IsUserMemberOfTeam(ctx, userID, teamID)
	if err != nil {
		return "", fmt.Errorf("failed to validate team membership: %w", err)
	}

	if !isMember {
		return "", fmt.Errorf("access denied")
	}

	// Verify team exists (the team service will return error if team doesn't exist)
	_, err = s.container.TeamService().GetTeam(ctx, user.ID, teamID)
	if err != nil {
		return "", fmt.Errorf("access denied")
	}

	return teamID, nil
}

// validateTeamAccess validates that a user has access to a specific team
// This function should be used for single-resource operations where team_id is provided
// Returns error with generic message to prevent team enumeration attacks
func (s *Server) validateTeamAccess(ctx context.Context, userID, teamID string) error {
	// Validate team membership
	isMember, err := s.container.TeamService().IsUserMemberOfTeam(ctx, userID, teamID)
	if err != nil {
		// Generic error message to prevent information leakage
		return fmt.Errorf("access denied")
	}

	if !isMember {
		// Generic error message - don't distinguish between "not a member" vs "team doesn't exist"
		return fmt.Errorf("access denied")
	}

	return nil
}

// PaginationParams holds validated pagination parameters
type PaginationParams struct {
	Page  int
	Limit int
}

// validatePaginationParams parses and validates pagination query parameters
// Returns validated page and limit with proper bounds checking to prevent performance issues
//
// Bounds:
//   - page: 1 to 10000 (prevents excessive offset calculations)
//   - limit: 1 to 100 (prevents loading too many records)
//
// Defaults:
//   - page: 1
//   - limit: 10
func validatePaginationParams(pageStr, limitStr string) PaginationParams {
	const (
		defaultPage  = 1
		maxPage      = 10000
		defaultLimit = 10
		maxLimit     = 100
		minLimit     = 1
	)

	page := defaultPage
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p >= 1 && p <= maxPage {
			page = p
		}
	}

	limit := defaultLimit
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l >= minLimit && l <= maxLimit {
			limit = l
		}
	}

	return PaginationParams{
		Page:  page,
		Limit: limit,
	}
}
