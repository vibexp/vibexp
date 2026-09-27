import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { toast } from '@/lib/toast'
import type {
  AdminInstanceEmailSettings,
  AdminInstanceEmailTestResponse,
} from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { ApiError } from '@/types/errors'

// Stable handleError reference (like the real useCallback-backed hook) so the
// load callback isn't recreated every render.
const mockHandleError = vi.hoisted(() => vi.fn())
vi.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('@/services/adminService', () => ({
  adminService: {
    getInstanceEmailSettings: vi.fn(),
    upsertInstanceEmailSettings: vi.fn(),
    deleteInstanceEmailSettings: vi.fn(),
    testInstanceEmailSettings: vi.fn(),
    listInstanceEmailSettingsAudit: vi.fn(),
  },
}))

import { AdminEmailSettings } from '../AdminEmailSettings'

const service = vi.mocked(adminService)
const mockedToast = vi.mocked(toast)

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
  settings: {
    smtp: { host: 'smtp.acme.test', port: '587', username: 'mailer' },
  },
  has_credential: true,
  from_address: 'noreply@acme.test',
  from_name: 'Acme',
  contact_recipient_address: 'hello@acme.test',
  privacy_policy_url: null,
  is_healthy: true,
  last_success_at: '2026-09-20T10:00:00Z',
  updated_at: '2026-09-19T10:00:00Z',
  updated_by: '7d3c0b8e-8f7a-4a51-9a3e-1b2c3d4e5f60',
  ...overrides,
})

const sent: AdminInstanceEmailTestResponse = {
  is_valid: true,
  message: 'Test email sent',
  recipient: 'admin@acme.test',
  details: {},
}

const validationError = (field: string, message: string) =>
  new ApiError({
    type: 'about:blank',
    title: 'Bad Request',
    status: 400,
    detail: 'Instance email settings are invalid',
    code: 'INSTANCE_EMAIL_PROVIDER_VALIDATION_FAILED',
    request_id: 'req-1',
    timestamp: '2026-09-27T00:00:00Z',
    validation_errors: [{ field, message }],
  } as ConstructorParameters<typeof ApiError>[0])

const conflict = () =>
  new ApiError({
    type: 'about:blank',
    title: 'Conflict',
    status: 409,
    detail: 'No instance email provider is configured',
    code: 'INSTANCE_EMAIL_PROVIDER_NOT_CONFIGURED',
    request_id: 'req-2',
    timestamp: '2026-09-27T00:00:00Z',
  })

const renderPage = () =>
  render(
    <MemoryRouter>
      <AdminEmailSettings />
    </MemoryRouter>
  )

const saveButton = () => screen.getByRole('button', { name: /save changes/i })
const testButton = () =>
  screen.getByRole('button', { name: /send test email/i })

beforeEach(() => {
  vi.clearAllMocks()
  service.getInstanceEmailSettings.mockResolvedValue(unconfigured)
  service.listInstanceEmailSettingsAudit.mockResolvedValue({
    entries: [],
    next_cursor: null,
  })
})

describe('AdminEmailSettings — states', () => {
  it('says unconfigured instance mail is discarded and offers an empty form', async () => {
    renderPage()

    expect(
      await screen.findByText(/instance email is not configured/i)
    ).toBeInTheDocument()
    expect(screen.getByText(/are\s+discarded until a provider/i)).toBeVisible()
    expect(screen.getByLabelText(/from address/i)).toHaveValue('')
    expect(
      screen.queryByRole('button', { name: /remove configuration/i })
    ).not.toBeInTheDocument()
  })

  it('shows a healthy provider with its last success and who saved it', async () => {
    service.getInstanceEmailSettings.mockResolvedValue(configured())
    renderPage()

    const status = await screen.findByTestId('instance-email-status')
    expect(within(status).getByText('Healthy')).toBeInTheDocument()
    expect(
      within(status).getByText(/last delivered successfully at/i)
    ).toBeInTheDocument()
    expect(
      within(status).getByRole('link', { name: /this administrator/i })
    ).toHaveAttribute(
      'href',
      '/admin/users/7d3c0b8e-8f7a-4a51-9a3e-1b2c3d4e5f60'
    )
    expect(screen.getByLabelText(/^host$/i)).toHaveValue('smtp.acme.test')
    expect(screen.getByLabelText(/contact form recipient/i)).toHaveValue(
      'hello@acme.test'
    )
  })

  it('attributes a save with no actor to the config.yaml import', async () => {
    service.getInstanceEmailSettings.mockResolvedValue(
      configured({ updated_by: null })
    )
    renderPage()

    expect(
      await screen.findByText(/by the config\.yaml import/i)
    ).toBeInTheDocument()
  })

  it('shows a failing provider with its last error and time', async () => {
    service.getInstanceEmailSettings.mockResolvedValue(
      configured({
        is_healthy: false,
        last_error: 'smtp: connection refused',
        last_error_at: '2026-09-21T10:00:00Z',
      })
    )
    renderPage()

    const status = await screen.findByTestId('instance-email-status')
    expect(within(status).getByText('Delivery failing')).toBeInTheDocument()
    expect(within(status).getByText(/last delivery failed at/i)).toBeVisible()
    expect(
      within(status).getByText('smtp: connection refused')
    ).toBeInTheDocument()
  })

  it('does not call a recovered provider failing because an error is retained', async () => {
    service.getInstanceEmailSettings.mockResolvedValue(
      configured({ last_error: 'old failure' })
    )
    renderPage()

    const status = await screen.findByTestId('instance-email-status')
    expect(within(status).getByText('Healthy')).toBeInTheDocument()
    expect(within(status).getByText(/has since recovered/i)).toBeVisible()
  })

  it('reports a load failure', async () => {
    service.getInstanceEmailSettings.mockRejectedValue(new Error('boom'))
    renderPage()

    expect(await screen.findByText('boom')).toBeInTheDocument()
  })
})

