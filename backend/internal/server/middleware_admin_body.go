package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
)

// Unknown-field rejection for admin request bodies (#455).
//
// The spec marks AdminUserUpdateRequest `additionalProperties: false`, but that
// is documentation only: oapi-codegen emits a plain
// `json.NewDecoder(r.Body).Decode(&body)` with no DisallowUnknownFields, and
// encoding/json ignores unknown keys by default. So `{"name":"x",
// "email":"attacker@example.com"}` would decode cleanly, the email would vanish,
// and the caller would get a 200 implying their change was applied.
//
// For an admin surface that edits accounts, silently discarding a field the
// caller believes they changed is worse than refusing it — hence #455's
// acceptance criterion that identity fields are REJECTED, not ignored.
//
// The allowed field set is derived by reflection from the generated request type
// rather than hand-listed, so it cannot drift from the spec: add a property to
// the schema, regenerate, and the guard widens with it.
//
// The same gap exists for `required`: a non-pointer field the body omits
// decodes to its zero value. For the whole-row settings replaces (#1200) that
// would store a value the caller never sent, so those operations also set
// requireAll, and the required set is derived from the same generated type.

// adminGuardedBody describes one guarded operation: the method and path it
// matches, plus a zero value of the generated request-body type whose JSON tags
// define the allowed field set.
//
// The path is matched with a regexp rather than chi's RoutePattern because this
// middleware is registered with Use() on the mux, which runs BEFORE routing — at
// that point RouteContext.RoutePattern() is still empty.
type adminGuardedBody struct {
	method   string
	path     *regexp.Regexp
	bodyType any
	// hint is appended to the rejection message; empty for none.
	hint string
	// requireAll also rejects a body that omits (or nulls) any field the
	// generated type declares without `omitempty` — i.e. every field the
	// schema marks required. The decoder would otherwise zero-fill it
	// silently, which on a whole-row replace stores a value the caller never
	// sent (an omitted `enabled` would switch a feature off).
	requireAll bool
}

// adminIdentityFieldsHint explains why the user-edit bodies are so narrow.
const adminIdentityFieldsHint = " Email and identity-provider fields are owned by the identity provider."

var adminGuardedBodies = []adminGuardedBody{
	{
		method:   http.MethodPost,
		path:     regexp.MustCompile(`^/api/v1/admin/users$`),
		bodyType: admingen.AdminUserCreateRequest{},
		hint:     adminIdentityFieldsHint,
	},
	{
		method:   http.MethodPatch,
		path:     regexp.MustCompile(`^/api/v1/admin/users/[^/]+$`),
		bodyType: admingen.AdminUserUpdateRequest{},
		hint:     adminIdentityFieldsHint,
	},
	{
		method:   http.MethodPut,
		path:     regexp.MustCompile(`^/api/v1/admin/saved-filters/[^/]+$`),
		bodyType: admingen.AdminSavedFiltersReplaceRequest{},
	},
	{
		method:   http.MethodPut,
		path:     regexp.MustCompile(`^/api/v1/admin/settings/email$`),
		bodyType: admingen.AdminInstanceEmailSettingsRequest{},
	},
	{
		method:     http.MethodPut,
		path:       regexp.MustCompile(`^/api/v1/admin/settings/search$`),
		bodyType:   admingen.AdminInstanceSearchSettingsUpdate{},
		requireAll: true,
	},
	{
		method:     http.MethodPut,
		path:       regexp.MustCompile(`^/api/v1/admin/settings/ai-summary$`),
		bodyType:   admingen.AdminInstanceAISummarySettingsUpdate{},
		requireAll: true,
	},
	{
		// No recipient field: a test message always goes to the acting admin.
		method:   http.MethodPost,
		path:     regexp.MustCompile(`^/api/v1/admin/settings/email/test$`),
		bodyType: admingen.AdminInstanceEmailTestRequest{},
	},
	// Instance authentication settings (#1238).
	{
		method:   http.MethodPost,
		path:     regexp.MustCompile(`^/api/v1/admin/settings/auth/providers$`),
		bodyType: admingen.AdminAuthProviderCreate{},
	},
	{
		// type and slug are immutable, so they are not fields of this body.
		method:     http.MethodPut,
		path:       regexp.MustCompile(`^/api/v1/admin/settings/auth/providers/[^/]+$`),
		bodyType:   admingen.AdminAuthProviderUpdate{},
		requireAll: true,
	},
	{
		method:   http.MethodPost,
		path:     regexp.MustCompile(`^/api/v1/admin/settings/auth/providers/test$`),
		bodyType: admingen.AdminAuthProviderTestRequest{},
	},
	{
		method:     http.MethodPut,
		path:       regexp.MustCompile(`^/api/v1/admin/settings/auth/allowlist$`),
		bodyType:   admingen.AdminAuthAllowlistUpdate{},
		requireAll: true,
	},
	{
		method:     http.MethodPost,
		path:       regexp.MustCompile(`^/api/v1/admin/settings/auth/allowlist/preview$`),
		bodyType:   admingen.AdminAuthAllowlistPreviewRequest{},
		requireAll: true,
	},
	{
		method:   http.MethodPost,
		path:     regexp.MustCompile(`^/api/v1/admin/settings/auth/admins$`),
		bodyType: admingen.AdminInstanceAdminGrant{},
	},
}

