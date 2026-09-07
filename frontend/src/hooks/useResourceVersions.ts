import { useEffect, useMemo, useState } from 'react'

import type { VersionHistoryMeta } from '@/components/metadata/MetadataPanel'

/**
 * The one field the derivation reads from a version snapshot. Every resource's
 * version type satisfies it (they are all aliases of the shared
 * `ResourceVersion`), so callers keep their own concrete types.
 */
export interface ResourceVersionLike {
  version_number: number
}

export interface UseResourceVersionsOptions<T extends ResourceVersionLike> {
  /**
   * Loads the resource's version snapshots, or `null` while the caller still
   * lacks the context to ask (no team resolved yet, no slug/id in the route).
   * Callers re-create it on every render by design — the effect keys off
   * `deps`, not off this callback's identity, so it needs no memoization.
   */
  fetch: (() => Promise<{ versions: T[] }>) | null
  /**
   * react-router target for the "View version history" footer link, or
   * `undefined` until the resource itself has loaded (the route is built from
   * its payload). No affordance is produced without it.
   */
  to: string | undefined
  /** The resource's `updated_at` — rendered beside the version number. */
  editedAt?: string
  /**
   * Identity of what is being fetched (team, slug, id, readiness). A change
   * re-fetches and invalidates whatever was already in flight.
   */
  deps: readonly unknown[]
}

export interface ResourceVersionsState<T> {
  versions: T[]
  versionHistory: VersionHistoryMeta | undefined
  loading: boolean
}

/**
 * Loads a resource's version snapshots and derives the Metadata panel's
 * version-history affordance from them.
 *
 * Extracted so the four versioned resource detail pages share one
 * implementation instead of four copies of the same fetch-then-derive block
 * (#905). Two properties are the point:
 *
 * - **The stale-response guard now applies to every caller.** Three of the four
 *   pages had it; the prompt page did not, so a slow response for a previously
 *   viewed slug could land afterwards and render that prompt's version count.
 * - **The load stays best-effort.** The list only powers the panel's footer
 *   link and count chip, so a failure resolves to "no history" instead of
 *   surfacing as a page error — a 404/403 on `/versions` must never break the
 *   resource view itself.
 */
export function useResourceVersions<T extends ResourceVersionLike>({
  fetch,
  to,
  editedAt,
  deps,
}: UseResourceVersionsOptions<T>): ResourceVersionsState<T> {
  const [versions, setVersions] = useState<T[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!fetch) {
      // Drop whatever a previous identity left behind — but preserve the array
      // identity when there is nothing to drop. Handing `useState` a fresh `[]`
      // here forces a re-render on every mount, which re-runs the caller's own
      // detail effect a beat earlier than it used to.
      setVersions(previous => (previous.length === 0 ? previous : []))
      setLoading(false)
      return
    }
    // Guard against stale responses: when `deps` change mid-flight, a slower
    // earlier request must not overwrite the newer resource's version state.
    let active = true
    setLoading(true)
    const load = async () => {
      try {
        const history = await fetch()
        if (active) setVersions(history.versions)
      } catch {
        if (active) setVersions([])
      } finally {
        if (active) setLoading(false)
      }
    }
    void load()
    return () => {
      active = false
    }
    // The effect keys off the caller-declared `deps`, not off `fetch`: callers
    // build that closure inline, so a fresh identity every render would refetch
    // forever. `exhaustive-deps` cannot statically verify a non-literal list and
    // warns; the list is the documented contract of the `deps` option.
  }, deps)

  const versionHistory = useMemo<VersionHistoryMeta | undefined>(() => {
    // Only surface the affordance once there is history to show and the route
    // it links to is known; a "0" chip linking to an empty page would mislead.
    if (versions.length === 0 || to === undefined) return undefined
    // Snapshots capture the *prior* content and version numbers are monotonic
    // (never reused, oldest pruned past the retention cap), so the live
    // resource's version is one past the highest retained snapshot number.
    // `versions.length` is the number of entries shown on the linked history
    // page — the chip count.
    const latestVersionNumber = versions.reduce(
      (max, v) => Math.max(max, v.version_number),
      0
    )
    return {
      count: versions.length,
      currentVersion: latestVersionNumber + 1,
      editedAt,
      to,
    }
  }, [versions, to, editedAt])

  return { versions, versionHistory, loading }
}