describe('AdminEmailSettings — save', () => {
  it('requires a credential the first time, before any request', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.type(await screen.findByLabelText(/^host$/i), 'mailpit')
    await user.type(screen.getByLabelText(/^port$/i), '1025')
    await user.type(screen.getByLabelText(/from address/i), 'no@acme.test')
    await user.click(saveButton())

    expect(
      await screen.findByText('A credential is required')
    ).toBeInTheDocument()
    expect(service.upsertInstanceEmailSettings).not.toHaveBeenCalled()
  })

  it('omits the secret when saving the same provider type, and reloads the history', async () => {
    const user = userEvent.setup()
    service.getInstanceEmailSettings.mockResolvedValue(configured())
    service.upsertInstanceEmailSettings.mockResolvedValue(
      configured({ from_name: 'Acme Mail' })
    )
    renderPage()

    const name = await screen.findByLabelText(/display name/i)
    await user.clear(name)
    await user.type(name, 'Acme Mail')
    await user.click(saveButton())

    await waitFor(() => {
      expect(service.upsertInstanceEmailSettings).toHaveBeenCalledTimes(1)
    })
    const body = service.upsertInstanceEmailSettings.mock.calls[0][0]
    expect(body).not.toHaveProperty('secret')
    expect(body).toMatchObject({
      from_name: 'Acme Mail',
      contact_recipient_address: 'hello@acme.test',
      privacy_policy_url: null,
    })
    expect(mockedToast.success).toHaveBeenCalledWith(
      'Instance email settings saved'
    )
    await waitFor(() => {
      expect(service.listInstanceEmailSettingsAudit).toHaveBeenCalledTimes(2)
    })
  })

  it('demands a new credential when the provider type is switched', async () => {
    const user = userEvent.setup()
    service.getInstanceEmailSettings.mockResolvedValue(configured())
    renderPage()

    await user.click(await screen.findByRole('radio', { name: /sendgrid/i }))
    await user.click(saveButton())

    expect(
      await screen.findByText(/new provider’s credential/i)
    ).toBeInTheDocument()
    expect(service.upsertInstanceEmailSettings).not.toHaveBeenCalled()
  })

  it('places a server field error on its input', async () => {
    const user = userEvent.setup()
    service.getInstanceEmailSettings.mockResolvedValue(configured())
    service.upsertInstanceEmailSettings.mockRejectedValue(
      validationError('settings.smtp.host', 'is not reachable')
    )
    renderPage()

    await screen.findByLabelText(/^host$/i)
    await user.click(saveButton())

    expect(await screen.findByText('is not reachable')).toBeInTheDocument()
    expect(mockHandleError).not.toHaveBeenCalled()
  })

  it('falls back to the error handler for an error with no matching input', async () => {
    const user = userEvent.setup()
    service.getInstanceEmailSettings.mockResolvedValue(configured())
    const error = validationError('settings.smtp', 'is required')
    service.upsertInstanceEmailSettings.mockRejectedValue(error)
    renderPage()

    await screen.findByLabelText(/^host$/i)
    await user.click(saveButton())

    await waitFor(() => {
      expect(mockHandleError).toHaveBeenCalledWith(
        error,
        'Failed to save the email settings'
      )
    })
  })
})