// guardedBodyFor returns the guarded-operation entry matching this request.
func guardedBodyFor(r *http.Request) (adminGuardedBody, bool) {
	for _, g := range adminGuardedBodies {
		if r.Method == g.method && g.path.MatchString(r.URL.Path) {
			return g, true
		}
	}
	return adminGuardedBody{}, false
}

// allowedJSONFields returns the set of JSON object keys a struct declares,
// reading the `json` tags of its exported fields.
func allowedJSONFields(v any) map[string]struct{} {
	allowed := make(map[string]struct{})
	for _, f := range jsonFields(v) {
		allowed[f.name] = struct{}{}
	}
	return allowed
}

// requiredJSONFields returns, sorted, the JSON keys a generated request type
// declares without `omitempty`: oapi-codegen adds omitempty to exactly the
// optional properties, so these are the schema's required ones.
func requiredJSONFields(v any) []string {
	var required []string
	for _, f := range jsonFields(v) {
		if !f.omitempty {
			required = append(required, f.name)
		}
	}
	sort.Strings(required)
	return required
}

// jsonField is one exported struct field's JSON key and whether it is
// omitempty.
type jsonField struct {
	name      string
	omitempty bool
}

// jsonFields reads the JSON keys of a struct's exported fields from their
// `json` tags (the Go field name when untagged).
func jsonFields(v any) []jsonField {
	t := reflect.TypeOf(v)
	fields := make([]jsonField, 0, t.NumField())
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		f := jsonField{name: field.Name}
		if tag, ok := field.Tag.Lookup("json"); ok {
			parts := strings.Split(tag, ",")
			if parts[0] == "-" {
				continue
			}
			if parts[0] != "" {
				f.name = parts[0]
			}
			f.omitempty = slices.Contains(parts[1:], "omitempty")
		}
		fields = append(fields, f)
	}
	return fields
}

// rejectUnknownAdminBodyFields is chi middleware that 400s an admin request
// whose JSON body carries a field the operation's schema does not declare, or,
// for a requireAll operation, omits or nulls a required one.
//
// It buffers the body to inspect it and then restores it, so the generated
// decoder downstream still sees a readable stream.
func (s *Server) rejectUnknownAdminBodyFields(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		guard, guarded := guardedBodyFor(r)
		if !guarded || r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError("Failed to read request body"))
			return
		}
		// Restore the body for the generated decoder regardless of the outcome.
		r.Body = io.NopCloser(bytes.NewReader(raw))

		// An empty or non-object body is the generated decoder's problem, not
		// ours; let it produce its usual error.
		if len(bytes.TrimSpace(raw)) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			next.ServeHTTP(w, r)
			return
		}

		if msg := guard.bodyProblem(fields); msg != "" {
			apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msg))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// bodyProblem reports why a decoded JSON object body is rejected, or "" when
// it is acceptable: an unknown key first, then (for a requireAll operation) a
// required key that is absent or null.
func (g adminGuardedBody) bodyProblem(fields map[string]json.RawMessage) string {
	allowed := allowedJSONFields(g.bodyType)
	if unknown := unknownFields(fields, allowed); len(unknown) > 0 {
		return fmt.Sprintf("Unknown or non-editable field(s): %s. Only %s may be sent here.%s",
			strings.Join(unknown, ", "), strings.Join(sortedKeys(allowed), ", "), g.hint)
	}
	if !g.requireAll {
		return ""
	}
	if missing := missingFields(fields, requiredJSONFields(g.bodyType)); len(missing) > 0 {
		return fmt.Sprintf("Missing required field(s): %s. This endpoint replaces the whole "+
			"setting, so every field must be supplied (null is not a value).", strings.Join(missing, ", "))
	}
	return ""
}

// missingFields returns the required keys that are absent from the body or
// explicitly null, in the order given.
func missingFields(body map[string]json.RawMessage, required []string) []string {
	missing := make([]string, 0)
	for _, key := range required {
		raw, ok := body[key]
		if !ok || string(bytes.TrimSpace(raw)) == "null" {
			missing = append(missing, key)
		}
	}
	return missing
}

// unknownFields returns the sorted body keys that are not in the allowed set.
func unknownFields(body map[string]json.RawMessage, allowed map[string]struct{}) []string {
	unknown := make([]string, 0)
	for key := range body {
		if _, ok := allowed[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	return unknown
}

// sortedKeys returns a set's keys in a stable order for error messages.
func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
