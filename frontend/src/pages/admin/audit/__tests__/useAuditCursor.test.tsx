import { act, renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import {
  auditCursorReducer,
  INITIAL_CURSOR_STATE,
  useAuditCursor,
} from '../useAuditCursor'

describe('auditCursorReducer', () => {
  it('pushes on next and pops on previous', () => {
    let state = auditCursorReducer(INITIAL_CURSOR_STATE, {
      type: 'next',
      cursor: 'c1',
    })
    state = auditCursorReducer(state, { type: 'next', cursor: 'c2' })
    expect(state.stack).toEqual(['c1', 'c2'])
    state = auditCursorReducer(state, { type: 'previous' })
    expect(state.stack).toEqual(['c1'])
  })

  it('stays on the first page when there is nothing to go back to', () => {
    expect(auditCursorReducer(INITIAL_CURSOR_STATE, { type: 'previous' })).toBe(
      INITIAL_CURSOR_STATE
    )
  })

  it('resets to the first page', () => {
    const state = auditCursorReducer({ stack: ['c1', 'c2'] }, { type: 'reset' })
    expect(state).toEqual(INITIAL_CURSOR_STATE)
  })
})

describe('useAuditCursor', () => {
  it('walks forward and back with the cursor each page was loaded with', () => {
    const { result } = renderHook(() => useAuditCursor('filters-a'))
    expect(result.current).toMatchObject({
      cursor: undefined,
      pageNumber: 1,
      hasPrevious: false,
    })

    act(() => {
      result.current.next('c1')
    })
    act(() => {
      result.current.next('c2')
    })
    expect(result.current).toMatchObject({
      cursor: 'c2',
      pageNumber: 3,
      hasPrevious: true,
    })

    act(() => {
      result.current.previous()
    })
    expect(result.current.cursor).toBe('c1')
    act(() => {
      result.current.previous()
    })
    expect(result.current).toMatchObject({ cursor: undefined, pageNumber: 1 })
  })

  it('starts over when the filters change', () => {
    const { result, rerender } = renderHook(({ key }) => useAuditCursor(key), {
      initialProps: { key: 'filters-a' },
    })
    act(() => {
      result.current.next('c1')
    })
    expect(result.current.pageNumber).toBe(2)

    rerender({ key: 'filters-b' })

    expect(result.current).toMatchObject({ cursor: undefined, pageNumber: 1 })
  })
})
