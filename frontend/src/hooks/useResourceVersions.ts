import { useEffect, useMemo, useState } from 'react'

import type { VersionHistoryMeta } from '@/components/metadata/MetadataPanel'
import type {
  ContentVersion,
  ResourceVersionListResponse,
} from '@/types/version'

export interface UseResourceVersionsOptions {
  /**
   * Loads the resource's version snapshots, or `null` while the caller still
   * lacks the context to ask (no team resolved yet, no slug/id in the route).
   * Callers build it inline — the effect keys off `deps`, not off this
   * callback's identity, so it needs no memoization.
   */
  loadVersions: (() => Promise<ResourceVersionListResponse>) | null
  /**
   * react-router target for the "View version history" footer link, or
   * `undefined` until the resource itself has loaded (the route is built from
   * its payload). No affordance is produced without it.
   */
  to: string | undefined
  /** The resource's `updated_at` — rendered beside the version number. */
  editedAt?: string
  /**
   * Identity of what is being loaded (team, slug, id, readiness). A change
   * discards the snapshots held for the previous identity and reloads.
   */
  deps: readonly unknown[]
}

export interface ResourceVersionsState {
  versions: ContentVersion[]
  versionHistory: VersionHistoryMeta | undefined
  loading: boolean
}

/**
 * Empties the list, preserving the array identity when it is already empty so
 * `useState` can bail out instead of paying a wasted render.
 */
const clearVersions = (previous: ContentVersion[]): ContentVersion[] =>
  previous.length === 0 ? previous : []

/**
 * Loads a resource's version snapshots and derives the Metadata panel's
 * version-history affordance from them.
 *
 * Extracted so the four versioned resource detail pages share one
 * implementation instead of four copies of the same fetch-then-derive block
 * (#905). Three properties are the point:
 *
 * - **The stale-response guard now applies to every caller.** Three of the four
 *   pages had it; the prompt page did not, so a slow response for a previously
 *   viewed slug could land afterwards and render that prompt's version count.
 * - **A change of identity drops the snapshots it belonged to.** The pages share
 *   one component instance across a route-param change (the routes carry no
 *   `key`), and the affordance's `to` follows the new resource immediately — so
 *   keeping the old count would pair one resource's history with another's link
 *   for as long as the reload takes.
 * - **The load stays best-effort.** The list only powers the panel's footer link
 *   and count chip, so a failure resolves to "no history" instead of surfacing
 *   as a page error — a 404/403 on `/versions` must never break the resource
 *   view itself.
 */
export function useResourceVersions({
  loadVersions,
  to,
  editedAt,
  deps,
}: UseResourceVersionsOptions): ResourceVersionsState {
  const [versions, setVersions] = useState<ContentVersion[]>([])
  const [loading, setLoading] = useState(loadVersions !== null)

  useEffect(() => {
    // Anything held belongs to the identity that has just been replaced.
    setVersions(clearVersions)
    if (!loadVersions) {
      setLoading(false)
      return
    }
    // Guard against stale responses: when `deps` change mid-flight, a slower
    // earlier request must not overwrite the newer resource's version state.
    let active = true
    setLoading(true)
    const load = async () => {
      try {
        const history = await loadVersions()
        if (active) setVersions(history.versions)
      } catch {
        if (active) setVersions(clearVersions)
      } finally {
        if (active) setLoading(false)
      }
    }
    void load()
    return () => {
      active = false
    }
    // The effect keys off the caller-declared `deps`, not off `loadVersions`:
    // callers build that closure inline, so a fresh identity every render would
    // reload forever. `exhaustive-deps` cannot statically verify a non-literal
    // list and warns; the list is the `deps` option's contract.
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
