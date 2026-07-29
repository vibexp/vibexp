import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import type { Mock } from 'vitest'

import type { Team, TeamStats } from '@/services/teamService'

vi.mock('@/services/teamService', () => ({
  teamService: {
    getTeamDetails: vi.fn(),
    getTeamStats: vi.fn(),
  },
}))

// Stub the charts so the page test stays focused on layout + wiring (the charts
// have their own tests). Each stub echoes the range it received so we can assert
// a single page-level filter drives both.
vi.mock('@/components/TeamResourceAccessChart', () => ({
  TeamResourceAccessChart: ({ range }: { range: string }) => (
    <div data-testid="access-chart">access:{range}</div>
  ),
}))
vi.mock('@/components/TeamResourceCreationChart', () => ({
  TeamResourceCreationChart: ({ range }: { range: string }) => (
    <div data-testid="creation-chart">creation:{range}</div>
  ),
}))

import { teamService } from '@/services/teamService'

import { TeamAnalyticsPage } from '../TeamAnalyticsPage'

const mockTeam = { id: 'team-1', name: 'Test Team' } as Team

const mockStats: TeamStats = {
  total_projects: 4,
  total_prompts: 25,
  total_artifacts: 13,
  total_blueprints: 6,
  total_memories: 40,
  total_feed_items: 52,
}

function renderPage(id = 'team-1') {
  return render(
    <MemoryRouter initialEntries={[`/teams/${id}/analytics`]}>
      <Routes>
        <Route path="/teams/:id/analytics" element={<TeamAnalyticsPage />} />
      </Routes>
    </MemoryRouter>
  )
}

describe('TeamAnalyticsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders stat cards and both charts once data loads', async () => {
    ;(teamService.getTeamDetails as Mock).mockResolvedValue(mockTeam)
    ;(teamService.getTeamStats as Mock).mockResolvedValue(mockStats)

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Team Analytics')).toBeInTheDocument()
    })

    // Stat cards (projects, prompts, artifacts, blueprints, memories, feed items).
    expect(screen.getByText('Projects')).toBeInTheDocument()
    expect(screen.getByText('4')).toBeInTheDocument()
    expect(screen.getByText('Feed Items')).toBeInTheDocument()
    expect(screen.getByText('52')).toBeInTheDocument()

    // Both charts render and receive the same default range (single page filter).
    expect(screen.getByTestId('access-chart')).toHaveTextContent('access:30d')
    expect(screen.getByTestId('creation-chart')).toHaveTextContent(
      'creation:30d'
    )
  })

  it('shows an error alert when loading fails', async () => {
    ;(teamService.getTeamDetails as Mock).mockRejectedValue(new Error('boom'))
    ;(teamService.getTeamStats as Mock).mockRejectedValue(new Error('boom'))

    renderPage()

    await waitFor(() => {
      expect(
        screen.getByText('Could not load team analytics')
      ).toBeInTheDocument()
    })
    expect(screen.queryByTestId('access-chart')).not.toBeInTheDocument()
  })
})
