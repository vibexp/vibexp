import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router'

import {
  deriveSiteInfo,
  EMPTY_SITE_INFO,
  type SiteInfo,
} from '@/features/site-info/siteInfo'
import { SiteInfoContext } from '@/features/site-info/SiteInfoContext'

import { buildDocumentTitle, useDocumentTitle } from '../documentTitle'
import { ShellProvider, useBreadcrumbLeaf } from '../ShellContext'

function Titled({ leaf }: Readonly<{ leaf?: string }>) {
  useDocumentTitle()
  useBreadcrumbLeaf(leaf)
  return null
}

function renderAt(path: string, page: ReactNode, info: Partial<SiteInfo> = {}) {
  return render(
    <SiteInfoContext.Provider
      value={deriveSiteInfo({ ...EMPTY_SITE_INFO, ...info })}
    >
      <MemoryRouter initialEntries={[path]}>
        <ShellProvider>{page}</ShellProvider>
      </MemoryRouter>
    </SiteInfoContext.Provider>
  )
}

describe('buildDocumentTitle', () => {
  it('puts the most specific part first', () => {
    expect(
      buildDocumentTitle({
        leaf: 'Search ranking redesign — decision record',
        section: 'Artifacts',
        site: 'VibeXP',
      })
    ).toBe('Search ranking redesign — decision record · Artifacts · VibeXP')
  })

  it('leaves out what a page does not have', () => {
    expect(
      buildDocumentTitle({ leaf: null, section: 'Artifacts', site: 'VibeXP' })
    ).toBe('Artifacts · VibeXP')
    expect(
      buildDocumentTitle({ leaf: null, section: null, site: 'VibeXP' })
    ).toBe('VibeXP')
    expect(
      buildDocumentTitle({ leaf: '  ', section: 'Memories', site: 'VibeXP' })
    ).toBe('Memories · VibeXP')
  })
})

describe('useDocumentTitle', () => {
  afterEach(() => {
    document.title = ''
  })

  it('names the page and the site on a list page', () => {
    renderAt('/artifacts', <Titled />)

    expect(document.title).toBe('Artifacts · VibeXP')
  })

  it('names the resource first on a resource page, from the breadcrumb leaf', () => {
    renderAt('/artifacts/p1/doc', <Titled leaf="Release checklist" />)

    expect(document.title).toBe('Release checklist · Artifacts · VibeXP')
  })

  it('serves the edit and version pages of a resource the same way', () => {
    renderAt('/memories/m1/edit', <Titled leaf="Vitest mock hoisting trap" />)

    expect(document.title).toBe('Vitest mock hoisting trap · Memories · VibeXP')
  })

  it('uses the instance’s own site name where it is set', () => {
    renderAt('/prompts', <Titled />, { company_name: 'Acme' })

    expect(document.title).toBe('Prompts · Acme')
  })

  it.each([
    ['/', 'Dashboard · VibeXP'],
    ['/connect/cli', 'Connect · VibeXP'],
    ['/notifications', 'Notifications · VibeXP'],
    ['/somewhere/unknown', 'VibeXP'],
  ])('titles %s as %j', (path, title) => {
    renderAt(path, <Titled />)

    expect(document.title).toBe(title)
  })

  it('follows the page: the resource name goes when its page does', () => {
    const view = renderAt('/artifacts/p1/doc', <Titled leaf="Doc" />)
    expect(document.title).toBe('Doc · Artifacts · VibeXP')

    view.rerender(
      <SiteInfoContext.Provider value={deriveSiteInfo(EMPTY_SITE_INFO)}>
        <MemoryRouter initialEntries={['/artifacts/p1/doc']}>
          <ShellProvider>
            <Titled />
          </ShellProvider>
        </MemoryRouter>
      </SiteInfoContext.Provider>
    )

    expect(document.title).toBe('Artifacts · VibeXP')
    expect(screen.queryByText('Doc')).not.toBeInTheDocument()
  })
})
