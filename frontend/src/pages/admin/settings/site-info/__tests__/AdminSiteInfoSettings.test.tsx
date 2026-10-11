import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { SITE_INFO_SAVED_EVENT } from '@/features/site-info/siteInfo'
import { toast } from '@/lib/toast'
import {
  adminSettingsService,
  type AdminSiteInfo,
} from '@/services/adminSettingsService'
import { ApiError } from '@/types/errors'

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('@/services/adminSettingsService', () => ({
  adminSettingsService: {
    getSiteInfo: vi.fn(),
    updateSiteInfo: vi.fn(),
    resetSiteInfo: vi.fn(),
    listSiteInfoAudit: vi.fn(),
  },
}))

import { AdminSiteInfoSettings } from '../AdminSiteInfoSettings'

const service = vi.mocked(adminSettingsService)

const empty: AdminSiteInfo['values'] = {
  company_name: '',
  company_email: '',
  website: '',
  support_url: '',
  terms_url: '',
  privacy_url: '',
  logo_url: '',
  logo_dark_url: '',
}

const unconfigured: AdminSiteInfo = {
  values: empty,
  limits: {
    company_name_max_length: 120,
    company_email_max_length: 254,
    url_max_length: 2048,
  },
  updated_at: null,
  updated_by_user_id: null,
  updated_by_name: null,
  version: null,
}

const configured: AdminSiteInfo = {
  ...unconfigured,
  values: {
    ...empty,
    company_name: 'Acme Inc.',
    company_email: 'hello@acme.test',
    website: 'https://acme.test',
    logo_url: 'https://cdn.acme.test/logo.svg',
  },
  updated_at: '2026-10-10T10:00:00Z',
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
    timestamp: '2026-10-11T00:00:00Z',
    validation_errors,
  } as ConstructorParameters<typeof ApiError>[0])

const LABELS = [
  'Company name',
  'Company email',
  'Website',
  'Support URL',
  'Terms of service URL',
  'Privacy policy URL',
  'Logo URL',
  'Dark-mode logo URL',
]

const saveButton = () => screen.getByRole('button', { name: 'Save changes' })

let savedEvents = 0
const onSaved = () => {
  savedEvents += 1
}

beforeEach(() => {
  vi.clearAllMocks()
  savedEvents = 0
  window.addEventListener(SITE_INFO_SAVED_EVENT, onSaved)
  service.getSiteInfo.mockResolvedValue(unconfigured)
  service.listSiteInfoAudit.mockResolvedValue({
    entries: [],
    next_cursor: null,
  })
})

afterEach(() => {
  window.removeEventListener(SITE_INFO_SAVED_EVENT, onSaved)
})

describe('AdminSiteInfoSettings — status and form', () => {
  it('shows all eight fields, blank, as not configured, with nothing to clear', async () => {
    render(<AdminSiteInfoSettings />)

    expect(
      await screen.findByTestId('instance-settings-source')
    ).toHaveTextContent('Not configured')
    for (const label of LABELS) {
      expect(screen.getByLabelText(label)).toHaveValue('')
    }
    expect(
      screen.queryByRole('button', { name: 'Clear site info' })
    ).not.toBeInTheDocument()
    expect(saveButton()).toBeDisabled()
  })

  it('seeds the form from stored values and says who changed them', async () => {
    service.getSiteInfo.mockResolvedValue(configured)
    render(<AdminSiteInfoSettings />)

    const status = await screen.findByTestId('instance-settings-status')
    expect(within(status).getByText('Configured')).toBeInTheDocument()
    expect(status).toHaveTextContent(/last changed by ada admin at/i)
    expect(screen.getByLabelText('Company name')).toHaveValue('Acme Inc.')
    expect(screen.getByLabelText('Logo URL')).toHaveValue(
      'https://cdn.acme.test/logo.svg'
    )
    expect(
      screen.getByRole('button', { name: 'Clear site info' })
    ).toBeInTheDocument()
  })

  it('never shows team-override or built-in-default wording', async () => {
    service.getSiteInfo.mockResolvedValue(configured)
    const { container } = render(<AdminSiteInfoSettings />)

    await screen.findByTestId('instance-settings-status')
    expect(
      screen.queryByTestId('instance-settings-overrides')
    ).not.toBeInTheDocument()
    expect(container).not.toHaveTextContent(/team overrides?/i)
    expect(container).not.toHaveTextContent(/built-in defaults/i)
  })

  it('says that VibeXP branding always remains', async () => {
    render(<AdminSiteInfoSettings />)

    expect(
      await screen.findByText(/“Powered by VibeXP” always remains/)
    ).toBeInTheDocument()
  })

  it('previews the logo being typed, before it is saved', async () => {
    const user = userEvent.setup()
    render(<AdminSiteInfoSettings />)

    await user.type(
      await screen.findByLabelText('Logo URL'),
      'https://cdn.acme.test/new.svg'
    )

    const light = screen.getByTestId('site-info-preview-light')
    expect(within(light).getByRole('img')).toHaveAttribute(
      'src',
      'https://cdn.acme.test/new.svg'
    )
    expect(
      screen.getByTestId('site-info-preview-dark-fallback')
    ).toBeInTheDocument()
    expect(service.updateSiteInfo).not.toHaveBeenCalled()
  })
})

