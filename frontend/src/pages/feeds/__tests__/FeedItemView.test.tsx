import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Feed, FeedItem } from '@/services/feedService'
import type { TeamMember } from '@/services/teamService'

// The shared lucide mock lists icons explicitly and misses some used here
// (ArchiveRestore, …) — serve any icon via a Proxy instead (PromptEditor pattern).
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

// Render markdown as plain text in tests
jest.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => <div>{content}</div>,
}))

// FeedItemReplies has its own suite (FeedItemReplies.test.tsx) — probe it here
// to assert the page wires the thread to the loaded item.
jest.mock('@/pages/feeds/FeedItemReplies', () => ({
  FeedItemReplies: ({ teamId, itemId }: { teamId: string; itemId: string }) => (
    <div
      data-testid="feed-item-replies"
      data-team-id={teamId}
      data-item-id={itemId}
    />
  ),
}))

jest.mock('@/services/feedService', () => ({
  feedService: {
    getFeedItem: jest.fn(),
    getFeed: jest.fn(),
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

// Mutable so each test chooses the server-granted permissions array — the page
// gates delete via the real usePermissions hook (never mocked, #225).
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

const mockTrackEvent = jest.fn()
const mockShowSuccess = jest.fn()
jest.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess: mockShowSuccess, showError: jest.fn() }),
  useAnalytics: () => ({ trackEvent: mockTrackEvent }),
}))

const mockHandleError = jest.fn()
jest.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

import { feedService } from '@/services/feedService'
import { projectService } from '@/services/projectService'
import { teamService } from '@/services/teamService'

import { FeedItemView } from '../FeedItemView'

const mockFeed: Feed = {
  id: 'feed-1',
  team_id: 'team-1',
  name: 'Product Updates',
  created_by_user_id: 'user-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
}

