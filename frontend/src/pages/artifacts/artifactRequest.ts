import type { ResourceFormValues } from '@/components/patterns/resource'
import {
  recordValue,
  stringListValue,
  stringValue,
} from '@/components/patterns/resource'
import type { CreateArtifactRequest } from '@/services/artifactService'

/**
 * The artifact request body, built from a generated form's parsed values.
 *
 * `status` is sent exactly as the form holds it: it is a system status or one
 * of the team's custom ones (#1263), which only the server can check, so
 * narrowing it here against a fixed list would silently swap a custom status
 * for a default.
 *
 * Shared by `ArtifactCreate` and `ArtifactEdit` because the mapping is the same
 * for both — `UpdateArtifactRequest`'s fields are all optional, so a full
 * create body satisfies it. Building the payload stays with the pages (it is
 * the half `ResourceFormReadingPage` deliberately does not own); this is only what
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
    status: stringValue(values, 'status'),
    content: stringValue(values, 'content'),
    labels: stringListValue(values, 'labels'),
    metadata: recordValue(values, 'metadata'),
  }
}
