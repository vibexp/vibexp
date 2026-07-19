import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { SessionCounts } from '@/services/aiToolsService'

jest.mock('@/services/aiToolsService', () => ({
  aiToolsService: {
    getClaudeCodeSessionCounts: jest.fn(),
    getCursorIDESessionCounts: jest.fn(),
    getClaudeCodeOverviewStats: jest.fn(),
    getClaudeCodeRecentActivities: jest.fn(),
    getCursorIDEOverviewStats: jest.fn(),
    getCursorIDERecentActivities: jest.fn(),
  },
}))

import { aiToolsService } from '@/services/aiToolsService'

import { AIToolsOverview } from '../AIToolsOverview'

function buildCounts(total: number): SessionCounts {
  return { total_sessions: total, counts: [] }
}

function renderOverview() {
  return render(
    <MemoryRouter initialEntries={['/ai-tools']}>
      <Routes>
        <Route path="/ai-tools" element={<AIToolsOverview />} />
        <Route
          path="/ai-tools/claude-code/overview"
          element={<div data-testid="claude-probe">Claude Code probe</div>}
        />
        <Route
          path="/ai-tools/cursor-ide/overview"
          element={<div data-testid="cursor-probe">Cursor IDE probe</div>}
        />
      </Routes>
    </MemoryRouter>
  )
}

const claudeCountsMock = aiToolsService.getClaudeCodeSessionCounts as jest.Mock
const cursorCountsMock = aiToolsService.getCursorIDESessionCounts as jest.Mock

describe('AIToolsOverview page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    claudeCountsMock.mockResolvedValue(buildCounts(12))
    cursorCountsMock.mockResolvedValue(buildCounts(5))
  })

  it('renders the page shell and both tool cards', async () => {
    renderOverview()

    expect(screen.getByText('AI Tools')).toBeInTheDocument()
    expect(screen.getByText('Total tools')).toBeInTheDocument()
    expect(screen.getByText('Active sessions (7d)')).toBeInTheDocument()
    expect(screen.getByText('Connected tools')).toBeInTheDocument()
    expect(screen.getByText('Claude Code')).toBeInTheDocument()
    expect(screen.getByText('Cursor IDE')).toBeInTheDocument()

    await waitFor(() => {
      expect(claudeCountsMock).toHaveBeenCalledWith('7d')
    })
    expect(cursorCountsMock).toHaveBeenCalledWith('7d')
  })

  it('shows per-tool session counts and the 7d total once loaded', async () => {
    renderOverview()

    await waitFor(() => {
      expect(screen.getByText('12')).toBeInTheDocument()
    })
    expect(screen.getByText('5')).toBeInTheDocument()
    // 12 + 5 aggregated into the Active sessions stat card.
    expect(screen.getByText('17')).toBeInTheDocument()
    // Both tools report as connected.
    expect(screen.getAllByText('Connected')).toHaveLength(2)
  })

  it('navigates to the Claude Code overview from its Manage button', async () => {
    renderOverview()
    await screen.findByText('12')

    const user = userEvent.setup()
    const [claudeManage] = screen.getAllByRole('button', { name: 'Manage' })
    await user.click(claudeManage)

    expect(screen.getByTestId('claude-probe')).toBeInTheDocument()
  })

  it('navigates to the Cursor IDE overview from its Manage button', async () => {
    renderOverview()
    await screen.findByText('12')

    const user = userEvent.setup()
    const manageButtons = screen.getAllByRole('button', { name: 'Manage' })
    await user.click(manageButtons[1])

    expect(screen.getByTestId('cursor-probe')).toBeInTheDocument()
  })

  it('logs the failure and settles at zero sessions when the fetch fails', async () => {
    const consoleError = jest
      .spyOn(console, 'error')
      .mockImplementation(() => undefined)
    claudeCountsMock.mockRejectedValue(new Error('metrics down'))
    cursorCountsMock.mockRejectedValue(new Error('metrics down'))

    renderOverview()

    await waitFor(() => {
      expect(consoleError).toHaveBeenCalledWith(
        'Failed to fetch session counts:',
        expect.any(Error)
      )
    })
    // Loading is over: the aggregate and both per-tool counts fall back to 0.
    expect(screen.getAllByText('0').length).toBeGreaterThanOrEqual(3)

    consoleError.mockRestore()
  })
})
