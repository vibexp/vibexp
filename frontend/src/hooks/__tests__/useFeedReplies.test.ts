import { act, renderHook, waitFor } from '@testing-library/react'
import type { Mock } from 'vitest'

import { REPLIES_PAGE_SIZE, useFeedReplies } from '@/hooks/useFeedReplies'
import type {
  FeedItemReply,
  FeedItemReplyListResponse,
} from '@/services/feedService'
import { feedService } from '@/services/feedService'
import type { TeamMember } from '@/services/teamService'
import { teamService } from '@/services/teamService'

vi.mock('@/services/feedService', () => ({
  feedService: {
    listReplies: vi.fn(),
    createReply: vi.fn(),
  },
}))

vi.mock('@/services/teamService', () => ({
  teamService: { getTeamMembers: vi.fn() },
}))

const mockHandleError = vi.hoisted(() => vi.fn())
vi.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

const mockList = feedService.listReplies as Mock
const mockCreate = feedService.createReply as Mock
const mockMembers = teamService.getTeamMembers as Mock

function makeReply(overrides: Partial<FeedItemReply> = {}): FeedItemReply {
  return {
    id: 'r1',
    team_id: 'team-1',
    feed_item_id: 'item-1',
    content: 'Hello',
    posted_by_user_id: 'user-1',
    ai_assistant_name: null,
    posted_at: '2026-09-12T09:00:00Z',
    ...overrides,
  }
}

/** `count` replies with ids `${prefix}0`, `${prefix}1`, … */
function makeReplies(count: number, prefix = 'r'): FeedItemReply[] {
  return Array.from({ length: count }, (_, i) =>
    makeReply({ id: `${prefix}${String(i)}` })
  )
}

function page(
  replies: FeedItemReply[],
  totalCount: number,
  pageNum = 1
): FeedItemReplyListResponse {
  return {
    replies,
    total_count: totalCount,
    page: pageNum,
    per_page: REPLIES_PAGE_SIZE,
    total_pages: Math.ceil(totalCount / REPLIES_PAGE_SIZE),
  }
}

const member: TeamMember = {
  user_id: 'user-1',
  email: 'alice@example.com',
  name: 'Alice',
  role: 'member',
  joined_at: '2026-01-01T00:00:00Z',
}

beforeEach(() => {
  vi.clearAllMocks()
  mockMembers.mockResolvedValue([member])
})

function render(itemId = 'item-1') {
  return renderHook(({ id }) => useFeedReplies('team-1', id), {
    initialProps: { id: itemId },
  })
}

