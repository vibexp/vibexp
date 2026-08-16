package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// The prompts read path converted to generated strict-server types (#777, epic
// #122), the last of the four read-domain conversions after memories (#779),
// artifacts (#776) and blueprints (#778). These are the spec-validated
// assertions the conversion exists to buy: every response body is checked
// against backend/openapi.yaml rather than trusted, which is the class of drift
// that crashed the SPA three times (#105 / #121 / #132).

const (
	strictPrTeamID  = "550e8400-e29b-41d4-a716-446655440000"
	strictPrProject = "550e8400-e29b-41d4-a716-446655440041"
	strictPrUserID  = "user-123"
	strictPrSlug    = "test-prompt"
)

// strictPromptServer wires the real route tree (createTestServer mounts
// mountPromptsHandlers) with the team-membership check satisfied.
func strictPromptServer(t *testing.T) (*Server, *MockPromptContainer) {
	t.Helper()
	container := newMockPromptContainer(t)
	container.teamService.On("IsUserMemberOfTeam", mock.Anything, strictPrUserID, strictPrTeamID).
		Return(true, nil).Maybe()
	return createTestServer(container), container
}

func strictPromptRequest(t *testing.T, srv *Server, path string) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	req := makeAuthenticatedRequest("GET", path, nil, strictPrUserID)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	return req, w
}

