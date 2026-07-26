import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router-dom'

import type {
  GitHubInstallationStatus,
  GitHubRepository,
} from '@/services/githubIntegrationService'
import { ApiError } from '@/types/errors'

// The shared lucide mock misses a few icons used only here (e.g. FolderGit2);
// wrap it in a Proxy that synthesizes any missing icon on the fly.
jest.mock('lucide-react', () => {
  const actual = jest.requireActual<Record<string, unknown>>('lucide-react')
  const React = jest.requireActual<typeof import('react')>('react')
  return new Proxy(actual, {
    get(target, prop) {
      if (typeof prop !== 'string' || prop in target) {
        return target[prop as string]
      }
      const MockIcon = (props: React.SVGProps<SVGSVGElement>) =>
        React.createElement('svg', {
          'data-testid': `${prop.toLowerCase()}-icon`,
          ...props,
        })
      MockIcon.displayName = prop
      return MockIcon
    },
  })
})

// Radix Select (RepositoryFilters) can loop in JSDOM — replace with plain divs.
jest.mock('@/components/ui/select', () => ({
  Select: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select">{children}</div>
  ),
  SelectTrigger: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select-trigger">{children}</div>
  ),
  SelectValue: ({ placeholder }: { placeholder?: string }) => (
    <span>{placeholder}</span>
  ),
  SelectContent: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select-content">{children}</div>
  ),
  SelectItem: ({
    children,
    value,
  }: {
    children: React.ReactNode
    value: string
  }) => <div data-value={value}>{children}</div>,
}))

jest.mock('@/services/githubIntegrationService', () => ({
  githubIntegrationService: {
    getStatus: jest.fn(),
    getInstallUrl: jest.fn(),
    handleCallback: jest.fn(),
    getRepositories: jest.fn(),
    disconnect: jest.fn(),
    importProject: jest.fn(),
    importBlueprints: jest.fn(),
  },
}))

// The page now reads the team's own GitHub App (#484). Default it to a
// configured team so the pre-existing install/repository tests keep exercising
// what they were written for; the precondition tests below override it.
jest.mock('@/services/githubAppConfigService', () => ({
  ...jest.requireActual('@/services/githubAppConfigService'),
  githubAppConfigService: {
    getAppConfig: jest.fn(),
    createAppConfig: jest.fn(),
    updateAppConfig: jest.fn(),
    deleteAppConfig: jest.fn(),
    validateAppConfig: jest.fn(),
    rotateWebhookToken: jest.fn(),
  },
}))

jest.mock('@/hooks/usePermissions', () => ({
  usePermissions: () => ({
    can: jest.fn(() => true),
    canDeleteResource: jest.fn(() => true),
    canDeleteFeedContent: jest.fn(() => true),
  }),
}))

jest.mock('@/contexts/TeamContext', () => {
  const currentTeam = { id: 'team-1', name: 'Test Team' }
  return {
    useTeam: () => ({ currentTeam, teams: [currentTeam], isLoading: false }),
  }
})

jest.mock('@/hooks', () => {
  const trackEvent = jest.fn()
  const showSuccess = jest.fn()
  const showError = jest.fn()
  return {
    useAnalytics: () => ({ trackEvent }),
    useAlerts: () => ({ showSuccess, showError }),
  }
})

jest.mock('@/hooks/useErrorHandler', () => {
  const handleError = jest.fn(() => ({}))
  return {
    useErrorHandler: () => ({ handleError }),
  }
})

jest.mock('@/lib/toast', () => ({
  toast: {
    success: jest.fn(),
    error: jest.fn(),
    info: jest.fn(),
    warning: jest.fn(),
    message: jest.fn(),
  },
}))

// The install CTA redirects the browser via safeRedirect (allowlisted to
// github.com); mock it so jsdom never attempts real navigation and the
// mechanism itself is assertable.
jest.mock('@/utils/urlValidation', () => ({
  safeRedirect: jest.fn(),
}))

import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toast } from '@/lib/toast'
import { githubAppConfigService } from '@/services/githubAppConfigService'
import { githubIntegrationService } from '@/services/githubIntegrationService'
import type { Team } from '@/services/teamService'
import { safeRedirect } from '@/utils/urlValidation'

import { GitHubIntegration } from '../GitHubIntegration'

