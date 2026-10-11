import { render, screen } from '@testing-library/react'
import type React from 'react'
import { describe, expect, it, vi } from 'vitest'

import { PoweredBy } from '../PoweredBy'
import { deriveSiteInfo, EMPTY_SITE_INFO } from '../siteInfo'
import { SiteInfoContext } from '../SiteInfoContext'

vi.mock('@vibexp/design-system/react', () => ({
  LogoMark: ({
    size,
    ...rest
  }: { size?: number } & React.HTMLAttributes<HTMLSpanElement>) => (
    <span data-testid="vibexp-mark" data-size={size} {...rest} />
  ),
}))

describe('PoweredBy', () => {
  it('credits VibeXP with a link to the product site in a new tab', () => {
    render(<PoweredBy />)

    const link = screen.getByRole('link', { name: 'Powered by VibeXP' })
    expect(link).toHaveAttribute('href', 'https://vibexp.io')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    // The mark names itself "VibeXP"; hidden, it does not double the link's name.
    expect(screen.getByTestId('vibexp-mark')).toHaveAttribute(
      'aria-hidden',
      'true'
    )
  })

  it('keeps its own classes when the caller adds some', () => {
    render(<PoweredBy className="mt-4" />)

    const link = screen.getByRole('link', { name: 'Powered by VibeXP' })
    expect(link).toHaveClass('mt-4', 'inline-flex')
  })

  it('is the same whatever the instance has configured', () => {
    render(
      <SiteInfoContext.Provider
        value={deriveSiteInfo({
          ...EMPTY_SITE_INFO,
          company_name: 'Acme Inc.',
          website: 'https://www.acme.test',
          logo_url: 'https://cdn.acme.test/logo.svg',
        })}
      >
        <PoweredBy />
      </SiteInfoContext.Provider>
    )

    const link = screen.getByRole('link', { name: 'Powered by VibeXP' })
    expect(link).toHaveAttribute('href', 'https://vibexp.io')
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })
})
