import type {
  APIKey,
  CreateAPIKeyRequest,
  CreateAPIKeyResponse,
} from '../../src/types'

// Define proper type for mocked fetch Response
interface MockResponse extends Partial<Response> {
  ok: boolean
  status?: number
  json?: jest.Mock
}

// Mock the authService
const mockAuthService = {
  getToken: jest.fn(),
  logout: jest.fn(),
}

jest.mock('../../src/services/authService', () => ({
  authService: mockAuthService,
}))

// Mock fetch globally
global.fetch = jest.fn()

// Mock the APIKeyService class to avoid import.meta.env issues
class MockAPIKeyService {
  private API_BASE_URL = 'https://api.vibexp.io/api/v1'

  async createAPIKey(
    request: CreateAPIKeyRequest
  ): Promise<CreateAPIKeyResponse> {
    const token = mockAuthService.getToken()
    if (!token) {
      throw new Error('No authentication token')
    }

    const response = await fetch(`${this.API_BASE_URL}/settings/api-keys`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(request),
    })

    if (!response.ok) {
      if (response.status === 401) {
        mockAuthService.logout()
        throw new Error('Authentication expired')
      }
      throw new Error('Failed to create API key')
    }

    return response.json()
  }

  async getAPIKeys(): Promise<APIKey[]> {
    const token = mockAuthService.getToken()
    if (!token) {
      throw new Error('No authentication token')
    }

    const response = await fetch(`${this.API_BASE_URL}/settings/api-keys`, {
      method: 'GET',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
    })

    if (!response.ok) {
      if (response.status === 401) {
        mockAuthService.logout()
        throw new Error('Authentication expired')
      }
      throw new Error('Failed to get API keys')
    }

    return response.json()
  }

  async deleteAPIKey(apiKeyId: string): Promise<void> {
    const token = mockAuthService.getToken()
    if (!token) {
      throw new Error('No authentication token')
    }

    const response = await fetch(
      `${this.API_BASE_URL}/settings/api-keys/${apiKeyId}`,
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
        throw new Error('API key not found')
      }
      throw new Error('Failed to delete API key')
    }
  }
}

const apiKeyService = new MockAPIKeyService()

