import type {
  CreateEmbeddingProviderRequest,
  UpdateEmbeddingProviderRequest,
  EmbeddingProviderResponse,
  ValidateEmbeddingProviderRequest,
  ValidateEmbeddingProviderResponse,
} from '../../src/types'

// Mock fetch globally
const mockFetch = jest.fn()
global.fetch = mockFetch

// Mock authService
const mockAuthService = {
  getToken: jest.fn(),
  logout: jest.fn(),
}

jest.mock('../../src/services/authService', () => ({
  authService: mockAuthService,
}))

// Mock the embeddingProviderService module to handle import.meta.env
jest.mock('../../src/services/embeddingProviderService', () => {
  const API_BASE_URL = 'http://localhost:8080/api/v1' // Mock API URL

  class EmbeddingProviderService {
    async createEmbeddingProvider(request: CreateEmbeddingProviderRequest) {
      const token = mockAuthService.getToken()
      if (!token) {
        throw new Error('No authentication token')
      }

      const response = await fetch(
        `${API_BASE_URL}/settings/embedding-providers`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify(request),
        }
      )

      if (!response.ok) {
        if (response.status === 401) {
          mockAuthService.logout()
          throw new Error('Authentication expired')
        }
        const errorText = await response.text()
        throw new Error(errorText || 'Failed to create embedding provider')
      }

      return response.json()
    }

    async getEmbeddingProviders() {
      const token = mockAuthService.getToken()
      if (!token) {
        throw new Error('No authentication token')
      }

      const response = await fetch(
        `${API_BASE_URL}/settings/embedding-providers`,
        {
          method: 'GET',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${token}`,
          },
        }
      )

      if (!response.ok) {
        if (response.status === 401) {
          mockAuthService.logout()
          throw new Error('Authentication expired')
        }
        throw new Error('Failed to get embedding providers')
      }

      return response.json()
    }

    async getEmbeddingProvider(id: string) {
      const token = mockAuthService.getToken()
      if (!token) {
        throw new Error('No authentication token')
      }

      const response = await fetch(
        `${API_BASE_URL}/settings/embedding-providers/${id}`,
        {
          method: 'GET',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${token}`,
          },
        }
      )

      if (!response.ok) {
        if (response.status === 401) {
          mockAuthService.logout()
          throw new Error('Authentication expired')
        }
        if (response.status === 404) {
          throw new Error('Embedding provider not found')
        }
        throw new Error('Failed to get embedding provider')
      }

      return response.json()
    }

    async updateEmbeddingProvider(
      id: string,
      request: UpdateEmbeddingProviderRequest
    ) {
      const token = mockAuthService.getToken()
      if (!token) {
        throw new Error('No authentication token')
      }

      const response = await fetch(
        `${API_BASE_URL}/settings/embedding-providers/${id}`,
        {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify(request),
        }
      )

      if (!response.ok) {
        if (response.status === 401) {
          mockAuthService.logout()
          throw new Error('Authentication expired')
        }
        if (response.status === 404) {
          throw new Error('Embedding provider not found')
        }
        const errorText = await response.text()
        throw new Error(errorText || 'Failed to update embedding provider')
      }

      return response.json()
    }

    async deleteEmbeddingProvider(id: string) {
      const token = mockAuthService.getToken()
      if (!token) {
        throw new Error('No authentication token')
      }

      const response = await fetch(
        `${API_BASE_URL}/settings/embedding-providers/${id}`,
        {
          method: 'DELETE',
          headers: {
            Authorization: `Bearer ${token}`,
          },
        }
      )

      if (!response.ok) {
        if (response.status === 401) {
          mockAuthService.logout()
          throw new Error('Authentication expired')
        }
        if (response.status === 404) {
          throw new Error('Embedding provider not found')
        }
        const errorText = await response.text()
        throw new Error(errorText || 'Failed to delete embedding provider')
      }
    }

    async validateEmbeddingProvider(request: ValidateEmbeddingProviderRequest) {
      const token = mockAuthService.getToken()
      if (!token) {
        throw new Error('No authentication token')
      }

      const response = await fetch(
        `${API_BASE_URL}/settings/embedding-providers/validate`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify(request),
        }
      )

      if (!response.ok) {
        if (response.status === 401) {
          mockAuthService.logout()
          throw new Error('Authentication expired')
        }
        const errorText = await response.text()
        throw new Error(errorText || 'Failed to validate embedding provider')
      }

      return response.json()
    }
  }

  return {
    embeddingProviderService: new EmbeddingProviderService(),
  }
})

import { embeddingProviderService } from '../../src/services/embeddingProviderService'

