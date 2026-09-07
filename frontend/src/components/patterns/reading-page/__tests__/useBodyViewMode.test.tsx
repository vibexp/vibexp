import { act, renderHook } from '@testing-library/react'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import { storage } from '@/utils/storage'

import { useBodyViewMode } from '../useBodyViewMode'

describe('useBodyViewMode', () => {
  beforeEach(() => {
    storage.clear()
  })

  it('defaults to rendered when nothing is stored', () => {
    const { result } = renderHook(() => useBodyViewMode())
    expect(result.current[0]).toBe('rendered')
  })

  it('round-trips the choice through localStorage', () => {
    const { result, unmount } = renderHook(() => useBodyViewMode())

    act(() => {
      result.current[1]('raw')
    })
    expect(result.current[0]).toBe('raw')
    unmount()

    // The regression guard for the reason this is a boolean: `storage.set`
    // writes a string value verbatim and `storage.getJSON` always JSON.parses,
    // so persisting the mode STRING silently resets on every remount.
    const remounted = renderHook(() => useBodyViewMode())
    expect(remounted.result.current[0]).toBe('raw')
  })

  it('stores a boolean under the shared key', () => {
    const { result } = renderHook(() => useBodyViewMode())

    act(() => {
      result.current[1]('raw')
    })
    expect(storage.get(STORAGE_KEYS.BODY_VIEW_RAW)).toBe('true')

    act(() => {
      result.current[1]('rendered')
    })
    expect(storage.get(STORAGE_KEYS.BODY_VIEW_RAW)).toBe('false')
  })

  it('keeps a stable setter so callers may list it in effect deps', () => {
    const { result, rerender } = renderHook(() => useBodyViewMode())
    const first = result.current[1]
    rerender()
    expect(result.current[1]).toBe(first)
  })
})
