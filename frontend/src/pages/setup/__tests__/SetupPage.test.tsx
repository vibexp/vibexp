import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router'

import { authService } from '@/services/authService'
import { authSettingsService } from '@/services/authSettingsService'
import { setupService } from '@/services/setupService'
import { ApiError } from '@/types/errors'

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('@/services/setupService', () => ({
  setupService: { createSession: vi.fn(), getStatus: vi.fn() },
}))

vi.mock('@/services/authService', () => ({
  authService: { getLoginUrl: vi.fn() },
}))

vi.mock('@/services/authSettingsService', () => ({
  authSettingsService: {
    listProviders: vi.fn(),
    getAllowlist: vi.fn(),
    listAdmins: vi.fn(),
    listAudit: vi.fn(),
  },
}))

import {
  allowlist,
  google,
  provider,
} from '@/pages/admin/settings/auth/__tests__/fixtures'

import { SetupFinishPrompt, SetupPage } from '../SetupPage'

const setup = vi.mocked(setupService)
const settings = vi.mocked(authSettingsService)
const auth = vi.mocked(authService)

const problem = (status: number, code: string) =>
  new ApiError({
    type: 'about:blank',
    title: 'Error',
    status,
    detail: `${code} detail`,
    code,
    request_id: 'req-1',
    timestamp: '2026-10-10T00:00:00Z',
  })

function Address() {
  const location = useLocation()
  return <p data-testid="address">{location.pathname + location.search}</p>
}

function renderAt(url: string) {
  render(
    <MemoryRouter initialEntries={[url]}>
      <SetupPage />
      <Address />
    </MemoryRouter>
  )
}

const assign = vi.fn()

