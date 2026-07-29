import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import type { GitHubRepository } from '@/services/githubIntegrationService'
import { githubIntegrationService } from '@/services/githubIntegrationService'

const mockUseTeam = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

vi.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: vi.fn() }),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('@/services/githubIntegrationService', () => ({
  githubIntegrationService: {
    importProject: vi.fn(),
  },
}))

// Stub the modal with a button that surfaces onConfirm so we can trigger
// the import flow without rendering the full modal tree.
vi.mock('./ImportProjectModal', () => ({
  ImportProjectModal: ({
    isOpen,
    onConfirm,
  }: {
    isOpen: boolean
    onConfirm: () => void
  }) =>
    isOpen ? (
      <button type="button" onClick={onConfirm}>
        confirm-import
      </button>
    ) : null,
}))

import { ImportProjectButton } from './ImportProjectButton'

const baseRepository: GitHubRepository = {
  id: 12345,
  name: 'awesome-repo',
  full_name: 'octocat/awesome-repo',
  private: false,
  html_url: 'https://github.com/octocat/awesome-repo',
  description: 'An awesome repo',
  owner: { login: 'octocat', type: 'User' },
}

function renderButton(repository: GitHubRepository) {
  return render(
    <MemoryRouter>
      <ImportProjectButton repository={repository} />
    </MemoryRouter>
  )
}

describe('ImportProjectButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseTeam.mockReturnValue({
      currentTeam: { id: 'team-1', name: 'Test Team' },
      teams: [{ id: 'team-1', name: 'Test Team' }],
      isLoading: false,
      setCurrentTeam: vi.fn(),
      refreshTeams: vi.fn() as () => Promise<void>,
    })
  })

  it('renders "View Project" link to the project page when imported_project_slug is set', () => {
    renderButton({ ...baseRepository, imported_project_slug: 'abc' })

    const link = screen.getByRole('link', { name: /view project/i })
    expect(link).toBeInTheDocument()
    expect(link.getAttribute('href')).toBe('/teams/team-1/projects/abc')
    expect(
      screen.queryByRole('button', { name: /import as project/i })
    ).not.toBeInTheDocument()
  })

  it('renders "Import as Project" button when imported_project_slug is unset', () => {
    renderButton(baseRepository)

    expect(
      screen.getByRole('button', { name: /import as project/i })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: /view project/i })
    ).not.toBeInTheDocument()
  })

  it('flips to "View Project" using the returned slug after a same-session successful import', async () => {
    const importProject = vi.mocked(githubIntegrationService.importProject)
    importProject.mockResolvedValueOnce({
      project: {
        id: 'p-1',
        user_id: 'u-1',
        team_id: 'team-1',
        name: 'awesome-repo',
        slug: 'awesome-repo',
        description: '',
        git_url: 'https://github.com/octocat/awesome-repo',
        homepage: '',
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
        version: 1,
      },
      created: true,
    })

    const user = userEvent.setup()
    renderButton(baseRepository)

    await user.click(screen.getByRole('button', { name: /import as project/i }))
    await user.click(screen.getByRole('button', { name: /confirm-import/i }))

    await waitFor(() => {
      const link = screen.getByRole('link', { name: /view project/i })
      expect(link.getAttribute('href')).toBe(
        '/teams/team-1/projects/awesome-repo'
      )
    })
    expect(
      screen.queryByRole('button', { name: /import as project/i })
    ).not.toBeInTheDocument()
  })
})
