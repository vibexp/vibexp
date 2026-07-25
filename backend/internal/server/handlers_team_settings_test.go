package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

const (
	testTeamSettingsTeamID = "550e8400-e29b-41d4-a716-446655440000"
	testTeamSettingsUserID = "660e8400-e29b-41d4-a716-446655440001"
	teamSettingsPath       = "/api/v1/" + testTeamSettingsTeamID + "/settings/search"
)

// MockTeamSettingsContainer overrides the team search settings service on the
// base container.
type MockTeamSettingsContainer struct {
	BaseMockContainer
	teamSearchSettingsService services.TeamSearchSettingsServiceInterface
}

func (c *MockTeamSettingsContainer) TeamSearchSettingsService() services.TeamSearchSettingsServiceInterface {
	return c.teamSearchSettingsService
}

func createTestTeamSettingsServer(svc services.TeamSearchSettingsServiceInterface) *Server {
	r := chi.NewRouter()
	srv := &Server{
		container: &MockTeamSettingsContainer{teamSearchSettingsService: svc},
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
	r.Use(srv.requireCompleteSearchSettingsBody)
	teamsettingsgen.HandlerWithOptions(strict, teamsettingsgen.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: srv.teamSettingsBindErrorHandler,
	})
	return srv
}

func makeTeamSettingsRequest(method, path, body string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testTeamSettingsUserID))
}

func sampleInstanceView() *models.TeamSearchSettingsView {
	defaults := models.TeamSearchSettingsValues{
		RecencyRankingEnabled: true,
		RankWeightRelevance:   0.5,
		RankWeightCreated:     0.3,
		RankWeightUpdated:     0.2,
		RankHalfLifeDays:      90,
	}
	return &models.TeamSearchSettingsView{
		Source:           models.TeamSearchSettingsSourceInstance,
		Values:           defaults,
		InstanceDefaults: defaults,
		RankCandidateCap: 200,
	}
}

func sampleTeamView() *models.TeamSearchSettingsView {
	view := sampleInstanceView()
	view.Source = models.TeamSearchSettingsSourceTeam
	view.Values = models.TeamSearchSettingsValues{
		RecencyRankingEnabled: false,
		RankWeightRelevance:   0.9,
		RankWeightCreated:     0.05,
		RankWeightUpdated:     0.05,
		RankHalfLifeDays:      7,
	}
	return view
}

const validUpdateBody = `{"recency_ranking_enabled":false,"rank_weight_relevance":0.9,` +
	`"rank_weight_created":0.05,"rank_weight_updated":0.05,"rank_half_life_days":7}`

func TestGetTeamSearchSettings_NoOverrideReportsInstanceSource(t *testing.T) {
	svc := servicesmocks.NewMockTeamSearchSettingsServiceInterface(t)
	svc.EXPECT().Get(mock.Anything, testTeamSettingsTeamID).Return(sampleInstanceView(), nil)

	srv := createTestTeamSettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodGet, teamSettingsPath, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "instance", resp["source"])
	assert.EqualValues(t, 200, resp["rank_candidate_cap"])
	assert.NotNil(t, resp["instance_defaults"], "clients preview a reset from this without a 2nd call")
}

func TestUpdateTeamSearchSettings_StoresAndReportsTeamSource(t *testing.T) {
	svc := servicesmocks.NewMockTeamSearchSettingsServiceInterface(t)
	svc.EXPECT().Update(mock.Anything, testTeamSettingsUserID, testTeamSettingsTeamID,
		mock.MatchedBy(func(v models.TeamSearchSettingsValues) bool {
			return v.RankHalfLifeDays == 7 && v.RankWeightRelevance == 0.9
		})).Return(sampleTeamView(), nil)

	srv := createTestTeamSettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodPut, teamSettingsPath, validUpdateBody)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "team", resp["source"])
	assert.EqualValues(t, 7, resp["rank_half_life_days"])
}

// rank_candidate_cap is instance-owned and absent from the request schema, so a
// body carrying it is rejected rather than silently dropped — a caller must not
// be able to believe they raised an instance-wide limit.
func TestUpdateTeamSearchSettings_RejectsCandidateCapInBody(t *testing.T) {
	svc := servicesmocks.NewMockTeamSearchSettingsServiceInterface(t)
	// No Update expectation: the request must never reach the service.

	body := `{"recency_ranking_enabled":false,"rank_weight_relevance":0.9,` +
		`"rank_weight_created":0.05,"rank_weight_updated":0.05,"rank_half_life_days":7,` +
		`"rank_candidate_cap":99999}`

	srv := createTestTeamSettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodPut, teamSettingsPath, body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "rank_candidate_cap")
}

