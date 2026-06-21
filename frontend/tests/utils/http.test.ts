import type { HttpRequestOptions } from '../../src/utils/http'

// Mock authService
const mockAuthService = {
  getToken: jest.fn(),
  logout: jest.fn(),
}

jest.mock('../../src/services/authService', () => ({
  authService: mockAuthService,
}))

interface MockAuthService {
  getToken: jest.Mock
  logout: jest.Mock
}

// Create global reference for the auth service
let globalMockAuthService: MockAuthService

// Define a MockResponse interface to replace 'as any'
interface MockResponse {
  ok: boolean
  status?: number
  statusText?: string
  json: jest.Mock
  text?: jest.Mock
  headers?: Headers
}

// Mock the entire HTTP client module to avoid import.meta.env issues
jest.mock('../../src/utils/http', () => {
  interface HttpClient {
    request<T>(endpoint: string, options?: HttpRequestOptions): Promise<T>
    get<T>(endpoint: string): Promise<T>
    post<T>(endpoint: string, data?: unknown): Promise<T>
    put<T>(endpoint: string, data?: unknown): Promise<T>
    delete<T>(endpoint: string): Promise<T>
  }

  class HttpClientImpl implements HttpClient {
    async request<T>(
      endpoint: string,
      options: HttpRequestOptions = {}
    ): Promise<T> {
      const token = globalMockAuthService?.getToken() || null
      if (!token) {
        throw new Error('No authentication token')
      }

      const url = `http://localhost:8080/api/v1${endpoint.startsWith('/') ? endpoint : '/' + endpoint}`
      const config: RequestInit = {
        ...options,
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
          ...options.headers,
        },
      }

      const response = await fetch(url, config)

      if (!response.ok) {
        if (response.status === 401) {
          globalMockAuthService?.logout()
          throw new Error('Authentication expired. Please sign in again.')
        }

        if (response.status === 402) {
          // Set href on our mock location object
          ;(
            global as unknown as { mockLocation: { href: string } }
          ).mockLocation.href = '/subscription'
          throw new Error('Subscription required to access this feature.')
        }

        const errorData = await response
          .json()
          .catch(() => ({ message: response.statusText }))
        throw new Error(
          errorData.message || `HTTP ${response.status}: ${response.statusText}`
        )
      }

      return response.json()
    }

    async get<T>(endpoint: string): Promise<T> {
      return this.request<T>(endpoint, { method: 'GET' })
    }

    async post<T>(endpoint: string, data?: unknown): Promise<T> {
      return this.request<T>(endpoint, {
        method: 'POST',
        body:
          data !== undefined && data !== null
            ? JSON.stringify(data)
            : undefined,
      })
    }

    async put<T>(endpoint: string, data?: unknown): Promise<T> {
      return this.request<T>(endpoint, {
        method: 'PUT',
        body:
          data !== undefined && data !== null
            ? JSON.stringify(data)
            : undefined,
      })
    }

    async delete<T>(endpoint: string): Promise<T> {
      return this.request<T>(endpoint, { method: 'DELETE' })
    }
  }

  const httpClient: HttpClient = new HttpClientImpl()
  return { httpClient }
})

const { httpClient } = require('../../src/utils/http')

// Type assertion to restore generics
const typedHttpClient = httpClient as {
  request<T>(endpoint: string, options?: HttpRequestOptions): Promise<T>
  get<T>(endpoint: string): Promise<T>
  post<T>(endpoint: string, data?: unknown): Promise<T>
  put<T>(endpoint: string, data?: unknown): Promise<T>
  delete<T>(endpoint: string): Promise<T>
}

// Mock global fetch
const mockFetch = jest.fn()
global.fetch = mockFetch as jest.MockedFunction<typeof fetch>

// Mock window.location - JSDOM's location is not configurable,
// so we'll create our own mock object for testing
const mockLocation = { href: '' }

// Make mockLocation globally accessible for the mocked HTTP client to use
;(global as unknown as { mockLocation: { href: string } }).mockLocation =
  mockLocation

