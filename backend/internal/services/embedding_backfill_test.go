package services_test

import (
	"bytes"
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
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/pkg/events"
	eventmocks "github.com/vibexp/vibexp/pkg/events/mocks"
)

// "" is the configured embedding model the service threads into the
// missing_only NOT EXISTS filter.

func newTestBackfillService(t *testing.T) (
	*services.EmbeddingBackfillService,
	*repomocks.MockEmbeddingBackfillRepository,
	*eventmocks.MockEventPublisher,
	*svcmocks.MockPromptBodyRenderer,
) {
	repo := repomocks.NewMockEmbeddingBackfillRepository(t)
	publisher := eventmocks.NewMockEventPublisher(t)
	renderer := svcmocks.NewMockPromptBodyRenderer(t)
	logger := slog.New(slog.DiscardHandler)
	// nil coverage getter: the coverage snapshot (#755) is opt-in, and only the
	// tests below that exercise it wire one in.
	svc := services.NewEmbeddingBackfillService(repo, publisher, renderer, nil, logger)
	return svc, repo, publisher, renderer
}

// newLoggingBackfillService builds a service whose logs are captured, so a test can
// assert on the run's terminal and coverage log lines (#755). coverage may be nil to
// exercise the not-wired path.
func newLoggingBackfillService(t *testing.T, coverage services.EmbeddingCoverageGetter) (
	*services.EmbeddingBackfillService,
	*repomocks.MockEmbeddingBackfillRepository,
	*eventmocks.MockEventPublisher,
	*bytes.Buffer,
) {
	repo := repomocks.NewMockEmbeddingBackfillRepository(t)
	publisher := eventmocks.NewMockEventPublisher(t)
	renderer := svcmocks.NewMockPromptBodyRenderer(t)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	svc := services.NewEmbeddingBackfillService(repo, publisher, renderer, coverage, logger)
	return svc, repo, publisher, &logs
}

// passthroughRender makes the renderer echo whatever body it is given, so a test
// that doesn't care about reference resolution still satisfies the prompt path.
func passthroughRender(renderer *svcmocks.MockPromptBodyRenderer) {
	renderer.EXPECT().RenderPromptBody(mock.Anything, mock.Anything).
		RunAndReturn(func(_, body string) (string, error) { return body, nil })
}

func backfillEntity(entityType, id string) models.BackfillEntity {
	return models.BackfillEntity{
		EntityType: entityType,
		EntityID:   id,
		UserID:     "user-1",
		TeamID:     "team-1",
		Title:      "title",
		Body:       "body",
		CreatedAt:  time.Now(),
	}
}

// supportedTypes is the canonical set the backfill enumerates for all=true.
var supportedTypes = []string{"prompt", "artifact", "memory", "blueprint", "feed_item"}

// emptyForRemainingTypes sets up each remaining (non-filtered) supported type to
// return an empty first page, so an all-types run touches them without publishing.
func emptyForRemainingTypes(repo *repomocks.MockEmbeddingBackfillRepository, except ...string) {
	skip := make(map[string]bool, len(except))
	for _, e := range except {
		skip[e] = true
	}
	for _, et := range supportedTypes {
		if skip[et] {
			continue
		}
		repo.EXPECT().ListEntities(mock.Anything, et, "", "", false, mock.Anything, 0).
			Return([]models.BackfillEntity{}, nil).Once()
	}
}

func TestEmbeddingBackfill_AllTypes_PublishesEach(t *testing.T) {
	svc, repo, publisher, renderer := newTestBackfillService(t)
	passthroughRender(renderer)

	repo.EXPECT().ListEntities(mock.Anything, "prompt", "", "", false, mock.Anything, 0).
		Return([]models.BackfillEntity{backfillEntity("prompt", "p1"), backfillEntity("prompt", "p2")}, nil).Once()
	emptyForRemainingTypes(repo, "prompt")
	publisher.EXPECT().Publish(mock.Anything, mock.Anything).Return(nil).Times(2)

	result, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{All: true})

	require.NoError(t, err)
	assert.False(t, result.DryRun)
	assert.Equal(t, 2, result.TotalSeen)
	assert.Equal(t, 2, result.TotalPublished)
	assert.Equal(t, 0, result.TotalFailed)
	// All five supported types are enumerated; feed_item_reply is no longer one.
	assert.Len(t, result.Results, 5)
}

func TestEmbeddingBackfill_NoScope_ReturnsScopeRequired(t *testing.T) {
	svc, _, _, _ := newTestBackfillService(t)

	_, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{})

	require.Error(t, err)
	assert.ErrorIs(t, err, services.ErrBackfillScopeRequired)
}

