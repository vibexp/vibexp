import { STORAGE_KEYS } from '@/constants/storageKeys'

import {
  consumeReturnTo,
  DEFAULT_RETURN_TO,
  sanitizeReturnTo,
  stashReturnTo,
} from './returnTo'

describe('sanitizeReturnTo', () => {
  it('returns a same-origin path unchanged', () => {
    expect(sanitizeReturnTo('/oauth/consent?login=abc')).toBe(
      '/oauth/consent?login=abc'
    )
    expect(sanitizeReturnTo('/')).toBe('/')
    expect(sanitizeReturnTo('/settings/profile')).toBe('/settings/profile')
  })

  it('defaults to "/" for null/undefined/non-strings', () => {
    expect(sanitizeReturnTo(null)).toBe(DEFAULT_RETURN_TO)
    expect(sanitizeReturnTo(undefined)).toBe(DEFAULT_RETURN_TO)
  })

  it('rejects open-redirect targets (cross-origin / absolute / non-path)', () => {
    // Protocol-relative and backslash variants resolve to a foreign origin.
    expect(sanitizeReturnTo('//evil.com')).toBe(DEFAULT_RETURN_TO)
    expect(sanitizeReturnTo('/\\evil.com')).toBe(DEFAULT_RETURN_TO)
    // Absolute URLs and non-path schemes.
    expect(sanitizeReturnTo('https://evil.com')).toBe(DEFAULT_RETURN_TO)
    expect(sanitizeReturnTo('http://evil.com/path')).toBe(DEFAULT_RETURN_TO)
    expect(sanitizeReturnTo('javascript:alert(1)')).toBe(DEFAULT_RETURN_TO)
    // Relative (no leading slash) and empty.
    expect(sanitizeReturnTo('evil.com')).toBe(DEFAULT_RETURN_TO)
    expect(sanitizeReturnTo('')).toBe(DEFAULT_RETURN_TO)
  })

  it('rejects tab/newline that would unmask a protocol-relative target', () => {
    // Browsers strip \t \n \r before parsing, collapsing these to "//evil.com".
    expect(sanitizeReturnTo('/\t/\tevil.com')).toBe(DEFAULT_RETURN_TO)
    expect(sanitizeReturnTo('/\n/evil.com')).toBe(DEFAULT_RETURN_TO)
    expect(sanitizeReturnTo('/\r/\\evil.com')).toBe(DEFAULT_RETURN_TO)
    // A benign embedded newline in an otherwise same-origin path is stripped,
    // leaving a safe same-origin path.
    expect(sanitizeReturnTo('/set\ntings')).toBe('/settings')
  })
})

describe('stashReturnTo / consumeReturnTo', () => {
  beforeEach(() => {
    window.sessionStorage.clear()
  })

  it('round-trips a valid path and clears it (single-use)', () => {
    stashReturnTo('/oauth/consent?login=abc')
    expect(window.sessionStorage.getItem(STORAGE_KEYS.RETURN_TO)).toBe(
      '/oauth/consent?login=abc'
    )

    expect(consumeReturnTo()).toBe('/oauth/consent?login=abc')
    // Cleared after consuming.
    expect(window.sessionStorage.getItem(STORAGE_KEYS.RETURN_TO)).toBeNull()
    expect(consumeReturnTo()).toBe(DEFAULT_RETURN_TO)
  })

  it('sanitizes before storing (open-redirect cannot be stashed)', () => {
    stashReturnTo('//evil.com')
    expect(window.sessionStorage.getItem(STORAGE_KEYS.RETURN_TO)).toBe(
      DEFAULT_RETURN_TO
    )
  })

  it('defaults to "/" when nothing is stashed', () => {
    expect(consumeReturnTo()).toBe(DEFAULT_RETURN_TO)
  })
})
