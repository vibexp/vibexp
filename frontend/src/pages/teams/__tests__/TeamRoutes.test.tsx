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
// The four configuration pages relocated in #540. Stubbed for the same reason:
// this file tests the routing table, not the pages.
jest.mock('@/pages/teams/settings/search/SearchSettings', () => ({
  SearchSettings: ({ team }: { team: Team }) => (
    <div data-testid="search">{team.name}</div>
  ),
}))
jest.mock('@/pages/teams/settings/model-providers/ModelProviders', () => ({
  ModelProviders: () => <div data-testid="model-providers" />,
}))
jest.mock(
  '@/pages/teams/settings/embedding-providers/EmbeddingProviders',
  () => ({
    EmbeddingProviders: () => <div data-testid="embedding-providers" />,
  })
)
jest.mock('@/pages/teams/settings/customization/Customization', () => ({
  Customization: () => <div data-testid="customization" />,
}))
jest.mock(
  '@/pages/teams/settings/integrations/github/GitHubIntegration',
  () => ({
    GitHubIntegration: () => <div data-testid="github-integration" />,
  })
)

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

  // #540 replaced the `settings/*` splat with explicit children, so a nested
  // settings path now renders its own page rather than the hub. That is the
  // #539 tripwire firing by design, not a regression.
  it.each([
    ['/teams/team-a/settings/search', 'search'],
    ['/teams/team-a/settings/model-providers', 'model-providers'],
    ['/teams/team-a/settings/embedding-providers', 'embedding-providers'],
    ['/teams/team-a/settings/customization', 'customization'],
    // #541 - note the nested path, which a non-splat sibling route still matches.
    ['/teams/team-a/settings/integrations/github', 'github-integration'],
  ])('renders the relocated page at %s', (path, testId) => {
    renderAt(path)
    expect(screen.getByTestId(testId)).toBeInTheDocument()
    expect(screen.queryByTestId('settings')).not.toBeInTheDocument()
  })

  it('passes the resolved team to SearchSettings for permission gating', () => {
    // SearchSettings gates on the URL's team, so the route must hand it over
    // rather than leaving it to read the ambient context.
    renderAt('/teams/team-a/settings/search')
    expect(screen.getByTestId('search')).toHaveTextContent('Engineering')
  })

  it('still 404s an unknown path under settings', () => {
    renderAt('/teams/team-a/settings/nonsense')
    expect(
      screen.getByRole('heading', { level: 1, name: /page not found/i })
    ).toBeInTheDocument()
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
