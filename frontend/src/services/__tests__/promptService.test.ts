import type { ApiResponse } from '../../types/api'
import type { Prompt } from '../../types/prompt'

// Mock apiClient
const mockApiClient = {
  get: jest.fn(),
  post: jest.fn(),
  put: jest.fn(),
  delete: jest.fn(),
}

jest.mock('../../lib/apiClient', () => ({
  apiClient: mockApiClient,
}))

// Import the promptService after mocking
import { promptService } from '../promptService'

describe('PromptService', () => {
  const teamId = 'team-123'
  const slug = 'my-prompt'

  const mockPrompt: Prompt = {
    id: 'prompt-1',
    name: 'My Prompt',
    slug: 'my-prompt',
    description: 'A test prompt',
    body: 'Hello {{name}}',
    user_id: 'user-1',
    project_id: 'project-1',
    status: 'published',
    mcp_expose: true,
    is_shared: false,
    labels: ['greeting'],
    created_at: '2023-01-01T00:00:00Z',
    updated_at: '2023-01-02T00:00:00Z',
    version: 3,
  }

  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getPrompt', () => {
    it('returns the prompt from a bare object response', async () => {
      mockApiClient.get.mockResolvedValue(mockPrompt)

      const result = await promptService.getPrompt(teamId, slug)

      expect(mockApiClient.get).toHaveBeenCalledWith(
        `/${teamId}/prompts/${slug}`
      )
      expect(result).toEqual(mockPrompt)
    })

    it('returns the prompt from an enveloped {status, message, data} response', async () => {
      const envelope: ApiResponse<Prompt> = {
        status: 'success',
        message: 'ok',
        data: mockPrompt,
      }
      mockApiClient.get.mockResolvedValue(envelope)

      const result = await promptService.getPrompt(teamId, slug)

      expect(mockApiClient.get).toHaveBeenCalledWith(
        `/${teamId}/prompts/${slug}`
      )
      expect(result).toEqual(mockPrompt)
    })

    it('returns equivalent prompts for bare and enveloped shapes', async () => {
      mockApiClient.get.mockResolvedValueOnce(mockPrompt)
      const fromBare = await promptService.getPrompt(teamId, slug)

      mockApiClient.get.mockResolvedValueOnce({
        status: 'success',
        message: 'ok',
        data: mockPrompt,
      })
      const fromEnvelope = await promptService.getPrompt(teamId, slug)

      expect(fromBare).toEqual(fromEnvelope)
    })

    it('throws a clear error when the response is null', async () => {
      mockApiClient.get.mockResolvedValue(null)

      await expect(promptService.getPrompt(teamId, slug)).rejects.toThrow(
        'No prompt data received from server'
      )
    })

    it('throws a clear error when the response is undefined', async () => {
      mockApiClient.get.mockResolvedValue(undefined)

      await expect(promptService.getPrompt(teamId, slug)).rejects.toThrow(
        'No prompt data received from server'
      )
    })

    it('throws a clear error when the envelope data is null', async () => {
      mockApiClient.get.mockResolvedValue({
        status: 'error',
        message: 'not found',
        data: null,
      })

      await expect(promptService.getPrompt(teamId, slug)).rejects.toThrow(
        'No prompt data received from server'
      )
    })

    it('throws a clear error when the response is a non-object', async () => {
      mockApiClient.get.mockResolvedValue('unexpected string')

      await expect(promptService.getPrompt(teamId, slug)).rejects.toThrow(
        'No prompt data received from server'
      )
    })
  })

  describe('createPrompt', () => {
    const createRequest = {
      name: 'My Prompt',
      slug: 'my-prompt',
      description: 'A test prompt',
      body: 'Hello {{name}}',
      project_id: 'project-1',
    }

    it('normalizes a bare object response', async () => {
      mockApiClient.post.mockResolvedValue(mockPrompt)

      const result = await promptService.createPrompt(teamId, createRequest)

      expect(mockApiClient.post).toHaveBeenCalledWith(
        `/${teamId}/prompts`,
        createRequest
      )
      expect(result).toEqual(mockPrompt)
    })

    it('normalizes an enveloped response', async () => {
      mockApiClient.post.mockResolvedValue({
        status: 'success',
        message: 'created',
        data: mockPrompt,
      })

      const result = await promptService.createPrompt(teamId, createRequest)

      expect(result).toEqual(mockPrompt)
    })
  })

  describe('updatePrompt', () => {
    const updateRequest = { name: 'Renamed Prompt' }

    it('normalizes a bare object response', async () => {
      mockApiClient.put.mockResolvedValue(mockPrompt)

      const result = await promptService.updatePrompt(
        teamId,
        slug,
        updateRequest
      )

      expect(mockApiClient.put).toHaveBeenCalledWith(
        `/${teamId}/prompts/${slug}`,
        updateRequest
      )
      expect(result).toEqual(mockPrompt)
    })

    it('normalizes an enveloped response', async () => {
      mockApiClient.put.mockResolvedValue({
        status: 'success',
        message: 'updated',
        data: mockPrompt,
      })

      const result = await promptService.updatePrompt(
        teamId,
        slug,
        updateRequest
      )

      expect(result).toEqual(mockPrompt)
    })
  })
})
