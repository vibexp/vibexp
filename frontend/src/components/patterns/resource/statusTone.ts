import type { StatusTone } from '@/components/StatusBadge'

import { fieldOfRole } from './fieldOfRole'
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
  return fieldOfRole(descriptor, 'status')
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
 * Every value a `status` or closed `type` field can hold, in display order.
 *
 * This is the ONE reader of that vocabulary, so a filter's option list and the
 * page's "is this URL value in the enum?" guard cannot disagree — the failure
 * that shape produces is silent (the control offers a value the page then
 * drops from the request, so the list quietly shows everything).
 *
 * `valueLabels` is deliberately not consulted: it is partial by design, so its
 * keys are a subset, not the value set. A field with no exhaustive list — the
 * artifact `type`, an open string matched against the team's registered
 * types — correctly yields nothing to enumerate.
 */
export function fieldValues(field: FieldSpec | undefined): readonly string[] {
  return field?.statusValues ?? field?.typeValues ?? []
}

/** The values of the descriptor's field in a given role, for a page's guard. */
export function roleValues(
  descriptor: ResourceDescriptor,
  role: FieldSpec['role']
): ReadonlySet<string> {
  return new Set(fieldValues(descriptor.fields.find(f => f.role === role)))
}

/**
 * The badge tone for a resource kind's status value — the call-site-friendly
 * form, used by list columns that hold a row rather than a descriptor.
 */
export function statusTone(kind: ResourceKindKey, value: string): StatusTone {
  return fieldTone(statusFieldOf(getResourceDescriptor(kind)), value)
}

/**
 * The display text for a resource kind's status value — the same
 * call-site-friendly form as `statusTone`, so a badge's tone and its wording
 * come from one place. Falls back to the raw value for a status the descriptor
 * has no label for.
 */
export function statusLabel(kind: ResourceKindKey, value: string): string {
  return fieldLabel(statusFieldOf(getResourceDescriptor(kind)), value)
}
