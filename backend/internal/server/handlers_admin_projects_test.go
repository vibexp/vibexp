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
	lastAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	populated := models.AdminProjectList{
		Projects: []models.AdminProjectListItem{
			{
				ID: uuid.NewString(), Name: "Platform", Slug: "platform",
				Team: team, Owner: owner, CreatedAt: time.Now(), UpdatedAt: time.Now(),
				ResourceCounts: models.AdminProjectResourceCounts{
					Prompts: 1, Artifacts: 2, Memories: 3, Blueprints: 4, FeedItems: 5, Total: 15,
				},
				LastResourceCreatedAt: &lastAt,
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
				// Distinct values per count, so a transposed mapping fails.
				assert.Equal(t, admingen.AdminProjectResourceCounts{
					Prompts: 1, Artifacts: 2, Memories: 3, Blueprints: 4, FeedItems: 5, Total: 15,
				}, resp.Projects[0].ResourceCounts)
				require.NotNil(t, resp.Projects[0].LastResourceCreatedAt)
				assert.True(t, lastAt.Equal(*resp.Projects[0].LastResourceCreatedAt))
				// A project with no resources carries an explicit null, not an
				// omitted field: the property is required.
				assert.Nil(t, resp.Projects[1].LastResourceCreatedAt)
				assert.Contains(t, rr.Body.String(), `"last_resource_created_at":null`)
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

	ownerEmail := "Creator@Example.com"
	later := stamp.Add(time.Hour)
	n := func(v int64) *int64 { return &v }

	want := repositories.AdminProjectFilters{
		Search:                  &search,
		TeamID:                  &teamIDStr,
		CreatedFrom:             &stamp,
		CreatedTo:               &stamp,
		OwnerEmail:              &ownerEmail,
		PromptCount:             repositories.AdminCountRange{Min: n(1), Max: n(2)},
		MemoryCount:             repositories.AdminCountRange{Min: n(3), Max: n(4)},
		ArtifactCount:           repositories.AdminCountRange{Min: n(5), Max: n(6)},
		BlueprintCount:          repositories.AdminCountRange{Min: n(7), Max: n(8)},
		FeedItemCount:           repositories.AdminCountRange{Min: n(9), Max: n(10)},
		TotalResourceCount:      repositories.AdminCountRange{Min: n(11), Max: n(12)},
		LastResourceCreatedFrom: &stamp,
		LastResourceCreatedTo:   &later,
		SortBy:                  "last_resource_created_at",
		SortOrder:               "asc",
		Page:                    2,
		Limit:                   50,
	}

	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("ListProjects", mock.Anything, want).Return(models.AdminProjectList{
		Projects: []models.AdminProjectListItem{}, Page: 2, PerPage: 50,
	}, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET",
		"/api/v1/admin/projects?page=2&limit=50&search=plat&team_id="+teamID.String()+
			"&created_from="+stamp.Format(time.RFC3339)+"&created_to="+stamp.Format(time.RFC3339)+
			"&owner_email=Creator@Example.com"+
			"&prompt_count_min=1&prompt_count_max=2&memory_count_min=3&memory_count_max=4"+
			"&artifact_count_min=5&artifact_count_max=6&blueprint_count_min=7&blueprint_count_max=8"+
			"&feed_item_count_min=9&feed_item_count_max=10"+
			"&total_resource_count_min=11&total_resource_count_max=12"+
			"&last_resource_created_from="+stamp.Format(time.RFC3339)+
			"&last_resource_created_to="+later.Format(time.RFC3339)+
			"&sort_by=last_resource_created_at&sort_order=asc", nil)
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

// TestListAdminProjects_InvalidAdvancedFiltersReturn400 pins the cross-field and
// format checks the generated binder does not perform (#1143): each must be a
// 400 before the service is reached, not a silently empty page.
func TestListAdminProjects_InvalidAdvancedFiltersReturn400(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"negative min", "prompt_count_min=-1"},
		{"negative max", "feed_item_count_max=-1"},
		{"min above max", "total_resource_count_min=5&total_resource_count_max=2"},
		{"inverted last_resource_created range", "last_resource_created_from=2026-09-02T00:00:00Z&last_resource_created_to=2026-09-01T00:00:00Z"},
		{"malformed owner_email", "owner_email=not-an-email"},
		{"owner_email with a display name", "owner_email=Creator+%3Ccreator%40example.com%3E"},
		{"unknown sort_by", "sort_by=comment_count"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// No service expectation: the request must be rejected first.
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

			req := httptest.NewRequest("GET", "/api/v1/admin/projects?"+tc.query, nil)
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestListAdminProjects_BlankOwnerEmailIsNoFilter pins that an empty or
// whitespace-only owner_email narrows nothing rather than 400ing.
func TestListAdminProjects_BlankOwnerEmailIsNoFilter(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("ListProjects", mock.Anything, repositories.AdminProjectFilters{}).
		Return(models.AdminProjectList{Projects: []models.AdminProjectListItem{}}, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/projects?owner_email=+", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
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

// TestGetAdminProject_Found asserts the detail payload, including the five
// resource counts plus total and the explicit absence of the excluded types.
func TestGetAdminProject_Found(t *testing.T) {
	id := uuid.NewString()
	detail := &models.AdminProjectDetail{
		ID: id, Name: "Platform", Slug: "platform",
		Description: "Core platform work",
		GitURL:      "https://github.com/acme/platform",
		Homepage:    "https://platform.acme.dev",
		Team:        adminProjectTeam(), Owner: adminProjectOwner(),
		ResourceCounts: models.AdminProjectResourceCounts{
			Prompts: 12, Artifacts: 4, Memories: 27, Blueprints: 3, FeedItems: 9, Total: 55,
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
	assert.Equal(t, int64(9), resp.ResourceCounts.FeedItems)
	assert.Equal(t, int64(55), resp.ResourceCounts.Total)

	// agents/feeds/comments/attachments are NOT part of the payload: none of
	// those tables carries a project_id.
	for _, key := range []string{`"agents"`, `"feeds"`, `"comments"`, `"attachments"`} {
		assert.NotContains(t, rr.Body.String(), key)
	}

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

// TestListAdminProjects_MalformedTeamIDReturns400 pins that a non-UUID team_id is
// rejected by the binder rather than silently becoming a filter that matches
// nothing — which would look like "this team has no projects".
func TestListAdminProjects_MalformedTeamIDReturns400(t *testing.T) {
	// No service expectation: the request must not reach it.
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("GET", "/api/v1/admin/projects?team_id=not-a-uuid", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}
