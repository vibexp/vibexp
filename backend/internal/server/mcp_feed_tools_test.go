package server

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

// newMCPFeedTestServer creates a server whose feed services + team service are mocked,
// with the member user belonging to testTeamUUID/testTeamSlug so resolveTeam succeeds.
// Pass non-nil service mocks for the services a given test exercises.
func newMCPFeedTestServer(
	t *testing.T,
	feedSvc services.FeedServiceInterface,
	feedItemSvc services.FeedItemServiceInterface,
	feedItemReplySvc services.FeedItemReplyServiceInterface,
) *Server {
	t.Helper()
	srv := newServerWithNullLogger(t)
	srv.config.Frontend.BaseURL = "https://app.vibexp.io"
	mockTeamService := mocks.NewMockTeamServiceInterface(t)
	stubUserTeams(mockTeamService, []models.Team{memberTeam()})
	srv.container = &TestContainer{
		FeedServiceMock:          feedSvc,
		FeedItemServiceMock:      feedItemSvc,
		FeedItemReplyServiceMock: feedItemReplySvc,
		ResourceUsageServiceMock: newAllowedResourceUsageMock(t),
		TeamServiceMock:          mockTeamService,
	}
	return srv
}

const feedUUID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

// ─── vibexp_io_list_feeds ────────────────────────────────────────────────────

