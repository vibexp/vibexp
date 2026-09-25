package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// adminConfigTeamRepo returns a TeamRepository mock that knows exactly one team.
func adminConfigTeamRepo(t *testing.T, teamID string) *repomocks.MockTeamRepository {
	t.Helper()
	teams := repomocks.NewMockTeamRepository(t)
	teams.EXPECT().GetByID(mock.Anything, teamID).Return(&models.Team{ID: teamID, Name: "Acme"}, nil).Maybe()
	return teams
}

// serveAdminConfig runs one request through the strict admin router.
func serveAdminConfig(t *testing.T, c *adminMockContainer, path string) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	srv := newAdminTestServer(&config.Config{}, c)
	req := httptest.NewRequest("GET", path, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)
	return req, rr
}

// adminConfigPaths lists every config op, for the shared 404/500 guard tests.
func adminConfigPaths(teamID string) map[string]string {
	base := "/api/v1/admin/teams/" + teamID + "/config/"
	return map[string]string{
		"search":         base + "search",
		"ai-summary":     base + "ai-summary",
		"freshness":      base + "freshness",
		"artifact-types": base + "artifact-types",
		"settings-audit": base + "settings-audit",
		// Credential-bearing sections (#1141).
		"model-providers":     base + "model-providers",
		"embedding-providers": base + "embedding-providers",
		"email-provider":      base + "email-provider",
		"github":              base + "github",
	}
}

