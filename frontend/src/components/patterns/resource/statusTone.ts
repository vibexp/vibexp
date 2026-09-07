import type { StatusTone } from '@/components/StatusBadge'

import { getResourceDescriptor, type ResourceKindKey } from './registry'
import type { FieldSpec, ResourceDescriptor } from './types'

/*
 * Status presentation, read off the descriptor.
 *
 * Before #903 the tone was inlined at every call site — `status === 'published'
 * ? 'success' : 'warning'` on one page, `memoryStatusTone(...)` on another, a
 * bespoke dot pill on the prompts list — so adding a status value meant hunting
 * them all down. #900 already put `tone` (and now `valueLabels`) on the
 * descriptor's `status` field; these helpers are the only readers, which makes
 * the descriptor the single source and a new status value a one-line change.
 *
 * Lookups go through a `Map` rather than indexing the record with a runtime
 * key: `security/detect-object-injection` flags the latter, and the repo's
 * existing status modules already contort to avoid it.
 */

/** Reads the field's `tone`/`valueLabels` map without a computed property access. */
function lookup(
  map: Readonly<Record<string, string>> | undefined,
  key: string
): string | undefined {
  if (!map) return undefined
  return new Map(Object.entries(map)).get(key)
}

/** The descriptor's `status` field, or `undefined` for a kind that has none. */
export function statusFieldOf(
  descriptor: ResourceDescriptor
): FieldSpec | undefined {
  return descriptor.fields.find(field => field.role === 'status')
}

/**
 * The badge tone for a status value of a given field. Unknown values (a status
 * the server added that the descriptor has not caught up with) fall back to
 * `neutral` rather than to the default badge, so they still read as a status.
 */
export function fieldTone(
  field: FieldSpec | undefined,
  value: string
): StatusTone {
  return (lookup(field?.tone, value) as StatusTone | undefined) ?? 'neutral'
}

/** The display text for a value of a `status` or `type` field. */
export function fieldLabel(
  field: FieldSpec | undefined,
  value: string
): string {
  return lookup(field?.valueLabels, value) ?? value
}

/**
 * The badge tone for a resource kind's status value — the call-site-friendly
 * form, used by list columns that hold a row rather than a descriptor.
 */
export function statusTone(kind: ResourceKindKey, value: string): StatusTone {
  return fieldTone(statusFieldOf(getResourceDescriptor(kind)), value)
}
