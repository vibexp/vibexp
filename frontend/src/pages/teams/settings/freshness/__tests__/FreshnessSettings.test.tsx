import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import type { Team } from '@/services/teamService'

const mockUseTeam = vi.hoisted(() => vi.fn())

vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

// The three tab bodies are exercised by their own suites; here they are stubbed
// so this suite tests the shell — which tab mounts, and when.
vi.mock('../FreshnessRulesTab', () => ({
  FreshnessRulesTab: () => <div data-testid="rules-tab" />,
}))
vi.mock('../FreshnessAnalytics', () => ({
  FreshnessAnalytics: ({ teamId }: { teamId: string }) => (
    <div data-testid="analytics-tab">{teamId}</div>
  ),
}))
vi.mock('../FreshnessAudit', () => ({
  FreshnessAudit: ({ teamId }: { teamId: string }) => (
    <div data-testid="audit-tab">{teamId}</div>
  ),
}))

import { FreshnessSettings } from '../FreshnessSettings'

const team = {
  id: 'team-1',
  name: 'Test Team',
  permissions: ['resource.create'],
} as unknown as Team

beforeEach(() => {
  vi.clearAllMocks()
  mockUseTeam.mockReturnValue({
    currentTeam: team,
    teams: [team],
    isLoading: false,
    setCurrentTeam: vi.fn(),
    refreshTeams: vi.fn() as () => Promise<void>,
  })
})

const renderPage = () =>
  render(
    <MemoryRouter>
      <FreshnessSettings team={team} />
    </MemoryRouter>
  )

describe('FreshnessSettings tab shell', () => {
  it('renders the page title once, above the tabs', () => {
    renderPage()
    expect(screen.getAllByText('Resource Freshness')).toHaveLength(1)
  })

  it('opens on the settings tab', () => {
    renderPage()
    expect(screen.getByTestId('rules-tab')).toBeInTheDocument()
  })

  it('does not mount analytics or audit until their tab is opened', () => {
    // Radix unmounts inactive content, which is what keeps arriving on the page
    // to one settings + one rules request instead of six.
    renderPage()

    expect(screen.queryByTestId('analytics-tab')).not.toBeInTheDocument()
    expect(screen.queryByTestId('audit-tab')).not.toBeInTheDocument()
  })

  it('shows analytics when its tab is selected', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('tab', { name: 'Analytics' }))

    await waitFor(() => {
      expect(screen.getByTestId('analytics-tab')).toBeInTheDocument()
    })
    expect(screen.getByTestId('analytics-tab')).toHaveTextContent('team-1')
  })

  it('shows the audit log when its tab is selected', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('tab', { name: 'Audit' }))

    await waitFor(() => {
      expect(screen.getByTestId('audit-tab')).toBeInTheDocument()
    })
  })

  it('offers analytics and audit to a member who cannot edit settings', async () => {
    // Explicit product decision: the engine writes to everyone's resources, so
    // every member may inspect what it did — only writes are gated, inside the
    // settings tab.
    const user = userEvent.setup()
    renderPage()

    expect(screen.getByRole('tab', { name: 'Analytics' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Audit' })).toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: 'Audit' }))
    await waitFor(() => {
      expect(screen.getByTestId('audit-tab')).toBeInTheDocument()
    })
  })
})
