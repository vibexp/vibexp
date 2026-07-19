import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type {
  OverviewStats,
  RecentActivities,
  RecentActivity,
} from '@/services/aiToolsService'

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

import { ClaudeCodeOverview } from '../claude-code/ClaudeCodeOverview'
import { CursorIDEOverview } from '../cursor-ide/CursorIDEOverview'

function buildStats(overrides: Partial<OverviewStats> = {}): OverviewStats {
  return {
    total_sessions: 42,
    sessions_this_week: 3,
    sessions_last_week: 2,
    weekly_trend_percent: 12.5,
    avg_user_prompts_per_session: 3.46,
    total_unique_tools: 7,
    top_tools: [
      { tool_name: 'Bash', count: 20 },
      { tool_name: 'Read', count: 15 },
      { tool_name: 'Edit', count: 10 },
      { tool_name: 'Grep', count: 5 },
    ],
    avg_session_duration_minutes: 95,
    total_memories: 12,
    ...overrides,
  }
}

function buildActivity(
  overrides: Partial<RecentActivity> = {}
): RecentActivity {
  return {
    session_id: 'session-1',
    cwd: '/home/dev/vibexp',
    tool_name: 'Bash',
    tool_input: { command: 'ls -la' },
    hook_event_name: 'PreToolUse',
    created_at: new Date().toISOString(),
    ...overrides,
  }
}

function buildActivities(activities: RecentActivity[]): RecentActivities {
  return {
    activities,
    page: 1,
    limit: 10,
    total: activities.length,
    total_pages: 1,
  }
}

function renderClaudeOverview() {
  return render(
    <MemoryRouter initialEntries={['/ai-tools/claude-code/overview']}>
      <Routes>
        <Route
          path="/ai-tools/claude-code/overview"
          element={<ClaudeCodeOverview />}
        />
        <Route
          path="/ai-tools/claude-code/sessions"
          element={<div data-testid="sessions-probe">Sessions probe</div>}
        />
        <Route
          path="/ai-tools/claude-code/setup"
          element={<div data-testid="setup-probe">Setup probe</div>}
        />
      </Routes>
    </MemoryRouter>
  )
}

const statsMock = aiToolsService.getClaudeCodeOverviewStats as jest.Mock
const activitiesMock = aiToolsService.getClaudeCodeRecentActivities as jest.Mock
const cursorStatsMock = aiToolsService.getCursorIDEOverviewStats as jest.Mock
const cursorActivitiesMock =
  aiToolsService.getCursorIDERecentActivities as jest.Mock

