/**
 * Tests for the dashboard page: the order of its blocks (what needs attention,
 * what changed, how the knowledge is used, then totals and growth), what each
 * block derives from the API, and the empty-workspace state. The charts and
 * the Top accessed grid are stubbed so this focuses on Home's own wiring.
 */
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import type { Mocked } from 'vitest'

import type {
  ActivitiesResponse,
  Activity as ActivityType,
} from '@/services/activityService'
import type { FeedItem, FeedItemListResponse } from '@/services/feedService'
import type { TeamStats } from '@/services/teamService'

// Mutable so a test can select a project in the header.
const mockProject = vi.hoisted(() => ({
  current: null as { id: string; name: string } | null,
}))
vi.mock('@/contexts/ProjectContext', () => ({
  useProject: () => ({ currentProject: mockProject.current, isLoading: false }),
}))

vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({ currentTeam: { id: 'team-1', name: 'Test Team' } }),
}))

vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({
    user: {
      id: 'user-1',
      name: 'Alice Test',
      created_at: '2023-01-01T00:00:00Z',
    },
  }),
}))

vi.mock('@/services/activityService', () => ({
  activityService: { getActivities: vi.fn() },
}))
vi.mock('@/services/feedService', () => ({
  feedService: { getFeedItems: vi.fn() },
}))
vi.mock('@/services/teamService', () => ({
  teamService: {
    getTeamStats: vi.fn(),
    getTeamResourceCreationMetrics: vi.fn(),
    getTeamFeedCreationMetrics: vi.fn(),
    getTeamMembers: vi.fn(),
    getTeamResourceAccessMetrics: vi.fn(),
  },
}))
vi.mock('@/services/artifactService', () => ({
  artifactService: { getArtifacts: vi.fn() },
}))
vi.mock('@/services/blueprintService', () => ({
  blueprintService: { getBlueprints: vi.fn() },
}))
vi.mock('@/services/memoryService', () => ({
  memoryService: { getMemories: vi.fn() },
}))
vi.mock('@/services/promptService', () => ({
  promptService: { getPrompts: vi.fn() },
}))
vi.mock('@/services/commentService', () => ({
  commentService: { recent: vi.fn() },
}))
vi.mock('@/services/agentService', () => ({
  agentService: { getAgentStats: vi.fn() },
}))

// Analytics widgets and the top-accessed grid are covered by their own tests.
vi.mock('@/components/TeamResourceAccessChart', () => ({
  TeamResourceAccessChart: () => <div data-testid="access-chart" />,
}))
vi.mock('@/components/TeamResourceCreationChart', () => ({
  TeamResourceCreationChart: () => <div data-testid="creation-chart" />,
}))
vi.mock('@/components/TeamResourceCumulativeChart', () => ({
  TeamResourceCumulativeChart: () => <div data-testid="cumulative-chart" />,
}))
vi.mock('@/components/TeamFeedCreationChart', () => ({
  TeamFeedCreationChart: () => <div data-testid="feed-chart" />,
}))
vi.mock('@/pages/home/TopAccessedResources', () => ({
  TopAccessedResources: () => <div data-testid="top-accessed" />,
}))
vi.mock('@/pages/home/activityHelpers', () => ({
  getActivityIcon: () =>
    function MockIcon() {
      return <svg />
    },
}))

import { activityService } from '@/services/activityService'
import { agentService } from '@/services/agentService'
import { artifactService } from '@/services/artifactService'
import { blueprintService } from '@/services/blueprintService'
import { commentService } from '@/services/commentService'
import { feedService } from '@/services/feedService'
import { memoryService } from '@/services/memoryService'
import { promptService } from '@/services/promptService'
import { teamService } from '@/services/teamService'

import { Home } from '../Home'

const mockActivity = activityService as Mocked<typeof activityService>
const mockFeed = feedService as Mocked<typeof feedService>
const mockTeam = teamService as Mocked<typeof teamService>
const mockAgent = agentService as Mocked<typeof agentService>
const mockComment = commentService as Mocked<typeof commentService>
const mockArtifacts = artifactService as Mocked<typeof artifactService>
const mockBlueprints = blueprintService as Mocked<typeof blueprintService>
const mockMemories = memoryService as Mocked<typeof memoryService>
const mockPrompts = promptService as Mocked<typeof promptService>

const page = { page: 1, per_page: 10, total_pages: 1 }

/** A list-endpoint mock that answers the stale query and the recent one apart. */
function listMock<T>(
  key: string,
  recent: T[],
  stale: T[] = [],
  staleTotal?: number
) {
  return (_teamId: string, filters: { freshness?: string } = {}) => {
    const rows = filters.freshness === 'stale' ? stale : recent
    const total =
      filters.freshness === 'stale' ? (staleTotal ?? stale.length) : rows.length
    return Promise.resolve({ [key]: rows, total_count: total, ...page })
  }
}

