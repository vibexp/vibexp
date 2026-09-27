/**
 * InstanceEmailCard (#1192): the dashboard's "Instance email" health entry, one
 * rendering per state, each linking to the settings page.
 */
import { act, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router'

import type { AdminInstanceEmailSettings } from '@/services/adminService'
import { adminService } from '@/services/adminService'

vi.mock('@/services/adminService', () => ({
  adminService: { getInstanceEmailSettings: vi.fn() },
}))

import { emitInstanceEmailChanged } from '../../instanceEmailEvents'
import { InstanceEmailCard } from '../InstanceEmailCard'

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
  provider_type: 'mailgun',
  has_credential: true,
  is_healthy: true,
  last_success_at: '2026-09-20T10:00:00Z',
  ...overrides,
})

const renderCard = () =>
  render(
    <MemoryRouter>
      <InstanceEmailCard />
    </MemoryRouter>
  )

const stateLine = () => screen.findByTestId('instance-email-state')

const expectSettingsLink = () => {
  expect(screen.getByRole('link', { name: 'Email settings' })).toHaveAttribute(
    'href',
    '/admin/settings/email'
  )
}

beforeEach(() => {
  vi.clearAllMocks()
})

it('shows a skeleton while the status loads', () => {
  service.getInstanceEmailSettings.mockReturnValue(new Promise(() => undefined))
  const { container } = renderCard()

  expect(screen.getByText('Instance email')).toBeInTheDocument()
  expect(container.querySelectorAll('.animate-pulse')).toHaveLength(1)
  expect(screen.queryByTestId('instance-email-state')).not.toBeInTheDocument()
  expectSettingsLink()
})

it('reads "Configured" with the provider and the last success', async () => {
  service.getInstanceEmailSettings.mockResolvedValue(configured())
  renderCard()

  const line = await stateLine()
  expect(line).toHaveTextContent('Configured')
  expect(line).not.toHaveClass('text-destructive')
  expect(screen.getByText(/^Mailgun · last sent .*2026/)).toBeInTheDocument()
  expectSettingsLink()
})

it('reads "Configured" with "No sends yet" for a provider that never sent', async () => {
  service.getInstanceEmailSettings.mockResolvedValue(
    configured({ last_success_at: null })
  )
  renderCard()

  expect(await stateLine()).toHaveTextContent('Configured')
  expect(screen.getByText('Mailgun · No sends yet')).toBeInTheDocument()
})

it('reads "Not configured" and says instance mail is discarded', async () => {
  service.getInstanceEmailSettings.mockResolvedValue(unconfigured)
  renderCard()

  const line = await stateLine()
  expect(line).toHaveTextContent('Not configured')
  expect(line).toHaveClass('text-destructive')
  expect(
    screen.getByText(
      'Instance mail is being discarded until a provider is configured.'
    )
  ).toBeInTheDocument()
  expectSettingsLink()
})

it('reads "Last send failed" with its time and the truncated error', async () => {
  const longError = `550 rejected: ${'x'.repeat(300)}`
  service.getInstanceEmailSettings.mockResolvedValue(
    configured({
      is_healthy: false,
      last_error: longError,
      last_error_at: '2026-09-21T10:00:00Z',
    })
  )
  renderCard()

  const line = await stateLine()
  expect(line).toHaveTextContent('Last send failed')
  expect(line).toHaveClass('text-destructive')
  expect(screen.getByText(/^Failed at .*2026/)).toBeInTheDocument()
  const error = screen.getByText(/^550 rejected: x+…$/)
  expect(error.textContent).toHaveLength(160)
  expectSettingsLink()
})

it('shows a short error in full', async () => {
  service.getInstanceEmailSettings.mockResolvedValue(
    configured({
      is_healthy: false,
      last_error: 'connection refused',
      last_error_at: null,
    })
  )
  renderCard()

  expect(await stateLine()).toHaveTextContent('Last send failed')
  expect(screen.getByText('connection refused')).toBeInTheDocument()
  expect(screen.queryByText(/^Failed at/)).not.toBeInTheDocument()
})

it('reads "Status unavailable" when the status cannot be read', async () => {
  const consoleError = vi
    .spyOn(console, 'error')
    .mockImplementation(() => undefined)
  service.getInstanceEmailSettings.mockRejectedValue(new Error('offline'))
  renderCard()

  const line = await stateLine()
  expect(line).toHaveTextContent('Status unavailable')
  expect(line).not.toHaveClass('text-destructive')
  expectSettingsLink()
  consoleError.mockRestore()
})

it('updates without a reload when the settings change', async () => {
  service.getInstanceEmailSettings
    .mockResolvedValueOnce(unconfigured)
    .mockResolvedValueOnce(configured())
  renderCard()
  expect(await stateLine()).toHaveTextContent('Not configured')

  act(() => {
    emitInstanceEmailChanged()
  })

  await waitFor(() => {
    expect(screen.getByTestId('instance-email-state')).toHaveTextContent(
      'Configured'
    )
  })
})
