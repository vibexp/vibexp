// Focused coverage for the memory version-history service methods. The broader
// memoryService.test.ts exercises CRUD via a re-implemented testable class; here we
// drive the REAL memoryService against a mocked apiClient to lock in the id-based URL
// construction for the version endpoints (mirrors blueprintService version tests).
const mockApiClient = {
  get: jest.fn(),
  post: jest.fn(),
}

jest.mock('../../src/lib/apiClient', () => ({
  apiClient: mockApiClient,
}))

import { memoryService } from '../../src/services/memoryService'

describe('memoryService version history', () => {
  const teamId = 'team-123'
  const id = 'mem-1'

  const versionList = {
    versions: [
      {
        id: 'ver-1',
        team_id: teamId,
        resource_type: 'memory',
        resource_id: id,
        version_number: 1,
        content: 'old text',
        change_summary: null,
        actor_type: 'human' as const,
        created_by: 'user-123',
        author: null,
        created_at: '2026-01-01T00:00:00Z',
      },
    ],
  }

  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('getMemoryVersions fetches the versions endpoint', async () => {
    mockApiClient.get.mockResolvedValue(versionList)

    const result = await memoryService.getMemoryVersions(teamId, id)

    expect(mockApiClient.get).toHaveBeenCalledWith(
      `/${teamId}/memories/${id}/versions`
    )
    expect(result).toEqual(versionList)
  })

  it('getMemoryVersion fetches a single version by number', async () => {
    mockApiClient.get.mockResolvedValue(versionList.versions[0])

    await memoryService.getMemoryVersion(teamId, id, 2)

    expect(mockApiClient.get).toHaveBeenCalledWith(
      `/${teamId}/memories/${id}/versions/2`
    )
  })

  it('restoreMemoryVersion posts to the restore endpoint', async () => {
    const restored = { ...versionList.versions[0], id }
    mockApiClient.post.mockResolvedValue(restored)

    const result = await memoryService.restoreMemoryVersion(teamId, id, 1)

    expect(mockApiClient.post).toHaveBeenCalledWith(
      `/${teamId}/memories/${id}/versions/1/restore`
    )
    expect(result).toEqual(restored)
  })

  it('encodes special characters in the memory id', async () => {
    mockApiClient.get.mockResolvedValue(versionList)

    await memoryService.getMemoryVersions(teamId, 'mem id/1')

    expect(mockApiClient.get).toHaveBeenCalledWith(
      `/${teamId}/memories/mem%20id%2F1/versions`
    )
  })
})
