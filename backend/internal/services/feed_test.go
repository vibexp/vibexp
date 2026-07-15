package services_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
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

// helpers
func newTestLogger() *slog.Logger {
	l := slog.New(slog.DiscardHandler)
	return l
}

func strPtr(s string) *string { return &s }

// --------------------------------------------------------------------------
// FeedService tests
// --------------------------------------------------------------------------

//nolint:funlen // Table-driven test with multiple cases
func TestFeedService_CreateFeed(t *testing.T) {
	tests := []struct {
		name         string
		userID       string
		teamID       string
		req          *models.CreateFeedRequest
		isMember     bool
		memberErr    error
		repoErr      error
		wantErr      bool
		skipTeamCall bool // when teamService is nil
	}{
		{
			name:     "success",
			userID:   "user-1",
			teamID:   "team-1",
			req:      &models.CreateFeedRequest{Name: "My Feed", Description: strPtr("A test feed")},
			isMember: true,
		},
		{
			name:    "repo error",
			userID:  "user-1",
			teamID:  "team-1",
			req:     &models.CreateFeedRequest{Name: "Dupe Feed"},
			repoErr: errors.New("feed with name 'Dupe Feed' already exists for this team"),
			wantErr: true,

			isMember: true,
		},
		{
			name:    "not a team member",
			userID:  "outsider",
			teamID:  "team-1",
			req:     &models.CreateFeedRequest{Name: "Unauthorized Feed"},
			wantErr: true,

			isMember: false,
		},
		{
			name:      "team membership check fails",
			userID:    "user-1",
			teamID:    "team-1",
			req:       &models.CreateFeedRequest{Name: "Some Feed"},
			wantErr:   true,
			memberErr: errors.New("db error"),
		},
		{
			name:         "empty name rejected",
			userID:       "user-1",
			teamID:       "team-1",
			req:          &models.CreateFeedRequest{Name: ""},
			wantErr:      true,
			skipTeamCall: true,
		},
		{
			name:         "name too long rejected",
			userID:       "user-1",
			teamID:       "team-1",
			req:          &models.CreateFeedRequest{Name: strings.Repeat("a", 256)},
			wantErr:      true,
			skipTeamCall: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := repoMocks.NewMockFeedRepository(t)
			mockTeam := svcMocks.NewMockTeamServiceInterface(t)
			mockEvent := &eventMocks.MockEventPublisher{}

			svc := services.NewFeedService(mockRepo, mockTeam, permissiveAuthz(t), mockEvent, newTestLogger())

			if !tt.skipTeamCall {
				mockTeam.On("IsUserMemberOfTeam", mock.Anything, tt.userID, tt.teamID).
					Return(tt.isMember, tt.memberErr).Maybe()
			}

			if tt.isMember && tt.memberErr == nil && !tt.skipTeamCall {
				mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Feed")).
					Return(tt.repoErr).Maybe()
			}

			got, err := svc.CreateFeed(context.Background(), tt.userID, tt.teamID, tt.req)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, tt.req.Name, got.Name)
				assert.Equal(t, tt.teamID, got.TeamID)
				assert.Equal(t, tt.userID, got.CreatedByUserID)
			}
		})
	}
}

func TestFeedService_GetFeed(t *testing.T) {
	existingFeed := &models.Feed{
		ID:              "feed-1",
		TeamID:          "team-1",
		Name:            "Feed 1",
		CreatedByUserID: "user-1",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	tests := []struct {
		name    string
		feedID  string
		repo    *models.Feed
		repoErr error
		wantErr bool
	}{
		{
			name:   "success",
			feedID: "feed-1",
			repo:   existingFeed,
		},
		{
			name:    "not found",
			feedID:  "nope",
			repoErr: errors.New("feed not found"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := repoMocks.NewMockFeedRepository(t)
			svc := services.NewFeedService(mockRepo, nil, permissiveAuthz(t), nil, newTestLogger())

			mockRepo.On("GetByID", mock.Anything, "user-1", "team-1", tt.feedID).
				Return(tt.repo, tt.repoErr)

			got, err := svc.GetFeed(context.Background(), "user-1", "team-1", tt.feedID)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.repo, got)
			}
		})
	}
}