describe('AdminEmailSettings — test send', () => {
  it('tests the stored configuration with no body when the form is untouched', async () => {
    const user = userEvent.setup()
    service.getInstanceEmailSettings.mockResolvedValue(configured())
    service.testInstanceEmailSettings.mockResolvedValue(sent)
    renderPage()

    await screen.findByLabelText(/^host$/i)
    await user.click(testButton())

    await waitFor(() => {
      expect(service.testInstanceEmailSettings).toHaveBeenCalledWith(undefined)
    })
    const result = await screen.findByTestId('instance-email-test-result')
    expect(result).toHaveTextContent('Test email sent')
    expect(result).toHaveTextContent('Sent to admin@acme.test.')
  })

  it('tests edited values as a candidate, borrowing the credential for the same destination', async () => {
    const user = userEvent.setup()
    service.getInstanceEmailSettings.mockResolvedValue(configured())
    service.testInstanceEmailSettings.mockResolvedValue(sent)
    renderPage()

    const from = await screen.findByLabelText(/from address/i)
    await user.clear(from)
    await user.type(from, 'mail@acme.test')
    await user.click(testButton())

    await waitFor(() => {
      expect(service.testInstanceEmailSettings).toHaveBeenCalledTimes(1)
    })
    const body = service.testInstanceEmailSettings.mock.calls[0][0]
    expect(body).toMatchObject({ from_address: 'mail@acme.test' })
    expect(body).not.toHaveProperty('secret')
  })

  it('requires a credential to test a different destination', async () => {
    const user = userEvent.setup()
    service.getInstanceEmailSettings.mockResolvedValue(configured())
    renderPage()

    const host = await screen.findByLabelText(/^host$/i)
    await user.clear(host)
    await user.type(host, 'collector.attacker.test')
    await user.click(testButton())

    expect(
      await screen.findByText(/to test a different destination/i)
    ).toBeInTheDocument()
    expect(service.testInstanceEmailSettings).not.toHaveBeenCalled()
  })

  it('reports a failed send inline, not as an error', async () => {
    const user = userEvent.setup()
    service.getInstanceEmailSettings.mockResolvedValue(configured())
    service.testInstanceEmailSettings.mockResolvedValue({
      is_valid: false,
      message: 'Sending failed',
      recipient: 'admin@acme.test',
      details: { error_details: 'send_failed' },
    })
    renderPage()

    await screen.findByLabelText(/^host$/i)
    await user.click(testButton())

    const result = await screen.findByTestId('instance-email-test-result')
    expect(result).toHaveTextContent('Test email failed')
    expect(result).toHaveTextContent('Reason: send_failed')
    expect(mockHandleError).not.toHaveBeenCalled()
  })
})

describe('AdminEmailSettings — remove', () => {
  it('confirms, deletes and returns to the unconfigured state', async () => {
    const user = userEvent.setup()
    service.getInstanceEmailSettings
      .mockResolvedValueOnce(configured())
      .mockResolvedValue(unconfigured)
    service.deleteInstanceEmailSettings.mockResolvedValue(undefined)
    renderPage()

    await user.click(
      await screen.findByRole('button', { name: /remove configuration/i })
    )
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent(/teams with their own email provider/i)
    expect(service.deleteInstanceEmailSettings).not.toHaveBeenCalled()
    await user.click(within(dialog).getByRole('button', { name: /^remove$/i }))

    await waitFor(() => {
      expect(service.deleteInstanceEmailSettings).toHaveBeenCalledTimes(1)
    })
    expect(
      await screen.findByText(/instance email is not configured/i)
    ).toBeInTheDocument()
  })

  it('treats a 409 (already removed) as removed and refetches', async () => {
    const user = userEvent.setup()
    service.getInstanceEmailSettings
      .mockResolvedValueOnce(configured())
      .mockResolvedValue(unconfigured)
    service.deleteInstanceEmailSettings.mockRejectedValue(conflict())
    renderPage()

    await user.click(
      await screen.findByRole('button', { name: /remove configuration/i })
    )
    await user.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: /^remove$/i,
      })
    )

    expect(
      await screen.findByText(/instance email is not configured/i)
    ).toBeInTheDocument()
    expect(mockHandleError).not.toHaveBeenCalled()
  })

  it('reports any other delete failure and keeps the configuration', async () => {
    const user = userEvent.setup()
    service.getInstanceEmailSettings.mockResolvedValue(configured())
    const error = new Error('network down')
    service.deleteInstanceEmailSettings.mockRejectedValue(error)
    renderPage()

    await user.click(
      await screen.findByRole('button', { name: /remove configuration/i })
    )
    await user.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: /^remove$/i,
      })
    )

    await waitFor(() => {
      expect(mockHandleError).toHaveBeenCalledWith(
        error,
        'Failed to remove the email configuration'
      )
    })
    expect(service.getInstanceEmailSettings).toHaveBeenCalledTimes(1)
  })
})
