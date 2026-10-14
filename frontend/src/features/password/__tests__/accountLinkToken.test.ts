import { renderHook } from '@testing-library/react'

import { takeAccountLinkToken, useAccountLinkToken } from '../accountLinkToken'

describe('account link token', () => {
  afterEach(() => {
    globalThis.history.replaceState(null, '', '/')
  })

  it('reads the token from the fragment and removes the fragment', () => {
    globalThis.history.replaceState(
      null,
      '',
      '/account/setup?x=1#token=abc-123_XYZ'
    )

    expect(takeAccountLinkToken()).toBe('abc-123_XYZ')
    expect(globalThis.location.hash).toBe('')
    expect(globalThis.location.pathname + globalThis.location.search).toBe(
      '/account/setup?x=1'
    )
    // Single use: the fragment is gone.
    expect(takeAccountLinkToken()).toBeNull()
  })

  it("keeps the router's history entry state", () => {
    globalThis.history.replaceState(
      { usr: null, key: 'k1', idx: 0 },
      '',
      '/account/setup#token=abc'
    )
    expect(takeAccountLinkToken()).toBe('abc')
    expect(globalThis.history.state).toEqual({ usr: null, key: 'k1', idx: 0 })
  })

  it('returns null when there is no token, and still clears a stray fragment', () => {
    globalThis.history.replaceState(null, '', '/account/setup')
    expect(takeAccountLinkToken()).toBeNull()

    globalThis.history.replaceState(null, '', '/account/setup#token=')
    expect(takeAccountLinkToken()).toBeNull()
    expect(globalThis.location.hash).toBe('')
  })

  it('keeps the token across re-renders', () => {
    globalThis.history.replaceState(null, '', '/account/setup#token=t-1')
    const { result, rerender } = renderHook(() => useAccountLinkToken())
    expect(result.current).toBe('t-1')
    rerender()
    expect(result.current).toBe('t-1')
    expect(globalThis.location.hash).toBe('')
  })
})
