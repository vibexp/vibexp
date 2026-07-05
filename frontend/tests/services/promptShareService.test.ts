import type {
  CreateShareRequest,
  ShareResponse,
  SharedPromptResponse,
} from '../../src/services/promptShareService'

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

import { promptShareService } from '../../src/services/promptShareService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })
const failure = (status: number, detail: string) =>
  Promise.resolve({
    error: {
      type: 'https://api.vibexp.io/errors/ERROR',
      title: 'Error',
      status,
      detail,
      code: 'ERROR',
      request_id: 'req-1',
      timestamp: '2024-01-01T10:00:00Z',
    },
    response: { ok: false, status, statusText: 'Error' },
  })

describe('PromptShareService', () => {
  const mockTeamId = 'team-123'
  const mockSlug = 'test-prompt'
  const mockToken = 'share-token-123'

  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('createShare', () => {
    it('creates a public share', async () => {
      const request: CreateShareRequest = { share_type: 'public' }
      const mockResponse: ShareResponse = {
        share_token: mockToken,
        share_url: `https://app.vibexp.io/shared/prompts/${mockToken}`,
        share_type: 'public',
        created_at: '2024-01-01T00:00:00Z',
      }
      mockGeneratedClient.POST.mockReturnValue(success(mockResponse))

      const result = await promptShareService.createShare(
        mockTeamId,
        mockSlug,
        request
      )

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts/{slug}/share',
        {
          params: { path: { team_id: mockTeamId, slug: mockSlug } },
          body: request,
        }
      )
      expect(result).toEqual(mockResponse)
    })

    it('creates a restricted share with emails', async () => {
      const request: CreateShareRequest = {
        share_type: 'restricted',
        emails: ['user1@example.com', 'user2@example.com'],
      }
      const mockResponse: ShareResponse = {
        share_token: mockToken,
        share_url: `https://app.vibexp.io/shared/prompts/${mockToken}`,
        share_type: 'restricted',
        emails: ['user1@example.com', 'user2@example.com'],
        created_at: '2024-01-01T00:00:00Z',
      }
      mockGeneratedClient.POST.mockReturnValue(success(mockResponse))

      const result = await promptShareService.createShare(
        mockTeamId,
        mockSlug,
        request
      )

      expect(result.share_type).toBe('restricted')
      expect(result.emails).toEqual(['user1@example.com', 'user2@example.com'])
    })

    it('propagates API errors', async () => {
      mockGeneratedClient.POST.mockReturnValue(
        failure(500, 'Share creation failed')
      )

      await expect(
        promptShareService.createShare(mockTeamId, mockSlug, {
          share_type: 'public',
        })
      ).rejects.toThrow('Share creation failed')
    })
  })

  describe('getShare', () => {
    it('gets share information for a prompt', async () => {
      const mockResponse: ShareResponse = {
        share_token: mockToken,
        share_url: `https://app.vibexp.io/shared/prompts/${mockToken}`,
        share_type: 'public',
        created_at: '2024-01-01T00:00:00Z',
      }
      mockGeneratedClient.GET.mockReturnValue(success(mockResponse))

      const result = await promptShareService.getShare(mockTeamId, mockSlug)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts/{slug}/share',
        { params: { path: { team_id: mockTeamId, slug: mockSlug } } }
      )
      expect(result).toEqual(mockResponse)
    })
  })

  describe('deleteShare', () => {
    it('deletes a share', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(
        Promise.resolve({
          data: undefined,
          response: { ok: true, status: 204 } as Response,
        })
      )

      await promptShareService.deleteShare(mockTeamId, mockSlug)

      expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts/{slug}/share',
        { params: { path: { team_id: mockTeamId, slug: mockSlug } } }
      )
    })

    it('propagates API errors', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(failure(500, 'Delete failed'))

      await expect(
        promptShareService.deleteShare(mockTeamId, mockSlug)
      ).rejects.toThrow('Delete failed')
    })
  })

  describe('getSharedPrompt', () => {
    it('gets a shared prompt by token', async () => {
      const mockResponse: SharedPromptResponse = {
        prompt: {
          id: 'prompt-123',
          name: 'Shared Prompt',
          slug: mockSlug,
          description: 'A shared prompt',
          body: 'Prompt content',
          user_id: 'user-123',
          project_id: 'project-123',
          team_id: 'team-123',
          status: 'published',
          mcp_expose: false,
          is_shared: true,
          labels: [],
          version: 1,
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2024-01-01T00:00:00Z',
        },
        share_type: 'public',
        rendered_body: '<p>Prompt content</p>',
      }
      mockGeneratedClient.GET.mockReturnValue(success(mockResponse))

      const result = await promptShareService.getSharedPrompt(mockToken)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/shared/prompts/{token}',
        { params: { path: { token: mockToken } } }
      )
      expect(result).toEqual(mockResponse)
    })

    it('propagates an invalid-token error', async () => {
      mockGeneratedClient.GET.mockReturnValue(failure(404, 'Share not found'))

      await expect(
        promptShareService.getSharedPrompt('invalid-token')
      ).rejects.toThrow('Share not found')
    })
  })
})
