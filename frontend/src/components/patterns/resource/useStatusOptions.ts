import { useMemo } from 'react'

import type { StatusTone } from '@/components/StatusBadge'
import { useStatusCatalog } from '@/hooks/useStatusCatalog'
import type { StatusResourceType } from '@/services/statusService'

import { fieldLabel, fieldTone, fieldValues, statusFieldOf } from './statusTone'
import type { ResourceDescriptor } from './types'

/*
 * A resource's status vocabulary: the descriptor's system values plus the
 * team's custom statuses.
 *
 * The descriptor stays the baseline — it owns the system values, their order,
 * their tones and their labels — and the catalog adds what the team defined at
 * runtime (#1263). This hook is the ONE reader of the merged vocabulary, for
 * the same reason `statusTone.ts` is the one reader of the baseline: a form
 * select, a list filter, the page's URL guard and a badge that each merged for
 * themselves would disagree silently, and the failure that shape produces is a
 * filter offering a value the page then drops from the request.
 */

/** The kinds whose status is a team catalog, keyed by the kind itself. */
const CATALOG_RESOURCE_TYPES: ReadonlyMap<string, StatusResourceType> = new Map(
  [
    ['prompt', 'prompt'],
    ['artifact', 'artifact'],
    ['blueprint', 'blueprint'],
    ['memory', 'memory'],
  ]
)

/**
 * The catalog a kind's status is read from, or `undefined` for a kind whose
 * status is closed (an agent's `active | paused | error` is not a catalog).
 */
export function statusResourceTypeOf(
  kind: string
): StatusResourceType | undefined {
  return CATALOG_RESOURCE_TYPES.get(kind)
}

export interface StatusOption {
  value: string
  label: string
}

export interface StatusOptions {
  /** System values in descriptor order, then the team's custom statuses. */
  options: readonly StatusOption[]
  /** The value is a system status or one of the team's custom statuses. */
  has: (value: string) => boolean
  /**
   * A value from outside the app — a URL parameter — that cannot be judged
   * yet: not a system status, and the catalog has not answered. It may be one
   * of the team's custom statuses or junk, so a caller neither acts on it (the
   * API answers an unknown status with a 400) nor drops it (that would strip a
   * custom status filter from a reloaded page) — it waits. Once the catalog
   * has answered nothing is pending: a value is known, or it is dropped.
   */
  isPending: (value: string) => boolean
  /** Descriptor label, else the custom status's display name, else the raw value. */
  labelOf: (value: string) => string
  /** The descriptor's tone for a system value; `neutral` for everything else. */
  toneOf: (value: string) => StatusTone
  /** The catalog has not answered yet; `options` holds the system values only. */
  isLoading: boolean
}

/** The status vocabulary of the descriptor's kind for the current team. */
export function useStatusOptions(
  descriptor: ResourceDescriptor
): StatusOptions {
  const { statuses, isLoading } = useStatusCatalog(
    statusResourceTypeOf(descriptor.kind)
  )

  return useMemo(() => {
    const field = statusFieldOf(descriptor)
    const system = fieldValues(field)
    const names = new Map<string, string>()
    for (const status of statuses) {
      // The catalog lists the system statuses too; the descriptor already
      // carries those, with the app's own wording.
      if (!status.is_system && !system.includes(status.slug)) {
        names.set(status.slug, status.name)
      }
    }
    const known = new Set([...system, ...names.keys()])
    const labelOf = (value: string) =>
      system.includes(value)
        ? fieldLabel(field, value)
        : (names.get(value) ?? value)
    return {
      options: [...known].map(value => ({ value, label: labelOf(value) })),
      has: value => known.has(value),
      isPending: value => isLoading && !known.has(value),
      labelOf,
      toneOf: value => fieldTone(field, value),
      isLoading,
    }
  }, [descriptor, statuses, isLoading])
}
