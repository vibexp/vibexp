import { act, renderHook } from '@testing-library/react'

import { deriveSiteInfo, EMPTY_SITE_INFO, type SiteInfo } from '../siteInfo'
import { SiteInfoContext } from '../SiteInfoContext'
import { useBrandLockup } from '../useBrandLockup'

const { theme } = vi.hoisted(
  (): {
    theme: { resolvedTheme: 'light' | 'dark' }
  } => ({ theme: { resolvedTheme: 'light' } })
)

vi.mock('@/lib/theme', () => ({
  useResolvedTheme: () => theme.resolvedTheme,
}))

const LIGHT = 'https://cdn.acme.test/logo.svg'
const DARK = 'https://cdn.acme.test/logo-dark.svg'

function renderLockup(info: Partial<SiteInfo>) {
  return renderHook(() => useBrandLockup(), {
    initialProps: info,
    wrapper: ({ children }: { children: React.ReactNode }) => (
      <SiteInfoContext.Provider
        value={deriveSiteInfo({ ...EMPTY_SITE_INFO, ...current.info })}
      >
        {children}
      </SiteInfoContext.Provider>
    ),
  })
}

// The wrapper reads the site info from here so a test can change it and
// rerender, the way a save does in the app.
const current: { info: Partial<SiteInfo> } = { info: {} }

function lockupFor(info: Partial<SiteInfo>) {
  current.info = info
  return renderLockup(info)
}

describe('useBrandLockup', () => {
  beforeEach(() => {
    theme.resolvedTheme = 'light'
    current.info = {}
  })

  it('is the named, decorative VibeXP mark when nothing is configured', () => {
    const { result } = lockupFor({})

    expect(result.current.logo.src).toBeNull()
    expect(result.current.name).toBe('VibeXP')
    expect(result.current.markIsDecorative).toBe(true)
  })

  it('names the VibeXP mark after the company when only a name is configured', () => {
    const { result } = lockupFor({ company_name: 'Acme Inc.' })

    expect(result.current.logo.src).toBeNull()
    expect(result.current.name).toBe('Acme Inc.')
    expect(result.current.markIsDecorative).toBe(true)
  })

  it("lets a company's own logo stand alone when no name is configured", () => {
    const { result } = lockupFor({ logo_url: LIGHT })

    expect(result.current.logo.src).toBe(LIGHT)
    expect(result.current.name).toBeNull()
    expect(result.current.markIsDecorative).toBe(false)
  })

  it('pairs a company logo with the company name', () => {
    const { result } = lockupFor({ company_name: 'Acme Inc.', logo_url: LIGHT })

    expect(result.current.logo.src).toBe(LIGHT)
    expect(result.current.name).toBe('Acme Inc.')
    expect(result.current.markIsDecorative).toBe(false)
  })

  describe('a dark-only logo', () => {
    it('is not on screen in light theme, so the VibeXP mark is named', () => {
      const { result } = lockupFor({ logo_dark_url: DARK })

      expect(result.current.logo.src).toBeNull()
      expect(result.current.name).toBe('VibeXP')
      expect(result.current.markIsDecorative).toBe(true)
    })

    it('stands alone in dark theme', () => {
      theme.resolvedTheme = 'dark'
      const { result } = lockupFor({ logo_dark_url: DARK })

      expect(result.current.logo.src).toBe(DARK)
      expect(result.current.name).toBeNull()
      expect(result.current.markIsDecorative).toBe(false)
    })
  })

  it('prefers the dark logo in dark theme and the main one otherwise', () => {
    theme.resolvedTheme = 'dark'
    expect(
      lockupFor({ logo_url: LIGHT, logo_dark_url: DARK }).result.current.logo
        .src
    ).toBe(DARK)

    theme.resolvedTheme = 'light'
    expect(
      lockupFor({ logo_url: LIGHT, logo_dark_url: DARK }).result.current.logo
        .src
    ).toBe(LIGHT)
  })

  describe('a logo that fails to load', () => {
    it('gives way to a named VibeXP mark when there is no company name', () => {
      const { result } = lockupFor({ logo_url: LIGHT })
      expect(result.current.name).toBeNull()

      act(() => {
        result.current.logo.onError()
      })

      expect(result.current.logo.src).toBeNull()
      expect(result.current.name).toBe('VibeXP')
      expect(result.current.markIsDecorative).toBe(true)
    })

    it('keeps the company name, beside a decorative mark', () => {
      const { result } = lockupFor({
        company_name: 'Acme Inc.',
        logo_url: LIGHT,
      })

      act(() => {
        result.current.logo.onError()
      })

      expect(result.current.logo.src).toBeNull()
      expect(result.current.name).toBe('Acme Inc.')
      expect(result.current.markIsDecorative).toBe(true)
    })

    it('gives a new logo URL a fresh attempt', () => {
      const { result, rerender } = lockupFor({ logo_url: LIGHT })
      act(() => {
        result.current.logo.onError()
      })
      expect(result.current.logo.src).toBeNull()

      current.info = { logo_url: 'https://cdn.acme.test/logo-v2.svg' }
      rerender(current.info)

      expect(result.current.logo.src).toBe('https://cdn.acme.test/logo-v2.svg')
      expect(result.current.name).toBeNull()
    })
  })
})
