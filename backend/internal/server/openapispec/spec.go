// Package openapispec embeds the bundled, self-contained OpenAPI specification
// so the server can serve it at /openapi.yaml and /openapi.json with no runtime
// dependency on a file on disk.
//
// The bundled files are committed generated artifacts: `make
// backend-generate-openapi-bundle` bundles the split source spec (openapi.yaml
// root index + paths/*.yaml + schemas/*.yaml) into self-contained YAML and JSON
// here, and `make backend-openapi-bundle-check` drift-gates the committed bytes
// against a fresh bundle — the same "generated artifact is committed +
// drift-gated" convention as config.schema.json and the oapi-codegen output.
//
// The committed copies live inside this package because go:embed cannot reach
// ../../openapi.yaml (precedent: internal/server/favicon.go embeds its asset the
// same way).
package openapispec

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
)

// YAML is the fully-bundled OpenAPI spec as application/yaml.
//
//go:embed openapi.bundled.yaml
var YAML []byte

// JSON is the fully-bundled OpenAPI spec as application/json (the same document
// as YAML).
//
//go:embed openapi.bundled.json
var JSON []byte

// ETagYAML and ETagJSON are strong ETags over the embedded bytes, computed once
// at package init so handlers can answer conditional requests without rehashing
// on every request.
var (
	ETagYAML = computeETag(YAML)
	ETagJSON = computeETag(JSON)
)

// computeETag returns a strong ETag (a quoted hex SHA-256) over b.
func computeETag(b []byte) string {
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%q", hex.EncodeToString(sum[:]))
}
