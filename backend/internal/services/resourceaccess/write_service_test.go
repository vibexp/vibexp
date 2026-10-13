package resourceaccess

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/pkg/events"
)

func newWriteServiceWithFake(repo *mocks.MockResourceWriteEventRepository, submitter taskSubmitter) *WriteService {
	return &WriteService{repo: repo, submitter: submitter, logger: newTestLogger()}
}

func sampleWriteEvent() *models.ResourceWriteEvent {
	return &models.ResourceWriteEvent{
		TeamID:        "team-1",
		ProjectID:     strPtr("project-1"),
		UserID:        strPtr("user-1"),
		ResourceType:  "memory",
		ResourceID:    "res-1",
		Action:        models.WriteActionCreate,
		Source:        SourceMCP,
		AssistantName: strPtr("claude-code"),
	}
}

func TestNewWriteService(t *testing.T) {
	t.Parallel()

	pool := events.NewWorkerPool(1)
	svc := NewWriteService(mocks.NewMockResourceWriteEventRepository(t), pool, newTestLogger())

	require.NotNil(t, svc)
	assert.Equal(t, pool, svc.submitter)
}

func TestWriteService_RecordWrite_PersistsEvent(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceWriteEventRepository(t)
	submitter := &syncSubmitter{}
	svc := newWriteServiceWithFake(repo, submitter)

	event := sampleWriteEvent()
	repo.EXPECT().Create(mock.Anything, event).Return(nil).Once()

	svc.RecordWrite(event)

	assert.Equal(t, 1, submitter.submitted)
}

// The write is recorded off the request path, so the repository must get a
// context with its own deadline rather than one that never expires.
func TestWriteService_RecordWrite_UsesBoundedContext(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceWriteEventRepository(t)
	svc := newWriteServiceWithFake(repo, &syncSubmitter{})

	repo.EXPECT().
		Create(mock.MatchedBy(func(ctx context.Context) bool {
			deadline, ok := ctx.Deadline()
			return ok && time.Until(deadline) <= recordTimeout
		}), mock.Anything).
		Return(nil).Once()

	svc.RecordWrite(sampleWriteEvent())
}

func TestWriteService_RecordWrite_NilEventIsIgnored(t *testing.T) {
	t.Parallel()

	submitter := &syncSubmitter{}
	svc := newWriteServiceWithFake(mocks.NewMockResourceWriteEventRepository(t), submitter)

	svc.RecordWrite(nil)

	assert.Zero(t, submitter.submitted)
}

// An event with no write channel is dropped rather than stored under a guessed
// one: "unknown" is how the metrics report a MISSING event, never a value an
// event carries.
func TestWriteService_RecordWrite_DropsEventWithoutWriteSource(t *testing.T) {
	t.Parallel()

	for _, source := range []string{"", SourceUnknown, "browser"} {
		submitter := &syncSubmitter{}
		svc := newWriteServiceWithFake(mocks.NewMockResourceWriteEventRepository(t), submitter)

		event := sampleWriteEvent()
		event.Source = source
		svc.RecordWrite(event)

		assert.Zero(t, submitter.submitted, "source %q", source)
	}
}

// Recording can never fail the write it describes: a repository error is
// swallowed (RecordWrite has no error to return) and does not panic.
func TestWriteService_RecordWrite_RepositoryErrorIsSwallowed(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceWriteEventRepository(t)
	svc := newWriteServiceWithFake(repo, &syncSubmitter{})

	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("db down")).Once()

	assert.NotPanics(t, func() { svc.RecordWrite(sampleWriteEvent()) })
}

func TestWriteService_RecordWrite_RecoversFromPanic(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceWriteEventRepository(t)
	svc := newWriteServiceWithFake(repo, &syncSubmitter{})

	repo.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(context.Context, *models.ResourceWriteEvent) { panic("boom") }).
		Return(nil).Once()

	assert.NotPanics(t, func() { svc.RecordWrite(sampleWriteEvent()) })
}