function buildItem(overrides: Partial<FeedItem> = {}): FeedItem {
  return {
    id: 'item-1',
    team_id: 'team-1',
    feed_id: 'feed-1',
    title: 'Sprint Retro Summary',
    content: '## Summary\nAll done.',
    excerpt: 'All done.',
    // The composer's sentinel for human-authored posts (feedActor.ts) — the
    // FeedItem schema requires a string here, so `null` is not an option.
    ai_assistant_name: 'User Post',
    posted_by_user_id: 'user-2',
    posted_at: new Date().toISOString(),
    reply_count: 0,
    project_id: 'proj-1',
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

function renderFeedItemView() {
  return render(
    <MemoryRouter initialEntries={['/feed-items/item-1']}>
      <Routes>
        <Route path="/feed-items/:itemId" element={<FeedItemView />} />
      </Routes>
    </MemoryRouter>
  )
}

describe('FeedItemView page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    setTeamPermissions([])
    ;(feedService.getFeedItem as jest.Mock).mockResolvedValue(buildItem())
    ;(feedService.getFeed as jest.Mock).mockResolvedValue(mockFeed)
    ;(projectService.getProjects as jest.Mock).mockResolvedValue({
      projects: [
        {
          id: 'proj-1',
          user_id: 'user-1',
          team_id: 'team-1',
          name: 'Apollo Project',
          slug: 'apollo-project',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
          version: 1,
        },
      ],
      total_count: 1,
      page: 1,
      per_page: 100,
      total_pages: 1,
    })
    ;(teamService.getTeamMembers as jest.Mock).mockResolvedValue([member])
  })

  it('shows a loading state while the item is loading', () => {
    ;(feedService.getFeedItem as jest.Mock).mockImplementation(
      () => new Promise(() => undefined)
    )
    renderFeedItemView()
    expect(screen.getByText('Loading feed item…')).toBeInTheDocument()
  })

  it('renders the item with author, feed, project, and content', async () => {
    renderFeedItemView()
    expect(
      (await screen.findAllByText('Sprint Retro Summary')).length
    ).toBeGreaterThan(0)
    expect(feedService.getFeedItem).toHaveBeenCalledWith('team-1', 'item-1')

    // Human post: author resolved from the team member list
    expect(screen.getByText('Bob Jones')).toBeInTheDocument()
    expect(screen.queryByText('AI')).not.toBeInTheDocument()
    // Feed + project metadata
    expect(
      await screen.findByRole('button', { name: 'Product Updates' })
    ).toBeInTheDocument()
    expect(await screen.findByText('Apollo Project')).toBeInTheDocument()
    // Markdown content (mocked renderer prints it verbatim)
    expect(screen.getByText(/## Summary/)).toBeInTheDocument()
    expect(mockTrackEvent).toHaveBeenCalledWith(
      expect.objectContaining({
        properties: expect.objectContaining({ feed_item_id: 'item-1' }),
      })
    )
  })

  it('passes the loaded item to the replies thread', async () => {
    renderFeedItemView()
    const replies = await screen.findByTestId('feed-item-replies')
    expect(replies).toHaveAttribute('data-team-id', 'team-1')
    expect(replies).toHaveAttribute('data-item-id', 'item-1')
  })

  it('shows the AI badge for assistant-posted items', async () => {
    ;(feedService.getFeedItem as jest.Mock).mockResolvedValue(
      buildItem({ ai_assistant_name: 'claude-sonnet-4-5' })
    )
    renderFeedItemView()
    await screen.findAllByText('Sprint Retro Summary')
    expect(screen.getByText('claude-sonnet-4-5')).toBeInTheDocument()
    expect(screen.getByText('AI')).toBeInTheDocument()
  })

  it('renders the not-found state when the item fails to load', async () => {
    const user = userEvent.setup()
    ;(feedService.getFeedItem as jest.Mock).mockRejectedValue(
      new Error('Network error')
    )
    renderFeedItemView()
    expect(
      (await screen.findAllByText('Feed item not found')).length
    ).toBeGreaterThan(0)
    expect(screen.getByText('Network error')).toBeInTheDocument()
    expect(mockHandleError).toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: /back to feeds/i }))
    expect(mockNavigate).toHaveBeenCalledWith('/feeds')
  })

  it('shows a no-team error when no team is selected', async () => {
    mockTeamState.currentTeam = null
    renderFeedItemView()
    expect(
      await screen.findByText(
        'No team available. Please select or create a team first.'
      )
    ).toBeInTheDocument()
    expect(feedService.getFeedItem).not.toHaveBeenCalled()
  })

  it('still renders the item when the feed lookup fails (non-fatal)', async () => {
    ;(feedService.getFeed as jest.Mock).mockRejectedValue(
      new Error('Feed gone')
    )
    renderFeedItemView()
    expect(
      (await screen.findAllByText('Sprint Retro Summary')).length
    ).toBeGreaterThan(0)
    expect(
      screen.queryByRole('button', { name: 'Product Updates' })
    ).not.toBeInTheDocument()
  })

  it('archives the item and flips to the archived presentation', async () => {
    const user = userEvent.setup()
    ;(feedService.archiveFeedItem as jest.Mock).mockResolvedValue(undefined)
    renderFeedItemView()
    await screen.findAllByText('Sprint Retro Summary')
    expect(screen.queryByText('Archived')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Archive' }))
    await waitFor(() => {
      expect(feedService.archiveFeedItem).toHaveBeenCalledWith(
        'team-1',
        'item-1'
      )
    })
    expect(
      await screen.findByRole('button', { name: 'Unarchive' })
    ).toBeInTheDocument()
    expect(screen.getByText('Archived')).toBeInTheDocument()
    expect(mockShowSuccess).toHaveBeenCalledWith(
      'Feed item archived',
      'Success'
    )
  })

  it('unarchives an archived item', async () => {
    const user = userEvent.setup()
    ;(feedService.getFeedItem as jest.Mock).mockResolvedValue(
      buildItem({ archived_at: '2026-01-05T00:00:00Z' })
    )
    ;(feedService.unarchiveFeedItem as jest.Mock).mockResolvedValue(undefined)
    renderFeedItemView()
    await screen.findAllByText('Sprint Retro Summary')
    expect(screen.getByText('Archived')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Unarchive' }))
    await waitFor(() => {
      expect(feedService.unarchiveFeedItem).toHaveBeenCalledWith(
        'team-1',
        'item-1'
      )
    })
    expect(
      await screen.findByRole('button', { name: 'Archive' })
    ).toBeInTheDocument()
    expect(screen.queryByText('Archived')).not.toBeInTheDocument()
  })

  describe('delete gating (canDeleteFeedContent, #225)', () => {
    it("hides delete on someone else's item without feed.delete.any", async () => {
      setTeamPermissions(['resource.delete.own'])
      renderFeedItemView()
      await screen.findAllByText('Sprint Retro Summary')
      expect(
        screen.queryByRole('button', { name: 'Delete' })
      ).not.toBeInTheDocument()
    })

    it("shows delete on someone else's item with feed.delete.any", async () => {
      setTeamPermissions(['feed.delete.any'])
      renderFeedItemView()
      await screen.findAllByText('Sprint Retro Summary')
      expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument()
    })

    it('shows delete on your own item with resource.delete.own', async () => {
      setTeamPermissions(['resource.delete.own'])
      ;(feedService.getFeedItem as jest.Mock).mockResolvedValue(
        buildItem({ posted_by_user_id: 'user-1' })
      )
      renderFeedItemView()
      await screen.findAllByText('Sprint Retro Summary')
      expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument()
    })

    it('hides delete on your own item when only feed.delete.any is granted', async () => {
      // The "own" half is resource.delete.own — feed.delete.any is moderation
      // over other people's posts only (mirrors the backend pair, #225).
      setTeamPermissions(['feed.delete.any'])
      ;(feedService.getFeedItem as jest.Mock).mockResolvedValue(
        buildItem({ posted_by_user_id: 'user-1' })
      )
      renderFeedItemView()
      await screen.findAllByText('Sprint Retro Summary')
      expect(
        screen.queryByRole('button', { name: 'Delete' })
      ).not.toBeInTheDocument()
    })
  })

  it('deletes the item through the confirm dialog and navigates back', async () => {
    const user = userEvent.setup()
    setTeamPermissions(['feed.delete.any'])
    ;(feedService.deleteFeedItem as jest.Mock).mockResolvedValue(undefined)
    renderFeedItemView()
    await screen.findAllByText('Sprint Retro Summary')

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
    expect(mockNavigate).toHaveBeenCalledWith('/feeds')
  })

  it('navigates back to the feed via Back and the feed name link', async () => {
    const user = userEvent.setup()
    renderFeedItemView()
    await screen.findAllByText('Sprint Retro Summary')

    await user.click(screen.getByRole('button', { name: 'Back' }))
    expect(mockNavigate).toHaveBeenCalledWith('/feeds/feed-1')

    await user.click(
      await screen.findByRole('button', { name: 'Product Updates' })
    )
    expect(mockNavigate).toHaveBeenLastCalledWith('/feeds/feed-1')
  })
})
