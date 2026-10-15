import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useCopyToClipboard } from '../useCopyToClipboard'

const writeText = vi.fn<(text: string) => Promise<void>>()

function setClipboard(value: unknown) {
  Object.defineProperty(navigator, 'clipboard', { value, configurable: true })
}

beforeEach(() => {
  vi.useFakeTimers()
  writeText.mockReset().mockResolvedValue(undefined)
  setClipboard({ writeText })
  vi.spyOn(console, 'error').mockImplementation(() => undefined)
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('useCopyToClipboard', () => {
  it('copies and says so for a moment', async () => {
    const { result } = renderHook(() => useCopyToClipboard(1000))
    await act(async () => {
      result.current.copy('hello')
      await Promise.resolve()
    })
    expect(writeText).toHaveBeenCalledWith('hello')
    expect(result.current.copied).toBe(true)
    act(() => {
      vi.advanceTimersByTime(1000)
    })
    expect(result.current.copied).toBe(false)
  })

  it('hands a refused copy to onFailure', async () => {
    writeText.mockRejectedValue(new Error('NotAllowedError'))
    const onFailure = vi.fn()
    const { result } = renderHook(() => useCopyToClipboard(1000, onFailure))
    await act(async () => {
      result.current.copy('hello')
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(onFailure).toHaveBeenCalledWith('hello')
    expect(result.current.copied).toBe(false)
  })

  it('does not throw without a clipboard API (plain http), and reports it', () => {
    setClipboard(undefined)
    const onFailure = vi.fn()
    const { result } = renderHook(() => useCopyToClipboard(1000, onFailure))
    expect(() => {
      result.current.copy('hello')
    }).not.toThrow()
    expect(onFailure).toHaveBeenCalledWith('hello')
    expect(console.error).toHaveBeenCalled()
  })

  it('only logs a failure when no onFailure is given', () => {
    setClipboard(undefined)
    const { result } = renderHook(() => useCopyToClipboard())
    expect(() => {
      result.current.copy('hello')
    }).not.toThrow()
    expect(console.error).toHaveBeenCalled()
  })
})
