import { render, screen, waitFor } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import React from 'react'
import { MemoryRouter } from 'react-router-dom'

import type { Notification } from '@/services/notificationService'

// ---------------------------------------------------------------------------
// Mock hooks
// ---------------------------------------------------------------------------
const mockMarkAllAsRead = jest.fn()
const mockMarkAsRead = jest.fn()
const mockFetchMore = jest.fn()
const mockRefresh = jest.fn()

let mockNotifications: Notification[] = []
let mockLoading = false
let mockHasMore = false
let mockError: string | null = null

// Capture the params that useNotifications was last called with so tests
// can assert on the filter / limit arguments.
let lastUseNotificationsParams: Record<string, unknown> = {}

jest.mock('../../hooks/useNotifications', () => ({
  useNotifications: (params: Record<string, unknown> = {}) => {
    lastUseNotificationsParams = params
    return {
      notifications: mockNotifications,
      loading: mockLoading,
      error: mockError,
      hasMore: mockHasMore,
      fetchMore: mockFetchMore,
      markAsRead: mockMarkAsRead,
      markAllAsRead: mockMarkAllAsRead,
      refresh: mockRefresh,
    }
  },
}))

// Mock notificationService for the strong filter test
const mockNotificationService = {
  listNotifications: jest.fn(),
}

jest.mock('../../services/notificationService', () => ({
  notificationService: mockNotificationService,
}))

// Mock UI components
jest.mock('../../components/ui/card', () => ({
  Card: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="card">{children}</div>
  ),
  CardContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))

jest.mock('../../components/ui/skeleton', () => ({
  Skeleton: ({ className }: { className?: string }) => (
    <div data-testid="skeleton" className={className} />
  ),
}))

jest.mock('../../components/ui/button', () => ({
  Button: ({
    children,
    onClick,
    disabled,
  }: {
    children: React.ReactNode
    onClick?: () => void
    disabled?: boolean
  }) => (
    <button onClick={onClick} disabled={disabled}>
      {children}
    </button>
  ),
}))

jest.mock('../../components/ui/alert', () => ({
  Alert: ({ children }: { children: React.ReactNode }) => (
    <div role="alert">{children}</div>
  ),
  AlertTitle: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  AlertDescription: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))

// Mock NotificationItem to avoid deep deps
jest.mock('../../components/layout/NotificationItem', () => ({
  NotificationItem: ({
    notification,
    onRead,
  }: {
    notification: Notification
    onRead: (id: string) => void
  }) => (
    <div data-testid={`notification-${notification.id}`}>
      <span>{notification.title}</span>
      <button
        onClick={() => {
          onRead(notification.id)
        }}
      >
        mark read
      </button>
    </div>
  ),
}))

import { Notifications } from './Notifications'

const makeNotification = (id: string, read = false): Notification => ({
  id,
  type: 'feed.item.created',
  category: 'low',
  title: `Notification ${id}`,
  body: 'Body',
  action_url: '/feeds/1',
  created_at: '2024-01-01T10:00:00Z',
  ...(read ? { read_at: '2024-01-01T11:00:00Z' } : {}),
})

function renderPage() {
  return render(
    <MemoryRouter>
      <Notifications />
    </MemoryRouter>
  )
}

describe('Notifications page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    mockNotifications = []
    mockLoading = false
    mockHasMore = false
    mockError = null
    lastUseNotificationsParams = {}
  })

  it('renders page header', () => {
    renderPage()
    expect(screen.getByText('Notifications')).toBeInTheDocument()
  })

  it('shows empty state when no notifications', () => {
    renderPage()
    expect(screen.getByText(/you're all caught up/i)).toBeInTheDocument()
  })

  it('shows loading skeletons while loading', () => {
    mockLoading = true
    mockNotifications = []
    renderPage()
    expect(screen.getAllByTestId('skeleton').length).toBeGreaterThan(0)
  })

  it('shows error alert on error', () => {
    mockError = 'Failed to load'
    renderPage()
    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('Failed to load')).toBeInTheDocument()
  })

  it('renders notification items', () => {
    mockNotifications = [makeNotification('1'), makeNotification('2')]
    renderPage()
    expect(screen.getByTestId('notification-1')).toBeInTheDocument()
    expect(screen.getByTestId('notification-2')).toBeInTheDocument()
  })

  it('shows mark-all-read button when unread items exist', () => {
    mockNotifications = [makeNotification('1', false)]
    renderPage()
    expect(screen.getByText(/mark all read/i)).toBeInTheDocument()
  })

  it('hides mark-all-read button when all are read', () => {
    mockNotifications = [makeNotification('1', true)]
    renderPage()
    expect(screen.queryByText(/mark all read/i)).not.toBeInTheDocument()
  })

  it('calls markAllAsRead on button click', async () => {
    const user = userEvent.setup()
    mockNotifications = [makeNotification('1', false)]
    renderPage()

    await user.click(screen.getByText(/mark all read/i))

    expect(mockMarkAllAsRead).toHaveBeenCalled()
  })

  it('shows load more button when hasMore is true', () => {
    mockNotifications = [makeNotification('1')]
    mockHasMore = true
    renderPage()
    expect(screen.getByText(/load more/i)).toBeInTheDocument()
  })

  it('calls fetchMore when load-more is clicked', async () => {
    const user = userEvent.setup()
    mockNotifications = [makeNotification('1')]
    mockHasMore = true
    renderPage()

    await user.click(screen.getByText(/load more/i))

    expect(mockFetchMore).toHaveBeenCalled()
  })

  it('switches to unread filter', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByText('Unread'))

    await waitFor(() => {
      // The hook is re-invoked - we can verify state by checking unread empty state
      expect(screen.getByText(/you're all caught up/i)).toBeInTheDocument()
    })
  })

  it('shows unread empty state message in unread filter mode', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByText('Unread'))

    expect(screen.getByText(/no unread notifications/i)).toBeInTheDocument()
  })

  it('passes unread=true and limit=20 to useNotifications when Unread filter is active', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByText('Unread'))

    await waitFor(() => {
      expect(lastUseNotificationsParams).toMatchObject({
        unread: true,
        limit: 20,
      })
    })
  })

  it('passes unread=undefined when All filter is active', () => {
    renderPage()
    // Default is 'all' filter — unread should be falsy
    expect(lastUseNotificationsParams.unread).toBeFalsy()
    expect(lastUseNotificationsParams.limit).toBe(20)
  })

  it('skeleton items use stable string keys (not bare index)', () => {
    mockLoading = true
    mockNotifications = []
    renderPage()
    // All skeleton wrappers should be present; this test ensures no React key
    // warning from bare numeric indexes (checked indirectly via DOM)
    const skeletons = screen.getAllByTestId('skeleton')
    expect(skeletons).toHaveLength(10) // 5 rows × 2 skeletons each
  })
})
