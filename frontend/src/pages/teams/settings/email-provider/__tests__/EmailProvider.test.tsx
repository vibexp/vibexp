import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import { toast } from '@/lib/toast'
import type {
  TeamEmailProviderResponse,
  TeamEmailProviderTestResponse,
} from '@/services/emailProviderService'
import { emailProviderService } from '@/services/emailProviderService'
import type { Team } from '@/services/teamService'

// usePermissions is deliberately NOT mocked — it reads the permissions off the
// team the page passes it, so the fixtures below exercise the real gating.
const mockUseTeam = jest.fn()

jest.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

// Stable handleError reference (like the real useCallback-backed hook) so the
// load callback isn't recreated every render — an unstable one loops the mount
// effect and remounts the page under test.
jest.mock('@/hooks/useErrorHandler', () => {
  const handleError = jest.fn()
  return { useErrorHandler: () => ({ handleError }) }
})

jest.mock('@/lib/toast', () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}))

jest.mock('@/services/emailProviderService', () => ({
  emailProviderService: {
    getEmailProvider: jest.fn(),
    upsertEmailProvider: jest.fn(),
    deleteEmailProvider: jest.fn(),
    testEmailProvider: jest.fn(),
  },
}))

import { EmailProvider } from '../EmailProvider'

const service = jest.mocked(emailProviderService)
const mockedToast = jest.mocked(toast)

const inheriting: TeamEmailProviderResponse = {
  configured: false,
  source: 'instance',
  effective_from_address: 'noreply@instance.test',
  provider_type: null,
  has_credential: false,
}

const configured = (
  overrides: Partial<TeamEmailProviderResponse> = {}
): TeamEmailProviderResponse => ({
  configured: true,
  source: 'team',
  effective_from_address: 'hello@acme.test',
  provider_type: 'mailgun',
  has_credential: true,
  from_address: 'hello@acme.test',
  from_name: 'Acme Team',
  settings: { mailgun: { domain: 'mg.acme.test' } },
  is_healthy: true,
  last_success_at: '2026-07-20T10:00:00Z',
  ...overrides,
})

// Module-stable context objects — a fresh `currentTeam` per render would loop
// any effect keyed on its identity.
const adminTeam = {
  currentTeam: {
    id: 'team-1',
    name: 'Test Team',
    permissions: ['team.update'],
  },
  teams: [{ id: 'team-1', name: 'Test Team' }],
  isLoading: false,
  setCurrentTeam: jest.fn(),
  refreshTeams: jest.fn() as () => Promise<void>,
}

const memberTeam = {
  ...adminTeam,
  currentTeam: {
    id: 'team-1',
    name: 'Test Team',
    // A member holds no team.update — gating must fail closed.
    permissions: ['resource.create'],
  },
}

const asTeam = (ctx: typeof adminTeam) => ctx.currentTeam as unknown as Team

const renderPage = (team: Team = asTeam(adminTeam)) =>
  render(
    <MemoryRouter>
      <EmailProvider team={team} />
    </MemoryRouter>
  )

beforeEach(() => {
  jest.clearAllMocks()
  mockUseTeam.mockReturnValue(adminTeam)
  service.getEmailProvider.mockResolvedValue(inheriting)
})

describe('EmailProvider — unconfigured state', () => {
  it('says the team is using the instance default and names that address', async () => {
    renderPage()

    expect(
      await screen.findByText(/using the instance default/i)
    ).toBeInTheDocument()
    expect(screen.getByText('noreply@instance.test')).toBeInTheDocument()
  })

  it('still offers an empty, editable form rather than nothing', async () => {
    renderPage()

    expect(await screen.findByLabelText(/from address/i)).toHaveValue('')
    expect(screen.getByRole('radio', { name: /smtp/i })).toBeChecked()
  })

  it('offers no revert action, since there is nothing to revert', async () => {
    renderPage()

    await screen.findByText(/using the instance default/i)
    expect(
      screen.queryByRole('button', { name: /revert to instance default/i })
    ).not.toBeInTheDocument()
  })
})

describe('EmailProvider — provider type switching', () => {
  it('shows only the selected type’s credential fields', async () => {
    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByLabelText(/^host$/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/sending domain/i)).not.toBeInTheDocument()

    await user.click(screen.getByRole('radio', { name: /mailgun/i }))

    expect(await screen.findByLabelText(/sending domain/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/^host$/i)).not.toBeInTheDocument()
  })

  it('relabels the secret field for the selected provider', async () => {
    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByLabelText(/smtp password/i)).toBeInTheDocument()

    await user.click(screen.getByRole('radio', { name: /postmark/i }))

    expect(await screen.findByLabelText(/server token/i)).toBeInTheDocument()
  })
})

