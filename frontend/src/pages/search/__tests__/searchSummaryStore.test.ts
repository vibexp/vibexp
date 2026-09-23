import { waitFor } from '@testing-library/react'
import type { Mock } from 'vitest'

vi.mock('@/services/searchService', () => ({
  searchService: { summarize: vi.fn() },
}))

import { searchService } from '@/services/searchService'

import {
  getSearchSummary,
  requestSearchSummary,
  resetSearchSummaries,
  subscribeSearchSummaries,
} from '../searchSummaryStore'

const mockSummarize = searchService.summarize as Mock

const summary = {
  summary: 'An answer',
  sources: [],
  model: 'gpt-test',
  provider_id: 'prov-1',
  generated_at: '2026-01-01T00:00:00Z',
}

function request(key: string) {
  requestSearchSummary(key, 'team-1', { query: key })
}

describe('searchSummaryStore', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetSearchSummaries()
    mockSummarize.mockResolvedValue(summary)
  })

  it('sends one request per key while it is loading', async () => {
    request('a')
    request('a')

    await waitFor(() => {
      expect(getSearchSummary('a')?.status).toBe('ready')
    })
    expect(mockSummarize).toHaveBeenCalledTimes(1)
  })

  it('notifies subscribers until they unsubscribe', async () => {
    const listener = vi.fn()
    const unsubscribe = subscribeSearchSummaries(listener)

    request('a')
    await waitFor(() => {
      expect(getSearchSummary('a')?.status).toBe('ready')
    })
    expect(listener).toHaveBeenCalledTimes(2) // loading, then ready

    unsubscribe()
    request('b')
    expect(listener).toHaveBeenCalledTimes(2)
  })

  it('evicts the oldest settled summaries beyond its cap, never a loading one', async () => {
    mockSummarize.mockReturnValueOnce(new Promise(() => undefined))
    request('pending')
    for (let i = 0; i < 50; i++) request(`k${String(i)}`)
    await waitFor(() => {
      expect(getSearchSummary('k49')?.status).toBe('ready')
    })

    expect(getSearchSummary('pending')).toEqual({ status: 'loading' })
    expect(getSearchSummary('k0')).toBeUndefined()
    expect(getSearchSummary('k1')).toBeDefined()
  })

  it('drops a request that settles after a reset', async () => {
    let resolve: (value: typeof summary) => void = () => undefined
    mockSummarize.mockReturnValueOnce(
      new Promise(r => {
        resolve = r
      })
    )
    request('a')
    resetSearchSummaries()
    resolve(summary)
    await Promise.resolve()
    await Promise.resolve()

    expect(getSearchSummary('a')).toBeUndefined()
  })
})
