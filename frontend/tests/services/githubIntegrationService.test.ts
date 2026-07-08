import type {
  BlueprintImportReport,
  GitHubInstallCallbackRequest,
  GitHubInstallationStatus,
  GitHubRepositoriesResponse,
  ImportProjectResponse,
} from '../../src/services/githubIntegrationService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  GET: jest.fn(),
  POST: jest.fn(),
  DELETE: jest.fn(),
}

jest.mock('../../src/lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../src/lib/apiClientGenerated')
  >('../../src/lib/apiClientGenerated')
  return {
    ...actual,
    generatedClient: mockGeneratedClient,
  }
})

import { githubIntegrationService } from '../../src/services/githubIntegrationService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

describe('GitHubIntegrationService', () => {
  const teamId = 'team-123'

  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getStatus', () => {
    it('returns the real backend status shape (suspended, installed_at)', async () => {
      // The backend returns models.GitHubInstallationStatus, not the spec's shape
      // (see the DRIFT note in the service).
      const status: GitHubInstallationStatus = {
        installed: true,
        account_login: 'my-org',
        installation_id: 42,
        suspended: false,
        installed_at: '2024-01-01T00:00:00Z',
      }
      mockGeneratedClient.GET.mockReturnValue(success(status))

      const result = await githubIntegrationService.getStatus(teamId)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/integrations/github/status',
        { params: { path: { team_id: teamId } } }
      )
      expect(result).toEqual(status)
    })
  })

  describe('getInstallUrl', () => {
    it('fetches the install URL', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        success({
          install_url: 'https://github.com/apps/vibexp/installations/new',
        })
      )

      const result = await githubIntegrationService.getInstallUrl(teamId)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/integrations/github/install-url',
        { params: { path: { team_id: teamId } } }
      )
      expect(result.install_url).toContain('github.com')
    })
  })

  describe('handleCallback', () => {
    it('posts the callback body and returns the reconnected flag', async () => {
      const body: GitHubInstallCallbackRequest = {
        installation_id: 42,
        state: 'signed-state',
        setup_action: 'install',
      }
      mockGeneratedClient.POST.mockReturnValue(success({ reconnected: true }))

      const result = await githubIntegrationService.handleCallback(teamId, body)

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/integrations/github/callback',
        { params: { path: { team_id: teamId } }, body }
      )
      expect(result.reconnected).toBe(true)
    })
  })

  describe('getRepositories', () => {
    it('lists repositories with the page query param', async () => {
      const repos: GitHubRepositoriesResponse = {
        repositories: [],
        total_count: 0,
        page: 2,
        per_page: 100,
      }
      mockGeneratedClient.GET.mockReturnValue(success(repos))
      const controller = new AbortController()

      const result = await githubIntegrationService.getRepositories(
        teamId,
        2,
        controller.signal
      )

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/integrations/github/repositories',
        {
          params: { path: { team_id: teamId }, query: { page: 2 } },
          signal: controller.signal,
        }
      )
      expect(result).toEqual(repos)
    })

    it('defaults to page 1', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        success({ repositories: [], total_count: 0, page: 1, per_page: 100 })
      )

      await githubIntegrationService.getRepositories(teamId)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/integrations/github/repositories',
        {
          params: { path: { team_id: teamId }, query: { page: 1 } },
          signal: undefined,
        }
      )
    })
  })

  describe('disconnect', () => {
    it('deletes the integration', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(
        Promise.resolve({
          data: undefined,
          response: { ok: true, status: 204 } as Response,
        })
      )

      await githubIntegrationService.disconnect(teamId)

      expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
        '/api/v1/{team_id}/integrations/github/disconnect',
        { params: { path: { team_id: teamId } } }
      )
    })
  })

  describe('importProject', () => {
    it('imports a repository as a project, coercing the repo id to a number', async () => {
      const response: ImportProjectResponse = {
        project: {
          id: 'proj-1',
          user_id: 'user-123',
          team_id: teamId,
          name: 'my-repo',
          slug: 'my-repo',
          description: '',
          git_url: 'https://github.com/my-org/my-repo',
          homepage: '',
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2024-01-01T00:00:00Z',
          version: 1,
        },
        created: true,
      }
      mockGeneratedClient.POST.mockReturnValue(success(response))

      const result = await githubIntegrationService.importProject(
        teamId,
        '123456'
      )

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/integrations/github/repositories/{repo_id}/import-project',
        { params: { path: { team_id: teamId, repo_id: 123456 } } }
      )
      expect(result).toEqual(response)
    })
  })

  describe('importBlueprints', () => {
    it('posts the repository_id and returns the import report', async () => {
      const report: BlueprintImportReport = {
        total_scanned: 1,
        total_successful: 1,
        total_failed: 0,
        total_skipped: 0,
        successful_items: [],
        failed_items: [],
        skipped_items: [],
      }
      mockGeneratedClient.POST.mockReturnValue(success(report))

      const result = await githubIntegrationService.importBlueprints(
        teamId,
        123456
      )

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/integrations/github/import-blueprints',
        {
          params: { path: { team_id: teamId } },
          body: { repository_id: 123456 },
        }
      )
      expect(result).toEqual(report)
    })
  })
})
