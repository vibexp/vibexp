package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

const (
	msgLimitRange = "limit must be between 1 and 100"
	msgPageRange  = "page must be between 1 and 10000"
)

// validatePaginationParams must reject a provided out-of-range or non-numeric
// value with a 400 naming the range, and only an OMITTED value takes the
// default (#1107). The old behaviour replaced both with the default.
func TestValidatePaginationParams(t *testing.T) {
	for _, tc := range []struct {
		name      string
		page      string
		limit     string
		wantPage  int
		wantLimit int
		wantErr   string
	}{
		{name: "both omitted take defaults", wantPage: 1, wantLimit: 10},
		{name: "valid values pass through", page: "3", limit: "50", wantPage: 3, wantLimit: 50},
		{name: "lower bounds are valid", page: "1", limit: "1", wantPage: 1, wantLimit: 1},
		{name: "upper bounds are valid", page: "10000", limit: "100", wantPage: 10000, wantLimit: 100},
		{name: "limit above max", limit: "101", wantErr: msgLimitRange},
		{name: "limit far above max", limit: "200", wantErr: msgLimitRange},
		{name: "limit zero", limit: "0", wantErr: msgLimitRange},
		{name: "limit negative", limit: "-1", wantErr: msgLimitRange},
		{name: "limit garbage", limit: "abc", wantErr: msgLimitRange},
		{name: "page above max", page: "10001", wantErr: msgPageRange},
		{name: "page zero", page: "0", wantErr: msgPageRange},
		{name: "page negative", page: "-1", wantErr: msgPageRange},
		{name: "page garbage", page: "abc", wantErr: msgPageRange},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validatePaginationParams(tc.page, tc.limit)
			if tc.wantErr == "" {
				require.NoError(t, err)
				assert.Equal(t, PaginationParams{Page: tc.wantPage, Limit: tc.wantLimit}, got)
				return
			}

			var apiErr *apierrors.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, http.StatusBadRequest, apiErr.Status)
			assert.Equal(t, apierrors.CodeBadRequest, apiErr.Code)
			assert.Equal(t, tc.wantErr, apiErr.Detail)
		})
	}
}

// In the search body and the MCP tool the fields are plain ints, so zero is
// "omitted"; any other out-of-range value is rejected under the caller's name
// for the limit parameter.
func TestNormalizeSearchPagination(t *testing.T) {
	for _, tc := range []struct {
		name      string
		page      int
		limit     int
		wantPage  int
		wantLimit int
		wantErr   string
	}{
		{name: "zeros take defaults", wantPage: 1, wantLimit: 10},
		{name: "valid values pass through", page: 2, limit: 100, wantPage: 2, wantLimit: 100},
		{name: "limit above max", page: 1, limit: 500, wantErr: "per_page must be between 1 and 100"},
		{name: "limit negative", page: 1, limit: -1, wantErr: "per_page must be between 1 and 100"},
		{name: "page above max", page: 10001, limit: 10, wantErr: msgPageRange},
		{name: "page negative", page: -1, limit: 10, wantErr: msgPageRange},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, limit, err := normalizeSearchPagination(tc.page, tc.limit, "per_page")
			if tc.wantErr == "" {
				require.NoError(t, err)
				assert.Equal(t, tc.wantPage, page)
				assert.Equal(t, tc.wantLimit, limit)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tc.wantErr, errorMessage(err))
		})
	}
}

func TestErrorMessage(t *testing.T) {
	assert.Equal(t, "the detail", errorMessage(apierrors.NewBadRequestError("the detail")),
		"an APIError surfaces its Detail, not its code-prefixed Error()")
	assert.Equal(t, "plain", errorMessage(errors.New("plain")))
}

// outOfRangePaginationQueries is the query-string matrix every list endpoint
// must reject. Non-numeric values are included: the strict-server domains'
// binder rejects those itself (with its own message), the chi ones reach
// validatePaginationParams.
var outOfRangePaginationQueries = []struct {
	query   string
	message string
}{
	{"?limit=101", msgLimitRange},
	{"?limit=0", msgLimitRange},
	{"?limit=-1", msgLimitRange},
	{"?page=10001", msgPageRange},
	{"?page=0", msgPageRange},
}