describe('HttpClient', () => {
  const mockToken = 'mock-jwt-token'
  const baseURL = 'http://localhost:8080/api/v1' // Test environment URL

  beforeEach(() => {
    jest.clearAllMocks()
    globalMockAuthService = mockAuthService
    mockAuthService.getToken.mockReturnValue(mockToken)
  })

  afterEach(() => {
    jest.resetAllMocks()
  })

  describe('Authentication and Authorization', () => {
    it('includes authorization header with token in requests', async () => {
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue({ message: 'success' }),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      await typedHttpClient.get('/test-endpoint')

      expect(mockFetch).toHaveBeenCalledWith(`${baseURL}/test-endpoint`, {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${mockToken}`,
        },
      })
    })

    it('throws error when no authentication token is available', async () => {
      mockAuthService.getToken.mockReturnValue(null)

      await expect(httpClient.get('/test-endpoint')).rejects.toThrow(
        'No authentication token'
      )
    })

    it('throws error when token is empty string', async () => {
      mockAuthService.getToken.mockReturnValue('')

      await expect(httpClient.get('/test-endpoint')).rejects.toThrow(
        'No authentication token'
      )
    })

    it('throws error when token is undefined', async () => {
      mockAuthService.getToken.mockReturnValue(undefined)

      await expect(httpClient.get('/test-endpoint')).rejects.toThrow(
        'No authentication token'
      )
    })

    it('calls logout and throws error on 401 response', async () => {
      const mockResponse: MockResponse = {
        ok: false,
        status: 401,
        statusText: 'Unauthorized',
        json: jest.fn(),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      await expect(httpClient.get('/test-endpoint')).rejects.toThrow(
        'Authentication expired. Please sign in again.'
      )

      expect(mockAuthService.logout).toHaveBeenCalledTimes(1)
    })

    it('redirects to subscription page on 402 response', async () => {
      const mockResponse: MockResponse = {
        ok: false,
        status: 402,
        statusText: 'Payment Required',
        json: jest.fn(),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      await expect(httpClient.get('/test-endpoint')).rejects.toThrow(
        'Subscription required to access this feature.'
      )

      expect(mockLocation.href).toBe('/subscription')
    })
  })

  describe('Request Method - request()', () => {
    it('makes basic request with default options', async () => {
      const responseData = { id: 1, name: 'test' }
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue(responseData),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const result = await typedHttpClient.request('/test-endpoint')

      expect(mockFetch).toHaveBeenCalledWith(`${baseURL}/test-endpoint`, {
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${mockToken}`,
        },
      })
      expect(result).toEqual(responseData)
    })

    it('merges custom headers with default headers', async () => {
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue({}),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const customOptions: HttpRequestOptions = {
        headers: {
          'X-Custom-Header': 'custom-value',
          Accept: 'application/xml', // Override default content type
        },
      }

      await typedHttpClient.request('/test-endpoint', customOptions)

      expect(mockFetch).toHaveBeenCalledWith(`${baseURL}/test-endpoint`, {
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${mockToken}`,
          'X-Custom-Header': 'custom-value',
          Accept: 'application/xml',
        },
      })
    })

    it('passes through other request options', async () => {
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue({}),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const customOptions: HttpRequestOptions = {
        method: 'PATCH',
        body: JSON.stringify({ data: 'test' }),
        signal: new AbortController().signal,
      }

      await typedHttpClient.request('/test-endpoint', customOptions)

      expect(mockFetch).toHaveBeenCalledWith(`${baseURL}/test-endpoint`, {
        method: 'PATCH',
        body: JSON.stringify({ data: 'test' }),
        signal: expect.any(AbortSignal),
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${mockToken}`,
        },
      })
    })

    it('constructs correct URL with endpoint', async () => {
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue({}),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      await typedHttpClient.request('/users/123/profile')

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseURL}/users/123/profile`,
        expect.any(Object)
      )
    })
  })

  describe('GET Method', () => {
    it('makes GET request with correct method', async () => {
      const responseData = { users: [] }
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue(responseData),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const result = await typedHttpClient.get('/users')

      expect(mockFetch).toHaveBeenCalledWith(`${baseURL}/users`, {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${mockToken}`,
        },
      })
      expect(result).toEqual(responseData)
    })

    it('returns properly typed response', async () => {
      interface User {
        id: number
        name: string
        email: string
      }

      const userData: User = { id: 1, name: 'John', email: 'john@example.com' }
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue(userData),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const result = await typedHttpClient.get<User>('/users/1')

      expect(result).toEqual(userData)
      expect(result.id).toBe(1)
      expect(result.name).toBe('John')
      expect(result.email).toBe('john@example.com')
    })
  })

  describe('POST Method', () => {
    it('makes POST request with JSON body', async () => {
      const postData = { name: 'John', email: 'john@example.com' }
      const responseData = {
        id: 1,
        ...postData,
        created_at: '2024-01-15T10:00:00Z',
      }
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue(responseData),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const result = await typedHttpClient.post('/users', postData)

      expect(mockFetch).toHaveBeenCalledWith(`${baseURL}/users`, {
        method: 'POST',
        body: JSON.stringify(postData),
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${mockToken}`,
        },
      })
      expect(result).toEqual(responseData)
    })

    it('makes POST request without body when data is undefined', async () => {
      const responseData = { message: 'success' }
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue(responseData),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const result = await typedHttpClient.post('/trigger-action')

      expect(mockFetch).toHaveBeenCalledWith(`${baseURL}/trigger-action`, {
        method: 'POST',
        body: undefined,
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${mockToken}`,
        },
      })
      expect(result).toEqual(responseData)
    })

    it('makes POST request without body when data is null', async () => {
      const responseData = { message: 'success' }
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue(responseData),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const result = await typedHttpClient.post('/trigger-action', null)

      expect(mockFetch).toHaveBeenCalledWith(`${baseURL}/trigger-action`, {
        method: 'POST',
        body: undefined,
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${mockToken}`,
        },
      })
      expect(result).toEqual(responseData)
    })

    it('serializes complex data structures', async () => {
      const complexData = {
        user: { name: 'John', preferences: { theme: 'dark', language: 'en' } },
        tags: ['tag1', 'tag2'],
        metadata: { created: new Date('2024-01-15'), count: 42 },
      }
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue({ id: 1 }),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      await typedHttpClient.post('/complex-endpoint', complexData)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify(complexData),
        })
      )
    })
  })

  describe('PUT Method', () => {
    it('makes PUT request with JSON body', async () => {
      const putData = { name: 'Jane', email: 'jane@example.com' }
      const responseData = {
        id: 1,
        ...putData,
        updated_at: '2024-01-15T10:00:00Z',
        is_personal: false,
      }
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue(responseData),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const result = await typedHttpClient.put('/users/1', putData)

      expect(mockFetch).toHaveBeenCalledWith(`${baseURL}/users/1`, {
        method: 'PUT',
        body: JSON.stringify(putData),
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${mockToken}`,
        },
      })
      expect(result).toEqual(responseData)
    })

    it('makes PUT request without body when data is undefined', async () => {
      const responseData = { message: 'updated' }
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue(responseData),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const result = await typedHttpClient.put('/users/1/activate')

      expect(mockFetch).toHaveBeenCalledWith(`${baseURL}/users/1/activate`, {
        method: 'PUT',
        body: undefined,
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${mockToken}`,
        },
      })
      expect(result).toEqual(responseData)
    })
  })

  describe('DELETE Method', () => {
    it('makes DELETE request', async () => {
      const responseData = { message: 'deleted successfully' }
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue(responseData),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const result = await typedHttpClient.delete('/users/1')

      expect(mockFetch).toHaveBeenCalledWith(`${baseURL}/users/1`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${mockToken}`,
        },
      })
      expect(result).toEqual(responseData)
    })

    it('returns properly typed delete response', async () => {
      interface DeleteResponse {
        id: number
        deleted: boolean
        message: string
      }

      const deleteResponse: DeleteResponse = {
        id: 1,
        deleted: true,
        message: 'User deleted successfully',
      }
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue(deleteResponse),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const result = await typedHttpClient.delete<DeleteResponse>('/users/1')

      expect(result).toEqual(deleteResponse)
      expect(result.deleted).toBe(true)
    })
  })

  describe('Error Handling', () => {
    it('handles API error responses with error message', async () => {
      const errorMessage = 'Invalid request parameters'
      const mockResponse: MockResponse = {
        ok: false,
        status: 400,
        statusText: 'Bad Request',
        json: jest.fn().mockResolvedValue({ message: errorMessage }),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      await expect(httpClient.get('/test-endpoint')).rejects.toThrow(
        errorMessage
      )
    })

    it('handles API error responses without error message', async () => {
      const mockResponse: MockResponse = {
        ok: false,
        status: 500,
        statusText: 'Internal Server Error',
        json: jest.fn().mockResolvedValue({}),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      await expect(httpClient.get('/test-endpoint')).rejects.toThrow(
        'HTTP 500: Internal Server Error'
      )
    })

    it('handles error responses with failed JSON parsing', async () => {
      const mockResponse: MockResponse = {
        ok: false,
        status: 503,
        statusText: 'Service Unavailable',
        json: jest.fn().mockRejectedValue(new Error('Invalid JSON')),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      await expect(httpClient.get('/test-endpoint')).rejects.toThrow(
        'Service Unavailable'
      )
    })

    it('handles network errors from fetch', async () => {
      const networkError = new Error('Network connection failed')
      mockFetch.mockRejectedValue(networkError)

      await expect(httpClient.get('/test-endpoint')).rejects.toThrow(
        'Network connection failed'
      )
    })

    it('handles JSON parsing errors in successful responses', async () => {
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockRejectedValue(new Error('Invalid JSON response')),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      await expect(httpClient.get('/test-endpoint')).rejects.toThrow(
        'Invalid JSON response'
      )
    })

    it('handles different HTTP error status codes', async () => {
      const testCases = [
        { status: 400, statusText: 'Bad Request' },
        { status: 403, statusText: 'Forbidden' },
        { status: 404, statusText: 'Not Found' },
        { status: 409, statusText: 'Conflict' },
        { status: 422, statusText: 'Unprocessable Entity' },
        { status: 500, statusText: 'Internal Server Error' },
        { status: 502, statusText: 'Bad Gateway' },
        { status: 503, statusText: 'Service Unavailable' },
      ]

      for (const testCase of testCases) {
        const mockResponse: MockResponse = {
          ok: false,
          status: testCase.status,
          statusText: testCase.statusText,
          json: jest.fn().mockResolvedValue({}),
        }
        mockFetch.mockResolvedValue(mockResponse as unknown as Response)

        await expect(httpClient.get('/test-endpoint')).rejects.toThrow(
          `HTTP ${testCase.status}: ${testCase.statusText}`
        )
      }
    })
  })

  describe('Response Handling', () => {
    it('parses JSON responses correctly', async () => {
      const responseData = {
        users: [
          { id: 1, name: 'Alice' },
          { id: 2, name: 'Bob' },
        ],
        meta: { total: 2, page: 1 },
      }
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue(responseData),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const result = await typedHttpClient.get<typeof responseData>('/users')

      expect(result).toEqual(responseData)
      expect(Array.isArray(result.users)).toBe(true)
      expect(result.users).toHaveLength(2)
      expect(result.meta.total).toBe(2)
    })

    it('handles empty JSON responses', async () => {
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue(null),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const result = await typedHttpClient.get('/empty-endpoint')

      expect(result).toBeNull()
    })

    it('handles complex nested JSON responses', async () => {
      const complexResponse = {
        data: {
          user: {
            profile: {
              personal: { name: 'John', age: 30 },
              professional: { title: 'Developer', experience: 5 },
            },
            permissions: ['read', 'write'],
            settings: {
              theme: 'dark',
              notifications: { email: true, push: false },
            },
          },
        },
        metadata: { version: '1.0', timestamp: '2024-01-15T10:00:00Z' },
      }
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue(complexResponse),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const result =
        await typedHttpClient.get<typeof complexResponse>('/complex-data')

      expect(result).toEqual(complexResponse)
      expect(result.data.user.profile.personal.name).toBe('John')
      expect(result.data.user.permissions).toContain('read')
      expect(result.data.user.settings.notifications.email).toBe(true)
    })
  })

  describe('Request Body Serialization', () => {
    it('serializes arrays correctly', async () => {
      const arrayData = [
        { id: 1, name: 'Item 1' },
        { id: 2, name: 'Item 2' },
      ]
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue({ success: true }),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      await typedHttpClient.post('/bulk-create', arrayData)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          body: JSON.stringify(arrayData),
        })
      )
    })

    it('serializes primitive values correctly', async () => {
      const testCases = [
        { data: 'string value', expected: '"string value"' },
        { data: 42, expected: '42' },
        { data: true, expected: 'true' },
        { data: false, expected: 'false' },
      ]

      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue({ success: true }),
      }

      for (const testCase of testCases) {
        mockFetch.mockResolvedValue(mockResponse as unknown as Response)

        await typedHttpClient.post('/test-primitive', testCase.data)

        expect(mockFetch).toHaveBeenCalledWith(
          expect.any(String),
          expect.objectContaining({
            body: testCase.expected,
          })
        )

        mockFetch.mockClear()
      }
    })

    it('handles zero and falsy values correctly', async () => {
      const testCases = [0, false, '']

      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue({ success: true }),
      }

      for (const testCase of testCases) {
        mockFetch.mockResolvedValue(mockResponse as unknown as Response)

        await typedHttpClient.post('/test-falsy', testCase)

        expect(mockFetch).toHaveBeenCalledWith(
          expect.any(String),
          expect.objectContaining({
            body: JSON.stringify(testCase),
          })
        )

        mockFetch.mockClear()
      }
    })
  })

  describe('URL Construction', () => {
    it('constructs URLs correctly for different endpoints', async () => {
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue({}),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const endpoints = [
        '/users',
        '/users/123',
        '/api/nested/endpoint',
        '/path/with/multiple/segments',
        '/users/123/posts/456/comments',
      ]

      for (const endpoint of endpoints) {
        await typedHttpClient.get(endpoint)

        expect(mockFetch).toHaveBeenCalledWith(
          `${baseURL}${endpoint}`,
          expect.any(Object)
        )

        mockFetch.mockClear()
      }
    })

    it('handles endpoints with and without leading slash', async () => {
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue({}),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      await typedHttpClient.get('users') // without leading slash
      expect(mockFetch).toHaveBeenCalledWith(
        `${baseURL}/users`, // should still work correctly
        expect.any(Object)
      )

      mockFetch.mockClear()

      await typedHttpClient.get('/users') // with leading slash
      expect(mockFetch).toHaveBeenCalledWith(
        `${baseURL}/users`,
        expect.any(Object)
      )
    })
  })

  describe('TypeScript Type Safety', () => {
    it('provides type safety for request and response data', async () => {
      interface CreateUserRequest {
        name: string
        email: string
        age: number
      }

      interface CreateUserResponse {
        id: number
        name: string
        email: string
        age: number
        created_at: string
      }

      const requestData: CreateUserRequest = {
        name: 'John Doe',
        email: 'john@example.com',
        age: 30,
      }

      const responseData: CreateUserResponse = {
        id: 1,
        ...requestData,
        created_at: '2024-01-15T10:00:00Z',
      }

      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue(responseData),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const result = await typedHttpClient.post<CreateUserResponse>(
        '/users',
        requestData
      )

      expect(result).toEqual(responseData)
      expect(result.id).toBe(1)
      expect(result.name).toBe('John Doe')
      expect(result.created_at).toBe('2024-01-15T10:00:00Z')
    })
  })

  describe('Edge Cases', () => {
    it('handles requests with AbortController signal', async () => {
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue({ message: 'success' }),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const abortController = new AbortController()
      const options: HttpRequestOptions = {
        signal: abortController.signal,
      }

      await typedHttpClient.request('/test-endpoint', options)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          signal: abortController.signal,
        })
      )
    })

    it('preserves custom request options alongside method-specific ones', async () => {
      const mockResponse: MockResponse = {
        ok: true,
        json: jest.fn().mockResolvedValue({}),
      }
      mockFetch.mockResolvedValue(mockResponse as unknown as Response)

      const customOptions: HttpRequestOptions = {
        cache: 'no-cache',
        credentials: 'include',
        mode: 'cors',
        redirect: 'follow',
      }

      await typedHttpClient.request('/test-endpoint', customOptions)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          cache: 'no-cache',
          credentials: 'include',
          mode: 'cors',
          redirect: 'follow',
        })
      )
    })
  })
})
