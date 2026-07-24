package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// Regression tests for #463: the install callback must not bind an installation
// the caller cannot prove authority over, and must surface each denial reason
// distinctly rather than as a generic 500.

// runCallbackServiceErrorCase drives the handler with a valid state and code,
// stubs the service to return serviceErr, and asserts the mapped response.
func runCallbackServiceErrorCase(
	t *testing.T, serviceErr error, wantStatus int, wantCode string,
) {
	t.Helper()

	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	const installationID = int64(4242)
	state := srv.signGitHubState(githubTestTeamID, 0)

	container.gitHubAppService.On(
		"HandleInstallationCallback",
		mock.Anything,
		githubTestUserID,
		githubTestTeamID,
		installationID,
		githubTestInstallCode,
	).Return(false, serviceErr)

	req := makeCallbackRequest(map[string]interface{}{
		"installation_id": installationID,
		"state":           state,
		"code":            githubTestInstallCode,
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubCallback(rr, req)

	assert.Equal(t, wantStatus, rr.Code)

	var resp map[string]interface{}
	assert.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, wantCode, resp["code"])

	container.gitHubAppService.AssertExpectations(t)
}

// TestHandleGitHubCallback_ForeignInstallationDenied is the core regression:
// a valid state for the caller's own team plus an installation they cannot
// administer must be refused with 403, not stored.
func TestHandleGitHubCallback_ForeignInstallationDenied(t *testing.T) {
	runCallbackServiceErrorCase(t,
		services.ErrInstallationNotAuthorized, http.StatusForbidden, "installation_not_authorized")
}

// TestHandleGitHubCallback_MemberRoleDenied verifies connecting an integration
// is an owner/admin action: a plain member is refused.
func TestHandleGitHubCallback_MemberRoleDenied(t *testing.T) {
	// "forbidden" is a known error type, so writeErrorResponse emits its
	// canonical FORBIDDEN code rather than the raw string.
	runCallbackServiceErrorCase(t,
		services.ErrPermissionDenied, http.StatusForbidden, "FORBIDDEN")
}

// TestHandleGitHubCallback_UserAuthNotConfigured verifies the flow fails closed
// when the instance has no GitHub App OAuth credentials, rather than silently
// falling back to the app-JWT-only path this issue closed.
func TestHandleGitHubCallback_UserAuthNotConfigured(t *testing.T) {
	runCallbackServiceErrorCase(t,
		services.ErrGitHubUserAuthUnavailable, http.StatusServiceUnavailable, "github_user_auth_not_configured")
}

// TestHandleGitHubCallback_MissingCode verifies the callback rejects a request
// with no authorization code before reaching the service.
func TestHandleGitHubCallback_MissingCode(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	req := makeCallbackRequest(map[string]interface{}{
		"installation_id": 4242,
		"state":           srv.signGitHubState(githubTestTeamID, 0),
		// no code
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubCallback(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	container.gitHubAppService.AssertNotCalled(t, "HandleInstallationCallback")
}

// TestHandleGitHubCallback_StateInstallationMismatch verifies a state bound to
// one installation cannot be replayed against another.
func TestHandleGitHubCallback_StateInstallationMismatch(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	// State bound to installation 1111, submitted for 2222.
	req := makeCallbackRequest(map[string]interface{}{
		"installation_id": 2222,
		"state":           srv.signGitHubState(githubTestTeamID, 1111),
		"code":            githubTestInstallCode,
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubCallback(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	container.gitHubAppService.AssertNotCalled(t, "HandleInstallationCallback")
}

// TestHandleGitHubCallback_StateBoundToMatchingInstallation verifies the bound
// form is accepted when it does match, so the check is not a blanket refusal.
func TestHandleGitHubCallback_StateBoundToMatchingInstallation(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	const installationID = int64(1111)

	container.gitHubAppService.On(
		"HandleInstallationCallback",
		mock.Anything,
		githubTestUserID,
		githubTestTeamID,
		installationID,
		githubTestInstallCode,
	).Return(false, nil)

	req := makeCallbackRequest(map[string]interface{}{
		"installation_id": installationID,
		"state":           srv.signGitHubState(githubTestTeamID, installationID),
		"code":            githubTestInstallCode,
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubCallback(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	container.gitHubAppService.AssertExpectations(t)
}

// TestHandleGitHubDisconnect_MemberRoleDenied verifies disconnect is gated at
// the same level as connect.
func TestHandleGitHubDisconnect_MemberRoleDenied(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	container.gitHubAppService.On(
		"DisconnectInstallation", mock.Anything, githubTestUserID, githubTestTeamID,
	).Return(services.ErrPermissionDenied)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/"+githubTestTeamID+"/integrations/github/disconnect", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, githubTestUserID))
	req = addGitHubChiParams(req, map[string]string{"team_id": githubTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleGitHubDisconnect(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	container.gitHubAppService.AssertExpectations(t)
}

// TestHandleGitHubInstallURL_Authorization covers the gate added with #463:
// minting an install URL needs the same permission as completing the callback,
// so a member cannot install the App on a GitHub org and then be refused at the
// end — which would leave the org connected to nothing.
func TestHandleGitHubInstallURL_Authorization(t *testing.T) {
	tests := []struct {
		name       string
		authzErr   error
		wantStatus int
	}{
		{"owner or admin", nil, http.StatusOK},
		{"plain member", services.ErrPermissionDenied, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container, _ := newGitHubTestContainer(t)
			authzSvc := svcmocks.NewMockAuthorizationServiceInterface(t)
			authzSvc.On("Can", mock.Anything, githubTestUserID, githubTestTeamID, authz.TeamUpdate).
				Return(tt.authzErr)
			container.authzService = authzSvc

			srv := createGitHubTestServer(container)
			srv.config = &config.Config{GitHub: config.GitHubConfig{AppSlug: "vibexp"}}

			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/"+githubTestTeamID+"/integrations/github/install-url", nil)
			req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, githubTestUserID))
			req = addGitHubChiParams(req, map[string]string{"team_id": githubTestTeamID})
			rr := httptest.NewRecorder()

			srv.handleGitHubInstallURL(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)
			authzSvc.AssertExpectations(t)
		})
	}
}
