import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import type { FeedItemListResponse, FeedListResponse } from '@/types/feed'

// Mock Radix UI components that cause infinite loops in JSDOM
jest.mock('@/components/ui/tabs', () => ({
  Tabs: ({
    children,
    value,
    onValueChange,
  }: {
    children: React.ReactNode
    value?: string
    onValueChange?: (v: string) => void
  }) => (
    <div data-testid="tabs" data-value={value}>
      {React.Children.map(children, child =>
        React.isValidElement(child)
          ? React.cloneElement(
              child as React.ReactElement<{
                onValueChange?: (v: string) => void
              }>,
              { onValueChange }
            )
          : child
      )}
    </div>
  ),
  TabsList: ({
    children,
    onValueChange,
  }: {
    children: React.ReactNode
    onValueChange?: (v: string) => void
  }) => (
    <div data-testid="tabs-list">
      {React.Children.map(children, child =>
        React.isValidElement(child)
          ? React.cloneElement(
              child as React.ReactElement<{
                onValueChange?: (v: string) => void
              }>,
              { onValueChange }
            )
          : child
      )}
    </div>
  ),
  TabsTrigger: ({
    children,
    value,
    onValueChange,
  }: {
    children: React.ReactNode
    value?: string
    onValueChange?: (v: string) => void
  }) => (
    <button
      data-testid={`tab-${value ?? ''}`}
      onClick={() => {
        if (value && onValueChange) onValueChange(value)
      }}
    >
      {children}
    </button>
  ),
  TabsContent: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="tabs-content">{children}</div>
  ),
}))

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

jest.mock('@/components/ui/dropdown-menu', () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DropdownMenuLabel: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DropdownMenuSeparator: () => <hr />,
  DropdownMenuItem: ({
    children,
    onClick,
  }: {
    children: React.ReactNode
    onClick?: () => void
  }) => <button onClick={onClick}>{children}</button>,
}))

// Mock services
jest.mock('@/services/feedService', () => ({
  feedService: {
    getFeedItems: jest.fn(),
    getFeeds: jest.fn(),
    archiveFeedItem: jest.fn(),
    unarchiveFeedItem: jest.fn(),
    deleteFeedItem: jest.fn(),
    deleteFeed: jest.fn(),
  },
}))

jest.mock('@/services/projectService', () => ({
  projectService: {
    getProjects: jest.fn(),
  },
}))

// Mock TeamContext — use stable object references to prevent useCallback dep instability
jest.mock('@/contexts/TeamContext', () => {
  const currentTeam = { id: 'team-1', name: 'Test Team' }
  const teams = [currentTeam]
  return {
    useTeam: () => ({ currentTeam, teams, isLoading: false }),
  }
})

// Mock ProjectContext — stable references, "All projects" selected
jest.mock('@/contexts/ProjectContext', () => {
  const setCurrentProject = jest.fn()
  return {
    useProject: () => ({
      currentProject: null,
      setCurrentProject,
      isLoading: false,
    }),
  }
})

// Mock hooks — use stable function references via module-level singletons
jest.mock('@/hooks', () => {
  const showSuccess = jest.fn()
  const showError = jest.fn()
  const trackEvent = jest.fn()
  return {
    useAlerts: () => ({ showSuccess, showError }),
    useAnalytics: () => ({ trackEvent }),
  }
})

jest.mock('@/hooks/useErrorHandler', () => {
  const handleError = jest.fn()
  return {
    useErrorHandler: () => ({ handleError }),
  }
})

import userEvent from '@testing-library/user-event'
import React from 'react'

import { feedService } from '@/services/feedService'
import { projectService } from '@/services/projectService'

import { Feeds } from '../Feeds'

