import { act, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { isLoopRunning, knowledgeLoopOf } from '../knowledgeLoop'
import {
  KnowledgeLoopStatus,
  LOOP_REFRESH_MS,
} from '../steps/KnowledgeLoopStatus'

const mockAuth = vi.hoisted(() => ({
  user: null as object | null,
  refreshUser: vi.fn<() => Promise<void>>(),
}))
vi.mock('@/contexts/useAuth', () => ({ useAuth: () => mockAuth }))

const READ_AT = '2026-10-15T10:00:00Z'
const WRITE_AT = '2026-10-15T10:05:00Z'

function setLoop(first_read_at: string | null, first_write_at: string | null) {
  mockAuth.user = {
    id: 'u1',
    knowledge_loop: { first_read_at, first_write_at },
  }
}

const rows = () =>
  within(screen.getByRole('list', { name: 'Knowledge loop' })).getAllByRole(
    'listitem'
  )

beforeEach(() => {
  vi.useFakeTimers()
  mockAuth.refreshUser.mockReset().mockResolvedValue(undefined)
  setLoop(null, null)
})

afterEach(() => {
  vi.useRealTimers()
})

describe('KnowledgeLoopStatus', () => {
  it('shows both steps pending, with what to run', () => {
    render(<KnowledgeLoopStatus />)
    // Native list semantics, not ARIA roles.
    expect(screen.getByRole('list', { name: 'Knowledge loop' }).tagName).toBe(
      'UL'
    )
    const [read, write] = rows()
    expect(read.tagName).toBe('LI')
    expect(read).toHaveAttribute('data-state', 'pending')
    expect(read).toHaveTextContent('Your agent read from the shared brain')
    expect(read).toHaveTextContent('Not yet: run /vibexp:prime in a project')
    expect(write).toHaveAttribute('data-state', 'pending')
    expect(write).toHaveTextContent(
      "Not yet: /vibexp:wrap saves what's worth keeping"
    )
  })

  it('shows a done step with when it happened', () => {
    setLoop(READ_AT, null)
    render(<KnowledgeLoopStatus />)
    const [read, write] = rows()
    expect(read).toHaveAttribute('data-state', 'done')
    expect(within(read).getByRole('time')).toHaveAttribute(
      'datetime',
      new Date(READ_AT).toISOString()
    )
    expect(write).toHaveAttribute('data-state', 'pending')
  })

  it('re-reads the user when it opens, then every 10s while pending', () => {
    render(<KnowledgeLoopStatus />)
    expect(mockAuth.refreshUser).toHaveBeenCalledTimes(1)
    act(() => {
      vi.advanceTimersByTime(LOOP_REFRESH_MS)
    })
    expect(mockAuth.refreshUser).toHaveBeenCalledTimes(2)
  })

  it('does not re-read in a background tab', () => {
    const hidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    render(<KnowledgeLoopStatus />)
    act(() => {
      vi.advanceTimersByTime(LOOP_REFRESH_MS * 3)
    })
    expect(mockAuth.refreshUser).toHaveBeenCalledTimes(1)
    hidden.mockRestore()
  })

  it('stops refreshing once the loop is complete, and when it closes', () => {
    const view = render(<KnowledgeLoopStatus />)
    setLoop(READ_AT, WRITE_AT)
    view.rerender(<KnowledgeLoopStatus />)
    expect(rows().map(r => r.getAttribute('data-state'))).toEqual([
      'done',
      'done',
    ])
    act(() => {
      vi.advanceTimersByTime(LOOP_REFRESH_MS * 3)
    })
    expect(mockAuth.refreshUser).toHaveBeenCalledTimes(1)

    setLoop(null, null)
    view.rerender(<KnowledgeLoopStatus />)
    view.unmount()
    act(() => {
      vi.advanceTimersByTime(LOOP_REFRESH_MS * 3)
    })
    expect(mockAuth.refreshUser).toHaveBeenCalledTimes(1)
  })
})

describe('knowledgeLoopOf', () => {
  it('reads as not yet without a signed-in user', () => {
    expect(knowledgeLoopOf(null)).toEqual({
      first_read_at: null,
      first_write_at: null,
    })
  })

  it('is running only once both have happened', () => {
    expect(
      isLoopRunning({ first_read_at: READ_AT, first_write_at: null })
    ).toBe(false)
    expect(
      isLoopRunning({ first_read_at: READ_AT, first_write_at: WRITE_AT })
    ).toBe(true)
  })
})
