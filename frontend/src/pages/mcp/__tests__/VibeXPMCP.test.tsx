import { render, screen } from '@testing-library/react'

import type { Team } from '@/services/teamService'

const mockUseTeam = vi.hoisted(() => vi.fn())

vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

vi.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess: vi.fn() }),
  useAnalytics: () => ({ trackEvent: vi.fn() }),
}))

import { VibeXPMCP } from '../VibeXPMCP'

const team: Team = {
  id: 'uuid-aaa',
  owner_id: 'owner-1',
  name: 'Acme Team',
  slug: 'acme-team',
  description: '',
  permissions: [],
  member_count: 1,
  is_personal: false,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

describe('VibeXPMCP setup steps', () => {
  beforeEach(() => {
    mockUseTeam.mockReturnValue({
      currentTeam: team,
      teams: [team],
      isLoading: false,
    })
  })

  it('describes step 3 as the standard MCP OAuth sign-in, with no team picking', () => {
    render(<VibeXPMCP />)

    expect(screen.getByText('Sign in')).toBeInTheDocument()
    const desc = screen.getByText(
      'Your client opens the browser to authorize via the standard MCP OAuth flow.'
    )
    expect(desc).toBeInTheDocument()
    expect(desc.textContent).not.toMatch(/team/i)
    expect(screen.queryByText(/pick a team/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/team_id per call/i)).not.toBeInTheDocument()
  })
  it('does not render the team identifiers section or any team UUID/slug', () => {
    render(<VibeXPMCP />)

    expect(screen.queryByText('Your team identifiers')).not.toBeInTheDocument()
    expect(screen.queryByText('uuid-aaa')).not.toBeInTheDocument()
    expect(screen.queryByText('acme-team')).not.toBeInTheDocument()
  })
})
