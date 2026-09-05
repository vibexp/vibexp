import { act, renderHook } from '@testing-library/react'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import { storage } from '@/utils/storage'

import { useLocalStorage } from '../useLocalStorage'

describe('useLocalStorage', () => {
  beforeEach(() => {
    storage.clear()
  })

  it('falls back to the initial value when nothing is stored', () => {
    const { result } = renderHook(() =>
      useLocalStorage(STORAGE_KEYS.NAV_COLLAPSED, false)
    )
    expect(result.current[0]).toBe(false)
  })

  it('round-trips booleans through JSON', () => {
    const { result } = renderHook(() =>
      useLocalStorage(STORAGE_KEYS.NAV_COLLAPSED, false)
    )
    act(() => {
      result.current[1](true)
    })
    expect(result.current[0]).toBe(true)
    expect(storage.get(STORAGE_KEYS.NAV_COLLAPSED)).toBe('true')

    // A fresh mount reads the stored boolean back as a boolean, not "true".
    const { result: remount } = renderHook(() =>
      useLocalStorage(STORAGE_KEYS.NAV_COLLAPSED, false)
    )
    expect(remount.current[0]).toBe(true)
  })

  it('supports functional updates', () => {
    const { result } = renderHook(() =>
      useLocalStorage(STORAGE_KEYS.DETAILS_COLLAPSED, false)
    )
    act(() => {
      result.current[1](v => !v)
    })
    expect(result.current[0]).toBe(true)
    act(() => {
      result.current[1](v => !v)
    })
    expect(result.current[0]).toBe(false)
    expect(storage.get(STORAGE_KEYS.DETAILS_COLLAPSED)).toBe('false')
  })
})
