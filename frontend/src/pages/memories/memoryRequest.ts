import type { ResourceFormValues } from '@/components/patterns/resource'
import {
  enumValue,
  metadataOrUndefined,
  recordValue,
  stringListValue,
  stringValue,
} from '@/components/patterns/resource'
import type {
  CreateMemoryRequest,
  Memory,
  MemoryStatus,
} from '@/services/memoryService'

const MEMORY_STATUSES: readonly [MemoryStatus, ...MemoryStatus[]] = [
  'active',
  'draft',
  'archived',
]

/**
 * The metadata key the Tags chip editor owns.
 *
 * A memory has no `tags` FIELD — the chips edit `metadata.tags` — so the
 * metadata editor must hide the key rather than let it be edited twice. Kept at
 * module scope because `ResourceFormPage` documents `metadataReservedKeys` as
 * needing a stable reference.
 */
export const RESERVED_METADATA_KEYS = ['tags']

/** The `tags` a memory's metadata carries, if any. */
export function extractTags(meta?: Record<string, unknown>): string[] {
  const tags = meta?.tags
  if (!Array.isArray(tags)) return []
  return tags.filter((tag): tag is string => typeof tag === 'string')
}

/** Everything in a memory's metadata EXCEPT the lifted `tags` key. */
export function extractExtras(
  meta?: Record<string, unknown>
): Record<string, unknown> {
  const entries = Object.entries(meta ?? {}).filter(
    ([key]) => !RESERVED_METADATA_KEYS.includes(key)
  )
  return Object.fromEntries(entries)
}

/**
 * A fetched memory as form values.
 *
 * Unlike the other kinds the resource cannot be handed to `ResourceFormPage`
 * unchanged: `metadata` is shown by two controls at once — the chip card owns
 * `tags` and the metadata editor owns the rest — so the form's copy must not
 * carry `tags` or the reserved key would come back through the editor.
 */
export function memoryInitialValues(memory: Memory): ResourceFormValues {
  return { ...memory, metadata: extractExtras(memory.metadata) }
}

/**
 * What an empty title field means, which is the one asymmetry between creating
 * a memory and editing one: per the spec, omitting the key on an update leaves
 * the title unchanged while `null` clears it — so an edit that empties the
 * field must send the explicit null, and a create with no title must send
 * nothing at all.
 */
export type EmptyTitle = 'omit' | 'clear'

/**
 * The memory request body, built from a generated form's parsed values plus the
 * page-owned tag chips, which are folded back into `metadata`.
 */
export function toMemoryRequest(
  values: ResourceFormValues,
  tags: string[],
  emptyTitle: EmptyTitle
): CreateMemoryRequest {
  const metadata = extractExtras(recordValue(values, 'metadata'))
  if (tags.length > 0) metadata.tags = tags
  const request: CreateMemoryRequest = {
    project_id: stringValue(values, 'project_id'),
    text: stringValue(values, 'text'),
    status: enumValue(values, 'status', MEMORY_STATUSES),
    labels: stringListValue(values, 'labels'),
    metadata: metadataOrUndefined(metadata),
  }
  const title = stringValue(values, 'title')
  if (title !== '') request.title = title
  else if (emptyTitle === 'clear') request.title = null
  return request
}