// Every strict-server list operation that calls validatePaginationParams must
// answer 400 with the range message -- no service expectation is registered,
// so reaching the service is itself a failure.
func TestStrictListEndpoints_RejectOutOfRangePagination(t *testing.T) {
	// Each entry serves one request through that domain's own test server and
	// request helper (they differ in how they authenticate).
	endpoints := map[string]func(t *testing.T, query string) *httptest.ResponseRecorder{
		"listArtifacts": func(t *testing.T, query string) *httptest.ResponseRecorder {
			srv := strictArtifactServer(t, servicesmocks.NewMockArtifactServiceInterface(t))
			_, w := strictArtifactRequest(t, srv, "/api/v1/"+strictArtTeamID+"/artifacts"+query)
			return w
		},
		"listArtifactsByProject": func(t *testing.T, query string) *httptest.ResponseRecorder {
			srv := strictArtifactServer(t, servicesmocks.NewMockArtifactServiceInterface(t))
			_, w := strictArtifactRequest(t, srv,
				"/api/v1/"+strictArtTeamID+"/artifacts/"+strictArtProject+query)
			return w
		},
		"listSpecLibraries": func(t *testing.T, query string) *httptest.ResponseRecorder {
			srv := strictBlueprintServer(t, servicesmocks.NewMockBlueprintServiceInterface(t))
			_, w := strictBlueprintRequest(t, srv, "/api/v1/"+strictBpTeamID+"/blueprints"+query)
			return w
		},
		"listSpecLibrariesByProject": func(t *testing.T, query string) *httptest.ResponseRecorder {
			srv := strictBlueprintServer(t, servicesmocks.NewMockBlueprintServiceInterface(t))
			_, w := strictBlueprintRequest(t, srv,
				"/api/v1/"+strictBpTeamID+"/blueprints/"+strictBpProject+query)
			return w
		},
		"listMemories": func(t *testing.T, query string) *httptest.ResponseRecorder {
			srv := createMemoryTestServer(newMockMemoryContainer(t))
			req := makeMemoryAuthenticatedRequest(
				"GET", "/api/v1/"+memoriesTestTeamID+"/memories"+query, nil, memoriesTestUserID)
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, req)
			return w
		},
		"listPrompts": func(t *testing.T, query string) *httptest.ResponseRecorder {
			srv, _ := strictPromptServer(t)
			_, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts"+query)
			return w
		},
	}

	for name, serve := range endpoints {
		for _, tc := range outOfRangePaginationQueries {
			t.Run(name+tc.query, func(t *testing.T) {
				w := serve(t, tc.query)

				require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
				var body map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				assert.Equal(t, tc.message, body["detail"])
				assert.Equal(t, apierrors.CodeBadRequest, body["code"])
			})
		}
	}
}

// Omitting limit on the artifacts list must use the default, which is the
// `default:` the spec documents for that parameter.
func TestStrictListArtifacts_OmittedLimitUsesTheDocumentedDefault(t *testing.T) {
	got, err := artifactFiltersFromQuery(strictArtTeamID, "", artifactListQuery{})
	require.NoError(t, err)
	assert.Equal(t, 1, got.Page)
	assert.Equal(t, 10, got.Limit)
}

// The chi-routed list endpoints (agents, the three feed lists) reject the same
// matrix -- plus a non-numeric value, which only they parse themselves -- with
// a 400 carrying the range message.
func TestChiListEndpoints_RejectOutOfRangePagination(t *testing.T) {
	queries := append([]struct {
		query   string
		message string
	}{
		{"?limit=abc", msgLimitRange},
		{"?page=abc", msgPageRange},
	}, outOfRangePaginationQueries...)

	srv := &Server{config: &config.Config{}, logger: slog.New(slog.DiscardHandler)}
	const itemID = "550e8400-e29b-41d4-a716-446655440099"

	handlers := map[string]func(w http.ResponseWriter, r *http.Request){
		"listAgents": func(w http.ResponseWriter, r *http.Request) {
			_, ok := parseAgentFilters(w, r, strictArtTeamID)
			require.False(t, ok)
		},
		"listFeeds":           srv.handleListFeeds,
		"listFeedItems":       srv.handleListFeedItems,
		"listFeedItemsByFeed": srv.handleListFeedItemsByFeed,
		"listFeedItemReplies": srv.handleListFeedItemReplies,
	}

	for name, handler := range handlers {
		for _, tc := range queries {
			t.Run(name+tc.query, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, "/x"+tc.query, nil)
				rctx := chi.NewRouteContext()
				rctx.URLParams.Add("team_id", strictArtTeamID)
				rctx.URLParams.Add("feed_id", itemID)
				rctx.URLParams.Add("item_id", itemID)
				ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
				ctx = context.WithValue(ctx, contextKeyUserID, strictArtUserID)
				w := httptest.NewRecorder()

				// No container: reaching a service would panic, which is itself
				// the failure this test exists to catch.
				handler(w, req.WithContext(ctx))

				require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
				var body map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				assert.Equal(t, tc.message, body["detail"])
			})
		}
	}
}

// The feed lists keep their own default page size of 20 when limit is omitted
// (the spec's documented default for them), and accept the maximum.
func TestFeedPaginationParams_DefaultAndMax(t *testing.T) {
	got, err := feedPaginationParams(httptest.NewRequest(http.MethodGet, "/x", nil))
	require.NoError(t, err)
	assert.Equal(t, PaginationParams{Page: 1, Limit: 20}, got)

	got, err = feedPaginationParams(httptest.NewRequest(http.MethodGet, "/x?page=2&limit=100", nil))
	require.NoError(t, err)
	assert.Equal(t, PaginationParams{Page: 2, Limit: 100}, got)
}