const { handleError } = useErrorHandler()

const notInstalled: GitHubInstallationStatus = { installed: false }

const appConfigured = {
  id: 'cfg-1',
  team_id: 'team-1',
  app_id: '123456',
  app_slug: 'acme-app',
  client_id: 'Iv1.abc',
  has_private_key: true,
  has_client_secret: true,
  has_webhook_secret: true,
  webhook_url: 'https://vibexp.example.com/api/v1/webhooks/github/tok',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  version: 1,
}

const mockedGetAppConfig = githubAppConfigService.getAppConfig as jest.Mock

const installed: GitHubInstallationStatus = {
  installed: true,
  suspended: false,
  account_login: 'my-org',
  installation_id: 12345678,
  installed_at: '2026-01-15T10:30:00Z',
}

function buildRepo(
  overrides: Partial<GitHubRepository> = {}
): GitHubRepository {
  return {
    id: 1,
    name: 'awesome-repo',
    full_name: 'my-org/awesome-repo',
    description: 'An awesome repo',
    private: false,
    html_url: 'https://github.com/my-org/awesome-repo',
    owner: { login: 'my-org', type: 'Organization' },
    ...overrides,
  }
}

// The team TeamScopeLayout resolved from the URL (#584). Deliberately NOT the
// same id as the ambient team the TeamContext mock reports, so any code path
// that reaches for `useTeam()` again shows up as a wrong-id assertion failure.
const urlTeam = { id: 'team-1', name: 'Test Team' } as unknown as Team

function renderPage(
  initialEntry = '/settings/integrations/github',
  team: Team = urlTeam
) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <GitHubIntegration team={team} />
    </MemoryRouter>
  )
}

/** Renders the page with a probe that exposes the current URL. */
function renderPageWithLocation(initialEntry: string) {
  const LocationProbe = () => {
    const location = useLocation()
    return (
      <div data-testid="location">{`${location.pathname}${location.search}`}</div>
    )
  }
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <GitHubIntegration team={urlTeam} />
      <LocationProbe />
    </MemoryRouter>
  )
}

beforeEach(() => {
  jest.clearAllMocks()
  ;(githubIntegrationService.getStatus as jest.Mock).mockResolvedValue(
    notInstalled
  )
  ;(githubIntegrationService.getRepositories as jest.Mock).mockResolvedValue({
    repositories: [],
    total_count: 0,
  })
  mockedGetAppConfig.mockResolvedValue(appConfigured)
})

/**
 * The precondition #484 adds: installing needs an App to install. Without one
 * the install URL has no slug to point at and the callback 409s, so offering
 * "Connect GitHub" to an unconfigured team is an action guaranteed to fail.
 */
describe('GitHubIntegration — GitHub App precondition', () => {
  const notConfigured = () =>
    new ApiError({
      status: 409,
      code: 'GITHUB_APP_NOT_CONFIGURED',
      title: 'GitHub App Not Configured',
      detail: 'This team has no GitHub App configured',
      request_id: 'req-1',
      type: 'about:blank',
    } as ConstructorParameters<typeof ApiError>[0])

  it('hides the install button and shows the setup guide when no App is configured', async () => {
    mockedGetAppConfig.mockRejectedValue(notConfigured())

    render(
      <MemoryRouter>
        <GitHubIntegration team={urlTeam} />
      </MemoryRouter>
    )

    expect(await screen.findByText('Connect a GitHub App')).toBeVisible()
    await waitFor(() => {
      expect(
        screen.queryByRole('button', { name: /Connect GitHub/i })
      ).toBeNull()
    })
  })

  it('does not report the 409 as an error — it is the empty state', async () => {
    mockedGetAppConfig.mockRejectedValue(notConfigured())

    render(
      <MemoryRouter>
        <GitHubIntegration team={urlTeam} />
      </MemoryRouter>
    )

    await screen.findByText('Connect a GitHub App')
    expect(handleError).not.toHaveBeenCalledWith(
      expect.anything(),
      'Failed to load the GitHub App configuration'
    )
  })

  it('offers the install button once an App is configured', async () => {
    render(
      <MemoryRouter>
        <GitHubIntegration team={urlTeam} />
      </MemoryRouter>
    )

    expect(
      await screen.findByRole('button', { name: /Connect GitHub/i })
    ).toBeVisible()
  })
})

