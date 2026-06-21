import type {
  MigrationInventory,
  MigrationResult,
} from '../../types/projectMigration'

const mockGet = jest.fn()
const mockPost = jest.fn()

jest.mock('../../lib/apiClient', () => ({
  apiClient: {
    get: (...args: unknown[]) => mockGet(...args),
    post: (...args: unknown[]) => mockPost(...args),
  },
}))

import { projectMigrationService } from '../projectMigrationService'

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
}

describe('ProjectMigrationService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getInventory', () => {
    it('calls the correct endpoint and returns inventory', async () => {
      mockGet.mockResolvedValueOnce(mockInventory)

      const result = await projectMigrationService.getInventory(
        TEAM_ID,
        PROJECT_ID
      )

      expect(mockGet).toHaveBeenCalledWith(
        `/${TEAM_ID}/projects/${PROJECT_ID}/migration/inventory`
      )
      expect(result.prompts.count).toBe(2)
      expect(result.prompts.items).toHaveLength(2)
    })

    it('encodes team and project IDs in the URL', async () => {
      mockGet.mockResolvedValueOnce(mockInventory)

      await projectMigrationService.getInventory('team/1', 'proj/2')

      expect(mockGet).toHaveBeenCalledWith(
        '/team%2F1/projects/proj%2F2/migration/inventory'
      )
    })

    it('propagates errors', async () => {
      mockGet.mockRejectedValueOnce(new Error('Network error'))

      await expect(
        projectMigrationService.getInventory(TEAM_ID, PROJECT_ID)
      ).rejects.toThrow('Network error')
    })
  })

  describe('migrate', () => {
    it('calls the correct endpoint with the request body', async () => {
      mockPost.mockResolvedValueOnce(mockResult)

      const request = {
        destination_project_id: 'dest-project-id',
        resources: {
          prompts: { all: true, ids: ['p1', 'p2'] },
          artifacts: { all: false, ids: ['a1'] },
          blueprints: { all: false, ids: [] },
          feed_items: { all: false, ids: [] },
        },
        conflict_policy: 'skip' as const,
      }

      const result = await projectMigrationService.migrate(
        TEAM_ID,
        PROJECT_ID,
        request
      )

      expect(mockPost).toHaveBeenCalledWith(
        `/${TEAM_ID}/projects/${PROJECT_ID}/migration`,
        request
      )
      expect(result.migrated.prompts).toBe(2)
      expect(result.migrated.artifacts).toBe(1)
    })

    it('encodes team and project IDs in the URL', async () => {
      mockPost.mockResolvedValueOnce(mockResult)

      await projectMigrationService.migrate('team/1', 'proj/2', {
        destination_project_id: 'dest',
        resources: {
          prompts: { all: false, ids: [] },
          artifacts: { all: false, ids: [] },
          blueprints: { all: false, ids: [] },
          feed_items: { all: false, ids: [] },
        },
        conflict_policy: 'rename',
      })

      expect(mockPost).toHaveBeenCalledWith(
        '/team%2F1/projects/proj%2F2/migration',
        expect.objectContaining({ conflict_policy: 'rename' })
      )
    })

    it('propagates errors', async () => {
      mockPost.mockRejectedValueOnce(new Error('Migration failed'))

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
