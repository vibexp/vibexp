package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// The memories read path converted to generated strict-server types (#779, epic
// #122). These are the spec-validated assertions the conversion exists to buy:
// every response body is checked against backend/openapi.yaml rather than
// trusted, which is the class of drift that crashed the SPA three times
// (#105 / #121 / #132).

const (
	memoriesTestUserID    = "test-user-123"
	memoriesTestProjectID = "550e8400-e29b-41d4-a716-446655440013"
	memoriesTestRuleID    = "550e8400-e29b-41d4-a716-446655440014"
	memoriesTestRelationD = "550e8400-e29b-41d4-a716-446655440015"
	memoriesTestSimilarID = "550e8400-e29b-41d4-a716-446655440016"
)

// memoriesTestNow is a fixed instant so the golden-body timestamp assertion
// below compares against something stable.
func memoriesTestNow() time.Time {
	return time.Date(2026, 3, 1, 12, 30, 45, 0, time.UTC)
}

func sampleStrictMemory() *models.Memory {
	now := memoriesTestNow()
	return &models.Memory{
		ID:        memoriesTestMemoryID,
		UserID:    memoriesTestUserID,
		TeamID:    memoriesTestTeamID,
		ProjectID: memoriesTestProjectID,
		Text:      "Remember the aware/naive timestamp split",
		Status:    models.MemoryStatusActive,
		Metadata:  map[string]interface{}{"category": "work"},
		CreatedAt: now,
		UpdatedAt: now,
		Version:   3,
	}
}

func getMemoryRequest(t *testing.T, srv *Server, memoryID string) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	req := makeMemoryAuthenticatedRequest(
		"GET", "/api/v1/"+memoriesTestTeamID+"/memories/"+memoryID, nil, memoriesTestUserID)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	return req, w
}

// The detail response must satisfy the spec.
func TestGetMemory_ConformsToSpec(t *testing.T) {
	container := newMockMemoryContainer(t)
	memory := sampleStrictMemory()
	container.memoryService.On("GetMemory", memoriesTestUserID, memoriesTestTeamID, memoriesTestMemoryID).
		Return(memory, nil)

	srv := createMemoryTestServer(container)
	req, w := getMemoryRequest(t, srv, memoriesTestMemoryID)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, memoriesTestMemoryID, body["id"])
	assert.Equal(t, memory.Text, body["text"])
	assert.EqualValues(t, 3, body["version"])
}

// related, similar and freshness carry `db:"-"` and are attached by the handler
// after the service call (#421, #735). All three are OPTIONAL in the schema, so
// a converter that silently dropped them would still produce a spec-valid body
// -- AssertConformsToSpec cannot see this, which is why the converter is
// asserted directly.
func TestToGenMemory_CarriesTheAttachedNeighborhood(t *testing.T) {
	memory := sampleStrictMemory()
	memory.Related = models.JSONArray[models.RelatedResource]{{
		RelationID:   memoriesTestRelationD,
		RelationType: "explained-by",
		Direction:    "outgoing",
		Origin:       "ai",
		Status:       "confirmed",
		ResourceType: "artifact",
		ResourceID:   memoriesTestSimilarID,
		Title:        "A related artifact",
		CreatedAt:    memoriesTestNow(),
	}}
	memory.Similar = models.JSONArray[models.SimilarResource]{{
		Type:  "artifact",
		ID:    memoriesTestSimilarID,
		Title: "A similar artifact",
		Score: 0.87,
	}}
	memory.Freshness = &models.ResourceFreshnessState{
		Status:         models.FreshnessStatusStale,
		Reason:         models.FreshnessReasonRuleRun,
		Since:          memoriesTestNow(),
		MatchedRuleIDs: models.JSONArray[string]{memoriesTestRuleID},
	}

	got, err := toGenMemory(memory)
	require.NoError(t, err)

	require.NotNil(t, got.Related)
	require.Len(t, *got.Related, 1)
	assert.Equal(t, memoriesTestRelationD, (*got.Related)[0].RelationId.String())
	assert.Equal(t, "A related artifact", (*got.Related)[0].Title)

	require.NotNil(t, got.Similar)
	require.Len(t, *got.Similar, 1)
	assert.Equal(t, memoriesTestSimilarID, (*got.Similar)[0].Id.String())
	assert.InDelta(t, 0.87, (*got.Similar)[0].Score, 0.0001)

	require.NotNil(t, got.Freshness)
	assert.Equal(t, models.FreshnessStatusStale, string(got.Freshness.Status))
	require.Len(t, got.Freshness.MatchedRuleIds, 1)
	assert.Equal(t, memoriesTestRuleID, got.Freshness.MatchedRuleIds[0].String())
}

