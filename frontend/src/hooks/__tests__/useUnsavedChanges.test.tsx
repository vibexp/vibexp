import { act, renderHook } from '@testing-library/react'
import type { MockInstance } from 'vitest'

import {
  UNSAVED_CHANGES_MESSAGE,
  useUnsavedChanges,
} from '@/hooks/useUnsavedChanges'

describe('useUnsavedChanges', () => {
  let addSpy: MockInstance
  let removeSpy: MockInstance

  beforeEach(() => {
    addSpy = vi.spyOn(window, 'addEventListener')
    removeSpy = vi.spyOn(window, 'removeEventListener')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  function beforeUnloadListeners(spy: MockInstance): EventListener[] {
    return (spy.mock.calls as unknown[][])
      .filter(call => call[0] === 'beforeunload')
      .map(call => call[1] as EventListener)
  }

  it('registers a beforeunload listener while dirty', () => {
    renderHook(() => useUnsavedChanges(true))
    expect(beforeUnloadListeners(addSpy)).toHaveLength(1)
  })

  it('registers nothing while clean', () => {
    renderHook(() => useUnsavedChanges(false))
    expect(beforeUnloadListeners(addSpy)).toHaveLength(0)
  })

  it('removes the listener when the form goes clean again', () => {
    const { rerender } = renderHook(
      ({ dirty }: { dirty: boolean }) => useUnsavedChanges(dirty),
      { initialProps: { dirty: true } }
    )
    expect(beforeUnloadListeners(removeSpy)).toHaveLength(0)
    rerender({ dirty: false })
    expect(beforeUnloadListeners(removeSpy)).toHaveLength(1)
  })

  it('removes the listener on unmount', () => {
    const { unmount } = renderHook(() => useUnsavedChanges(true))
    unmount()
    expect(beforeUnloadListeners(removeSpy)).toHaveLength(1)
  })

  // Both halves matter across engines: Chrome/Safari honour preventDefault,
  // older Firefox keys off a non-empty returnValue. A hand-built event is used
  // rather than a real one because jsdom implements only the generic
  // `Event.returnValue` — a boolean alias for `!defaultPrevented` — so a real
  // event reports `false` here however the handler is written, and the
  // assertion would be vacuous.
  it('cancels the unload event both ways', () => {
    renderHook(() => useUnsavedChanges(true))
    const listener = beforeUnloadListeners(addSpy)[0]
    const event = {
      preventDefault: vi.fn(),
      returnValue: '',
    } as unknown as BeforeUnloadEvent
    act(() => {
      listener(event)
    })
    expect(event.preventDefault).toHaveBeenCalled()
    expect(event.returnValue).toBe(UNSAVED_CHANGES_MESSAGE)
  })

  describe('confirmLeave', () => {
    it('leaves without asking when nothing is dirty', () => {
      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
      const { result } = renderHook(() => useUnsavedChanges(false))
      expect(result.current.confirmLeave()).toBe(true)
      expect(confirmSpy).not.toHaveBeenCalled()
    })

    it('asks when dirty and leaves once confirmed', () => {
      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
      const { result } = renderHook(() => useUnsavedChanges(true))
      expect(result.current.confirmLeave()).toBe(true)
      expect(confirmSpy).toHaveBeenCalledWith(UNSAVED_CHANGES_MESSAGE)
    })

    it('stays put when the reader cancels', () => {
      vi.spyOn(window, 'confirm').mockReturnValue(false)
      const { result } = renderHook(() => useUnsavedChanges(true))
      expect(result.current.confirmLeave()).toBe(false)
    })
  })
})
