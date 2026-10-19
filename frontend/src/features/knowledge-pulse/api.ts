import { createTimeoutFetch, unwrap } from '@/lib/apiClientGenerated'
import { getApiBaseUrl } from '@/utils/environment'

import type { KnowledgePulseResponse } from './types'

const REQUEST_TIMEOUT_MS = 15000
const timeoutFetch = createTimeoutFetch(REQUEST_TIMEOUT_MS)

export interface PulseQuery {
  /** Cursor from the previous response; omit for the initial load. */
  since?: string
  limit?: number
  /** Ask for the snapshot even on a delta poll. */
  snapshot?: boolean
}

/**
 * Typed fetch for the knowledge-pulse endpoint. It stands in for the
 * generated client until the frontend adopts the `@vibexp/api-client` release
 * that includes this path (it publishes automatically once the spec change
 * lands on main); it reuses `unwrap` so errors map exactly like every other
 * service call. Replace it with `generatedClient.GET(...)` then.
 */
export async function fetchKnowledgePulse(
  teamId: string,
  query: PulseQuery = {},
  signal?: AbortSignal
): Promise<KnowledgePulseResponse> {
  const params = new URLSearchParams()
  if (query.since) params.set('since', query.since)
  if (query.limit) params.set('limit', String(query.limit))
  if (query.snapshot) params.set('snapshot', '1')
  const qs = params.toString()
  const url = `${getApiBaseUrl()}/${encodeURIComponent(teamId)}/knowledge-pulse${qs ? `?${qs}` : ''}`

  return unwrap(
    (async () => {
      const response = await timeoutFetch(
        new Request(url, { credentials: 'include', signal })
      )
      const body: unknown = await response.json().catch(() => undefined)
      return response.ok
        ? { data: body as KnowledgePulseResponse, response }
        : { error: body, response }
    })()
  )
}
