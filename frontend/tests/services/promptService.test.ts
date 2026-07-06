import type {
  CreatePromptRequest,
  Prompt,
  PromptListResponse,
  UpdatePromptRequest,
} from '../../src/services/promptService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  GET: jest.fn(),
  POST: jest.fn(),
  PUT: jest.fn(),
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

import { promptService } from '../../src/services/promptService'

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

const teamId = 'team-123'
const slug = 'my-prompt'

const mockPrompt: Prompt = {
  id: 'prompt-1',
  name: 'My Prompt',
  slug,
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

describe('PromptService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getPrompts', () => {
    it('lists prompts and unwraps the envelope', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        success({ status: 'success', message: 'ok', data: listResponse })
      )

      const result = await promptService.getPrompts(teamId, {
        status: 'published',
        page: 1,
        limit: 20,
      })

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts',
        {
          params: {
            path: { team_id: teamId },
            query: { status: 'published', page: 1, limit: 20 },
          },
        }
      )
      expect(result).toEqual(listResponse)
    })

    it('propagates errors', async () => {
      mockGeneratedClient.GET.mockReturnValue(failure(403, 'forbidden'))

      await expect(promptService.getPrompts(teamId)).rejects.toThrow(
        'forbidden'
      )
    })
  })

  describe('getPrompt', () => {
    it('fetches a single prompt by slug', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockPrompt))

      const result = await promptService.getPrompt(teamId, slug)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts/{slug}',
        { params: { path: { team_id: teamId, slug } } }
      )
      expect(result).toEqual(mockPrompt)
    })
  })

  describe('createPrompt', () => {
    it('posts the create request body', async () => {
      const body: CreatePromptRequest = {
        name: 'My Prompt',
        slug,
        body: 'Hello {{name}}',
        project_id: 'project-1',
      }
      mockGeneratedClient.POST.mockReturnValue(success(mockPrompt))

      const result = await promptService.createPrompt(teamId, body)

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts',
        { params: { path: { team_id: teamId } }, body }
      )
      expect(result).toEqual(mockPrompt)
    })
  })

  describe('updatePrompt', () => {
    it('puts the update request body', async () => {
      const body: UpdatePromptRequest = { name: 'Renamed' }
      mockGeneratedClient.PUT.mockReturnValue(success(mockPrompt))

      const result = await promptService.updatePrompt(teamId, slug, body)

      expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts/{slug}',
        { params: { path: { team_id: teamId, slug } }, body }
      )
      expect(result).toEqual(mockPrompt)
    })
  })

  describe('deletePrompt', () => {
    it('deletes a prompt by slug', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(
        Promise.resolve({
          data: undefined,
          response: { ok: true, status: 204 } as Response,
        })
      )

      await promptService.deletePrompt(teamId, slug)

      expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts/{slug}',
        { params: { path: { team_id: teamId, slug } } }
      )
    })
  })

  describe('getPromptPlaceholders', () => {
    it('returns the placeholders array', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        success({ placeholders: ['name', 'lang'] })
      )

      const result = await promptService.getPromptPlaceholders(teamId, slug)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts/{slug}/placeholders',
        { params: { path: { team_id: teamId, slug } } }
      )
      expect(result).toEqual(['name', 'lang'])
    })
  })

  describe('renderPrompt', () => {
    it('posts placeholders and returns the rendered body', async () => {
      const rendered = { rendered_body: 'Hello World' }
      mockGeneratedClient.POST.mockReturnValue(success(rendered))

      const result = await promptService.renderPrompt(teamId, slug, {
        name: 'World',
      })

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts/{slug}/render',
        {
          params: { path: { team_id: teamId, slug } },
          body: { placeholders: { name: 'World' } },
        }
      )
      expect(result).toEqual(rendered)
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

      expect(result).toEqual(['a', 'b'])
    })
  })

  describe('getPromptDependencies', () => {
    it('returns used_by/uses arrays', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        success({
          used_by: [{ id: 'p2', slug: 'p-2', name: 'Prompt 2' }],
          uses: [{ id: 'p3', slug: 'p-3', name: 'Prompt 3' }],
        })
      )

      const result = await promptService.getPromptDependencies(teamId, slug)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/prompts/{slug}/dependencies',
        { params: { path: { team_id: teamId, slug } } }
      )
      expect(result).toEqual({
        used_by: [{ id: 'p2', slug: 'p-2', name: 'Prompt 2' }],
        uses: [{ id: 'p3', slug: 'p-3', name: 'Prompt 3' }],
      })
    })

    it('defaults missing dependency arrays to empty', async () => {
      mockGeneratedClient.GET.mockReturnValue(success({}))

      const result = await promptService.getPromptDependencies(teamId, slug)

      expect(result).toEqual({ used_by: [], uses: [] })
    })
  })
})
