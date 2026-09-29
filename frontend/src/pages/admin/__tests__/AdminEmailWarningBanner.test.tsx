/**
 * AdminEmailWarningBanner (#1192): shown across the admin shell while instance
 * email is unconfigured or failing, dismissible for the browser session, and
 * never on the email settings page itself.
 */
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import type { AdminInstanceEmailSettings } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { sessionStore } from '@/utils/storage'

vi.mock('@/services/adminService', () => ({
  adminService: { getInstanceEmailSettings: vi.fn() },
}))

import { AdminEmailWarningBanner } from '../AdminEmailWarningBanner'
import { emitInstanceEmailChanged } from '../instanceEmailEvents'

const service = vi.mocked(adminService)

const unconfigured: AdminInstanceEmailSettings = {
  configured: false,
  provider_type: null,
  has_credential: false,
  is_healthy: null,
}

const configured = (
  overrides: Partial<AdminInstanceEmailSettings> = {}
): AdminInstanceEmailSettings => ({
  configured: true,
  provider_type: 'smtp',
  has_credential: true,
  is_healthy: true,
  last_success_at: '2026-09-20T10:00:00Z',
  ...overrides,
})

const failing = configured({
  is_healthy: false,
  last_error: 'connection refused',
  last_error_at: '2026-09-21T10:00:00Z',
})

const renderBanner = (path = '/admin/users') =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <AdminEmailWarningBanner />
    </MemoryRouter>
  )

/** Waits for the status read to settle, so "hidden" is not a loading artefact. */
const settled = () =>
  waitFor(() => {
    expect(service.getInstanceEmailSettings).toHaveBeenCalled()
  })

beforeEach(() => {
  vi.clearAllMocks()
  sessionStore.remove(STORAGE_KEYS.ADMIN_EMAIL_BANNER_DISMISSED)
})

it('warns that mail is being discarded when nothing is configured', async () => {
  service.getInstanceEmailSettings.mockResolvedValue(unconfigured)
  renderBanner()

  const banner = await screen.findByTestId('admin-email-warning')
  expect(banner).toHaveTextContent('Instance email is not configured')
  expect(banner).toHaveTextContent(
    'Invitations, notifications and digests from teams without their own mail provider are being discarded.'
  )
  expect(screen.getByRole('link', { name: 'Configure email' })).toHaveAttribute(
    'href',
    '/admin/settings/email'
  )
})

it('warns that the last send failed, with its time', async () => {
  service.getInstanceEmailSettings.mockResolvedValue(failing)
  renderBanner()

  const banner = await screen.findByTestId('admin-email-warning')
  expect(banner).toHaveTextContent('The last instance email send failed')
  expect(banner).toHaveTextContent(/It failed at .*2026/)
  expect(banner).not.toHaveTextContent('being discarded')
  expect(screen.getByRole('link', { name: 'Configure email' })).toHaveAttribute(
    'href',
    '/admin/settings/email'
  )
})

it('keeps the full failure message byte-identical, with and without its time', async () => {
  service.getInstanceEmailSettings.mockResolvedValue(failing)
  const { unmount } = renderBanner()
  const tail =
    'Invitations, notifications and digests from teams without their own mail provider may not be delivered.'

  const withTime = await screen.findByTestId('admin-email-warning')
  const withTimeText = withTime.querySelector('p')?.textContent ?? ''
  expect(withTimeText.startsWith('It failed at ')).toBe(true)
  expect(withTimeText.endsWith(`. ${tail}`)).toBe(true)
  unmount()

  service.getInstanceEmailSettings.mockResolvedValue(
    configured({ is_healthy: false, last_error_at: null })
  )
  renderBanner()
  const withoutTime = await screen.findByTestId('admin-email-warning')
  expect(withoutTime.querySelector('p')?.textContent).toBe(tail)
})

it.each([
  ['healthy', configured()],
  [
    'configured but never sent',
    configured({ last_success_at: null, last_error_at: null }),
  ],
])('stays hidden when instance email is %s', async (_label, settings) => {
  service.getInstanceEmailSettings.mockResolvedValue(settings)
  renderBanner()
  await settled()

  // Let the resolved read render before asserting absence.
  await act(async () => {
    await Promise.resolve()
  })
  expect(screen.queryByTestId('admin-email-warning')).not.toBeInTheDocument()
})

it('stays hidden when the status cannot be read', async () => {
  const consoleError = vi
    .spyOn(console, 'error')
    .mockImplementation(() => undefined)
  service.getInstanceEmailSettings.mockRejectedValue(new Error('offline'))
  renderBanner()

  await waitFor(() => {
    expect(consoleError).toHaveBeenCalled()
  })
  expect(screen.queryByTestId('admin-email-warning')).not.toBeInTheDocument()
  consoleError.mockRestore()
})

it('stays dismissed for the session, across remounts', async () => {
  const user = userEvent.setup()
  service.getInstanceEmailSettings.mockResolvedValue(unconfigured)
  const { unmount } = renderBanner()

  await user.click(await screen.findByRole('button', { name: 'Dismiss' }))

  expect(screen.queryByTestId('admin-email-warning')).not.toBeInTheDocument()
  expect(sessionStore.get(STORAGE_KEYS.ADMIN_EMAIL_BANNER_DISMISSED)).toBe(
    'true'
  )

  unmount()
  renderBanner('/admin/teams')
  await settled()
  await act(async () => {
    await Promise.resolve()
  })
  expect(screen.queryByTestId('admin-email-warning')).not.toBeInTheDocument()
})

it.each([
  '/admin/settings/email',
  '/admin/settings/email/',
  '/admin/settings/email//',
])('never renders on the email settings page (%s)', async path => {
  service.getInstanceEmailSettings.mockResolvedValue(unconfigured)
  renderBanner(path)
  await settled()
  await act(async () => {
    await Promise.resolve()
  })

  expect(screen.queryByTestId('admin-email-warning')).not.toBeInTheDocument()
})

it.each(['/admin/settings/', '/admin/settings/email/smtp/', '/'])(
  'still renders on other admin paths with trailing slashes (%s)',
  async path => {
    service.getInstanceEmailSettings.mockResolvedValue(unconfigured)
    renderBanner(path)

    expect(await screen.findByTestId('admin-email-warning')).toBeInTheDocument()
  }
)

it('updates without a reload when the settings change', async () => {
  service.getInstanceEmailSettings
    .mockResolvedValueOnce(unconfigured)
    .mockResolvedValueOnce(configured())
  renderBanner()
  await screen.findByTestId('admin-email-warning')

  act(() => {
    emitInstanceEmailChanged()
  })

  await waitFor(() => {
    expect(screen.queryByTestId('admin-email-warning')).not.toBeInTheDocument()
  })
  expect(service.getInstanceEmailSettings).toHaveBeenCalledTimes(2)
})
