import type {
  CreateShareRequest,
  CreateShareApiResponse,
  GetShareApiResponse,
  SharedPromptApiResponse,
} from '../../src/types'

// Mock apiClient
const mockApiClient = {
  get: jest.fn(),
  post: jest.fn(),
  delete: jest.fn(),
}

jest.mock('../../src/lib/apiClient', () => ({
  apiClient: mockApiClient,
}))

// Import after mocking
import { promptShareService } from '../../src/services/promptShareService'

describe('PromptShareService', () => {
  const mockTeamId = 'team-123'
  const mockSlug = 'test-prompt'
  const mockToken = 'share-token-123'

  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('createShare', () => {
    it('should create a public share', async () => {
      const request: CreateShareRequest = {
        share_type: 'public',
      }

      const mockResponse: CreateShareApiResponse = {
        share_token: mockToken,
        share_url: `https://app.vibexp.io/shared/prompts/${mockToken}`,
        share_type: 'public',
        created_at: '2024-01-01T00:00:00Z',
      }

      mockApiClient.post.mockResolvedValue(mockResponse)

      const result = await promptShareService.createShare(
        mockTeamId,
        mockSlug,
        request
      )

      expect(mockApiClient.post).toHaveBeenCalledWith(
        `/${mockTeamId}/prompts/${mockSlug}/share`,
        request
      )
      expect(result).toEqual(mockResponse)
    })

    it('should create a restricted share with emails', async () => {
      const request: CreateShareRequest = {
        share_type: 'restricted',
        emails: ['user1@example.com', 'user2@example.com'],
      }

      const mockResponse: CreateShareApiResponse = {
        share_token: mockToken,
        share_url: `https://app.vibexp.io/shared/prompts/${mockToken}`,
        share_type: 'restricted',
        emails: ['user1@example.com', 'user2@example.com'],
        created_at: '2024-01-01T00:00:00Z',
      }

      mockApiClient.post.mockResolvedValue(mockResponse)

      const result = await promptShareService.createShare(
        mockTeamId,
        mockSlug,
        request
      )

      expect(mockApiClient.post).toHaveBeenCalledWith(
        `/${mockTeamId}/prompts/${mockSlug}/share`,
        request
      )
      expect(result.share_type).toBe('restricted')
      expect(result.emails).toEqual(['user1@example.com', 'user2@example.com'])
    })

    it('should create a share with expiration', async () => {
      const request: CreateShareRequest = {
        share_type: 'public',
        expires_at: '2024-01-08T00:00:00Z',
      }

      const mockResponse: CreateShareApiResponse = {
        share_token: mockToken,
        share_url: `https://app.vibexp.io/shared/prompts/${mockToken}`,
        share_type: 'public',
        expires_at: '2024-01-08T00:00:00Z',
        created_at: '2024-01-01T00:00:00Z',
      }

      mockApiClient.post.mockResolvedValue(mockResponse)

      const result = await promptShareService.createShare(
        mockTeamId,
        mockSlug,
        request
      )

      expect(result.expires_at).toBe('2024-01-08T00:00:00Z')
    })

    it('should handle API errors', async () => {
      mockApiClient.post.mockRejectedValue(new Error('Share creation failed'))

      await expect(
        promptShareService.createShare(mockTeamId, mockSlug, {
          share_type: 'public',
        })
      ).rejects.toThrow('Share creation failed')
    })
  })

  describe('getShare', () => {
    it('should get share information for a prompt', async () => {
      const mockResponse: GetShareApiResponse = {
        share_token: mockToken,
        share_url: `https://app.vibexp.io/shared/prompts/${mockToken}`,
        share_type: 'public',
        created_at: '2024-01-01T00:00:00Z',
      }

      mockApiClient.get.mockResolvedValue(mockResponse)

      const result = await promptShareService.getShare(mockTeamId, mockSlug)

      expect(mockApiClient.get).toHaveBeenCalledWith(
        `/${mockTeamId}/prompts/${mockSlug}/share`
      )
      expect(result).toEqual(mockResponse)
    })

    it('should get share with expiration', async () => {
      const mockResponse: GetShareApiResponse = {
        share_token: mockToken,
        share_url: `https://app.vibexp.io/shared/prompts/${mockToken}`,
        share_type: 'public',
        expires_at: '2024-01-08T00:00:00Z',
        created_at: '2024-01-01T00:00:00Z',
      }

      mockApiClient.get.mockResolvedValue(mockResponse)

      const result = await promptShareService.getShare(mockTeamId, mockSlug)

      expect(result.expires_at).toBe('2024-01-08T00:00:00Z')
    })
  })

  describe('deleteShare', () => {
    it('should delete a share', async () => {
      mockApiClient.delete.mockResolvedValue(undefined)

      await promptShareService.deleteShare(mockTeamId, mockSlug)

      expect(mockApiClient.delete).toHaveBeenCalledWith(
        `/${mockTeamId}/prompts/${mockSlug}/share`
      )
    })

    it('should handle API errors', async () => {
      mockApiClient.delete.mockRejectedValue(new Error('Delete failed'))

      await expect(
        promptShareService.deleteShare(mockTeamId, mockSlug)
      ).rejects.toThrow('Delete failed')
    })
  })

  describe('getSharedPrompt', () => {
    it('should get a shared prompt by token', async () => {
      const mockResponse: SharedPromptApiResponse = {
        status: 'success',
        message: 'Shared prompt retrieved',
        data: {
          prompt: {
            id: 'prompt-123',
            name: 'Shared Prompt',
            slug: mockSlug,
            description: 'A shared prompt',
            body: 'Prompt content',
            user_id: 'user-123',
            project_id: 'project-123',
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
          expires_at: '2024-01-08T00:00:00Z',
        },
      }

      mockApiClient.get.mockResolvedValue(mockResponse)

      const result = await promptShareService.getSharedPrompt(mockToken)

      expect(mockApiClient.get).toHaveBeenCalledWith(
        `/shared/prompts/${mockToken}`
      )
      expect(result).toEqual(mockResponse)
    })

    it('should handle invalid token', async () => {
      mockApiClient.get.mockRejectedValue(new Error('Share not found'))

      await expect(
        promptShareService.getSharedPrompt('invalid-token')
      ).rejects.toThrow('Share not found')
    })

    it('should handle expired share', async () => {
      mockApiClient.get.mockRejectedValue(new Error('Share has expired'))

      await expect(
        promptShareService.getSharedPrompt(mockToken)
      ).rejects.toThrow('Share has expired')
    })
  })
})
