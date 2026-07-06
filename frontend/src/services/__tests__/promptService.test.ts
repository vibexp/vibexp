import type { Prompt, PromptListResponse } from '../promptService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  GET: jest.fn(),
  POST: jest.fn(),
  PUT: jest.fn(),
  DELETE: jest.fn(),
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

import { promptService } from '../promptService'

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
    team_id: teamId,
    project_id: 'project-1',
    status: 'published',
    mcp_expose: true,
    is_shared: false,
    labels: ['greeting'],
    created_at: '2023-01-01T00:00:00Z',
    updated_at: '2023-01-02T00:00:00Z',
    version: 3,
  }

  const listResponse: PromptListResponse = {
    prompts: [mockPrompt],
    total_count: 1,
    page: 1,
    per_page: 20,
    total_pages: 1,
  }

  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getPrompts', () => {
    it('unwraps the SuccessResponse envelope to the inner payload', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        success({ status: 'success', message: 'ok', data: listResponse })
      )

      const result = await promptService.getPrompts(teamId, { search: 'x' })

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts',
        { params: { path: { team_id: teamId }, query: { search: 'x' } } }
      )
      expect(result).toEqual(listResponse)
    })
  })

  describe('getPrompt', () => {
    it('returns the prompt', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockPrompt))

      const result = await promptService.getPrompt(teamId, slug)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts/{slug}',
        { params: { path: { team_id: teamId, slug } } }
      )
      expect(result).toEqual(mockPrompt)
    })

    it('propagates a not-found error', async () => {
      mockGeneratedClient.GET.mockReturnValue(failure(404, 'Prompt not found'))

      await expect(promptService.getPrompt(teamId, slug)).rejects.toThrow(
        'Prompt not found'
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

    it('posts the create request body', async () => {
      mockGeneratedClient.POST.mockReturnValue(success(mockPrompt))

      const result = await promptService.createPrompt(teamId, createRequest)

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts',
        { params: { path: { team_id: teamId } }, body: createRequest }
      )
      expect(result).toEqual(mockPrompt)
    })
  })

  describe('updatePrompt', () => {
    const updateRequest = { name: 'Renamed Prompt' }

    it('puts the update request body', async () => {
      mockGeneratedClient.PUT.mockReturnValue(success(mockPrompt))

      const result = await promptService.updatePrompt(
        teamId,
        slug,
        updateRequest
      )

      expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts/{slug}',
        {
          params: { path: { team_id: teamId, slug } },
          body: updateRequest,
        }
      )
      expect(result).toEqual(mockPrompt)
    })
  })

  describe('getPromptLabels', () => {
    it('unwraps the enveloped labels payload', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        success({
          status: 'success',
          message: 'ok',
          data: { labels: ['a', 'b'] },
        })
      )

      const result = await promptService.getPromptLabels(teamId)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts/labels',
        { params: { path: { team_id: teamId } } }
      )
      expect(result).toEqual(['a', 'b'])
    })
  })

  describe('getPromptDependencies', () => {
    it('adapts the backend used_by/uses payload, defaulting to arrays', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        success({
          used_by: [{ id: 'p2', slug: 'p-2', name: 'Prompt 2' }],
        })
      )

      const result = await promptService.getPromptDependencies(teamId, slug)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts/{slug}/dependencies',
        { params: { path: { team_id: teamId, slug } } }
      )
      expect(result).toEqual({
        used_by: [{ id: 'p2', slug: 'p-2', name: 'Prompt 2' }],
        uses: [],
      })
    })
  })

  describe('getPromptPlaceholders', () => {
    it('returns the placeholders array', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        success({ placeholders: ['name', 'lang'] })
      )

      const result = await promptService.getPromptPlaceholders(teamId, slug)

      expect(result).toEqual(['name', 'lang'])
    })
  })
})
