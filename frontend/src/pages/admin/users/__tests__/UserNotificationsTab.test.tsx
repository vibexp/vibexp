import { render, screen } from '@testing-library/react'

import type { AdminUserNotificationPreferences } from '@/services/adminService'

const mockGetPrefs = vi.hoisted(() => vi.fn())
vi.mock('@/services/adminService', () => ({
  adminService: {
    getUserNotificationPreferences: (...args: unknown[]) =>
      mockGetPrefs(...args),
  },
}))

import { UserNotificationsTab } from '../detail/UserNotificationsTab'

function prefs(
  overrides: Partial<AdminUserNotificationPreferences> = {}
): AdminUserNotificationPreferences {
  return {
    email_notification: {
      platform_announcement: true,
      account_security: true,
      new_feature: false,
      marketing_promotional: false,
    },
    notifications: {
      channels: { in_app: true, email: false },
      types: {
        'feed.item.created': { in_app: true, email: 'digest' },
        'custom.type': { in_app: false, email: 'none' },
      },
    },
    updated_at: '2026-09-01T12:00:00Z',
    is_default: false,
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
})

it('renders every preference value for the requested user', async () => {
  mockGetPrefs.mockResolvedValue(prefs())
  render(<UserNotificationsTab userId="u1" />)

  expect(await screen.findByText('Platform announcements')).toBeInTheDocument()
  expect(mockGetPrefs).toHaveBeenCalledWith('u1')
  expect(screen.getByText('Marketing and promotions')).toBeInTheDocument()
  // Known types use the shared label; unknown ones fall back to the raw key.
  expect(screen.getByText('New feed items')).toBeInTheDocument()
  expect(screen.getByText('custom.type')).toBeInTheDocument()
  expect(screen.getByText('Digest')).toBeInTheDocument()
  expect(screen.getByText('None')).toBeInTheDocument()
  // 2 email categories + in-app channel + 1 type in-app are on.
  expect(screen.getAllByText('On')).toHaveLength(4)
  expect(screen.getAllByText('Off')).toHaveLength(4)
  expect(screen.getByText(/Last changed/)).toBeInTheDocument()
})

it('is visibly read-only, with no interactive controls', async () => {
  mockGetPrefs.mockResolvedValue(prefs())
  const { container } = render(<UserNotificationsTab userId="u1" />)

  expect(await screen.findByText('Read-only')).toBeInTheDocument()
  expect(
    screen.getByText(/Read-only view of this user's notification settings/)
  ).toBeInTheDocument()
  for (const role of ['switch', 'checkbox', 'radio', 'tab', 'button']) {
    expect(screen.queryAllByRole(role)).toHaveLength(0)
  }
  expect(container.querySelector('input, select, textarea')).toBeNull()
})

it('says when the user never changed the defaults', async () => {
  mockGetPrefs.mockResolvedValue(prefs({ is_default: true, updated_at: null }))
  render(<UserNotificationsTab userId="u1" />)

  expect(await screen.findByText(/these are the defaults/)).toBeInTheDocument()
})

it('shows the loading then error state', async () => {
  mockGetPrefs.mockRejectedValue(new Error('prefs down'))
  render(<UserNotificationsTab userId="u1" />)

  expect(screen.getByTestId('notifications-loading')).toBeInTheDocument()
  expect(
    await screen.findByText('Failed to load notification preferences')
  ).toBeInTheDocument()
  expect(screen.getByText('prefs down')).toBeInTheDocument()
})
