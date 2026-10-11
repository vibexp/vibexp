import { act, renderHook, waitFor } from '@testing-library/react'

import type { StatusDefinition } from '@/services/statusService'

const mockList = vi.hoisted(() => vi.fn())
vi.mock('@/services/statusService', () => ({
  statusService: { list: (...args: unknown[]) => mockList(...args) },
}))

import { useStatuses } from '../useStatuses'

const status = (slug: string): StatusDefinition => ({
  resource_type: 'prompt',
  slug,
  name: slug,
  is_system: true,
  is_live: true,
  usage_count: 0,
})

/** A request the test settles by hand. */
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useStatuses', () => {
  it('loads the statuses of the given team and resource type', async () => {
    mockList.mockResolvedValue([status('draft')])

    const { result } = renderHook(() => useStatuses('team-1', 'prompt'))

    expect(result.current.isLoading).toBe(true)
    expect(result.current.statuses).toEqual([])

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })
    expect(mockList).toHaveBeenCalledWith('team-1', 'prompt')
    expect(result.current.statuses).toEqual([status('draft')])
    expect(result.current.error).toBeNull()
  })

  it('reports the failure and an empty list when the request fails', async () => {
    const failure = new Error('statuses down')
    mockList.mockRejectedValue(failure)

    const { result } = renderHook(() => useStatuses('team-1', 'prompt'))

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })
    expect(result.current.error).toBe(failure)
    expect(result.current.statuses).toEqual([])
  })

  it('reload refetches and keeps the current list on screen meanwhile', async () => {
    mockList.mockResolvedValueOnce([status('draft')])
    const { result } = renderHook(() => useStatuses('team-1', 'prompt'))
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })

    const second = deferred<StatusDefinition[]>()
    mockList.mockReturnValueOnce(second.promise)
    act(() => {
      result.current.reload()
    })

    await waitFor(() => {
      expect(mockList).toHaveBeenCalledTimes(2)
    })
    // Still the first answer, and not a loading state: a refetch must not blank
    // the table under an open dialog.
    expect(result.current.isLoading).toBe(false)
    expect(result.current.statuses).toEqual([status('draft')])

    act(() => {
      second.resolve([status('draft'), status('published')])
    })
    await waitFor(() => {
      expect(result.current.statuses).toHaveLength(2)
    })
  })

  it('goes back to loading, without the old rows, when the team changes', async () => {
    mockList.mockResolvedValueOnce([status('draft')])
    const { result, rerender } = renderHook(
      ({ teamId }: { teamId: string }) => useStatuses(teamId, 'prompt'),
      { initialProps: { teamId: 'team-1' } }
    )
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })

    const next = deferred<StatusDefinition[]>()
    mockList.mockReturnValueOnce(next.promise)
    rerender({ teamId: 'team-2' })

    expect(result.current.isLoading).toBe(true)
    expect(result.current.statuses).toEqual([])
    expect(mockList).toHaveBeenLastCalledWith('team-2', 'prompt')

    act(() => {
      next.resolve([status('published')])
    })
    await waitFor(() => {
      expect(result.current.statuses).toEqual([status('published')])
    })
  })

  it('ignores an answer that arrives after the team changed', async () => {
    const slow = deferred<StatusDefinition[]>()
    mockList.mockReturnValueOnce(slow.promise)
    const { result, rerender } = renderHook(
      ({ teamId }: { teamId: string }) => useStatuses(teamId, 'prompt'),
      { initialProps: { teamId: 'team-1' } }
    )

    mockList.mockResolvedValueOnce([status('published')])
    rerender({ teamId: 'team-2' })
    await waitFor(() => {
      expect(result.current.statuses).toEqual([status('published')])
    })

    // team-1's late answer must not replace team-2's list.
    act(() => {
      slow.resolve([status('draft')])
    })
    await Promise.resolve()
    expect(result.current.statuses).toEqual([status('published')])
  })
})
