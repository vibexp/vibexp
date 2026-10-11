import type { ResourceFormValues } from '@/components/patterns/resource'
import {
  enumValue,
  recordValue,
  stringListValue,
  stringValue,
} from '@/components/patterns/resource'
import type {
  Blueprint,
  CreateBlueprintRequest,
} from '@/services/blueprintService'

type BlueprintType = Blueprint['type']

/** The closed type enum, unlike the artifact's runtime type catalog. */
const BLUEPRINT_TYPES: readonly [BlueprintType, ...BlueprintType[]] = [
  'general',
  'claude-code',
  'claude',
  'cursor',
  'codex',
]

/**
 * The blueprint request body, built from a generated form's parsed values.
 * Shared by `BlueprintCreate` and `BlueprintEdit` — see `toArtifactRequest`,
 * including for why `status` is sent as the form holds it.
 */
export function toBlueprintRequest(
  values: ResourceFormValues
): CreateBlueprintRequest {
  return {
    title: stringValue(values, 'title'),
    slug: stringValue(values, 'slug'),
    description: stringValue(values, 'description'),
    project_id: stringValue(values, 'project_id'),
    type: enumValue(values, 'type', BLUEPRINT_TYPES),
    status: stringValue(values, 'status'),
    content: stringValue(values, 'content'),
    labels: stringListValue(values, 'labels'),
    metadata: recordValue(values, 'metadata'),
  }
}

/**
 * A sub-agents blueprint must carry a `model` metadata key
 * (`internal/services/blueprint.go`), so the metadata editor must not let it be
 * deleted or renamed. Module-level because `ResourceFormReadingPage` documents
 * `metadataRequiredKeys` as needing a stable reference.
 */
const SUB_AGENTS_REQUIRED_KEYS = ['model']

/** The metadata keys the editor locks for a blueprint, if any. */
export function requiredMetadataKeys(
  blueprint: Blueprint | null
): string[] | undefined {
  return blueprint?.subtype === 'sub-agents'
    ? SUB_AGENTS_REQUIRED_KEYS
    : undefined
}
