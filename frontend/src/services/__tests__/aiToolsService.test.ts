// Mock the generated client; unwrap stays real so the service exercises the
// same success/error resolution production uses (see searchService.test.ts).
const mockGeneratedClient = {
  GET: jest.fn(),
}

jest.mock('../../lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return {
    ...actual,
    generatedClient: mockGeneratedClient,
  }
})

import { aiToolsService } from '../aiToolsService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

describe('AIToolsService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('unwraps the envelope and returns the inner data for overview stats', async () => {
    const stats = {
      total_sessions: 3,
      sessions_this_week: 1,
      sessions_last_week: 2,
      weekly_trend_percent: -50,
      avg_user_prompts_per_session: 4,
      total_unique_tools: 5,
      top_tools: [{ tool_name: 'Bash', count: 2 }],
      avg_session_duration_minutes: 12,
      total_memories: 7,
    }
    mockGeneratedClient.GET.mockReturnValue(
      success({ status: 'success', message: 'ok', data: stats })
    )

    await expect(aiToolsService.getClaudeCodeOverviewStats()).resolves.toEqual(
      stats
    )
    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/ai-tools/claude-code/overview-stats'
    )
  })

  it('passes the range query and unwraps session counts', async () => {
    const counts = {
      total_sessions: 5,
      counts: [{ date: '2026-05-01T00:00:00Z', count: 5 }],
    }
    mockGeneratedClient.GET.mockReturnValue(
      success({ status: 'success', message: 'ok', data: counts })
    )

    await expect(
      aiToolsService.getClaudeCodeSessionCounts('30d')
    ).resolves.toEqual(counts)
    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/ai-tools/claude-code/session-counts',
      { params: { query: { range: '30d' } } }
    )
  })

  it('defaults the session-counts range to 7d', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({
        status: 'success',
        message: 'ok',
        data: { total_sessions: 0, counts: [] },
      })
    )

    await aiToolsService.getCursorIDESessionCounts()
    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/ai-tools/cursor-ide/session-counts',
      { params: { query: { range: '7d' } } }
    )
  })

  it('unwraps cursor recent activities', async () => {
    const activities = {
      activities: [
        {
          session_id: 's1',
          tool_name: 'Edit',
          input: { file_path: '/tmp/x' },
          hook_event_name: 'PostToolUse',
          created_at: '2026-05-01T00:00:00Z',
        },
      ],
      page: 1,
      limit: 20,
      total: 1,
      total_pages: 1,
    }
    mockGeneratedClient.GET.mockReturnValue(
      success({ status: 'success', message: 'ok', data: activities })
    )

    await expect(
      aiToolsService.getCursorIDERecentActivities()
    ).resolves.toEqual(activities)
    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/ai-tools/cursor-ide/recent-activities'
    )
  })
})
