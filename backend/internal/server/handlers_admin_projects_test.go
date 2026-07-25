package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

func adminProjectTeam() models.AdminProjectTeam {
	return models.AdminProjectTeam{ID: uuid.NewString(), Name: "Acme Engineering", Slug: "acme-engineering"}
}

func adminProjectOwner() models.AdminTeamOwner {
	return models.AdminTeamOwner{ID: uuid.NewString(), Email: "creator@example.com", Name: "Creator"}
}

// TestListAdminProjects verifies the page shape, that an empty page serializes as
// [], and spec conformance.
func TestListAdminProjects(t *testing.T) {
	team, owner := adminProjectTeam(), adminProjectOwner()
	populated := models.AdminProjectList{
		Projects: []models.AdminProjectListItem{
			{
				ID: uuid.NewString(), Name: "Platform", Slug: "platform",
				Team: team, Owner: owner, CreatedAt: time.Now(), UpdatedAt: time.Now(),
			},
			{
				ID: uuid.NewString(), Name: "Website", Slug: "website",
				Team: team, Owner: owner, CreatedAt: time.Now(), UpdatedAt: time.Now(),
			},
		},
		TotalCount: 2, Page: 1, PerPage: 20, TotalPages: 1,
	}
	empty := models.AdminProjectList{
		Projects: []models.AdminProjectListItem{}, TotalCount: 0, Page: 1, PerPage: 20, TotalPages: 0,
	}

	tests := []struct {
		name         string
		list         models.AdminProjectList
		wantProjects int
	}{
		{"populated page", populated, 2},
		{"empty page serializes as []", empty, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			mockAdmin.On("ListProjects", mock.Anything, repositories.AdminProjectFilters{}).Return(tc.list, nil)
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

			req := httptest.NewRequest("GET", "/api/v1/admin/projects", nil)
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			var resp admingen.AdminProjectListResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			assert.Len(t, resp.Projects, tc.wantProjects)
			assert.NotContains(t, rr.Body.String(), `"projects":null`)

			// Value-assert the joined team and owner: a dropped mapping would
			// serialize as zero values and still satisfy the schema.
			if tc.wantProjects > 0 {
				assert.Equal(t, "Platform", resp.Projects[0].Name)
				assert.Equal(t, "platform", resp.Projects[0].Slug)
				assert.Equal(t, "Acme Engineering", resp.Projects[0].Team.Name)
				assert.Equal(t, "acme-engineering", resp.Projects[0].Team.Slug)
				assert.Equal(t, "creator@example.com", string(resp.Projects[0].Owner.Email))
			}

			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestListAdminProjects_MapsQueryParams asserts every documented filter reaches
// the service as the matching field.
func TestListAdminProjects_MapsQueryParams(t *testing.T) {
	teamID := uuid.New()
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	search := "plat"
	teamIDStr := teamID.String()

	want := repositories.AdminProjectFilters{
		Search:      &search,
		TeamID:      &teamIDStr,
		CreatedFrom: &stamp,
		CreatedTo:   &stamp,
		SortBy:      "name",
		SortOrder:   "asc",
		Page:        2,
		Limit:       50,
	}

	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("ListProjects", mock.Anything, want).Return(models.AdminProjectList{
		Projects: []models.AdminProjectListItem{}, Page: 2, PerPage: 50,
	}, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET",
		"/api/v1/admin/projects?page=2&limit=50&search=plat&team_id="+teamID.String()+
			"&created_from="+stamp.Format(time.RFC3339)+"&created_to="+stamp.Format(time.RFC3339)+
			"&sort_by=name&sort_order=asc", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestListAdminProjects_InvalidSortEnumReturns400 pins the enum rejection the
// generated binder does not perform.
func TestListAdminProjects_InvalidSortEnumReturns400(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"unknown sort_by", "/api/v1/admin/projects?sort_by=owner"},
		{"unknown sort_order", "/api/v1/admin/projects?sort_order=sideways"},
		{"injection-shaped sort_by", "/api/v1/admin/projects?sort_by=id%3B+DROP+TABLE+projects--"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// No service expectation: the request must be rejected first.
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

			req := httptest.NewRequest("GET", tc.path, nil)
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

func TestListAdminProjects_ServiceErrorReturns500(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("ListProjects", mock.Anything, mock.Anything).
		Return(models.AdminProjectList{}, errors.New("db down"))
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/projects", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestListAdminProjects_ConversionErrorReturns500 covers a non-UUID id from the
// store.
func TestListAdminProjects_ConversionErrorReturns500(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("ListProjects", mock.Anything, mock.Anything).Return(models.AdminProjectList{
		Projects: []models.AdminProjectListItem{{ID: "not-a-uuid", Team: adminProjectTeam(), Owner: adminProjectOwner()}},
	}, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/projects", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestGetAdminProject_Found asserts the detail payload, including the four
// resource counts and the explicit absence of agents/feeds.
func TestGetAdminProject_Found(t *testing.T) {
	id := uuid.NewString()
	detail := &models.AdminProjectDetail{
		ID: id, Name: "Platform", Slug: "platform",
		Description: "Core platform work",
		GitURL:      "https://github.com/acme/platform",
		Homepage:    "https://platform.acme.dev",
		Team:        adminProjectTeam(), Owner: adminProjectOwner(),
		ResourceCounts: models.AdminProjectResourceCounts{
			Prompts: 12, Artifacts: 4, Memories: 27, Blueprints: 3,
		},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetProjectDetail", mock.Anything, id).Return(detail, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/projects/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp admingen.AdminProjectDetail
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, id, resp.Id.String())
	assert.Equal(t, "Core platform work", resp.Description)
	assert.Equal(t, "https://github.com/acme/platform", resp.GitUrl)
	assert.Equal(t, "https://platform.acme.dev", resp.Homepage)

	// Each count mapped individually — transposing two would still satisfy the
	// schema, so distinct values are asserted per field.
	assert.Equal(t, int64(12), resp.ResourceCounts.Prompts)
	assert.Equal(t, int64(4), resp.ResourceCounts.Artifacts)
	assert.Equal(t, int64(27), resp.ResourceCounts.Memories)
	assert.Equal(t, int64(3), resp.ResourceCounts.Blueprints)

	// agents/feeds are NOT part of the payload: neither table is project-scoped.
	assert.NotContains(t, rr.Body.String(), `"agents"`)
	assert.NotContains(t, rr.Body.String(), `"feeds"`)

	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestGetAdminProject_NotFound(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetProjectDetail", mock.Anything, id).Return(nil, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/projects/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestGetAdminProject_InvalidUUIDReturns400(t *testing.T) {
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
		adminService: servicesmocks.NewMockAdminServiceInterface(t),
	})

	req := httptest.NewRequest("GET", "/api/v1/admin/projects/not-a-uuid", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestGetAdminProject_ServiceErrorReturns500(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetProjectDetail", mock.Anything, id).Return(nil, errors.New("db down"))
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/projects/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestGetAdminProject_ConversionErrorReturns500 covers a non-UUID id from the
// store on the detail path.
func TestGetAdminProject_ConversionErrorReturns500(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetProjectDetail", mock.Anything, id).Return(&models.AdminProjectDetail{
		ID: "not-a-uuid", Team: adminProjectTeam(), Owner: adminProjectOwner(),
	}, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/projects/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestAdminProjectRoutes_NonAdminGets404 locks the non-advertisement contract for
// the two new operations.
func TestAdminProjectRoutes_NonAdminGets404(t *testing.T) {
	for _, path := range []string{
		"/api/v1/admin/projects",
		"/api/v1/admin/projects/" + uuid.NewString(),
	} {
		t.Run(path, func(t *testing.T) {
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
				adminService: servicesmocks.NewMockAdminServiceInterface(t),
				authService:  servicesmocks.NewMockAuthServiceInterface(t),
			})

			req := httptest.NewRequest("GET", path, nil)
			rr := httptest.NewRecorder()
			srv.router.ServeHTTP(rr, req)

			require.Equal(t, http.StatusNotFound, rr.Code)
		})
	}
}
