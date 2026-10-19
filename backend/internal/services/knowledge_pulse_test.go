package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

const pulseTestTeam = "550e8400-e29b-41d4-a716-446655440000"

var pulseT0 = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func pulseAt(sec int) time.Time { return pulseT0.Add(time.Duration(sec) * time.Second) }

func TestMergePulseEvents_SortsAcrossSourcesOldestFirst(t *testing.T) {
	reads := []models.PulseReadEvent{
		{ID: "r1", At: pulseAt(5), Actor: models.PulseActorRaw{Source: "mcp"}},
		{ID: "r2", At: pulseAt(1), Actor: models.PulseActorRaw{Source: "web", UserID: "u1", UserName: "Ada"}},
	}
	writes := []models.PulseWriteEvent{{ID: "w1", At: pulseAt(3), Action: "update", Actor: models.PulseActorRaw{Source: "cli"}}}
	links := []models.PulseLinkEvent{{ID: "l1", At: pulseAt(2), RelationType: "supersedes", Origin: "ai",
		From: models.PulseResource{Type: "memory", ID: "m1"}, To: models.PulseResource{Type: "artifact", ID: "a1"}}}
	posts := []models.PulseFeedEvent{{ID: "f1", At: pulseAt(4), Title: "Shipped", AssistantName: "claude-code"}}

	events := MergePulseEvents(reads, writes, links, posts, 0)

	ids := make([]string, 0, len(events))
	for _, e := range events {
		ids = append(ids, e.ID)
	}
	assert.Equal(t, []string{"read:r2", "link:l1", "write:w1", "feed_post:f1", "read:r1"}, ids)

	link := events[1]
	assert.Equal(t, PulseKindLink, link.Kind)
	assert.Equal(t, "supersedes", link.RelationType)
	require.NotNil(t, link.To)
	assert.Equal(t, "a1", link.To.ID)
	assert.Equal(t, "m1", link.Resource.ID)

	post := events[3]
	assert.Equal(t, PulseResourceFeedItem, post.Resource.Type)
	assert.Equal(t, "Shipped", post.Resource.Title)
	assert.Equal(t, "Claude Code", post.Actor.Name)
	assert.Equal(t, "update", events[2].Action)
}

func TestMergePulseEvents_TieBreaksOnIDAndKeepsOldestUnderLimit(t *testing.T) {
	reads := []models.PulseReadEvent{{ID: "b", At: pulseAt(1)}, {ID: "z", At: pulseAt(9)}}
	writes := []models.PulseWriteEvent{{ID: "a", At: pulseAt(1)}}

	events := MergePulseEvents(reads, writes, nil, nil, 2)

	require.Len(t, events, 2)
	// Same instant: "read:b" < "write:a" lexically; the newest event is the one cut.
	assert.Equal(t, "read:b", events[0].ID)
	assert.Equal(t, "write:a", events[1].ID)
}

func TestMergePulseEvents_EmptyIsNonNil(t *testing.T) {
	events := MergePulseEvents(nil, nil, nil, nil, 10)
	assert.NotNil(t, events)
	assert.Empty(t, events)
}

func TestResolvePulseActor(t *testing.T) {
	tests := []struct {
		name string
		in   models.PulseActorRaw
		want PulseActor
	}{
		{"web read is the person", models.PulseActorRaw{Source: "web", UserID: "u1", UserName: "Ada"},
			PulseActor{Type: "user", ID: "u1", Name: "Ada", Source: "web"}},
		{"web without a name", models.PulseActorRaw{Source: "web", UserID: "u1"},
			PulseActor{Type: "user", ID: "u1", Name: "A teammate", Source: "web"}},
		{"assistant name wins", models.PulseActorRaw{Source: "mcp", AssistantName: "claude-code", UserAgent: "Cursor/1.0"},
			PulseActor{Type: "agent", ID: "agent:claude-code", Name: "Claude Code", Source: "mcp"}},
		{"unknown assistant name is kept", models.PulseActorRaw{Source: "mcp", AssistantName: "My Bot"},
			PulseActor{Type: "agent", ID: "agent:my-bot", Name: "My Bot", Source: "mcp"}},
		{"user agent", models.PulseActorRaw{Source: "mcp", UserAgent: "Cursor/1.7 (darwin)", APIKeyName: "laptop"},
			PulseActor{Type: "agent", ID: "agent:cursor", Name: "Cursor", Source: "mcp"}},
		{"api key name when user agent is generic", models.PulseActorRaw{Source: "api", UserAgent: "Go-http-client/1.1", APIKeyName: "CI bot"},
			PulseActor{Type: "agent", ID: "agent:ci-bot", Name: "CI bot", Source: "api"}},
		{"falls back to the source", models.PulseActorRaw{Source: "cli"},
			PulseActor{Type: "agent", ID: "agent:vibexp-cli", Name: "VibeXP CLI", Source: "cli"}},
		{"unknown source maps to api", models.PulseActorRaw{Source: "unknown"},
			PulseActor{Type: "agent", ID: "agent:api-client", Name: "API client", Source: "api"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolvePulseActor(tt.in))
		})
	}
}

