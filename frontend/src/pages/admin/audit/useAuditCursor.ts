import { useCallback, useReducer } from 'react'

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
  switch (action.type) {
    case 'next':
      return { stack: [...state.stack, action.cursor] }
    case 'previous':
      return state.stack.length === 0
        ? state
        : { stack: state.stack.slice(0, -1) }
    case 'reset':
      return state.stack.length === 0 ? state : INITIAL_CURSOR_STATE
  }
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
 * nothing in another.
 */
export function useAuditCursor(filterKey: string): AuditCursor {
  // The stack remembers the filters it was built for. Comparing during render,
  // rather than resetting in an effect, means a filter change never issues one
  // request with the previous result set's cursor first.
  const [state, dispatch] = useReducer(
    (
      current: { key: string; cursors: AuditCursorState },
      action: { key: string; move: AuditCursorAction }
    ) => ({
      key: action.key,
      cursors: auditCursorReducer(
        current.key === action.key ? current.cursors : INITIAL_CURSOR_STATE,
        action.move
      ),
    }),
    { key: filterKey, cursors: INITIAL_CURSOR_STATE }
  )
  const stack = state.key === filterKey ? state.cursors.stack : []

  const next = useCallback(
    (cursor: string) => {
      dispatch({ key: filterKey, move: { type: 'next', cursor } })
    },
    [filterKey]
  )
  const previous = useCallback(() => {
    dispatch({ key: filterKey, move: { type: 'previous' } })
  }, [filterKey])

  return {
    cursor: stack.at(-1),
    pageNumber: stack.length + 1,
    hasPrevious: stack.length > 0,
    next,
    previous,
  }
}
