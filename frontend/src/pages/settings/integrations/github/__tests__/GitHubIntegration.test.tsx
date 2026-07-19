import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

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
import { githubIntegrationService } from '@/services/githubIntegrationService'
import { safeRedirect } from '@/utils/urlValidation'

import { GitHubIntegration } from '../GitHubIntegration'

const { handleError } = useErrorHandler()

const notInstalled: GitHubInstallationStatus = { installed: false }

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

function renderPage(initialEntry = '/settings/integrations/github') {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <GitHubIntegration />
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
    '/settings/integrations/github?installation_id=12345678&setup_action=install&state=csrf-state'

  it('posts the callback from ?installation_id/setup_action/state and toasts on a new connection', async () => {
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

  it('rejects a non-numeric installation_id without calling the service', async () => {
    renderPage(
      '/settings/integrations/github?installation_id=not-a-number&setup_action=install&state=csrf-state'
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
      expect(handleError).toHaveBeenCalledWith(
        expect.any(ApiError),
        'This GitHub organization is already connected to another team. Each GitHub org/account can only be connected to one team.'
      )
    })
  })

  it('reports a generic callback failure with the generic message', async () => {
    ;(githubIntegrationService.handleCallback as jest.Mock).mockRejectedValue(
      new Error('boom')
    )

    renderPage(callbackUrl)

    await waitFor(() => {
      expect(handleError).toHaveBeenCalledWith(
        expect.any(Error),
        'Failed to complete GitHub installation'
      )
    })
  })
})
