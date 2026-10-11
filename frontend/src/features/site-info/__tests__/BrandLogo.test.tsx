import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { BrandLogo } from '../BrandLogo'
import { deriveSiteInfo, EMPTY_SITE_INFO, type SiteInfo } from '../siteInfo'
import { SiteInfoContext } from '../SiteInfoContext'

const { theme } = vi.hoisted(
  (): {
    theme: { resolvedTheme: 'light' | 'dark' }
  } => ({ theme: { resolvedTheme: 'light' } })
)

vi.mock('@/lib/theme', () => ({
  useTheme: () => ({ ...theme, theme: theme.resolvedTheme, setTheme: vi.fn() }),
}))

vi.mock('@vibexp/design-system/react', () => ({
  LogoMark: ({ size }: { size?: number }) => (
    <span data-testid="vibexp-mark" data-size={size} />
  ),
}))

const LIGHT = 'https://cdn.acme.test/logo.svg'
const DARK = 'https://cdn.acme.test/logo-dark.svg'

function tree(info: Partial<SiteInfo>, size?: number) {
  return (
    <SiteInfoContext.Provider
      value={deriveSiteInfo({ ...EMPTY_SITE_INFO, ...info })}
    >
      <BrandLogo size={size} />
    </SiteInfoContext.Provider>
  )
}

describe('BrandLogo', () => {
  beforeEach(() => {
    theme.resolvedTheme = 'light'
  })

  it('renders the VibeXP mark when no logo is configured', () => {
    render(tree({ company_name: 'Acme Inc.' }))

    expect(screen.getByTestId('vibexp-mark')).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  it('renders the company logo, named after the company', () => {
    render(tree({ company_name: 'Acme Inc.', logo_url: LIGHT }))

    const img = screen.getByRole('img', { name: 'Acme Inc.' })
    expect(img).toHaveAttribute('src', LIGHT)
    expect(screen.queryByTestId('vibexp-mark')).not.toBeInTheDocument()
  })

  it('never names a company logo after the product', () => {
    render(tree({ logo_url: LIGHT }))

    expect(
      screen.getByRole('img', { name: 'Company logo' })
    ).toBeInTheDocument()
    expect(screen.queryByRole('img', { name: 'VibeXP' })).toBeNull()
  })

  it('uses the main logo in light mode even when a dark one is set', () => {
    render(tree({ logo_url: LIGHT, logo_dark_url: DARK }))

    expect(screen.getByRole('img')).toHaveAttribute('src', LIGHT)
  })

  it('uses the dark logo in dark mode when it is set', () => {
    theme.resolvedTheme = 'dark'
    render(tree({ logo_url: LIGHT, logo_dark_url: DARK }))

    expect(screen.getByRole('img')).toHaveAttribute('src', DARK)
  })

  it('uses the main logo in dark mode when no dark one is set', () => {
    theme.resolvedTheme = 'dark'
    render(tree({ logo_url: LIGHT }))

    expect(screen.getByRole('img')).toHaveAttribute('src', LIGHT)
  })

  it('shows the dark logo in dark mode even with no main logo', () => {
    theme.resolvedTheme = 'dark'
    render(tree({ logo_dark_url: DARK }))

    expect(screen.getByRole('img')).toHaveAttribute('src', DARK)
  })

  it('shows the VibeXP mark in light mode when only a dark logo is set', () => {
    render(tree({ logo_dark_url: DARK }))

    expect(screen.getByTestId('vibexp-mark')).toBeInTheDocument()
  })

  it('replaces a logo that fails to load with the VibeXP mark', () => {
    render(tree({ logo_url: LIGHT }))

    fireEvent.error(screen.getByRole('img'))

    expect(screen.getByTestId('vibexp-mark')).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  it('gives a new logo URL a fresh attempt after a failure', () => {
    const { rerender } = render(tree({ logo_url: LIGHT }))
    fireEvent.error(screen.getByRole('img'))
    expect(screen.getByTestId('vibexp-mark')).toBeInTheDocument()

    rerender(tree({ logo_url: 'https://cdn.acme.test/logo-v2.svg' }))

    expect(screen.getByRole('img')).toHaveAttribute(
      'src',
      'https://cdn.acme.test/logo-v2.svg'
    )
  })

  it('sizes the image and the fallback mark alike', () => {
    const { rerender } = render(tree({ logo_url: LIGHT }, 24))
    expect(screen.getByRole('img')).toHaveStyle({ height: '24px' })

    fireEvent.error(screen.getByRole('img'))
    expect(screen.getByTestId('vibexp-mark')).toHaveAttribute('data-size', '24')

    rerender(tree({}, 24))
    expect(screen.getByTestId('vibexp-mark')).toHaveAttribute('data-size', '24')
  })

  it('defaults both forms to the design-system mark size', () => {
    const { rerender } = render(tree({ logo_url: LIGHT }))
    expect(screen.getByRole('img')).toHaveStyle({ height: '34px' })

    rerender(tree({}))
    expect(screen.getByTestId('vibexp-mark')).toHaveAttribute('data-size', '34')
  })

  it('applies the caller class to the image', () => {
    const { container } = render(
      <SiteInfoContext.Provider
        value={deriveSiteInfo({ ...EMPTY_SITE_INFO, logo_url: LIGHT })}
      >
        <BrandLogo className="h-6" />
      </SiteInfoContext.Provider>
    )

    expect(container.querySelector('img')).toHaveClass('h-6')
  })
})
