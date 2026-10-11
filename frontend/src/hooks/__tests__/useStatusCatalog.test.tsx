import { act, renderHook, waitFor } from '@testing-library/react'

import type { StatusDefinition } from '@/services/statusService'

// The suite-wide default (tests/setup.ts) replaces this module; this is the
// one suite that is about the real thing.
vi.unmock('@/hooks/useStatusCatalog')

const mockList = vi.hoisted(() => vi.fn())
const team = vi.hoisted((): { current: { id: string } | null } => ({
  current: { id: 'team-1' },
}))

vi.mock('@/services/statusService', () => ({
  statusService: { list: mockList },
}))

vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({ currentTeam: team.current }),
}))

import {
  FRESH_FOR_MS,
  resetStatusCatalog,
  useStatusCatalog,
} from '../useStatusCatalog'

function status(slug: string, name: string): StatusDefinition {
  return {
    id: `id-${slug}`,
    resource_type: 'artifact',
    slug,
    name,
    is_system: false,
    is_live: true,
    usage_count: 0,
  }
}

const IN_REVIEW = status('in-review', 'In review')
const SHIPPED = status('shipped', 'Shipped')

/** A request the test settles by hand, to observe the state in between. */
function deferred() {
  let resolve: (statuses: StatusDefinition[]) => void = () => {}
  let reject: (error: unknown) => void = () => {}
  const promise = new Promise<StatusDefinition[]>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('useStatusCatalog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetStatusCatalog()
    team.current = { id: 'team-1' }
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('is loading until the first answer arrives, then holds it', async () => {
    const request = deferred()
    mockList.mockReturnValue(request.promise)

    const { result } = renderHook(() => useStatusCatalog('artifact'))
    expect(result.current).toEqual({ statuses: [], isLoading: true })
    expect(mockList).toHaveBeenCalledWith('team-1', 'artifact')

    act(() => {
      request.resolve([IN_REVIEW])
    })
    await waitFor(() => {
      expect(result.current).toEqual({
        statuses: [IN_REVIEW],
        isLoading: false,
      })
    })
  })

  it('asks once for every reader of the same team and resource type', async () => {
    mockList.mockResolvedValue([IN_REVIEW])

    const first = renderHook(() => useStatusCatalog('artifact'))
    const second = renderHook(() => useStatusCatalog('artifact'))

    await waitFor(() => {
      expect(second.result.current.statuses).toEqual([IN_REVIEW])
    })
    expect(first.result.current.statuses).toEqual([IN_REVIEW])
    expect(mockList).toHaveBeenCalledTimes(1)
  })

  it('keeps one answer per resource type', async () => {
    mockList.mockImplementation((_team: string, resourceType: string) =>
      Promise.resolve(resourceType === 'artifact' ? [IN_REVIEW] : [SHIPPED])
    )

    const artifact = renderHook(() => useStatusCatalog('artifact'))
    const memory = renderHook(() => useStatusCatalog('memory'))

    await waitFor(() => {
      expect(artifact.result.current.statuses).toEqual([IN_REVIEW])
    })
    await waitFor(() => {
      expect(memory.result.current.statuses).toEqual([SHIPPED])
    })
    expect(mockList).toHaveBeenCalledTimes(2)
  })

  it('asks again for another team and does not show the first team’s answer', async () => {
    mockList.mockResolvedValueOnce([IN_REVIEW])
    const { result, rerender } = renderHook(() => useStatusCatalog('artifact'))
    await waitFor(() => {
      expect(result.current.statuses).toEqual([IN_REVIEW])
    })

    const request = deferred()
    mockList.mockReturnValue(request.promise)
    team.current = { id: 'team-2' }
    rerender()

    expect(result.current).toEqual({ statuses: [], isLoading: true })
    expect(mockList).toHaveBeenLastCalledWith('team-2', 'artifact')
  })

  it('treats a failed request as an answer: nothing known, not loading', async () => {
    mockList.mockRejectedValue(new Error('boom'))

    const { result } = renderHook(() => useStatusCatalog('artifact'))

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })
    expect(result.current.statuses).toEqual([])
  })

  it('reuses a fresh answer for a reader that mounts soon after', async () => {
    mockList.mockResolvedValue([IN_REVIEW])
    const first = renderHook(() => useStatusCatalog('artifact'))
    await waitFor(() => {
      expect(first.result.current.isLoading).toBe(false)
    })

    const second = renderHook(() => useStatusCatalog('artifact'))

    expect(second.result.current).toEqual({
      statuses: [IN_REVIEW],
      isLoading: false,
    })
    expect(mockList).toHaveBeenCalledTimes(1)
  })

  // What lets a status created on the settings page show up in the next form
  // with no invalidation call between the two pages.
  it('asks again for a reader that mounts after the answer went stale, showing the old one meanwhile', async () => {
    const now = vi.spyOn(Date, 'now').mockReturnValue(1_000)
    mockList.mockResolvedValueOnce([IN_REVIEW])
    const first = renderHook(() => useStatusCatalog('artifact'))
    await waitFor(() => {
      expect(first.result.current.isLoading).toBe(false)
    })

    const request = deferred()
    mockList.mockReturnValue(request.promise)
    now.mockReturnValue(1_000 + FRESH_FOR_MS)
    const second = renderHook(() => useStatusCatalog('artifact'))

    expect(mockList).toHaveBeenCalledTimes(2)
    expect(second.result.current).toEqual({
      statuses: [IN_REVIEW],
      isLoading: false,
    })

    act(() => {
      request.resolve([IN_REVIEW, SHIPPED])
    })
    await waitFor(() => {
      expect(second.result.current.statuses).toEqual([IN_REVIEW, SHIPPED])
    })
    expect(first.result.current.statuses).toEqual([IN_REVIEW, SHIPPED])
    now.mockRestore()
  })

  it('keeps the last answer when a refresh fails', async () => {
    const now = vi.spyOn(Date, 'now').mockReturnValue(1_000)
    mockList.mockResolvedValueOnce([IN_REVIEW])
    const first = renderHook(() => useStatusCatalog('artifact'))
    await waitFor(() => {
      expect(first.result.current.isLoading).toBe(false)
    })

    mockList.mockRejectedValue(new Error('boom'))
    now.mockReturnValue(1_000 + FRESH_FOR_MS)
    const second = renderHook(() => useStatusCatalog('artifact'))

    await waitFor(() => {
      expect(mockList).toHaveBeenCalledTimes(2)
    })
    // Let the rejection settle before reading.
    await act(async () => {
      await Promise.resolve()
    })
    expect(second.result.current).toEqual({
      statuses: [IN_REVIEW],
      isLoading: false,
    })
    now.mockRestore()
  })

  it('stays loading, and asks nothing, while the team is unresolved', () => {
    team.current = null

    const { result } = renderHook(() => useStatusCatalog('artifact'))

    expect(result.current).toEqual({ statuses: [], isLoading: true })
    expect(mockList).not.toHaveBeenCalled()
  })

  it('has nothing to load for a kind with no catalog', () => {
    const { result } = renderHook(() => useStatusCatalog(undefined))

    expect(result.current).toEqual({ statuses: [], isLoading: false })
    expect(mockList).not.toHaveBeenCalled()
  })
})