func TestWriteService_GetTeamCreationSources_ZeroFillsAndSums(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceWriteEventRepository(t)
	svc := newWriteServiceWithFake(repo, &syncSubmitter{})
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	repo.EXPECT().GetTeamCreationCountsBySource(mock.Anything, "team-1", since).
		Return([]models.ResourceCreationSourceCount{
			{ResourceType: "memories", Source: SourceMCP, Count: 12},
			{ResourceType: "memories", Source: SourceUnknown, Count: 5},
			{ResourceType: "artifacts", Source: SourceMCP, Count: 6},
			{ResourceType: "prompts", Source: SourceWeb, Count: 4},
			{ResourceType: "blueprints", Source: SourceCLI, Count: 1},
		}, nil).Once()
	repo.EXPECT().GetTeamWriteCountsBySource(mock.Anything, "team-1", since).
		Return([]models.ResourceWriteSourceCount{
			{Source: SourceMCP, Count: 48},
			{Source: SourceWeb, Count: 31},
		}, nil).Once()
	repo.EXPECT().GetTeamUnsourcedUpdateCount(mock.Anything, "team-1", since).Return(3, nil).Once()

	got, err := svc.GetTeamCreationSources(context.Background(), "team-1", since)

	require.NoError(t, err)
	// Every channel is present, including the ones with nothing to report.
	assert.Len(t, got.Created, 5)
	assert.Equal(t, SourceCreationCounts{Memories: 12, Artifacts: 6, Total: 18}, got.Created[SourceMCP])
	assert.Equal(t, SourceCreationCounts{Prompts: 4, Total: 4}, got.Created[SourceWeb])
	assert.Equal(t, SourceCreationCounts{Blueprints: 1, Total: 1}, got.Created[SourceCLI])
	assert.Equal(t, SourceCreationCounts{}, got.Created[SourceAPI])
	assert.Equal(t, SourceCreationCounts{Memories: 5, Total: 5}, got.Created[SourceUnknown])

	// unknown is the 5 creations with no create event plus the 3 content
	// changes with no source.
	assert.Equal(t,
		map[string]int{SourceWeb: 31, SourceCLI: 0, SourceMCP: 48, SourceAPI: 0, SourceUnknown: 8}, got.Writes)
}

// The identity the metrics endpoint promises (#1418): the five write counts sum
// to every write of the window the database has a record of, which is the
// write events, the creations with no create event and the content changes
// with no source.
func TestWriteService_GetTeamCreationSources_WritesSumToTotal(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceWriteEventRepository(t)
	svc := newWriteServiceWithFake(repo, &syncSubmitter{})
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	const (
		webEvents, cliEvents, mcpEvents, apiEvents = 7, 2, 11, 4
		unknownPrompts, unknownMemories            = 3, 6
		unsourcedUpdates                           = 5
	)
	repo.EXPECT().GetTeamCreationCountsBySource(mock.Anything, "team-1", since).
		Return([]models.ResourceCreationSourceCount{
			{ResourceType: "prompts", Source: SourceWeb, Count: 2},
			{ResourceType: "prompts", Source: SourceUnknown, Count: unknownPrompts},
			{ResourceType: "memories", Source: SourceMCP, Count: 9},
			{ResourceType: "memories", Source: SourceUnknown, Count: unknownMemories},
		}, nil).Once()
	repo.EXPECT().GetTeamWriteCountsBySource(mock.Anything, "team-1", since).
		Return([]models.ResourceWriteSourceCount{
			{Source: SourceWeb, Count: webEvents},
			{Source: SourceCLI, Count: cliEvents},
			{Source: SourceMCP, Count: mcpEvents},
			{Source: SourceAPI, Count: apiEvents},
		}, nil).Once()
	repo.EXPECT().GetTeamUnsourcedUpdateCount(mock.Anything, "team-1", since).Return(unsourcedUpdates, nil).Once()

	got, err := svc.GetTeamCreationSources(context.Background(), "team-1", since)

	require.NoError(t, err)
	require.Len(t, got.Writes, 5)
	sum := 0
	for _, count := range got.Writes {
		sum += count
	}
	recorded := webEvents + cliEvents + mcpEvents + apiEvents
	unrecorded := unknownPrompts + unknownMemories + unsourcedUpdates
	assert.Equal(t, unrecorded, got.Writes[SourceUnknown])
	assert.Equal(t, recorded+unrecorded, sum)
	// A recorded creation is one write, counted once: by its event, and not a
	// second time through the creation breakdown.
	assert.Equal(t, got.Created[SourceUnknown].Total+unsourcedUpdates, got.Writes[SourceUnknown])
}

