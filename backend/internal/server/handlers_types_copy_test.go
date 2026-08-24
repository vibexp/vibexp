package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	testTypesSourceTeamID = "880e8400-e29b-41d4-a716-446655440003"
	testTypesCopyPath     = "/api/v1/" + testTypesTeamID + "/settings/types/copy"
)

func copyTypesBody(sourceTeamID string) string {
	return `{"source_team_id":"` + sourceTeamID + `"}`
}

func TestCopyTypesFromTeam_Success(t *testing.T) {
	svc := servicesmocks.NewMockTypeServiceInterface(t)
	svc.EXPECT().CopyFromTeam(mock.Anything, services.CopyTypesParams{
		TeamID:       testTypesTeamID,
		SourceTeamID: testTypesSourceTeamID,
		UserID:       testTypesUserID,
	}).Return(&services.CopyTypesResult{
		Added: []models.Type{{
			ID:           "990e8400-e29b-41d4-a716-446655440004",
			TeamID:       testTypesTeamID,
			ResourceType: "artifacts",
			Slug:         "bug-report",
			Name:         "Bug report",
			CreatedBy:    testTypesUserID,
			CreatedAt:    time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC),
		}},
		Skipped: []services.SkippedType{{ResourceType: "artifacts", Slug: "runbook"}},
	}, nil)

	srv := createTestTypesServer(svc)
	req := makeTypesRequest("POST", testTypesCopyPath, copyTypesBody(testTypesSourceTeamID))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.EqualValues(t, 1, resp["added_count"])
	assert.EqualValues(t, 1, resp["skipped_count"])

	added := resp["added"].([]interface{})
	require.Len(t, added, 1)
	first := added[0].(map[string]interface{})
	assert.Equal(t, "bug-report", first["slug"])
	// The created row belongs to the DESTINATION team.
	assert.Equal(t, testTypesTeamID, first["team_id"])

	skipped := resp["skipped"].([]interface{})
	require.Len(t, skipped, 1)
	assert.Equal(t, "runbook", skipped[0].(map[string]interface{})["slug"])
	assert.Equal(t, "artifacts", skipped[0].(map[string]interface{})["resource_type"])
}

// TestCopyTypesFromTeam_EmptyResultSerializesArrays is the wire half of issue
// #125 for this response: the generated strict-server type cannot use the
// models.JSONArray shim, so `added` and `skipped` are guaranteed non-null by
// the handler's make(...,0) construction instead — and this is what proves it.
func TestCopyTypesFromTeam_EmptyResultSerializesArrays(t *testing.T) {
	svc := servicesmocks.NewMockTypeServiceInterface(t)
	svc.EXPECT().CopyFromTeam(mock.Anything, mock.Anything).
		Return(&services.CopyTypesResult{}, nil)

	srv := createTestTypesServer(svc)
	req := makeTypesRequest("POST", testTypesCopyPath, copyTypesBody(testTypesSourceTeamID))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"added":[]`)
	assert.Contains(t, w.Body.String(), `"skipped":[]`)
	assert.NotContains(t, w.Body.String(), "null")
}

// TestCopyTypesFromTeam_ForbiddenIsIdenticalForEitherTeam is the API-level half
// of the no-team-existence-leak requirement: the service returns one sentinel
// for either side, and the handler must not widen it back out.
func TestCopyTypesFromTeam_ForbiddenIsIdenticalForEitherTeam(t *testing.T) {
	details := make([]string, 0, 2)
	for _, denied := range []string{testTypesTeamID, testTypesSourceTeamID} {
		svc := servicesmocks.NewMockTypeServiceInterface(t)
		// The service already collapses the two cases; wrapping the sentinel
		// with the denied team here proves the handler does not echo it.
		svc.EXPECT().CopyFromTeam(mock.Anything, mock.Anything).
			Return(nil, wrapDenied(denied))

		srv := createTestTypesServer(svc)
		req := makeTypesRequest("POST", testTypesCopyPath, copyTypesBody(testTypesSourceTeamID))
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		specconformance.AssertConformsToSpec(t, req, w)

		assertTypesProblem(t, w, http.StatusForbidden, apierrors.CodeForbidden)
		// The SOURCE team is the one a caller could probe for: it is the only
		// id they can vary freely, and the destination is already in their own
		// request URL (which `instance` echoes back).
		assert.NotContains(t, w.Body.String(), testTypesSourceTeamID,
			"the response must not confirm anything about the source team")

		var problem struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
		details = append(details, problem.Title+"|"+problem.Detail)
	}

	require.Len(t, details, 2)
	assert.Equal(t, details[0], details[1],
		"a non-member of the source and of the destination are indistinguishable")
}

// wrapDenied builds a denial that names a team, as an unwary service change
// might, so the handler's collapse to one message is what the test observes.
func wrapDenied(teamID string) error {
	return errors.Join(services.ErrPermissionDenied, errors.New("denied on "+teamID))
}

func TestCopyTypesFromTeam_BadSourceIsBadRequest(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"missing source", services.ErrCopySourceRequired},
		{"source is destination", services.ErrCopySourceIsDestination},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := servicesmocks.NewMockTypeServiceInterface(t)
			svc.EXPECT().CopyFromTeam(mock.Anything, mock.Anything).Return(nil, tc.err)

			srv := createTestTypesServer(svc)
			req := makeTypesRequest("POST", testTypesCopyPath, copyTypesBody(testTypesTeamID))
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			specconformance.AssertConformsToSpec(t, req, w)

			assertTypesProblem(t, w, http.StatusBadRequest, apierrors.CodeBadRequest)
		})
	}
}

// TestCopyTypesFromTeam_MissingSourceTeamID pins the gap oapi-codegen leaves:
// it enforces neither `required` nor additionalProperties on a request body, so
// `{}` binds source_team_id as the ZERO uuid instead of failing. Without the
// handler's uuid.Nil guard this reaches the service and answers 403 — the
// status the spec reserves for a team the caller is not in, which would imply
// the all-zero team exists.
func TestCopyTypesFromTeam_MissingSourceTeamID(t *testing.T) {
	// No EXPECT: reaching the service at all is the bug.
	srv := createTestTypesServer(servicesmocks.NewMockTypeServiceInterface(t))
	req := makeTypesRequest("POST", testTypesCopyPath, `{}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assertTypesProblem(t, w, http.StatusBadRequest, apierrors.CodeBadRequest)
}

func TestCopyTypesFromTeam_MalformedSourceTeamID(t *testing.T) {
	// No service call: the generated binder rejects a non-UUID body field.
	srv := createTestTypesServer(servicesmocks.NewMockTypeServiceInterface(t))
	req := makeTypesRequest("POST", testTypesCopyPath, `{"source_team_id":"not-a-uuid"}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assertTypesProblem(t, w, http.StatusBadRequest, apierrors.CodeBadRequest)
}

func TestCopyTypesFromTeam_InternalError(t *testing.T) {
	svc := servicesmocks.NewMockTypeServiceInterface(t)
	svc.EXPECT().CopyFromTeam(mock.Anything, mock.Anything).
		Return(nil, errors.New("audit storage down"))

	srv := createTestTypesServer(svc)
	req := makeTypesRequest("POST", testTypesCopyPath, copyTypesBody(testTypesSourceTeamID))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assertTypesProblem(t, w, http.StatusInternalServerError, apierrors.CodeInternalError)
	assert.NotContains(t, w.Body.String(), "audit storage down",
		"the internal detail stays in the log")
}
