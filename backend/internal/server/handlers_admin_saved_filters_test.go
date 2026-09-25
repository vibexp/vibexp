package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const savedFiltersActingAdmin = "11111111-1111-4111-8111-111111111111"

// newSavedFiltersRouter wires the REAL preferences service over a mocked
// repository, so these tests cover the validation → 400 mapping end to end.
func newSavedFiltersRouter(t *testing.T) (http.Handler, *repomocks.MockUserPreferencesRepository) {
	t.Helper()
	repo := repomocks.NewMockUserPreferencesRepository(t)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
		prefsService: services.NewUserPreferencesService(repo),
	})
	return mountAdminStrictRouter(srv), repo
}

func savedFiltersRequest(t *testing.T, method, list, actingID string, body any) *http.Request {
	t.Helper()
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, "/api/v1/admin/saved-filters/"+list, nil)
	} else {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req = httptest.NewRequest(method, "/api/v1/admin/saved-filters/"+list, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
	}
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, actingID))
}

func presetInput(name string) map[string]any {
	return map[string]any{"name": name, "query": map[string]string{"status": "active"}}
}

func TestGetAdminSavedFilters_Success(t *testing.T) {
	router, repo := newSavedFiltersRouter(t)
	id := uuid.NewString()
	repo.On("GetAdminSavedFilters", mock.Anything, savedFiltersActingAdmin, "teams").Return(
		[]models.AdminSavedFilterPreset{{ID: id, Name: "Dormant teams", Query: map[string]string{"sort_by": "created_at"}}},
		int64(3), nil,
	)

	req := savedFiltersRequest(t, http.MethodGet, "teams", savedFiltersActingAdmin, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp admingen.AdminSavedFilters
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, admingen.Teams, resp.List)
	assert.EqualValues(t, 3, resp.Version)
	require.Len(t, resp.Presets, 1)
	assert.Equal(t, id, resp.Presets[0].Id.String())
	assert.Equal(t, "Dormant teams", resp.Presets[0].Name)
	assert.Equal(t, "created_at", resp.Presets[0].Query["sort_by"])
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestGetAdminSavedFilters_EmptySerializesArrays: nothing saved yet reads as
// `presets: []` and `version: 0`, and a preset with a nil query still emits `{}`.
func TestGetAdminSavedFilters_EmptySerializesArrays(t *testing.T) {
	router, repo := newSavedFiltersRouter(t)
	repo.On("GetAdminSavedFilters", mock.Anything, savedFiltersActingAdmin, "users").
		Return(nil, int64(0), nil)

	req := savedFiltersRequest(t, http.MethodGet, "users", savedFiltersActingAdmin, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, `{"list":"users","presets":[],"version":0}`, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestToGenAdminSavedFilters_NilQueryIsEmptyObject(t *testing.T) {
	out, err := toGenAdminSavedFilters(admingen.Projects,
		[]models.AdminSavedFilterPreset{{ID: uuid.NewString(), Name: "x"}}, 1)
	require.NoError(t, err)
	raw, err := json.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"query":{}`)
}

func TestToGenAdminSavedFilters_BadStoredIDIsError(t *testing.T) {
	_, err := toGenAdminSavedFilters(admingen.Projects,
		[]models.AdminSavedFilterPreset{{ID: "not-a-uuid", Name: "x"}}, 1)
	require.Error(t, err)
}

func TestGetAdminSavedFilters_UnknownListReturns400(t *testing.T) {
	router, _ := newSavedFiltersRouter(t) // no repo expectation: never reached

	req := savedFiltersRequest(t, http.MethodGet, "widgets", savedFiltersActingAdmin, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "widgets")
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestGetAdminSavedFilters_RepoErrorReturns500(t *testing.T) {
	router, repo := newSavedFiltersRouter(t)
	repo.On("GetAdminSavedFilters", mock.Anything, savedFiltersActingAdmin, "users").
		Return(nil, int64(0), errors.New("db down"))

	req := savedFiltersRequest(t, http.MethodGet, "users", savedFiltersActingAdmin, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestGetAdminSavedFilters_StoredBadIDReturns500: a corrupt stored id is a
// server fault, not something to pass through as an invalid response.
func TestGetAdminSavedFilters_StoredBadIDReturns500(t *testing.T) {
	router, repo := newSavedFiltersRouter(t)
	repo.On("GetAdminSavedFilters", mock.Anything, savedFiltersActingAdmin, "users").
		Return([]models.AdminSavedFilterPreset{{ID: "garbage", Name: "x"}}, int64(1), nil)

	req := savedFiltersRequest(t, http.MethodGet, "users", savedFiltersActingAdmin, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestReplaceAdminSavedFilters_Success asserts the stored and returned presets:
// names trimmed, a sent id kept, a missing id assigned, and version+1 echoed.
func TestReplaceAdminSavedFilters_Success(t *testing.T) {
	router, repo := newSavedFiltersRouter(t)
	keptID := uuid.NewString()

	var stored []models.AdminSavedFilterPreset
	repo.On("ReplaceAdminSavedFilters", mock.Anything, savedFiltersActingAdmin, "projects",
		mock.Anything, models.DefaultPreferences(), int64(4)).
		Run(func(args mock.Arguments) {
			stored = args.Get(3).([]models.AdminSavedFilterPreset)
		}).
		Return(int64(5), nil)

	body := map[string]any{
		"version": 4,
		"presets": []map[string]any{
			{"id": keptID, "name": "  Renamed  ", "query": map[string]string{"team_id": "abc"}},
			presetInput("New one"),
		},
	}
	req := savedFiltersRequest(t, http.MethodPut, "projects", savedFiltersActingAdmin, body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp admingen.AdminSavedFilters
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.EqualValues(t, 5, resp.Version)
	assert.Equal(t, admingen.Projects, resp.List)
	require.Len(t, resp.Presets, 2)
	assert.Equal(t, keptID, resp.Presets[0].Id.String())
	assert.Equal(t, "Renamed", resp.Presets[0].Name)
	assert.NotEqual(t, uuid.Nil, resp.Presets[1].Id)
	assert.Equal(t, "New one", resp.Presets[1].Name)

	require.Len(t, stored, 2)
	assert.Equal(t, resp.Presets[1].Id.String(), stored[1].ID, "the assigned id is what gets stored")
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestReplaceAdminSavedFilters_StaleVersionReturns409(t *testing.T) {
	router, repo := newSavedFiltersRouter(t)
	repo.On("ReplaceAdminSavedFilters", mock.Anything, savedFiltersActingAdmin, "users",
		mock.Anything, mock.Anything, int64(1)).
		Return(int64(0), repositories.ErrUserPreferencesVersionConflict)

	req := savedFiltersRequest(t, http.MethodPut, "users", savedFiltersActingAdmin,
		map[string]any{"version": 1, "presets": []any{presetInput("A")}})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code)
	assert.Contains(t, rr.Body.String(), adminSavedFiltersConflictDetail)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestReplaceAdminSavedFilters_RepoErrorReturns500(t *testing.T) {
	router, repo := newSavedFiltersRouter(t)
	repo.On("ReplaceAdminSavedFilters", mock.Anything, savedFiltersActingAdmin, "users",
		mock.Anything, mock.Anything, int64(0)).
		Return(int64(0), errors.New("db down"))

	req := savedFiltersRequest(t, http.MethodPut, "users", savedFiltersActingAdmin,
		map[string]any{"version": 0, "presets": []any{}})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestReplaceAdminSavedFilters_InvalidReturns400 covers every rejection the
// acceptance criteria name. The repository mock has no expectation, so any
// persist attempt fails the test: nothing is saved on a 400.
func TestReplaceAdminSavedFilters_InvalidReturns400(t *testing.T) {
	tooMany := make([]any, 0, 21)
	for i := range 21 {
		tooMany = append(tooMany, presetInput(fmt.Sprintf("Preset %d", i)))
	}
	dupID := uuid.NewString()
	bigQuery := map[string]string{}
	for i := range 41 {
		bigQuery[fmt.Sprintf("k%d", i)] = "v"
	}

	tests := []struct {
		name    string
		list    string
		body    any
		wantMsg string
	}{
		{"21 presets", "users", map[string]any{"version": 0, "presets": tooMany}, "too many presets"},
		{"names differ only by case and space", "users", map[string]any{"version": 0, "presets": []any{
			presetInput("Power users"), presetInput("  power USERS "),
		}}, "used more than once"},
		{"empty name", "users", map[string]any{"version": 0, "presets": []any{presetInput("   ")}}, "name must be"},
		{"81-rune name", "users", map[string]any{"version": 0, "presets": []any{
			presetInput(strings.Repeat("é", 81)),
		}}, "name must be"},
		{"bad query key", "users", map[string]any{"version": 0, "presets": []any{
			map[string]any{"name": "A", "query": map[string]string{"Bad-Key": "x"}},
		}}, "query key"},
		{"oversize query value", "users", map[string]any{"version": 0, "presets": []any{
			map[string]any{"name": "A", "query": map[string]string{"q": strings.Repeat("x", 513)}},
		}}, "exceeds 512"},
		{"too many query keys", "users", map[string]any{"version": 0, "presets": []any{
			map[string]any{"name": "A", "query": bigQuery},
		}}, "at most 40"},
		{"duplicate id", "users", map[string]any{"version": 0, "presets": []any{
			map[string]any{"id": dupID, "name": "A", "query": map[string]string{}},
			map[string]any{"id": dupID, "name": "B", "query": map[string]string{}},
		}}, "used more than once"},
		{"negative version", "users", map[string]any{"version": -1, "presets": []any{}}, "version"},
		{"unknown list", "widgets", map[string]any{"version": 0, "presets": []any{}}, "widgets"},
		{"unknown body field", "users", map[string]any{"version": 0, "presets": []any{}, "owner": "x"}, "owner"},
		{"malformed id", "users", map[string]any{"version": 0, "presets": []any{
			map[string]any{"id": "not-a-uuid", "name": "A", "query": map[string]string{}},
		}}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router, _ := newSavedFiltersRouter(t)

			req := savedFiltersRequest(t, http.MethodPut, tc.list, savedFiltersActingAdmin, tc.body)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			assert.Contains(t, rr.Body.String(), tc.wantMsg)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestAdminSavedFilters_PerCaller: each admin reads and writes only their own
// presets — the acting admin's id is what reaches the service, never a param.
func TestAdminSavedFilters_PerCaller(t *testing.T) {
	adminA, adminB := uuid.NewString(), uuid.NewString()
	presetA := models.AdminSavedFilterPreset{ID: uuid.NewString(), Name: "A's preset", Query: map[string]string{}}

	prefs := servicesmocks.NewMockUserPreferencesServiceInterface(t)
	prefs.On("GetAdminSavedFilters", mock.Anything, adminA, "users").
		Return([]models.AdminSavedFilterPreset{presetA}, int64(2), nil)
	prefs.On("GetAdminSavedFilters", mock.Anything, adminB, "users").
		Return([]models.AdminSavedFilterPreset{}, int64(0), nil)
	router := mountAdminStrictRouter(newAdminTestServer(&config.Config{}, &adminMockContainer{prefsService: prefs}))

	for _, tc := range []struct {
		admin string
		want  int
	}{{adminA, 1}, {adminB, 0}} {
		req := savedFiltersRequest(t, http.MethodGet, "users", tc.admin, nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		var resp admingen.AdminSavedFilters
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.Len(t, resp.Presets, tc.want)
	}
}

// TestAdminSavedFilterRoutes_NonAdminGets404 locks the non-advertisement
// contract through the full router: unauthenticated callers get 404 on both ops.
func TestAdminSavedFilterRoutes_NonAdminGets404(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
				adminService: servicesmocks.NewMockAdminServiceInterface(t),
				authService:  servicesmocks.NewMockAuthServiceInterface(t),
				prefsService: servicesmocks.NewMockUserPreferencesServiceInterface(t),
			})

			req := httptest.NewRequest(method, "/api/v1/admin/saved-filters/users",
				strings.NewReader(`{"version":0,"presets":[]}`))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			srv.router.ServeHTTP(rr, req)

			require.Equal(t, http.StatusNotFound, rr.Code)
		})
	}
}