// samplePrompt mirrors what the repository returns: `id` and `user_id` are plain
// strings in the schema (unlike team_id/project_id), so they are deliberately
// NOT UUIDs here. Labels is nil on purpose — see the null test below.
func samplePrompt() *models.Prompt {
	now := time.Date(2026, 3, 1, 12, 30, 45, 0, time.UTC)
	return &models.Prompt{
		ID:          "prompt-1",
		Name:        "Test Prompt",
		Slug:        strictPrSlug,
		Description: "A test prompt",
		Body:        "This is a test prompt body",
		UserID:      strictPrUserID,
		TeamID:      strictPrTeamID,
		ProjectID:   strictPrProject,
		Status:      "published",
		MCPExpose:   true,
		Version:     3,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// fullyPopulatedPrompt sets every field the converters can touch, including the
// three `db:"-"` neighborhood fields. The sparse fixture above exercises the
// nil/absent half; this one is what makes the populated half of each branch
// visible — without it a whole converter can be deleted with the suite green
// (the defect the #778 review found).
func fullyPopulatedPrompt() *models.Prompt {
	now := time.Date(2026, 3, 1, 12, 30, 45, 0, time.UTC)
	relProject := "550e8400-e29b-41d4-a716-446655440042"
	relSlug := "neighbor-prompt"

	p := samplePrompt()
	p.IsShared = true
	p.Labels = []string{"code-review", "documentation"}
	p.Related = models.JSONArray[models.RelatedResource]{{
		RelationID:   "550e8400-e29b-41d4-a716-446655440043",
		RelationType: "governed-by",
		Direction:    "outgoing",
		Origin:       "human",
		Status:       "confirmed",
		ResourceType: "prompt",
		ResourceID:   "550e8400-e29b-41d4-a716-446655440044",
		Title:        "Neighbor Prompt",
		ProjectID:    &relProject,
		Slug:         &relSlug,
		CreatedAt:    now,
	}}
	p.Similar = models.JSONArray[models.SimilarResource]{{
		Type:  "prompt",
		ID:    "550e8400-e29b-41d4-a716-446655440045",
		Title: "Similar Prompt",
		Score: 0.91,
	}}
	p.Freshness = &models.ResourceFreshnessState{
		Status:         "stale",
		Reason:         "rule_run",
		Since:          now,
		MatchedRuleIDs: models.JSONArray[string]{"550e8400-e29b-41d4-a716-446655440046"},
	}
	return p
}

// promptToMap renders a value the way the wire does, so two representations of
// the same prompt can be compared key by key rather than field by field.
func promptToMap(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

// The conversion's whole criterion. Unlike artifacts and blueprints, the
// pre-flight check (#800 item 5) found NO undeclared model field here, so the
// generated body must equal the hand-marshaled one with nothing removed —
// asserted by comparing rendered maps, because an assertion list can only check
// the fields someone remembered to list.
func TestToGenPrompt_BodyMatchesTheHandMarshaledModel(t *testing.T) {
	for name, src := range map[string]*models.Prompt{
		"sparse":    samplePrompt(),
		"populated": fullyPopulatedPrompt(),
	} {
		t.Run(name, func(t *testing.T) {
			converted, err := toGenPrompt(src)
			require.NoError(t, err)
			assert.Equal(t, promptToMap(t, src), promptToMap(t, converted),
				"no field of models.Prompt is undeclared in the schema, so nothing may drop")
		})
	}
}

// Acceptance criterion 6. `labels` is spec-NULLABLE and a nil pq.StringArray
// serializes as null today, but the generated field is a *[]string with
// omitempty — a nil POINTER would omit the key entirely. Key-present-null vs
// key-absent is a wire difference no schema assertion can catch.
func TestStrictGetPrompt_NilLabelsStayNullNotAbsent(t *testing.T) {
	srv, container := strictPromptServer(t)
	container.promptService.On("GetPromptBySlug", strictPrUserID, strictPrTeamID, strictPrSlug).
		Return(samplePrompt(), nil)

	req, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts/"+strictPrSlug)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	assert.Contains(t, w.Body.String(), `"labels":null`,
		"a prompt with no labels emitted null before the conversion and must still")
	assert.NotContains(t, w.Body.String(), `"labels":[]`, "coercing to [] is the change AC 6 forbids")
}

// An empty (but non-nil) label set is a different value and must stay [].
func TestStrictGetPrompt_EmptyLabelsStayAnArray(t *testing.T) {
	prompt := samplePrompt()
	prompt.Labels = []string{}

	srv, container := strictPromptServer(t)
	container.promptService.On("GetPromptBySlug", strictPrUserID, strictPrTeamID, strictPrSlug).
		Return(prompt, nil)

	_, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts/"+strictPrSlug)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"labels":[]`)
}

// The detail read carries the neighborhood the handler attaches after the
// service call, and every one of those converters has its own failure mode.
func TestToGenPrompt_CarriesTheWholeNeighborhood(t *testing.T) {
	converted, err := toGenPrompt(fullyPopulatedPrompt())
	require.NoError(t, err)

	require.NotNil(t, converted.Related)
	require.Len(t, *converted.Related, 1)
	assert.Equal(t, "Neighbor Prompt", (*converted.Related)[0].Title)
	require.NotNil(t, converted.Similar)
	require.Len(t, *converted.Similar, 1)
	assert.InDelta(t, 0.91, (*converted.Similar)[0].Score, 0.0001)
	require.NotNil(t, converted.Freshness)
	assert.Equal(t, "stale", string(converted.Freshness.Status))
	require.Len(t, converted.Freshness.MatchedRuleIds, 1)
}

// Acceptance criterion 3: the legacy envelope, byte for byte. The expectation is
// built from the EXPRESSION the deleted chi handler used, so a renamed key, a
// reworded message or a different field order fails this test rather than
// shipping.
func TestStrictListPrompts_EnvelopeIsByteIdentical(t *testing.T) {
	response := &models.PromptListResponse{
		Prompts:    models.JSONArray[models.Prompt]{*samplePrompt()},
		TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
	}

	srv, container := strictPromptServer(t)
	container.promptService.On("ListPrompts", strictPrUserID, mock.Anything).Return(response, nil)

	req, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	// What writeOK(w, map[string]interface{}{...}) produced before #777.
	expected, err := json.Marshal(map[string]interface{}{
		"status":  "success",
		"message": "Prompts retrieved successfully",
		"data":    response,
	})
	require.NoError(t, err)

	// Same keys, same values, same nesting — a renamed key, a reworded message or
	// a dropped field fails here. Only the ORDER of keys within each object
	// differs, because oapi-codegen emits struct fields alphabetically where
	// models.Prompt declares them semantically; JSON objects are unordered and
	// every client parses them as such (#800 records it with the other
	// conversion-inherent differences).
	assert.JSONEq(t, string(expected), w.Body.String())
	assert.True(t, strings.HasSuffix(w.Body.String(), "\n"),
		"writeOK encoded with json.Encoder, so the body ended in a newline")

	// The envelope itself, spelled out: it is the only one of the four domains
	// that has one, and losing it would break every client of this endpoint.
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	assert.Equal(t, "success", envelope["status"])
	assert.Equal(t, "Prompts retrieved successfully", envelope["message"])
	require.Contains(t, envelope, "data")
	assert.Len(t, envelope["data"].(map[string]any)["prompts"], 1)
}

// The required `prompts` array must serialize as [] and never null — the
// generated type cannot use the models.JSONArray shim, so make(...,0) in the
// converter is the only guarantee (this schema is allowlisted for that reason).
func TestStrictListPrompts_EmptyPageSerializesAnArray(t *testing.T) {
	srv, container := strictPromptServer(t)
	container.promptService.On("ListPrompts", strictPrUserID, mock.Anything).
		Return(&models.PromptListResponse{
			Prompts: nil, // the shape a repository returns for an empty page
			Page:    1, PerPage: 10,
		}, nil)

	req, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)
	assert.Contains(t, w.Body.String(), `"prompts":[]`, "a nil slice must marshal as [], not null")
}

// Every filter must actually reach the service. mock.Anything on the filters
// argument makes this untestable, which is how a dropped filter ships: the
// endpoint answers 200 with the FULL list, which reads as a legitimate answer.
func TestStrictListPrompts_EveryFilterReachesTheService(t *testing.T) {
	srv, container := strictPromptServer(t)
	container.promptService.On("ListPrompts", strictPrUserID,
		mock.MatchedBy(func(f services.PromptFilters) bool {
			return f.TeamID == strictPrTeamID &&
				f.UserID == strictPrUserID &&
				f.Status == "published" &&
				f.Search == "review" &&
				f.Freshness == services.FreshnessFilterStale &&
				f.SortBy == "created_at" &&
				f.SortOrder == "asc" &&
				f.ProjectID != nil && *f.ProjectID == strictPrProject &&
				f.MCPExpose != nil && *f.MCPExpose &&
				f.IsShared != nil && !*f.IsShared &&
				len(f.Labels) == 2 && f.Labels[0] == "code-review" &&
				f.Page == 2 && f.Limit == 5
		}),
	).Return(&models.PromptListResponse{Page: 2, PerPage: 5}, nil)

	_, w := strictPromptRequest(t, srv,
		"/api/v1/"+strictPrTeamID+"/prompts"+
			"?status=published&search=review&freshness=stale&sort_by=created_at&sort_order=asc"+
			"&project_id="+strictPrProject+"&mcp_expose=true&shared=false"+
			"&labels="+url.QueryEscape("code-review,documentation")+"&page=2&limit=5")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	container.promptService.AssertExpectations(t)
}

// An empty query value meant "no filter" to the chi parser this replaced, and
// still does — dropEmptyQueryValues strips them ahead of the binder (#779).
func TestStrictListPrompts_EmptyQueryValuesAreIgnoredNotRejected(t *testing.T) {
	srv, container := strictPromptServer(t)
	container.promptService.On("ListPrompts", strictPrUserID,
		mock.MatchedBy(func(f services.PromptFilters) bool {
			return f.Status == "" && f.Search == "" && f.Freshness == "" && f.SortBy == "" &&
				f.ProjectID == nil && f.MCPExpose == nil && f.IsShared == nil &&
				len(f.Labels) == 0 && f.Page == 1 && f.Limit == 10
		}),
	).Return(&models.PromptListResponse{Page: 1, PerPage: 10}, nil)

	_, w := strictPromptRequest(t, srv,
		"/api/v1/"+strictPrTeamID+"/prompts?status=&search=&freshness=&sort_by=&project_id=&labels=&page=&mcp_expose=")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	container.promptService.AssertExpectations(t)
}

// The spec's own description of mcp_expose and shared promises that a
// non-boolean value is IGNORED (no filter applied), which is what parseBoolParam
// did. The binder would 400 instead — dropUnparseableBoolQueryValues is what
// keeps the handler, the behaviour it replaces and the published description
// saying the same thing. Prompts is the only converted domain with bool filters.
func TestStrictListPrompts_UnparseableBooleanIsIgnoredNotRejected(t *testing.T) {
	srv, container := strictPromptServer(t)
	container.promptService.On("ListPrompts", strictPrUserID,
		mock.MatchedBy(func(f services.PromptFilters) bool {
			return f.MCPExpose == nil && f.IsShared == nil
		}),
	).Return(&models.PromptListResponse{Page: 1, PerPage: 10}, nil)

	_, w := strictPromptRequest(t, srv,
		"/api/v1/"+strictPrTeamID+"/prompts?mcp_expose=yes&shared=maybe")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	container.promptService.AssertExpectations(t)
}

// A parseable boolean still filters — otherwise the test above would pass with
// the parameter dropped unconditionally.
func TestStrictListPrompts_ParseableBooleanStillFilters(t *testing.T) {
	srv, container := strictPromptServer(t)
	container.promptService.On("ListPrompts", strictPrUserID,
		mock.MatchedBy(func(f services.PromptFilters) bool {
			return f.MCPExpose != nil && !*f.MCPExpose && f.IsShared != nil && *f.IsShared
		}),
	).Return(&models.PromptListResponse{Page: 1, PerPage: 10}, nil)

	_, w := strictPromptRequest(t, srv,
		"/api/v1/"+strictPrTeamID+"/prompts?mcp_expose=0&shared=1")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	container.promptService.AssertExpectations(t)
}

// Pagination CLAMPS rather than rejects, which the binder cannot do — it only
// converts. An out-of-range page or limit must still answer 200 with the
// defaults, exactly as validatePaginationParams has always done.
func TestStrictListPrompts_OutOfRangePaginationIsClampedNotRejected(t *testing.T) {
	srv, container := strictPromptServer(t)
	container.promptService.On("ListPrompts", strictPrUserID,
		mock.MatchedBy(func(f services.PromptFilters) bool {
			return f.Page == 1 && f.Limit == 10
		}),
	).Return(&models.PromptListResponse{Page: 1, PerPage: 10}, nil)

	_, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts?page=0&limit=999")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	container.promptService.AssertExpectations(t)
}

// freshness and sort_by are the two enums the handler still rejects itself:
// oapi-codegen binds enums without validating their values, an ignored freshness
// returns the full list, and an unchecked sort_by reaches the repository.
func TestStrictListPrompts_KeepsTheHandRolledEnumChecks(t *testing.T) {
	cases := map[string]struct{ query, detail, code string }{
		// BAD_REQUEST, not the VALIDATION_FAILED these carried immediately after
		// the conversion. That code was preserved from the pre-conversion
		// handler's "validation_error" arm, while memories, artifacts and
		// blueprints answered BAD_REQUEST for the same class -- so the API gave
		// two codes for one thing. #800 converged all four on the apierrors
		// defaults; the sibling assertions live in each domain's own test.
		"freshness": {"?freshness=stail", "freshness must be stale", "BAD_REQUEST"},
		"sort_by":   {"?sort_by=passwords", "invalid sort_by value: passwords", "BAD_REQUEST"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// No ListPrompts expectation: reaching the service is itself the failure.
			srv, _ := strictPromptServer(t)
			_, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts"+tc.query)

			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, tc.detail, body["detail"])
			assert.Equal(t, tc.code, body["code"], "the error code is part of the body the client sees")
		})
	}
}