func TestPulseStatsFromDay(t *testing.T) {
	got := PulseStatsFromDay(models.PulseDayStats{Reads: 40, Writes: 7, Links: 5, Supersedes: 2, LinkedUpdates: 3})
	assert.Equal(t, PulseStats{Reads: 40, Writes: 7, Links: 5, CompoundingScore: 10}, got)
}

func TestClampPulseLimit(t *testing.T) {
	assert.Equal(t, PulseDefaultLimit, ClampPulseLimit(0))
	assert.Equal(t, PulseDefaultLimit, ClampPulseLimit(-3))
	assert.Equal(t, 50, ClampPulseLimit(50))
	assert.Equal(t, PulseMaxLimit, ClampPulseLimit(10_000))
}

func newPulseService(t *testing.T) (*KnowledgePulseService, *repomocks.MockKnowledgePulseRepository) {
	repo := repomocks.NewMockKnowledgePulseRepository(t)
	svc := NewKnowledgePulseService(repo, slog.New(slog.DiscardHandler))
	svc.now = func() time.Time { return pulseT0 }
	return svc, repo
}

func TestGetPulse_FirstLoadUsesWindowAndIncludesSnapshot(t *testing.T) {
	svc, repo := newPulseService(t)
	window := pulseT0.Add(-PulseInitialWindow)
	dayStart := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)

	repo.EXPECT().ListReadsSince(mock.Anything, pulseTestTeam, window, PulseDefaultLimit).
		Return([]models.PulseReadEvent{{ID: "r1", At: pulseAt(-60), Actor: models.PulseActorRaw{Source: "mcp"}}}, nil)
	repo.EXPECT().ListWritesSince(mock.Anything, pulseTestTeam, window, PulseDefaultLimit).
		Return([]models.PulseWriteEvent{{ID: "w1", At: pulseAt(-30), Action: "create"}}, nil)
	repo.EXPECT().ListLinksSince(mock.Anything, pulseTestTeam, window, PulseDefaultLimit).Return(nil, nil)
	repo.EXPECT().ListFeedPostsSince(mock.Anything, pulseTestTeam, window, PulseDefaultLimit).Return(nil, nil)
	repo.EXPECT().GetDayStats(mock.Anything, pulseTestTeam, dayStart).
		Return(models.PulseDayStats{Reads: 1, Writes: 1}, nil)
	repo.EXPECT().GetSnapshot(mock.Anything, pulseTestTeam, pulseT0.Add(-PulseActivityWindow), PulseSnapshotNodeCap).
		Return(&models.PulseSnapshotRaw{
			Nodes: []models.PulseNode{{Resource: models.PulseResource{Type: "memory", ID: "m1"}, Degree: 2}},
			Edges: []models.PulseEdge{},
			Actors: []models.PulseActorRaw{
				{Source: "mcp", UserAgent: "claude-code/2.0"},
				{Source: "mcp", AssistantName: "Claude Code"}, // same agent, de-duplicated
				{Source: "web", UserID: "u1", UserName: "Ada"},
			},
		}, nil)

	got, err := svc.GetPulse(context.Background(), pulseTestTeam, KnowledgePulseQuery{})

	require.NoError(t, err)
	require.Len(t, got.Events, 2)
	assert.Equal(t, pulseAt(-30), got.Cursor, "cursor is the newest returned event")
	require.NotNil(t, got.Snapshot)
	assert.Len(t, got.Snapshot.Nodes, 1)
	require.Len(t, got.Snapshot.Actors, 2)
	assert.Equal(t, "Ada", got.Snapshot.Actors[0].Name)
	assert.Equal(t, "Claude Code", got.Snapshot.Actors[1].Name)
	assert.Equal(t, 1, got.Stats.Reads)
}

