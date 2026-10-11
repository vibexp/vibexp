import { useEffect, useSyncExternalStore } from 'react'

import { useTeam } from '@/contexts/TeamContext'
import type {
  StatusDefinition,
  StatusResourceType,
} from '@/services/statusService'
import { statusService } from '@/services/statusService'

/*
 * The team's status catalog, shared by every reader on the page.
 *
 * A status badge sits in every row of a list, next to the filter that offers
 * the same vocabulary and the URL guard that checks against it. Each of those
 * fetching for itself would be one request per row, so the answer is held here
 * once per team and resource type and every mounted reader subscribes to it.
 *
 * `useStatuses` reads the same endpoint for the settings page that manages
 * the catalog, and is deliberately not this: it takes an explicit team (the
 * one in the settings URL), fetches for its one caller, and exposes the error
 * and a `reload` — what an editor of the list needs, and the opposite of what
 * dozens of read-only consumers on one page need.
 *
 * It is a cache with a short memory on purpose. A reader that mounts more than
 * `FRESH_FOR_MS` after the last request started asks again, and keeps showing
 * the answer it has until the new one lands — so a status created on the
 * settings page is offered by the next form that opens, with no invalidation
 * call for the two pages to keep in step.
 */

/** How long an answer is reused without asking again. */
export const FRESH_FOR_MS = 5000

interface Entry {
  /** The last answer, kept while a newer request is in flight or has failed. */
  readonly statuses: readonly StatusDefinition[] | null
  readonly inFlight: boolean
  /** The latest request failed. */
  readonly failed: boolean
  readonly startedAt: number
}

const entries = new Map<string, Entry>()
const listeners = new Set<() => void>()

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

function store(key: string, entry: Entry) {
  entries.set(key, entry)
  listeners.forEach(listener => {
    listener()
  })
}

function request(
  key: string,
  teamId: string,
  resourceType: StatusResourceType
) {
  const current = entries.get(key)
  if (current?.inFlight) return
  if (current && Date.now() - current.startedAt < FRESH_FOR_MS) return
  const startedAt = Date.now()
  const previous = current?.statuses ?? null
  store(key, { statuses: previous, inFlight: true, failed: false, startedAt })
  statusService
    .list(teamId, resourceType)
    .then(statuses => {
      store(key, { statuses, inFlight: false, failed: false, startedAt })
    })
    .catch(() => {
      store(key, {
        statuses: previous,
        inFlight: false,
        failed: true,
        startedAt,
      })
    })
}

/** Forgets every answer. For tests, which share this module between cases. */
export function resetStatusCatalog() {
  entries.clear()
}

const NO_STATUSES: readonly StatusDefinition[] = []

export interface UseStatusCatalogResult {
  /** System statuses first, then the team's custom ones. Empty until known. */
  statuses: readonly StatusDefinition[]
  /**
   * True until the first answer for this team and resource type arrives —
   * including while the team itself is still resolving. A failed request is an
   * answer: the catalog is then unknown, not pending.
   */
  isLoading: boolean
}

/**
 * The statuses the current team may give a resource of `resourceType`.
 *
 * `undefined` is for a kind that has no catalog (an agent's status is a closed
 * enum): nothing is fetched and nothing is ever loading.
 */
export function useStatusCatalog(
  resourceType: StatusResourceType | undefined
): UseStatusCatalogResult {
  const teamId = useTeam().currentTeam?.id
  const key = teamId && resourceType ? `${teamId}:${resourceType}` : undefined

  useEffect(() => {
    if (key && teamId && resourceType) request(key, teamId, resourceType)
  }, [key, teamId, resourceType])

  const entry = useSyncExternalStore(subscribe, () =>
    key ? entries.get(key) : undefined
  )

  if (!resourceType) return { statuses: NO_STATUSES, isLoading: false }
  return {
    statuses: entry?.statuses ?? NO_STATUSES,
    isLoading: !entry || (entry.statuses === null && !entry.failed),
  }
}
