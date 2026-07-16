package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/pkg/events"
)

// excerptMaxLen is the maximum character length for reply/memory excerpts in list responses.
const excerptMaxLen = 300

// ListFeedsParams defines the parameters for listing feeds.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type ListFeedsParams struct {
	TeamID string `json:"team_id"         jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	Page   int    `json:"page,omitempty"  jsonschema:"Page number, starting from 1 (default: 1)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Feeds per page, max 10 (default: 10)"`
}

// ListFeedItemsParams defines the parameters for listing items in a feed.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type ListFeedItemsParams struct {
	TeamID         string `json:"team_id"                  jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	FeedID         string `json:"feed_id"                  jsonschema:"UUID of the feed (call vibexp_io_list_feeds first to discover feed IDs)"`
	Page           int    `json:"page,omitempty"           jsonschema:"Page number, starting from 1 (default: 1)"`
	Limit          int    `json:"limit,omitempty"          jsonschema:"Items per page, max 10 (default: 10)"`
	FullDetails    bool   `json:"full_details,omitempty"   jsonschema:"When true, include full content field; default false returns excerpt only"`
	IncludeReplies bool   `json:"include_replies,omitempty" jsonschema:"When true, embed up to 3 recent reply excerpts per item (item content stays an excerpt). For one item's full replies use vibexp_io_get_feed_item."`
}

// GetFeedItemParams defines the parameters for getting a single feed item.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type GetFeedItemParams struct {
	TeamID     string `json:"team_id"      jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	FeedItemID string `json:"feed_item_id" jsonschema:"UUID of the feed item to retrieve (from vibexp_io_list_feed_items)"`
}

// ReplyToFeedItemParams defines the parameters for replying to a feed item.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type ReplyToFeedItemParams struct {
	TeamID          string `json:"team_id"           jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	FeedItemID      string `json:"feed_item_id"      jsonschema:"UUID of the feed item to reply to (call vibexp_io_list_feeds then use the item id)"`
	Content         string `json:"content"           jsonschema:"Reply content. Max 10 000 chars. Plain text or light Markdown."`
	AIAssistantName string `json:"ai_assistant_name" jsonschema:"Stable identifier for your tool. Use a consistent value. Max 30 chars."`
}

// replyToFeedItemResponse is the JSON payload returned by vibexp_io_reply_to_feed_item.
type replyToFeedItemResponse struct {
	ID      string `json:"id"`
	FullURL string `json:"full_url"`
}

