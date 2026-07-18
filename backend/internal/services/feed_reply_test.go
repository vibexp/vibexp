package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repoMocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	svcMocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/pkg/events"
	eventMocks "github.com/vibexp/vibexp/pkg/events/mocks"
)

// ─────────────────────────────────────────────────────────────────────────────
// FeedItemReplyService tests
// ─────────────────────────────────────────────────────────────────────────────

// createReplyCase is one CreateReply table case.
type createReplyCase struct {
	name        string
	userID      string
	teamID      string
	feedItemID  string
	req         *models.CreateFeedItemReplyRequest
	isMember    bool
	memberErr   error
	feedItem    *models.FeedItem
	feedItemErr error
	replyErr    error
	wantErr     bool
	errContains string
}

// setupCreateReplyMocks wires the membership, feed-item-lookup and
// reply-creation expectations one CreateReply table case needs to reach its
// outcome.
func setupCreateReplyMocks(
	ctx context.Context,
	tt createReplyCase,
	mockTeamSvc *svcMocks.MockTeamServiceInterface,
	mockFeedItemRepo *repoMocks.MockFeedItemRepository,
	mockReplyRepo *repoMocks.MockFeedItemReplyRepository,
) {
	// Team membership check
	switch {
	case tt.memberErr != nil:
		mockTeamSvc.On("IsUserMemberOfTeam", ctx, tt.userID, tt.teamID).
			Return(false, tt.memberErr)
	case !tt.isMember && tt.feedItemErr == nil:
		mockTeamSvc.On("IsUserMemberOfTeam", ctx, tt.userID, tt.teamID).
			Return(false, nil)
	case tt.isMember:
		mockTeamSvc.On("IsUserMemberOfTeam", ctx, tt.userID, tt.teamID).
			Return(true, nil)
	}

	// Feed item lookup (only when member and memberErr is nil)
	if tt.isMember && tt.memberErr == nil {
		if tt.feedItemErr != nil {
			mockFeedItemRepo.On("GetByID", ctx, tt.userID, tt.teamID, tt.feedItemID).
				Return((*models.FeedItem)(nil), tt.feedItemErr)
		} else if tt.feedItem != nil {
			mockFeedItemRepo.On("GetByID", ctx, tt.userID, tt.teamID, tt.feedItemID).
				Return(tt.feedItem, nil)
		}
	}

	// Reply creation (only when item is active and no error)
	itemReachable := tt.isMember && tt.memberErr == nil &&
		tt.feedItemErr == nil && tt.feedItem != nil && tt.feedItem.ArchivedAt == nil
	if itemReachable {
		if tt.replyErr != nil {
			mockReplyRepo.On("CreateReply", ctx, mock.AnythingOfType("*models.FeedItemReply")).
				Return((*models.FeedItemReply)(nil), tt.replyErr)
		} else {
			mockReplyRepo.On("CreateReply", ctx, mock.AnythingOfType("*models.FeedItemReply")).
				Return(&models.FeedItemReply{
					ID:         "reply-1",
					TeamID:     tt.teamID,
					FeedItemID: tt.feedItemID,
					Content:    tt.req.Content,
					PostedAt:   time.Now(),
				}, nil)
		}
	}
}