function artifact(overrides: Record<string, unknown> = {}) {
  return {
    id: 'art-1',
    title: 'Release checklist',
    slug: 'release-checklist',
    project_id: 'proj-1',
    user_id: 'user-1',
    created_at: '2026-10-01T10:00:00Z',
    updated_at: '2026-10-01T10:00:00Z',
    ...overrides,
  }
}

function memory(overrides: Record<string, unknown> = {}) {
  return {
    id: 'mem-1',
    title: 'Vitest mock hoisting trap',
    text: 'Declare mock variables with vi.hoisted.',
    project_id: 'proj-1',
    user_id: 'user-1',
    created_at: '2026-10-02T10:00:00Z',
    updated_at: '2026-10-02T10:00:00Z',
    ...overrides,
  }
}

function accessMetrics(
  day: Partial<Record<'web' | 'cli' | 'mcp' | 'api', number>>
) {
  const counts = { web: 0, cli: 0, mcp: 0, api: 0, ...day }
  const total = counts.web + counts.cli + counts.mcp + counts.api
  return {
    status: 'success',
    message: 'ok',
    data: {
      total_accesses: total,
      range: '30d' as const,
      counts: [{ date: '2026-10-01', ...counts, total }],
    },
  }
}

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
  mockComment.recent.mockResolvedValue({ comments: [], total_count: 0 })
  mockTeam.getTeamMembers.mockResolvedValue([])
  mockTeam.getTeamStats.mockResolvedValue(stats)
  mockTeam.getTeamResourceCreationMetrics.mockResolvedValue(emptyMetrics())
  mockTeam.getTeamFeedCreationMetrics.mockResolvedValue(emptyMetrics())
  mockTeam.getTeamResourceAccessMetrics.mockResolvedValue(
    accessMetrics({ web: 41 })
  )
  mockArtifacts.getArtifacts.mockImplementation(
    listMock('artifacts', [artifact()]) as never
  )
  mockBlueprints.getBlueprints.mockImplementation(
    listMock('blueprints', []) as never
  )
  mockMemories.getMemories.mockImplementation(
    listMock('memories', [memory()]) as never
  )
  mockPrompts.getPrompts.mockImplementation(listMock('prompts', []) as never)
  mockAgent.getAgentStats.mockResolvedValue({
    total_agents: 9,
    active_agents: 9,
    paused_agents: 0,
    error_agents: 0,
    total_runs: 0,
    avg_success_rate: 0,
    runs_today: 0,
    runs_this_week: 0,
    recent_activities: [],
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  mockProject.current = null
})

const section = (name: string) => screen.getByRole('region', { name })

function renderHome() {
  render(
    <MemoryRouter>
      <Home />
    </MemoryRouter>
  )
}

describe('Home — page order', () => {
  it('leads with what needs attention and what changed, and demotes the counts', async () => {
    setup()
    renderHome()

    await screen.findByText('Release checklist')
    const order = screen
      .getAllByRole('heading', { level: 2 })
      .map(heading => heading.textContent)

    expect(order).toEqual([
      'Needs your attention',
      'What changed',
      'How your knowledge is used',
      'Workspace totals',
      'Growth',
    ])
  })

  it('keeps the greeting and drops the subtitle and the quick actions', async () => {
    setup()
    renderHome()

    expect(
      await screen.findByRole('heading', {
        level: 1,
        name: 'Welcome back, Alice!',
      })
    ).toBeInTheDocument()
    expect(
      screen.queryByText(/Your shared brain is ready/)
    ).not.toBeInTheDocument()
    expect(screen.queryByText('Quick actions')).not.toBeInTheDocument()
    expect(screen.queryByText('Create Prompt')).not.toBeInTheDocument()
  })
})

