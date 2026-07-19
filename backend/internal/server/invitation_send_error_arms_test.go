package server

// Coverage for the invitation SEND path of team_invitation_handlers.go beyond
// #363's accept/reject arms (coverage epic #358 / issue #393):
// validateInvitationRequest's decode/validation branches and the
// handleInvitationError classification chain (personal workspace, duplicate
// members, no subscription, seat limit, permission, not-found, generic). Both
// are pure over (ResponseWriter[, Request], error) — driven directly, one row
// per branch.

import (
	stderrors "errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/services"
)

func invSendErrArmsServer() *Server {
	return &Server{logger: slog.New(slog.DiscardHandler)}
}

func TestValidateInvitationRequest_Arms(t *testing.T) {
	srv := invSendErrArmsServer()

	t.Run("valid body returns the parsed request", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/t/invitations",
			strings.NewReader(`{"emails":["a@example.com"],"role":"member"}`))
		got, role, ok := srv.validateInvitationRequest(rr, req, "user-1", "team-1")
		require.True(t, ok)
		assert.Equal(t, []string{"a@example.com"}, got.Emails)
		assert.EqualValues(t, "member", role)
	})

	t.Run("malformed JSON → 400", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/t/invitations",
			strings.NewReader(`{not json`))
		_, _, ok := srv.validateInvitationRequest(rr, req, "user-1", "team-1")
		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("empty emails fails struct validation → 400", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/t/invitations",
			strings.NewReader(`{"emails":[],"role":"member"}`))
		_, _, ok := srv.validateInvitationRequest(rr, req, "user-1", "team-1")
		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("bad role fails struct validation → 400", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/t/invitations",
			strings.NewReader(`{"emails":["a@example.com"],"role":"superadmin"}`))
		_, _, ok := srv.validateInvitationRequest(rr, req, "user-1", "team-1")
		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestHandleInvitationError_Arms(t *testing.T) {
	srv := invSendErrArmsServer()
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"personal workspace → 403", services.NewPersonalWorkspaceError("team-1"), http.StatusForbidden},
		{"duplicate members → 409", services.NewDuplicateMembersError([]string{"a@example.com"}), http.StatusConflict},
		{"no subscription → 403", services.NewNoActiveSubscriptionError("team-1"), http.StatusForbidden},
		{"seat limit → 403", services.NewSeatLimitExceededError("team-1", 3, 1, 4, 5), http.StatusForbidden},
		{"permission → 403", stderrors.New("permission denied for invite"), http.StatusForbidden},
		{"not found → 404", stderrors.New("team not found"), http.StatusNotFound},
		{"unknown → 500", stderrors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/t/invitations", nil)
			srv.handleInvitationError(rr, req, tt.err, "team-1")
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}