//nolint:funlen // Table-driven test with multiple cases
func TestFeedService_ListFeeds(t *testing.T) {
	feeds := []models.Feed{
		{ID: "feed-1", TeamID: "team-1", Name: "Feed A"},
		{ID: "feed-2", TeamID: "team-1", Name: "Feed B"},
	}

	tests := []struct {
		name            string
		filters         services.FeedFilters
		expectedFilters repositories.FeedFilters
		repoFeeds       []models.Feed
		repoTotal       int
		repoErr         error
		wantCount       int
		wantPages       int
		wantPage        int
		wantPerPage     int
		wantErr         bool
	}{
		{
			name:            "success with 2 feeds",
			filters:         services.FeedFilters{TeamID: "team-1", Page: 1, Limit: 10},
			expectedFilters: repositories.FeedFilters{TeamID: "team-1", Page: 1, Limit: 10},
			repoFeeds:       feeds,
			repoTotal:       2,
			wantCount:       2,
			wantPages:       1,
			wantPage:        1,
			wantPerPage:     10,
		},
		{
			name:            "empty result",
			filters:         services.FeedFilters{TeamID: "team-1", Page: 1, Limit: 10},
			expectedFilters: repositories.FeedFilters{TeamID: "team-1", Page: 1, Limit: 10},
			repoFeeds:       []models.Feed{},
			repoTotal:       0,
			wantCount:       0,
			wantPages:       0,
			wantPage:        1,
			wantPerPage:     10,
		},
		{
			name:            "repo error",
			filters:         services.FeedFilters{TeamID: "team-1", Page: 1, Limit: 10},
			expectedFilters: repositories.FeedFilters{TeamID: "team-1", Page: 1, Limit: 10},
			repoErr:         errors.New("db error"),
			wantErr:         true,
		},
		// H4: pagination clamping
		{
			name:            "page=0 clamped to 1",
			filters:         services.FeedFilters{TeamID: "team-1", Page: 0, Limit: 10},
			expectedFilters: repositories.FeedFilters{TeamID: "team-1", Page: 1, Limit: 10},
			repoFeeds:       feeds,
			repoTotal:       2,
			wantPage:        1,
			wantPerPage:     10,
			wantCount:       2,
			wantPages:       1,
		},
		{
			name:            "limit=0 defaulted to 20",
			filters:         services.FeedFilters{TeamID: "team-1", Page: 1, Limit: 0},
			expectedFilters: repositories.FeedFilters{TeamID: "team-1", Page: 1, Limit: 20},
			repoFeeds:       feeds,
			repoTotal:       2,
			wantPage:        1,
			wantPerPage:     20,
			wantCount:       2,
			wantPages:       1,
		},
		{
			name:            "limit>100 clamped to 100",
			filters:         services.FeedFilters{TeamID: "team-1", Page: 1, Limit: 500},
			expectedFilters: repositories.FeedFilters{TeamID: "team-1", Page: 1, Limit: 100},
			repoFeeds:       feeds,
			repoTotal:       2,
			wantPage:        1,
			wantPerPage:     100,
			wantCount:       2,
			wantPages:       1,
		},
		{
			name:            "negative page clamped to 1 and negative limit defaulted to 20",
			filters:         services.FeedFilters{TeamID: "team-1", Page: -5, Limit: -3},
			expectedFilters: repositories.FeedFilters{TeamID: "team-1", Page: 1, Limit: 20},
			repoFeeds:       []models.Feed{},
			repoTotal:       0,
			wantPage:        1,
			wantPerPage:     20,
			wantCount:       0,
			wantPages:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := repoMocks.NewMockFeedRepository(t)
			svc := services.NewFeedService(mockRepo, nil, permissiveAuthz(t), nil, newTestLogger())

			mockRepo.On("List", mock.Anything, "user-1", tt.expectedFilters).
				Return(tt.repoFeeds, tt.repoTotal, tt.repoErr)

			got, err := svc.ListFeeds(context.Background(), "user-1", tt.filters)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, got.Feeds, tt.wantCount)
				assert.Equal(t, tt.repoTotal, got.TotalCount)
				assert.Equal(t, tt.wantPages, got.TotalPages)
				assert.Equal(t, tt.wantPage, got.Page)
				assert.Equal(t, tt.wantPerPage, got.PerPage)
			}
		})
	}
}

