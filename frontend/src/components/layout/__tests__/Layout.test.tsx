import { render, screen } from '@testing-library/react'

import { Layout } from '../Layout'
import { useReadingShell } from '../ShellContext'

vi.mock('@/components/layout/Sidebar', () => ({
  Sidebar: () => <aside data-testid="sidebar" />,
}))
vi.mock('@/components/layout/Header', () => ({
  Header: () => <header data-testid="header" />,
}))
vi.mock('@/components/invitations/PendingInvitationsBanner', () => ({
  PendingInvitationsBanner: () => null,
}))

function ReadingPageStub() {
  useReadingShell({ details: true })
  return <div>reading</div>
}

describe('Layout', () => {
  it('frames ordinary pages in the centered container', () => {
    render(
      <Layout>
        <div>list page</div>
      </Layout>
    )
    const main = screen.getByRole('main')
    expect(main).toHaveAttribute('data-content-mode', 'contained')
    expect(main.querySelector('.max-w-screen-xl')).not.toBeNull()
    expect(screen.getByText('list page')).toBeInTheDocument()
  })

  it('goes full-bleed while a reading page is mounted', () => {
    render(
      <Layout>
        <ReadingPageStub />
      </Layout>
    )
    const main = screen.getByRole('main')
    expect(main).toHaveAttribute('data-content-mode', 'reading')
    // The reading row spans the whole content area and is NOT capped or
    // centered by the shell: the design pins the details column flush to the
    // right edge and centers the article in what is left beside it (#890).
    // Capping the row instead floated the column off the edge.
    const row = screen.getByTestId('reading-row')
    expect(row).toHaveClass('w-full', 'flex-1')
    for (const cls of Array.from(row.classList)) {
      expect(cls).not.toMatch(/^(mx-auto|max-w-)/)
    }
    expect(row).toContainElement(screen.getByText('reading'))
  })

  it('leaves the pending-invitations banner uncapped too', () => {
    render(
      <Layout>
        <ReadingPageStub />
      </Layout>
    )
    // The banner sits over the reading row, so a cap here would reintroduce
    // the same dead band the row no longer has.
    const banner = screen.getByRole('main').querySelector('.empty\\:hidden')
    expect(banner).not.toBeNull()
    for (const cls of Array.from(banner?.classList ?? [])) {
      expect(cls).not.toMatch(/^(mx-auto|max-w-)/)
    }
  })

  it('adds no containing block that would break the sticky details rail', () => {
    render(
      <Layout>
        <ReadingPageStub />
      </Layout>
    )
    // `position: sticky` resolves against the nearest scroll container, and
    // `transform`/`filter` create a containing block — any of these on the
    // row wrapper would pin the rail to the row instead of the viewport.
    const row = screen.getByTestId('reading-row')
    for (const cls of Array.from(row.classList)) {
      expect(cls).not.toMatch(/^(overflow|transform|filter|blur)(-|$)/)
    }
  })
})
