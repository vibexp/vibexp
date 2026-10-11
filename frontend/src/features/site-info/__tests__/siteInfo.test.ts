import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  deriveSiteInfo,
  EMPTY_SITE_INFO,
  notifySiteInfoSaved,
  readInlinedSiteInfo,
  SITE_INFO_SAVED_EVENT,
  type SiteInfo,
  toSiteInfo,
} from '../siteInfo'

const ACME: SiteInfo = {
  company_name: 'Acme Inc.',
  company_email: 'hello@acme.test',
  website: 'https://acme.test',
  support_url: 'https://acme.test/help',
  terms_url: 'https://acme.test/terms',
  privacy_url: 'https://acme.test/privacy',
  logo_url: 'https://cdn.acme.test/logo.svg',
  logo_dark_url: 'https://cdn.acme.test/logo-dark.svg',
}

describe('toSiteInfo', () => {
  it('keeps a fully configured, valid value as it is', () => {
    expect(toSiteInfo(ACME)).toEqual(ACME)
  })

  it.each([undefined, null, 'acme', 42, ['a'], true])(
    'treats the non-object %j as nothing configured',
    raw => {
      expect(toSiteInfo(raw)).toEqual(EMPTY_SITE_INFO)
    }
  )

  it('fills missing keys with null and ignores unknown ones', () => {
    expect(toSiteInfo({ company_name: 'Acme Inc.', favicon: 'x' })).toEqual({
      ...EMPTY_SITE_INFO,
      company_name: 'Acme Inc.',
    })
  })

  it('turns a non-string or blank value into null', () => {
    expect(
      toSiteInfo({ company_name: 7, company_email: '   ', website: {} })
    ).toEqual(EMPTY_SITE_INFO)
  })

  it('trims surrounding whitespace', () => {
    expect(toSiteInfo({ company_name: '  Acme Inc.  ' }).company_name).toBe(
      'Acme Inc.'
    )
  })

  it.each([
    'javascript:alert(1)',
    'data:image/svg+xml;base64,PHN2Zz4=',
    'mailto:help@acme.test',
    '/relative/logo.png',
    'not a url',
  ])('drops the non-http(s) URL %s from every URL field', bad => {
    const info = toSiteInfo({
      website: bad,
      support_url: bad,
      terms_url: bad,
      privacy_url: bad,
      logo_url: bad,
      logo_dark_url: bad,
    })
    expect(info).toEqual(EMPTY_SITE_INFO)
  })

  it('accepts both http and https URLs', () => {
    const info = toSiteInfo({
      website: 'http://localhost:8080',
      logo_url: 'https://cdn.acme.test/logo.svg',
    })
    expect(info.website).toBe('http://localhost:8080')
    expect(info.logo_url).toBe('https://cdn.acme.test/logo.svg')
  })
})

describe('readInlinedSiteInfo', () => {
  afterEach(() => {
    delete window.__VIBEXP_SITE_INFO__
  })

  it('is nothing configured when /config.js set no global (local dev)', () => {
    expect(readInlinedSiteInfo()).toEqual(EMPTY_SITE_INFO)
  })

  it('reads the global /config.js inlined', () => {
    window.__VIBEXP_SITE_INFO__ = { ...ACME, support_url: null }
    expect(readInlinedSiteInfo()).toEqual({ ...ACME, support_url: null })
  })

  it('survives a malformed global', () => {
    window.__VIBEXP_SITE_INFO__ = 'oops'
    expect(readInlinedSiteInfo()).toEqual(EMPTY_SITE_INFO)
  })
})

describe('deriveSiteInfo', () => {
  it('falls back to the product for an unbranded instance', () => {
    expect(deriveSiteInfo(EMPTY_SITE_INFO)).toEqual({
      ...EMPTY_SITE_INFO,
      displayName: 'VibeXP',
      hasCompanyBranding: false,
      supportHref: null,
    })
  })

  it('uses the company name as the display name', () => {
    const derived = deriveSiteInfo(ACME)
    expect(derived.displayName).toBe('Acme Inc.')
    expect(derived.hasCompanyBranding).toBe(true)
  })

  it('counts a logo alone as company branding', () => {
    const derived = deriveSiteInfo({
      ...EMPTY_SITE_INFO,
      logo_url: 'https://cdn.acme.test/logo.svg',
    })
    expect(derived.hasCompanyBranding).toBe(true)
    expect(derived.displayName).toBe('VibeXP')
  })

  it('does not count links alone as company branding', () => {
    expect(
      deriveSiteInfo({ ...EMPTY_SITE_INFO, website: 'https://acme.test' })
        .hasCompanyBranding
    ).toBe(false)
  })

  it('prefers the support URL for the support link', () => {
    expect(deriveSiteInfo(ACME).supportHref).toBe('https://acme.test/help')
  })

  it('falls back to a mailto for the company email', () => {
    expect(deriveSiteInfo({ ...ACME, support_url: null }).supportHref).toBe(
      'mailto:hello@acme.test'
    )
  })

  it('has no support link when neither is configured', () => {
    expect(
      deriveSiteInfo({ ...ACME, support_url: null, company_email: null })
        .supportHref
    ).toBeNull()
  })
})

describe('notifySiteInfoSaved', () => {
  it('fires the saved event on window', () => {
    const listener = vi.fn()
    window.addEventListener(SITE_INFO_SAVED_EVENT, listener)

    notifySiteInfoSaved()

    expect(listener).toHaveBeenCalledTimes(1)
    window.removeEventListener(SITE_INFO_SAVED_EVENT, listener)
  })
})