func TestEmbeddingBackfill_AllAndEntityTypes_ReturnsScopeAmbiguous(t *testing.T) {
	svc, _, _, _ := newTestBackfillService(t)

	_, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
		All:         true,
		EntityTypes: []string{"prompt"},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, services.ErrBackfillScopeAmbiguous)
}

func TestEmbeddingBackfill_EntityTypesFilter_OnlyRequested(t *testing.T) {
	svc, repo, publisher, _ := newTestBackfillService(t)

	repo.EXPECT().ListEntities(mock.Anything, "blueprint", "", "", false, mock.Anything, 0).
		Return([]models.BackfillEntity{backfillEntity("blueprint", "b1")}, nil).Once()
	repo.EXPECT().ListEntities(mock.Anything, "feed_item", "", "", false, mock.Anything, 0).
		Return([]models.BackfillEntity{backfillEntity("feed_item", "f1")}, nil).Once()
	publisher.EXPECT().Publish(mock.Anything, mock.Anything).Return(nil).Times(2)

	result, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
		EntityTypes: []string{"blueprint", "feed_item"},
	})

	require.NoError(t, err)
	require.Len(t, result.Results, 2)
	assert.Equal(t, "blueprint", result.Results[0].EntityType)
	assert.Equal(t, "feed_item", result.Results[1].EntityType)
	assert.Equal(t, 2, result.TotalPublished)
}

func TestEmbeddingBackfill_MissingOnly_PassesFilterToRepo(t *testing.T) {
	svc, repo, publisher, _ := newTestBackfillService(t)

	// Only the entity the repo's missing-only query returns is published; an
	// already-embedded entity is filtered out at the repo and never seen here.
	repo.EXPECT().ListEntities(mock.Anything, "memory", "", "", true, mock.Anything, 0).
		Return([]models.BackfillEntity{backfillEntity("memory", "missing-1")}, nil).Once()
	publisher.EXPECT().Publish(mock.Anything, mock.Anything).Return(nil).Once()

	result, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
		EntityTypes: []string{"memory"},
		MissingOnly: true,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalSeen)
	assert.Equal(t, 1, result.TotalPublished)
}

func TestEmbeddingBackfill_DryRun_CountsWithoutPublishing(t *testing.T) {
	svc, repo, publisher, _ := newTestBackfillService(t)

	repo.EXPECT().ListEntities(mock.Anything, "artifact", "", "", false, mock.Anything, 0).
		Return([]models.BackfillEntity{backfillEntity("artifact", "a1")}, nil).Once()

	result, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
		EntityTypes: []string{"artifact"},
		DryRun:      true,
	})

	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.Equal(t, 1, result.Results[0].Total)
	assert.Equal(t, 0, result.Results[0].Published)
	// Publisher is never called on a dry run.
	publisher.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything)
}

func TestEmbeddingBackfill_DryRunMissingOnly_CountsMissingWithoutPublishing(t *testing.T) {
	svc, repo, publisher, _ := newTestBackfillService(t)

	// Dry run still threads missing_only into the repo, so counts reflect only the
	// gaps; nothing is published.
	repo.EXPECT().ListEntities(mock.Anything, "artifact", "", "", true, mock.Anything, 0).
		Return([]models.BackfillEntity{backfillEntity("artifact", "a1"), backfillEntity("artifact", "a2")}, nil).Once()

	result, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
		EntityTypes: []string{"artifact"},
		MissingOnly: true,
		DryRun:      true,
	})

	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.Equal(t, 2, result.Results[0].Total)
	assert.Equal(t, 0, result.Results[0].Published)
	publisher.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything)
}

func TestEmbeddingBackfill_PublishFailure_LogsAndContinues(t *testing.T) {
	svc, repo, publisher, renderer := newTestBackfillService(t)
	passthroughRender(renderer)

	repo.EXPECT().ListEntities(mock.Anything, "prompt", "", "", false, mock.Anything, 0).
		Return([]models.BackfillEntity{backfillEntity("prompt", "p1"), backfillEntity("prompt", "p2")}, nil).Once()
	// First publish fails, second succeeds; the run must not abort.
	publisher.EXPECT().Publish(mock.Anything, mock.Anything).Return(errors.New("pubsub down")).Once()
	publisher.EXPECT().Publish(mock.Anything, mock.Anything).Return(nil).Once()

	result, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
		EntityTypes: []string{"prompt"},
	})

	require.NoError(t, err)
	assert.Equal(t, 2, result.Results[0].Total)
	assert.Equal(t, 1, result.Results[0].Published)
	assert.Equal(t, 1, result.Results[0].Failed)
	assert.Equal(t, 1, result.TotalFailed)
}

