import { render, screen } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'

import { SidebarProductMark } from '@/components/layout/SidebarProductMark'
import { TooltipProvider } from '@/components/ui/tooltip'
import {
  deriveSiteInfo,
  EMPTY_SITE_INFO,
  type SiteInfo,
} from '@/features/site-info/siteInfo'
import { SiteInfoContext } from '@/features/site-info/SiteInfoContext'

function renderMark(
  props: { showText?: boolean; expanded?: boolean } = {},
  info: Partial<SiteInfo> = {}
) {
  const result = render(
    <SiteInfoContext.Provider
      value={deriveSiteInfo({ ...EMPTY_SITE_INFO, ...info })}
    >
      <TooltipProvider delayDuration={0}>
        <SidebarProductMark {...props} />
      </TooltipProvider>
    </SiteInfoContext.Provider>
  )
  return { ...result, link: screen.getByRole('link') }
}

describe('SidebarProductMark', () => {
  it('links to vibexp.io in a new tab', () => {
    const { link } = renderMark()

    expect(link).toHaveAttribute('href', 'https://vibexp.io')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    expect(link.querySelector('svg')).not.toBeNull()
  })

  it('is named "VibeXP" on an unbranded instance', () => {
    const { link } = renderMark()

    expect(link).toHaveAccessibleName('VibeXP')
    expect(link).toHaveTextContent('VibeXP')
  })

  it.each([
    ['a company name', { company_name: 'Acme Inc.' }],
    ['a logo', { logo_url: 'https://cdn.acme.test/logo.svg' }],
    ['a dark logo only', { logo_dark_url: 'https://cdn.acme.test/dark.svg' }],
  ])('reads "Powered by VibeXP" once %s is configured', (_, info) => {
    const { link } = renderMark({}, info)

    expect(link).toHaveAccessibleName('Powered by VibeXP')
    expect(link).toHaveTextContent('Powered by VibeXP')
  })

  // No site info value can remove the mark: with every value set, it is still
  // there. The component takes no prop that hides it either.
  it('is still rendered with every site info value configured', () => {
    renderMark(
      {},
      {
        company_name: 'Acme Inc.',
        company_email: 'hello@acme.test',
        website: 'https://acme.test',
        support_url: 'https://acme.test/help',
        terms_url: 'https://acme.test/terms',
        privacy_url: 'https://acme.test/privacy',
        logo_url: 'https://cdn.acme.test/logo.svg',
        logo_dark_url: 'https://cdn.acme.test/dark.svg',
      }
    )

    expect(screen.getByTestId('sidebar-product-mark')).toBeInTheDocument()
    expect(screen.getByRole('link')).toHaveAttribute(
      'href',
      'https://vibexp.io'
    )
  })

  describe('label visibility', () => {
    const label = (link: HTMLElement) => link.querySelector('span')

    it('hides the label in the collapsed rail', () => {
      const { link } = renderMark({ expanded: false })

      expect(label(link)).toHaveClass('hidden')
      expect(label(link)).not.toHaveClass('lg:inline')
    })

    it('shows the label at lg+ in the expanded sidebar', () => {
      const { link } = renderMark({ expanded: true })

      expect(label(link)).toHaveClass('hidden', 'lg:inline')
    })

    it('shows the label at every size in a drawer', () => {
      const { link } = renderMark({ showText: true })

      expect(label(link)).toHaveClass('inline')
      expect(label(link)).not.toHaveClass('hidden')
    })
  })

  // ---------------------------------------------------------------------------
  // #891: the tooltip trigger must BE the link. jsdom cannot see where the
  // bubble lands, but it can see which element Radix made the popper anchor:
  // the one that receives the trigger props. A `display: contents` wrapper
  // there has no box, and the tooltip lands on the viewport origin.
  // ---------------------------------------------------------------------------
  describe('tooltip trigger anchors on the link itself (#891)', () => {
    it('puts the Radix trigger state on the <a>, not on a wrapper', () => {
      const { link, container } = renderMark({ expanded: false })

      expect(link).toHaveAttribute('data-state', 'closed')
      expect(container.querySelector('.contents')).toBeNull()
    })

    it('describes the hovered link itself once the tooltip opens', async () => {
      const user = userEvent.setup()
      const { link, container } = renderMark(
        { expanded: false },
        { company_name: 'Acme Inc.' }
      )

      await user.hover(link)

      const tooltip = await screen.findByRole('tooltip')
      expect(tooltip).toHaveTextContent('Powered by VibeXP')
      expect(link).toHaveAttribute('aria-describedby', tooltip.id)
      expect(link).toHaveAttribute('data-state', 'delayed-open')
      // Portalled, so no overflow ancestor in the sidebar can clip it.
      expect(container.contains(tooltip)).toBe(false)
    })

    it('renders no tooltip in a drawer, where the label is always visible', async () => {
      const user = userEvent.setup()
      const { link } = renderMark({ showText: true })

      expect(link).not.toHaveAttribute('data-state')
      await user.hover(link)
      expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
    })

    it('needs no TooltipProvider in a drawer', () => {
      render(<SidebarProductMark showText />)

      expect(screen.getByRole('link')).toHaveAttribute(
        'href',
        'https://vibexp.io'
      )
    })
  })
})
