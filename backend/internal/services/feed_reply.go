package services

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

const (
	feedItemReplyContentMaxLen   = 10000
	feedItemReplyAssistantMaxLen = 30
)

// FeedItemReplyService implements FeedItemReplyServiceInterface
type FeedItemReplyService struct {
	replyRepo    repositories.FeedItemReplyRepository
	feedItemRepo repositories.FeedItemRepository
	teamService  TeamServiceInterface
	eventManager events.EventPublisher
	logger       *logrus.Logger
}

// Ensure FeedItemReplyService implements FeedItemReplyServiceInterface
var _ FeedItemReplyServiceInterface = (*FeedItemReplyService)(nil)

// NewFeedItemReplyService creates a new FeedItemReplyService
func NewFeedItemReplyService(
	replyRepo repositories.FeedItemReplyRepository,
	feedItemRepo repositories.FeedItemRepository,
	teamService TeamServiceInterface,
	eventManager events.EventPublisher,
	logger *logrus.Logger,
) *FeedItemReplyService {
	return &FeedItemReplyService{
		replyRepo:    replyRepo,
		feedItemRepo: feedItemRepo,
		teamService:  teamService,
		eventManager: eventManager,
		logger:       logger,
	}
}

// validateReplyRequest trims and validates content and ai_assistant_name.
// It mutates req.Content so callers (both HTTP and MCP) store the canonical trimmed value.
func validateReplyRequest(req *models.CreateFeedItemReplyRequest) error {
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		return fmt.Errorf("content is required")
	}
	if len([]rune(req.Content)) > feedItemReplyContentMaxLen {
		return fmt.Errorf("content exceeds maximum length of %d characters", feedItemReplyContentMaxLen)
	}
	if req.AIAssistantName != nil && len([]rune(*req.AIAssistantName)) > feedItemReplyAssistantMaxLen {
		return fmt.Errorf("ai_assistant_name exceeds maximum length of %d characters", feedItemReplyAssistantMaxLen)
	}
	return nil
}

// checkReplyTeamMembership verifies the user is a member of the team.
func (s *FeedItemReplyService) checkReplyTeamMembership(ctx context.Context, userID, teamID string) error {
	if s.teamService == nil {
		return nil
	}
	isMember, err := s.teamService.IsUserMemberOfTeam(ctx, userID, teamID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"user_id": userID,
			"team_id": teamID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to validate team membership for CreateReply")
		return fmt.Errorf("failed to validate team membership")
	}
	if !isMember {
		s.logger.WithFields(logrus.Fields{
			"user_id": userID,
			"team_id": teamID,
		}).Warn("User attempted to reply to feed item in team they are not a member of")
		return fmt.Errorf("user is not a member of the specified team")
	}
	return nil
}

// CreateReply creates a new reply on a feed item.
func (s *FeedItemReplyService) CreateReply(
	ctx context.Context, userID, teamID, feedItemID string, req *models.CreateFeedItemReplyRequest,
) (*models.FeedItemReply, error) {
	if err := validateReplyRequest(req); err != nil {
		return nil, err
	}
	if err := s.checkReplyTeamMembership(ctx, userID, teamID); err != nil {
		return nil, err
	}

	item, err := s.feedItemRepo.GetByID(ctx, userID, teamID, feedItemID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"user_id":      userID,
			"team_id":      teamID,
			"feed_item_id": feedItemID,
			"error":        fmt.Sprintf("%+v", err),
		}).Error("Failed to get feed item for CreateReply")
		return nil, err
	}
	if item.ArchivedAt != nil {
		return nil, fmt.Errorf("feed item is archived")
	}

	reply := &models.FeedItemReply{
		TeamID:          teamID,
		FeedItemID:      feedItemID,
		Content:         req.Content,
		PostedByUserID:  userID,
		AIAssistantName: req.AIAssistantName,
		PostedAt:        time.Now(),
	}
	created, err := s.replyRepo.CreateReply(ctx, reply)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"user_id":      userID,
			"team_id":      teamID,
			"feed_item_id": feedItemID,
			"error":        fmt.Sprintf("%+v", err),
		}).Error("Failed to create feed item reply")
		return nil, err
	}

	// Publish feed item reply created event. userID is the reply's PostedByUserID,
	// which keys the embedding row written by the downstream pipeline.
	if s.eventManager != nil {
		event := events.NewFeedItemReplyCreatedEvent(
			created.ID, userID, teamID, feedItemID, created.Content, created.PostedAt,
		)
		if pubErr := s.eventManager.Publish(ctx, event); pubErr != nil {
			s.logger.WithError(pubErr).Warn("Failed to publish feed item reply created event")
		}
	}

	return created, nil
}

// ListReplyPosters returns the (reply_id, posted_by_user_id) pairs for every reply on
// feedItemID, after verifying team membership. Used by the delete handler to clean up
// each reply's embedding row (keyed by its poster) before a feed item is hard-deleted.
func (s *FeedItemReplyService) ListReplyPosters(
	ctx context.Context, userID, teamID, feedItemID string,
) ([]repositories.FeedItemReplyPoster, error) {
	if err := s.checkReplyTeamMembership(ctx, userID, teamID); err != nil {
		return nil, err
	}
	posters, err := s.replyRepo.ListReplyPostersByItemID(ctx, teamID, feedItemID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"user_id":      userID,
			"team_id":      teamID,
			"feed_item_id": feedItemID,
			"error":        fmt.Sprintf("%+v", err),
		}).Error("Failed to list reply posters for feed item")
		return nil, err
	}
	return posters, nil
}

// GetReply retrieves a single reply by ID, verifying team membership first.
func (s *FeedItemReplyService) GetReply(
	ctx context.Context, userID, teamID, replyID string,
) (*models.FeedItemReply, error) {
	if err := s.checkReplyTeamMembership(ctx, userID, teamID); err != nil {
		return nil, err
	}
	reply, err := s.replyRepo.GetReply(ctx, userID, teamID, replyID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"user_id":  userID,
			"team_id":  teamID,
			"reply_id": replyID,
			"error":    fmt.Sprintf("%+v", err),
		}).Error("Failed to get feed item reply")
		return nil, err
	}
	return reply, nil
}

// ListReplies retrieves paginated replies for a feed item, newest-first
func (s *FeedItemReplyService) ListReplies(
	ctx context.Context, userID, teamID, feedItemID string, page, limit int,
) (*models.FeedItemReplyListResponse, error) {
	// Defensive pagination defaults
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

	// Verify team membership
	if s.teamService != nil {
		isMember, err := s.teamService.IsUserMemberOfTeam(ctx, userID, teamID)
		if err != nil {
			s.logger.WithFields(logrus.Fields{
				"user_id": userID,
				"team_id": teamID,
				"error":   fmt.Sprintf("%+v", err),
			}).Error("Failed to validate team membership for ListReplies")
			return nil, fmt.Errorf("failed to validate team membership")
		}
		if !isMember {
			return nil, fmt.Errorf("user is not a member of the specified team")
		}
	}

	replies, totalCount, err := s.replyRepo.ListReplies(ctx, teamID, feedItemID, page, limit)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"user_id":      userID,
			"team_id":      teamID,
			"feed_item_id": feedItemID,
			"error":        fmt.Sprintf("%+v", err),
		}).Error("Failed to list feed item replies")
		return nil, err
	}

	totalPages := int(math.Ceil(float64(totalCount) / float64(limit)))

	return &models.FeedItemReplyListResponse{
		Replies:    replies,
		TotalCount: totalCount,
		Page:       page,
		PerPage:    limit,
		TotalPages: totalPages,
	}, nil
}