// A mistyped field name means its real field is absent; the generated struct
// would leave that weight at 0 and validation would still pass, silently storing
// a profile the caller never asked for. The whole-row design forbids that.
func TestUpdateTeamSearchSettings_RejectsPartialBody(t *testing.T) {
	svc := servicesmocks.NewMockTeamSearchSettingsServiceInterface(t)

	// rank_weight_relevance misspelled: without the guard it would store 0.
	body := `{"recency_ranking_enabled":false,"rank_weight_relevence":0.9,` +
		`"rank_weight_created":0.05,"rank_weight_updated":0.05,"rank_half_life_days":7}`

	srv := createTestTeamSettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodPut, teamSettingsPath, body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "rank_weight_relevence", "the unknown key is named")
}

func TestUpdateTeamSearchSettings_RejectsMissingField(t *testing.T) {
	svc := servicesmocks.NewMockTeamSearchSettingsServiceInterface(t)

	body := `{"recency_ranking_enabled":false,"rank_weight_relevance":0.9,` +
		`"rank_weight_created":0.05,"rank_weight_updated":0.05}`

	srv := createTestTeamSettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodPut, teamSettingsPath, body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "rank_half_life_days")
}

func TestResetTeamSearchSettings_Returns204(t *testing.T) {
	svc := servicesmocks.NewMockTeamSearchSettingsServiceInterface(t)
	svc.EXPECT().Reset(mock.Anything, testTeamSettingsUserID, testTeamSettingsTeamID).Return(nil)

	srv := createTestTeamSettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodDelete, teamSettingsPath, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())
}

// permissionDeniedCases pins that BOTH writes surface an authorization failure
// as 403 rather than a generic 500 — the caller must be able to tell a role
// problem from an outage.
func TestTeamSearchSettings_WritesReturn403WithoutPermission(t *testing.T) {
	denied := fmt.Errorf("%w: role %q may not perform this", services.ErrPermissionDenied, "member")

	tests := []struct {
		name   string
		method string
		body   string
		setup  func(*servicesmocks.MockTeamSearchSettingsServiceInterface)
	}{
		{
			name: "update", method: http.MethodPut, body: validUpdateBody,
			setup: func(m *servicesmocks.MockTeamSearchSettingsServiceInterface) {
				m.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, denied)
			},
		},
		{
			name: "reset", method: http.MethodDelete, body: "",
			setup: func(m *servicesmocks.MockTeamSearchSettingsServiceInterface) {
				m.EXPECT().Reset(mock.Anything, mock.Anything, mock.Anything).Return(denied)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := servicesmocks.NewMockTeamSearchSettingsServiceInterface(t)
			tt.setup(svc)

			srv := createTestTeamSettingsServer(svc)
			req := makeTeamSettingsRequest(tt.method, teamSettingsPath, tt.body)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusForbidden, w.Code)
		})
	}
}

func TestUpdateTeamSearchSettings_InvalidProfileReturns400(t *testing.T) {
	svc := servicesmocks.NewMockTeamSearchSettingsServiceInterface(t)
	svc.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("%w: rank_weight_* must not all be zero",
			services.ErrInvalidSearchSettings))

	srv := createTestTeamSettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodPut, teamSettingsPath,
		`{"recency_ranking_enabled":true,"rank_weight_relevance":0,"rank_weight_created":0,`+
			`"rank_weight_updated":0,"rank_half_life_days":90}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "must not all be zero",
		"the 400 must carry the validator's wording, not a generic message")
}

func TestTeamSearchSettings_InvalidTeamIDReturns400(t *testing.T) {
	svc := servicesmocks.NewMockTeamSearchSettingsServiceInterface(t)

	srv := createTestTeamSettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodGet, "/api/v1/not-a-uuid/settings/search", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "team_id must be a valid UUID")
}

func TestGetTeamSearchSettings_ServiceErrorReturns500(t *testing.T) {
	svc := servicesmocks.NewMockTeamSearchSettingsServiceInterface(t)
	svc.EXPECT().Get(mock.Anything, testTeamSettingsTeamID).
		Return(nil, fmt.Errorf("database unavailable"))

	srv := createTestTeamSettingsServer(svc)
	req := makeTeamSettingsRequest(http.MethodGet, teamSettingsPath, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "database unavailable",
		"internal error details must not leak to the client")
}
