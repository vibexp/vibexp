package server

// Payload-coverage ledger (#1714, epic #1693) — the companion to the route
// drift gate in openapi_drift_test.go. The drift gate proves every route is
// documented; this ledger tracks which documented operations also have at
// least one response validated against the spec via
// specconformance.AssertConformsToSpec.
//
// The ledger is shrink-only by construction, exactly like the drift-gate
// allowlist: an entry whose operation gains a spec-validated response, or
// whose operation leaves the spec, fails the suite and must be removed.
// New operations must ship with a spec-validated test — never a new entry.
//
// Enforcement runs in TestMain after the full package suite, because
// coverage is recorded by the tests themselves and parallel subtests only
// settle once m.Run returns. Partial runs (-run filters, -short) skip
// enforcement: they cannot observe full coverage.

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/vibexp/vibexp/internal/specconformance"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if code == 0 && isFullSuiteRun() {
		code = enforcePayloadCoverageLedger()
	}
	os.Exit(code)
}

// isFullSuiteRun reports whether every test in the package ran: -run/-skip
// filters, -list, and -short all skip tests, so observed coverage would be
// incomplete and enforcement would false-fail.
func isFullSuiteRun() bool {
	for _, name := range []string{"test.run", "test.skip", "test.list"} {
		if f := flag.Lookup(name); f != nil && f.Value.String() != "" {
			return false
		}
	}
	return !testing.Short()
}

func enforcePayloadCoverageLedger() int {
	violations, err := specconformance.LedgerViolations(payloadCoverageLedger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "payload-coverage ledger check: %v\n", err)
		return 1
	}
	if len(violations) == 0 {
		return 0
	}
	fmt.Fprintf(os.Stderr,
		"\npayload-coverage ledger check FAILED (%d violation(s)) — see openapi_payload_coverage_test.go:\n",
		len(violations))
	for _, v := range violations {
		fmt.Fprintf(os.Stderr, "  %s\n", v)
	}
	return 1
}

