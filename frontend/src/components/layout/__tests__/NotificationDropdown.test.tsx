import { render, screen } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import React from 'react'
import { MemoryRouter } from 'react-router-dom'

import type { Notification } from '@/services/notificationService'

// ---------------------------------------------------------------------------
// Mock hooks and services
// ---------------------------------------------------------------------------
const mockMarkAllAsRead = vi.hoisted(() => vi.fn())
const mockMarkAsRead = vi.hoisted(() => vi.fn())

let mockNotifications: Notification[] = []
let mockLoading = false

vi.mock('../../../hooks/useNotifications', () => ({
  useNotifications: () => ({
    notifications: mockNotifications,
    loading: mockLoading,
    error: null,
    hasMore: false,
    fetchMore: vi.fn(),
    markAsRead: mockMarkAsRead,
    markAllAsRead: mockMarkAllAsRead,
    refresh: vi.fn(),
  }),
}))

// Mock navigate
const mockNavigate = vi.hoisted(() => vi.fn())
vi.mock('react-router-dom', async () => ({
  ...(await vi.importActual('react-router-dom')),
  useNavigate: () => mockNavigate,
}))

// Mock UI primitives
vi.mock('../../../components/ui/scroll-area', () => ({
  ScrollArea: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="scroll-area">{children}</div>
  ),
  ScrollBar: () => null,
}))

vi.mock('../../../components/ui/separator', () => ({
  Separator: () => <hr />,
}))

vi.mock('../../../components/ui/button', () => ({
  Button: ({
    children,
    onClick,
    disabled,
    className,
  }: {
    children: React.ReactNode
    onClick?: () => void
    disabled?: boolean
    className?: string
  }) => (
    <button onClick={onClick} disabled={disabled} className={className}>
      {children}
    </button>
  ),
}))

import { NotificationDropdown } from '../NotificationDropdown'

const makeNotification = (id: string, read = false): Notification => ({
  id,
  type: 'feed.item.created',
  category: 'low',
  title: `Notification ${id}`,
  body: 'Body text',
  action_url: '/feeds/1',
  created_at: '2024-01-01T10:00:00Z',
  ...(read ? { read_at: '2024-01-01T11:00:00Z' } : {}),
})

function renderDropdown(onClose = vi.fn(), onUnreadChange = vi.fn()) {
  return render(
    <MemoryRouter>
      <NotificationDropdown onClose={onClose} onUnreadChange={onUnreadChange} />
    </MemoryRouter>
  )
}

describe('NotificationDropdown', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockNotifications = []
    mockLoading = false
  })

  it('renders empty state when no notifications', () => {
    renderDropdown()
    expect(screen.getByText(/you're all caught up/i)).toBeInTheDocument()
  })

  it('renders notifications list', () => {
    mockNotifications = [makeNotification('1'), makeNotification('2')]
    renderDropdown()
    expect(screen.getByText('Notification 1')).toBeInTheDocument()
    expect(screen.getByText('Notification 2')).toBeInTheDocument()
  })

  it('shows loading text when loading with empty list', () => {
    mockLoading = true
    mockNotifications = []
    renderDropdown()
    expect(screen.getByText(/loading/i)).toBeInTheDocument()
  })

  it('shows mark-all-read button when unread notifications exist', () => {
    mockNotifications = [makeNotification('1', false)]
    renderDropdown()
    expect(screen.getByText(/mark all read/i)).toBeInTheDocument()
  })

  it('hides mark-all-read button when no unread notifications', () => {
    mockNotifications = [makeNotification('1', true)]
    renderDropdown()
    expect(screen.queryByText(/mark all read/i)).not.toBeInTheDocument()
  })

  it('calls markAllAsRead and onUnreadChange(0) when mark-all clicked', async () => {
    const user = userEvent.setup()
    const onUnreadChange = vi.fn()
    mockNotifications = [makeNotification('1', false)]
    renderDropdown(vi.fn(), onUnreadChange)

    await user.click(screen.getByText(/mark all read/i))

    expect(mockMarkAllAsRead).toHaveBeenCalled()
    expect(onUnreadChange).toHaveBeenCalledWith(0)
  })

  it('does NOT call onUnreadChange when a per-item read happens', async () => {
    const user = userEvent.setup()
    const onUnreadChange = vi.fn()
    mockNotifications = [makeNotification('1', false)]
    renderDropdown(vi.fn(), onUnreadChange)

    // Find the notification link and click it to trigger onRead
    const link = screen.getByRole('link', { name: /notification 1/i })
    await user.click(link)

    expect(mockMarkAsRead).toHaveBeenCalledWith('1')
    // onUnreadChange must NOT be called for per-item reads
    expect(onUnreadChange).not.toHaveBeenCalled()
  })

  it('navigates to /notifications when see-all is clicked', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    renderDropdown(onClose)

    await user.click(screen.getByText(/see all notifications/i))

    expect(onClose).toHaveBeenCalled()
    expect(mockNavigate).toHaveBeenCalledWith('/notifications')
  })
})
