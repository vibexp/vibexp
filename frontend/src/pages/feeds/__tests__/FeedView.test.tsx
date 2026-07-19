import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type {
  Feed,
  FeedItem,
  FeedItemFilters,
  FeedItemListResponse,
} from '@/services/feedService'
import type { TeamMember } from '@/services/teamService'

// The shared lucide mock lists icons explicitly and misses some used here
// (RefreshCw, Rss, …) — serve any icon via a Proxy instead (PromptEditor pattern).
jest.mock(
  'lucide-react',
  () =>
    new Proxy(
      {},
      {
        get: (_target, name) => {
          if (name === '__esModule') return true
          const Icon = (props: object) => (
            <svg
              data-testid={`${String(name).toLowerCase()}-icon`}
              {...props}
            />
          )
          return Icon
        },
      }
    )
)

const mockNavigate = jest.fn()
jest.mock('react-router-dom', () => ({
  ...jest.requireActual<typeof import('react-router-dom')>('react-router-dom'),
  useNavigate: () => mockNavigate,
}))

// Mock Radix Select (FeedToolbar) — it can loop in JSDOM (Artifacts.test.tsx approach)
jest.mock('@/components/ui/select', () => ({
  Select: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select">{children}</div>
  ),
  SelectTrigger: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select-trigger">{children}</div>
  ),
  SelectValue: ({ placeholder }: { placeholder?: string }) => (
    <span>{placeholder}</span>
  ),
  SelectContent: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select-content">{children}</div>
  ),
  SelectItem: ({
    children,
    value,
  }: {
    children: React.ReactNode
    value: string
  }) => <div data-value={value}>{children}</div>,
}))

// FeedPostComposer has its own suite (FeedPostComposer.test.tsx) — probe it here
// so the page test only asserts visibility/wiring, not composer internals.
jest.mock('@/pages/feeds/FeedPostComposer', () => ({
  FeedPostComposer: ({
    feedId,
    onPosted,
  }: {
    feedId: string
    onPosted: () => void
  }) => (
    <div data-testid="feed-post-composer" data-feed-id={feedId}>
      <button type="button" onClick={onPosted}>
        mock-post
      </button>
    </div>
  ),
}))

jest.mock('@/services/feedService', () => ({
  feedService: {
    getFeed: jest.fn(),
    getFeedItemsForFeed: jest.fn(),
    archiveFeedItem: jest.fn(),
    unarchiveFeedItem: jest.fn(),
    deleteFeedItem: jest.fn(),
  },
}))

jest.mock('@/services/projectService', () => ({
  projectService: {
    getProjects: jest.fn(),
  },
}))

jest.mock('@/services/teamService', () => ({
  teamService: {
    getTeamMembers: jest.fn(),
  },
}))

// usePermissions (#225) reads the signed-in user for own-vs-any delete gating.
jest.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

// Mutable so each test chooses the server-granted permissions array — delete
// gating goes through the real usePermissions hook inside FeedItemList (#225).
const mockTeamState: {
  currentTeam: { id: string; name: string; permissions: string[] } | null
} = {
  currentTeam: { id: 'team-1', name: 'Test Team', permissions: [] },
}
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({
    currentTeam: mockTeamState.currentTeam,
    teams: mockTeamState.currentTeam ? [mockTeamState.currentTeam] : [],
    isLoading: false,
  }),
}))

jest.mock('@/contexts/ProjectContext', () => ({
  useProject: () => ({
    currentProject: null,
    setCurrentProject: jest.fn(),
    isLoading: false,
  }),
}))

jest.mock('@/hooks', () => {
  const showSuccess = jest.fn()
  const showError = jest.fn()
  const trackEvent = jest.fn()
  return {
    useAlerts: () => ({ showSuccess, showError }),
    useAnalytics: () => ({ trackEvent }),
  }
})

const mockHandleError = jest.fn()
jest.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

import React from 'react'

import { feedService } from '@/services/feedService'
import { projectService } from '@/services/projectService'
import { teamService } from '@/services/teamService'

import { FeedView } from '../FeedView'

const mockFeed: Feed = {
  id: 'feed-1',
  team_id: 'team-1',
  name: 'Product Updates',
  description: 'What shipped this sprint',
  created_by_user_id: 'user-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
}

function buildItem(overrides: Partial<FeedItem> = {}): FeedItem {
  return {
    id: 'item-1',
    team_id: 'team-1',
    feed_id: 'feed-1',
    title: 'Sprint Retrospective',
    content: '## Summary\nAll done.',
    excerpt: 'All done.',
    ai_assistant_name: 'claude-sonnet-4-5',
    posted_by_user_id: 'user-2',
    posted_at: new Date().toISOString(),
    reply_count: 0,
    ...overrides,
  }
}

function buildItemsResponse(
  items: FeedItem[],
  overrides: Partial<FeedItemListResponse> = {}
): FeedItemListResponse {
  return {
    items,
    total_count: items.length,
    page: 1,
    per_page: 20,
    total_pages: items.length > 0 ? 1 : 0,
    ...overrides,
  }
}