describe('AdminSiteInfoSettings — validation', () => {
  it('flags a bad email and a bad URL inline and blocks saving', async () => {
    const user = userEvent.setup()
    render(<AdminSiteInfoSettings />)

    await user.type(await screen.findByLabelText('Company email'), 'nope')
    await user.type(screen.getByLabelText('Website'), 'acme.test')

    expect(screen.getByLabelText('Company email')).toHaveAccessibleDescription(
      /one email address/i
    )
    expect(screen.getByLabelText('Website')).toBeInvalid()
    expect(screen.getByLabelText('Website')).toHaveAccessibleDescription(
      /starting with http:\/\/ or https:\/\//
    )
    expect(saveButton()).toBeDisabled()

    await user.clear(screen.getByLabelText('Company email'))
    await user.clear(screen.getByLabelText('Website'))
    await user.type(screen.getByLabelText('Website'), 'https://acme.test')

    expect(screen.getByLabelText('Website')).toBeValid()
    expect(saveButton()).toBeEnabled()
  })
})

describe('AdminSiteInfoSettings — saving', () => {
  it('sends every value trimmed, blanks as empty strings, with expected_version 0 on a first save', async () => {
    const user = userEvent.setup()
    service.updateSiteInfo.mockResolvedValue({
      ...configured,
      values: { ...empty, company_name: 'Acme Inc.' },
      version: 1,
    })
    render(<AdminSiteInfoSettings />)

    await user.type(
      await screen.findByLabelText('Company name'),
      '  Acme Inc. '
    )
    await user.click(saveButton())

    await waitFor(() => {
      expect(service.updateSiteInfo).toHaveBeenCalledWith({
        ...empty,
        company_name: 'Acme Inc.',
        expected_version: 0,
      })
    })
    expect(toast.success).toHaveBeenCalledWith('Instance site info saved')
    expect(
      await screen.findByTestId('instance-settings-source')
    ).toHaveTextContent('Configured')
    expect(savedEvents).toBe(1)
  })

  it('sends the loaded version, and clearing a field sends an empty string', async () => {
    const user = userEvent.setup()
    service.getSiteInfo.mockResolvedValue(configured)
    service.updateSiteInfo.mockResolvedValue({ ...configured, version: 5 })
    render(<AdminSiteInfoSettings />)

    await user.clear(await screen.findByLabelText('Company email'))
    await user.click(saveButton())

    await waitFor(() => {
      expect(service.updateSiteInfo).toHaveBeenCalledWith({
        ...configured.values,
        company_email: '',
        expected_version: 4,
      })
    })
  })

  it('on a version conflict, saves nothing, keeps the edits and offers a reload', async () => {
    const user = userEvent.setup()
    service.getSiteInfo.mockResolvedValue(configured)
    service.updateSiteInfo.mockRejectedValue(
      apiError(409, 'INSTANCE_SETTINGS_VERSION_CONFLICT')
    )
    render(<AdminSiteInfoSettings />)

    const name = await screen.findByLabelText('Company name')
    await user.clear(name)
    await user.type(name, 'Acme Ltd.')
    await user.click(saveButton())

    const alert = await screen.findByTestId('instance-settings-conflict')
    expect(name).toHaveValue('Acme Ltd.')
    expect(saveButton()).toBeDisabled()
    expect(savedEvents).toBe(0)
    expect(toast.success).not.toHaveBeenCalled()

    service.getSiteInfo.mockResolvedValue({
      ...configured,
      values: { ...configured.values, company_name: 'Acme GmbH' },
      version: 5,
    })
    await user.click(within(alert).getByRole('button', { name: 'Reload' }))

    await waitFor(() => {
      expect(screen.getByLabelText('Company name')).toHaveValue('Acme GmbH')
    })
    expect(
      screen.queryByTestId('instance-settings-conflict')
    ).not.toBeInTheDocument()
  })

  it('puts a server field error on its field, and keeps it until that field is edited', async () => {
    const user = userEvent.setup()
    service.updateSiteInfo.mockRejectedValue(
      apiError(400, 'VALIDATION_ERROR', [
        {
          field: 'website',
          message: 'website must be an absolute http or https URL',
        },
      ])
    )
    render(<AdminSiteInfoSettings />)

    const website = await screen.findByLabelText('Website')
    await user.type(website, 'https://acme.test')
    await user.click(saveButton())

    await waitFor(() => {
      expect(website).toHaveAccessibleDescription(
        /website must be an absolute http or https URL/
      )
    })
    expect(savedEvents).toBe(0)

    await user.type(website, '/')
    expect(website).toBeValid()
  })
})

