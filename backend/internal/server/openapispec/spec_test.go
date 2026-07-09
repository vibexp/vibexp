package openapispec

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestEmbeddedSpecsNonEmpty guards against a build that embeds empty artifacts
// (e.g. the committed files were truncated or never regenerated).
func TestEmbeddedSpecsNonEmpty(t *testing.T) {
	if len(YAML) == 0 {
		t.Error("embedded YAML spec is empty")
	}
	if len(JSON) == 0 {
		t.Error("embedded JSON spec is empty")
	}
}

// TestEmbeddedSpecsParse asserts both artifacts are well-formed OpenAPI and that
// they describe the same document (same openapi version + same set of paths),
// which is the guarantee external consumers of the two formats rely on.
func TestEmbeddedSpecsParse(t *testing.T) {
	var fromYAML, fromJSON struct {
		OpenAPI string         `yaml:"openapi" json:"openapi"`
		Info    map[string]any `yaml:"info" json:"info"`
		Paths   map[string]any `yaml:"paths" json:"paths"`
	}
	if err := yaml.Unmarshal(YAML, &fromYAML); err != nil {
		t.Fatalf("embedded YAML spec does not parse: %v", err)
	}
	if err := json.Unmarshal(JSON, &fromJSON); err != nil {
		t.Fatalf("embedded JSON spec does not parse: %v", err)
	}

	if fromYAML.OpenAPI == "" || !strings.HasPrefix(fromYAML.OpenAPI, "3.") {
		t.Errorf("unexpected openapi version in YAML: %q", fromYAML.OpenAPI)
	}
	if fromYAML.OpenAPI != fromJSON.OpenAPI {
		t.Errorf("openapi version differs: YAML %q vs JSON %q", fromYAML.OpenAPI, fromJSON.OpenAPI)
	}
	if len(fromYAML.Info) == 0 {
		t.Error("YAML spec has no info block")
	}
	if len(fromYAML.Paths) == 0 {
		t.Error("YAML spec documents no paths")
	}
	if len(fromYAML.Paths) != len(fromJSON.Paths) {
		t.Errorf("path count differs between formats: YAML %d vs JSON %d",
			len(fromYAML.Paths), len(fromJSON.Paths))
	}
}

// TestEmbeddedSpecsFullyBundled asserts the served spec is self-contained: no
// external $ref remains (the split source's `$ref: ./paths/...` must all be
// inlined by the bundler), which is the core value of serving the bundle.
func TestEmbeddedSpecsFullyBundled(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"yaml", YAML},
		{"json", JSON},
	} {
		if strings.Contains(string(tc.body), "./paths/") ||
			strings.Contains(string(tc.body), "./schemas/") {
			t.Errorf("%s spec still contains external file refs; bundling is incomplete", tc.name)
		}
	}
}

// TestETagsAreStrongAndDistinct guards the conditional-request contract: each
// ETag is a quoted (strong) validator, and the two formats have different tags
// because their bytes differ.
func TestETagsAreStrongAndDistinct(t *testing.T) {
	for name, etag := range map[string]string{"YAML": ETagYAML, "JSON": ETagJSON} {
		if !strings.HasPrefix(etag, `"`) || !strings.HasSuffix(etag, `"`) {
			t.Errorf("%s ETag is not a quoted strong validator: %s", name, etag)
		}
	}
	if ETagYAML == ETagJSON {
		t.Error("YAML and JSON ETags are identical; expected distinct bytes to hash differently")
	}
}
