import { beforeEach, describe, expect, it, vi } from 'vitest'

const mockGet = vi.hoisted(() => vi.fn())
vi.mock('@/lib/apiClientGenerated', () => ({
  generatedClient: { GET: mockGet },
  unwrap: async (p: Promise<{ data: unknown }>) => (await p).data,
}))

import { fetchKnowledgePulse } from '../api'

describe('fetchKnowledgePulse', () => {
  beforeEach(() => {
    mockGet.mockReset()
    mockGet.mockResolvedValue({ data: { cursor: 'c' } })
  })

  it('first load sends no cursor and no snapshot flag', async () => {
    const res = await fetchKnowledgePulse('team-1')
    expect(res).toEqual({ cursor: 'c' })
    expect(mockGet).toHaveBeenCalledWith('/api/v1/{team_id}/knowledge-pulse', {
      params: {
        path: { team_id: 'team-1' },
        query: { since: undefined, limit: undefined, snapshot: undefined },
      },
      signal: undefined,
    })
  })

  it('a delta poll passes the cursor, limit, snapshot and signal', async () => {
    const signal = new AbortController().signal
    await fetchKnowledgePulse(
      'team-1',
      { since: 'c1', limit: 50, snapshot: true },
      signal
    )
    expect(mockGet).toHaveBeenCalledWith('/api/v1/{team_id}/knowledge-pulse', {
      params: {
        path: { team_id: 'team-1' },
        query: { since: 'c1', limit: 50, snapshot: true },
      },
      signal,
    })
  })
})
