package services

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

const (
	feedNameMaxLen          = 255
	feedItemTitleMaxLen     = 255
	feedItemAssistantMaxLen = 30
)

const (
	feedItemExcerptMaxLen  = 300
	feedItemContentMaxSize = 204800 // 200 KB
)

// markdownStripPattern matches common markdown syntax for stripping
var markdownStripPattern = regexp.MustCompile(
	`(?m)(` +
		`\*\*(.+?)\*\*` + // bold
		`|\*(.+?)\*` + // italic
		`|~~(.+?)~~` + // strikethrough
		`|` + "`" + `(.+?)` + "`" + // inline code
		`|\[([^\]]*)\]\([^)]*\)` + // links
		`|#{1,6}\s*` + // headings
		`|>\s*` + // blockquotes
		`|\*\s+` + // unordered list
		`|\d+\.\s+` + // ordered list
		`|` + "```[\\s\\S]*?```" + // code blocks
		`|\n{2,}` + // multiple newlines
		`)`,
)

// computeExcerpt strips markdown from content and returns the first 300 chars,
// appending … if truncated. Mirrors the spec: strip markdown, take first 300
// chars, append … if truncated.
func computeExcerpt(content string) string {
	// Strip code blocks first (multi-line)
	stripped := regexp.MustCompile("(?s)```.*?```").ReplaceAllString(content, " ")

	// Strip remaining markdown inline syntax
	stripped = markdownStripPattern.ReplaceAllStringFunc(stripped, func(match string) string {
		// For links, return the link text
		linkMatch := regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`).FindStringSubmatch(match)
		if len(linkMatch) > 1 {
			return linkMatch[1]
		}
		// For bold/italic/strikethrough/inline code, return inner text
		for _, inner := range []string{
			`\*\*(.+?)\*\*`,
			`\*(.+?)\*`,
			`~~(.+?)~~`,
			"`(.+?)`",
		} {
			innerMatch := regexp.MustCompile(inner).FindStringSubmatch(match)
			if len(innerMatch) > 1 {
				return innerMatch[1]
			}
		}
		return " "
	})

	// Normalize whitespace
	stripped = strings.Join(strings.Fields(stripped), " ")
	stripped = strings.TrimSpace(stripped)

	runes := []rune(stripped)
	if len(runes) <= feedItemExcerptMaxLen {
		return stripped
	}

	return string(runes[:feedItemExcerptMaxLen]) + "…"
}

// FeedService implements FeedServiceInterface
type FeedService struct {
	feedRepo     repositories.FeedRepository
	teamService  TeamServiceInterface
	eventManager events.EventPublisher
	logger       *slog.Logger
}

// Ensure FeedService implements FeedServiceInterface
var _ FeedServiceInterface = (*FeedService)(nil)

// NewFeedService creates a new FeedService
func NewFeedService(
	feedRepo repositories.FeedRepository,
	teamService TeamServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) *FeedService {
	return &FeedService{
		feedRepo:     feedRepo,
		teamService:  teamService,
		eventManager: eventManager,
		logger:       logger,
	}
}

// CreateFeed creates a new feed for a team
func (s *FeedService) CreateFeed(
	ctx context.Context, userID, teamID string, req *models.CreateFeedRequest,
) (*models.Feed, error) {
	// M5: service-layer validation (defense-in-depth against callers that bypass handler validation)
	if len([]rune(req.Name)) == 0 {
		return nil, fmt.Errorf("name is required")
	}
	if len([]rune(req.Name)) > feedNameMaxLen {
		return nil, fmt.Errorf("name exceeds maximum length of %d characters", feedNameMaxLen)
	}

	// H1/H3: verify the caller is a member of the target team
	if s.teamService != nil {
		isMember, err := s.teamService.IsUserMemberOfTeam(ctx, userID, teamID)
		if err != nil {
			s.logger.With(
				"user_id", userID,
				"team_id", teamID,
				"error", fmt.Sprintf("%+v", err),
			).
				Error("Failed to validate team membership for CreateFeed")
			return nil, fmt.Errorf("failed to validate team membership")
		}
		if !isMember {
			s.logger.With(
				"user_id", userID,
				"team_id", teamID,
			).
				Warn("User attempted to create feed in team they are not a member of")
			return nil, fmt.Errorf("user is not a member of the specified team")
		}
	}

	now := time.Now()
	feed := &models.Feed{
		TeamID:          teamID,
		Name:            req.Name,
		Description:     req.Description,
		CreatedByUserID: userID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.feedRepo.Create(ctx, feed); err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to create feed")
		return nil, err
	}

	return feed, nil
}

// GetFeed retrieves a feed by ID
func (s *FeedService) GetFeed(ctx context.Context, userID, teamID, feedID string) (*models.Feed, error) {
	feed, err := s.feedRepo.GetByID(ctx, userID, teamID, feedID)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"feed_id", feedID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to get feed")
		return nil, err
	}

	return feed, nil
}

// ListFeeds retrieves feeds with filtering and pagination
func (s *FeedService) ListFeeds(
	ctx context.Context, userID string, filters FeedFilters,
) (*models.FeedListResponse, error) {
	// H4: defensive pagination defaults — clamp before offset calculation
	if filters.Page <= 0 {
		filters.Page = 1
	}
	if filters.Limit <= 0 {
		filters.Limit = 20
	} else if filters.Limit > 100 {
		filters.Limit = 100
	}

	repoFilters := repositories.FeedFilters{
		TeamID: filters.TeamID,
		Search: filters.Search,
		Page:   filters.Page,
		Limit:  filters.Limit,
	}

	feeds, totalCount, err := s.feedRepo.List(ctx, userID, repoFilters)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", filters.TeamID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to list feeds")
		return nil, err
	}

	totalPages := int(math.Ceil(float64(totalCount) / float64(filters.Limit)))

	return &models.FeedListResponse{
		Feeds:      feeds,
		TotalCount: totalCount,
		Page:       filters.Page,
		PerPage:    filters.Limit,
		TotalPages: totalPages,
	}, nil
}

// ListFeedsForMCP returns feeds enriched with last_post_at for the MCP list-feeds tool.
// It uses a single LEFT JOIN query (no N+1) and does NOT alter the REST API response shape.
func (s *FeedService) ListFeedsForMCP(
	ctx context.Context, userID string, filters FeedFilters,
) (*models.MCPFeedListResponse, error) {
	// Apply the same pagination defaults as ListFeeds.
	if filters.Page <= 0 {
		filters.Page = 1
	}
	if filters.Limit <= 0 {
		filters.Limit = 20
	} else if filters.Limit > 100 {
		filters.Limit = 100
	}

	repoFilters := repositories.FeedFilters{
		TeamID: filters.TeamID,
		Search: filters.Search,
		Page:   filters.Page,
		Limit:  filters.Limit,
	}

	feeds, err := s.feedRepo.ListWithLastPost(ctx, userID, repoFilters)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", filters.TeamID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to list feeds for MCP")
		return nil, err
	}

	if feeds == nil {
		feeds = make([]models.FeedWithLastPost, 0)
	}

	return &models.MCPFeedListResponse{Feeds: feeds}, nil
}

// UpdateFeed updates an existing feed
func (s *FeedService) UpdateFeed(
	ctx context.Context, userID, teamID, feedID string, req *models.UpdateFeedRequest,
) (*models.Feed, error) {
	feed, err := s.feedRepo.GetByID(ctx, userID, teamID, feedID)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		feed.Name = *req.Name
	}
	if req.Description != nil {
		feed.Description = req.Description
	}
	feed.UpdatedAt = time.Now()

	if err := s.feedRepo.Update(ctx, feed); err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"feed_id", feedID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to update feed")
		return nil, err
	}

	return feed, nil
}

// DeleteFeed deletes a feed (cascades to feed items via DB constraint)
func (s *FeedService) DeleteFeed(ctx context.Context, userID, teamID, feedID string) error {
	if err := s.feedRepo.Delete(ctx, userID, teamID, feedID); err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"feed_id", feedID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to delete feed")
		return err
	}

	return nil
}

// FeedItemService implements FeedItemServiceInterface
type FeedItemService struct {
	feedItemRepo repositories.FeedItemRepository
	replyRepo    repositories.FeedItemReplyRepository
	projectRepo  repositories.ProjectRepository
	teamService  TeamServiceInterface
	eventManager events.EventPublisher
	logger       *slog.Logger
}

// Ensure FeedItemService implements FeedItemServiceInterface
var _ FeedItemServiceInterface = (*FeedItemService)(nil)

// NewFeedItemService creates a new FeedItemService
func NewFeedItemService(
	feedItemRepo repositories.FeedItemRepository,
	replyRepo repositories.FeedItemReplyRepository,
	projectRepo repositories.ProjectRepository,
	teamService TeamServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) *FeedItemService {
	return &FeedItemService{
		feedItemRepo: feedItemRepo,
		replyRepo:    replyRepo,
		projectRepo:  projectRepo,
		teamService:  teamService,
		eventManager: eventManager,
		logger:       logger,
	}
}

// validateFeedItemRequest performs service-layer defense-in-depth validation (M5).
func validateFeedItemRequest(req *models.CreateFeedItemRequest) error {
	if len([]rune(req.Title)) == 0 {
		return fmt.Errorf("title is required")
	}
	if len([]rune(req.Title)) > feedItemTitleMaxLen {
		return fmt.Errorf("title exceeds maximum length of %d characters", feedItemTitleMaxLen)
	}
	if len([]rune(req.AIAssistantName)) == 0 {
		return fmt.Errorf("ai_assistant_name is required")
	}
	if len([]rune(req.AIAssistantName)) > feedItemAssistantMaxLen {
		return fmt.Errorf("ai_assistant_name exceeds maximum length of %d characters", feedItemAssistantMaxLen)
	}
	if len(req.Content) > feedItemContentMaxSize {
		return fmt.Errorf("content exceeds maximum size of 200 KB")
	}
	return nil
}

// checkFeedItemTeamMembership verifies userID is a member of teamID (H1/H3).
func (s *FeedItemService) checkFeedItemTeamMembership(ctx context.Context, userID, teamID string) error {
	if s.teamService == nil {
		return nil
	}
	isMember, err := s.teamService.IsUserMemberOfTeam(ctx, userID, teamID)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to validate team membership for CreateFeedItem")
		return fmt.Errorf("failed to validate team membership")
	}
	if !isMember {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
		).
			Warn("User attempted to create feed item in team they are not a member of")
		return fmt.Errorf("user is not a member of the specified team")
	}
	return nil
}

// validateFeedItemProject verifies the project belongs to teamID (H2).
func (s *FeedItemService) validateFeedItemProject(ctx context.Context, userID, teamID string, projectID *string) error {
	if projectID == nil || s.projectRepo == nil {
		return nil
	}
	project, err := s.projectRepo.GetByID(ctx, userID, *projectID)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"project_id", *projectID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to validate project for CreateFeedItem")
		return fmt.Errorf("project not found")
	}
	if project.TeamID != teamID {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"project_id", *projectID,
			"project_team_id", project.TeamID,
		).Warn("User attempted to attach feed item to project in different team")
		return fmt.Errorf("project does not belong to the specified team")
	}
	return nil
}

// CreateFeedItem creates a new feed item with server-set posted_at and computed excerpt
func (s *FeedItemService) CreateFeedItem(
	ctx context.Context, userID, teamID, feedID string, req *models.CreateFeedItemRequest,
) (*models.FeedItem, error) {
	if err := validateFeedItemRequest(req); err != nil {
		return nil, err
	}
	if err := s.checkFeedItemTeamMembership(ctx, userID, teamID); err != nil {
		return nil, err
	}
	if err := s.validateFeedItemProject(ctx, userID, teamID, req.ProjectID); err != nil {
		return nil, err
	}

	// Compute excerpt server-side
	excerpt := computeExcerpt(req.Content)

	item := &models.FeedItem{
		TeamID:          teamID,
		FeedID:          feedID,
		ProjectID:       req.ProjectID,
		Title:           req.Title,
		Content:         req.Content,
		Excerpt:         excerpt,
		AIAssistantName: req.AIAssistantName,
		PostedByUserID:  userID,
		PostedAt:        time.Now(), // server-set; do NOT accept from client
	}

	if err := s.feedItemRepo.Create(ctx, item); err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"feed_id", feedID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to create feed item")
		return nil, err
	}

	// Publish feed item created event. userID is the item's PostedByUserID, which
	// keys the embedding row written by the downstream pipeline.
	if s.eventManager != nil {
		event := events.NewFeedItemCreatedEvent(
			item.ID, userID, teamID, feedID, item.Title, item.Content, item.Excerpt, item.PostedAt,
		)
		if pubErr := s.eventManager.Publish(ctx, event); pubErr != nil {
			s.logger.With("error", pubErr).Warn("Failed to publish feed item created event")
		}
	}

	return item, nil
}

// GetFeedItem retrieves a feed item by ID
func (s *FeedItemService) GetFeedItem(ctx context.Context, userID, teamID, itemID string) (*models.FeedItem, error) {
	item, err := s.feedItemRepo.GetByID(ctx, userID, teamID, itemID)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"item_id", itemID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to get feed item")
		return nil, err
	}

	return item, nil
}

// ListFeedItems retrieves feed items with filtering and pagination
func (s *FeedItemService) ListFeedItems(
	ctx context.Context, userID string, filters FeedItemFilters,
) (*models.FeedItemListResponse, error) {
	// H4: defensive pagination defaults — clamp before offset calculation
	if filters.Page <= 0 {
		filters.Page = 1
	}
	if filters.Limit <= 0 {
		filters.Limit = 20
	} else if filters.Limit > 100 {
		filters.Limit = 100
	}

	repoFilters := repositories.FeedItemFilters{
		TeamID:          filters.TeamID,
		FeedID:          filters.FeedID,
		ProjectID:       filters.ProjectID,
		AIAssistantName: filters.AIAssistantName,
		Archived:        filters.Archived,
		Page:            filters.Page,
		Limit:           filters.Limit,
	}

	items, totalCount, err := s.feedItemRepo.List(ctx, userID, repoFilters)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", filters.TeamID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to list feed items")
		return nil, err
	}

	totalPages := int(math.Ceil(float64(totalCount) / float64(filters.Limit)))

	return &models.FeedItemListResponse{
		Items:      items,
		TotalCount: totalCount,
		Page:       filters.Page,
		PerPage:    filters.Limit,
		TotalPages: totalPages,
	}, nil
}

// ArchiveFeedItem sets archived_at = NOW() for a feed item
func (s *FeedItemService) ArchiveFeedItem(ctx context.Context, userID, teamID, itemID string) error {
	if err := s.feedItemRepo.Archive(ctx, userID, teamID, itemID); err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"item_id", itemID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to archive feed item")
		return err
	}

	return nil
}

// UnarchiveFeedItem sets archived_at = NULL for a feed item
func (s *FeedItemService) UnarchiveFeedItem(ctx context.Context, userID, teamID, itemID string) error {
	if err := s.feedItemRepo.Unarchive(ctx, userID, teamID, itemID); err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"item_id", itemID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to unarchive feed item")
		return err
	}

	return nil
}

// DeleteFeedItem hard-deletes a feed item
func (s *FeedItemService) DeleteFeedItem(ctx context.Context, userID, teamID, itemID string) error {
	if err := s.feedItemRepo.Delete(ctx, userID, teamID, itemID); err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"item_id", itemID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to delete feed item")
		return err
	}

	return nil
}

// EnrichWithReplyCounts annotates each FeedItem in the slice with its reply count.
// A single bulk COUNT query is used to avoid N+1 queries.
func (s *FeedItemService) EnrichWithReplyCounts(
	ctx context.Context, teamID string, items []models.FeedItem,
) ([]models.FeedItem, error) {
	if len(items) == 0 || s.replyRepo == nil {
		return items, nil
	}

	itemIDs := make([]string, len(items))
	for i, item := range items {
		itemIDs[i] = item.ID
	}

	counts, err := s.replyRepo.CountRepliesByItemIDs(ctx, teamID, itemIDs)
	if err != nil {
		s.logger.With(
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to count replies for feed items")
		return nil, err
	}

	for i := range items {
		items[i].ReplyCount = counts[items[i].ID]
	}

	return items, nil
}