func TestFeedService_UpdateFeed(t *testing.T) {
	existing := &models.Feed{
		ID:              "feed-1",
		TeamID:          "team-1",
		Name:            "Old Name",
		CreatedByUserID: "user-1",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	tests := []struct {
		name      string
		req       *models.UpdateFeedRequest
		getErr    error
		updateErr error
		wantName  string
		wantErr   bool
	}{
		{
			name:     "update name only",
			req:      &models.UpdateFeedRequest{Name: strPtr("New Name")},
			wantName: "New Name",
		},
		{
			name:    "get not found",
			req:     &models.UpdateFeedRequest{Name: strPtr("x")},
			getErr:  errors.New("feed not found"),
			wantErr: true,
		},
		{
			name:      "update repo error",
			req:       &models.UpdateFeedRequest{Name: strPtr("Conflict")},
			updateErr: errors.New("feed with name 'Conflict' already exists for this team"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := repoMocks.NewMockFeedRepository(t)
			svc := services.NewFeedService(mockRepo, nil, permissiveAuthz(t), nil, newTestLogger())

			mockRepo.On("GetByID", mock.Anything, "user-1", "team-1", "feed-1").
				Return(existing, tt.getErr).Maybe()

			if tt.getErr == nil {
				mockRepo.On("Update", mock.Anything, mock.AnythingOfType("*models.Feed")).
					Return(tt.updateErr).Maybe()
			}

			got, err := svc.UpdateFeed(context.Background(), "user-1", "team-1", "feed-1", tt.req)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantName, got.Name)
			}
		})
	}
}

