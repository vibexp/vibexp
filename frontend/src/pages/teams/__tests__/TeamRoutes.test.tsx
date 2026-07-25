import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Team } from '@/services/teamService'

// The three pages have their own suites; stub them so this file tests the
// routing table itself — which path renders what, and the in-shell 404.
jest.mock('@/pages/teams/TeamDetailsPage', () => ({
  TeamDetailsPage: () => <div data-testid="details" />,
}))
jest.mock('@/pages/teams/TeamAnalyticsPage', () => ({
  TeamAnalyticsPage: () => <div data-testid="analytics" />,
}))
jest.mock('@/pages/teams/settings/TeamSettings', () => ({
  TeamSettings: ({ team }: { team: Team }) => (
    <div data-testid="settings">{team.name}</div>
  ),
}))

import { TeamRoutes } from '../TeamRoutes'

const team: Team = {
  id: 'team-a',
  owner_id: 'owner-1',
  name: 'Engineering',
  slug: 'engineering',
  description: '',
  is_personal: false,
  permissions: [],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

/** Mounted exactly as `routes.tsx` mounts it: a splat route. */
function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/teams/:id/*" element={<TeamRoutes team={team} />} />
      </Routes>
    </MemoryRouter>
  )
}

describe('TeamRoutes', () => {
  it('renders the details page at the team index', () => {
    renderAt('/teams/team-a')
    expect(screen.getByTestId('details')).toBeInTheDocument()
  })

  it('renders the analytics page at /analytics', () => {
    renderAt('/teams/team-a/analytics')
    expect(screen.getByTestId('analytics')).toBeInTheDocument()
  })

  it('renders the settings hub at /settings, passing the resolved team', () => {
    renderAt('/teams/team-a/settings')
    expect(screen.getByTestId('settings')).toHaveTextContent('Engineering')
  })

  it('keeps the settings hub mounted on a nested settings path', () => {
    // `settings/*`, not `settings`: #540 and #541 nest pages underneath, and a
    // bare `settings` path would 404 every one of them.
    renderAt('/teams/team-a/settings/search')
    expect(screen.getByTestId('settings')).toBeInTheDocument()
  })

  it('renders an in-shell not-found for an unknown path under the scope', () => {
    // The acceptance criterion: /teams/:id/nonsense must NOT fall through to
    // the app-level NotFound, which would drop the team chrome and tab bar.
    renderAt('/teams/team-a/nonsense')

    expect(
      screen.getByRole('heading', { level: 1, name: /page not found/i })
    ).toBeInTheDocument()
    expect(
      screen.getByText(/that team page does not exist/i)
    ).toBeInTheDocument()
    expect(screen.queryByTestId('details')).not.toBeInTheDocument()
  })
})
