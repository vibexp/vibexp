package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// The artifacts read path converted to generated strict-server types (#776,
// epic #122), following the memories conversion (#779). These are the
// spec-validated assertions the conversion exists to buy: every response body
// is checked against backend/openapi.yaml rather than trusted, which is the
// class of drift that crashed the SPA three times (#105 / #121 / #132).

const (
	strictArtTeamID  = "550e8400-e29b-41d4-a716-446655440020"
	strictArtProject = "550e8400-e29b-41d4-a716-446655440021"
	strictArtUserID  = "user-123"
	strictArtSlug    = "test-slug"
)

func strictArtifactServer(t *testing.T, svc services.ArtifactServiceInterface) *Server {
	t.Helper()
	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
	srv.container = &MockArtifactContainer{ArtifactServiceMock: svc}
	return mountArtifactReadRoutes(srv)
}

func sampleStrictArtifact() *models.Artifact {
	now := time.Date(2026, 3, 1, 12, 30, 45, 0, time.UTC)
	return &models.Artifact{
		ID:        "art-1",
		ProjectID: strictArtProject,
		Slug:      strictArtSlug,
		UserID:    strictArtUserID,
		Title:     "Test Artifact",
		Type:      "general",
		Status:    "active",
		Metadata:  map[string]interface{}{"key": "value"},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func strictArtifactRequest(t *testing.T, srv *Server, path string) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	req := createAuthenticatedRequest("GET", path, "", strictArtUserID)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	return req, w
}

func TestStrictGetArtifact_ConformsToSpec(t *testing.T) {
	svc := servicesmocks.NewMockArtifactServiceInterface(t)
	svc.On("GetArtifactByProjectIDAndSlugInTeam",
		strictArtUserID, strictArtTeamID, strictArtProject, strictArtSlug).
		Return(sampleStrictArtifact(), nil)

	srv := strictArtifactServer(t, svc)
	req, w := strictArtifactRequest(t, srv,
		"/api/v1/"+strictArtTeamID+"/artifacts/"+strictArtProject+"/"+strictArtSlug)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "art-1", body["id"])
	assert.Equal(t, strictArtProject, body["project_id"])
}

// The three optional keys the hand-marshaled body always carried must still be
// present when empty. They are OPTIONAL in the schema, so omitting them stays
// spec-valid and AssertConformsToSpec cannot see the difference — this is the
// only thing standing between the conversion and a silent wire change (#779).
func TestStrictGetArtifact_EmptyOptionalKeysStayPresent(t *testing.T) {
	artifact := sampleStrictArtifact()
	artifact.Metadata = nil
	artifact.Description = ""

	svc := servicesmocks.NewMockArtifactServiceInterface(t)
	svc.On("GetArtifactByProjectIDAndSlugInTeam",
		strictArtUserID, strictArtTeamID, strictArtProject, strictArtSlug).
		Return(artifact, nil)

	srv := strictArtifactServer(t, svc)
	_, w := strictArtifactRequest(t, srv,
		"/api/v1/"+strictArtTeamID+"/artifacts/"+strictArtProject+"/"+strictArtSlug)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	body := w.Body.String()
	assert.Contains(t, body, `"metadata":null`, "a nil metadata map serialized as null before the conversion")
	assert.Contains(t, body, `"description":""`, "description carried no omitempty on models.Artifact")
	assert.Contains(t, body, `"related":[]`, "an empty neighborhood serialized as [] before the conversion")
	assert.Contains(t, body, `"similar":[]`)
	// content DOES carry omitempty, so an empty one must stay absent -- emitting
	// it would ADD a key that was never there.
	assert.NotContains(t, body, `"content"`)
}

// The required `artifacts` array must serialize as [] and never null.
func TestStrictListArtifacts_EmptyPageSerializesAnArray(t *testing.T) {
	svc := servicesmocks.NewMockArtifactServiceInterface(t)
	svc.On("ListArtifacts", strictArtUserID, mock.Anything).
		Return(&models.ArtifactListResponse{
			Artifacts: nil, // the shape a repository returns for an empty page
			Page:      1, PerPage: 20, TotalCount: 0, TotalPages: 0,
		}, nil)

	srv := strictArtifactServer(t, svc)
	req, w := strictArtifactRequest(t, srv, "/api/v1/"+strictArtTeamID+"/artifacts")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	assert.Contains(t, w.Body.String(), `"artifacts":[]`, "a nil slice must marshal as [], not null")
}

func TestStrictListArtifacts_ConformsToSpec(t *testing.T) {
	svc := servicesmocks.NewMockArtifactServiceInterface(t)
	svc.On("ListArtifacts", strictArtUserID, mock.Anything).
		Return(&models.ArtifactListResponse{
			Artifacts:  models.JSONArray[models.Artifact]{*sampleStrictArtifact()},
			Page:       1,
			PerPage:    20,
			TotalCount: 1,
			TotalPages: 1,
		}, nil)

	srv := strictArtifactServer(t, svc)
	req, w := strictArtifactRequest(t, srv, "/api/v1/"+strictArtTeamID+"/artifacts")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.EqualValues(t, 1, body["total_count"])
	require.Len(t, body["artifacts"], 1)
}