describe('useFeedReplies', () => {
  it('loads the first page with an explicit page/limit and resolves members', async () => {
    mockList.mockResolvedValueOnce(page([makeReply()], 1))
    const { result } = render()

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })
    expect(mockList).toHaveBeenCalledTimes(1)
    expect(mockList).toHaveBeenCalledWith(
      'team-1',
      'item-1',
      1,
      REPLIES_PAGE_SIZE
    )
    expect(result.current.replies).toHaveLength(1)
    expect(result.current.totalCount).toBe(1)
    expect(result.current.members.get('user-1')?.name).toBe('Alice')
    expect(result.current.hasMore).toBe(false)
  })

  it('leaves an empty thread when the initial load fails', async () => {
    mockList.mockRejectedValueOnce(new Error('boom'))
    const { result } = render()

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })
    expect(result.current.replies).toHaveLength(0)
    expect(result.current.totalCount).toBe(0)
    expect(result.current.hasMore).toBe(false)
  })

  it('appends later pages on loadMore, deduped, until every reply is loaded', async () => {
    // 23 replies over 3 pages of 10. Page 2 re-includes r9 (a prepend shifted
    // the server's window), which must not render twice.
    mockList
      .mockResolvedValueOnce(page(makeReplies(10), 23, 1))
      .mockResolvedValueOnce(
        page([makeReply({ id: 'r9' }), ...makeReplies(9, 's')], 23, 2)
      )
      .mockResolvedValueOnce(page(makeReplies(4, 't'), 23, 3))
    const { result } = render()
    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })
    expect(result.current.hasMore).toBe(true)

    act(() => {
      result.current.loadMore()
    })
    await waitFor(() => {
      expect(result.current.replies).toHaveLength(19)
    })
    expect(result.current.hasMore).toBe(true)
    expect(mockList).toHaveBeenLastCalledWith(
      'team-1',
      'item-1',
      2,
      REPLIES_PAGE_SIZE
    )

    act(() => {
      result.current.loadMore()
    })
    await waitFor(() => {
      expect(result.current.replies).toHaveLength(23)
    })
    const ids = result.current.replies.map(r => r.id)
    expect(new Set(ids).size).toBe(ids.length)
    expect(result.current.hasMore).toBe(false)

    // page 3 of ceil(23/10) = 3 loaded → loadMore is a no-op
    act(() => {
      result.current.loadMore()
    })
    expect(mockList).toHaveBeenCalledTimes(3)
  })

  it('ignores a second loadMore while one is in flight', async () => {
    let resolvePage2: (v: FeedItemReplyListResponse) => void = () => undefined
    mockList
      .mockResolvedValueOnce(page(makeReplies(10), 15, 1))
      .mockImplementationOnce(
        () =>
          new Promise<FeedItemReplyListResponse>(r => {
            resolvePage2 = r
          })
      )
    const { result } = render()
    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    act(() => {
      result.current.loadMore()
      result.current.loadMore()
    })
    expect(result.current.loadingMore).toBe(true)
    expect(mockList).toHaveBeenCalledTimes(2)

    await act(async () => {
      resolvePage2(page(makeReplies(5, 's'), 15, 2))
      await Promise.resolve()
    })
    await waitFor(() => {
      expect(result.current.loadingMore).toBe(false)
    })
    expect(result.current.replies).toHaveLength(15)
  })

  it('reports a failed loadMore and keeps what was loaded', async () => {
    mockList
      .mockResolvedValueOnce(page(makeReplies(10), 15, 1))
      .mockRejectedValueOnce(new Error('network'))
    const { result } = render()
    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    act(() => {
      result.current.loadMore()
    })
    await waitFor(() => {
      expect(mockHandleError).toHaveBeenCalledWith(
        expect.any(Error),
        'Failed to load more replies'
      )
    })
    expect(result.current.loadingMore).toBe(false)
    expect(result.current.replies).toHaveLength(10)
    // Still more to fetch, so the user can retry.
    expect(result.current.hasMore).toBe(true)
  })

  it('never lets a slower request for a previous item overwrite the current one', async () => {
    let resolveStale: (v: FeedItemReplyListResponse) => void = () => undefined
    mockList
      .mockImplementationOnce(
        () =>
          new Promise<FeedItemReplyListResponse>(r => {
            resolveStale = r
          })
      )
      .mockResolvedValueOnce(page([makeReply({ id: 'current' })], 1))
    const { result, rerender } = render('item-1')

    rerender({ id: 'item-2' })
    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })
    expect(result.current.replies.map(r => r.id)).toEqual(['current'])

    await act(async () => {
      resolveStale(page(makeReplies(10, 'stale'), 40))
      await Promise.resolve()
    })
    expect(result.current.replies.map(r => r.id)).toEqual(['current'])
    expect(result.current.totalCount).toBe(1)
  })

  it('prepends a created reply and bumps the total without refetching', async () => {
    mockList.mockResolvedValueOnce(page([makeReply({ id: 'a' })], 1))
    mockCreate.mockResolvedValueOnce(makeReply({ id: 'new', content: 'Fresh' }))
    const { result } = render()
    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    await act(async () => {
      await result.current.addReply('Fresh')
    })

    expect(mockCreate).toHaveBeenCalledWith('team-1', 'item-1', {
      content: 'Fresh',
    })
    expect(result.current.replies.map(r => r.id)).toEqual(['new', 'a'])
    expect(result.current.totalCount).toBe(2)
    expect(mockList).toHaveBeenCalledTimes(1)
  })

  it('propagates a failed create without touching the list', async () => {
    mockList.mockResolvedValueOnce(page([makeReply({ id: 'a' })], 1))
    mockCreate.mockRejectedValueOnce(new Error('nope'))
    const { result } = render()
    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    await act(async () => {
      await expect(result.current.addReply('x')).rejects.toThrow('nope')
    })
    expect(result.current.replies).toHaveLength(1)
    expect(result.current.totalCount).toBe(1)
  })
})