// payloadCoverageLedger lists every documented operation that does NOT yet
// have a spec-validated response in this package's tests, keyed
// "METHOD /path" with the reason it is still uncovered. Burn it down one
// domain per PR (#1714) by wiring specconformance.AssertConformsToSpec into
// that domain's handler tests and deleting the freed entries.
var payloadCoverageLedger = map[string]string{
	"DELETE /api/v1/ai-tools/claude-code/sessions/{session_id}":                        "TODO(#1714): uncovered",
	"DELETE /api/v1/ai-tools/cursor-ide/sessions/{session_id}":                         "TODO(#1714): uncovered",
	"DELETE /api/v1/api-keys/{id}":                                                     "TODO(#1714): uncovered",
	"DELETE /api/v1/device-tokens":                                                     "TODO(#1714): uncovered",
	"DELETE /api/v1/embedding-providers/{id}":                                          "TODO(#1714): uncovered",
	"DELETE /api/v1/settings/api-keys/{id}":                                            "TODO(#1714): uncovered",
	"DELETE /api/v1/settings/embedding-providers/{id}":                                 "TODO(#1714): uncovered",
	"DELETE /api/v1/{team_id}/artifacts/{project_id}/{slug}":                           "TODO(#1714): uncovered",
	"DELETE /api/v1/{team_id}/blueprints/{project_id}/{slug}":                          "TODO(#1714): uncovered",
	"DELETE /api/v1/{team_id}/feed-items/{item_id}":                                    "TODO(#1714): uncovered",
	"DELETE /api/v1/{team_id}/feeds/{feed_id}":                                         "TODO(#1714): uncovered",
	"DELETE /api/v1/{team_id}/integrations/github/disconnect":                          "TODO(#1714): uncovered",
	"DELETE /api/v1/{team_id}/memories/{id}":                                           "TODO(#1714): uncovered",
	"DELETE /api/v1/{team_id}/projects/{slug}":                                         "TODO(#1714): uncovered",
	"DELETE /api/v1/{team_id}/prompts/{slug}/share":                                    "TODO(#1714): uncovered",
	"DELETE /api/v1/{team_id}/prompts/{slug}":                                          "TODO(#1714): uncovered",
	"DELETE /api/v1/teams/{id}/invitations/{invitationId}":                             "TODO(#1714): uncovered",
	"DELETE /api/v1/teams/{id}/members/{userId}":                                       "TODO(#1714): uncovered",
	"DELETE /api/v1/teams/{id}":                                                        "TODO(#1714): uncovered",
	"GET /api/v1/activities/entity-types":                                              "TODO(#1714): uncovered",
	"GET /api/v1/activities/{id}":                                                      "TODO(#1714): uncovered",
	"GET /api/v1/activities/stats":                                                     "TODO(#1714): uncovered",
	"GET /api/v1/activities":                                                           "TODO(#1714): uncovered",
	"GET /api/v1/activities/types":                                                     "TODO(#1714): uncovered",
	"GET /api/v1/ai-tools/claude-code/hooks":                                           "TODO(#1714): uncovered",
	"GET /api/v1/ai-tools/claude-code/overview-stats":                                  "TODO(#1714): uncovered",
	"GET /api/v1/ai-tools/claude-code/recent-activities":                               "TODO(#1714): uncovered",
	"GET /api/v1/ai-tools/claude-code/session-counts":                                  "TODO(#1714): uncovered",
	"GET /api/v1/ai-tools/claude-code/sessions":                                        "TODO(#1714): uncovered",
	"GET /api/v1/ai-tools/cursor-ide/hooks":                                            "TODO(#1714): uncovered",
	"GET /api/v1/ai-tools/cursor-ide/overview-stats":                                   "TODO(#1714): uncovered",
	"GET /api/v1/ai-tools/cursor-ide/recent-activities":                                "TODO(#1714): uncovered",
	"GET /api/v1/ai-tools/cursor-ide/session-counts":                                   "TODO(#1714): uncovered",
	"GET /api/v1/ai-tools/cursor-ide/sessions":                                         "TODO(#1714): uncovered",
	"GET /api/v1/api-keys":                                                             "TODO(#1714): uncovered",
	"GET /api/v1/auth/callback":                                                        "TODO(#1714): uncovered",
	"GET /api/v1/auth/login":                                                           "TODO(#1714): uncovered",
	"GET /api/v1/auth/me":                                                              "TODO(#1714): uncovered",
	"GET /api/v1/embedding-providers/{id}":                                             "TODO(#1714): uncovered",
	"GET /api/v1/embedding-providers":                                                  "TODO(#1714): uncovered",
	"GET /api/v1/invitations/pending":                                                  "TODO(#1714): uncovered",
	"GET /api/v1/invitations/{token}":                                                  "TODO(#1714): uncovered",
	"GET /api/v1/preferences":                                                          "TODO(#1714): uncovered",
	"GET /api/v1/prompt-gallery/categories":                                            "TODO(#1714): uncovered",
	"GET /api/v1/prompt-gallery/prompts/{id}":                                          "TODO(#1714): uncovered",
	"GET /api/v1/prompt-gallery/prompts":                                               "TODO(#1714): uncovered",
	"GET /api/v1/resource-usage":                                                       "TODO(#1714): uncovered",
	"GET /api/v1/settings/api-keys":                                                    "TODO(#1714): uncovered",
	"GET /api/v1/settings/embedding-providers/{id}":                                    "TODO(#1714): uncovered",
	"GET /api/v1/settings/embedding-providers":                                         "TODO(#1714): uncovered",
	"GET /api/v1/shared/prompts/{token}":                                               "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/agents/{id}/executions":                                     "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/artifacts/{project_id}/{slug}":                              "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/artifacts/{project_id}":                                     "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/artifacts/stats":                                            "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/artifacts":                                                  "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/blueprints/{project_id}/{slug}":                             "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/blueprints/{project_id}":                                    "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/blueprints/stats":                                           "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/blueprints":                                                 "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/feed-items/{item_id}/replies":                               "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/feed-items/{item_id}":                                       "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/feed-items":                                                 "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/feeds/{feed_id}/items":                                      "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/feeds/{feed_id}":                                            "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/feeds":                                                      "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/integrations/github/install-url":                            "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/integrations/github/repositories":                           "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/integrations/github/status":                                 "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/memories/{id}":                                              "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/memories/search":                                            "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/memories":                                                   "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/projects/{project_id}/migration/inventory":                  "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/projects/{slug}/stats":                                      "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/projects/{slug}":                                            "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/projects":                                                   "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/prompts/{slug}/dependencies":                                "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/prompts/{slug}/placeholders":                                "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/prompts/{slug}/share":                                       "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/prompts/{slug}":                                             "TODO(#1714): uncovered",
	"GET /api/v1/{team_id}/resource-access-metrics":                                    "TODO(#1714): uncovered",
	"GET /api/v1/teams/{id}/invitations":                                               "TODO(#1714): uncovered",
	"GET /api/v1/teams/{id}/members":                                                   "TODO(#1714): uncovered",
	"GET /api/v1/teams/{id}":                                                           "TODO(#1714): uncovered",
	"GET /api/v1/teams":                                                                "TODO(#1714): uncovered",
	"GET /bo/v1/reports/usage-and-growth":                                              "TODO(#1714): uncovered",
	"GET /health":                                                                      "TODO(#1714): uncovered",
	"GET /ping":                                                                        "TODO(#1714): uncovered",
	"POST /api/v1/activities":                                                          "TODO(#1714): uncovered",
	"POST /api/v1/api-keys":                                                            "TODO(#1714): uncovered",
	"POST /api/v1/auth/dev/login":                                                      "TODO(#1714): uncovered",
	"POST /api/v1/auth/logout":                                                         "TODO(#1714): uncovered",
	"POST /api/v1/claude-code/hooks":                                                   "TODO(#1714): uncovered",
	"POST /api/v1/cursor-ide/hooks":                                                    "TODO(#1714): uncovered",
	"POST /api/v1/device-tokens":                                                       "TODO(#1714): uncovered",
	"POST /api/v1/embedding-providers":                                                 "TODO(#1714): uncovered",
	"POST /api/v1/embedding-providers/validate":                                        "TODO(#1714): uncovered",
	"POST /api/v1/invitations/{token}/accept":                                          "TODO(#1714): uncovered",
	"POST /api/v1/invitations/{token}/reject":                                          "TODO(#1714): uncovered",
	"POST /api/v1/prompt-gallery/prompts/{id}/use":                                     "TODO(#1714): uncovered",
	"POST /api/v1/settings/api-keys":                                                   "TODO(#1714): uncovered",
	"POST /api/v1/settings/embedding-providers":                                        "TODO(#1714): uncovered",
	"POST /api/v1/settings/embedding-providers/validate":                               "TODO(#1714): uncovered",
	"POST /api/v1/support/message":                                                     "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/agents/{id}/execute":                                       "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/agents/preview-card":                                       "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/artifacts":                                                 "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/blueprints":                                                "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/feed-items/{item_id}/archive":                              "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/feed-items/{item_id}/replies":                              "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/feed-items/{item_id}/unarchive":                            "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/feeds/{feed_id}/items":                                     "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/feeds":                                                     "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/integrations/github/callback":                              "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/integrations/github/import-blueprints":                     "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/integrations/github/repositories/{repo_id}/import-project": "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/memories":                                                  "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/projects/{project_id}/migration":                           "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/projects":                                                  "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/prompts/{slug}/render":                                     "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/prompts/{slug}/share":                                      "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/prompts":                                                   "TODO(#1714): uncovered",
	"POST /api/v1/{team_id}/search":                                                    "TODO(#1714): uncovered",
	"POST /api/v1/teams/{id}/invitations":                                              "TODO(#1714): uncovered",
	"POST /api/v1/teams":                                                               "TODO(#1714): uncovered",
	"POST /api/v1/user/onboarding/complete":                                            "TODO(#1714): uncovered",
	"POST /api/v1/webhooks/github":                                                     "TODO(#1714): uncovered",
	"POST /bo/v1/embeddings/backfill":                                                  "TODO(#1714): uncovered",
	"PUT /api/v1/embedding-providers/{id}":                                             "TODO(#1714): uncovered",
	"PUT /api/v1/preferences":                                                          "TODO(#1714): uncovered",
	"PUT /api/v1/settings/embedding-providers/{id}":                                    "TODO(#1714): uncovered",
	"PUT /api/v1/{team_id}/agents/{id}/credentials":                                    "TODO(#1714): uncovered",
	"PUT /api/v1/{team_id}/artifacts/{project_id}/{slug}":                              "TODO(#1714): uncovered",
	"PUT /api/v1/{team_id}/blueprints/{project_id}/{slug}":                             "TODO(#1714): uncovered",
	"PUT /api/v1/{team_id}/feeds/{feed_id}":                                            "TODO(#1714): uncovered",
	"PUT /api/v1/{team_id}/memories/{id}":                                              "TODO(#1714): uncovered",
	"PUT /api/v1/{team_id}/projects/{slug}":                                            "TODO(#1714): uncovered",
	"PUT /api/v1/{team_id}/prompts/{slug}":                                             "TODO(#1714): uncovered",
	"PUT /api/v1/teams/{id}":                                                           "TODO(#1714): uncovered",
}
