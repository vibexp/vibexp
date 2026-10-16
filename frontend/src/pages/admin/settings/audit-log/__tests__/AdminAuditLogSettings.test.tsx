import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { toast } from '@/lib/toast'
import {
  type AdminAuditLogSettings as AuditLogSettingsResponse,
  adminSettingsService,
} from '@/services/adminSettingsService'
import { ApiError } from '@/types/errors'

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('@/services/adminSettingsService', () => ({
  adminSettingsService: {
    getAuditLogSettings: vi.fn(),
    updateAuditLogSettings: vi.fn(),
    resetAuditLogSettings: vi.fn(),
    listAuditLogSettingsAudit: vi.fn(),
  },
}))

import { AdminAuditLogSettings } from '../AdminAuditLogSettings'

const service = vi.mocked(adminSettingsService)

const builtIn: AuditLogSettingsResponse['values'] = {
  retention_weeks: 2,
  capture_ip_ua: true,
}

const defaults: AuditLogSettingsResponse = {
  values: builtIn,
  source: 'default',
  built_in_defaults: builtIn,
  limits: { retention_weeks_min: 1, retention_weeks_max: 52 },
  updated_at: null,
  updated_by_user_id: null,
  updated_by_name: null,
  version: null,
}

const customized: AuditLogSettingsResponse = {
  ...defaults,
  source: 'instance',
  values: { retention_weeks: 8, capture_ip_ua: false },
  updated_at: '2026-10-16T10:00:00Z',
  updated_by_user_id: '7d3c0b8e-8f7a-4a51-9a3e-1b2c3d4e5f60',
  updated_by_name: 'Ada Admin',
  version: 4,
}

const apiError = (
  status: number,
  code: string,
  validation_errors?: { field: string; message: string }[]
) =>
  new ApiError({
    type: 'about:blank',
    title: 'Error',
    status,
    detail: 'Request failed',
    code,
    request_id: 'req-1',
    timestamp: '2026-10-16T00:00:00Z',
    validation_errors,
  } as ConstructorParameters<typeof ApiError>[0])

const saveButton = () => screen.getByRole('button', { name: 'Save changes' })
const retention = () => screen.findByLabelText('Retention')

beforeEach(() => {
  vi.clearAllMocks()
  service.getAuditLogSettings.mockResolvedValue(defaults)
  service.listAuditLogSettingsAudit.mockResolvedValue({
    entries: [],
    next_cursor: null,
  })
})

describe('AdminAuditLogSettings — status', () => {
  it('shows the built-in defaults with no override line and no reset', async () => {
    render(<AdminAuditLogSettings />)

    expect(
      await screen.findByTestId('instance-settings-source')
    ).toHaveTextContent('Using built-in defaults')
    expect(
      screen.queryByTestId('instance-settings-overrides')
    ).not.toBeInTheDocument()
    expect(await retention()).toHaveValue(2)
    // A short number: a narrow input with the unit after it.
    expect(await retention()).toHaveClass('w-24')
    expect(screen.getByText('weeks')).toBeInTheDocument()
    expect(
      screen.getByLabelText('Capture IP address and user agent')
    ).toBeChecked()
    expect(
      screen.queryByRole('button', { name: /reset to defaults/i })
    ).not.toBeInTheDocument()
    expect(saveButton()).toBeDisabled()
  })

  it('shows stored values and who changed them', async () => {
    service.getAuditLogSettings.mockResolvedValue(customized)
    render(<AdminAuditLogSettings />)

    const status = await screen.findByTestId('instance-settings-status')
    expect(within(status).getByText('Customized')).toBeInTheDocument()
    expect(status).toHaveTextContent(/last changed by ada admin at/i)
    expect(await retention()).toHaveValue(8)
    expect(
      screen.getByLabelText('Capture IP address and user agent')
    ).not.toBeChecked()
  })

  it('says turning capture off affects new events only', async () => {
    render(<AdminAuditLogSettings />)

    expect(
      await screen.findByLabelText('Capture IP address and user agent')
    ).toHaveAccessibleDescription(
      /affects new events only.*until they are purged/
    )
  })

  it('shows a load failure', async () => {
    service.getAuditLogSettings.mockRejectedValue(new Error('backend down'))
    render(<AdminAuditLogSettings />)

    expect(await screen.findByText('backend down')).toBeInTheDocument()
  })
})

