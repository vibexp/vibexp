import type {
  MigrationInventory,
  MigrationRequest,
  MigrationResult,
} from '../projectMigrationService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  GET: jest.fn(),
  POST: jest.fn(),
}

jest.mock('../../lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return {
    ...actual,
    generatedClient: mockGeneratedClient,
  }
})

import { projectMigrationService } from '../projectMigrationService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })
const failure = (detail: string) =>
  Promise.resolve({
    error: {
      type: 'https://api.vibexp.io/errors/INTERNAL_ERROR',
      title: 'Internal Server Error',
      status: 500,
      detail,
      code: 'INTERNAL_ERROR',
      request_id: 'req-1',
      timestamp: '2024-01-01T10:00:00Z',
    },
    response: { ok: false, status: 500, statusText: 'Server Error' },
  })

const TEAM_ID = 'team-abc'
const PROJECT_ID = 'project-xyz'

const mockInventory: MigrationInventory = {
  prompts: {
    count: 2,
    items: [
      { id: 'p1', name: 'Prompt A' },
      { id: 'p2', name: 'Prompt B' },
    ],
  },
  artifacts: { count: 1, items: [{ id: 'a1', name: 'Artifact A' }] },
  blueprints: { count: 0, items: [] },
  feed_items: { count: 3 },
}

const mockResult: MigrationResult = {
  migrated: { prompts: 2, artifacts: 1, blueprints: 0, feed_items: 0 },
  skipped: {},
  failed: {},
  source_project_name: 'Source',
  destination_project_name: 'Destination',
}

describe('ProjectMigrationService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getInventory', () => {
    it('calls the correct endpoint and returns inventory', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockInventory))

      const result = await projectMigrationService.getInventory(
        TEAM_ID,
        PROJECT_ID
      )

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/projects/{project_id}/migration/inventory',
        { params: { path: { team_id: TEAM_ID, project_id: PROJECT_ID } } }
      )
      expect(result.prompts?.count).toBe(2)
      expect(result.prompts?.items).toHaveLength(2)
    })

    it('passes team and project IDs through path params', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockInventory))

      await projectMigrationService.getInventory('team/1', 'proj/2')

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/projects/{project_id}/migration/inventory',
        { params: { path: { team_id: 'team/1', project_id: 'proj/2' } } }
      )
    })

    it('propagates errors', async () => {
      mockGeneratedClient.GET.mockReturnValue(failure('Network error'))

      await expect(
        projectMigrationService.getInventory(TEAM_ID, PROJECT_ID)
      ).rejects.toThrow('Network error')
    })
  })

  describe('migrate', () => {
    it('calls the correct endpoint with the request body', async () => {
      mockGeneratedClient.POST.mockReturnValue(success(mockResult))

      const request: MigrationRequest = {
        destination_project_id: 'dest-project-id',
        resources: {
          prompts: { all: true, ids: ['p1', 'p2'] },
          artifacts: { all: false, ids: ['a1'] },
          blueprints: { all: false, ids: [] },
          feed_items: { all: false, ids: [] },
        },
        conflict_policy: 'skip',
      }

      const result = await projectMigrationService.migrate(
        TEAM_ID,
        PROJECT_ID,
        request
      )

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/projects/{project_id}/migration',
        {
          params: { path: { team_id: TEAM_ID, project_id: PROJECT_ID } },
          body: request,
        }
      )
      expect(result.migrated.prompts).toBe(2)
      expect(result.migrated.artifacts).toBe(1)
    })

    it('passes team and project IDs through path params', async () => {
      mockGeneratedClient.POST.mockReturnValue(success(mockResult))

      const request: MigrationRequest = {
        destination_project_id: 'dest',
        resources: {
          prompts: { all: false, ids: [] },
          artifacts: { all: false, ids: [] },
          blueprints: { all: false, ids: [] },
          feed_items: { all: false, ids: [] },
        },
        conflict_policy: 'rename',
      }

      await projectMigrationService.migrate('team/1', 'proj/2', request)

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/projects/{project_id}/migration',
        {
          params: { path: { team_id: 'team/1', project_id: 'proj/2' } },
          body: expect.objectContaining({ conflict_policy: 'rename' }),
        }
      )
    })

    it('propagates errors', async () => {
      mockGeneratedClient.POST.mockReturnValue(failure('Migration failed'))

      await expect(
        projectMigrationService.migrate(TEAM_ID, PROJECT_ID, {
          destination_project_id: 'dest',
          resources: {
            prompts: { all: false, ids: [] },
            artifacts: { all: false, ids: [] },
            blueprints: { all: false, ids: [] },
            feed_items: { all: false, ids: [] },
          },
          conflict_policy: 'overwrite',
        })
      ).rejects.toThrow('Migration failed')
    })
  })
})