describe('GitHubIntegration — not installed', () => {
  it('shows the not-connected card and the Connect CTA, without fetching repos', async () => {
    renderPage()

    expect(
      await screen.findByText(
        'No GitHub account connected to this team workspace.'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /connect github/i })
    ).toBeInTheDocument()
    expect(githubIntegrationService.getStatus).toHaveBeenCalledWith('team-1')
    expect(githubIntegrationService.getRepositories).not.toHaveBeenCalled()
  })

  it('launching the install fetches the install URL and redirects via safeRedirect pinned to github.com', async () => {
    ;(githubIntegrationService.getInstallUrl as jest.Mock).mockResolvedValue({
      install_url:
        'https://github.com/apps/vibexp-app/installations/new?state=team-1%3Asig',
    })

    renderPage()
    await screen.findByText(
      'No GitHub account connected to this team workspace.'
    )

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /connect github/i }))

    // Install modal opens with the step-by-step instructions.
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Connect GitHub')).toBeInTheDocument()

    await user.click(
      within(dialog).getByRole('button', { name: 'Install GitHub App' })
    )

    await waitFor(() => {
      expect(githubIntegrationService.getInstallUrl).toHaveBeenCalledWith(
        'team-1'
      )
    })
    await waitFor(() => {
      expect(safeRedirect).toHaveBeenCalledWith(
        'https://github.com/apps/vibexp-app/installations/new?state=team-1%3Asig',
        ['github.com']
      )
    })
  })

  it('reports an install-URL failure and re-enables the launch button', async () => {
    ;(githubIntegrationService.getInstallUrl as jest.Mock).mockRejectedValue(
      new Error('boom')
    )

    renderPage()
    await screen.findByText(
      'No GitHub account connected to this team workspace.'
    )

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /connect github/i }))
    const dialog = await screen.findByRole('dialog')
    await user.click(
      within(dialog).getByRole('button', { name: 'Install GitHub App' })
    )

    await waitFor(() => {
      expect(handleError).toHaveBeenCalledWith(
        expect.any(Error),
        'Failed to get GitHub install URL'
      )
    })
    expect(safeRedirect).not.toHaveBeenCalled()
    expect(
      within(dialog).getByRole('button', { name: 'Install GitHub App' })
    ).toBeEnabled()
  })

  it('reports a status load failure', async () => {
    ;(githubIntegrationService.getStatus as jest.Mock).mockRejectedValue(
      new Error('boom')
    )

    renderPage()

    await waitFor(() => {
      expect(handleError).toHaveBeenCalledWith(
        expect.any(Error),
        'Failed to load GitHub integration status'
      )
    })
  })
})