func TestFeedItemReplyService_CreateReply(t *testing.T) {
	archivedAt := time.Now()
	assistantName := "claude-test"
	tests := []createReplyCase{
		{
			name:       "success",
			userID:     "user-1",
			teamID:     "team-1",
			feedItemID: "item-1",
			req: &models.CreateFeedItemReplyRequest{
				Content:         "This is a reply",
				AIAssistantName: &assistantName,
			},
			isMember: true,
			feedItem: &models.FeedItem{
				ID:         "item-1",
				TeamID:     "team-1",
				ArchivedAt: nil,
			},
		},
		{
			name:       "archived item rejected",
			userID:     "user-1",
			teamID:     "team-1",
			feedItemID: "item-1",
			req:        &models.CreateFeedItemReplyRequest{Content: "reply"},
			isMember:   true,
			feedItem: &models.FeedItem{
				ID:         "item-1",
				TeamID:     "team-1",
				ArchivedAt: &archivedAt,
			},
			wantErr:     true,
			errContains: "feed item is archived",
		},
		{
			name:        "not a team member",
			userID:      "user-1",
			teamID:      "team-1",
			feedItemID:  "item-1",
			req:         &models.CreateFeedItemReplyRequest{Content: "reply"},
			isMember:    false,
			wantErr:     true,
			errContains: "user is not a member of the specified team",
		},
		{
			name:        "team membership check fails",
			userID:      "user-1",
			teamID:      "team-1",
			feedItemID:  "item-1",
			req:         &models.CreateFeedItemReplyRequest{Content: "reply"},
			isMember:    false,
			memberErr:   errors.New("db error"),
			wantErr:     true,
			errContains: "failed to validate team membership",
		},
		{
			name:        "content is empty",
			userID:      "user-1",
			teamID:      "team-1",
			feedItemID:  "item-1",
			req:         &models.CreateFeedItemReplyRequest{Content: ""},
			wantErr:     true,
			errContains: "content is required",
		},
		{
			name:        "feed item not found",
			userID:      "user-1",
			teamID:      "team-1",
			feedItemID:  "item-missing",
			req:         &models.CreateFeedItemReplyRequest{Content: "reply"},
			isMember:    true,
			feedItemErr: errors.New("feed item not found"),
			wantErr:     true,
			errContains: "feed item not found",
		},
		{
			name:       "repo create fails",
			userID:     "user-1",
			teamID:     "team-1",
			feedItemID: "item-1",
			req:        &models.CreateFeedItemReplyRequest{Content: "reply"},
			isMember:   true,
			feedItem: &models.FeedItem{
				ID:         "item-1",
				TeamID:     "team-1",
				ArchivedAt: nil,
			},
			replyErr:    errors.New("db error"),
			wantErr:     true,
			errContains: "db error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockReplyRepo := repoMocks.NewMockFeedItemReplyRepository(t)
			mockFeedItemRepo := repoMocks.NewMockFeedItemRepository(t)
			mockTeamSvc := svcMocks.NewMockTeamServiceInterface(t)
			logger := newTestLogger()

			svc := services.NewFeedItemReplyService(mockReplyRepo, mockFeedItemRepo, mockTeamSvc, permissiveAuthz(t), nil, logger)

			ctx := context.Background()

			// Empty content — no service calls needed
			if tt.req.Content == "" {
				_, err := svc.CreateReply(ctx, tt.userID, tt.teamID, tt.feedItemID, tt.req)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}

			setupCreateReplyMocks(ctx, tt, mockTeamSvc, mockFeedItemRepo, mockReplyRepo)

			reply, err := svc.CreateReply(ctx, tt.userID, tt.teamID, tt.feedItemID, tt.req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, reply)
				assert.Equal(t, tt.req.Content, reply.Content)
			}
		})
	}
}

// listRepliesCase is one ListReplies table case.
type listRepliesCase struct {
	name        string
	userID      string
	teamID      string
	feedItemID  string
	page        int
	limit       int
	isMember    bool
	memberErr   error
	replies     []models.FeedItemReply
	total       int
	repoErr     error
	wantErr     bool
	errContains string
	wantPage    int
	wantLimit   int
}

