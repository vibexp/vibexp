package server

import (
	"bytes"
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
	"github.com/vibexp/vibexp/internal/repositories"
	freshnessgen "github.com/vibexp/vibexp/internal/server/gen/freshness"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	testFreshnessTeamID    = "550e8400-e29b-41d4-a716-446655440000"
	testFreshnessUserID    = "660e8400-e29b-41d4-a716-446655440001"
	testFreshnessRuleID    = "770e8400-e29b-41d4-a716-446655440002"
	testFreshnessProjectID = "880e8400-e29b-41d4-a716-446655440003"

	freshnessRulesPath    = "/api/v1/" + testFreshnessTeamID + "/freshness/rules"
	freshnessRulePath     = freshnessRulesPath + "/" + testFreshnessRuleID
	freshnessSettingsPath = "/api/v1/" + testFreshnessTeamID + "/settings/freshness"
)

// MockFreshnessContainer installs only the accessor this domain needs; the
// embedded base returns nil for everything else.
type MockFreshnessContainer struct {
	BaseMockContainer
	freshnessService services.FreshnessServiceInterface
}

func (c *MockFreshnessContainer) FreshnessService() services.FreshnessServiceInterface {
	return c.freshnessService
}

func createTestFreshnessServer(svc services.FreshnessServiceInterface) *Server {
	r := chi.NewRouter()
	srv := &Server{
		container: &MockFreshnessContainer{freshnessService: svc},
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
		router:    r,
	}
	strict := freshnessgen.NewStrictHandlerWithOptions(
		&freshnessStrictServer{s: srv},
		nil,
		freshnessgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  srv.freshnessBindErrorHandler,
			ResponseErrorHandlerFunc: srv.freshnessResponseErrorHandler,
		},
	)
	r.Use(srv.requireCompleteFreshnessBody)
	freshnessgen.HandlerWithOptions(strict, freshnessgen.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: srv.freshnessBindErrorHandler,
	})
	return srv
}

func makeFreshnessRequest(method, path, body string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	}
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testFreshnessUserID))
}

