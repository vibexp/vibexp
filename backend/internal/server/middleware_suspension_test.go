package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// #454 requires ONE test per authentication entry point proving a suspended
// account is rejected. The other legs live where their transport does:
//   - MCP bearer      → TestMCPTokenContextBridge_RejectsSuspendedUser (mcp_oauth_test.go)
//   - OIDC sign-in    → TestHandleCallbackSuccess_RejectsSuspendedUser (auth_handlers_*_test.go)
//
// This file covers the shared chokepoint every other path runs through, which is
// what actually makes the guarantee hold.

const suspensionTestUserID = "user-under-test"

// suspensionServer builds a Server whose user repository reports the test user
// with the given status.
func suspensionServer(t *testing.T, status string) *Server {
	t.Helper()
	users := repomocks.NewMockUserRepository(t)
	users.On("GetByID", mock.Anything, suspensionTestUserID).
		Return(&models.User{ID: suspensionTestUserID, Status: status}, nil).Maybe()
	return &Server{container: containerWithUsers{BaseMockContainer: &BaseMockContainer{}, users: users}}
}

// suspensionServerErroring builds a Server whose user lookup fails.
func suspensionServerErroring(t *testing.T, lookupErr error) *Server {
	t.Helper()
	users := repomocks.NewMockUserRepository(t)
	users.On("GetByID", mock.Anything, suspensionTestUserID).Return(nil, lookupErr).Maybe()
	return &Server{container: containerWithUsers{BaseMockContainer: &BaseMockContainer{}, users: users}}
}

// TestAuthenticateUser_StatusGate is the core contract: an active account gets a
// populated context, and everything else gets an error and NO context.
func TestAuthenticateUser_StatusGate(t *testing.T) {
	tests := []struct {
		name       string
		server     func(*testing.T) *Server
		wantErr    error
		wantErrMsg string
	}{
		{
			name:   "active account authenticates",
			server: func(t *testing.T) *Server { return suspensionServer(t, models.UserStatusActive) },
		},
		{
			name:    "suspended account is rejected",
			server:  func(t *testing.T) *Server { return suspensionServer(t, models.UserStatusSuspended) },
			wantErr: errUserSuspended,
		},
		{
			name: "unknown status is treated as active (fails OPEN, so a future " +
				"non-blocking state cannot lock everyone out)",
			server: func(t *testing.T) *Server { return suspensionServer(t, "some_future_state") },
		},
		{
			name:   "empty status is treated as active (row predates the column)",
			server: func(t *testing.T) *Server { return suspensionServer(t, "") },
		},
		{
			name: "deleted account is rejected, not 500",
			server: func(t *testing.T) *Server {
				return suspensionServerErroring(t, repositories.ErrUserNotFound)
			},
			wantErr: errUserSuspended,
		},
		{
			name: "lookup failure fails CLOSED (must not authenticate)",
			server: func(t *testing.T) *Server {
				return suspensionServerErroring(t, errors.New("db down"))
			},
			wantErrMsg: "suspension check",
		},
		{
			name:       "nil repository fails CLOSED",
			server:     func(_ *testing.T) *Server { return &Server{container: &BaseMockContainer{}} },
			wantErrMsg: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := tc.server(t)
			ctx, err := srv.authenticateUser(context.Background(), suspensionTestUserID, "cookie", nil)

			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
				assert.Nil(t, ctx, "no authenticated context may be built on rejection")
			case tc.wantErrMsg != "":
				require.Error(t, err)
				assert.Nil(t, ctx)
			default:
				require.NoError(t, err)
				require.NotNil(t, ctx)
				assert.Equal(t, suspensionTestUserID, ctx.Value(contextkeys.UserID))
				assert.Equal(t, "cookie", ctx.Value(contextkeys.AuthType))
			}
		})
	}
}

// TestAuthenticateUser_NilRepositoryFailsClosed is called out separately because
// "the container is not fully wired" must never mean "everyone is authenticated".
func TestAuthenticateUser_NilRepositoryFailsClosed(t *testing.T) {
	srv := &Server{container: &BaseMockContainer{}}
	// BaseMockContainer now returns an always-active stub, so shadow it with a
	// container that genuinely has none.
	srv.container = containerWithUsers{BaseMockContainer: &BaseMockContainer{}, users: nil}

	ctx, err := srv.authenticateUser(context.Background(), suspensionTestUserID, "cookie", nil)
	require.Error(t, err)
	assert.NotErrorIs(t, err, errUserSuspended, "an infrastructure failure is not a suspension")
	assert.Nil(t, ctx)
}

// TestWriteSuspensionAuthError_StatusCodes pins the two failure shapes: a
// suspended account is a 401 (credential rejected), an infrastructure failure is
// a 500 — never the other way round, or a DB outage would look like a bad token.
func TestWriteSuspensionAuthError_StatusCodes(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantBody string
	}{
		{"suspended", errUserSuspended, http.StatusUnauthorized, suspendedAuthDetail},
		{"infrastructure", errors.New("db down"), http.StatusInternalServerError, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := &Server{}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
			rr := httptest.NewRecorder()

			srv.writeSuspensionAuthError(rr, req, tc.err)

			assert.Equal(t, tc.wantCode, rr.Code)
			if tc.wantBody != "" {
				assert.Contains(t, rr.Body.String(), tc.wantBody)
			}
		})
	}
}

// TestSuspension_RoundTrip is the reactivation acceptance criterion at the
// enforcement layer: the SAME credential that is refused while suspended works
// again once the account is active, with nothing else changed.
func TestSuspension_RoundTrip(t *testing.T) {
	users := repomocks.NewMockUserRepository(t)
	status := models.UserStatusActive
	users.On("GetByID", mock.Anything, suspensionTestUserID).
		Return(func(_ context.Context, id string) (*models.User, error) {
			return &models.User{ID: id, Status: status}, nil
		}).Maybe()
	srv := &Server{container: containerWithUsers{BaseMockContainer: &BaseMockContainer{}, users: users}}

	// Active → authenticates.
	ctx, err := srv.authenticateUser(context.Background(), suspensionTestUserID, "api_key", nil)
	require.NoError(t, err)
	require.NotNil(t, ctx)

	// Suspended → the very same credential is refused, immediately.
	status = models.UserStatusSuspended
	ctx, err = srv.authenticateUser(context.Background(), suspensionTestUserID, "api_key", nil)
	require.ErrorIs(t, err, errUserSuspended)
	require.Nil(t, ctx)

	// Reactivated → access restored, no other state involved.
	status = models.UserStatusActive
	ctx, err = srv.authenticateUser(context.Background(), suspensionTestUserID, "api_key", nil)
	require.NoError(t, err)
	require.NotNil(t, ctx)
	assert.Equal(t, suspensionTestUserID, ctx.Value(contextkeys.UserID))
}
