package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	teamsettingsgen "github.com/vibexp/vibexp/internal/server/gen/teamsettings"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const teamAISummarySettingsPath = "/api/v1/" + testTeamSettingsTeamID + "/settings/ai-summary"

// MockTeamAISummarySettingsContainer overrides the team AI summary settings
// service on the base container, mirroring MockTeamSettingsContainer.
type MockTeamAISummarySettingsContainer struct {
	BaseMockContainer
	teamAISummarySettingsService services.TeamAISummarySettingsServiceInterface
}

func (c *MockTeamAISummarySettingsContainer) TeamAISummarySettingsService() services.TeamAISummarySettingsServiceInterface {
	return c.teamAISummarySettingsService
}

func createTestTeamAISummarySettingsServer(svc services.TeamAISummarySettingsServiceInterface) *Server {
	r := chi.NewRouter()
	srv := &Server{
		container: &MockTeamAISummarySettingsContainer{teamAISummarySettingsService: svc},
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
		router:    r,
	}
	strict := teamsettingsgen.NewStrictHandlerWithOptions(
		&teamSettingsStrictServer{s: srv},
		nil,
		teamsettingsgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  srv.teamSettingsBindErrorHandler,
			ResponseErrorHandlerFunc: srv.teamSettingsResponseErrorHandler,
		},
	)
	// Both settings-body middlewares, exactly as setupTeamSettingsRoutes chains
	// them in production — this is what proves each is scoped to its own path
	// and doesn't reject the other's request body.
	r.Use(srv.requireCompleteSearchSettingsBody)
	r.Use(srv.requireCompleteAISummarySettingsBody)
	teamsettingsgen.HandlerWithOptions(strict, teamsettingsgen.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: srv.teamSettingsBindErrorHandler,
	})
	return srv
}

func sampleAISummaryInstanceView() *models.TeamAISummarySettingsView {
	defaults := models.TeamAISummarySettingsValues{
		Enabled:         true,
		ModelProviderID: nil,
		TopN:            5,
		Style:           models.AISummaryStyleBalanced,
		MaxOutputTokens: 800,
	}
	return &models.TeamAISummarySettingsView{
		Source:           models.TeamAISummarySettingsSourceInstance,
		Values:           defaults,
		InstanceDefaults: defaults,
		MaxTopN:          10,
		// Deliberately not the 4096 default, so the test proves it is echoed.
		MaxOutputTokensCeiling: 3000,
		Available:              true,
	}
}

func sampleAISummaryTeamView() *models.TeamAISummarySettingsView {
	view := sampleAISummaryInstanceView()
	view.Source = models.TeamAISummarySettingsSourceTeam
	providerID := "770e8400-e29b-41d4-a716-446655440002"
	view.Values = models.TeamAISummarySettingsValues{
		Enabled:         false,
		ModelProviderID: &providerID,
		TopN:            3,
		Style:           models.AISummaryStyleDetailed,
		MaxOutputTokens: 400,
	}
	return view
}

const validAISummaryUpdateBody = `{"enabled":false,"model_provider_id":"770e8400-e29b-41d4-a716-446655440002",` +
	`"top_n":3,"style":"detailed","max_output_tokens":400}`

const validAISummaryUpdateBodyNullProvider = `{"enabled":true,"model_provider_id":null,` +
	`"top_n":5,"style":"balanced","max_output_tokens":800}`

func TestGetTeamAISummarySettings_NoOverrideReportsInstanceSource(t *testing.T) {
	svc := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)
	svc.EXPECT().Get(mock.Anything, testTeamSettingsTeamID).Return(sampleAISummaryInstanceView(), nil)

	srv := createTestTeamAISummarySettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodGet, teamAISummarySettingsPath, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "instance", resp["source"])
	assert.EqualValues(t, 10, resp["max_top_n"])
	assert.EqualValues(t, 3000, resp["max_output_tokens_ceiling"])
	assert.Equal(t, true, resp["available"])
	assert.NotNil(t, resp["instance_defaults"], "clients preview a reset from this without a 2nd call")
	values, ok := resp["values"].(map[string]any)
	require.True(t, ok, "values must be a nested object")
	assert.Nil(t, values["model_provider_id"])
}

func TestUpdateTeamAISummarySettings_StoresAndReportsTeamSource(t *testing.T) {
	svc := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)
	svc.EXPECT().Update(mock.Anything, testTeamSettingsUserID, testTeamSettingsTeamID,
		mock.MatchedBy(func(v models.TeamAISummarySettingsValues) bool {
			return v.TopN == 3 && v.Style == "detailed" && !v.Enabled &&
				v.ModelProviderID != nil && *v.ModelProviderID == "770e8400-e29b-41d4-a716-446655440002"
		})).Return(sampleAISummaryTeamView(), nil)

	srv := createTestTeamAISummarySettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodPut, teamAISummarySettingsPath, validAISummaryUpdateBody)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "team", resp["source"])
}