// The by-project operation is the same listing narrowed by a PATH parameter --
// it must reach the service with that project, not the query one.
func TestStrictListArtifactsByProject_ConformsAndFiltersByPath(t *testing.T) {
	svc := servicesmocks.NewMockArtifactServiceInterface(t)
	svc.On("ListArtifacts", strictArtUserID,
		mock.MatchedBy(func(f services.ArtifactFilters) bool {
			return f.ProjectID == strictArtProject && f.TeamID == strictArtTeamID
		}),
	).Return(&models.ArtifactListResponse{Page: 1, PerPage: 20}, nil)

	srv := strictArtifactServer(t, svc)
	req, w := strictArtifactRequest(t, srv,
		"/api/v1/"+strictArtTeamID+"/artifacts/"+strictArtProject)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	svc.AssertExpectations(t)
}

// Every filter must actually reach the service. mock.Anything on the filters
// argument makes this untestable, which is how a dropped filter ships: the
// endpoint answers 200 with the FULL list, which reads as a legitimate answer
// (#779).
func TestStrictListArtifacts_EveryFilterReachesTheService(t *testing.T) {
	svc := servicesmocks.NewMockArtifactServiceInterface(t)
	svc.On("ListArtifacts", strictArtUserID,
		mock.MatchedBy(func(f services.ArtifactFilters) bool {
			return f.TeamID == strictArtTeamID &&
				f.ProjectID == strictArtProject &&
				f.Status == "active" &&
				f.Type == "general" &&
				f.Search == "vector" &&
				f.Freshness == services.FreshnessFilterStale &&
				f.SortBy == "created_at" &&
				f.SortOrder == "asc" &&
				f.Page == 2 && f.Limit == 5 &&
				len(f.MetadataFilter) == 1
		}),
	).Return(&models.ArtifactListResponse{Page: 2, PerPage: 5}, nil)

	srv := strictArtifactServer(t, svc)
	_, w := strictArtifactRequest(t, srv,
		"/api/v1/"+strictArtTeamID+"/artifacts"+
			"?project_id="+strictArtProject+"&status=active&type=general&search=vector"+
			"&freshness=stale&sort_by=created_at&sort_order=asc&page=2&limit=5"+
			"&metadata="+url.QueryEscape(`{"env":["prod"]}`))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	svc.AssertExpectations(t)
}

// An empty query value meant "no filter" to the chi parser this replaced, and
// still does — dropEmptyQueryValues strips them ahead of the binder, which
// would otherwise treat `?status=` as a present-but-empty parameter (#779).
func TestStrictListArtifacts_EmptyQueryValuesAreIgnoredNotRejected(t *testing.T) {
	svc := servicesmocks.NewMockArtifactServiceInterface(t)
	svc.On("ListArtifacts", strictArtUserID,
		mock.MatchedBy(func(f services.ArtifactFilters) bool {
			return f.Status == "" && f.ProjectID == "" && f.Freshness == "" &&
				f.Type == "" && f.Search == "" && f.Page == 1
		}),
	).Return(&models.ArtifactListResponse{Page: 1, PerPage: 20}, nil)

	srv := strictArtifactServer(t, svc)
	_, w := strictArtifactRequest(t, srv,
		"/api/v1/"+strictArtTeamID+"/artifacts?status=&project_id=&type=&search=&page=&freshness=")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	svc.AssertExpectations(t)
}

// freshness is the one enum the handler still rejects itself: oapi-codegen
// binds enums without validating their values, and an ignored freshness filter
// returns the full list, which reads as a legitimate answer.
func TestStrictListArtifacts_RejectsUnknownFreshnessValue(t *testing.T) {
	// The service mock has no ListArtifacts expectation: reaching it is itself
	// the failure this test exists to catch.
	srv := strictArtifactServer(t, servicesmocks.NewMockArtifactServiceInterface(t))
	_, w := strictArtifactRequest(t, srv, "/api/v1/"+strictArtTeamID+"/artifacts?freshness=stail")

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "freshness must be stale")
}

// The binder types team_id and project_id as UUIDs, so a malformed one is a 400
// naming the parameter rather than reaching the service. The slug is a plain
// string and must NOT be rejected.
func TestStrictArtifactsBinder_RejectsNonUUIDPathParams(t *testing.T) {
	srv := strictArtifactServer(t, servicesmocks.NewMockArtifactServiceInterface(t))

	for _, path := range []string{
		"/api/v1/not-a-uuid/artifacts",
		"/api/v1/" + strictArtTeamID + "/artifacts/not-a-uuid",
		"/api/v1/" + strictArtTeamID + "/artifacts/not-a-uuid/some-slug",
	} {
		_, w := strictArtifactRequest(t, srv, path)
		assert.Equal(t, http.StatusBadRequest, w.Code, path)
		assert.Contains(t, w.Body.String(), "must be a valid UUID", path)
	}
}

// chi hands back the still-encoded path segment whenever the request path
// contains percent-encoding (#251/#257), and the generated binder does not
// decode it either — so the handler must, or an exact-match lookup misses on
// every slug carrying an encoded character.
func TestStrictGetArtifact_DecodesAPercentEncodedSlug(t *testing.T) {
	svc := servicesmocks.NewMockArtifactServiceInterface(t)
	svc.On("GetArtifactByProjectIDAndSlugInTeam",
		strictArtUserID, strictArtTeamID, strictArtProject, "a b/c").
		Return(sampleStrictArtifact(), nil)

	srv := strictArtifactServer(t, svc)
	_, w := strictArtifactRequest(t, srv,
		"/api/v1/"+strictArtTeamID+"/artifacts/"+strictArtProject+"/a%20b%2Fc")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	svc.AssertExpectations(t)
}