// PostToFeedParams defines the parameters for posting an item to a feed.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type PostToFeedParams struct {
	TeamID          string  `json:"team_id"              jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	FeedID          string  `json:"feed_id"              jsonschema:"UUID of the feed (call vibexp_io_list_feeds first)"`
	Title           string  `json:"title"                jsonschema:"Short, descriptive title (max 255 chars). Example: 'Refactored auth module — 12 files touched'"`
	Content         string  `json:"content"              jsonschema:"Body of the update in Markdown. Max 200 KB. Use code blocks for code, tables where helpful."`
	AIAssistantName string  `json:"ai_assistant_name"    jsonschema:"Stable identifier for your tool. Use a consistent value across calls — never random or timestamped. Examples: 'Claude Code CLI', 'Claude Code Web', 'Codex', 'Gemini CLI'. Max 30 chars."`
	ProjectID       *string `json:"project_id,omitempty" jsonschema:"Optional UUID of the project this update relates to. Must be a project in the same team."`
}

// postToFeedResponse is the JSON payload returned by vibexp_io_post_to_feed.
type postToFeedResponse struct {
	ID      string `json:"id"`
	FullURL string `json:"full_url"`
}

// feedItemExcerpt is the slim per-item shape used in list responses when full_details=false.
type feedItemExcerpt struct {
	ID              string  `json:"id"`
	TeamID          string  `json:"team_id"`
	FeedID          string  `json:"feed_id"`
	ProjectID       *string `json:"project_id,omitempty"`
	Title           string  `json:"title"`
	Excerpt         string  `json:"excerpt"`
	AIAssistantName string  `json:"ai_assistant_name"`
	PostedByUserID  string  `json:"posted_by_user_id"`
	PostedAt        string  `json:"posted_at"`
	ReplyCount      int     `json:"reply_count"`
}

// feedItemExcerptListResponse is the list shape returned when full_details=false.
type feedItemExcerptListResponse struct {
	Items      []feedItemExcerpt `json:"items"`
	TotalCount int               `json:"total_count"`
	Page       int               `json:"page"`
	PerPage    int               `json:"per_page"`
	TotalPages int               `json:"total_pages"`
}

// replyExcerpt is the slim per-reply shape used in list responses when full_details=false.
type replyExcerpt struct {
	ID              string  `json:"id"`
	TeamID          string  `json:"team_id"`
	FeedItemID      string  `json:"feed_item_id"`
	Content         string  `json:"content"`
	Truncated       bool    `json:"truncated,omitempty"`
	PostedByUserID  string  `json:"posted_by_user_id"`
	AIAssistantName *string `json:"ai_assistant_name,omitempty"`
	PostedAt        string  `json:"posted_at"`
}

// Reply-embedding limits for the consolidated read tools: get_feed_item returns
// the item with up to getFeedItemRepliesLimit full replies; list_feed_items with
// include_replies embeds up to listFeedItemRepliesEmbedLimit reply excerpts per item.
const (
	getFeedItemRepliesLimit       = 50
	listFeedItemRepliesEmbedLimit = 3
)

// feedItemWithReplies is the get_feed_item response: the full item plus its
// replies (full content) inline. RepliesTruncated is true when the item has more
// replies than the embedded page.
type feedItemWithReplies struct {
	*models.FeedItem
	Replies          []models.FeedItemReply `json:"replies"`
	RepliesTruncated bool                   `json:"replies_truncated,omitempty"`
}

// feedItemExcerptWithReplies is a feed item excerpt plus a bounded set of recent
// reply excerpts, used by list_feed_items when include_replies is set.
type feedItemExcerptWithReplies struct {
	feedItemExcerpt
	Replies []replyExcerpt `json:"replies"`
}

// feedItemExcerptWithRepliesListResponse is the list shape returned by
// list_feed_items when include_replies is set.
type feedItemExcerptWithRepliesListResponse struct {
	Items      []feedItemExcerptWithReplies `json:"items"`
	TotalCount int                          `json:"total_count"`
	Page       int                          `json:"page"`
	PerPage    int                          `json:"per_page"`
	TotalPages int                          `json:"total_pages"`
}

// truncateString truncates s to at most maxLen runes and appends "..." if truncated.
func truncateString(s string, maxLen int) (string, bool) {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s, false
	}
	return string(runes[:maxLen]) + "...", true
}

// listFeeds implements the vibexp_io_list_feeds MCP tool.
func (s *Server) listFeeds(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *ListFeedsParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	page := params.Page
	if page <= 0 {
		page = 1
	}
	limit := params.Limit
	if limit <= 0 || limit > 10 {
		limit = 10
	}

	filters := services.FeedFilters{
		TeamID: teamID,
		Page:   page,
		Limit:  limit,
	}

	response, err := s.container.FeedService().ListFeedsForMCP(ctx, userID, filters)
	if err != nil {
		slog.Error(
			"Failed to list feeds via MCP",
			"tool", "vibexp_io_list_feeds",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to list feeds: %v", err)},
			},
			IsError: true,
		}, nil, nil
	}

	// Ensure feeds is never null in JSON output.
	if response.Feeds == nil {
		response.Feeds = make([]models.FeedWithLastPost, 0)
	}

	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal response to JSON: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
		StructuredContent: response,
	}, response, nil
}

// postToFeed implements the vibexp_io_post_to_feed MCP tool.
//
//nolint:funlen // MCP tool handler requires team resolution, inline validation, quota check, and response building
func (s *Server) postToFeed(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *PostToFeedParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	// Validate feed_id is a well-formed UUID before handing off to the service.
	if !isValidUUID(params.FeedID) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "feed_id must be a valid UUID"},
			},
			IsError: true,
		}, nil, nil
	}

	// Validate project_id if provided.
	if params.ProjectID != nil && strings.TrimSpace(*params.ProjectID) != "" && !isValidUUID(*params.ProjectID) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "project_id must be a valid UUID"},
			},
			IsError: true,
		}, nil, nil
	}

	allowed, err := s.container.ResourceUsageService().CheckResourceLimit(ctx, userID, events.ResourceTypeFeedItem)
	if err != nil {
		slog.Error(
			"Failed to check feed item resource limit",
			"tool", "vibexp_io_post_to_feed",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Failed to check resource limit"},
			},
			IsError: true,
		}, nil, nil
	}
	if !allowed {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "You have reached the maximum number of feed items allowed for your subscription plan"},
			},
			IsError: true,
		}, nil, nil
	}

	req := &models.CreateFeedItemRequest{
		Title:           params.Title,
		Content:         params.Content,
		AIAssistantName: params.AIAssistantName,
		ProjectID:       params.ProjectID,
	}

	item, err := s.container.FeedItemService().CreateFeedItem(ctx, userID, teamID, params.FeedID, req)
	if err != nil {
		slog.Error(
			"Failed to post to feed via MCP",
			"tool", "vibexp_io_post_to_feed",
			"user_id", userID,
			"team_id", teamID,
			"feed_id", params.FeedID,
			"error", fmt.Sprintf("%+v", err),
		)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to post to feed: %v", err)},
			},
			IsError: true,
		}, nil, nil
	}

	baseURL := strings.TrimRight(s.config.Frontend.BaseURL, "/")
	result := &postToFeedResponse{
		ID:      item.ID,
		FullURL: fmt.Sprintf("%s/feed-items/%s", baseURL, item.ID),
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal response to JSON: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
		StructuredContent: result,
	}, result, nil
}

// replyToFeedItem implements the vibexp_io_reply_to_feed_item MCP tool.
//
//nolint:funlen // team resolution, UUID validation, quota check, service call, response building
func (s *Server) replyToFeedItem(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *ReplyToFeedItemParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	if !isValidUUID(params.FeedItemID) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "feed_item_id must be a valid UUID"},
			},
			IsError: true,
		}, nil, nil
	}

	assistantName := params.AIAssistantName
	var assistantNamePtr *string
	if assistantName != "" {
		assistantNamePtr = &assistantName
	}

	allowed, err := s.container.ResourceUsageService().CheckResourceLimit(ctx, userID, events.ResourceTypeFeedItem)
	if err != nil {
		slog.Error(
			"Failed to check feed item resource limit",
			"tool", "vibexp_io_reply_to_feed_item",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Failed to check resource limit"},
			},
			IsError: true,
		}, nil, nil
	}
	if !allowed {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "You have reached the maximum number of feed items allowed for your subscription plan"},
			},
			IsError: true,
		}, nil, nil
	}

	req := &models.CreateFeedItemReplyRequest{
		Content:         params.Content,
		AIAssistantName: assistantNamePtr,
	}

	reply, err := s.container.FeedItemReplyService().CreateReply(ctx, userID, teamID, params.FeedItemID, req)
	if err != nil {
		slog.Error(
			"Failed to reply to feed item via MCP",
			"tool", "vibexp_io_reply_to_feed_item",
			"user_id", userID,
			"team_id", teamID,
			"feed_item_id", params.FeedItemID,
			"error", fmt.Sprintf("%+v", err),
		)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to reply to feed item: %v", err)},
			},
			IsError: true,
		}, nil, nil
	}

	baseURL := strings.TrimRight(s.config.Frontend.BaseURL, "/")
	result := &replyToFeedItemResponse{
		ID:      reply.ID,
		FullURL: fmt.Sprintf("%s/feed-items/%s", baseURL, params.FeedItemID),
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal response to JSON: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
		StructuredContent: result,
	}, result, nil
}

// listFeedItems implements the vibexp_io_list_feed_items MCP tool.
//
//nolint:funlen // MCP tool handler requires team resolution, pagination clamping, enrichment, and response building
func (s *Server) listFeedItems(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *ListFeedItemsParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	if !isValidUUID(params.FeedID) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "feed_id must be a valid UUID"},
			},
			IsError: true,
		}, nil, nil
	}

	page := params.Page
	if page <= 0 {
		page = 1
	}
	limit := params.Limit
	if limit <= 0 || limit > 10 {
		limit = 10
	}

	feedID := params.FeedID
	filters := services.FeedItemFilters{
		TeamID: teamID,
		FeedID: &feedID,
		Page:   page,
		Limit:  limit,
	}

	response, err := s.container.FeedItemService().ListFeedItems(ctx, userID, filters)
	if err != nil {
		slog.Error(
			"Failed to list feed items via MCP",
			"tool", "vibexp_io_list_feed_items",
			"user_id", userID,
			"team_id", teamID,
			"feed_id", params.FeedID,
			"error", fmt.Sprintf("%+v", err),
		)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to list feed items: %v", err)},
			},
			IsError: true,
		}, nil, nil
	}

	// Ensure items is never null in JSON output.
	if response.Items == nil {
		response.Items = make([]models.FeedItem, 0)
	}

	enriched, enrichErr := s.container.FeedItemService().EnrichWithReplyCounts(ctx, teamID, response.Items)
	if enrichErr != nil {
		slog.Warn(
			"Failed to enrich feed items with reply counts",
			"tool", "vibexp_io_list_feed_items",
			"team_id", teamID,
			"error", enrichErr,
		)
	} else {
		response.Items = enriched
	}

	// When include_replies is set, return excerpt items each with a bounded set of
	// recent reply excerpts (this takes precedence over full_details, which is for
	// item content; for one item's full replies use get_feed_item).
	if params.IncludeReplies {
		return mcpJSONResult(s.buildFeedItemsWithReplies(ctx, userID, teamID, response))
	}
	// When full_details is true, return the complete response (including content).
	if params.FullDetails {
		return mcpJSONResult(response)
	}
	return mcpJSONResult(buildFeedItemExcerptResponse(response))
}

// toFeedItemExcerpt maps a feed item to its slim excerpt shape (omits content).
func toFeedItemExcerpt(item models.FeedItem) feedItemExcerpt {
	return feedItemExcerpt{
		ID:              item.ID,
		TeamID:          item.TeamID,
		FeedID:          item.FeedID,
		ProjectID:       item.ProjectID,
		Title:           item.Title,
		Excerpt:         item.Excerpt,
		AIAssistantName: item.AIAssistantName,
		PostedByUserID:  item.PostedByUserID,
		PostedAt:        item.PostedAt.UTC().Format("2006-01-02T15:04:05Z"),
		ReplyCount:      item.ReplyCount,
	}
}

// buildFeedItemExcerptResponse builds the slim excerpt-only list response (omits content).
func buildFeedItemExcerptResponse(response *models.FeedItemListResponse) *feedItemExcerptListResponse {
	excerpts := make([]feedItemExcerpt, 0, len(response.Items))
	for _, item := range response.Items {
		excerpts = append(excerpts, toFeedItemExcerpt(item))
	}
	return &feedItemExcerptListResponse{
		Items:      excerpts,
		TotalCount: response.TotalCount,
		Page:       response.Page,
		PerPage:    response.PerPage,
		TotalPages: response.TotalPages,
	}
}

// buildFeedItemsWithReplies builds the include_replies list response, embedding
// up to listFeedItemRepliesEmbedLimit recent reply excerpts per item. A per-item
// reply-load failure is logged and yields an empty replies slice rather than
// failing the whole listing.
func (s *Server) buildFeedItemsWithReplies(
	ctx context.Context, userID, teamID string, response *models.FeedItemListResponse,
) *feedItemExcerptWithRepliesListResponse {
	items := make([]feedItemExcerptWithReplies, 0, len(response.Items))
	for i := range response.Items {
		item := response.Items[i]
		replies := make([]replyExcerpt, 0)
		if item.ReplyCount > 0 {
			replyResp, err := s.container.FeedItemReplyService().ListReplies(
				ctx, userID, teamID, item.ID, 1, listFeedItemRepliesEmbedLimit,
			)
			if err != nil {
				slog.Warn(
					"Failed to embed replies for feed item via MCP",
					"tool", "vibexp_io_list_feed_items",
					"team_id", teamID,
					"feed_item_id", item.ID,
					"error", err,
				)
			} else {
				replies = buildReplyExcerpts(replyResp.Replies)
			}
		}
		items = append(items, feedItemExcerptWithReplies{
			feedItemExcerpt: toFeedItemExcerpt(item),
			Replies:         replies,
		})
	}
	return &feedItemExcerptWithRepliesListResponse{
		Items:      items,
		TotalCount: response.TotalCount,
		Page:       response.Page,
		PerPage:    response.PerPage,
		TotalPages: response.TotalPages,
	}
}

// buildReplyExcerpts converts replies to the slim excerpt shape with content truncated to excerptMaxLen.
func buildReplyExcerpts(replies []models.FeedItemReply) []replyExcerpt {
	excerpts := make([]replyExcerpt, 0, len(replies))
	for _, reply := range replies {
		truncated, wasTruncated := truncateString(reply.Content, excerptMaxLen)
		excerpts = append(excerpts, replyExcerpt{
			ID:              reply.ID,
			TeamID:          reply.TeamID,
			FeedItemID:      reply.FeedItemID,
			Content:         truncated,
			Truncated:       wasTruncated,
			PostedByUserID:  reply.PostedByUserID,
			AIAssistantName: reply.AIAssistantName,
			PostedAt:        reply.PostedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return excerpts
}

// getFeedItem implements the vibexp_io_get_feed_item MCP tool.
func (s *Server) getFeedItem(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *GetFeedItemParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	if !isValidUUID(params.FeedItemID) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "feed_item_id must be a valid UUID"},
			},
			IsError: true,
		}, nil, nil
	}

	item, err := s.container.FeedItemService().GetFeedItem(ctx, userID, teamID, params.FeedItemID)
	if err != nil {
		slog.Error(
			"Failed to get feed item via MCP",
			"tool", "vibexp_io_get_feed_item",
			"user_id", userID,
			"team_id", teamID,
			"feed_item_id", params.FeedItemID,
			"error", fmt.Sprintf("%+v", err),
		)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to get feed item: %v", err)},
			},
			IsError: true,
		}, nil, nil
	}

	withReplies, replyErrResult := s.loadFeedItemWithReplies(ctx, item, userID, teamID)
	if replyErrResult != nil {
		return replyErrResult, nil, nil
	}
	return mcpJSONResult(withReplies)
}

// loadFeedItemWithReplies fetches an item's replies (bounded to
// getFeedItemRepliesLimit) and assembles the get_feed_item response, setting the
// item's reply_count from the total. It returns an error result on failure.
func (s *Server) loadFeedItemWithReplies(
	ctx context.Context, item *models.FeedItem, userID, teamID string,
) (*feedItemWithReplies, *mcp.CallToolResult) {
	replyResp, err := s.container.FeedItemReplyService().ListReplies(
		ctx, userID, teamID, item.ID, 1, getFeedItemRepliesLimit,
	)
	if err != nil {
		slog.Error(
			"Failed to load replies for feed item via MCP",
			"tool", "vibexp_io_get_feed_item",
			"user_id", userID,
			"team_id", teamID,
			"feed_item_id", item.ID,
			"error", fmt.Sprintf("%+v", err),
		)
		return nil, &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to get feed item replies: %v", err)},
			},
			IsError: true,
		}
	}

	replies := replyResp.Replies
	if replies == nil {
		replies = make([]models.FeedItemReply, 0)
	}
	item.ReplyCount = replyResp.TotalCount

	return &feedItemWithReplies{
		FeedItem:         item,
		Replies:          replies,
		RepliesTruncated: replyResp.TotalCount > len(replies),
	}, nil
}

// addFeedTools registers the feed MCP tools. Each handler resolves the per-call
// team_id (UUID or slug) and validates membership before any service call.
//
//nolint:funlen // Registers multiple feed tools; each registration is a few lines; cannot be meaningfully split
func (m *MCPToolsManager) addFeedTools(mcpServer *mcp.Server, userID string) {
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name: "vibexp_io_list_feeds",
		Description: "List all AI Feeds available in the specified VibeXP team. Use this **before** calling " +
			"`vibexp_io_post_to_feed` to discover the feed_id you need. Each feed represents a topical " +
			"channel where AI assistants publish status updates, summaries, and reports for human team " +
			"members to read later. Returns the feed id (UUID), name, description, and last_post_at " +
			"timestamp so you can pick the most active or topic-appropriate feed. Paginated, max 10 per page.",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *ListFeedsParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.listFeeds(ctx, req, params, userID)
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name: "vibexp_io_post_to_feed",
		Description: "Post a status update, summary, or report to a VibeXP AI Feed so the human team " +
			"can read it later. Use this when you've completed a meaningful chunk of work, generated a " +
			"useful summary, or have an update worth sharing asynchronously — anything you'd otherwise " +
			"put in chat. **Do not use for finished, polished, reusable artifacts** (use the artifact " +
			"creation tool for those). Call `vibexp_io_list_feeds` first to find the right feed_id. " +
			"Content is rendered as Markdown to the user; code blocks, tables, and links are supported. " +
			"Returns { id, full_url } — no echoed message content.",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *PostToFeedParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.postToFeed(ctx, req, params, userID)
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name: "vibexp_io_reply_to_feed_item",
		Description: "Post a reply to an existing AI Feed item. Use this to add follow-up updates, " +
			"progress notes, or responses to a specific feed item. Call vibexp_io_list_feeds first to " +
			"find the right feed, then use the feed item id. Returns { id, full_url }.",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *ReplyToFeedItemParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.replyToFeedItem(ctx, req, params, userID)
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name: "vibexp_io_list_feed_items",
		Description: "List items posted to a specific AI Feed, newest first (paginated, max 10 per page). " +
			"Call vibexp_io_list_feeds first to discover the feed_id. By default returns id, title, excerpt, " +
			"ai_assistant_name, posted_at, and reply_count — content is omitted. " +
			"Set full_details=true to include the full content field, or include_replies=true to embed up to " +
			"3 recent reply excerpts per item. For one item plus all its replies, use vibexp_io_get_feed_item.",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *ListFeedItemsParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.listFeedItems(ctx, req, params, userID)
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name: "vibexp_io_get_feed_item",
		Description: "Get a single feed item with its full content and its replies (full content) inline, " +
			"newest replies first. Use after vibexp_io_list_feed_items when you need the full body of a " +
			"specific item and the conversation on it — e.g. to check for human replies before continuing work.",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *GetFeedItemParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.getFeedItem(ctx, req, params, userID)
	})
}
