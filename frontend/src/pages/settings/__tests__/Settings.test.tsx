import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import type { Team } from '@/services/teamService'

// Module-stable context object: a fresh currentTeam per render loops any effect
// keyed on its identity.
const teamContext: { currentTeam: Team | null; isLoading: boolean } = {
  currentTeam: null,
  isLoading: false,
}

vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({
    currentTeam: teamContext.currentTeam,
    isLoading: teamContext.isLoading,
    teams: teamContext.currentTeam ? [teamContext.currentTeam] : [],
    setCurrentTeam: vi.fn(),
    refreshTeams: vi.fn(),
  }),
}))

import { Settings } from '../Settings'

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

/** Every card this page is allowed to show (#543). */
const PERSONAL_CARDS = ['Activities', 'Notification Preferences', 'API Keys']

/** Destinations that moved out under /teams/:id/ during epic #536. */
const RELOCATED_CARDS = [
  'Teams',
  'Projects',
  'Artifact Types',
  'Search Settings',
  'Model Providers',
  'Embedding Providers',
  'GitHub Integration',
]

function renderSettings() {
  return render(
    <MemoryRouter>
      <Settings />
    </MemoryRouter>
  )
}

describe('Settings (personal)', () => {
  beforeEach(() => {
    teamContext.currentTeam = team
    teamContext.isLoading = false
  })

  it('describes itself as personal-only', () => {
    renderSettings()
    expect(
      screen.getByRole('heading', { level: 1, name: 'Settings' })
    ).toBeInTheDocument()
    expect(screen.getByText(/personal account settings/i)).toBeInTheDocument()
  })

  it.each(PERSONAL_CARDS)('keeps the %s card', title => {
    renderSettings()
    expect(screen.getByText(title)).toBeInTheDocument()
  })

  // The anti-creep guard: a team-scoped card re-added here fails this test
  // rather than silently reappearing on the personal page.
  it.each(RELOCATED_CARDS)('no longer shows the %s card', title => {
    renderSettings()
    expect(screen.queryByText(title)).not.toBeInTheDocument()
  })

  it('renders exactly the personal cards plus the team pointer', () => {
    renderSettings()
    // Strict count: three personal + one pointer. Any new card has to come here
    // and justify itself.
    expect(screen.getAllByRole('button')).toHaveLength(
      PERSONAL_CARDS.length + 1
    )
  })

  it('points at the current team settings hub, named after the team', () => {
    renderSettings()
    expect(screen.getByText('Team settings')).toBeInTheDocument()
    // Naming the team is what makes the destination unambiguous.
    expect(screen.getByText(/Engineering/)).toBeInTheDocument()
  })

  it('hides the team pointer while the team list is still loading', () => {
    teamContext.isLoading = true
    renderSettings()
    expect(screen.queryByText('Team settings')).not.toBeInTheDocument()
    expect(screen.getAllByRole('button')).toHaveLength(PERSONAL_CARDS.length)
  })

  it('hides the team pointer when there is no team at all', () => {
    // Never `/teams/undefined/settings`, never a crash - the AC's explicit case.
    teamContext.currentTeam = null
    renderSettings()
    expect(screen.queryByText('Team settings')).not.toBeInTheDocument()
    expect(screen.queryByText(/teams\/undefined/)).not.toBeInTheDocument()
    expect(screen.getAllByRole('button')).toHaveLength(PERSONAL_CARDS.length)
  })
})
