import { render, waitFor } from '@testing-library/react'
import { Helmet, HelmetProvider } from 'react-helmet-async'

import { deriveSiteInfo, EMPTY_SITE_INFO, type SiteInfo } from '../siteInfo'
import { SiteInfoContext } from '../SiteInfoContext'
import { SiteInfoTitle } from '../SiteInfoTitle'

function tree(info: Partial<SiteInfo>, children?: React.ReactNode) {
  return (
    <HelmetProvider>
      <SiteInfoContext.Provider
        value={deriveSiteInfo({ ...EMPTY_SITE_INFO, ...info })}
      >
        <SiteInfoTitle />
        {children}
      </SiteInfoContext.Provider>
    </HelmetProvider>
  )
}

describe('SiteInfoTitle', () => {
  it('is "VibeXP" when no company name is configured', async () => {
    render(tree({}))

    await waitFor(() => {
      expect(document.title).toBe('VibeXP')
    })
  })

  it('leads with the company name when one is configured', async () => {
    render(tree({ company_name: 'Acme Inc.' }))

    await waitFor(() => {
      expect(document.title).toBe('Acme Inc. · VibeXP')
    })
  })

  it('does not name the product twice when the company is called VibeXP', async () => {
    render(tree({ company_name: 'VIBEXP' }))

    await waitFor(() => {
      expect(document.title).toBe('VibeXP')
    })
  })

  it('is not changed by a logo alone', async () => {
    render(tree({ logo_url: 'https://cdn.acme.test/logo.svg' }))

    await waitFor(() => {
      expect(document.title).toBe('VibeXP')
    })
  })

  it('follows a site info change without a reload', async () => {
    const { rerender } = render(tree({}))
    await waitFor(() => {
      expect(document.title).toBe('VibeXP')
    })

    rerender(tree({ company_name: 'Acme Inc.' }))

    await waitFor(() => {
      expect(document.title).toBe('Acme Inc. · VibeXP')
    })
  })

  it("lets a page's own title win", async () => {
    render(
      tree(
        { company_name: 'Acme Inc.' },
        <Helmet>
          <title>Code Review | Shared Prompt | Acme Inc.</title>
        </Helmet>
      )
    )

    await waitFor(() => {
      expect(document.title).toBe('Code Review | Shared Prompt | Acme Inc.')
    })
  })
})
