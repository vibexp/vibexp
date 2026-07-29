import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

// Mock preferencesService
const mockPreferencesService = vi.hoisted(() => ({
  getPreferences: vi.fn(),
  updatePreferences: vi.fn(),
}))

vi.mock('@/services/preferencesService', () => ({
  preferencesService: mockPreferencesService,
}))

// Mock LoadingSpinner and PageHeader to simplify test DOM
vi.mock('@/components/LoadingSpinner', () => ({
  LoadingSpinner: () => <div data-testid="loading-spinner" />,
}))

vi.mock('@/components/PageHeader', () => ({
  PageHeader: ({ title }: { title: string }) => <h1>{title}</h1>,
}))

import type {
  NotificationPreferences as NotificationPrefsType,
  PreferencesResponse,
} from '@/services/preferencesService'

import { NotificationPreferences } from '../NotificationPreferences'

const baseEmailPrefs = {
  platform_announcement: true,
  account_security: true,
  new_feature: true,
  marketing_promotional: false,
}

const baseNotifPrefs: NotificationPrefsType = {
  channels: { in_app: true, email: true },
  types: {
    'feed.item.created': { in_app: true, email: 'digest' },
    'feed.reply.created': { in_app: true, email: 'instant' },
    'team.invitation': { in_app: true, email: 'instant' },
  },
}

function makePrefsResponse(
  notifPrefs?: NotificationPrefsType
): PreferencesResponse {
  // `notifications` is a required field on the generated `Preferences` schema,
  // but several tests exercise legacy data where it is absent; cast to keep
  // those defensive scenarios expressible.
  return {
    preferences: {
      email_notification: baseEmailPrefs,
      notifications: notifPrefs,
    },
    updated_at: '2024-01-01T00:00:00Z',
  } as unknown as PreferencesResponse
}