func TestFeedService_DeleteFeed(t *testing.T) {
	tests := []struct {
		name    string
		repoErr error
		wantErr bool
	}{
		{name: "success"},
		{name: "not found", repoErr: errors.New("feed not found"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := repoMocks.NewMockFeedRepository(t)
			svc := services.NewFeedService(mockRepo, nil, permissiveAuthz(t), nil, newTestLogger())

			// Delete now fetches first to learn the feed's creator: the author may
			// delete their own, Owner/Admin may delete anyone's (moderation).
			mockRepo.On("GetByID", mock.Anything, "user-1", "team-1", "feed-1").
				Return(&models.Feed{ID: "feed-1", TeamID: "team-1", CreatedByUserID: "user-1"}, nil)
			mockRepo.On("Delete", mock.Anything, "user-1", "team-1", "feed-1").
				Return(tt.repoErr)

			err := svc.DeleteFeed(context.Background(), "user-1", "team-1", "feed-1")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// --------------------------------------------------------------------------
// FeedItemService tests
// --------------------------------------------------------------------------

//nolint:funlen,gocognit,gocyclo // Table-driven test with multiple cases and complex setup logic
func TestFeedItemService_CreateFeedItem(t *testing.T) {
	tests := []struct {
		name         string
		req          *models.CreateFeedItemRequest
		isMember     bool
		memberErr    error
		repoErr      error
		wantErr      bool
		skipTeamCall bool
	}{
		{
			name: "success",
			req: &models.CreateFeedItemRequest{
				Title:           "Test Title",
				Content:         "Hello world **bold** content",
				AIAssistantName: "Claude Code",
			},
			isMember: true,
		},
		{
			name: "content too large",
			req: &models.CreateFeedItemRequest{
				Title:           "T",
				Content:         strings.Repeat("x", 204801),
				AIAssistantName: "Bot",
			},
			wantErr:      true,
			skipTeamCall: true,
		},
		{
			name: "repo error",
			req: &models.CreateFeedItemRequest{
				Title:           "Good Title",
				Content:         "Good content",
				AIAssistantName: "Claude",
			},
			repoErr:  errors.New("feed not found"),
			wantErr:  true,
			isMember: true,
		},
		// H1: membership check
		{
			name: "not a team member",
			req: &models.CreateFeedItemRequest{
				Title:           "Forbidden Title",
				Content:         "Some content",
				AIAssistantName: "Bot",
			},
			isMember: false,
			wantErr:  true,
		},
		{
			name: "team membership check errors",
			req: &models.CreateFeedItemRequest{
				Title:           "Title",
				Content:         "Some content",
				AIAssistantName: "Bot",
			},
			memberErr: errors.New("db connection failed"),
			wantErr:   true,
		},
		// M5: input validation
		{
			name: "empty title rejected",
			req: &models.CreateFeedItemRequest{
				Title:           "",
				Content:         "Some content",
				AIAssistantName: "Bot",
			},
			wantErr:      true,
			skipTeamCall: true,
		},
		{
			name: "title too long rejected",
			req: &models.CreateFeedItemRequest{
				Title:           strings.Repeat("a", 256),
				Content:         "Some content",
				AIAssistantName: "Bot",
			},
			wantErr:      true,
			skipTeamCall: true,
		},
		{
			name: "empty assistant name rejected",
			req: &models.CreateFeedItemRequest{
				Title:           "Good Title",
				Content:         "Some content",
				AIAssistantName: "",
			},
			wantErr:      true,
			skipTeamCall: true,
		},
		{
			name: "assistant name too long rejected",
			req: &models.CreateFeedItemRequest{
				Title:           "Good Title",
				Content:         "Some content",
				AIAssistantName: strings.Repeat("a", 31),
			},
			wantErr:      true,
			skipTeamCall: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockItemRepo := repoMocks.NewMockFeedItemRepository(t)
			mockProjectRepo := repoMocks.NewMockProjectRepository(t)
			mockTeam := svcMocks.NewMockTeamServiceInterface(t)
			mockEvent := &eventMocks.MockEventPublisher{}

			svc := services.NewFeedItemService(mockItemRepo, nil, mockProjectRepo, mockTeam, permissiveAuthz(t), mockEvent, newTestLogger())

			if !tt.skipTeamCall {
				mockTeam.On("IsUserMemberOfTeam", mock.Anything, "user-1", "team-1").
					Return(tt.isMember, tt.memberErr).Maybe()
			}

			needsRepo := tt.isMember && tt.memberErr == nil && !tt.skipTeamCall && len(tt.req.Content) <= 204800 &&
				len([]rune(tt.req.Title)) > 0 && len([]rune(tt.req.Title)) <= 255 &&
				len([]rune(tt.req.AIAssistantName)) > 0 && len([]rune(tt.req.AIAssistantName)) <= 30

			if needsRepo {
				mockItemRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.FeedItem")).
					Return(tt.repoErr).Maybe()
			}

			if needsRepo && tt.repoErr == nil {
				mockEvent.On("Publish", mock.Anything, mock.Anything).Return(nil).Maybe()
			}

			got, err := svc.CreateFeedItem(context.Background(), "user-1", "team-1", "feed-1", tt.req)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, tt.req.Title, got.Title)
				assert.Equal(t, "team-1", got.TeamID)
				assert.Equal(t, "feed-1", got.FeedID)
				assert.NotEmpty(t, got.Excerpt)
			}
		})
	}
}

// TestFeedItemService_CreateFeedItem_PublishesEventWithBody asserts the feed item
// created event carries the item body (Title/Content/Excerpt) and is keyed by the
// posting user, so the downstream embedding pipeline can embed real content (#1361).
func TestFeedItemService_CreateFeedItem_PublishesEventWithBody(t *testing.T) {
	mockItemRepo := repoMocks.NewMockFeedItemRepository(t)
	mockProjectRepo := repoMocks.NewMockProjectRepository(t)
	mockTeam := svcMocks.NewMockTeamServiceInterface(t)
	mockEvent := &eventMocks.MockEventPublisher{}

	svc := services.NewFeedItemService(mockItemRepo, nil, mockProjectRepo, mockTeam, permissiveAuthz(t), mockEvent, newTestLogger())

	mockTeam.On("IsUserMemberOfTeam", mock.Anything, "poster-1", "team-1").Return(true, nil)
	mockItemRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.FeedItem")).
		Run(func(args mock.Arguments) {
			item := args.Get(1).(*models.FeedItem)
			item.ID = "item-1"
		}).Return(nil)

	var published events.Event
	mockEvent.On("Publish", mock.Anything, mock.MatchedBy(func(e events.Event) bool {
		published = e
		return true
	})).Return(nil)

	req := &models.CreateFeedItemRequest{
		Title:           "Shipped the feed embedding wiring",
		Content:         "Detailed feed item content that must be embedded.",
		AIAssistantName: "claude-code",
	}
	got, err := svc.CreateFeedItem(context.Background(), "poster-1", "team-1", "feed-1", req)
	require.NoError(t, err)
	require.NotNil(t, got)

	require.NotNil(t, published)
	assert.Equal(t, events.EventTypeFeedItemCreated, published.Type())
	assert.Equal(t, "poster-1", published.UserID(), "event must be keyed by the posting user")
	payload, ok := published.Payload().(*events.FeedItemCreatedPayload)
	require.True(t, ok)
	assert.Equal(t, "item-1", payload.ItemID)
	assert.Equal(t, "poster-1", payload.UserID)
	assert.Equal(t, req.Title, payload.Title)
	assert.Equal(t, req.Content, payload.Content)
	assert.NotEmpty(t, payload.Excerpt)
	mockEvent.AssertExpectations(t)
}

// TestFeedItemService_CreateFeedItem_ProjectCrossTeamValidation tests H2:
// project_id must belong to the same team as the feed item being created.
//
//nolint:funlen // Table-driven test with multiple cases
func TestFeedItemService_CreateFeedItem_ProjectCrossTeamValidation(t *testing.T) {
	projectInSameTeam := &models.Project{
		ID:     "proj-1",
		TeamID: "team-1",
		UserID: "user-1",
	}
	projectInOtherTeam := &models.Project{
		ID:     "proj-2",
		TeamID: "team-2", // different team
		UserID: "user-2",
	}

	tests := []struct {
		name          string
		projectID     *string
		projectResult *models.Project
		projectErr    error
		wantErr       bool
		errContains   string
	}{
		{
			name:      "no project_id — skip check",
			projectID: nil,
		},
		{
			name:          "project belongs to same team — allowed",
			projectID:     strPtr("proj-1"),
			projectResult: projectInSameTeam,
		},
		{
			name:          "project belongs to different team — rejected",
			projectID:     strPtr("proj-2"),
			projectResult: projectInOtherTeam,
			wantErr:       true,
			errContains:   "does not belong to the specified team",
		},
		{
			name:        "project not found — rejected",
			projectID:   strPtr("proj-missing"),
			projectErr:  errors.New("project not found"),
			wantErr:     true,
			errContains: "project not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockItemRepo := repoMocks.NewMockFeedItemRepository(t)
			mockProjectRepo := repoMocks.NewMockProjectRepository(t)
			mockTeam := svcMocks.NewMockTeamServiceInterface(t)
			mockEvent := &eventMocks.MockEventPublisher{}

			svc := services.NewFeedItemService(mockItemRepo, nil, mockProjectRepo, mockTeam, permissiveAuthz(t), mockEvent, newTestLogger())

			// Team membership always passes in these tests
			mockTeam.On("IsUserMemberOfTeam", mock.Anything, "user-1", "team-1").
				Return(true, nil).Maybe()

			if tt.projectID != nil {
				mockProjectRepo.On("GetByID", mock.Anything, "user-1", *tt.projectID).
					Return(tt.projectResult, tt.projectErr).Maybe()
			}

			// Only expect repo Create if no error is expected
			if !tt.wantErr {
				mockItemRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.FeedItem")).
					Return(nil).Maybe()
				mockEvent.On("Publish", mock.Anything, mock.Anything).Return(nil).Maybe()
			}

			req := &models.CreateFeedItemRequest{
				Title:           "Title",
				Content:         "Content",
				AIAssistantName: "Claude",
				ProjectID:       tt.projectID,
			}

			got, err := svc.CreateFeedItem(context.Background(), "user-1", "team-1", "feed-1", req)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
			}
		})
	}
}

