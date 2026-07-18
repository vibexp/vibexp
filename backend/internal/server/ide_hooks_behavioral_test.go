package server

// Behavioral coverage for the 14 IDE-hooks operations (issue #360, #122
// conformance rule): every recorded response for a documented operation is
// validated against openapi.yaml via specconformance.AssertConformsToSpec,
// and the freed entries are deleted from the payload-coverage ledger.
//
// The Claude Code and Cursor IDE families are structural mirrors, so the
// tests are table-driven over both families wherever the behavior is shared.
// 401-middleware coverage already lives in claude_code_hooks_test.go /
// cursor_ide_hooks_test.go and is intentionally not repeated here.

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	hooksTestUserID    = "hooks-user-123"
	hooksTestTeamID    = "990e8400-e29b-41d4-a716-446655440000"
	hooksTestSessionID = "sess-abc-123"
)

// mockIDEHooksContainer wires mocked hooks repositories (and, for the POST
// flow, auth/team services) into the server under test.
type mockIDEHooksContainer struct {
	BaseMockContainer
	claudeRepo *repomocks.MockClaudeCodeHooksRepository
	cursorRepo *repomocks.MockCursorIDEHooksRepository
	authSvc    services.AuthServiceInterface
	teamSvc    services.TeamServiceInterface
}

func (c *mockIDEHooksContainer) ClaudeCodeHooksRepository() repositories.ClaudeCodeHooksRepository {
	return c.claudeRepo
}

func (c *mockIDEHooksContainer) CursorIDEHooksRepository() repositories.CursorIDEHooksRepository {
	return c.cursorRepo
}

func (c *mockIDEHooksContainer) AuthService() services.AuthServiceInterface {
	if c.authSvc != nil {
		return c.authSvc
	}
	return c.BaseMockContainer.AuthService()
}

func (c *mockIDEHooksContainer) TeamService() services.TeamServiceInterface {
	if c.teamSvc != nil {
		return c.teamSvc
	}
	return c.BaseMockContainer.TeamService()
}

func newIDEHooksTestServer(t *testing.T) (*Server, *mockIDEHooksContainer) {
	t.Helper()
	c := &mockIDEHooksContainer{
		claudeRepo: repomocks.NewMockClaudeCodeHooksRepository(t),
		cursorRepo: repomocks.NewMockCursorIDEHooksRepository(t),
	}
	srv := &Server{
		port:      "8080",
		container: c,
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
	}
	return srv, c
}

// newHooksAuthedRequest builds a request carrying the authenticated test
// user in context, the way the auth middleware would.
func newHooksAuthedRequest(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, hooksTestUserID))
}

// wireHooksPostTeamLookup satisfies getUserDefaultTeamID for the POST flow.
func wireHooksPostTeamLookup(t *testing.T, c *mockIDEHooksContainer) {
	t.Helper()
	teamID := hooksTestTeamID
	authSvc := servicesmocks.NewMockAuthServiceInterface(t)
	authSvc.On("GetUserByID", mock.Anything, hooksTestUserID).
		Return(&models.User{ID: hooksTestUserID, Email: "dev@example.com", DefaultTeamID: &teamID}, nil)
	teamSvc := servicesmocks.NewMockTeamServiceInterface(t)
	teamSvc.On("IsUserMemberOfTeam", mock.Anything, hooksTestUserID, hooksTestTeamID).Return(true, nil)
	teamSvc.On("GetTeam", mock.Anything, hooksTestUserID, hooksTestTeamID).
		Return(&models.Team{ID: hooksTestTeamID, OwnerID: hooksTestUserID, Name: "Dev Team"}, nil)
	c.authSvc = authSvc
	c.teamSvc = teamSvc
}

func hooksTestTime() time.Time {
	return time.Date(2026, 7, 17, 10, 30, 0, 0, time.UTC)
}

func sampleClaudeHookRecord() models.ClaudeCodeHookPayload {
	userID := hooksTestUserID
	cwd := "/home/dev/project"
	tool := "Bash"
	return models.ClaudeCodeHookPayload{
		ID:            42,
		UserID:        &userID,
		TeamID:        hooksTestTeamID,
		SessionID:     hooksTestSessionID,
		CWD:           &cwd,
		HookEventName: "PreToolUse",
		ToolName:      &tool,
		Payload:       models.JSONBData{"session_id": hooksTestSessionID, "hook_event_name": "PreToolUse"},
		CreatedAt:     hooksTestTime(),
		UpdatedAt:     hooksTestTime(),
	}
}

