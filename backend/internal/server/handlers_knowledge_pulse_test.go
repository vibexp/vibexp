package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	knowledgepulsegen "github.com/vibexp/vibexp/internal/server/gen/knowledgepulse"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	testPulseTeamID = "550e8400-e29b-41d4-a716-446655440000"
	pulsePath       = "/api/v1/" + testPulseTeamID + "/knowledge-pulse"
)

type MockKnowledgePulseContainer struct {
	BaseMockContainer
	pulseService services.KnowledgePulseServiceInterface
}

func (c *MockKnowledgePulseContainer) KnowledgePulseService() services.KnowledgePulseServiceInterface {
	return c.pulseService
}

func createTestKnowledgePulseServer(svc services.KnowledgePulseServiceInterface) *Server {
	r := chi.NewRouter()
	srv := &Server{
		container: &MockKnowledgePulseContainer{pulseService: svc},
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
		router:    r,
	}
	strict := knowledgepulsegen.NewStrictHandlerWithOptions(
		&knowledgePulseStrictServer{s: srv},
		nil,
		knowledgepulsegen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  srv.knowledgePulseBindErrorHandler,
			ResponseErrorHandlerFunc: srv.knowledgePulseResponseErrorHandler,
		},
	)
	knowledgepulsegen.HandlerWithOptions(strict, knowledgepulsegen.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: srv.knowledgePulseBindErrorHandler,
	})
	return srv
}

func makePulseRequest(path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "660e8400-e29b-41d4-a716-446655440001"))
}

func samplePulseResult() *services.KnowledgePulseResult {
	t0 := time.Date(2026, 10, 19, 19, 29, 58, 123456000, time.UTC)
	to := models.PulseResource{Type: "artifact", ID: "a1", Title: "Runbook", Slug: "runbook", ProjectID: "p1"}
	return &services.KnowledgePulseResult{
		Cursor: t0.Add(2 * time.Second),
		Events: []services.PulseEvent{
			{ID: "read:1", Kind: "read", At: t0,
				Actor:    services.PulseActor{Type: "agent", ID: "agent:claude-code", Name: "Claude Code", Source: "mcp"},
				Resource: models.PulseResource{Type: "memory", ID: "m1", Title: "Deploy notes", ProjectID: "p1"}},
			{ID: "write:2", Kind: "write", At: t0.Add(time.Second), Action: "update",
				Actor:    services.PulseActor{Type: "user", ID: "u1", Name: "Ada", Source: "web"},
				Resource: models.PulseResource{Type: "prompt", ID: "pr1", Title: "Review", Slug: "review", ProjectID: "p1"}},
			{ID: "link:3", Kind: "link", At: t0.Add(2 * time.Second), RelationType: "supersedes", To: &to,
				Actor:    services.PulseActor{Type: "agent", ID: "agent:ai-agent", Name: "AI agent", Source: "mcp"},
				Resource: models.PulseResource{Type: "memory", ID: "m1", Title: "Deploy notes", ProjectID: "p1"}},
		},
		Snapshot: &services.PulseSnapshot{
			Nodes: []models.PulseNode{{Resource: models.PulseResource{Type: "memory", ID: "m1", Title: "Deploy notes"}, Degree: 1}},
			Edges: []models.PulseEdge{{FromID: "m1", ToID: "a1", RelationType: "supersedes"}},
			Actors: []services.PulseActor{
				{Type: "agent", ID: "agent:claude-code", Name: "Claude Code", Source: "mcp"},
			},
		},
		Stats: services.PulseStats{Reads: 1, Writes: 1, Links: 1, CompoundingScore: 2},
	}
}

func TestGetKnowledgePulse_FirstLoadConformsToSpec(t *testing.T) {
	svc := servicesmocks.NewMockKnowledgePulseServiceInterface(t)
	svc.EXPECT().GetPulse(mock.Anything, testPulseTeamID, services.KnowledgePulseQuery{}).
		Return(samplePulseResult(), nil)

	srv := createTestKnowledgePulseServer(svc)
	req := makePulseRequest(pulsePath)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "2026-10-19T19:30:00.123456Z", resp["cursor"])
	events := resp["events"].([]any)
	require.Len(t, events, 3)
	read := events[0].(map[string]any)
	assert.NotContains(t, read, "detail")
	assert.NotContains(t, read["resource"], "slug", "an empty slug is omitted")
	link := events[2].(map[string]any)["detail"].(map[string]any)
	assert.Equal(t, "supersedes", link["relation_type"])
	assert.Equal(t, "runbook", link["to"].(map[string]any)["slug"])
	assert.Contains(t, resp, "snapshot")
	assert.Equal(t, float64(2), resp["stats_today"].(map[string]any)["compounding_score"])
}

func TestGetKnowledgePulse_DeltaPassesParamsAndOmitsSnapshot(t *testing.T) {
	since := time.Date(2026, 10, 19, 19, 30, 0, 123456000, time.UTC)
	svc := servicesmocks.NewMockKnowledgePulseServiceInterface(t)
	svc.EXPECT().GetPulse(mock.Anything, testPulseTeamID, mock.MatchedBy(func(q services.KnowledgePulseQuery) bool {
		return q.Since != nil && q.Since.Equal(since) && q.Limit == 50 && q.IncludeSnapshot
	})).Return(&services.KnowledgePulseResult{Cursor: since}, nil)

	srv := createTestKnowledgePulseServer(svc)
	req := makePulseRequest(pulsePath + "?since=2026-10-19T19:30:00.123456Z&limit=50&snapshot=1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, []any{}, resp["events"], "no events serializes as [], never null")
	assert.NotContains(t, resp, "snapshot")
}

func TestGetKnowledgePulse_InvalidSince(t *testing.T) {
	srv := createTestKnowledgePulseServer(servicesmocks.NewMockKnowledgePulseServiceInterface(t))
	req := makePulseRequest(pulsePath + "?since=yesterday")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetKnowledgePulse_ServiceErrorIs500(t *testing.T) {
	svc := servicesmocks.NewMockKnowledgePulseServiceInterface(t)
	svc.EXPECT().GetPulse(mock.Anything, testPulseTeamID, mock.Anything).Return(nil, errors.New("db down"))

	srv := createTestKnowledgePulseServer(svc)
	req := makePulseRequest(pulsePath)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "db down")
}