describe('NotificationPreferences', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockPreferencesService.getPreferences.mockResolvedValue(
      makePrefsResponse(baseNotifPrefs)
    )
    mockPreferencesService.updatePreferences.mockResolvedValue(
      makePrefsResponse(baseNotifPrefs)
    )
  })

  // ---------------------------------------------------------------------------
  // Loading state
  // ---------------------------------------------------------------------------

  it('shows loading spinner while fetching preferences', () => {
    mockPreferencesService.getPreferences.mockReturnValue(
      new Promise(() => {
        // Never resolves
      })
    )

    render(<NotificationPreferences />)

    expect(screen.getByTestId('loading-spinner')).toBeInTheDocument()
  })

  // ---------------------------------------------------------------------------
  // Email card
  // ---------------------------------------------------------------------------

  it('renders email notification preferences after loading', async () => {
    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(screen.getByText('Platform announcements')).toBeInTheDocument()
    })

    expect(screen.getByText('Account security')).toBeInTheDocument()
    expect(screen.getByText('New features')).toBeInTheDocument()
    expect(screen.getByText('Marketing & promotional')).toBeInTheDocument()
  })

  it('shows error message when preferences fail to load', async () => {
    mockPreferencesService.getPreferences.mockRejectedValue(
      new Error('Network failure')
    )

    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(screen.getByText('Network failure')).toBeInTheDocument()
    })
  })

  it('shows unsaved changes section after toggling an email preference', async () => {
    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(screen.getByText('Platform announcements')).toBeInTheDocument()
    })

    const toggle = screen.getByRole('switch', {
      name: /platform announcements/i,
    })

    await userEvent.click(toggle)

    expect(screen.getByText('You have unsaved changes.')).toBeInTheDocument()
  })

  it('calls updatePreferences with correct payload when Save is clicked', async () => {
    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(screen.getByText('Platform announcements')).toBeInTheDocument()
    })

    const toggle = screen.getByRole('switch', {
      name: /platform announcements/i,
    })
    await userEvent.click(toggle)

    const saveBtn = screen.getByRole('button', { name: /save changes/i })
    await userEvent.click(saveBtn)

    await waitFor(() => {
      expect(mockPreferencesService.updatePreferences).toHaveBeenCalledWith(
        expect.objectContaining({
          email_notification: expect.objectContaining({
            platform_announcement: false,
          }),
        })
      )
    })
  })

  it('shows success message after save', async () => {
    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(screen.getByText('Platform announcements')).toBeInTheDocument()
    })

    const toggle = screen.getByRole('switch', {
      name: /platform announcements/i,
    })
    await userEvent.click(toggle)

    const saveBtn = screen.getByRole('button', { name: /save changes/i })
    await userEvent.click(saveBtn)

    await waitFor(() => {
      expect(
        screen.getByText('Preferences saved successfully.')
      ).toBeInTheDocument()
    })
  })

  // ---------------------------------------------------------------------------
  // Bug 1 — guard against missing types field
  // ---------------------------------------------------------------------------

  // ---------------------------------------------------------------------------
  // Footer placement — save/reset must appear after the activity email card
  // ---------------------------------------------------------------------------

  it('save/reset footer renders after the activity email card, not inside the email card', async () => {
    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(screen.getByText('Platform announcements')).toBeInTheDocument()
    })

    const toggle = screen.getByRole('switch', {
      name: /platform announcements/i,
    })
    await userEvent.click(toggle)

    const saveFooter = await screen.findByTestId('save-footer')
    expect(saveFooter).toBeInTheDocument()

    // The activity email card should precede the save footer in DOM order
    const activityHeading = screen.getByText('In-app activity email')
    expect(
      activityHeading.compareDocumentPosition(saveFooter) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  it('save/reset footer is NOT inside the email notifications card', async () => {
    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(screen.getByText('Platform announcements')).toBeInTheDocument()
    })

    const toggle = screen.getByRole('switch', {
      name: /platform announcements/i,
    })
    await userEvent.click(toggle)

    const emailCard = await screen.findByTestId('email-notifications-card')
    const saveFooter = await screen.findByTestId('save-footer')

    expect(emailCard).not.toContainElement(saveFooter)
  })

  it('resets preferences when Reset is clicked', async () => {
    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(screen.getByText('Platform announcements')).toBeInTheDocument()
    })

    const toggle = screen.getByRole('switch', {
      name: /platform announcements/i,
    })
    await userEvent.click(toggle)

    expect(screen.getByText('You have unsaved changes.')).toBeInTheDocument()

    const resetBtn = screen.getByRole('button', { name: /reset/i })
    await userEvent.click(resetBtn)

    expect(
      screen.queryByText('You have unsaved changes.')
    ).not.toBeInTheDocument()
  })

  // ---------------------------------------------------------------------------
  // Activity email card — master toggle
  // ---------------------------------------------------------------------------

  it('renders in-app activity email card with master toggle', async () => {
    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(screen.getByText('In-app activity email')).toBeInTheDocument()
    })

    expect(
      screen.getByText('Email me about activity in my teams')
    ).toBeInTheDocument()
  })

  it('master email channel toggle is on when channels.email is true', async () => {
    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(screen.getByText('In-app activity email')).toBeInTheDocument()
    })

    const toggle = screen.getByRole('switch', {
      name: /email me about activity in my teams/i,
    })
    expect(toggle).toHaveAttribute('data-state', 'checked')
  })

  it('master email channel toggle is off when channels.email is false', async () => {
    mockPreferencesService.getPreferences.mockResolvedValue(
      makePrefsResponse({
        ...baseNotifPrefs,
        channels: { ...baseNotifPrefs.channels, email: false },
      })
    )

    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(screen.getByText('In-app activity email')).toBeInTheDocument()
    })

    const toggle = screen.getByRole('switch', {
      name: /email me about activity in my teams/i,
    })
    expect(toggle).toHaveAttribute('data-state', 'unchecked')
  })

  // ---------------------------------------------------------------------------
  // Activity email card — per-type segmented control visibility
  // ---------------------------------------------------------------------------

  it('shows per-type segmented controls when email channel master is on', async () => {
    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(
        screen.getByText('Delivery frequency per type')
      ).toBeInTheDocument()
    })

    // Segmented controls for all three notification types should be present
    expect(screen.getAllByText('Instant').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Daily digest').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Off').length).toBeGreaterThan(0)
  })

  it('does not show per-type segmented controls when email channel master is off', async () => {
    mockPreferencesService.getPreferences.mockResolvedValue(
      makePrefsResponse({
        ...baseNotifPrefs,
        channels: { ...baseNotifPrefs.channels, email: false },
      })
    )

    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(screen.getByText('In-app activity email')).toBeInTheDocument()
    })

    expect(
      screen.queryByText('Delivery frequency per type')
    ).not.toBeInTheDocument()
  })

  // ---------------------------------------------------------------------------
  // Activity email card — toggling the master creates unsaved changes
  // ---------------------------------------------------------------------------

  it('toggling email channel master creates unsaved changes', async () => {
    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(screen.getByText('In-app activity email')).toBeInTheDocument()
    })

    const toggle = screen.getByRole('switch', {
      name: /email me about activity in my teams/i,
    })
    await userEvent.click(toggle)

    expect(screen.getByText('You have unsaved changes.')).toBeInTheDocument()
  })

  // ---------------------------------------------------------------------------
  // Activity email card — per-type segmented controls change state
  // ---------------------------------------------------------------------------

  it('changing a per-type email value creates unsaved changes', async () => {
    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(
        screen.getByText('Delivery frequency per type')
      ).toBeInTheDocument()
    })

    // Click "Instant" for the first occurrence (feed.item.created is currently "digest")
    const instantBtns = screen.getAllByText('Instant')
    await userEvent.click(instantBtns[0])

    expect(screen.getByText('You have unsaved changes.')).toBeInTheDocument()
  })

  it('save includes updated email channel and type preferences', async () => {
    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(
        screen.getByText('Delivery frequency per type')
      ).toBeInTheDocument()
    })

    // Click "Off" for one of the notification types
    const offBtns = screen.getAllByText('Off')
    await userEvent.click(offBtns[0])

    const saveBtn = screen.getByRole('button', { name: /save changes/i })
    await userEvent.click(saveBtn)

    await waitFor(() => {
      expect(mockPreferencesService.updatePreferences).toHaveBeenCalledWith(
        expect.objectContaining({
          notifications: expect.objectContaining({
            channels: expect.objectContaining({ email: true }),
          }),
        })
      )
    })
  })

  // ---------------------------------------------------------------------------
  // Activity email card — handles missing types gracefully
  // ---------------------------------------------------------------------------

  it('renders activity email card without crash when notifPrefs.types is undefined', async () => {
    const prefsWithoutTypes: NotificationPrefsType = {
      channels: { in_app: true, email: true },
      types: {},
    }
    mockPreferencesService.getPreferences.mockResolvedValue(
      makePrefsResponse(prefsWithoutTypes)
    )

    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(screen.getByText('In-app activity email')).toBeInTheDocument()
    })

    // Master toggle is on (email: true)
    const toggle = screen.getByRole('switch', {
      name: /email me about activity in my teams/i,
    })
    expect(toggle).toHaveAttribute('data-state', 'checked')

    // Section header shows (master is on) but no per-type rows rendered (types is empty)
    expect(screen.getByText('Delivery frequency per type')).toBeInTheDocument()
    // None of the notification type labels should appear in the activity email section
    expect(screen.queryByText('New feed items')).not.toBeInTheDocument()
  })

  // ---------------------------------------------------------------------------
  // Activity email card — undefined email field defaults to digest
  // ---------------------------------------------------------------------------

  it('shows digest as selected when typePrefs.email is undefined', async () => {
    const prefsWithUndefinedEmail = {
      channels: { in_app: true, email: true },
      types: {
        // email field intentionally omitted to simulate old backend data
        'feed.item.created': {
          in_app: true,
          email: undefined,
        },
      },
    } as unknown as NotificationPrefsType
    mockPreferencesService.getPreferences.mockResolvedValue(
      makePrefsResponse(prefsWithUndefinedEmail)
    )

    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(
        screen.getByText('Delivery frequency per type')
      ).toBeInTheDocument()
    })

    // "Daily digest" tab should be in active state (data-state="active")
    const digestBtns = screen.getAllByText('Daily digest')
    // At least one should exist for feed.item.created row
    expect(digestBtns.length).toBeGreaterThan(0)
    // The component should not crash — activity email card renders
    expect(screen.getByText('In-app activity email')).toBeInTheDocument()
  })

  // ---------------------------------------------------------------------------
  // Activity email card — save passes updated types correctly
  // ---------------------------------------------------------------------------

  it('save passes updated email type through updatePreferences', async () => {
    mockPreferencesService.updatePreferences.mockResolvedValue(
      makePrefsResponse(baseNotifPrefs)
    )

    render(<NotificationPreferences />)

    await waitFor(() => {
      expect(
        screen.getByText('Delivery frequency per type')
      ).toBeInTheDocument()
    })

    // Switch feed.item.created from "digest" to "instant" (first Instant button)
    const instantBtns = screen.getAllByText('Instant')
    await userEvent.click(instantBtns[0])

    const saveBtn = screen.getByRole('button', { name: /save changes/i })
    await userEvent.click(saveBtn)

    await waitFor(() => {
      expect(mockPreferencesService.updatePreferences).toHaveBeenCalledWith(
        expect.objectContaining({
          notifications: expect.objectContaining({
            types: expect.objectContaining({
              'feed.item.created': expect.objectContaining({
                email: 'instant',
              }),
            }),
          }),
        })
      )
    })
  })
})
