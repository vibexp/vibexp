package services

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

func newInsightsService(t *testing.T) (*AdminService, *repomocks.MockAdminRepository) {
	t.Helper()
	repo := repomocks.NewMockAdminRepository(t)
	return &AdminService{adminRepo: repo}, repo
}

func TestGetUserInsights_PivotsTotalsTeamsAndProjects(t *testing.T) {
	svc, repo := newInsightsService(t)
	userID := uuid.NewString()
	teamA, teamB := uuid.NewString(), uuid.NewString()
	projA1, projA2 := uuid.NewString(), uuid.NewString()

	repo.On("UserExists", mock.Anything, userID).Return(true, nil)
	repo.On("GetUserResourceCounts", mock.Anything, userID).Return([]models.AdminUserResourceCountRow{
		{ResourceType: models.AdminResourceTypeAgent, TeamID: teamA, TeamName: "Alpha", IsMember: true, Count: 1},
		{ResourceType: models.AdminResourceTypeComment, TeamID: teamA, TeamName: "Alpha", IsMember: true, Count: 4},
		{
			ResourceType: models.AdminResourceTypePrompt, TeamID: teamA, TeamName: "Alpha", IsMember: true,
			ProjectID: &projA1, ProjectName: strPtr("One"), Count: 3,
		},
		{
			ResourceType: models.AdminResourceTypeMemory, TeamID: teamA, TeamName: "Alpha", IsMember: true,
			ProjectID: &projA1, ProjectName: strPtr("One"), Count: 2,
		},
		{
			ResourceType: models.AdminResourceTypeBlueprint, TeamID: teamA, TeamName: "Alpha", IsMember: true,
			ProjectID: &projA2, ProjectName: nil, Count: 1,
		},
		{ResourceType: models.AdminResourceTypeFeedItem, TeamID: teamB, TeamName: "Beta", Count: 5},
		{
			ResourceType: models.AdminResourceTypeArtifact, TeamID: teamB, TeamName: "Beta",
			ProjectID: strPtr(uuid.NewString()), ProjectName: strPtr("Two"), Count: 6,
		},
		{ResourceType: models.AdminResourceTypeFeed, TeamID: teamB, TeamName: "Beta", Count: 1},
		{ResourceType: models.AdminResourceTypeAttachment, TeamID: teamB, TeamName: "Beta", Count: 2},
		{ResourceType: "unknown", TeamID: teamB, TeamName: "Beta", Count: 99},
	}, nil)

	got, err := svc.GetUserInsights(context.Background(), userID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, userID, got.UserID)
	assert.Equal(t, models.AdminResourceCounts{
		Prompts: 3, Memories: 2, Artifacts: 6, Blueprints: 1, Agents: 1,
		Feeds: 1, FeedItems: 5, Comments: 4, Attachments: 2, Total: 25,
	}, got.Totals, "an unknown type must not leak into Total")

	require.Len(t, got.Teams, 2)
	alpha, beta := got.Teams[0], got.Teams[1]
	assert.Equal(t, "Alpha", alpha.TeamName)
	assert.True(t, alpha.IsMember)
	assert.Equal(t, int64(11), alpha.Counts.Total)
	assert.False(t, beta.IsMember)
	assert.Equal(t, int64(14), beta.Counts.Total)
	assert.Equal(t, got.Totals.Total, alpha.Counts.Total+beta.Counts.Total)

	require.Len(t, alpha.Projects, 2)
	assert.Equal(t, models.AdminUserProjectResourceCounts{
		ProjectID: projA1, ProjectName: "One",
		Counts: models.AdminProjectResourceCounts{Prompts: 3, Memories: 2},
	}, alpha.Projects[0])
	assert.Equal(t, "", alpha.Projects[1].ProjectName, "a missing project name degrades to empty")
	assert.Equal(t, int64(1), alpha.Projects[1].Counts.Blueprints)
	require.Len(t, beta.Projects, 1)
	assert.Equal(t, int64(6), beta.Projects[0].Counts.Artifacts)
}

func TestGetUserInsights_NoResourcesHasEmptyTeams(t *testing.T) {
	svc, repo := newInsightsService(t)
	repo.On("UserExists", mock.Anything, "u").Return(true, nil)
	repo.On("GetUserResourceCounts", mock.Anything, "u").Return([]models.AdminUserResourceCountRow{}, nil)

	got, err := svc.GetUserInsights(context.Background(), "u")
	require.NoError(t, err)
	require.NotNil(t, got.Teams)
	assert.Empty(t, got.Teams)
}

