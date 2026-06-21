package specconformance

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func recordedResponse(t *testing.T, status int, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	if contentType != "" {
		rec.Header().Set("Content-Type", contentType)
	}
	rec.WriteHeader(status)
	if _, err := rec.Body.WriteString(body); err != nil {
		t.Fatalf("write body: %v", err)
	}
	return rec
}

// recordingTB captures failure reports from AssertConformsToSpec so its
// public failure/skip paths can be tested without failing the real test.
type recordingTB struct {
	testing.TB
	errored bool
	fataled bool
}

func (r *recordingTB) Helper()                      {}
func (r *recordingTB) Logf(string, ...any)          {}
func (r *recordingTB) Errorf(string, ...any)        { r.errored = true }
func (r *recordingTB) Fatalf(f string, args ...any) { r.fataled = true; r.TB.Fatalf(f, args...) }

// TestAssertConformsToSpecReportsFailure drives the exported entry point
// with a schema-violating response: it must report a failure and must not
// record coverage.
func TestAssertConformsToSpecReportsFailure(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := recordedResponse(t, http.StatusOK, "application/json", `{"unexpected":true}`)

	shim := &recordingTB{TB: t}
	AssertConformsToSpec(shim, req, rec)

	if shim.fataled {
		t.Fatal("unexpected Fatalf from AssertConformsToSpec")
	}
	if !shim.errored {
		t.Fatal("non-conforming response did not fail the calling test")
	}
	if _, ok := ValidatedOperations()["GET /ping"]; ok {
		t.Error("failed assertion must not record coverage for GET /ping")
	}
}

// problemJSONBody is a fully-conforming RFC 9457 error body matching the
// ErrorResponse schema (all required fields present), used to exercise the
// error-response content-type validation that previously lived behind the
// #1704 known divergence.
const problemJSONBody = `{"type":"https://api.vibexp.io/errors/UNAUTHORIZED",` +
	`"title":"Unauthorized","status":401,"detail":"Authentication required",` +
	`"code":"UNAUTHORIZED","request_id":"req-123","timestamp":"2025-11-08T10:15:30Z"}`

// TestAssertConformsToSpecValidatesErrorResponse drives the exported entry
// point with a conforming application/problem+json error response. The spec
// now documents problem+json for error responses (#1802), so this must pass
// validation and record coverage — there is no longer a divergence to skip.
func TestAssertConformsToSpecValidatesErrorResponse(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents", nil)
	rec := recordedResponse(t, http.StatusUnauthorized, "application/problem+json", problemJSONBody)

	shim := &recordingTB{TB: t}
	AssertConformsToSpec(shim, req, rec)

	if shim.errored || shim.fataled {
		t.Fatal("conforming problem+json error response must not fail")
	}
	if _, ok := ValidatedOperations()["GET /api/v1/{team_id}/agents"]; !ok {
		t.Error("conforming error response did not record coverage for GET /api/v1/{team_id}/agents")
	}
}