//nolint:funlen // Comprehensive happy-path assertions
func TestListFeeds_HappyPath(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	lastPost := now.Add(-1 * time.Hour)

	mockFeedSvc := mocks.NewMockFeedServiceInterface(t)
	srv := newMCPFeedTestServer(t, mockFeedSvc, nil, nil)

	desc := "daily engineering updates"
	expected := &models.MCPFeedListResponse{
		Feeds: []models.FeedWithLastPost{
			{ID: "feed-uuid-1", TeamID: testTeamUUID, Name: "Engineering Daily", Description: &desc, LastPostAt: &lastPost},
			{ID: "feed-uuid-2", TeamID: testTeamUUID, Name: "Security Updates"},
		},
	}
	mockFeedSvc.On(
		"ListFeedsForMCP", mock.Anything, testMemberUserID,
		mock.MatchedBy(func(f services.FeedFilters) bool {
			return f.TeamID == testTeamUUID && f.Page == 1 && f.Limit == 10
		}),
	).Return(expected, nil)

	params := &ListFeedsParams{TeamID: testTeamSlug}
	result, structured, err := srv.listFeeds(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	resp, ok := structured.(*models.MCPFeedListResponse)
	assert.True(t, ok)
	assert.Len(t, resp.Feeds, 2)
	assert.Equal(t, "feed-uuid-1", resp.Feeds[0].ID)
}

func TestListFeeds_EmptyList(t *testing.T) {
	mockFeedSvc := mocks.NewMockFeedServiceInterface(t)
	srv := newMCPFeedTestServer(t, mockFeedSvc, nil, nil)

	mockFeedSvc.On("ListFeedsForMCP", mock.Anything, testMemberUserID, mock.AnythingOfType("services.FeedFilters")).
		Return(&models.MCPFeedListResponse{Feeds: []models.FeedWithLastPost{}}, nil)

	params := &ListFeedsParams{TeamID: testTeamUUID}
	result, structured, err := srv.listFeeds(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	resp := structured.(*models.MCPFeedListResponse)
	assert.NotNil(t, resp.Feeds)
	assert.Empty(t, resp.Feeds)
	textContent := result.Content[0].(*mcp.TextContent)
	assert.Contains(t, textContent.Text, `"feeds": []`)
}

func TestListFeeds_ServiceError(t *testing.T) {
	mockFeedSvc := mocks.NewMockFeedServiceInterface(t)
	srv := newMCPFeedTestServer(t, mockFeedSvc, nil, nil)

	mockFeedSvc.On("ListFeedsForMCP", mock.Anything, testMemberUserID, mock.AnythingOfType("services.FeedFilters")).
		Return(nil, errors.New("db timeout"))

	params := &ListFeedsParams{TeamID: testTeamUUID}
	result, structured, err := srv.listFeeds(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Nil(t, structured)
}

func TestListFeeds_NonMemberTeamDenied(t *testing.T) {
	mockFeedSvc := mocks.NewMockFeedServiceInterface(t)
	srv := newMCPFeedTestServer(t, mockFeedSvc, nil, nil)

	params := &ListFeedsParams{TeamID: testOtherTeamUUID}
	result, structured, err := srv.listFeeds(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.Nil(t, structured)
	assertGenericAccessDenied(t, result)
	mockFeedSvc.AssertNotCalled(t, "ListFeedsForMCP")
}

func TestListFeeds_MissingTeamID(t *testing.T) {
	mockFeedSvc := mocks.NewMockFeedServiceInterface(t)
	srv := newMCPFeedTestServer(t, mockFeedSvc, nil, nil)

	result, _, err := srv.listFeeds(context.Background(), nil, &ListFeedsParams{}, testMemberUserID)

	assert.NoError(t, err)
	text := extractText(t, result)
	assert.Contains(t, text, "team_id is required")
	assert.Contains(t, text, "vibexp_io_list_teams")
	mockFeedSvc.AssertNotCalled(t, "ListFeedsForMCP")
}

func TestListFeeds_PaginationCapped(t *testing.T) {
	mockFeedSvc := mocks.NewMockFeedServiceInterface(t)
	srv := newMCPFeedTestServer(t, mockFeedSvc, nil, nil)

	mockFeedSvc.On(
		"ListFeedsForMCP", mock.Anything, testMemberUserID,
		mock.MatchedBy(func(f services.FeedFilters) bool { return f.Limit == 10 && f.Page == 2 }),
	).Return(&models.MCPFeedListResponse{Feeds: []models.FeedWithLastPost{}}, nil)

	params := &ListFeedsParams{TeamID: testTeamUUID, Page: 2, Limit: 100}
	result, _, err := srv.listFeeds(context.Background(), nil, params, testMemberUserID)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
}

// ─── vibexp_io_post_to_feed ──────────────────────────────────────────────────

func TestPostToFeed_HappyPath(t *testing.T) {
	now := time.Now().UTC()
	mockFeedItemSvc := mocks.NewMockFeedItemServiceInterface(t)
	srv := newMCPFeedTestServer(t, nil, mockFeedItemSvc, nil)

	projectID := "11111111-2222-3333-4444-555555555555"
	createdItem := &models.FeedItem{
		ID: "item-uuid-123", TeamID: testTeamUUID, FeedID: feedUUID, PostedAt: now, ProjectID: &projectID,
	}

	mockFeedItemSvc.On(
		"CreateFeedItem", mock.Anything, testMemberUserID, testTeamUUID, feedUUID,
		mock.MatchedBy(func(req *models.CreateFeedItemRequest) bool {
			return req.Title == "Refactored auth" && req.ProjectID != nil && *req.ProjectID == projectID
		}),
	).Return(createdItem, nil)

	params := &PostToFeedParams{
		TeamID:          testTeamUUID,
		FeedID:          feedUUID,
		Title:           "Refactored auth",
		Content:         "## Summary",
		AIAssistantName: "Claude Code CLI",
		ProjectID:       &projectID,
	}
	result, structured, err := srv.postToFeed(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	resp := structured.(*postToFeedResponse)
	assert.Equal(t, "item-uuid-123", resp.ID)
	assert.Equal(t, "https://app.vibexp.io/feed-items/item-uuid-123", resp.FullURL)
}

func TestPostToFeed_NonMemberTeamDenied(t *testing.T) {
	mockFeedItemSvc := mocks.NewMockFeedItemServiceInterface(t)
	srv := newMCPFeedTestServer(t, nil, mockFeedItemSvc, nil)

	params := &PostToFeedParams{
		TeamID: testOtherTeamUUID, FeedID: feedUUID, Title: "t", Content: "c", AIAssistantName: "a",
	}
	result, structured, err := srv.postToFeed(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.Nil(t, structured)
	assertGenericAccessDenied(t, result)
	mockFeedItemSvc.AssertNotCalled(t, "CreateFeedItem")
}

func TestPostToFeed_InvalidFeedIDNotUUID(t *testing.T) {
	srv := newMCPFeedTestServer(t, nil, nil, nil)

	params := &PostToFeedParams{TeamID: testTeamUUID, FeedID: "not-a-uuid", Title: "t", Content: "c", AIAssistantName: "a"}
	result, structured, err := srv.postToFeed(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Nil(t, structured)
}

func TestPostToFeed_InvalidProjectIDNotUUID(t *testing.T) {
	srv := newMCPFeedTestServer(t, nil, nil, nil)

	badProjectID := "not-a-uuid"
	params := &PostToFeedParams{
		TeamID: testTeamUUID, FeedID: feedUUID, Title: "t", Content: "c", AIAssistantName: "a", ProjectID: &badProjectID,
	}
	result, structured, err := srv.postToFeed(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Nil(t, structured)
}

func TestPostToFeed_ServiceError(t *testing.T) {
	mockFeedItemSvc := mocks.NewMockFeedItemServiceInterface(t)
	srv := newMCPFeedTestServer(t, nil, mockFeedItemSvc, nil)

	mockFeedItemSvc.On(
		"CreateFeedItem", mock.Anything, testMemberUserID, testTeamUUID, feedUUID,
		mock.AnythingOfType("*models.CreateFeedItemRequest"),
	).Return(nil, errors.New("title exceeds maximum length"))

	params := &PostToFeedParams{TeamID: testTeamUUID, FeedID: feedUUID, Title: "t", Content: "c", AIAssistantName: "a"}
	result, structured, err := srv.postToFeed(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Nil(t, structured)
}

func TestPostToFeed_SlimResponseHasFullURL(t *testing.T) {
	mockFeedItemSvc := mocks.NewMockFeedItemServiceInterface(t)
	srv := newMCPFeedTestServer(t, nil, mockFeedItemSvc, nil)

	itemID := "cccccccc-dddd-eeee-ffff-aaaaaaaaaaaa"
	mockFeedItemSvc.On(
		"CreateFeedItem", mock.Anything, testMemberUserID, testTeamUUID, feedUUID,
		mock.AnythingOfType("*models.CreateFeedItemRequest"),
	).Return(&models.FeedItem{ID: itemID, FeedID: feedUUID}, nil)

	params := &PostToFeedParams{TeamID: testTeamUUID, FeedID: feedUUID, Title: "T", Content: "c", AIAssistantName: "a"}
	result, structured, err := srv.postToFeed(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	resp := structured.(*postToFeedResponse)
	assert.Equal(t, "https://app.vibexp.io/feed-items/"+itemID, resp.FullURL)
	textContent := result.Content[0].(*mcp.TextContent)
	assert.NotContains(t, textContent.Text, `"message"`)
}

// ─── vibexp_io_reply_to_feed_item ────────────────────────────────────────────

func TestReplyToFeedItem_SlimResponseHasFullURL(t *testing.T) {
	mockFeedItemReplySvc := mocks.NewMockFeedItemReplyServiceInterface(t)
	srv := newMCPFeedTestServer(t, nil, nil, mockFeedItemReplySvc)

	replyID := "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
	mockFeedItemReplySvc.On(
		"CreateReply", mock.Anything, testMemberUserID, testTeamUUID, feedUUID,
		mock.AnythingOfType("*models.CreateFeedItemReplyRequest"),
	).Return(&models.FeedItemReply{ID: replyID, FeedItemID: feedUUID}, nil)

	params := &ReplyToFeedItemParams{TeamID: testTeamUUID, FeedItemID: feedUUID, Content: "Reply", AIAssistantName: "a"}
	result, structured, err := srv.replyToFeedItem(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	resp := structured.(*replyToFeedItemResponse)
	assert.Equal(t, replyID, resp.ID)
	assert.Equal(t, "https://app.vibexp.io/feed-items/"+feedUUID, resp.FullURL)
}

func TestReplyToFeedItem_NonMemberTeamDenied(t *testing.T) {
	mockFeedItemReplySvc := mocks.NewMockFeedItemReplyServiceInterface(t)
	srv := newMCPFeedTestServer(t, nil, nil, mockFeedItemReplySvc)

	params := &ReplyToFeedItemParams{TeamID: testOtherTeamUUID, FeedItemID: feedUUID, Content: "x", AIAssistantName: "a"}
	result, structured, err := srv.replyToFeedItem(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.Nil(t, structured)
	assertGenericAccessDenied(t, result)
	mockFeedItemReplySvc.AssertNotCalled(t, "CreateReply")
}

func TestReplyToFeedItem_InvalidUUID(t *testing.T) {
	srv := newMCPFeedTestServer(t, nil, nil, nil)

	params := &ReplyToFeedItemParams{TeamID: testTeamUUID, FeedItemID: "not-a-uuid", Content: "x", AIAssistantName: "a"}
	result, structured, err := srv.replyToFeedItem(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Nil(t, structured)
	assert.Contains(t, extractText(t, result), "feed_item_id must be a valid UUID")
}

// ─── vibexp_io_list_feed_items ───────────────────────────────────────────────

//nolint:funlen // Comprehensive happy-path assertions
func TestListFeedItems_HappyPath(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	mockFeedItemSvc := mocks.NewMockFeedItemServiceInterface(t)
	srv := newMCPFeedTestServer(t, nil, mockFeedItemSvc, nil)

	items := []models.FeedItem{
		{
			ID: "item-uuid-1", TeamID: testTeamUUID, FeedID: feedUUID, Title: "Refactored auth",
			Content: "## Summary", Excerpt: "Refactored.", AIAssistantName: "Claude", PostedByUserID: testMemberUserID,
			PostedAt: now, ReplyCount: 2,
		},
	}
	expectedResp := &models.FeedItemListResponse{Items: items, TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1}

	mockFeedItemSvc.On(
		"ListFeedItems", mock.Anything, testMemberUserID,
		mock.MatchedBy(func(f services.FeedItemFilters) bool {
			return f.TeamID == testTeamUUID && f.FeedID != nil && *f.FeedID == feedUUID && f.Page == 1 && f.Limit == 10
		}),
	).Return(expectedResp, nil)

	enriched := make([]models.FeedItem, len(items))
	copy(enriched, items)
	enriched[0].ReplyCount = 3
	mockFeedItemSvc.On("EnrichWithReplyCounts", mock.Anything, testTeamUUID, items).Return(enriched, nil)

	params := &ListFeedItemsParams{TeamID: testTeamUUID, FeedID: feedUUID, Page: 1, Limit: 10}
	result, structured, err := srv.listFeedItems(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	resp, ok := structured.(*feedItemExcerptListResponse)
	assert.True(t, ok)
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, 3, resp.Items[0].ReplyCount)
	textContent := result.Content[0].(*mcp.TextContent)
	assert.NotContains(t, textContent.Text, `"content"`)
}

//nolint:funlen // Comprehensive full_details=true assertions
func TestListFeedItems_FullDetails(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	mockFeedItemSvc := mocks.NewMockFeedItemServiceInterface(t)
	srv := newMCPFeedTestServer(t, nil, mockFeedItemSvc, nil)

	items := []models.FeedItem{
		{ID: "item-uuid-1", TeamID: testTeamUUID, FeedID: feedUUID, Title: "T", Content: "## Body", PostedAt: now},
	}
	expectedResp := &models.FeedItemListResponse{Items: items, TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1}

	mockFeedItemSvc.On(
		"ListFeedItems", mock.Anything, testMemberUserID,
		mock.MatchedBy(func(f services.FeedItemFilters) bool { return f.TeamID == testTeamUUID }),
	).Return(expectedResp, nil)
	enriched := make([]models.FeedItem, len(items))
	copy(enriched, items)
	mockFeedItemSvc.On("EnrichWithReplyCounts", mock.Anything, testTeamUUID, items).Return(enriched, nil)

	params := &ListFeedItemsParams{TeamID: testTeamUUID, FeedID: feedUUID, Page: 1, Limit: 10, FullDetails: true}
	result, structured, err := srv.listFeedItems(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	resp, ok := structured.(*models.FeedItemListResponse)
	assert.True(t, ok)
	assert.Equal(t, "## Body", resp.Items[0].Content)
	textContent := result.Content[0].(*mcp.TextContent)
	assert.Contains(t, textContent.Text, "content")
}

func TestListFeedItems_NonMemberTeamDenied(t *testing.T) {
	mockFeedItemSvc := mocks.NewMockFeedItemServiceInterface(t)
	srv := newMCPFeedTestServer(t, nil, mockFeedItemSvc, nil)

	params := &ListFeedItemsParams{TeamID: testOtherTeamUUID, FeedID: feedUUID}
	result, structured, err := srv.listFeedItems(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.Nil(t, structured)
	assertGenericAccessDenied(t, result)
	mockFeedItemSvc.AssertNotCalled(t, "ListFeedItems")
}

func TestListFeedItems_InvalidUUID(t *testing.T) {
	srv := newMCPFeedTestServer(t, nil, nil, nil)

	params := &ListFeedItemsParams{TeamID: testTeamUUID, FeedID: "not-a-uuid"}
	result, structured, err := srv.listFeedItems(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Nil(t, structured)
	assert.Contains(t, extractText(t, result), "feed_id must be a valid UUID")
}

// ─── vibexp_io_get_feed_item ─────────────────────────────────────────────────

func TestGetFeedItem_HappyPath(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	mockFeedItemSvc := mocks.NewMockFeedItemServiceInterface(t)
	mockReplySvc := mocks.NewMockFeedItemReplyServiceInterface(t)
	srv := newMCPFeedTestServer(t, nil, mockFeedItemSvc, mockReplySvc)

	expectedItem := &models.FeedItem{
		ID: feedUUID, TeamID: testTeamUUID, Content: "## Full body", PostedAt: now,
	}
	mockFeedItemSvc.On("GetFeedItem", mock.Anything, testMemberUserID, testTeamUUID, feedUUID).Return(expectedItem, nil)
	// get_feed_item embeds the item's replies (full content) inline and derives
	// reply_count from the total.
	mockReplySvc.On("ListReplies", mock.Anything, testMemberUserID, testTeamUUID, feedUUID, 1, getFeedItemRepliesLimit).
		Return(&models.FeedItemReplyListResponse{
			Replies:    []models.FeedItemReply{{ID: "r1", Content: "a human reply", PostedAt: now}},
			TotalCount: 1, Page: 1, PerPage: getFeedItemRepliesLimit, TotalPages: 1,
		}, nil)

	params := &GetFeedItemParams{TeamID: testTeamUUID, FeedItemID: feedUUID}
	result, structured, err := srv.getFeedItem(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	res := structured.(*feedItemWithReplies)
	assert.Equal(t, "## Full body", res.Content)
	assert.Equal(t, 1, res.ReplyCount)
	assert.False(t, res.RepliesTruncated)
	assert.Len(t, res.Replies, 1)
	assert.Equal(t, "a human reply", res.Replies[0].Content)
}

func TestGetFeedItem_NonMemberTeamDenied(t *testing.T) {
	mockFeedItemSvc := mocks.NewMockFeedItemServiceInterface(t)
	srv := newMCPFeedTestServer(t, nil, mockFeedItemSvc, nil)

	params := &GetFeedItemParams{TeamID: testOtherTeamUUID, FeedItemID: feedUUID}
	result, structured, err := srv.getFeedItem(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.Nil(t, structured)
	assertGenericAccessDenied(t, result)
	mockFeedItemSvc.AssertNotCalled(t, "GetFeedItem")
}

func TestGetFeedItem_InvalidUUID(t *testing.T) {
	srv := newMCPFeedTestServer(t, nil, nil, nil)

	params := &GetFeedItemParams{TeamID: testTeamUUID, FeedItemID: "not-a-uuid"}
	result, structured, err := srv.getFeedItem(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Nil(t, structured)
	assert.Contains(t, extractText(t, result), "feed_item_id must be a valid UUID")
}

func TestGetFeedItem_RepliesTruncated(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	mockFeedItemSvc := mocks.NewMockFeedItemServiceInterface(t)
	mockReplySvc := mocks.NewMockFeedItemReplyServiceInterface(t)
	srv := newMCPFeedTestServer(t, nil, mockFeedItemSvc, mockReplySvc)

	mockFeedItemSvc.On("GetFeedItem", mock.Anything, testMemberUserID, testTeamUUID, feedUUID).
		Return(&models.FeedItem{ID: feedUUID, TeamID: testTeamUUID, PostedAt: now}, nil)
	// Embedded page holds 1 reply but the item has 5 total → replies_truncated.
	mockReplySvc.On("ListReplies", mock.Anything, testMemberUserID, testTeamUUID, feedUUID, 1, getFeedItemRepliesLimit).
		Return(&models.FeedItemReplyListResponse{
			Replies:    []models.FeedItemReply{{ID: "r1", Content: "one", PostedAt: now}},
			TotalCount: 5, Page: 1, PerPage: getFeedItemRepliesLimit, TotalPages: 1,
		}, nil)

	params := &GetFeedItemParams{TeamID: testTeamUUID, FeedItemID: feedUUID}
	_, structured, err := srv.getFeedItem(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	res := structured.(*feedItemWithReplies)
	assert.Equal(t, 5, res.ReplyCount)
	assert.True(t, res.RepliesTruncated)
}

// TestListFeedItems_IncludeReplies verifies include_replies embeds bounded reply
// excerpts per item and returns the excerpt-with-replies list shape.
func TestListFeedItems_IncludeReplies(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	mockFeedItemSvc := mocks.NewMockFeedItemServiceInterface(t)
	mockReplySvc := mocks.NewMockFeedItemReplyServiceInterface(t)
	srv := newMCPFeedTestServer(t, nil, mockFeedItemSvc, mockReplySvc)

	items := []models.FeedItem{
		{ID: "item-1", TeamID: testTeamUUID, FeedID: feedUUID, Title: "T", Content: "## Summary", Excerpt: "exc", PostedAt: now},
	}
	mockFeedItemSvc.On("ListFeedItems", mock.Anything, testMemberUserID, mock.Anything).
		Return(&models.FeedItemListResponse{Items: items, TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1}, nil)

	// Enrichment gives the item a non-zero reply_count, so its replies are fetched.
	enriched := []models.FeedItem{items[0]}
	enriched[0].ReplyCount = 2
	mockFeedItemSvc.On("EnrichWithReplyCounts", mock.Anything, testTeamUUID, items).Return(enriched, nil)
	mockReplySvc.On("ListReplies", mock.Anything, testMemberUserID, testTeamUUID, "item-1", 1, listFeedItemRepliesEmbedLimit).
		Return(&models.FeedItemReplyListResponse{
			Replies:    []models.FeedItemReply{{ID: "r1", Content: "reply body", PostedAt: now}},
			TotalCount: 2, Page: 1, PerPage: listFeedItemRepliesEmbedLimit, TotalPages: 1,
		}, nil)

	params := &ListFeedItemsParams{TeamID: testTeamUUID, FeedID: feedUUID, Page: 1, Limit: 10, IncludeReplies: true}
	result, structured, err := srv.listFeedItems(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	resp, ok := structured.(*feedItemExcerptWithRepliesListResponse)
	assert.True(t, ok)
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, 2, resp.Items[0].ReplyCount)
	assert.Len(t, resp.Items[0].Replies, 1)
	assert.Equal(t, "reply body", resp.Items[0].Replies[0].Content)
}

// ─── schema reflection regression guard ──────────────────────────────────────

// TestAddFeedTools_SchemaReflectionNoPanic guards against the jsonschema reflector
// panicking on struct tag syntax. AddTool triggers reflection eagerly.
func TestAddFeedTools_SchemaReflectionNoPanic(t *testing.T) {
	srv := newServerWithNullLogger(t)
	srv.container = &TestContainer{}

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.1"}, nil)
	manager := NewMCPToolsManager(srv)

	assert.NotPanics(t, func() {
		manager.addFeedTools(mcpServer, "user-id")
	})
}

// TestPostToFeed_JSONShape verifies the slim response marshals to the documented shape.
func TestPostToFeed_JSONShape(t *testing.T) {
	mockFeedItemSvc := mocks.NewMockFeedItemServiceInterface(t)
	srv := newMCPFeedTestServer(t, nil, mockFeedItemSvc, nil)

	mockFeedItemSvc.On(
		"CreateFeedItem", mock.Anything, testMemberUserID, testTeamUUID, feedUUID,
		mock.AnythingOfType("*models.CreateFeedItemRequest"),
	).Return(&models.FeedItem{ID: "item-1", FeedID: feedUUID}, nil)

	params := &PostToFeedParams{TeamID: testTeamUUID, FeedID: feedUUID, Title: "T", Content: "c", AIAssistantName: "a"}
	result, _, err := srv.postToFeed(context.Background(), nil, params, testMemberUserID)
	assert.NoError(t, err)

	var parsed postToFeedResponse
	assert.NoError(t, json.Unmarshal([]byte(extractText(t, result)), &parsed))
	assert.Equal(t, "item-1", parsed.ID)
}