const mockFeedItemsResponse: FeedItemListResponse = {
  items: [
    {
      id: 'item-1',
      team_id: 'team-1',
      feed_id: 'feed-1',
      title: 'Test Sprint Retrospective',
      content: '## Summary\nAll done.',
      excerpt: 'All done.',
      ai_assistant_name: 'claude-sonnet-4-5',
      posted_by_user_id: 'user-1',
      posted_at: new Date().toISOString(),
    },
  ],
  total_count: 1,
  page: 1,
  per_page: 20,
  total_pages: 1,
}

const mockFeedsResponse: FeedListResponse = {
  feeds: [
    {
      id: 'feed-1',
      team_id: 'team-1',
      name: 'Product Updates',
      created_by_user_id: 'user-1',
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    },
  ],
  total_count: 1,
  page: 1,
  per_page: 100,
  total_pages: 1,
}

describe('Feeds page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    ;(feedService.getFeedItems as jest.Mock).mockResolvedValue(
      mockFeedItemsResponse
    )
    ;(feedService.getFeeds as jest.Mock).mockResolvedValue(mockFeedsResponse)
    ;(projectService.getProjects as jest.Mock).mockResolvedValue({
      projects: [],
      total_count: 0,
      page: 1,
      per_page: 100,
      total_pages: 0,
    })
  })

  function renderFeeds() {
    return render(
      <MemoryRouter>
        <Feeds />
      </MemoryRouter>
    )
  }

  it('renders the page title', () => {
    renderFeeds()
    expect(screen.getByText('AI Feeds')).toBeInTheDocument()
  })

  it('renders New feed button', () => {
    renderFeeds()
    expect(screen.getByText('New feed')).toBeInTheDocument()
  })

  it('renders Active and Archived tabs', () => {
    renderFeeds()
    expect(screen.getByText('Active')).toBeInTheDocument()
    expect(screen.getByText('Archived')).toBeInTheDocument()
  })

  it('renders feed items after loading', async () => {
    renderFeeds()
    await waitFor(() => {
      expect(screen.getByText('Test Sprint Retrospective')).toBeInTheDocument()
    })
  })

  it('calls getFeedItems on mount with archived: "false"', async () => {
    renderFeeds()
    await waitFor(() => {
      expect(feedService.getFeedItems).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ archived: 'false' })
      )
    })
  })

  it('calls getFeedItems with archived: "true" when switching to Archived tab', async () => {
    const user = userEvent.setup()
    renderFeeds()

    // Wait for initial load
    await waitFor(() => {
      expect(feedService.getFeedItems).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ archived: 'false' })
      )
    })

    // Clear calls so we can assert the next one cleanly
    ;(feedService.getFeedItems as jest.Mock).mockClear()
    ;(feedService.getFeedItems as jest.Mock).mockResolvedValue({
      items: [],
      total_count: 0,
      page: 1,
      per_page: 20,
      total_pages: 0,
    })

    // Click the Archived tab
    const archivedTab = screen.getByRole('button', { name: /archived/i })
    await user.click(archivedTab)

    await waitFor(() => {
      expect(feedService.getFeedItems).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ archived: 'true' })
      )
    })
  })

  it('renders loading skeletons initially', () => {
    // Keep getFeedItems pending to see loading state
    ;(feedService.getFeedItems as jest.Mock).mockImplementation(
      () => new Promise(() => undefined)
    )
    renderFeeds()
    // Page title should still be visible during loading
    expect(screen.getByText('AI Feeds')).toBeInTheDocument()
  })

  it('renders empty state when no items', async () => {
    ;(feedService.getFeedItems as jest.Mock).mockResolvedValue({
      items: [],
      total_count: 0,
      page: 1,
      per_page: 20,
      total_pages: 0,
    })
    renderFeeds()
    await waitFor(() => {
      expect(screen.getByText('No feed items yet')).toBeInTheDocument()
    })
  })

  it('renders error state when fetch fails', async () => {
    ;(feedService.getFeedItems as jest.Mock).mockRejectedValue(
      new Error('Network error')
    )
    renderFeeds()
    await waitFor(() => {
      expect(screen.getByText('Failed to load feed items')).toBeInTheDocument()
    })
  })
})