// project_id is format: uuid in the spec, so a malformed one now fails in the
// binder. The message must name it as a UUID rather than say "not in the
// expected format", as the sibling domains do.
func TestStrictListPrompts_RejectsNonUUIDProjectID(t *testing.T) {
	srv, _ := strictPromptServer(t)
	_, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts?project_id=not-a-uuid")

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "project_id must be a valid UUID")
}

// The one documented 404, still detected by the repository sentinel rather than
// by a string fragment — so it can never leak onto the list operation.
func TestStrictGetPrompt_MissingPromptIsA404(t *testing.T) {
	srv, container := strictPromptServer(t)
	container.promptService.On("GetPromptBySlug", strictPrUserID, strictPrTeamID, strictPrSlug).
		Return(nil, repositories.ErrPromptNotFound)

	_, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts/"+strictPrSlug)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Prompt not found")
}

// Any other service failure keeps the opaque 500 with the wording the chi
// handler used. A list failure that merely MENTIONS "not found" must not become
// a 404 — a status neither list operation documents.
func TestStrictPrompts_ServiceErrorsKeepTheirStatuses(t *testing.T) {
	t.Run("detail 500", func(t *testing.T) {
		srv, container := strictPromptServer(t)
		container.promptService.On("GetPromptBySlug", strictPrUserID, strictPrTeamID, strictPrSlug).
			Return(nil, errors.New("database exploded"))

		_, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts/"+strictPrSlug)

		require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "Failed to get prompt")
	})

	t.Run("list 500 even when the error says not found", func(t *testing.T) {
		srv, container := strictPromptServer(t)
		container.promptService.On("ListPrompts", strictPrUserID, mock.Anything).
			Return(nil, errors.New("project not found while listing"))

		_, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts")

		require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "Failed to list prompts")
	})
}