describe('EmailProvider — configured state', () => {
  it('seeds the form from the stored provider but leaves the secret blank', async () => {
    service.getEmailProvider.mockResolvedValue(configured())
    renderPage()

    expect(await screen.findByLabelText(/sending domain/i)).toHaveValue(
      'mg.acme.test'
    )
    expect(screen.getByLabelText(/from address/i)).toHaveValue(
      'hello@acme.test'
    )
    expect(screen.getByLabelText(/sending key/i)).toHaveValue('')
  })

  it('masks the secret input and says a blank value keeps the stored key', async () => {
    service.getEmailProvider.mockResolvedValue(configured())
    renderPage()

    const secret = await screen.findByLabelText(/sending key/i)
    expect(secret).toHaveAttribute('type', 'password')
    expect(secret).toHaveAttribute(
      'placeholder',
      'Leave blank to keep current key'
    )
  })

  it('omits the secret from the request when it was not touched', async () => {
    // The stored credential must survive an unrelated edit; sending "" would be
    // rejected by the backend rather than treated as "keep".
    const user = userEvent.setup()
    service.getEmailProvider.mockResolvedValue(configured())
    service.upsertEmailProvider.mockResolvedValue(configured())
    renderPage()

    await screen.findByLabelText(/sending domain/i)
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => {
      expect(service.upsertEmailProvider).toHaveBeenCalled()
    })
    const body = service.upsertEmailProvider.mock.calls[0][1]
    expect(body).not.toHaveProperty('secret')
    expect(body.settings).toEqual({ mailgun: { domain: 'mg.acme.test' } })
    expect(mockedToast.success).toHaveBeenCalledWith('Email provider saved')
  })

  it('sends a newly entered secret', async () => {
    const user = userEvent.setup()
    service.getEmailProvider.mockResolvedValue(configured())
    service.upsertEmailProvider.mockResolvedValue(configured())
    renderPage()

    await user.type(await screen.findByLabelText(/sending key/i), 'key-new')
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => {
      expect(service.upsertEmailProvider).toHaveBeenCalled()
    })
    expect(service.upsertEmailProvider.mock.calls[0][1].secret).toBe('key-new')
  })

  it('requires a credential the first time a provider is configured', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.type(
      await screen.findByLabelText(/from address/i),
      'hello@acme.test'
    )
    await user.type(screen.getByLabelText(/^host$/i), 'smtp.acme.test')
    await user.type(screen.getByLabelText(/^port$/i), '587')
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    expect(
      await screen.findByText(/a credential is required/i)
    ).toBeInTheDocument()
    expect(service.upsertEmailProvider).not.toHaveBeenCalled()
  })
})

describe('EmailProvider — health banner', () => {
  it('reports the last successful delivery when healthy', async () => {
    service.getEmailProvider.mockResolvedValue(configured())
    renderPage()

    expect(
      await screen.findByText(/last delivered successfully/i)
    ).toBeInTheDocument()
  })

  it('surfaces the last error and its time when delivery is failing', async () => {
    service.getEmailProvider.mockResolvedValue(
      configured({
        is_healthy: false,
        last_error: 'smtp: connection refused',
        last_error_at: '2026-07-21T09:00:00Z',
      })
    )
    renderPage()

    expect(await screen.findByText(/last delivery failed/i)).toBeInTheDocument()
    expect(screen.getByText(/smtp: connection refused/)).toBeInTheDocument()
    expect(screen.getByText(/not falling back/i)).toBeInTheDocument()
  })

  it('does not call a recovered provider broken just because an error is retained', async () => {
    // The backend keeps last_error after recovery for diagnosis, so is_healthy
    // — not the presence of last_error — is the verdict.
    service.getEmailProvider.mockResolvedValue(
      configured({ is_healthy: true, last_error: 'an old failure' })
    )
    renderPage()

    expect(
      await screen.findByText(/last delivered successfully/i)
    ).toBeInTheDocument()
    expect(screen.queryByText(/last delivery failed/i)).not.toBeInTheDocument()
    expect(screen.getByText(/since recovered/i)).toBeInTheDocument()
  })
})

