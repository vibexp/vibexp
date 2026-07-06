// Focused coverage for the prompt version-history service methods. Drives the
// REAL promptService against a mocked generated client to lock in the operation
// + path params for the version endpoints (mirrors blueprintService tests).
const mockGeneratedClient = {
  GET: jest.fn(),
  POST: jest.fn(),
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

import { promptService } from '../../src/services/promptService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

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
    mockGeneratedClient.GET.mockReturnValue(success(versionList))

    const result = await promptService.getPromptVersions(teamId, slug)

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/{team_id}/prompts/{slug}/versions',
      { params: { path: { team_id: teamId, slug } } }
    )
    expect(result).toEqual(versionList)
  })

  it('getPromptVersion fetches a single version by number', async () => {
    mockGeneratedClient.GET.mockReturnValue(success(versionList.versions[0]))

    await promptService.getPromptVersion(teamId, slug, 2)

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/{team_id}/prompts/{slug}/versions/{version_number}',
      { params: { path: { team_id: teamId, slug, version_number: 2 } } }
    )
  })

  it('restorePromptVersion posts to the restore endpoint', async () => {
    const restored = { id: 'prompt-1', slug, name: 'Weekly report' }
    mockGeneratedClient.POST.mockReturnValue(success(restored))

    const result = await promptService.restorePromptVersion(teamId, slug, 1)

    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/{team_id}/prompts/{slug}/versions/{version_number}/restore',
      { params: { path: { team_id: teamId, slug, version_number: 1 } } }
    )
    expect(result).toEqual(restored)
  })
})
