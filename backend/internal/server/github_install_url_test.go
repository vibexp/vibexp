package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// The install URL is built from the requesting team's OWN App (#482): with
// per-team Apps an instance-wide slug would send every team to an App they do
// not own, and a team with no App must be told so rather than handed a link to
// a GitHub App that does not exist.

// newGitHubInstallURLRequest builds an authenticated GET for the install-URL
// endpoint. The path matches the spec template so responses can be validated
// against it.
func newGitHubInstallURLRequest(teamID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/"+teamID+"/integrations/github/install-url", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, githubTestUserID))
	return addGitHubChiParams(req, map[string]string{"team_id": teamID})
}

// allowGitHubInstallURL wires an authz service that permits the install-URL
// call, so a test can focus on what the handler does after the gate.
func allowGitHubInstallURL(t *testing.T, container *GitHubTestContainer, teamID string) {
	t.Helper()
	authzSvc := svcmocks.NewMockAuthorizationServiceInterface(t)
	authzSvc.On("Can", mock.Anything, githubTestUserID, teamID, authz.TeamUpdate).Return(nil)
	container.authzService = authzSvc
}

// TestHandleGitHubInstallURL_UsesTeamsOwnApp is the core of #482: the URL
// carries the team's own slug, and the state it embeds is bound to that team's
// App config id.
func TestHandleGitHubInstallURL_UsesTeamsOwnApp(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	allowGitHubInstallURL(t, container, githubTestTeamID)
	expectGitHubAppConfig(container)

	srv := createGitHubTestServer(container)
	req := newGitHubInstallURLRequest(githubTestTeamID)
	rr := httptest.NewRecorder()

	srv.handleGitHubInstallURL(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	installURL := decodeInstallURL(t, rr)
	assert.Contains(t, installURL, "https://github.com/apps/"+githubTestAppSlug+"/installations/new?state=")

	state := installURLState(t, installURL)
	verified, ok := srv.verifyGitHubState(state)
	require.True(t, ok, "the minted state must verify")
	assert.Equal(t, githubTestTeamID, verified.teamID)
	assert.Equal(t, githubTestAppConfigID, verified.appConfigID)
	assert.Equal(t, int64(0), verified.installationID,
		"the installation does not exist yet, so the state is minted unbound")
}

// TestHandleGitHubInstallURL_TwoTeamsTwoURLs proves the URL actually varies by
// team rather than merely being read from a per-team source once.
func TestHandleGitHubInstallURL_TwoTeamsTwoURLs(t *testing.T) {
	const otherTeamID = "550e8400-e29b-41d4-a716-446655440002"
	const otherAppSlug = "globex-vibexp"
	const otherAppConfigID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"

	firstContainer, _ := newGitHubTestContainer(t)
	allowGitHubInstallURL(t, firstContainer, githubTestTeamID)
	expectGitHubAppConfig(firstContainer)
	firstURL := runInstallURL(t, firstContainer, githubTestTeamID)

	secondContainer, _ := newGitHubTestContainer(t)
	allowGitHubInstallURL(t, secondContainer, otherTeamID)
	secondContainer.gitHubAppConfigService.On("GetAppConfig", mock.Anything, otherTeamID).
		Return(&models.GitHubAppConfigResponse{
			GitHubAppConfig: models.GitHubAppConfig{
				ID:      otherAppConfigID,
				TeamID:  otherTeamID,
				AppSlug: otherAppSlug,
			},
		}, nil)
	secondURL := runInstallURL(t, secondContainer, otherTeamID)

	assert.Contains(t, firstURL, "/apps/"+githubTestAppSlug+"/")
	assert.Contains(t, secondURL, "/apps/"+otherAppSlug+"/")
	assert.NotEqual(t, firstURL, secondURL, "two teams must not share one install URL")
}

// TestHandleGitHubInstallURL_NoAppConfigured verifies a team with no App gets
// 409 GITHUB_APP_NOT_CONFIGURED — not a 500, and not a link to an App that does
// not exist.
func TestHandleGitHubInstallURL_NoAppConfigured(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	allowGitHubInstallURL(t, container, githubTestTeamID)
	expectNoGitHubAppConfig(container)

	srv := createGitHubTestServer(container)
	req := newGitHubInstallURLRequest(githubTestTeamID)
	rr := httptest.NewRecorder()

	srv.handleGitHubInstallURL(rr, req)

	assert.Equal(t, http.StatusConflict, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "GITHUB_APP_NOT_CONFIGURED", resp["code"],
		"the canonical code #479 already emits for this condition")
}

// TestHandleGitHubInstallURL_LookupFailure verifies an unexpected repository
// failure stays a 500 rather than being flattened into the 409 that means
// "register your App".
func TestHandleGitHubInstallURL_LookupFailure(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	allowGitHubInstallURL(t, container, githubTestTeamID)
	container.gitHubAppConfigService.On("GetAppConfig", mock.Anything, githubTestTeamID).
		Return(nil, assert.AnError)

	srv := createGitHubTestServer(container)
	req := newGitHubInstallURLRequest(githubTestTeamID)
	rr := httptest.NewRecorder()

	srv.handleGitHubInstallURL(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestHandleGitHubCallback_AppConfigReplaced is the replay guard: a state minted
// against one App config must not complete an install after the team replaces
// its App.
func TestHandleGitHubCallback_AppConfigReplaced(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	// The state was minted while the team had a different App registered.
	const retiredAppConfigID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	state := srv.signGitHubState(githubTestTeamID, retiredAppConfigID, 0)
	expectGitHubAppConfig(container)

	req := makeCallbackRequest(map[string]interface{}{
		"installation_id": 4242,
		"state":           state,
		"code":            githubTestInstallCode,
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubCallback(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	container.gitHubAppService.AssertNotCalled(t, "HandleInstallationCallback")
}

// TestHandleGitHubCallback_NoAppConfigured verifies a callback for a team with
// no App is a 409 rather than a confusing state-mismatch 400.
func TestHandleGitHubCallback_NoAppConfigured(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	state := srv.signGitHubState(githubTestTeamID, githubTestAppConfigID, 0)
	expectNoGitHubAppConfig(container)

	req := makeCallbackRequest(map[string]interface{}{
		"installation_id": 4242,
		"state":           state,
		"code":            githubTestInstallCode,
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubCallback(rr, req)

	assert.Equal(t, http.StatusConflict, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "GITHUB_APP_NOT_CONFIGURED", resp["code"],
		"the canonical code #479 already emits for this condition")
	container.gitHubAppService.AssertNotCalled(t, "HandleInstallationCallback")
}

// TestHandleGitHubCallback_CrossTeamStateRejected pins the pre-existing
// guarantee that survives the layout change: a state minted for one team cannot
// be redeemed on another team's callback.
func TestHandleGitHubCallback_CrossTeamStateRejected(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	// Minted for a different team, submitted on githubTestTeamID's callback.
	state := srv.signGitHubState("550e8400-e29b-41d4-a716-446655440002", githubTestAppConfigID, 0)

	req := makeCallbackRequest(map[string]interface{}{
		"installation_id": 4242,
		"state":           state,
		"code":            githubTestInstallCode,
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubCallback(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	container.gitHubAppService.AssertNotCalled(t, "HandleInstallationCallback")
}

// runInstallURL drives the handler once and returns the minted URL.
func runInstallURL(t *testing.T, container *GitHubTestContainer, teamID string) string {
	t.Helper()

	srv := createGitHubTestServer(container)
	req := newGitHubInstallURLRequest(teamID)
	rr := httptest.NewRecorder()

	srv.handleGitHubInstallURL(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	return decodeInstallURL(t, rr)
}

func decodeInstallURL(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()

	var resp map[string]string
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	require.NotEmpty(t, resp["install_url"])
	return resp["install_url"]
}

// installURLState pulls the state parameter back out of a minted install URL.
func installURLState(t *testing.T, installURL string) string {
	t.Helper()

	parsed, err := url.Parse(installURL)
	require.NoError(t, err)
	state := parsed.Query().Get("state")
	require.NotEmpty(t, state, "install URL must carry a state parameter")
	return state
}