describe('ToolOverview via ClaudeCodeOverview', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    statsMock.mockResolvedValue(buildStats())
    activitiesMock.mockResolvedValue(buildActivities([]))
  })

  it('renders the stat cards from the fetched overview stats', async () => {
    renderClaudeOverview()

    await waitFor(() => {
      expect(screen.getByText('42')).toBeInTheDocument()
    })
    expect(
      screen.getByText('+12.5% vs last week (3 this week)')
    ).toBeInTheDocument()
    // 95 minutes formatted as hours + minutes.
    expect(screen.getByText('1h 35m')).toBeInTheDocument()
    expect(screen.getByText('95.0 minutes average')).toBeInTheDocument()
    // Top tools trimmed to the first three.
    expect(screen.getByText('Top: Bash, Read, Edit')).toBeInTheDocument()
    expect(screen.getByText('7')).toBeInTheDocument()
    // Average prompts rounded to one decimal.
    expect(screen.getByText('3.5')).toBeInTheDocument()
  })

  it('formats a flat trend, sub-hour duration and missing tool data', async () => {
    statsMock.mockResolvedValue(
      buildStats({
        weekly_trend_percent: 0,
        avg_session_duration_minutes: 45,
        top_tools: [],
        total_unique_tools: 0,
      })
    )

    renderClaudeOverview()

    await waitFor(() => {
      expect(screen.getByText('No change from last week')).toBeInTheDocument()
    })
    expect(screen.getByText('45m')).toBeInTheDocument()
    expect(screen.getByText('No tool data')).toBeInTheDocument()
  })

  it('shows the empty state when there is no recent activity', async () => {
    renderClaudeOverview()

    await waitFor(() => {
      expect(screen.getByText('No recent activity')).toBeInTheDocument()
    })
    expect(
      screen.getByText('Claude Code sessions and tool events will appear here.')
    ).toBeInTheDocument()
  })

  it('renders recent activities with tool input, cwd and relative time', async () => {
    const now = Date.now()
    activitiesMock.mockResolvedValue(
      buildActivities([
        buildActivity({ created_at: new Date(now - 10 * 1000).toISOString() }),
        buildActivity({
          session_id: 'session-2',
          tool_name: 'Grep',
          tool_input: { pattern: 'TODO' },
          cwd: null,
          created_at: new Date(now - 5 * 60 * 1000).toISOString(),
        }),
        buildActivity({
          session_id: 'session-3',
          tool_name: 'Read',
          tool_input: { file_path: '/tmp/notes.md' },
          cwd: '/',
          created_at: new Date(now - 3 * 3600 * 1000).toISOString(),
        }),
        buildActivity({
          session_id: 'session-4',
          tool_name: null,
          tool_input: null,
          cwd: null,
          created_at: new Date(now - 2 * 86400 * 1000).toISOString(),
        }),
      ])
    )

    renderClaudeOverview()

    await waitFor(() => {
      expect(screen.getByText('Bash')).toBeInTheDocument()
    })
    // Input formatting per shape: command, pattern, file_path.
    expect(screen.getByText('(ls -la)')).toBeInTheDocument()
    expect(screen.getByText('(TODO)')).toBeInTheDocument()
    expect(screen.getByText('(/tmp/notes.md)')).toBeInTheDocument()
    // cwd renders its last path segment; a bare "/" falls back to the full cwd.
    expect(screen.getByText('(vibexp)')).toBeInTheDocument()
    expect(screen.getByText('(/)')).toBeInTheDocument()
    // Null tool_name falls back to System.
    expect(screen.getByText('System')).toBeInTheDocument()
    // Relative timestamps across the branches.
    expect(screen.getByText('just now')).toBeInTheDocument()
    expect(screen.getByText('5m ago')).toBeInTheDocument()
    expect(screen.getByText('3h ago')).toBeInTheDocument()
    expect(screen.getByText('2d ago')).toBeInTheDocument()
    // Hook event badges.
    expect(screen.getAllByText('PreToolUse')).toHaveLength(4)
  })

  it('navigates to sessions from the header and the activity footer', async () => {
    activitiesMock.mockResolvedValue(buildActivities([buildActivity()]))

    renderClaudeOverview()
    await screen.findByText('Bash')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'View all sessions' }))

    expect(screen.getByTestId('sessions-probe')).toBeInTheDocument()
  })

  it('navigates to the sessions page from the Sessions button', async () => {
    renderClaudeOverview()
    await screen.findByText('No recent activity')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /Sessions/ }))

    expect(screen.getByTestId('sessions-probe')).toBeInTheDocument()
  })

  it('navigates to the setup page from the Setup button', async () => {
    renderClaudeOverview()
    await screen.findByText('No recent activity')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /Setup/ }))

    expect(screen.getByTestId('setup-probe')).toBeInTheDocument()
  })

  it('logs the failure and falls back to zeroed stats when fetching fails', async () => {
    const consoleError = jest
      .spyOn(console, 'error')
      .mockImplementation(() => undefined)
    statsMock.mockRejectedValue(new Error('stats down'))
    activitiesMock.mockRejectedValue(new Error('stats down'))

    renderClaudeOverview()

    await waitFor(() => {
      expect(consoleError).toHaveBeenCalledWith(
        'Failed to fetch overview data:',
        expect.any(Error)
      )
    })
    expect(screen.getByText('No recent activity')).toBeInTheDocument()
    expect(screen.getByText('0m')).toBeInTheDocument()

    consoleError.mockRestore()
  })
})

describe('CursorIDEOverview wiring', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    cursorStatsMock.mockResolvedValue(
      buildStats({ total_sessions: 9, weekly_trend_percent: -50 })
    )
    cursorActivitiesMock.mockResolvedValue(buildActivities([]))
  })

  it('fetches the Cursor IDE stats and renders its own shell', async () => {
    render(
      <MemoryRouter initialEntries={['/ai-tools/cursor-ide/overview']}>
        <Routes>
          <Route
            path="/ai-tools/cursor-ide/overview"
            element={<CursorIDEOverview />}
          />
        </Routes>
      </MemoryRouter>
    )

    await waitFor(() => {
      expect(screen.getByText('9')).toBeInTheDocument()
    })
    expect(cursorStatsMock).toHaveBeenCalled()
    expect(cursorActivitiesMock).toHaveBeenCalled()
    // Claude Code service methods are untouched on the Cursor page.
    expect(statsMock).not.toHaveBeenCalled()
    expect(screen.getByText('Cursor IDE')).toBeInTheDocument()
    // Negative trends keep their sign.
    expect(
      screen.getByText('-50.0% vs last week (3 this week)')
    ).toBeInTheDocument()
    expect(
      screen.getByText('Cursor IDE sessions and tool events will appear here.')
    ).toBeInTheDocument()
  })
})
