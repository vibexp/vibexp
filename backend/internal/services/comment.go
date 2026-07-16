package services

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

const (
	commentDefaultListLimit   = 20
	commentMaxListLimit       = 100
	commentDefaultRecentLimit = 10
)

// CommentService implements CommentServiceInterface.
//
// It deliberately takes NO event publisher: comment create/update must never
// reach the embedding worker or the search type map (comments are not
// searchable and not exposed over MCP — that is guaranteed by construction, by
// this service emitting no domain event).
type CommentService struct {
	repo        repositories.CommentRepository
	teamService TeamServiceInterface
	authz       AuthorizationServiceInterface
	logger      *slog.Logger
}

// Ensure CommentService implements CommentServiceInterface.
var _ CommentServiceInterface = (*CommentService)(nil)

// NewCommentService creates a new CommentService.
func NewCommentService(
	repo repositories.CommentRepository,
	teamService TeamServiceInterface,
	authzService AuthorizationServiceInterface,
	logger *slog.Logger,
) *CommentService {
	return &CommentService{
		repo:        repo,
		teamService: teamService,
		authz:       authzService,
		logger:      logger,
	}
}

// validateCommentContent trims and length-checks a comment body, returning the
// canonical trimmed value.
func validateCommentContent(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", fmt.Errorf("content is required")
	}
	if len([]rune(content)) > models.CommentContentMaxLen {
		return "", fmt.Errorf("content exceeds maximum length of %d characters", models.CommentContentMaxLen)
	}
	return content, nil
}

// checkMembership verifies the user is a member of the team. Reads (list) have
// no matrix permission of their own, so membership is the gate.
func (s *CommentService) checkMembership(ctx context.Context, userID, teamID string) error {
	if s.teamService == nil {
		return nil
	}
	isMember, err := s.teamService.IsUserMemberOfTeam(ctx, userID, teamID)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to validate team membership for comment operation")
		return fmt.Errorf("failed to validate team membership")
	}
	if !isMember {
		return fmt.Errorf("user is not a member of the specified team")
	}
	return nil
}

// Create adds a comment on a resource.
func (s *CommentService) Create(
	ctx context.Context, userID, teamID string, req *models.CreateCommentRequest,
) (*models.Comment, error) {
	if !models.IsValidCommentResourceType(req.ResourceType) {
		return nil, fmt.Errorf("invalid resource type: %q", req.ResourceType)
	}
	content, err := validateCommentContent(req.Content)
	if err != nil {
		return nil, err
	}

	// Creating a comment is open to any team member (epic #220), but the caller
	// must still BE one — Can proves the role grants the action.
	if authzErr := s.authz.Can(ctx, userID, teamID, authz.ResourceCreate); authzErr != nil {
		return nil, authzErr
	}

	// The target resource must exist in the same team; this rejects comments on
	// a missing or foreign resource.
	exists, err := s.repo.ResourceExists(ctx, teamID, req.ResourceType, req.ResourceID)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"resource_type", req.ResourceType,
			"resource_id", req.ResourceID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to verify resource existence for comment")
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("resource not found")
	}

	comment := &models.Comment{
		TeamID:       teamID,
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		UserID:       userID,
		Content:      content,
	}
	if createErr := s.repo.Create(ctx, comment); createErr != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"resource_type", req.ResourceType,
			"resource_id", req.ResourceID,
			"error", fmt.Sprintf("%+v", createErr),
		).Error("Failed to create comment")
		return nil, createErr
	}
	return comment, nil
}

// Update edits a comment's content. Editing is strictly author-only for every
// role — there is deliberately no matrix permission that grants editing
// another member's comment.
func (s *CommentService) Update(
	ctx context.Context, userID, teamID, commentID string, req *models.UpdateCommentRequest,
) (*models.Comment, error) {
	content, err := validateCommentContent(req.Content)
	if err != nil {
		return nil, err
	}

	comment, err := s.repo.GetByID(ctx, teamID, commentID)
	if err != nil {
		return nil, err
	}

	if comment.UserID != userID {
		return nil, fmt.Errorf("%w: only the author may edit a comment", ErrPermissionDenied)
	}

	updated, err := s.repo.UpdateContent(ctx, teamID, commentID, content)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"comment_id", commentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to update comment")
		return nil, err
	}
	return updated, nil
}

// Delete removes a comment. Delete is own-vs-any: the author deletes their own,
// Admin/Owner delete any comment in the team.
func (s *CommentService) Delete(ctx context.Context, userID, teamID, commentID string) error {
	comment, err := s.repo.GetByID(ctx, teamID, commentID)
	if err != nil {
		return err
	}

	if authzErr := s.authz.CanActOnResource(
		ctx, userID, teamID, comment.UserID,
		authz.ResourceDeleteOwn, authz.ResourceDeleteAny,
	); authzErr != nil {
		return authzErr
	}

	if delErr := s.repo.Delete(ctx, teamID, commentID); delErr != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"comment_id", commentID,
			"error", fmt.Sprintf("%+v", delErr),
		).Error("Failed to delete comment")
		return delErr
	}
	return nil
}

// ListByResource returns a page of a resource's comments, newest-first.
func (s *CommentService) ListByResource(
	ctx context.Context, userID, teamID, resourceType, resourceID string, page, limit int,
) (*models.CommentListResponse, error) {
	if !models.IsValidCommentResourceType(resourceType) {
		return nil, fmt.Errorf("invalid resource type: %q", resourceType)
	}

	page, limit = clampCommentPage(page, limit)

	if err := s.checkMembership(ctx, userID, teamID); err != nil {
		return nil, err
	}

	comments, totalCount, err := s.repo.ListByResource(ctx, teamID, resourceType, resourceID, page, limit)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"resource_type", resourceType,
			"resource_id", resourceID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list comments")
		return nil, err
	}

	totalPages := int(math.Ceil(float64(totalCount) / float64(limit)))
	return &models.CommentListResponse{
		Comments:   comments,
		TotalCount: totalCount,
		Page:       page,
		PerPage:    limit,
		TotalPages: totalPages,
	}, nil
}

// ListRecentByTeam returns the team's most recent comment activity.
func (s *CommentService) ListRecentByTeam(
	ctx context.Context, userID, teamID string, limit int,
) ([]models.CommentActivity, error) {
	if limit <= 0 {
		limit = commentDefaultRecentLimit
	} else if limit > commentMaxListLimit {
		limit = commentMaxListLimit
	}

	if err := s.checkMembership(ctx, userID, teamID); err != nil {
		return nil, err
	}

	activities, err := s.repo.ListRecentByTeam(ctx, teamID, limit)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list recent comments")
		return nil, err
	}
	return activities, nil
}

// clampCommentPage applies the standard pagination defaults/ceilings.
func clampCommentPage(page, limit int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = commentDefaultListLimit
	} else if limit > commentMaxListLimit {
		limit = commentMaxListLimit
	}
	return page, limit
}
