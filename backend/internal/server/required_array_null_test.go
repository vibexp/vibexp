package server

import (
	"encoding/json"
	"testing"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// TestRequiredResponseArraysNeverNull enforces issue #125 (Layer C of the
// response-conformance epic #122): a *required* array field in a documented
// API response must never serialize as JSON `null` — always `[]` — regardless
// of test coverage.
//
// It derives the required-array response fields from the OpenAPI spec
// (specconformance.RequiredArrayFields, the source of truth) and checks each
// against the Go type that produces it:
//
//   - Named Go response types (requiredArrayResponseRegistry) are marshaled
//     from their ZERO value; every spec-required array field must serialize as
//     `[]`. This holds by construction because those fields use
//     models.JSONArray[T], so the guarantee is coverage-independent.
//   - A small, documented set of responses built ad-hoc (map/generic envelope)
//     or by generated types that cannot use the shim is allowlisted; those
//     coerce nil→[] at their construction site (see each reason).
//   - Request-body schemas that happen to have required arrays are skipped.
//
// A NEW documented response schema with a required array that is neither
// registered (JSONArray-protected) nor allowlisted fails this test — that is
// the CI regression guard (acceptance criterion #2).
func TestRequiredResponseArraysNeverNull(t *testing.T) {
	fields, err := specconformance.RequiredArrayFields()
	if err != nil {
		t.Fatalf("enumerate required array fields from spec: %v", err)
	}
	if len(fields) == 0 {
		t.Fatal("spec reported no required array fields — enumeration is broken")
	}

	// 1. Completeness: every required-array schema is accounted for.
	assertRequiredArraySchemasAccounted(t, fields)

	// 2. By construction: each registered type marshals its required arrays as [].
	assertRegistryMarshalsEmptyArrays(t, fields)

	// 3. Keep the allowlist honest: an entry whose schema no longer has a
	//    required array (or was renamed) must be removed.
	assertAllowlistEntriesStillRequired(t, fields)
}

// assertRequiredArraySchemasAccounted checks every required-array schema in the
// spec is registered (JSONArray-protected), allowlisted, or request-only.
func assertRequiredArraySchemasAccounted(t *testing.T, fields map[string][]string) {
	t.Helper()
	for schema := range fields {
		if _, ok := requestOnlyRequiredArraySchemas[schema]; ok {
			continue
		}
		_, registered := requiredArrayResponseRegistry[schema]
		_, allowed := adHocRequiredArrayAllowlist[schema]
		if !registered && !allowed {
			t.Errorf("schema %q has required array field(s) %v but is neither registered "+
				"(give it a Go response type whose array fields use models.JSONArray[T]) nor in "+
				"the documented ad-hoc allowlist. A required array must serialize as [] by "+
				"construction, never null (issue #125).", schema, fields[schema])
		}
	}
}

// assertRegistryMarshalsEmptyArrays marshals each registered type's zero value
// and checks every spec-required array field serializes as [].
func assertRegistryMarshalsEmptyArrays(t *testing.T, fields map[string][]string) {
	t.Helper()
	for schema, proto := range requiredArrayResponseRegistry {
		want, ok := fields[schema]
		if !ok {
			t.Errorf("registry entry %q is no longer a required-array schema in the spec — remove it", schema)
			continue
		}
		b, err := json.Marshal(proto)
		if err != nil {
			t.Errorf("registry %q: marshal zero value: %v", schema, err)
			continue
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(b, &m); err != nil {
			t.Errorf("registry %q: zero value did not marshal to a JSON object (%s): %v", schema, b, err)
			continue
		}
		for _, field := range want {
			assertRequiredFieldIsEmptyArray(t, schema, field, m)
		}
	}
}

// assertRequiredFieldIsEmptyArray checks one required array field marshaled as [].
func assertRequiredFieldIsEmptyArray(t *testing.T, schema, field string, m map[string]json.RawMessage) {
	t.Helper()
	raw, present := m[field]
	if !present {
		t.Errorf("registry %q: required array field %q is absent from the marshaled zero value", schema, field)
		return
	}
	if string(raw) != "[]" {
		t.Errorf("registry %q: required array field %q serialized as %s from a zero value; "+
			"want []. Use models.JSONArray[T] for this field (issue #125).", schema, field, raw)
	}
}

// assertAllowlistEntriesStillRequired fails on allowlist/request-only entries
// whose schema no longer has a required array in the spec.
func assertAllowlistEntriesStillRequired(t *testing.T, fields map[string][]string) {
	t.Helper()
	for schema := range adHocRequiredArrayAllowlist {
		if _, ok := fields[schema]; !ok {
			t.Errorf("allowlist entry %q is no longer a required-array schema in the spec — remove it", schema)
		}
	}
	for schema := range requestOnlyRequiredArraySchemas {
		if _, ok := fields[schema]; !ok {
			t.Errorf("request-only entry %q is no longer a required-array schema in the spec — remove it", schema)
		}
	}
}

// requiredArrayResponseRegistry maps an OpenAPI response schema name to a
// zero-value instance of the Go type the live handler serializes for it. Every
// listed type's required array field(s) use models.JSONArray[T], so a nil
// value serializes as [] by construction. Add an entry when you introduce a
// documented response with a required array (and use JSONArray[T] for it).
var requiredArrayResponseRegistry = map[string]any{
	"APIKey":                           models.APIKey{},
	"AgentListResponse":                models.AgentListResponse{},
	"ArtifactListResponse":             models.ArtifactListResponse{},
	"ArtifactVersionListResponse":      models.ArtifactVersionListResponse{},
	"AttachmentListResponse":           models.AttachmentListResponse{},
	"BlueprintImportReport":            models.BlueprintImportReport{},
	"BlueprintListResponse":            models.BlueprintListResponse{},
	"BlueprintVersionListResponse":     models.BlueprintVersionListResponse{},
	"ClaudeCodeHooksPaginatedResponse": models.ClaudeCodeHooksPaginatedResponse{},
	"ConversationListResponse":         models.ConversationListResponse{},
	"CursorIDEHooksPaginatedResponse":  models.CursorIDEHooksPaginatedResponse{},
	"CursorOverviewStats":              models.CursorOverviewStats{},
	"CursorRecentActivitiesResponse":   models.CursorRecentActivitiesResponse{},
	"CursorSessionsResponse":           models.CursorSessionsResponse{},
	"EmbeddingCoverageResponse":        models.EmbeddingCoverageResponse{},
	"EmbeddingProviderListResponse":    models.EmbeddingProviderListResponse{},
	"FeedItemListResponse":             models.FeedItemListResponse{},
	"FeedItemReplyListResponse":        models.FeedItemReplyListResponse{},
	"FeedListResponse":                 models.FeedListResponse{},
	"GitHubRepositoriesResponse":       models.GitHubRepositoriesResponse{},
	"MemoryListResponse":               models.MemoryListResponse{},
	"MemoryVersionListResponse":        models.MemoryVersionListResponse{},
	"ModelProviderListResponse":        models.ModelProviderListResponse{},
	"OverviewStats":                    models.OverviewStats{},
	"PendingInvitationsListResponse":   models.PendingInvitationsListResponse{},
	"ProjectListResponse":              models.ProjectListResponse{},
	"PromptDependenciesResponse":       models.PromptDependenciesResponse{},
	"PromptGalleryListResponse":        models.PromptGalleryListResponse{},
	"PromptListResponse":               models.PromptListResponse{},
	"PromptVersionListResponse":        models.PromptVersionListResponse{},
	"RecentActivitiesResponse":         models.RecentActivitiesResponse{},
	"SearchResultsResponse":            models.SearchResultsResponse{},
	"SessionCountsResponse":            models.SessionCountsResponse{},
	"SessionsResponse":                 models.SessionsResponse{},
	"Team":                             models.Team{},
	"TeamListResponse":                 models.TeamListResponse{},
	"TeamMembersListResponse":          models.TeamMembersListResponse{},
	"ProvidersResponse":                ProvidersResponse{},
}

// adHocRequiredArrayAllowlist lists required-array response schemas whose body
// is NOT a shim-protected Go struct: it is built ad-hoc (a map or a generic
// data-envelope) with the slice always initialized via make(...,0) at the
// construction site, or it is an oapi-codegen generated type that cannot use
// the shim. Each is safe today by construction-site initialization; the value
// documents where. These are covered by their handler tests / the payload
// coverage ledger rather than the zero-value marshal check above.
var adHocRequiredArrayAllowlist = map[string]string{
	"NotificationListResponse":         "generated strict-server type (internal/server/gen); handler builds via make(...,0) — handlers_notifications.go toGenNotifications",
	"TypeListResponse":                 "generated strict-server type (internal/server/gen/types); handler builds via make(...,0) — handlers_types.go toGenTypes",
	"CommentListResponse":              "generated strict-server type (internal/server/gen/comments); handler builds via make(...,0) — handlers_comments.go toGenCommentListResponse",
	"AdminUserListResponse":            "generated strict-server type (internal/server/gen/admin); handler builds users via make(...,0) — handlers_admin.go toGenAdminUserList",
	"AdminUserDetail":                  "generated strict-server type (internal/server/gen/admin); handler builds memberships via make(...,0) — handlers_admin.go toGenAdminUserDetail",
	"AdminTeamListResponse":            "generated strict-server type (internal/server/gen/admin); handler builds teams via make(...,0) — handlers_admin.go toGenAdminTeamList",
	"AdminTeamDetail":                  "generated strict-server type (internal/server/gen/admin); handler builds members via make(...,0) — handlers_admin.go toGenAdminTeamDetail",
	"RecentCommentListResponse":        "generated strict-server type (internal/server/gen/comments); handler builds via make(...,0) — handlers_comments.go toGenRecentComments",
	"ActivityListResponse":             "activities-pkg wire type wrapped in a data-envelope map; slices always make(...,0) — services/activities/service.go",
	"ActivityStatsResponse":            "activities-pkg wire type wrapped in a data-envelope map; slices always make(...,0) — services/activities/service.go",
	"ActivityTypesResponse":            "activities-pkg wire type wrapped in a data-envelope map; literal non-nil string slices — services/activities/service.go",
	"ActivityEntityTypesEnvelope":      "bare []string in a data-envelope map (structural, not object) — activity_handlers.go handleActivitiesEntityTypesGet; literal slice",
	"EntityTypesResponse":              "structural spec drift: /activities/entity-types emits a bare array on the wire (#122); literal non-nil slice",
	"AgentExecutionEventsPageResponse": "ad-hoc map response; events always make(...,0) — agent_execution_handlers.go handlePageBasedPagination",
	"AgentExecutionEventsPollResponse": "ad-hoc map response; events always make(...,0) — agent_execution_handlers.go handleCursorBasedPolling",
	"AgentExecutionListResponse":       "ad-hoc map response; executions always make(...,0) — agent_handlers.go handleListAgentExecutions",
	"ConversationExecutionsResponse":   "ad-hoc map response; executions always make(...,0) — agent_execution_handlers.go handleGetConversationExecutions",
	"PromptPlaceholdersResponse":       "ad-hoc map response; placeholders coerced nil->[] in handler (#121) — prompt_handlers.go handleGetPromptPlaceholders",
	"APIKeyListResponse":               "structural spec drift: GET /api-keys emits a top-level array, not an object (#122); slice always make(...,0)",
	"PaginatedResponse":                "orphan generic schema — not referenced by any operation; no Go type or endpoint",
	"ResourceAccessMetricsData":        "component used only as the nested `data` of ResourceAccessMetricsResponse, not a top-level response array; counts built via make(...,0) — resource_access_handlers.go buildResourceAccessMetricsData",
}

// requestOnlyRequiredArraySchemas are request-body schemas whose required
// array fields are not response payloads and so are out of scope for #125.
var requestOnlyRequiredArraySchemas = map[string]struct{}{
	"CreateAPIKeyRequest":    {}, // integration_codes
	"SendInvitationsRequest": {}, // emails
}
