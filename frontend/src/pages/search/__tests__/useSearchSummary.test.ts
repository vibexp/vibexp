import { act, renderHook, waitFor } from '@testing-library/react'
import type { Mock } from 'vitest'

import type { SearchFilterType } from '@/services/searchService'

vi.mock('@/services/searchService', () => ({
  searchService: { summarize: vi.fn() },
}))

import { searchService } from '@/services/searchService'

import { useSearchSummary } from '../useSearchSummary'

const mockSummarize = searchService.summarize as Mock

const summary = {
  summary: 'An answer [1]',
  sources: [],
  model: 'gpt-test',
  provider_id: 'prov-1',
  generated_at: '2026-01-01T00:00:00Z',
}

interface Props {
  query: string
  type?: SearchFilterType
  projectId?: string
}

function setup(initial: Props) {
  return renderHook(
    ({ query, type, projectId }: Props) =>
      useSearchSummary('team-1', query, type, projectId),
    { initialProps: initial }
  )
}

describe('useSearchSummary', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockSummarize.mockResolvedValue(summary)
  })

  it('requests once per search, however often generate is called', async () => {
    const { result } = setup({ query: 'q' })

    act(() => {
      result.current.generate()
      result.current.generate()
    })
    await waitFor(() => {
      expect(result.current.state?.status).toBe('ready')
    })
    act(() => {
      result.current.generate()
    })

    expect(mockSummarize).toHaveBeenCalledTimes(1)
    expect(mockSummarize).toHaveBeenCalledWith('team-1', { query: 'q' })
  })

  it('sends the type and project filters but no paging fields', async () => {
    const { result } = setup({ query: 'q', type: 'memories', projectId: 'p1' })

    act(() => {
      result.current.generate()
    })

    await waitFor(() => {
      expect(result.current.state?.status).toBe('ready')
    })
    expect(mockSummarize).toHaveBeenCalledWith('team-1', {
      query: 'q',
      types: ['memories'],
      project_id: 'p1',
    })
  })

  it.each<[string, Props]>([
    ['query', { query: 'other' }],
    ['type', { query: 'q', type: 'prompts' }],
    ['project', { query: 'q', projectId: 'p2' }],
  ])('a %s change is a new summary', async (_, next) => {
    const { result, rerender } = setup({ query: 'q' })
    act(() => {
      result.current.generate()
    })
    await waitFor(() => {
      expect(result.current.state?.status).toBe('ready')
    })

    rerender(next)
    expect(result.current.state).toBeUndefined()
    act(() => {
      result.current.generate()
    })

    await waitFor(() => {
      expect(mockSummarize).toHaveBeenCalledTimes(2)
    })
  })

  it('keeps each search summary when the user comes back to it', async () => {
    const { result, rerender } = setup({ query: 'q' })
    act(() => {
      result.current.generate()
    })
    await waitFor(() => {
      expect(result.current.state?.status).toBe('ready')
    })

    rerender({ query: 'other' })
    rerender({ query: 'q' })

    expect(result.current.state?.status).toBe('ready')
    act(() => {
      result.current.generate()
    })
    expect(mockSummarize).toHaveBeenCalledTimes(1)
  })

  it('stores a classified error and retries on demand', async () => {
    mockSummarize.mockRejectedValueOnce(
      new Error('Network error: Unable to connect to server')
    )
    const { result } = setup({ query: 'q' })

    act(() => {
      result.current.generate()
    })
    await waitFor(() => {
      expect(result.current.state).toMatchObject({
        status: 'error',
        code: 'NETWORK_ERROR',
      })
    })

    // generate does not re-request a search that already has a state...
    act(() => {
      result.current.generate()
    })
    expect(mockSummarize).toHaveBeenCalledTimes(1)

    // ...Retry does.
    act(() => {
      result.current.retry()
    })
    await waitFor(() => {
      expect(result.current.state?.status).toBe('ready')
    })
    expect(mockSummarize).toHaveBeenCalledTimes(2)
  })
})