// A conversion failure is a server-side bug, not a client error: it must surface
// as the operation's own 500 and never as a half-written 200.
func TestStrictPrompts_ConversionFailureIsA500(t *testing.T) {
	broken := samplePrompt()
	broken.ProjectID = "not-a-uuid" // never reachable via the path, but the DB is not the only writer

	t.Run("detail", func(t *testing.T) {
		srv, container := strictPromptServer(t)
		container.promptService.On("GetPromptBySlug", strictPrUserID, strictPrTeamID, strictPrSlug).
			Return(broken, nil)

		_, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts/"+strictPrSlug)

		require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "Failed to get prompt")
	})

	t.Run("list", func(t *testing.T) {
		srv, container := strictPromptServer(t)
		container.promptService.On("ListPrompts", strictPrUserID, mock.Anything).
			Return(&models.PromptListResponse{
				Prompts: models.JSONArray[models.Prompt]{*broken},
				Page:    1, PerPage: 10, TotalCount: 1, TotalPages: 1,
			}, nil)

		_, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts")

		require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "Failed to list prompts")
	})
}

// Every id the spec types as a UUID is parsed, and a malformed one must fail the
// conversion rather than serialize a zero UUID — a zero would pass the schema
// and point at nothing.
func TestToGenPrompt_RejectsMalformedUUIDs(t *testing.T) {
	notAUUID := "not-a-uuid"
	cases := map[string]func(*models.Prompt){
		"team_id":         func(p *models.Prompt) { p.TeamID = notAUUID },
		"project_id":      func(p *models.Prompt) { p.ProjectID = notAUUID },
		"relation_id":     func(p *models.Prompt) { p.Related[0].RelationID = notAUUID },
		"resource_id":     func(p *models.Prompt) { p.Related[0].ResourceID = notAUUID },
		"related project": func(p *models.Prompt) { p.Related[0].ProjectID = &notAUUID },
		"similar id":      func(p *models.Prompt) { p.Similar[0].ID = notAUUID },
		"matched rule id": func(p *models.Prompt) { p.Freshness.MatchedRuleIDs[0] = notAUUID },
	}

	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			src := fullyPopulatedPrompt()
			corrupt(src)

			_, err := toGenPrompt(src)
			require.Error(t, err)
			assert.Contains(t, err.Error(), notAUUID)
		})
	}
}