// A neighborhood id the schema types as a UUID but the database does not hold
// as one is reported, not silently omitted: dropping the neighbor would hide a
// data problem behind a valid-looking response.
func TestToGenMemory_RejectsANonUUIDNeighborID(t *testing.T) {
	memory := sampleStrictMemory()
	memory.Similar = models.JSONArray[models.SimilarResource]{{
		Type: "artifact", ID: "not-a-uuid", Title: "Broken", Score: 0.5,
	}}

	_, err := toGenMemory(memory)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a UUID")
}

// memories.created_at / updated_at are `timestamp without time zone` -- one of
// only two naive columns among the four resource tables -- so the wire form is
// the thing to pin: a generated time.Time field must not introduce or drop the
// trailing Z relative to what the hand-marshaled response produced.
func TestGetMemory_TimestampWireFormatIsUnchanged(t *testing.T) {
	container := newMockMemoryContainer(t)
	container.memoryService.On("GetMemory", memoriesTestUserID, memoriesTestTeamID, memoriesTestMemoryID).
		Return(sampleStrictMemory(), nil)

	srv := createMemoryTestServer(container)
	_, w := getMemoryRequest(t, srv, memoriesTestMemoryID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	// The exact string the hand-marshaled models.Memory produced for this
	// instant, RFC3339 with a Z: encoding/json renders time.Time identically
	// either side of the conversion, and this is what says so.
	assert.Equal(t, "2026-03-01T12:30:45Z", body["created_at"])
	assert.Equal(t, "2026-03-01T12:30:45Z", body["updated_at"])
}

// A memory the team does not own is a 404, which the spec documents. Without
// the errors.As arm in memoriesResponseErrorHandler this is a 500.
func TestGetMemory_NotFoundConformsToSpec(t *testing.T) {
	container := newMockMemoryContainer(t)
	container.memoryService.On("GetMemory", memoriesTestUserID, memoriesTestTeamID, memoriesTestMissingID).
		Return(nil, repositories.ErrMemoryNotFound)

	srv := createMemoryTestServer(container)
	req, w := getMemoryRequest(t, srv, memoriesTestMissingID)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
}

// The required `memories` array must serialize as [] and never null. Generated
// types cannot use the models.JSONArray shim, so this is the assertion that
// stands in for it (MemoryListResponse is in adHocRequiredArrayAllowlist for
// exactly this reason).
func TestListMemories_EmptyPageSerializesAnArray(t *testing.T) {
	container := newMockMemoryContainer(t)
	container.memoryService.On("ListMemories", memoriesTestUserID, mock.Anything).
		Return(&models.MemoryListResponse{
			Memories: nil, // the shape a repository returns for an empty page
			Page:     1, PerPage: 20, TotalCount: 0, TotalPages: 0,
		}, nil)

	srv := createMemoryTestServer(container)
	req := makeMemoryAuthenticatedRequest(
		"GET", "/api/v1/"+memoriesTestTeamID+"/memories", nil, memoriesTestUserID)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	assert.Contains(t, w.Body.String(), `"memories":[]`,
		"a nil slice must marshal as [], not null")
}

// The list response as a whole must satisfy the spec, with a real page in it.
func TestListMemories_ConformsToSpec(t *testing.T) {
	container := newMockMemoryContainer(t)
	container.memoryService.On("ListMemories", memoriesTestUserID, mock.Anything).
		Return(&models.MemoryListResponse{
			Memories:   models.JSONArray[models.Memory]{*sampleStrictMemory()},
			Page:       1,
			PerPage:    20,
			TotalCount: 1,
			TotalPages: 1,
		}, nil)

	srv := createMemoryTestServer(container)
	req := makeMemoryAuthenticatedRequest(
		"GET", "/api/v1/"+memoriesTestTeamID+"/memories", nil, memoriesTestUserID)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.EqualValues(t, 1, body["total_count"])
	require.Len(t, body["memories"], 1)
}

// The binder types team_id and id as UUIDs, so a malformed one is a 400 naming
// the parameter rather than reaching the service. This is new behaviour: the
// chi handler passed any string straight through.
func TestMemoriesStrictBinder_RejectsNonUUIDPathParams(t *testing.T) {
	srv := createMemoryTestServer(newMockMemoryContainer(t))

	for _, path := range []string{
		"/api/v1/not-a-uuid/memories",
		"/api/v1/" + memoriesTestTeamID + "/memories/not-a-uuid",
	} {
		req := makeMemoryAuthenticatedRequest("GET", path, nil, memoriesTestUserID)
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, path)
		assert.Contains(t, w.Body.String(), "must be a valid UUID", path)
	}
}

