import { generatedClient, unwrap } from '@/lib/apiClientGenerated'

import type { KnowledgePulseResponse } from './types'

export interface PulseQuery {
  /** Cursor from the previous response; omit for the initial load. */
  since?: string
  limit?: number
  /** Ask for the snapshot even on a delta poll. */
  snapshot?: boolean
}

/** One knowledge-pulse poll: the initial load without `since`, a delta with it. */
export async function fetchKnowledgePulse(
  teamId: string,
  query: PulseQuery = {},
  signal?: AbortSignal
): Promise<KnowledgePulseResponse> {
  return unwrap(
    generatedClient.GET('/api/v1/{team_id}/knowledge-pulse', {
      params: {
        path: { team_id: teamId },
        query: {
          since: query.since,
          limit: query.limit,
          snapshot: query.snapshot ? true : undefined,
        },
      },
      signal,
    })
  )
}
