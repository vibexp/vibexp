package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
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
}

var adminGuardedBodies = []adminGuardedBody{
	{
		method:   http.MethodPatch,
		path:     regexp.MustCompile(`^/api/v1/admin/users/[^/]+$`),
		bodyType: admingen.AdminUserUpdateRequest{},
	},
}

// guardedBodyFor returns the guarded-operation entry matching this request.
func guardedBodyFor(r *http.Request) (any, bool) {
	for _, g := range adminGuardedBodies {
		if r.Method == g.method && g.path.MatchString(r.URL.Path) {
			return g.bodyType, true
		}
	}
	return nil, false
}

// allowedJSONFields returns the set of JSON object keys a struct declares,
// reading the `json` tags of its exported fields.
func allowedJSONFields(v any) map[string]struct{} {
	allowed := make(map[string]struct{})
	t := reflect.TypeOf(v)
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name := field.Name
		if tag, ok := field.Tag.Lookup("json"); ok {
			if base := strings.Split(tag, ",")[0]; base != "" && base != "-" {
				name = base
			}
		}
		allowed[name] = struct{}{}
	}
	return allowed
}

// rejectUnknownAdminBodyFields is chi middleware that 400s an admin request
// whose JSON body carries a field the operation's schema does not declare.
//
// It buffers the body to inspect it and then restores it, so the generated
// decoder downstream still sees a readable stream.
func (s *Server) rejectUnknownAdminBodyFields(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyType, guarded := guardedBodyFor(r)
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

		if unknown := unknownFields(fields, allowedJSONFields(bodyType)); len(unknown) > 0 {
			apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(fmt.Sprintf(
				"Unknown or non-editable field(s): %s. Only %s may be changed here; "+
					"email and identity-provider fields are owned by the identity provider.",
				strings.Join(unknown, ", "), strings.Join(sortedKeys(allowedJSONFields(bodyType)), ", "),
			)))
			return
		}

		next.ServeHTTP(w, r)
	})
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
