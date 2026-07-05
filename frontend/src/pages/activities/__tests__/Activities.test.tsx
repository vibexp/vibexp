/**
 * Tests for Activities page — relative time display with absolute-time tooltip,
 * and general rendering behaviour.
 */
import { render, screen, waitFor } from '@testing-library/react'
import React from 'react'

import type {
  ActivitiesResponse,
  Activity as ActivityType,
} from '@/services/activityService'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// Radix UI Tooltip primitives don't work in JSDOM — stub them out so we can
// assert on the rendered text without worrying about portal / pointer events.
jest.mock('@/components/ui/tooltip', () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="tooltip-content">{children}</div>
  ),
  TooltipProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}))

jest.mock('@/services/activityService', () => ({
  activityService: {
    getActivities: jest.fn(),
  },
}))

// Stable showError so useCallback's dependency array doesn't change each render
const mockShowError = jest.fn()
jest.mock('@/hooks', () => ({
  useAlerts: () => ({ showError: mockShowError }),
}))

// ---------------------------------------------------------------------------
// Static imports after mocks
// ---------------------------------------------------------------------------
import { activityService } from '@/services/activityService'

import { Activities } from '../Activities'

const mockActivityService = activityService as jest.Mocked<
  typeof activityService
>

// ---------------------------------------------------------------------------
// Fixture helpers
// ---------------------------------------------------------------------------

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
    created_at: new Date('2024-06-01T11:55:00Z').toISOString(),
    ...overrides,
  }
}

function emptyResponse(): Promise<ActivitiesResponse> {
  return Promise.resolve({
    status: 'success',
    message: 'ok',
    data: {
      activities: [],
      total_count: 0,
      page: 1,
      per_page: 25,
      total_pages: 0,
    },
  })
}

function activityResponse(
  activities: ActivityType[]
): Promise<ActivitiesResponse> {
  return Promise.resolve({
    status: 'success',
    message: 'ok',
    data: {
      activities,
      total_count: activities.length,
      page: 1,
      per_page: 25,
      total_pages: 1,
    },
  })
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('Activities page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('renders the page header', async () => {
    mockActivityService.getActivities.mockReturnValue(emptyResponse())
    render(<Activities />)

    await waitFor(() => {
      expect(screen.getByText('Activities')).toBeInTheDocument()
    })
  })

  it('shows empty state when there are no activities', async () => {
    mockActivityService.getActivities.mockReturnValue(emptyResponse())
    render(<Activities />)

    await waitFor(() => {
      expect(screen.getByText('No activities yet')).toBeInTheDocument()
    })
  })

  it('renders activity description when activities are returned', async () => {
    mockActivityService.getActivities.mockReturnValue(
      activityResponse([makeActivity()])
    )
    render(<Activities />)

    await waitFor(() => {
      expect(screen.getByText('Created a prompt')).toBeInTheDocument()
    })
  })

  it('renders relative time as the visible trigger text', async () => {
    // Use a time close to now so formatRelativeTime returns a "Xm ago" label
    const recentTime = new Date(Date.now() - 5 * 60 * 1000).toISOString()
    mockActivityService.getActivities.mockReturnValue(
      activityResponse([makeActivity({ created_at: recentTime })])
    )
    render(<Activities />)

    await waitFor(() => {
      expect(screen.getByText('5m ago')).toBeInTheDocument()
    })
  })

  it('renders absolute time inside tooltip content', async () => {
    // Use a well-known date for deterministic assertion
    const knownDate = new Date('2024-01-15T10:30:00Z').toISOString()
    mockActivityService.getActivities.mockReturnValue(
      activityResponse([makeActivity({ created_at: knownDate })])
    )
    render(<Activities />)

    await waitFor(() => {
      const tooltipContents = screen.getAllByTestId('tooltip-content')
      const hasAbsoluteTime = tooltipContents.some(el => {
        const text = el.textContent ?? ''
        return text.includes('January') && text.includes('2024')
      })
      expect(hasAbsoluteTime).toBe(true)
    })
  })

  it('shows "just now" for a very recent activity', async () => {
    const tenSecondsAgo = new Date(Date.now() - 10 * 1000).toISOString()
    mockActivityService.getActivities.mockReturnValue(
      activityResponse([makeActivity({ created_at: tenSecondsAgo })])
    )
    render(<Activities />)

    await waitFor(() => {
      expect(screen.getByText('just now')).toBeInTheDocument()
    })
  })

  it('shows hours-ago label for an older activity', async () => {
    const twoHoursAgo = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString()
    mockActivityService.getActivities.mockReturnValue(
      activityResponse([makeActivity({ created_at: twoHoursAgo })])
    )
    render(<Activities />)

    await waitFor(() => {
      expect(screen.getByText('2h ago')).toBeInTheDocument()
    })
  })

  it('displays activity_type and entity_type metadata', async () => {
    mockActivityService.getActivities.mockReturnValue(
      activityResponse([makeActivity({ entity_type: 'artifact' })])
    )
    render(<Activities />)

    await waitFor(() => {
      expect(screen.getByText(/artifact/)).toBeInTheDocument()
    })
  })
})
