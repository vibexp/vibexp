import { fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'

import { SidebarBrand } from '@/components/layout/SidebarBrand'
import {
  deriveSiteInfo,
  EMPTY_SITE_INFO,
  type SiteInfo,
} from '@/features/site-info/siteInfo'
import { SiteInfoContext } from '@/features/site-info/SiteInfoContext'

const LOGO = 'https://cdn.acme.test/logo.svg'

function renderBrand(
  info: Partial<SiteInfo> = {},
  props: { showText?: boolean; expanded?: boolean } = {}
) {
  render(
    <SiteInfoContext.Provider
      value={deriveSiteInfo({ ...EMPTY_SITE_INFO, ...info })}
    >
      <MemoryRouter>
        <SidebarBrand {...props} />
      </MemoryRouter>
    </SiteInfoContext.Provider>
  )
  return screen.getByRole('link')
}

describe('SidebarBrand', () => {
  it('links to the in-app home when unbranded', () => {
    expect(renderBrand()).toHaveAttribute('href', '/')
  })

  it('still links to the in-app home, not the company website, when branded', () => {
    expect(
      renderBrand({ company_name: 'Acme Inc.', website: 'https://acme.test' })
    ).toHaveAttribute('href', '/')
  })

  describe('with nothing configured', () => {
    it('is the VibeXP mark with the two-line wordmark, as before', () => {
      const link = renderBrand()

      const mark = within(link).getByRole('img', { name: 'VibeXP' })
      expect(mark.tagName.toLowerCase()).toBe('svg')
      expect(link.querySelector('img')).toBeNull()
      expect(
        within(link).getByText('VibeXP', { selector: 'span' })
      ).toBeInTheDocument()
      expect(
        within(link).getByText("Your team's shared brain")
      ).toBeInTheDocument()
    })
  })

  describe('with company branding', () => {
    it('shows the company logo and name, without the VibeXP tagline', () => {
      const link = renderBrand({ company_name: 'Acme Inc.', logo_url: LOGO })

      expect(
        within(link).getByRole('img', { name: 'Acme Inc.' })
      ).toHaveAttribute('src', LOGO)
      expect(within(link).getByText('Acme Inc.')).toBeInTheDocument()
      expect(
        within(link).queryByText('VibeXP', { selector: 'span' })
      ).not.toBeInTheDocument()
      expect(
        within(link).queryByText("Your team's shared brain")
      ).not.toBeInTheDocument()
    })

    it('shows the VibeXP mark beside the company name when there is no logo', () => {
      const link = renderBrand({ company_name: 'Acme Inc.' })

      expect(link.querySelector('svg')).not.toBeNull()
      expect(link.querySelector('img')).toBeNull()
      expect(within(link).getByText('Acme Inc.')).toBeInTheDocument()
      expect(
        within(link).queryByText("Your team's shared brain")
      ).not.toBeInTheDocument()
    })

    it('lets a company logo without a name stand alone', () => {
      const link = renderBrand({ logo_url: LOGO })

      expect(link.querySelector('img')).toHaveAttribute('src', LOGO)
      expect(link).toHaveTextContent('')
    })
  })

  // The wordmark follows what is on screen, not what is configured (#1386).
  describe('when the VibeXP mark stands in for a company logo', () => {
    it('shows the product wordmark and tagline after a nameless logo fails to load', () => {
      const link = renderBrand({ logo_url: LOGO })
      expect(link).toHaveTextContent('')

      fireEvent.error(within(link).getByRole('img'))

      expect(link.querySelector('img')).toBeNull()
      expect(
        within(link).getByText('VibeXP', { selector: 'span' })
      ).toBeInTheDocument()
      expect(
        within(link).getByText("Your team's shared brain")
      ).toBeInTheDocument()
    })

    it('shows the product wordmark for a dark-only logo in light theme', () => {
      const link = renderBrand({
        logo_dark_url: 'https://cdn.acme.test/dark.svg',
      })

      expect(link.querySelector('img')).toBeNull()
      expect(
        within(link).getByText("Your team's shared brain")
      ).toBeInTheDocument()
    })

    it('keeps the company name, with no tagline, after a named logo fails to load', () => {
      const link = renderBrand({ company_name: 'Acme Inc.', logo_url: LOGO })
      fireEvent.error(within(link).getByRole('img'))

      expect(within(link).getByText('Acme Inc.')).toBeInTheDocument()
      expect(
        within(link).queryByText("Your team's shared brain")
      ).not.toBeInTheDocument()
    })
  })

  describe('the mark and assistive technology', () => {
    it('keeps its name in the sidebar, where the rail hides the wordmark', () => {
      const link = renderBrand()

      expect(link.querySelector('svg')).not.toHaveAttribute('aria-hidden')
      expect(
        within(link).getByRole('img', { name: 'VibeXP' })
      ).toBeInTheDocument()
    })

    it('is decorative in a drawer, where the wordmark is always on screen', () => {
      const link = renderBrand({}, { showText: true })

      expect(link.querySelector('svg')).toHaveAttribute('aria-hidden', 'true')
      expect(within(link).queryByRole('img')).toBeNull()
    })
  })

  describe('rail and expanded forms', () => {
    const wordmark = (link: HTMLElement) =>
      within(link).getByText('Acme Inc.').parentElement

    it('holds the logo to the rail box and hides the wordmark in the collapsed rail', () => {
      const link = renderBrand(
        { company_name: 'Acme Inc.', logo_url: LOGO },
        { expanded: false }
      )

      const logo = link.querySelector('img')
      expect(logo).toHaveClass('max-w-[34px]')
      expect(logo).not.toHaveClass('lg:max-w-40')
      expect(wordmark(link)).toHaveClass('hidden')
      expect(wordmark(link)).not.toHaveClass('lg:flex')
    })

    it('lifts the width cap and shows the wordmark at lg+ when expanded', () => {
      const link = renderBrand({ company_name: 'Acme Inc.', logo_url: LOGO })

      const logo = link.querySelector('img')
      expect(logo).toHaveClass('max-w-[34px]', 'lg:max-w-40')
      expect(wordmark(link)).toHaveClass('hidden', 'lg:flex')
    })

    it('shows the wordmark and the wider logo at every size in a drawer', () => {
      const link = renderBrand(
        { company_name: 'Acme Inc.', logo_url: LOGO },
        { showText: true }
      )

      const logo = link.querySelector('img')
      expect(logo).toHaveClass('max-w-40')
      expect(logo).not.toHaveClass('max-w-[34px]')
      expect(wordmark(link)).toHaveClass('flex')
      expect(wordmark(link)).not.toHaveClass('hidden')
    })
  })
})