const member: TeamMember = {
  user_id: 'user-2',
  email: 'bob@example.com',
  name: 'Bob Jones',
  role: 'member',
  joined_at: '2026-01-01T00:00:00Z',
}

function setTeamPermissions(permissions: string[]) {
  mockTeamState.currentTeam = { id: 'team-1', name: 'Test Team', permissions }
}

function renderFeedView() {
  return render(
    <MemoryRouter initialEntries={['/feeds/feed-1']}>
      <Routes>
        <Route path="/feeds/:feedId" element={<FeedView />} />
      </Routes>
    </MemoryRouter>
  )
}

/** Main-list fetches only — the tab-badge count fetch uses limit: 1. */
function mainFetchCalls(): FeedItemFilters[] {
  return (feedService.getFeedItemsForFeed as jest.Mock).mock.calls
    .map(call => call[2] as FeedItemFilters)
    .filter(f => f.limit !== 1)
}

describe('FeedView page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    setTeamPermissions([])
    ;(feedService.getFeed as jest.Mock).mockResolvedValue(mockFeed)
    ;(feedService.getFeedItemsForFeed as jest.Mock).mockResolvedValue(
      buildItemsResponse([buildItem()])
    )
    ;(projectService.getProjects as jest.Mock).mockResolvedValue({
      projects: [],
      total_count: 0,
      page: 1,
      per_page: 100,
      total_pages: 0,
    })
    ;(teamService.getTeamMembers as jest.Mock).mockResolvedValue([member])
  })

  it('renders the feed name and description after loading', async () => {
    renderFeedView()
    expect(await screen.findByText('Product Updates')).toBeInTheDocument()
    expect(screen.getByText('What shipped this sprint')).toBeInTheDocument()
    expect(feedService.getFeed).toHaveBeenCalledWith('team-1', 'feed-1')
  })

  it('shows a loading title while the feed is loading', async () => {
    ;(feedService.getFeed as jest.Mock).mockImplementation(
      () => new Promise(() => undefined)
    )
    renderFeedView()
    expect(screen.getByText('Loading feed…')).toBeInTheDocument()
    // Let the (unblocked) items fetch settle so no state update lands post-test
    await screen.findByText('Sprint Retrospective')
  })

  it('fetches active items on mount and renders them', async () => {
    renderFeedView()
    expect(await screen.findByText('Sprint Retrospective')).toBeInTheDocument()
    expect(feedService.getFeedItemsForFeed).toHaveBeenCalledWith(
      'team-1',
      'feed-1',
      expect.objectContaining({ archived: 'false', page: 1, limit: 20 })
    )
  })

  it('renders the empty state when the feed has no items', async () => {
    ;(feedService.getFeedItemsForFeed as jest.Mock).mockResolvedValue(
      buildItemsResponse([])
    )
    renderFeedView()
    expect(await screen.findByText('No feed items yet')).toBeInTheDocument()
  })

  it('renders the error state when the items fetch fails', async () => {
    ;(feedService.getFeedItemsForFeed as jest.Mock).mockRejectedValue(
      new Error('Network error')
    )
    renderFeedView()
    expect(
      await screen.findByText('Failed to load feed items')
    ).toBeInTheDocument()
    expect(screen.getByText('Network error')).toBeInTheDocument()
    expect(mockHandleError).toHaveBeenCalled()
  })

  it('shows the composer on the Active tab and hides it on Archived', async () => {
    const user = userEvent.setup()
    renderFeedView()
    expect(await screen.findByTestId('feed-post-composer')).toHaveAttribute(
      'data-feed-id',
      'feed-1'
    )
    ;(feedService.getFeedItemsForFeed as jest.Mock).mockResolvedValue(
      buildItemsResponse([])
    )
    await user.click(screen.getByRole('button', { name: /archived/i }))

    await waitFor(() => {
      expect(feedService.getFeedItemsForFeed).toHaveBeenCalledWith(
        'team-1',
        'feed-1',
        expect.objectContaining({ archived: 'true', page: 1 })
      )
    })
    expect(screen.queryByTestId('feed-post-composer')).not.toBeInTheDocument()
    expect(
      await screen.findByText('No archived feed items')
    ).toBeInTheDocument()
  })

  it('re-fetches items after the composer reports a post', async () => {
    const user = userEvent.setup()
    renderFeedView()
    await screen.findByText('Sprint Retrospective')

    const before = mainFetchCalls().length
    await user.click(screen.getByRole('button', { name: 'mock-post' }))
    await waitFor(() => {
      expect(mainFetchCalls().length).toBeGreaterThan(before)
    })
  })

  it('re-fetches items when Refresh is clicked', async () => {
    const user = userEvent.setup()
    renderFeedView()
    await screen.findByText('Sprint Retrospective')

    const before = mainFetchCalls().length
    await user.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => {
      expect(mainFetchCalls().length).toBeGreaterThan(before)
    })
  })

  it('navigates via the All feeds and Edit feed header actions', async () => {
    const user = userEvent.setup()
    renderFeedView()
    await screen.findByText('Sprint Retrospective')

    await user.click(screen.getByRole('button', { name: /all feeds/i }))
    expect(mockNavigate).toHaveBeenCalledWith('/feeds')

    await user.click(screen.getByRole('button', { name: 'Edit feed' }))
    expect(mockNavigate).toHaveBeenCalledWith('/feeds/feed-1/edit')
  })

  it('archives an item from the card and re-fetches the list', async () => {
    const user = userEvent.setup()
    ;(feedService.archiveFeedItem as jest.Mock).mockResolvedValue(undefined)
    renderFeedView()
    await screen.findByText('Sprint Retrospective')

    const before = mainFetchCalls().length
    await user.click(screen.getByRole('button', { name: 'Archive' }))

    await waitFor(() => {
      expect(feedService.archiveFeedItem).toHaveBeenCalledWith(
        'team-1',
        'item-1'
      )
    })
    await waitFor(() => {
      expect(mainFetchCalls().length).toBeGreaterThan(before)
    })
  })

  it('unarchives an item from the Archived tab', async () => {
    const user = userEvent.setup()
    const archivedItem = buildItem({
      id: 'item-9',
      title: 'Old Post',
      archived_at: '2026-01-05T00:00:00Z',
    })
    ;(feedService.getFeedItemsForFeed as jest.Mock).mockImplementation(
      (_teamId: string, _feedId: string, filters: FeedItemFilters) =>
        Promise.resolve(
          filters.archived === 'true'
            ? buildItemsResponse([archivedItem])
            : buildItemsResponse([buildItem()])
        )
    )
    ;(feedService.unarchiveFeedItem as jest.Mock).mockResolvedValue(undefined)
    renderFeedView()
    await screen.findByText('Sprint Retrospective')

    await userEvent
      .setup()
      .click(screen.getByRole('button', { name: /archived/i }))
    await screen.findByText('Old Post')

    await user.click(screen.getByRole('button', { name: 'Unarchive' }))
    await waitFor(() => {
      expect(feedService.unarchiveFeedItem).toHaveBeenCalledWith(
        'team-1',
        'item-9'
      )
    })
  })

  describe('delete gating (server permissions array, #225)', () => {
    it("hides delete on someone else's item without feed.delete.any", async () => {
      setTeamPermissions(['resource.delete.own'])
      renderFeedView()
      await screen.findByText('Sprint Retrospective')
      expect(
        screen.queryByRole('button', { name: 'Delete' })
      ).not.toBeInTheDocument()
    })

    it("shows delete on someone else's item with feed.delete.any", async () => {
      setTeamPermissions(['feed.delete.any'])
      renderFeedView()
      await screen.findByText('Sprint Retrospective')
      expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument()
    })

    it('shows delete on your own item with resource.delete.own', async () => {
      setTeamPermissions(['resource.delete.own'])
      ;(feedService.getFeedItemsForFeed as jest.Mock).mockResolvedValue(
        buildItemsResponse([buildItem({ posted_by_user_id: 'user-1' })])
      )
      renderFeedView()
      await screen.findByText('Sprint Retrospective')
      expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument()
    })
  })

  it('deletes an item through the confirm dialog and re-fetches', async () => {
    const user = userEvent.setup()
    setTeamPermissions(['feed.delete.any'])
    ;(feedService.deleteFeedItem as jest.Mock).mockResolvedValue(undefined)
    renderFeedView()
    await screen.findByText('Sprint Retrospective')

    await user.click(screen.getByRole('button', { name: 'Delete' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Delete feed item?')).toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))
    await waitFor(() => {
      expect(feedService.deleteFeedItem).toHaveBeenCalledWith(
        'team-1',
        'item-1'
      )
    })
    await waitFor(() => {
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    })
  })

  it('paginates with Next when there are multiple pages', async () => {
    const user = userEvent.setup()
    ;(feedService.getFeedItemsForFeed as jest.Mock).mockResolvedValue(
      buildItemsResponse([buildItem()], { total_count: 25, total_pages: 2 })
    )
    renderFeedView()
    await screen.findByText('Page 1 of 2')

    await user.click(screen.getByRole('button', { name: 'Next' }))
    await waitFor(() => {
      expect(feedService.getFeedItemsForFeed).toHaveBeenCalledWith(
        'team-1',
        'feed-1',
        expect.objectContaining({ page: 2 })
      )
    })
  })

  it('debounces the search input into a filtered fetch', async () => {
    const user = userEvent.setup()
    renderFeedView()
    await screen.findByText('Sprint Retrospective')

    await user.type(screen.getByPlaceholderText('Search feed items…'), 'retro')
    await waitFor(
      () => {
        expect(feedService.getFeedItemsForFeed).toHaveBeenCalledWith(
          'team-1',
          'feed-1',
          expect.objectContaining({ search: 'retro', page: 1 })
        )
      },
      { timeout: 2000 }
    )
  })
})