// TestConformingResponsePasses is the helper's own positive control: a
// response matching the documented schema must produce no errors and must
// mark the operation as covered.
func TestConformingResponsePasses(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := recordedResponse(t, http.StatusOK, "application/json", `{"status":"healthy","sha":"abc12345"}`)

	op, skip, errs, err := validateRecordedResponse(req, rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != "" {
		t.Fatalf("unexpected skip: %s", skip)
	}
	if len(errs) > 0 {
		t.Fatalf("conforming response reported errors: %v", errs)
	}
	if op != "GET /health" {
		t.Errorf("operation: got %q, want %q", op, "GET /health")
	}

	AssertConformsToSpec(t, req, rec)
	if _, ok := ValidatedOperations()["GET /health"]; !ok {
		t.Error("conforming response did not record coverage for GET /health")
	}
}

// TestNonConformingBodyFails is the helper's negative control: a body that
// violates the documented schema (status must be a string) must be reported.
func TestNonConformingBodyFails(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := recordedResponse(t, http.StatusOK, "application/json", `{"status":12345}`)

	op, skip, errs, err := validateRecordedResponse(req, rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != "" {
		t.Fatalf("unexpected skip: %s", skip)
	}
	if len(errs) == 0 {
		t.Fatal("schema-violating response body was not reported")
	}
	if op != "GET /health" {
		t.Errorf("operation: got %q, want %q", op, "GET /health")
	}
}

// TestWrongContentTypeFails: /ping documents text/plain only.
func TestWrongContentTypeFails(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := recordedResponse(t, http.StatusOK, "application/json", `{"unexpected":true}`)

	_, skip, errs, err := validateRecordedResponse(req, rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != "" || len(errs) == 0 {
		t.Fatalf("undocumented content type was not reported (skip=%q, errs=%d)", skip, len(errs))
	}
}

// TestUndocumentedStatusFails: /ping documents only 200.
func TestUndocumentedStatusFails(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := recordedResponse(t, http.StatusTeapot, "text/plain", "pong")

	_, skip, errs, err := validateRecordedResponse(req, rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != "" || len(errs) == 0 {
		t.Fatalf("undocumented status code was not reported (skip=%q, errs=%d)", skip, len(errs))
	}
}

// TestUndocumentedPathFails: a request that matches no documented path must
// be reported, not silently passed.
func TestUndocumentedPathFails(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/this-route-does-not-exist", nil)
	rec := recordedResponse(t, http.StatusOK, "application/json", `{}`)

	_, skip, errs, err := validateRecordedResponse(req, rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != "" || len(errs) == 0 {
		t.Fatalf("undocumented path was not reported (skip=%q, errs=%d)", skip, len(errs))
	}
}

// TestErrorResponseContentTypeValidated: the spec now documents
// application/problem+json for error responses (#1802, formerly the #1704
// known divergence). A conforming problem+json 401 must validate cleanly —
// no skip, no errors — proving the error content type is validated rather
// than skipped.
func TestErrorResponseContentTypeValidated(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents", nil)
	rec := recordedResponse(t, http.StatusUnauthorized, "application/problem+json", problemJSONBody)

	op, skip, errs, err := validateRecordedResponse(req, rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != "" {
		t.Fatalf("error response must be validated, not skipped: got skip %q", skip)
	}
	if len(errs) > 0 {
		t.Fatalf("conforming problem+json error response reported errors: %v", errs)
	}
	if op != "GET /api/v1/{team_id}/agents" {
		t.Errorf("operation: got %q, want %q", op, "GET /api/v1/{team_id}/agents")
	}
}

func TestDocumentedOperationsSanity(t *testing.T) {
	ops, err := DocumentedOperations()
	if err != nil {
		t.Fatalf("DocumentedOperations: %v", err)
	}
	if _, ok := ops["GET /ping"]; !ok {
		t.Error("GET /ping missing from documented operations")
	}
	if _, ok := ops["GET /api/v1/{team_id}/agents"]; !ok {
		t.Error("GET /api/v1/{team_id}/agents missing from documented operations")
	}
	if len(ops) < 150 {
		t.Errorf("suspiciously few documented operations: %d (multi-file $ref resolution broken?)", len(ops))
	}
}

func TestLedgerViolations(t *testing.T) {
	documented := map[string]struct{}{
		"GET /a":  {},
		"POST /b": {},
		"PUT /c":  {},
	}
	covered := map[string]struct{}{"GET /a": {}}

	tests := []struct {
		name         string
		ledger       map[string]string
		wantCount    int
		wantContains string
	}{
		{
			name:      "exact ledger has no violations",
			ledger:    map[string]string{"POST /b": "reason", "PUT /c": "reason"},
			wantCount: 0,
		},
		{
			name:         "uncovered operation missing from ledger",
			ledger:       map[string]string{"POST /b": "reason"},
			wantCount:    1,
			wantContains: `"PUT /c"`,
		},
		{
			name: "covered operation still in ledger is stale",
			ledger: map[string]string{
				"GET /a": "reason", "POST /b": "reason", "PUT /c": "reason",
			},
			wantCount:    1,
			wantContains: "shrink-only",
		},
		{
			name: "ledger entry for undocumented operation is stale",
			ledger: map[string]string{
				"POST /b": "reason", "PUT /c": "reason", "DELETE /gone": "reason",
			},
			wantCount:    1,
			wantContains: "no longer documented",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ledgerViolations(tc.ledger, documented, covered)
			if len(got) != tc.wantCount {
				t.Fatalf("violations: got %d (%v), want %d", len(got), got, tc.wantCount)
			}
			if tc.wantContains != "" && !strings.Contains(got[0], tc.wantContains) {
				t.Errorf("violation %q does not contain %q", got[0], tc.wantContains)
			}
		})
	}
}

// TestValidateRecordedResponseConcurrentNoRace drives validateRecordedResponse
// (FindPath + ValidateHttpResponse) and DocumentedOperations from many
// goroutines against the shared singleton model. The validator mutates that
// model while rendering, so this exercises validateMu and must produce
// identical conforming outcomes under -race.
func TestValidateRecordedResponseConcurrentNoRace(t *testing.T) {
	const goroutines = 32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			// Alternate between validation and model iteration so both
			// shared-model access paths run concurrently.
			if i%2 == 0 {
				assertConcurrentValidateHealth(t)
			} else {
				assertConcurrentDocumentedOperations(t)
			}
		}(i)
	}
	wg.Wait()
}

func assertConcurrentValidateHealth(t *testing.T) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	// Build the recorder inline (not via recordedResponse) so this helper uses
	// only t.Errorf, never t.Fatalf: it runs in a spawned goroutine, where
	// t.Fatalf is illegal (it only exits the calling goroutine). An in-memory
	// recorder write never errors, so a plain t.Errorf guard is sufficient.
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	rec.WriteHeader(http.StatusOK)
	if _, err := rec.Body.WriteString(`{"status":"healthy","sha":"abc12345"}`); err != nil {
		t.Errorf("write body: %v", err)
		return
	}
	op, skip, errs, err := validateRecordedResponse(req, rec)
	if err != nil {
		t.Errorf("validateRecordedResponse error: %v", err)
	}
	if skip != "" {
		t.Errorf("unexpected skip: %s", skip)
	}
	if len(errs) > 0 {
		t.Errorf("conforming response reported errors: %v", errs)
	}
	if op != "GET /health" {
		t.Errorf("operation: got %q, want %q", op, "GET /health")
	}
}

func assertConcurrentDocumentedOperations(t *testing.T) {
	t.Helper()
	ops, err := DocumentedOperations()
	if err != nil {
		t.Errorf("DocumentedOperations error: %v", err)
	}
	if _, ok := ops["GET /ping"]; !ok {
		t.Errorf("GET /ping missing from documented operations")
	}
}