beforeEach(() => {
  vi.clearAllMocks()
  // Only what the page reads: the origin (the derived redirect URI) and the
  // navigation it performs.
  vi.stubGlobal('location', { origin: 'http://localhost:3000', assign })
  settings.listProviders.mockResolvedValue({ providers: [], version: 1 })
  settings.getAllowlist.mockResolvedValue(allowlist())
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('SetupPage', () => {
  it('exchanges the token, strips it from the URL and shows only providers + allowlist', async () => {
    setup.createSession.mockResolvedValue({
      expires_at: '2026-10-10T13:00:00Z',
    })
    renderAt('/setup?token=tok-123')

    expect(await screen.findByTestId('auth-providers-section')).toBeVisible()
    expect(await screen.findByTestId('auth-allowlist-section')).toBeVisible()
    expect(setup.createSession).toHaveBeenCalledWith('tok-123')
    expect(screen.getByTestId('address')).toHaveTextContent(/^\/setup$/)
    // Stripping the token changes the URL; it must not exchange it again.
    expect(setup.createSession).toHaveBeenCalledTimes(1)

    // A setup session cannot reach the admins or the audit log (404), so the
    // page neither renders nor requests them.
    expect(screen.queryByTestId('auth-admins-section')).toBeNull()
    expect(screen.queryByTestId('auth-audit-section')).toBeNull()
    expect(settings.listAdmins).not.toHaveBeenCalled()
    expect(settings.listAudit).not.toHaveBeenCalled()

    // Nothing is enabled yet, so there is nothing to sign in through.
    expect(screen.getByTestId('setup-next-step')).toBeVisible()
    expect(screen.queryByTestId('setup-sign-in')).toBeNull()
  })

  it('does not call the settings API before the exchange succeeds', async () => {
    let resolve: (value: { expires_at: string }) => void = () => {}
    setup.createSession.mockReturnValue(
      new Promise(done => {
        resolve = done
      })
    )
    renderAt('/setup?token=tok-123')
    expect(screen.getByTestId('setup-loading')).toBeVisible()
    expect(settings.listProviders).not.toHaveBeenCalled()

    resolve({ expires_at: '2026-10-10T13:00:00Z' })
    await screen.findByTestId('auth-providers-section')
    expect(settings.listProviders).toHaveBeenCalledTimes(1)
  })

  it('prompts to sign in as a root admin once a provider is enabled', async () => {
    setup.createSession.mockResolvedValue({
      expires_at: '2026-10-10T13:00:00Z',
    })
    settings.listProviders.mockResolvedValue({
      providers: [provider()],
      version: 2,
    })
    auth.getLoginUrl.mockResolvedValue('https://example.okta.com/authorize?x=1')
    const user = userEvent.setup()
    renderAt('/setup?token=tok-123')

    const button = await screen.findByRole('button', {
      name: 'Sign in with Okta as a root admin to finish setup',
    })
    await user.click(button)
    await waitFor(() => {
      expect(assign).toHaveBeenCalledWith(
        'https://example.okta.com/authorize?x=1'
      )
    })
    expect(auth.getLoginUrl).toHaveBeenCalledWith('okta')
  })

  it('renders a terminal state for an invalid or expired token (401)', async () => {
    setup.createSession.mockRejectedValue(problem(401, 'AUTH_INVALID'))
    renderAt('/setup?token=stale')
    expect(await screen.findByTestId('setup-invalid')).toHaveTextContent(
      'This setup link is no longer valid'
    )
    expect(screen.getByTestId('setup-invalid')).toHaveTextContent(
      'vibexp admin auth setup rearm'
    )
    expect(screen.queryByTestId('auth-providers-section')).toBeNull()
    expect(settings.listProviders).not.toHaveBeenCalled()
  })

  it('renders a terminal state when the instance is not in setup mode (404)', async () => {
    setup.createSession.mockRejectedValue(problem(404, 'RESOURCE_NOT_FOUND'))
    renderAt('/setup?token=tok-123')
    expect(await screen.findByTestId('setup-inactive')).toHaveTextContent(
      'This instance is already set up'
    )
    expect(screen.queryByTestId('auth-providers-section')).toBeNull()
  })

  it('reports any other exchange failure', async () => {
    setup.createSession.mockRejectedValue(new Error('Network error'))
    renderAt('/setup?token=tok-123')
    expect(await screen.findByTestId('setup-failed')).toHaveTextContent(
      'Network error'
    )
  })

  it('resumes on the setup cookie when reloaded without the token', async () => {
    renderAt('/setup')
    expect(await screen.findByTestId('auth-providers-section')).toBeVisible()
    expect(setup.createSession).not.toHaveBeenCalled()
  })

  it('says the link is incomplete with no token and no session', async () => {
    settings.listProviders.mockRejectedValue(problem(404, 'RESOURCE_NOT_FOUND'))
    renderAt('/setup')
    expect(await screen.findByTestId('setup-missing')).toHaveTextContent(
      'This setup link is incomplete'
    )
    expect(setup.createSession).not.toHaveBeenCalled()
  })

  it('reads a 401 on the resume as a missing token too', async () => {
    settings.listProviders.mockRejectedValue(problem(401, 'AUTH_REQUIRED'))
    renderAt('/setup')
    expect(await screen.findByTestId('setup-missing')).toBeVisible()
  })

  it.each([
    ['a network error', new Error('Network error: Unable to connect')],
    ['a server error', problem(500, 'INTERNAL_ERROR')],
  ])(
    'reports %s on the resume as a failure, not a bad link',
    async (_, err) => {
      settings.listProviders.mockRejectedValue(err)
      renderAt('/setup')
      expect(await screen.findByTestId('setup-failed')).toHaveTextContent(
        err.message
      )
      expect(screen.queryByTestId('setup-missing')).toBeNull()
    }
  )
})

describe('SetupFinishPrompt', () => {
  it('offers every enabled provider that can be used, and no disabled one', () => {
    render(
      <SetupFinishPrompt
        providers={[provider(), { ...google, enabled: false }]}
      />
    )
    const buttons = screen.getAllByTestId('setup-sign-in')
    expect(buttons).toHaveLength(1)
    expect(buttons[0]).toHaveAttribute('data-provider-slug', 'okta')
  })

  it('calls out an enabled provider that failed to build', () => {
    render(
      <SetupFinishPrompt
        providers={[
          provider({
            health: {
              status: 'unhealthy',
              last_error: 'discovery failed',
              checked_at: null,
            },
          }),
        ]}
      />
    )
    expect(screen.getByTestId('setup-unhealthy')).toBeVisible()
    expect(screen.queryByTestId('setup-sign-in')).toBeNull()
  })

  it('shows a sign-in that could not start', async () => {
    auth.getLoginUrl.mockRejectedValue(new Error('provider unavailable'))
    const user = userEvent.setup()
    render(<SetupFinishPrompt providers={[provider()]} />)
    await user.click(screen.getByTestId('setup-sign-in'))
    expect(await screen.findByTestId('setup-sign-in-error')).toHaveTextContent(
      'provider unavailable'
    )
    expect(assign).not.toHaveBeenCalled()
    expect(screen.getByTestId('setup-sign-in')).toBeEnabled()
  })
})