func TestEmbeddingBackfill_Pagination_WalksAllPages(t *testing.T) {
	svc, repo, publisher, _ := newTestBackfillService(t)

	// A full page (500) forces a second fetch; the short second page ends the loop.
	fullPage := make([]models.BackfillEntity, 500)
	for i := range fullPage {
		fullPage[i] = backfillEntity("memory", "m")
	}
	repo.EXPECT().ListEntities(mock.Anything, "memory", "", "", false, 500, 0).Return(fullPage, nil).Once()
	repo.EXPECT().ListEntities(mock.Anything, "memory", "", "", false, 500, 500).
		Return([]models.BackfillEntity{backfillEntity("memory", "last")}, nil).Once()
	publisher.EXPECT().Publish(mock.Anything, mock.Anything).Return(nil).Times(501)

	result, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
		EntityTypes: []string{"memory"},
	})

	require.NoError(t, err)
	assert.Equal(t, 501, result.TotalSeen)
	assert.Equal(t, 501, result.TotalPublished)
}

func TestEmbeddingBackfill_UnsupportedType_ReturnsSentinel(t *testing.T) {
	svc, _, _, _ := newTestBackfillService(t)

	_, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
		EntityTypes: []string{"widget"},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, services.ErrUnsupportedBackfillEntityType)
}

func TestEmbeddingBackfill_FeedItemReply_NoLongerSupported(t *testing.T) {
	svc, _, _, _ := newTestBackfillService(t)

	// Replies must not be embedded; feed_item_reply is rejected like any unknown type.
	_, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
		EntityTypes: []string{"feed_item_reply"},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, services.ErrUnsupportedBackfillEntityType)
}

func TestEmbeddingBackfill_BuildsCorrectEventTypePerEntity(t *testing.T) {
	cases := map[string]string{
		"prompt":    "prompt.created",
		"artifact":  "artifact.created",
		"memory":    "memory.created",
		"blueprint": "blueprint.created",
		"feed_item": "feed_item.created",
	}

	for entityType, wantEventType := range cases {
		t.Run(entityType, func(t *testing.T) {
			svc, repo, publisher, renderer := newTestBackfillService(t)
			if entityType == "prompt" {
				passthroughRender(renderer)
			}
			repo.EXPECT().ListEntities(mock.Anything, entityType, "", "", false, mock.Anything, 0).
				Return([]models.BackfillEntity{backfillEntity(entityType, "x")}, nil).Once()
			publisher.EXPECT().
				Publish(mock.Anything, mock.MatchedBy(func(e events.Event) bool {
					return e.Type() == wantEventType
				})).
				Return(nil).Once()

			_, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
				EntityTypes: []string{entityType},
			})
			require.NoError(t, err)
		})
	}
}

func TestEmbeddingBackfill_Prompt_EmbedsRenderedBody(t *testing.T) {
	svc, repo, publisher, renderer := newTestBackfillService(t)

	entity := backfillEntity("prompt", "p1")
	entity.Body = "Intro: @shared-context"                     // raw template with a reference
	const renderedBody = "Intro: resolved shared context body" // what the live path would embed

	repo.EXPECT().ListEntities(mock.Anything, "prompt", "", "", false, mock.Anything, 0).
		Return([]models.BackfillEntity{entity}, nil).Once()
	renderer.EXPECT().RenderPromptBody("user-1", entity.Body).Return(renderedBody, nil).Once()

	var captured events.Event
	publisher.EXPECT().
		Publish(mock.Anything, mock.MatchedBy(func(e events.Event) bool { captured = e; return true })).
		Return(nil).Once()

	_, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
		EntityTypes: []string{"prompt"},
	})
	require.NoError(t, err)

	payload, ok := captured.Payload().(*events.PromptCreatedPayload)
	require.True(t, ok)
	assert.Equal(t, renderedBody, payload.Body, "backfill must embed the rendered body, not the raw template")
	assert.NotContains(t, payload.Body, "@shared-context")
}

