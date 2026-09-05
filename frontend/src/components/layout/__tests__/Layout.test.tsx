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
    expect(main.querySelector('.max-w-screen-xl')).toBeNull()
    expect(screen.getByText('reading')).toBeInTheDocument()
  })
})
