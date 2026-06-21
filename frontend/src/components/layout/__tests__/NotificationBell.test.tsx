import { render, screen } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import React from 'react'
import { MemoryRouter } from 'react-router-dom'

// ---------------------------------------------------------------------------
// Mock hooks
// ---------------------------------------------------------------------------
const mockResetUnread = jest.fn()
let mockUnreadCount = 0

jest.mock('../../../hooks/useUnreadCount', () => ({
  useUnreadCount: () => ({
    unreadCount: mockUnreadCount,
    incrementUnread: jest.fn(),
    resetUnread: mockResetUnread,
    refresh: jest.fn(),
  }),
}))

const mockMarkAllAsRead = jest.fn()
const mockMarkAsRead = jest.fn()

jest.mock('../../../hooks/useNotifications', () => ({
  useNotifications: () => ({
    notifications: [],
    loading: false,
    error: null,
    hasMore: false,
    fetchMore: jest.fn(),
    markAsRead: mockMarkAsRead,
    markAllAsRead: mockMarkAllAsRead,
    refresh: jest.fn(),
  }),
}))

// ---------------------------------------------------------------------------
// Mock UI primitives (Radix Popover does not render portal in JSDOM)
// ---------------------------------------------------------------------------
jest.mock('../../../components/ui/popover', () => {
  interface PopoverCtx {
    open: boolean
    setOpen: (v: boolean) => void
  }
  const PopoverContext = React.createContext<PopoverCtx>({
    open: false,
    setOpen: () => {},
  })

  const Popover = ({
    children,
    open,
    onOpenChange,
  }: {
    children: React.ReactNode
    open?: boolean
    onOpenChange?: (open: boolean) => void
  }) => {
    const [internalOpen, setInternalOpen] = React.useState(open ?? false)
    const currentOpen = open ?? internalOpen
    const handleChange = (val: boolean) => {
      setInternalOpen(val)
      onOpenChange?.(val)
    }
    return (
      <PopoverContext.Provider
        value={{ open: currentOpen, setOpen: handleChange }}
      >
        {children}
      </PopoverContext.Provider>
    )
  }

  const PopoverTrigger = ({
    children,
    asChild,
  }: {
    children: React.ReactElement
    asChild?: boolean
  }) => {
    const ctx = React.useContext(PopoverContext)
    if (asChild) {
      return React.cloneElement(
        children as React.ReactElement<{ onClick?: () => void }>,
        {
          onClick: () => {
            ctx.setOpen(!ctx.open)
          },
        }
      )
    }
    return (
      <button
        onClick={() => {
          ctx.setOpen(!ctx.open)
        }}
      >
        {children}
      </button>
    )
  }

  const PopoverContent = ({
    children,
  }: {
    children: React.ReactNode
    align?: string
    sideOffset?: number
    className?: string
    onOpenAutoFocus?: (e: Event) => void
  }) => {
    const ctx = React.useContext(PopoverContext)
    return ctx.open ? <div data-testid="popover-content">{children}</div> : null
  }

  return { Popover, PopoverTrigger, PopoverContent }
})

// Mock NotificationDropdown to avoid deeply nested dependencies
jest.mock('../NotificationDropdown', () => ({
  NotificationDropdown: ({
    onClose,
  }: {
    onClose: () => void
    onUnreadChange: (count: number) => void
  }) => (
    <div data-testid="notification-dropdown">
      <button onClick={onClose}>close</button>
    </div>
  ),
}))

import { NotificationBell } from '../NotificationBell'

function renderWithRouter(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>)
}

describe('NotificationBell', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    mockUnreadCount = 0
  })

  it('renders the bell icon button', () => {
    renderWithRouter(<NotificationBell />)
    expect(
      screen.getByRole('button', { name: /notifications/i })
    ).toBeInTheDocument()
  })

  it('shows no badge when unread count is zero', () => {
    mockUnreadCount = 0
    renderWithRouter(<NotificationBell />)
    expect(screen.queryByText(/\d+/)).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Notifications' })
    ).toBeInTheDocument()
  })

  it('shows badge with count when unread notifications exist', () => {
    mockUnreadCount = 3
    renderWithRouter(<NotificationBell />)
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /3 unread/i })
    ).toBeInTheDocument()
  })

  it('caps badge display at 99+', () => {
    mockUnreadCount = 150
    renderWithRouter(<NotificationBell />)
    expect(screen.getByText('99+')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /99\+ unread/i })
    ).toBeInTheDocument()
  })

  it('shows exactly 99 for count of 99', () => {
    mockUnreadCount = 99
    renderWithRouter(<NotificationBell />)
    expect(screen.getByText('99')).toBeInTheDocument()
  })

  it('opens the dropdown on click', async () => {
    const user = userEvent.setup()
    mockUnreadCount = 1
    renderWithRouter(<NotificationBell />)

    await user.click(screen.getByRole('button', { name: /notifications/i }))

    expect(screen.getByTestId('notification-dropdown')).toBeInTheDocument()
  })

  it('closes the dropdown when onClose is called', async () => {
    const user = userEvent.setup()
    mockUnreadCount = 1
    renderWithRouter(<NotificationBell />)

    await user.click(screen.getByRole('button', { name: /notifications/i }))
    expect(screen.getByTestId('notification-dropdown')).toBeInTheDocument()

    await user.click(screen.getByText('close'))
    expect(
      screen.queryByTestId('notification-dropdown')
    ).not.toBeInTheDocument()
  })
})
