import type {
  APIKey,
  CreateAPIKeyRequest,
  CreateAPIKeyResponse,
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

// Import the apiKeyService after mocking
import { apiKeyService } from '../apiKeyService'

describe('ApiKeyService', () => {
  const mockAPIKey: APIKey = {
    id: 'test-api-key-id',
    user_id: 'test-user-id',
    name: 'Test API Key',
    key_prefix: 'vxk_test_',
    integrations: ['ai_tools', 'cli'],
    is_legacy: false,
    last_used_at: null,
    created_at: '2023-01-01T00:00:00Z',
    updated_at: '2023-01-01T00:00:00Z',
  }

  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('createAPIKey', () => {
    const mockRequest: CreateAPIKeyRequest = {
      name: 'Test Key',
      integration_codes: ['ai_tools', 'cli', 'mcp_server', 'marketplace'],
    }
    const mockResponse: CreateAPIKeyResponse = {
      api_key: mockAPIKey,
      full_key: 'vxp_test_full_key_value',
      key_prefix: 'vxp_test_',
    }

    it('should create API key successfully', async () => {
      mockApiClient.post.mockResolvedValue(mockResponse)

      const result = await apiKeyService.createAPIKey(mockRequest)

      expect(mockApiClient.post).toHaveBeenCalledWith(
        '/settings/api-keys',
        mockRequest
      )
      expect(result).toEqual(mockResponse)
    })

    it('should throw ApiError with meaningful message on failure', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/API_KEY_ALREADY_EXISTS',
        title: 'Conflict',
        status: 409,
        detail: "API key with name 'Test Key' already exists",
        code: 'API_KEY_ALREADY_EXISTS',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.post.mockRejectedValue(new ApiError(errorResponse))

      await expect(apiKeyService.createAPIKey(mockRequest)).rejects.toThrow(
        ApiError
      )
      await expect(apiKeyService.createAPIKey(mockRequest)).rejects.toThrow(
        "API key with name 'Test Key' already exists"
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
      mockApiClient.post.mockRejectedValue(new ApiError(errorResponse))

      await expect(apiKeyService.createAPIKey(mockRequest)).rejects.toThrow(
        ApiError
      )

      const error = (await apiKeyService
        .createAPIKey(mockRequest)
        .catch((e: unknown) => e)) as ApiError
      expect(error.isAuthError()).toBe(true)
    })

    it('should handle network errors', async () => {
      mockApiClient.post.mockRejectedValue(new Error('Network Error'))

      await expect(apiKeyService.createAPIKey(mockRequest)).rejects.toThrow(
        'Network Error'
      )
    })

    it('should handle validation errors with field details', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/VALIDATION_FAILED',
        title: 'Validation Failed',
        status: 400,
        detail: 'Validation failed',
        code: 'VALIDATION_FAILED',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
        validation_errors: [
          {
            field: 'name',
            message: 'Name cannot be empty',
            code: 'REQUIRED',
          },
        ],
      }
      mockApiClient.post.mockRejectedValue(new ApiError(errorResponse))

      const error = (await apiKeyService
        .createAPIKey({ name: '', integration_codes: [] })
        .catch((e: unknown) => e)) as ApiError

      expect(error.isValidationError()).toBe(true)
      expect(error.validationErrors).toHaveLength(1)
      expect(error.getFieldErrors('name')).toHaveLength(1)
    })

    it('should handle JSON parsing errors', async () => {
      mockApiClient.post.mockRejectedValue(new Error('Invalid JSON'))

      await expect(apiKeyService.createAPIKey(mockRequest)).rejects.toThrow(
        'Invalid JSON'
      )
    })
  })

  describe('getAPIKeys', () => {
    const mockAPIKeys: APIKey[] = [
      mockAPIKey,
      {
        ...mockAPIKey,
        id: 'test-api-key-id-2',
        name: 'Another Test Key',
        key_prefix: 'vxp_test2_',
      },
    ]

    it('should get API keys successfully', async () => {
      mockApiClient.get.mockResolvedValue(mockAPIKeys)

      const result = await apiKeyService.getAPIKeys()

      expect(mockApiClient.get).toHaveBeenCalledWith('/settings/api-keys')
      expect(result).toEqual(mockAPIKeys)
    })

    it('should return empty array when no keys exist', async () => {
      mockApiClient.get.mockResolvedValue([])

      const result = await apiKeyService.getAPIKeys()

      expect(result).toEqual([])
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

      await expect(apiKeyService.getAPIKeys()).rejects.toThrow(ApiError)
      await expect(apiKeyService.getAPIKeys()).rejects.toThrow(
        'Authentication token is invalid or expired'
      )
    })

    it('should throw ApiError on server error', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/INTERNAL_ERROR',
        title: 'Internal Server Error',
        status: 500,
        detail: 'Failed to retrieve API keys',
        code: 'INTERNAL_ERROR',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.get.mockRejectedValue(new ApiError(errorResponse))

      await expect(apiKeyService.getAPIKeys()).rejects.toThrow(ApiError)
      await expect(apiKeyService.getAPIKeys()).rejects.toThrow(
        'Failed to retrieve API keys'
      )
    })

    it('should handle network errors', async () => {
      mockApiClient.get.mockRejectedValue(new Error('Network Error'))

      await expect(apiKeyService.getAPIKeys()).rejects.toThrow('Network Error')
    })

    it('should handle JSON parsing errors', async () => {
      mockApiClient.get.mockRejectedValue(new Error('Invalid JSON'))

      await expect(apiKeyService.getAPIKeys()).rejects.toThrow('Invalid JSON')
    })
  })

  describe('deleteAPIKey', () => {
    const testApiKeyId = 'test-api-key-id'

    it('should delete API key successfully', async () => {
      mockApiClient.delete.mockResolvedValue({})

      await apiKeyService.deleteAPIKey(testApiKeyId)

      expect(mockApiClient.delete).toHaveBeenCalledWith(
        `/settings/api-keys/${testApiKeyId}`
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

      await expect(apiKeyService.deleteAPIKey(testApiKeyId)).rejects.toThrow(
        ApiError
      )
      await expect(apiKeyService.deleteAPIKey(testApiKeyId)).rejects.toThrow(
        'Authentication token is invalid or expired'
      )
    })

    it('should throw ApiError on not found error', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/RESOURCE_NOT_FOUND',
        title: 'Not Found',
        status: 404,
        detail: 'API key not found',
        code: 'RESOURCE_NOT_FOUND',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.delete.mockRejectedValue(new ApiError(errorResponse))

      await expect(apiKeyService.deleteAPIKey(testApiKeyId)).rejects.toThrow(
        ApiError
      )
      await expect(apiKeyService.deleteAPIKey(testApiKeyId)).rejects.toThrow(
        'API key not found'
      )

      const error = (await apiKeyService
        .deleteAPIKey(testApiKeyId)
        .catch((e: unknown) => e)) as ApiError
      expect(error.isNotFoundError()).toBe(true)
    })

    it('should throw ApiError on server error', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/INTERNAL_ERROR',
        title: 'Internal Server Error',
        status: 500,
        detail: 'Failed to delete API key',
        code: 'INTERNAL_ERROR',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.delete.mockRejectedValue(new ApiError(errorResponse))

      await expect(apiKeyService.deleteAPIKey(testApiKeyId)).rejects.toThrow(
        ApiError
      )
      await expect(apiKeyService.deleteAPIKey(testApiKeyId)).rejects.toThrow(
        'Failed to delete API key'
      )
    })

    it('should handle network errors', async () => {
      mockApiClient.delete.mockRejectedValue(new Error('Network Error'))

      await expect(apiKeyService.deleteAPIKey(testApiKeyId)).rejects.toThrow(
        'Network Error'
      )
    })

    it('should properly handle special characters in API key ID', async () => {
      const specialKeyId = 'test key with spaces & symbols'
      mockApiClient.delete.mockResolvedValue({})

      await apiKeyService.deleteAPIKey(specialKeyId)

      expect(mockApiClient.delete).toHaveBeenCalledWith(
        `/settings/api-keys/${specialKeyId}`
      )
    })
  })

  describe('Security and Edge Cases', () => {
    it('should handle concurrent API key operations', async () => {
      const mockRequest: CreateAPIKeyRequest = {
        name: 'Concurrent Test',
        integration_codes: ['ai_tools', 'cli', 'mcp_server', 'marketplace'],
      }
      const mockResponse: CreateAPIKeyResponse = {
        api_key: mockAPIKey,
        full_key: 'vxp_concurrent_key',
        key_prefix: 'vxp_test_',
      }

      mockApiClient.post.mockResolvedValue(mockResponse)

      const promises = [
        apiKeyService.createAPIKey(mockRequest),
        apiKeyService.createAPIKey(mockRequest),
        apiKeyService.createAPIKey(mockRequest),
      ]

      const results = await Promise.all(promises)

      expect(results).toHaveLength(3)
      expect(mockApiClient.post).toHaveBeenCalledTimes(3)
      results.forEach(result => {
        expect(result).toEqual(mockResponse)
      })
    })
  })

  describe('Integration Scenarios', () => {
    it('should handle complete API key lifecycle', async () => {
      const createRequest: CreateAPIKeyRequest = {
        name: 'Integration Test Key',
        integration_codes: ['ai_tools', 'cli', 'mcp_server', 'marketplace'],
      }
      const createResponse: CreateAPIKeyResponse = {
        api_key: mockAPIKey,
        full_key: 'vxp_integration_full_key',
        key_prefix: 'vxp_int_',
      }

      // Step 1: Create API key
      mockApiClient.post.mockResolvedValueOnce(createResponse)

      const createdKey = await apiKeyService.createAPIKey(createRequest)
      expect(createdKey).toEqual(createResponse)

      // Step 2: List API keys
      const listResponse: APIKey[] = [mockAPIKey]
      mockApiClient.get.mockResolvedValueOnce(listResponse)

      const keys = await apiKeyService.getAPIKeys()
      expect(keys).toEqual(listResponse)

      // Step 3: Delete API key
      mockApiClient.delete.mockResolvedValueOnce({})

      await apiKeyService.deleteAPIKey(mockAPIKey.id)
      expect(mockApiClient.delete).toHaveBeenCalledWith(
        `/settings/api-keys/${mockAPIKey.id}`
      )
    })
  })

  describe('API Endpoint Validation', () => {
    it('should call the correct endpoint for createAPIKey', async () => {
      mockApiClient.post.mockResolvedValue({
        api_key: mockAPIKey,
        full_key: 'test',
        key_prefix: 'test_',
      })

      await apiKeyService.createAPIKey({
        name: 'Test',
        integration_codes: ['ai_tools', 'cli', 'mcp_server', 'marketplace'],
      })

      expect(mockApiClient.post).toHaveBeenCalledWith(
        '/settings/api-keys',
        expect.any(Object)
      )
    })

    it('should call the correct endpoint for getAPIKeys', async () => {
      mockApiClient.get.mockResolvedValue([])

      await apiKeyService.getAPIKeys()

      expect(mockApiClient.get).toHaveBeenCalledWith('/settings/api-keys')
    })

    it('should call the correct endpoint for deleteAPIKey', async () => {
      mockApiClient.delete.mockResolvedValue({})

      await apiKeyService.deleteAPIKey('test-id')

      expect(mockApiClient.delete).toHaveBeenCalledWith(
        '/settings/api-keys/test-id'
      )
    })
  })
})