// A null model_provider_id clears the override; the key must still be
// PRESENT (nullable, not optional) for the whole-row-replace contract.
func TestUpdateTeamAISummarySettings_AcceptsNullModelProviderID(t *testing.T) {
	svc := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)
	svc.EXPECT().Update(mock.Anything, testTeamSettingsUserID, testTeamSettingsTeamID,
		mock.MatchedBy(func(v models.TeamAISummarySettingsValues) bool {
			return v.ModelProviderID == nil
		})).Return(sampleAISummaryInstanceView(), nil)

	srv := createTestTeamAISummarySettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodPut, teamAISummarySettingsPath, validAISummaryUpdateBodyNullProvider)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

// max_top_n and available are computed, not settable; a body carrying either
// is rejected rather than silently dropped.
func TestUpdateTeamAISummarySettings_RejectsComputedFieldsInBody(t *testing.T) {
	svc := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)
	// No Update expectation: the request must never reach the service.

	body := `{"enabled":false,"model_provider_id":null,"top_n":3,"style":"detailed",` +
		`"max_output_tokens":400,"max_top_n":999}`

	srv := createTestTeamAISummarySettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodPut, teamAISummarySettingsPath, body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "max_top_n")
}

// A mistyped field name means its real field is absent; without the guard the
// generated struct would leave that field zero-valued and store a profile the
// caller never asked for.
func TestUpdateTeamAISummarySettings_RejectsPartialBody(t *testing.T) {
	svc := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)

	body := `{"enabled":false,"model_provider_id":null,"top_n":3,"stlye":"detailed",` +
		`"max_output_tokens":400}`

	srv := createTestTeamAISummarySettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodPut, teamAISummarySettingsPath, body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "stlye", "the unknown key is named")
}

// model_provider_id is nullable but still REQUIRED to be present; omitting
// the key entirely (rather than sending null) must 400.
func TestUpdateTeamAISummarySettings_RejectsMissingModelProviderIDKey(t *testing.T) {
	svc := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)

	body := `{"enabled":false,"top_n":3,"style":"detailed","max_output_tokens":400}`

	srv := createTestTeamAISummarySettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodPut, teamAISummarySettingsPath, body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "model_provider_id")
}

func TestResetTeamAISummarySettings_Returns204(t *testing.T) {
	svc := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)
	svc.EXPECT().Reset(mock.Anything, testTeamSettingsUserID, testTeamSettingsTeamID).Return(nil)

	srv := createTestTeamAISummarySettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodDelete, teamAISummarySettingsPath, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())
}

// Both writes must surface an authorization failure as 403 rather than a
// generic 500 — the caller must be able to tell a role problem from an outage.
func TestTeamAISummarySettings_WritesReturn403WithoutPermission(t *testing.T) {
	denied := fmt.Errorf("%w: role %q may not perform this", services.ErrPermissionDenied, "member")

	tests := []struct {
		name   string
		method string
		body   string
		setup  func(*servicesmocks.MockTeamAISummarySettingsServiceInterface)
	}{
		{
			name: "update", method: http.MethodPut, body: validAISummaryUpdateBody,
			setup: func(m *servicesmocks.MockTeamAISummarySettingsServiceInterface) {
				m.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, denied)
			},
		},
		{
			name: "reset", method: http.MethodDelete, body: "",
			setup: func(m *servicesmocks.MockTeamAISummarySettingsServiceInterface) {
				m.EXPECT().Reset(mock.Anything, mock.Anything, mock.Anything).Return(denied)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)
			tt.setup(svc)

			srv := createTestTeamAISummarySettingsServer(svc)
			req := makeTeamSettingsRequest(tt.method, teamAISummarySettingsPath, tt.body)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusForbidden, w.Code)
		})
	}
}

func TestUpdateTeamAISummarySettings_InvalidProfileReturns400(t *testing.T) {
	svc := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)
	svc.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("%w: top_n must be between 1 and 10, got 999",
			services.ErrInvalidAISummarySettings))

	srv := createTestTeamAISummarySettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodPut, teamAISummarySettingsPath,
		`{"enabled":true,"model_provider_id":null,"top_n":999,"style":"balanced","max_output_tokens":800}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "top_n must be between 1 and 10",
		"the 400 must carry the validator's wording, not a generic message")
}

// max_output_tokens above the instance ceiling (#1085) is the service's 400,
// carried through with the ceiling in the message.
func TestUpdateTeamAISummarySettings_MaxOutputTokensAboveCeilingReturns400(t *testing.T) {
	svc := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)
	svc.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything,
		mock.MatchedBy(func(v models.TeamAISummarySettingsValues) bool { return v.MaxOutputTokens == 3001 })).
		Return(nil, fmt.Errorf("%w: max_output_tokens must be between 1 and 3000, got 3001",
			services.ErrInvalidAISummarySettings))

	srv := createTestTeamAISummarySettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodPut, teamAISummarySettingsPath,
		`{"enabled":true,"model_provider_id":null,"top_n":5,"style":"balanced","max_output_tokens":3001}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "max_output_tokens must be between 1 and 3000")
}

