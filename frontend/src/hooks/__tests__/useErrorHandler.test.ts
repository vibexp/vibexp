import { renderHook } from '@testing-library/react'
import { act } from 'react'

import { ApiError } from '../../types/errors'
import { useErrorHandler } from '../useErrorHandler'

// Mock AlertContext
const mockShowAlert = jest.fn()
jest.mock('../../contexts/AlertContext', () => ({
  useAlertContext: () => ({
    showAlert: mockShowAlert,
  }),
}))

// Mock storage utilities
jest.mock('../../utils/storage', () => ({
  storage: {
    get: jest.fn(),
    set: jest.fn(),
    remove: jest.fn(),
    clear: jest.fn(),
    has: jest.fn(),
  },
  sessionStore: {
    get: jest.fn(),
    set: jest.fn(),
    remove: jest.fn(),
    clear: jest.fn(),
    has: jest.fn(),
  },
  storageUtils: {
    clearVibeXPData: jest.fn(),
    getAllVibeXPData: jest.fn(),
    isStorageAvailable: jest.fn(),
  },
}))

import { storageUtils } from '../../utils/storage'

// Get the mocked function
const mockClearVibeXPData = storageUtils.clearVibeXPData as jest.Mock

describe('useErrorHandler', () => {
  let consoleErrorSpy: jest.SpyInstance

  beforeEach(() => {
    jest.clearAllMocks()
    // Suppress jsdom "Not implemented: navigation" errors in test output
    consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    consoleErrorSpy.mockRestore()
  })

  describe('handleError', () => {
    describe('authentication errors', () => {
      it('should remove token from localStorage on auth error', () => {
        const { result } = renderHook(() => useErrorHandler())

        const authError = new ApiError({
          type: 'https://api.vibexp.io/errors/AUTH_REQUIRED',
          title: 'Authentication Required',
          status: 401,
          detail: 'Authentication token is invalid or expired',
          code: 'AUTH_REQUIRED',
          request_id: 'req-123',
          timestamp: new Date().toISOString(),
        })

        act(() => {
          result.current.handleError(authError)
        })

        expect(mockClearVibeXPData).toHaveBeenCalled()
      })

      it('should show alert on auth error', () => {
        const { result } = renderHook(() => useErrorHandler())

        const authError = new ApiError({
          type: 'https://api.vibexp.io/errors/AUTH_REQUIRED',
          title: 'Authentication Required',
          status: 401,
          detail: 'Authentication token is invalid or expired',
          code: 'AUTH_REQUIRED',
          request_id: 'req-123',
          timestamp: new Date().toISOString(),
        })

        act(() => {
          result.current.handleError(authError)
        })

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'Authentication required. Please log in again.',
          type: 'error',
        })
      })

      it('should handle invalid auth error', () => {
        const { result } = renderHook(() => useErrorHandler())

        const invalidAuthError = new ApiError({
          type: 'https://api.vibexp.io/errors/AUTH_INVALID',
          title: 'Invalid Authentication',
          status: 401,
          detail: 'Invalid credentials',
          code: 'AUTH_INVALID',
          request_id: 'req-123',
          timestamp: new Date().toISOString(),
        })

        // jsdom logs "Not implemented: navigation" error but doesn't throw
        act(() => {
          result.current.handleError(invalidAuthError)
        })

        expect(mockClearVibeXPData).toHaveBeenCalled()
      })

      it('should handle expired auth error', () => {
        const { result } = renderHook(() => useErrorHandler())

        const expiredAuthError = new ApiError({
          type: 'https://api.vibexp.io/errors/AUTH_EXPIRED',
          title: 'Expired Authentication',
          status: 401,
          detail: 'Authentication token has expired',
          code: 'AUTH_EXPIRED',
          request_id: 'req-123',
          timestamp: new Date().toISOString(),
        })

        // jsdom logs "Not implemented: navigation" error but doesn't throw
        act(() => {
          result.current.handleError(expiredAuthError)
        })

        expect(mockClearVibeXPData).toHaveBeenCalled()
      })
    })

    describe('validation errors', () => {
      it('should display first validation error message', () => {
        const { result } = renderHook(() => useErrorHandler())

        const validationError = new ApiError({
          type: 'https://api.vibexp.io/errors/VALIDATION_FAILED',
          title: 'Validation Failed',
          status: 400,
          detail: 'Validation failed',
          code: 'VALIDATION_FAILED',
          request_id: 'req-123',
          timestamp: new Date().toISOString(),
          validation_errors: [
            { field: 'name', message: 'Name is required', code: 'REQUIRED' },
            { field: 'email', message: 'Email is invalid', code: 'INVALID' },
          ],
        })

        act(() => {
          result.current.handleError(validationError)
        })

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'Please fix 2 validation errors',
          type: 'error',
        })
      })

      it('should handle validation error without errors array', () => {
        const { result } = renderHook(() => useErrorHandler())

        const validationError = new ApiError({
          type: 'https://api.vibexp.io/errors/VALIDATION_FAILED',
          title: 'Validation Failed',
          status: 400,
          detail: 'General validation error',
          code: 'VALIDATION_FAILED',
          request_id: 'req-123',
          timestamp: new Date().toISOString(),
        })

        act(() => {
          result.current.handleError(validationError)
        })

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'General validation error',
          type: 'error',
        })
      })
    })

    describe('resource limit errors', () => {
      it('should display resource limit error as warning', () => {
        const { result } = renderHook(() => useErrorHandler())

        const limitError = new ApiError({
          type: 'https://api.vibexp.io/errors/RESOURCE_LIMIT_EXCEEDED',
          title: 'Resource Limit Exceeded',
          status: 429,
          detail: 'You have reached the maximum number of prompts',
          code: 'RESOURCE_LIMIT_EXCEEDED',
          request_id: 'req-123',
          timestamp: new Date().toISOString(),
        })

        act(() => {
          result.current.handleError(limitError)
        })

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'You have reached the maximum number of prompts',
          type: 'warning',
        })
      })
    })

    describe('forbidden errors (#225)', () => {
      const forbiddenError = () =>
        new ApiError({
          type: 'https://api.vibexp.io/errors/FORBIDDEN',
          title: 'Forbidden',
          status: 403,
          detail:
            'You do not have permission to perform this action on this team',
          code: 'FORBIDDEN',
          request_id: 'req-403',
          timestamp: new Date().toISOString(),
        })

      it('should explain a permission failure as a warning, not a generic error', () => {
        const { result } = renderHook(() => useErrorHandler())

        act(() => {
          result.current.handleError(forbiddenError())
        })

        expect(mockShowAlert).toHaveBeenCalledWith({
          message:
            'You do not have permission to perform this action on this team',
          type: 'warning',
        })
      })

      it('should not log the user out — a forbidden caller is authenticated', () => {
        // Regression guard: routing 403 into the auth branch would wipe storage
        // and bounce the user to /login for merely lacking a permission.
        const { result } = renderHook(() => useErrorHandler())

        act(() => {
          result.current.handleError(forbiddenError())
        })

        expect(mockClearVibeXPData).not.toHaveBeenCalled()
      })
    })

    describe('general API errors', () => {
      it('should display general API error message', () => {
        const { result } = renderHook(() => useErrorHandler())

        const apiError = new ApiError({
          type: 'https://api.vibexp.io/errors/INTERNAL_ERROR',
          title: 'Internal Server Error',
          status: 500,
          detail: 'Something went wrong',
          code: 'INTERNAL_ERROR',
          request_id: 'req-123',
          timestamp: new Date().toISOString(),
        })

        act(() => {
          result.current.handleError(apiError)
        })

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'Something went wrong',
          type: 'error',
        })
      })
    })

    describe('network errors', () => {
      it('should handle timeout error', () => {
        const { result } = renderHook(() => useErrorHandler())

        const timeoutError = new Error('Request timeout')

        act(() => {
          result.current.handleError(timeoutError)
        })

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'Request timed out. Please try again.',
          type: 'error',
        })
      })

      it('should handle network error', () => {
        const { result } = renderHook(() => useErrorHandler())

        const networkError = new Error('Network error occurred')

        act(() => {
          result.current.handleError(networkError)
        })

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'Network error. Please check your connection.',
          type: 'error',
        })
      })

      it('should handle generic Error', () => {
        const { result } = renderHook(() => useErrorHandler())

        const genericError = new Error('Something failed')

        act(() => {
          result.current.handleError(genericError)
        })

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'Something failed',
          type: 'error',
        })
      })
    })

    describe('fallback error handling', () => {
      it('should use default message for unknown error', () => {
        const { result } = renderHook(() => useErrorHandler())

        act(() => {
          result.current.handleError('some unknown error')
        })

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'An unexpected error occurred',
          type: 'error',
        })
      })

      it('should use custom default message when provided', () => {
        const { result } = renderHook(() => useErrorHandler())

        act(() => {
          result.current.handleError('unknown', 'Custom error message')
        })

        expect(mockShowAlert).toHaveBeenCalledWith({
          message: 'Custom error message',
          type: 'error',
        })
      })
    })
  })

  describe('getFieldError', () => {
    it('should return error message for specific field', () => {
      const { result } = renderHook(() => useErrorHandler())

      const validationError = new ApiError({
        type: 'https://api.vibexp.io/errors/VALIDATION_FAILED',
        title: 'Validation Failed',
        status: 400,
        detail: 'Validation failed',
        code: 'VALIDATION_FAILED',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
        validation_errors: [
          { field: 'name', message: 'Name is required', code: 'REQUIRED' },
          { field: 'email', message: 'Email is invalid', code: 'INVALID' },
        ],
      })

      const fieldError = result.current.getFieldError(validationError, 'name')

      expect(fieldError).toBe('Name is required')
    })

    it('should return null for field without error', () => {
      const { result } = renderHook(() => useErrorHandler())

      const validationError = new ApiError({
        type: 'https://api.vibexp.io/errors/VALIDATION_FAILED',
        title: 'Validation Failed',
        status: 400,
        detail: 'Validation failed',
        code: 'VALIDATION_FAILED',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
        validation_errors: [
          { field: 'name', message: 'Name is required', code: 'REQUIRED' },
        ],
      })

      const fieldError = result.current.getFieldError(validationError, 'email')

      expect(fieldError).toBeNull()
    })

    it('should return null for non-ApiError', () => {
      const { result } = renderHook(() => useErrorHandler())

      const fieldError = result.current.getFieldError(
        new Error('Generic error'),
        'name'
      )

      expect(fieldError).toBeNull()
    })

    it('should return null for ApiError without validation errors', () => {
      const { result } = renderHook(() => useErrorHandler())

      const apiError = new ApiError({
        type: 'https://api.vibexp.io/errors/INTERNAL_ERROR',
        title: 'Internal Server Error',
        status: 500,
        detail: 'Something went wrong',
        code: 'INTERNAL_ERROR',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
      })

      const fieldError = result.current.getFieldError(apiError, 'name')

      expect(fieldError).toBeNull()
    })
  })

  describe('isFieldError', () => {
    it('should return true when field has error', () => {
      const { result } = renderHook(() => useErrorHandler())

      const validationError = new ApiError({
        type: 'https://api.vibexp.io/errors/VALIDATION_FAILED',
        title: 'Validation Failed',
        status: 400,
        detail: 'Validation failed',
        code: 'VALIDATION_FAILED',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
        validation_errors: [
          { field: 'name', message: 'Name is required', code: 'REQUIRED' },
        ],
      })

      const hasError = result.current.isFieldError(validationError, 'name')

      expect(hasError).toBe(true)
    })

    it('should return false when field does not have error', () => {
      const { result } = renderHook(() => useErrorHandler())

      const validationError = new ApiError({
        type: 'https://api.vibexp.io/errors/VALIDATION_FAILED',
        title: 'Validation Failed',
        status: 400,
        detail: 'Validation failed',
        code: 'VALIDATION_FAILED',
        request_id: 'req-123',
        timestamp: new Date().toISOString(),
        validation_errors: [
          { field: 'name', message: 'Name is required', code: 'REQUIRED' },
        ],
      })

      const hasError = result.current.isFieldError(validationError, 'email')

      expect(hasError).toBe(false)
    })

    it('should return false for non-ApiError', () => {
      const { result } = renderHook(() => useErrorHandler())

      const hasError = result.current.isFieldError(
        new Error('Generic error'),
        'name'
      )

      expect(hasError).toBe(false)
    })
  })
})