func sampleCursorHookRecord() models.CursorIDEHookPayload {
	userID := hooksTestUserID
	conversationID := hooksTestSessionID
	tool := "shell"
	return models.CursorIDEHookPayload{
		ID:             42,
		UserID:         &userID,
		TeamID:         hooksTestTeamID,
		SessionID:      hooksTestSessionID,
		ConversationID: &conversationID,
		HookEventName:  "beforeShellExecution",
		ToolName:       &tool,
		Payload:        models.JSONBData{"conversation_id": hooksTestSessionID},
		CreatedAt:      hooksTestTime(),
		UpdatedAt:      hooksTestTime(),
	}
}

// --- POST /api/v1/{claude-code,cursor-ide}/hooks ------------------------------

func TestIDEHooksPost_HappyPath(t *testing.T) {
	t.Run("claude-code stores payload under authenticated user", func(t *testing.T) {
		srv, c := newIDEHooksTestServer(t)
		wireHooksPostTeamLookup(t, c)

		// Existing session: no resource-limit checks, no first-use event.
		c.claudeRepo.On("SessionExists", mock.Anything, hooksTestUserID, hooksTestSessionID).
			Return(true, nil)
		var stored *models.ClaudeCodeHookPayload
		c.claudeRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.ClaudeCodeHookPayload")).
			Run(func(args mock.Arguments) {
				stored = args.Get(1).(*models.ClaudeCodeHookPayload)
				stored.ID = 42
				stored.CreatedAt = hooksTestTime()
				stored.UpdatedAt = hooksTestTime()
			}).Return(nil)

		body := `{"session_id":"` + hooksTestSessionID +
			`","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"ls"}}`
		req := newHooksAuthedRequest(t, http.MethodPost, "/api/v1/claude-code/hooks", body)
		rr := httptest.NewRecorder()
		srv.handleClaudeCodeHooksPost(rr, req)

		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)

		// Tenancy: the stored payload is attributed to the authenticated user.
		require.NotNil(t, stored)
		require.NotNil(t, stored.UserID)
		assert.Equal(t, hooksTestUserID, *stored.UserID)
		assert.Equal(t, hooksTestTeamID, stored.TeamID)
		assert.Equal(t, hooksTestSessionID, stored.SessionID)
		assert.Equal(t, "PreToolUse", stored.HookEventName)
	})

	t.Run("cursor-ide falls back to conversation_id as session id", func(t *testing.T) {
		srv, c := newIDEHooksTestServer(t)
		wireHooksPostTeamLookup(t, c)

		c.cursorRepo.On("SessionExists", mock.Anything, hooksTestUserID, "conv-777").
			Return(true, nil)
		var stored *models.CursorIDEHookPayload
		c.cursorRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.CursorIDEHookPayload")).
			Run(func(args mock.Arguments) {
				stored = args.Get(1).(*models.CursorIDEHookPayload)
				stored.ID = 43
				stored.CreatedAt = hooksTestTime()
				stored.UpdatedAt = hooksTestTime()
			}).Return(nil)

		body := `{"conversation_id":"conv-777","hook_event_name":"beforeShellExecution","command":"ls"}`
		req := newHooksAuthedRequest(t, http.MethodPost, "/api/v1/cursor-ide/hooks", body)
		rr := httptest.NewRecorder()
		srv.handleCursorIDEHooksPost(rr, req)

		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)

		require.NotNil(t, stored)
		require.NotNil(t, stored.UserID)
		assert.Equal(t, hooksTestUserID, *stored.UserID)
		assert.Equal(t, hooksTestTeamID, stored.TeamID)
		assert.Equal(t, "conv-777", stored.SessionID, "conversation_id must be used as session_id")
		require.NotNil(t, stored.ConversationID)
		assert.Equal(t, "conv-777", *stored.ConversationID)
	})
}

func TestIDEHooksPost_InvalidJSON(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		handler func(s *Server) http.HandlerFunc
	}{
		{"claude-code", "/api/v1/claude-code/hooks",
			func(s *Server) http.HandlerFunc { return s.handleClaudeCodeHooksPost }},
		{"cursor-ide", "/api/v1/cursor-ide/hooks",
			func(s *Server) http.HandlerFunc { return s.handleCursorIDEHooksPost }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newIDEHooksTestServer(t)
			req := newHooksAuthedRequest(t, http.MethodPost, tc.target, `{not-json`)
			rr := httptest.NewRecorder()
			tc.handler(srv)(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			specconformance.AssertConformsToSpec(t, req, rr)
			assert.Contains(t, rr.Body.String(), "Invalid JSON payload")
		})
	}
}

