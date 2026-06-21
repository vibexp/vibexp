import { render, screen } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import type { Notification } from '@/services/notificationService'

import { NotificationItem, safeHref } from '../NotificationItem'

// ---------------------------------------------------------------------------
// safeHref unit tests
// ---------------------------------------------------------------------------
describe('safeHref', () => {
  it('returns null for javascript: URLs', () => {
    expect(safeHref('javascript:alert(1)')).toBeNull()
  })

  it('returns null for data: URLs', () => {
    expect(safeHref('data:text/html,<script>alert(1)</script>')).toBeNull()
  })

  it('returns null for vbscript: URLs', () => {
    expect(safeHref('vbscript:msgbox(1)')).toBeNull()
  })

  it('returns the URL for valid https:// links', () => {
    expect(safeHref('https://example.com/path')).toBe(
      'https://example.com/path'
    )
  })

  it('returns the URL for valid http:// links', () => {
    expect(safeHref('http://example.com')).toBe('http://example.com')
  })

  it('returns the URL for relative paths starting with /', () => {
    expect(safeHref('/path/to/resource')).toBe('/path/to/resource')
  })

  it('returns null for empty string', () => {
    expect(safeHref('')).toBeNull()
  })

  it('returns null for null', () => {
    expect(safeHref(null)).toBeNull()
  })

  it('returns null for undefined', () => {
    expect(safeHref(undefined)).toBeNull()
  })
})

// ---------------------------------------------------------------------------
// NotificationItem component tests
// ---------------------------------------------------------------------------

const makeNotification = (
  overrides: Partial<Notification> = {}
): Notification => ({
  id: 'n-1',
  type: 'feed.item.created',
  category: 'low',
  title: 'Test notification',
  body: 'Notification body',
  action_url: '/feeds/1',
  created_at: '2024-01-01T10:00:00Z',
  ...overrides,
})

function renderItem(notification = makeNotification(), onRead = jest.fn()) {
  return render(
    <MemoryRouter>
      <NotificationItem notification={notification} onRead={onRead} />
    </MemoryRouter>
  )
}

describe('NotificationItem', () => {
  it('renders as an anchor for a valid https URL', () => {
    renderItem(makeNotification({ action_url: 'https://example.com' }))
    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('href', 'https://example.com')
  })

  it('renders as an anchor for a valid relative path', () => {
    renderItem(makeNotification({ action_url: '/feeds/1' }))
    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('href', '/feeds/1')
  })

  it('renders as a button for a javascript: URL (no navigation)', () => {
    renderItem(makeNotification({ action_url: 'javascript:alert(1)' }))
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(screen.getByRole('button')).toBeInTheDocument()
  })

  it('renders as a button for a data: URL (no navigation)', () => {
    renderItem(makeNotification({ action_url: 'data:text/html,<h1>x</h1>' }))
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(screen.getByRole('button')).toBeInTheDocument()
  })

  it('renders as a button for an empty action_url', () => {
    renderItem(makeNotification({ action_url: '' }))
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(screen.getByRole('button')).toBeInTheDocument()
  })

  it('calls onRead when unread notification is clicked (anchor)', async () => {
    const user = userEvent.setup()
    const onRead = jest.fn()
    renderItem(makeNotification({ action_url: '/feeds/1' }), onRead)

    await user.click(screen.getByRole('link'))
    expect(onRead).toHaveBeenCalledWith('n-1')
  })

  it('does not call onRead when already read (anchor)', async () => {
    const user = userEvent.setup()
    const onRead = jest.fn()
    renderItem(
      makeNotification({
        action_url: '/feeds/1',
        read_at: '2024-01-01T11:00:00Z',
      }),
      onRead
    )

    await user.click(screen.getByRole('link'))
    expect(onRead).not.toHaveBeenCalled()
  })

  it('calls onRead when unread notification rendered as button is clicked', async () => {
    const user = userEvent.setup()
    const onRead = jest.fn()
    renderItem(makeNotification({ action_url: 'javascript:alert(1)' }), onRead)

    await user.click(screen.getByRole('button'))
    expect(onRead).toHaveBeenCalledWith('n-1')
  })

  it('sets target="_blank" for external https links', () => {
    renderItem(makeNotification({ action_url: 'https://external.example.com' }))
    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('does not set target="_blank" for relative paths', () => {
    renderItem(makeNotification({ action_url: '/local/path' }))
    const link = screen.getByRole('link')
    expect(link).not.toHaveAttribute('target')
  })
})
