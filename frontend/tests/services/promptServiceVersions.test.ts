// Focused coverage for the prompt version-history service methods. The broader
// prompt service tests exercise CRUD elsewhere; here we drive the REAL promptService
// against a mocked apiClient to lock in the slug-based URL construction for the version
// endpoints (mirrors memoryService / blueprintService version tests).
const mockApiClient = {
  get: jest.fn(),
  post: jest.fn(),
}

jest.mock('../../src/lib/apiClient', () => ({
  apiClient: mockApiClient,
}))

import { promptService } from '../../src/services/promptService'

describe('promptService version history', () => {
  const teamId = 'team-123'
  const slug = 'weekly-report'

  const versionList = {
    versions: [
      {
        id: 'ver-1',
        team_id: teamId,
        resource_type: 'prompt',
        resource_id: 'prompt-1',
        version_number: 1,
        content: 'Hello {{name}}',
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

  it('getPromptVersions fetches the versions endpoint', async () => {
    mockApiClient.get.mockResolvedValue(versionList)

    const result = await promptService.getPromptVersions(teamId, slug)

    expect(mockApiClient.get).toHaveBeenCalledWith(
      `/${teamId}/prompts/${slug}/versions`
    )
    expect(result).toEqual(versionList)
  })

  it('getPromptVersion fetches a single version by number', async () => {
    mockApiClient.get.mockResolvedValue(versionList.versions[0])

    await promptService.getPromptVersion(teamId, slug, 2)

    expect(mockApiClient.get).toHaveBeenCalledWith(
      `/${teamId}/prompts/${slug}/versions/2`
    )
  })

  it('restorePromptVersion posts to the restore endpoint and unwraps the prompt', async () => {
    const restored = { id: 'prompt-1', slug, name: 'Weekly report' }
    mockApiClient.post.mockResolvedValue(restored)

    const result = await promptService.restorePromptVersion(teamId, slug, 1)

    expect(mockApiClient.post).toHaveBeenCalledWith(
      `/${teamId}/prompts/${slug}/versions/1/restore`
    )
    expect(result).toEqual(restored)
  })

  it('encodes special characters in the prompt slug', async () => {
    mockApiClient.get.mockResolvedValue(versionList)

    await promptService.getPromptVersions(teamId, 'my prompt/1')

    expect(mockApiClient.get).toHaveBeenCalledWith(
      `/${teamId}/prompts/my%20prompt%2F1/versions`
    )
  })
})