describe('EmailProvider — test send', () => {
  const okTest: TeamEmailProviderTestResponse = {
    is_valid: true,
    message: 'Test message accepted',
    recipient: 'admin@acme.test',
    details: {},
  }

  it('reports success inline without saving', async () => {
    const user = userEvent.setup()
    service.getEmailProvider.mockResolvedValue(configured())
    service.testEmailProvider.mockResolvedValue(okTest)
    renderPage()

    await user.type(await screen.findByLabelText(/sending key/i), 'key-new')
    await user.click(screen.getByRole('button', { name: /send test email/i }))

    expect(await screen.findByText(/test email sent/i)).toBeInTheDocument()
    expect(screen.getByText(/admin@acme.test/)).toBeInTheDocument()
    // Testing must never be a hidden save.
    expect(service.upsertEmailProvider).not.toHaveBeenCalled()
  })

  it('surfaces details.error_details on a failed send', async () => {
    const user = userEvent.setup()
    service.getEmailProvider.mockResolvedValue(configured())
    service.testEmailProvider.mockResolvedValue({
      is_valid: false,
      message: 'Sending failed: connection refused',
      recipient: 'admin@acme.test',
      details: { error_details: 'send_failed' },
    })
    renderPage()

    await user.type(await screen.findByLabelText(/sending key/i), 'key-new')
    await user.click(screen.getByRole('button', { name: /send test email/i }))

    expect(await screen.findByText(/test email failed/i)).toBeInTheDocument()
    expect(screen.getByText(/send_failed/)).toBeInTheDocument()
  })

  it('does not block saving after a failed test', async () => {
    const user = userEvent.setup()
    service.getEmailProvider.mockResolvedValue(configured())
    service.testEmailProvider.mockResolvedValue({
      is_valid: false,
      message: 'Sending failed',
      recipient: 'admin@acme.test',
      details: { error_details: 'send_failed' },
    })
    service.upsertEmailProvider.mockResolvedValue(configured())
    renderPage()

    await user.type(await screen.findByLabelText(/sending key/i), 'key-new')
    await user.click(screen.getByRole('button', { name: /send test email/i }))
    await screen.findByText(/test email failed/i)

    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => {
      expect(service.upsertEmailProvider).toHaveBeenCalled()
    })
  })

  it('demands the credential even on a configured provider', async () => {
    // The endpoint sends with the request body, not the stored configuration,
    // so a blank secret cannot be tested.
    const user = userEvent.setup()
    service.getEmailProvider.mockResolvedValue(configured())
    renderPage()

    await screen.findByLabelText(/sending key/i)
    await user.click(screen.getByRole('button', { name: /send test email/i }))

    expect(
      await screen.findByText(/enter the credential to send a test/i)
    ).toBeInTheDocument()
    expect(service.testEmailProvider).not.toHaveBeenCalled()
  })
})

describe('EmailProvider — revert', () => {
  it('confirms, deletes, and returns to the inheriting state', async () => {
    const user = userEvent.setup()
    service.getEmailProvider
      .mockResolvedValueOnce(configured())
      .mockResolvedValue(inheriting)
    service.deleteEmailProvider.mockResolvedValue(undefined)
    renderPage()

    await user.click(
      await screen.findByRole('button', {
        name: /revert to instance default/i,
      })
    )
    await user.click(await screen.findByRole('button', { name: /^revert$/i }))

    await waitFor(() => {
      expect(service.deleteEmailProvider).toHaveBeenCalledWith('team-1')
    })
    expect(
      await screen.findByText(/using the instance default/i)
    ).toBeInTheDocument()
  })

  it('does not delete until the confirmation is accepted', async () => {
    const user = userEvent.setup()
    service.getEmailProvider.mockResolvedValue(configured())
    renderPage()

    await user.click(
      await screen.findByRole('button', {
        name: /revert to instance default/i,
      })
    )

    expect(service.deleteEmailProvider).not.toHaveBeenCalled()
  })
})

describe('EmailProvider — permission gating', () => {
  it('renders read-only for a member, with no save, test or revert', async () => {
    mockUseTeam.mockReturnValue(memberTeam)
    service.getEmailProvider.mockResolvedValue(configured())
    renderPage(asTeam(memberTeam))

    expect(
      await screen.findByText(/only team owners and admins/i)
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /save changes/i })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /send test email/i })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /revert to instance default/i })
    ).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/sending key/i)).not.toBeInTheDocument()
  })

  it('still shows a member the effective sender, which they already see in every email', async () => {
    mockUseTeam.mockReturnValue(memberTeam)
    renderPage(asTeam(memberTeam))

    expect(
      await screen.findByText(/using the instance default/i)
    ).toBeInTheDocument()
    // Shown twice for a member — once in the status card and once in the
    // read-only summary — so assert presence rather than uniqueness.
    expect(screen.getAllByText('noreply@instance.test').length).toBeGreaterThan(
      0
    )
  })

  it('gives an owner or admin the full set of controls', async () => {
    service.getEmailProvider.mockResolvedValue(configured())
    renderPage()

    expect(
      await screen.findByRole('button', { name: /save changes/i })
    ).toBeEnabled()
    expect(
      screen.getByRole('button', { name: /send test email/i })
    ).toBeEnabled()
    expect(
      screen.getByRole('button', { name: /revert to instance default/i })
    ).toBeEnabled()
  })
})

describe('EmailProvider — load failure', () => {
  it('reports a failed load instead of rendering an empty form', async () => {
    service.getEmailProvider.mockRejectedValue(new Error('boom'))
    renderPage()

    expect(await screen.findByText('boom')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /save changes/i })
    ).not.toBeInTheDocument()
  })
})
