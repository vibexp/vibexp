import type { ResourceFormValues } from '@/components/patterns/resource'
import {
  enumValue,
  metadataOrUndefined,
  recordValue,
  stringListValue,
  stringValue,
} from '@/components/patterns/resource'
import type {
  ArtifactStatus,
  CreateArtifactRequest,
} from '@/services/artifactService'

/**
 * The statuses an artifact can be created or updated with.
 *
 * Typed as `ArtifactStatus` rather than inferred, so narrowing the spec enum
 * fails the build here instead of 400-ing at runtime; the descriptor declares
 * the same three values and `buildFormSchema` has already rejected anything
 * outside them by the time this runs.
 */
const ARTIFACT_STATUSES: readonly [ArtifactStatus, ...ArtifactStatus[]] = [
  'active',
  'draft',
  'archived',
]

/**
 * The artifact request body, built from a generated form's parsed values.
 *
 * Shared by `ArtifactCreate` and `ArtifactEdit` because the mapping is the same
 * for both — `UpdateArtifactRequest`'s fields are all optional, so a full
 * create body satisfies it. Building the payload stays with the pages (it is
 * the half `ResourceFormPage` deliberately does not own); this is only what
 * they have in common.
 */
export function toArtifactRequest(
  values: ResourceFormValues
): CreateArtifactRequest {
  return {
    title: stringValue(values, 'title'),
    slug: stringValue(values, 'slug'),
    description: stringValue(values, 'description'),
    project_id: stringValue(values, 'project_id'),
    type: stringValue(values, 'type'),
    status: enumValue(values, 'status', ARTIFACT_STATUSES),
    content: stringValue(values, 'content'),
    labels: stringListValue(values, 'labels'),
    metadata: metadataOrUndefined(recordValue(values, 'metadata')),
  }
}
