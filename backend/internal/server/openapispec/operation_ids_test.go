package openapispec

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// operationId uniqueness gate (#573).
//
// This exists because a duplicate operationId is invisible to every gate we had:
//
//   - `make backend-validate-openapi` passes. Each id appears exactly once in
//     paths/*.yaml; the collision only materialises AFTER bundling, when one path
//     item is mounted at two URLs (the settings-prefix aliases).
//   - The route drift gate compares METHOD+path, not operationIds.
//   - `publish-api-client.yml` in this repo reports success — it only DISPATCHES
//     the downstream job, so its green check is not evidence the client published.
//
// The actual failure surfaced two repos away: openapi-typescript keys its
// generated `operations` interface on operationId, so duplicates became duplicate
// TS identifiers and the publish job's tsc gate died before `npm publish`. The
// api-client was silently unpublishable for four merged spec changes.
//
// This runs against the BUNDLED document (which is drift-gated, so it is always
// current) because the unbundled openapi.yaml cannot show the collision.

// httpMethods are the OpenAPI path-item keys that carry an operation. Anything
// else in a path item (parameters, summary, servers, …) is not an operation.
var httpMethods = map[string]bool{
	"get": true, "put": true, "post": true, "delete": true,
	"options": true, "head": true, "patch": true, "trace": true,
}

// bundledPaths parses the embedded YAML bundle into path -> method -> operation.
func bundledPaths(t *testing.T) map[string]map[string]map[string]any {
	t.Helper()

	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(YAML, &doc); err != nil {
		t.Fatalf("embedded YAML spec does not parse: %v", err)
	}
	if len(doc.Paths) == 0 {
		t.Fatal("embedded spec documents no paths")
	}

	out := make(map[string]map[string]map[string]any, len(doc.Paths))
	for path, item := range doc.Paths {
		ops := make(map[string]map[string]any)
		for key, value := range item {
			if !httpMethods[strings.ToLower(key)] {
				continue
			}
			op, ok := value.(map[string]any)
			if !ok {
				t.Fatalf("%s %s: operation is not a mapping", strings.ToUpper(key), path)
			}
			ops[strings.ToLower(key)] = op
		}
		out[path] = ops
	}
	return out
}

// TestNoDuplicateOperationIDs is the gate. An operationId must be unique across
// the whole document: it is the identifier every generator turns into a function
// or type name, so a duplicate is not a style problem — it makes the document
// ungeneratable.
func TestNoDuplicateOperationIDs(t *testing.T) {
	seen := make(map[string][]string)
	missing := make([]string, 0)

	for path, ops := range bundledPaths(t) {
		for method, op := range ops {
			id, _ := op["operationId"].(string)
			if id == "" {
				missing = append(missing, fmt.Sprintf("%s %s", strings.ToUpper(method), path))
				continue
			}
			seen[id] = append(seen[id], fmt.Sprintf("%s %s", strings.ToUpper(method), path))
		}
	}

	duplicates := make([]string, 0)
	for id, locations := range seen {
		if len(locations) > 1 {
			sort.Strings(locations)
			duplicates = append(duplicates, fmt.Sprintf("  %q is used by: %s", id, strings.Join(locations, ", ")))
		}
	}
	sort.Strings(duplicates)

	if len(duplicates) > 0 {
		t.Errorf(
			"%d duplicate operationId(s) in the bundled spec — this makes the document "+
				"ungeneratable and BREAKS the api-client publish (#573):\n%s\n\n"+
				"Mounting one path item at two URLs reuses its operationIds. Give each mount "+
				"its own path item with suffixed ids (see the settings-prefix aliases in "+
				"paths/github-app-config.yaml).",
			len(duplicates), strings.Join(duplicates, "\n"))
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("operation(s) without an operationId — generated clients name these badly:\n  %s",
			strings.Join(missing, "\n  "))
	}
}

// settingsAliases maps an alias path to the canonical path it duplicates.
//
// Both URLs are genuinely served (see githubAppConfigMountPrefixes and
// teamEmailProviderMountPrefixes in server.go): the integrations/bare prefix
// groups the surface with its domain, the settings prefix is what the SPA calls.
// Keeping both means two hand-maintained copies of the same operations, and two
// copies drift — usually discovered much later, by a client consumer.
var settingsAliases = map[string]string{
	"/api/v1/{team_id}/settings/github-app":                      "/api/v1/{team_id}/integrations/github/app",
	"/api/v1/{team_id}/settings/github-app/validate":             "/api/v1/{team_id}/integrations/github/app/validate",
	"/api/v1/{team_id}/settings/github-app/rotate-webhook-token": "/api/v1/{team_id}/integrations/github/app/rotate-webhook-token",
	"/api/v1/{team_id}/settings/email-provider":                  "/api/v1/{team_id}/email-provider",
	"/api/v1/{team_id}/settings/email-provider/test":             "/api/v1/{team_id}/email-provider/test",
}

// TestSettingsAliasPathItemsMatchCanonical asserts each alias is identical to its
// canonical counterpart apart from the operationId. Without this, the duplication
// forced by operationId uniqueness silently becomes divergence: an endpoint that
// behaves one way on one prefix and another way on the other, documented as both.
func TestSettingsAliasPathItemsMatchCanonical(t *testing.T) {
	paths := bundledPaths(t)

	for alias, canonical := range settingsAliases {
		aliasOps, ok := paths[alias]
		if !ok {
			t.Errorf("alias path %q is not in the spec — remove it from settingsAliases or document it", alias)
			continue
		}
		canonOps, ok := paths[canonical]
		if !ok {
			t.Errorf("canonical path %q is not in the spec (alias %q points at nothing)", canonical, alias)
			continue
		}

		if len(aliasOps) != len(canonOps) {
			t.Errorf("%q documents %d operation(s) but its canonical %q documents %d — the mounts have drifted",
				alias, len(aliasOps), canonical, len(canonOps))
			continue
		}

		for method, canonOp := range canonOps {
			aliasOp, ok := aliasOps[method]
			if !ok {
				t.Errorf("%q has no %s, but canonical %q does", alias, strings.ToUpper(method), canonical)
				continue
			}

			aliasID, _ := aliasOp["operationId"].(string)
			canonID, _ := canonOp["operationId"].(string)
			if aliasID == canonID {
				t.Errorf("%s %q reuses the canonical operationId %q — that is the duplicate this gate exists to prevent",
					strings.ToUpper(method), alias, canonID)
			}

			// Compare everything else. The ids are expected to differ, so they
			// are the only key excluded.
			if !reflect.DeepEqual(withoutOperationID(aliasOp), withoutOperationID(canonOp)) {
				t.Errorf("%s %q has drifted from its canonical %q — the two mounts serve the same handler, "+
					"so their documentation must stay identical apart from operationId",
					strings.ToUpper(method), alias, canonical)
			}
		}
	}
}

func withoutOperationID(op map[string]any) map[string]any {
	out := make(map[string]any, len(op))
	for k, v := range op {
		if k == "operationId" {
			continue
		}
		out[k] = v
	}
	return out
}