describe('AdminSiteInfoSettings — clearing', () => {
  it('clears after a confirmation that names the consequence, then reloads', async () => {
    const user = userEvent.setup()
    service.getSiteInfo.mockResolvedValueOnce(configured)
    service.resetSiteInfo.mockResolvedValue(undefined)
    render(<AdminSiteInfoSettings />)

    await user.click(
      await screen.findByRole('button', { name: 'Clear site info' })
    )
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent('Clear the site info?')
    expect(dialog).toHaveTextContent(/shows no company branding/i)
    expect(service.resetSiteInfo).not.toHaveBeenCalled()

    await user.click(
      within(dialog).getByRole('button', { name: 'Clear site info' })
    )

    await waitFor(() => {
      expect(service.resetSiteInfo).toHaveBeenCalledTimes(1)
    })
    expect(toast.success).toHaveBeenCalledWith('Site info cleared')
    await waitFor(() => {
      expect(screen.getByLabelText('Company name')).toHaveValue('')
    })
    expect(savedEvents).toBe(1)
  })

  it('reports a failed clear and fires no saved event', async () => {
    const user = userEvent.setup()
    service.getSiteInfo.mockResolvedValue(configured)
    service.resetSiteInfo.mockRejectedValue(new Error('boom'))
    render(<AdminSiteInfoSettings />)

    await user.click(
      await screen.findByRole('button', { name: 'Clear site info' })
    )
    const dialog = await screen.findByRole('alertdialog')
    await user.click(
      within(dialog).getByRole('button', { name: 'Clear site info' })
    )

    expect(
      await screen.findByTestId('instance-settings-error')
    ).toHaveTextContent('boom')
    expect(savedEvents).toBe(0)
  })
})

describe('AdminSiteInfoSettings — history', () => {
  it('lists the changed fields and labels a delete as removed, not as a reset to defaults', async () => {
    service.listSiteInfoAudit.mockResolvedValue({
      entries: [
        {
          id: 'e2',
          setting: 'site_info',
          action: 'delete',
          actor_user_id: 'u1',
          actor_name: 'Ada Admin',
          before: { ...configured.values, version: 2 },
          after: null,
          created_at: '2026-10-10T11:00:00Z',
        },
        {
          id: 'e1',
          setting: 'site_info',
          action: 'upsert',
          actor_user_id: 'u1',
          actor_name: 'Ada Admin',
          before: { ...configured.values, company_name: 'Acme', version: 1 },
          after: { ...configured.values, version: 2 },
          created_at: '2026-10-10T10:00:00Z',
        },
      ],
      next_cursor: null,
    })
    render(<AdminSiteInfoSettings />)

    const entries = await screen.findAllByTestId(
      'instance-site-info-audit-entry'
    )
    expect(entries).toHaveLength(2)
    expect(entries[0]).toHaveTextContent('Removed')
    expect(entries[0]).not.toHaveTextContent(/reset to defaults/i)
    expect(entries[1]).toHaveTextContent('Saved')
    expect(entries[1]).toHaveTextContent(/Company name\s*Acme\s*→\s*Acme Inc\./)
    expect(entries[1]).not.toHaveTextContent('version')
  })
})
