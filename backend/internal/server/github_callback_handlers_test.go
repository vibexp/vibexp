package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// =============================================================================
// Tests for handleGitHubCallback
// =============================================================================

// makeCallbackRequest builds an authenticated POST request for the callback endpoint.
func makeCallbackRequest(body interface{}) *http.Request {
	var reqBody []byte
	if body != nil {
		var err error
		reqBody, err = json.Marshal(body)
		if err != nil {
			panic(err)
		}
	}
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/"+githubTestTeamID+"/integrations/github/callback",
		bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), contextKeyUserID, githubTestUserID)
	req = req.WithContext(ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("team_id", githubTestTeamID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// TestHandleGitHubCallback_NewInstallation verifies that a successful new installation
// responds with 201 and {"reconnected": false}.
func TestHandleGitHubCallback_NewInstallation(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	state := srv.signGitHubState(githubTestTeamID)

	container.gitHubAppService.On(
		"HandleInstallationCallback",
		mock.Anything,
		githubTestUserID,
		githubTestTeamID,
		int64(999),
	).Return(false, nil)

	reqBody := map[string]interface{}{
		"installation_id": 999,
		"state":           state,
	}
	req := makeCallbackRequest(reqBody)
	rr := httptest.NewRecorder()

	srv.handleGitHubCallback(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	var resp map[string]interface{}
	err := json.NewDecoder(rr.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.Equal(t, false, resp["reconnected"], "new installation should return reconnected=false")

	container.gitHubAppService.AssertExpectations(t)
}

// TestHandleGitHubCallback_Reconnect verifies that same-team reconnection
// responds with 201 and {"reconnected": true}.
func TestHandleGitHubCallback_Reconnect(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	state := srv.signGitHubState(githubTestTeamID)

	container.gitHubAppService.On(
		"HandleInstallationCallback",
		mock.Anything,
		githubTestUserID,
		githubTestTeamID,
		int64(777),
	).Return(true, nil) // reconnected=true

	reqBody := map[string]interface{}{
		"installation_id": 777,
		"state":           state,
	}
	req := makeCallbackRequest(reqBody)
	rr := httptest.NewRecorder()

	srv.handleGitHubCallback(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(rr.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.Equal(t, true, resp["reconnected"], "reconnection should return reconnected=true")

	container.gitHubAppService.AssertExpectations(t)
}

// TestHandleGitHubCallback_CrossTeamConflict verifies that when the service returns
// ErrInstallationAlreadyConnected, the handler responds with 409 and the correct error code.
func TestHandleGitHubCallback_CrossTeamConflict(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	state := srv.signGitHubState(githubTestTeamID)

	container.gitHubAppService.On(
		"HandleInstallationCallback",
		mock.Anything,
		githubTestUserID,
		githubTestTeamID,
		int64(555),
	).Return(false, services.ErrInstallationAlreadyConnected)

	reqBody := map[string]interface{}{
		"installation_id": 555,
		"state":           state,
	}
	req := makeCallbackRequest(reqBody)
	rr := httptest.NewRecorder()

	srv.handleGitHubCallback(rr, req)

	assert.Equal(t, http.StatusConflict, rr.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(rr.Body).Decode(&resp)
	assert.NoError(t, err)
	// The writeErrorResponse with unknown type creates an APIError with the type as code
	assert.Equal(t, "installation_already_connected", resp["code"])

	container.gitHubAppService.AssertExpectations(t)
}

// TestHandleGitHubCallback_InternalError verifies that generic service errors
// return 500.
func TestHandleGitHubCallback_InternalError(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	state := srv.signGitHubState(githubTestTeamID)

	container.gitHubAppService.On(
		"HandleInstallationCallback",
		mock.Anything,
		githubTestUserID,
		githubTestTeamID,
		int64(111),
	).Return(false, assert.AnError)

	reqBody := map[string]interface{}{
		"installation_id": 111,
		"state":           state,
	}
	req := makeCallbackRequest(reqBody)
	rr := httptest.NewRecorder()

	srv.handleGitHubCallback(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	container.gitHubAppService.AssertExpectations(t)
}

// TestHandleGitHubCallback_MissingInstallationID verifies that a request without
// installation_id returns 400.
func TestHandleGitHubCallback_MissingInstallationID(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	state := srv.signGitHubState(githubTestTeamID)

	reqBody := map[string]interface{}{
		"installation_id": 0, // zero = missing
		"state":           state,
	}
	req := makeCallbackRequest(reqBody)
	rr := httptest.NewRecorder()

	srv.handleGitHubCallback(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	container.gitHubAppService.AssertNotCalled(t, "HandleInstallationCallback")
}

// TestHandleGitHubCallback_MissingState verifies that a request without state returns 400.
func TestHandleGitHubCallback_MissingState(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	reqBody := map[string]interface{}{
		"installation_id": 123,
		// no state
	}
	req := makeCallbackRequest(reqBody)
	rr := httptest.NewRecorder()

	srv.handleGitHubCallback(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	container.gitHubAppService.AssertNotCalled(t, "HandleInstallationCallback")
}

// TestHandleGitHubCallback_InvalidState verifies that a tampered state returns 400.
func TestHandleGitHubCallback_InvalidState(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	reqBody := map[string]interface{}{
		"installation_id": 123,
		"state":           "invalid:1234:badsig",
	}
	req := makeCallbackRequest(reqBody)
	rr := httptest.NewRecorder()

	srv.handleGitHubCallback(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	container.gitHubAppService.AssertNotCalled(t, "HandleInstallationCallback")
}