describe('Home — needs your attention', () => {
  it('says so calmly when nothing does', async () => {
    setup()
    renderHome()

    expect(
      await screen.findByText('Nothing needs your attention right now.')
    ).toBeInTheDocument()
  })

  it('counts stale resources per type and links to the pre-filtered list', async () => {
    setup()
    mockMemories.getMemories.mockImplementation(
      listMock(
        'memories',
        [memory()],
        [memory({ id: 'mem-stale', title: 'Old deploy notes' })],
        5
      ) as never
    )
    renderHome()

    const attention = within(section('Needs your attention'))
    expect(
      await attention.findByRole('link', { name: '5 stale memories' })
    ).toHaveAttribute('href', '/memories?freshness=stale')
    expect(
      attention.getByRole('link', { name: 'Old deploy notes' })
    ).toHaveAttribute('href', '/memories/mem-stale')
    expect(attention.getByText(/and 4 more/)).toBeInTheDocument()
    // The stale query is its own request, scoped like the page.
    expect(mockMemories.getMemories).toHaveBeenCalledWith(
      'team-1',
      expect.objectContaining({ freshness: 'stale' })
    )
  })

  it('lists feed posts with zero replies, and never one that has a reply', async () => {
    setup()
    mockFeed.getFeedItems.mockResolvedValue(
      feedResponse([
        makeFeedItem({ id: 'feed-open', title: 'Dual-run finished' }),
        makeFeedItem({
          id: 'feed-answered',
          title: 'Already discussed',
          reply_count: 2,
        }),
      ])
    )
    renderHome()

    const attention = within(section('Needs your attention'))
    expect(
      await attention.findByRole('link', { name: 'Dual-run finished' })
    ).toHaveAttribute('href', '/feeds/items/feed-open')
    expect(attention.getByText(/no replies yet/)).toBeInTheDocument()
    expect(attention.queryByText('Already discussed')).not.toBeInTheDocument()
    expect(attention.queryByText(/awaiting/i)).not.toBeInTheDocument()
  })

  it('does not call it all clear when a source could not be checked', async () => {
    setup()
    mockFeed.getFeedItems.mockRejectedValue(new Error('boom'))
    renderHome()

    expect(
      await screen.findByText('Could not check feed posts just now.')
    ).toBeInTheDocument()
    expect(
      screen.queryByText('Nothing needs your attention right now.')
    ).not.toBeInTheDocument()
    // One failing request does not blank the rest of the page.
    expect(screen.getByText('Release checklist')).toBeInTheDocument()
    expect(screen.getByTestId('access-chart')).toBeInTheDocument()
  })
})

describe('Home — what changed', () => {
  it('merges recent resources across types, newest first, as real links', async () => {
    setup()
    renderHome()

    const changed = within(section('What changed'))
    const links = await changed.findAllByRole('link')

    // The memory (Oct 2) is newer than the artifact (Oct 1).
    expect(links.map(link => link.getAttribute('href'))).toEqual([
      '/memories/mem-1',
      '/artifacts/proj-1/release-checklist',
    ])
    expect(links[0]).toHaveTextContent('Vitest mock hoisting trap')
    expect(links[0]).toHaveTextContent('Created memory')
  })

  it('names the creator of a new resource, and nobody for an edit', async () => {
    setup()
    mockTeam.getTeamMembers.mockResolvedValue([
      {
        user_id: 'user-1',
        email: 'a@x.com',
        name: 'Alice',
        role: 'member',
        joined_at: '2026-01-01T00:00:00Z',
      },
    ])
    mockArtifacts.getArtifacts.mockImplementation(
      listMock('artifacts', [
        artifact({ updated_at: '2026-10-09T10:00:00Z' }),
      ]) as never
    )
    renderHome()

    const changed = within(section('What changed'))
    expect(
      await changed.findByText('Created memory · Alice')
    ).toBeInTheDocument()
    // The API records who created a resource, not who last edited it.
    expect(changed.getByText('Updated artifact')).toBeInTheDocument()
  })

  it('holds the feed, comments and activity lists behind one view switch', async () => {
    const user = userEvent.setup()
    setup()
    mockFeed.getFeedItems.mockResolvedValue(feedResponse([makeFeedItem()]))
    renderHome()

    const changed = within(section('What changed'))
    await changed.findByText('Vitest mock hoisting trap')
    expect(changed.queryByText('Created a prompt')).not.toBeInTheDocument()

    await user.click(changed.getByRole('tab', { name: 'AI feed' }))
    expect(changed.getByText('AI generated a summary')).toBeInTheDocument()
    expect(changed.getByRole('link', { name: /See all/ })).toHaveAttribute(
      'href',
      '/feeds'
    )

    await user.click(changed.getByRole('tab', { name: 'Your activity' }))
    expect(changed.getByText('Created a prompt')).toBeInTheDocument()

    await user.click(changed.getByRole('tab', { name: 'Comments' }))
    expect(
      changed.getByText('No comments yet in this team.')
    ).toBeInTheDocument()
  })

  it('scopes its requests to the project selected in the header', async () => {
    mockProject.current = { id: 'proj-7', name: 'Project 7' }
    setup()
    renderHome()

    await screen.findByText('Release checklist')
    for (const list of [
      mockArtifacts.getArtifacts,
      mockBlueprints.getBlueprints,
      mockMemories.getMemories,
      mockPrompts.getPrompts,
    ]) {
      expect(list).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ project_id: 'proj-7' })
      )
    }
    expect(mockFeed.getFeedItems).toHaveBeenCalledWith(
      'team-1',
      expect.objectContaining({ project_id: 'proj-7' })
    )
  })
})

