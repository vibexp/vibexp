import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import {
  deriveSiteInfo,
  EMPTY_SITE_INFO,
  type SiteInfo,
} from '@/features/site-info/siteInfo'
import { SiteInfoContext } from '@/features/site-info/SiteInfoContext'

import { OnboardingBrand } from '../OnboardingBrand'

function renderWith(info: Partial<SiteInfo> = {}) {
  return render(
    <SiteInfoContext.Provider
      value={deriveSiteInfo({ ...EMPTY_SITE_INFO, ...info })}
    >
      <OnboardingBrand />
    </SiteInfoContext.Provider>
  )
}

describe('OnboardingBrand', () => {
  it('is the VibeXP lockup on an unbranded instance', () => {
    const { container } = renderWith()
    expect(container.querySelector('svg')).not.toBeNull()
    expect(screen.queryByRole('img', { name: 'Company logo' })).toBeNull()
  })

  it("shows the company's name beside the mark when only a name is set", () => {
    renderWith({ company_name: 'Acme' })
    expect(screen.getByText('Acme')).toBeInTheDocument()
  })

  it("shows the company's logo when one is configured", () => {
    renderWith({ company_name: 'Acme', logo_url: 'https://acme.test/logo.png' })
    expect(screen.getByRole('img', { name: 'Acme' })).toHaveAttribute(
      'src',
      'https://acme.test/logo.png'
    )
  })
})