describe('GitHubIntegration — installed', () => {
  beforeEach(() => {
    ;(githubIntegrationService.getStatus as jest.Mock).mockResolvedValue(
      installed
    )
  })

  it('shows the connected account and lists repositories from page 1', async () => {
    ;(githubIntegrationService.getRepositories as jest.Mock).mockResolvedValue({
      repositories: [
        buildRepo(),
        buildRepo({
          id: 2,
          name: 'private-repo',
          full_name: 'my-org/private-repo',
          private: true,
          description: null,
        }),
      ],
      total_count: 2,
    })

    renderPage()

    expect(await screen.findByText('my-org')).toBeInTheDocument()
    expect(screen.getByText('Accessible Repositories')).toBeInTheDocument()
    // No Connect CTA when installed.
    expect(
      screen.queryByRole('button', { name: /connect github/i })
    ).not.toBeInTheDocument()

    await waitFor(() => {
      expect(githubIntegrationService.getRepositories).toHaveBeenCalledWith(
        'team-1',
        1,
        expect.anything()
      )
    })

    expect(await screen.findByText('awesome-repo')).toBeInTheDocument()
    expect(screen.getByText('private-repo')).toBeInTheDocument()
    expect(screen.getByText('Private')).toBeInTheDocument()
    expect(screen.getByText('No description')).toBeInTheDocument()
    expect(screen.getByText(/Showing 2 of 2 repositories/)).toBeInTheDocument()
  })

  it('expanding the connection card reveals installation details', async () => {
    renderPage()

    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { expanded: false }))

    expect(screen.getByText('Installation ID')).toBeInTheDocument()
    expect(screen.getByText('12345678')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Disconnect GitHub' })
    ).toBeInTheDocument()
  })

  it('shows the empty repositories state when the installation has none', async () => {
    renderPage()

    expect(await screen.findByText('No repositories found')).toBeInTheDocument()
  })

  it('reports a repositories load failure', async () => {
    ;(githubIntegrationService.getRepositories as jest.Mock).mockRejectedValue(
      new Error('boom')
    )

    renderPage()

    await waitFor(() => {
      expect(handleError).toHaveBeenCalledWith(
        expect.any(Error),
        'Failed to load repositories'
      )
    })
  })

  it('loads the next server page via Load More and appends the results', async () => {
    const firstPage = Array.from({ length: 3 }, (_, i) =>
      buildRepo({ id: i + 1, name: `repo-${String(i + 1)}` })
    )
    ;(githubIntegrationService.getRepositories as jest.Mock)
      .mockResolvedValueOnce({ repositories: firstPage, total_count: 150 })
      .mockResolvedValueOnce({
        repositories: [buildRepo({ id: 200, name: 'aaa-appended' })],
        total_count: 150,
      })

    renderPage()

    expect(await screen.findByText('repo-1')).toBeInTheDocument()

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Load More' }))

    await waitFor(() => {
      expect(githubIntegrationService.getRepositories).toHaveBeenCalledWith(
        'team-1',
        2,
        expect.anything()
      )
    })
    expect(await screen.findByText('aaa-appended')).toBeInTheDocument()
    // 2 * 100 loaded >= 150 total — no more pages.
    expect(
      screen.queryByRole('button', { name: 'Load More' })
    ).not.toBeInTheDocument()
  })

  it('disconnects after confirmation, resets state, and walks through the uninstall step', async () => {
    ;(githubIntegrationService.getRepositories as jest.Mock).mockResolvedValue({
      repositories: [buildRepo()],
      total_count: 1,
    })
    ;(githubIntegrationService.disconnect as jest.Mock).mockResolvedValue(
      undefined
    )

    renderPage()
    await screen.findByText('awesome-repo')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { expanded: false }))
    await user.click(screen.getByRole('button', { name: 'Disconnect GitHub' }))

    const confirm = await screen.findByRole('alertdialog')
    expect(
      within(confirm).getByText('Disconnect GitHub Integration')
    ).toBeInTheDocument()
    await user.click(
      within(confirm).getByRole('button', { name: 'Disconnect' })
    )

    await waitFor(() => {
      expect(githubIntegrationService.disconnect).toHaveBeenCalledWith('team-1')
    })
    expect(toast.success).toHaveBeenCalledWith(
      'GitHub integration disconnected successfully'
    )

    // The uninstall step points at the concrete GitHub installation, and the
    // Organization owner surfaces the org-admin caveat.
    const step = await screen.findByRole('dialog')
    expect(
      within(step).getByText('GitHub disconnected — one more step')
    ).toBeInTheDocument()
    expect(
      within(step).getByRole('link', { name: /uninstall from github/i })
    ).toHaveAttribute(
      'href',
      'https://github.com/settings/installations/12345678'
    )
    expect(
      within(step).getByText('Organization installation')
    ).toBeInTheDocument()
  })

  it('shows the suspended alert and skips repository loading', async () => {
    ;(githubIntegrationService.getStatus as jest.Mock).mockResolvedValue({
      ...installed,
      suspended: true,
    })

    renderPage()

    expect(
      await screen.findByText('GitHub Integration Suspended')
    ).toBeInTheDocument()
    expect(
      screen.queryByText('Accessible Repositories')
    ).not.toBeInTheDocument()
    expect(githubIntegrationService.getRepositories).not.toHaveBeenCalled()
  })
})