describe('Home — how your knowledge is used', () => {
  it('leads with reads by channel and says when no AI tool has read anything', async () => {
    setup()
    renderHome()

    const usage = within(section('How your knowledge is used'))
    expect(
      await usage.findByText(
        /No AI tool has read from this workspace in the last 30 days\./
      )
    ).toBeInTheDocument()
    expect(
      usage.getByRole('link', { name: /Connect one over MCP/ })
    ).toHaveAttribute('href', '/mcp-servers/vibexp-mcp')
    // The plain number, for the one channel that was used.
    expect(usage.getByText('Web').nextElementSibling).toHaveTextContent('41')
    expect(usage.getByText('MCP').nextElementSibling).toHaveTextContent('0')
    expect(usage.getByTestId('access-chart')).toBeInTheDocument()
    expect(usage.getByTestId('top-accessed')).toBeInTheDocument()
  })

  it('does not say so once an AI tool has read through MCP', async () => {
    setup()
    mockTeam.getTeamResourceAccessMetrics.mockResolvedValue(
      accessMetrics({ web: 41, mcp: 3 })
    )
    renderHome()

    const usage = within(section('How your knowledge is used'))
    await waitFor(() => {
      expect(usage.getByText('MCP').nextElementSibling).toHaveTextContent('3')
    })
    expect(usage.queryByText(/No AI tool has read/)).not.toBeInTheDocument()
  })

  it('shows no write-back figure anywhere on the page', async () => {
    setup()
    renderHome()

    await screen.findByText('Release checklist')
    expect(
      screen.queryByText(/written by ai|write-?back/i)
    ).not.toBeInTheDocument()
  })

  it('reloads the reads when the range changes', async () => {
    const user = userEvent.setup()
    setup()
    renderHome()

    const usage = within(section('How your knowledge is used'))
    await usage.findByText('Reads by channel in the last 30 days.')
    await user.click(usage.getByRole('tab', { name: '7 days' }))

    await waitFor(() => {
      expect(mockTeam.getTeamResourceAccessMetrics).toHaveBeenCalledWith(
        'team-1',
        '7d'
      )
    })
    expect(
      await usage.findByText('Reads by channel in the last 7 days.')
    ).toBeInTheDocument()
  })
})

describe('Home — workspace totals and growth', () => {
  it('shows the counts as one compact row, without MCP tools', async () => {
    setup()
    renderHome()

    const totals = within(section('Workspace totals'))
    await waitFor(() => {
      expect(totals.getByText('1,862')).toBeInTheDocument()
    })
    expect(totals.getByText('Artifacts').closest('div')).toHaveTextContent(
      '110'
    )
    expect(totals.getByText('Agents').closest('div')).toHaveTextContent('9')
    expect(screen.queryByText(/MCP Tools/i)).not.toBeInTheDocument()
  })

  it('keeps the remaining charts below everything else', async () => {
    setup()
    renderHome()

    const growth = within(section('Growth'))
    expect(await growth.findByTestId('creation-chart')).toBeInTheDocument()
    expect(growth.getByTestId('cumulative-chart')).toBeInTheDocument()
    expect(growth.getByTestId('feed-chart')).toBeInTheDocument()
  })
})

describe('Home — empty workspace', () => {
  const emptyStats = makeStats({
    // Every team has a default project; it does not make a workspace non-empty.
    total_projects: 1,
    total_prompts: 0,
    total_artifacts: 0,
    total_blueprints: 0,
    total_memories: 0,
    total_feed_items: 0,
  })

  it('shows a compact getting-started block: connect an AI tool, then create', async () => {
    setup(emptyStats)
    renderHome()

    const started = within(
      await screen.findByRole('region', { name: 'Get started' })
    )
    const steps = started.getAllByRole('listitem')
    expect(steps[0]).toHaveTextContent('1. Connect an AI tool')
    expect(
      within(steps[0]).getByRole('link', { name: /Set up MCP/ })
    ).toHaveAttribute('href', '/mcp-servers/vibexp-mcp')
    expect(steps[1]).toHaveTextContent('2. Add your first resource')
    expect(
      within(steps[1]).getByRole('link', { name: '+ Prompt' })
    ).toHaveAttribute('href', '/prompts/new')
  })

  it('leaves out the blocks that have nothing to show yet', async () => {
    setup(emptyStats)
    renderHome()

    await screen.findByRole('region', { name: 'Get started' })
    expect(screen.queryByText('Needs your attention')).not.toBeInTheDocument()
    expect(screen.queryByText('What changed')).not.toBeInTheDocument()
    expect(screen.queryByTestId('access-chart')).not.toBeInTheDocument()
    expect(screen.getByText('Workspace totals')).toBeInTheDocument()
  })
})
