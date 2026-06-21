import { ApiError, type APIErrorResponse } from '../../types/errors'
import { getErrorCode, getErrorMessage, isErrorCode } from '../errorHandling'

describe('errorHandling', () => {
  describe('getErrorMessage', () => {
    it('should extract message from ApiError using getMessage()', () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/bad-request',
        title: 'Bad Request',
        status: 400,
        detail: 'personal workspaces cannot have team subscriptions',
        code: 'BAD_REQUEST',
        request_id: 'req-123',
        timestamp: '2026-02-08T10:00:00Z',
      }
      const apiError = new ApiError(errorResponse)

      const message = getErrorMessage(apiError, 'Fallback message')

      expect(message).toBe('personal workspaces cannot have team subscriptions')
    })

    it('should extract validation errors from ApiError', () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/validation',
        title: 'Validation Failed',
        status: 422,
        detail: 'Validation failed',
        code: 'VALIDATION_FAILED',
        request_id: 'req-456',
        timestamp: '2026-02-08T10:00:00Z',
        validation_errors: [
          {
            field: 'email',
            message: 'Email is required',
            code: 'required',
          },
          {
            field: 'name',
            message: 'Name must be at least 3 characters',
            code: 'min_length',
          },
        ],
      }
      const apiError = new ApiError(errorResponse)

      const message = getErrorMessage(apiError, 'Fallback message')

      expect(message).toBe(
        'Email is required, Name must be at least 3 characters'
      )
    })

    it('should extract message from standard Error', () => {
      const error = new Error('Network timeout')

      const message = getErrorMessage(error, 'Fallback message')

      expect(message).toBe('Network timeout')
    })

    it('should use fallback message for unknown error type', () => {
      const error = { something: 'unexpected' }

      const message = getErrorMessage(error, 'Custom fallback')

      expect(message).toBe('Custom fallback')
    })

    it('should use default fallback when not provided', () => {
      const error = null

      const message = getErrorMessage(error)

      expect(message).toBe('An unexpected error occurred')
    })

    it('should prioritize ApiError over Error', () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/not-found',
        title: 'Not Found',
        status: 404,
        detail: 'Resource not found',
        code: 'RESOURCE_NOT_FOUND',
        request_id: 'req-789',
        timestamp: '2026-02-08T10:00:00Z',
      }
      const apiError = new ApiError(errorResponse)

      const message = getErrorMessage(apiError, 'Should not use this')

      expect(message).toBe('Resource not found')
    })

    it('should handle ApiError with only title and no detail', () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/generic',
        title: 'Internal Server Error',
        status: 500,
        detail: '',
        code: 'INTERNAL_ERROR',
        request_id: 'req-999',
        timestamp: '2026-02-08T10:00:00Z',
      }
      const apiError = new ApiError(errorResponse)

      const message = getErrorMessage(apiError, 'Fallback')

      expect(message).toBe('Internal Server Error')
    })
  })

  describe('getErrorCode', () => {
    it('should extract code from ApiError', () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/bad-request',
        title: 'Bad Request',
        status: 400,
        detail: 'Invalid request',
        code: 'BAD_REQUEST',
        request_id: 'req-123',
        timestamp: '2026-02-08T10:00:00Z',
      }
      const apiError = new ApiError(errorResponse)

      const code = getErrorCode(apiError)

      expect(code).toBe('BAD_REQUEST')
    })

    it('should return null for standard Error', () => {
      const error = new Error('Some error')

      const code = getErrorCode(error)

      expect(code).toBeNull()
    })

    it('should return null for unknown error type', () => {
      const error = { code: 'UNKNOWN' }

      const code = getErrorCode(error)

      expect(code).toBeNull()
    })

    it('should return null for null/undefined', () => {
      expect(getErrorCode(null)).toBeNull()
      expect(getErrorCode(undefined)).toBeNull()
    })
  })

  describe('isErrorCode', () => {
    it('should return true for matching ApiError code', () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/subscription',
        title: 'Subscription Required',
        status: 402,
        detail: 'This feature requires an active subscription',
        code: 'SUBSCRIPTION_REQUIRED',
        request_id: 'req-123',
        timestamp: '2026-02-08T10:00:00Z',
      }
      const apiError = new ApiError(errorResponse)

      const result = isErrorCode(apiError, 'SUBSCRIPTION_REQUIRED')

      expect(result).toBe(true)
    })

    it('should return false for non-matching ApiError code', () => {
      const errorResponse: APIErrorResponse = {
        type: 'https://api.vibexp.io/errors/bad-request',
        title: 'Bad Request',
        status: 400,
        detail: 'Invalid request',
        code: 'BAD_REQUEST',
        request_id: 'req-456',
        timestamp: '2026-02-08T10:00:00Z',
      }
      const apiError = new ApiError(errorResponse)

      const result = isErrorCode(apiError, 'NOT_FOUND')

      expect(result).toBe(false)
    })

    it('should return false for standard Error', () => {
      const error = new Error('Some error')

      const result = isErrorCode(error, 'BAD_REQUEST')

      expect(result).toBe(false)
    })

    it('should return false for unknown error type', () => {
      const error = { code: 'SOME_CODE' }

      const result = isErrorCode(error, 'SOME_CODE')

      expect(result).toBe(false)
    })

    it('should return false for null/undefined', () => {
      expect(isErrorCode(null, 'ANY_CODE')).toBe(false)
      expect(isErrorCode(undefined, 'ANY_CODE')).toBe(false)
    })
  })
})
