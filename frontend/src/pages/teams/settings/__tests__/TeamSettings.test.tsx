import { render, screen } from '@testing-library/react'
import { Shapes, SlidersHorizontal } from 'lucide-react'
import { MemoryRouter } from 'react-router-dom'

import type { SettingItem } from '@/components/settings/SettingsGrid'
import type { Team } from '@/services/teamService'

// Drives the card list per test. The real builder returns [] until #540/#541
// relocate pages here, so mocking it is the only way to execute the grid path
// the acceptance criteria describe ("renders the hub using the shared grid").
const cards: { items: SettingItem[] } = { items: [] }
jest.mock('@/pages/teams/settings/team-settings-cards', () => ({
  teamSettingsCardsFor: (teamId: string) =>
    cards.items.map(item => ({
      ...item,
      href: item.href.replace(':id', teamId),
    })),
}))

import { TeamSettings } from '../TeamSettings'

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

function renderHub() {
  return render(
    <MemoryRouter>
      <TeamSettings team={team} />
    </MemoryRouter>
  )
}

describe('TeamSettings', () => {
  beforeEach(() => {
    cards.items = []
  })

  it('names the team being configured', () => {
    renderHub()
    // The AC's point: this hub looks identical to the personal one, so the user
    // must be able to tell WHICH team they are configuring.
    expect(
      screen.getByRole('heading', { level: 1, name: /team settings/i })
    ).toBeInTheDocument()
    expect(screen.getByText(/Engineering/)).toBeInTheDocument()
  })

  it('explains the empty state while pages are still being relocated', () => {
    renderHub()
    expect(
      screen.getByText(/no team settings have moved here yet/i)
    ).toBeInTheDocument()
    expect(screen.queryAllByRole('button')).toHaveLength(0)
  })

  it('renders cards through the shared SettingSection grid when there are any', () => {
    cards.items = [
      {
        title: 'Search Settings',
        description: 'Choose how search results are ranked.',
        icon: SlidersHorizontal,
        href: '/teams/:id/settings/search',
      },
      {
        title: 'Artifact Types',
        description: 'Custom categories for artifacts.',
        icon: Shapes,
        href: '/teams/:id/settings/customization',
      },
    ]

    renderHub()

    // Same interactive card the personal hub renders - one role=button per
    // item - which is what makes the two hubs identical by construction.
    expect(screen.getAllByRole('button')).toHaveLength(2)
    expect(screen.getByText('Search Settings')).toBeInTheDocument()
    expect(
      screen.queryByText(/no team settings have moved here yet/i)
    ).not.toBeInTheDocument()
  })

  it('builds card hrefs under the resolved team id', () => {
    cards.items = [
      {
        title: 'Search Settings',
        description: 'Choose how search results are ranked.',
        icon: SlidersHorizontal,
        href: '/teams/:id/settings/search',
      },
    ]

    renderHub()

    // The hub must scope its links to the team from the URL, not the ambient
    // one - otherwise deep-linking to team B offers links into team A.
    expect(
      screen.getByRole('button', { name: /Search Settings/ })
    ).toBeInTheDocument()
    expect(
      screen.getByText('Choose how search results are ranked.')
    ).toBeInTheDocument()
  })
})