describe('GitHubIntegration — install callback via URL params', () => {
  const callbackUrl =
    '/settings/integrations/github?installation_id=12345678&setup_action=install&state=csrf-state&code=gh-oauth-code'

  it('posts the callback from ?installation_id/setup_action/state/code and toasts on a new connection', async () => {
    ;(githubIntegrationService.handleCallback as jest.Mock).mockResolvedValue({
      reconnected: false,
    })

    renderPage(callbackUrl)

    await waitFor(() => {
      expect(githubIntegrationService.handleCallback).toHaveBeenCalledWith(
        'team-1',
        {
          installation_id: 12345678,
          setup_action: 'install',
          state: 'csrf-state',
          code: 'gh-oauth-code',
        }
      )
    })
    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith(
        'GitHub integration connected successfully'
      )
    })
    // Status is refreshed after a successful callback.
    await waitFor(() => {
      expect(
        (githubIntegrationService.getStatus as jest.Mock).mock.calls.length
      ).toBeGreaterThan(1)
    })
  })

  it('toasts the reconnect variant when the callback reports reconnected', async () => {
    ;(githubIntegrationService.handleCallback as jest.Mock).mockResolvedValue({
      reconnected: true,
    })

    renderPage(callbackUrl)

    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith(
        'Reconnected to existing GitHub installation'
      )
    })
  })

  it('explains a missing code instead of firing a doomed request', async () => {
    // The code is required by the callback since #463 (it is exchanged for a
    // user token proving the caller can access the installation), so posting
    // without it is a guaranteed 400. Say what to do rather than relay the
    // server's rejection of a request we knew would fail.
    renderPage(
      '/settings/integrations/github?installation_id=12345678&setup_action=install&state=csrf-state'
    )

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        expect.stringContaining('Start the install again')
      )
    })
    expect(githubIntegrationService.handleCallback).not.toHaveBeenCalled()
  })

  it('never writes the code to the console', async () => {
    const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {})
    const consoleLog = jest.spyOn(console, 'log').mockImplementation(() => {})
    ;(githubIntegrationService.handleCallback as jest.Mock).mockRejectedValue(
      new Error('boom')
    )

    renderPage(callbackUrl)

    await waitFor(() => {
      expect(githubIntegrationService.handleCallback).toHaveBeenCalled()
    })
    // The code is a short-lived single-use credential; it must not be logged,
    // including on the failure path where a payload dump is most tempting.
    for (const spy of [consoleSpy, consoleLog]) {
      for (const call of spy.mock.calls) {
        expect(JSON.stringify(call)).not.toContain('gh-oauth-code')
      }
    }
    consoleSpy.mockRestore()
    consoleLog.mockRestore()
  })

  it('rejects a non-numeric installation_id without calling the service', async () => {
    renderPage(
      '/settings/integrations/github?installation_id=not-a-number&setup_action=install&state=csrf-state&code=gh-oauth-code'
    )

    await waitFor(() => {
      expect(handleError).toHaveBeenCalledWith(
        expect.any(Error),
        'Failed to complete GitHub installation'
      )
    })
    expect(githubIntegrationService.handleCallback).not.toHaveBeenCalled()
  })

  it('surfaces the already-connected-to-another-team message for that API error code', async () => {
    ;(githubIntegrationService.handleCallback as jest.Mock).mockRejectedValue(
      new ApiError({
        type: 'about:blank',
        title: 'Conflict',
        detail: 'Installation already connected',
        status: 409,
        code: 'installation_already_connected',
        request_id: 'req-1',
        timestamp: '2026-01-15T10:30:00Z',
      })
    )

    renderPage(callbackUrl)

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        expect.stringContaining('disconnect it there first')
      )
    })
  })

  // A consumed authorization code is single-use: leaving it in the URL means a
  // refresh replays it and the admin sees a failure for something that already
  // succeeded.
  it.each([
    ['success', false],
    ['failure', true],
  ])(
    'strips the callback params from the URL after %s',
    async (_name, fails) => {
      const mock = githubIntegrationService.handleCallback as jest.Mock
      if (fails) mock.mockRejectedValue(new Error('boom'))
      else mock.mockResolvedValue({ reconnected: false })

      renderPageWithLocation(callbackUrl)

      await waitFor(() => {
        expect(githubIntegrationService.handleCallback).toHaveBeenCalled()
      })
      await waitFor(() => {
        expect(screen.getByTestId('location').textContent).toBe(
          '/settings/integrations/github'
        )
      })
    }
  )

  it('strips the params wherever the page is mounted, not at a fixed path', async () => {
    // #541 relocates this page to /teams/:id/settings/integrations/github. The
    // redirect derives from the current pathname precisely so that move does not
    // silently start redirecting into a 404 — this test is what would catch it.
    ;(githubIntegrationService.handleCallback as jest.Mock).mockResolvedValue({
      reconnected: false,
    })
    const future =
      '/teams/team-1/settings/integrations/github?installation_id=12345678&setup_action=install&state=csrf-state&code=gh-oauth-code'

    renderPageWithLocation(future)

    await waitFor(() => {
      expect(githubIntegrationService.handleCallback).toHaveBeenCalled()
    })
    await waitFor(() => {
      expect(screen.getByTestId('location').textContent).toBe(
        '/teams/team-1/settings/integrations/github'
      )
    })
  })

  it('reports a generic callback failure with the generic message', async () => {
    ;(githubIntegrationService.handleCallback as jest.Mock).mockRejectedValue(
      new Error('boom')
    )

    renderPage(callbackUrl)

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        expect.stringContaining('Please try again')
      )
    })
  })

  // Every documented arm gets its own instruction; a shared "installation
  // failed" leaves the admin with nothing to do but retry, which never helps
  // for any of these (#485).
  it.each([
    [
      'installation_not_authorized',
      403,
      /cannot administer this installation/i,
    ],
    [
      'GITHUB_APP_NOT_CONFIGURED',
      409,
      /Register one under the team’s Settings/i,
    ],
    ['github_user_auth_not_configured', 503, /Client ID and secret/i],
  ])('maps %s to its own actionable message', async (code, status, match) => {
    ;(githubIntegrationService.handleCallback as jest.Mock).mockRejectedValue(
      new ApiError({
        type: 'about:blank',
        title: 'Error',
        detail: 'detail',
        status,
        code,
        request_id: 'req-1',
        timestamp: '2026-01-15T10:30:00Z',
      })
    )

    renderPage(callbackUrl)

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(expect.stringMatching(match))
    })
  })
})

