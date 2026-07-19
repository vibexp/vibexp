package server

// Repository-error arms for the IDE-hooks GET/DELETE handlers (coverage epic
// #358 / issue #393). The behavioral suite (ide_hooks_behavioral_test.go)
// covers the happy paths and the delete-not-found arm; this file drives the
// remaining "repository failed" branches (→ 500) across both the Claude Code
// and Cursor families, reusing that suite's mock container + authed-request
// helpers.

import (
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestIDEHooksGetHandlers_RepositoryError(t *testing.T) {
	repoErr := stderrors.New("hooks repo unavailable")

	tests := []struct {
		name   string
		method string
		target string
		setup  func(c *mockIDEHooksContainer)
		invoke func(s *Server, w http.ResponseWriter, r *http.Request)
	}{
		{
			"claude sessions", http.MethodGet, "/api/v1/ai-tools/claude-code/sessions",
			func(c *mockIDEHooksContainer) {
				c.claudeRepo.On("GetSessions", mock.Anything, mock.Anything).Return(nil, repoErr)
			},
			func(s *Server, w http.ResponseWriter, r *http.Request) { s.handleClaudeCodeSessionsGet(w, r) },
		},
		{
			"cursor sessions", http.MethodGet, "/api/v1/ai-tools/cursor-ide/sessions",
			func(c *mockIDEHooksContainer) {
				c.cursorRepo.On("GetSessions", mock.Anything, mock.Anything).Return(nil, repoErr)
			},
			func(s *Server, w http.ResponseWriter, r *http.Request) { s.handleCursorIDESessionsGet(w, r) },
		},
		{
			"claude session-counts", http.MethodGet, "/api/v1/ai-tools/claude-code/session-counts",
			func(c *mockIDEHooksContainer) {
				c.claudeRepo.On("GetSessionCounts", mock.Anything, mock.Anything, mock.Anything).Return(nil, repoErr)
			},
			func(s *Server, w http.ResponseWriter, r *http.Request) { s.handleClaudeCodeSessionCountsGet(w, r) },
		},
		{
			"cursor session-counts", http.MethodGet, "/api/v1/ai-tools/cursor-ide/session-counts",
			func(c *mockIDEHooksContainer) {
				c.cursorRepo.On("GetSessionCounts", mock.Anything, mock.Anything, mock.Anything).Return(nil, repoErr)
			},
			func(s *Server, w http.ResponseWriter, r *http.Request) { s.handleCursorIDESessionCountsGet(w, r) },
		},
		{
			"claude overview-stats", http.MethodGet, "/api/v1/ai-tools/claude-code/overview-stats",
			func(c *mockIDEHooksContainer) {
				c.claudeRepo.On("GetOverviewStats", mock.Anything, mock.Anything).Return(nil, repoErr)
			},
			func(s *Server, w http.ResponseWriter, r *http.Request) { s.handleClaudeCodeOverviewStatsGet(w, r) },
		},
		{
			"cursor overview-stats", http.MethodGet, "/api/v1/ai-tools/cursor-ide/overview-stats",
			func(c *mockIDEHooksContainer) {
				c.cursorRepo.On("GetOverviewStats", mock.Anything, mock.Anything).Return(nil, repoErr)
			},
			func(s *Server, w http.ResponseWriter, r *http.Request) { s.handleCursorIDEOverviewStatsGet(w, r) },
		},
		{
			"claude recent-activities", http.MethodGet, "/api/v1/ai-tools/claude-code/recent-activities",
			func(c *mockIDEHooksContainer) {
				c.claudeRepo.On("GetRecentActivities", mock.Anything, mock.Anything, mock.Anything).Return(nil, repoErr)
			},
			func(s *Server, w http.ResponseWriter, r *http.Request) { s.handleClaudeCodeRecentActivitiesGet(w, r) },
		},
		{
			"cursor recent-activities", http.MethodGet, "/api/v1/ai-tools/cursor-ide/recent-activities",
			func(c *mockIDEHooksContainer) {
				c.cursorRepo.On("GetRecentActivities", mock.Anything, mock.Anything, mock.Anything).Return(nil, repoErr)
			},
			func(s *Server, w http.ResponseWriter, r *http.Request) { s.handleCursorIDERecentActivitiesGet(w, r) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, c := newIDEHooksTestServer(t)
			tt.setup(c)

			req := newHooksAuthedRequest(t, tt.method, tt.target, "")
			rr := httptest.NewRecorder()
			tt.invoke(srv, rr, req)

			assert.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())
		})
	}
}

func TestIDESessionDelete_RepositoryError(t *testing.T) {
	repoErr := stderrors.New("hooks repo unavailable")

	tests := []struct {
		name    string
		target  string
		setup   func(c *mockIDEHooksContainer)
		handler func(s *Server) http.HandlerFunc
	}{
		{
			"claude-code", "/api/v1/ai-tools/claude-code/sessions/" + hooksTestSessionID,
			func(c *mockIDEHooksContainer) {
				c.claudeRepo.On("DeleteSession", mock.Anything, hooksTestUserID, hooksTestSessionID).Return(repoErr)
			},
			func(s *Server) http.HandlerFunc { return s.handleClaudeCodeSessionDelete },
		},
		{
			"cursor-ide", "/api/v1/ai-tools/cursor-ide/sessions/" + hooksTestSessionID,
			func(c *mockIDEHooksContainer) {
				c.cursorRepo.On("DeleteSession", mock.Anything, hooksTestUserID, hooksTestSessionID).Return(repoErr)
			},
			func(s *Server) http.HandlerFunc { return s.handleCursorIDESessionDelete },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, c := newIDEHooksTestServer(t)
			tt.setup(c)

			req := newHooksAuthedRequest(t, http.MethodDelete, tt.target, "")
			req.SetPathValue("session_id", hooksTestSessionID)
			rr := httptest.NewRecorder()
			tt.handler(srv)(rr, req)

			assert.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())
		})
	}
}
