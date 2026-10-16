import { useCallback, useState } from 'react'

/**
 * The audit log's keyset paging (#1517): Next follows the page's `next_cursor`;
 * Previous needs the cursor each earlier page was loaded with, which the API
 * cannot give back, so the client keeps them on a stack. There is no total
 * count, by design (#1516).
 *
 * `stack[i]` is the cursor page i+1 was loaded with; page 1 has none.
 */
export interface AuditCursorState {
  stack: readonly string[]
}

export type AuditCursorAction =
  { type: 'next'; cursor: string } | { type: 'previous' } | { type: 'reset' }

export const INITIAL_CURSOR_STATE: AuditCursorState = { stack: [] }

export function auditCursorReducer(
  state: AuditCursorState,
  action: AuditCursorAction
): AuditCursorState {
  if (action.type === 'next') {
    return { stack: [...state.stack, action.cursor] }
  }
  return state.stack.length === 0 ? state : { stack: state.stack.slice(0, -1) }
}

export interface AuditCursor {
  /** The cursor the current page is loaded with; undefined on page 1. */
  cursor: string | undefined
  /** 1-based number of the current page. */
  pageNumber: number
  hasPrevious: boolean
  next: (cursor: string) => void
  previous: () => void
}

/**
 * The cursor stack for one filter set. `filterKey` identifies the filters: when
 * it changes the stack resets, because a cursor from one result set means
 * nothing in another — and stays reset, so returning to earlier filters starts
 * from their newest page again.
 */
export function useAuditCursor(filterKey: string): AuditCursor {
  const [state, setState] = useState<{
    key: string
    cursors: AuditCursorState
  }>({ key: filterKey, cursors: INITIAL_CURSOR_STATE })

  // Reset during render ("adjusting state when a prop changes"), not in an
  // effect: the reset render never issues a request with the previous result
  // set's cursor first.
  let current = state
  if (state.key !== filterKey) {
    current = { key: filterKey, cursors: INITIAL_CURSOR_STATE }
    setState(current)
  }
  const { stack } = current.cursors

  const next = useCallback((cursor: string) => {
    setState(prev => ({
      key: prev.key,
      cursors: auditCursorReducer(prev.cursors, { type: 'next', cursor }),
    }))
  }, [])
  const previous = useCallback(() => {
    setState(prev => ({
      key: prev.key,
      cursors: auditCursorReducer(prev.cursors, { type: 'previous' }),
    }))
  }, [])

  return {
    cursor: stack.at(-1),
    pageNumber: stack.length + 1,
    hasPrevious: stack.length > 0,
    next,
    previous,
  }
}
