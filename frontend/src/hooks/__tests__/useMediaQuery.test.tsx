import { act, renderHook } from '@testing-library/react'

import { mockViewportWidth } from '@/lib/testing/matchMedia'

import { BREAKPOINT_QUERIES, useMediaQuery } from '../useMediaQuery'

describe('useMediaQuery', () => {
  it('returns the fallback when matchMedia is unavailable', () => {
    // jsdom has no matchMedia by default.
    expect(typeof window.matchMedia).toBe('undefined')
    const { result: desktop } = renderHook(() =>
      useMediaQuery(BREAKPOINT_QUERIES.lg)
    )
    expect(desktop.current).toBe(true)
    const { result: mobile } = renderHook(() =>
      useMediaQuery(BREAKPOINT_QUERIES.lg, false)
    )
    expect(mobile.current).toBe(false)
  })

  it('tracks the query and re-renders on change', () => {
    const viewport = mockViewportWidth(390)
    try {
      const { result } = renderHook(() => useMediaQuery(BREAKPOINT_QUERIES.md))
      expect(result.current).toBe(false)
      act(() => {
        viewport.setWidth(900)
      })
      expect(result.current).toBe(true)
      act(() => {
        viewport.setWidth(500)
      })
      expect(result.current).toBe(false)
    } finally {
      viewport.restore()
    }
  })
})