func TestGetUserInsights_UnknownUserAndErrors(t *testing.T) {
	t.Run("unknown user is (nil, nil)", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		repo.On("UserExists", mock.Anything, "u").Return(false, nil)
		got, err := svc.GetUserInsights(context.Background(), "u")
		require.NoError(t, err)
		assert.Nil(t, got)
	})
	t.Run("existence check error", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		repo.On("UserExists", mock.Anything, "u").Return(false, errors.New("boom"))
		_, err := svc.GetUserInsights(context.Background(), "u")
		require.Error(t, err)
	})
	t.Run("counts error", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		repo.On("UserExists", mock.Anything, "u").Return(true, nil)
		repo.On("GetUserResourceCounts", mock.Anything, "u").Return(nil, errors.New("boom"))
		_, err := svc.GetUserInsights(context.Background(), "u")
		require.Error(t, err)
	})
}

func TestUserExists_Delegates(t *testing.T) {
	svc, repo := newInsightsService(t)
	repo.On("UserExists", mock.Anything, "u").Return(true, nil)
	ok, err := svc.UserExists(context.Background(), "u")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestGetUserCreationMetrics_GapFillsEveryTypeAndBucket(t *testing.T) {
	svc, repo := newInsightsService(t)
	// A Wednesday at month granularity: from snaps down to the 1st.
	from := time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	jul, aug, sep := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	repo.On("UserExists", mock.Anything, "u").Return(true, nil)
	repo.On("GetUserCreationSeries", mock.Anything, "u", jul, to, adminGranularityMonth).
		Return([]models.AdminGrowthCount{
			{Entity: models.AdminResourceTypePrompt, Bucket: jul, Count: 2},
			{Entity: models.AdminResourceTypeMemory, Bucket: jul, Count: 1},
			{Entity: models.AdminResourceTypeArtifact, Bucket: sep, Count: 3},
			{Entity: models.AdminResourceTypeBlueprint, Bucket: sep, Count: 4},
			{Entity: models.AdminResourceTypeAgent, Bucket: sep, Count: 5},
			{Entity: models.AdminResourceTypeFeed, Bucket: sep, Count: 6},
			{Entity: models.AdminResourceTypeFeedItem, Bucket: sep, Count: 7},
			{Entity: models.AdminResourceTypeComment, Bucket: sep, Count: 8},
			{Entity: models.AdminResourceTypeAttachment, Bucket: sep, Count: 9},
			// A bucket outside the range is dropped, not appended.
			{Entity: models.AdminResourceTypePrompt, Bucket: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Count: 1},
		}, nil)

	got, err := svc.GetUserCreationMetrics(context.Background(), "u", AdminTimeseriesQuery{
		From: &from, To: &to, Granularity: adminGranularityMonth,
	})
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, jul, got.From)
	assert.Equal(t, to, got.To)
	assert.Equal(t, adminGranularityMonth, got.Granularity)
	require.Len(t, got.Series, 3)
	assert.Equal(t, models.AdminUserCreationPoint{Bucket: jul, Prompts: 2, Memories: 1}, got.Series[0])
	assert.Equal(t, models.AdminUserCreationPoint{Bucket: aug}, got.Series[1], "an empty bucket is all zeros")
	assert.Equal(t, models.AdminUserCreationPoint{
		Bucket: sep, Artifacts: 3, Blueprints: 4, Agents: 5, Feeds: 6, FeedItems: 7, Comments: 8, Attachments: 9,
	}, got.Series[2])
}

func TestGetUserCreationMetrics_Errors(t *testing.T) {
	t.Run("invalid range is ErrAdminTimeseriesRange before any query", func(t *testing.T) {
		svc, _ := newInsightsService(t)
		_, err := svc.GetUserCreationMetrics(context.Background(), "u", AdminTimeseriesQuery{Granularity: "hour"})
		var rangeErr *ErrAdminTimeseriesRange
		require.ErrorAs(t, err, &rangeErr)
	})
	t.Run("unknown user is (nil, nil)", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		repo.On("UserExists", mock.Anything, "u").Return(false, nil)
		got, err := svc.GetUserCreationMetrics(context.Background(), "u", AdminTimeseriesQuery{})
		require.NoError(t, err)
		assert.Nil(t, got)
	})
	t.Run("series error", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		repo.On("UserExists", mock.Anything, "u").Return(true, nil)
		repo.On("GetUserCreationSeries", mock.Anything, "u", mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("boom"))
		_, err := svc.GetUserCreationMetrics(context.Background(), "u", AdminTimeseriesQuery{})
		require.Error(t, err)
	})
}