describe('AdminAuditLogSettings — validation and warning', () => {
  it.each(['0', '53'])(
    'blocks saving retention %s with an inline error',
    async raw => {
      const user = userEvent.setup()
      render(<AdminAuditLogSettings />)

      const field = await retention()
      await user.clear(field)
      await user.type(field, raw)

      expect(field).toHaveAttribute('aria-invalid', 'true')
      expect(field).toHaveAccessibleDescription(
        /Enter a whole number between 1 and 52\./
      )
      expect(saveButton()).toBeDisabled()
    }
  )

  it('warns before save when retention is lowered below the saved value', async () => {
    const user = userEvent.setup()
    service.getAuditLogSettings.mockResolvedValue(customized)
    render(<AdminAuditLogSettings />)

    const field = await retention()
    expect(
      screen.queryByTestId('audit-log-retention-warning')
    ).not.toBeInTheDocument()

    await user.clear(field)
    await user.type(field, '3')
    expect(screen.getByTestId('audit-log-retention-warning')).toHaveTextContent(
      /older than 3 weeks will be deleted at the next daily purge/
    )

    // Raising it back to (or above) the saved value clears the warning.
    await user.clear(field)
    await user.type(field, '10')
    expect(
      screen.queryByTestId('audit-log-retention-warning')
    ).not.toBeInTheDocument()
  })

  it('compares against the saved value, not the previous keystroke', async () => {
    const user = userEvent.setup()
    service.getAuditLogSettings.mockResolvedValue(customized)
    service.updateAuditLogSettings.mockResolvedValue({
      ...customized,
      values: { ...customized.values, retention_weeks: 4 },
      version: 5,
    })
    render(<AdminAuditLogSettings />)

    const field = await retention()
    await user.clear(field)
    await user.type(field, '4')
    await user.click(saveButton())
    await waitFor(() => {
      expect(saveButton()).toBeDisabled()
    })
    // Saved at 4, so 4 no longer warns and 5 (a raise) does not either.
    expect(
      screen.queryByTestId('audit-log-retention-warning')
    ).not.toBeInTheDocument()
    await user.clear(field)
    await user.type(field, '5')
    expect(
      screen.queryByTestId('audit-log-retention-warning')
    ).not.toBeInTheDocument()
  })
})

describe('AdminAuditLogSettings — saving', () => {
  it('sends the whole row with the last-read version and refreshes the history', async () => {
    const user = userEvent.setup()
    service.getAuditLogSettings.mockResolvedValue(customized)
    service.updateAuditLogSettings.mockResolvedValue({
      ...customized,
      values: { retention_weeks: 12, capture_ip_ua: false },
      version: 5,
    })
    render(<AdminAuditLogSettings />)

    const field = await retention()
    await user.clear(field)
    await user.type(field, '12')
    await user.click(saveButton())

    expect(service.updateAuditLogSettings).toHaveBeenCalledWith({
      retention_weeks: 12,
      capture_ip_ua: false,
      expected_version: 4,
    })
    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith(
        'Instance audit log settings saved'
      )
    })
    expect(service.listAuditLogSettingsAudit).toHaveBeenCalledTimes(2)
  })

  it('sends version 0 when nothing is stored yet', async () => {
    const user = userEvent.setup()
    service.updateAuditLogSettings.mockResolvedValue(customized)
    render(<AdminAuditLogSettings />)

    await user.click(
      await screen.findByLabelText('Capture IP address and user agent')
    )
    await user.click(saveButton())

    expect(service.updateAuditLogSettings).toHaveBeenCalledWith({
      retention_weeks: 2,
      capture_ip_ua: false,
      expected_version: 0,
    })
  })

  it('puts a server validation error under the field', async () => {
    const user = userEvent.setup()
    service.updateAuditLogSettings.mockRejectedValue(
      apiError(400, 'INSTANCE_SETTINGS_VALIDATION_FAILED', [
        { field: 'retention_weeks', message: 'retention is out of range' },
      ])
    )
    render(<AdminAuditLogSettings />)

    const field = await retention()
    await user.clear(field)
    await user.type(field, '6')
    await user.click(saveButton())

    expect(
      await screen.findByText('retention is out of range')
    ).toBeInTheDocument()
    expect(field).toHaveAttribute('aria-invalid', 'true')
  })

  it('on a version conflict, keeps the edits, blocks saving and offers a reload', async () => {
    const user = userEvent.setup()
    service.getAuditLogSettings.mockResolvedValue(customized)
    service.updateAuditLogSettings.mockRejectedValue(
      apiError(409, 'INSTANCE_SETTINGS_VERSION_CONFLICT')
    )
    render(<AdminAuditLogSettings />)

    const field = await retention()
    await user.clear(field)
    await user.type(field, '20')
    await user.click(saveButton())

    const alert = await screen.findByTestId('instance-settings-conflict')
    expect(alert).toHaveTextContent(/someone else changed these settings/i)
    expect(field).toHaveValue(20)
    expect(saveButton()).toBeDisabled()

    service.getAuditLogSettings.mockResolvedValue({
      ...customized,
      values: { ...customized.values, retention_weeks: 30 },
      version: 5,
    })
    await user.click(within(alert).getByRole('button', { name: 'Reload' }))

    await waitFor(() => {
      expect(screen.getByLabelText('Retention')).toHaveValue(30)
    })
    expect(
      screen.queryByTestId('instance-settings-conflict')
    ).not.toBeInTheDocument()
  })
})

