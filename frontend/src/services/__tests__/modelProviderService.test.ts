import { ApiError } from '../../types/errors'
import type {
  CreateModelProviderRequest,
  ModelProviderResponse,
  UpdateModelProviderRequest,
  ValidateModelProviderRequest,
  ValidateModelProviderResponse,
} from '../modelProviderService'

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

import { modelProviderService } from '../modelProviderService'

const teamId = 'team-1'
const providerId = 'provider-123'
const base = '/api/v1/{team_id}/settings/model-providers'
const byId = `${base}/{id}`

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const noContent = {
  ok: true,
  status: 204,
  statusText: 'No Content',
} as Response

const success = <T>(data: T, response: Response = okResponse) =>
  Promise.resolve({ data, response })

// An RFC 9457 problem-details error body as openapi-fetch surfaces it.
const problem = (status: number, detail: string, code: string) =>
  Promise.resolve({
    error: {
      type: `https://api.vibexp.io/errors/${code}`,
      title: code,
      status,
      detail,
      code,
      request_id: 'req-1',
      timestamp: '2024-01-01T00:00:00Z',
    },
    response: { ok: false, status, statusText: code } as Response,
  })

const provider: ModelProviderResponse = {
  id: providerId,
  user_id: 'user-456',
  name: 'OpenAI Provider',
  provider_type: 'openai_compatible',
  model: 'gpt-4o-mini',
  is_default: true,
  base_url: 'https://api.openai.com/v1',
  configuration: '{}',
  created_at: '2023-01-01T00:00:00Z',
  updated_at: '2023-01-01T00:00:00Z',
  version: 1,
  has_api_key: true,
}