// The detail read must still record a resource-access event through the new
// mount. recordResourceAccess reports nothing unless the handler sets the id, so
// this is unpinned unless asserted.
func TestStrictGetPrompt_RecordsAnAccessEvent(t *testing.T) {
	container := newMockPromptContainer(t)
	container.teamService.On("IsUserMemberOfTeam", mock.Anything, strictPrUserID, strictPrTeamID).
		Return(true, nil).Maybe()
	container.promptService.On("GetPromptBySlug", strictPrUserID, strictPrTeamID, strictPrSlug).
		Return(samplePrompt(), nil)

	access := &spyResourceAccessService{}
	container.resourceAccessService = access
	srv := createTestServer(container)

	_, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts/"+strictPrSlug)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Len(t, access.events, 1, "the detail read must record exactly one access event")
	assert.Equal(t, "prompt-1", access.events[0].ResourceID)
	assert.Equal(t, resourceTypePrompt, access.events[0].ResourceType)
}

// Acceptance criterion 7, at the HANDLER level rather than the converter's.
// GetPrompt attaches the neighborhood after the service call, and with no
// relation/freshness service installed those helpers return nil — so deleting
// the three attachment lines is invisible unless a test installs the services
// and reads the fields back off the wire.
func TestStrictGetPrompt_AttachesTheNeighborhoodItLoads(t *testing.T) {
	container := newMockPromptContainer(t)
	container.teamService.On("IsUserMemberOfTeam", mock.Anything, strictPrUserID, strictPrTeamID).
		Return(true, nil).Maybe()
	container.promptService.On("GetPromptBySlug", strictPrUserID, strictPrTeamID, strictPrSlug).
		Return(samplePrompt(), nil)

	populated := fullyPopulatedPrompt()
	relations := svcmocks.NewMockRelationServiceInterface(t)
	relations.On("ListByResource", mock.Anything, strictPrUserID, strictPrTeamID,
		models.RelationResourceTypePrompt, "prompt-1", mock.Anything, mock.Anything).
		Return(&models.RelationListResponse{Related: populated.Related}, nil)
	container.relationService = relations

	freshness := svcmocks.NewMockFreshnessServiceInterface(t)
	freshness.On("GetResourceFreshness", mock.Anything, strictPrTeamID,
		models.RelationResourceTypePrompt, "prompt-1").
		Return(populated.Freshness, nil)
	container.freshnessService = freshness

	srv := createTestServer(container)
	req, w := strictPromptRequest(t, srv, "/api/v1/"+strictPrTeamID+"/prompts/"+strictPrSlug)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body["related"], 1, "the handler must attach what relatedForResource loaded")
	assert.Equal(t, "Neighbor Prompt", body["related"].([]any)[0].(map[string]any)["title"])
	require.NotNil(t, body["freshness"], "the handler must attach what freshnessForResource loaded")
	assert.Equal(t, "stale", body["freshness"].(map[string]any)["status"])
	relations.AssertExpectations(t)
	freshness.AssertExpectations(t)
}

// A missing user id is a wiring bug, not a client error: the auth middleware
// always sets it, so the handlers report an opaque 500 rather than panicking as
// the chi handler's type assertion would have.
func TestPromptsUserID_MissingUserIsAnInternalError(t *testing.T) {
	_, err := promptsUserID(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), promptsMsgInternalError)
}
