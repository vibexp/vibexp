/**
 * Tests for the redesigned dashboard page composition: the eight Overview cards,
 * Quick actions, the Analytics section (charts vs. empty state), Top accessed,
 * and the side-by-side Activity lists. The analytics charts and the Top accessed
 * grid are stubbed so this test focuses on Home's own wiring and data flow.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import type {
  ActivitiesResponse,
  Activity as ActivityType,
} from '@/services/activityService'
import type { FeedItem, FeedItemListResponse } from '@/services/feedService'
import type { TeamStats } from '@/services/teamService'

const mockNavigate = jest.fn()
jest.mock('react-router-dom', () => ({
  ...jest.requireActual<typeof import('react-router-dom')>('react-router-dom'),
  useNavigate: () => mockNavigate,
}))

jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({ currentTeam: { id: 'team-1', name: 'Test Team' } }),
}))

jest.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({
    user: {
      id: 'user-1',
      name: 'Alice Test',
      created_at: '2023-01-01T00:00:00Z',
    },
  }),
}))

jest.mock('@/services/activityService', () => ({
  activityService: { getActivities: jest.fn() },
}))
jest.mock('@/services/feedService', () => ({
  feedService: { getFeedItems: jest.fn() },
}))
jest.mock('@/services/teamService', () => ({
  teamService: {
    getTeamStats: jest.fn(),
    getTeamResourceCreationMetrics: jest.fn(),
    getTeamFeedCreationMetrics: jest.fn(),
  },
}))
jest.mock('@/services/agentService', () => ({
  agentService: { getAgentStats: jest.fn() },
}))
jest.mock('@/services/aiToolsService', () => ({
  aiToolsService: { getClaudeCodeOverviewStats: jest.fn() },
}))

// Analytics widgets and the top-accessed grid are covered by their own tests.
jest.mock('@/components/TeamResourceAccessChart', () => ({
  TeamResourceAccessChart: () => <div data-testid="access-chart" />,
}))
jest.mock('@/components/TeamResourceCreationChart', () => ({
  TeamResourceCreationChart: () => <div data-testid="creation-chart" />,
}))
jest.mock('@/components/TeamResourceCumulativeChart', () => ({
  TeamResourceCumulativeChart: () => <div data-testid="cumulative-chart" />,
}))
jest.mock('@/components/TeamFeedCreationChart', () => ({
  TeamFeedCreationChart: () => <div data-testid="feed-chart" />,
}))
jest.mock('@/pages/home/TopAccessedResources', () => ({
  TopAccessedResources: () => <div data-testid="top-accessed" />,
}))
jest.mock('@/pages/home/activityHelpers', () => ({
  getActivityIcon: () =>
    function MockIcon() {
      return <svg />
    },
}))

import { activityService } from '@/services/activityService'
import { agentService } from '@/services/agentService'
import { aiToolsService } from '@/services/aiToolsService'
import { feedService } from '@/services/feedService'
import { teamService } from '@/services/teamService'

import { Home } from '../Home'

const mockActivity = activityService as jest.Mocked<typeof activityService>
const mockFeed = feedService as jest.Mocked<typeof feedService>
const mockTeam = teamService as jest.Mocked<typeof teamService>
const mockAgent = agentService as jest.Mocked<typeof agentService>
const mockAiTools = aiToolsService as jest.Mocked<typeof aiToolsService>

function makeStats(overrides: Partial<TeamStats> = {}): TeamStats {
  return {
    total_projects: 1,
    total_prompts: 4,
    total_artifacts: 110,
    total_blueprints: 24,
    total_memories: 51,
    total_feed_items: 1862,
    ...overrides,
  }
}

function makeActivity(overrides: Partial<ActivityType> = {}): ActivityType {
  return {
    id: 'act-1',
    user_id: 'user-1',
    activity_type: 'prompt.created',
    entity_type: 'prompt',
    entity_id: 'uuid-abc-123',
    entity_name: null,
    actor_name: null,
    session_id: null,
    description: 'Created a prompt',
    metadata: {},
    source_ip: null,
    user_agent: null,
    created_at: new Date().toISOString(),
    ...overrides,
  }
}

function activitiesResponse(items: ActivityType[]): ActivitiesResponse {
  return {
    status: 'success',
    message: 'ok',
    data: {
      activities: items,
      total_count: items.length,
      page: 1,
      per_page: 25,
      total_pages: 1,
    },
  }
}

function makeFeedItem(overrides: Partial<FeedItem> = {}): FeedItem {
  return {
    id: 'feed-1',
    team_id: 'team-1',
    feed_id: 'feed-general',
    project_id: null,
    title: 'AI generated a summary',
    content: 'full content',
    excerpt: 'A short excerpt',
    ai_assistant_name: 'Claude Code CLI',
    posted_by_user_id: 'user-1',
    archived_at: null,
    posted_at: new Date().toISOString(),
    reply_count: 0,
    ...overrides,
  }
}

function feedResponse(items: FeedItem[]): FeedItemListResponse {
  return {
    items,
    total_count: items.length,
    page: 1,
    per_page: 10,
    total_pages: 1,
  }
}

function emptyMetrics() {
  return {
    status: 'success',
    message: 'ok',
    data: { total_created: 0, range: '7d' as const, counts: [] },
  }
}

function setup(stats: TeamStats = makeStats(), activities = [makeActivity()]) {
  mockActivity.getActivities.mockResolvedValue(activitiesResponse(activities))
  mockFeed.getFeedItems.mockResolvedValue(feedResponse([]))
  mockTeam.getTeamStats.mockResolvedValue(stats)
  mockTeam.getTeamResourceCreationMetrics.mockResolvedValue(emptyMetrics())
  mockTeam.getTeamFeedCreationMetrics.mockResolvedValue(emptyMetrics())
  mockAgent.getAgentStats.mockResolvedValue({
    status: 'success',
    message: 'ok',
    data: {
      total_agents: 9,
      active_agents: 9,
      paused_agents: 0,
      error_agents: 0,
      total_runs: 0,
      avg_success_rate: 0,
      runs_today: 0,
      runs_this_week: 0,
      recent_activities: [],
    },
  })
  mockAiTools.getClaudeCodeOverviewStats.mockResolvedValue({
    total_sessions: 0,
    sessions_this_week: 0,
    sessions_last_week: 0,
    weekly_trend_percent: 0,
    avg_user_prompts_per_session: 0,
    total_unique_tools: 0,
    top_tools: [],
    avg_session_duration_minutes: 0,
    total_memories: 51,
  })
}

beforeEach(() => {
  jest.clearAllMocks()
})

function renderHome() {
  render(
    <MemoryRouter>
      <Home />
    </MemoryRouter>
  )
}

describe('Home — overview', () => {
  it('renders the eight overview cards with values from the metric sources', async () => {
    setup()
    renderHome()

    expect(await screen.findByText('Total Artifacts')).toBeInTheDocument()
    expect(screen.getByText('AI Sessions')).toBeInTheDocument()
    expect(screen.getByText('Total Agents')).toBeInTheDocument()
    expect(screen.getByText('AI Feed updates')).toBeInTheDocument()
    expect(screen.getByText('MCP Tools')).toBeInTheDocument()

    // Values resolve (1,862 feed items formatted; 110 artifacts).
    await waitFor(() => {
      expect(screen.getByText('1,862')).toBeInTheDocument()
    })
    expect(screen.getByText('110')).toBeInTheDocument()
  })

  it('renders the design-spec quick actions', async () => {
    setup()
    renderHome()

    expect(await screen.findByText('Create Prompt')).toBeInTheDocument()
    expect(screen.getByText('Setup MCP Server')).toBeInTheDocument()
    expect(screen.getByText('Manage Blueprints')).toBeInTheDocument()
  })
})

describe('Home — analytics', () => {
  it('renders the four charts and range tabs when the workspace has resources', async () => {
    setup()
    renderHome()

    expect(await screen.findByTestId('access-chart')).toBeInTheDocument()
    expect(screen.getByTestId('cumulative-chart')).toBeInTheDocument()
    expect(screen.getByTestId('feed-chart')).toBeInTheDocument()
    expect(screen.getByTestId('top-accessed')).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '30 days' })).toBeInTheDocument()
  })

  it('shows the analytics empty state for a workspace with no resources', async () => {
    setup(
      makeStats({
        total_projects: 0,
        total_prompts: 0,
        total_artifacts: 0,
        total_blueprints: 0,
        total_memories: 0,
        total_feed_items: 0,
      })
    )
    renderHome()

    expect(
      await screen.findByText('Start building your workspace')
    ).toBeInTheDocument()
    expect(screen.queryByTestId('access-chart')).not.toBeInTheDocument()
    // The range tabs are hidden in the empty state.
    expect(
      screen.queryByRole('tab', { name: '30 days' })
    ).not.toBeInTheDocument()
  })
})

describe('Home — activity', () => {
  it('fetches feed items team-scoped and renders feed before activity', async () => {
    setup()
    mockFeed.getFeedItems.mockResolvedValue(feedResponse([makeFeedItem()]))
    renderHome()

    await waitFor(() => {
      expect(mockFeed.getFeedItems).toHaveBeenCalledWith('team-1', {
        limit: 10,
        archived: 'false',
        page: 1,
      })
    })
    const feedHeading = await screen.findByText('Recent AI feed')
    const activityHeading = screen.getByText('Recent activity')
    expect(
      feedHeading.compareDocumentPosition(activityHeading) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  it('navigates to /feeds from the feed "See more"', async () => {
    setup(makeStats(), [])
    mockFeed.getFeedItems.mockResolvedValue(feedResponse([makeFeedItem()]))
    renderHome()

    const seeMore = await screen.findAllByRole('button', { name: /see more/i })
    fireEvent.click(seeMore[0])
    expect(mockNavigate).toHaveBeenCalledWith('/feeds')
  })

  it('surfaces the feed error state when the fetch fails', async () => {
    setup()
    mockFeed.getFeedItems.mockRejectedValue(new Error('boom'))
    renderHome()

    await waitFor(() => {
      expect(
        screen.getByText('Failed to load recent feed items')
      ).toBeInTheDocument()
    })
  })
})