func TestWriteService_GetTeamCreationSources_EmptyIsZeroFilled(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceWriteEventRepository(t)
	svc := newWriteServiceWithFake(repo, &syncSubmitter{})

	repo.EXPECT().GetTeamCreationCountsBySource(mock.Anything, "team-1", mock.Anything).
		Return([]models.ResourceCreationSourceCount{}, nil).Once()
	repo.EXPECT().GetTeamWriteCountsBySource(mock.Anything, "team-1", mock.Anything).
		Return([]models.ResourceWriteSourceCount{}, nil).Once()
	repo.EXPECT().GetTeamUnsourcedUpdateCount(mock.Anything, "team-1", mock.Anything).Return(0, nil).Once()

	got, err := svc.GetTeamCreationSources(context.Background(), "team-1", time.Now())

	require.NoError(t, err)
	for _, source := range []string{SourceWeb, SourceCLI, SourceMCP, SourceAPI, SourceUnknown} {
		counts, ok := got.Created[source]
		assert.True(t, ok, source)
		assert.Equal(t, SourceCreationCounts{}, counts, source)
	}
	assert.Equal(t,
		map[string]int{SourceWeb: 0, SourceCLI: 0, SourceMCP: 0, SourceAPI: 0, SourceUnknown: 0}, got.Writes)
}

func TestWriteService_GetTeamCreationSources_PropagatesErrors(t *testing.T) {
	t.Parallel()

	boom := errors.New("db down")

	t.Run("creation counts", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockResourceWriteEventRepository(t)
		svc := newWriteServiceWithFake(repo, &syncSubmitter{})
		repo.EXPECT().GetTeamCreationCountsBySource(mock.Anything, "team-1", mock.Anything).Return(nil, boom).Once()

		_, err := svc.GetTeamCreationSources(context.Background(), "team-1", time.Now())

		require.ErrorIs(t, err, boom)
	})

	t.Run("write counts", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockResourceWriteEventRepository(t)
		svc := newWriteServiceWithFake(repo, &syncSubmitter{})
		repo.EXPECT().GetTeamCreationCountsBySource(mock.Anything, "team-1", mock.Anything).
			Return([]models.ResourceCreationSourceCount{}, nil).Once()
		repo.EXPECT().GetTeamWriteCountsBySource(mock.Anything, "team-1", mock.Anything).Return(nil, boom).Once()

		_, err := svc.GetTeamCreationSources(context.Background(), "team-1", time.Now())

		require.ErrorIs(t, err, boom)
	})

	t.Run("unsourced update count", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockResourceWriteEventRepository(t)
		svc := newWriteServiceWithFake(repo, &syncSubmitter{})
		repo.EXPECT().GetTeamCreationCountsBySource(mock.Anything, "team-1", mock.Anything).
			Return([]models.ResourceCreationSourceCount{}, nil).Once()
		repo.EXPECT().GetTeamWriteCountsBySource(mock.Anything, "team-1", mock.Anything).
			Return([]models.ResourceWriteSourceCount{}, nil).Once()
		repo.EXPECT().GetTeamUnsourcedUpdateCount(mock.Anything, "team-1", mock.Anything).Return(0, boom).Once()

		_, err := svc.GetTeamCreationSources(context.Background(), "team-1", time.Now())

		require.ErrorIs(t, err, boom)
	})
}