describe('AdminAuditLogSettings — reset', () => {
  it('previews the built-in defaults, then resets on confirm', async () => {
    const user = userEvent.setup()
    service.getAuditLogSettings.mockResolvedValue(customized)
    service.resetAuditLogSettings.mockResolvedValue(undefined)
    render(<AdminAuditLogSettings />)

    await user.click(
      await screen.findByRole('button', { name: /reset to defaults/i })
    )
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent(
      'retention 2 weeks, IP address and user agent capture on. Audit events older than 2 weeks will be deleted at the next daily purge.'
    )
    service.getAuditLogSettings.mockResolvedValue(defaults)
    await user.click(
      within(dialog).getByRole('button', { name: 'Reset to defaults' })
    )

    await waitFor(() => {
      expect(screen.getByTestId('instance-settings-source')).toHaveTextContent(
        'Using built-in defaults'
      )
    })
    expect(service.resetAuditLogSettings).toHaveBeenCalledTimes(1)
    expect(screen.getByLabelText('Retention')).toHaveValue(2)
    expect(service.listAuditLogSettingsAudit).toHaveBeenCalledTimes(2)
  })
})

describe('AdminAuditLogSettings — history', () => {
  it('lists the changed fields of an edit', async () => {
    service.listAuditLogSettingsAudit.mockResolvedValue({
      entries: [
        {
          id: 'e1',
          setting: 'audit_log',
          action: 'upsert',
          actor_user_id: 'u1',
          actor_name: 'Ada Admin',
          before: { ...builtIn, version: 1 },
          after: { retention_weeks: 8, capture_ip_ua: false, version: 2 },
          created_at: '2026-10-16T10:00:00Z',
        },
      ],
      next_cursor: null,
    })
    render(<AdminAuditLogSettings />)

    const entry = await screen.findByTestId('instance-audit-log-audit-entry')
    expect(entry).toHaveTextContent('Saved')
    expect(entry).toHaveTextContent(/Retention \(weeks\)\s*2\s*→\s*8/)
    expect(entry).toHaveTextContent(/Capture IP address and user agent/)
    expect(entry).not.toHaveTextContent('version')
  })
})

describe('AdminAuditLogSettings — history defaults', () => {
  it('shows the built-in value for the missing side of a first save and a reset', async () => {
    service.listAuditLogSettingsAudit.mockResolvedValue({
      entries: [
        {
          id: 'reset',
          setting: 'audit_log',
          action: 'delete',
          actor_user_id: 'u1',
          actor_name: 'Ada Admin',
          before: { retention_weeks: 12, capture_ip_ua: false, version: 2 },
          after: null,
          created_at: '2026-10-16T11:00:00Z',
        },
        {
          id: 'first',
          setting: 'audit_log',
          action: 'upsert',
          actor_user_id: 'u1',
          actor_name: 'Ada Admin',
          before: null,
          after: { retention_weeks: 8, capture_ip_ua: true, version: 1 },
          created_at: '2026-10-16T10:00:00Z',
        },
      ],
      next_cursor: null,
    })
    render(<AdminAuditLogSettings />)

    const entries = await screen.findAllByTestId(
      'instance-audit-log-audit-entry'
    )
    expect(entries[0]).toHaveTextContent(
      /Retention \(weeks\)\s*12\s*→\s*2 \(default\)/
    )
    expect(entries[0]).toHaveTextContent(/No\s*→\s*Yes \(default\)/)
    expect(entries[1]).toHaveTextContent(
      /Retention \(weeks\)\s*2 \(default\)\s*→\s*8/
    )
    // Capture stayed at its default, so it is not listed as a change.
    expect(entries[1]).not.toHaveTextContent('Capture IP address')
    expect(entries[1]).not.toHaveTextContent('—')
  })
})