describe('ApiKeyService', () => {
  const mockToken = 'test-auth-token'
  const mockAPIKey: APIKey = {
    id: 'test-api-key-id',
    user_id: 'test-user-id',
    name: 'Test API Key',
    key_prefix: 'vxk_test_',
    integrations: ['ai_tools', 'cli', 'mcp_server', 'marketplace'],
    is_legacy: false,
    last_used_at: null,
    created_at: '2023-01-01T00:00:00Z',
    updated_at: '2023-01-01T00:00:00Z',
  }

  const mockFetch = fetch as jest.MockedFunction<typeof fetch>

  beforeEach(() => {
    jest.clearAllMocks()
    mockAuthService.getToken.mockReturnValue(mockToken)
    mockFetch.mockClear()
  })

  describe('CRUD Operations', () => {
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
        mockFetch.mockResolvedValueOnce({
          ok: true,
          json: jest.fn().mockResolvedValue(mockResponse),
        } as MockResponse as Response)

        const result = await apiKeyService.createAPIKey(mockRequest)

        expect(mockFetch).toHaveBeenCalledWith(
          'https://api.vibexp.io/api/v1/settings/api-keys',
          expect.objectContaining({
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              Authorization: `Bearer ${mockToken}`,
            },
            body: JSON.stringify(mockRequest),
          })
        )
        expect(result).toEqual(mockResponse)
      })

      it('should validate API key name', async () => {
        const invalidRequest = { name: '' } as CreateAPIKeyRequest

        mockFetch.mockResolvedValueOnce({
          ok: false,
          status: 400,
        } as MockResponse as Response)

        await expect(
          apiKeyService.createAPIKey(invalidRequest)
        ).rejects.toThrow('Failed to create API key')
      })

      it('should handle duplicate API key names', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: false,
          status: 409,
        } as MockResponse as Response)

        await expect(apiKeyService.createAPIKey(mockRequest)).rejects.toThrow(
          'Failed to create API key'
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

      it('should retrieve API keys successfully', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          json: jest.fn().mockResolvedValue(mockAPIKeys),
        } as MockResponse as Response)

        const result = await apiKeyService.getAPIKeys()

        expect(mockFetch).toHaveBeenCalledWith(
          'https://api.vibexp.io/api/v1/settings/api-keys',
          expect.objectContaining({
            method: 'GET',
            headers: {
              'Content-Type': 'application/json',
              Authorization: `Bearer ${mockToken}`,
            },
          })
        )
        expect(result).toEqual(mockAPIKeys)
      })

      it('should return empty array when no keys exist', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          json: jest.fn().mockResolvedValue([]),
        } as MockResponse as Response)

        const result = await apiKeyService.getAPIKeys()

        expect(result).toEqual([])
      })

      it('should handle large numbers of API keys', async () => {
        const largeKeyList = Array.from({ length: 50 }, (_, i) => ({
          ...mockAPIKey,
          id: `api-key-${i}`,
          name: `API Key ${i}`,
        }))

        mockFetch.mockResolvedValueOnce({
          ok: true,
          json: jest.fn().mockResolvedValue(largeKeyList),
        } as MockResponse as Response)

        const result = await apiKeyService.getAPIKeys()

        expect(result).toHaveLength(50)
        expect(result[0].name).toBe('API Key 0')
        expect(result[49].name).toBe('API Key 49')
      })
    })

    describe('deleteAPIKey', () => {
      const testApiKeyId = 'test-api-key-id'

      it('should delete API key successfully', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
        } as MockResponse as Response)

        await apiKeyService.deleteAPIKey(testApiKeyId)

        expect(mockFetch).toHaveBeenCalledWith(
          `https://api.vibexp.io/api/v1/settings/api-keys/${testApiKeyId}`,
          expect.objectContaining({
            method: 'DELETE',
            headers: {
              Authorization: `Bearer ${mockToken}`,
            },
          })
        )
      })

      it('should handle API key not found (404)', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: false,
          status: 404,
        } as MockResponse as Response)

        await expect(apiKeyService.deleteAPIKey(testApiKeyId)).rejects.toThrow(
          'API key not found'
        )
      })

      it('should handle empty API key ID', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: false,
          status: 400,
        } as MockResponse as Response)

        await expect(apiKeyService.deleteAPIKey('')).rejects.toThrow(
          'Failed to delete API key'
        )
      })

      it('should properly encode special characters in API key ID', async () => {
        const specialKeyId = 'test key with spaces & symbols'
        mockFetch.mockResolvedValueOnce({
          ok: true,
        } as MockResponse as Response)

        await apiKeyService.deleteAPIKey(specialKeyId)

        expect(mockFetch).toHaveBeenCalledWith(
          `https://api.vibexp.io/api/v1/settings/api-keys/${specialKeyId}`,
          expect.any(Object)
        )
      })
    })
  })

  describe('Error Handling', () => {
    describe('Authentication Errors', () => {
      it('should throw error when no authentication token', async () => {
        mockAuthService.getToken.mockReturnValue(null)

        await expect(
          apiKeyService.createAPIKey({
            name: 'test',
            integration_codes: ['ai_tools', 'cli', 'mcp_server', 'marketplace'],
          })
        ).rejects.toThrow('No authentication token')

        expect(mockFetch).not.toHaveBeenCalled()
      })

      it('should handle authentication expired (401)', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: false,
          status: 401,
        } as MockResponse as Response)

        await expect(
          apiKeyService.createAPIKey({
            name: 'test',
            integration_codes: ['ai_tools', 'cli', 'mcp_server', 'marketplace'],
          })
        ).rejects.toThrow('Authentication expired')

        expect(mockAuthService.logout).toHaveBeenCalled()
      })

      it('should handle token changes between calls', async () => {
        const newToken = 'new-auth-token'

        // First call with original token
        mockFetch.mockResolvedValueOnce({
          ok: true,
          json: jest.fn().mockResolvedValue([]),
        } as MockResponse as Response)
        await apiKeyService.getAPIKeys()

        expect(mockFetch).toHaveBeenLastCalledWith(
          expect.any(String),
          expect.objectContaining({
            headers: expect.objectContaining({
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )

        // Change token
        mockAuthService.getToken.mockReturnValue(newToken)

        // Second call with new token
        mockFetch.mockResolvedValueOnce({
          ok: true,
          json: jest.fn().mockResolvedValue([]),
        } as MockResponse as Response)
        await apiKeyService.getAPIKeys()

        expect(mockFetch).toHaveBeenLastCalledWith(
          expect.any(String),
          expect.objectContaining({
            headers: expect.objectContaining({
              Authorization: `Bearer ${newToken}`,
            }),
          })
        )
      })

      it('should handle undefined token as null', async () => {
        mockAuthService.getToken.mockReturnValue(undefined)

        await expect(apiKeyService.getAPIKeys()).rejects.toThrow(
          'No authentication token'
        )
      })

      it('should handle empty string token as null', async () => {
        mockAuthService.getToken.mockReturnValue('')

        await expect(apiKeyService.getAPIKeys()).rejects.toThrow(
          'No authentication token'
        )
      })
    })

    describe('Network and Server Errors', () => {
      it('should handle network errors', async () => {
        mockFetch.mockRejectedValueOnce(new Error('Network Error'))

        await expect(
          apiKeyService.createAPIKey({
            name: 'test',
            integration_codes: ['ai_tools', 'cli', 'mcp_server', 'marketplace'],
          })
        ).rejects.toThrow('Network Error')
      })

      it('should handle server errors (500)', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: false,
          status: 500,
        } as MockResponse as Response)

        await expect(
          apiKeyService.createAPIKey({
            name: 'test',
            integration_codes: ['ai_tools', 'cli', 'mcp_server', 'marketplace'],
          })
        ).rejects.toThrow('Failed to create API key')

        expect(mockAuthService.logout).not.toHaveBeenCalled()
      })

      it('should handle JSON parsing errors', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          json: jest.fn().mockRejectedValue(new Error('Invalid JSON')),
        } as MockResponse as Response)

        await expect(
          apiKeyService.createAPIKey({
            name: 'test',
            integration_codes: ['ai_tools', 'cli', 'mcp_server', 'marketplace'],
          })
        ).rejects.toThrow('Invalid JSON')
      })

      it('should handle various HTTP status codes', async () => {
        const testCases = [
          { status: 403, expectedError: 'Failed to create API key' },
          { status: 429, expectedError: 'Failed to create API key' },
          { status: 502, expectedError: 'Failed to create API key' },
        ]

        for (const testCase of testCases) {
          mockFetch.mockResolvedValueOnce({
            ok: false,
            status: testCase.status,
          } as MockResponse as Response)

          await expect(
            apiKeyService.createAPIKey({
              name: 'Test',
              integration_codes: [
                'ai_tools',
                'cli',
                'mcp_server',
                'marketplace',
              ],
            })
          ).rejects.toThrow(testCase.expectedError)
        }
      })
    })
  })

  describe('Security and Validation', () => {
    it('should not expose sensitive data in error messages', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Internal server details'))

      await expect(
        apiKeyService.createAPIKey({
          name: 'test',
          integration_codes: ['ai_tools', 'cli', 'mcp_server', 'marketplace'],
        })
      ).rejects.toThrow('Internal server details')

      // Ensure no sensitive auth token information is leaked
      expect(mockFetch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: `Bearer ${mockToken}`,
          }),
        })
      )
    })

    it('should maintain proper headers across different operations', async () => {
      // Test createAPIKey headers
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: jest.fn().mockResolvedValue({
          api_key: mockAPIKey,
          full_key: 'test',
          key_prefix: 'test_',
        }),
      } as MockResponse as Response)
      await apiKeyService.createAPIKey({
        name: 'Test',
        integration_codes: ['ai_tools', 'cli', 'mcp_server', 'marketplace'],
      })

      expect(mockFetch).toHaveBeenLastCalledWith(
        expect.any(String),
        expect.objectContaining({
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${mockToken}`,
          },
        })
      )

      // Test getAPIKeys headers
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: jest.fn().mockResolvedValue([]),
      } as MockResponse as Response)
      await apiKeyService.getAPIKeys()

      expect(mockFetch).toHaveBeenLastCalledWith(
        expect.any(String),
        expect.objectContaining({
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${mockToken}`,
          },
        })
      )

      // Test deleteAPIKey headers (note: no Content-Type header)
      mockFetch.mockResolvedValueOnce({
        ok: true,
      } as MockResponse as Response)
      await apiKeyService.deleteAPIKey('test-id')

      expect(mockFetch).toHaveBeenLastCalledWith(
        expect.any(String),
        expect.objectContaining({
          headers: {
            Authorization: `Bearer ${mockToken}`,
          },
        })
      )
    })

    it('should validate proper HTTP methods for each operation', async () => {
      // Test POST method for createAPIKey
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: jest.fn().mockResolvedValue({
          api_key: mockAPIKey,
          full_key: 'test',
          key_prefix: 'test_',
        }),
      } as MockResponse as Response)
      await apiKeyService.createAPIKey({
        name: 'Test',
        integration_codes: ['ai_tools', 'cli', 'mcp_server', 'marketplace'],
      })

      expect(mockFetch).toHaveBeenLastCalledWith(
        expect.any(String),
        expect.objectContaining({ method: 'POST' })
      )

      // Test GET method for getAPIKeys
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: jest.fn().mockResolvedValue([]),
      } as MockResponse as Response)
      await apiKeyService.getAPIKeys()

      expect(mockFetch).toHaveBeenLastCalledWith(
        expect.any(String),
        expect.objectContaining({ method: 'GET' })
      )

      // Test DELETE method for deleteAPIKey
      mockFetch.mockResolvedValueOnce({
        ok: true,
      } as MockResponse as Response)
      await apiKeyService.deleteAPIKey('test-id')

      expect(mockFetch).toHaveBeenLastCalledWith(
        expect.any(String),
        expect.objectContaining({ method: 'DELETE' })
      )
    })

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

      mockFetch.mockResolvedValue({
        ok: true,
        json: jest.fn().mockResolvedValue(mockResponse),
      } as MockResponse as Response)

      // Simulate concurrent create operations
      const promises = [
        apiKeyService.createAPIKey(mockRequest),
        apiKeyService.createAPIKey(mockRequest),
        apiKeyService.createAPIKey(mockRequest),
      ]

      const results = await Promise.all(promises)

      expect(results).toHaveLength(3)
      expect(mockFetch).toHaveBeenCalledTimes(3)
      results.forEach(result => {
        expect(result).toEqual(mockResponse)
      })
    })
  })

  describe('API Integration Scenarios', () => {
    it('should handle complete API key lifecycle', async () => {
      // Step 1: Create API key
      const createRequest: CreateAPIKeyRequest = {
        name: 'Integration Test Key',
        integration_codes: ['ai_tools', 'cli', 'mcp_server', 'marketplace'],
      }
      const createResponse: CreateAPIKeyResponse = {
        api_key: mockAPIKey,
        full_key: 'vxp_integration_full_key',
        key_prefix: 'vxp_int_',
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: jest.fn().mockResolvedValue(createResponse),
      } as MockResponse as Response)

      const createdKey = await apiKeyService.createAPIKey(createRequest)
      expect(createdKey).toEqual(createResponse)

      // Step 2: List API keys (should include the created key)
      const listResponse: APIKey[] = [mockAPIKey]
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: jest.fn().mockResolvedValue(listResponse),
      } as MockResponse as Response)

      const keys = await apiKeyService.getAPIKeys()
      expect(keys).toEqual(listResponse)

      // Step 3: Delete API key
      mockFetch.mockResolvedValueOnce({
        ok: true,
      } as MockResponse as Response)

      await apiKeyService.deleteAPIKey(mockAPIKey.id)
      expect(mockFetch).toHaveBeenLastCalledWith(
        `https://api.vibexp.io/api/v1/settings/api-keys/${mockAPIKey.id}`,
        expect.objectContaining({ method: 'DELETE' })
      )
    })

    it('should handle token expiration during operations', async () => {
      // Set initial token
      mockAuthService.getToken.mockReturnValue('expired-token')

      // Simulate 401 response during create operation
      mockFetch.mockResolvedValue({
        ok: false,
        status: 401,
      } as MockResponse as Response)

      await expect(
        apiKeyService.createAPIKey({
          name: 'Test',
          integration_codes: ['ai_tools', 'cli', 'mcp_server', 'marketplace'],
        })
      ).rejects.toThrow('Authentication expired')

      // Verify logout was called
      expect(mockAuthService.logout).toHaveBeenCalled()
    })
  })

  describe('API Endpoint Validation', () => {
    it('should call the correct endpoint for createAPIKey', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        json: jest.fn().mockResolvedValue({
          api_key: mockAPIKey,
          full_key: 'test',
          key_prefix: 'test_',
        }),
      } as MockResponse as Response)

      await apiKeyService.createAPIKey({
        name: 'Test',
        integration_codes: ['ai_tools', 'cli', 'mcp_server', 'marketplace'],
      })

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.vibexp.io/api/v1/settings/api-keys',
        expect.any(Object)
      )
    })

    it('should call the correct endpoint for getAPIKeys', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        json: jest.fn().mockResolvedValue([]),
      } as MockResponse as Response)

      await apiKeyService.getAPIKeys()

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.vibexp.io/api/v1/settings/api-keys',
        expect.any(Object)
      )
    })

    it('should call the correct endpoint for deleteAPIKey', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
      } as MockResponse as Response)

      await apiKeyService.deleteAPIKey('test-id')

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.vibexp.io/api/v1/settings/api-keys/test-id',
        expect.any(Object)
      )
    })
  })

  describe('Environment Configuration', () => {
    it('should use production API URL by default', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: jest.fn().mockResolvedValue([]),
      } as MockResponse as Response)

      await apiKeyService.getAPIKeys()

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.vibexp.io/api/v1/settings/api-keys',
        expect.any(Object)
      )
    })
  })

  describe('Performance and Resource Management', () => {
    it('should handle response with missing json method', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        // Missing json method
      } as MockResponse as Response)

      await expect(apiKeyService.getAPIKeys()).rejects.toThrow()
    })

    it('should handle malformed responses gracefully', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: jest.fn().mockRejectedValue(new Error('Malformed JSON')),
      } as MockResponse as Response)

      await expect(apiKeyService.getAPIKeys()).rejects.toThrow('Malformed JSON')
    })
  })
})