func TestIDEHooksPost_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name       string
		target     string
		handler    func(s *Server) http.HandlerFunc
		body       string
		wantInBody string
	}{
		{"claude-code missing session_id", "/api/v1/claude-code/hooks",
			func(s *Server) http.HandlerFunc { return s.handleClaudeCodeHooksPost },
			`{"hook_event_name":"PreToolUse"}`,
			"Missing required field: session_id"},
		{"claude-code missing hook_event_name", "/api/v1/claude-code/hooks",
			func(s *Server) http.HandlerFunc { return s.handleClaudeCodeHooksPost },
			`{"session_id":"sess-1"}`,
			"Missing required field: hook_event_name"},
		{"cursor-ide missing session_id and conversation_id", "/api/v1/cursor-ide/hooks",
			func(s *Server) http.HandlerFunc { return s.handleCursorIDEHooksPost },
			`{"hook_event_name":"beforeShellExecution"}`,
			"Missing required field: session_id or conversation_id"},
		{"cursor-ide missing hook_event_name", "/api/v1/cursor-ide/hooks",
			func(s *Server) http.HandlerFunc { return s.handleCursorIDEHooksPost },
			`{"conversation_id":"conv-1"}`,
			"Missing required field: hook_event_name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newIDEHooksTestServer(t)
			req := newHooksAuthedRequest(t, http.MethodPost, tc.target, tc.body)
			rr := httptest.NewRecorder()
			tc.handler(srv)(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			specconformance.AssertConformsToSpec(t, req, rr)
			assert.Contains(t, rr.Body.String(), tc.wantInBody)
		})
	}
}

// --- GET /api/v1/ai-tools/{family}/hooks --------------------------------------

func TestIDEHooksList_DefaultsAndTenancy(t *testing.T) {
	t.Run("claude-code", func(t *testing.T) {
		srv, c := newIDEHooksTestServer(t)
		c.claudeRepo.On("List", mock.Anything, mock.MatchedBy(func(f repositories.ClaudeCodeHooksFilters) bool {
			return f.UserID != nil && *f.UserID == hooksTestUserID && f.Page == 1 && f.Limit == 10
		})).Return(&models.ClaudeCodeHooksPaginatedResponse{
			Data:       models.JSONArray[models.ClaudeCodeHookPayload]{sampleClaudeHookRecord()},
			Page:       1,
			Limit:      10,
			Total:      1,
			TotalPages: 1,
		}, nil)

		req := newHooksAuthedRequest(t, http.MethodGet, "/api/v1/ai-tools/claude-code/hooks", "")
		rr := httptest.NewRecorder()
		srv.handleClaudeCodeHooksGet(rr, req)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.Contains(t, rr.Body.String(), hooksTestSessionID)
	})

	t.Run("cursor-ide", func(t *testing.T) {
		srv, c := newIDEHooksTestServer(t)
		c.cursorRepo.On("List", mock.Anything, mock.MatchedBy(func(f repositories.CursorIDEHooksFilters) bool {
			return f.UserID != nil && *f.UserID == hooksTestUserID && f.Page == 1 && f.Limit == 10
		})).Return(&models.CursorIDEHooksPaginatedResponse{
			Data:       models.JSONArray[models.CursorIDEHookPayload]{sampleCursorHookRecord()},
			Page:       1,
			Limit:      10,
			Total:      1,
			TotalPages: 1,
		}, nil)

		req := newHooksAuthedRequest(t, http.MethodGet, "/api/v1/ai-tools/cursor-ide/hooks", "")
		rr := httptest.NewRecorder()
		srv.handleCursorIDEHooksGet(rr, req)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.Contains(t, rr.Body.String(), hooksTestSessionID)
	})
}

