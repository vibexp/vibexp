import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router'

import { HeaderBreadcrumb } from '../HeaderBreadcrumb'
import { ShellProvider, useBreadcrumbLeaf } from '../ShellContext'

function Page({ leaf }: Readonly<{ leaf?: string | null }>) {
  useBreadcrumbLeaf(leaf)
  return null
}

function renderAt(path: string, page: ReactNode) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <ShellProvider>
        <HeaderBreadcrumb />
        {page}
      </ShellProvider>
    </MemoryRouter>
  )
}

describe('HeaderBreadcrumb — naming the resource', () => {
  it('names the resource as the last crumb and links the section to its list', () => {
    renderAt(
      '/artifacts/p1/search-ranking-redesign',
      <Page leaf="Search ranking redesign" />
    )

    const nav = screen.getByRole('navigation', { name: 'Breadcrumb' })
    expect(nav).toHaveTextContent('VibeXP/Artifacts/Search ranking redesign')
    expect(screen.getByRole('link', { name: 'Artifacts' })).toHaveAttribute(
      'href',
      '/artifacts'
    )
    const leaf = screen.getByText('Search ranking redesign')
    expect(leaf).toHaveAttribute('aria-current', 'page')
    // The title is the crumb that truncates; it never wraps the header.
    expect(leaf).toHaveClass('truncate')
  })

  it('serves the edit and version pages of a resource the same way', () => {
    renderAt('/artifacts/p1/doc/edit', <Page leaf="Doc" />)

    expect(screen.getByRole('link', { name: 'Artifacts' })).toHaveAttribute(
      'href',
      '/artifacts'
    )
    expect(screen.getByText('Doc')).toHaveAttribute('aria-current', 'page')
  })

  it.each([[undefined], [null], [''], ['   ']])(
    'keeps the plain section crumb for a leaf of %j',
    leaf => {
      renderAt('/artifacts', <Page leaf={leaf} />)

      expect(screen.queryByRole('link')).not.toBeInTheDocument()
      expect(screen.getByText('Artifacts')).toHaveClass('font-semibold')
    }
  )

  it('drops the leaf when the page that named it goes away', () => {
    const view = renderAt('/artifacts/p1/doc', <Page leaf="Doc" />)
    expect(screen.getByText('Doc')).toBeInTheDocument()

    view.rerender(
      <MemoryRouter initialEntries={['/artifacts/p1/doc']}>
        <ShellProvider>
          <HeaderBreadcrumb />
        </ShellProvider>
      </MemoryRouter>
    )

    expect(screen.queryByText('Doc')).not.toBeInTheDocument()
  })
})