//nolint:funlen // Table-driven test with multiple cases
func TestFeedItemService_CreateFeedItem_ExcerptComputation(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		wantTruncated bool
		assertFn      func(t *testing.T, excerpt string)
	}{
		{
			name:          "short content unchanged",
			content:       "Hello world",
			wantTruncated: false,
		},
		{
			name:          "300-char content not truncated",
			content:       strings.Repeat("a", 300),
			wantTruncated: false,
		},
		{
			name:          "301-char content truncated with ellipsis",
			content:       strings.Repeat("b", 301),
			wantTruncated: true,
		},
		{
			name:          "markdown stripped from excerpt",
			content:       "**bold** and *italic* text",
			wantTruncated: false,
		},
		// M3: additional edge cases
		{
			name:    "empty content produces empty excerpt without panic",
			content: "",
			assertFn: func(t *testing.T, excerpt string) {
				t.Helper()
				assert.Empty(t, excerpt, "empty content should produce empty excerpt")
			},
		},
		{
			name:    "pure markdown produces non-empty stripped excerpt",
			content: "**bold**",
			assertFn: func(t *testing.T, excerpt string) {
				t.Helper()
				assert.NotEmpty(t, excerpt, "stripped markdown text should be non-empty")
				assert.NotContains(t, excerpt, "**", "bold markers should be stripped")
			},
		},
		{
			name:    "multi-byte runes at 300-char boundary — no panic, ellipsis appended",
			content: strings.Repeat("é", 305),
			assertFn: func(t *testing.T, excerpt string) {
				t.Helper()
				assert.True(t, strings.HasSuffix(excerpt, "…"), "should be truncated with ellipsis")
				// 300 runes + 1 ellipsis rune = 301 runes
				runes := []rune(excerpt)
				assert.LessOrEqual(t, len(runes), 302, "excerpt should not exceed 301 runes + ellipsis")
			},
		},
		{
			name:    "emoji string at boundary — no panic, ellipsis appended",
			content: strings.Repeat("😀", 305),
			assertFn: func(t *testing.T, excerpt string) {
				t.Helper()
				assert.True(t, strings.HasSuffix(excerpt, "…"), "should be truncated with ellipsis")
				runes := []rune(excerpt)
				assert.LessOrEqual(t, len(runes), 302, "excerpt rune count should be bounded")
			},
		},
		{
			name:    "code block only content produces empty/whitespace excerpt",
			content: "```go\nfoo := 1\n```",
			assertFn: func(t *testing.T, excerpt string) {
				t.Helper()
				assert.Empty(t, strings.TrimSpace(excerpt), "code-block-only content should yield empty excerpt after strip")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := repoMocks.NewMockFeedItemRepository(t)
			mockProjectRepo := repoMocks.NewMockProjectRepository(t)
			mockEvent := &eventMocks.MockEventPublisher{}
			// Use nil teamService so membership check is skipped for these tests
			svc := services.NewFeedItemService(mockRepo, nil, mockProjectRepo, nil, permissiveAuthz(t), mockEvent, newTestLogger())

			mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.FeedItem")).
				Return(nil)
			mockEvent.On("Publish", mock.Anything, mock.Anything).Return(nil).Maybe()

			req := &models.CreateFeedItemRequest{
				Title:           "T",
				Content:         tt.content,
				AIAssistantName: "Bot",
			}

			got, err := svc.CreateFeedItem(context.Background(), "u", "t", "f", req)
			require.NoError(t, err)
			require.NotNil(t, got)

			switch {
			case tt.assertFn != nil:
				tt.assertFn(t, got.Excerpt)
			case tt.wantTruncated:
				assert.True(t, strings.HasSuffix(got.Excerpt, "…"), "excerpt should end with ellipsis")
			default:
				assert.False(t, strings.HasSuffix(got.Excerpt, "…"), "short content should not be truncated")
			}
		})
	}
}