func TestWriteService_GetProjectCreationSources(t *testing.T) {
	t.Parallel()

	repo := mocks.NewMockResourceWriteEventRepository(t)
	svc := newWriteServiceWithFake(repo, &syncSubmitter{})
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	repo.EXPECT().GetProjectCreationCountsBySource(mock.Anything, "team-1", "my-project", since).
		Return([]models.ResourceCreationSourceCount{
			{ResourceType: "prompts", Source: SourceAPI, Count: 2},
		}, nil).Once()
	repo.EXPECT().GetProjectWriteCountsBySource(mock.Anything, "team-1", "my-project", since).
		Return([]models.ResourceWriteSourceCount{{Source: SourceAPI, Count: 7}}, nil).Once()
	repo.EXPECT().GetProjectUnsourcedUpdateCount(mock.Anything, "team-1", "my-project", since).Return(4, nil).Once()

	got, err := svc.GetProjectCreationSources(context.Background(), "team-1", "my-project", since)

	require.NoError(t, err)
	assert.Equal(t, SourceCreationCounts{Prompts: 2, Total: 2}, got.Created[SourceAPI])
	assert.Equal(t, 7, got.Writes[SourceAPI])
	assert.Equal(t, 4, got.Writes[SourceUnknown])
}

func TestWriteService_GetProjectCreationSources_PropagatesErrors(t *testing.T) {
	t.Parallel()

	boom := errors.New("db down")

	t.Run("creation counts", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockResourceWriteEventRepository(t)
		svc := newWriteServiceWithFake(repo, &syncSubmitter{})
		repo.EXPECT().GetProjectCreationCountsBySource(mock.Anything, "team-1", "p", mock.Anything).
			Return(nil, boom).Once()

		_, err := svc.GetProjectCreationSources(context.Background(), "team-1", "p", time.Now())

		require.ErrorIs(t, err, boom)
	})

	t.Run("write counts", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockResourceWriteEventRepository(t)
		svc := newWriteServiceWithFake(repo, &syncSubmitter{})
		repo.EXPECT().GetProjectCreationCountsBySource(mock.Anything, "team-1", "p", mock.Anything).
			Return([]models.ResourceCreationSourceCount{}, nil).Once()
		repo.EXPECT().GetProjectWriteCountsBySource(mock.Anything, "team-1", "p", mock.Anything).
			Return(nil, boom).Once()

		_, err := svc.GetProjectCreationSources(context.Background(), "team-1", "p", time.Now())

		require.ErrorIs(t, err, boom)
	})

	t.Run("unsourced update count", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockResourceWriteEventRepository(t)
		svc := newWriteServiceWithFake(repo, &syncSubmitter{})
		repo.EXPECT().GetProjectCreationCountsBySource(mock.Anything, "team-1", "p", mock.Anything).
			Return([]models.ResourceCreationSourceCount{}, nil).Once()
		repo.EXPECT().GetProjectWriteCountsBySource(mock.Anything, "team-1", "p", mock.Anything).
			Return([]models.ResourceWriteSourceCount{}, nil).Once()
		repo.EXPECT().GetProjectUnsourcedUpdateCount(mock.Anything, "team-1", "p", mock.Anything).
			Return(0, boom).Once()

		_, err := svc.GetProjectCreationSources(context.Background(), "team-1", "p", time.Now())

		require.ErrorIs(t, err, boom)
	})
}

// A creation row whose source is not a write channel is folded into "unknown"
// rather than dropped, so a type's sources still sum to its total. A row for a
// resource type the breakdown does not track is ignored, and so is a write row
// with no write channel.
func TestBuildCreationSources_UnexpectedRows(t *testing.T) {
	t.Parallel()

	got := buildCreationSources(
		[]models.ResourceCreationSourceCount{
			{ResourceType: "memories", Source: "browser", Count: 3},
			{ResourceType: "memories", Source: SourceUnknown, Count: 2},
			{ResourceType: "projects", Source: SourceWeb, Count: 9},
		},
		[]models.ResourceWriteSourceCount{
			{Source: "browser", Count: 4},
			{Source: SourceCLI, Count: 1},
		},
		0,
	)

	assert.Equal(t, SourceCreationCounts{Memories: 5, Total: 5}, got.Created[SourceUnknown])
	assert.Equal(t, SourceCreationCounts{}, got.Created[SourceWeb])
	// The 5 unknown creations are unknown writes. The 4 "browser" events are
	// not added on top: a create event among them belongs to a resource that
	// is already one of the 5.
	assert.Equal(t,
		map[string]int{SourceWeb: 0, SourceCLI: 1, SourceMCP: 0, SourceAPI: 0, SourceUnknown: 5}, got.Writes)
}