// ---------------------------------------------------------------------------
// #584 — the install callback must POST to the URL's team
// ---------------------------------------------------------------------------
// This is the failure that made #584 more than cosmetic. The callback fires
// from a mount effect, and React runs child effects before parent effects, so
// on a cold deep-link back from GitHub the ambient team was still whichever one
// was last persisted. The POST went to that team, the backend rejected it 403
// ("State parameter does not match team"), and the catch arm called
// clearCallbackParams() — destroying the params before the ambient team caught
// up. The install could not complete and had to be restarted from scratch.
//
// The TeamContext mock above reports team-1 as the ambient team, so rendering
// with an explicit team-b prop reproduces exactly that divergence.
describe('GitHubIntegration — install callback targets the URL team (#584)', () => {
  const urlTeamB = { id: 'team-b', name: 'Team B' } as unknown as Team
  const callbackUrl =
    '/settings/integrations/github?installation_id=12345678&setup_action=install&state=csrf-state&code=gh-oauth-code'

  it('posts to the URL team, never the ambient one', async () => {
    ;(githubIntegrationService.handleCallback as jest.Mock).mockResolvedValue({
      reconnected: false,
    })

    renderPage(callbackUrl, urlTeamB)

    await waitFor(() => {
      expect(githubIntegrationService.handleCallback).toHaveBeenCalledWith(
        'team-b',
        {
          installation_id: 12345678,
          setup_action: 'install',
          state: 'csrf-state',
          code: 'gh-oauth-code',
        }
      )
    })
    expect(githubIntegrationService.handleCallback).not.toHaveBeenCalledWith(
      'team-1',
      expect.anything()
    )
  })

  it('reads status, repositories and the install URL for the URL team too', async () => {
    ;(githubIntegrationService.getStatus as jest.Mock).mockResolvedValue({
      installed: true,
      suspended: false,
    })

    renderPage('/settings/integrations/github', urlTeamB)

    await waitFor(() => {
      expect(githubIntegrationService.getStatus).toHaveBeenCalledWith('team-b')
    })
    expect(githubIntegrationService.getStatus).not.toHaveBeenCalledWith(
      'team-1'
    )
    await waitFor(() => {
      expect(githubIntegrationService.getRepositories).toHaveBeenCalledWith(
        'team-b',
        1,
        expect.anything()
      )
    })
  })
})