// The ceiling is computed, not settable: a body carrying it is rejected like
// max_top_n.
func TestUpdateTeamAISummarySettings_RejectsMaxOutputTokensCeilingInBody(t *testing.T) {
	svc := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)

	srv := createTestTeamAISummarySettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodPut, teamAISummarySettingsPath,
		`{"enabled":true,"model_provider_id":null,"top_n":5,"style":"balanced",`+
			`"max_output_tokens":400,"max_output_tokens_ceiling":99999}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "max_output_tokens_ceiling")
}

// A model_provider_id from another team surfaces as the same
// ErrInvalidAISummarySettings-wrapped 400 as any other invalid profile — the
// tenancy check lives in the service (requireOwnedProvider), not the handler.
func TestUpdateTeamAISummarySettings_CrossTeamProviderReturns400(t *testing.T) {
	svc := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)
	svc.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("%w: model_provider_id %q is not a model provider of this team",
			services.ErrInvalidAISummarySettings, "770e8400-e29b-41d4-a716-446655440002"))

	srv := createTestTeamAISummarySettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodPut, teamAISummarySettingsPath, validAISummaryUpdateBody)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "is not a model provider of this team")
}

func TestTeamAISummarySettings_InvalidTeamIDReturns400(t *testing.T) {
	svc := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)

	srv := createTestTeamAISummarySettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodGet, "/api/v1/not-a-uuid/settings/ai-summary", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "team_id must be a valid UUID")
}

func TestGetTeamAISummarySettings_ServiceErrorReturns500(t *testing.T) {
	svc := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)
	svc.EXPECT().Get(mock.Anything, testTeamSettingsTeamID).
		Return(nil, fmt.Errorf("database unavailable"))

	srv := createTestTeamAISummarySettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodGet, teamAISummarySettingsPath, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "database unavailable",
		"internal error details must not leak to the client")
}

// MockTeamSettingsAndAISummaryContainer carries BOTH settings services, so the
// cross-domain test below can exercise the two body-completeness middlewares
// chained exactly as setupTeamSettingsRoutes wires them in production.
type MockTeamSettingsAndAISummaryContainer struct {
	BaseMockContainer
	teamSearchSettingsService    services.TeamSearchSettingsServiceInterface
	teamAISummarySettingsService services.TeamAISummarySettingsServiceInterface
}

func (c *MockTeamSettingsAndAISummaryContainer) TeamSearchSettingsService() services.TeamSearchSettingsServiceInterface {
	return c.teamSearchSettingsService
}

func (c *MockTeamSettingsAndAISummaryContainer) TeamAISummarySettingsService() services.TeamAISummarySettingsServiceInterface {
	return c.teamAISummarySettingsService
}

// TestTeamSettingsBodyMiddlewares_AreDomainScoped proves each settings body
// middleware only acts on its own path: with BOTH middlewares chained (as in
// production), a valid search-settings PUT must not be rejected for missing
// ai-summary fields, and a valid ai-summary PUT must not be rejected for
// missing search fields.
func TestTeamSettingsBodyMiddlewares_AreDomainScoped(t *testing.T) {
	searchSvc := servicesmocks.NewMockTeamSearchSettingsServiceInterface(t)
	searchSvc.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(sampleTeamView(), nil)
	aiSvc := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)
	aiSvc.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(sampleAISummaryTeamView(), nil)

	r := chi.NewRouter()
	srv := &Server{
		container: &MockTeamSettingsAndAISummaryContainer{
			teamSearchSettingsService:    searchSvc,
			teamAISummarySettingsService: aiSvc,
		},
		logger: slog.New(slog.DiscardHandler),
		config: &config.Config{},
		router: r,
	}
	strict := teamsettingsgen.NewStrictHandlerWithOptions(
		&teamSettingsStrictServer{s: srv},
		nil,
		teamsettingsgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  srv.teamSettingsBindErrorHandler,
			ResponseErrorHandlerFunc: srv.teamSettingsResponseErrorHandler,
		},
	)
	r.Use(srv.requireCompleteSearchSettingsBody)
	r.Use(srv.requireCompleteAISummarySettingsBody)
	teamsettingsgen.HandlerWithOptions(strict, teamsettingsgen.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: srv.teamSettingsBindErrorHandler,
	})

	searchReq := makeTeamSettingsRequest(http.MethodPut, teamSettingsPath, validUpdateBody)
	searchW := httptest.NewRecorder()
	srv.ServeHTTP(searchW, searchReq)
	require.Equal(t, http.StatusOK, searchW.Code, "search PUT rejected: %s", searchW.Body.String())

	aiReq := makeTeamSettingsRequest(http.MethodPut, teamAISummarySettingsPath, validAISummaryUpdateBody)
	aiW := httptest.NewRecorder()
	srv.ServeHTTP(aiW, aiReq)
	require.Equal(t, http.StatusOK, aiW.Code, "ai-summary PUT rejected: %s", aiW.Body.String())
}