// oapi-codegen binds query enums as string-typed named types WITHOUT validating
// their values, so these rejections have to stay in the handler. Losing them
// would send an unrecognized filter to the service, which answers with the full
// list -- a silently ignored filter that reads as a legitimate answer.
func TestListMemories_RejectsInvalidEnumQueryValues(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"freshness", "?freshness=stail", "freshness must be stale"},
		{"sort_by", "?sort_by=drop_table", "invalid sort_by value"},
		{"status", "?status=pending", "status must be one of"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The service mock has no ListMemories expectation: reaching it is
			// itself the failure this test exists to catch.
			srv := createMemoryTestServer(newMockMemoryContainer(t))
			req := makeMemoryAuthenticatedRequest(
				"GET", "/api/v1/"+memoriesTestTeamID+"/memories"+tt.query, nil, memoriesTestUserID)
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, req)

			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), tt.want)
		})
	}
}

// Every filter must actually reach the service. mock.Anything on the filters
// argument makes this untestable, which is how a dropped filter ships: the
// endpoint answers 200 with the FULL list, which reads as a legitimate answer.
// Proven by mutation — hard-coding any of these to its zero value fails here
// and nowhere else in the package.
func TestListMemories_EveryFilterReachesTheService(t *testing.T) {
	container := newMockMemoryContainer(t)
	container.memoryService.On("ListMemories", memoriesTestUserID,
		mock.MatchedBy(func(f services.MemoryFilters) bool {
			return f.TeamID == memoriesTestTeamID &&
				f.Search == "vector" &&
				f.ProjectID != nil && *f.ProjectID == memoriesTestProjectID &&
				f.Status != nil && *f.Status == models.MemoryStatusDraft &&
				f.Freshness == services.FreshnessFilterStale &&
				f.SortBy == "created_at" &&
				f.SortOrder == "asc" &&
				f.Page == 2 && f.Limit == 5 &&
				len(f.MetadataFilter) == 1
		}),
	).Return(&models.MemoryListResponse{Page: 2, PerPage: 5}, nil)

	srv := createMemoryTestServer(container)
	req := makeMemoryAuthenticatedRequest("GET",
		"/api/v1/"+memoriesTestTeamID+"/memories"+
			"?search=vector&project_id="+memoriesTestProjectID+
			"&status=draft&freshness=stale&sort_by=created_at&sort_order=ASC"+
			"&page=2&limit=5&metadata="+url.QueryEscape(`{"env":["prod"]}`),
		nil, memoriesTestUserID)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	container.memoryService.AssertExpectations(t)
}

// An empty query value meant "no filter" to the chi parser this replaced, and
// still does. Without dropEmptyQueryValues the generated binder treats `?status=`
// as a present-but-empty parameter and the request 400s — breaking the ordinary
// "clear the filter" idiom on the list endpoint.
func TestListMemories_EmptyQueryValuesAreIgnoredNotRejected(t *testing.T) {
	container := newMockMemoryContainer(t)
	container.memoryService.On("ListMemories", memoriesTestUserID,
		mock.MatchedBy(func(f services.MemoryFilters) bool {
			// Every filter unset, and the pagination defaults applied rather
			// than an empty page= being parsed as zero.
			return f.Status == nil && f.ProjectID == nil && f.Freshness == "" &&
				f.SortBy == "" && f.Search == "" && f.Page == 1
		}),
	).Return(&models.MemoryListResponse{Page: 1, PerPage: 20}, nil)

	srv := createMemoryTestServer(container)
	req := makeMemoryAuthenticatedRequest("GET",
		"/api/v1/"+memoriesTestTeamID+"/memories"+
			"?status=&freshness=&sort_by=&project_id=&page=&search=",
		nil, memoriesTestUserID)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	container.memoryService.AssertExpectations(t)
}

// The three optional keys the hand-marshaled body always carried must still be
// present when empty. All three are OPTIONAL in the schema, so omitting them
// stays spec-valid and AssertConformsToSpec cannot see the difference — this is
// the only thing standing between the conversion and a silent wire change.
func TestGetMemory_EmptyOptionalKeysStayPresent(t *testing.T) {
	container := newMockMemoryContainer(t)
	memory := sampleStrictMemory()
	memory.Metadata = nil // the shape a memory with no metadata has
	container.memoryService.On("GetMemory", memoriesTestUserID, memoriesTestTeamID, memoriesTestMemoryID).
		Return(memory, nil)

	srv := createMemoryTestServer(container)
	_, w := getMemoryRequest(t, srv, memoriesTestMemoryID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	body := w.Body.String()
	assert.Contains(t, body, `"metadata":null`, "a nil metadata map serialized as null before the conversion")
	assert.Contains(t, body, `"related":[]`, "an empty neighborhood serialized as [] before the conversion")
	assert.Contains(t, body, `"similar":[]`)
}
