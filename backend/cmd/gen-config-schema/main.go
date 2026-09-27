// Command gen-config-schema generates backend/config.schema.json from the nested
// config.Config struct. The JSON Schema gives editors (VS Code / JetBrains via
// the YAML language server) validation and autocomplete for config.yaml and
// config.example.yaml.
//
// The output is committed and drift-checked in CI
// (make backend-config-schema-check); never hand-edit it — change the Config
// struct and regenerate with `make backend-generate-config-schema`.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/invopop/jsonschema"

	"github.com/vibexp/vibexp/internal/config"
)

// outputPath is relative to the backend/ module root (the generator is invoked
// as `cd backend && go run ./cmd/gen-config-schema`).
const outputPath = "config.schema.json"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen-config-schema:", err)
		os.Exit(1)
	}
}

func run() error {
	r := &jsonschema.Reflector{
		// The config structs carry koanf tags, not json tags, so derive every
		// property name from the koanf tag — otherwise Go field names would leak
		// into the schema and never match the YAML keys.
		FieldNameTag: "koanf",
		// Require only fields explicitly tagged `jsonschema:"required"` (none are):
		// every field has a code default or is optional, and the loader validates
		// the real invariants at startup. This keeps a minimal, partial config.yaml
		// free of spurious "missing property" diagnostics. additionalProperties is
		// left at invopop's default of false so editors flag typo'd keys.
		RequiredFromJSONSchemaTags: true,
		// Disambiguate $defs keys by package: the embedded event_bus and otel
		// sections are both types literally named "Config" (in pkg/events and
		// internal/observability), which would otherwise collide with the root
		// config.Config and make event_bus/otel $ref the wrong definition.
		Namer: defName,
	}

	// Render time.Duration as a Go duration string ("15m", "720h", "200ms"),
	// matching how config.yaml expresses durations (the loader coerces them via a
	// StringToTimeDuration decode hook). Without this the schema would demand an
	// integer (Duration's underlying int64) and reject the example file's strings.
	r.Mapper = func(t reflect.Type) *jsonschema.Schema {
		if t == reflect.TypeFor[time.Duration]() {
			return &jsonschema.Schema{
				Type: "string",
				// A literal Go duration, OR a ${VAR:-default} env placeholder — the
				// combined-image config.docker.yaml expresses every operator knob as
				// a placeholder and is validated against this schema before
				// interpolation (like every other string knob, which has no pattern).
				Pattern:     `^(\$\{[^}]+\}|-?(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+)$`,
				Description: `Go duration string, e.g. "15m", "720h", "200ms".`,
			}
		}
		if t == reflect.TypeFor[config.EnvStringSlice]() {
			// A YAML string array, OR a single ${VAR:-default} placeholder holding a
			// comma-separated list — config.docker.yaml authors these env-driven
			// lists as one placeholder that koanf splits at load, and is validated
			// against this schema before interpolation (cf. the time.Duration
			// mapper above).
			return &jsonschema.Schema{
				OneOf: []*jsonschema.Schema{
					{Type: "array", Items: &jsonschema.Schema{Type: "string"}},
					{Type: "string", Pattern: `^\$\{[^}]+\}$`},
				},
			}
		}
		if t == reflect.TypeFor[config.EnvBool]() {
			// A literal YAML bool, OR a single ${VAR:-default} placeholder.
			// Decoding a placeholder already works (the loader's
			// WeaklyTypedInput coerces the interpolated string), so this arm
			// exists only so the RAW config.docker.yaml — which is
			// schema-validated before interpolation — passes. Scoped to the
			// named type on purpose: every plain bool keeps the strict schema.
			return &jsonschema.Schema{
				OneOf: []*jsonschema.Schema{
					{Type: "boolean"},
					{Type: "string", Pattern: `^\$\{[^}]+\}$`},
				},
			}
		}
		if t == reflect.TypeFor[config.EnvInt]() {
			// Integer counterpart of the EnvBool mapper above.
			return &jsonschema.Schema{
				OneOf: []*jsonschema.Schema{
					{Type: "integer"},
					{Type: "string", Pattern: `^\$\{[^}]+\}$`},
				},
			}
		}
		return nil
	}

	schema := r.Reflect(&config.Config{})
	if err := markLegacySectionsDeprecated(schema); err != nil {
		return err
	}
	schema.Title = "VibeXP backend configuration (config.yaml)"
	schema.Description = "Schema for VibeXP's config.yaml. Generated from the Go config.Config struct " +
		"by backend/cmd/gen-config-schema; do not edit by hand."

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal schema: %w", err)
	}
	data = append(data, '\n')

	// 0o600 keeps gosec happy; git records the committed file as 0644 regardless.
	if err := os.WriteFile(outputPath, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", outputPath, err)
	}
	fmt.Printf("gen-config-schema: wrote %s (%d bytes)\n", outputPath, len(data))
	return nil
}

// legacyDeprecations describes the deprecated root sections, keyed by their
// config.yaml name: `email:` (#1190), `search:` and `ai_summary:` (#1201).
// invopop/jsonschema has a Schema.Deprecated field but no struct-tag keyword
// for it, so it is set here after reflection.
var legacyDeprecations = []struct{ property, description string }{
	{"email", "Deprecated: imported into the database once at boot when no instance email " +
		"provider is stored, ignored afterwards, and removed in the next minor release. " +
		"Configure instance mail under Admin → Settings → Email."},
	{"search", "Deprecated: imported into the database once at boot when it differs from the " +
		"built-in defaults and no instance search settings are stored, ignored afterwards, and " +
		"removed in the next minor release. Configure search ranking under Admin → Settings → Search."},
	{"ai_summary", "Deprecated: imported into the database once at boot when it differs from the " +
		"built-in defaults and no instance AI summary settings are stored, ignored afterwards, and " +
		"removed in the next minor release (max_top_n and max_output_tokens_ceiling are ignored). " +
		"Configure AI summaries under Admin → Settings → AI Summary."},
}

// markLegacySectionsDeprecated flags the root config's deprecated properties
// so editors strike them through.
func markLegacySectionsDeprecated(schema *jsonschema.Schema) error {
	root, ok := schema.Definitions["Config"]
	if !ok || root.Properties == nil {
		return fmt.Errorf("mark sections deprecated: no Config definition")
	}
	for _, d := range legacyDeprecations {
		section, ok := root.Properties.Get(d.property)
		if !ok {
			return fmt.Errorf("mark sections deprecated: Config has no %s property", d.property)
		}
		section.Deprecated = true
		section.Description = d.description
	}
	return nil
}

// defName names a $defs entry for a type, qualifying types outside the primary
// config package with their package to avoid collisions (e.g. pkg/events.Config
// and internal/observability.Config both have the bare name "Config"). Types in
// the config package keep their already-unique names (ServerConfig, Config, …).
func defName(t reflect.Type) string {
	name := t.Name()
	pkg := t.PkgPath()
	if pkg == "" || strings.HasSuffix(pkg, "/internal/config") {
		return name
	}
	base := pkg[strings.LastIndex(pkg, "/")+1:]
	if base == "" {
		return name
	}
	return strings.ToUpper(base[:1]) + base[1:] + name
}