// setupListRepliesMocks wires the membership and repository expectations one
// ListReplies table case needs, mirroring the service's pagination defaults.
func setupListRepliesMocks(
	ctx context.Context,
	tt listRepliesCase,
	mockTeamSvc *svcMocks.MockTeamServiceInterface,
	mockReplyRepo *repoMocks.MockFeedItemReplyRepository,
) {
	if tt.memberErr != nil {
		mockTeamSvc.On("IsUserMemberOfTeam", ctx, tt.userID, tt.teamID).
			Return(false, tt.memberErr)
	} else {
		mockTeamSvc.On("IsUserMemberOfTeam", ctx, tt.userID, tt.teamID).
			Return(tt.isMember, nil)
	}

	if tt.isMember && tt.memberErr == nil {
		expectedPage := tt.page
		if expectedPage <= 0 {
			expectedPage = 1
		}
		expectedLimit := tt.limit
		if expectedLimit <= 0 {
			expectedLimit = 20
		} else if expectedLimit > 100 {
			expectedLimit = 100
		}

		if tt.repoErr != nil {
			mockReplyRepo.On("ListReplies", ctx, tt.teamID, tt.feedItemID, expectedPage, expectedLimit).
				Return(nil, 0, tt.repoErr)
		} else {
			mockReplyRepo.On("ListReplies", ctx, tt.teamID, tt.feedItemID, expectedPage, expectedLimit).
				Return(tt.replies, tt.total, nil)
		}
	}
}

