import { useCallback, useEffect, useState } from 'react'

import type {
  StatusDefinition,
  StatusResourceType,
} from '@/services/statusService'
import { statusService } from '@/services/statusService'

interface UseStatusesResult {
  /** System statuses first, then the team's custom ones. */
  statuses: StatusDefinition[]
  /** True until the first answer for this team and resource type arrives. */
  isLoading: boolean
  /** The failure of the latest request, or null. */
  error: unknown
  /** Refetches in place: the current list stays on screen meanwhile. */
  reload: () => void
}

interface Loaded {
  key: string
  statuses: StatusDefinition[]
  error: unknown
}

/**
 * The statuses a team may give a resource of `resourceType`.
 *
 * Same contract as `useTypes`, with one deliberate difference: the team is an
 * argument rather than the ambient `useTeam()` one. Team settings pages act on
 * the team resolved from the URL (#584), which on a cold deep-link is not yet
 * the ambient team when the first effect runs.
 */
export function useStatuses(
  teamId: string,
  resourceType: StatusResourceType
): UseStatusesResult {
  const key = `${teamId}:${resourceType}`
  const [loaded, setLoaded] = useState<Loaded | null>(null)
  const [version, setVersion] = useState(0)

  useEffect(() => {
    let cancelled = false
    statusService
      .list(teamId, resourceType)
      .then(statuses => {
        if (!cancelled) setLoaded({ key, statuses, error: null })
      })
      .catch((error: unknown) => {
        if (!cancelled) setLoaded({ key, statuses: [], error })
      })
    return () => {
      cancelled = true
    }
  }, [teamId, resourceType, key, version])

  const reload = useCallback(() => {
    setVersion(current => current + 1)
  }, [])

  // An answer for another team or resource type is not this one's: until the
  // matching answer lands the hook reports loading rather than stale rows.
  const current = loaded?.key === key ? loaded : null

  return {
    statuses: current?.statuses ?? [],
    isLoading: current === null,
    error: current?.error ?? null,
    reload,
  }
}
