import { render, screen } from '@testing-library/react'

import { PublicCardPage, PublicPageBrand } from '../PublicPageBrand'
import { deriveSiteInfo, EMPTY_SITE_INFO, type SiteInfo } from '../siteInfo'
import { SiteInfoContext } from '../SiteInfoContext'

const LOGO = 'https://cdn.acme.test/logo.svg'

function withInfo(info: Partial<SiteInfo>, node: React.ReactNode) {
  return (
    <SiteInfoContext.Provider
      value={deriveSiteInfo({ ...EMPTY_SITE_INFO, ...info })}
    >
      {node}
    </SiteInfoContext.Provider>
  )
}

describe('PublicPageBrand', () => {
  it('renders nothing on an unbranded instance', () => {
    const { container } = render(withInfo({}, <PublicPageBrand />))

    expect(container).toBeEmptyDOMElement()
  })

  it('does not count links alone as branding', () => {
    const { container } = render(
      withInfo({ website: 'https://acme.test' }, <PublicPageBrand />)
    )

    expect(container).toBeEmptyDOMElement()
  })

  it('shows the company logo and name', () => {
    render(
      withInfo(
        { company_name: 'Acme Inc.', logo_url: LOGO },
        <PublicPageBrand />
      )
    )

    expect(screen.getByRole('img', { name: 'Acme Inc.' })).toHaveAttribute(
      'src',
      LOGO
    )
    expect(screen.getByText('Acme Inc.')).toBeInTheDocument()
  })

  it('lets a logo without a name stand alone, never beside "VibeXP"', () => {
    render(withInfo({ logo_url: LOGO }, <PublicPageBrand />))

    const brand = screen.getByTestId('public-page-brand')
    expect(brand.querySelector('img')).toHaveAttribute('src', LOGO)
    expect(brand).toHaveTextContent('')
  })

  it('shows the VibeXP mark beside a company name that has no logo', () => {
    render(withInfo({ company_name: 'Acme Inc.' }, <PublicPageBrand />))

    const brand = screen.getByTestId('public-page-brand')
    expect(brand.querySelector('svg')).not.toBeNull()
    expect(brand.querySelector('img')).toBeNull()
    expect(brand).toHaveTextContent('Acme Inc.')
  })

  it('is plain content, not a link, without a website', () => {
    render(withInfo({ company_name: 'Acme Inc.' }, <PublicPageBrand />))

    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('links to the company website in a new tab when one is configured', () => {
    render(
      withInfo(
        { company_name: 'Acme Inc.', website: 'https://acme.test/?ref=app' },
        <PublicPageBrand />
      )
    )

    const link = screen.getByRole('link', { name: /acme inc\./i })
    expect(link).toHaveAttribute('href', 'https://acme.test/?ref=app')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })
})

describe('PublicCardPage', () => {
  const card = <div data-testid="card">card</div>

  it('is only the centred wrapper around its content when unbranded', () => {
    const { container } = render(
      withInfo({}, <PublicCardPage>{card}</PublicCardPage>)
    )

    const wrapper = container.firstElementChild
    expect(wrapper).toHaveClass(
      'min-h-screen',
      'items-center',
      'justify-center'
    )
    // The card is the wrapper's direct and only child: no lockup, no credit,
    // no extra column. This is the markup the pages had before.
    expect(wrapper?.children).toHaveLength(1)
    expect(wrapper?.firstElementChild).toBe(screen.getByTestId('card'))
    expect(screen.queryByText(/powered by/i)).not.toBeInTheDocument()
  })

  it('puts the company lockup above the card and the credit below when branded', () => {
    const { container } = render(
      withInfo(
        { company_name: 'Acme Inc.', logo_url: LOGO },
        <PublicCardPage>{card}</PublicCardPage>
      )
    )

    const column = container.firstElementChild?.firstElementChild
    expect(column?.children).toHaveLength(3)
    expect(column?.children[0]).toBe(screen.getByTestId('public-page-brand'))
    expect(column?.children[1]).toBe(screen.getByTestId('card'))
    const credit = screen.getByRole('link', { name: /powered by vibexp/i })
    expect(column?.children[2]).toBe(credit)
    expect(credit).toHaveAttribute('href', 'https://vibexp.io')
  })

  it('credits VibeXP for a dark-only logo too', () => {
    render(
      withInfo(
        { logo_dark_url: 'https://cdn.acme.test/dark.svg' },
        <PublicCardPage>{card}</PublicCardPage>
      )
    )

    expect(
      screen.getByRole('link', { name: /powered by vibexp/i })
    ).toBeInTheDocument()
  })
})
