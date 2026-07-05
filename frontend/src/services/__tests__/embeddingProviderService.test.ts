import type {
  CreateEmbeddingProviderRequest,
  EmbeddingProviderResponse,
  UpdateEmbeddingProviderRequest,
  ValidateEmbeddingProviderRequest,
  ValidateEmbeddingProviderResponse,
} from '../../types'
import { ApiError, type APIErrorResponse } from '../../types/errors'

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

// Import the embeddingProviderService after mocking
import { embeddingProviderService } from '../embeddingProviderService'

describe('EmbeddingProviderService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('createEmbeddingProvider', () => {
    const mockRequest: CreateEmbeddingProviderRequest = {
      name: 'OpenAI Provider',
      provider_type: 'openai',
      model: 'text-embedding-3-small',
      is_default: true,
      base_url: 'https://api.openai.com/v1',
      api_key: 'sk-test-key',
      configuration: {
        model: 'text-embedding-ada-002',
      },
    }

    const mockResponse: EmbeddingProviderResponse = {
      id: 'provider-123',
      user_id: 'user-456',
      name: 'OpenAI Provider',
      provider_type: 'openai',
      model: 'text-embedding-3-small',
      is_default: true,
      base_url: 'https://api.openai.com/v1',
      configuration: '{"model":"text-embedding-ada-002"}',
      has_api_key: true,
      chunk_size: 1000,
      chunk_overlap: 200,
      created_at: '2023-01-01T00:00:00Z',
      updated_at: '2023-01-01T00:00:00Z',
    }

    it('should create embedding provider successfully', async () => {
      mockApiClient.post.mockResolvedValue(mockResponse)

      const result = await embeddingProviderService.createEmbeddingProvider(
        'team-1',
        mockRequest
      )

      expect(mockApiClient.post).toHaveBeenCalledWith(
        '/team-1/settings/embedding-providers',
        mockRequest
      )
      expect(result).toEqual(mockResponse)
    })

    it('should throw ApiError on authentication error', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/AUTH_REQUIRED',
        title: 'Unauthorized',
        status: 401,
        detail: 'Authentication token is invalid or expired',
        code: 'AUTH_REQUIRED',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.post.mockRejectedValue(new ApiError(errorResponse))

      await expect(
        embeddingProviderService.createEmbeddingProvider('team-1', mockRequest)
      ).rejects.toThrow(ApiError)
      await expect(
        embeddingProviderService.createEmbeddingProvider('team-1', mockRequest)
      ).rejects.toThrow('Authentication token is invalid or expired')
    })

    it('should throw ApiError with meaningful message on validation failure', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/VALIDATION_FAILED',
        title: 'Validation Failed',
        status: 400,
        detail: 'Invalid API key format',
        code: 'VALIDATION_FAILED',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.post.mockRejectedValue(new ApiError(errorResponse))

      await expect(
        embeddingProviderService.createEmbeddingProvider('team-1', mockRequest)
      ).rejects.toThrow('Invalid API key format')
    })

    it('should handle network errors', async () => {
      mockApiClient.post.mockRejectedValue(new Error('Network error'))

      await expect(
        embeddingProviderService.createEmbeddingProvider('team-1', mockRequest)
      ).rejects.toThrow('Network error')
    })
  })

  describe('getEmbeddingProviders', () => {
    const mockProviders: EmbeddingProviderResponse[] = [
      {
        id: 'provider-1',
        user_id: 'user-456',
        name: 'OpenAI Provider',
        provider_type: 'openai',
        model: 'text-embedding-3-small',
        is_default: true,
        base_url: 'https://api.openai.com/v1',
        configuration: '{}',
        has_api_key: true,
        chunk_size: 1000,
        chunk_overlap: 200,
        created_at: '2023-01-01T00:00:00Z',
        updated_at: '2023-01-01T00:00:00Z',
      },
      {
        id: 'provider-2',
        user_id: 'user-456',
        name: 'Ollama Provider',
        provider_type: 'ollama',
        model: 'text-embedding-3-small',
        is_default: false,
        base_url: 'http://localhost:11434',
        configuration: '{}',
        has_api_key: false,
        chunk_size: 1000,
        chunk_overlap: 200,
        created_at: '2023-01-02T00:00:00Z',
        updated_at: '2023-01-02T00:00:00Z',
      },
    ]

    it('should get embedding providers successfully', async () => {
      mockApiClient.get.mockResolvedValue(mockProviders)

      const result =
        await embeddingProviderService.getEmbeddingProviders('team-1')

      expect(mockApiClient.get).toHaveBeenCalledWith(
        '/team-1/settings/embedding-providers'
      )
      expect(result).toEqual(mockProviders)
    })

    it('should throw ApiError on authentication error', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/AUTH_REQUIRED',
        title: 'Unauthorized',
        status: 401,
        detail: 'Authentication token is invalid or expired',
        code: 'AUTH_REQUIRED',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.get.mockRejectedValue(new ApiError(errorResponse))

      await expect(
        embeddingProviderService.getEmbeddingProviders('team-1')
      ).rejects.toThrow(ApiError)
      await expect(
        embeddingProviderService.getEmbeddingProviders('team-1')
      ).rejects.toThrow('Authentication token is invalid or expired')
    })

    it('should throw ApiError on server error', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/INTERNAL_ERROR',
        title: 'Internal Server Error',
        status: 500,
        detail: 'Failed to retrieve embedding providers',
        code: 'INTERNAL_ERROR',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.get.mockRejectedValue(new ApiError(errorResponse))

      await expect(
        embeddingProviderService.getEmbeddingProviders('team-1')
      ).rejects.toThrow('Failed to retrieve embedding providers')
    })

    it('should handle network errors', async () => {
      mockApiClient.get.mockRejectedValue(new Error('Network error'))

      await expect(
        embeddingProviderService.getEmbeddingProviders('team-1')
      ).rejects.toThrow('Network error')
    })
  })

  describe('getEmbeddingProvider', () => {
    const providerId = 'provider-123'
    const mockProvider: EmbeddingProviderResponse = {
      id: providerId,
      user_id: 'user-456',
      name: 'OpenAI Provider',
      provider_type: 'openai',
      model: 'text-embedding-3-small',
      is_default: true,
      base_url: 'https://api.openai.com/v1',
      configuration: '{}',
      has_api_key: true,
      chunk_size: 1000,
      chunk_overlap: 200,
      created_at: '2023-01-01T00:00:00Z',
      updated_at: '2023-01-01T00:00:00Z',
    }

    it('should get embedding provider successfully', async () => {
      mockApiClient.get.mockResolvedValue(mockProvider)

      const result = await embeddingProviderService.getEmbeddingProvider(
        'team-1',
        providerId
      )

      expect(mockApiClient.get).toHaveBeenCalledWith(
        `/team-1/settings/embedding-providers/${providerId}`
      )
      expect(result).toEqual(mockProvider)
    })

    it('should throw ApiError on authentication error', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/AUTH_REQUIRED',
        title: 'Unauthorized',
        status: 401,
        detail: 'Authentication token is invalid or expired',
        code: 'AUTH_REQUIRED',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.get.mockRejectedValue(new ApiError(errorResponse))

      await expect(
        embeddingProviderService.getEmbeddingProvider('team-1', providerId)
      ).rejects.toThrow(ApiError)
      await expect(
        embeddingProviderService.getEmbeddingProvider('team-1', providerId)
      ).rejects.toThrow('Authentication token is invalid or expired')
    })

    it('should throw ApiError on not found error', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/RESOURCE_NOT_FOUND',
        title: 'Not Found',
        status: 404,
        detail: 'Embedding provider not found',
        code: 'RESOURCE_NOT_FOUND',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.get.mockRejectedValue(new ApiError(errorResponse))

      await expect(
        embeddingProviderService.getEmbeddingProvider('team-1', providerId)
      ).rejects.toThrow('Embedding provider not found')

      const error = (await embeddingProviderService
        .getEmbeddingProvider('team-1', providerId)
        .catch((e: unknown) => e)) as ApiError
      expect(error.isNotFoundError()).toBe(true)
    })

    it('should handle network errors', async () => {
      mockApiClient.get.mockRejectedValue(new Error('Network error'))

      await expect(
        embeddingProviderService.getEmbeddingProvider('team-1', providerId)
      ).rejects.toThrow('Network error')
    })
  })

  describe('updateEmbeddingProvider', () => {
    const providerId = 'provider-123'
    const mockRequest: UpdateEmbeddingProviderRequest = {
      name: 'Updated OpenAI Provider',
      is_default: false,
      configuration: {
        model: 'text-embedding-3-small',
      },
    }

    const mockResponse: EmbeddingProviderResponse = {
      id: providerId,
      user_id: 'user-456',
      name: 'Updated OpenAI Provider',
      provider_type: 'openai',
      model: 'text-embedding-3-small',
      is_default: false,
      base_url: 'https://api.openai.com/v1',
      configuration: '{"model":"text-embedding-3-small"}',
      has_api_key: true,
      chunk_size: 1000,
      chunk_overlap: 200,
      created_at: '2023-01-01T00:00:00Z',
      updated_at: '2023-01-01T10:00:00Z',
    }

    it('should update embedding provider successfully', async () => {
      mockApiClient.put.mockResolvedValue(mockResponse)

      const result = await embeddingProviderService.updateEmbeddingProvider(
        'team-1',
        providerId,
        mockRequest
      )

      expect(mockApiClient.put).toHaveBeenCalledWith(
        `/team-1/settings/embedding-providers/${providerId}`,
        mockRequest
      )
      expect(result).toEqual(mockResponse)
    })

    it('should throw ApiError on authentication error', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/AUTH_REQUIRED',
        title: 'Unauthorized',
        status: 401,
        detail: 'Authentication token is invalid or expired',
        code: 'AUTH_REQUIRED',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.put.mockRejectedValue(new ApiError(errorResponse))

      await expect(
        embeddingProviderService.updateEmbeddingProvider(
          'team-1',
          providerId,
          mockRequest
        )
      ).rejects.toThrow(ApiError)
      await expect(
        embeddingProviderService.updateEmbeddingProvider(
          'team-1',
          providerId,
          mockRequest
        )
      ).rejects.toThrow('Authentication token is invalid or expired')
    })

    it('should throw ApiError on not found error', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/RESOURCE_NOT_FOUND',
        title: 'Not Found',
        status: 404,
        detail: 'Embedding provider not found',
        code: 'RESOURCE_NOT_FOUND',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.put.mockRejectedValue(new ApiError(errorResponse))

      await expect(
        embeddingProviderService.updateEmbeddingProvider(
          'team-1',
          providerId,
          mockRequest
        )
      ).rejects.toThrow('Embedding provider not found')
    })

    it('should throw ApiError with meaningful message on validation failure', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/VALIDATION_FAILED',
        title: 'Validation Failed',
        status: 400,
        detail: 'Invalid configuration format',
        code: 'VALIDATION_FAILED',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.put.mockRejectedValue(new ApiError(errorResponse))

      await expect(
        embeddingProviderService.updateEmbeddingProvider(
          'team-1',
          providerId,
          mockRequest
        )
      ).rejects.toThrow('Invalid configuration format')
    })

    it('should handle network errors', async () => {
      mockApiClient.put.mockRejectedValue(new Error('Network error'))

      await expect(
        embeddingProviderService.updateEmbeddingProvider(
          'team-1',
          providerId,
          mockRequest
        )
      ).rejects.toThrow('Network error')
    })
  })

  describe('deleteEmbeddingProvider', () => {
    const providerId = 'provider-123'

    it('should delete embedding provider successfully', async () => {
      mockApiClient.delete.mockResolvedValue({})

      await expect(
        embeddingProviderService.deleteEmbeddingProvider('team-1', providerId)
      ).resolves.toBeUndefined()

      expect(mockApiClient.delete).toHaveBeenCalledWith(
        `/team-1/settings/embedding-providers/${providerId}`
      )
    })

    it('should throw ApiError on authentication error', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/AUTH_REQUIRED',
        title: 'Unauthorized',
        status: 401,
        detail: 'Authentication token is invalid or expired',
        code: 'AUTH_REQUIRED',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.delete.mockRejectedValue(new ApiError(errorResponse))

      await expect(
        embeddingProviderService.deleteEmbeddingProvider('team-1', providerId)
      ).rejects.toThrow(ApiError)
      await expect(
        embeddingProviderService.deleteEmbeddingProvider('team-1', providerId)
      ).rejects.toThrow('Authentication token is invalid or expired')
    })

    it('should throw ApiError on not found error', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/RESOURCE_NOT_FOUND',
        title: 'Not Found',
        status: 404,
        detail: 'Embedding provider not found',
        code: 'RESOURCE_NOT_FOUND',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.delete.mockRejectedValue(new ApiError(errorResponse))

      await expect(
        embeddingProviderService.deleteEmbeddingProvider('team-1', providerId)
      ).rejects.toThrow('Embedding provider not found')
    })

    it('should throw ApiError with meaningful message on failure', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/INTERNAL_ERROR',
        title: 'Internal Server Error',
        status: 500,
        detail: 'Failed to delete embedding provider due to database error',
        code: 'INTERNAL_ERROR',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.delete.mockRejectedValue(new ApiError(errorResponse))

      await expect(
        embeddingProviderService.deleteEmbeddingProvider('team-1', providerId)
      ).rejects.toThrow(
        'Failed to delete embedding provider due to database error'
      )
    })

    it('should handle network errors', async () => {
      mockApiClient.delete.mockRejectedValue(new Error('Network error'))

      await expect(
        embeddingProviderService.deleteEmbeddingProvider('team-1', providerId)
      ).rejects.toThrow('Network error')
    })
  })

  describe('validateEmbeddingProvider', () => {
    const mockRequest: ValidateEmbeddingProviderRequest = {
      provider_type: 'openai',
      model: 'text-embedding-3-small',
      base_url: 'https://api.openai.com/v1',
      api_key: 'sk-test-key',
      configuration: {
        model: 'text-embedding-ada-002',
      },
    }

    const mockResponse: ValidateEmbeddingProviderResponse = {
      is_valid: true,
      message: 'Provider configuration is valid',
      details: {
        response_time_ms: 150,
        status_code: 200,
      },
    }

    it('should validate embedding provider successfully', async () => {
      mockApiClient.post.mockResolvedValue(mockResponse)

      const result = await embeddingProviderService.validateEmbeddingProvider(
        'team-1',
        mockRequest
      )

      expect(mockApiClient.post).toHaveBeenCalledWith(
        '/team-1/settings/embedding-providers/validate',
        mockRequest
      )
      expect(result).toEqual(mockResponse)
    })

    it('should handle invalid provider configuration', async () => {
      const invalidResponse: ValidateEmbeddingProviderResponse = {
        is_valid: false,
        message: 'Invalid API key',
        details: {
          response_time_ms: 50,
          status_code: 401,
          error_details: 'Unauthorized',
        },
      }

      mockApiClient.post.mockResolvedValue(invalidResponse)

      const result = await embeddingProviderService.validateEmbeddingProvider(
        'team-1',
        mockRequest
      )

      expect(result).toEqual(invalidResponse)
      expect(result.is_valid).toBe(false)
    })

    it('should throw ApiError on authentication error', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/AUTH_REQUIRED',
        title: 'Unauthorized',
        status: 401,
        detail: 'Authentication token is invalid or expired',
        code: 'AUTH_REQUIRED',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.post.mockRejectedValue(new ApiError(errorResponse))

      await expect(
        embeddingProviderService.validateEmbeddingProvider(
          'team-1',
          mockRequest
        )
      ).rejects.toThrow(ApiError)
      await expect(
        embeddingProviderService.validateEmbeddingProvider(
          'team-1',
          mockRequest
        )
      ).rejects.toThrow('Authentication token is invalid or expired')
    })

    it('should throw ApiError with meaningful message on service unavailable', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/SERVICE_UNAVAILABLE',
        title: 'Service Unavailable',
        status: 503,
        detail: 'Validation service is temporarily unavailable',
        code: 'SERVICE_UNAVAILABLE',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.post.mockRejectedValue(new ApiError(errorResponse))

      await expect(
        embeddingProviderService.validateEmbeddingProvider(
          'team-1',
          mockRequest
        )
      ).rejects.toThrow('Validation service is temporarily unavailable')
    })

    it('should handle network errors', async () => {
      mockApiClient.post.mockRejectedValue(new Error('Network error'))

      await expect(
        embeddingProviderService.validateEmbeddingProvider(
          'team-1',
          mockRequest
        )
      ).rejects.toThrow('Network error')
    })
  })

  describe('edge cases and error handling', () => {
    it('should handle malformed JSON response in createEmbeddingProvider', async () => {
      mockApiClient.post.mockRejectedValue(new Error('Invalid JSON'))

      const mockRequest: CreateEmbeddingProviderRequest = {
        name: 'Test Provider',
        provider_type: 'openai',
        model: 'text-embedding-3-small',
      }

      await expect(
        embeddingProviderService.createEmbeddingProvider('team-1', mockRequest)
      ).rejects.toThrow('Invalid JSON')
    })

    it('should handle malformed JSON response in getEmbeddingProviders', async () => {
      mockApiClient.get.mockRejectedValue(new Error('Invalid JSON'))

      await expect(
        embeddingProviderService.getEmbeddingProviders('team-1')
      ).rejects.toThrow('Invalid JSON')
    })

    it('should handle malformed JSON response in validateEmbeddingProvider', async () => {
      const mockRequest: ValidateEmbeddingProviderRequest = {
        provider_type: 'openai',
        model: 'text-embedding-3-small',
        base_url: 'https://api.openai.com/v1',
      }

      mockApiClient.post.mockRejectedValue(new Error('Invalid JSON'))

      await expect(
        embeddingProviderService.validateEmbeddingProvider(
          'team-1',
          mockRequest
        )
      ).rejects.toThrow('Invalid JSON')
    })
  })

  describe('integration scenarios', () => {
    it('should handle complete provider lifecycle', async () => {
      const createRequest: CreateEmbeddingProviderRequest = {
        name: 'Test Provider',
        provider_type: 'openai',
        model: 'text-embedding-3-small',
        api_key: 'sk-test',
      }

      const createdProvider: EmbeddingProviderResponse = {
        id: 'provider-123',
        user_id: 'user-456',
        name: 'Test Provider',
        provider_type: 'openai',
        model: 'text-embedding-3-small',
        is_default: false,
        configuration: '{}',
        has_api_key: true,
        chunk_size: 1000,
        chunk_overlap: 200,
        created_at: '2023-01-01T00:00:00Z',
        updated_at: '2023-01-01T00:00:00Z',
      }

      // Create provider
      mockApiClient.post.mockResolvedValueOnce(createdProvider)

      const created = await embeddingProviderService.createEmbeddingProvider(
        'team-1',
        createRequest
      )
      expect(created).toEqual(createdProvider)

      // Get provider
      mockApiClient.get.mockResolvedValueOnce(createdProvider)

      const retrieved = await embeddingProviderService.getEmbeddingProvider(
        'team-1',
        created.id
      )
      expect(retrieved).toEqual(createdProvider)

      // Update provider
      const updateRequest: UpdateEmbeddingProviderRequest = {
        name: 'Updated Test Provider',
      }
      const updatedProvider = {
        ...createdProvider,
        name: 'Updated Test Provider',
      }

      mockApiClient.put.mockResolvedValueOnce(updatedProvider)

      const updated = await embeddingProviderService.updateEmbeddingProvider(
        'team-1',
        created.id,
        updateRequest
      )
      expect(updated.name).toBe('Updated Test Provider')

      // Delete provider
      mockApiClient.delete.mockResolvedValueOnce({})

      await expect(
        embeddingProviderService.deleteEmbeddingProvider('team-1', created.id)
      ).resolves.toBeUndefined()
    })

    it('should handle validation before creation', async () => {
      const validateRequest: ValidateEmbeddingProviderRequest = {
        provider_type: 'openai',
        model: 'text-embedding-3-small',
        base_url: 'https://api.openai.com/v1',
        api_key: 'sk-test',
      }

      const validationResponse: ValidateEmbeddingProviderResponse = {
        is_valid: true,
        message: 'Configuration is valid',
      }

      // Validate first
      mockApiClient.post.mockResolvedValueOnce(validationResponse)

      const validation =
        await embeddingProviderService.validateEmbeddingProvider(
          'team-1',
          validateRequest
        )
      expect(validation.is_valid).toBe(true)

      // Then create if valid
      if (validation.is_valid) {
        const createRequest: CreateEmbeddingProviderRequest = {
          name: 'Validated Provider',
          provider_type: validateRequest.provider_type,
          model: validateRequest.model,
          base_url: validateRequest.base_url,
          api_key: validateRequest.api_key,
        }

        const createdProvider: EmbeddingProviderResponse = {
          id: 'provider-456',
          user_id: 'user-789',
          name: 'Validated Provider',
          provider_type: 'openai',
          model: 'text-embedding-3-small',
          is_default: false,
          base_url: 'https://api.openai.com/v1',
          configuration: '{}',
          has_api_key: true,
          chunk_size: 1000,
          chunk_overlap: 200,
          created_at: '2023-01-01T00:00:00Z',
          updated_at: '2023-01-01T00:00:00Z',
        }

        mockApiClient.post.mockResolvedValueOnce(createdProvider)

        const created = await embeddingProviderService.createEmbeddingProvider(
          'team-1',
          createRequest
        )
        expect(created).toEqual(createdProvider)
      }
    })
  })
})