func TestFeedItemService_ArchiveFeedItem(t *testing.T) {
	tests := []struct {
		name    string
		repoErr error
		wantErr bool
	}{
		{name: "success"},
		{name: "already archived", repoErr: errors.New("feed item not found or already archived"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := repoMocks.NewMockFeedItemRepository(t)
			mockProjectRepo := repoMocks.NewMockProjectRepository(t)
			svc := services.NewFeedItemService(mockRepo, nil, mockProjectRepo, nil, permissiveAuthz(t), nil, newTestLogger())

			mockRepo.On("Archive", mock.Anything, "user-1", "team-1", "item-1").
				Return(tt.repoErr)

			err := svc.ArchiveFeedItem(context.Background(), "user-1", "team-1", "item-1")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestFeedItemService_UnarchiveFeedItem(t *testing.T) {
	tests := []struct {
		name    string
		repoErr error
		wantErr bool
	}{
		{name: "success"},
		{name: "not archived", repoErr: errors.New("feed item not found or not archived"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := repoMocks.NewMockFeedItemRepository(t)
			mockProjectRepo := repoMocks.NewMockProjectRepository(t)
			svc := services.NewFeedItemService(mockRepo, nil, mockProjectRepo, nil, permissiveAuthz(t), nil, newTestLogger())

			mockRepo.On("Unarchive", mock.Anything, "user-1", "team-1", "item-1").
				Return(tt.repoErr)

			err := svc.UnarchiveFeedItem(context.Background(), "user-1", "team-1", "item-1")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestFeedItemService_DeleteFeedItem(t *testing.T) {
	tests := []struct {
		name    string
		repoErr error
		wantErr bool
	}{
		{name: "success"},
		{name: "not found", repoErr: errors.New("feed item not found"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := repoMocks.NewMockFeedItemRepository(t)
			mockProjectRepo := repoMocks.NewMockProjectRepository(t)
			svc := services.NewFeedItemService(mockRepo, nil, mockProjectRepo, nil, permissiveAuthz(t), nil, newTestLogger())

			// Delete now fetches first to learn who posted the item: the author
			// may delete their own, Owner/Admin may delete anyone's (moderation).
			mockRepo.On("GetByID", mock.Anything, "user-1", "team-1", "item-1").
				Return(&models.FeedItem{ID: "item-1", TeamID: "team-1", PostedByUserID: "user-1"}, nil)
			mockRepo.On("Delete", mock.Anything, "user-1", "team-1", "item-1").
				Return(tt.repoErr)

			err := svc.DeleteFeedItem(context.Background(), "user-1", "team-1", "item-1")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

//nolint:funlen // Table-driven test with multiple cases
func TestFeedItemService_ListFeedItems(t *testing.T) {
	items := []models.FeedItem{
		{ID: "item-1", TeamID: "team-1", FeedID: "feed-1", Title: "A"},
	}

	tests := []struct {
		name            string
		filters         services.FeedItemFilters
		expectedFilters repositories.FeedItemFilters
		repoItems       []models.FeedItem
		repoTotal       int
		repoErr         error
		wantPage        int
		wantPerPage     int
		wantErr         bool
	}{
		{
			name:            "success",
			filters:         services.FeedItemFilters{TeamID: "team-1", Page: 1, Limit: 20},
			expectedFilters: repositories.FeedItemFilters{TeamID: "team-1", Page: 1, Limit: 20},
			repoItems:       items,
			repoTotal:       1,
			wantPage:        1,
			wantPerPage:     20,
		},
		{
			name:            "repo error",
			filters:         services.FeedItemFilters{TeamID: "team-1", Page: 1, Limit: 20},
			expectedFilters: repositories.FeedItemFilters{TeamID: "team-1", Page: 1, Limit: 20},
			repoErr:         errors.New("db error"),
			wantErr:         true,
		},
		// H4: pagination clamping
		{
			name:            "page=0 clamped to 1",
			filters:         services.FeedItemFilters{TeamID: "team-1", Page: 0, Limit: 20},
			expectedFilters: repositories.FeedItemFilters{TeamID: "team-1", Page: 1, Limit: 20},
			repoItems:       items,
			repoTotal:       1,
			wantPage:        1,
			wantPerPage:     20,
		},
		{
			name:            "limit=0 defaulted to 20",
			filters:         services.FeedItemFilters{TeamID: "team-1", Page: 1, Limit: 0},
			expectedFilters: repositories.FeedItemFilters{TeamID: "team-1", Page: 1, Limit: 20},
			repoItems:       items,
			repoTotal:       1,
			wantPage:        1,
			wantPerPage:     20,
		},
		{
			name:            "limit>100 clamped to 100",
			filters:         services.FeedItemFilters{TeamID: "team-1", Page: 1, Limit: 999},
			expectedFilters: repositories.FeedItemFilters{TeamID: "team-1", Page: 1, Limit: 100},
			repoItems:       items,
			repoTotal:       1,
			wantPage:        1,
			wantPerPage:     100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := repoMocks.NewMockFeedItemRepository(t)
			mockProjectRepo := repoMocks.NewMockProjectRepository(t)
			svc := services.NewFeedItemService(mockRepo, nil, mockProjectRepo, nil, permissiveAuthz(t), nil, newTestLogger())

			mockRepo.On("List", mock.Anything, "user-1", tt.expectedFilters).
				Return(tt.repoItems, tt.repoTotal, tt.repoErr)

			got, err := svc.ListFeedItems(context.Background(), "user-1", tt.filters)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, got.Items, tt.repoTotal)
				assert.Equal(t, tt.wantPage, got.Page)
				assert.Equal(t, tt.wantPerPage, got.PerPage)
			}
		})
	}
}

// The feed permission matrix from epic #220 §4:
//
//	| Action                          | Owner | Admin | Member |
//	|---------------------------------|-------|-------|--------|
//	| Delete own post                 |  yes  |  yes  |  yes   |
//	| Delete anyone's post (moderate) |  yes  |  yes  |   no   |
//
// Moderation uses feed.delete.any (NOT resource.delete.any) — a distinct
// permission so a team can be given post-moderation without resource deletion.
//
// These drive a REAL AuthorizationService over a mocked TeamMemberRepository, so
// the decision under test is the shipped matrix rather than a restatement of it.

const (
	feedRBACTeam   = "team-rbac"
	feedRBACCaller = "user-caller"
	feedRBACOther  = "user-other"
)

func feedAuthzForRole(t *testing.T, role models.TeamMemberRole) services.AuthorizationServiceInterface {
	t.Helper()
	memberRepo := repoMocks.NewMockTeamMemberRepository(t)
	if role == "" {
		memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, feedRBACTeam, feedRBACCaller).
			Return(nil, repositories.ErrTeamMemberNotFound).Maybe()
	} else {
		memberRepo.EXPECT().GetByTeamAndUser(mock.Anything, feedRBACTeam, feedRBACCaller).
			Return(&models.TeamMember{TeamID: feedRBACTeam, UserID: feedRBACCaller, Role: role}, nil).Maybe()
	}
	return services.NewAuthorizationService(memberRepo, newTestLogger())
}

var feedModerationCases = []struct {
	name     string
	role     models.TeamMemberRole
	posterID string
	allowed  bool
}{
	{"member deletes own post", models.TeamMemberRoleMember, feedRBACCaller, true},
	{"member cannot delete another's post", models.TeamMemberRoleMember, feedRBACOther, false},
	{"admin moderates another's post", models.TeamMemberRoleAdmin, feedRBACOther, true},
	{"owner moderates another's post", models.TeamMemberRoleOwner, feedRBACOther, true},
	{"non-member cannot delete own post", "", feedRBACCaller, false},
}

func TestFeedItemService_DeleteFeedItem_Moderation(t *testing.T) {
	for _, tc := range feedModerationCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := repoMocks.NewMockFeedItemRepository(t)
			repo.On("GetByID", mock.Anything, feedRBACCaller, feedRBACTeam, "item-1").
				Return(&models.FeedItem{ID: "item-1", TeamID: feedRBACTeam, PostedByUserID: tc.posterID}, nil)
			if tc.allowed {
				repo.On("Delete", mock.Anything, feedRBACCaller, feedRBACTeam, "item-1").Return(nil)
			}

			svc := services.NewFeedItemService(
				repo, nil, repoMocks.NewMockProjectRepository(t), nil,
				feedAuthzForRole(t, tc.role), nil, newTestLogger(),
			)
			err := svc.DeleteFeedItem(context.Background(), feedRBACCaller, feedRBACTeam, "item-1")

			if tc.allowed {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, services.ErrPermissionDenied)
			repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestFeedService_DeleteFeed_Moderation(t *testing.T) {
	for _, tc := range feedModerationCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := repoMocks.NewMockFeedRepository(t)
			repo.On("GetByID", mock.Anything, feedRBACCaller, feedRBACTeam, "feed-1").
				Return(&models.Feed{ID: "feed-1", TeamID: feedRBACTeam, CreatedByUserID: tc.posterID}, nil)
			if tc.allowed {
				repo.On("Delete", mock.Anything, feedRBACCaller, feedRBACTeam, "feed-1").Return(nil)
			}

			svc := services.NewFeedService(repo, nil, feedAuthzForRole(t, tc.role), nil, newTestLogger())
			err := svc.DeleteFeed(context.Background(), feedRBACCaller, feedRBACTeam, "feed-1")

			if tc.allowed {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, services.ErrPermissionDenied)
			repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestFeedItemService_CreateFeedItem_NonMemberDenied(t *testing.T) {
	repo := repoMocks.NewMockFeedItemRepository(t)

	svc := services.NewFeedItemService(
		repo, nil, repoMocks.NewMockProjectRepository(t), nil,
		feedAuthzForRole(t, ""), nil, newTestLogger(),
	)
	_, err := svc.CreateFeedItem(context.Background(), feedRBACCaller, feedRBACTeam, "feed-1",
		&models.CreateFeedItemRequest{Title: "T", Content: "C", AIAssistantName: "Claude"})

	require.ErrorIs(t, err, services.ErrPermissionDenied)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}