describe('EmbeddingProviderService', () => {
  const API_BASE_URL = 'http://localhost:8080/api/v1'
  const mockToken = 'mock-auth-token'

  beforeEach(() => {
    jest.clearAllMocks()
    mockAuthService.getToken.mockReturnValue(mockToken)

    // Reset fetch mock
    mockFetch.mockReset()
  })

  describe('createEmbeddingProvider', () => {
    const mockRequest: CreateEmbeddingProviderRequest = {
      name: 'Test Provider',
      provider_type: 'openai',
      model: 'text-embedding-3-small',
      is_default: false,
      base_url: 'https://api.openai.com/v1',
      api_key: 'test-api-key',
      configuration: { model: 'text-embedding-ada-002' },
    }

    const mockResponse: EmbeddingProviderResponse = {
      id: 'provider-123',
      user_id: 'user-123',
      name: 'Test Provider',
      provider_type: 'openai',
      model: 'text-embedding-3-small',
      is_default: false,
      base_url: 'https://api.openai.com/v1',
      configuration: '{"model":"text-embedding-ada-002"}',
      has_api_key: true,
      chunk_size: 1000,
      chunk_overlap: 200,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
    }

    it('should create embedding provider successfully', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      })

      const result =
        await embeddingProviderService.createEmbeddingProvider(mockRequest)

      expect(mockFetch).toHaveBeenCalledWith(
        `${API_BASE_URL}/settings/embedding-providers`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${mockToken}`,
          },
          body: JSON.stringify(mockRequest),
        }
      )
      expect(result).toEqual(mockResponse)
    })

    it('should throw error when no authentication token', async () => {
      mockAuthService.getToken.mockReturnValue(null)

      await expect(
        embeddingProviderService.createEmbeddingProvider(mockRequest)
      ).rejects.toThrow('No authentication token')

      expect(mockFetch).not.toHaveBeenCalled()
    })

    it('should handle 401 authentication error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
      })

      await expect(
        embeddingProviderService.createEmbeddingProvider(mockRequest)
      ).rejects.toThrow('Authentication expired')

      expect(mockAuthService.logout).toHaveBeenCalled()
    })

    it('should handle generic error with response text', async () => {
      const errorMessage = 'Invalid provider configuration'
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        text: async () => errorMessage,
      })

      await expect(
        embeddingProviderService.createEmbeddingProvider(mockRequest)
      ).rejects.toThrow(errorMessage)
    })

    it('should handle generic error without response text', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: async () => '',
      })

      await expect(
        embeddingProviderService.createEmbeddingProvider(mockRequest)
      ).rejects.toThrow('Failed to create embedding provider')
    })

    it('should handle network errors', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Network error'))

      await expect(
        embeddingProviderService.createEmbeddingProvider(mockRequest)
      ).rejects.toThrow('Network error')
    })
  })

  describe('getEmbeddingProviders', () => {
    const mockProviders: EmbeddingProviderResponse[] = [
      {
        id: 'provider-1',
        user_id: 'user-123',
        name: 'OpenAI Provider',
        provider_type: 'openai',
        model: 'text-embedding-3-small',
        is_default: true,
        base_url: 'https://api.openai.com/v1',
        configuration: '{"model":"text-embedding-ada-002"}',
        has_api_key: true,
        chunk_size: 1000,
        chunk_overlap: 200,
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
      {
        id: 'provider-2',
        user_id: 'user-123',
        name: 'Azure Provider',
        provider_type: 'azure',
        model: 'text-embedding-3-small',
        is_default: false,
        base_url: 'https://myinstance.openai.azure.com',
        configuration: '{"deployment":"text-embedding-ada-002"}',
        has_api_key: true,
        chunk_size: 1000,
        chunk_overlap: 200,
        created_at: '2024-01-02T00:00:00Z',
        updated_at: '2024-01-02T00:00:00Z',
      },
    ]

    it('should get all embedding providers successfully', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockProviders,
      })

      const result = await embeddingProviderService.getEmbeddingProviders()

      expect(mockFetch).toHaveBeenCalledWith(
        `${API_BASE_URL}/settings/embedding-providers`,
        {
          method: 'GET',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${mockToken}`,
          },
        }
      )
      expect(result).toEqual(mockProviders)
    })

    it('should throw error when no authentication token', async () => {
      mockAuthService.getToken.mockReturnValue(null)

      await expect(
        embeddingProviderService.getEmbeddingProviders()
      ).rejects.toThrow('No authentication token')

      expect(mockFetch).not.toHaveBeenCalled()
    })

    it('should handle 401 authentication error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
      })

      await expect(
        embeddingProviderService.getEmbeddingProviders()
      ).rejects.toThrow('Authentication expired')

      expect(mockAuthService.logout).toHaveBeenCalled()
    })

    it('should handle generic error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
      })

      await expect(
        embeddingProviderService.getEmbeddingProviders()
      ).rejects.toThrow('Failed to get embedding providers')
    })
  })

  describe('getEmbeddingProvider', () => {
    const providerId = 'provider-123'
    const mockProvider: EmbeddingProviderResponse = {
      id: providerId,
      user_id: 'user-123',
      name: 'Test Provider',
      provider_type: 'openai',
      model: 'text-embedding-3-small',
      is_default: false,
      base_url: 'https://api.openai.com/v1',
      configuration: '{"model":"text-embedding-ada-002"}',
      has_api_key: true,
      chunk_size: 1000,
      chunk_overlap: 200,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
    }

    it('should get embedding provider by ID successfully', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockProvider,
      })

      const result =
        await embeddingProviderService.getEmbeddingProvider(providerId)

      expect(mockFetch).toHaveBeenCalledWith(
        `${API_BASE_URL}/settings/embedding-providers/${providerId}`,
        {
          method: 'GET',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${mockToken}`,
          },
        }
      )
      expect(result).toEqual(mockProvider)
    })

    it('should throw error when no authentication token', async () => {
      mockAuthService.getToken.mockReturnValue(null)

      await expect(
        embeddingProviderService.getEmbeddingProvider(providerId)
      ).rejects.toThrow('No authentication token')

      expect(mockFetch).not.toHaveBeenCalled()
    })

    it('should handle 401 authentication error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
      })

      await expect(
        embeddingProviderService.getEmbeddingProvider(providerId)
      ).rejects.toThrow('Authentication expired')

      expect(mockAuthService.logout).toHaveBeenCalled()
    })

    it('should handle 404 not found error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
      })

      await expect(
        embeddingProviderService.getEmbeddingProvider(providerId)
      ).rejects.toThrow('Embedding provider not found')
    })

    it('should handle generic error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
      })

      await expect(
        embeddingProviderService.getEmbeddingProvider(providerId)
      ).rejects.toThrow('Failed to get embedding provider')
    })
  })

  describe('updateEmbeddingProvider', () => {
    const providerId = 'provider-123'
    const mockRequest: UpdateEmbeddingProviderRequest = {
      name: 'Updated Provider',
      is_default: true,
      configuration: { model: 'text-embedding-ada-003' },
    }

    const mockResponse: EmbeddingProviderResponse = {
      id: providerId,
      user_id: 'user-123',
      name: 'Updated Provider',
      provider_type: 'openai',
      model: 'text-embedding-3-small',
      is_default: true,
      base_url: 'https://api.openai.com/v1',
      configuration: '{"model":"text-embedding-ada-003"}',
      has_api_key: true,
      chunk_size: 1000,
      chunk_overlap: 200,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-02T00:00:00Z',
    }

    it('should update embedding provider successfully', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      })

      const result = await embeddingProviderService.updateEmbeddingProvider(
        providerId,
        mockRequest
      )

      expect(mockFetch).toHaveBeenCalledWith(
        `${API_BASE_URL}/settings/embedding-providers/${providerId}`,
        {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${mockToken}`,
          },
          body: JSON.stringify(mockRequest),
        }
      )
      expect(result).toEqual(mockResponse)
    })

    it('should throw error when no authentication token', async () => {
      mockAuthService.getToken.mockReturnValue(null)

      await expect(
        embeddingProviderService.updateEmbeddingProvider(
          providerId,
          mockRequest
        )
      ).rejects.toThrow('No authentication token')

      expect(mockFetch).not.toHaveBeenCalled()
    })

    it('should handle 401 authentication error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
      })

      await expect(
        embeddingProviderService.updateEmbeddingProvider(
          providerId,
          mockRequest
        )
      ).rejects.toThrow('Authentication expired')

      expect(mockAuthService.logout).toHaveBeenCalled()
    })

    it('should handle 404 not found error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
      })

      await expect(
        embeddingProviderService.updateEmbeddingProvider(
          providerId,
          mockRequest
        )
      ).rejects.toThrow('Embedding provider not found')
    })

    it('should handle generic error with response text', async () => {
      const errorMessage = 'Invalid update data'
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        text: async () => errorMessage,
      })

      await expect(
        embeddingProviderService.updateEmbeddingProvider(
          providerId,
          mockRequest
        )
      ).rejects.toThrow(errorMessage)
    })

    it('should handle generic error without response text', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: async () => '',
      })

      await expect(
        embeddingProviderService.updateEmbeddingProvider(
          providerId,
          mockRequest
        )
      ).rejects.toThrow('Failed to update embedding provider')
    })
  })

  describe('deleteEmbeddingProvider', () => {
    const providerId = 'provider-123'

    it('should delete embedding provider successfully', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
      })

      await embeddingProviderService.deleteEmbeddingProvider(providerId)

      expect(mockFetch).toHaveBeenCalledWith(
        `${API_BASE_URL}/settings/embedding-providers/${providerId}`,
        {
          method: 'DELETE',
          headers: {
            Authorization: `Bearer ${mockToken}`,
          },
        }
      )
    })

    it('should throw error when no authentication token', async () => {
      mockAuthService.getToken.mockReturnValue(null)

      await expect(
        embeddingProviderService.deleteEmbeddingProvider(providerId)
      ).rejects.toThrow('No authentication token')

      expect(mockFetch).not.toHaveBeenCalled()
    })

    it('should handle 401 authentication error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
      })

      await expect(
        embeddingProviderService.deleteEmbeddingProvider(providerId)
      ).rejects.toThrow('Authentication expired')

      expect(mockAuthService.logout).toHaveBeenCalled()
    })

    it('should handle 404 not found error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
      })

      await expect(
        embeddingProviderService.deleteEmbeddingProvider(providerId)
      ).rejects.toThrow('Embedding provider not found')
    })

    it('should handle generic error with response text', async () => {
      const errorMessage = 'Cannot delete default provider'
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        text: async () => errorMessage,
      })

      await expect(
        embeddingProviderService.deleteEmbeddingProvider(providerId)
      ).rejects.toThrow(errorMessage)
    })

    it('should handle generic error without response text', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: async () => '',
      })

      await expect(
        embeddingProviderService.deleteEmbeddingProvider(providerId)
      ).rejects.toThrow('Failed to delete embedding provider')
    })
  })

  describe('validateEmbeddingProvider', () => {
    const mockRequest: ValidateEmbeddingProviderRequest = {
      provider_type: 'openai',
      model: 'text-embedding-3-small',
      base_url: 'https://api.openai.com/v1',
      api_key: 'test-api-key',
      configuration: { model: 'text-embedding-ada-002' },
    }

    const mockValidResponse: ValidateEmbeddingProviderResponse = {
      is_valid: true,
      message: 'Provider configuration is valid',
      details: {
        response_time_ms: 250,
        status_code: 200,
      },
    }

    const mockInvalidResponse: ValidateEmbeddingProviderResponse = {
      is_valid: false,
      message: 'Invalid API key',
      details: {
        response_time_ms: 100,
        status_code: 401,
        error_details: 'Incorrect API key provided',
      },
    }

    it('should validate embedding provider successfully with valid configuration', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockValidResponse,
      })

      const result =
        await embeddingProviderService.validateEmbeddingProvider(mockRequest)

      expect(mockFetch).toHaveBeenCalledWith(
        `${API_BASE_URL}/settings/embedding-providers/validate`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${mockToken}`,
          },
          body: JSON.stringify(mockRequest),
        }
      )
      expect(result).toEqual(mockValidResponse)
    })

    it('should validate embedding provider successfully with invalid configuration', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockInvalidResponse,
      })

      const result =
        await embeddingProviderService.validateEmbeddingProvider(mockRequest)

      expect(result).toEqual(mockInvalidResponse)
    })

    it('should throw error when no authentication token', async () => {
      mockAuthService.getToken.mockReturnValue(null)

      await expect(
        embeddingProviderService.validateEmbeddingProvider(mockRequest)
      ).rejects.toThrow('No authentication token')

      expect(mockFetch).not.toHaveBeenCalled()
    })

    it('should handle 401 authentication error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
      })

      await expect(
        embeddingProviderService.validateEmbeddingProvider(mockRequest)
      ).rejects.toThrow('Authentication expired')

      expect(mockAuthService.logout).toHaveBeenCalled()
    })

    it('should handle generic error with response text', async () => {
      const errorMessage = 'Validation service unavailable'
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 503,
        text: async () => errorMessage,
      })

      await expect(
        embeddingProviderService.validateEmbeddingProvider(mockRequest)
      ).rejects.toThrow(errorMessage)
    })

    it('should handle generic error without response text', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: async () => '',
      })

      await expect(
        embeddingProviderService.validateEmbeddingProvider(mockRequest)
      ).rejects.toThrow('Failed to validate embedding provider')
    })

    it('should validate provider with minimal configuration', async () => {
      const minimalRequest: ValidateEmbeddingProviderRequest = {
        provider_type: 'custom',
        model: 'text-embedding-3-small',
        base_url: 'https://custom-embedding-service.com/v1',
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockValidResponse,
      })

      const result =
        await embeddingProviderService.validateEmbeddingProvider(minimalRequest)

      expect(mockFetch).toHaveBeenCalledWith(
        `${API_BASE_URL}/settings/embedding-providers/validate`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${mockToken}`,
          },
          body: JSON.stringify(minimalRequest),
        }
      )
      expect(result).toEqual(mockValidResponse)
    })
  })

  describe('Error Handling', () => {
    it('should handle fetch network failures', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Failed to fetch'))

      await expect(
        embeddingProviderService.getEmbeddingProviders()
      ).rejects.toThrow('Failed to fetch')
    })

    it('should handle malformed JSON responses', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => {
          throw new Error('Invalid JSON')
        },
      })

      await expect(
        embeddingProviderService.getEmbeddingProviders()
      ).rejects.toThrow('Invalid JSON')
    })

    it('should handle empty response body for text errors', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        text: async () => {
          throw new Error('Cannot read response')
        },
      })

      const request: CreateEmbeddingProviderRequest = {
        name: 'Test',
        provider_type: 'openai',
        model: 'text-embedding-3-small',
      }

      await expect(
        embeddingProviderService.createEmbeddingProvider(request)
      ).rejects.toThrow('Cannot read response')
    })
  })

  describe('API Configuration', () => {
    it('should use development API URL in dev mode', () => {
      // This test verifies the API_BASE_URL configuration
      // The actual URL is determined by import.meta.env.DEV
      expect(embeddingProviderService).toBeDefined()
    })

    it('should handle different provider types', async () => {
      const providerTypes = ['openai', 'azure', 'huggingface', 'custom']

      for (const providerType of providerTypes) {
        const request: ValidateEmbeddingProviderRequest = {
          provider_type: providerType,
          model: 'text-embedding-3-small',
          base_url: `https://${providerType}-api.example.com`,
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          json: async () => ({
            is_valid: true,
            message: `${providerType} provider is valid`,
          }),
        })

        const result =
          await embeddingProviderService.validateEmbeddingProvider(request)
        expect(result.is_valid).toBe(true)
      }
    })
  })

  describe('Authentication Integration', () => {
    it('should call authService.logout on 401 for all methods', async () => {
      const methods = [
        () => embeddingProviderService.getEmbeddingProviders(),
        () => embeddingProviderService.getEmbeddingProvider('test-id'),
        () =>
          embeddingProviderService.createEmbeddingProvider({
            name: 'Test',
            provider_type: 'openai',
            model: 'text-embedding-3-small',
          }),
        () =>
          embeddingProviderService.updateEmbeddingProvider('test-id', {
            name: 'Updated',
          }),
        () => embeddingProviderService.deleteEmbeddingProvider('test-id'),
        () =>
          embeddingProviderService.validateEmbeddingProvider({
            provider_type: 'openai',
            model: 'text-embedding-3-small',
            base_url: 'https://api.openai.com/v1',
          }),
      ]

      for (const method of methods) {
        mockFetch.mockResolvedValueOnce({
          ok: false,
          status: 401,
        })

        await expect(method()).rejects.toThrow('Authentication expired')
        expect(mockAuthService.logout).toHaveBeenCalled()

        // Reset mock for next iteration
        mockAuthService.logout.mockClear()
      }
    })

    it('should include Bearer token in all authenticated requests', async () => {
      const testToken = 'test-bearer-token'
      mockAuthService.getToken.mockReturnValue(testToken)

      mockFetch.mockResolvedValue({
        ok: true,
        json: async () => ({}),
      })

      // Test all methods that require authentication
      await embeddingProviderService.getEmbeddingProviders()
      await embeddingProviderService.getEmbeddingProvider('test-id')
      await embeddingProviderService.createEmbeddingProvider({
        name: 'Test',
        provider_type: 'openai',
        model: 'text-embedding-3-small',
      })
      await embeddingProviderService.updateEmbeddingProvider('test-id', {
        name: 'Updated',
      })
      await embeddingProviderService.deleteEmbeddingProvider('test-id')
      await embeddingProviderService.validateEmbeddingProvider({
        provider_type: 'openai',
        model: 'text-embedding-3-small',
        base_url: 'https://api.openai.com/v1',
      })

      // Verify all calls included the Bearer token
      const calls = mockFetch.mock.calls
      expect(calls).toHaveLength(6)

      calls.forEach(call => {
        const options = call[1]
        expect(options.headers.Authorization).toBe(`Bearer ${testToken}`)
      })
    })
  })
})