func TestIDEHooksList_PaginationCapAndFilters(t *testing.T) {
	const target = "?page=3&limit=250&session_id=" + hooksTestSessionID
	t.Run("claude-code caps limit at 100", func(t *testing.T) {
		srv, c := newIDEHooksTestServer(t)
		c.claudeRepo.On("List", mock.Anything, mock.MatchedBy(func(f repositories.ClaudeCodeHooksFilters) bool {
			return f.UserID != nil && *f.UserID == hooksTestUserID &&
				f.Page == 3 && f.Limit == 100 &&
				f.SessionID != nil && *f.SessionID == hooksTestSessionID
		})).Return(&models.ClaudeCodeHooksPaginatedResponse{
			Data: models.JSONArray[models.ClaudeCodeHookPayload]{}, Page: 3, Limit: 100, Total: 0, TotalPages: 0,
		}, nil)

		req := newHooksAuthedRequest(t, http.MethodGet, "/api/v1/ai-tools/claude-code/hooks"+target, "")
		rr := httptest.NewRecorder()
		srv.handleClaudeCodeHooksGet(rr, req)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	t.Run("cursor-ide caps limit at 100", func(t *testing.T) {
		srv, c := newIDEHooksTestServer(t)
		c.cursorRepo.On("List", mock.Anything, mock.MatchedBy(func(f repositories.CursorIDEHooksFilters) bool {
			return f.UserID != nil && *f.UserID == hooksTestUserID &&
				f.Page == 3 && f.Limit == 100 &&
				f.SessionID != nil && *f.SessionID == hooksTestSessionID
		})).Return(&models.CursorIDEHooksPaginatedResponse{
			Data: models.JSONArray[models.CursorIDEHookPayload]{}, Page: 3, Limit: 100, Total: 0, TotalPages: 0,
		}, nil)

		req := newHooksAuthedRequest(t, http.MethodGet, "/api/v1/ai-tools/cursor-ide/hooks"+target, "")
		rr := httptest.NewRecorder()
		srv.handleCursorIDEHooksGet(rr, req)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
	})
}

// --- GET /api/v1/ai-tools/{family}/sessions -----------------------------------

func TestIDESessionsList_DefaultsAndTenancy(t *testing.T) {
	t.Run("claude-code", func(t *testing.T) {
		srv, c := newIDEHooksTestServer(t)
		cwd := "/home/dev/project"
		c.claudeRepo.On("GetSessions", mock.Anything, mock.MatchedBy(func(f repositories.SessionFilters) bool {
			return f.UserID != nil && *f.UserID == hooksTestUserID && f.Page == 1 && f.Limit == 10
		})).Return(&models.SessionsResponse{
			Data: models.JSONArray[models.SessionSummary]{{
				SessionID:   hooksTestSessionID,
				FirstSeen:   hooksTestTime(),
				LastSeen:    hooksTestTime().Add(time.Hour),
				HookCount:   12,
				LatestCWD:   &cwd,
				UniqueTools: 3,
			}},
			Page: 1, Limit: 10, Total: 1, TotalPages: 1,
		}, nil)

		req := newHooksAuthedRequest(t, http.MethodGet, "/api/v1/ai-tools/claude-code/sessions", "")
		rr := httptest.NewRecorder()
		srv.handleClaudeCodeSessionsGet(rr, req)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.Contains(t, rr.Body.String(), hooksTestSessionID)
	})

	t.Run("cursor-ide", func(t *testing.T) {
		srv, c := newIDEHooksTestServer(t)
		c.cursorRepo.On("GetSessions", mock.Anything, mock.MatchedBy(func(f repositories.CursorSessionFilters) bool {
			return f.UserID != nil && *f.UserID == hooksTestUserID && f.Page == 1 && f.Limit == 10
		})).Return(&models.CursorSessionsResponse{
			Data: models.JSONArray[models.CursorSessionSummary]{{
				SessionID:   hooksTestSessionID,
				FirstSeen:   hooksTestTime(),
				LastSeen:    hooksTestTime().Add(time.Hour),
				HookCount:   12,
				UniqueTools: 3,
			}},
			Page: 1, Limit: 10, Total: 1, TotalPages: 1,
		}, nil)

		req := newHooksAuthedRequest(t, http.MethodGet, "/api/v1/ai-tools/cursor-ide/sessions", "")
		rr := httptest.NewRecorder()
		srv.handleCursorIDESessionsGet(rr, req)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.Contains(t, rr.Body.String(), hooksTestSessionID)
	})
}

// --- GET /api/v1/ai-tools/{family}/session-counts -----------------------------

func TestIDESessionCounts_RangeAndTenancy(t *testing.T) {
	counts := func() *models.SessionCountsResponse {
		return &models.SessionCountsResponse{
			TotalSessions: 5,
			Counts: models.JSONArray[models.SessionCountByDate]{
				{Date: "2026-07-16", Count: 2},
				{Date: "2026-07-17", Count: 3},
			},
		}
	}
	cursorCounts := func() *models.CursorSessionCountsResponse {
		return &models.CursorSessionCountsResponse{
			TotalSessions: 5,
			Counts: models.JSONArray[models.SessionCountByDate]{
				{Date: "2026-07-16", Count: 2},
				{Date: "2026-07-17", Count: 3},
			},
		}
	}

	cases := []struct {
		name     string
		query    string
		wantDays int
	}{
		{"default range is 7 days", "", 7},
		{"30d range", "?range=30d", 30},
		{"unknown range falls back to 7 days", "?range=bogus", 7},
	}

	for _, tc := range cases {
		t.Run("claude-code "+tc.name, func(t *testing.T) {
			srv, c := newIDEHooksTestServer(t)
			c.claudeRepo.On("GetSessionCounts", mock.Anything, hooksTestUserID, tc.wantDays).
				Return(counts(), nil)

			req := newHooksAuthedRequest(t, http.MethodGet, "/api/v1/ai-tools/claude-code/session-counts"+tc.query, "")
			rr := httptest.NewRecorder()
			srv.handleClaudeCodeSessionCountsGet(rr, req)

			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			specconformance.AssertConformsToSpec(t, req, rr)
		})

		t.Run("cursor-ide "+tc.name, func(t *testing.T) {
			srv, c := newIDEHooksTestServer(t)
			c.cursorRepo.On("GetSessionCounts", mock.Anything, hooksTestUserID, tc.wantDays).
				Return(cursorCounts(), nil)

			req := newHooksAuthedRequest(t, http.MethodGet, "/api/v1/ai-tools/cursor-ide/session-counts"+tc.query, "")
			rr := httptest.NewRecorder()
			srv.handleCursorIDESessionCountsGet(rr, req)

			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// --- GET /api/v1/ai-tools/{family}/overview-stats -----------------------------

func TestIDEOverviewStats_HappyPath(t *testing.T) {
	t.Run("claude-code", func(t *testing.T) {
		srv, c := newIDEHooksTestServer(t)
		c.claudeRepo.On("GetOverviewStats", mock.Anything, hooksTestUserID).
			Return(&models.OverviewStats{
				TotalSessions:             10,
				SessionsThisWeek:          4,
				SessionsLastWeek:          2,
				WeeklyTrendPercent:        100,
				AvgUserPromptsPerSession:  5.5,
				TotalUniqueTools:          6,
				TopTools:                  models.JSONArray[models.ToolUsageCount]{{ToolName: "Bash", Count: 12}},
				AvgSessionDurationMinutes: 34.2,
				TotalMemories:             3,
			}, nil)

		req := newHooksAuthedRequest(t, http.MethodGet, "/api/v1/ai-tools/claude-code/overview-stats", "")
		rr := httptest.NewRecorder()
		srv.handleClaudeCodeOverviewStatsGet(rr, req)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.Contains(t, rr.Body.String(), `"total_sessions":10`)
	})

	t.Run("cursor-ide", func(t *testing.T) {
		srv, c := newIDEHooksTestServer(t)
		c.cursorRepo.On("GetOverviewStats", mock.Anything, hooksTestUserID).
			Return(&models.CursorOverviewStats{
				TotalSessions:             10,
				SessionsThisWeek:          4,
				SessionsLastWeek:          2,
				WeeklyTrendPercent:        100,
				AvgUserPromptsPerSession:  5.5,
				TotalUniqueTools:          6,
				TopTools:                  models.JSONArray[models.ToolUsageCount]{{ToolName: "shell", Count: 12}},
				AvgSessionDurationMinutes: 34.2,
			}, nil)

		req := newHooksAuthedRequest(t, http.MethodGet, "/api/v1/ai-tools/cursor-ide/overview-stats", "")
		rr := httptest.NewRecorder()
		srv.handleCursorIDEOverviewStatsGet(rr, req)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.Contains(t, rr.Body.String(), `"total_sessions":10`)
	})
}

// --- GET /api/v1/ai-tools/{family}/recent-activities --------------------------

func TestIDERecentActivities_DefaultsAndTenancy(t *testing.T) {
	t.Run("claude-code default limit is 20", func(t *testing.T) {
		srv, c := newIDEHooksTestServer(t)
		tool := "Bash"
		c.claudeRepo.On("GetRecentActivities", mock.Anything,
			mock.MatchedBy(func(f repositories.RecentActivitiesFilters) bool {
				return f.UserID != nil && *f.UserID == hooksTestUserID && f.Page == 1 && f.Limit == 20
			})).Return(&models.RecentActivitiesResponse{
			Activities: models.JSONArray[models.RecentActivity]{{
				SessionID:     hooksTestSessionID,
				ToolName:      &tool,
				HookEventName: "PreToolUse",
				CreatedAt:     hooksTestTime(),
			}},
			Page: 1, Limit: 20, Total: 1, TotalPages: 1,
		}, nil)

		req := newHooksAuthedRequest(t, http.MethodGet, "/api/v1/ai-tools/claude-code/recent-activities", "")
		rr := httptest.NewRecorder()
		srv.handleClaudeCodeRecentActivitiesGet(rr, req)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.Contains(t, rr.Body.String(), hooksTestSessionID)
	})

	t.Run("cursor-ide default limit is 20", func(t *testing.T) {
		srv, c := newIDEHooksTestServer(t)
		tool := "shell"
		c.cursorRepo.On("GetRecentActivities", mock.Anything,
			mock.MatchedBy(func(f repositories.CursorRecentActivitiesFilters) bool {
				return f.UserID != nil && *f.UserID == hooksTestUserID && f.Page == 1 && f.Limit == 20
			})).Return(&models.CursorRecentActivitiesResponse{
			Activities: models.JSONArray[models.CursorRecentActivity]{{
				SessionID:     hooksTestSessionID,
				ToolName:      &tool,
				HookEventName: "beforeShellExecution",
				CreatedAt:     hooksTestTime(),
			}},
			Page: 1, Limit: 20, Total: 1, TotalPages: 1,
		}, nil)

		req := newHooksAuthedRequest(t, http.MethodGet, "/api/v1/ai-tools/cursor-ide/recent-activities", "")
		rr := httptest.NewRecorder()
		srv.handleCursorIDERecentActivitiesGet(rr, req)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.Contains(t, rr.Body.String(), hooksTestSessionID)
	})
}

// --- DELETE /api/v1/ai-tools/{family}/sessions/{session_id} -------------------

func TestIDESessionDelete_Behavior(t *testing.T) {
	cases := []struct {
		name       string
		target     string
		handler    func(s *Server) http.HandlerFunc
		setup      func(t *testing.T, c *mockIDEHooksContainer)
		wantStatus int
	}{
		{
			name:   "claude-code deletes the authenticated user's session",
			target: "/api/v1/ai-tools/claude-code/sessions/" + hooksTestSessionID,
			handler: func(s *Server) http.HandlerFunc {
				return s.handleClaudeCodeSessionDelete
			},
			setup: func(t *testing.T, c *mockIDEHooksContainer) {
				t.Helper()
				c.claudeRepo.On("DeleteSession", mock.Anything, hooksTestUserID, hooksTestSessionID).
					Return(nil)
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:   "claude-code returns 404 for a session it cannot see",
			target: "/api/v1/ai-tools/claude-code/sessions/" + hooksTestSessionID,
			handler: func(s *Server) http.HandlerFunc {
				return s.handleClaudeCodeSessionDelete
			},
			setup: func(t *testing.T, c *mockIDEHooksContainer) {
				t.Helper()
				c.claudeRepo.On("DeleteSession", mock.Anything, hooksTestUserID, hooksTestSessionID).
					Return(repositories.ErrHookSessionNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "cursor-ide deletes the authenticated user's session",
			target: "/api/v1/ai-tools/cursor-ide/sessions/" + hooksTestSessionID,
			handler: func(s *Server) http.HandlerFunc {
				return s.handleCursorIDESessionDelete
			},
			setup: func(t *testing.T, c *mockIDEHooksContainer) {
				t.Helper()
				c.cursorRepo.On("DeleteSession", mock.Anything, hooksTestUserID, hooksTestSessionID).
					Return(nil)
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:   "cursor-ide returns 404 for a session it cannot see",
			target: "/api/v1/ai-tools/cursor-ide/sessions/" + hooksTestSessionID,
			handler: func(s *Server) http.HandlerFunc {
				return s.handleCursorIDESessionDelete
			},
			setup: func(t *testing.T, c *mockIDEHooksContainer) {
				t.Helper()
				c.cursorRepo.On("DeleteSession", mock.Anything, hooksTestUserID, hooksTestSessionID).
					Return(repositories.ErrHookSessionNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, c := newIDEHooksTestServer(t)
			tc.setup(t, c)

			req := newHooksAuthedRequest(t, http.MethodDelete, tc.target, "")
			req.SetPathValue("session_id", hooksTestSessionID)
			rr := httptest.NewRecorder()
			tc.handler(srv)(rr, req)

			require.Equal(t, tc.wantStatus, rr.Code, rr.Body.String())
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}
