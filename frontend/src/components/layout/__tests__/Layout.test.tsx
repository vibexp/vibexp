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

  it('centers the whole reading row while a reading page is mounted', () => {
    render(
      <Layout>
        <ReadingPageStub />
      </Layout>
    )
    const main = screen.getByRole('main')
    expect(main).toHaveAttribute('data-content-mode', 'reading')
    // Not the narrower `contained` cap...
    expect(main.querySelector('.max-w-screen-xl')).toBeNull()
    // ...but the row that holds the article AND the details rail is centered
    // as one group, so the leftover width splits evenly (#888).
    const row = screen.getByTestId('reading-row')
    expect(row).toHaveClass('mx-auto', 'max-w-screen-2xl', 'w-full')
    expect(row).toContainElement(screen.getByText('reading'))
  })

  it('adds no containing block that would break the sticky details rail', () => {
    render(
      <Layout>
        <ReadingPageStub />
      </Layout>
    )
    // `position: sticky` resolves against the nearest scroll container, and
    // `transform`/`filter` create a containing block — any of these on the
    // centering wrapper would pin the rail to the row instead of the viewport.
    const row = screen.getByTestId('reading-row')
    for (const cls of Array.from(row.classList)) {
      expect(cls).not.toMatch(/^(overflow|transform|filter|blur)(-|$)/)
    }
  })
})
