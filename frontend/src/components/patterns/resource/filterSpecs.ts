import type { FilterSpec } from './types'

/**
 * Builders for the filter specs every resource list shares.
 *
 * Four descriptors declaring the same search / status / freshness / metadata
 * entries by hand is the duplication #908 set out to remove — it had simply
 * moved from the filter components into the descriptors. Declarative
 * duplication is cheaper than component duplication, but it drifts the same
 * way, and Sonar's new-code duplication gate says so out loud.
 *
 * Resource-specific entries (the artifact type catalog, the prompt taxonomy)
 * stay written out on their descriptor: they are the part that differs, and a
 * builder used once is just indirection.
 */

/** Free-text search over the resource's own text. Not backed by a field. */
export function searchFilter(plural: string): FilterSpec {
  return { key: 'search', control: 'search', label: `Search ${plural}` }
}

/** The lifecycle badge, read off the kind's own `status` field. */
export function statusFilter(kind: string): FilterSpec {
  return {
    key: 'status',
    control: 'select',
    label: 'Filter by status',
    allLabel: 'All statuses',
    optionsFrom: 'field',
    testId: `${kind}-status-filter`,
  }
}

/** "Stale only", from the freshness rules (#738). Not backed by a field. */
export function freshnessFilter(kind: string, plural: string): FilterSpec {
  return {
    key: 'freshness',
    control: 'freshness',
    label: `Filter ${plural} by freshness`,
    testId: `${kind}-freshness-filter`,
  }
}

/** The free-form key/value bag. Not backed by a field the bar can enumerate. */
export function metadataFilter(plural: string): FilterSpec {
  return {
    key: 'metadata',
    control: 'metadata',
    label: `Filter ${plural} by metadata`,
  }
}