describe('ModelProviderService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('createModelProvider', () => {
    const request: CreateModelProviderRequest = {
      name: 'OpenAI Provider',
      provider_type: 'openai_compatible',
      model: 'gpt-4o-mini',
      is_default: true,
      base_url: 'https://api.openai.com/v1',
      api_key: 'sk-test-key',
    }

    it('posts to the team-scoped settings endpoint and returns the provider', async () => {
      mockGeneratedClient.POST.mockReturnValue(success(provider))

      const result = await modelProviderService.createModelProvider(
        teamId,
        request
      )

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(base, {
        params: { path: { team_id: teamId } },
        body: request,
      })
      expect(result).toEqual(provider)
    })

    it('throws ApiError with the backend detail on a validation failure', async () => {
      mockGeneratedClient.POST.mockReturnValue(
        problem(400, 'Invalid API key format', 'VALIDATION_FAILED')
      )

      await expect(
        modelProviderService.createModelProvider(teamId, request)
      ).rejects.toThrow('Invalid API key format')
    })

    it('maps a network failure to a friendly message', async () => {
      mockGeneratedClient.POST.mockRejectedValue(new TypeError('fetch failed'))

      await expect(
        modelProviderService.createModelProvider(teamId, request)
      ).rejects.toThrow('Network error: Unable to connect to server')
    })
  })

  describe('getModelProviders', () => {
    it('lists providers as a bare array from the settings endpoint', async () => {
      const providers = [provider, { ...provider, id: 'provider-2' }]
      mockGeneratedClient.GET.mockReturnValue(success(providers))

      const result = await modelProviderService.getModelProviders(teamId)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(base, {
        params: { path: { team_id: teamId } },
      })
      expect(result).toEqual(providers)
    })

    it('throws ApiError on a server error', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        problem(500, 'Failed to retrieve model providers', 'INTERNAL_ERROR')
      )

      await expect(
        modelProviderService.getModelProviders(teamId)
      ).rejects.toThrow('Failed to retrieve model providers')
    })
  })

  describe('getModelProvider', () => {
    it('fetches a single provider by id', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(provider))

      const result = await modelProviderService.getModelProvider(
        teamId,
        providerId
      )

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(byId, {
        params: { path: { team_id: teamId, id: providerId } },
      })
      expect(result).toEqual(provider)
    })

    it('throws a not-found ApiError', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        problem(404, 'Model provider not found', 'MODEL_PROVIDER_NOT_FOUND')
      )

      const error = (await modelProviderService
        .getModelProvider(teamId, providerId)
        .catch((e: unknown) => e)) as ApiError
      expect(error).toBeInstanceOf(ApiError)
      expect(error.status).toBe(404)
      expect(error.message).toContain('Model provider not found')
    })
  })

  describe('updateModelProvider', () => {
    const request: UpdateModelProviderRequest = {
      name: 'Updated OpenAI Provider',
      is_default: false,
    }

    it('puts the changed fields to the by-id endpoint', async () => {
      const updated = { ...provider, name: 'Updated OpenAI Provider' }
      mockGeneratedClient.PUT.mockReturnValue(success(updated))

      const result = await modelProviderService.updateModelProvider(
        teamId,
        providerId,
        request
      )

      expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(byId, {
        params: { path: { team_id: teamId, id: providerId } },
        body: request,
      })
      expect(result.name).toBe('Updated OpenAI Provider')
    })

    it('throws ApiError on a validation failure', async () => {
      mockGeneratedClient.PUT.mockReturnValue(
        problem(400, 'Invalid configuration format', 'VALIDATION_FAILED')
      )

      await expect(
        modelProviderService.updateModelProvider(teamId, providerId, request)
      ).rejects.toThrow('Invalid configuration format')
    })
  })

  describe('deleteModelProvider', () => {
    it('deletes by id and resolves void on a 204', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(success(undefined, noContent))

      await expect(
        modelProviderService.deleteModelProvider(teamId, providerId)
      ).resolves.toBeUndefined()

      expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(byId, {
        params: { path: { team_id: teamId, id: providerId } },
      })
    })

    it('surfaces the last-provider guard as an ApiError', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(
        problem(
          400,
          'Cannot delete the last model provider',
          'MODEL_PROVIDER_LAST_DELETE_BLOCKED'
        )
      )

      await expect(
        modelProviderService.deleteModelProvider(teamId, providerId)
      ).rejects.toThrow('Cannot delete the last model provider')
    })
  })

  describe('validateModelProvider', () => {
    const request: ValidateModelProviderRequest = {
      provider_type: 'openai_compatible',
      model: 'gpt-4o-mini',
      base_url: 'https://api.openai.com/v1',
      api_key: 'sk-test-key',
    }

    it('posts to the validate endpoint and returns the outcome', async () => {
      const response: ValidateModelProviderResponse = {
        is_valid: true,
        message: 'Provider configuration is valid',
        details: { response_time_ms: 150, status_code: 200 },
      }
      mockGeneratedClient.POST.mockReturnValue(success(response))

      const result = await modelProviderService.validateModelProvider(
        teamId,
        request
      )

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        `${base}/validate`,
        {
          params: { path: { team_id: teamId } },
          body: request,
        }
      )
      expect(result).toEqual(response)
    })

    it('returns an is_valid:false outcome without throwing (200 body)', async () => {
      const response: ValidateModelProviderResponse = {
        is_valid: false,
        message: 'Invalid API key',
        details: { status_code: 401, error_details: 'Unauthorized' },
      }
      mockGeneratedClient.POST.mockReturnValue(success(response))

      const result = await modelProviderService.validateModelProvider(
        teamId,
        request
      )

      expect(result.is_valid).toBe(false)
    })

    it('throws ApiError on an internal service error', async () => {
      mockGeneratedClient.POST.mockReturnValue(
        problem(
          500,
          'Validation service is temporarily unavailable',
          'INTERNAL_ERROR'
        )
      )

      await expect(
        modelProviderService.validateModelProvider(teamId, request)
      ).rejects.toThrow('Validation service is temporarily unavailable')
    })
  })
})
