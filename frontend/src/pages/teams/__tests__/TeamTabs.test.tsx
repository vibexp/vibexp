import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'

import { TeamTabs } from '../TeamTabs'

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <TeamTabs teamId="team-a" />
    </MemoryRouter>
  )
}

const current = (name: RegExp) =>
  screen.getByRole('link', { name }).getAttribute('aria-current')

describe('TeamTabs', () => {
  it('renders the tab bar as a labelled navigation region', () => {
    renderAt('/teams/team-a')
    expect(
      screen.getByRole('navigation', { name: /team sections/i })
    ).toBeInTheDocument()
  })

  it('marks Overview active on the team index only', () => {
    renderAt('/teams/team-a')
    expect(current(/^overview$/i)).toBe('page')
    expect(current(/^analytics$/i)).toBeNull()
    expect(current(/^settings$/i)).toBeNull()
  })

  it('marks Analytics active on the analytics route, and not Overview', () => {
    renderAt('/teams/team-a/analytics')
    expect(current(/^analytics$/i)).toBe('page')
    // The `end` flag on Overview is what keeps it inactive here - its href is a
    // prefix of this path.
    expect(current(/^overview$/i)).toBeNull()
  })

  it('marks Settings active on the settings route, and not Overview', () => {
    renderAt('/teams/team-a/settings')
    expect(current(/^settings$/i)).toBe('page')
    expect(current(/^overview$/i)).toBeNull()
  })

  it('keeps Settings active on a nested settings path (#540/#541 children)', () => {
    renderAt('/teams/team-a/settings/search')
    expect(current(/^settings$/i)).toBe('page')
  })
})