// TestAdminTeamConfig_TeamGuard asserts every config op answers a malformed id
// with 400, an unknown team with 404 and a team lookup failure with 500 — without touching the settings
// getters, which would otherwise report `source: instance` for a bogus id.
func TestAdminTeamConfig_TeamGuard(t *testing.T) {
	teamID := uuid.NewString()
	malformed := adminConfigPaths("not-a-uuid")
	for name, path := range adminConfigPaths(teamID) {
		t.Run(name+" malformed team id is 400", func(t *testing.T) {
			// Rejected by the binder before the handler: no team lookup at all.
			req, rr := serveAdminConfig(t, &adminMockContainer{}, malformed[name])

			assert.Equal(t, http.StatusBadRequest, rr.Code)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
		t.Run(name+" unknown team is 404", func(t *testing.T) {
			teams := repomocks.NewMockTeamRepository(t)
			teams.EXPECT().GetByID(mock.Anything, teamID).Return(nil, repositories.ErrTeamNotFound)

			req, rr := serveAdminConfig(t, &adminMockContainer{teamRepo: teams}, path)

			assert.Equal(t, http.StatusNotFound, rr.Code)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
		t.Run(name+" team lookup failure is 500", func(t *testing.T) {
			teams := repomocks.NewMockTeamRepository(t)
			teams.EXPECT().GetByID(mock.Anything, teamID).Return(nil, errors.New("db down"))

			req, rr := serveAdminConfig(t, &adminMockContainer{teamRepo: teams}, path)

			assert.Equal(t, http.StatusInternalServerError, rr.Code)
			assert.NotContains(t, rr.Body.String(), "db down")
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestAdminTeamConfigRoutes_UnauthenticatedGets404 exercises the FULL router
// (setupAdminRoutes: optionalAuthMiddleware + instanceAdminMiddleware) for every
// config op: an unauthenticated request is answered 404 and never reaches a
// handler — the settings mocks carry no expectations, so any call fails.
func TestAdminTeamConfigRoutes_UnauthenticatedGets404(t *testing.T) {
	for name, path := range adminConfigPaths(uuid.NewString()) {
		t.Run(name, func(t *testing.T) {
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
				authService:           servicesmocks.NewMockAuthServiceInterface(t),
				teamRepo:              repomocks.NewMockTeamRepository(t),
				searchSettingsService: servicesmocks.NewMockTeamSearchSettingsServiceInterface(t),
				aiSummaryService:      servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t),
				freshnessService:      servicesmocks.NewMockFreshnessServiceInterface(t),
				typeService:           servicesmocks.NewMockTypeServiceInterface(t),
				settingsAuditRepo:     repomocks.NewMockTeamSettingsAuditRepository(t),

				modelProviderService:     servicesmocks.NewMockModelProviderServiceInterface(t),
				embeddingProviderService: servicesmocks.NewMockEmbeddingProviderServiceInterface(t),
				embeddingStatusService:   servicesmocks.NewMockEmbeddingCoverageGetter(t),
				emailProviderService:     servicesmocks.NewMockTeamEmailProviderServiceInterface(t),
				githubAppConfigService:   servicesmocks.NewMockGitHubAppConfigServiceInterface(t),
				githubAppService:         servicesmocks.NewMockGitHubAppServiceInterface(t),
			})

			rr := httptest.NewRecorder()
			srv.router.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))

			require.Equal(t, http.StatusNotFound, rr.Code)
		})
	}
}

// TestAdminTeamConfig_NonAdminIs404 covers the other half of the gate: an
// authenticated NON-admin is stopped by instanceAdminMiddleware on a config
// path. It drives the middleware directly (the full router's auth needs a real
// session), so route mounting is pinned by the unauthenticated test above.
func TestAdminTeamConfig_NonAdminIs404(t *testing.T) {
	cfg := &config.Config{Auth: config.AuthConfig{InstanceAdmins: config.EnvStringSlice{"admin@example.com"}}}
	mockAuth := servicesmocks.NewMockAuthServiceInterface(t)
	mockAuth.On("GetUserByID", mock.Anything, "user-2").
		Return(&models.User{ID: "user-2", Email: "user@example.com"}, nil)
	srv := newAdminTestServer(cfg, &adminMockContainer{authService: mockAuth})

	req := httptest.NewRequest("GET", "/api/v1/admin/teams/"+uuid.NewString()+"/config/search", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-2"))
	rr := httptest.NewRecorder()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("non-admin reached the config handler")
	})
	srv.instanceAdminMiddleware(next).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

var adminSearchValuesPaths = []string{
	"recency_ranking_enabled", "rank_weight_relevance", "rank_weight_created",
	"rank_weight_updated", "rank_half_life_days",
}

func prefixed(prefix string, keys []string) []string {
	out := make([]string, 0, len(keys)+1)
	out = append(out, prefix)
	for _, k := range keys {
		out = append(out, prefix+"."+k)
	}
	return out
}

func TestGetAdminTeamSearchConfig(t *testing.T) {
	teamID := uuid.NewString()
	defaults := models.TeamSearchSettingsValues{
		RecencyRankingEnabled: true, RankWeightRelevance: 0.5, RankWeightCreated: 0.3,
		RankWeightUpdated: 0.2, RankHalfLifeDays: 90,
	}
	custom := models.TeamSearchSettingsValues{RankWeightRelevance: 1, RankHalfLifeDays: 30}

	tests := []struct {
		name   string
		view   models.TeamSearchSettingsView
		source string
		values models.TeamSearchSettingsValues
	}{
		{"inherited team reports instance", models.TeamSearchSettingsView{
			Source: models.TeamSearchSettingsSourceInstance, Values: defaults,
			InstanceDefaults: defaults, RankCandidateCap: 500,
		}, "instance", defaults},
		{"team override reports team", models.TeamSearchSettingsView{
			Source: models.TeamSearchSettingsSourceTeam, Values: custom,
			InstanceDefaults: defaults, RankCandidateCap: 500,
		}, "team", custom},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			search := servicesmocks.NewMockTeamSearchSettingsServiceInterface(t)
			search.EXPECT().Get(mock.Anything, teamID).Return(&tc.view, nil)

			req, rr := serveAdminConfig(t, &adminMockContainer{
				teamRepo: adminConfigTeamRepo(t, teamID), searchSettingsService: search,
			}, adminConfigPaths(teamID)["search"])

			require.Equal(t, http.StatusOK, rr.Code)
			var resp admingen.AdminTeamSearchConfig
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			assert.Equal(t, tc.source, string(resp.Source))
			assert.InDelta(t, tc.values.RankHalfLifeDays, resp.Values.RankHalfLifeDays, 1e-9)
			assert.InDelta(t, 90, resp.InstanceDefaults.RankHalfLifeDays, 1e-9)
			assert.Equal(t, 500, resp.RankCandidateCap)

			want := append([]string{"source", "rank_candidate_cap"}, prefixed("values", adminSearchValuesPaths)...)
			want = append(want, prefixed("instance_defaults", adminSearchValuesPaths)...)
			assertJSONKeyPaths(t, rr.Body.Bytes(), want...)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

func TestGetAdminTeamSearchConfig_ServiceError(t *testing.T) {
	teamID := uuid.NewString()
	search := servicesmocks.NewMockTeamSearchSettingsServiceInterface(t)
	search.EXPECT().Get(mock.Anything, teamID).Return(nil, errors.New("boom"))

	req, rr := serveAdminConfig(t, &adminMockContainer{
		teamRepo: adminConfigTeamRepo(t, teamID), searchSettingsService: search,
	}, adminConfigPaths(teamID)["search"])

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

var adminAISummaryValuesPaths = []string{"enabled", "model_provider_id", "top_n", "style", "max_output_tokens"}

func TestGetAdminTeamAISummaryConfig(t *testing.T) {
	teamID := uuid.NewString()
	providerID := uuid.NewString()
	defaults := models.TeamAISummarySettingsValues{
		Enabled: true, TopN: 5, Style: models.AISummaryStyleBalanced, MaxOutputTokens: 800,
	}
	custom := models.TeamAISummarySettingsValues{
		Enabled: true, ModelProviderID: &providerID, TopN: 3,
		Style: models.AISummaryStyleConcise, MaxOutputTokens: 400,
	}
	apiKey := "sk-SENTINEL-NEVER-SERIALIZED"

	tests := []struct {
		name         string
		view         models.TeamAISummarySettingsView
		setupRepo    func(m *repomocks.MockModelProviderRepository)
		source       string
		providerName *string
	}{
		{
			name: "inherited team reports instance with no provider lookup",
			view: models.TeamAISummarySettingsView{
				Source: models.TeamAISummarySettingsSourceInstance, Values: defaults, InstanceDefaults: defaults,
				MaxTopN: 20, MaxOutputTokensCeiling: 4000, Available: false,
			},
			setupRepo: func(*repomocks.MockModelProviderRepository) {},
			source:    "instance",
		},
		{
			name: "team override resolves the provider name only",
			view: models.TeamAISummarySettingsView{
				Source: models.TeamAISummarySettingsSourceTeam, Values: custom, InstanceDefaults: defaults,
				MaxTopN: 20, MaxOutputTokensCeiling: 4000, Available: true,
			},
			setupRepo: func(m *repomocks.MockModelProviderRepository) {
				m.EXPECT().GetByID(mock.Anything, teamID, providerID).Return(&models.ModelProvider{
					ID: providerID, Name: "OpenAI (Platform)", APIKeyEncrypted: &apiKey,
				}, nil)
			},
			source:       "team",
			providerName: strPtr("OpenAI (Platform)"),
		},
		{
			name: "a deleted provider is a null name, not an error",
			view: models.TeamAISummarySettingsView{
				Source: models.TeamAISummarySettingsSourceTeam, Values: custom, InstanceDefaults: defaults,
				MaxTopN: 20, MaxOutputTokensCeiling: 4000, Available: true,
			},
			setupRepo: func(m *repomocks.MockModelProviderRepository) {
				m.EXPECT().GetByID(mock.Anything, teamID, providerID).Return(nil, repositories.ErrModelProviderNotFound)
			},
			source: "team",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ai := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)
			ai.EXPECT().Get(mock.Anything, teamID).Return(&tc.view, nil)
			providers := repomocks.NewMockModelProviderRepository(t)
			tc.setupRepo(providers)

			req, rr := serveAdminConfig(t, &adminMockContainer{
				teamRepo: adminConfigTeamRepo(t, teamID), aiSummaryService: ai, modelProviderRepo: providers,
			}, adminConfigPaths(teamID)["ai-summary"])

			require.Equal(t, http.StatusOK, rr.Code)
			var resp admingen.AdminTeamAISummaryConfig
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			assert.Equal(t, tc.source, string(resp.Source))
			assert.Equal(t, tc.providerName, resp.ModelProviderName)
			assert.Equal(t, 20, resp.MaxTopN)
			assert.Equal(t, 4000, resp.MaxOutputTokensCeiling)
			assert.Equal(t, tc.view.Available, resp.Available)
			assert.Equal(t, tc.view.Values.TopN, resp.Values.TopN)
			assert.Equal(t, 5, resp.InstanceDefaults.TopN)

			want := append([]string{
				"source", "model_provider_name", "max_top_n", "max_output_tokens_ceiling", "available",
			}, prefixed("values", adminAISummaryValuesPaths)...)
			want = append(want, prefixed("instance_defaults", adminAISummaryValuesPaths)...)
			assertJSONKeyPaths(t, rr.Body.Bytes(), want...)
			assertBodyExcludes(t, rr.Body.Bytes(), apiKey)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

func TestGetAdminTeamAISummaryConfig_ProviderLookupError(t *testing.T) {
	teamID, providerID := uuid.NewString(), uuid.NewString()
	ai := servicesmocks.NewMockTeamAISummarySettingsServiceInterface(t)
	ai.EXPECT().Get(mock.Anything, teamID).Return(&models.TeamAISummarySettingsView{
		Source: models.TeamAISummarySettingsSourceTeam,
		Values: models.TeamAISummarySettingsValues{ModelProviderID: &providerID, Style: "balanced"},
	}, nil)
	providers := repomocks.NewMockModelProviderRepository(t)
	providers.EXPECT().GetByID(mock.Anything, teamID, providerID).Return(nil, errors.New("db down"))

	req, rr := serveAdminConfig(t, &adminMockContainer{
		teamRepo: adminConfigTeamRepo(t, teamID), aiSummaryService: ai, modelProviderRepo: providers,
	}, adminConfigPaths(teamID)["ai-summary"])

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

var adminFreshnessRulePaths = []string{
	"rules[].id", "rules[].project_id", "rules[].resource_types", "rules[].mediums",
	"rules[].threshold_days", "rules[].enabled", "rules[].created_at", "rules[].updated_at",
}

func TestGetAdminTeamFreshnessConfig(t *testing.T) {
	teamID, projectID := uuid.NewString(), uuid.NewString()
	defaults := models.FreshnessSettingsValues{IntervalSeconds: 86400, ReversibilityEnabled: true}
	now := time.Now().UTC()
	rules := []*models.FreshnessRule{
		{ID: uuid.NewString(), TeamID: teamID, ResourceTypes: []string{"memory"}, ThresholdDays: 30,
			Enabled: true, CreatedAt: now, UpdatedAt: now},
		{ID: uuid.NewString(), TeamID: teamID, ProjectID: &projectID, ResourceTypes: []string{"prompt"},
			Mediums: []string{"mcp"}, ThresholdDays: 60, CreatedAt: now, UpdatedAt: now},
	}

	tests := []struct {
		name   string
		view   models.TeamFreshnessSettingsView
		rules  []*models.FreshnessRule
		source string
		want   []string
	}{
		{
			name: "inherited team with no rules serializes rules as []",
			view: models.TeamFreshnessSettingsView{
				Source: models.FreshnessSettingsSourceInstance, Values: defaults, Defaults: defaults,
			},
			rules:  []*models.FreshnessRule{},
			source: "instance",
		},
		{
			name: "team override with project-scoped and team-wide rules",
			view: models.TeamFreshnessSettingsView{
				Source:   models.FreshnessSettingsSourceTeam,
				Values:   models.FreshnessSettingsValues{IntervalSeconds: 3600},
				Defaults: defaults,
			},
			rules:  rules,
			source: "team",
			want:   adminFreshnessRulePaths,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fresh := servicesmocks.NewMockFreshnessServiceInterface(t)
			fresh.EXPECT().GetSettings(mock.Anything, teamID).Return(&tc.view, nil)
			fresh.EXPECT().ListRules(mock.Anything, teamID).Return(tc.rules, nil)

			req, rr := serveAdminConfig(t, &adminMockContainer{
				teamRepo: adminConfigTeamRepo(t, teamID), freshnessService: fresh,
			}, adminConfigPaths(teamID)["freshness"])

			require.Equal(t, http.StatusOK, rr.Code)
			var resp admingen.AdminTeamFreshnessConfig
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			assert.Equal(t, tc.source, string(resp.Source))
			assert.Equal(t, tc.view.Values.IntervalSeconds, resp.Values.IntervalSeconds)
			assert.Equal(t, 86400, resp.Defaults.IntervalSeconds)
			require.Len(t, resp.Rules, len(tc.rules))
			assert.NotContains(t, rr.Body.String(), `"rules":null`)
			if len(tc.rules) > 0 {
				assert.Nil(t, resp.Rules[0].ProjectId)
				assert.Equal(t, []string{}, resp.Rules[0].Mediums)
				require.NotNil(t, resp.Rules[1].ProjectId)
				assert.Equal(t, projectID, resp.Rules[1].ProjectId.String())
			}

			want := append([]string{
				"source", "values", "values.interval_seconds", "values.reversibility_enabled",
				"defaults", "defaults.interval_seconds", "defaults.reversibility_enabled", "rules",
			}, tc.want...)
			assertJSONKeyPaths(t, rr.Body.Bytes(), want...)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

func TestGetAdminTeamFreshnessConfig_ServiceErrors(t *testing.T) {
	teamID := uuid.NewString()
	view := &models.TeamFreshnessSettingsView{Source: models.FreshnessSettingsSourceInstance}

	t.Run("settings", func(t *testing.T) {
		fresh := servicesmocks.NewMockFreshnessServiceInterface(t)
		fresh.EXPECT().GetSettings(mock.Anything, teamID).Return(nil, errors.New("boom"))
		req, rr := serveAdminConfig(t, &adminMockContainer{
			teamRepo: adminConfigTeamRepo(t, teamID), freshnessService: fresh,
		}, adminConfigPaths(teamID)["freshness"])
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})
	t.Run("rules", func(t *testing.T) {
		fresh := servicesmocks.NewMockFreshnessServiceInterface(t)
		fresh.EXPECT().GetSettings(mock.Anything, teamID).Return(view, nil)
		fresh.EXPECT().ListRules(mock.Anything, teamID).Return(nil, errors.New("boom"))
		req, rr := serveAdminConfig(t, &adminMockContainer{
			teamRepo: adminConfigTeamRepo(t, teamID), freshnessService: fresh,
		}, adminConfigPaths(teamID)["freshness"])
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})
}

func TestGetAdminTeamArtifactTypes(t *testing.T) {
	teamID := uuid.NewString()
	now := time.Now().UTC()
	types := []models.Type{
		{ID: uuid.NewString(), ResourceType: "artifacts", Slug: "general", Name: "General",
			IsSystem: true, CreatedAt: now, UpdatedAt: now},
		{ID: uuid.NewString(), TeamID: teamID, ResourceType: "artifacts", Slug: "bug-report",
			Name: "Bug report", CreatedBy: "creator-SENTINEL", CreatedAt: now, UpdatedAt: now},
	}

	tests := []struct {
		name  string
		types []models.Type
		want  []string
	}{
		{"empty set serializes as []", []models.Type{}, nil},
		{"system and custom types", types, []string{
			"types[].id", "types[].slug", "types[].name", "types[].is_system",
			"types[].created_at", "types[].updated_at",
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			typeSvc := servicesmocks.NewMockTypeServiceInterface(t)
			typeSvc.EXPECT().List(mock.Anything, teamID, "artifacts").Return(tc.types, nil)

			req, rr := serveAdminConfig(t, &adminMockContainer{
				teamRepo: adminConfigTeamRepo(t, teamID), typeService: typeSvc,
			}, adminConfigPaths(teamID)["artifact-types"])

			require.Equal(t, http.StatusOK, rr.Code)
			var resp admingen.AdminTeamArtifactTypes
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			require.Len(t, resp.Types, len(tc.types))
			assert.NotContains(t, rr.Body.String(), `"types":null`)
			if len(tc.types) > 0 {
				assert.True(t, resp.Types[0].IsSystem)
				assert.Equal(t, "bug-report", resp.Types[1].Slug)
				assert.False(t, resp.Types[1].IsSystem)
			}

			assertJSONKeyPaths(t, rr.Body.Bytes(), append([]string{"types"}, tc.want...)...)
			assertBodyExcludes(t, rr.Body.Bytes(), "creator-SENTINEL", teamID)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

func TestGetAdminTeamArtifactTypes_ServiceError(t *testing.T) {
	teamID := uuid.NewString()
	typeSvc := servicesmocks.NewMockTypeServiceInterface(t)
	typeSvc.EXPECT().List(mock.Anything, teamID, "artifacts").Return(nil, errors.New("boom"))

	req, rr := serveAdminConfig(t, &adminMockContainer{
		teamRepo: adminConfigTeamRepo(t, teamID), typeService: typeSvc,
	}, adminConfigPaths(teamID)["artifact-types"])

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

var adminAuditEntryPaths = []string{
	"entries", "entries[].id", "entries[].surface", "entries[].actor_user_id", "entries[].actor_name",
	"entries[].source_team_id", "entries[].source_team_name", "entries[].source_resource_id",
	"entries[].created_resource_id", "entries[].detail", "entries[].created_at",
	"total_count", "page", "per_page", "total_pages",
}

// auditLeakSentinel marks fixture values that must not appear in a response.
const auditLeakSentinel = "LEAK-SENTINEL"

func mustAuditDetail(t *testing.T, detail map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(detail)
	require.NoError(t, err)
	return raw
}

func TestListAdminTeamSettingsAudit(t *testing.T) {
	teamID := uuid.NewString()
	actorID, sourceTeamID := uuid.NewString(), uuid.NewString()
	srcResource, newResource := uuid.NewString(), uuid.NewString()
	entries := []*models.TeamSettingsAudit{
		{
			ID: uuid.NewString(), TeamID: teamID, ActorUserID: &actorID, SourceTeamID: &sourceTeamID,
			Surface: models.SettingsAuditSurfaceModelProvider, SourceResourceID: &srcResource,
			CreatedResourceID: &newResource, CreatedAt: time.Now().UTC(),
			Detail: mustAuditDetail(t, map[string]any{
				"source_name": "OpenAI", "created_name": "OpenAI", "provider_type": "openai",
				"model": "gpt", "has_api_key": true,
				// Keys no copy surface writes, carrying values that must never surface.
				"api_key": auditLeakSentinel + "-key", "secret": auditLeakSentinel + "-secret",
			}),
		},
		{
			ID: uuid.NewString(), TeamID: teamID, Surface: models.SettingsAuditSurfaceEmbeddingProvider,
			CreatedAt: time.Now().UTC(),
			Detail: json.RawMessage(`{"source_name":"E","created_name":"E","provider_type":"openai",` +
				`"model":"m","has_api_key":false,"becomes_active":true,"displaced_model":"old",` +
				`"displaced_embedded_resources":3,"base_url":"http://internal-SENTINEL"}`),
		},
		{
			ID: uuid.NewString(), TeamID: teamID, Surface: models.SettingsAuditSurfaceCustomTypes,
			CreatedAt: time.Now().UTC(),
			Detail: json.RawMessage(`{"added_ids":["x"],"added_slugs":["bug"],"skipped_slugs":[],` +
				`"token":"tok-SENTINEL"}`),
		},
	}

	audit := repomocks.NewMockTeamSettingsAuditRepository(t)
	audit.EXPECT().ListByTeam(mock.Anything, teamID, 3, 3).Return(entries, 9, nil)
	users := repomocks.NewMockUserRepository(t)
	users.EXPECT().GetNamesByIDs(mock.Anything, []string{actorID}).Return(map[string]string{actorID: "Ada"}, nil)
	teams := adminConfigTeamRepo(t, teamID)
	teams.EXPECT().GetNamesByIDs(mock.Anything, []string{sourceTeamID}).
		Return(map[string]string{sourceTeamID: "Platform"}, nil)

	req, rr := serveAdminConfig(t, &adminMockContainer{
		teamRepo: teams, userRepo: users, settingsAuditRepo: audit,
	}, adminConfigPaths(teamID)["settings-audit"]+"?page=2&limit=3")

	require.Equal(t, http.StatusOK, rr.Code)
	var resp admingen.AdminTeamSettingsAuditListResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Entries, 3)
	assert.Equal(t, 9, resp.TotalCount)
	assert.Equal(t, 2, resp.Page)
	assert.Equal(t, 3, resp.PerPage)
	assert.Equal(t, 3, resp.TotalPages)
	require.NotNil(t, resp.Entries[0].ActorName)
	assert.Equal(t, "Ada", *resp.Entries[0].ActorName)
	require.NotNil(t, resp.Entries[0].SourceTeamName)
	assert.Equal(t, "Platform", *resp.Entries[0].SourceTeamName)

	// detail keeps each surface's allowlisted keys and drops everything else.
	details := []string{
		"entries[].detail.source_name", "entries[].detail.created_name", "entries[].detail.provider_type",
		"entries[].detail.model", "entries[].detail.has_api_key", "entries[].detail.becomes_active",
		"entries[].detail.displaced_model", "entries[].detail.displaced_embedded_resources",
		"entries[].detail.added_ids", "entries[].detail.added_slugs", "entries[].detail.skipped_slugs",
	}
	assertJSONKeyPaths(t, rr.Body.Bytes(), append(slices.Clone(adminAuditEntryPaths), details...)...)
	assertBodyExcludes(t, rr.Body.Bytes(), auditLeakSentinel, "internal-SENTINEL", "tok-SENTINEL")
	assert.Len(t, resp.Entries[0].Detail, 5)
	assert.Len(t, resp.Entries[2].Detail, 3)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestListAdminTeamSettingsAudit_EmptyPage(t *testing.T) {
	teamID := uuid.NewString()
	audit := repomocks.NewMockTeamSettingsAuditRepository(t)
	audit.EXPECT().ListByTeam(mock.Anything, teamID, 20, 0).Return([]*models.TeamSettingsAudit{}, 0, nil)
	users := repomocks.NewMockUserRepository(t)
	users.EXPECT().GetNamesByIDs(mock.Anything, []string{}).Return(map[string]string{}, nil)
	teams := adminConfigTeamRepo(t, teamID)
	teams.EXPECT().GetNamesByIDs(mock.Anything, []string{}).Return(map[string]string{}, nil)

	req, rr := serveAdminConfig(t, &adminMockContainer{
		teamRepo: teams, userRepo: users, settingsAuditRepo: audit,
	}, adminConfigPaths(teamID)["settings-audit"])

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"entries":[]`)
	assertJSONKeyPaths(t, rr.Body.Bytes(), "entries", "total_count", "page", "per_page", "total_pages")
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestListAdminTeamSettingsAudit_Paging400(t *testing.T) {
	teamID := uuid.NewString()
	for _, query := range []string{"?limit=0", "?limit=101", "?page=0"} {
		t.Run(query, func(t *testing.T) {
			// No team lookup and no repository read: bounds are checked first.
			req, rr := serveAdminConfig(t, &adminMockContainer{},
				adminConfigPaths(teamID)["settings-audit"]+query)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

func TestListAdminTeamSettingsAudit_RepositoryError(t *testing.T) {
	teamID := uuid.NewString()
	audit := repomocks.NewMockTeamSettingsAuditRepository(t)
	audit.EXPECT().ListByTeam(mock.Anything, teamID, 20, 0).Return(nil, 0, errors.New("boom"))

	req, rr := serveAdminConfig(t, &adminMockContainer{
		teamRepo: adminConfigTeamRepo(t, teamID), settingsAuditRepo: audit,
	}, adminConfigPaths(teamID)["settings-audit"])

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestFilterSettingsAuditDetail pins the fail-closed allowlist: an unknown
// surface keeps no keys, and malformed detail is an empty object.
func TestFilterSettingsAuditDetail(t *testing.T) {
	assert.Equal(t, map[string]interface{}{},
		filterSettingsAuditDetail("future_surface", json.RawMessage(`{"source_name":"x"}`)))
	assert.Equal(t, map[string]interface{}{},
		filterSettingsAuditDetail(models.SettingsAuditSurfaceModelProvider, json.RawMessage(`not json`)))
	assert.Equal(t, map[string]interface{}{"model": "m"},
		filterSettingsAuditDetail(models.SettingsAuditSurfaceModelProvider, json.RawMessage(`{"model":"m","api_key":"k"}`)))
}
