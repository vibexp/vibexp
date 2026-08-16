package server

import (
	"github.com/vibexp/vibexp/internal/models"
)

// Write-response shapes for the two domains whose models carry keys their
// schemas never declared (#800, epic #122).
//
// `models.Artifact` and `models.Blueprint` both serialize `team_id` (no
// omitempty, so on every body) and `version`, and neither appears in
// `schemas/artifacts.yaml` / `schemas/blueprints.yaml`. The read paths moved to
// generated types in #776/#801 and #778/#802, whose structs have no field for
// either, so both keys silently left the read bodies. The write paths still
// marshal the models directly, so create-then-read returned two different
// shapes for one resource. These wrappers close that.
//
// # Why a wrapper rather than the alternatives
//
// Adding the fields to the schemas was rejected: it is a spec change, so it
// fires `publish-api-client.yml` and auto-minor-bumps BOTH generated clients
// for two keys nothing reads.
//
// Tagging the model fields `json:"-"` was rejected too, and is explicitly out
// of bounds: `TeamID` and `Version` are read by non-API paths and their `db`
// tags are load-bearing.
//
// Reusing the read path's `toGenArtifact` / `toGenBlueprint` converters was the
// issue's preferred option and is NOT used here, deliberately. Those produce
// the full generated type, which would also drop `related`/`similar` from
// `[]` to absent and — on blueprints — drop `raw_content`, which the write
// paths populate (`services.regenerateRaw`) and which is declared only on
// `BlueprintDetail`. Only `team_id` and `version` were decided on #800, with
// the blast radius checked for those two; widening the removal to a field
// blueprint sync may depend on is a separate decision, not a side effect of
// this one. A wrapper removes exactly what was decided and leaves every other
// key byte-identical.
//
// # How the shadowing works
//
// The wrapper embeds the model and redeclares the two fields at depth 0 with
// the SAME json name plus omitempty, left at their zero values. encoding/json
// resolves the name conflict in favour of the shallower field, which then omits
// itself.
//
// Note the tempting spelling `json:"-"` does NOT work: such fields are excluded
// from conflict resolution entirely, so the embedded field wins and the key is
// still emitted. Verified both ways before this was written, and pinned by
// TestWriteResponseShapesDropUndeclaredKeys — an assertion that would otherwise
// pass against a wrapper that does nothing.

// artifactRESTWriteBody is `models.Artifact` minus the two undeclared keys.
type artifactRESTWriteBody struct {
	*models.Artifact
	TeamID  string `json:"team_id,omitempty"`
	Version int64  `json:"version,omitempty"`
}

// blueprintRESTWriteBody is `models.Blueprint` minus the two undeclared keys.
type blueprintRESTWriteBody struct {
	*models.Blueprint
	TeamID  string `json:"team_id,omitempty"`
	Version int64  `json:"version,omitempty"`
}

// artifactWriteBody wraps an artifact for a create/update/restore response.
func artifactWriteBody(a *models.Artifact) artifactRESTWriteBody {
	return artifactRESTWriteBody{Artifact: a}
}

// blueprintWriteBody wraps a blueprint for a create/update/restore response.
func blueprintWriteBody(b *models.Blueprint) blueprintRESTWriteBody {
	return blueprintRESTWriteBody{Blueprint: b}
}