func timelineEvent(at time.Time) models.AdminUserTimelineEvent {
	return models.AdminUserTimelineEvent{
		ResourceType: models.AdminResourceTypePrompt, Action: models.AdminTimelineActionCreated,
		ResourceID: uuid.NewString(), TeamID: uuid.NewString(), TeamName: "Acme", OccurredAt: at,
	}
}

func TestGetUserTimeline_PagesWithCursor(t *testing.T) {
	svc, repo := newInsightsService(t)
	base := time.Date(2026, 9, 20, 10, 0, 0, 123456000, time.UTC)
	events := []models.AdminUserTimelineEvent{timelineEvent(base), timelineEvent(base), timelineEvent(base.Add(-time.Hour))}

	repo.On("UserExists", mock.Anything, "u").Return(true, nil)
	// limit 2 → the repository is asked for 3; the third says "there is more".
	repo.On("ListUserTimeline", mock.Anything, "u", (*models.AdminTimelineCursor)(nil), 3).Return(events, nil)

	page, err := svc.GetUserTimeline(context.Background(), "u", "", 2)
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	require.NotNil(t, page.NextCursor)

	decoded, err := decodeAdminTimelineCursor(*page.NextCursor)
	require.NoError(t, err)
	assert.Equal(t, models.AdminTimelineCursor{
		OccurredAt: events[1].OccurredAt, ResourceType: events[1].ResourceType,
		Action: events[1].Action, ResourceID: events[1].ResourceID,
	}, decoded, "the cursor round-trips the last item exactly, microseconds included")

	// The next call passes that position to the repository.
	repo.On("ListUserTimeline", mock.Anything, "u", &decoded, 3).Return(events[2:], nil)
	next, err := svc.GetUserTimeline(context.Background(), "u", *page.NextCursor, 2)
	require.NoError(t, err)
	assert.Len(t, next.Items, 1)
	assert.Nil(t, next.NextCursor, "the last page has no cursor")
}

func TestGetUserTimeline_LimitDefaultsAndCap(t *testing.T) {
	for _, tc := range []struct{ in, want int }{{0, AdminUserTimelineDefaultLimit + 1}, {500, AdminUserTimelineMaxLimit + 1}} {
		svc, repo := newInsightsService(t)
		repo.On("UserExists", mock.Anything, "u").Return(true, nil)
		repo.On("ListUserTimeline", mock.Anything, "u", (*models.AdminTimelineCursor)(nil), tc.want).
			Return([]models.AdminUserTimelineEvent{}, nil)
		page, err := svc.GetUserTimeline(context.Background(), "u", "", tc.in)
		require.NoError(t, err)
		assert.Empty(t, page.Items)
		assert.Nil(t, page.NextCursor)
	}
}

func TestGetUserTimeline_Errors(t *testing.T) {
	t.Run("unknown user is (nil, nil)", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		repo.On("UserExists", mock.Anything, "u").Return(false, nil)
		got, err := svc.GetUserTimeline(context.Background(), "u", "", 10)
		require.NoError(t, err)
		assert.Nil(t, got)
	})
	t.Run("repository error", func(t *testing.T) {
		svc, repo := newInsightsService(t)
		repo.On("UserExists", mock.Anything, "u").Return(true, nil)
		repo.On("ListUserTimeline", mock.Anything, "u", mock.Anything, mock.Anything).Return(nil, errors.New("boom"))
		_, err := svc.GetUserTimeline(context.Background(), "u", "", 10)
		require.Error(t, err)
	})
	t.Run("malformed cursor fails before any query", func(t *testing.T) {
		svc, _ := newInsightsService(t)
		_, err := svc.GetUserTimeline(context.Background(), "u", "!!!", 10)
		var cursorErr *ErrAdminInvalidCursor
		require.ErrorAs(t, err, &cursorErr)
	})
}

func TestDecodeAdminTimelineCursor_RejectsTampering(t *testing.T) {
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	id := uuid.NewString()
	for name, cursor := range map[string]string{
		"not base64":   "%%%",
		"not json":     enc("nope"),
		"bad time":     enc(`{"t":"yesterday","r":"prompt","a":"created","i":"` + id + `"}`),
		"unknown type": enc(`{"t":"2026-09-20T10:00:00Z","r":"secret","a":"created","i":"` + id + `"}`),
		"bad action":   enc(`{"t":"2026-09-20T10:00:00Z","r":"prompt","a":"deleted","i":"` + id + `"}`),
		"bad id":       enc(`{"t":"2026-09-20T10:00:00Z","r":"prompt","a":"created","i":"x"}`),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decodeAdminTimelineCursor(cursor)
			var cursorErr *ErrAdminInvalidCursor
			require.ErrorAs(t, err, &cursorErr)
			assert.Equal(t, "invalid cursor", cursorErr.Error())
		})
	}
}
