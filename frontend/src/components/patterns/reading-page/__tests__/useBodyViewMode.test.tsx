import { act, renderHook } from '@testing-library/react'

import { useBodyViewMode } from '../useBodyViewMode'

describe('useBodyViewMode', () => {
  beforeEach(() => {
    globalThis.localStorage.clear()
    window.history.replaceState(null, '', '/artifacts/p1/one')
  })

  it('opens on rendered', () => {
    const { result } = renderHook(() => useBodyViewMode())
    expect(result.current[0]).toBe('rendered')
  })

  it('switches to raw and back for the current page view', () => {
    const { result } = renderHook(() => useBodyViewMode())

    act(() => {
      result.current[1]('raw')
    })
    expect(result.current[0]).toBe('raw')

    act(() => {
      result.current[1]('rendered')
    })
    expect(result.current[0]).toBe('rendered')
  })

  it('does not remember raw across a remount (a reload, or another page)', () => {
    const { result, unmount } = renderHook(() => useBodyViewMode())
    act(() => {
      result.current[1]('raw')
    })
    unmount()

    const remounted = renderHook(() => useBodyViewMode())
    expect(remounted.result.current[0]).toBe('rendered')
  })

  it('stores nothing, and ignores a stale key left by an older build', () => {
    globalThis.localStorage.setItem('vx_body_view_raw', 'true')
    const { result } = renderHook(() => useBodyViewMode())
    expect(result.current[0]).toBe('rendered')

    globalThis.localStorage.clear()
    act(() => {
      result.current[1]('raw')
    })
    expect(globalThis.localStorage).toHaveLength(0)
  })

  it('returns to rendered when the reader navigates to another resource in the same mounted page', () => {
    // Navigating between two resources of one kind keeps the page component
    // mounted, so unmounting alone cannot be what resets the view.
    const { result, rerender } = renderHook(() => useBodyViewMode())
    act(() => {
      result.current[1]('raw')
    })
    expect(result.current[0]).toBe('raw')

    window.history.pushState(null, '', '/artifacts/p1/two')
    rerender()

    expect(result.current[0]).toBe('rendered')
  })

  it('keeps a stable setter so callers may list it in effect deps', () => {
    const { result, rerender } = renderHook(() => useBodyViewMode())
    const first = result.current[1]
    rerender()
    expect(result.current[1]).toBe(first)
  })
})