func TestGetPulse_BusyFirstLoadNarrowsWindowToReachNow(t *testing.T) {
	svc, repo := newPulseService(t)
	wide := pulseT0.Add(-PulseInitialWindow)
	narrow := pulseT0.Add(-PulseInitialWindow / 4)

	// The full window overflows the page (2 reads at limit 2), so the
	// service retries with a quarter of the window, which fits.
	repo.EXPECT().ListReadsSince(mock.Anything, pulseTestTeam, wide, 2).
		Return([]models.PulseReadEvent{{ID: "old1", At: pulseAt(-800)}, {ID: "old2", At: pulseAt(-790)}}, nil)
	repo.EXPECT().ListReadsSince(mock.Anything, pulseTestTeam, narrow, 2).
		Return([]models.PulseReadEvent{{ID: "new1", At: pulseAt(-10)}}, nil)
	for _, since := range []time.Time{wide, narrow} {
		repo.EXPECT().ListWritesSince(mock.Anything, pulseTestTeam, since, 2).Return(nil, nil)
		repo.EXPECT().ListLinksSince(mock.Anything, pulseTestTeam, since, 2).Return(nil, nil)
		repo.EXPECT().ListFeedPostsSince(mock.Anything, pulseTestTeam, since, 2).Return(nil, nil)
	}
	repo.EXPECT().GetDayStats(mock.Anything, pulseTestTeam, mock.Anything).Return(models.PulseDayStats{}, nil)
	repo.EXPECT().GetSnapshot(mock.Anything, pulseTestTeam, mock.Anything, PulseSnapshotNodeCap).
		Return(&models.PulseSnapshotRaw{}, nil)

	got, err := svc.GetPulse(context.Background(), pulseTestTeam, KnowledgePulseQuery{Limit: 2})

	require.NoError(t, err)
	require.Len(t, got.Events, 1)
	assert.Equal(t, "read:new1", got.Events[0].ID)
	assert.Equal(t, pulseAt(-10), got.Cursor)
}

func TestGetPulse_DeltaWithoutEventsKeepsCursorAndSkipsSnapshot(t *testing.T) {
	svc, repo := newPulseService(t)
	since := pulseAt(-5)

	repo.EXPECT().ListReadsSince(mock.Anything, pulseTestTeam, since, 50).Return(nil, nil)
	repo.EXPECT().ListWritesSince(mock.Anything, pulseTestTeam, since, 50).Return(nil, nil)
	repo.EXPECT().ListLinksSince(mock.Anything, pulseTestTeam, since, 50).Return(nil, nil)
	repo.EXPECT().ListFeedPostsSince(mock.Anything, pulseTestTeam, since, 50).Return(nil, nil)
	repo.EXPECT().GetDayStats(mock.Anything, pulseTestTeam, mock.Anything).Return(models.PulseDayStats{}, nil)

	got, err := svc.GetPulse(context.Background(), pulseTestTeam, KnowledgePulseQuery{Since: &since, Limit: 50})

	require.NoError(t, err)
	assert.Empty(t, got.Events)
	assert.Equal(t, since, got.Cursor)
	assert.Nil(t, got.Snapshot)
}

func TestGetPulse_DeltaWithSnapshotRequested(t *testing.T) {
	svc, repo := newPulseService(t)
	since := pulseAt(-5)

	repo.EXPECT().ListReadsSince(mock.Anything, pulseTestTeam, since, PulseMaxLimit).Return(nil, nil)
	repo.EXPECT().ListWritesSince(mock.Anything, pulseTestTeam, since, PulseMaxLimit).Return(nil, nil)
	repo.EXPECT().ListLinksSince(mock.Anything, pulseTestTeam, since, PulseMaxLimit).Return(nil, nil)
	repo.EXPECT().ListFeedPostsSince(mock.Anything, pulseTestTeam, since, PulseMaxLimit).Return(nil, nil)
	repo.EXPECT().GetDayStats(mock.Anything, pulseTestTeam, mock.Anything).Return(models.PulseDayStats{}, nil)
	repo.EXPECT().GetSnapshot(mock.Anything, pulseTestTeam, mock.Anything, PulseSnapshotNodeCap).
		Return(&models.PulseSnapshotRaw{}, nil)

	got, err := svc.GetPulse(context.Background(), pulseTestTeam,
		KnowledgePulseQuery{Since: &since, Limit: 9999, IncludeSnapshot: true})

	require.NoError(t, err)
	require.NotNil(t, got.Snapshot)
	assert.NotNil(t, got.Snapshot.Actors)
}

func TestGetPulse_PropagatesRepositoryError(t *testing.T) {
	svc, repo := newPulseService(t)
	repo.EXPECT().ListReadsSince(mock.Anything, pulseTestTeam, mock.Anything, mock.Anything).
		Return(nil, errors.New("boom"))

	_, err := svc.GetPulse(context.Background(), pulseTestTeam, KnowledgePulseQuery{})

	require.ErrorContains(t, err, "boom")
}
