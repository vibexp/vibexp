import type { AuthProvider, LoginUrlResponse, User } from '../../types'
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

// Import the authService after mocking
import { authService } from '../authService'

const mockUser: User = {
  id: 'user-123',
  google_id: 'google-456',
  email: 'test@example.com',
  name: 'Test User',
  avatar_url: 'https://example.com/avatar.jpg',
  created_at: '2023-01-01T00:00:00Z',
  updated_at: '2023-01-01T00:00:00Z',
  onboarding_completed: true,
}

describe('AuthService (cookie-based auth)', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getProviders', () => {
    it('should return the enabled providers from the backend', async () => {
      const providers: AuthProvider[] = [
        { name: 'github', display_name: 'GitHub' },
        { name: 'google', display_name: 'Google' },
      ]
      mockApiClient.get.mockResolvedValue({ providers })

      const result = await authService.getProviders()

      expect(mockApiClient.get).toHaveBeenCalledWith('/auth/providers')
      expect(result).toEqual(providers)
    })

    it('should return an empty list when no provider is configured', async () => {
      mockApiClient.get.mockResolvedValue({ providers: [] })

      const result = await authService.getProviders()

      expect(result).toEqual([])
    })

    it('should throw error when the request fails', async () => {
      mockApiClient.get.mockRejectedValue(new Error('Network error'))

      await expect(authService.getProviders()).rejects.toThrow('Network error')
    })
  })

  describe('getLoginUrl', () => {
    it('should return the login URL from the backend', async () => {
      const mockResponse: LoginUrlResponse = {
        url: 'https://idp.example.com/authorize?...',
      }
      mockApiClient.get.mockResolvedValue(mockResponse)

      const result = await authService.getLoginUrl()

      expect(mockApiClient.get).toHaveBeenCalledWith('/auth/login')
      expect(result).toBe(mockResponse.url)
    })

    it('should append provider query param when provider is supplied', async () => {
      const mockResponse: LoginUrlResponse = {
        url: 'https://idp.example.com/authorize?provider=google',
      }
      mockApiClient.get.mockResolvedValue(mockResponse)

      const result = await authService.getLoginUrl('google')

      expect(mockApiClient.get).toHaveBeenCalledWith(
        '/auth/login?provider=google'
      )
      expect(result).toBe(mockResponse.url)
    })

    it('should URL-encode the provider name', async () => {
      const mockResponse: LoginUrlResponse = {
        url: 'https://idp.example.com/authorize?provider=acme+oidc',
      }
      mockApiClient.get.mockResolvedValue(mockResponse)

      await authService.getLoginUrl('acme oidc')

      expect(mockApiClient.get).toHaveBeenCalledWith(
        '/auth/login?provider=acme%20oidc'
      )
    })

    it('should not append query param when called with no args', async () => {
      const mockResponse: LoginUrlResponse = {
        url: 'https://idp.example.com/authorize?...',
      }
      mockApiClient.get.mockResolvedValue(mockResponse)

      await authService.getLoginUrl()

      expect(mockApiClient.get).toHaveBeenCalledWith('/auth/login')
    })

    it('should throw ApiError when request fails', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/INTERNAL_ERROR',
        title: 'Internal Server Error',
        status: 500,
        detail: 'Failed to generate login URL',
        code: 'INTERNAL_ERROR',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.get.mockRejectedValue(new ApiError(errorResponse))

      await expect(authService.getLoginUrl()).rejects.toThrow(ApiError)
      await expect(authService.getLoginUrl()).rejects.toThrow(
        'Failed to generate login URL'
      )
    })

    it('should throw error when network request fails', async () => {
      mockApiClient.get.mockRejectedValue(new Error('Network error'))

      await expect(authService.getLoginUrl()).rejects.toThrow('Network error')
    })
  })

  describe('getCurrentUser', () => {
    it('should fetch the current user via session cookie', async () => {
      mockApiClient.get.mockResolvedValue(mockUser)

      const result = await authService.getCurrentUser()

      expect(mockApiClient.get).toHaveBeenCalledWith('/auth/me')
      expect(result).toEqual(mockUser)
    })

    it('should throw ApiError when session is invalid (401)', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/AUTH_REQUIRED',
        title: 'Authentication Required',
        status: 401,
        detail: 'Session is invalid or expired',
        code: 'AUTH_REQUIRED',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.get.mockRejectedValue(new ApiError(errorResponse))

      await expect(authService.getCurrentUser()).rejects.toThrow(ApiError)
      await expect(authService.getCurrentUser()).rejects.toThrow(
        'Session is invalid or expired'
      )
    })

    it('should throw error when network request fails', async () => {
      mockApiClient.get.mockRejectedValue(new Error('Network error'))

      await expect(authService.getCurrentUser()).rejects.toThrow(
        'Network error'
      )
    })
  })

  describe('logout', () => {
    it('should call the backend logout endpoint to clear the session cookie', async () => {
      mockApiClient.post.mockResolvedValue({ message: 'logged out' })

      await authService.logout()

      expect(mockApiClient.post).toHaveBeenCalledWith('/auth/logout')
    })

    it('should throw error when logout request fails', async () => {
      mockApiClient.post.mockRejectedValue(new Error('Network error'))

      await expect(authService.logout()).rejects.toThrow('Network error')
    })
  })

  describe('markOnboardingComplete', () => {
    it('should call the correct endpoint and return updated user', async () => {
      const updatedUser: User = { ...mockUser, onboarding_completed: true }
      mockApiClient.post.mockResolvedValue(updatedUser)

      const result = await authService.markOnboardingComplete()

      expect(mockApiClient.post).toHaveBeenCalledWith(
        '/user/onboarding/complete'
      )
      expect(result).toEqual(updatedUser)
    })
  })

  describe('devLogin', () => {
    it('should dev login successfully and return user', async () => {
      mockApiClient.post.mockResolvedValue(mockUser)

      const result = await authService.devLogin('dev@example.com')

      expect(mockApiClient.post).toHaveBeenCalledWith('/auth/dev/login', {
        email: 'dev@example.com',
        name: 'Dev User',
      })
      expect(result).toEqual(mockUser)
    })

    it('should use custom name when provided', async () => {
      mockApiClient.post.mockResolvedValue(mockUser)

      await authService.devLogin('dev@example.com', 'Custom Name')

      expect(mockApiClient.post).toHaveBeenCalledWith('/auth/dev/login', {
        email: 'dev@example.com',
        name: 'Custom Name',
      })
    })

    it('should throw error when dev login fails', async () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/AUTH_FAILED',
        title: 'Dev Login Disabled',
        status: 403,
        detail: 'Dev login is disabled in this environment',
        code: 'AUTH_FAILED',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      }
      mockApiClient.post.mockRejectedValue(new ApiError(errorResponse))

      await expect(authService.devLogin('dev@example.com')).rejects.toThrow(
        ApiError
      )
    })
  })

  describe('integration scenarios', () => {
    it('should handle complete cookie-based authentication flow', async () => {
      // Step 1: Get login URL
      const loginUrlResponse: LoginUrlResponse = {
        url: 'https://idp.example.com/authorize?...',
      }
      mockApiClient.get.mockResolvedValueOnce(loginUrlResponse)

      const loginUrl = await authService.getLoginUrl()
      expect(loginUrl).toBe(loginUrlResponse.url)

      // Step 2: After redirect + backend sets cookie, get current user
      mockApiClient.get.mockResolvedValueOnce(mockUser)
      const user = await authService.getCurrentUser()
      expect(user).toEqual(mockUser)

      // Step 3: Logout
      mockApiClient.post.mockResolvedValueOnce({ message: 'logged out' })
      await authService.logout()
      expect(mockApiClient.post).toHaveBeenCalledWith('/auth/logout')
    })
  })

  describe('no localStorage or in-memory token management', () => {
    it('should not expose setToken method', () => {
      expect(
        'setToken' in (authService as unknown as Record<string, unknown>)
      ).toBe(false)
    })

    it('should not expose getToken method', () => {
      expect(
        'getToken' in (authService as unknown as Record<string, unknown>)
      ).toBe(false)
    })

    it('should not expose isAuthenticated method', () => {
      expect(
        'isAuthenticated' in (authService as unknown as Record<string, unknown>)
      ).toBe(false)
    })
  })
})