func TestFeedItemReplyService_ListReplies(t *testing.T) {
	tests := []listRepliesCase{
		{
			name:       "success with two replies",
			userID:     "user-1",
			teamID:     "team-1",
			feedItemID: "item-1",
			page:       1,
			limit:      10,
			isMember:   true,
			replies: []models.FeedItemReply{
				{ID: "r1", Content: "first reply"},
				{ID: "r2", Content: "second reply"},
			},
			total:     2,
			wantPage:  1,
			wantLimit: 10,
		},
		{
			name:        "not a team member",
			userID:      "user-1",
			teamID:      "team-1",
			feedItemID:  "item-1",
			page:        1,
			limit:       10,
			isMember:    false,
			wantErr:     true,
			errContains: "user is not a member of the specified team",
		},
		{
			name:       "pagination defaults applied",
			userID:     "user-1",
			teamID:     "team-1",
			feedItemID: "item-1",
			page:       0,
			limit:      0,
			isMember:   true,
			replies:    []models.FeedItemReply{},
			total:      0,
			wantPage:   1,
			wantLimit:  20,
		},
		{
			name:       "limit capped at 100",
			userID:     "user-1",
			teamID:     "team-1",
			feedItemID: "item-1",
			page:       1,
			limit:      200,
			isMember:   true,
			replies:    []models.FeedItemReply{},
			total:      0,
			wantPage:   1,
			wantLimit:  100,
		},
		{
			name:        "repo error",
			userID:      "user-1",
			teamID:      "team-1",
			feedItemID:  "item-1",
			page:        1,
			limit:       10,
			isMember:    true,
			repoErr:     errors.New("db failure"),
			wantErr:     true,
			errContains: "db failure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockReplyRepo := repoMocks.NewMockFeedItemReplyRepository(t)
			mockFeedItemRepo := repoMocks.NewMockFeedItemRepository(t)
			mockTeamSvc := svcMocks.NewMockTeamServiceInterface(t)
			logger := newTestLogger()

			svc := services.NewFeedItemReplyService(mockReplyRepo, mockFeedItemRepo, mockTeamSvc, permissiveAuthz(t), nil, logger)
			ctx := context.Background()

			setupListRepliesMocks(ctx, tt, mockTeamSvc, mockReplyRepo)

			result, err := svc.ListReplies(ctx, tt.userID, tt.teamID, tt.feedItemID, tt.page, tt.limit)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.total, result.TotalCount)
				assert.Equal(t, tt.wantPage, result.Page)
				assert.Equal(t, tt.wantLimit, result.PerPage)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// FeedItemService.EnrichWithReplyCounts tests
// ─────────────────────────────────────────────────────────────────────────────

// enrichReplyCountsCase is one EnrichWithReplyCounts table case.
type enrichReplyCountsCase struct {
	name    string
	teamID  string
	items   []models.FeedItem
	counts  map[string]int
	repoErr error
	wantErr bool
}

// setupReplyCountMocks wires the CountRepliesByItemIDs expectation one
// EnrichWithReplyCounts table case needs (none when there are no items).
func setupReplyCountMocks(
	ctx context.Context, tt enrichReplyCountsCase, mockReplyRepo *repoMocks.MockFeedItemReplyRepository,
) {
	if len(tt.items) == 0 {
		return
	}
	itemIDs := make([]string, len(tt.items))
	for i, item := range tt.items {
		itemIDs[i] = item.ID
	}

	if tt.repoErr != nil {
		mockReplyRepo.On("CountRepliesByItemIDs", ctx, tt.teamID, mock.AnythingOfType("[]string")).
			Return((map[string]int)(nil), tt.repoErr)
	} else {
		mockReplyRepo.On("CountRepliesByItemIDs", ctx, tt.teamID, mock.AnythingOfType("[]string")).
			Return(tt.counts, nil)
	}
}

// assertEnrichedReplyCounts verifies one EnrichWithReplyCounts table case
// outcome: the propagated error, the untouched empty slice, or the annotated
// per-item reply counts.
func assertEnrichedReplyCounts(t *testing.T, tt enrichReplyCountsCase, result []models.FeedItem, err error) {
	t.Helper()
	if tt.wantErr {
		require.Error(t, err)
		return
	}
	require.NoError(t, err)
	if len(tt.items) == 0 {
		assert.Equal(t, tt.items, result)
		return
	}
	for _, item := range result {
		assert.Equal(t, tt.counts[item.ID], item.ReplyCount)
	}
}

func TestFeedItemService_EnrichWithReplyCounts(t *testing.T) {
	tests := []enrichReplyCountsCase{
		{
			name:   "annotates items with counts",
			teamID: "team-1",
			items: []models.FeedItem{
				{ID: "item-1"},
				{ID: "item-2"},
				{ID: "item-3"},
			},
			counts: map[string]int{
				"item-1": 3,
				"item-2": 0,
				"item-3": 7,
			},
		},
		{
			name:   "empty items returns early",
			teamID: "team-1",
			items:  []models.FeedItem{},
		},
		{
			name:    "repo error propagated",
			teamID:  "team-1",
			items:   []models.FeedItem{{ID: "item-1"}},
			repoErr: errors.New("db failure"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockFeedItemRepo := repoMocks.NewMockFeedItemRepository(t)
			mockReplyRepo := repoMocks.NewMockFeedItemReplyRepository(t)
			logger := newTestLogger()

			svc := services.NewFeedItemService(mockFeedItemRepo, mockReplyRepo, nil, nil, permissiveAuthz(t), nil, logger)
			ctx := context.Background()

			setupReplyCountMocks(ctx, tt, mockReplyRepo)

			result, err := svc.EnrichWithReplyCounts(ctx, tt.teamID, tt.items)

			assertEnrichedReplyCounts(t, tt, result, err)
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// FeedItemService.GetFeedItem — single-item reply-count enrichment (#101)
// ─────────────────────────────────────────────────────────────────────────────

//nolint:funlen // Table-driven test with multiple cases
func TestFeedItemService_GetFeedItem_EnrichesReplyCount(t *testing.T) {
	const (
		userID = "user-1"
		teamID = "team-1"
		itemID = "item-1"
	)

	tests := []struct {
		name      string
		count     int
		getErr    error
		countErr  error
		wantCount int
		wantErr   bool
	}{
		{name: "item with replies is enriched", count: 3, wantCount: 3},
		{name: "item with no replies stays zero", count: 0, wantCount: 0},
		{name: "get error propagated", getErr: errors.New("not found"), wantErr: true},
		{name: "reply-count error propagated", countErr: errors.New("db failure"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockFeedItemRepo := repoMocks.NewMockFeedItemRepository(t)
			mockReplyRepo := repoMocks.NewMockFeedItemReplyRepository(t)
			svc := services.NewFeedItemService(mockFeedItemRepo, mockReplyRepo, nil, nil, permissiveAuthz(t), nil, newTestLogger())
			ctx := context.Background()

			if tt.getErr != nil {
				mockFeedItemRepo.On("GetByID", ctx, userID, teamID, itemID).
					Return((*models.FeedItem)(nil), tt.getErr)
			} else {
				mockFeedItemRepo.On("GetByID", ctx, userID, teamID, itemID).
					Return(&models.FeedItem{ID: itemID, TeamID: teamID}, nil)

				if tt.countErr != nil {
					mockReplyRepo.On("CountRepliesByItemIDs", ctx, teamID, mock.AnythingOfType("[]string")).
						Return((map[string]int)(nil), tt.countErr)
				} else {
					mockReplyRepo.On("CountRepliesByItemIDs", ctx, teamID, mock.AnythingOfType("[]string")).
						Return(map[string]int{itemID: tt.count}, nil)
				}
			}

			item, err := svc.GetFeedItem(ctx, userID, teamID, itemID)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, item)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, item)
			assert.Equal(t, tt.wantCount, item.ReplyCount)
		})
	}
}

// setupReplyCreateMocks wires the common mock expectations for a successful CreateReply:
// team membership passes, the parent item is active, and the repo returns a created reply.
func setupReplyCreateMocks(
	mockReplyRepo *repoMocks.MockFeedItemReplyRepository,
	mockFeedItemRepo *repoMocks.MockFeedItemRepository,
	mockTeamSvc *svcMocks.MockTeamServiceInterface,
) {
	mockTeamSvc.On("IsUserMemberOfTeam", mock.Anything, "poster-1", "team-1").Return(true, nil)
	mockFeedItemRepo.On("GetByID", mock.Anything, "poster-1", "team-1", "item-1").
		Return(&models.FeedItem{ID: "item-1", TeamID: "team-1"}, nil)
	mockReplyRepo.On("CreateReply", mock.Anything, mock.AnythingOfType("*models.FeedItemReply")).
		Return(&models.FeedItemReply{
			ID:             "reply-1",
			TeamID:         "team-1",
			FeedItemID:     "item-1",
			Content:        "A substantive reply",
			PostedByUserID: "poster-1",
			PostedAt:       time.Now(),
		}, nil)
}

// TestFeedItemReplyService_CreateReply_PublishesEvent asserts the reply created event
// carries the reply content and is keyed by the posting user (#1361).
func TestFeedItemReplyService_CreateReply_PublishesEvent(t *testing.T) {
	mockReplyRepo := repoMocks.NewMockFeedItemReplyRepository(t)
	mockFeedItemRepo := repoMocks.NewMockFeedItemRepository(t)
	mockTeamSvc := svcMocks.NewMockTeamServiceInterface(t)
	mockEvent := &eventMocks.MockEventPublisher{}

	svc := services.NewFeedItemReplyService(mockReplyRepo, mockFeedItemRepo, mockTeamSvc, permissiveAuthz(t), mockEvent, newTestLogger())
	setupReplyCreateMocks(mockReplyRepo, mockFeedItemRepo, mockTeamSvc)

	var published events.Event
	mockEvent.On("Publish", mock.Anything, mock.MatchedBy(func(e events.Event) bool {
		published = e
		return true
	})).Return(nil)

	req := &models.CreateFeedItemReplyRequest{Content: "A substantive reply"}
	got, err := svc.CreateReply(context.Background(), "poster-1", "team-1", "item-1", req)
	require.NoError(t, err)
	require.NotNil(t, got)

	require.NotNil(t, published)
	assert.Equal(t, events.EventTypeFeedItemReplyCreated, published.Type())
	assert.Equal(t, "poster-1", published.UserID(), "event must be keyed by the posting user")
	payload, ok := published.Payload().(*events.FeedItemReplyCreatedPayload)
	require.True(t, ok)
	assert.Equal(t, "reply-1", payload.ReplyID)
	assert.Equal(t, "poster-1", payload.UserID)
	assert.Equal(t, "item-1", payload.FeedItemID)
	assert.Equal(t, "A substantive reply", payload.Content)
	mockEvent.AssertExpectations(t)
}

// TestFeedItemReplyService_CreateReply_NilEventManagerSkipsPublish asserts CreateReply
// succeeds without publishing when no event manager is wired (mirrors CreateFeedItem).
func TestFeedItemReplyService_CreateReply_NilEventManagerSkipsPublish(t *testing.T) {
	mockReplyRepo := repoMocks.NewMockFeedItemReplyRepository(t)
	mockFeedItemRepo := repoMocks.NewMockFeedItemRepository(t)
	mockTeamSvc := svcMocks.NewMockTeamServiceInterface(t)

	svc := services.NewFeedItemReplyService(mockReplyRepo, mockFeedItemRepo, mockTeamSvc, permissiveAuthz(t), nil, newTestLogger())
	setupReplyCreateMocks(mockReplyRepo, mockFeedItemRepo, mockTeamSvc)

	req := &models.CreateFeedItemReplyRequest{Content: "A substantive reply"}
	got, err := svc.CreateReply(context.Background(), "poster-1", "team-1", "item-1", req)
	require.NoError(t, err)
	require.NotNil(t, got)
}

// TestFeedItemReplyService_ListReplyPosters asserts the service verifies team membership
// then returns the reply (id, poster) pairs used by the delete handler to clean up each
// reply's poster-keyed embedding row (#1361).
func TestFeedItemReplyService_ListReplyPosters(t *testing.T) {
	t.Run("returns posters after membership check", func(t *testing.T) {
		mockReplyRepo := repoMocks.NewMockFeedItemReplyRepository(t)
		mockFeedItemRepo := repoMocks.NewMockFeedItemRepository(t)
		mockTeamSvc := svcMocks.NewMockTeamServiceInterface(t)
		svc := services.NewFeedItemReplyService(mockReplyRepo, mockFeedItemRepo, mockTeamSvc, permissiveAuthz(t), nil, newTestLogger())

		mockTeamSvc.On("IsUserMemberOfTeam", mock.Anything, "user-1", "team-1").Return(true, nil)
		want := []repositories.FeedItemReplyPoster{
			{ReplyID: "reply-1", PostedByUserID: "poster-a"},
			{ReplyID: "reply-2", PostedByUserID: "poster-b"},
		}
		mockReplyRepo.On("ListReplyPostersByItemID", mock.Anything, "team-1", "item-1").Return(want, nil)

		got, err := svc.ListReplyPosters(context.Background(), "user-1", "team-1", "item-1")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("rejects non-member", func(t *testing.T) {
		mockReplyRepo := repoMocks.NewMockFeedItemReplyRepository(t)
		mockFeedItemRepo := repoMocks.NewMockFeedItemRepository(t)
		mockTeamSvc := svcMocks.NewMockTeamServiceInterface(t)
		svc := services.NewFeedItemReplyService(mockReplyRepo, mockFeedItemRepo, mockTeamSvc, permissiveAuthz(t), nil, newTestLogger())

		mockTeamSvc.On("IsUserMemberOfTeam", mock.Anything, "user-1", "team-1").Return(false, nil)

		got, err := svc.ListReplyPosters(context.Background(), "user-1", "team-1", "item-1")
		require.Error(t, err)
		assert.Nil(t, got)
	})

	t.Run("propagates repo error", func(t *testing.T) {
		mockReplyRepo := repoMocks.NewMockFeedItemReplyRepository(t)
		mockFeedItemRepo := repoMocks.NewMockFeedItemRepository(t)
		mockTeamSvc := svcMocks.NewMockTeamServiceInterface(t)
		svc := services.NewFeedItemReplyService(mockReplyRepo, mockFeedItemRepo, mockTeamSvc, permissiveAuthz(t), nil, newTestLogger())

		mockTeamSvc.On("IsUserMemberOfTeam", mock.Anything, "user-1", "team-1").Return(true, nil)
		mockReplyRepo.On("ListReplyPostersByItemID", mock.Anything, "team-1", "item-1").
			Return(([]repositories.FeedItemReplyPoster)(nil), errors.New("db error"))

		got, err := svc.ListReplyPosters(context.Background(), "user-1", "team-1", "item-1")
		require.Error(t, err)
		assert.Nil(t, got)
	})
}