func sampleFreshnessRule() *models.FreshnessRule {
	projectID := testFreshnessProjectID
	now := time.Now().UTC()
	return &models.FreshnessRule{
		ID:            testFreshnessRuleID,
		TeamID:        testFreshnessTeamID,
		ProjectID:     &projectID,
		ResourceTypes: []string{"artifact", "prompt"},
		Mediums:       []string{"web"},
		ThresholdDays: 90,
		Enabled:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func sampleFreshnessView(source string) *models.TeamFreshnessSettingsView {
	return &models.TeamFreshnessSettingsView{
		Source:   source,
		Values:   models.FreshnessSettingsValues{IntervalSeconds: 7200, ReversibilityEnabled: false},
		Defaults: models.DefaultFreshnessSettingsValues(),
	}
}

func TestListFreshnessRules_ReturnsRules(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	svc.EXPECT().ListRules(mock.Anything, testFreshnessTeamID).
		Return([]*models.FreshnessRule{sampleFreshnessRule()}, nil)

	srv := createTestFreshnessServer(svc)
	req := makeFreshnessRequest(http.MethodGet, freshnessRulesPath, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	rules, ok := resp["rules"].([]any)
	require.True(t, ok)
	require.Len(t, rules, 1)
	rule := rules[0].(map[string]any)
	assert.Equal(t, testFreshnessRuleID, rule["id"])
	assert.EqualValues(t, 90, rule["threshold_days"])
	assert.Equal(t, []any{"artifact", "prompt"}, rule["resource_types"])
}

// A team with no rules must send `[]`, never `null` — the distinction the
// required-array gate exists to protect.
func TestListFreshnessRules_EmptyIsArrayNotNull(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	svc.EXPECT().ListRules(mock.Anything, testFreshnessTeamID).Return(nil, nil)

	srv := createTestFreshnessServer(svc)
	req := makeFreshnessRequest(http.MethodGet, freshnessRulesPath, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"rules":[]`)
	assert.NotContains(t, w.Body.String(), `"rules":null`)
}

// A rule with no mediums and no project must still emit `mediums: []` and an
// explicit `project_id: null` — both are required fields.
func TestListFreshnessRules_EmptyMediumsAndNullProject(t *testing.T) {
	rule := sampleFreshnessRule()
	rule.ProjectID = nil
	rule.Mediums = nil

	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	svc.EXPECT().ListRules(mock.Anything, testFreshnessTeamID).
		Return([]*models.FreshnessRule{rule}, nil)

	srv := createTestFreshnessServer(svc)
	req := makeFreshnessRequest(http.MethodGet, freshnessRulesPath, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"mediums":[]`)
	assert.Contains(t, w.Body.String(), `"project_id":null`)
}

func TestCreateFreshnessRule_Created(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	svc.EXPECT().
		CreateRule(mock.Anything, testFreshnessUserID, testFreshnessTeamID,
			mock.MatchedBy(func(in services.FreshnessRuleInput) bool {
				return in.ThresholdDays == 90 && in.Enabled && len(in.ResourceTypes) == 2
			})).
		Return(sampleFreshnessRule(), nil)

	srv := createTestFreshnessServer(svc)
	body := `{"resource_types":["artifact","prompt"],"threshold_days":90}`
	req := makeFreshnessRequest(http.MethodPost, freshnessRulesPath, body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusCreated, w.Code)
}

// `enabled` defaults to true when omitted, per the schema.
func TestCreateFreshnessRule_EnabledDefaultsToTrue(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantEnabled bool
	}{
		{name: "omitted", body: `{"resource_types":["memory"],"threshold_days":30}`, wantEnabled: true},
		{name: "explicit false", body: `{"resource_types":["memory"],"threshold_days":30,"enabled":false}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := servicesmocks.NewMockFreshnessServiceInterface(t)
			svc.EXPECT().
				CreateRule(mock.Anything, testFreshnessUserID, testFreshnessTeamID,
					mock.MatchedBy(func(in services.FreshnessRuleInput) bool {
						return in.Enabled == tt.wantEnabled
					})).
				Return(sampleFreshnessRule(), nil)

			srv := createTestFreshnessServer(svc)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, makeFreshnessRequest(http.MethodPost, freshnessRulesPath, tt.body))

			require.Equal(t, http.StatusCreated, w.Code)
		})
	}
}

func TestUpdateFreshnessRule_OK(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	svc.EXPECT().
		UpdateRule(mock.Anything, testFreshnessUserID, testFreshnessTeamID, testFreshnessRuleID, mock.Anything).
		Return(sampleFreshnessRule(), nil)

	srv := createTestFreshnessServer(svc)
	body := `{"project_id":null,"resource_types":["artifact"],"mediums":[],` +
		`"threshold_days":45,"enabled":false}`
	req := makeFreshnessRequest(http.MethodPut, freshnessRulePath, body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestDeleteFreshnessRule_NoContent(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	svc.EXPECT().
		DeleteRule(mock.Anything, testFreshnessUserID, testFreshnessTeamID, testFreshnessRuleID).
		Return(nil)

	srv := createTestFreshnessServer(svc)
	req := makeFreshnessRequest(http.MethodDelete, freshnessRulePath, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())
}

func TestGetTeamFreshnessSettings_ReportsSourceAndDefaults(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	svc.EXPECT().GetSettings(mock.Anything, testFreshnessTeamID).
		Return(sampleFreshnessView(models.FreshnessSettingsSourceTeam), nil)

	srv := createTestFreshnessServer(svc)
	req := makeFreshnessRequest(http.MethodGet, freshnessSettingsPath, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "team", resp["source"])
	assert.EqualValues(t, 7200, resp["interval_seconds"])
	assert.Equal(t, false, resp["reversibility_enabled"])

	defaults, ok := resp["defaults"].(map[string]any)
	require.True(t, ok, "clients preview a reset from this without a 2nd call")
	assert.EqualValues(t, models.DefaultFreshnessIntervalSeconds, defaults["interval_seconds"])
	assert.Equal(t, true, defaults["reversibility_enabled"])
}

func TestUpdateTeamFreshnessSettings_OK(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	svc.EXPECT().
		UpdateSettings(mock.Anything, testFreshnessUserID, testFreshnessTeamID,
			models.FreshnessSettingsValues{IntervalSeconds: 7200, ReversibilityEnabled: false}).
		Return(sampleFreshnessView(models.FreshnessSettingsSourceTeam), nil)

	srv := createTestFreshnessServer(svc)
	body := `{"interval_seconds":7200,"reversibility_enabled":false}`
	req := makeFreshnessRequest(http.MethodPut, freshnessSettingsPath, body)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestResetTeamFreshnessSettings_NoContent(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	svc.EXPECT().ResetSettings(mock.Anything, testFreshnessUserID, testFreshnessTeamID).Return(nil)

	srv := createTestFreshnessServer(svc)
	req := makeFreshnessRequest(http.MethodDelete, freshnessSettingsPath, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusNoContent, w.Code)
}

// Service errors must land on the documented status codes, not a blanket 500.
func TestFreshnessHandlers_ErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "permission denied is 403", err: services.ErrPermissionDenied, wantStatus: http.StatusForbidden},
		{
			name:       "unknown rule is 404",
			err:        repositories.ErrFreshnessRuleNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "invalid rule is 400",
			err:        services.ErrInvalidFreshnessRule,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "anything else is an opaque 500",
			err:        errors.New("db exploded"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := servicesmocks.NewMockFreshnessServiceInterface(t)
			svc.EXPECT().
				DeleteRule(mock.Anything, testFreshnessUserID, testFreshnessTeamID, testFreshnessRuleID).
				Return(tt.err)

			srv := createTestFreshnessServer(svc)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, makeFreshnessRequest(http.MethodDelete, freshnessRulePath, ""))

			require.Equal(t, tt.wantStatus, w.Code)
			assert.NotContains(t, w.Body.String(), "db exploded", "internal detail must never reach the client")
		})
	}
}

// The spec marks both PUT bodies additionalProperties:false with every field
// required, and oapi-codegen enforces NEITHER. Without the middleware a typo
// would silently zero-value that dimension of a complete replacement.
func TestFreshnessHandlers_PutRejectsIncompleteBody(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		body   string
		wantIn string
	}{
		{
			name:   "rule missing fields",
			path:   freshnessRulePath,
			body:   `{"resource_types":["artifact"],"threshold_days":30}`,
			wantIn: "missing required field(s): project_id, mediums, enabled",
		},
		{
			name: "rule unknown field",
			path: freshnessRulePath,
			body: `{"project_id":null,"resource_types":["artifact"],"mediums":[],` +
				`"threshold_days":30,"enabled":true,"treshold_days":99}`,
			wantIn: "unknown field(s): treshold_days",
		},
		{
			name:   "settings missing field",
			path:   freshnessSettingsPath,
			body:   `{"interval_seconds":7200}`,
			wantIn: "missing required field(s): reversibility_enabled",
		},
		{
			name:   "settings unknown field",
			path:   freshnessSettingsPath,
			body:   `{"interval_seconds":7200,"reversibility_enabled":true,"extra":1}`,
			wantIn: "unknown field(s): extra",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No service expectations: the request must never reach the service.
			svc := servicesmocks.NewMockFreshnessServiceInterface(t)
			srv := createTestFreshnessServer(svc)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, makeFreshnessRequest(http.MethodPut, tt.path, tt.body))

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), tt.wantIn)
		})
	}
}

// POST is a create with documented optional fields, so the completeness guard
// must not apply to it.
func TestFreshnessHandlers_PostIsNotSubjectToCompletenessGuard(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	svc.EXPECT().CreateRule(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(sampleFreshnessRule(), nil)

	srv := createTestFreshnessServer(svc)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, makeFreshnessRequest(
		http.MethodPost, freshnessRulesPath, `{"resource_types":["memory"],"threshold_days":30}`))

	require.Equal(t, http.StatusCreated, w.Code)
}

func TestFreshnessHandlers_InvalidUUIDIsBadRequest(t *testing.T) {
	svc := servicesmocks.NewMockFreshnessServiceInterface(t)
	srv := createTestFreshnessServer(svc)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, makeFreshnessRequest(
		http.MethodDelete, "/api/v1/"+testFreshnessTeamID+"/freshness/rules/not-a-uuid", ""))

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "rule_id must be a valid UUID")
}

// freshnessBodyProblem is a pure function; exercise its ordering directly.
func TestFreshnessBodyProblem(t *testing.T) {
	required := []string{"a", "b"}

	t.Run("unknown fields are reported before missing ones", func(t *testing.T) {
		fields := map[string]json.RawMessage{"a": []byte(`1`), "z": []byte(`1`)}
		assert.Equal(t, "unknown field(s): z", freshnessBodyProblem(fields, required))
	})

	t.Run("unknown fields are sorted for a stable message", func(t *testing.T) {
		fields := map[string]json.RawMessage{"a": []byte(`1`), "b": []byte(`1`), "z": []byte(`1`), "y": []byte(`1`)}
		assert.Equal(t, "unknown field(s): y, z", freshnessBodyProblem(fields, required))
	})

	t.Run("missing fields keep the declared order", func(t *testing.T) {
		assert.Equal(t, "missing required field(s): a, b",
			freshnessBodyProblem(map[string]json.RawMessage{}, required))
	})

	t.Run("a complete body has no problem", func(t *testing.T) {
		fields := map[string]json.RawMessage{"a": []byte(`1`), "b": []byte(`1`)}
		assert.Empty(t, freshnessBodyProblem(fields, required))
	})
}
