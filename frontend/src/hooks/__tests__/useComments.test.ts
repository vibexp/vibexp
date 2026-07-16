import { act, renderHook, waitFor } from '@testing-library/react'

import { useComments } from '@/hooks/useComments'
import type { Comment } from '@/services/commentService'
import { commentService } from '@/services/commentService'
import type { TeamMember } from '@/services/teamService'
import { teamService } from '@/services/teamService'

jest.mock('@/services/commentService', () => ({
  commentService: {
    list: jest.fn(),
    create: jest.fn(),
    update: jest.fn(),
    remove: jest.fn(),
  },
}))

jest.mock('@/services/teamService', () => ({
  teamService: { getTeamMembers: jest.fn() },
}))

const mockList = commentService.list as jest.Mock
const mockCreate = commentService.create as jest.Mock
const mockUpdate = commentService.update as jest.Mock
const mockRemove = commentService.remove as jest.Mock
const mockMembers = teamService.getTeamMembers as jest.Mock

function makeComment(overrides: Partial<Comment> = {}): Comment {
  return {
    id: 'c1',
    team_id: 'team-1',
    resource_type: 'artifact',
    resource_id: 'res-1',
    user_id: 'user-1',
    content: 'Hello',
    created_at: '2026-07-16T09:00:00Z',
    updated_at: '2026-07-16T09:00:00Z',
    ...overrides,
  }
}

const member: TeamMember = {
  user_id: 'user-1',
  email: 'alice@example.com',
  name: 'Alice',
  role: 'member',
  joined_at: '2026-01-01T00:00:00Z',
}

function page(
  comments: Comment[],
  totalCount: number,
  totalPages: number,
  pageNum = 1
) {
  return {
    comments,
    total_count: totalCount,
    page: pageNum,
    per_page: 5,
    total_pages: totalPages,
  }
}

beforeEach(() => {
  jest.clearAllMocks()
  mockMembers.mockResolvedValue([member])
})

function render() {
  return renderHook(() => useComments('team-1', 'artifact', 'res-1'))
}

describe('useComments', () => {
  it('loads the first page and resolves members', async () => {
    mockList.mockResolvedValueOnce(page([makeComment()], 1, 1))
    const { result } = render()

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })
    expect(result.current.comments).toHaveLength(1)
    expect(result.current.totalCount).toBe(1)
    expect(result.current.members.get('user-1')?.name).toBe('Alice')
    expect(result.current.hasMore).toBe(false)
    expect(result.current.error).toBe(false)
  })

  it('sets error when the initial load fails', async () => {
    mockList.mockRejectedValueOnce(new Error('boom'))
    const { result } = render()

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })
    expect(result.current.error).toBe(true)
    expect(result.current.comments).toHaveLength(0)
  })

  it('appends the next page on loadMore, deduped, and terminates', async () => {
    // 8 total over 2 pages of 5. hasMore is derived from totalCount, not a
    // server total_pages field.
    mockList
      .mockResolvedValueOnce(
        page(
          [
            makeComment({ id: 'a' }),
            makeComment({ id: 'b' }),
            makeComment({ id: 'c' }),
            makeComment({ id: 'd' }),
            makeComment({ id: 'e' }),
          ],
          8,
          2,
          1
        )
      )
      // page 2 re-includes 'e' (a prepend shifted the window) plus new rows
      .mockResolvedValueOnce(
        page(
          [
            makeComment({ id: 'e' }),
            makeComment({ id: 'f' }),
            makeComment({ id: 'g' }),
          ],
          8,
          2,
          2
        )
      )
    const { result } = render()
    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })
    expect(result.current.hasMore).toBe(true)

    await act(async () => {
      result.current.loadMore()
      await Promise.resolve()
    })

    await waitFor(() => {
      expect(result.current.comments).toHaveLength(7)
    })
    // 'e' is not duplicated
    expect(result.current.comments.map(c => c.id)).toEqual([
      'a',
      'b',
      'c',
      'd',
      'e',
      'f',
      'g',
    ])
    // page 2 of 2 loaded → no further pages
    expect(result.current.hasMore).toBe(false)
  })

  it('prepends a created comment and bumps the total', async () => {
    mockList.mockResolvedValueOnce(page([makeComment({ id: 'a' })], 1, 1))
    mockCreate.mockResolvedValueOnce(
      makeComment({ id: 'new', content: 'Fresh' })
    )
    const { result } = render()
    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    await act(async () => {
      await result.current.addComment('Fresh')
    })

    expect(mockCreate).toHaveBeenCalledWith('team-1', {
      resource_type: 'artifact',
      resource_id: 'res-1',
      content: 'Fresh',
    })
    expect(result.current.comments[0].id).toBe('new')
    expect(result.current.totalCount).toBe(2)
  })

  it('replaces a comment in place on edit', async () => {
    mockList.mockResolvedValueOnce(
      page([makeComment({ id: 'a', content: 'Old' })], 1, 1)
    )
    mockUpdate.mockResolvedValueOnce(
      makeComment({
        id: 'a',
        content: 'New',
        updated_at: '2026-07-16T10:00:00Z',
      })
    )
    const { result } = render()
    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    await act(async () => {
      await result.current.editComment('a', 'New')
    })

    expect(mockUpdate).toHaveBeenCalledWith('team-1', 'a', { content: 'New' })
    expect(result.current.comments[0].content).toBe('New')
    expect(result.current.totalCount).toBe(1)
  })

  it('removes a comment and decrements the total', async () => {
    mockList.mockResolvedValueOnce(
      page([makeComment({ id: 'a' }), makeComment({ id: 'b' })], 2, 1)
    )
    mockRemove.mockResolvedValueOnce(undefined)
    const { result } = render()
    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    await act(async () => {
      await result.current.removeComment('a')
    })

    expect(mockRemove).toHaveBeenCalledWith('team-1', 'a')
    expect(result.current.comments.map(c => c.id)).toEqual(['b'])
    expect(result.current.totalCount).toBe(1)
  })
})