func TestEmbeddingBackfill_Prompt_RenderFailureFallsBackToRawBody(t *testing.T) {
	svc, repo, publisher, renderer := newTestBackfillService(t)

	entity := backfillEntity("prompt", "p1")
	entity.Body = "Intro: @missing-ref"

	repo.EXPECT().ListEntities(mock.Anything, "prompt", "", "", false, mock.Anything, 0).
		Return([]models.BackfillEntity{entity}, nil).Once()
	renderer.EXPECT().RenderPromptBody("user-1", entity.Body).
		Return("", errors.New("circular reference")).Once()

	var captured events.Event
	publisher.EXPECT().
		Publish(mock.Anything, mock.MatchedBy(func(e events.Event) bool { captured = e; return true })).
		Return(nil).Once()

	_, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
		EntityTypes: []string{"prompt"},
	})
	require.NoError(t, err)

	payload, ok := captured.Payload().(*events.PromptCreatedPayload)
	require.True(t, ok)
	assert.Equal(t, entity.Body, payload.Body, "render failure must fall back to the raw body, never abort")
}

func TestEmbeddingBackfill_RepoError_Aborts(t *testing.T) {
	svc, repo, _, _ := newTestBackfillService(t)

	repo.EXPECT().ListEntities(mock.Anything, "prompt", "", "", false, mock.Anything, 0).
		Return(nil, errors.New("db down")).Once()

	_, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
		EntityTypes: []string{"prompt"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "backfill of prompt failed")
}

func TestEmbeddingBackfill_PublishesBackfillOriginEvents(t *testing.T) {
	svc, repo, publisher, renderer := newTestBackfillService(t)
	passthroughRender(renderer)

	repo.EXPECT().ListEntities(mock.Anything, "prompt", "", "", false, mock.Anything, 0).
		Return([]models.BackfillEntity{backfillEntity("prompt", "p1")}, nil).Once()

	var captured events.Event
	publisher.EXPECT().
		Publish(mock.Anything, mock.MatchedBy(func(e events.Event) bool { captured = e; return true })).
		Return(nil).Once()

	_, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
		EntityTypes: []string{"prompt"},
	})
	require.NoError(t, err)

	require.NotNil(t, captured)
	assert.True(t, events.IsBackfillOrigin(captured),
		"republished events must be tagged backfill-origin so side-effect listeners skip them")
}

// --- publish-vs-embedded observability (#755) ---

// coverageResponse builds a coverage payload with one entity type, so a test can
// assert the exact numbers that reach the snapshot log line.
func coverageResponse(entityType string, total, embedded int64) *models.EmbeddingCoverageResponse {
	model := "test-model"
	return &models.EmbeddingCoverageResponse{
		HasActiveProvider: true,
		ActiveModel:       &model,
		Coverage: []models.EmbeddingCoverageItem{{
			EntityType: entityType,
			Total:      total,
			Embedded:   embedded,
			Pending:    total - embedded,
		}},
	}
}

// The terminal line must not read as an end-to-end guarantee: it says events were
// published, and keeps its field names so existing greps/alerts still match.
func TestEmbeddingBackfill_TerminalLogSaysPublishedNotCompleted(t *testing.T) {
	svc, repo, publisher, logs := newLoggingBackfillService(t, nil)

	repo.EXPECT().ListEntities(mock.Anything, "memory", "", "team-1", false, mock.Anything, 0).
		Return([]models.BackfillEntity{backfillEntity("memory", "m1")}, nil).Once()
	publisher.EXPECT().Publish(mock.Anything, mock.Anything).Return(nil).Once()

	_, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
		EntityTypes: []string{"memory"},
		TeamID:      "team-1",
	})
	require.NoError(t, err)

	out := logs.String()
	assert.Contains(t, out, "Embedding backfill: events published",
		"the terminal message must name publishing, not completion")
	assert.NotContains(t, out, "Embedding backfill completed",
		"the old end-to-end-sounding message must be gone")
	assert.Contains(t, out, "generation is async",
		"the line must say generation has not happened yet")
	// Field names are the compatibility surface and must survive the rewording.
	assert.Contains(t, out, `"total_seen":1`)
	assert.Contains(t, out, `"total_published":1`)
	assert.Contains(t, out, `"total_failed":0`)
}

