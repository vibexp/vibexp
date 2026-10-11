import { fireEvent, render, screen, within } from '@testing-library/react'

import { SiteInfoLogoPreview } from '../SiteInfoLogoPreview'

const LOGO = 'https://cdn.acme.test/logo.svg'
const DARK = 'https://cdn.acme.test/logo-dark.svg'

const light = () => screen.getByTestId('site-info-preview-light')
const dark = () => screen.getByTestId('site-info-preview-dark')
const image = (surface: HTMLElement) => within(surface).queryByRole('img')

describe('SiteInfoLogoPreview', () => {
  it('shows the VibeXP mark on both surfaces when no logo is set', () => {
    render(<SiteInfoLogoPreview logoUrl="" logoDarkUrl="" />)

    for (const surface of [light(), dark()]) {
      expect(image(surface)).toBeNull()
      expect(surface.querySelector('svg')).not.toBeNull()
      expect(surface).toHaveTextContent('No logo set')
    }
    expect(
      screen.queryByTestId('site-info-preview-dark-fallback')
    ).not.toBeInTheDocument()
  })

  it('shows each logo on its own surface', () => {
    render(<SiteInfoLogoPreview logoUrl={LOGO} logoDarkUrl={DARK} />)

    expect(image(light())).toHaveAttribute('src', LOGO)
    expect(image(dark())).toHaveAttribute('src', DARK)
    expect(
      screen.queryByTestId('site-info-preview-dark-fallback')
    ).not.toBeInTheDocument()
  })

  it('uses the logo on the dark surface when no dark logo is set, and says so', () => {
    render(<SiteInfoLogoPreview logoUrl={LOGO} logoDarkUrl="" />)

    expect(image(dark())).toHaveAttribute('src', LOGO)
    expect(
      screen.getByTestId('site-info-preview-dark-fallback')
    ).toHaveTextContent(/dark theme uses the logo above/i)
  })

  it('keeps the mark on the light surface for a dark-only logo, as the app does', () => {
    render(<SiteInfoLogoPreview logoUrl="" logoDarkUrl={DARK} />)

    expect(image(light())).toBeNull()
    expect(image(dark())).toHaveAttribute('src', DARK)
  })

  it('falls back to the mark when the image fails, and retries a corrected URL', () => {
    const { rerender } = render(
      <SiteInfoLogoPreview logoUrl={LOGO} logoDarkUrl={DARK} />
    )

    const broken = image(light())
    if (!broken) throw new Error('no light image')
    fireEvent.error(broken)

    expect(image(light())).toBeNull()
    expect(light()).toHaveTextContent(/could not be loaded/i)
    // Only the surface whose image failed falls back.
    expect(image(dark())).toHaveAttribute('src', DARK)

    rerender(
      <SiteInfoLogoPreview
        logoUrl="https://cdn.acme.test/fixed.svg"
        logoDarkUrl={DARK}
      />
    )
    expect(image(light())).toHaveAttribute(
      'src',
      'https://cdn.acme.test/fixed.svg'
    )
  })

  it.each(['cdn.acme.test/logo.svg', 'https://', 'javascript:alert(1)'])(
    'requests nothing for the incomplete or unsafe value %s',
    value => {
      render(<SiteInfoLogoPreview logoUrl={value} logoDarkUrl="" />)

      expect(screen.queryByRole('img')).toBeNull()
    }
  )

  it('trims the URL it requests', () => {
    render(<SiteInfoLogoPreview logoUrl={`  ${LOGO}  `} logoDarkUrl="" />)

    expect(image(light())).toHaveAttribute('src', LOGO)
  })
})