// A team-scoped run with a coverage getter wired logs what actually landed, so an
// operator has one line reporting embedded/total/pending rather than only publishes.
func TestEmbeddingBackfill_LogsCoverageSnapshotAfterTeamRun(t *testing.T) {
	coverage := svcmocks.NewMockEmbeddingCoverageGetter(t)
	coverage.EXPECT().GetCoverage(mock.Anything, "team-1").
		Return(coverageResponse("memory", 10, 6), nil).Once()

	svc, repo, publisher, logs := newLoggingBackfillService(t, coverage)
	repo.EXPECT().ListEntities(mock.Anything, "memory", "", "team-1", true, mock.Anything, 0).
		Return([]models.BackfillEntity{backfillEntity("memory", "m1")}, nil).Once()
	publisher.EXPECT().Publish(mock.Anything, mock.Anything).Return(nil).Once()

	_, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
		EntityTypes: []string{"memory"},
		TeamID:      "team-1",
		MissingOnly: true,
	})
	require.NoError(t, err)

	out := logs.String()
	assert.Contains(t, out, "Embedding backfill: coverage after run")
	assert.Contains(t, out, "snapshot", "the line must say it is a snapshot, not a completion assertion")
	// One stable "coverage" key carrying the whole per-type array, so the log
	// schema does not grow a column per entity type.
	assert.Contains(t, out, `"coverage":[{"entity_type":"memory","total":10,"embedded":6,"pending":4,`)
	assert.Contains(t, out, `"has_active_provider":true`)
}

// The snapshot is skipped where it would be meaningless or wrong: no getter wired,
// a dry run (nothing was published), and an all-teams run (coverage is per-team).
func TestEmbeddingBackfill_CoverageSnapshotSkipped(t *testing.T) {
	t.Run("no coverage getter wired", func(t *testing.T) {
		svc, repo, publisher, logs := newLoggingBackfillService(t, nil)
		repo.EXPECT().ListEntities(mock.Anything, "memory", "", "team-1", false, mock.Anything, 0).
			Return([]models.BackfillEntity{backfillEntity("memory", "m1")}, nil).Once()
		publisher.EXPECT().Publish(mock.Anything, mock.Anything).Return(nil).Once()

		_, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
			EntityTypes: []string{"memory"},
			TeamID:      "team-1",
		})
		require.NoError(t, err)
		assert.NotContains(t, logs.String(), "coverage after run")
	})

	t.Run("dry run publishes nothing", func(t *testing.T) {
		// The mock is strict: a GetCoverage call it was never told to expect fails
		// the test, which is exactly the assertion.
		coverage := svcmocks.NewMockEmbeddingCoverageGetter(t)
		svc, repo, _, logs := newLoggingBackfillService(t, coverage)
		repo.EXPECT().ListEntities(mock.Anything, "memory", "", "team-1", false, mock.Anything, 0).
			Return([]models.BackfillEntity{backfillEntity("memory", "m1")}, nil).Once()

		_, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
			EntityTypes: []string{"memory"},
			TeamID:      "team-1",
			DryRun:      true,
		})
		require.NoError(t, err)
		assert.NotContains(t, logs.String(), "coverage after run")
	})

	t.Run("all-teams run has no team to report on", func(t *testing.T) {
		coverage := svcmocks.NewMockEmbeddingCoverageGetter(t)
		svc, repo, publisher, logs := newLoggingBackfillService(t, coverage)
		repo.EXPECT().ListEntities(mock.Anything, "memory", "", "", false, mock.Anything, 0).
			Return([]models.BackfillEntity{backfillEntity("memory", "m1")}, nil).Once()
		publisher.EXPECT().Publish(mock.Anything, mock.Anything).Return(nil).Once()

		_, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
			EntityTypes: []string{"memory"},
		})
		require.NoError(t, err)
		assert.NotContains(t, logs.String(), "coverage after run")
	})
}

// A coverage read failure is a diagnostic failure, not a run failure: the backfill
// still succeeds and the problem is logged.
func TestEmbeddingBackfill_CoverageReadFailureDoesNotFailTheRun(t *testing.T) {
	coverage := svcmocks.NewMockEmbeddingCoverageGetter(t)
	coverage.EXPECT().GetCoverage(mock.Anything, "team-1").
		Return(nil, errors.New("coverage query exploded")).Once()

	svc, repo, publisher, logs := newLoggingBackfillService(t, coverage)
	repo.EXPECT().ListEntities(mock.Anything, "memory", "", "team-1", false, mock.Anything, 0).
		Return([]models.BackfillEntity{backfillEntity("memory", "m1")}, nil).Once()
	publisher.EXPECT().Publish(mock.Anything, mock.Anything).Return(nil).Once()

	result, err := svc.Backfill(context.Background(), services.EmbeddingBackfillRequest{
		EntityTypes: []string{"memory"},
		TeamID:      "team-1",
	})

	require.NoError(t, err, "a diagnostic read must never fail the backfill")
	assert.Equal(t, 1, result.TotalPublished)
	out := logs.String()
	assert.Contains(t, out, "Failed to read embedding coverage after backfill")
	assert.Contains(t, out, `"level":"WARN"`)
}
